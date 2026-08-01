package scraper

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tobiasmarkusbraun/rgx-calc/internal/cache"
	"github.com/tobiasmarkusbraun/rgx-calc/internal/db"
)

// PlayerInfo, GameResult and TodayResult mirror the JSON shape returned by the
// GET /api/players/{id}/today endpoint and its job-status counterpart. They
// live here (rather than in internal/api) so ScrapePlayerToday can run outside
// an HTTP request, e.g. from the background job worker.
type PlayerInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ClubName    string `json:"club_name"`
	AvatarURL   string `json:"avatar_url"`
	ClubLogoURL string `json:"club_logo_url"`
	RGX         int    `json:"rgx"`
}

type TodayResult struct {
	Status     string       `json:"status"`
	Stale      bool         `json:"stale,omitempty"`
	Player     *PlayerInfo  `json:"player,omitempty"`
	Date       string       `json:"date,omitempty"`
	TotalDelta int          `json:"total_delta,omitempty"`
	HasProxy   bool         `json:"has_proxy,omitempty"`
	Games      []GameResult `json:"games,omitempty"`
}

// ScrapeDeps bundles the dependencies ScrapePlayerToday needs, shared between
// the HTTP handler (dev-mode short-circuits aside) and the background job
// worker (internal/jobqueue).
type ScrapeDeps struct {
	Fetcher  Fetcher
	Cache    *cache.Cache
	ProxyURL func(string) string
}

// ScrapePlayerToday runs the full scrape pipeline (player profile → today's
// team → schedule → division sidebar → division teams → RGX deltas) for one
// player, always attempting the scrape regardless of the current
// weekday/time window. Callers are responsible for the weekday/time-window
// guard and any result caching.
func ScrapePlayerToday(deps ScrapeDeps, player *db.Player, now time.Time) (TodayResult, error) {
	pInfo := &PlayerInfo{
		ID:          player.ID,
		Name:        player.Name,
		ClubName:    player.ClubName,
		AvatarURL:   deps.ProxyURL(player.AvatarURL),
		ClubLogoURL: deps.ProxyURL(player.ClubLogoURL),
		RGX:         player.RGX,
	}
	dateStr := now.Format("2006-01-02")

	numericID, err := NumericPlayerID(player.ID)
	if err != nil {
		return TodayResult{}, fmt.Errorf("resolve numeric player ID: %w", err)
	}

	entries, err := fetchProfileEntries(deps, numericID)
	if err != nil {
		return TodayResult{}, fmt.Errorf("fetch player profile: %w", err)
	}

	today := now.Format("02.01.06")
	var teamID int
	found := false
	for _, e := range entries {
		if e.Date == today {
			teamID = e.TournamentTeamID
			found = true
			break
		}
	}
	if !found {
		return TodayResult{Status: "no_tournament", Player: pInfo}, nil
	}

	schedule, sidebar, err := fetchTeamScheduleAndSidebar(deps, teamID)
	if err != nil {
		return TodayResult{}, fmt.Errorf("fetch team schedule: %w", err)
	}

	var teamsPath string
	for _, d := range sidebar {
		if strings.EqualFold(d.Name, schedule.DivisionName) {
			teamsPath = d.TeamsPath
			break
		}
	}
	if teamsPath == "" {
		return TodayResult{}, fmt.Errorf("division %q not found in schedule page sidebar", schedule.DivisionName)
	}

	divisionTeams, err := fetchDivisionTeamsByPath(deps, teamsPath)
	if err != nil {
		return TodayResult{}, fmt.Errorf("fetch division teams: %w", err)
	}

	var ourTeam TeamEntry
	for _, t := range divisionTeams {
		if t.TournamentTeamID == teamID {
			ourTeam = t
			break
		}
	}

	// Use RGX values from the schedule page (more current than weekly cache).
	// A woman's DB index RGX is always her Women RGX (see MergeWomenRGX), but she
	// may be playing an Open tournament today at a different, live Open RGX —
	// so ownRGX (this player's baseline for today) comes from the schedule page,
	// not the DB, falling back to the DB value only if no match is found there.
	r1 := 0
	hasProxy := false
	ownRGX := player.RGX
	for _, sp := range schedule.Players {
		spRGX := sp.RGX
		if spRGX <= 0 {
			spRGX = DivisionStartingRGX(schedule.DivisionName)
			hasProxy = true
		}
		r1 += spRGX
		if fuzzyPlayerNameMatch(sp.Name, player.Name) {
			ownRGX = spRGX
		}
	}
	if r1 == 0 {
		r1 = ourTeam.CombinedRGX
	}

	var games []GameResult
	totalDelta := 0

	for _, g := range schedule.Games {
		r2 := 0
		for _, t := range divisionTeams {
			if strings.EqualFold(t.Name, g.OpponentName) {
				r2 = t.CombinedRGX
				break
			}
		}
		opponentHasProxy := r2 == 0
		if opponentHasProxy {
			r2 = DivisionStartingRGX(schedule.DivisionName) * 2
			hasProxy = true
		}

		gr := GameResultFromCard(g, r1, r2, opponentHasProxy)
		totalDelta += gr.RGXDelta
		games = append(games, gr)
	}

	cappedRGX := DivisionCap(schedule.DivisionName, ownRGX+totalDelta)
	cappedDelta := cappedRGX - ownRGX
	pInfo.RGX = ownRGX

	return TodayResult{
		Status:     "ok",
		Player:     pInfo,
		Date:       dateStr,
		TotalDelta: cappedDelta,
		HasProxy:   hasProxy,
		Games:      games,
	}, nil
}

// --- playerzone fetchers (live or fixture-backed, via deps.Fetcher) ---

func fetchProfileEntries(deps ScrapeDeps, numericID string) ([]ProfileTournamentEntry, error) {
	cacheKey := "profile:" + numericID
	if cached, ok := deps.Cache.Get(cacheKey); ok {
		return cached.([]ProfileTournamentEntry), nil
	}

	body, err := deps.Fetcher.Fetch("/players/" + numericID)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	entries, err := ParseProfilePage(body)
	if err != nil {
		return nil, err
	}
	deps.Cache.Set(cacheKey, entries, cache.TTLPlayerProfile)
	return entries, nil
}

// fetchTeamScheduleAndSidebar fetches the team schedule page once and parses
// it twice — for the schedule itself and for the tournament's division
// sidebar rendered alongside it. The body is buffered so both parsers can
// read it independently.
func fetchTeamScheduleAndSidebar(deps ScrapeDeps, teamID int) (TeamScheduleResult, []Division, error) {
	body, err := deps.Fetcher.Fetch(fmt.Sprintf("/tournaments/schedule/team/%d", teamID))
	if err != nil {
		return TeamScheduleResult{}, nil, err
	}
	defer body.Close()

	raw, err := io.ReadAll(body)
	if err != nil {
		return TeamScheduleResult{}, nil, err
	}

	schedule, err := ParseTeamSchedule(bytes.NewReader(raw), teamID)
	if err != nil {
		return TeamScheduleResult{}, nil, err
	}
	sidebar, err := ParseDivisionSidebar(bytes.NewReader(raw))
	if err != nil {
		return TeamScheduleResult{}, nil, err
	}
	return schedule, sidebar, nil
}

func fetchDivisionTeamsByPath(deps ScrapeDeps, teamsPath string) ([]TeamEntry, error) {
	cacheKey := "division_teams:" + teamsPath
	if cached, ok := deps.Cache.Get(cacheKey); ok {
		return cached.([]TeamEntry), nil
	}

	body, err := deps.Fetcher.Fetch(teamsPath)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	teams, err := ParseDivisionTeams(body)
	if err != nil {
		return nil, err
	}
	deps.Cache.Set(cacheKey, teams, cache.TTLDivisionTeams)
	return teams, nil
}

// fuzzyPlayerNameMatch compares a name as rendered on a tournament page against
// the indexed player's name. Playerzone renders names inconsistently across
// page types (full name vs. first-name-only vs. nicknamed), so an exact
// case-insensitive match or a one-directional substring match both count.
func fuzzyPlayerNameMatch(pageName, playerName string) bool {
	return strings.EqualFold(pageName, playerName) ||
		strings.Contains(strings.ToLower(playerName), strings.ToLower(pageName))
}

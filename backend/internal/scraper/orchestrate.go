package scraper

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
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
	Status                  string       `json:"status"`
	Stale                   bool         `json:"stale,omitempty"`
	Player                  *PlayerInfo  `json:"player,omitempty"`
	Date                    string       `json:"date,omitempty"`
	TotalDelta              int          `json:"total_delta,omitempty"`
	TotalPerformanceDelta   int          `json:"total_performance_delta,omitempty"`
	HasProxy                bool         `json:"has_proxy,omitempty"`
	Games                   []GameResult `json:"games,omitempty"`
}

// ScrapeDeps bundles the dependencies ScrapePlayerToday needs, shared between
// the HTTP handler (dev-mode short-circuits aside) and the background job
// worker (internal/jobqueue).
type ScrapeDeps struct {
	Fetcher     Fetcher
	Cache       *cache.Cache
	ProxyURL    func(string) string
	Tournaments *db.TournamentStore
}

// ScrapePlayerToday runs the full scrape pipeline (player profile → today's
// team → schedule → division sidebar → GRT classification → division teams →
// RGX deltas) for one player, always attempting the scrape regardless of the
// current weekday/time window. Callers are responsible for the weekday/time-window
// guard and any result caching. A tournament that isn't part of the German
// Roundnet Tour (e.g. EURA-rated ones) never produces RGX changes and is
// reported the same as no tournament today — see resolveTournamentGRT.
//
// The second return value is the numeric playerzone tournament ID the result
// was computed from (0 if the player has no tournament today) — used by the
// job worker to record usage statistics, not part of the JSON API response.
func ScrapePlayerToday(deps ScrapeDeps, player *db.Player, now time.Time) (TodayResult, int, error) {
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
		return TodayResult{}, 0, fmt.Errorf("resolve numeric player ID: %w", err)
	}

	entries, err := fetchProfileEntries(deps, numericID)
	if err != nil {
		return TodayResult{}, 0, fmt.Errorf("fetch player profile: %w", err)
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
		return TodayResult{Status: "no_tournament", Player: pInfo}, 0, nil
	}

	schedule, sidebar, tournamentID, err := fetchTeamSchedulePage(deps, teamID)
	if err != nil {
		return TodayResult{}, 0, fmt.Errorf("fetch team schedule: %w", err)
	}

	isGRT, err := resolveTournamentGRT(deps, tournamentID, now)
	if err != nil {
		return TodayResult{}, 0, fmt.Errorf("resolve tournament GRT status: %w", err)
	}
	if !isGRT {
		// Non-GRT tournaments (e.g. EURA-rated ones) never produce RGX changes —
		// treated exactly like no tournament today. Resolved before the
		// division team list is needed, so a non-GRT tournament degrades
		// gracefully even if that lookup would fail.
		return TodayResult{Status: "no_tournament", Player: pInfo}, tournamentID, nil
	}

	// The division team list exists to price opponents. With no games played yet there are
	// no opponents to price, so the result is reported without it rather than failing over
	// a page we don't need — this is the "registered today, nothing played yet" state, and
	// before a tournament starts its schedule page doesn't link a division at all.
	var divisionTeams []TeamEntry
	if len(schedule.Games) > 0 {
		teamsPath, err := resolveDivisionTeamsPath(deps, sidebar, schedule.DivisionName, tournamentID)
		if err != nil {
			return TodayResult{}, 0, err
		}
		divisionTeams, err = fetchDivisionTeamsByPath(deps, teamsPath)
		if err != nil {
			return TodayResult{}, 0, fmt.Errorf("fetch division teams: %w", err)
		}
	}

	day := ComputeTeamDayResult(schedule, divisionTeams, teamID)

	// Use RGX values from the schedule page (more current than weekly cache).
	// A woman's DB index RGX is always her Women RGX (see MergeWomenRGX), but she
	// may be playing an Open tournament today at a different, live Open RGX —
	// so ownRGX (this player's baseline for today) comes from the schedule page,
	// not the DB, falling back to the DB value only if no match is found there.
	ownRGX := player.RGX
	for _, sp := range schedule.Players {
		spRGX := sp.RGX
		if spRGX <= 0 {
			spRGX = DivisionStartingRGX(schedule.DivisionName)
		}
		if fuzzyPlayerNameMatch(sp.Name, player.Name) {
			ownRGX = spRGX
		}
	}
	pInfo.RGX = ownRGX

	return TodayResult{
		Status:                "ok",
		Player:                pInfo,
		Date:                  dateStr,
		TotalDelta:            day.TotalDelta,
		TotalPerformanceDelta: day.TotalPerformanceDelta,
		HasProxy:              day.HasProxy,
		Games:                 day.Games,
	}, tournamentID, nil
}

// TeamDayResult is everything the RGX pipeline derives for one team's tournament day: the
// team's combined pre-tournament RGX baseline and a delta for each of its games. It is
// player-independent — both players on the team share these numbers.
type TeamDayResult struct {
	TeamRGX                 int          `json:"team_rgx"` // r1 in the RGX formula
	HasProxy                bool         `json:"has_proxy"`
	TotalDelta              int          `json:"total_delta"`
	TotalPerformanceDelta   int          `json:"total_performance_delta"`
	Games                   []GameResult `json:"games"`
}

// ComputeTeamDayResult turns a parsed team schedule plus its division's team list into
// per-game RGX deltas. It holds the whole r1/r2 resolution policy — schedule-page player
// RGX preferred over the division team list, division starting values proxied in for
// players and opponents with no RGX yet — so anything that wants a team's deltas (the
// per-player API pipeline above, the whole-tournament capture in cmd/fixturecapture) gets
// identical numbers rather than a second, drifting implementation.
func ComputeTeamDayResult(schedule TeamScheduleResult, divisionTeams []TeamEntry, ourTeamID int) TeamDayResult {
	var ourTeam TeamEntry
	for _, t := range divisionTeams {
		if t.TournamentTeamID == ourTeamID {
			ourTeam = t
			break
		}
	}

	r1 := 0
	hasProxy := false
	for _, sp := range schedule.Players {
		spRGX := sp.RGX
		if spRGX <= 0 {
			spRGX = DivisionStartingRGX(schedule.DivisionName)
			hasProxy = true
		}
		r1 += spRGX
	}
	if r1 == 0 {
		r1 = ourTeam.CombinedRGX
	}

	var games []GameResult
	totalDelta := 0
	totalPerformanceDelta := 0

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
		gr.OurTeamName = schedule.TeamName
		totalDelta += gr.RGXDelta
		totalPerformanceDelta += gr.PerformanceDelta
		games = append(games, gr)
	}

	return TeamDayResult{
		TeamRGX:               r1,
		HasProxy:              hasProxy,
		TotalDelta:            totalDelta,
		TotalPerformanceDelta: totalPerformanceDelta,
		Games:                 games,
	}
}

// --- playerzone fetchers (live or fixture-backed, via deps.Fetcher) ---

func fetchProfileEntries(deps ScrapeDeps, numericID string) ([]ProfileTournamentEntry, error) {
	cacheKey := "profile:" + numericID
	if cached, ok := deps.Cache.Get(cacheKey); ok {
		if entries, ok := cached.([]ProfileTournamentEntry); ok {
			return entries, nil
		}
		// Wrong type under this key should never happen (this cache is only ever
		// populated a few lines below), but a bad assertion must not panic —
		// treat it as a cache miss and re-fetch rather than crash the caller.
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
	var profileTTL time.Duration
	if len(entries) >= 1 {
		profileTTL = cache.TTLPlayerProfileWithTournament
	} else {
		profileTTL = cache.TTLPlayerProfile
	}
	deps.Cache.Set(cacheKey, entries, profileTTL)
	return entries, nil
}

// fetchTeamSchedulePage fetches the team schedule page once and reads everything the
// pipeline needs off it: the schedule itself, the divisions it links, and the ID of the
// tournament it belongs to. The body is buffered so each parser can read it independently.
//
// Only the schedule is required. A page that links no division is still usable — before a
// tournament starts it links none, and once it has been played it links only the team's own
// one — so the division list is best-effort, and the tournament ID falls back to the
// tournament-scoped links every such page carries.
func fetchTeamSchedulePage(deps ScrapeDeps, teamID int) (TeamScheduleResult, []Division, int, error) {
	body, err := deps.Fetcher.Fetch(fmt.Sprintf("/tournaments/schedule/team/%d", teamID))
	if err != nil {
		return TeamScheduleResult{}, nil, 0, err
	}
	defer body.Close()

	raw, err := io.ReadAll(body)
	if err != nil {
		return TeamScheduleResult{}, nil, 0, err
	}

	schedule, err := ParseTeamSchedule(bytes.NewReader(raw), teamID)
	if err != nil {
		return TeamScheduleResult{}, nil, 0, err
	}

	// A sidebar parse failure is not fatal here: it only means this page doesn't link any
	// division, which the caller handles.
	sidebar, _ := ParseDivisionSidebar(bytes.NewReader(raw))

	// Every division link carries the tournament ID in its own URL, so prefer it; otherwise
	// read it off any other tournament-scoped link on the page.
	for _, d := range sidebar {
		if id, err := strconv.Atoi(d.TournamentID); err == nil && id != 0 {
			return schedule, sidebar, id, nil
		}
	}
	tournamentID, err := ParseTournamentID(bytes.NewReader(raw))
	if err != nil {
		return TeamScheduleResult{}, nil, 0, fmt.Errorf("resolve tournament ID from schedule page: %w", err)
	}
	return schedule, sidebar, tournamentID, nil
}

// resolveDivisionTeamsPath picks the path to our division's team list. The schedule page
// normally links it — either among every division of the tournament, or as the single
// division the team plays in — and the tournament's own detail page is consulted only if it
// doesn't.
func resolveDivisionTeamsPath(deps ScrapeDeps, sidebar []Division, divisionName string, tournamentID int) (string, error) {
	if path := matchDivision(sidebar, divisionName); path != "" {
		return path, nil
	}

	divisions, err := fetchTournamentDivisions(deps, tournamentID)
	if err != nil {
		return "", fmt.Errorf("division %q not linked from the schedule page, and listing the tournament's divisions failed: %w",
			divisionName, err)
	}
	if path := matchDivision(divisions, divisionName); path != "" {
		return path, nil
	}
	return "", fmt.Errorf("division %q not found among the tournament's %d divisions", divisionName, len(divisions))
}

// matchDivision finds a division by name, falling back to the only division there is: a
// schedule page of a played tournament links exactly one division — the team's own — and
// names it after the navigation tab rather than the division, so a name match can't be
// required in that case.
func matchDivision(divisions []Division, name string) string {
	for _, d := range divisions {
		if strings.EqualFold(d.Name, name) {
			return d.TeamsPath
		}
	}
	if len(divisions) == 1 {
		return divisions[0].TeamsPath
	}
	return ""
}

// fetchTournamentDivisions lists a tournament's divisions from its detail page, cached for
// as long as a division team list is, since the two are fetched together.
func fetchTournamentDivisions(deps ScrapeDeps, tournamentID int) ([]Division, error) {
	cacheKey := fmt.Sprintf("tournament_divisions:%d", tournamentID)
	if cached, ok := deps.Cache.Get(cacheKey); ok {
		if divisions, ok := cached.([]Division); ok {
			return divisions, nil
		}
		// See fetchProfileEntries: a mismatched type is treated as a cache miss rather
		// than being allowed to panic.
	}

	body, err := deps.Fetcher.Fetch(fmt.Sprintf("/tournaments/%d", tournamentID))
	if err != nil {
		return nil, err
	}
	defer body.Close()

	divisions, err := ParseDivisionSidebar(body)
	if err != nil {
		return nil, err
	}
	deps.Cache.Set(cacheKey, divisions, cache.TTLDivisionTeamsSuccess)
	return divisions, nil
}

// resolveTournamentGRT reports whether tournamentID is a German Roundnet Tour
// tournament, consulting the permanent tournaments table first and falling back to
// scraping the tournament's own detail page only the first time it's ever seen — GRT
// status is assumed fixed for good once a tournament has started, so nothing here
// needs a TTL or re-fetch path.
func resolveTournamentGRT(deps ScrapeDeps, tournamentID int, now time.Time) (bool, error) {
	existing, err := deps.Tournaments.Get(tournamentID)
	if err != nil {
		return false, fmt.Errorf("load tournament %d: %w", tournamentID, err)
	}
	if existing != nil {
		return existing.IsGRT, nil
	}

	body, err := deps.Fetcher.Fetch(fmt.Sprintf("/tournaments/%d", tournamentID))
	if err != nil {
		return false, fmt.Errorf("fetch tournament %d: %w", tournamentID, err)
	}
	defer body.Close()

	detail, err := ParseTournamentDetail(body)
	if err != nil {
		return false, fmt.Errorf("parse tournament %d: %w", tournamentID, err)
	}

	if err := deps.Tournaments.Create(db.Tournament{
		ID:        tournamentID,
		Name:      detail.Name,
		IsGRT:     detail.IsGRT,
		Date:      detail.Date,
		ScrapedAt: now,
	}); err != nil {
		return false, fmt.Errorf("save tournament %d: %w", tournamentID, err)
	}

	return detail.IsGRT, nil
}

func fetchDivisionTeamsByPath(deps ScrapeDeps, teamsPath string) ([]TeamEntry, error) {
	cacheKey := "division_teams:" + teamsPath
	if cached, ok := deps.Cache.Get(cacheKey); ok {
		if teams, ok := cached.([]TeamEntry); ok {
			return teams, nil
		}
		// See the matching comment in fetchProfileEntries: treat a mismatched
		// type as a cache miss rather than letting the assertion panic.
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
	var teamsTTL time.Duration
	if len(teams) >= 1 {
		teamsTTL = cache.TTLDivisionTeamsSuccess
	} else {
		teamsTTL = cache.TTLDivisionTeams
	}
	deps.Cache.Set(cacheKey, teams, teamsTTL)
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

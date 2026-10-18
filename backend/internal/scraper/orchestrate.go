package scraper

import (
	"fmt"
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
	Status                string       `json:"status"`
	Competition           string       `json:"competition,omitempty"` // CompetitionLeague, or "" for a tournament
	Stale                 bool         `json:"stale,omitempty"`
	Player                *PlayerInfo  `json:"player,omitempty"`
	Date                  string       `json:"date,omitempty"`
	TotalDelta            int          `json:"total_delta,omitempty"`
	TotalPerformanceDelta int          `json:"total_performance_delta,omitempty"`
	HasProxy              bool         `json:"has_proxy,omitempty"`
	Games                 []GameResult `json:"games,omitempty"`
	// Division is only for the job worker's log line, not part of the API.
	Division string `json:"-"`
}

// ScrapeDeps bundles the dependencies ScrapePlayerToday needs, shared between
// the HTTP handler (dev-mode short-circuits aside) and the background job
// worker (internal/jobqueue).
type ScrapeDeps struct {
	Fetcher     Fetcher
	Cache       *cache.Cache
	ProxyURL    func(string) string
	Tournaments *db.TournamentStore
	League      *db.LeagueStore
}

// competitionDayRollover is how long after midnight the previous day's competition still counts
// as today, so results stay reachable through a long tournament or matchday evening.
const competitionDayRollover = 2 * time.Hour

// CompetitionDay returns a time on the calendar day whose competition is "today" at t: until
// 02:00 that is the previous day. Only its date (in t's location) is meaningful.
func CompetitionDay(t time.Time) time.Time {
	return t.Add(-competitionDayRollover)
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
	competitionDay := CompetitionDay(now)
	dateStr := competitionDay.Format("2006-01-02")

	numericID, err := NumericPlayerID(player.ID)
	if err != nil {
		return TodayResult{}, 0, fmt.Errorf("resolve numeric player ID: %w", err)
	}

	entries, err := fetchProfileEntries(deps, numericID)
	if err != nil {
		return TodayResult{}, 0, fmt.Errorf("fetch player profile: %w", err)
	}

	today := competitionDay.Format("02.01.06")
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
		Division:              schedule.DivisionName,
	}, tournamentID, nil
}

// TeamDayResult is everything the RGX pipeline derives for one team's tournament day: the
// team's combined pre-tournament RGX baseline and a delta for each of its games. It is
// player-independent — both players on the team share these numbers.
type TeamDayResult struct {
	TeamRGX               int          `json:"team_rgx"` // r1 in the RGX formula
	HasProxy              bool         `json:"has_proxy"`
	TotalDelta            int          `json:"total_delta"`
	TotalPerformanceDelta int          `json:"total_performance_delta"`
	Games                 []GameResult `json:"games"`
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

// fuzzyPlayerNameMatch compares a name as rendered on a tournament page against
// the indexed player's name. Playerzone renders names inconsistently across
// page types (full name vs. first-name-only vs. nicknamed), so an exact
// case-insensitive match or a one-directional substring match both count.
func fuzzyPlayerNameMatch(pageName, playerName string) bool {
	return strings.EqualFold(pageName, playerName) ||
		strings.Contains(strings.ToLower(playerName), strings.ToLower(pageName))
}

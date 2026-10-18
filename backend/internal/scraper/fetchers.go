package scraper

// Playerzone fetchers, live or fixture-backed via deps.Fetcher, each behind
// deps.Cache where the page changes slowly.

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/metrics"
)

// cacheLookup looks key up in deps.Cache and counts it as a hit or miss of the
// named cache. A value of the wrong type should never be stored, but a bad
// assertion must not panic — it's treated as a miss, so the caller re-fetches.
func cacheLookup[T any](deps ScrapeDeps, name, key string) (T, bool) {
	v, _ := deps.Cache.Get(key)
	t, ok := v.(T)
	result := "miss"
	if ok {
		result = "hit"
	}
	metrics.ScraperCache.WithLabelValues(name, result).Inc()
	return t, ok
}

func fetchProfileEntries(deps ScrapeDeps, numericID string) ([]ProfileTournamentEntry, error) {
	cacheKey := "profile:" + numericID
	if entries, ok := cacheLookup[[]ProfileTournamentEntry](deps, "profile", cacheKey); ok {
		return entries, nil
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
	if divisions, ok := cacheLookup[[]Division](deps, "divisions", cacheKey); ok {
		return divisions, nil
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
	if teams, ok := cacheLookup[[]TeamEntry](deps, "division_teams", cacheKey); ok {
		return teams, nil
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

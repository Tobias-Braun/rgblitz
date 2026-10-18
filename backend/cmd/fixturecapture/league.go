package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// captureLeague snapshots one league team's matchday per team registration ID. Every roster
// player runs through the real ScrapePlayerLeagueToday, so a snapshot holds exactly the pages
// production fetches for that team: its schedule page, the matchday group page of each
// Begegnung, and for today's Begegnungen their Partien and the opponents' schedule pages.
// Returns the number of teams that failed.
func captureLeague(run runContext, teamArgs []string) int {
	league := db.NewLeagueStore(run.conn)
	failures := 0
	for _, arg := range teamArgs {
		if !captureLeagueSnapshot(run, league, arg) {
			failures++
		}
	}
	return failures
}

// captureLeagueSnapshot captures and saves one team's snapshot, reporting whether it succeeded.
func captureLeagueSnapshot(run runContext, league *db.LeagueStore, arg string) bool {
	teamID, err := strconv.Atoi(arg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  skipping %q: --league takes numeric team IDs (the {id} of /tournaments/schedule/team/{id})\n", arg)
		return false
	}
	snapDir, err := run.snapshotDir(fmt.Sprintf("league-%d", teamID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "  skipping team %d: %v\n", teamID, err)
		return false
	}
	run.rec.startSnapshot(snapDir, uniquePageName)
	fmt.Printf("capturing league team %d → %s\n", teamID, snapDir)

	snapshot := captureLeagueTeam(run, league, teamID)
	snapshot.Fetched = run.rec.records()
	snapshot.Parsed = reparseLeague(snapDir, snapshot.Fetched, teamID)

	if !saveSnapshot(run, snapDir, nil, snapshot, leagueReport(snapshot)) {
		return false
	}
	fmt.Printf("  %s\n", summarizeLeague(snapshot))
	return snapshot.FatalError == "" && snapshot.failedPlayers() == 0
}

func captureLeagueTeam(run runContext, league *db.LeagueStore, teamID int) leagueCapture {
	snapshot := leagueCapture{
		CapturedAt: run.now.Format(time.RFC3339),
		Date:       run.now.Format("2006-01-02"),
		Weekday:    run.now.Format("Monday"),
		TeamID:     teamID,
	}

	roster, seasonID, err := fetchLeagueTeam(run.rec, teamID)
	if err != nil {
		snapshot.FatalError = err.Error()
		return snapshot
	}
	snapshot.SeasonID = seasonID
	if err := indexRoster(league, seasonID, teamID, roster.Players); err != nil {
		snapshot.FatalError = err.Error()
		return snapshot
	}

	// One cache for the whole team: production shares schedule pages, matchday groups and
	// Partien between every player of a team too, so this fetches what one team costs it.
	deps := scraper.ScrapeDeps{
		Fetcher:  run.rec,
		Cache:    cache.New(),
		ProxyURL: func(s string) string { return s },
		League:   league,
	}
	for _, p := range roster.Players {
		snapshot.Players = append(snapshot.Players, captureLeaguePlayer(deps, p, run.now))
	}
	return snapshot
}

// fetchLeagueTeam reads a team's roster and its season's tournament ID off its schedule page.
func fetchLeagueTeam(f scraper.Fetcher, teamID int) (scraper.TeamScheduleResult, int, error) {
	raw, err := fetchBytes(f, fmt.Sprintf("/tournaments/schedule/team/%d", teamID))
	if err != nil {
		return scraper.TeamScheduleResult{}, 0, fmt.Errorf("fetch team schedule: %w", err)
	}
	roster, err := scraper.ParseTeamSchedule(bytes.NewReader(raw), teamID)
	if err != nil {
		return roster, 0, fmt.Errorf("parse team schedule: %w", err)
	}
	fixtures, err := scraper.ParseLeagueFixtures(bytes.NewReader(raw), teamID)
	if err != nil {
		return roster, 0, fmt.Errorf("parse league fixtures: %w", err)
	}
	return roster, seasonIDFromPath(fixtures[0].MatchdayPath), nil
}

// indexRoster fills the roster index the pipeline finds a player's team in with exactly this
// roster, instead of indexing the whole season.
func indexRoster(league *db.LeagueStore, seasonID, teamID int, players []scraper.TeamPlayer) error {
	var members []db.LeagueMembership
	for _, p := range players {
		if p.NumericID != "" {
			members = append(members, db.LeagueMembership{PlayerID: p.NumericID, TeamID: teamID})
		}
	}
	if err := league.SaveSeason(seasonID, members); err != nil {
		return fmt.Errorf("fill roster index: %w", err)
	}
	return nil
}

func captureLeaguePlayer(deps scraper.ScrapeDeps, p scraper.TeamPlayer, now time.Time) leaguePlayerCapture {
	pc := leaguePlayerCapture{Name: p.Name, NumericID: p.NumericID, RosterRGX: p.RGX}
	if p.NumericID == "" {
		pc.Error = "roster card links no profile — the pipeline can't find this player"
		return pc
	}
	player := &db.Player{ID: p.NumericID, Name: p.Name, RGX: p.RGX}
	result, _, handled, err := scraper.ScrapePlayerLeagueToday(deps, player, now)
	pc.Handled = handled
	switch {
	case err != nil:
		pc.Error = err.Error()
		fmt.Fprintf(os.Stderr, "  %s: pipeline error: %v\n", p.Name, err)
	case handled:
		pc.Result = &result
	}
	return pc
}

var reSeasonPath = regexp.MustCompile(`^/tournaments/(\d+)-`)

// seasonIDFromPath reads the season's tournament ID off a matchday tag's link,
// e.g. /tournaments/793-deutsche-roundnet-liga/schedule/… → 793.
func seasonIDFromPath(path string) int {
	id, _ := pathID(reSeasonPath, path)
	return id
}

// pathID reads the numeric ID re captures from path.
func pathID(re *regexp.Regexp, path string) (int, bool) {
	m := re.FindStringSubmatch(path)
	if len(m) < 2 {
		return 0, false
	}
	id, err := strconv.Atoi(m[1])
	return id, err == nil
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
		teamID, err := strconv.Atoi(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skipping %q: --league takes numeric team IDs (the {id} of /tournaments/schedule/team/{id})\n", arg)
			failures++
			continue
		}
		snapDir, err := run.snapshotDir(fmt.Sprintf("league-%d", teamID))
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skipping team %d: %v\n", teamID, err)
			failures++
			continue
		}
		run.rec.startSnapshot(snapDir, uniquePageName)
		fmt.Printf("capturing league team %d → %s\n", teamID, snapDir)

		snapshot := captureLeagueTeam(run, league, teamID)
		snapshot.Fetched = run.rec.records()
		snapshot.Parsed = reparseLeague(snapDir, snapshot.Fetched, teamID)

		if err := run.rec.writeManifest(snapDir, nil); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: write manifest: %v\n", err)
		}
		if err := writeLeagueCapture(snapDir, snapshot); err != nil {
			fmt.Fprintf(os.Stderr, "  write capture: %v\n", err)
			failures++
			continue
		}
		fmt.Printf("  %s\n", summarizeLeague(snapshot))
		if snapshot.FatalError != "" || snapshot.failedPlayers() > 0 {
			failures++
		}
	}
	return failures
}

func captureLeagueTeam(run runContext, league *db.LeagueStore, teamID int) leagueCapture {
	snapshot := leagueCapture{
		CapturedAt: run.now.Format(time.RFC3339),
		Date:       run.now.Format("2006-01-02"),
		Weekday:    run.now.Format("Monday"),
		TeamID:     teamID,
	}

	raw, err := fetchBytes(run.rec, fmt.Sprintf("/tournaments/schedule/team/%d", teamID))
	if err != nil {
		snapshot.FatalError = fmt.Sprintf("fetch team schedule: %v", err)
		return snapshot
	}
	roster, err := scraper.ParseTeamSchedule(bytes.NewReader(raw), teamID)
	if err != nil {
		snapshot.FatalError = fmt.Sprintf("parse team schedule: %v", err)
		return snapshot
	}
	fixtures, err := scraper.ParseLeagueFixtures(bytes.NewReader(raw), teamID)
	if err != nil {
		snapshot.FatalError = fmt.Sprintf("parse league fixtures: %v", err)
		return snapshot
	}
	snapshot.SeasonID = seasonIDFromPath(fixtures[0].MatchdayPath)

	// The pipeline finds a player's team in the roster index, so the capture fills it with
	// exactly this roster instead of indexing the whole season.
	var members []db.LeagueMembership
	for _, p := range roster.Players {
		if p.NumericID != "" {
			members = append(members, db.LeagueMembership{PlayerID: p.NumericID, TeamID: teamID})
		}
	}
	if err := league.SaveSeason(snapshot.SeasonID, members); err != nil {
		snapshot.FatalError = fmt.Sprintf("fill roster index: %v", err)
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
		pc := leaguePlayerCapture{Name: p.Name, NumericID: p.NumericID, RosterRGX: p.RGX}
		if p.NumericID == "" {
			pc.Error = "roster card links no profile — the pipeline can't find this player"
			snapshot.Players = append(snapshot.Players, pc)
			continue
		}
		player := &db.Player{ID: p.NumericID, Name: p.Name, RGX: p.RGX}
		result, _, handled, err := scraper.ScrapePlayerLeagueToday(deps, player, run.now)
		pc.Handled = handled
		if err != nil {
			pc.Error = err.Error()
			fmt.Fprintf(os.Stderr, "  %s: pipeline error: %v\n", p.Name, err)
		} else if handled {
			pc.Result = &result
		}
		snapshot.Players = append(snapshot.Players, pc)
	}
	return snapshot
}

var reSeasonPath = regexp.MustCompile(`^/tournaments/(\d+)-`)

// seasonIDFromPath reads the season's tournament ID off a matchday tag's link,
// e.g. /tournaments/793-deutsche-roundnet-liga/schedule/… → 793.
func seasonIDFromPath(path string) int {
	m := reSeasonPath.FindStringSubmatch(path)
	if len(m) < 2 {
		return 0
	}
	id, _ := strconv.Atoi(m[1])
	return id
}

// --- capture record ---

type leagueCapture struct {
	CapturedAt string                `json:"captured_at"`
	Date       string                `json:"date"`
	Weekday    string                `json:"weekday"`
	TeamID     int                   `json:"team_id"`
	SeasonID   int                   `json:"season_id,omitempty"`
	FatalError string                `json:"fatal_error,omitempty"`
	Players    []leaguePlayerCapture `json:"players,omitempty"`
	Fetched    []fetchRecord         `json:"fetched"`
	Parsed     leagueParsed          `json:"parsed"`
}

func (c leagueCapture) failedPlayers() int {
	n := 0
	for _, p := range c.Players {
		if p.Error != "" && p.NumericID != "" {
			n++
		}
	}
	return n
}

// leaguePlayerCapture is one roster player's pipeline outcome. RosterRGX is the RGX the schedule
// page shows: the pre-matchday baseline while a matchday runs, the recalculated value after it.
type leaguePlayerCapture struct {
	Name      string               `json:"name"`
	NumericID string               `json:"numeric_id,omitempty"`
	RosterRGX int                  `json:"roster_rgx"`
	Handled   bool                 `json:"handled"`
	Error     string               `json:"error,omitempty"`
	Result    *scraper.TodayResult `json:"result,omitempty"`
}

// perPlayerDelta is the player's own RGX change: each Partie moves both players of a pair by
// half the pair's delta.
func (p leaguePlayerCapture) perPlayerDelta() float64 {
	if p.Result == nil {
		return 0
	}
	sum := 0.0
	for _, g := range p.Result.Games {
		sum += float64(g.RGXDeltaPerPlayer)
	}
	return sum
}

// leagueParsed holds the intermediates re-derived from the saved pages, which also verifies
// that the snapshot replays to what the live run saw.
type leagueParsed struct {
	Rosters        map[int]scraper.TeamScheduleResult `json:"rosters,omitempty"`
	Fixtures       []scraper.LeagueFixture            `json:"fixtures,omitempty"`
	MatchdayGroups map[int]scraper.MatchdayGroup      `json:"matchday_groups,omitempty"`
	Partien        map[int][]scraper.Partie           `json:"partien,omitempty"`
	Errors         map[string]string                  `json:"errors,omitempty"`
}

var (
	reMatchdayGroupParam = regexp.MustCompile(`[?&]md_group=(\d+)`)
	reSubgamesPath       = regexp.MustCompile(`^/ajax/get-subgames/(\d+)$`)
)

// reparseLeague runs each league parser over the pages the capture fetched, keyed by the path
// they were fetched from. A page that fails to parse is recorded, not fatal.
func reparseLeague(dir string, records []fetchRecord, ourTeamID int) leagueParsed {
	p := leagueParsed{
		Rosters:        map[int]scraper.TeamScheduleResult{},
		MatchdayGroups: map[int]scraper.MatchdayGroup{},
		Partien:        map[int][]scraper.Partie{},
		Errors:         map[string]string{},
	}
	for _, rec := range records {
		if rec.Error != "" || rec.File == "" || rec.Reused {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, rec.File))
		if err != nil {
			p.Errors[rec.File] = err.Error()
			continue
		}
		switch {
		case reTeamSchedulePath.MatchString(rec.Path):
			teamID, _ := strconv.Atoi(reTeamSchedulePath.FindStringSubmatch(rec.Path)[1])
			roster, err := scraper.ParseTeamSchedule(bytes.NewReader(raw), teamID)
			if err != nil {
				p.Errors[rec.File] = err.Error()
				continue
			}
			p.Rosters[teamID] = roster
			if teamID == ourTeamID {
				p.Fixtures, err = scraper.ParseLeagueFixtures(bytes.NewReader(raw), teamID)
				if err != nil {
					p.Errors[rec.File+":fixtures"] = err.Error()
				}
			}
		case reMatchdayGroupParam.MatchString(rec.Path):
			groupID, _ := strconv.Atoi(reMatchdayGroupParam.FindStringSubmatch(rec.Path)[1])
			group, err := scraper.ParseMatchdayGroup(bytes.NewReader(raw), groupID)
			if err != nil {
				p.Errors[rec.File] = err.Error()
				continue
			}
			p.MatchdayGroups[groupID] = group
		case reSubgamesPath.MatchString(rec.Path):
			gameID, _ := strconv.Atoi(reSubgamesPath.FindStringSubmatch(rec.Path)[1])
			partien, err := scraper.ParseSubgames(bytes.NewReader(raw))
			if err != nil {
				p.Errors[rec.File] = err.Error()
				continue
			}
			p.Partien[gameID] = partien
		}
	}
	if len(p.Errors) == 0 {
		p.Errors = nil
	}
	return p
}

func writeLeagueCapture(dir string, c leagueCapture) error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "capture.json"), raw, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.txt"), []byte(leagueReport(c)), 0o644)
}

func leagueReport(c leagueCapture) string {
	var b strings.Builder
	fmt.Fprintf(&b, "captured %s (%s)\nleague team %d, season %d\n", c.CapturedAt, c.Weekday, c.TeamID, c.SeasonID)
	if c.FatalError != "" {
		fmt.Fprintf(&b, "FATAL: %s\n", c.FatalError)
	}

	b.WriteString("\n--- fetched pages ---\n")
	writeFetchedPages(&b, c.Fetched)

	p := c.Parsed
	if len(p.Errors) > 0 {
		b.WriteString("\n--- parse errors ---\n")
		for file, msg := range p.Errors {
			fmt.Fprintf(&b, "  %s: %s\n", file, msg)
		}
	}

	if len(p.Fixtures) > 0 {
		b.WriteString("\n--- Begegnungen ---\n")
		for _, f := range p.Fixtures {
			group := p.MatchdayGroups[f.MatchdayGroupID]
			marker := " "
			if group.Date == c.Date {
				marker = "*"
			}
			fmt.Fprintf(&b, " %s game %-7d %-22s %-10s %-14s vs %-28s team_id=%-6d we_are_team_one=%t\n",
				marker, f.GameID, f.Matchday, group.Date, group.City, f.OpponentName, f.OpponentTeamID, f.WeAreTeamOne)
		}
	}

	if len(p.Partien) > 0 {
		b.WriteString("\n--- Partien (Begegnungen today) ---\n")
		for _, gameID := range sortedKeys(p.Partien) {
			fmt.Fprintf(&b, "  game %d\n", gameID)
			for _, partie := range p.Partien[gameID] {
				fmt.Fprintf(&b, "    %-8s %-32s vs %-32s sets=%s\n", partie.Label,
					strings.Join(partie.Pairs[0], "/"), strings.Join(partie.Pairs[1], "/"), formatSetScores(partie.SetScores))
			}
		}
	}

	for _, teamID := range sortedKeys(p.Rosters) {
		roster := p.Rosters[teamID]
		fmt.Fprintf(&b, "\n--- roster %d %q (division %q) ---\n", teamID, roster.TeamName, roster.DivisionName)
		for _, pl := range roster.Players {
			fmt.Fprintf(&b, "  %-28s id=%-6s rgx=%d\n", pl.Name, pl.NumericID, pl.RGX)
		}
	}

	b.WriteString("\n--- per player (roster RGX is the pre-matchday baseline only while the matchday runs) ---\n")
	for _, pl := range c.Players {
		switch {
		case pl.Error != "":
			fmt.Fprintf(&b, "  %-28s rgx=%-5d ERROR: %s\n", pl.Name, pl.RosterRGX, pl.Error)
		case !pl.Handled:
			fmt.Fprintf(&b, "  %-28s rgx=%-5d not handled (team doesn't play today)\n", pl.Name, pl.RosterRGX)
		default:
			delta := pl.perPlayerDelta()
			fmt.Fprintf(&b, "  %-28s rgx=%-5d partien=%d pair_delta_sum=%+d player_delta=%+.1f → %.1f\n",
				pl.Name, pl.RosterRGX, len(pl.Result.Games), pl.Result.TotalDelta, delta, float64(pl.RosterRGX)+delta)
			writeGameLines(&b, pl.Result.Games)
		}
	}
	return b.String()
}

func summarizeLeague(c leagueCapture) string {
	if c.FatalError != "" {
		return "error: " + c.FatalError
	}
	played := 0
	for _, p := range c.Players {
		if p.Result != nil && len(p.Result.Games) > 0 {
			played++
		}
	}
	return fmt.Sprintf("%d roster players, %d played today, %d pipeline errors, %d parse errors",
		len(c.Players), played, c.failedPlayers(), len(c.Parsed.Errors))
}

func sortedKeys[V any](m map[int]V) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// tournamentOptions are the tournament-mode knobs from the command line.
type tournamentOptions struct {
	explicitIDs   []string // capture these tournament IDs instead of discovering today's
	divisionsLike []string // only capture divisions whose name contains one of these
	includeNonGRT bool
	maxRequests   int
}

// captureTournaments snapshots every tournament running today in full: the detail page, each
// division's team list, and each team's schedule, with RGX deltas computed per team through
// the same scraper.ComputeTeamDayResult the API pipeline uses. Returns the number of
// tournaments that failed.
func captureTournaments(run runContext, opts tournamentOptions) int {
	candidates, sharedRecords, err := discoverTournaments(run, opts)
	if err != nil {
		fatalf("%v", err)
	}
	if len(candidates) == 0 {
		fmt.Printf("no tournament is running on %s — nothing to capture\n", run.now.Format("2006-01-02 (Monday)"))
		return 0
	}

	fmt.Printf("candidate tournaments for today: %s\n", describeCandidates(candidates))

	failures := 0
	for _, entry := range candidates {
		if run.rec.requestCount() >= opts.maxRequests {
			fmt.Fprintf(os.Stderr, "  request budget of %d reached — skipping tournament %d (%s)\n",
				opts.maxRequests, entry.ID, entry.Name)
			failures++
			continue
		}

		snapDir, err := run.snapshotDir(fmt.Sprintf("t%d", entry.ID))
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skipping tournament %d: %v\n", entry.ID, err)
			failures++
			continue
		}
		sharedPaths, err := linkShared(run.sharedDir, snapDir, sharedRecords)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: tournament %d: link shared pages: %v\n", entry.ID, err)
		}
		run.rec.startSnapshot(snapDir, uniquePageName)

		snapshot := captureOneTournament(run, opts, entry, snapDir)
		snapshot.Fetched = run.rec.records()

		if err := run.rec.writeManifest(snapDir, sharedPaths); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: write manifest: %v\n", err)
		}
		if err := writeTournamentCapture(snapDir, snapshot); err != nil {
			fmt.Fprintf(os.Stderr, "  write capture: %v\n", err)
			failures++
			continue
		}
		fmt.Printf("  %s\n", summarizeTournament(snapshot))
		if snapshot.FatalError != "" {
			failures++
		}
	}

	return failures
}

// discoverTournaments resolves which tournaments this run should capture: the ones the
// site's own listings say could be running today, or exactly those named by --tournaments.
// The second return value is the shared pages fetched during discovery, to be linked into
// every snapshot.
func discoverTournaments(run runContext, opts tournamentOptions) ([]scraper.TournamentListEntry, []fetchRecord, error) {
	if len(opts.explicitIDs) > 0 {
		var entries []scraper.TournamentListEntry
		for _, raw := range opts.explicitIDs {
			id, err := strconv.Atoi(raw)
			if err != nil {
				return nil, nil, fmt.Errorf("--tournaments takes numeric IDs, got %q", raw)
			}
			entries = append(entries, scraper.TournamentListEntry{ID: id, Name: "(explicitly requested)"})
		}
		return entries, nil, nil
	}

	run.rec.startSnapshot(run.sharedDir, uniquePageName)
	fmt.Println("discovering tournaments running today ...")

	body, err := run.rec.Fetch(scraper.TournamentsUpcomingPath)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch tournament list: %w", err)
	}
	defer body.Close()

	all, err := scraper.ParseTournamentList(body)
	if err != nil {
		return nil, nil, fmt.Errorf("parse tournament list: %w", err)
	}

	var candidates []scraper.TournamentListEntry
	for _, e := range all {
		if e.MightRunOn(run.now) {
			candidates = append(candidates, e)
		}
	}
	fmt.Printf("  %d tournaments listed, %d could be running today\n", len(all), len(candidates))
	return candidates, run.rec.records(), nil
}

// captureOneTournament walks a single tournament: detail page → divisions → teams →
// schedules, computing each team's day result. Every failure is recorded in the snapshot
// rather than aborting, so a capture of a partly-broken tournament is still worth reading.
func captureOneTournament(run runContext, opts tournamentOptions, entry scraper.TournamentListEntry, snapDir string) tournamentCapture {
	snapshot := tournamentCapture{
		CapturedAt: run.now.Format(time.RFC3339),
		Date:       run.now.Format("2006-01-02"),
		Weekday:    run.now.Format("Monday"),
		Listed:     entry,
	}

	fmt.Printf("capturing tournament %d (%s) → %s\n", entry.ID, entry.Name, snapDir)

	detailPath := fmt.Sprintf("/tournaments/%d", entry.ID)
	raw, err := fetchBytes(run.rec, detailPath)
	if err != nil {
		snapshot.FatalError = fmt.Sprintf("fetch tournament detail: %v", err)
		return snapshot
	}

	detail, err := scraper.ParseTournamentDetail(bytes.NewReader(raw))
	if err != nil {
		snapshot.FatalError = fmt.Sprintf("parse tournament detail: %v", err)
		return snapshot
	}
	snapshot.Detail = &detail

	// The detail page is the authority on the date range: the listing only ever gives the
	// start day for a tournament that has already begun (see ParseTournamentList).
	today := run.now.Format("2006-01-02")
	if detail.Date != "" && detail.EndDate != "" && (today < detail.Date || today > detail.EndDate) {
		snapshot.Skipped = fmt.Sprintf("runs %s..%s, not today (%s)", detail.Date, detail.EndDate, today)
		fmt.Printf("  skipped: %s\n", snapshot.Skipped)
		return snapshot
	}
	if !detail.IsGRT && !opts.includeNonGRT {
		snapshot.Skipped = "not a German Roundnet Tour tournament (the backend reports these as no_tournament); pass --include-non-grt to capture anyway"
		fmt.Printf("  skipped: %s\n", snapshot.Skipped)
		return snapshot
	}

	// The tournament detail page renders the same division sidebar as a team schedule page,
	// so the divisions come from the page already fetched — no extra request.
	divisions, err := scraper.ParseDivisionSidebar(bytes.NewReader(raw))
	if err != nil {
		snapshot.FatalError = fmt.Sprintf("parse division sidebar: %v", err)
		return snapshot
	}

	for _, div := range divisions {
		if !divisionWanted(div.Name, opts.divisionsLike) {
			continue
		}
		if run.rec.requestCount() >= opts.maxRequests {
			snapshot.Truncated = true
			break
		}
		snapshot.Divisions = append(snapshot.Divisions, captureDivision(run, opts, div, &snapshot))
	}

	snapshot.Checks = runConsistencyChecks(snapshot.Divisions)
	return snapshot
}

// captureDivision fetches one division's team list and then every team's schedule.
func captureDivision(run runContext, opts tournamentOptions, div scraper.Division, snapshot *tournamentCapture) divisionCapture {
	dc := divisionCapture{Name: div.Name, ID: div.ID, TeamsPath: div.TeamsPath}

	raw, err := fetchBytes(run.rec, div.TeamsPath)
	if err != nil {
		dc.Error = fmt.Sprintf("fetch division teams: %v", err)
		return dc
	}
	teams, err := scraper.ParseDivisionTeams(bytes.NewReader(raw))
	if err != nil {
		dc.Error = fmt.Sprintf("parse division teams: %v", err)
		return dc
	}
	dc.Teams = teams
	fmt.Printf("  division %-22s %d teams\n", div.Name, len(teams))

	for _, team := range teams {
		if run.rec.requestCount() >= opts.maxRequests {
			snapshot.Truncated = true
			break
		}
		dc.TeamDays = append(dc.TeamDays, captureTeam(run, team, teams))
	}
	return dc
}

// captureTeam fetches one team's schedule and computes its day result through the exact
// function the API pipeline calls, so a bug in that logic shows up here for every team.
func captureTeam(run runContext, team scraper.TeamEntry, divisionTeams []scraper.TeamEntry) teamDayCapture {
	tc := teamDayCapture{TeamID: team.TournamentTeamID, TeamName: team.Name, CombinedRGX: team.CombinedRGX}

	path := fmt.Sprintf("/tournaments/schedule/team/%d", team.TournamentTeamID)
	raw, err := fetchBytes(run.rec, path)
	if err != nil {
		tc.Error = fmt.Sprintf("fetch team schedule: %v", err)
		return tc
	}
	schedule, err := scraper.ParseTeamSchedule(bytes.NewReader(raw), team.TournamentTeamID)
	if err != nil {
		tc.Error = fmt.Sprintf("parse team schedule: %v", err)
		return tc
	}
	tc.Schedule = &schedule

	result := scraper.ComputeTeamDayResult(schedule, divisionTeams, team.TournamentTeamID)
	tc.Result = &result

	// ComputeTeamDayResult walks schedule.Games in order, so the i-th result belongs to the
	// i-th card and carries its game ID — which is what lets the consistency checks pair a
	// game up with the same game as seen from the opponent's schedule. Guarded because a
	// length mismatch would mean that assumption no longer holds.
	if len(schedule.Games) == len(result.Games) {
		tc.GameIDs = make([]int, len(schedule.Games))
		for i, g := range schedule.Games {
			tc.GameIDs[i] = g.GameID
		}
	} else {
		tc.Error = fmt.Sprintf("schedule has %d game cards but %d results were computed",
			len(schedule.Games), len(result.Games))
	}
	return tc
}

func divisionWanted(name string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if strings.Contains(strings.ToLower(name), strings.ToLower(f)) {
			return true
		}
	}
	return false
}

// fetchBytes reads a page fully into memory so it can be handed to more than one parser
// (the tournament detail page feeds both the detail and the division-sidebar parser).
func fetchBytes(f scraper.Fetcher, path string) ([]byte, error) {
	body, err := f.Fetch(path)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}

// --- capture record ---

type tournamentCapture struct {
	CapturedAt string                      `json:"captured_at"`
	Date       string                      `json:"date"`
	Weekday    string                      `json:"weekday"`
	Listed     scraper.TournamentListEntry `json:"listed"`
	Detail     *scraper.TournamentDetail   `json:"detail,omitempty"`
	Skipped    string                      `json:"skipped,omitempty"`
	FatalError string                      `json:"fatal_error,omitempty"`
	Truncated  bool                        `json:"truncated,omitempty"`
	Divisions  []divisionCapture           `json:"divisions,omitempty"`
	Checks     consistencyChecks           `json:"checks"`
	Fetched    []fetchRecord               `json:"fetched,omitempty"`
}

type divisionCapture struct {
	Name      string              `json:"name"`
	ID        string              `json:"id"`
	TeamsPath string              `json:"teams_path"`
	Error     string              `json:"error,omitempty"`
	Teams     []scraper.TeamEntry `json:"teams,omitempty"`
	TeamDays  []teamDayCapture    `json:"team_days,omitempty"`
}

type teamDayCapture struct {
	TeamID      int                         `json:"team_id"`
	TeamName    string                      `json:"team_name"`
	CombinedRGX int                         `json:"combined_rgx_from_team_list"`
	Error       string                      `json:"error,omitempty"`
	Schedule    *scraper.TeamScheduleResult `json:"schedule,omitempty"`
	Result      *scraper.TeamDayResult      `json:"result,omitempty"`
	GameIDs     []int                       `json:"game_ids,omitempty"` // parallel to Result.Games
}

// --- consistency checks ---

// consistencyChecks are the cross-checks a whole-tournament capture makes possible: with
// both sides of every game in hand, the backend's own numbers can be audited against each
// other without knowing the "right" answer. Each finding is a concrete lead for a wrong
// result on screen.
type consistencyChecks struct {
	// DivisionDeltaSums is the sum of every team's total delta per division, in capture
	// order. RGX is zero-sum per game (x2 = -x1), so a division should land on ~0; anything
	// else means deltas aren't mirroring — the most common symptom of an r1/r2 mix-up.
	DivisionDeltaSums []divisionDeltaSum `json:"division_delta_sums"`
	// MirrorMismatches are games whose two sides disagree about what happened.
	MirrorMismatches []mirrorMismatch `json:"mirror_mismatches,omitempty"`
	// UnmatchedOpponents are games whose opponent wasn't found in the division team list, so
	// the opponent's RGX fell back to a division proxy instead of their real combined RGX.
	UnmatchedOpponents []string `json:"unmatched_opponents,omitempty"`
	// ProxiedTeams are teams whose own r1 needed a proxy value (a player with no RGX yet).
	ProxiedTeams []string `json:"proxied_teams,omitempty"`
}

// divisionDeltaSum is one division's zero-sum check result.
type divisionDeltaSum struct {
	Division string `json:"division"`
	Sum      int    `json:"sum"`
}

type mirrorMismatch struct {
	Division string `json:"division"`
	GameID   int    `json:"game_id"`
	TeamA    string `json:"team_a"`
	TeamB    string `json:"team_b"`
	Reason   string `json:"reason"`
	DetailA  string `json:"detail_a"`
	DetailB  string `json:"detail_b"`
}

// gameSide is one team's view of one game, used to pair the two sides up by game ID.
type gameSide struct {
	teamName string
	result   scraper.GameResult
}

func runConsistencyChecks(divisions []divisionCapture) consistencyChecks {
	var checks consistencyChecks

	for _, div := range divisions {
		sum := 0
		sides := map[int][]gameSide{}

		for _, team := range div.TeamDays {
			if team.Result == nil {
				continue
			}
			sum += team.Result.TotalDelta
			if team.Result.HasProxy {
				checks.ProxiedTeams = append(checks.ProxiedTeams,
					fmt.Sprintf("%s / %s", div.Name, team.TeamName))
			}
			for i, g := range team.Result.Games {
				if g.HasProxy {
					checks.UnmatchedOpponents = append(checks.UnmatchedOpponents,
						fmt.Sprintf("%s / %s vs %q not in the division team list", div.Name, team.TeamName, g.OpponentName))
				}
				if i < len(team.GameIDs) {
					id := team.GameIDs[i]
					sides[id] = append(sides[id], gameSide{teamName: team.TeamName, result: g})
				}
			}
		}
		checks.DivisionDeltaSums = append(checks.DivisionDeltaSums,
			divisionDeltaSum{Division: div.Name, Sum: sum})

		for id, pair := range sides {
			if len(pair) != 2 {
				continue // only one side captured (bye, cross-division game, failed fetch)
			}
			a, b := pair[0], pair[1]
			if m := compareSides(div.Name, id, a, b); m != nil {
				checks.MirrorMismatches = append(checks.MirrorMismatches, *m)
			}
		}
	}

	return checks
}

// compareSides checks that two teams' views of the same game agree: mirrored set scores, and
// deltas that cancel out. A disagreement is a parsing or RGX-input bug, not a data quirk —
// the same game row on playerzone produced both.
func compareSides(division string, gameID int, a, b gameSide) *mirrorMismatch {
	mismatch := func(reason string) *mirrorMismatch {
		return &mirrorMismatch{
			Division: division,
			GameID:   gameID,
			TeamA:    a.teamName,
			TeamB:    b.teamName,
			Reason:   reason,
			DetailA: fmt.Sprintf("%s vs %s sets=%s delta=%+d r1=%d r2=%d",
				a.result.Round, a.result.OpponentName, formatSetResults(a.result.Sets),
				a.result.RGXDelta, a.result.TeamRGX, a.result.OpponentRGX),
			DetailB: fmt.Sprintf("%s vs %s sets=%s delta=%+d r1=%d r2=%d",
				b.result.Round, b.result.OpponentName, formatSetResults(b.result.Sets),
				b.result.RGXDelta, b.result.TeamRGX, b.result.OpponentRGX),
		}
	}

	if len(a.result.Sets) != len(b.result.Sets) {
		return mismatch("the two sides report a different number of sets")
	}
	for i := range a.result.Sets {
		if a.result.Sets[i].OurScore != b.result.Sets[i].OpponentScore ||
			a.result.Sets[i].OpponentScore != b.result.Sets[i].OurScore {
			return mismatch("set scores aren't mirrored — one side's game card was oriented wrongly")
		}
	}

	// Each side computes its own delta from its own r1 and the opponent's r2; if both sides
	// read the same two numbers, the deltas must cancel out exactly.
	if a.result.TeamRGX != b.result.OpponentRGX || b.result.TeamRGX != a.result.OpponentRGX {
		return mismatch("the sides disagree on the teams' combined RGX (schedule page vs. division team list)")
	}
	if a.result.RGXDelta != -b.result.RGXDelta {
		return mismatch("deltas don't cancel out although both sides used the same RGX inputs")
	}
	return nil
}

// --- output ---

func writeTournamentCapture(dir string, c tournamentCapture) error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "capture.json"), raw, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.txt"), []byte(tournamentReport(c)), 0o644)
}

func tournamentReport(c tournamentCapture) string {
	var b strings.Builder
	fmt.Fprintf(&b, "captured %s (%s)\n", c.CapturedAt, c.Weekday)
	fmt.Fprintf(&b, "tournament: %d %s\n", c.Listed.ID, c.Listed.Name)
	if c.Detail != nil {
		fmt.Fprintf(&b, "detail:     %s, runs %s..%s, grt=%t\n",
			c.Detail.Name, c.Detail.Date, c.Detail.EndDate, c.Detail.IsGRT)
	}
	if c.Skipped != "" {
		fmt.Fprintf(&b, "skipped:    %s\n", c.Skipped)
	}
	if c.FatalError != "" {
		fmt.Fprintf(&b, "FATAL:      %s\n", c.FatalError)
	}
	if c.Truncated {
		fmt.Fprintf(&b, "TRUNCATED:  the request budget ran out before every team was captured\n")
	}

	for _, div := range c.Divisions {
		fmt.Fprintf(&b, "\n=== division %s (id=%s) ===\n", div.Name, div.ID)
		if div.Error != "" {
			fmt.Fprintf(&b, "  ERROR: %s\n", div.Error)
			continue
		}
		b.WriteString("\n--- teams (combined RGX from the division team list) ---\n")
		writeDivisionTeams(&b, div.Teams)

		for _, team := range div.TeamDays {
			fmt.Fprintf(&b, "\n--- %s (team_id=%d) ---\n", team.TeamName, team.TeamID)
			if team.Error != "" {
				fmt.Fprintf(&b, "  ERROR: %s\n", team.Error)
			}
			if team.Schedule != nil {
				writeSchedulePlayers(&b, *team.Schedule)
			}
			if team.Result != nil {
				writeGameLines(&b, team.Result.Games)
				fmt.Fprintf(&b, "  total delta: %+d (has_proxy=%t)\n", team.Result.TotalDelta, team.Result.HasProxy)
			}
		}
	}

	b.WriteString("\n=== consistency checks ===\n")
	writeChecks(&b, c.Checks)

	b.WriteString("\n--- fetched pages ---\n")
	writeFetchedPages(&b, c.Fetched)

	return b.String()
}

// writeChecks renders the cross-checks, leading with the ones that indicate a real bug.
func writeChecks(b *strings.Builder, checks consistencyChecks) {
	if len(checks.DivisionDeltaSums) == 0 {
		b.WriteString("  (no divisions captured)\n")
		return
	}

	b.WriteString("  delta sum per division (RGX is zero-sum per game, so ~0 is expected):\n")
	for _, d := range checks.DivisionDeltaSums {
		flag := ""
		if d.Sum != 0 {
			flag = "   <-- does not cancel out"
		}
		fmt.Fprintf(b, "    %-24s %+d%s\n", d.Division, d.Sum, flag)
	}

	if len(checks.MirrorMismatches) > 0 {
		fmt.Fprintf(b, "\n  %d game(s) where the two sides disagree:\n", len(checks.MirrorMismatches))
		for i, m := range checks.MirrorMismatches {
			if i == maxReportLines {
				fmt.Fprintf(b, "    ... and %d more (all of them are in capture.json)\n",
					len(checks.MirrorMismatches)-maxReportLines)
				break
			}
			fmt.Fprintf(b, "    [%s] game %d: %s\n", m.Division, m.GameID, m.Reason)
			fmt.Fprintf(b, "      %-30s %s\n", m.TeamA, m.DetailA)
			fmt.Fprintf(b, "      %-30s %s\n", m.TeamB, m.DetailB)
		}
	} else {
		b.WriteString("\n  every game's two sides agree on sets, RGX inputs and deltas\n")
	}

	writeCappedList(b, checks.UnmatchedOpponents,
		"game(s) whose opponent wasn't found in the division team list (r2 fell back to a proxy)")
	writeCappedList(b, checks.ProxiedTeams,
		"team(s) whose own r1 used a division starting value for a player with no RGX")
}

// maxReportLines caps how many findings of one kind report.txt spells out. A tournament-wide
// capture can produce hundreds of identical findings; the full list always stays in
// capture.json, and the report exists to be read.
const maxReportLines = 20

func writeCappedList(b *strings.Builder, items []string, heading string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n  %d %s:\n", len(items), heading)
	for i, s := range items {
		if i == maxReportLines {
			fmt.Fprintf(b, "    ... and %d more (all of them are in capture.json)\n", len(items)-maxReportLines)
			break
		}
		fmt.Fprintf(b, "    %s\n", s)
	}
}

func summarizeTournament(c tournamentCapture) string {
	if c.FatalError != "" {
		return "error: " + c.FatalError
	}
	if c.Skipped != "" {
		return "skipped: " + c.Skipped
	}
	teams, games := 0, 0
	for _, div := range c.Divisions {
		for _, t := range div.TeamDays {
			teams++
			if t.Result != nil {
				games += len(t.Result.Games)
			}
		}
	}
	summary := fmt.Sprintf("captured %d divisions, %d teams, %d game results",
		len(c.Divisions), teams, games)
	if n := len(c.Checks.MirrorMismatches); n > 0 {
		summary += fmt.Sprintf(" — %d MISMATCHED game(s), see report.txt", n)
	}
	return summary
}

func describeCandidates(entries []scraper.TournamentListEntry) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, fmt.Sprintf("%d (%s)", e.ID, e.Name))
	}
	return strings.Join(parts, ", ")
}

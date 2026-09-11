package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// capturePlayers snapshots exactly what each named player's /today request fetches, by
// running the real ScrapePlayerToday pipeline. Returns the number of players that failed.
func capturePlayers(run runContext, queries []string) int {
	// The rankings are the player index the pipeline resolves names against; fetched once
	// for the run and linked into every snapshot so each stays self-contained.
	run.rec.startSnapshot(run.sharedDir, fixturePageName)
	fmt.Println("fetching the player index (Open + Women rankings) ...")
	open, err := scraper.FetchRankingPage(run.rec, scraper.RankingOpenPath)
	if err != nil {
		fatalf("fetch open ranking page: %v", err)
	}
	women, err := scraper.FetchRankingPage(run.rec, scraper.RankingWomenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: fetch women ranking page failed, indexing Open RGX only: %v\n", err)
		women = nil
	}
	index := scraper.MergeWomenRGX(open, women)
	fmt.Printf("  indexed %d players (%d with a Women RGX override)\n", len(index), len(women))
	sharedRecords := run.rec.records()

	tournaments := db.NewTournamentStore(run.conn)
	failures := 0

	for _, query := range queries {
		player, err := findPlayer(index, query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skipping %q: %v\n", query, err)
			failures++
			continue
		}

		snapDir, err := run.snapshotDir(player.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skipping %s: %v\n", player.Name, err)
			failures++
			continue
		}
		sharedPaths, err := linkShared(run.sharedDir, snapDir, sharedRecords)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: %s: link shared pages: %v\n", player.Name, err)
		}

		run.rec.startSnapshot(snapDir, fixturePageName)

		fmt.Printf("capturing %s (id=%s, index rgx=%d) → %s\n", player.Name, player.ID, player.RGX, snapDir)
		// Each player gets a fresh cache so nothing is answered from a previous player's
		// fetch — a capture must contain every page the pipeline used, not a subset.
		deps := scraper.ScrapeDeps{
			Fetcher:     run.rec,
			Cache:       cache.New(),
			ProxyURL:    func(s string) string { return s },
			Tournaments: tournaments,
		}

		snapshot := playerCapture{
			CapturedAt:    run.now.Format(time.RFC3339),
			Date:          run.now.Format("2006-01-02"),
			Weekday:       run.now.Format("Monday"),
			Query:         query,
			IndexedPlayer: player,
		}

		result, tournamentID, err := scraper.ScrapePlayerToday(deps, player, run.now)
		if err != nil {
			snapshot.PipelineError = err.Error()
			fmt.Fprintf(os.Stderr, "  pipeline error: %v\n", err)
			failures++
		} else {
			snapshot.Result = &result
			snapshot.TournamentID = tournamentID
		}
		snapshot.Fetched = run.rec.records()
		snapshot.Parsed = reparse(snapDir)

		if err := run.rec.writeManifest(snapDir, sharedPaths); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: write manifest: %v\n", err)
		}
		if err := writePlayerCapture(snapDir, snapshot); err != nil {
			fmt.Fprintf(os.Stderr, "  write capture: %v\n", err)
			failures++
			continue
		}
		fmt.Printf("  %s\n", summarizePlayer(snapshot))
	}

	return failures
}

// playerCapture is the full record of one player's snapshot, serialized to capture.json.
type playerCapture struct {
	CapturedAt    string               `json:"captured_at"`
	Date          string               `json:"date"`
	Weekday       string               `json:"weekday"`
	Query         string               `json:"query"`
	IndexedPlayer *db.Player           `json:"indexed_player"`
	TournamentID  int                  `json:"tournament_id,omitempty"`
	PipelineError string               `json:"pipeline_error,omitempty"`
	Result        *scraper.TodayResult `json:"result,omitempty"`
	Fetched       []fetchRecord        `json:"fetched"`
	Parsed        parsed               `json:"parsed"`
}

// parsed holds the intermediates re-derived from the saved HTML. Re-parsing the files on
// disk (rather than reusing the pipeline's in-memory values) also verifies that the
// snapshot really does replay to the same numbers the live run produced.
type parsed struct {
	ProfileEntries   []scraper.ProfileTournamentEntry `json:"profile_entries,omitempty"`
	Schedule         *scraper.TeamScheduleResult      `json:"schedule,omitempty"`
	DivisionSidebar  []scraper.Division               `json:"division_sidebar,omitempty"`
	DivisionTeams    []scraper.TeamEntry              `json:"division_teams,omitempty"`
	TournamentDetail *scraper.TournamentDetail        `json:"tournament_detail,omitempty"`
	Errors           map[string]string                `json:"errors,omitempty"`
}

// reparse runs each parser over the captured HTML, recording per-file errors instead of
// failing the capture — a page that fails to parse is exactly what this tool is meant to
// preserve for inspection.
func reparse(dir string) parsed {
	p := parsed{Errors: map[string]string{}}

	withFile(dir, "player-profile.html", p.Errors, func(r io.Reader) error {
		entries, err := scraper.ParseProfilePage(r)
		p.ProfileEntries = entries
		return err
	})

	// ParseTeamSchedule needs our team ID to tell which side of each game card we are on;
	// it comes from today's profile entry, the same way the pipeline resolves it.
	ourTeamID := 0
	for _, e := range p.ProfileEntries {
		if e.Date == todayProfileFormat(dir) {
			ourTeamID = e.TournamentTeamID
			break
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "team-schedule.html"))
	switch {
	case errors.Is(err, os.ErrNotExist):
		// No schedule page fetched (no tournament today) — nothing to parse.
	case err != nil:
		p.Errors["team-schedule.html"] = err.Error()
	default:
		schedule, err := scraper.ParseTeamSchedule(bytes.NewReader(raw), ourTeamID)
		if err != nil {
			p.Errors["team-schedule.html"] = err.Error()
		} else {
			p.Schedule = &schedule
		}
		sidebar, err := scraper.ParseDivisionSidebar(bytes.NewReader(raw))
		if err != nil {
			p.Errors["team-schedule.html:sidebar"] = err.Error()
		} else {
			p.DivisionSidebar = sidebar
		}
	}

	withFile(dir, "division-teams.html", p.Errors, func(r io.Reader) error {
		teams, err := scraper.ParseDivisionTeams(r)
		p.DivisionTeams = teams
		return err
	})

	withFile(dir, "tournament-detail.html", p.Errors, func(r io.Reader) error {
		detail, err := scraper.ParseTournamentDetail(r)
		if err == nil {
			p.TournamentDetail = &detail
		}
		return err
	})

	if len(p.Errors) == 0 {
		p.Errors = nil
	}
	return p
}

// todayProfileFormat derives the DD.MM.YY date the profile page uses from the snapshot's
// parent directory name (YYYY-MM-DD), so re-parsing matches the same tournament entry the
// live run did even when a capture is re-processed on a later day.
func todayProfileFormat(dir string) string {
	day := filepath.Base(filepath.Dir(dir))
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return time.Now().Format("02.01.06")
	}
	return t.Format("02.01.06")
}

// withFile opens a captured page and hands it to parse, recording a parse failure under
// the file's name. A file that was never fetched is not an error.
func withFile(dir, name string, errs map[string]string, parse func(io.Reader) error) {
	f, err := os.Open(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		errs[name] = err.Error()
		return
	}
	defer f.Close()
	if err := parse(f); err != nil {
		errs[name] = err.Error()
	}
}

func writePlayerCapture(dir string, c playerCapture) error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "capture.json"), raw, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.txt"), []byte(playerReport(c)), 0o644)
}

// playerReport renders a player capture as the human-readable debugging view: what was
// fetched, what each parser made of it, and every intermediate of the RGX formula per game.
func playerReport(c playerCapture) string {
	var b strings.Builder
	fmt.Fprintf(&b, "captured %s (%s)\n", c.CapturedAt, c.Weekday)
	if c.IndexedPlayer != nil {
		fmt.Fprintf(&b, "player:   %s (id=%s, club=%s, index rgx=%d)\n",
			c.IndexedPlayer.Name, c.IndexedPlayer.ID, c.IndexedPlayer.ClubName, c.IndexedPlayer.RGX)
	}
	if c.TournamentID != 0 {
		fmt.Fprintf(&b, "tournament id: %d\n", c.TournamentID)
	}
	if c.PipelineError != "" {
		fmt.Fprintf(&b, "PIPELINE ERROR: %s\n", c.PipelineError)
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

	if len(p.ProfileEntries) > 0 {
		b.WriteString("\n--- profile tournament entries ---\n")
		for _, e := range p.ProfileEntries {
			fmt.Fprintf(&b, "  %s  team_id=%d\n", e.Date, e.TournamentTeamID)
		}
	}

	if p.TournamentDetail != nil {
		fmt.Fprintf(&b, "\n--- tournament detail ---\n  %s (%s..%s) grt=%t\n",
			p.TournamentDetail.Name, p.TournamentDetail.Date, p.TournamentDetail.EndDate, p.TournamentDetail.IsGRT)
	}

	if len(p.DivisionSidebar) > 0 {
		b.WriteString("\n--- division sidebar ---\n")
		for _, d := range p.DivisionSidebar {
			fmt.Fprintf(&b, "  %-24s id=%-6s tournament=%-6s %s\n", d.Name, d.ID, d.TournamentID, d.TeamsPath)
		}
	}

	if p.Schedule != nil {
		fmt.Fprintf(&b, "\n--- team schedule (division %q, team %q) ---\n", p.Schedule.DivisionName, p.Schedule.TeamName)
		writeSchedulePlayers(&b, *p.Schedule)
		for _, g := range p.Schedule.Games {
			fmt.Fprintf(&b, "  game %-14s vs %-30s we_are_team_one=%t cancelled=%t sets=%s\n",
				g.Round, g.OpponentName, g.WeAreTeamOne, g.WasCancelled, formatSetScores(g.SetScores))
		}
	}

	if len(p.DivisionTeams) > 0 {
		b.WriteString("\n--- division teams ---\n")
		writeDivisionTeams(&b, p.DivisionTeams)
	}

	if c.Result != nil {
		fmt.Fprintf(&b, "\n--- computed result: %s ---\n", c.Result.Status)
		writeGameLines(&b, c.Result.Games)
		fmt.Fprintf(&b, "  total delta: %+d (has_proxy=%t)\n", c.Result.TotalDelta, c.Result.HasProxy)
		if c.Result.Player != nil {
			fmt.Fprintf(&b, "  %s: %d -> %d\n", c.Result.Player.Name, c.Result.Player.RGX, c.Result.Player.RGX+c.Result.TotalDelta)
		}
	}

	return b.String()
}

func summarizePlayer(c playerCapture) string {
	if c.PipelineError != "" {
		return "error: " + c.PipelineError
	}
	if c.Result == nil {
		return "no result"
	}
	if c.Result.Status != "ok" {
		return c.Result.Status
	}
	return fmt.Sprintf("ok: %d games, total delta %+d", len(c.Result.Games), c.Result.TotalDelta)
}

// findPlayer resolves a query against the parsed player index: exact ID match first, then
// an exact case-insensitive name match, then an unambiguous substring name match.
func findPlayer(players []db.Player, query string) (*db.Player, error) {
	for i := range players {
		if players[i].ID == query {
			return &players[i], nil
		}
	}
	lower := strings.ToLower(query)
	for i := range players {
		if strings.ToLower(players[i].Name) == lower {
			return &players[i], nil
		}
	}
	var candidates []*db.Player
	for i := range players {
		if strings.Contains(strings.ToLower(players[i].Name), lower) {
			candidates = append(candidates, &players[i])
		}
	}
	switch len(candidates) {
	case 0:
		return nil, fmt.Errorf("no player matches %q (checked ID and name against %d players)", query, len(players))
	case 1:
		return candidates[0], nil
	default:
		names := make([]string, len(candidates))
		for i, c := range candidates {
			names[i] = fmt.Sprintf("%s (%s)", c.Name, c.ID)
		}
		return nil, fmt.Errorf("%q matches multiple players, be more specific: %s", query, strings.Join(names, ", "))
	}
}

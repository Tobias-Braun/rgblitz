// fixturecapture takes a timestamped snapshot of every playerzone page the RGX pipeline
// touches for one or more players, and records what the backend made of it. It exists to
// debug wrong parse/calculation results: the HTML that produced a bad result is captured
// verbatim next to the parsed intermediates and the computed deltas, so the numbers on
// screen at the tournament can be compared against what the parsers actually saw.
//
//	go run ./cmd/fixturecapture --players "PAULCHEN,Anna Beispiel"
//
// Each player gets a directory under --out named <YYYY-MM-DD>/<HHMM>-<player-id>/ containing:
//
//	*.html        the fetched pages, named exactly as scraper.FixtureFetcher expects them,
//	              so the directory can be replayed offline as a fixture set:
//	                go run ./cmd/scrapecheck --player X --fixture-dir <snapshot> --date <YYYY-MM-DD>
//	capture.json  machine-readable: fetched paths, the player record, the parsed
//	              intermediates (profile/schedule/sidebar/division teams/tournament detail)
//	              and the final TodayResult
//	report.txt    the same information rendered for reading, including every RGX formula
//	              intermediate (b, p, e1) per game
//
// Every request goes through the shared 1-request-per-2s rate limiter, exactly like the
// server's own scrapes.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

func main() {
	playersArg := flag.String("players", "", "comma-separated player IDs (e.g. PAULCHEN) or full names to capture — required")
	outDir := flag.String("out", envOr("CAPTURE_DIR", "../scraper/captures"), "root directory for capture snapshots")
	dbPath := flag.String("db-path", filepath.Join(os.TempDir(), "rgblitz-fixturecapture.db"), "SQLite file for the tournaments table (GRT status cache)")
	fromFixtures := flag.String("from-fixtures", "", "capture from a local fixture directory instead of the live site — for exercising this tool itself without network traffic")
	dateStr := flag.String("date", "", "override today's date as YYYY-MM-DD (Europe/Berlin); only useful together with --from-fixtures, whose snapshot is from a fixed day")
	flag.Parse()

	queries := splitList(*playersArg)
	if len(queries) == 0 {
		fmt.Fprintln(os.Stderr, "error: --players is required (comma-separated player IDs or names)")
		flag.Usage()
		os.Exit(1)
	}

	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		fatalf("load Europe/Berlin timezone: %v", err)
	}
	now := time.Now().In(berlin)
	if *dateStr != "" {
		parsed, err := time.ParseInLocation("2006-01-02", *dateStr, berlin)
		if err != nil {
			fatalf("--date must be YYYY-MM-DD: %v", err)
		}
		// Keep a plausible time of day so the snapshot directory name is a real capture hour.
		now = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 12, 0, 0, 0, berlin)
	}

	runDir := filepath.Join(*outDir, now.Format("2006-01-02"))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		fatalf("create capture directory %s: %v", runDir, err)
	}

	// One rate limiter and one recorder for the whole run: the ranking pages are fetched
	// once and shared by every player captured in this run.
	var source scraper.Fetcher
	if *fromFixtures != "" {
		fmt.Printf("source: fixtures in %s (no live requests)\n", *fromFixtures)
		source = scraper.NewFixtureFetcher(*fromFixtures)
	} else {
		source = scraper.NewHTTPFetcher(scraper.NewRateLimiter(2 * time.Second))
	}
	rec := &recordingFetcher{inner: source}

	rankingDir := filepath.Join(runDir, "_ranking-"+now.Format("1504"))
	if err := os.MkdirAll(rankingDir, 0o755); err != nil {
		fatalf("create ranking directory %s: %v", rankingDir, err)
	}
	rec.dir = rankingDir

	fmt.Printf("capture at %s (Europe/Berlin) → %s\n", now.Format("2006-01-02 15:04 Monday"), runDir)
	fmt.Println("fetching the player index (Open + Women rankings) ...")
	open, err := scraper.FetchRankingPage(rec, scraper.RankingOpenPath)
	if err != nil {
		fatalf("fetch open ranking page: %v", err)
	}
	women, err := scraper.FetchRankingPage(rec, scraper.RankingWomenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: fetch women ranking page failed, indexing Open RGX only: %v\n", err)
		women = nil
	}
	index := scraper.MergeWomenRGX(open, women)
	fmt.Printf("  indexed %d players (%d with a Women RGX override)\n", len(index), len(women))

	conn, err := db.Open(*dbPath)
	if err != nil {
		fatalf("open capture db %s: %v", *dbPath, err)
	}
	defer conn.Close()
	tournaments := db.NewTournamentStore(conn)

	failures := 0
	for _, query := range queries {
		player, err := findPlayer(index, query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skipping %q: %v\n", query, err)
			failures++
			continue
		}

		snapDir := filepath.Join(runDir, fmt.Sprintf("%s-%s", now.Format("1504"), sanitize(player.ID)))
		if err := os.MkdirAll(snapDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "  skipping %s: create snapshot dir: %v\n", player.Name, err)
			failures++
			continue
		}
		// The ranking pages belong to the run, not to one player; link them into each
		// snapshot so every snapshot is a self-contained, replayable fixture set.
		if err := shareRankingPages(rankingDir, snapDir); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: %s: share ranking pages: %v\n", player.Name, err)
		}

		rec.dir = snapDir
		rec.reset()

		fmt.Printf("capturing %s (id=%s, index rgx=%d) → %s\n", player.Name, player.ID, player.RGX, snapDir)
		// Each player gets a fresh cache so nothing is answered from a previous player's
		// fetch — a capture must contain every page the pipeline used, not a subset.
		deps := scraper.ScrapeDeps{
			Fetcher:     rec,
			Cache:       cache.New(),
			ProxyURL:    func(s string) string { return s },
			Tournaments: tournaments,
		}

		snapshot := capture{
			CapturedAt:    now.Format(time.RFC3339),
			Date:          now.Format("2006-01-02"),
			Weekday:       now.Format("Monday"),
			Query:         query,
			IndexedPlayer: player,
		}

		result, tournamentID, err := scraper.ScrapePlayerToday(deps, player, now)
		if err != nil {
			snapshot.PipelineError = err.Error()
			fmt.Fprintf(os.Stderr, "  pipeline error: %v\n", err)
			failures++
		} else {
			snapshot.Result = &result
			snapshot.TournamentID = tournamentID
		}
		snapshot.Fetched = rec.records()
		snapshot.Parsed = reparse(snapDir)

		if err := writeCapture(snapDir, snapshot); err != nil {
			fmt.Fprintf(os.Stderr, "  write capture: %v\n", err)
			failures++
			continue
		}
		fmt.Printf("  %s\n", summarize(snapshot))
	}

	if failures > 0 {
		os.Exit(1)
	}
}

// capture is the full record of one player's snapshot, serialized to capture.json.
type capture struct {
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

// fetchRecord notes one live request and the file its body was written to.
type fetchRecord struct {
	Path      string `json:"path"`
	File      string `json:"file"`
	Bytes     int    `json:"bytes"`
	FetchedAt string `json:"fetched_at"`
	Error     string `json:"error,omitempty"`
}

// recordingFetcher wraps the live fetcher, writing every response body to disk under the
// name FixtureFetcher would later look it up by, then handing the buffered body on to the
// pipeline unchanged.
type recordingFetcher struct {
	inner scraper.Fetcher
	dir   string
	log   []fetchRecord
	// seen counts how many times each fixture file name has been written in the current
	// snapshot, so two different paths mapping to the same name (e.g. two division team
	// pages) don't silently overwrite each other.
	seen map[string]int
}

func (r *recordingFetcher) reset() {
	r.log = nil
	r.seen = map[string]int{}
}

func (r *recordingFetcher) records() []fetchRecord { return r.log }

func (r *recordingFetcher) Fetch(path string) (io.ReadCloser, error) {
	rec := fetchRecord{Path: path, FetchedAt: time.Now().Format(time.RFC3339)}

	body, err := r.inner.Fetch(path)
	if err != nil {
		rec.Error = err.Error()
		r.log = append(r.log, rec)
		return nil, err
	}
	defer body.Close()

	raw, err := io.ReadAll(body)
	if err != nil {
		rec.Error = err.Error()
		r.log = append(r.log, rec)
		return nil, err
	}
	rec.Bytes = len(raw)

	name, nameErr := scraper.FixtureFileForPath(path)
	if nameErr != nil {
		// An unmapped path still gets captured, just under a derived name — it can't be
		// replayed through FixtureFetcher, but the bytes are what matter for debugging.
		name = sanitize(strings.TrimPrefix(path, "/")) + ".html"
	}
	if r.seen == nil {
		r.seen = map[string]int{}
	}
	r.seen[name]++
	if n := r.seen[name]; n > 1 {
		ext := filepath.Ext(name)
		name = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), n, ext)
	}
	rec.File = name

	if err := os.WriteFile(filepath.Join(r.dir, name), raw, 0o644); err != nil {
		rec.Error = err.Error()
	}
	r.log = append(r.log, rec)

	return io.NopCloser(bytes.NewReader(raw)), nil
}

// shareRankingPages makes the run's ranking snapshots available inside a player snapshot.
// Hard links keep a weekend of hourly captures from costing a multi-megabyte ranking copy
// per player; a copy is used where linking isn't possible.
func shareRankingPages(from, to string) error {
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(from, e.Name())
		dst := filepath.Join(to, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.Link(src, dst); err == nil {
			continue
		}
		raw, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeCapture(dir string, c capture) error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "capture.json"), raw, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.txt"), []byte(report(c)), 0o644)
}

// report renders a capture as the human-readable debugging view: what was fetched, what
// each parser made of it, and every intermediate of the RGX formula per game.
func report(c capture) string {
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
	for _, f := range c.Fetched {
		fmt.Fprintf(&b, "  %-46s -> %-28s %8d bytes %s\n", f.Path, f.File, f.Bytes, f.Error)
	}

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
		fmt.Fprintf(&b, "\n--- tournament detail ---\n  %s (%s) grt=%t\n",
			p.TournamentDetail.Name, p.TournamentDetail.Date, p.TournamentDetail.IsGRT)
	}

	if len(p.DivisionSidebar) > 0 {
		b.WriteString("\n--- division sidebar ---\n")
		for _, d := range p.DivisionSidebar {
			fmt.Fprintf(&b, "  %-24s id=%-6s tournament=%-6s %s\n", d.Name, d.ID, d.TournamentID, d.TeamsPath)
		}
	}

	if p.Schedule != nil {
		fmt.Fprintf(&b, "\n--- team schedule (division %q, team %q) ---\n", p.Schedule.DivisionName, p.Schedule.TeamName)
		r1 := 0
		for _, pl := range p.Schedule.Players {
			note := ""
			if pl.RGX <= 0 {
				note = fmt.Sprintf(" (no RGX on page → proxy %d)", scraper.DivisionStartingRGX(p.Schedule.DivisionName))
			}
			fmt.Fprintf(&b, "  player %-28s rgx=%d%s\n", pl.Name, pl.RGX, note)
			if pl.RGX > 0 {
				r1 += pl.RGX
			} else {
				r1 += scraper.DivisionStartingRGX(p.Schedule.DivisionName)
			}
		}
		fmt.Fprintf(&b, "  r1 (our combined rgx from schedule page) = %d\n", r1)
		for _, g := range p.Schedule.Games {
			fmt.Fprintf(&b, "  game %-14s vs %-30s we_are_team_one=%t cancelled=%t sets=%s\n",
				g.Round, g.OpponentName, g.WeAreTeamOne, g.WasCancelled, formatSetScores(g.SetScores))
		}
	}

	if len(p.DivisionTeams) > 0 {
		b.WriteString("\n--- division teams ---\n")
		for _, t := range p.DivisionTeams {
			fmt.Fprintf(&b, "  %-34s team_id=%-7d combined_rgx=%-5d %s\n",
				t.Name, t.TournamentTeamID, t.CombinedRGX, strings.Join(t.PlayerNames, " + "))
		}
	}

	if c.Result != nil {
		fmt.Fprintf(&b, "\n--- computed result: %s ---\n", c.Result.Status)
		for _, g := range c.Result.Games {
			won, lost := setsFromResult(g)
			br := scraper.ComputeDeltaBreakdown(g.TeamRGX, g.OpponentRGX, won, lost)
			proxy := ""
			if g.HasProxy {
				proxy = " proxy"
			}
			fmt.Fprintf(&b, "  %-14s vs %-30s %s %+4d  r1=%d r2=%d sets=%d:%d b=%.2f p=%.2f e1=%.4f%s\n",
				g.Round, g.OpponentName, g.Result, g.RGXDelta,
				g.TeamRGX, g.OpponentRGX, won, lost, br.B, br.P, br.E1, proxy)
		}
		fmt.Fprintf(&b, "  total delta: %+d (has_proxy=%t)\n", c.Result.TotalDelta, c.Result.HasProxy)
		if c.Result.Player != nil {
			fmt.Fprintf(&b, "  %s: %d -> %d\n", c.Result.Player.Name, c.Result.Player.RGX, c.Result.Player.RGX+c.Result.TotalDelta)
		}
	}

	return b.String()
}

// setsFromResult recounts sets won/lost from the oriented set scores in a GameResult, so
// the report can show the same (won, lost) pair the delta was computed from.
func setsFromResult(g scraper.GameResult) (won, lost int) {
	for _, s := range g.Sets {
		switch {
		case s.OurScore > s.OpponentScore:
			won++
		case s.OpponentScore > s.OurScore:
			lost++
		}
	}
	return won, lost
}

func formatSetScores(sets []scraper.SetScore) string {
	parts := make([]string, 0, len(sets))
	for _, s := range sets {
		parts = append(parts, strconv.Itoa(s.TeamOneScore)+":"+strconv.Itoa(s.TeamTwoScore))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}

func summarize(c capture) string {
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

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// sanitize reduces a player ID or URL path to something safe for a file name.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, s)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

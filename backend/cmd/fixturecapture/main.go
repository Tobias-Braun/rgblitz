// fixturecapture takes a timestamped snapshot of the playerzone pages the RGX pipeline
// reads, and records what the backend made of them. It exists to debug wrong parse or
// calculation results: the HTML that produced a bad number is captured verbatim next to the
// parsed intermediates and the computed deltas, so what's on screen at the tournament can be
// compared against what the parsers actually saw.
//
// It has three modes.
//
// # Tournament mode (no arguments)
//
//	go run ./cmd/fixturecapture
//
// Finds every tournament running today and captures *all* of it: the tournament detail page,
// every division's team list, and every team's schedule, with RGX deltas computed for each
// team through the same code the API uses. Because it sees both sides of every game, it also
// cross-checks the results — see the "consistency checks" section of the report.
//
// # Player mode
//
//	go run ./cmd/fixturecapture --players "LUC2110,Lucas Christiani"
//
// Captures just what one player's /api/players/{id}/today request would fetch, via the real
// ScrapePlayerToday pipeline.
//
// # League mode
//
//	go run ./cmd/fixturecapture --league "39500,39823"
//
// Captures one Deutsche Roundnet Liga team's matchday per team ID (the {id} of the team's
// /tournaments/schedule/team/{id} page): its schedule page, the matchday group page of each
// Begegnung, and for today's Begegnungen the /ajax/get-subgames Partien and the opponents'
// schedule pages. Every roster player runs through the real ScrapePlayerLeagueToday, and the
// report lists each player's per-Partie deltas next to the RGX their roster card shows — the
// pre-matchday baseline during a matchday, the recalculated value once playerzone has run it.
//
// A league snapshot replays offline through the same mode:
//
//	go run ./cmd/fixturecapture --league <id> --from-fixtures <snapshot> --date <YYYY-MM-DD>
//
// Every snapshot directory holds:
//
//	*.html         every fetched page (*.json for league Partien)
//	manifest.json  URL path → file, so the snapshot replays offline with full fidelity:
//	                 go run ./cmd/scrapecheck --player X --fixture-dir <snapshot> --date <YYYY-MM-DD>
//	capture.json   machine-readable: fetched paths, parsed intermediates, computed results
//	report.txt     the same rendered for reading, including every RGX formula intermediate
//
// Every request goes through the shared 1-request-per-2s rate limiter, exactly like the
// server's own scrapes.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"database/sql"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

func main() {
	playersArg := flag.String("players", "", "comma-separated player IDs (e.g. LUC2110) or full names; omit to capture every tournament running today")
	tournamentsArg := flag.String("tournaments", "", "comma-separated numeric tournament IDs to capture instead of discovering today's")
	outDir := flag.String("out", envOr("CAPTURE_DIR", "../scraper/captures"), "root directory for capture snapshots")
	dbPath := flag.String("db-path", filepath.Join(os.TempDir(), "rgblitz-fixturecapture.db"), "SQLite file for the tournaments table (GRT status cache)")
	fromFixtures := flag.String("from-fixtures", "", "capture from a local fixture directory instead of the live site — for exercising this tool itself without network traffic")
	dateStr := flag.String("date", "", "override today's date as YYYY-MM-DD (Europe/Berlin); mainly useful together with --from-fixtures, whose snapshot is from a fixed day")
	divisionFilter := flag.String("divisions", "", "tournament mode: only capture divisions whose name contains one of these comma-separated substrings (e.g. \"Open,Women\")")
	includeNonGRT := flag.Bool("include-non-grt", false, "tournament mode: also capture tournaments that aren't part of the German Roundnet Tour (the backend reports those as no_tournament)")
	leagueArg := flag.String("league", "", "comma-separated league team IDs (the {id} of /tournaments/schedule/team/{id}) to capture today's league matchday for")
	maxRequests := flag.Int("max-requests", 600, "tournament mode: stop capturing after this many playerzone requests, so one huge tournament can't run for hours at 1 req/2s")
	flag.Parse()

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

	var source scraper.Fetcher
	if *fromFixtures != "" {
		fmt.Printf("source: fixtures in %s (no live requests)\n", *fromFixtures)
		if scraper.HasManifest(*fromFixtures) {
			mf, err := scraper.NewManifestFetcher(*fromFixtures)
			if err != nil {
				fatalf("%v", err)
			}
			source = mf
		} else {
			source = scraper.NewFixtureFetcher(*fromFixtures)
		}
	} else {
		source = scraper.NewHTTPFetcher(scraper.NewRateLimiter(2 * time.Second))
	}

	// One recorder for the whole run, so shared pages (the rankings in player mode, the
	// tournament list in tournament mode) are fetched once and linked into every snapshot.
	rec := &recordingFetcher{inner: source}
	sharedDir := filepath.Join(runDir, "_shared-"+now.Format("1504"))
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		fatalf("create shared page directory %s: %v", sharedDir, err)
	}

	conn, err := db.Open(*dbPath)
	if err != nil {
		fatalf("open capture db %s: %v", *dbPath, err)
	}
	defer conn.Close()

	fmt.Printf("capture at %s (Europe/Berlin) → %s\n", now.Format("2006-01-02 15:04 Monday"), runDir)

	run := runContext{
		now:       now,
		runDir:    runDir,
		sharedDir: sharedDir,
		rec:       rec,
		conn:      conn,
	}

	var failures int
	if teams := splitList(*leagueArg); len(teams) > 0 {
		failures = captureLeague(run, teams)
	} else if queries := splitList(*playersArg); len(queries) > 0 {
		failures = capturePlayers(run, queries)
	} else {
		failures = captureTournaments(run, tournamentOptions{
			explicitIDs:   splitList(*tournamentsArg),
			divisionsLike: splitList(*divisionFilter),
			includeNonGRT: *includeNonGRT,
			maxRequests:   *maxRequests,
		})
	}

	if failures > 0 {
		os.Exit(1)
	}
}

// runContext is the per-invocation state both capture modes need.
type runContext struct {
	now       time.Time
	runDir    string
	sharedDir string
	rec       *recordingFetcher
	conn      *sql.DB
}

// snapshotDir creates the directory for one snapshot of this run, named by capture hour so
// consecutive hourly captures of the same subject sit next to each other in time order.
func (r runContext) snapshotDir(suffix string) (string, error) {
	dir := filepath.Join(r.runDir, fmt.Sprintf("%s-%s", r.now.Format("1504"), sanitize(suffix)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create snapshot dir: %w", err)
	}
	return dir, nil
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

// sanitize reduces an ID or URL path to something safe for a file name.
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

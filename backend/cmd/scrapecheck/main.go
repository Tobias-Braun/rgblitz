// scrapecheck dry-runs the full scrape → RGX pipeline for one player, without ever going
// through the HTTP server. It prints every step as it happens so a developer can see exactly
// what the scraper found and how the RGX delta was computed.
//
// Dev mode (default) reads HTML fixtures from disk — no network access needed:
//
//	go run ./cmd/scrapecheck --player PAULCHEN
//
// Prod mode hits the real playerzone site (respecting the 1-request-per-2s rate limit):
//
//	go run ./cmd/scrapecheck --mode prod --player "Paul Siemer"
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

func main() {
	mode := flag.String("mode", "dev", `"dev" reads local fixtures, "prod" scrapes the live site`)
	playerQuery := flag.String("player", "", "player ID (e.g. PAULCHEN) or full name (e.g. \"Paul Siemer\") to check — required")
	fixtureDir := flag.String("fixture-dir", envOr("FIXTURE_DIR", "../scraper/fixtures"), "directory of HTML fixtures (dev mode only)")
	dateStr := flag.String("date", "", "override today's date as YYYY-MM-DD (Europe/Berlin), for testing tournament-date matching against fixtures captured on 2026-07-19")
	flag.Parse()

	if *playerQuery == "" {
		fmt.Fprintln(os.Stderr, "error: --player is required (a player ID or full name)")
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
		// Keep a realistic time-of-day so the 08:00 tournament-window guard (if ever
		// exercised) isn't accidentally tripped by midnight.
		now = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 12, 0, 0, 0, berlin)
	}

	var fetcher scraper.Fetcher
	switch *mode {
	case "dev":
		fmt.Printf("mode: dev — reading fixtures from %s\n", *fixtureDir)
		fetcher = scraper.NewFixtureFetcher(*fixtureDir)
	case "prod":
		fmt.Println("mode: prod — scraping https://playerzone.roundnetgermany.de (rate-limited to 1 req/2s)")
		fetcher = scraper.NewHTTPFetcher(scraper.NewRateLimiter(2 * time.Second))
	default:
		fatalf(`--mode must be "dev" or "prod", got %q`, *mode)
	}

	fmt.Printf("today (Europe/Berlin): %s\n\n", now.Format("2006-01-02 15:04 (Monday)"))

	fmt.Println("step 1/2: fetching the player index (Open + Women rankings) ...")
	open, err := scraper.FetchRankingPage(fetcher, scraper.RankingOpenPath)
	if err != nil {
		fatalf("fetch open ranking page: %v", err)
	}
	women, err := scraper.FetchRankingPage(fetcher, scraper.RankingWomenPath)
	if err != nil {
		fmt.Printf("  warning: fetch women ranking page failed, indexing Open RGX only: %v\n", err)
		women = nil
	}
	players := scraper.MergeWomenRGX(open, women)
	fmt.Printf("  found %d players in the index (%d with a Women RGX override)\n", len(players), len(women))

	player, err := findPlayer(players, *playerQuery)
	if err != nil {
		fatalf("%v", err)
	}
	fmt.Printf("  matched: %s (id=%s, club=%s, rgx=%d)\n\n", player.Name, player.ID, player.ClubName, player.RGX)

	// The tournaments table only needs to persist across runs so repeated checks against
	// the same tournament don't re-fetch its detail page — an ephemeral file in the OS
	// temp dir is enough, no --db-path flag needed for a dry-run tool.
	tournamentsConn, err := db.Open(filepath.Join(os.TempDir(), "rgblitz-scrapecheck.db"))
	if err != nil {
		fatalf("open scrapecheck db: %v", err)
	}
	defer tournamentsConn.Close()

	fmt.Println("step 2/2: running the scrape+RGX pipeline (profile → schedule → division sidebar → division teams) ...")
	deps := scraper.ScrapeDeps{
		Fetcher:     fetcher,
		Cache:       cache.New(),
		ProxyURL:    func(s string) string { return s },
		Tournaments: db.NewTournamentStore(tournamentsConn),
	}
	resp, tournamentID, err := scraper.ScrapePlayerToday(deps, player, now)
	if err != nil {
		fatalf("pipeline error: %v", err)
	}
	if tournamentID != 0 {
		fmt.Printf("  tournament id: %d\n", tournamentID)
	}

	printTodayResponse(resp)
}

func printTodayResponse(resp scraper.TodayResult) {
	fmt.Println()
	if resp.Status == "no_tournament" {
		fmt.Println("result: no tournament found for this player today")
		return
	}

	fmt.Printf("result: %s on %s\n", resp.Status, resp.Date)
	if resp.HasProxy {
		fmt.Println("  (uses a division default RGX for at least one new player)")
	}
	if len(resp.Games) == 0 {
		fmt.Println("  no games found on the team schedule yet")
	}
	for _, g := range resp.Games {
		proxyNote := ""
		if g.HasProxy {
			proxyNote = " (proxy RGX)"
		}
		fmt.Printf("  %-12s vs %-30s %s  %+d%s\n", g.Round, g.OpponentName, g.Result, g.RGXDelta, proxyNote)
	}
	fmt.Printf("\n  total delta: %+d\n", resp.TotalDelta)
	if resp.Player != nil {
		fmt.Printf("  %s: %d -> %d\n", resp.Player.Name, resp.Player.RGX, resp.Player.RGX+resp.TotalDelta)
	}
}

// findPlayer resolves a query against the parsed player index: exact ID match first, then an
// exact case-insensitive name match, then a substring name match (only if it's unambiguous).
func findPlayer(players []db.Player, query string) (*db.Player, error) {
	for i := range players {
		if players[i].ID == query {
			return &players[i], nil
		}
	}

	lowerQuery := strings.ToLower(query)
	for i := range players {
		if strings.ToLower(players[i].Name) == lowerQuery {
			return &players[i], nil
		}
	}

	var candidates []*db.Player
	for i := range players {
		if strings.Contains(strings.ToLower(players[i].Name), lowerQuery) {
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

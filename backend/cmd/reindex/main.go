// reindex runs the live player index scrape (the same job the Friday 22:00 CET
// cron trigger runs) immediately against the real playerzone site, and upserts
// the results into SQLite. Use it to sanity-check the scraper after a change,
// or to manually refresh/repair the index outside the weekly schedule.
// Usage: go run ./cmd/reindex --db ../data/players.db
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/config"
	"github.com/tobiasmarkusbraun/rgblitz/internal/cron"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

func main() {
	dbPath := flag.String("db", "./data/players.db", "path to sqlite database")
	flag.Parse()

	cfg := config.Load()

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	players := db.NewPlayerStore(database)
	permissions := db.NewPermissionStore(database)
	jobs := db.NewJobStore(database)
	league := db.NewLeagueStore(database)
	before, err := players.Count()
	if err != nil {
		log.Fatalf("count players: %v", err)
	}

	// Same 2s-per-request limiter the server uses — this is a real request to
	// playerzone, not a fixture, so the rate limit rule still applies.
	rateLimiter := scraper.NewRateLimiter(2 * time.Second)
	fetcher := scraper.NewHTTPFetcher(rateLimiter)
	scheduler := cron.NewScheduler(players, jobs, league, fetcher, permissions, cfg.RespectPermission)

	fmt.Printf("scraping live player index (%d players currently in %q)...\n", before, *dbPath)
	if err := scheduler.ScrapeNow(); err != nil {
		log.Fatalf("scrape failed: %v", err)
	}

	after, err := players.Count()
	if err != nil {
		log.Fatalf("count players: %v", err)
	}
	fmt.Printf("reindex complete — %d players in %q (%+d)\n", after, *dbPath, after-before)
}

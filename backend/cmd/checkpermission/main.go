// checkpermission fetches the live robots.txt from playerzone, evaluates
// whether RGBlitzBot is allowed to scrape every page type this app needs
// (scraper.RequiredScrapePaths), and stores the result in the same
// scrape_permission row that RESPECT_PERMISSION consults for both the weekly
// index scrape and live /today API requests. Run it any time you want to
// refresh the stored permission without waiting for the next Friday scrape.
//
//	go run ./cmd/checkpermission --db ./data/players.db
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/tobiasmarkusbraun/rgx-calc/internal/db"
	"github.com/tobiasmarkusbraun/rgx-calc/internal/scraper"
)

func main() {
	dbPath := flag.String("db", "./data/players.db", "path to sqlite database")
	flag.Parse()

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	permissions := db.NewPermissionStore(database)

	// Same rate limiter shape the rest of the scraper uses, even though this
	// command only ever makes a single request (NewRateLimiter panics on a
	// non-positive interval, so 0 isn't an option here).
	fetcher := scraper.NewHTTPFetcher(scraper.NewRateLimiter(2 * time.Second))
	fmt.Println("fetching https://playerzone.roundnetgermany.de/robots.txt ...")
	allowed, err := scraper.CheckPermission(fetcher, scraper.DefaultUserAgent, scraper.RequiredScrapePaths)
	if err != nil {
		log.Fatalf("check permission: %v", err)
	}

	now := time.Now()
	if err := permissions.Set(allowed, now); err != nil {
		log.Fatalf("store permission: %v", err)
	}

	if allowed {
		fmt.Printf("robots.txt allows %q for all required paths — scraping permitted (stored %s)\n", scraper.DefaultUserAgent, now.Format(time.RFC3339))
	} else {
		fmt.Printf("robots.txt disallows %q for at least one required path — scraping BLOCKED (stored %s)\n", scraper.DefaultUserAgent, now.Format(time.RFC3339))
	}
	fmt.Println("checked paths:", scraper.RequiredScrapePaths)
}

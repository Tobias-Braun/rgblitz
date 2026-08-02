package main

import (
	"log"
	"net/http"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/api"
	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/config"
	"github.com/tobiasmarkusbraun/rgblitz/internal/cron"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/jobqueue"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	players := db.NewPlayerStore(database)
	permissions := db.NewPermissionStore(database)
	jobs := db.NewJobStore(database)
	memCache := cache.New()

	// One request to playerzone every 2 seconds across all goroutines.
	rateLimiter := scraper.NewRateLimiter(2 * time.Second)

	fetcher := scraper.NewHTTPFetcher(rateLimiter)

	scheduler := cron.NewScheduler(players, jobs, fetcher, permissions, cfg.RespectPermission)

	// Guard against serving an empty/stale index on first boot or after a lost
	// volume — the weekly cron alone wouldn't catch up until the next Saturday.
	count, err := players.Count()
	if err != nil {
		log.Fatalf("count players: %v", err)
	}
	if count < 1000 {
		log.Printf("player index has only %d rows — running initial scrape before serving requests", count)
		if err := scheduler.ScrapeNow(); err != nil {
			log.Printf("initial scrape failed: %v", err)
		}
	}

	scheduler.Start()
	defer scheduler.Stop()

	scrapeDeps := scraper.ScrapeDeps{Fetcher: fetcher, Cache: memCache, ProxyURL: api.ProxyURL}
	queue := jobqueue.New(jobs, players, scrapeDeps, permissions, cfg.RespectPermission, time.Now)

	// Recover any job left "running" by a crash mid-scrape, then start the
	// single background worker that drains the queue in FIFO order.
	stuck, err := jobs.ResetStuckRunning()
	if err != nil {
		log.Printf("reset stuck jobs: %v", err)
	}
	for _, id := range stuck {
		queue.Enqueue(id)
	}
	queue.Start()

	handler := api.NewHandler(cfg, players, memCache, fetcher, permissions, jobs, queue)
	router := api.NewRouter(handler)

	addr := ":" + cfg.Port
	log.Printf("rgblitz server starting on %s (dev_mode=%v)", addr, cfg.DevMode)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server: %v", err)
	}
}

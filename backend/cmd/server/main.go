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
	"github.com/tobiasmarkusbraun/rgblitz/internal/email"
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
	league := db.NewLeagueStore(database)
	jobStats := db.NewJobStatsStore(database)
	tournaments := db.NewTournamentStore(database)
	feedback := db.NewFeedbackStore(database)
	notifier := email.NewResendClient(cfg.ResendAPIKey, cfg.ResendFromEmail, cfg.FeedbackNotifyEmail)
	memCache := cache.New()

	// One request to playerzone every 2 seconds across all goroutines.
	rateLimiter := scraper.NewRateLimiter(2 * time.Second)

	fetcher := scraper.NewHTTPFetcher(rateLimiter)

	scheduler := cron.NewScheduler(players, jobs, league, fetcher, permissions, cfg.RespectPermission)

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

	// The league roster index takes minutes (one request per team), so unlike the player
	// index it is built in the background; league players fall back to the tournament path
	// until it is done. It parses scraped pages, hence the recover.
	if n, err := league.Count(); err == nil && n == 0 {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("initial league roster index panicked: %v", r)
				}
			}()
			if err := scheduler.ScrapeLeagueNow(); err != nil {
				log.Printf("initial league roster index failed: %v", err)
			}
		}()
	}

	scrapeDeps := scraper.ScrapeDeps{Fetcher: fetcher, Cache: memCache, ProxyURL: api.ProxyURL, Tournaments: tournaments, League: league}
	queue := jobqueue.New(jobs, jobStats, players, scrapeDeps, permissions, cfg.RespectPermission, time.Now)

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

	handler := api.NewHandler(cfg, players, memCache, fetcher, permissions, jobs, jobStats, queue, feedback, notifier)
	router := api.NewRouter(handler)

	addr := ":" + cfg.Port
	log.Printf("rgblitz server starting on %s (dev_mode=%v)", addr, cfg.DevMode)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server: %v", err)
	}
}

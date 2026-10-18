package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/tobiasmarkusbraun/rgblitz/internal/api"
	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/config"
	"github.com/tobiasmarkusbraun/rgblitz/internal/cron"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/email"
	"github.com/tobiasmarkusbraun/rgblitz/internal/jobqueue"
	"github.com/tobiasmarkusbraun/rgblitz/internal/metrics"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

func main() {
	// Set as default so every slog call — and any leftover log.Print — is JSON on stdout.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		fatal("open db", err)
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
		fatal("count players", err)
	}
	if count < 1000 {
		slog.Info("player index too small, running initial scrape before serving requests", "players", count)
		if err := scheduler.ScrapeNow(); err != nil {
			slog.Error("initial scrape failed", "error", err)
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
					slog.Error("initial league roster index panicked", "panic", r)
				}
			}()
			if err := scheduler.ScrapeLeagueNow(); err != nil {
				slog.Error("initial league roster index failed", "error", err)
			}
		}()
	}

	scrapeDeps := scraper.ScrapeDeps{Fetcher: fetcher, Cache: memCache, ProxyURL: api.ProxyURL, Tournaments: tournaments, League: league}
	queue := jobqueue.New(jobs, jobStats, players, scrapeDeps, permissions, cfg.RespectPermission, time.Now)

	// Recover any job left "running" by a crash mid-scrape, then start the
	// single background worker that drains the queue in FIFO order.
	stuck, err := jobs.ResetStuckRunning()
	if err != nil {
		slog.Error("reset stuck jobs", "error", err)
	}
	for _, id := range stuck {
		queue.Enqueue(id)
	}
	queue.Start()

	handler := api.NewHandler(cfg, players, memCache, fetcher, permissions, jobs, jobStats, queue, feedback, notifier)
	router := api.NewRouter(handler)

	prometheus.MustRegister(&metrics.DBCollector{Players: players, Stats: jobStats, Tournaments: tournaments,
		Feedback: feedback, Permissions: permissions, RespectPermission: cfg.RespectPermission})
	go func() {
		addr := ":" + cfg.MetricsPort
		slog.Info("metrics server starting", "addr", addr)
		fatal("metrics server", http.ListenAndServe(addr, metrics.Handler()))
	}()

	addr := ":" + cfg.Port
	slog.Info("rgblitz server starting", "addr", addr, "dev_mode", cfg.DevMode)
	fatal("server", http.ListenAndServe(addr, router))
}

func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

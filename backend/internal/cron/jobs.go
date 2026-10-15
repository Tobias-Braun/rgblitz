package cron

import (
	"fmt"
	"log/slog"
	"time"

	robfigcron "github.com/robfig/cron/v3"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/metrics"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// Scheduler wraps the cron runner and dependencies needed for the weekly scrape.
type Scheduler struct {
	c                 *robfigcron.Cron
	players           *db.PlayerStore
	jobs              *db.JobStore
	fetcher           scraper.Fetcher
	permissions       *db.PermissionStore
	respectPermission bool
}

// NewScheduler builds a Scheduler. When respectPermission is true, every scrape
// (cron-triggered or via ScrapeNow) first checks robots.txt and refuses to make
// any playerzone request at all if RGBlitzBot isn't allowed — the result is
// persisted via permissions so it can also gate live API requests.
func NewScheduler(ps *db.PlayerStore, jobs *db.JobStore, fetcher scraper.Fetcher, permissions *db.PermissionStore, respectPermission bool) *Scheduler {
	loc, _ := time.LoadLocation("Europe/Berlin")
	// Recover wraps every registered job: an unrecovered panic in a cron
	// job's goroutine would otherwise crash the whole process, same as an
	// unrecovered panic in the jobqueue worker (see internal/jobqueue.Queue.process).
	c := robfigcron.New(robfigcron.WithLocation(loc), robfigcron.WithChain(robfigcron.Recover(cronLogger())))
	return &Scheduler{c: c, players: ps, jobs: jobs, fetcher: fetcher, permissions: permissions, respectPermission: respectPermission}
}

// Start registers the player index scrape, the daily RgxJob cleanup, and
// starts the cron runner.
// Saturday and Sunday 03:00 CET: "0 3 * * 6,0" — scraping runs on both
// tournament days (not just Saturday) so that RGX changes from a Saturday
// tournament are picked up before a Sunday tournament computes its deltas.
// Every day 03:00 CET: "0 3 * * *" — purges rgx_jobs (see docs job-queue
// design); aligned with the scrape time so both daily maintenance jobs run
// together.
func (s *Scheduler) Start() {
	_, err := s.c.AddFunc("0 3 * * 6,0", func() {
		slog.Info("cron: starting player index scrape")
		if err := s.scrapePlayerIndex(); err != nil {
			slog.Error("cron: scrape failed", "error", err)
		}
	})
	if err != nil {
		slog.Error("cron: failed to register scrape job", "error", err)
		return
	}

	_, err = s.c.AddFunc("0 3 * * *", func() {
		if err := s.jobs.DeleteAll(); err != nil {
			slog.Error("cron: rgx_jobs cleanup failed", "error", err)
			return
		}
		slog.Info("cron: rgx_jobs cleanup complete")
	})
	if err != nil {
		slog.Error("cron: failed to register rgx_jobs cleanup job", "error", err)
		return
	}

	s.c.Start()
	slog.Info("cron: scheduler started (player index scrape every Saturday and Sunday 03:00 CET, daily rgx_jobs cleanup at 03:00 CET)")
}

func (s *Scheduler) Stop() {
	s.c.Stop()
}

// ScrapeNow runs the player index scrape immediately — useful for initial setup.
func (s *Scheduler) ScrapeNow() error {
	return s.scrapePlayerIndex()
}

// scrapePlayerIndex records every run in the index scrape metrics. outcome
// starts as "error" so a panic (recovered by the cron chain) counts as one.
func (s *Scheduler) scrapePlayerIndex() error {
	start := time.Now()
	outcome := "error"
	defer func() {
		metrics.IndexScrapeRuns.WithLabelValues(outcome).Inc()
		metrics.IndexScrapeDuration.Set(time.Since(start).Seconds())
		if outcome == "ok" {
			metrics.IndexLastSuccess.SetToCurrentTime()
		}
	}()

	if s.respectPermission {
		allowed, err := scraper.CheckPermission(s.fetcher, scraper.DefaultUserAgent, scraper.RequiredScrapePaths)
		if err != nil {
			return fmt.Errorf("check robots.txt permission: %w", err)
		}
		if s.permissions != nil {
			if err := s.permissions.Set(allowed, time.Now()); err != nil {
				slog.Error("cron: store scrape permission", "error", err)
			}
		}
		if !allowed {
			slog.Warn("cron: robots.txt disallows RGBlitzBot for one or more required paths — skipping scrape, no requests made")
			outcome = "robots_denied"
			return nil
		}
	}

	open, err := scraper.FetchRankingPage(s.fetcher, scraper.RankingOpenPath)
	if err != nil {
		return err
	}

	// The Women tab only overrides RGX for players who appear on it — if it
	// fails to load, fall back to the Open-only index rather than aborting
	// the whole scrape.
	women, err := scraper.FetchRankingPage(s.fetcher, scraper.RankingWomenPath)
	if err != nil {
		slog.Warn("cron: women ranking fetch failed, indexing Open RGX only", "error", err)
		women = nil
	}

	players := scraper.MergeWomenRGX(open, women)

	updated, added, errs := 0, 0, 0
	for _, p := range players {
		existing, _ := s.players.GetByID(p.ID)
		if err := s.players.Upsert(p); err != nil {
			slog.Error("cron: upsert player", "player_id", p.ID, "error", err)
			errs++
			continue
		}
		if existing == nil {
			added++
		} else {
			updated++
		}
	}
	slog.Info("cron: player index scrape complete", "updated", updated, "added", added, "errors", errs)
	outcome = "ok"
	return nil
}

// cronLogger routes robfig/cron's own messages (e.g. recovered panics) into
// slog at error level, so they're JSON like every other log line.
func cronLogger() robfigcron.Logger {
	return robfigcron.PrintfLogger(slog.NewLogLogger(slog.Default().Handler(), slog.LevelError))
}

package cron

import (
	"fmt"
	"log"
	"time"

	robfigcron "github.com/robfig/cron/v3"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
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
	c := robfigcron.New(robfigcron.WithLocation(loc))
	return &Scheduler{c: c, players: ps, jobs: jobs, fetcher: fetcher, permissions: permissions, respectPermission: respectPermission}
}

// Start registers the weekly player index scrape, the daily RgxJob cleanup,
// and starts the cron runner.
// Saturday 03:00 CET: "0 3 * * 6"
// Every day 00:05 CET: "5 0 * * *" — purges rgx_jobs (see docs job-queue design)
func (s *Scheduler) Start() {
	_, err := s.c.AddFunc("0 3 * * 6", func() {
		log.Println("cron: starting weekly player index scrape")
		if err := s.scrapePlayerIndex(); err != nil {
			log.Printf("cron: weekly scrape failed: %v", err)
		}
	})
	if err != nil {
		log.Printf("cron: failed to register weekly scrape job: %v", err)
		return
	}

	_, err = s.c.AddFunc("5 0 * * *", func() {
		if err := s.jobs.DeleteAll(); err != nil {
			log.Printf("cron: rgx_jobs cleanup failed: %v", err)
			return
		}
		log.Println("cron: rgx_jobs cleanup complete")
	})
	if err != nil {
		log.Printf("cron: failed to register rgx_jobs cleanup job: %v", err)
		return
	}

	s.c.Start()
	log.Println("cron: scheduler started (weekly scrape every Saturday 03:00 CET, daily rgx_jobs cleanup at 00:05 CET)")
}

func (s *Scheduler) Stop() {
	s.c.Stop()
}

// ScrapeNow runs the player index scrape immediately — useful for initial setup.
func (s *Scheduler) ScrapeNow() error {
	return s.scrapePlayerIndex()
}

func (s *Scheduler) scrapePlayerIndex() error {
	if s.respectPermission {
		allowed, err := scraper.CheckPermission(s.fetcher, scraper.DefaultUserAgent, scraper.RequiredScrapePaths)
		if err != nil {
			return fmt.Errorf("check robots.txt permission: %w", err)
		}
		if s.permissions != nil {
			if err := s.permissions.Set(allowed, time.Now()); err != nil {
				log.Printf("cron: store scrape permission: %v", err)
			}
		}
		if !allowed {
			log.Println("cron: robots.txt disallows RGBlitzBot for one or more required paths — skipping scrape, no requests made")
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
		log.Printf("cron: women ranking fetch failed, indexing Open RGX only: %v", err)
		women = nil
	}

	players := scraper.MergeWomenRGX(open, women)

	updated, added, errs := 0, 0, 0
	for _, p := range players {
		existing, _ := s.players.GetByID(p.ID)
		if err := s.players.Upsert(p); err != nil {
			log.Printf("cron: upsert player %s: %v", p.ID, err)
			errs++
			continue
		}
		if existing == nil {
			added++
		} else {
			updated++
		}
	}
	log.Printf("cron: player index scrape complete — %d updated, %d added, %d errors", updated, added, errs)
	return nil
}

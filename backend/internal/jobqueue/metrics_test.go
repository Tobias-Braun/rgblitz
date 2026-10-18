package jobqueue_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/jobqueue"
	"github.com/tobiasmarkusbraun/rgblitz/internal/metrics"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// TestProcess_CountsFinishedOutcome checks every failure branch of process
// lands in its own rgblitz_jobs_finished_total outcome. All of them fail before
// (or at) the first playerzone request, so no fixtures are needed.
func TestProcess_CountsFinishedOutcome(t *testing.T) {
	cases := []struct {
		outcome           string
		respectPermission bool // with no stored robots.txt check, this denies scraping
		seedPlayer        bool
	}{
		{outcome: "robots_denied", respectPermission: true, seedPlayer: true},
		{outcome: "player_not_found"},
		{outcome: "panic", seedPlayer: true}, // panicFetcher panics on the first request
	}
	for _, c := range cases {
		t.Run(c.outcome, func(t *testing.T) {
			conn, err := db.Open(filepath.Join(t.TempDir(), "queue.db"))
			if err != nil {
				t.Fatalf("open db: %v", err)
			}
			t.Cleanup(func() { conn.Close() })
			jobs, players := db.NewJobStore(conn), db.NewPlayerStore(conn)
			deps := scraper.ScrapeDeps{Fetcher: panicFetcher{}, Cache: cache.New(), ProxyURL: func(u string) string { return u }, Tournaments: db.NewTournamentStore(conn)}
			q := jobqueue.New(jobs, db.NewJobStatsStore(conn), players, deps, db.NewPermissionStore(conn), c.respectPermission, time.Now)

			if c.seedPlayer {
				if err := players.Upsert(db.Player{ID: "PZ1", Name: "Some Player", RGX: 1200}); err != nil {
					t.Fatalf("seed player: %v", err)
				}
			}
			job, err := jobs.Create("PZ1", "2026-08-16", time.Now())
			if err != nil {
				t.Fatalf("create job: %v", err)
			}

			counter := metrics.JobsFinished.WithLabelValues(c.outcome)
			before := testutil.ToFloat64(counter)
			q.Start()
			q.Enqueue(job.ID)

			deadline := time.Now().Add(2 * time.Second)
			for testutil.ToFloat64(counter) == before && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if got := testutil.ToFloat64(counter) - before; got != 1 {
				t.Fatalf("rgblitz_jobs_finished_total{outcome=%q} grew by %v, want 1", c.outcome, got)
			}
		})
	}
}

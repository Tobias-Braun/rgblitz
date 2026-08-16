package jobqueue_test

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/jobqueue"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// panicFetcher simulates an unexpected bug anywhere in the scrape pipeline
// (e.g. a parser indexing an empty slice on an unusual page layout) by
// panicking on the very first outbound request.
type panicFetcher struct{}

func (panicFetcher) Fetch(path string) (io.ReadCloser, error) {
	panic("simulated parser bug for path " + path)
}

func newTestQueue(t *testing.T) (*jobqueue.Queue, *db.JobStore, *db.PlayerStore) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	players := db.NewPlayerStore(conn)
	jobs := db.NewJobStore(conn)
	stats := db.NewJobStatsStore(conn)
	tournaments := db.NewTournamentStore(conn)

	deps := scraper.ScrapeDeps{
		Fetcher:     panicFetcher{},
		Cache:       cache.New(),
		ProxyURL:    func(u string) string { return u },
		Tournaments: tournaments,
	}

	q := jobqueue.New(jobs, stats, players, deps, nil, false, time.Now)
	return q, jobs, players
}

// TestProcess_PanicDuringScrapeMarksJobAsError proves the jobqueue worker
// survives a panic anywhere in the scrape pipeline: the job must end up in a
// terminal "error" state (never left "running", which would make it eligible
// for db.JobStore.ResetStuckRunning to re-enqueue and panic again on restart),
// and — critically — the panic must not escape process() and crash the
// worker goroutine (which would silently stop the queue for every other job
// too, since jobqueue.Queue has only one worker).
func TestProcess_PanicDuringScrapeMarksJobAsError(t *testing.T) {
	q, jobs, players := newTestQueue(t)

	if err := players.Upsert(db.Player{ID: "PZ1", Name: "Panic Player", RGX: 1200}); err != nil {
		t.Fatalf("seed player: %v", err)
	}
	job, err := jobs.Create("PZ1", "2026-08-16", time.Now())
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	q.Start()
	q.Enqueue(job.ID)

	var final *db.Job
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		final, err = jobs.GetByID(job.ID)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if final.Status == db.JobStatusDone || final.Status == db.JobStatusError {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if final == nil || final.Status != db.JobStatusError {
		t.Fatalf("job status = %+v, want a terminal %q status after a panicking scrape", final, db.JobStatusError)
	}
	if !strings.Contains(final.Error, "simulated parser bug") {
		t.Errorf("job error = %q, want it to mention the panic value", final.Error)
	}

	// The worker goroutine must still be alive: a second job queued after the
	// panic has to be processed normally, proving the panic didn't take down
	// the single background worker for good.
	job2, err := jobs.Create("PZ1", "2026-08-16", time.Now())
	if err != nil {
		t.Fatalf("create second job: %v", err)
	}
	q.Enqueue(job2.ID)

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		final, err = jobs.GetByID(job2.ID)
		if err != nil {
			t.Fatalf("get second job: %v", err)
		}
		if final.Status == db.JobStatusDone || final.Status == db.JobStatusError {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if final == nil || final.Status != db.JobStatusError {
		t.Fatalf("second job status = %+v, want the worker to still be processing jobs after the earlier panic", final)
	}
}

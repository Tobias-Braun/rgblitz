package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

func newTestStatsStore(t *testing.T) *db.JobStatsStore {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return db.NewJobStatsStore(conn)
}

func TestJobStatsStore_TotalsAcrossLifecycle(t *testing.T) {
	store := newTestStatsStore(t)
	now := time.Now()

	if err := store.Create(1, "PLAYERA", now); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.Create(2, "PLAYERB", now); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.Create(3, "PLAYERC", now); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Job 1 succeeds and reports the tournament it resolved to.
	if err := store.MarkDone(1, 692, now.Add(time.Minute)); err != nil {
		t.Fatalf("mark done: %v", err)
	}
	// Job 2 fails.
	if err := store.MarkError(2, now.Add(time.Minute)); err != nil {
		t.Fatalf("mark error: %v", err)
	}
	// Job 3 is left queued (in flight) — must not count as succeeded or failed.

	totals, err := store.Totals()
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if totals.Created != 3 {
		t.Errorf("Created = %d, want 3", totals.Created)
	}
	if totals.Succeeded != 1 {
		t.Errorf("Succeeded = %d, want 1", totals.Succeeded)
	}
	if totals.Failed != 1 {
		t.Errorf("Failed = %d, want 1", totals.Failed)
	}
}

func TestJobStatsStore_TotalsEmpty(t *testing.T) {
	store := newTestStatsStore(t)

	totals, err := store.Totals()
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if totals != (db.JobStatsTotals{}) {
		t.Errorf("totals on empty table = %+v, want zero value", totals)
	}
}

// TestJobStatsStore_PersistsIndependentlyOfRgxJobs proves the core design
// requirement: rgx_jobs_stats rows survive even though they're keyed by a
// rgx_jobs row's ID, which the daily cron purge (JobStore.DeleteAll) deletes.
func TestJobStatsStore_PersistsIndependentlyOfRgxJobs(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "combined.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	jobs := db.NewJobStore(conn)
	stats := db.NewJobStatsStore(conn)
	now := time.Now()

	job, err := jobs.Create("PLAYERA", "2026-08-01", now)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := stats.Create(job.ID, "PLAYERA", now); err != nil {
		t.Fatalf("create stats: %v", err)
	}
	if err := stats.MarkDone(job.ID, 692, now); err != nil {
		t.Fatalf("mark done: %v", err)
	}

	if err := jobs.DeleteAll(); err != nil {
		t.Fatalf("delete all rgx_jobs: %v", err)
	}

	totals, err := stats.Totals()
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if totals.Succeeded != 1 {
		t.Errorf("Succeeded after rgx_jobs purge = %d, want 1", totals.Succeeded)
	}
}

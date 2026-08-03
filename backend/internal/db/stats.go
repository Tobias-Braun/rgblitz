package db

import (
	"database/sql"
	"fmt"
	"time"
)

// JobStatsStore records permanent usage statistics for RgxJobs — one row per
// genuinely created job, never for a job reused from the freshness/dedup cache
// in TodayResult. Unlike rgx_jobs (purged daily, see internal/cron), rows here
// are never deleted, so all-time totals survive the cleanup.
type JobStatsStore struct {
	db *sql.DB
}

func NewJobStatsStore(db *sql.DB) *JobStatsStore {
	return &JobStatsStore{db: db}
}

// Create records that a new job was created. Call this only when
// JobStore.Create actually inserted a new rgx_jobs row — never for a cached
// result served without creating one.
func (s *JobStatsStore) Create(jobID int64, playerID string, createdAt time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO rgx_jobs_stats (job_id, player_id, status, created_at)
		VALUES (?, ?, ?, ?)
	`, jobID, playerID, JobStatusQueued, createdAt)
	if err != nil {
		return fmt.Errorf("create job stats: %w", err)
	}
	return nil
}

// MarkDone records that job jobID finished successfully, along with the
// tournament it was found to be part of. tournamentID of 0 (unknown, e.g. a
// job that resolved to no_tournament) is stored as NULL.
func (s *JobStatsStore) MarkDone(jobID int64, tournamentID int, finishedAt time.Time) error {
	var tid sql.NullInt64
	if tournamentID > 0 {
		tid = sql.NullInt64{Int64: int64(tournamentID), Valid: true}
	}
	_, err := s.db.Exec(`
		UPDATE rgx_jobs_stats SET status = ?, tournament_id = ?, finished_at = ? WHERE job_id = ?
	`, JobStatusDone, tid, finishedAt, jobID)
	if err != nil {
		return fmt.Errorf("mark job stats done: %w", err)
	}
	return nil
}

// MarkError records that job jobID failed.
func (s *JobStatsStore) MarkError(jobID int64, finishedAt time.Time) error {
	_, err := s.db.Exec(`
		UPDATE rgx_jobs_stats SET status = ?, finished_at = ? WHERE job_id = ?
	`, JobStatusError, finishedAt, jobID)
	if err != nil {
		return fmt.Errorf("mark job stats error: %w", err)
	}
	return nil
}

// JobStatsTotals is the all-time job count summary returned by Totals.
type JobStatsTotals struct {
	Created   int
	Succeeded int
	Failed    int
}

// Totals aggregates all-time counts across every job ever created. Backed by
// an index on status, so this stays cheap even as the table grows — if it
// ever doesn't, replace it with running counters updated alongside Create/
// MarkDone/MarkError instead of re-aggregating the whole table.
func (s *JobStatsStore) Totals() (JobStatsTotals, error) {
	var t JobStatsTotals
	err := s.db.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0)
		FROM rgx_jobs_stats
	`, JobStatusDone, JobStatusError).Scan(&t.Created, &t.Succeeded, &t.Failed)
	if err != nil {
		return JobStatsTotals{}, fmt.Errorf("job stats totals: %w", err)
	}
	return t, nil
}

package db

import (
	"database/sql"
	"fmt"
	"time"
)

const (
	JobStatusQueued  = "queued"
	JobStatusRunning = "running"
	JobStatusDone    = "done"
	JobStatusError   = "error"
)

// Job is a persisted RGX scrape request for a single player on a single
// tournament day. Rows are purged daily (see internal/cron).
type Job struct {
	ID         int64
	PlayerID   string
	Date       string // YYYY-MM-DD, Europe/Berlin
	Status     string
	ResultJSON string
	Error      string
	QueuedAt   time.Time
	StartedAt  sql.NullTime
	FinishedAt sql.NullTime
}

type JobStore struct {
	db *sql.DB
}

func NewJobStore(db *sql.DB) *JobStore {
	return &JobStore{db: db}
}

// Create inserts a new queued job and returns it with its assigned ID.
func (s *JobStore) Create(playerID, date string, queuedAt time.Time) (*Job, error) {
	res, err := s.db.Exec(`
		INSERT INTO rgx_jobs (player_id, date, status, queued_at)
		VALUES (?, ?, ?, ?)
	`, playerID, date, JobStatusQueued, queuedAt)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	return &Job{ID: id, PlayerID: playerID, Date: date, Status: JobStatusQueued, QueuedAt: queuedAt}, nil
}

func (s *JobStore) GetByID(id int64) (*Job, error) {
	return s.scanOne(s.db.QueryRow(`
		SELECT id, player_id, date, status, result_json, error, queued_at, started_at, finished_at
		FROM rgx_jobs WHERE id = ?
	`, id))
}

// LatestForPlayerDate returns the most recently queued job for a player on a
// given day, or nil if none exists.
func (s *JobStore) LatestForPlayerDate(playerID, date string) (*Job, error) {
	return s.scanOne(s.db.QueryRow(`
		SELECT id, player_id, date, status, result_json, error, queued_at, started_at, finished_at
		FROM rgx_jobs WHERE player_id = ? AND date = ?
		ORDER BY queued_at DESC, id DESC
		LIMIT 1
	`, playerID, date))
}

func (s *JobStore) scanOne(row *sql.Row) (*Job, error) {
	j := &Job{}
	var resultJSON, errMsg sql.NullString
	err := row.Scan(&j.ID, &j.PlayerID, &j.Date, &j.Status, &resultJSON, &errMsg,
		&j.QueuedAt, &j.StartedAt, &j.FinishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	j.ResultJSON = resultJSON.String
	j.Error = errMsg.String
	return j, nil
}

func (s *JobStore) MarkRunning(id int64, startedAt time.Time) error {
	_, err := s.db.Exec(`UPDATE rgx_jobs SET status = ?, started_at = ? WHERE id = ?`, JobStatusRunning, startedAt, id)
	return err
}

func (s *JobStore) MarkDone(id int64, resultJSON string, finishedAt time.Time) error {
	_, err := s.db.Exec(`UPDATE rgx_jobs SET status = ?, result_json = ?, finished_at = ? WHERE id = ?`,
		JobStatusDone, resultJSON, finishedAt, id)
	return err
}

func (s *JobStore) MarkError(id int64, errMsg string, finishedAt time.Time) error {
	_, err := s.db.Exec(`UPDATE rgx_jobs SET status = ?, error = ?, finished_at = ? WHERE id = ?`,
		JobStatusError, errMsg, finishedAt, id)
	return err
}

// CountQueuedBefore returns how many queued jobs are ahead of the given job
// in FIFO order — the 1-based position of job itself is this count + 1.
func (s *JobStore) CountQueuedBefore(queuedAt time.Time, id int64) (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM rgx_jobs
		WHERE status = ? AND (queued_at < ? OR (queued_at = ? AND id < ?))
	`, JobStatusQueued, queuedAt, queuedAt, id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count queued jobs: %w", err)
	}
	return n, nil
}

// DeleteAll purges every job row. Run once a day (see internal/cron).
func (s *JobStore) DeleteAll() error {
	_, err := s.db.Exec(`DELETE FROM rgx_jobs`)
	return err
}

// ResetStuckRunning flips any job left in "running" state (e.g. after a
// server crash mid-scrape) back to "queued" so it gets re-enqueued and
// returns their IDs in FIFO order.
func (s *JobStore) ResetStuckRunning() ([]int64, error) {
	rows, err := s.db.Query(`SELECT id FROM rgx_jobs WHERE status = ? ORDER BY queued_at ASC, id ASC`, JobStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("find stuck jobs: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if _, err := s.db.Exec(`UPDATE rgx_jobs SET status = ? WHERE status = ?`, JobStatusQueued, JobStatusRunning); err != nil {
		return nil, fmt.Errorf("reset stuck jobs: %w", err)
	}
	return ids, nil
}

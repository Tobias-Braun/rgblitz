// Package jobqueue runs RGX scrapes for /api/players/{id}/today off the HTTP
// request path. A single background worker drains queued db.Job rows in FIFO
// order, so playerzone requests stay serialized through the same shared
// scraper.Fetcher/rate limiter as before — the worker loop itself is now the
// "global scraper lock".
package jobqueue

import (
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// Queue enqueues job IDs for a single background worker to process.
type Queue struct {
	jobs              *db.JobStore
	stats             *db.JobStatsStore
	players           *db.PlayerStore
	deps              scraper.ScrapeDeps
	permissions       *db.PermissionStore
	respectPermission bool
	now               func() time.Time
	ch                chan int64
}

func New(jobs *db.JobStore, stats *db.JobStatsStore, players *db.PlayerStore, deps scraper.ScrapeDeps, permissions *db.PermissionStore, respectPermission bool, now func() time.Time) *Queue {
	return &Queue{
		jobs:              jobs,
		stats:             stats,
		players:           players,
		deps:              deps,
		permissions:       permissions,
		respectPermission: respectPermission,
		now:               now,
		ch:                make(chan int64, 256),
	}
}

// Start launches the single background worker goroutine. Call once at
// startup, after re-enqueueing any jobs recovered via db.JobStore.ResetStuckRunning.
func (q *Queue) Start() {
	go q.run()
}

// Enqueue schedules a queued job for processing. Non-blocking as long as the
// queue isn't absurdly backed up (buffered channel of 256 job IDs).
func (q *Queue) Enqueue(jobID int64) {
	q.ch <- jobID
}

func (q *Queue) run() {
	for jobID := range q.ch {
		q.process(jobID)
	}
}

// process runs one job end-to-end. A top-level recover guards the whole
// function: an unrecovered panic anywhere in the scrape path (e.g. a parser
// bug triggered by an unexpected page layout) would otherwise crash the
// entire process, since Go terminates the whole program on an unrecovered
// goroutine panic — not just this one. On restart, db.JobStore.ResetStuckRunning
// would then re-enqueue the same job, which would panic again on the same
// input, crash-looping the server indefinitely. Recovering here instead marks
// the job as a normal, terminal "error" — it's never left in "running" state,
// so it's never picked up by ResetStuckRunning and never retried.
func (q *Queue) process(jobID int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("jobqueue: recovered panic processing job %d: %v\n%s", jobID, r, debug.Stack())
			q.fail(jobID, fmt.Sprintf("internal error (panic): %v", r))
		}
	}()

	job, err := q.jobs.GetByID(jobID)
	if err != nil || job == nil {
		log.Printf("jobqueue: load job %d: %v", jobID, err)
		return
	}

	if err := q.jobs.MarkRunning(jobID, q.now()); err != nil {
		log.Printf("jobqueue: mark job %d running: %v", jobID, err)
	}

	if !q.permissions.Permitted(q.respectPermission) {
		q.fail(jobID, "playerzone scraping is currently disallowed by robots.txt")
		return
	}

	player, err := q.players.GetByID(job.PlayerID)
	if err != nil {
		q.fail(jobID, fmt.Sprintf("load player: %v", err))
		return
	}
	if player == nil {
		q.fail(jobID, "player not found")
		return
	}

	// Scrape as of when the job was queued (the original request's clock), not
	// whenever the worker happens to get around to it — a job processed a few
	// seconds late (or, in a test, with a frozen/overridden clock) must still
	// compute "today" the way the requester saw it.
	result, tournamentID, err := scraper.ScrapePlayerToday(q.deps, player, job.QueuedAt)
	if err != nil {
		q.fail(jobID, err.Error())
		return
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		q.fail(jobID, fmt.Sprintf("encode result: %v", err))
		return
	}

	now := q.now()
	if err := q.jobs.MarkDone(jobID, string(resultJSON), now); err != nil {
		log.Printf("jobqueue: mark job %d done: %v", jobID, err)
	}
	if q.stats != nil {
		if err := q.stats.MarkDone(jobID, tournamentID, now); err != nil {
			log.Printf("jobqueue: mark job stats %d done: %v", jobID, err)
		}
	}
}

func (q *Queue) fail(jobID int64, msg string) {
	log.Printf("jobqueue: job %d failed: %s", jobID, msg)
	now := q.now()
	if err := q.jobs.MarkError(jobID, msg, now); err != nil {
		log.Printf("jobqueue: mark job %d error: %v", jobID, err)
	}
	if q.stats != nil {
		if err := q.stats.MarkError(jobID, now); err != nil {
			log.Printf("jobqueue: mark job stats %d error: %v", jobID, err)
		}
	}
}

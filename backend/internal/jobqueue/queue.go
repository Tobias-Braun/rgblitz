// Package jobqueue runs RGX scrapes for /api/players/{id}/today off the HTTP
// request path. A single background worker drains queued db.Job rows in FIFO
// order, so playerzone requests stay serialized through the same shared
// scraper.Fetcher/rate limiter as before — the worker loop itself is now the
// "global scraper lock".
package jobqueue

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/metrics"
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
	metrics.JobQueueDepth.Set(float64(len(q.ch)))
}

func (q *Queue) run() {
	for jobID := range q.ch {
		metrics.JobQueueDepth.Set(float64(len(q.ch)))
		q.process(jobID)
	}
}

// jobRun is what the job metrics and the "rgx job finished" log line need to
// know about a job beyond its outcome.
type jobRun struct {
	id      int64
	started time.Time
	wait    time.Duration
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
	run := jobRun{id: jobID, started: q.now()}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("jobqueue: recovered panic", "job_id", jobID, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			q.fail(run, "panic", fmt.Sprintf("internal error (panic): %v", r))
		}
	}()

	job, err := q.jobs.GetByID(jobID)
	if err != nil || job == nil {
		slog.Error("jobqueue: load job", "job_id", jobID, "error", err)
		return
	}

	run.wait = run.started.Sub(job.QueuedAt)
	metrics.JobQueueWait.Observe(run.wait.Seconds())
	if err := q.jobs.MarkRunning(jobID, run.started); err != nil {
		slog.Error("jobqueue: mark job running", "job_id", jobID, "error", err)
	}

	if !q.permissions.Permitted(q.respectPermission) {
		q.fail(run, "robots_denied", "playerzone scraping is currently disallowed by robots.txt")
		return
	}

	player, err := q.players.GetByID(job.PlayerID)
	if err != nil {
		q.fail(run, "db", fmt.Sprintf("load player: %v", err))
		return
	}
	if player == nil {
		q.fail(run, "player_not_found", "player not found")
		return
	}

	// Scrape as of when the job was queued (the original request's clock), not
	// whenever the worker happens to get around to it — a job processed a few
	// seconds late (or, in a test, with a frozen/overridden clock) must still
	// compute "today" the way the requester saw it.
	result, tournamentID, handled, err := scraper.ScrapePlayerLeagueToday(q.deps, player, job.QueuedAt)
	if err == nil && !handled {
		result, tournamentID, err = scraper.ScrapePlayerToday(q.deps, player, job.QueuedAt)
	}
	if err != nil {
		q.fail(run, "scrape", err.Error())
		return
	}

	q.done(run, result, tournamentID)
}

func (q *Queue) done(run jobRun, result scraper.TodayResult, tournamentID int) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		q.fail(run, "encode", fmt.Sprintf("encode result: %v", err))
		return
	}

	jobID := run.id
	now := q.now()
	if err := q.jobs.MarkDone(jobID, string(resultJSON), now); err != nil {
		slog.Error("jobqueue: mark job done", "job_id", jobID, "error", err)
	}
	if q.stats != nil {
		if err := q.stats.MarkDone(jobID, tournamentID, now); err != nil {
			slog.Error("jobqueue: mark job stats done", "job_id", jobID, "error", err)
		}
	}

	for _, g := range result.Games {
		metrics.Games.WithLabelValues(g.Result).Inc()
		metrics.RGXGap.Observe(float64(g.OpponentRGX - g.TeamRGX))
	}
	if result.Status == "ok" {
		metrics.RGXTotalDelta.Observe(float64(result.TotalDelta))
	}
	q.finished(run, result.Status, now, "tournament", q.tournamentLabel(tournamentID),
		"division", result.Division, "games", len(result.Games), "total_delta", result.TotalDelta)
}

func (q *Queue) fail(run jobRun, outcome, msg string) {
	now := q.now()
	if err := q.jobs.MarkError(run.id, msg, now); err != nil {
		slog.Error("jobqueue: mark job error", "job_id", run.id, "error", err)
	}
	if q.stats != nil {
		if err := q.stats.MarkError(run.id, now); err != nil {
			slog.Error("jobqueue: mark job stats error", "job_id", run.id, "error", err)
		}
	}
	q.finished(run, outcome, now, "error", msg)
}

// finished records a job's terminal outcome: its metrics plus exactly one
// "rgx job finished" log line. Player names are never logged.
func (q *Queue) finished(run jobRun, outcome string, at time.Time, attrs ...any) {
	duration := at.Sub(run.started)
	metrics.JobsFinished.WithLabelValues(outcome).Inc()
	metrics.JobDuration.Observe(duration.Seconds())
	slog.Info("rgx job finished", append([]any{"job_id", run.id, "outcome", outcome,
		"wait_seconds", run.wait.Seconds(), "duration_seconds", duration.Seconds()}, attrs...)...)
}

// tournamentLabel names a tournament for logs: its stored name if known, else
// its ID, or "" for none.
func (q *Queue) tournamentLabel(id int) string {
	if id == 0 {
		return ""
	}
	if q.deps.Tournaments != nil {
		if t, err := q.deps.Tournaments.Get(id); err == nil && t != nil && t.Name != "" {
			return t.Name
		}
	}
	return strconv.Itoa(id)
}

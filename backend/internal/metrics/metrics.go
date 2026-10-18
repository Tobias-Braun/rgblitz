// Package metrics holds every Prometheus metric the backend exports. The
// metrics are package-level promauto vars on the default registry, so they're
// registered exactly once per process no matter how often routers, queues or
// schedulers are constructed (tests build many). The Grafana dashboards in
// vps-infra depend on these exact names — don't rename them.
package metrics

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rgblitz_http_requests_total",
		Help: "HTTP requests served, by chi route pattern, method and status code.",
	}, []string{"route", "method", "status"})
	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "rgblitz_http_request_duration_seconds",
		Help:    "HTTP request latency, by chi route pattern and method.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route", "method"})

	TodayRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rgblitz_today_requests_total",
		Help: "GET /api/players/{id}/today requests, by which branch answered them.",
	}, []string{"outcome"})

	JobsFinished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rgblitz_jobs_finished_total",
		Help: "RGX jobs that reached a terminal state, by outcome.",
	}, []string{"outcome"})
	JobQueueWait = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "rgblitz_job_queue_wait_seconds",
		Help:    "Time an RGX job spent queued before the worker picked it up.",
		Buckets: prometheus.ExponentialBuckets(0.5, 2, 10),
	})
	// A job makes several rate-limited (2s) playerzone requests, so it
	// routinely takes tens of seconds and can take minutes under contention.
	JobDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "rgblitz_job_duration_seconds",
		Help:    "Time the worker spent processing one RGX job.",
		Buckets: []float64{1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 180, 240, 300},
	})
	JobQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rgblitz_job_queue_depth",
		Help: "RGX jobs waiting in the worker's queue.",
	})

	PlayerzoneRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rgblitz_playerzone_requests_total",
		Help: "Outbound playerzone requests, by page type and HTTP status (or \"error\").",
	}, []string{"page", "status"})
	PlayerzoneDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "rgblitz_playerzone_request_duration_seconds",
		Help:    "Outbound playerzone request latency, excluding rate-limit wait.",
		Buckets: prometheus.DefBuckets,
	}, []string{"page"})
	RateLimitWait = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "rgblitz_playerzone_ratelimit_wait_seconds",
		Help:    "Time spent blocked in the shared playerzone rate limiter.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8, 15, 30, 60},
	})
	ScraperCache = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rgblitz_scraper_cache_total",
		Help: "In-memory scraper cache lookups, by cache and hit/miss.",
	}, []string{"cache", "result"})

	RGXTotalDelta = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "rgblitz_rgx_total_delta",
		Help:    "Total RGX delta of each successfully finished job.",
		Buckets: prometheus.LinearBuckets(-100, 20, 11),
	})
	RGXGap = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "rgblitz_rgx_gap",
		Help:    "Opponent RGX minus team RGX, per game of a finished job.",
		Buckets: prometheus.LinearBuckets(-600, 100, 13),
	})
	Games = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rgblitz_games_total",
		Help: "Games in finished jobs, by result (W, L, D, C).",
	}, []string{"result"})

	IndexScrapeRuns = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rgblitz_index_scrape_runs_total",
		Help: "Player index scrape runs, by outcome.",
	}, []string{"outcome"})
	IndexLastSuccess = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rgblitz_index_last_success_timestamp_seconds",
		Help: "Unix time of the last successful player index scrape.",
	})
	IndexScrapeDuration = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rgblitz_index_scrape_duration_seconds",
		Help: "Duration of the last player index scrape run.",
	})

	FeedbackSubmitted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rgblitz_feedback_submitted_total",
		Help: "Feedback submissions, by star rating.",
	}, []string{"rating"})
)

// Pre-creating every known label value makes each series exist (at 0) from
// startup, so dashboards show 0 instead of "no data" and rate() works from
// the first increment.
func init() {
	for _, o := range []string{"weekday", "early", "in_flight", "cache_hit", "robots_denied", "new_job", "dev"} {
		TodayRequests.WithLabelValues(o)
	}
	for _, o := range []string{"ok", "no_tournament", "robots_denied", "player_not_found", "db", "scrape", "encode", "panic"} {
		JobsFinished.WithLabelValues(o)
	}
	for _, c := range []string{"profile", "divisions", "division_teams"} {
		ScraperCache.WithLabelValues(c, "hit")
		ScraperCache.WithLabelValues(c, "miss")
	}
	for _, r := range []string{"W", "L", "D", "C"} {
		Games.WithLabelValues(r)
	}
	for _, o := range []string{"ok", "robots_denied", "error"} {
		IndexScrapeRuns.WithLabelValues(o)
	}
	for rating := 1; rating <= 5; rating++ {
		FeedbackSubmitted.WithLabelValues(strconv.Itoa(rating))
	}
}

// Handler serves /metrics only; it's meant for its own listener, never the
// public router.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

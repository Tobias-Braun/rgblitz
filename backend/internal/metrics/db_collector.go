package metrics

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

var (
	playersIndexedDesc  = prometheus.NewDesc("rgblitz_players_indexed", "Players in the search index.", nil, nil)
	newestScrapeDesc    = prometheus.NewDesc("rgblitz_index_newest_scrape_timestamp_seconds", "Unix time of the most recently scraped player in the index.", nil, nil)
	jobsAllTimeDesc     = prometheus.NewDesc("rgblitz_jobs_all_time", "All-time RGX jobs from rgx_jobs_stats, by state.", []string{"state"}, nil)
	uniquePlayersDesc   = prometheus.NewDesc("rgblitz_unique_players_served", "Distinct players any RGX job was ever created for.", nil, nil)
	tournamentsDesc     = prometheus.NewDesc("rgblitz_tournaments_known", "Tournaments classified so far, by GRT status.", []string{"grt"}, nil)
	feedbackTotalDesc   = prometheus.NewDesc("rgblitz_feedback_total", "Feedback rows stored.", nil, nil)
	feedbackAvgDesc     = prometheus.NewDesc("rgblitz_feedback_rating_avg", "Average feedback star rating (0 if none).", nil, nil)
	scrapePermittedDesc = prometheus.NewDesc("rgblitz_scrape_permitted", "1 if live playerzone requests are currently permitted, else 0.", nil, nil)
)

// DBCollector reads its metrics from SQLite at scrape time, so they survive
// restarts and reflect rows written by any process. Every query is a cheap
// aggregate over a small table; a failing one is logged and its metric left
// out rather than failing the whole scrape.
type DBCollector struct {
	Players           *db.PlayerStore
	Stats             *db.JobStatsStore
	Tournaments       *db.TournamentStore
	Feedback          *db.FeedbackStore
	Permissions       *db.PermissionStore
	RespectPermission bool
}

func (c *DBCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{playersIndexedDesc, newestScrapeDesc, jobsAllTimeDesc, uniquePlayersDesc,
		tournamentsDesc, feedbackTotalDesc, feedbackAvgDesc, scrapePermittedDesc} {
		ch <- d
	}
}

func (c *DBCollector) Collect(ch chan<- prometheus.Metric) {
	gauge := func(d *prometheus.Desc, v float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, labels...)
	}

	if n, err := c.Players.Count(); ok(err, "players_indexed") {
		gauge(playersIndexedDesc, float64(n))
	}
	if t, err := c.Players.NewestScrapedAt(); ok(err, "index_newest_scrape") && !t.IsZero() {
		gauge(newestScrapeDesc, float64(t.Unix()))
	}
	if t, err := c.Stats.Totals(); ok(err, "jobs_all_time") {
		gauge(jobsAllTimeDesc, float64(t.Created), "created")
		gauge(jobsAllTimeDesc, float64(t.Succeeded), "succeeded")
		gauge(jobsAllTimeDesc, float64(t.Failed), "failed")
	}
	if n, err := c.Stats.UniquePlayers(); ok(err, "unique_players_served") {
		gauge(uniquePlayersDesc, float64(n))
	}
	if grt, other, err := c.Tournaments.CountByGRT(); ok(err, "tournaments_known") {
		gauge(tournamentsDesc, float64(grt), "true")
		gauge(tournamentsDesc, float64(other), "false")
	}
	if n, avg, err := c.Feedback.Summary(); ok(err, "feedback") {
		ch <- prometheus.MustNewConstMetric(feedbackTotalDesc, prometheus.CounterValue, float64(n))
		gauge(feedbackAvgDesc, avg)
	}
	permitted := 0.0
	if c.Permissions.Permitted(c.RespectPermission) {
		permitted = 1
	}
	gauge(scrapePermittedDesc, permitted)
}

func ok(err error, metric string) bool {
	if err != nil {
		slog.Warn("metrics: db query failed", "metric", metric, "error", err)
	}
	return err == nil
}

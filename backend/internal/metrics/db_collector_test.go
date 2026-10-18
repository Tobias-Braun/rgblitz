package metrics_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/metrics"
)

func TestDBCollector(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	players := db.NewPlayerStore(conn)
	stats := db.NewJobStatsStore(conn)
	tournaments := db.NewTournamentStore(conn)
	feedback := db.NewFeedbackStore(conn)
	permissions := db.NewPermissionStore(conn)

	newest := time.Date(2026, 10, 10, 3, 0, 0, 0, time.UTC)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(players.Upsert(db.Player{ID: "A", Name: "A", ScrapedAt: newest.Add(-time.Hour)}))
	must(players.Upsert(db.Player{ID: "B", Name: "B", ScrapedAt: newest}))
	must(stats.Create(1, "A", newest))
	must(stats.Create(2, "A", newest))
	must(stats.Create(3, "B", newest))
	must(stats.MarkDone(1, 0, newest))
	must(stats.MarkError(2, newest))
	must(tournaments.Create(db.Tournament{ID: 1, Name: "GRT 1", IsGRT: true, ScrapedAt: newest}))
	must(tournaments.Create(db.Tournament{ID: 2, Name: "GRT 2", IsGRT: true, ScrapedAt: newest}))
	must(tournaments.Create(db.Tournament{ID: 3, Name: "EURA", IsGRT: false, ScrapedAt: newest}))
	_, err = feedback.Create(4, "", newest)
	must(err)
	_, err = feedback.Create(5, "", newest)
	must(err)
	must(permissions.Set(true, newest))

	c := &metrics.DBCollector{Players: players, Stats: stats, Tournaments: tournaments,
		Feedback: feedback, Permissions: permissions, RespectPermission: true}

	want := `
# HELP rgblitz_feedback_rating_avg Average feedback star rating (0 if none).
# TYPE rgblitz_feedback_rating_avg gauge
rgblitz_feedback_rating_avg 4.5
# HELP rgblitz_feedback_total Feedback rows stored.
# TYPE rgblitz_feedback_total counter
rgblitz_feedback_total 2
# HELP rgblitz_index_newest_scrape_timestamp_seconds Unix time of the most recently scraped player in the index.
# TYPE rgblitz_index_newest_scrape_timestamp_seconds gauge
rgblitz_index_newest_scrape_timestamp_seconds 1.7916012e+09
# HELP rgblitz_jobs_all_time All-time RGX jobs from rgx_jobs_stats, by state.
# TYPE rgblitz_jobs_all_time gauge
rgblitz_jobs_all_time{state="created"} 3
rgblitz_jobs_all_time{state="failed"} 1
rgblitz_jobs_all_time{state="succeeded"} 1
# HELP rgblitz_players_indexed Players in the search index.
# TYPE rgblitz_players_indexed gauge
rgblitz_players_indexed 2
# HELP rgblitz_scrape_permitted 1 if live playerzone requests are currently permitted, else 0.
# TYPE rgblitz_scrape_permitted gauge
rgblitz_scrape_permitted 1
# HELP rgblitz_tournaments_known Tournaments classified so far, by GRT status.
# TYPE rgblitz_tournaments_known gauge
rgblitz_tournaments_known{grt="false"} 1
rgblitz_tournaments_known{grt="true"} 2
# HELP rgblitz_unique_players_served Distinct players any RGX job was ever created for.
# TYPE rgblitz_unique_players_served gauge
rgblitz_unique_players_served 2
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want)); err != nil {
		t.Error(err)
	}
	if lint, err := testutil.CollectAndLint(c); err != nil || len(lint) > 0 {
		t.Errorf("lint problems: %v %v", lint, err)
	}
}

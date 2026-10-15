package api_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/api"
	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/config"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// upcomingHTML mirrors the card markup of /tournaments/upcoming, plus a nav-dropdown link
// for a tournament that is already running.
const upcomingHTML = `<html><body>
<a href="/tournaments/600-2026-10-17-running-cup">Running Cup</a>
<rg-expandable-card onclick="display_tournament_body(702)" title="Late Cup" subtitle="07.11.2026 - 08.11.2026, Kiel"></rg-expandable-card>
<rg-expandable-card onclick="display_tournament_body(701)" title="Early Cup" subtitle="24.10.2026, Utrecht"></rg-expandable-card>
<rg-expandable-card onclick="display_tournament_body(700)" title="Past Cup" subtitle="10.10.2026, Bern"></rg-expandable-card>
</body></html>`

// upcomingNow is a Thursday after Past Cup and before every other card.
var upcomingNow = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

type stubFetcher struct {
	body string
	err  error
}

func (f stubFetcher) Fetch(string) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.body)), nil
}

// newUpcomingHandler builds a Handler with only what UpcomingTournaments needs — unlike
// newTestHandler it doesn't depend on the gitignored ranking fixture.
func newUpcomingHandler(t *testing.T, fetcher scraper.Fetcher, cfg *config.Config) (*api.Handler, *db.PermissionStore) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	permissions := db.NewPermissionStore(conn)
	h := api.NewHandler(cfg, nil, cache.New(), fetcher, permissions, nil, nil, nil, nil, fakeFeedbackNotifier{})
	h.Now = func() time.Time { return upcomingNow }
	return h, permissions
}

func getUpcoming(t *testing.T, h *api.Handler) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	api.NewRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tournaments/upcoming", nil))
	return rec
}

func TestUpcomingTournaments_FiltersSortsAndCaches(t *testing.T) {
	fetcher := &countingFetcher{inner: stubFetcher{body: upcomingHTML}}
	h, _ := newUpcomingHandler(t, fetcher, &config.Config{})

	rec := getUpcoming(t, h)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []scraper.TournamentListEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []scraper.TournamentListEntry{
		{ID: 701, Name: "Early Cup", Location: "Utrecht", StartDate: "2026-10-24", EndDate: "2026-10-24"},
		{ID: 702, Name: "Late Cup", Location: "Kiel", StartDate: "2026-11-07", EndDate: "2026-11-08"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	getUpcoming(t, h)
	if fetcher.calls != 1 {
		t.Errorf("fetch calls = %d, want 1 — second request must be served from cache", fetcher.calls)
	}
}

func TestUpcomingTournaments_PermissionDeniedReturnsEmptyList(t *testing.T) {
	h, permissions := newUpcomingHandler(t, noNetworkFetcher{t}, &config.Config{RespectPermission: true})
	if err := permissions.Set(false, time.Now()); err != nil {
		t.Fatalf("seed permission: %v", err)
	}

	rec := getUpcoming(t, h)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("got %d %q, want 200 []", rec.Code, rec.Body.String())
	}
}

func TestUpcomingTournaments_FailuresReturnBadGateway(t *testing.T) {
	for name, fetcher := range map[string]scraper.Fetcher{
		"fetch error":  stubFetcher{err: errors.New("playerzone down")},
		"garbage html": stubFetcher{body: "<html><body><p>maintenance</p></body></html>"},
	} {
		t.Run(name, func(t *testing.T) {
			h, _ := newUpcomingHandler(t, fetcher, &config.Config{})
			if rec := getUpcoming(t, h); rec.Code != http.StatusBadGateway {
				t.Errorf("status = %d, want 502", rec.Code)
			}
		})
	}
}

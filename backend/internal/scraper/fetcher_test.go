package scraper_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgx-calc/internal/scraper"
)

func TestFixtureFetcher_RoutesKnownPaths(t *testing.T) {
	cases := []struct {
		path         string
		wantFile     string
		wantContains string
	}{
		{"/players/2110", "player-profile.html", ""},
		{"/ranking/rg-index/", "ranking.html", ""},
		{scraper.RankingOpenPath, "ranking.html", ""},
		{scraper.RankingWomenPath, "ranking-women.html", ""},
		{"/tournaments/upcoming", "tournaments-upcoming.html", ""},
		{"/tournaments/schedule/team/60615", "team-schedule.html", ""},
		{"/tournaments/692-2026-07-18-2-leguan-cup/teams/2265", "division-teams.html", ""},
		{"/robots.txt", "robots.txt", ""},
	}

	fetcher := scraper.NewFixtureFetcher("../../../scraper/fixtures")

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			body, err := fetcher.Fetch(tc.path)
			if err != nil {
				t.Fatalf("Fetch(%q): %v", tc.path, err)
			}
			defer body.Close()
			data, err := io.ReadAll(body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if len(data) == 0 {
				t.Errorf("Fetch(%q) returned empty body (expected contents of %s)", tc.path, tc.wantFile)
			}
		})
	}
}

func TestFixtureFetcher_UnknownPath(t *testing.T) {
	fetcher := scraper.NewFixtureFetcher("../../../scraper/fixtures")
	_, err := fetcher.Fetch("/some/unmapped/path")
	if err == nil {
		t.Fatal("expected an error for an unmapped path, got nil")
	}
}

func TestFixtureFetcher_MissingFixtureFile(t *testing.T) {
	fetcher := scraper.NewFixtureFetcher("/nonexistent/directory")
	_, err := fetcher.Fetch("/ranking/rg-index/")
	if err == nil {
		t.Fatal("expected an error when the fixture directory doesn't exist, got nil")
	}
}

func TestHTTPFetcher_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ranking/rg-index/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte("<html>ok</html>"))
	}))
	defer srv.Close()

	fetcher := &scraper.HTTPFetcher{
		BaseURL: srv.URL,
		RL:      scraper.NewRateLimiter(time.Millisecond),
		Client:  srv.Client(),
	}

	body, err := fetcher.Fetch("/ranking/rg-index/")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer body.Close()
	data, _ := io.ReadAll(body)
	if string(data) != "<html>ok</html>" {
		t.Errorf("unexpected body: %q", data)
	}
}

func TestHTTPFetcher_SendsRGBlitzBotUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	fetcher := &scraper.HTTPFetcher{
		BaseURL: srv.URL,
		RL:      scraper.NewRateLimiter(time.Millisecond),
		Client:  srv.Client(),
	}

	body, err := fetcher.Fetch("/")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	body.Close()

	if gotUA != scraper.DefaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, scraper.DefaultUserAgent)
	}
}

func TestHTTPFetcher_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	fetcher := &scraper.HTTPFetcher{
		BaseURL: srv.URL,
		RL:      scraper.NewRateLimiter(time.Millisecond),
		Client:  srv.Client(),
	}

	_, err := fetcher.Fetch("/anything")
	if err == nil {
		t.Fatal("expected an error for a non-200 response, got nil")
	}
}

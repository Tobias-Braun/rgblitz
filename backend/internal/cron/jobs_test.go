package cron_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cron"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// spyFetcher records every path fetched and serves canned bodies.
type spyFetcher struct {
	bodies  map[string]string
	fetched []string
}

func (f *spyFetcher) Fetch(path string) (io.ReadCloser, error) {
	f.fetched = append(f.fetched, path)
	body, ok := f.bodies[path]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

func newTestDB(t *testing.T) *db.PlayerStore {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return db.NewPlayerStore(conn)
}

func newTestJobStore(t *testing.T) *db.JobStore {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return db.NewJobStore(conn)
}

func TestScrapePlayerIndex_RespectsDisallowedRobotsTxt(t *testing.T) {
	players := newTestDB(t)
	conn, _ := db.Open(filepath.Join(t.TempDir(), "test2.db"))
	defer conn.Close()
	permissions := db.NewPermissionStore(conn)

	fetcher := &spyFetcher{bodies: map[string]string{
		"/robots.txt": "User-agent: *\nDisallow: /ranking\nDisallow: /tournaments/*\n",
	}}

	scheduler := cron.NewScheduler(players, newTestJobStore(t), nil, fetcher, permissions, true)
	if err := scheduler.ScrapeNow(); err != nil {
		t.Fatalf("ScrapeNow: %v", err)
	}

	if len(fetcher.fetched) != 1 || fetcher.fetched[0] != "/robots.txt" {
		t.Fatalf("fetched = %v, want only a single /robots.txt request (no ranking requests when disallowed)", fetcher.fetched)
	}

	allowed, _, found, err := permissions.Get()
	if err != nil || !found {
		t.Fatalf("permissions.Get(): found=%v err=%v", found, err)
	}
	if allowed {
		t.Error("stored permission = true, want false (robots.txt disallows /ranking and /tournaments/*)")
	}

	count, _ := players.Count()
	if count != 0 {
		t.Errorf("player count = %d, want 0 (index scrape must be skipped entirely)", count)
	}
}

func TestScrapePlayerIndex_IgnoresRobotsWhenFlagOff(t *testing.T) {
	players := newTestDB(t)
	fetcher := &spyFetcher{bodies: map[string]string{
		"/robots.txt": "User-agent: *\nDisallow: /ranking\n",
	}}

	// respectPermission=false: robots.txt must never even be fetched.
	scheduler := cron.NewScheduler(players, newTestJobStore(t), nil, fetcher, nil, false)
	_ = scheduler.ScrapeNow()

	for _, p := range fetcher.fetched {
		if p == "/robots.txt" {
			t.Fatal("robots.txt was fetched even though RESPECT_PERMISSION is off")
		}
	}
}

// A failed Women tab must not make the index forget a woman's Women RGX until the next scrape.
func TestScrapePlayerIndex_KeepsWomenRGXWhenWomenTabFails(t *testing.T) {
	ranking, err := os.ReadFile("../../../scraper/fixtures/ranking.html")
	if err != nil {
		t.Fatalf("read ranking fixture: %v", err)
	}
	open, err := scraper.ParseRankingPage(bytes.NewReader(ranking))
	if err != nil || len(open) == 0 {
		t.Fatalf("parse ranking fixture: %d players, %v", len(open), err)
	}
	woman := open[0]
	woman.OpenRGX, woman.WomenRGX, woman.RGX = woman.RGX, 1234, 1234

	players := newTestDB(t)
	if err := players.Upsert(woman); err != nil {
		t.Fatal(err)
	}
	fetcher := &spyFetcher{bodies: map[string]string{scraper.RankingOpenPath: string(ranking)}}
	if err := cron.NewScheduler(players, newTestJobStore(t), nil, fetcher, nil, false).ScrapeNow(); err != nil {
		t.Fatalf("ScrapeNow: %v", err)
	}

	got, err := players.GetByID(woman.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %+v, %v", got, err)
	}
	if got.WomenRGX != 1234 || got.RGX != 1234 || got.OpenRGX != woman.OpenRGX {
		t.Errorf("rgx/open/women = %d/%d/%d, want 1234/%d/1234", got.RGX, got.OpenRGX, got.WomenRGX, woman.OpenRGX)
	}
}

package cron_test

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tobiasmarkusbraun/rgx-calc/internal/cron"
	"github.com/tobiasmarkusbraun/rgx-calc/internal/db"
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

	scheduler := cron.NewScheduler(players, newTestJobStore(t), fetcher, permissions, true)
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
	scheduler := cron.NewScheduler(players, newTestJobStore(t), fetcher, nil, false)
	_ = scheduler.ScrapeNow()

	for _, p := range fetcher.fetched {
		if p == "/robots.txt" {
			t.Fatal("robots.txt was fetched even though RESPECT_PERMISSION is off")
		}
	}
}

package scraper

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

// singleFixtureFetcher always serves the same fixture file regardless of the requested
// path, tracking how many times it was called — unlike FixtureFetcher (which routes by URL
// shape and can't serve two different tournament detail pages, since both shapes look
// identical), this lets a test pin down exactly which tournament detail fixture is behind
// resolveTournamentGRT's fetch.
type singleFixtureFetcher struct {
	path  string
	calls int
}

func (f *singleFixtureFetcher) Fetch(string) (io.ReadCloser, error) {
	f.calls++
	return os.Open(f.path)
}

func newTournamentTestStore(t *testing.T) *db.TournamentStore {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "tournaments.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return db.NewTournamentStore(conn)
}

func TestResolveTournamentGRT_GRTTournament(t *testing.T) {
	fetcher := &singleFixtureFetcher{path: "../../../scraper/fixtures/tournament-detail.html"}
	deps := ScrapeDeps{Fetcher: fetcher, Tournaments: newTournamentTestStore(t)}

	isGRT, err := resolveTournamentGRT(deps, 692, time.Now())
	if err != nil {
		t.Fatalf("resolveTournamentGRT: %v", err)
	}
	if !isGRT {
		t.Error("isGRT = false, want true for the Leguan Cup fixture")
	}

	stored, err := deps.Tournaments.Get(692)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored == nil || !stored.IsGRT || stored.Name != "2. Leguan Cup" {
		t.Errorf("stored tournament = %+v, want a GRT row named %q", stored, "2. Leguan Cup")
	}
}

func TestResolveTournamentGRT_NonGRTTournament(t *testing.T) {
	fetcher := &singleFixtureFetcher{path: "../../../scraper/fixtures/tournament-detail-non-grt.html"}
	deps := ScrapeDeps{Fetcher: fetcher, Tournaments: newTournamentTestStore(t)}

	isGRT, err := resolveTournamentGRT(deps, 672, time.Now())
	if err != nil {
		t.Fatalf("resolveTournamentGRT: %v", err)
	}
	if isGRT {
		t.Error("isGRT = true, want false for the ETS Championship Basel fixture")
	}

	stored, err := deps.Tournaments.Get(672)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored == nil || stored.IsGRT {
		t.Errorf("stored tournament = %+v, want a non-GRT row", stored)
	}
}

// TestResolveTournamentGRT_CachesAfterFirstScrape proves the whole point of the
// tournaments table: once a tournament's GRT status is known, resolveTournamentGRT never
// fetches its detail page again.
func TestResolveTournamentGRT_CachesAfterFirstScrape(t *testing.T) {
	fetcher := &singleFixtureFetcher{path: "../../../scraper/fixtures/tournament-detail.html"}
	deps := ScrapeDeps{Fetcher: fetcher, Tournaments: newTournamentTestStore(t)}

	if _, err := resolveTournamentGRT(deps, 692, time.Now()); err != nil {
		t.Fatalf("first resolveTournamentGRT: %v", err)
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetch calls after first resolve = %d, want 1", fetcher.calls)
	}

	isGRT, err := resolveTournamentGRT(deps, 692, time.Now())
	if err != nil {
		t.Fatalf("second resolveTournamentGRT: %v", err)
	}
	if !isGRT {
		t.Error("isGRT on cached lookup = false, want true")
	}
	if fetcher.calls != 1 {
		t.Errorf("fetch calls after second resolve = %d, want still 1 (should be served from the DB)", fetcher.calls)
	}
}

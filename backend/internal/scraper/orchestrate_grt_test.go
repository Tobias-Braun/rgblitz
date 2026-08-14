package scraper

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
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

// nonGRTDetailFetcher serves the same fixture set as FixtureFetcher, except every
// tournament *detail* page (matched the same way FixtureFetcher's own routing does: not
// the schedule or teams shape) is swapped for the non-GRT fixture, and any request for a
// division's teams page fails the test outright.
type nonGRTDetailFetcher struct {
	t    *testing.T
	dir  string
	base *FixtureFetcher
}

func (f *nonGRTDetailFetcher) Fetch(path string) (io.ReadCloser, error) {
	if strings.Contains(path, "/teams/") {
		f.t.Fatalf("division teams page fetched for a non-GRT tournament: %q", path)
	}
	if strings.HasPrefix(path, "/tournaments/") && !strings.HasPrefix(path, "/tournaments/schedule/team/") {
		return os.Open(filepath.Join(f.dir, "tournament-detail-non-grt.html"))
	}
	return f.base.Fetch(path)
}

// TestScrapePlayerToday_NonGRTTournament_SkipsDivisionMatchAndTeamsFetch proves the
// GRT-before-division-match ordering in ScrapePlayerToday: resolving GRT status only needs
// the tournament ID (shared by every division in the sidebar), not a successful
// schedule-page-DivisionName-to-sidebar-Name match. So for a non-GRT tournament, the
// pipeline must resolve to "no_tournament" using only the tournament ID from the sidebar,
// without ever needing (or fetching) the division's teams page — exercising exactly the
// path that a real ETS/EURA tournament takes, end to end through ScrapePlayerToday rather
// than just the resolveTournamentGRT unit in isolation.
func TestScrapePlayerToday_NonGRTTournament_SkipsDivisionMatchAndTeamsFetch(t *testing.T) {
	fixtureDir := "../../../scraper/fixtures"
	fetcher := &nonGRTDetailFetcher{t: t, dir: fixtureDir, base: NewFixtureFetcher(fixtureDir)}
	deps := ScrapeDeps{
		Fetcher:     fetcher,
		Cache:       cache.New(),
		ProxyURL:    func(u string) string { return u },
		Tournaments: newTournamentTestStore(t),
	}

	player := &db.Player{ID: "LUC2110", Name: "Lucas Christiani", RGX: 1400}
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC) // the day the fixtures were captured on

	result, tournamentID, err := ScrapePlayerToday(deps, player, now)
	if err != nil {
		t.Fatalf("ScrapePlayerToday: %v", err)
	}
	if result.Status != "no_tournament" {
		t.Errorf("status = %q, want %q for a non-GRT tournament", result.Status, "no_tournament")
	}
	if tournamentID == 0 {
		t.Error("tournamentID = 0, want the sidebar's tournament ID even though the tournament is non-GRT")
	}

	stored, err := deps.Tournaments.Get(tournamentID)
	if err != nil {
		t.Fatalf("get stored tournament: %v", err)
	}
	if stored == nil || stored.IsGRT {
		t.Errorf("stored tournament = %+v, want a permanently cached non-GRT row", stored)
	}
}

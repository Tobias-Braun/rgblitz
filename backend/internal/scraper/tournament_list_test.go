package scraper_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

const tournamentListFixture = "../../../scraper/fixtures/tournaments-upcoming.html"

func TestParseTournamentList_Fixture(t *testing.T) {
	f, err := os.Open(tournamentListFixture)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	defer f.Close()

	entries, err := scraper.ParseTournamentList(f)
	if err != nil {
		t.Fatalf("ParseTournamentList: %v", err)
	}
	if len(entries) < 10 {
		t.Fatalf("parsed %d tournaments, expected the full upcoming list", len(entries))
	}

	byID := map[int]scraper.TournamentListEntry{}
	for _, e := range entries {
		byID[e.ID] = e
	}

	// Tournament 692 had already started when this fixture was captured (2026-07-19), so it
	// is gone from the card list and only the nav dropdown still carries it — exactly the
	// case that makes "which tournament is running today" answerable. Nav entries know the
	// start date from their slug but no end date, so they qualify via MightRunOn, not RunsOn.
	leguan, ok := byID[692]
	if !ok {
		t.Fatal("tournament 692 (2. Leguan Cup) missing — the nav-dropdown fallback found nothing")
	}
	if leguan.Name != "2. Leguan Cup" {
		t.Errorf("name: got %q, want %q", leguan.Name, "2. Leguan Cup")
	}
	if leguan.StartDate != "2026-07-18" {
		t.Errorf("start date: got %q, want 2026-07-18", leguan.StartDate)
	}
	captureDay := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	if !leguan.MightRunOn(captureDay) {
		t.Error("MightRunOn(2026-07-19) = false for a tournament that started the day before")
	}

	// A single-day tournament: start and end must be the same day.
	if nk, ok := byID[767]; ok {
		if nk.StartDate != "2026-07-25" || nk.EndDate != "2026-07-25" {
			t.Errorf("single-day dates: got %s..%s, want 2026-07-25 twice", nk.StartDate, nk.EndDate)
		}
	}
}

// The date part of a card subtitle is rendered several different ways across tournaments;
// every form has to resolve to the same start/end pair shape.
func TestParseTournamentList_SubtitleDateForms(t *testing.T) {
	cases := []struct {
		subtitle   string
		start, end string
		location   string
	}{
		{"25.07.2026, Utrecht", "2026-07-25", "2026-07-25", "Utrecht"},
		{"18.07.2026 - 19.07.2026, Bochum", "2026-07-18", "2026-07-19", "Bochum"},
		{"15.08.2026-16.08.2026, Mannheim", "2026-08-15", "2026-08-16", "Mannheim"},
		{"02. - 06.09.2026, Champigny-sur-Marne", "2026-09-02", "2026-09-06", "Champigny-sur-Marne"},
	}

	for _, tc := range cases {
		html := `<rg-expandable-card onclick="display_tournament_body(1)" title="T" subtitle="` + tc.subtitle + `"></rg-expandable-card>`
		entries, err := scraper.ParseTournamentList(strings.NewReader(html))
		if err != nil {
			t.Fatalf("%q: ParseTournamentList: %v", tc.subtitle, err)
		}
		got := entries[0]
		if got.StartDate != tc.start || got.EndDate != tc.end {
			t.Errorf("%q: dates got %s..%s, want %s..%s", tc.subtitle, got.StartDate, got.EndDate, tc.start, tc.end)
		}
		if got.Location != tc.location {
			t.Errorf("%q: location got %q, want %q", tc.subtitle, got.Location, tc.location)
		}
	}
}

func TestTournamentListEntry_RunsOn(t *testing.T) {
	twoDay := scraper.TournamentListEntry{StartDate: "2026-07-18", EndDate: "2026-07-19"}
	day := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return d
	}

	for _, s := range []string{"2026-07-18", "2026-07-19"} {
		if !twoDay.RunsOn(day(s)) {
			t.Errorf("RunsOn(%s) = false, want true", s)
		}
	}
	for _, s := range []string{"2026-07-17", "2026-07-20"} {
		if twoDay.RunsOn(day(s)) {
			t.Errorf("RunsOn(%s) = true, want false", s)
		}
	}

	// A card whose dates couldn't be parsed must never match a day — otherwise it would
	// pull an unrelated tournament into a capture.
	undated := scraper.TournamentListEntry{ID: 1, Name: "no dates"}
	if undated.RunsOn(day("2026-07-18")) {
		t.Error("RunsOn on an entry with unparsed dates = true, want false")
	}
	if undated.MightRunOn(day("2026-07-18")) {
		t.Error("MightRunOn on an entry with unparsed dates = true, want false")
	}
}

func TestTournamentListEntry_MightRunOn(t *testing.T) {
	day := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return d
	}

	// Nav-dropdown entry: start date only. A Saturday start still counts on Sunday.
	navEntry := scraper.TournamentListEntry{StartDate: "2026-07-18"}
	for _, s := range []string{"2026-07-18", "2026-07-19"} {
		if !navEntry.MightRunOn(day(s)) {
			t.Errorf("MightRunOn(%s) = false, want true", s)
		}
	}
	for _, s := range []string{"2026-07-17", "2026-07-20"} {
		if navEntry.MightRunOn(day(s)) {
			t.Errorf("MightRunOn(%s) = true, want false", s)
		}
	}
}

// A card missing the fields the parser reads must be skipped, not crash or invent data —
// playerzone mixes in tournaments from other national bodies with differently-rendered cards.
func TestParseTournamentList_SkipsUnreadableCards(t *testing.T) {
	html := `
		<rg-expandable-card onclick="something_else(5)" title="not a tournament"></rg-expandable-card>
		<rg-expandable-card onclick="display_tournament_body(abc)" title="bad id"></rg-expandable-card>
		<rg-expandable-card onclick="display_tournament_body(42)" title="Fine" subtitle="01.08.2026, Hannover"></rg-expandable-card>
		<rg-expandable-card onclick="display_tournament_body(43)" title="No dates" subtitle="somewhere"></rg-expandable-card>`

	entries, err := scraper.ParseTournamentList(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseTournamentList: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("parsed %d entries, want 2 (the two with a usable ID)", len(entries))
	}
	if entries[0].ID != 42 || entries[0].StartDate != "2026-08-01" {
		t.Errorf("first entry: got %+v", entries[0])
	}
	if entries[1].StartDate != "" || entries[1].EndDate != "" {
		t.Errorf("undated entry should keep empty dates, got %+v", entries[1])
	}
}

func TestParseTournamentList_Garbage(t *testing.T) {
	if _, err := scraper.ParseTournamentList(strings.NewReader(garbageHTML)); err == nil {
		t.Fatal("expected an error for garbage HTML, got nil")
	}
}

func TestParseTournamentList_Empty(t *testing.T) {
	if _, err := scraper.ParseTournamentList(strings.NewReader("")); err == nil {
		t.Fatal("expected an error for empty HTML, got nil")
	}
}

func TestParseTournamentList_MalformedDoesNotPanic(t *testing.T) {
	malformed := `<rg-expandable-card onclick="display_tournament_body(7)" title="T" subtitle="99.99.9999 - , `
	if _, err := scraper.ParseTournamentList(strings.NewReader(malformed)); err != nil {
		t.Logf("malformed HTML returned an error (acceptable): %v", err)
	}
}

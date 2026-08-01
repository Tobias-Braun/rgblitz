package scraper_test

import (
	"strings"
	"testing"

	"github.com/tobiasmarkusbraun/rgx-calc/internal/scraper"
)

// These tests assert every parser fails loudly (returns a clear error, never panics)
// when the site's HTML no longer matches the expected structure, per
// docs/05-scraping-algorithm.md's "fail loudly rather than silently returning empty data" rule.

const garbageHTML = `<html><body><p>this page has nothing the scraper is looking for</p></body></html>`

func TestParseRankingPage_Garbage(t *testing.T) {
	_, err := scraper.ParseRankingPage(strings.NewReader(garbageHTML))
	if err == nil {
		t.Fatal("expected an error for garbage HTML, got nil")
	}
}

func TestParseRankingPage_Empty(t *testing.T) {
	_, err := scraper.ParseRankingPage(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected an error for empty HTML, got nil")
	}
}

func TestParseProfilePage_Garbage(t *testing.T) {
	_, err := scraper.ParseProfilePage(strings.NewReader(garbageHTML))
	if err == nil {
		t.Fatal("expected an error for garbage HTML, got nil")
	}
}

func TestParseDivisionSidebar_Garbage(t *testing.T) {
	_, err := scraper.ParseDivisionSidebar(strings.NewReader(garbageHTML))
	if err == nil {
		t.Fatal("expected an error for garbage HTML, got nil")
	}
}

func TestParseDivisionTeams_Garbage(t *testing.T) {
	_, err := scraper.ParseDivisionTeams(strings.NewReader(garbageHTML))
	if err == nil {
		t.Fatal("expected an error for garbage HTML, got nil")
	}
}

func TestParseTeamSchedule_Garbage(t *testing.T) {
	_, err := scraper.ParseTeamSchedule(strings.NewReader(garbageHTML), 12345)
	if err == nil {
		t.Fatal("expected an error for garbage HTML with no player cards, got nil")
	}
}

// Malformed (but not empty) HTML must not panic any parser — goquery is lenient, but
// truncated/broken markup is exactly the kind of input a live DOM change could produce.
func TestParsers_MalformedHTMLDoesNotPanic(t *testing.T) {
	malformed := `<html><body><table id="rgx-main-table"><tr><td class="player-col"><div class="player-img"><img src="broken`

	mustNotPanic := func(name string, fn func()) {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v", r)
				}
			}()
			fn()
		})
	}

	mustNotPanic("ParseRankingPage", func() {
		_, _ = scraper.ParseRankingPage(strings.NewReader(malformed))
	})
	mustNotPanic("ParseProfilePage", func() {
		_, _ = scraper.ParseProfilePage(strings.NewReader(malformed))
	})
	mustNotPanic("ParseDivisionSidebar", func() {
		_, _ = scraper.ParseDivisionSidebar(strings.NewReader(malformed))
	})
	mustNotPanic("ParseDivisionTeams", func() {
		_, _ = scraper.ParseDivisionTeams(strings.NewReader(malformed))
	})
	mustNotPanic("ParseTeamSchedule", func() {
		_, _ = scraper.ParseTeamSchedule(strings.NewReader(malformed), 1)
	})
}

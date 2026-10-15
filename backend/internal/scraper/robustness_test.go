package scraper_test

import (
	"strings"
	"testing"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
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
	mustNotPanic("ParseLeagueFixtures", func() {
		_, _ = scraper.ParseLeagueFixtures(strings.NewReader(malformed), 1)
	})
	mustNotPanic("ParseMatchdayGroup", func() {
		_, _ = scraper.ParseMatchdayGroup(strings.NewReader(malformed), 1)
	})
	mustNotPanic("ParseSubgames", func() {
		_, _ = scraper.ParseSubgames(strings.NewReader(`{"html": "` + strings.ReplaceAll(malformed, `"`, `\"`) + `"}`))
	})
}

// Non-GRT tournaments (e.g. EURA-rated ones) don't render an RGX badge on player
// cards at all — unlike the malformed-HTML case above, the player card here is
// otherwise well-formed (a real name, no truncation), it's specifically the
// span.rgx-value element that's absent. This must not panic and must report rgx 0
// for that player (which callers treat as "no RGX", falling back to a proxy value).
const noRGXBadgeHTML = `<html><body>
<div class="row section-title"><h3 class="event-title">Open</h3></div>
<div class="second-title"><span><a>Our Team</a></span></div>
<div class="player-card">
	<div class="info-player-name"><a>Alex Example</a></div>
</div>
</body></html>`

func TestParseTeamSchedule_PlayerCardWithoutRGXBadge(t *testing.T) {
	result, err := scraper.ParseTeamSchedule(strings.NewReader(noRGXBadgeHTML), 1)
	if err != nil {
		t.Fatalf("ParseTeamSchedule: %v", err)
	}
	if len(result.Players) != 1 {
		t.Fatalf("players = %d, want 1", len(result.Players))
	}
	if result.Players[0].RGX != 0 {
		t.Errorf("RGX = %d, want 0 for a player card with no RGX badge", result.Players[0].RGX)
	}
}

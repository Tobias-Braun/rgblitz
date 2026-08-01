package scraper_test

import (
	"os"
	"strings"
	"testing"

	"github.com/tobiasmarkusbraun/rgx-calc/internal/scraper"
)

const profileFixture = "../../../scraper/fixtures/player-profile.html"

func TestParseProfilePage(t *testing.T) {
	f, err := os.Open(profileFixture)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	defer f.Close()

	entries, err := scraper.ParseProfilePage(f)
	if err != nil {
		t.Fatalf("ParseProfilePage: %v", err)
	}

	// Only 2 of the 3 <li> entries in the fixture have a published "Games"
	// button — the third is an upcoming tournament without a bracket yet and
	// must be skipped rather than error the whole parse.
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (upcoming tournament without a Games button skipped), got %d: %+v", len(entries), entries)
	}

	found := false
	for _, e := range entries {
		if e.Date == "19.07.26" {
			found = true
			if e.TournamentTeamID != 61234 {
				t.Errorf("today's entry team ID: got %d, want 61234", e.TournamentTeamID)
			}
		}
	}
	if !found {
		t.Errorf("entry for 19.07.26 not found; got %+v", entries)
	}
}

func TestParseProfilePage_NoEntries(t *testing.T) {
	_, err := scraper.ParseProfilePage(strings.NewReader("<html><body>no tournaments here</body></html>"))
	if err == nil {
		t.Error("expected an error when the profile page has 0 tournament entries")
	}
}

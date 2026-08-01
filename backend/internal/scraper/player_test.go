package scraper_test

import (
	"os"
	"testing"

	"github.com/tobiasmarkusbraun/rgx-calc/internal/db"
	"github.com/tobiasmarkusbraun/rgx-calc/internal/scraper"
)

const (
	rankingFixture      = "../../../scraper/fixtures/ranking.html"
	rankingWomenFixture = "../../../scraper/fixtures/ranking-women.html"
)

func TestParseRankingPage(t *testing.T) {
	f, err := os.Open(rankingFixture)
	if err != nil {
		t.Skipf("fixture not found at %s: %v", rankingFixture, err)
	}
	defer f.Close()

	players, err := scraper.ParseRankingPage(f)
	if err != nil {
		t.Fatalf("ParseRankingPage: %v", err)
	}
	if len(players) == 0 {
		t.Fatal("expected at least one player")
	}

	// Known top player from fixture captured on 2026-07-19.
	found := false
	for _, p := range players {
		if p.ID == "PAULCHEN" {
			found = true
			if p.Name != "Paul Siemer" {
				t.Errorf("name: got %q, want %q", p.Name, "Paul Siemer")
			}
			if p.RGX != 1963 {
				t.Errorf("RGX: got %d, want 1963", p.RGX)
			}
			if p.ClubName == "" {
				t.Error("club name should not be empty")
			}
			if p.AvatarURL == "" {
				t.Error("avatar URL should not be empty")
			}
		}
	}
	if !found {
		t.Error("player PAULCHEN not found in parsed output")
	}

	t.Logf("parsed %d players; first: %s (%s) RGX=%d", len(players), players[0].Name, players[0].ID, players[0].RGX)
}

func TestPlayerIDFromAvatarURL(t *testing.T) {
	// All three IDs appear in the ranking fixture (top-30 snapshot from 2026-07-19).
	cases := []struct {
		want string
	}{
		{"PAULCHEN"},
		{"LUC2110"},
		{"RAS1811"},
	}

	f, err := os.Open(rankingFixture)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	defer f.Close()

	players, _ := scraper.ParseRankingPage(f)
	idSet := map[string]bool{}
	for _, p := range players {
		idSet[p.ID] = true
	}

	for _, tc := range cases {
		if !idSet[tc.want] {
			t.Errorf("player ID %q not found in parsed ranking output", tc.want)
		}
	}
}

// Past roughly the top 30, the fixture's rows drop the avatar <div> entirely —
// this must not cause those rows to be silently skipped.
func TestParseRankingPage_RowsWithoutAvatar(t *testing.T) {
	f, err := os.Open(rankingFixture)
	if err != nil {
		t.Skipf("fixture not found at %s: %v", rankingFixture, err)
	}
	defer f.Close()

	players, err := scraper.ParseRankingPage(f)
	if err != nil {
		t.Fatalf("ParseRankingPage: %v", err)
	}

	// Row 31 ("Ramon Felix") has no avatar image in the fixture; the parser
	// must fall back to the show_history(id, ...) onclick handler for its ID.
	found := false
	for _, p := range players {
		if p.Name == "Ramon Felix" {
			found = true
			if p.ID != "PZ2995" {
				t.Errorf("ID: got %q, want %q", p.ID, "PZ2995")
			}
			if p.AvatarURL != "" {
				t.Errorf("expected empty AvatarURL for an avatar-less row, got %q", p.AvatarURL)
			}
		}
	}
	if !found {
		t.Error("Ramon Felix (avatar-less row) not found — rows without an avatar are being skipped")
	}
}

func TestParseRankingPage_WomenFixture(t *testing.T) {
	f, err := os.Open(rankingWomenFixture)
	if err != nil {
		t.Skipf("fixture not found at %s: %v", rankingWomenFixture, err)
	}
	defer f.Close()

	players, err := scraper.ParseRankingPage(f)
	if err != nil {
		t.Fatalf("ParseRankingPage: %v", err)
	}
	if len(players) != 2 {
		t.Fatalf("expected 2 players in the women fixture, got %d", len(players))
	}

	byID := map[string]db.Player{}
	for _, p := range players {
		byID[p.ID] = p
	}
	if p, ok := byID["TESTW01"]; !ok || p.RGX != 1450 {
		t.Errorf("TESTW01: got %+v, want RGX=1450", p)
	}
	if p, ok := byID["TESTW02"]; !ok || p.RGX != 1210 {
		t.Errorf("TESTW02: got %+v, want RGX=1210", p)
	}
}

func TestNumericPlayerID(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"LUC2110", "2110"}, // avatar-derived
		{"PZ8517", "8517"},  // show_history-derived
	}
	for _, tc := range cases {
		got, err := scraper.NumericPlayerID(tc.id)
		if err != nil {
			t.Fatalf("NumericPlayerID(%q): %v", tc.id, err)
		}
		if got != tc.want {
			t.Errorf("NumericPlayerID(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}

	if _, err := scraper.NumericPlayerID("PAULCHEN"); err == nil {
		t.Error("expected an error for an ID with no trailing digits")
	}
}

func TestMergeWomenRGX(t *testing.T) {
	open := []db.Player{
		{ID: "A", Name: "Alice", RGX: 1500},
		{ID: "B", Name: "Bob", RGX: 1600},
		{ID: "C", Name: "Carla", RGX: 1300},
	}
	women := []db.Player{
		{ID: "A", Name: "Alice", RGX: 1450}, // Alice's Open RGX (1500) should be overridden.
		{ID: "D", Name: "Dana", RGX: 1700},  // Never played Open — must still be indexed.
	}

	merged := scraper.MergeWomenRGX(open, women)

	if len(merged) != len(open)+1 {
		t.Fatalf("merged length = %d, want %d (Open list plus Dana, who is Women-only)", len(merged), len(open)+1)
	}

	byID := map[string]db.Player{}
	for _, p := range merged {
		byID[p.ID] = p
	}

	if got := byID["A"].RGX; got != 1450 {
		t.Errorf("Alice RGX = %d, want 1450 (Women RGX should override Open RGX)", got)
	}
	if got := byID["B"].RGX; got != 1600 {
		t.Errorf("Bob RGX = %d, want 1600 (unaffected by the Women list)", got)
	}
	if got := byID["C"].RGX; got != 1300 {
		t.Errorf("Carla RGX = %d, want 1300 (not present on the Women list)", got)
	}
	if got, ok := byID["D"]; !ok || got.RGX != 1700 {
		t.Errorf("Dana = %+v, ok=%v — a Women-only player must still appear in the merged index", got, ok)
	}
}

// A player's ID is avatar-derived when her ranking row has an avatar and a
// numeric fallback otherwise (see playerIDFromShowHistory); avatar presence
// depends on rank position within that specific tab, so the same woman can
// easily end up with two different IDs on the Open vs. Women tab. The merge
// must still find her by name in that case.
func TestMergeWomenRGX_FallsBackToNameWhenIDsDiffer(t *testing.T) {
	open := []db.Player{
		// No avatar on the (much longer) Open tab -> numeric fallback ID.
		{ID: "PZ9001", Name: "Wanda Testfrau", RGX: 1500},
	}
	women := []db.Player{
		// Cracks the top of the shorter Women tab -> avatar-derived ID.
		{ID: "TESTW01", Name: "Wanda Testfrau", RGX: 1450},
	}

	merged := scraper.MergeWomenRGX(open, women)

	if len(merged) != 1 || merged[0].RGX != 1450 {
		t.Fatalf("got %+v, want a single entry with RGX=1450 (Women RGX via name match)", merged)
	}
	if merged[0].ID != "PZ9001" {
		t.Errorf("ID = %q, want unchanged %q (only RGX should be overridden)", merged[0].ID, "PZ9001")
	}
}

package scraper_test

import (
	"math"
	"testing"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

func TestMatchP(t *testing.T) {
	cases := []struct {
		won, lost int
		want      float64
	}{
		// single-set / best-of-3
		{2, 0, 1.0},
		{1, 0, 1.0},
		{2, 1, 0.67},
		{1, 1, 0.5},
		{1, 2, 0.33},
		{0, 2, 0.0},
		{0, 1, 0.0},
		// best-of-5
		{3, 0, 1.0},
		{3, 1, 0.75},
		{3, 2, 0.6},
		{2, 2, 0.5},
		{2, 3, 0.4},
		{1, 3, 0.25},
		{0, 3, 0.0},
	}
	for _, tc := range cases {
		got := scraper.MatchP(tc.won, tc.lost)
		if got != tc.want {
			t.Errorf("MatchP(%d, %d) = %v, want %v", tc.won, tc.lost, got, tc.want)
		}
	}
}

// TestComputeDeltaBreakdown locks in hand-verified values for the RGX formula
// (e1 = 1/(1+10^((r2-r1)/550)); delta = round(b*50*(p-e1))) across all five
// p-values, including the single-set (b=0.75) case.
func TestComputeDeltaBreakdown(t *testing.T) {
	cases := []struct {
		name              string
		r1, r2            int
		setsWon, setsLost int
		wantB             float64
		wantP             float64
		wantDelta         int
	}{
		{"best-of-3 2:0 favourite win", 2347, 2401, 2, 0, 1.0, 1.0, 28},
		{"best-of-3 0:2 favourite loss", 2347, 2401, 0, 2, 1.0, 0.0, -22},
		{"single-set 1:0 underdog win", 2000, 2200, 1, 0, 0.75, 1.0, 26},
		{"best-of-3 2:1 close win", 2347, 2401, 2, 1, 1.0, 0.67, 11},
		{"best-of-3 1:1 draw-ish", 2347, 2401, 1, 1, 1.0, 0.5, 3},
		{"best-of-3 1:2 close loss", 2347, 2401, 1, 2, 1.0, 0.33, -6},
		{"equal teams 2:0", 2000, 2000, 2, 0, 1.0, 1.0, 25},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bd := scraper.ComputeDeltaBreakdown(tc.r1, tc.r2, tc.setsWon, tc.setsLost)
			if bd.B != tc.wantB {
				t.Errorf("B = %v, want %v", bd.B, tc.wantB)
			}
			if bd.P != tc.wantP {
				t.Errorf("P = %v, want %v", bd.P, tc.wantP)
			}
			wantE1 := 1.0 / (1.0 + math.Pow(10, float64(tc.r2-tc.r1)/550.0))
			if math.Abs(bd.E1-wantE1) > 1e-9 {
				t.Errorf("E1 = %v, want %v", bd.E1, wantE1)
			}
			if bd.Delta != tc.wantDelta {
				t.Errorf("Delta = %d, want %d", bd.Delta, tc.wantDelta)
			}
		})
	}
}

func TestComputeDeltaBreakdown_ZeroSumOpposingTeams(t *testing.T) {
	// A single game's delta for one team must be the exact negation of the
	// other team's delta for the same result, per the RGX spec (x2 = -x1).
	r1, r2 := 2100, 1900
	setsWon, setsLost := 2, 1

	ours := scraper.ComputeDeltaBreakdown(r1, r2, setsWon, setsLost)
	theirs := scraper.ComputeDeltaBreakdown(r2, r1, setsLost, setsWon)

	if ours.Delta != -theirs.Delta {
		t.Errorf("expected opposing deltas to sum to zero, got %d and %d", ours.Delta, theirs.Delta)
	}
}

// TestGameResultFromCard_RGXDeltaPerPlayer locks in that RGXDeltaPerPlayer is always exactly
// half of RGXDelta, including the fractional .5 case for odd deltas.
func TestGameResultFromCard_RGXDeltaPerPlayer(t *testing.T) {
	cases := []struct {
		name     string
		r1, r2   int
		game     scraper.GameCard
		wantHalf float32
	}{
		{
			name: "even delta splits cleanly",
			r1:   2000, r2: 2000,
			game: scraper.GameCard{
				WeAreTeamOne: true,
				SetScores:    []scraper.SetScore{{TeamOneScore: 21, TeamTwoScore: 15}, {TeamOneScore: 21, TeamTwoScore: 15}},
			},
			wantHalf: 12.5, // delta=25 for a 2:0 win between equal teams -> +25, half = 12.5
		},
		{
			name: "odd delta produces a .5 per-player share",
			r1:   2347, r2: 2401,
			game: scraper.GameCard{
				WeAreTeamOne: true,
				SetScores: []scraper.SetScore{
					{TeamOneScore: 21, TeamTwoScore: 15},
					{TeamOneScore: 15, TeamTwoScore: 21},
					{TeamOneScore: 21, TeamTwoScore: 15},
				},
			},
			wantHalf: 5.5, // delta=11 for a 2:1 win, half = 5.5
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gr := scraper.GameResultFromCard(tc.game, tc.r1, tc.r2, false)
			wantDelta := int(math.Round(float64(tc.wantHalf * 2)))
			if gr.RGXDelta != wantDelta {
				t.Fatalf("RGXDelta = %d, want %d (test setup check)", gr.RGXDelta, wantDelta)
			}
			if gr.RGXDeltaPerPlayer != tc.wantHalf {
				t.Errorf("RGXDeltaPerPlayer = %v, want %v (exactly half of RGXDelta=%d)", gr.RGXDeltaPerPlayer, tc.wantHalf, gr.RGXDelta)
			}
			if gr.RGXDeltaPerPlayer*2 != float32(gr.RGXDelta) {
				t.Errorf("RGXDeltaPerPlayer*2 = %v, want RGXDelta = %d", gr.RGXDeltaPerPlayer*2, gr.RGXDelta)
			}
		})
	}
}

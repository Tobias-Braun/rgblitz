package main

import (
	"strings"
	"testing"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// twoSidedDivision builds one division holding both teams' view of a single game, so the
// consistency checks have a pair to compare. a and b are the two GameResults as each team
// would see them.
func twoSidedDivision(a, b scraper.GameResult) divisionCapture {
	return divisionCapture{
		Name: "Advanced Open",
		TeamDays: []teamDayCapture{
			{
				TeamID: 1, TeamName: "Team A",
				Result:  &scraper.TeamDayResult{TeamRGX: a.TeamRGX, TotalDelta: a.RGXDelta, Games: []scraper.GameResult{a}},
				GameIDs: []int{777},
			},
			{
				TeamID: 2, TeamName: "Team B",
				Result:  &scraper.TeamDayResult{TeamRGX: b.TeamRGX, TotalDelta: b.RGXDelta, Games: []scraper.GameResult{b}},
				GameIDs: []int{777},
			},
		},
	}
}

// A game both teams agree on: mirrored sets, swapped RGX inputs, deltas that cancel out.
func consistentPair() (scraper.GameResult, scraper.GameResult) {
	a := scraper.GameResult{
		Round: "Gr. A #1", OpponentName: "Team B", Result: "W", RGXDelta: 12,
		TeamRGX: 2200, OpponentRGX: 2300,
		Sets: []scraper.SetResult{{OurScore: 21, OpponentScore: 18}},
	}
	b := scraper.GameResult{
		Round: "Gr. A #1", OpponentName: "Team A", Result: "L", RGXDelta: -12,
		TeamRGX: 2300, OpponentRGX: 2200,
		Sets: []scraper.SetResult{{OurScore: 18, OpponentScore: 21}},
	}
	return a, b
}

func TestConsistencyChecks_AgreeingSidesProduceNoFindings(t *testing.T) {
	a, b := consistentPair()
	checks := runConsistencyChecks([]divisionCapture{twoSidedDivision(a, b)})

	if len(checks.MirrorMismatches) != 0 {
		t.Fatalf("expected no mismatches, got %+v", checks.MirrorMismatches)
	}
	if len(checks.DivisionDeltaSums) != 1 || checks.DivisionDeltaSums[0].Sum != 0 {
		t.Errorf("delta sums: got %+v, want a single division summing to 0", checks.DivisionDeltaSums)
	}
}

// Set scores that aren't mirrored mean one side's game card was oriented wrongly — the bug
// class this check exists for.
func TestConsistencyChecks_UnmirroredSets(t *testing.T) {
	a, b := consistentPair()
	b.Sets = []scraper.SetResult{{OurScore: 21, OpponentScore: 18}} // both sides claim the win

	checks := runConsistencyChecks([]divisionCapture{twoSidedDivision(a, b)})
	if len(checks.MirrorMismatches) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(checks.MirrorMismatches))
	}
	if !strings.Contains(checks.MirrorMismatches[0].Reason, "mirrored") {
		t.Errorf("reason %q should point at the set orientation", checks.MirrorMismatches[0].Reason)
	}
}

// Each side computes its delta from its own r1 and the opponent's r2. If those two numbers
// disagree between the sides, the teams' RGX was read from two sources that don't match.
func TestConsistencyChecks_DisagreeingRGXInputs(t *testing.T) {
	a, b := consistentPair()
	b.TeamRGX = 2350 // A thinks B is on 2300, B thinks it's on 2350

	checks := runConsistencyChecks([]divisionCapture{twoSidedDivision(a, b)})
	if len(checks.MirrorMismatches) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(checks.MirrorMismatches))
	}
	if !strings.Contains(checks.MirrorMismatches[0].Reason, "combined RGX") {
		t.Errorf("reason %q should point at the RGX inputs", checks.MirrorMismatches[0].Reason)
	}
}

// Same inputs on both sides but deltas that don't cancel out means the formula itself was
// applied differently to the two teams.
func TestConsistencyChecks_DeltasDoNotCancel(t *testing.T) {
	a, b := consistentPair()
	b.RGXDelta = -9

	checks := runConsistencyChecks([]divisionCapture{twoSidedDivision(a, b)})
	if len(checks.MirrorMismatches) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(checks.MirrorMismatches))
	}
	if !strings.Contains(checks.MirrorMismatches[0].Reason, "cancel out") {
		t.Errorf("reason %q should point at the deltas", checks.MirrorMismatches[0].Reason)
	}
	if checks.DivisionDeltaSums[0].Sum != 3 {
		t.Errorf("division sum: got %d, want 3 (the leftover RGX the division gained)", checks.DivisionDeltaSums[0].Sum)
	}
}

// A game only one captured team played (a bye, or the opponent's schedule failed to fetch)
// has nothing to compare against and must not be reported as a mismatch.
func TestConsistencyChecks_SingleSidedGameIsNotAMismatch(t *testing.T) {
	a, _ := consistentPair()
	div := divisionCapture{
		Name: "Advanced Open",
		TeamDays: []teamDayCapture{{
			TeamID: 1, TeamName: "Team A",
			Result:  &scraper.TeamDayResult{TeamRGX: a.TeamRGX, TotalDelta: a.RGXDelta, Games: []scraper.GameResult{a}},
			GameIDs: []int{777},
		}},
	}

	checks := runConsistencyChecks([]divisionCapture{div})
	if len(checks.MirrorMismatches) != 0 {
		t.Fatalf("expected no mismatches for a one-sided game, got %+v", checks.MirrorMismatches)
	}
}

// Proxy RGX is reported so a surprising delta can be traced back to a player or opponent the
// pipeline had no real RGX for.
func TestConsistencyChecks_ReportsProxies(t *testing.T) {
	a, b := consistentPair()
	a.HasProxy = true
	div := twoSidedDivision(a, b)
	div.TeamDays[0].Result.HasProxy = true

	checks := runConsistencyChecks([]divisionCapture{div})
	if len(checks.UnmatchedOpponents) != 1 {
		t.Errorf("unmatched opponents: got %+v, want 1 entry", checks.UnmatchedOpponents)
	}
	if len(checks.ProxiedTeams) != 1 {
		t.Errorf("proxied teams: got %+v, want 1 entry", checks.ProxiedTeams)
	}
}

func TestDivisionWanted(t *testing.T) {
	if !divisionWanted("Advanced Open", nil) {
		t.Error("no filter should accept every division")
	}
	if !divisionWanted("Advanced Open", []string{"open"}) {
		t.Error("filter should match case-insensitively")
	}
	if divisionWanted("Basic Women", []string{"open", "mixed"}) {
		t.Error("a division matching no filter should be skipped")
	}
}

func TestUniquePageName(t *testing.T) {
	cases := map[string]string{
		"/tournaments/schedule/team/61234":                    "team-61234-schedule.html",
		"/tournaments/692-2026-07-18-2-leguan-cup/teams/2265": "tournament-692-division-2265-teams.html",
		"/tournaments/692":                                    "tournament-692-detail.html",
		"/tournaments/692-2026-07-18-2-leguan-cup":            "tournament-692-detail.html",
		"/players/2110":                                       "player-2110-profile.html",
		"/tournaments/upcoming":                               "tournaments-upcoming.html",
	}
	for path, want := range cases {
		if got := uniquePageName(path); got != want {
			t.Errorf("uniquePageName(%q) = %q, want %q", path, got, want)
		}
	}
}

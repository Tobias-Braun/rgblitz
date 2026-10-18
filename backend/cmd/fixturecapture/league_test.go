package main

import "testing"

// League pages must get distinct names: a matchday group's path also matches the tournament
// pattern, and would otherwise overwrite every other group as "tournament-793-detail.html".
func TestUniquePageName_LeaguePages(t *testing.T) {
	cases := map[string]string{
		"/tournaments/793-liga/schedule/mixed-14358-gruppenphase?group_id=76368&md=110&md_group=657": "matchday-group-657.html",
		"/ajax/get-subgames/419767":        "subgames-419767.json",
		"/tournaments/schedule/team/59944": "team-59944-schedule.html",
	}
	for path, want := range cases {
		if got := uniquePageName(path); got != want {
			t.Errorf("uniquePageName(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestSeasonIDFromPath(t *testing.T) {
	cases := map[string]int{
		"/tournaments/793-2027-04-11-deutsche-roundnet-liga-2026-2027/schedule/mixed?md_group=1": 793,
		"/tournaments/schedule/team/59944": 0,
		"":                                 0,
	}
	for path, want := range cases {
		if got := seasonIDFromPath(path); got != want {
			t.Errorf("seasonIDFromPath(%q) = %d, want %d", path, got, want)
		}
	}
}

package scraper_test

import (
	"strings"
	"testing"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// Markup trimmed from live DRL pages (team 39500 "KA-Boom", 2025/26, and team 59549, 2026/27).

const leagueSchedulePage = `<html><head><title>Schedule KA-Boom - 1. Bundesliga</title></head><body>
<rg-navigation><rg-navigation-item href="/tournaments/793-2027-04-11-deutsche-roundnet-liga-2026-2027">League</rg-navigation-item></rg-navigation>
<div class="player-card">
	<div class="info-player-name"><a class="info-text" href="/players/2329-jonas-ka">Jonas Bottler</a></div>
	<rg-score-badge score="1671"></rg-score-badge>
</div>
<div class="player-card">
	<div class="info-player-name"><a class="info-text" href="/players/1001-lena">Lena Schirop</a></div>
	<rg-score-badge score="1500"></rg-score-badge>
</div>
<div id="game-392883">
	<rg-game-card variant="game" outcome="win">
		<div slot="actions"><rg-icon onclick="show_subgames('392883')"></rg-icon></div>
		<div class="teams">
			<div class="team winner"><a class="details" onclick="load_team_info(30819, 39500, null, null, 392883)"><span class="name">KA-Boom</span></a></div>
			<div class="team loser"><a class="details" onclick="load_team_info(1, 39999, null, null, 392883)"><span class="name">Final Opponent</span></a></div>
		</div>
	</rg-game-card>
</div>
<div id="game-324345">
	<rg-game-card variant="game" outcome="win">
		<div slot="tags"><a class="tag" href="/tournaments/543-x/schedule/mixed-12779-vorrunde?group_id=49914&amp;md=76&amp;md_group=525">
			5. Matchday #3</a></div>
		<div slot="actions"><rg-icon onclick="show_subgames('324345')"></rg-icon></div>
		<div class="teams">
			<div class="team winner"><a class="details" onclick="load_team_info(30819, 39500, null, null, 324345)"><span class="name">KA-Boom</span></a></div>
			<div class="team loser"><a class="details" onclick="load_team_info(28314, 39823, null, null, 324345)"><span class="name">MaMa Mia</span></a></div>
		</div>
	</rg-game-card>
</div>
<div id="game-324333">
	<rg-game-card variant="game" outcome="">
		<div slot="tags"><a class="tag" href="/tournaments/543-x/schedule/mixed-12779-vorrunde?group_id=49914&amp;md=76&amp;md_group=525">5. Matchday #2</a></div>
		<div slot="actions"><rg-icon onclick="show_subgames('324333')"></rg-icon></div>
		<div class="teams">
			<div class="team"><a class="details" onclick="load_team_info(5, 40001, null, null, 324333)"><span class="name">MRC I</span></a></div>
			<div class="team"><a class="details" onclick="load_team_info(30819, 39500, null, null, 324333)"><span class="name">KA-Boom</span></a></div>
		</div>
	</rg-game-card>
</div>
</body></html>`

func TestParseLeagueFixtures(t *testing.T) {
	fixtures, err := scraper.ParseLeagueFixtures(strings.NewReader(leagueSchedulePage), 39500)
	if err != nil {
		t.Fatalf("ParseLeagueFixtures: %v", err)
	}
	// The play-off Begegnung carries no matchday group and is out of scope.
	if len(fixtures) != 2 {
		t.Fatalf("fixtures = %d, want 2", len(fixtures))
	}

	f := fixtures[0]
	if f.GameID != 324345 || f.MatchdayGroupID != 525 || f.Matchday != "5. Matchday #3" {
		t.Errorf("first fixture = %+v", f)
	}
	if f.OpponentTeamID != 39823 || f.OpponentName != "MaMa Mia" || !f.WeAreTeamOne {
		t.Errorf("first fixture opponent = %+v", f)
	}
	if !strings.Contains(f.MatchdayPath, "md_group=525") {
		t.Errorf("MatchdayPath = %q", f.MatchdayPath)
	}

	// An upcoming Begegnung (no outcome yet) is still a fixture, here with us listed second.
	if g := fixtures[1]; g.OpponentTeamID != 40001 || g.WeAreTeamOne {
		t.Errorf("second fixture = %+v", g)
	}
}

func TestParseTeamSchedule_RosterNumericIDs(t *testing.T) {
	schedule, err := scraper.ParseTeamSchedule(strings.NewReader(leagueSchedulePage), 39500)
	if err != nil {
		t.Fatalf("ParseTeamSchedule: %v", err)
	}
	if len(schedule.Players) != 2 || schedule.Players[0].NumericID != "2329" || schedule.Players[0].RGX != 1671 {
		t.Errorf("players = %+v", schedule.Players)
	}
}

func TestParseMatchdayGroup(t *testing.T) {
	html := `<html><body>
	<div class="md-group-container md-group-656" data-md-group-id="656"></div>
	<div class="md-group-container md-group-657">
		<div class="md-group-header">
			<span class="md-group-name">Gruppe B</span> | <span class="md-group-date">18.10.2026</span> |
			<span class="md-group-city">
				Karlsruhe
			</span>
		</div>
	</div>
	</body></html>`

	group, err := scraper.ParseMatchdayGroup(strings.NewReader(html), 657)
	if err != nil {
		t.Fatalf("ParseMatchdayGroup: %v", err)
	}
	want := scraper.MatchdayGroup{Name: "Gruppe B", Date: "2026-10-18", City: "Karlsruhe"}
	if group != want {
		t.Errorf("got %+v, want %+v", group, want)
	}

	// The placeholder of a group that wasn't requested has no header to read.
	if _, err := scraper.ParseMatchdayGroup(strings.NewReader(html), 656); err == nil {
		t.Error("expected an error for a placeholder group, got nil")
	}
}

const subgamesJSON = `{"html": "<rg-game-card variant=\"subgame\"><div slot=\"tags\"><span>\n Open 1\n </span></div><div class=\"teams\"><div class=\"team winner\"><a class=\"details\"><span class=\"name\">Schirop/Bottler</span></a></div><div class=\"team loser\"><a class=\"details\"><span class=\"name\">Prüfer/Nerad</span></a></div></div><div class=\"scores\"><div class=\"set\" set=\"1\"><span class=\"score winner\">17</span><span class=\"score loser\">15</span></div><div class=\"set\" set=\"2\"><span class=\"score winner\">15</span><span class=\"score loser\">11</span></div></div></rg-game-card><rg-game-card variant=\"subgame\"><div slot=\"tags\"><span>Damen</span></div><div class=\"teams\"><div class=\"team\"><span class=\"name\">F. Stadler/J. Stadler</span></div><div class=\"team\"><span class=\"name\">Haas/Gilde</span></div></div></rg-game-card>", "results": [], "teams": null}`

func TestParseSubgames(t *testing.T) {
	parties, err := scraper.ParseSubgames(strings.NewReader(subgamesJSON))
	if err != nil {
		t.Fatalf("ParseSubgames: %v", err)
	}
	if len(parties) != 2 {
		t.Fatalf("Partien = %d, want 2", len(parties))
	}

	open := parties[0]
	if open.Label != "Open 1" || len(open.SetScores) != 2 || open.SetScores[0].TeamOneScore != 17 {
		t.Errorf("Open 1 = %+v", open)
	}
	if strings.Join(open.Pairs[0], "+") != "Schirop+Bottler" || strings.Join(open.Pairs[1], "+") != "Prüfer+Nerad" {
		t.Errorf("Open 1 pairs = %v", open.Pairs)
	}

	// Not played yet: no sets.
	if damen := parties[1]; damen.Label != "Damen" || len(damen.SetScores) != 0 || damen.Pairs[0][0] != "F. Stadler" {
		t.Errorf("Damen = %+v", damen)
	}
}

func TestPairNameMatches(t *testing.T) {
	cases := []struct {
		pair, full string
		want       bool
	}{
		{"Bottler", "Jonas Bottler", true},
		{"bottler", "Jonas Bottler", true},
		{"F. Stadler", "Franziska Stadler", true},
		{"J. Stadler", "Franziska Stadler", false},
		{"von Bergen", "Max von Bergen", true},
		{"Bergen", "Max von Bergen", true},
		{"Ottler", "Jonas Bottler", false},
		{"Grönebaum", `Kai "Grö" Grönebaum`, true},
	}
	for _, c := range cases {
		if got := scraper.PairNameMatches(c.pair, c.full); got != c.want {
			t.Errorf("PairNameMatches(%q, %q) = %v, want %v", c.pair, c.full, got, c.want)
		}
	}
}

func TestActiveLeagueSeason(t *testing.T) {
	entries := []scraper.TournamentListEntry{
		{ID: 839, Name: "Swiss Winter Squad Championships 2027"},
		{ID: 842, Name: "Karlsruher Kojoten Cup"},
		{ID: 793, Name: "Deutsche Roundnet Liga 2026/2027"},
	}
	season, ok := scraper.ActiveLeagueSeason(entries)
	if !ok || season.ID != 793 {
		t.Errorf("got %+v, %v; want 793", season, ok)
	}
	if _, ok := scraper.ActiveLeagueSeason(entries[:2]); ok {
		t.Error("expected no season off-season")
	}
}

func TestLeagueParsers_Garbage(t *testing.T) {
	if _, err := scraper.ParseLeagueFixtures(strings.NewReader(garbageHTML), 1); err == nil {
		t.Error("ParseLeagueFixtures: expected an error, got nil")
	}
	if _, err := scraper.ParseMatchdayGroup(strings.NewReader(garbageHTML), 1); err == nil {
		t.Error("ParseMatchdayGroup: expected an error, got nil")
	}
	if _, err := scraper.ParseSubgames(strings.NewReader(garbageHTML)); err == nil {
		t.Error("ParseSubgames (not JSON): expected an error, got nil")
	}
	if _, err := scraper.ParseSubgames(strings.NewReader(`{"html": "` + "<p>nothing</p>" + `"}`)); err == nil {
		t.Error("ParseSubgames (no Partien): expected an error, got nil")
	}
}

// Fields a live DOM change could drop must degrade, not panic or invent data.
func TestLeagueParsers_MissingFields(t *testing.T) {
	noDate := `<div class="md-group-container md-group-1"><div class="md-group-header"><span class="md-group-name">A</span></div></div>`
	if _, err := scraper.ParseMatchdayGroup(strings.NewReader(noDate), 1); err == nil {
		t.Error("ParseMatchdayGroup without a date: expected an error, got nil")
	}

	oneSide := `{"html": "<rg-game-card variant=\"subgame\"><div class=\"teams\"><div class=\"team\"><span class=\"name\">A/B</span></div></div></rg-game-card>"}`
	if _, err := scraper.ParseSubgames(strings.NewReader(oneSide)); err == nil {
		t.Error("ParseSubgames with a one-sided Partie: expected an error, got nil")
	}

	noOpponentID := strings.ReplaceAll(leagueSchedulePage, "load_team_info(28314, 39823, null, null, 324345)", "load_team_info(, )")
	fixtures, err := scraper.ParseLeagueFixtures(strings.NewReader(noOpponentID), 39500)
	if err != nil || len(fixtures) != 1 {
		t.Errorf("a fixture without an opponent ID must be skipped: %v, %+v", err, fixtures)
	}
}

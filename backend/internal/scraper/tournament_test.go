package scraper_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

const (
	divisionTeamsFixture = "../../../scraper/fixtures/division-teams.html"
	teamScheduleFixture  = "../../../scraper/fixtures/team-schedule.html"
)

// Fixtures were captured on 2026-07-19 (Saturday). The Leguan Cup ran 2026-07-18 to 2026-07-19.
var fixtureDate = time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

func TestParseDivisionSidebar(t *testing.T) {
	f, err := os.Open(teamScheduleFixture)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	defer f.Close()

	divs, err := scraper.ParseDivisionSidebar(f)
	if err != nil {
		t.Fatalf("ParseDivisionSidebar: %v", err)
	}
	if len(divs) == 0 {
		t.Fatal("expected at least one division")
	}

	// The team-schedule fixture is itself for the "Intermediate Women" division
	// (div 2263), which must appear in its own sidebar with the matching teams path.
	found := false
	for _, d := range divs {
		if d.ID == "2263" {
			found = true
			if d.Name != "Intermediate Women" {
				t.Errorf("division 2263 name: got %q, want %q", d.Name, "Intermediate Women")
			}
			if d.TeamsPath != "/tournaments/692-2026-07-18-2-leguan-cup/teams/2263" {
				t.Errorf("division 2263 teams path: got %q", d.TeamsPath)
			}
			if d.TournamentID != "692" {
				t.Errorf("division 2263 tournament id: got %q, want %q", d.TournamentID, "692")
			}
		}
	}
	if !found {
		t.Errorf("division 2263 not found; got %+v", divs)
	}
	t.Logf("parsed %d divisions", len(divs))
}

func TestParseDivisionTeams(t *testing.T) {
	f, err := os.Open(divisionTeamsFixture)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	defer f.Close()

	teams, err := scraper.ParseDivisionTeams(f)
	if err != nil {
		t.Fatalf("ParseDivisionTeams: %v", err)
	}
	if len(teams) == 0 {
		t.Fatal("expected at least one team")
	}

	// First team in fixture has data-rgx="2347" and name "Ein Prillantes Team".
	t1 := teams[0]
	if t1.CombinedRGX != 2347 {
		t.Errorf("first team combined RGX: got %d, want 2347", t1.CombinedRGX)
	}
	if t1.Name != "Ein Prillantes Team" {
		t.Errorf("first team name: got %q, want %q", t1.Name, "Ein Prillantes Team")
	}
	if len(t1.PlayerNames) < 2 {
		t.Errorf("expected at least 2 player names in first team, got %v", t1.PlayerNames)
	}
	t.Logf("parsed %d teams; first: %q RGX=%d players=%v", len(teams), t1.Name, t1.CombinedRGX, t1.PlayerNames)
}

func TestParseTeamSchedule(t *testing.T) {
	f, err := os.Open(teamScheduleFixture)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	defer f.Close()

	// The fixture is for team 60615 (La Joce Vita).
	result, err := scraper.ParseTeamSchedule(f, 60615)
	if err != nil {
		t.Fatalf("ParseTeamSchedule: %v", err)
	}

	if result.TeamName != "La Joce Vita" {
		t.Errorf("team name: got %q, want %q", result.TeamName, "La Joce Vita")
	}
	if len(result.Players) < 2 {
		t.Errorf("expected at least 2 players, got %d", len(result.Players))
	}
	if len(result.Games) == 0 {
		t.Fatal("expected at least one game")
	}

	// The first game in the fixture is the Finale (game 434994), outcome=win.
	g0 := result.Games[0]
	if g0.Outcome != "win" {
		t.Errorf("first game outcome: got %q, want %q", g0.Outcome, "win")
	}
	if g0.Round != "Finale" {
		t.Errorf("first game round: got %q, want %q", g0.Round, "Finale")
	}

	t.Logf("team=%q players=%v games=%d", result.TeamName, result.Players, len(result.Games))
}

func TestParseTeamSchedule_SkipsUnplayedGames(t *testing.T) {
	const html = `
<div class="player-card">
	<div class="info-player-name"><a class="info-text">Test Player</a></div>
	<span class="rgx-value">1200</span>
</div>
<div id="game-1">
	<rg-game-card outcome="win">
		<div class="teams">
			<div class="team"><a class="details" onclick="load_team_info(1, 100)"><span class="name">Us</span></a></div>
			<div class="team"><a class="details" onclick="load_team_info(2, 200)"><span class="name">Them</span></a></div>
		</div>
		<div class="scores">
			<div class="set"><span class="score">15</span><span class="score">10</span></div>
		</div>
	</rg-game-card>
</div>
<div id="game-2">
	<rg-game-card outcome="">
		<div class="teams">
			<div class="team"><a class="details" onclick="load_team_info(1, 100)"><span class="name">Us</span></a></div>
			<div class="team"><a class="details" onclick="load_team_info(3, 300)"><span class="name">NotYetPlayed</span></a></div>
		</div>
		<div class="scores"></div>
	</rg-game-card>
</div>
`
	result, err := scraper.ParseTeamSchedule(strings.NewReader(html), 100)
	if err != nil {
		t.Fatalf("ParseTeamSchedule: %v", err)
	}
	if len(result.Games) != 1 {
		t.Fatalf("expected 1 game (unplayed game should be skipped), got %d: %+v", len(result.Games), result.Games)
	}
	if result.Games[0].OpponentName != "Them" {
		t.Errorf("expected the played game to remain, got opponent %q", result.Games[0].OpponentName)
	}
}

func TestParseTeamSchedule_SkipsByeGames(t *testing.T) {
	// Markup mirrors a real bye row (captured from playerzone.roundnetgermany.de/
	// tournaments/schedule/team/61542): outcome="win", empty scores, and the
	// opponent's onclick is "load_team_info(, )" with no team ID.
	const html = `
<div class="player-card">
	<div class="info-player-name"><a class="info-text">Test Player</a></div>
	<span class="rgx-value">1200</span>
</div>
<div id="game-1">
	<rg-game-card outcome="win">
		<div class="teams">
			<div class="team winner"><a class="details" onclick="load_team_info(1, 100)"><span class="name">Us</span></a></div>
			<div class="team loser"><a class="details" href="#!" onclick="load_team_info(, )"><span class="name">Bye</span></a></div>
		</div>
		<div class="scores"></div>
	</rg-game-card>
</div>
<div id="game-2">
	<rg-game-card outcome="win">
		<div class="teams">
			<div class="team"><a class="details" onclick="load_team_info(1, 100)"><span class="name">Us</span></a></div>
			<div class="team"><a class="details" onclick="load_team_info(3, 300)"><span class="name">Played</span></a></div>
		</div>
		<div class="scores">
			<div class="set"><span class="score">15</span><span class="score">10</span></div>
		</div>
	</rg-game-card>
</div>
`
	result, err := scraper.ParseTeamSchedule(strings.NewReader(html), 100)
	if err != nil {
		t.Fatalf("ParseTeamSchedule: %v", err)
	}
	if len(result.Games) != 1 {
		t.Fatalf("expected 1 game (bye should be skipped), got %d: %+v", len(result.Games), result.Games)
	}
	if result.Games[0].OpponentName != "Played" {
		t.Errorf("expected the played game to remain, got opponent %q", result.Games[0].OpponentName)
	}
}

func TestParseTeamSchedule_KeepsCancelledGames(t *testing.T) {
	const html = `
<div class="player-card">
	<div class="info-player-name"><a class="info-text">Test Player</a></div>
	<span class="rgx-value">1200</span>
</div>
<div id="game-1">
	<rg-game-card outcome="" class="slideRight  cancelled ">
		<div slot="tags">
			<a class="tag" href="#">P5-8</a>
			<span class="tag game-cancelled">Cancelled</span>
		</div>
		<div class="teams">
			<div class="team winner"><a class="details" onclick="load_team_info(1, 100)"><span class="name">Us</span></a></div>
			<div class="team loser"><a class="details" onclick="load_team_info(2, 200)"><span class="name">Them</span></a></div>
		</div>
		<div class="scores">
			<div class="set"><span class="score">1</span><span class="score">0</span></div>
		</div>
	</rg-game-card>
</div>
<div id="game-2">
	<rg-game-card outcome="win">
		<div class="teams">
			<div class="team"><a class="details" onclick="load_team_info(1, 100)"><span class="name">Us</span></a></div>
			<div class="team"><a class="details" onclick="load_team_info(3, 300)"><span class="name">Played</span></a></div>
		</div>
		<div class="scores">
			<div class="set"><span class="score">15</span><span class="score">10</span></div>
		</div>
	</rg-game-card>
</div>
`
	result, err := scraper.ParseTeamSchedule(strings.NewReader(html), 100)
	if err != nil {
		t.Fatalf("ParseTeamSchedule: %v", err)
	}
	if len(result.Games) != 2 {
		t.Fatalf("expected 2 games (cancelled game should be kept), got %d: %+v", len(result.Games), result.Games)
	}

	cancelled := result.Games[0]
	if !cancelled.WasCancelled {
		t.Error("expected first game to be marked WasCancelled")
	}
	if cancelled.OpponentName != "Them" {
		t.Errorf("cancelled game opponent: got %q, want %q", cancelled.OpponentName, "Them")
	}
	if len(cancelled.SetScores) != 1 || cancelled.SetScores[0].TeamOneScore != 1 || cancelled.SetScores[0].TeamTwoScore != 0 {
		t.Errorf("cancelled game set scores: got %+v", cancelled.SetScores)
	}

	if result.Games[1].WasCancelled {
		t.Error("expected second (played) game to not be marked WasCancelled")
	}

	gr := scraper.GameResultFromCard(cancelled, 2400, 2400, false)
	if !gr.WasCancelled {
		t.Error("expected GameResult.WasCancelled to be true")
	}
	if gr.Result != "C" {
		t.Errorf("cancelled game result: got %q, want %q", gr.Result, "C")
	}
	if gr.RGXDelta != 0 {
		t.Errorf("cancelled game should not affect RGX, got delta %d", gr.RGXDelta)
	}
	if len(gr.Sets) != 1 || gr.Sets[0].OurScore != 1 || gr.Sets[0].OpponentScore != 0 {
		t.Errorf("cancelled game oriented sets: got %+v", gr.Sets)
	}
}

func TestComputeGameDelta(t *testing.T) {
	// Test the RGX formula with known values.
	// r1=2347 (our team), r2=2401 (opponents): equal-ish strength.
	// Expected outcome: win 2:0 → should get positive delta.
	game := scraper.GameCard{
		WeAreTeamOne: true,
		SetScores: []scraper.SetScore{
			{TeamOneScore: 21, TeamTwoScore: 15},
			{TeamOneScore: 21, TeamTwoScore: 18},
		},
		Outcome: "win",
	}
	delta := scraper.ComputeGameDelta(2347, 2401, game)
	if delta <= 0 {
		t.Errorf("expected positive delta for 2:0 win against stronger opponents, got %d", delta)
	}

	// Loss 0:2 should produce a negative delta.
	loss := scraper.GameCard{
		WeAreTeamOne: true,
		SetScores: []scraper.SetScore{
			{TeamOneScore: 15, TeamTwoScore: 21},
			{TeamOneScore: 18, TeamTwoScore: 21},
		},
		Outcome: "loss",
	}
	lossD := scraper.ComputeGameDelta(2347, 2401, loss)
	if lossD >= 0 {
		t.Errorf("expected negative delta for 0:2 loss, got %d", lossD)
	}

	t.Logf("win delta=%d, loss delta=%d", delta, lossD)
}

func TestDivisionStartingRGX(t *testing.T) {
	cases := []struct {
		division string
		want     int
	}{
		{"Beginner", 1000},
		{"Basic Open", 1000},
		{"Intermediate Women", 1100},
		{"Advanced Mixed", 1250},
		{"Contender Open", 1400},
		{"Pro Division", 1600},
		{"Unknown", 1200},
	}
	for _, tc := range cases {
		got := scraper.DivisionStartingRGX(tc.division)
		if got != tc.want {
			t.Errorf("DivisionStartingRGX(%q) = %d, want %d", tc.division, got, tc.want)
		}
	}
}

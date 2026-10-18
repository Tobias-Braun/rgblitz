package scraper_test

import (
	"strings"
	"testing"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// Playerzone rebuilt its tournament pages around web components in September 2026: RGX moved
// from a text span into an <rg-score-badge score="…">, the division sidebar became a
// tournament navigation bar, the profile's tournament list became expandable cards, and game
// cards started calling load_team_info with extra arguments. Every parser has to read both
// that layout and the older one, since the fixtures captured before the change are still the
// offline test corpus. The old layout is covered by the fixture-backed tests; these pin the
// new one down with the minimum markup each parser needs, so they keep running even where
// the (gitignored) fixtures aren't available.

func TestParseRankingPage_NewScoreBadge(t *testing.T) {
	html := `
	<table id="rgx-main-table"><tbody>
		<tr>
			<td class="pos-col">1.</td>
			<td class="player-col img-col">
				<div class="player-img"><img src="https://cdn/assets/players/profile_LUC2110-2026-02-12-14-03-16.jpg"></div>
				<div class="player-name"><a href="#">Lucas Christiani</a></div>
				<span class="player-club hide-on-med-and-down">Torpedo Phoenix Roundnet Dortmund</span>
			</td>
			<td class="games-col">373</td>
			<td class="rgx-col">
				<a onclick="show_history(2110, 1)">
					<rg-score-badge score="1912" logo-src="x.png" diff-icon="⬆"></rg-score-badge>
				</a>
			</td>
		</tr>
		<tr>
			<td class="player-col img-col">
				<div class="player-img"><img src="https://cdn/assets/players/profile_NEW9999-2026-08-01-09-00-00.jpg"></div>
				<div class="player-name"><a href="#">New Player</a></div>
			</td>
			<td class="games-col">0</td>
			<td class="rgx-col"></td>
		</tr>
	</tbody></table>`

	players, err := scraper.ParseRankingPage(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseRankingPage: %v", err)
	}
	if len(players) != 2 {
		t.Fatalf("parsed %d players, want 2", len(players))
	}
	if players[0].ID != "LUC2110" || players[0].RGX != 1912 {
		t.Errorf("first player: got id=%q rgx=%d, want LUC2110 / 1912", players[0].ID, players[0].RGX)
	}
	if players[0].ClubName != "Torpedo Phoenix Roundnet Dortmund" {
		t.Errorf("club: got %q", players[0].ClubName)
	}
	// A player with no RGX yet renders no badge at all; that has to read as 0 so the
	// division-proxy logic downstream takes over, not as a parse failure.
	if players[1].RGX != 0 {
		t.Errorf("player without a badge: got rgx=%d, want 0", players[1].RGX)
	}
}

func TestParseProfilePage_NewCardLayout(t *testing.T) {
	html := `
	<ul>
		<li class="rg-player-tournaments--item" data-year="2026">
			<rg-expandable-card title="Biber Fieber" subtitle="05.09.26, Bamberg">
				<div class="rg-player-tournaments--actions">
					<rg-button size="small" href="/tournaments/791-2026-09-05-biber-fieber">Tournament Details</rg-button>
					<rg-button size="small" type="button" onclick="load_tournament_games(62594, 'Biber Fieber')">Games</rg-button>
				</div>
			</rg-expandable-card>
		</li>
		<li class="rg-player-tournaments--item" data-year="2026">
			<rg-expandable-card title="Registered, not played" subtitle="19.09.26, Ulm">
				<div class="rg-player-tournaments--actions">
					<rg-button size="small" href="/tournaments/818-2026-09-19-ulmer-spatzenschau">Tournament Details</rg-button>
				</div>
			</rg-expandable-card>
		</li>
	</ul>`

	entries, err := scraper.ParseProfilePage(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseProfilePage: %v", err)
	}
	// The second card has no "Games" button (no bracket yet), so it carries no team ID and
	// must be skipped rather than guessed at.
	if len(entries) != 1 {
		t.Fatalf("parsed %d entries, want 1", len(entries))
	}
	if entries[0].Date != "05.09.26" {
		t.Errorf("date: got %q, want %q (the subtitle's leading date, without the location)", entries[0].Date, "05.09.26")
	}
	if entries[0].TournamentTeamID != 62594 {
		t.Errorf("team id: got %d, want 62594", entries[0].TournamentTeamID)
	}
}

// newTeamSchedulePage is a played team's schedule in the current layout: no section heading
// or sidebar, the division in the tournament navigation, RGX in badges, and game cards whose
// load_team_info calls carry five arguments.
const newTeamSchedulePage = `
<html><head><title>
	Schedule LyRik - Basic Mixed
</title></head><body>
<div class="rg-tournament-nav">
	<a class="rg-tournament-nav--identity" href="/tournaments/791-2026-09-05-biber-fieber">
		<span class="rg-tournament-nav--title">Biber Fieber</span>
	</a>
	<nav class="rg-tournament-nav--tabs">
		<a class="rg-tournament-nav--tab" href="/tournaments/791-2026-09-05-biber-fieber/teams/2580">Results</a>
	</nav>
</div>
<div class="rg-tournament-nav--context">
	<rg-navigation-item type="inline">
		Basic Mixed
		<rg-navigation-dropdown slot="dropdown" type="inline" columns>
			<div class="rg-tournament-nav--division-group">
				<rg-navigation-dropdown-item type="inline" href="/tournaments/791-2026-09-05-biber-fieber/schedule/mixed" class="active">Basic Mixed</rg-navigation-dropdown-item>
			</div>
		</rg-navigation-dropdown>
	</rg-navigation-item>
</div>

<div class="player-card">
	<div class="info-player-name"><a href="/players/8375-erik">Erik Ziedelmann</a></div>
	<div class="player-sm"><rg-score-badge score="1075" logo-src="x.png"></rg-score-badge></div>
</div>
<div class="player-card">
	<div class="info-player-name"><a href="/players/8376-lydia">Lydia Fritsch</a></div>
	<div class="player-sm"><rg-score-badge score="1102" logo-src="x.png"></rg-score-badge></div>
</div>

<div id="game-457400">
	<rg-game-card variant="game" outcome="win">
		<div slot="tags"><a class="tag" href="#">Finale</a></div>
		<div class="teams">
			<div class="team loser">
				<a class="details" onclick="load_team_info(42202, 62386, null, null, 457400)">
					<span class="name">Marielle und Kai der Hai</span>
				</a>
			</div>
			<div class="team winner">
				<a class="details" onclick="load_team_info(40744, 62594, null, null, 457400)">
					<span class="name">LyRik</span>
				</a>
			</div>
		</div>
		<div class="scores">
			<div class="set" set="1"><span class="score loser">14</span><span class="score winner">17</span></div>
			<div class="set" set="2"><span class="score winner">17</span><span class="score loser">15</span></div>
		</div>
	</rg-game-card>
</div>
</body></html>`

func TestParseTeamSchedule_NewLayout(t *testing.T) {
	schedule, err := scraper.ParseTeamSchedule(strings.NewReader(newTeamSchedulePage), 62594)
	if err != nil {
		t.Fatalf("ParseTeamSchedule: %v", err)
	}

	if schedule.DivisionName != "Basic Mixed" {
		t.Errorf("division: got %q, want %q", schedule.DivisionName, "Basic Mixed")
	}
	if schedule.TeamName != "LyRik" {
		t.Errorf("team: got %q, want %q", schedule.TeamName, "LyRik")
	}

	if len(schedule.Players) != 2 {
		t.Fatalf("parsed %d players, want 2", len(schedule.Players))
	}
	// The whole point of the badge change: reading 0 here silently replaces both players'
	// real RGX with a division starting value and every delta of the day comes out wrong.
	if schedule.Players[0].RGX != 1075 || schedule.Players[1].RGX != 1102 {
		t.Errorf("player RGX: got %d and %d, want 1075 and 1102",
			schedule.Players[0].RGX, schedule.Players[1].RGX)
	}

	if len(schedule.Games) != 1 {
		t.Fatalf("parsed %d games, want 1", len(schedule.Games))
	}
	g := schedule.Games[0]
	if g.GameID != 457400 || g.Round != "Finale" {
		t.Errorf("game: got id=%d round=%q", g.GameID, g.Round)
	}
	// Both of these come from the load_team_info call, which now passes five arguments.
	if g.OpponentName != "Marielle und Kai der Hai" {
		t.Errorf("opponent: got %q", g.OpponentName)
	}
	if g.WeAreTeamOne {
		t.Error("WeAreTeamOne = true, but our team is listed second — set scores would be mirrored")
	}
	if len(g.SetScores) != 2 || g.SetScores[0] != (scraper.SetScore{TeamOneScore: 14, TeamTwoScore: 17}) {
		t.Errorf("sets: got %+v", g.SetScores)
	}
}

// A schedule page linking exactly one division — its own — must still yield that link, since
// it is where the pipeline gets the division team list from.
func TestParseDivisionSidebar_NewLayoutSingleDivision(t *testing.T) {
	divs, err := scraper.ParseDivisionSidebar(strings.NewReader(newTeamSchedulePage))
	if err != nil {
		t.Fatalf("ParseDivisionSidebar: %v", err)
	}
	if len(divs) != 1 {
		t.Fatalf("parsed %d divisions, want 1", len(divs))
	}
	if divs[0].TeamsPath != "/tournaments/791-2026-09-05-biber-fieber/teams/2580" {
		t.Errorf("teams path: got %q", divs[0].TeamsPath)
	}
	if divs[0].TournamentID != "791" || divs[0].ID != "2580" {
		t.Errorf("ids: got tournament=%q division=%q, want 791 / 2580", divs[0].TournamentID, divs[0].ID)
	}
	// The link's own text is the navigation tab's label ("Results"), so the name has to come
	// from the division the page is showing instead.
	if divs[0].Name != "Basic Mixed" {
		t.Errorf("name: got %q, want %q (taken from the active navigation entry)", divs[0].Name, "Basic Mixed")
	}
}

// A tournament with grouped divisions splits the name across the group ("Women") and the
// entry ("Advanced/Contender/Pro"). Schedule pages spell it tier-first, and the two have to
// match for the pipeline to find the right division.
func TestParseDivisionSidebar_NewLayoutGroupedDivisions(t *testing.T) {
	html := `
	<div class="rg-tournament-nav--context">
		<rg-navigation-item type="inline">
			Basic/Intermediate Open
			<rg-navigation-dropdown slot="dropdown" type="inline" columns>
				<div class="rg-tournament-nav--division-group" role="group" aria-label="Open">
					<span class="rg-tournament-nav--division-group-label">Open</span>
					<rg-navigation-dropdown-item type="inline" href="/tournaments/695-2026-09-12-hamburger-hafen-spritz-tour/teams/2430" class="active">Basic/Intermediate</rg-navigation-dropdown-item>
					<rg-navigation-dropdown-item type="inline" href="/tournaments/695-2026-09-12-hamburger-hafen-spritz-tour/teams/2431">Advanced</rg-navigation-dropdown-item>
				</div>
				<div class="rg-tournament-nav--division-group" role="group" aria-label="Women">
					<span class="rg-tournament-nav--division-group-label">Women</span>
					<rg-navigation-dropdown-item type="inline" href="/tournaments/695-2026-09-12-hamburger-hafen-spritz-tour/teams/2436">Advanced/Contender/Pro</rg-navigation-dropdown-item>
				</div>
			</rg-navigation-dropdown>
		</rg-navigation-item>
	</div>`

	divs, err := scraper.ParseDivisionSidebar(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseDivisionSidebar: %v", err)
	}
	if len(divs) != 3 {
		t.Fatalf("parsed %d divisions, want 3", len(divs))
	}

	want := map[string]string{
		"2430": "Basic/Intermediate Open",
		"2431": "Advanced Open",
		"2436": "Advanced/Contender/Pro Women",
	}
	for _, d := range divs {
		if want[d.ID] != d.Name {
			t.Errorf("division %s: got name %q, want %q", d.ID, d.Name, want[d.ID])
		}
	}
}

func TestParseTournamentDetail_NewTitleLocation(t *testing.T) {
	html := `
	<html><head><title>Biber Fieber - RG Playerzone</title></head><body>
	<a class="rg-tournament-nav--identity" href="/tournaments/791-2026-09-05-biber-fieber">
		<span class="rg-tournament-nav--title">Biber Fieber</span>
	</a>
	<div class="card-panel tournament-details">
		<div class="row info-row">
			<p class="tournament-info">Date: <b>Sept. 5, 2026</b></p>
		</div>
		<div class="row info-row">
			<div class="col s12"><img class="logo" src="grt.png">&nbsp;German Roundnet Tour 2026</div>
		</div>
	</div>
	</body></html>`

	detail, err := scraper.ParseTournamentDetail(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseTournamentDetail: %v", err)
	}
	if detail.Name != "Biber Fieber" {
		t.Errorf("name: got %q, want %q", detail.Name, "Biber Fieber")
	}
	if !detail.IsGRT {
		t.Error("IsGRT = false, want true")
	}
	if detail.Date != "2026-09-05" || detail.EndDate != "2026-09-05" {
		t.Errorf("dates: got %s..%s, want 2026-09-05 twice", detail.Date, detail.EndDate)
	}
}

// The title is the last resort, for a tournament-scoped page that renders neither heading.
func TestParseTournamentDetail_FallsBackToDocumentTitle(t *testing.T) {
	html := `
	<html><head><title>
		Hamburger Hafen Spritz Tour - RG Playerzone
	</title></head><body>
	<div class="card-panel tournament-details">
		<div class="row info-row"><p class="tournament-info">Date: <b>Sept. 12, 2026 - Sept. 13, 2026</b></p></div>
		<div class="row info-row"><div class="col s12">Not rated</div></div>
	</div>
	</body></html>`

	detail, err := scraper.ParseTournamentDetail(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseTournamentDetail: %v", err)
	}
	if detail.Name != "Hamburger Hafen Spritz Tour" {
		t.Errorf("name: got %q", detail.Name)
	}
	if detail.IsGRT {
		t.Error("IsGRT = true for a page with no GRT label")
	}
	if detail.EndDate != "2026-09-13" {
		t.Errorf("end date: got %q, want 2026-09-13 (second day of the range)", detail.EndDate)
	}
}

func TestParseTournamentID(t *testing.T) {
	id, err := scraper.ParseTournamentID(strings.NewReader(newTeamSchedulePage))
	if err != nil {
		t.Fatalf("ParseTournamentID: %v", err)
	}
	if id != 791 {
		t.Errorf("got %d, want 791", id)
	}

	if _, err := scraper.ParseTournamentID(strings.NewReader(garbageHTML)); err == nil {
		t.Error("expected an error for a page with no tournament link, got nil")
	}
}

// Every live page opens with the site navigation, whose "League" item links the current league
// season — it must not be mistaken for the tournament the page belongs to.
func TestParseTournamentID_IgnoresSiteNavigation(t *testing.T) {
	html := `<html><body>
	<rg-navigation>
		<rg-navigation-item href="/tournaments/793-2027-04-11-deutsche-roundnet-liga-2026-2027">League</rg-navigation-item>
	</rg-navigation>
	<a class="rg-tournament-nav--identity" href="/tournaments/695-2026-09-12-hamburger-hafen-spritz-tour">Hamburg</a>
	</body></html>`

	id, err := scraper.ParseTournamentID(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseTournamentID: %v", err)
	}
	if id != 695 {
		t.Errorf("got %d, want 695", id)
	}
}

// Before a tournament starts, its schedule pages carry a summary line instead of the
// navigation, and no division link at all.
func TestParseTeamSchedule_UpcomingTournamentLayout(t *testing.T) {
	html := `
	<html><head><title>Hai on life - RG Playerzone</title></head><body>
	<div class="row section-title"><h3 class="event-title">Hai on life</h3></div>
	<div class="tournament-event">
		<div class="tournament-name"><a href="/tournaments/695-2026-09-12-hamburger-hafen-spritz-tour">Hamburger Hafen Spritz Tour</a></div>
		<div class="tournament-division">Advanced/Contender/Pro Women, 15 Teams</div>
		<div class="tournament-team">Hai on life</div>
	</div>
	<div class="player-card">
		<div class="info-player-name"><a href="/players/7958-luisa">Luisa &quot;Isi&quot; Dubrau</a></div>
		<div class="player-sm"><rg-score-badge score="1078"></rg-score-badge></div>
	</div>
	</body></html>`

	schedule, err := scraper.ParseTeamSchedule(strings.NewReader(html), 62300)
	if err != nil {
		t.Fatalf("ParseTeamSchedule: %v", err)
	}
	// "Advanced/Contender/Pro Women, 15 Teams" — the team count isn't part of the name, and
	// h3.event-title holds the team here, not the division as it did in the old layout.
	if schedule.DivisionName != "Advanced/Contender/Pro Women" {
		t.Errorf("division: got %q", schedule.DivisionName)
	}
	if schedule.TeamName != "Hai on life" {
		t.Errorf("team: got %q", schedule.TeamName)
	}
	if len(schedule.Players) != 1 || schedule.Players[0].RGX != 1078 {
		t.Fatalf("players: got %+v", schedule.Players)
	}
	if len(schedule.Games) != 0 {
		t.Errorf("games: got %d, want 0 for a tournament that hasn't started", len(schedule.Games))
	}
}

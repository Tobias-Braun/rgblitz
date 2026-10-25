package scraper_test

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// pageFetcher serves pages by path prefix and counts requests.
type pageFetcher struct {
	pages    map[string]string
	requests int
}

func (f *pageFetcher) Fetch(path string) (io.ReadCloser, error) {
	f.requests++
	for prefix, page := range f.pages {
		if strings.HasPrefix(path, prefix) {
			return io.NopCloser(strings.NewReader(page)), nil
		}
	}
	return nil, fmt.Errorf("unexpected request %s", path)
}

func matchdayPage(groupID int, date string) string {
	return fmt.Sprintf(`<div class="md-group-container md-group-%d"><div class="md-group-header">
		<span class="md-group-name">Gruppe S</span> | <span class="md-group-date">%s</span> | <span class="md-group-city">Karlsruhe</span>
	</div></div>`, groupID, date)
}

const opponentSchedulePage = `<html><body>
<div class="player-card"><div class="info-player-name"><a href="/players/50-a">Anna Prüfer</a></div><rg-score-badge score="1400"></rg-score-badge></div>
<div class="player-card"><div class="info-player-name"><a href="/players/51-b">Ben Nerad</a></div></div>
<div id="game-324345">
	<rg-game-card variant="game" outcome="loss">
		<div slot="tags"><a class="tag" href="/tournaments/543-x/schedule/mixed?md=76&amp;md_group=525">5. Matchday #3</a></div>
		<div slot="actions"><rg-icon onclick="show_subgames('324345')"></rg-icon></div>
		<div class="teams">
			<div class="team"><a class="details" onclick="load_team_info(30819, 39500, null, null, 324345)"><span class="name">KA-Boom</span></a></div>
			<div class="team"><a class="details" onclick="load_team_info(28314, 39823, null, null, 324345)"><span class="name">MaMa Mia</span></a></div>
		</div>
	</rg-game-card>
</div>
</body></html>`

// Open 1 played by our player; Open 2 played without them; Damen not played yet.
const playedSubgames = `{"html": "` +
	`<rg-game-card variant=\"subgame\"><div slot=\"tags\"><span>Open 1</span></div><div class=\"teams\"><div class=\"team\"><span class=\"name\">Schirop/Bottler</span></div><div class=\"team\"><span class=\"name\">Prüfer/Nerad</span></div></div><div class=\"scores\"><div class=\"set\"><span class=\"score\">17</span><span class=\"score\">15</span></div><div class=\"set\"><span class=\"score\">15</span><span class=\"score\">11</span></div></div></rg-game-card>` +
	`<rg-game-card variant=\"subgame\"><div slot=\"tags\"><span>Open 2</span></div><div class=\"teams\"><div class=\"team\"><span class=\"name\">Schirop/Werner</span></div><div class=\"team\"><span class=\"name\">Prüfer/Nerad</span></div></div><div class=\"scores\"><div class=\"set\"><span class=\"score\">17</span><span class=\"score\">15</span></div></div></rg-game-card>` +
	`<rg-game-card variant=\"subgame\"><div slot=\"tags\"><span>Women</span></div><div class=\"teams\"><div class=\"team\"><span class=\"name\">Bottler/Schirop</span></div><div class=\"team\"><span class=\"name\">Prüfer/Nerad</span></div></div></rg-game-card>` +
	`"}`

// unplayedSubgames is a Begegnung with one Partie the player hasn't played yet.
const unplayedSubgames = `{"html": "<rg-game-card variant=\"subgame\"><div class=\"teams\"><div class=\"team\"><span class=\"name\">A/B</span></div><div class=\"team\"><span class=\"name\">Bottler/Schirop</span></div></div></rg-game-card>"}`

func newLeagueDeps(t *testing.T, fetcher scraper.Fetcher, members ...db.LeagueMembership) scraper.ScrapeDeps {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "league.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	league := db.NewLeagueStore(conn)
	if err := league.SaveSeason(543, members); err != nil {
		t.Fatalf("save season: %v", err)
	}
	return scraper.ScrapeDeps{Fetcher: fetcher, Cache: cache.New(), ProxyURL: func(s string) string { return s }, League: league, Players: db.NewPlayerStore(conn)}
}

func TestScrapePlayerLeagueToday(t *testing.T) {
	fetcher := &pageFetcher{pages: map[string]string{
		"/tournaments/schedule/team/39500": leagueSchedulePage,
		"/tournaments/schedule/team/39823": opponentSchedulePage,
		"/tournaments/schedule/team/40001": opponentSchedulePage,
		"/tournaments/543-x/schedule/":     matchdayPage(525, "21.02.2026"),
		"/ajax/get-subgames/324345":        playedSubgames,
		"/ajax/get-subgames/324333":        unplayedSubgames,
	}}
	deps := newLeagueDeps(t, fetcher, db.LeagueMembership{PlayerID: "2329", TeamID: 39500})
	player := &db.Player{ID: "JON2329", Name: "Jonas Bottler", RGX: 1600}

	result, tournamentID, handled, err := scraper.ScrapePlayerLeagueToday(deps, player, time.Date(2026, 2, 21, 15, 0, 0, 0, time.UTC))
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if tournamentID != 543 || result.Status != "ok" || result.Competition != scraper.CompetitionLeague {
		t.Errorf("result = %+v, tournament %d", result, tournamentID)
	}
	// The roster's RGX is today's baseline, not the index value.
	if result.Player.RGX != 1671 {
		t.Errorf("player RGX = %d, want 1671", result.Player.RGX)
	}

	if len(result.Games) != 1 {
		t.Fatalf("games = %+v, want only the played Open 1", result.Games)
	}
	g := result.Games[0]
	// Ben Nerad has no RGX: counted with the 1200 proxy.
	if g.TeamRGX != 1500+1671 || g.OpponentRGX != 1400+1200 || !g.HasProxy {
		t.Errorf("r1/r2 = %d/%d proxy=%v", g.TeamRGX, g.OpponentRGX, g.HasProxy)
	}
	want := scraper.ComputeDeltaBreakdown(3171, 2600, 2, 0).Delta
	if g.Result != "W" || g.RGXDelta != want || result.TotalDelta != want {
		t.Errorf("game = %+v, total %d; want W with delta %d", g, result.TotalDelta, want)
	}
	if g.Round != "Open 1 · 5. Matchday #3" || g.OurTeamName != "Schirop/Bottler" || g.OpponentName != "Prüfer/Nerad (MaMa Mia)" {
		t.Errorf("labels = %q / %q / %q", g.Round, g.OurTeamName, g.OpponentName)
	}

	// A second teammate's request reuses every cached page except the live Partien.
	before := fetcher.requests
	if _, _, _, err := scraper.ScrapePlayerLeagueToday(deps, player, time.Date(2026, 2, 21, 15, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if extra := fetcher.requests - before; extra != 0 {
		t.Errorf("repeat request made %d new requests within the live TTL, want 0", extra)
	}
}

// The roster badge shows a woman's Women RGX, so her Open Partien take her Open RGX from the
// player index, and a woman with no Open RGX is proxied. Mixed Partien and men keep the badge.
// Only a woman's Open Partie is marked as counting towards her Open RGX.
func TestScrapePlayerLeagueToday_WomanInOpenPartie(t *testing.T) {
	const subgames = `{"html": "` +
		`<rg-game-card variant=\"subgame\"><div slot=\"tags\"><span>Open 1</span></div><div class=\"teams\"><div class=\"team\"><span class=\"name\">Schirop/Bottler</span></div><div class=\"team\"><span class=\"name\">Prüfer/Nerad</span></div></div><div class=\"scores\"><div class=\"set\"><span class=\"score\">17</span><span class=\"score\">15</span></div></div></rg-game-card>` +
		`<rg-game-card variant=\"subgame\"><div slot=\"tags\"><span>Mixed 1</span></div><div class=\"teams\"><div class=\"team\"><span class=\"name\">Schirop/Bottler</span></div><div class=\"team\"><span class=\"name\">Prüfer/Nerad</span></div></div><div class=\"scores\"><div class=\"set\"><span class=\"score\">17</span><span class=\"score\">15</span></div></div></rg-game-card>` +
		`"}`
	fetcher := &pageFetcher{pages: map[string]string{
		"/tournaments/schedule/team/39500": leagueSchedulePage,
		"/tournaments/schedule/team/39823": opponentSchedulePage,
		"/tournaments/schedule/team/40001": opponentSchedulePage,
		"/tournaments/543-x/schedule/":     matchdayPage(525, "21.02.2026"),
		"/ajax/get-subgames/324345":        subgames,
		"/ajax/get-subgames/324333":        unplayedSubgames,
	}}
	deps := newLeagueDeps(t, fetcher, db.LeagueMembership{PlayerID: "2329", TeamID: 39500}, db.LeagueMembership{PlayerID: "1001", TeamID: 39500})
	lena := db.Player{ID: "PZ1001", Name: "Lena Schirop", RGX: 1490, OpenRGX: 1350, WomenRGX: 1490}
	for _, p := range []db.Player{
		{ID: "JON2329", Name: "Jonas Bottler", RGX: 1650, OpenRGX: 1650},
		lena,
		{ID: "PZ50", Name: "Anna Prüfer", RGX: 1390, WomenRGX: 1390},
	} {
		if err := deps.Players.Upsert(p); err != nil {
			t.Fatal(err)
		}
	}

	result, _, _, err := scraper.ScrapePlayerLeagueToday(deps, &db.Player{ID: "JON2329", Name: "Jonas Bottler"}, time.Date(2026, 2, 21, 15, 0, 0, 0, time.UTC))
	if err != nil || len(result.Games) != 2 {
		t.Fatalf("games = %+v, err = %v; want Open 1 and Mixed 1", result.Games, err)
	}
	// Open 1: Lena's index Open RGX, Jonas's badge; Anna has no Open RGX and Ben no RGX at all.
	if g := result.Games[0]; g.TeamRGX != 1350+1671 || g.OpponentRGX != 1200+1200 || !g.HasProxy {
		t.Errorf("Open 1 r1/r2 = %d/%d proxy=%v, want %d/%d proxied", g.TeamRGX, g.OpponentRGX, g.HasProxy, 1350+1671, 2400)
	}
	// Mixed 1: every roster badge as it is, only Ben proxied.
	if g := result.Games[1]; g.TeamRGX != 1500+1671 || g.OpponentRGX != 1400+1200 {
		t.Errorf("Mixed 1 r1/r2 = %d/%d, want %d/%d", g.TeamRGX, g.OpponentRGX, 1500+1671, 1400+1200)
	}
	if result.Games[0].OpenRating || result.Games[1].OpenRating {
		t.Error("a man's game is marked as an Open rating")
	}

	result, _, _, err = scraper.ScrapePlayerLeagueToday(deps, &lena, time.Date(2026, 2, 21, 15, 0, 0, 0, time.UTC))
	if err != nil || len(result.Games) != 2 {
		t.Fatalf("Lena: games = %+v, err = %v", result.Games, err)
	}
	if !result.Games[0].OpenRating || result.Games[1].OpenRating {
		t.Errorf("Lena open_rating = %v/%v, want Open 1 only", result.Games[0].OpenRating, result.Games[1].OpenRating)
	}
}

// A player on two rosters is found through the team that plays today, even when the other
// roster is checked first.
func TestScrapePlayerLeagueToday_SeveralRosters(t *testing.T) {
	otherTeamPage := strings.NewReplacer("39823", "30000", "543-x", "543-y", "md_group=525", "md_group=526").Replace(opponentSchedulePage)
	fetcher := &pageFetcher{pages: map[string]string{
		"/tournaments/schedule/team/30000": otherTeamPage,
		"/tournaments/543-y/schedule/":     matchdayPage(526, "22.02.2026"),
		"/tournaments/schedule/team/39500": leagueSchedulePage,
		"/tournaments/schedule/team/39823": opponentSchedulePage,
		"/tournaments/schedule/team/40001": opponentSchedulePage,
		"/tournaments/543-x/schedule/":     matchdayPage(525, "21.02.2026"),
		"/ajax/get-subgames/324345":        playedSubgames,
		"/ajax/get-subgames/324333":        unplayedSubgames,
	}}
	deps := newLeagueDeps(t, fetcher,
		db.LeagueMembership{PlayerID: "2329", TeamID: 39500},
		db.LeagueMembership{PlayerID: "2329", TeamID: 30000},
	)

	result, tournamentID, handled, err := scraper.ScrapePlayerLeagueToday(deps, &db.Player{ID: "JON2329", Name: "Jonas Bottler"}, time.Date(2026, 2, 21, 15, 0, 0, 0, time.UTC))
	if err != nil || !handled || tournamentID != 543 {
		t.Fatalf("handled=%v tournament=%d err=%v", handled, tournamentID, err)
	}
	if len(result.Games) != 1 || result.Games[0].OurTeamName != "Schirop/Bottler" {
		t.Errorf("games = %+v, want the Open 1 played for team 39500", result.Games)
	}
}

// Until 02:00 the previous day's matchday is still today's.
func TestScrapePlayerLeagueToday_AfterMidnight(t *testing.T) {
	fetcher := &pageFetcher{pages: map[string]string{
		"/tournaments/schedule/team/39500": leagueSchedulePage,
		"/tournaments/schedule/team/39823": opponentSchedulePage,
		"/tournaments/schedule/team/40001": opponentSchedulePage,
		"/tournaments/543-x/schedule/":     matchdayPage(525, "21.02.2026"),
		"/ajax/get-subgames/324345":        playedSubgames,
		"/ajax/get-subgames/324333":        unplayedSubgames,
	}}
	deps := newLeagueDeps(t, fetcher, db.LeagueMembership{PlayerID: "2329", TeamID: 39500})
	player := &db.Player{ID: "JON2329", Name: "Jonas Bottler"}

	result, _, handled, err := scraper.ScrapePlayerLeagueToday(deps, player, time.Date(2026, 2, 22, 1, 30, 0, 0, time.UTC))
	if err != nil || !handled || result.Date != "2026-02-21" || len(result.Games) != 1 {
		t.Errorf("01:30: handled=%v err=%v date=%q games=%d; want the 21.02. matchday", handled, err, result.Date, len(result.Games))
	}
	if _, _, handled, _ := scraper.ScrapePlayerLeagueToday(deps, player, time.Date(2026, 2, 22, 2, 30, 0, 0, time.UTC)); handled {
		t.Error("02:30: still handled, want the 21.02. matchday over")
	}
}

func TestScrapePlayerLeagueToday_NotHandled(t *testing.T) {
	fetcher := &pageFetcher{pages: map[string]string{
		"/tournaments/schedule/team/39500": leagueSchedulePage,
		"/tournaments/543-x/schedule/":     matchdayPage(525, "21.02.2026"),
	}}
	deps := newLeagueDeps(t, fetcher, db.LeagueMembership{PlayerID: "2329", TeamID: 39500})

	// Not on any roster: no playerzone request at all.
	_, _, handled, err := scraper.ScrapePlayerLeagueToday(deps, &db.Player{ID: "PZ7", Name: "Someone"}, time.Date(2026, 2, 21, 15, 0, 0, 0, time.UTC))
	if handled || err != nil || fetcher.requests != 0 {
		t.Errorf("non-league player: handled=%v err=%v requests=%d", handled, err, fetcher.requests)
	}

	// On a roster, but the team doesn't play today.
	_, _, handled, err = scraper.ScrapePlayerLeagueToday(deps, &db.Player{ID: "JON2329", Name: "Jonas Bottler"}, time.Date(2026, 2, 22, 15, 0, 0, 0, time.UTC))
	if handled || err != nil {
		t.Errorf("no matchday today: handled=%v err=%v", handled, err)
	}
}

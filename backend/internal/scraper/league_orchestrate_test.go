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
	return scraper.ScrapeDeps{Fetcher: fetcher, Cache: cache.New(), ProxyURL: func(s string) string { return s }, League: league}
}

func TestScrapePlayerLeagueToday(t *testing.T) {
	fetcher := &pageFetcher{pages: map[string]string{
		"/tournaments/schedule/team/39500": leagueSchedulePage,
		"/tournaments/schedule/team/39823": opponentSchedulePage,
		"/tournaments/schedule/team/40001": opponentSchedulePage,
		"/tournaments/543-x/schedule/":     matchdayPage(525, "21.02.2026"),
		"/ajax/get-subgames/324345":        playedSubgames,
		"/ajax/get-subgames/324333":        `{"html": "<rg-game-card variant=\"subgame\"><div class=\"teams\"><div class=\"team\"><span class=\"name\">A/B</span></div><div class=\"team\"><span class=\"name\">Bottler/Schirop</span></div></div></rg-game-card>"}`,
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

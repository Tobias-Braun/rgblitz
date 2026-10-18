package db_test

import (
	"path/filepath"
	"testing"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

func newTestLeagueStore(t *testing.T) *db.LeagueStore {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "league.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return db.NewLeagueStore(conn)
}

func TestLeagueStore_SaveSeasonThenLookup(t *testing.T) {
	store := newTestLeagueStore(t)

	if got, err := store.TeamsForPlayer("2329"); err != nil || len(got) != 0 {
		t.Fatalf("lookup on empty table = %+v, %v; want none", got, err)
	}

	if err := store.SaveSeason(543, []db.LeagueMembership{{PlayerID: "2329", TeamID: 39500}, {PlayerID: "7", TeamID: 39500}}); err != nil {
		t.Fatalf("save 2025/26: %v", err)
	}
	if err := store.SaveSeason(793, []db.LeagueMembership{{PlayerID: "2329", TeamID: 59549}}); err != nil {
		t.Fatalf("save 2026/27: %v", err)
	}

	got, err := store.TeamsForPlayer("2329")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(got) != 1 || got[0].TeamID != 59549 || got[0].TournamentID != 793 {
		t.Errorf("memberships = %+v, want team 59549 in 793", got)
	}

	// A player only on last season's roster no longer counts.
	if got, _ := store.TeamsForPlayer("7"); len(got) != 0 {
		t.Errorf("earlier-season memberships = %+v, want none", got)
	}
	if n, _ := store.Count(); n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
}

// A player listed on two teams' rosters keeps both memberships.
func TestLeagueStore_PlayerOnSeveralRosters(t *testing.T) {
	store := newTestLeagueStore(t)

	if err := store.SaveSeason(793, []db.LeagueMembership{{PlayerID: "75", TeamID: 60171}, {PlayerID: "75", TeamID: 60067}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.TeamsForPlayer("75")
	if err != nil || len(got) != 2 || got[0].TeamID != 60067 || got[1].TeamID != 60171 {
		t.Errorf("memberships = %+v, %v; want teams 60067 and 60171", got, err)
	}
}

// A run that misses a team's roster keeps that team's earlier rows; a refreshed roster replaces
// its earlier rows.
func TestLeagueStore_SaveSeasonKeepsUnrefreshedTeams(t *testing.T) {
	store := newTestLeagueStore(t)

	_ = store.SaveSeason(793, []db.LeagueMembership{{PlayerID: "1", TeamID: 10}, {PlayerID: "3", TeamID: 10}, {PlayerID: "2", TeamID: 20}})
	_ = store.SaveSeason(793, []db.LeagueMembership{{PlayerID: "1", TeamID: 10}})

	if got, _ := store.TeamsForPlayer("2"); len(got) != 1 || got[0].TeamID != 20 {
		t.Errorf("memberships = %+v, want team 20 kept", got)
	}
	if got, _ := store.TeamsForPlayer("3"); len(got) != 0 {
		t.Errorf("memberships = %+v, want player 3 dropped from refreshed team 10", got)
	}
}

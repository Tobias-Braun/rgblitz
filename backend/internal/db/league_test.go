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

	if got, err := store.TeamForPlayer("2329"); err != nil || got != nil {
		t.Fatalf("lookup on empty table = %+v, %v; want nil, nil", got, err)
	}

	if err := store.SaveSeason(543, []db.LeagueMembership{{PlayerID: "2329", TeamID: 39500}, {PlayerID: "7", TeamID: 39500}}); err != nil {
		t.Fatalf("save 2025/26: %v", err)
	}
	if err := store.SaveSeason(793, []db.LeagueMembership{{PlayerID: "2329", TeamID: 59549}}); err != nil {
		t.Fatalf("save 2026/27: %v", err)
	}

	got, err := store.TeamForPlayer("2329")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got == nil || got.TeamID != 59549 || got.TournamentID != 793 {
		t.Errorf("membership = %+v, want team 59549 in 793", got)
	}

	// A player only on last season's roster no longer counts.
	if got, _ := store.TeamForPlayer("7"); got != nil {
		t.Errorf("earlier-season membership = %+v, want nil", got)
	}
	if n, _ := store.Count(); n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
}

// A run that misses a team's roster keeps that team's earlier rows.
func TestLeagueStore_SaveSeasonKeepsUnrefreshedTeams(t *testing.T) {
	store := newTestLeagueStore(t)

	_ = store.SaveSeason(793, []db.LeagueMembership{{PlayerID: "1", TeamID: 10}, {PlayerID: "2", TeamID: 20}})
	_ = store.SaveSeason(793, []db.LeagueMembership{{PlayerID: "1", TeamID: 10}})

	if got, _ := store.TeamForPlayer("2"); got == nil || got.TeamID != 20 {
		t.Errorf("membership = %+v, want team 20 kept", got)
	}
}

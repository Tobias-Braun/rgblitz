package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

func TestPlayerStore_GetByNumericID(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "players.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	store := db.NewPlayerStore(conn)

	now := time.Now()
	for _, p := range []db.Player{
		{ID: "PZ2329", Name: "Jonas Bottler", ScrapedAt: now.Add(-time.Hour)},
		{ID: "JON2329", Name: "Jonas Bottler", OpenRGX: 1671, ScrapedAt: now},
		{ID: "PZ12329", Name: "Someone Else", ScrapedAt: now.Add(time.Hour)},
	} {
		if err := store.Upsert(p); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.GetByNumericID("2329")
	if err != nil || got == nil || got.ID != "JON2329" || got.OpenRGX != 1671 {
		t.Errorf("GetByNumericID(2329) = %+v, %v; want the newest row JON2329, not PZ12329", got, err)
	}
	if got, err := store.GetByNumericID("99"); got != nil || err != nil {
		t.Errorf("GetByNumericID(99) = %+v, %v; want none", got, err)
	}
}

// A database created before the index kept both ranking values gets the columns added.
func TestOpen_AddsRankingColumnsToOldPlayersTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE players (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, club_name TEXT NOT NULL DEFAULT '',
		avatar_url TEXT NOT NULL DEFAULT '', club_logo_url TEXT NOT NULL DEFAULT '',
		rgx INTEGER NOT NULL DEFAULT 0, games_played INTEGER NOT NULL DEFAULT 0,
		is_new BOOLEAN NOT NULL DEFAULT 0, name_collision BOOLEAN NOT NULL DEFAULT 0,
		scraped_at DATETIME NOT NULL);
		INSERT INTO players (id, name, rgx, scraped_at) VALUES ('PZ1', 'Lena', 1490, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("open old db: %v", err)
	}
	defer conn.Close()
	got, err := db.NewPlayerStore(conn).GetByID("PZ1")
	if err != nil || got == nil || got.RGX != 1490 || got.OpenRGX != 0 || got.WomenRGX != 0 {
		t.Errorf("migrated row = %+v, %v", got, err)
	}
}

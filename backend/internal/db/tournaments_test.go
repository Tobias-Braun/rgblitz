package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

func newTestTournamentStore(t *testing.T) *db.TournamentStore {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "tournaments.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return db.NewTournamentStore(conn)
}

func TestTournamentStore_GetMissing(t *testing.T) {
	store := newTestTournamentStore(t)

	got, err := store.Get(692)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Errorf("get on empty table = %+v, want nil", got)
	}
}

func TestTournamentStore_CreateThenGet(t *testing.T) {
	store := newTestTournamentStore(t)
	now := time.Now().Truncate(time.Second)

	want := db.Tournament{ID: 692, Name: "2. Leguan Cup", IsGRT: true, Date: "2026-07-18", ScrapedAt: now}
	if err := store.Create(want); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.Get(692)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("get after create = nil, want a row")
	}
	if got.ID != want.ID || got.Name != want.Name || got.IsGRT != want.IsGRT || got.Date != want.Date {
		t.Errorf("get = %+v, want %+v", *got, want)
	}
	if !got.ScrapedAt.Equal(want.ScrapedAt) {
		t.Errorf("ScrapedAt = %v, want %v", got.ScrapedAt, want.ScrapedAt)
	}
}

func TestTournamentStore_NonGRT(t *testing.T) {
	store := newTestTournamentStore(t)

	if err := store.Create(db.Tournament{ID: 672, Name: "ETS Championship Basel 2026", IsGRT: false, Date: "2026-08-08", ScrapedAt: time.Now()}); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.Get(672)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.IsGRT {
		t.Errorf("get = %+v, want IsGRT=false", got)
	}
}

// TestTournamentStore_CreateIsIdempotent proves a second Create for an already-seen
// tournament ID doesn't error or overwrite the first row — the ON CONFLICT DO NOTHING
// clause is what makes it safe for resolveTournamentGRT's check-then-insert to run
// concurrently across processes without a transaction.
func TestTournamentStore_CreateIsIdempotent(t *testing.T) {
	store := newTestTournamentStore(t)

	if err := store.Create(db.Tournament{ID: 692, Name: "2. Leguan Cup", IsGRT: true, Date: "2026-07-18", ScrapedAt: time.Now()}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := store.Create(db.Tournament{ID: 692, Name: "different name", IsGRT: false, Date: "2099-01-01", ScrapedAt: time.Now()}); err != nil {
		t.Fatalf("second create: %v", err)
	}

	got, err := store.Get(692)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "2. Leguan Cup" || !got.IsGRT {
		t.Errorf("get after duplicate create = %+v, want the first insert's values preserved", *got)
	}
}

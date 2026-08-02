package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

func TestPermissionStore_GetSet(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	store := db.NewPermissionStore(conn)

	if _, _, found, err := store.Get(); err != nil || found {
		t.Fatalf("Get() before any Set: found=%v err=%v, want found=false", found, err)
	}

	checkedAt := time.Now().Truncate(time.Second)
	if err := store.Set(false, checkedAt); err != nil {
		t.Fatalf("Set: %v", err)
	}
	allowed, got, found, err := store.Get()
	if err != nil || !found || allowed {
		t.Fatalf("Get() after Set(false): allowed=%v found=%v err=%v", allowed, found, err)
	}
	if !got.Equal(checkedAt) {
		t.Errorf("checkedAt = %v, want %v", got, checkedAt)
	}

	// Setting again must overwrite, not add a second row.
	if err := store.Set(true, time.Now()); err != nil {
		t.Fatalf("Set (overwrite): %v", err)
	}
	allowed, _, found, err = store.Get()
	if err != nil || !found || !allowed {
		t.Fatalf("Get() after Set(true): allowed=%v found=%v err=%v", allowed, found, err)
	}
}

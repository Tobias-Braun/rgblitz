package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

func newTestFeedbackStore(t *testing.T) *db.FeedbackStore {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "feedback.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return db.NewFeedbackStore(conn)
}

func TestFeedbackStore_Create(t *testing.T) {
	store := newTestFeedbackStore(t)
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

	fb, err := store.Create(5, "Super App!", now)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if fb.ID == 0 {
		t.Fatal("expected a non-zero ID")
	}
	if fb.Rating != 5 || fb.Text != "Super App!" || !fb.CreatedAt.Equal(now) {
		t.Fatalf("unexpected feedback: %+v", fb)
	}
}

func TestFeedbackStore_Create_EmptyText(t *testing.T) {
	store := newTestFeedbackStore(t)
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

	fb, err := store.Create(1, "", now)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if fb.Rating != 1 || fb.Text != "" {
		t.Fatalf("unexpected feedback: %+v", fb)
	}
}

func TestFeedbackStore_Create_DistinctIDs(t *testing.T) {
	store := newTestFeedbackStore(t)
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

	first, err := store.Create(3, "first", now)
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := store.Create(4, "second", now)
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("expected distinct IDs, got %d for both", first.ID)
	}
}

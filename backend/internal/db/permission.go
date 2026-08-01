package db

import (
	"database/sql"
	"fmt"
	"time"
)

// PermissionStore persists the outcome of the RESPECT_PERMISSION robots.txt
// check (see internal/scraper/robots.go), so it can be consulted by both the
// weekly scrape and live API requests without re-fetching robots.txt each time.
type PermissionStore struct {
	db *sql.DB
}

func NewPermissionStore(db *sql.DB) *PermissionStore {
	return &PermissionStore{db: db}
}

// Set stores the latest robots.txt permission check result.
func (s *PermissionStore) Set(allowed bool, checkedAt time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO scrape_permission (id, allowed, checked_at)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			allowed    = excluded.allowed,
			checked_at = excluded.checked_at
	`, allowed, checkedAt)
	return err
}

// Get returns the last stored permission check result. found is false if no
// check has ever been recorded.
func (s *PermissionStore) Get() (allowed bool, checkedAt time.Time, found bool, err error) {
	err = s.db.QueryRow(`SELECT allowed, checked_at FROM scrape_permission WHERE id = 1`).Scan(&allowed, &checkedAt)
	if err == sql.ErrNoRows {
		return false, time.Time{}, false, nil
	}
	if err != nil {
		return false, time.Time{}, false, fmt.Errorf("get scrape permission: %w", err)
	}
	return allowed, checkedAt, true, nil
}

// Permitted reports whether live playerzone requests may be made. When
// respect is false, requests are always permitted. When it's true, it defers
// to the last robots.txt check stored by the weekly scrape (see
// cron.Scheduler) — no check having ever run, or robots.txt disallowing
// RGBlitzBot, both fail closed (no request is made). Safe to call on a nil
// *PermissionStore (treated as "no check has ever run").
func (s *PermissionStore) Permitted(respect bool) bool {
	if !respect {
		return true
	}
	if s == nil {
		return false
	}
	allowed, _, found, err := s.Get()
	if err != nil || !found {
		return false
	}
	return allowed
}

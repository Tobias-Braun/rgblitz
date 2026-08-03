package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Tournament is a permanent classification record for one playerzone tournament,
// keyed by its numeric ID. Rows are never updated or purged — once a tournament has
// started, GRT status is assumed fixed for good (see internal/scraper.ScrapePlayerToday).
type Tournament struct {
	ID        int
	Name      string
	IsGRT     bool
	Date      string // YYYY-MM-DD, first day of the tournament
	ScrapedAt time.Time
}

type TournamentStore struct {
	db *sql.DB
}

func NewTournamentStore(db *sql.DB) *TournamentStore {
	return &TournamentStore{db: db}
}

// Get returns the tournament row for id, or (nil, nil) if it hasn't been scraped yet.
func (s *TournamentStore) Get(id int) (*Tournament, error) {
	var t Tournament
	err := s.db.QueryRow(`
		SELECT id, name, is_grt, date, scraped_at FROM tournaments WHERE id = ?
	`, id).Scan(&t.ID, &t.Name, &t.IsGRT, &t.Date, &t.ScrapedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tournament %d: %w", id, err)
	}
	return &t, nil
}

// Create records a newly-scraped tournament. ON CONFLICT DO NOTHING makes this safe to
// call even if another process (e.g. cmd/scrapecheck run against the same DB) inserted
// the same tournament between the caller's Get check and this call.
func (s *TournamentStore) Create(t Tournament) error {
	_, err := s.db.Exec(`
		INSERT INTO tournaments (id, name, is_grt, date, scraped_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, t.ID, t.Name, t.IsGRT, t.Date, t.ScrapedAt)
	if err != nil {
		return fmt.Errorf("create tournament %d: %w", t.ID, err)
	}
	return nil
}

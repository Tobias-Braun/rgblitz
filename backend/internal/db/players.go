package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Player struct {
	ID          string
	Name        string
	ClubName    string
	AvatarURL   string
	ClubLogoURL string
	// RGX is the value shown for the player: her Women RGX for a woman who has one, otherwise
	// the Open RGX. OpenRGX and WomenRGX are the two ranking values on their own (0 = none);
	// a WomenRGX marks the player as a woman. scraper.MergeWomenRGX sets all three.
	RGX           int
	OpenRGX       int
	WomenRGX      int
	GamesPlayed   int
	IsNew         bool
	NameCollision bool
	ScrapedAt     time.Time
}

type PlayerStore struct {
	db *sql.DB
}

func NewPlayerStore(db *sql.DB) *PlayerStore {
	return &PlayerStore{db: db}
}

const playerColumns = `id, name, club_name, avatar_url, club_logo_url, rgx, open_rgx, women_rgx, games_played, is_new, name_collision, scraped_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPlayer(row rowScanner) (Player, error) {
	var p Player
	err := row.Scan(&p.ID, &p.Name, &p.ClubName, &p.AvatarURL, &p.ClubLogoURL,
		&p.RGX, &p.OpenRGX, &p.WomenRGX, &p.GamesPlayed, &p.IsNew, &p.NameCollision, &p.ScrapedAt)
	return p, err
}

// getOne returns the single player a query selects, or nil if there is none.
func (s *PlayerStore) getOne(query string, args ...any) (*Player, error) {
	p, err := scanPlayer(s.db.QueryRow(`SELECT `+playerColumns+` FROM players `+query, args...))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *PlayerStore) getMany(query string, args ...any) ([]Player, error) {
	rows, err := s.db.Query(`SELECT `+playerColumns+` FROM players `+query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []Player
	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

// Count returns the total number of players currently indexed.
func (s *PlayerStore) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM players`).Scan(&n)
	return n, err
}

func (s *PlayerStore) Upsert(p Player) error {
	_, err := s.db.Exec(`
		INSERT INTO players (`+playerColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name          = excluded.name,
			club_name     = excluded.club_name,
			avatar_url    = excluded.avatar_url,
			club_logo_url = excluded.club_logo_url,
			rgx           = excluded.rgx,
			open_rgx      = excluded.open_rgx,
			women_rgx     = excluded.women_rgx,
			games_played  = excluded.games_played,
			is_new        = excluded.is_new,
			scraped_at    = excluded.scraped_at
	`,
		p.ID, p.Name, p.ClubName, p.AvatarURL, p.ClubLogoURL,
		p.RGX, p.OpenRGX, p.WomenRGX, p.GamesPlayed, p.IsNew, p.NameCollision, p.ScrapedAt,
	)
	return err
}

func (s *PlayerStore) GetByID(id string) (*Player, error) {
	p, err := s.getOne(`WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("get player: %w", err)
	}
	return p, nil
}

func (s *PlayerStore) GetByName(name string) (*Player, error) {
	p, err := s.getOne(`WHERE LOWER(name) = LOWER(?)`, name)
	if err != nil {
		return nil, fmt.Errorf("get player by name: %w", err)
	}
	return p, nil
}

// GetByNumericID finds a player by the bare playerzone ID that both ID shapes end in
// ("LUC2110", "PZ2110" — see scraper.NumericPlayerID). Index rows are never deleted, so a
// player whose ID shape changed has an old row too; the most recently scraped one wins.
func (s *PlayerStore) GetByNumericID(numericID string) (*Player, error) {
	p, err := s.getOne(`WHERE id GLOB '*[^0-9]' || ? ORDER BY scraped_at DESC LIMIT 1`, numericID)
	if err != nil {
		return nil, fmt.Errorf("get player by numeric ID: %w", err)
	}
	return p, nil
}

// Search returns up to 10 players whose name contains q (case-insensitive), ordered by descending RGX.
func (s *PlayerStore) Search(q string) ([]Player, error) {
	q = "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	players, err := s.getMany(`WHERE LOWER(name) LIKE ? ORDER BY rgx DESC LIMIT 10`, q)
	if err != nil {
		return nil, fmt.Errorf("search players: %w", err)
	}
	return players, nil
}

// RandomPlayers returns n random players excluding the given ID, used for dev-mode fake games.
func (s *PlayerStore) RandomPlayers(excludeID string, n int) ([]Player, error) {
	players, err := s.getMany(`WHERE id != ? AND rgx > 0 ORDER BY RANDOM() LIMIT ?`, excludeID, n)
	if err != nil {
		return nil, fmt.Errorf("random players: %w", err)
	}
	return players, nil
}

// NewestScrapedAt returns the most recent scraped_at in the index, or the zero
// time if the index is empty. Ordering by the column (instead of MAX()) keeps
// its DATETIME type, so the driver still decodes it as a time.Time.
func (s *PlayerStore) NewestScrapedAt() (time.Time, error) {
	var t time.Time
	err := s.db.QueryRow(`SELECT scraped_at FROM players ORDER BY scraped_at DESC LIMIT 1`).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	return t, err
}

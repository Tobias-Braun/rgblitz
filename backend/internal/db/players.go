package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Player struct {
	ID            string
	Name          string
	ClubName      string
	AvatarURL     string
	ClubLogoURL   string
	RGX           int
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

// Count returns the total number of players currently indexed.
func (s *PlayerStore) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM players`).Scan(&n)
	return n, err
}

func (s *PlayerStore) Upsert(p Player) error {
	_, err := s.db.Exec(`
		INSERT INTO players (id, name, club_name, avatar_url, club_logo_url, rgx, games_played, is_new, name_collision, scraped_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name          = excluded.name,
			club_name     = excluded.club_name,
			avatar_url    = excluded.avatar_url,
			club_logo_url = excluded.club_logo_url,
			rgx           = excluded.rgx,
			games_played  = excluded.games_played,
			is_new        = excluded.is_new,
			scraped_at    = excluded.scraped_at
	`,
		p.ID, p.Name, p.ClubName, p.AvatarURL, p.ClubLogoURL,
		p.RGX, p.GamesPlayed, p.IsNew, p.NameCollision, p.ScrapedAt,
	)
	return err
}

func (s *PlayerStore) GetByID(id string) (*Player, error) {
	p := &Player{}
	err := s.db.QueryRow(`
		SELECT id, name, club_name, avatar_url, club_logo_url, rgx, games_played, is_new, name_collision, scraped_at
		FROM players WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.ClubName, &p.AvatarURL, &p.ClubLogoURL,
		&p.RGX, &p.GamesPlayed, &p.IsNew, &p.NameCollision, &p.ScrapedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get player: %w", err)
	}
	return p, nil
}

func (s *PlayerStore) GetByName(name string) (*Player, error) {
	p := &Player{}
	err := s.db.QueryRow(`
		SELECT id, name, club_name, avatar_url, club_logo_url, rgx, games_played, is_new, name_collision, scraped_at
		FROM players WHERE LOWER(name) = LOWER(?)
	`, name).Scan(&p.ID, &p.Name, &p.ClubName, &p.AvatarURL, &p.ClubLogoURL,
		&p.RGX, &p.GamesPlayed, &p.IsNew, &p.NameCollision, &p.ScrapedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get player by name: %w", err)
	}
	return p, nil
}

// Search returns up to 10 players whose name contains q (case-insensitive), ordered by descending RGX.
func (s *PlayerStore) Search(q string) ([]Player, error) {
	q = "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	rows, err := s.db.Query(`
		SELECT id, name, club_name, avatar_url, club_logo_url, rgx, games_played, is_new, name_collision, scraped_at
		FROM players
		WHERE LOWER(name) LIKE ?
		ORDER BY rgx DESC
		LIMIT 10
	`, q)
	if err != nil {
		return nil, fmt.Errorf("search players: %w", err)
	}
	defer rows.Close()

	var players []Player
	for rows.Next() {
		var p Player
		if err := rows.Scan(&p.ID, &p.Name, &p.ClubName, &p.AvatarURL, &p.ClubLogoURL,
			&p.RGX, &p.GamesPlayed, &p.IsNew, &p.NameCollision, &p.ScrapedAt); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

// RandomPlayers returns n random players excluding the given ID, used for dev-mode fake games.
func (s *PlayerStore) RandomPlayers(excludeID string, n int) ([]Player, error) {
	rows, err := s.db.Query(`
		SELECT id, name, club_name, avatar_url, club_logo_url, rgx, games_played, is_new, name_collision, scraped_at
		FROM players
		WHERE id != ? AND rgx > 0
		ORDER BY RANDOM()
		LIMIT ?
	`, excludeID, n)
	if err != nil {
		return nil, fmt.Errorf("random players: %w", err)
	}
	defer rows.Close()

	var players []Player
	for rows.Next() {
		var p Player
		if err := rows.Scan(&p.ID, &p.Name, &p.ClubName, &p.AvatarURL, &p.ClubLogoURL,
			&p.RGX, &p.GamesPlayed, &p.IsNew, &p.NameCollision, &p.ScrapedAt); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

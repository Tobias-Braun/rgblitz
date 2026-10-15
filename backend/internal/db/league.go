package db

import (
	"database/sql"
	"errors"
	"fmt"
)

// LeagueMembership maps a player to the league team (tournament registration) they are on in
// the current Deutsche Roundnet Liga season. Player IDs are playerzone's bare numeric IDs, the
// only ID a team roster links.
type LeagueMembership struct {
	PlayerID     string
	TeamID       int
	TournamentID int
}

type LeagueStore struct {
	db *sql.DB
}

func NewLeagueStore(db *sql.DB) *LeagueStore {
	return &LeagueStore{db: db}
}

// SaveSeason records the rosters of a league season and drops every earlier season. Rows are
// upserted rather than replaced wholesale, so a team whose roster failed to load in this run
// keeps last run's players instead of silently losing league support for a week.
func (s *LeagueStore) SaveSeason(tournamentID int, members []LeagueMembership) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM league_players WHERE tournament_id != ?`, tournamentID); err != nil {
		return fmt.Errorf("drop earlier seasons: %w", err)
	}
	for _, m := range members {
		if _, err := tx.Exec(`
			INSERT INTO league_players (player_id, team_id, tournament_id) VALUES (?, ?, ?)
			ON CONFLICT(player_id) DO UPDATE SET team_id = excluded.team_id, tournament_id = excluded.tournament_id
		`, m.PlayerID, m.TeamID, tournamentID); err != nil {
			return fmt.Errorf("save league player %s: %w", m.PlayerID, err)
		}
	}
	return tx.Commit()
}

// TeamForPlayer returns the player's league membership, or (nil, nil) if they're on no roster.
func (s *LeagueStore) TeamForPlayer(playerID string) (*LeagueMembership, error) {
	m := LeagueMembership{PlayerID: playerID}
	err := s.db.QueryRow(`SELECT team_id, tournament_id FROM league_players WHERE player_id = ?`, playerID).
		Scan(&m.TeamID, &m.TournamentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get league team of player %s: %w", playerID, err)
	}
	return &m, nil
}

func (s *LeagueStore) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM league_players`).Scan(&n)
	return n, err
}

package db

import (
	"database/sql"
	"fmt"
)

// LeagueMembership puts a player on the roster of a league team (tournament registration) in
// the current Deutsche Roundnet Liga season. A player can be on several teams' rosters, of clubs
// other than their own too, so only the rosters tell which teams they may play for. Player IDs are playerzone's bare
// numeric IDs, the only ID a team roster links.
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

// SaveSeason records the rosters of a league season and drops every earlier season. Only the
// teams present in members are replaced, so a team whose roster failed to load in this run
// keeps last run's players instead of silently losing league support for a week.
func (s *LeagueStore) SaveSeason(tournamentID int, members []LeagueMembership) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM league_rosters WHERE tournament_id != ?`, tournamentID); err != nil {
		return fmt.Errorf("drop earlier seasons: %w", err)
	}
	cleared := map[int]bool{}
	for _, m := range members {
		if !cleared[m.TeamID] {
			cleared[m.TeamID] = true
			if _, err := tx.Exec(`DELETE FROM league_rosters WHERE team_id = ?`, m.TeamID); err != nil {
				return fmt.Errorf("clear roster of team %d: %w", m.TeamID, err)
			}
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO league_rosters (player_id, team_id, tournament_id) VALUES (?, ?, ?)`,
			m.PlayerID, m.TeamID, tournamentID); err != nil {
			return fmt.Errorf("save league player %s: %w", m.PlayerID, err)
		}
	}
	return tx.Commit()
}

// TeamsForPlayer returns every league roster the player is on, by team ID; none if they're on
// no roster.
func (s *LeagueStore) TeamsForPlayer(playerID string) ([]LeagueMembership, error) {
	rows, err := s.db.Query(`SELECT team_id, tournament_id FROM league_rosters WHERE player_id = ? ORDER BY team_id`, playerID)
	if err != nil {
		return nil, fmt.Errorf("get league teams of player %s: %w", playerID, err)
	}
	defer rows.Close()

	var teams []LeagueMembership
	for rows.Next() {
		m := LeagueMembership{PlayerID: playerID}
		if err := rows.Scan(&m.TeamID, &m.TournamentID); err != nil {
			return nil, fmt.Errorf("scan league team of player %s: %w", playerID, err)
		}
		teams = append(teams, m)
	}
	return teams, rows.Err()
}

func (s *LeagueStore) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM league_rosters`).Scan(&n)
	return n, err
}

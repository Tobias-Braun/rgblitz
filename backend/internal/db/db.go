package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return conn, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS players (
			id            TEXT PRIMARY KEY,
			name          TEXT NOT NULL,
			club_name     TEXT NOT NULL DEFAULT '',
			avatar_url    TEXT NOT NULL DEFAULT '',
			club_logo_url TEXT NOT NULL DEFAULT '',
			rgx           INTEGER NOT NULL DEFAULT 0,
			games_played  INTEGER NOT NULL DEFAULT 0,
			is_new        BOOLEAN NOT NULL DEFAULT 0,
			name_collision BOOLEAN NOT NULL DEFAULT 0,
			scraped_at    DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_players_name ON players(name);

		CREATE TABLE IF NOT EXISTS scrape_permission (
			id         INTEGER PRIMARY KEY CHECK (id = 1),
			allowed    BOOLEAN NOT NULL,
			checked_at DATETIME NOT NULL
		);

		CREATE TABLE IF NOT EXISTS rgx_jobs (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			player_id   TEXT NOT NULL,
			date        TEXT NOT NULL,
			status      TEXT NOT NULL,
			result_json TEXT,
			error       TEXT,
			queued_at   DATETIME NOT NULL,
			started_at  DATETIME,
			finished_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_rgx_jobs_player_date ON rgx_jobs(player_id, date);
		CREATE INDEX IF NOT EXISTS idx_rgx_jobs_status_queued ON rgx_jobs(status, queued_at);

		-- rgx_jobs_stats is a permanent usage log, deliberately separate from rgx_jobs
		-- (which is purged daily by cron — see internal/cron). One row per genuinely
		-- created job; cache hits that reuse an existing rgx_jobs row never insert here.
		-- job_id is not a foreign key on purpose: rgx_jobs rows disappear daily, these
		-- must not.
		CREATE TABLE IF NOT EXISTS rgx_jobs_stats (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id         INTEGER NOT NULL UNIQUE,
			player_id      TEXT NOT NULL,
			tournament_id  INTEGER,
			status         TEXT NOT NULL,
			created_at     DATETIME NOT NULL,
			finished_at    DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_rgx_jobs_stats_status ON rgx_jobs_stats(status);

		-- tournaments is a permanent classification cache, never purged: once a
		-- tournament has started, its GRT status is assumed fixed forever, so it only
		-- ever needs to be scraped once (see internal/scraper.ScrapePlayerToday).
		CREATE TABLE IF NOT EXISTS tournaments (
			id         INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			is_grt     BOOLEAN NOT NULL,
			date       TEXT NOT NULL,
			scraped_at DATETIME NOT NULL
		);

		-- league_rosters is the roster index of the current Deutsche Roundnet Liga season,
		-- rebuilt weekly by cron (see internal/scraper.ScrapeLeagueRosters). A player can be on
		-- several teams' rosters, so it holds one row per (player, team). It replaces
		-- league_players, which kept one team per player; the server rebuilds the index at
		-- startup while it's empty.
		DROP TABLE IF EXISTS league_players;
		CREATE TABLE IF NOT EXISTS league_rosters (
			player_id     TEXT NOT NULL,
			team_id       INTEGER NOT NULL,
			tournament_id INTEGER NOT NULL,
			PRIMARY KEY (player_id, team_id)
		);

		CREATE TABLE IF NOT EXISTS feedback (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			rating     INTEGER NOT NULL,
			text       TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL
		);
	`)
	return err
}

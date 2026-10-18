package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

type leagueCapture struct {
	CapturedAt string                `json:"captured_at"`
	Date       string                `json:"date"`
	Weekday    string                `json:"weekday"`
	TeamID     int                   `json:"team_id"`
	SeasonID   int                   `json:"season_id,omitempty"`
	FatalError string                `json:"fatal_error,omitempty"`
	Players    []leaguePlayerCapture `json:"players,omitempty"`
	Fetched    []fetchRecord         `json:"fetched"`
	Parsed     leagueParsed          `json:"parsed"`
}

func (c leagueCapture) failedPlayers() int {
	n := 0
	for _, p := range c.Players {
		if p.Error != "" && p.NumericID != "" {
			n++
		}
	}
	return n
}

// leaguePlayerCapture is one roster player's pipeline outcome. RosterRGX is the RGX the schedule
// page shows: the pre-matchday baseline while a matchday runs, the recalculated value after it.
type leaguePlayerCapture struct {
	Name      string               `json:"name"`
	NumericID string               `json:"numeric_id,omitempty"`
	RosterRGX int                  `json:"roster_rgx"`
	Handled   bool                 `json:"handled"`
	Error     string               `json:"error,omitempty"`
	Result    *scraper.TodayResult `json:"result,omitempty"`
}

// perPlayerDelta is the player's own RGX change: each Partie moves both players of a pair by
// half the pair's delta.
func (p leaguePlayerCapture) perPlayerDelta() float64 {
	if p.Result == nil {
		return 0
	}
	sum := 0.0
	for _, g := range p.Result.Games {
		sum += float64(g.RGXDeltaPerPlayer)
	}
	return sum
}

// leagueParsed holds the intermediates re-derived from the saved pages, which also verifies
// that the snapshot replays to what the live run saw.
type leagueParsed struct {
	Rosters        map[int]scraper.TeamScheduleResult `json:"rosters,omitempty"`
	Fixtures       []scraper.LeagueFixture            `json:"fixtures,omitempty"`
	MatchdayGroups map[int]scraper.MatchdayGroup      `json:"matchday_groups,omitempty"`
	Partien        map[int][]scraper.Partie           `json:"partien,omitempty"`
	Errors         map[string]string                  `json:"errors,omitempty"`
}

var (
	reMatchdayGroupParam = regexp.MustCompile(`[?&]md_group=(\d+)`)
	reSubgamesPath       = regexp.MustCompile(`^/ajax/get-subgames/(\d+)$`)
)

// reparseLeague runs each league parser over the pages the capture fetched, keyed by the path
// they were fetched from. A page that fails to parse is recorded, not fatal.
func reparseLeague(dir string, records []fetchRecord, ourTeamID int) leagueParsed {
	p := leagueParsed{
		Rosters:        map[int]scraper.TeamScheduleResult{},
		MatchdayGroups: map[int]scraper.MatchdayGroup{},
		Partien:        map[int][]scraper.Partie{},
		Errors:         map[string]string{},
	}
	for _, rec := range records {
		if rec.Error != "" || rec.File == "" || rec.Reused {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, rec.File))
		if err == nil {
			err = p.add(rec.Path, raw, ourTeamID)
		}
		if err != nil {
			p.Errors[rec.File] = err.Error()
		}
	}
	if len(p.Errors) == 0 {
		p.Errors = nil
	}
	return p
}

// add parses one captured page into the intermediates its path says it holds. Pages of any
// other kind are ignored.
func (p *leagueParsed) add(path string, raw []byte, ourTeamID int) error {
	if teamID, ok := pathID(reTeamSchedulePath, path); ok {
		return p.addSchedule(teamID, raw, teamID == ourTeamID)
	}
	if groupID, ok := pathID(reMatchdayGroupParam, path); ok {
		group, err := scraper.ParseMatchdayGroup(bytes.NewReader(raw), groupID)
		if err == nil {
			p.MatchdayGroups[groupID] = group
		}
		return err
	}
	if gameID, ok := pathID(reSubgamesPath, path); ok {
		partien, err := scraper.ParseSubgames(bytes.NewReader(raw))
		if err == nil {
			p.Partien[gameID] = partien
		}
		return err
	}
	return nil
}

// addSchedule records a team's roster, and for the captured team itself its Begegnungen.
func (p *leagueParsed) addSchedule(teamID int, raw []byte, ours bool) error {
	roster, err := scraper.ParseTeamSchedule(bytes.NewReader(raw), teamID)
	if err != nil {
		return err
	}
	p.Rosters[teamID] = roster
	if !ours {
		return nil
	}
	if p.Fixtures, err = scraper.ParseLeagueFixtures(bytes.NewReader(raw), teamID); err != nil {
		return fmt.Errorf("fixtures: %w", err)
	}
	return nil
}

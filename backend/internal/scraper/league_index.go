package scraper

import (
	"bytes"
	"fmt"
	"io"
	"log"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

// ScrapeLeagueRosters builds the roster index of the current Deutsche Roundnet Liga season:
// upcoming list → season → its divisions → every team's schedule page, whose player cards link
// the roster's profiles. Roughly one request per team (~130 per season).
//
// Reports tournamentID 0 and no error when no season is listed (off-season). A team whose
// schedule page fails is logged and skipped, so one broken page doesn't cost every other team
// its league support.
func ScrapeLeagueRosters(fetcher Fetcher) (int, []db.LeagueMembership, error) {
	season, ok, err := fetchActiveLeagueSeason(fetcher)
	if err != nil || !ok {
		return 0, nil, err
	}

	teamIDs, err := fetchLeagueTeamIDs(fetcher, season.ID)
	if err != nil {
		return 0, nil, err
	}

	var members []db.LeagueMembership
	failed := 0
	for _, teamID := range teamIDs {
		roster, err := fetchLeagueRoster(fetcher, teamID)
		if err != nil {
			log.Printf("league index: team %d: %v", teamID, err)
			failed++
			continue
		}
		for _, p := range roster.Players {
			if p.NumericID != "" {
				members = append(members, db.LeagueMembership{PlayerID: p.NumericID, TeamID: teamID, TournamentID: season.ID})
			}
		}
	}
	if failed == len(teamIDs) {
		return 0, nil, fmt.Errorf("all %d league team pages failed", failed)
	}
	return season.ID, members, nil
}

func fetchActiveLeagueSeason(fetcher Fetcher) (TournamentListEntry, bool, error) {
	body, err := fetcher.Fetch(TournamentsUpcomingPath)
	if err != nil {
		return TournamentListEntry{}, false, fmt.Errorf("fetch upcoming tournaments: %w", err)
	}
	defer body.Close()

	entries, err := ParseTournamentList(body)
	if err != nil {
		return TournamentListEntry{}, false, err
	}
	season, ok := ActiveLeagueSeason(entries)
	return season, ok, nil
}

// fetchLeagueTeamIDs lists the registration IDs of every team in every division of the season.
// The season's own page links only its first division, but each division's team list links all
// of them, so the division list is read off the first team list.
func fetchLeagueTeamIDs(fetcher Fetcher, tournamentID int) ([]int, error) {
	detail, err := fetchPage(fetcher, fmt.Sprintf("/tournaments/%d", tournamentID))
	if err != nil {
		return nil, fmt.Errorf("fetch league season %d: %w", tournamentID, err)
	}
	first, err := ParseDivisionLinks(bytes.NewReader(detail))
	if err != nil {
		return nil, fmt.Errorf("league season %d: %w", tournamentID, err)
	}

	firstTeams, err := fetchPage(fetcher, first[0].TeamsPath)
	if err != nil {
		return nil, fmt.Errorf("fetch league division %s: %w", first[0].ID, err)
	}
	divisions, err := ParseDivisionLinks(bytes.NewReader(firstTeams))
	if err != nil {
		divisions = first
	}

	var teamIDs []int
	seen := map[int]bool{}
	for _, d := range divisions {
		page := firstTeams
		if d.TeamsPath != first[0].TeamsPath {
			if page, err = fetchPage(fetcher, d.TeamsPath); err != nil {
				log.Printf("league index: division %s: %v", d.Name, err)
				continue
			}
		}
		teams, err := ParseDivisionTeams(bytes.NewReader(page))
		if err != nil {
			log.Printf("league index: division %s: %v", d.Name, err)
			continue
		}
		for _, t := range teams {
			if !seen[t.TournamentTeamID] {
				seen[t.TournamentTeamID] = true
				teamIDs = append(teamIDs, t.TournamentTeamID)
			}
		}
	}
	if len(teamIDs) == 0 {
		return nil, fmt.Errorf("league season %d listed no teams", tournamentID)
	}
	return teamIDs, nil
}

func fetchLeagueRoster(fetcher Fetcher, teamID int) (TeamScheduleResult, error) {
	page, err := fetchPage(fetcher, teamSchedulePath(teamID))
	if err != nil {
		return TeamScheduleResult{}, err
	}
	return ParseTeamSchedule(bytes.NewReader(page), teamID)
}

func teamSchedulePath(teamID int) string {
	return fmt.Sprintf("/tournaments/schedule/team/%d", teamID)
}

func fetchPage(fetcher Fetcher, path string) ([]byte, error) {
	body, err := fetcher.Fetch(path)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}

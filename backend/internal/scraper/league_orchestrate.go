package scraper

import (
	"bytes"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

// CompetitionLeague marks a TodayResult computed from a league matchday rather than a tournament.
const CompetitionLeague = "league"

// leagueSchedule is everything a league team's schedule page yields: its roster with each
// player's pre-matchday RGX, and its Begegnungen.
type leagueSchedule struct {
	TeamScheduleResult
	Fixtures []LeagueFixture
}

// ScrapePlayerLeagueToday computes a player's RGX deltas for today's league matchday: every team
// whose roster the player is on (from the roster index) → that team's Begegnungen today (by
// matchday group date) → the Partien the player played in each → one RGX game per Partie,
// priced by the two pairs' RGX sums. A player can be on several rosters, so each one is checked:
// the Partien alone tell which team they played for.
//
// handled is false when the player isn't on a league roster or none of their teams plays today;
// the caller then runs the tournament pipeline (ScrapePlayerToday) instead.
func ScrapePlayerLeagueToday(deps ScrapeDeps, player *db.Player, now time.Time) (result TodayResult, tournamentID int, handled bool, err error) {
	if deps.League == nil {
		return TodayResult{}, 0, false, nil
	}
	numericID, err := NumericPlayerID(player.ID)
	if err != nil {
		return TodayResult{}, 0, false, nil
	}
	teams, err := deps.League.TeamsForPlayer(numericID)
	if err != nil || len(teams) == 0 {
		return TodayResult{}, 0, false, err
	}

	pInfo := &PlayerInfo{
		ID:          player.ID,
		Name:        player.Name,
		ClubName:    player.ClubName,
		AvatarURL:   deps.ProxyURL(player.AvatarURL),
		ClubLogoURL: deps.ProxyURL(player.ClubLogoURL),
		RGX:         player.RGX,
	}
	date := CompetitionDay(now).Format("2006-01-02")
	day := TodayResult{Status: "ok", Competition: CompetitionLeague, Player: pInfo, Date: date}

	for _, team := range teams {
		ours, err := fetchLeagueSchedule(deps, team.TeamID)
		if err != nil {
			return TodayResult{}, 0, false, fmt.Errorf("fetch league team schedule: %w", err)
		}
		if len(ours.Fixtures) == 0 {
			return TodayResult{}, 0, false, fmt.Errorf("league team %d lists no matchday fixtures — possible DOM change", team.TeamID)
		}
		todays, err := fixturesOn(deps, ours.Fixtures, date)
		if err != nil {
			return TodayResult{}, 0, false, err
		}
		if len(todays) == 0 {
			continue
		}
		handled, tournamentID = true, team.TournamentID

		if self, ok := rosterPlayerByID(ours.Players, numericID); ok && self.RGX > 0 {
			pInfo.RGX = self.RGX
		}
		for _, f := range todays {
			games, err := playerPartien(deps, ours, f, numericID)
			if err != nil {
				return TodayResult{}, 0, true, err
			}
			for _, g := range games {
				day.TotalDelta += g.RGXDelta
				day.TotalPerformanceDelta += g.PerformanceDelta
				day.HasProxy = day.HasProxy || g.HasProxy
			}
			day.Games = append(day.Games, games...)
		}
	}
	if !handled {
		return TodayResult{}, 0, false, nil
	}
	return day, tournamentID, true, nil
}

// fixturesOn keeps the Begegnungen whose matchday group plays on date (YYYY-MM-DD), in playing
// order — the schedule page lists them newest first.
func fixturesOn(deps ScrapeDeps, fixtures []LeagueFixture, date string) ([]LeagueFixture, error) {
	var todays []LeagueFixture
	for _, f := range fixtures {
		group, err := fetchMatchdayGroup(deps, f)
		if err != nil {
			return nil, fmt.Errorf("resolve matchday of Begegnung %d: %w", f.GameID, err)
		}
		if group.Date == date {
			todays = append(todays, f)
		}
	}
	slices.SortFunc(todays, func(a, b LeagueFixture) int { return a.GameID - b.GameID })
	return todays, nil
}

// playerPartien turns the played Partien of one Begegnung that the player took part in into
// game results. Each Partie is a pair game: r1 and r2 are the two pairs' RGX sums. Women and
// Mixed Partien count the roster badge RGX; Open Partien count each player's Open RGX (see
// openRGX).
func playerPartien(deps ScrapeDeps, ours leagueSchedule, f LeagueFixture, playerID string) ([]GameResult, error) {
	parties, err := fetchSubgames(deps, f.GameID)
	if err != nil {
		return nil, fmt.Errorf("fetch Partien of Begegnung %d: %w", f.GameID, err)
	}
	opponent, err := fetchLeagueSchedule(deps, f.OpponentTeamID)
	if err != nil {
		return nil, fmt.Errorf("fetch roster of %s: %w", f.OpponentName, err)
	}

	ourSide, theirSide := 0, 1
	if !f.WeAreTeamOne {
		ourSide, theirSide = 1, 0
	}
	proxy := DivisionStartingRGX(ours.DivisionName)

	var games []GameResult
	for _, p := range parties {
		if len(p.SetScores) == 0 || !pairIncludes(p.Pairs[ourSide], ours.Players, playerID) {
			continue
		}
		rgxOf := func(tp TeamPlayer) int { return tp.RGX }
		if isOpenPartie(p.Label) {
			rgxOf = func(tp TeamPlayer) int { return openRGX(deps.Players, tp) }
		}
		r1, ourProxy := pairRGX(p.Pairs[ourSide], ours.Players, rgxOf, proxy)
		r2, theirProxy := pairRGX(p.Pairs[theirSide], opponent.Players, rgxOf, proxy)

		card := GameCard{Round: p.Label + " · " + f.Matchday, WeAreTeamOne: f.WeAreTeamOne, SetScores: p.SetScores}
		g := GameResultFromCard(card, r1, r2, ourProxy || theirProxy)
		g.OurTeamName = strings.Join(p.Pairs[ourSide], "/")
		g.OpponentName = fmt.Sprintf("%s (%s)", strings.Join(p.Pairs[theirSide], "/"), f.OpponentName)
		games = append(games, g)
	}
	return games, nil
}

// resolvePairName finds the roster player a Partie name ("Bottler", "F. Stadler") stands for.
// Only a unique match counts, so two players sharing a surname are never confused.
func resolvePairName(name string, roster []TeamPlayer) (TeamPlayer, bool) {
	var found TeamPlayer
	matches := 0
	for _, p := range roster {
		if PairNameMatches(name, p.Name) {
			found = p
			matches++
		}
	}
	return found, matches == 1
}

func pairIncludes(pair []string, roster []TeamPlayer, playerID string) bool {
	for _, name := range pair {
		if p, ok := resolvePairName(name, roster); ok && p.NumericID == playerID {
			return true
		}
	}
	return false
}

// isOpenPartie reports whether a Partie ("Open 1", "Damen", "Mixed 2", …) is rated with the
// Open RGX. Women and Mixed Partien are rated with the Women RGX.
func isOpenPartie(label string) bool {
	return strings.HasPrefix(strings.ToLower(label), "open")
}

// openRGX is a roster player's Open RGX. The roster badge shows a woman's Women RGX, so for a
// woman (the index holds a Women RGX for her) it comes from the index instead: 0 when she has
// no Open RGX, which the caller proxies like any missing value.
func openRGX(players *db.PlayerStore, p TeamPlayer) int {
	if players == nil || p.NumericID == "" {
		return p.RGX
	}
	indexed, err := players.GetByNumericID(p.NumericID)
	if err != nil {
		slog.Warn("league: player index lookup failed, using roster RGX", "player_id", p.NumericID, "error", err)
		return p.RGX
	}
	if indexed == nil || indexed.WomenRGX == 0 {
		return p.RGX
	}
	return indexed.OpenRGX
}

// pairRGX sums a pair's RGX, as rgxOf reads it from each roster player. A player who can't be
// resolved from the roster, or has no RGX, counts with the proxy value.
func pairRGX(pair []string, roster []TeamPlayer, rgxOf func(TeamPlayer) int, proxy int) (sum int, hasProxy bool) {
	for i := range 2 {
		rgx := 0
		if i < len(pair) {
			if p, ok := resolvePairName(pair[i], roster); ok {
				rgx = rgxOf(p)
			}
		}
		if rgx <= 0 {
			rgx, hasProxy = proxy, true
		}
		sum += rgx
	}
	return sum, hasProxy
}

func rosterPlayerByID(roster []TeamPlayer, playerID string) (TeamPlayer, bool) {
	for _, p := range roster {
		if p.NumericID == playerID {
			return p, true
		}
	}
	return TeamPlayer{}, false
}

// --- cached league fetchers ---
//
// Rosters, RGX and matchday dates are fixed for the day (RGX is recalculated after a matchday),
// so schedule pages and matchday groups are cached for hours and shared by every player of a
// team and its opponents. Only the Partien change while a matchday is played.

func fetchLeagueSchedule(deps ScrapeDeps, teamID int) (leagueSchedule, error) {
	return cached(deps, fmt.Sprintf("league_schedule:%d", teamID), cache.TTLLeagueMatchday, func() (leagueSchedule, error) {
		page, err := fetchPage(deps.Fetcher, teamSchedulePath(teamID))
		if err != nil {
			return leagueSchedule{}, err
		}
		schedule, err := ParseTeamSchedule(bytes.NewReader(page), teamID)
		if err != nil {
			return leagueSchedule{}, err
		}
		// Opponents' pages are fetched for their roster alone, so missing fixtures are only an
		// error for the player's own team (see ScrapePlayerLeagueToday).
		fixtures, _ := ParseLeagueFixtures(bytes.NewReader(page), teamID)
		return leagueSchedule{TeamScheduleResult: schedule, Fixtures: fixtures}, nil
	})
}

func fetchMatchdayGroup(deps ScrapeDeps, f LeagueFixture) (MatchdayGroup, error) {
	return cached(deps, fmt.Sprintf("league_md_group:%d", f.MatchdayGroupID), cache.TTLLeagueMatchday, func() (MatchdayGroup, error) {
		page, err := fetchPage(deps.Fetcher, f.MatchdayPath)
		if err != nil {
			return MatchdayGroup{}, err
		}
		return ParseMatchdayGroup(bytes.NewReader(page), f.MatchdayGroupID)
	})
}

func fetchSubgames(deps ScrapeDeps, gameID int) ([]Partie, error) {
	return cached(deps, fmt.Sprintf("league_subgames:%d", gameID), cache.TTLLeagueLive, func() ([]Partie, error) {
		page, err := fetchPage(deps.Fetcher, SubgamesPath(gameID))
		if err != nil {
			return nil, err
		}
		return ParseSubgames(bytes.NewReader(page))
	})
}

// cached returns the value under key, loading and storing it on a miss. A value of the wrong
// type under the key is treated as a miss rather than allowed to panic.
func cached[T any](deps ScrapeDeps, key string, ttl time.Duration, load func() (T, error)) (T, error) {
	if v, ok := deps.Cache.Get(key); ok {
		if t, ok := v.(T); ok {
			return t, nil
		}
	}
	t, err := load()
	if err != nil {
		return t, err
	}
	deps.Cache.Set(key, t, ttl)
	return t, nil
}

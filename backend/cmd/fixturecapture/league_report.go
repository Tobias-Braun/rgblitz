package main

import (
	"fmt"
	"sort"
	"strings"
)

func leagueReport(c leagueCapture) string {
	var b strings.Builder
	fmt.Fprintf(&b, "captured %s (%s)\nleague team %d, season %d\n", c.CapturedAt, c.Weekday, c.TeamID, c.SeasonID)
	if c.FatalError != "" {
		fmt.Fprintf(&b, "FATAL: %s\n", c.FatalError)
	}
	writeFetchedAndParseErrors(&b, c.Fetched, c.Parsed.Errors)
	writeLeagueFixtures(&b, c.Parsed, c.Date)
	writeLeaguePartien(&b, c.Parsed)
	writeLeagueRosters(&b, c.Parsed)

	b.WriteString("\n--- per player (roster RGX is the pre-matchday baseline only while the matchday runs) ---\n")
	for _, pl := range c.Players {
		writeLeaguePlayer(&b, pl)
	}
	return b.String()
}

// writeLeagueFixtures lists the team's Begegnungen with their matchday date, marking today's.
func writeLeagueFixtures(b *strings.Builder, p leagueParsed, today string) {
	if len(p.Fixtures) == 0 {
		return
	}
	b.WriteString("\n--- Begegnungen ---\n")
	for _, f := range p.Fixtures {
		group := p.MatchdayGroups[f.MatchdayGroupID]
		marker := " "
		if group.Date == today {
			marker = "*"
		}
		fmt.Fprintf(b, " %s game %-7d %-22s %-10s %-14s vs %-28s team_id=%-6d we_are_team_one=%t\n",
			marker, f.GameID, f.Matchday, group.Date, group.City, f.OpponentName, f.OpponentTeamID, f.WeAreTeamOne)
	}
}

func writeLeaguePartien(b *strings.Builder, p leagueParsed) {
	if len(p.Partien) == 0 {
		return
	}
	b.WriteString("\n--- Partien (Begegnungen today) ---\n")
	for _, gameID := range sortedKeys(p.Partien) {
		fmt.Fprintf(b, "  game %d\n", gameID)
		for _, partie := range p.Partien[gameID] {
			fmt.Fprintf(b, "    %-8s %-32s vs %-32s sets=%s\n", partie.Label,
				strings.Join(partie.Pairs[0], "/"), strings.Join(partie.Pairs[1], "/"), formatSetScores(partie.SetScores))
		}
	}
}

func writeLeagueRosters(b *strings.Builder, p leagueParsed) {
	for _, teamID := range sortedKeys(p.Rosters) {
		roster := p.Rosters[teamID]
		fmt.Fprintf(b, "\n--- roster %d %q (division %q) ---\n", teamID, roster.TeamName, roster.DivisionName)
		for _, pl := range roster.Players {
			fmt.Fprintf(b, "  %-28s id=%-6s rgx=%d\n", pl.Name, pl.NumericID, pl.RGX)
		}
	}
}

func writeLeaguePlayer(b *strings.Builder, pl leaguePlayerCapture) {
	switch {
	case pl.Error != "":
		fmt.Fprintf(b, "  %-28s rgx=%-5d ERROR: %s\n", pl.Name, pl.RosterRGX, pl.Error)
	case !pl.Handled:
		fmt.Fprintf(b, "  %-28s rgx=%-5d not handled (team doesn't play today)\n", pl.Name, pl.RosterRGX)
	default:
		delta := pl.perPlayerDelta()
		fmt.Fprintf(b, "  %-28s rgx=%-5d partien=%d pair_delta_sum=%+d player_delta=%+.1f → %.1f\n",
			pl.Name, pl.RosterRGX, len(pl.Result.Games), pl.Result.TotalDelta, delta, float64(pl.RosterRGX)+delta)
		writeGameLines(b, pl.Result.Games)
	}
}

func summarizeLeague(c leagueCapture) string {
	if c.FatalError != "" {
		return "error: " + c.FatalError
	}
	played := 0
	for _, p := range c.Players {
		if p.Result != nil && len(p.Result.Games) > 0 {
			played++
		}
	}
	return fmt.Sprintf("%d roster players, %d played today, %d pipeline errors, %d parse errors",
		len(c.Players), played, c.failedPlayers(), len(c.Parsed.Errors))
}

func sortedKeys[V any](m map[int]V) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

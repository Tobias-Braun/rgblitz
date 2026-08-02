package scraper

import (
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Division is a competition category within a tournament.
type Division struct {
	ID        string // numeric string from URL, e.g. "2265"
	Name      string
	TeamsPath string // full path to this division's teams page, e.g. "/tournaments/692-.../teams/2265"
}

// TeamEntry is a team found on a division's teams page.
type TeamEntry struct {
	TournamentTeamID int // used to fetch schedule via /tournaments/schedule/team/{id}
	Name             string
	PlayerNames      []string
	CombinedRGX      int
}

// SetScore is the score pair for one set.
type SetScore struct {
	TeamOneScore int
	TeamTwoScore int
}

// GameCard is a parsed game from the team schedule page.
type GameCard struct {
	GameID       int
	Round        string
	OpponentName string
	// True if our team appears first in the HTML (position 1), false if second.
	WeAreTeamOne bool
	SetScores    []SetScore
	// "win", "loss", or "draw" as reported by playerzone.
	Outcome string
	// True if playerzone marked the game as cancelled (e.g. forfeit/injury).
	// Cancelled games render with outcome="" just like not-yet-played games,
	// so they're distinguished by the "cancelled" class on rg-game-card.
	WasCancelled bool
}

// TeamScheduleResult holds the data from /tournaments/schedule/team/{id}.
type TeamScheduleResult struct {
	DivisionName string
	TeamName     string
	Players      []TeamPlayer
	Games        []GameCard
}

// TeamPlayer is a player card shown on the team schedule page.
type TeamPlayer struct {
	Name string
	RGX  int
}

// SetResult is one set's score, oriented so OurScore/OpponentScore are always
// from our team's perspective regardless of which side playerzone listed first.
type SetResult struct {
	OurScore      int `json:"our_score"`
	OpponentScore int `json:"opponent_score"`
}

// GameResult is the computed result for one game.
type GameResult struct {
	Round             string      `json:"round"`
	OpponentName      string      `json:"opponent_name"`
	Result            string      `json:"result"` // "W", "L", "D", or "C" for a cancelled game
	RGXDelta          int         `json:"rgx_delta"`
	RGXDeltaPerPlayer float32     `json:"rgx_delta_per_player"`
	HasProxy          bool        `json:"has_proxy"`
	WasCancelled      bool        `json:"was_cancelled"`
	Sets              []SetResult `json:"sets"`
}

// reTournamentTeamID matches onclick="load_team_info(\d+, 60615)".
var reTournamentTeamIDOnclick = regexp.MustCompile(`load_team_info\(\d+,\s*(\d+)\)`)

// reDivisionTeamsHref matches /tournaments/{id}-{slug}/teams/{div_id}.
var reDivisionTeamsHref = regexp.MustCompile(`/tournaments/[^/]+/teams/(\d+)$`)

// ParseDivisionSidebar extracts every division of the tournament from the sidebar
// rendered on a team schedule page (/tournaments/schedule/team/{id}), keyed by
// the division's display name so callers can match it against
// TeamScheduleResult.DivisionName to find the division our own team belongs to.
func ParseDivisionSidebar(r io.Reader) ([]Division, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse division sidebar html: %w", err)
	}

	var divs []Division

	doc.Find("div.sub-section").Each(func(_ int, sec *goquery.Selection) {
		name := strings.TrimSpace(sec.Find("div.sub-title").First().Text())
		if name == "" {
			return
		}

		var teamsPath string
		var divID string
		sec.Find("div.big-title a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			href, _ := a.Attr("href")
			m := reDivisionTeamsHref.FindStringSubmatch(href)
			if len(m) < 2 {
				return true // keep looking; this a wasn't the teams link
			}
			teamsPath = href
			divID = m[1]
			return false
		})
		if teamsPath == "" {
			return
		}

		divs = append(divs, Division{ID: divID, Name: name, TeamsPath: teamsPath})
	})

	if len(divs) == 0 {
		return nil, fmt.Errorf("division sidebar parsed 0 divisions — possible DOM change")
	}
	return divs, nil
}

// ParseDivisionTeams extracts team cards from a division teams page.
func ParseDivisionTeams(r io.Reader) ([]TeamEntry, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse division teams html: %w", err)
	}

	var teams []TeamEntry

	doc.Find("div.tournament-team-card[data-rgx][onclick]").Each(func(_ int, card *goquery.Selection) {
		onclick, _ := card.Attr("onclick")
		m := reTournamentTeamIDOnclick.FindStringSubmatch(onclick)
		if len(m) < 2 {
			return
		}
		teamID, _ := strconv.Atoi(m[1])

		combinedRGXStr, _ := card.Attr("data-rgx")
		combinedRGX, _ := strconv.Atoi(strings.TrimSpace(combinedRGXStr))

		name := strings.TrimSpace(card.Find("div.team-card-name").Text())

		var playerNames []string
		card.Find("div.team-card-players span").Each(func(_ int, s *goquery.Selection) {
			if n := strings.TrimSpace(s.Text()); n != "" {
				playerNames = append(playerNames, n)
			}
		})

		teams = append(teams, TeamEntry{
			TournamentTeamID: teamID,
			Name:             name,
			PlayerNames:      playerNames,
			CombinedRGX:      combinedRGX,
		})
	})

	if len(teams) == 0 {
		return nil, fmt.Errorf("division teams parsed 0 teams — possible DOM change")
	}
	return teams, nil
}

// ParseTeamSchedule parses the game schedule for one team.
// ourTeamID is the tournament_team_id (numeric) from the page URL — used to identify
// which team in each game card is "us" so we can orient set scores correctly.
func ParseTeamSchedule(r io.Reader, ourTeamID int) (TeamScheduleResult, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return TeamScheduleResult{}, fmt.Errorf("parse team schedule html: %w", err)
	}

	result := TeamScheduleResult{}

	result.DivisionName = strings.TrimSpace(doc.Find("div.row.section-title h3.event-title").First().Text())
	result.TeamName = strings.TrimSpace(doc.Find("div.second-title span a").First().Text())

	// Parse each player card on the page — these are our team's players with individual RGX.
	doc.Find("div.player-card").Each(func(_ int, card *goquery.Selection) {
		name := strings.TrimSpace(card.Find("div.info-player-name a").First().Text())
		// HTML-encoded quotes in names (like Joceline &quot;Jocie&quot;) come through decoded by goquery.
		name = cleanPlayerName(name)
		if name == "" {
			return
		}
		rgxText := strings.TrimSpace(card.Find("span.rgx-value").First().Text())
		rgxText = strings.Fields(rgxText)[0]
		rgx, _ := strconv.Atoi(strings.ReplaceAll(rgxText, ".", ""))
		result.Players = append(result.Players, TeamPlayer{Name: name, RGX: rgx})
	})

	if len(result.Players) == 0 {
		return TeamScheduleResult{}, fmt.Errorf("team schedule parsed 0 players — possible DOM change")
	}

	// Parse each game card in document order (chronological on the page).
	doc.Find("div[id^='game-']").Each(func(_ int, gameDiv *goquery.Selection) {
		idAttr, _ := gameDiv.Attr("id")
		gameIDStr := strings.TrimPrefix(idAttr, "game-")
		gameID, _ := strconv.Atoi(gameIDStr)

		card := gameDiv.Find("rg-game-card")
		outcome, _ := card.Attr("outcome")

		// Cancelled games (forfeit/injury/etc.) also render with outcome="",
		// but carry a "cancelled" class token on rg-game-card and a
		// span.tag.game-cancelled marker in the tags slot — check both since
		// either can appear depending on how the game was cancelled.
		wasCancelled := false
		if classAttr, ok := card.Attr("class"); ok {
			for _, c := range strings.Fields(classAttr) {
				if c == "cancelled" {
					wasCancelled = true
					break
				}
			}
		}
		if !wasCancelled && card.Find("div[slot='tags'] span.tag.game-cancelled").Length() > 0 {
			wasCancelled = true
		}

		// Scheduled-but-not-yet-played games render with outcome="" and no set scores.
		// Without this check they fall through to MatchP's 0:0 default (p=0.5), which
		// misreports them as a draw instead of omitting them from the results.
		// Cancelled games are kept even though outcome is also "" for them.
		if strings.TrimSpace(outcome) == "" && !wasCancelled {
			return
		}

		round := strings.TrimSpace(card.Find("div[slot='tags'] a.tag").First().Text())

		// Collect the two teams in DOM order: [0] = first team, [1] = second team.
		teamDivs := card.Find("div.teams div.team")
		if teamDivs.Length() < 2 {
			return
		}

		weAreTeamOne := false
		opponentName := ""

		teamDivs.Each(func(i int, t *goquery.Selection) {
			onclick, _ := t.Find("a.details").Attr("onclick")
			m := reTournamentTeamIDOnclick.FindStringSubmatch(onclick)
			if len(m) >= 2 {
				tid, _ := strconv.Atoi(m[1])
				if tid == ourTeamID {
					weAreTeamOne = (i == 0)
				} else {
					opponentName = strings.TrimSpace(t.Find("span.name").Text())
				}
			}
		})

		// Parse set scores in order; each div.set contains exactly two span.score children.
		var sets []SetScore
		card.Find("div.scores div.set").Each(func(_ int, setDiv *goquery.Selection) {
			spans := setDiv.Find("span.score")
			if spans.Length() < 2 {
				return
			}
			s1, _ := strconv.Atoi(strings.TrimSpace(spans.Eq(0).Text()))
			s2, _ := strconv.Atoi(strings.TrimSpace(spans.Eq(1).Text()))
			sets = append(sets, SetScore{TeamOneScore: s1, TeamTwoScore: s2})
		})

		result.Games = append(result.Games, GameCard{
			GameID:       gameID,
			Round:        round,
			OpponentName: opponentName,
			WeAreTeamOne: weAreTeamOne,
			SetScores:    sets,
			Outcome:      outcome,
			WasCancelled: wasCancelled,
		})
	})

	return result, nil
}

// cleanPlayerName strips nickname patterns like `Joceline "Jocie" Hinz` → `Joceline  Hinz`
// and collapses extra whitespace. Playerzone sometimes embeds nicknames in double quotes.
func cleanPlayerName(name string) string {
	// Remove quoted nickname segments.
	re := regexp.MustCompile(`\s*"[^"]*"\s*`)
	name = re.ReplaceAllString(name, " ")
	return strings.Join(strings.Fields(name), " ")
}

// --- RGX formula ---

// DivisionStartingRGX returns the proxy RGX for a new player in the given division.
// Contender is a distinct division from Pro (unlike the old rules, where Contender
// was treated as a synonym for Pro) — see playerzone.roundnetgermany.de/ranking/rg-index/faq.
func DivisionStartingRGX(divisionName string) int {
	lower := strings.ToLower(divisionName)
	switch {
	case strings.Contains(lower, "pro"):
		return 1600
	case strings.Contains(lower, "contender"):
		return 1400
	case strings.Contains(lower, "advanced"):
		return 1250
	case strings.Contains(lower, "intermediate"):
		return 1100
	case strings.Contains(lower, "beginner"), strings.Contains(lower, "basic"):
		return 1000
	default:
		return 1200
	}
}

// RGXBreakdown exposes every intermediate value of the RGX formula, useful for displaying
// or debugging a single game's calculation (see cmd/rgxcalc).
type RGXBreakdown struct {
	B     float64 // 1.0 for Best-of-3, 0.75 for single-set
	P     float64 // match outcome value, see MatchP
	E1    float64 // expected score for our team given r1/r2
	Delta int     // rounded RGX delta for our team
}

// ComputeDeltaBreakdown computes the RGX formula for a single game and returns every
// intermediate value alongside the final rounded delta.
//   - r1: combined pre-tournament RGX of our team
//   - r2: combined pre-tournament RGX of the opponent team
//   - setsWon, setsLost: sets won/lost by our team in this game
func ComputeDeltaBreakdown(r1, r2, setsWon, setsLost int) RGXBreakdown {
	totalSets := setsWon + setsLost

	var b float64
	if totalSets <= 1 {
		b = 0.75 // single-set format
	} else {
		b = 1.0 // multi-set format (Best-of-3 or Best-of-5)
	}

	p := MatchP(setsWon, setsLost)
	e1 := 1.0 / (1.0 + math.Pow(10, float64(r2-r1)/550.0))
	delta := b * 50.0 * (p - e1)

	return RGXBreakdown{B: b, P: p, E1: e1, Delta: int(math.Round(delta))}
}

// ComputeGameDelta calculates the RGX delta for our team in a single game.
//   - r1: combined pre-tournament RGX of our team
//   - r2: combined pre-tournament RGX of the opponent team
//   - game: parsed game card from the schedule
func ComputeGameDelta(r1, r2 int, game GameCard) int {
	setsWon, setsLost := countSets(game)
	return ComputeDeltaBreakdown(r1, r2, setsWon, setsLost).Delta
}

// countSets counts how many sets our team won and lost.
func countSets(game GameCard) (won, lost int) {
	for _, s := range game.SetScores {
		var ourScore, theirScore int
		if game.WeAreTeamOne {
			ourScore, theirScore = s.TeamOneScore, s.TeamTwoScore
		} else {
			ourScore, theirScore = s.TeamTwoScore, s.TeamOneScore
		}
		if ourScore > theirScore {
			won++
		} else if theirScore > ourScore {
			lost++
		}
	}
	return
}

// MatchP maps a (won:lost) set record to the p value used in the RGX formula, per the
// current table at playerzone.roundnetgermany.de/ranking/rg-index/faq. Covers single-set,
// best-of-3, and best-of-5 records.
func MatchP(won, lost int) float64 {
	switch fmt.Sprintf("%d:%d", won, lost) {
	case "1:0", "2:0", "3:0":
		return 1.0
	case "3:1":
		return 0.75
	case "2:1":
		return 0.67
	case "3:2":
		return 0.6
	case "1:1", "2:2":
		return 0.5
	case "2:3":
		return 0.4
	case "1:2":
		return 0.33
	case "1:3":
		return 0.25
	case "0:1", "0:2", "0:3":
		return 0.0
	default:
		return 0.5
	}
}

// GameResultFromCard converts a parsed GameCard into a GameResult using the RGX formula.
// A cancelled game never contributes to RGX (delta 0) since it wasn't actually
// played to completion, but its (possibly partial) set scores are still reported.
func GameResultFromCard(game GameCard, r1, r2 int, hasProxy bool) GameResult {
	sets := orientSets(game)
	if game.WasCancelled {
		return GameResult{
			Round:             game.Round,
			OpponentName:      game.OpponentName,
			Result:            "C",
			HasProxy:          hasProxy,
			WasCancelled:      true,
			Sets:              sets,
			RGXDelta:          0,
			RGXDeltaPerPlayer: 0,
		}
	}

	setsWon, setsLost := countSets(game)
	delta := ComputeGameDelta(r1, r2, game)
	result := setResultChar(setsWon, setsLost)
	return GameResult{
		Round:             game.Round,
		OpponentName:      game.OpponentName,
		Result:            result,
		RGXDelta:          delta,
		RGXDeltaPerPlayer: 0.5 * float32(delta),
		HasProxy:          hasProxy,
		Sets:              sets,
	}
}

// orientSets converts a game's raw team-one/team-two set scores into
// our-score/opponent-score pairs based on which side we played on.
func orientSets(game GameCard) []SetResult {
	sets := make([]SetResult, 0, len(game.SetScores))
	for _, s := range game.SetScores {
		if game.WeAreTeamOne {
			sets = append(sets, SetResult{OurScore: s.TeamOneScore, OpponentScore: s.TeamTwoScore})
		} else {
			sets = append(sets, SetResult{OurScore: s.TeamTwoScore, OpponentScore: s.TeamOneScore})
		}
	}
	return sets
}

func setResultChar(won, lost int) string {
	if won > lost {
		return "W"
	}
	if lost > won {
		return "L"
	}
	return "D"
}

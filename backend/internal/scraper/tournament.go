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
	ID           string // numeric division ID from URL, e.g. "2265"
	TournamentID string // numeric tournament ID from URL, e.g. "692"
	Name         string
	TeamsPath    string // full path to this division's teams page, e.g. "/tournaments/692-.../teams/2265"
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
	NumericID string // playerzone's numeric player ID, "" if the card links no profile
	Name      string
	RGX       int
}

// reProfileHref matches a player profile link, e.g. /players/2329-jonas-ka.
var reProfileHref = regexp.MustCompile(`^/players/(\d+)`)

// SetResult is one set's score, oriented so OurScore/OpponentScore are always
// from our team's perspective regardless of which side playerzone listed first.
type SetResult struct {
	OurScore      int `json:"our_score"`
	OpponentScore int `json:"opponent_score"`
}

// GameResult is the computed result for one game.
type GameResult struct {
	Round        string `json:"round"`
	OpponentName string `json:"opponent_name"`
	Result       string `json:"result"` // "W", "L", "D", or "C" for a cancelled game
	// TeamRGX/OpponentRGX are the combined pre-tournament RGX values (r1/r2)
	// the RGX formula used for this game. TeamRGX is fixed across all of a
	// player's games today; OpponentRGX varies per game/opponent. Team names
	// are set by the caller (OurTeamName here, OpponentName above).
	OurTeamName         string      `json:"our_team_name"`
	TeamRGX             int         `json:"team_rgx"`
	OpponentRGX         int         `json:"opponent_rgx"`
	RGXDelta            int         `json:"rgx_delta"`
	RGXDeltaPerPlayer   float32     `json:"rgx_delta_per_player"`
	PerformanceDelta    int         `json:"performance_delta"`
	PerformanceDeltaPerPlayer float32 `json:"performance_delta_per_player"`
	HasProxy            bool        `json:"has_proxy"`
	WasCancelled        bool        `json:"was_cancelled"`
	Sets                []SetResult `json:"sets"`
}

// reTournamentTeamIDOnclick matches onclick="load_team_info(\d+, 60615)" and captures the
// second argument, the tournament_team_id. Game cards call the same function with further
// arguments — load_team_info(40744, 62594, null, null, 457400) — so the match must not
// require the capture group to be followed by the closing parenthesis.
var reTournamentTeamIDOnclick = regexp.MustCompile(`load_team_info\(\s*\d+\s*,\s*(\d+)\s*[,)]`)

// reDivisionTeamsHref matches /tournaments/{id}-{slug}/teams/{div_id}, capturing
// both the tournament ID and the division ID.
var reDivisionTeamsHref = regexp.MustCompile(`/tournaments/(\d+)-[^/]+/teams/(\d+)$`)

// reTournamentHref matches any tournament-scoped path, capturing the numeric tournament ID.
var reTournamentHref = regexp.MustCompile(`^/tournaments/(\d+)-`)

// ParseTournamentID reads the numeric ID of the tournament a page belongs to, from the
// tournament-scoped links every tournament page carries (its navigation identity link, its
// tabs, its division links). It's what identifies the tournament when the page doesn't
// render a division sidebar to take the ID from.
//
// The site-wide navigation (rg-navigation) precedes those links on every page and links
// other tournaments — the current league season under "League", upcoming tournaments in a
// dropdown — so it is skipped.
func ParseTournamentID(r io.Reader) (int, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return 0, fmt.Errorf("parse tournament id html: %w", err)
	}

	found := 0
	doc.Find("[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		if a.Closest("rg-navigation").Length() > 0 {
			return true
		}
		href, _ := a.Attr("href")
		m := reTournamentHref.FindStringSubmatch(href)
		if len(m) < 2 {
			return true
		}
		id, err := strconv.Atoi(m[1])
		if err != nil {
			return true
		}
		found = id
		return false
	})

	if found == 0 {
		return 0, fmt.Errorf("no tournament link found on the page")
	}
	return found, nil
}

// ParseDivisionSidebar extracts every division of the tournament linked from the page,
// keyed by the division's display name so callers can match it against
// TeamScheduleResult.DivisionName to find the division our own team belongs to. It works on
// a team schedule page (/tournaments/schedule/team/{id}) and on a tournament's own detail
// page, both of which link a division's team list.
//
// Two layouts are supported. The original one renders a `div.sub-section` block per
// division; the current one renders a tournament navigation bar whose dropdown links the
// divisions, and on a played team's schedule page links only that team's own division (the
// "Results" tab). When the newer layout leaves a link without a usable name of its own, the
// division the page is currently showing is used — see currentDivisionName.
func ParseDivisionSidebar(r io.Reader) ([]Division, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse division sidebar html: %w", err)
	}

	divs := parseDivisionSections(doc)
	if len(divs) == 0 {
		divs = parseDivisionLinks(doc)
	}
	if len(divs) == 0 {
		return nil, fmt.Errorf("division sidebar parsed 0 divisions — possible DOM change")
	}
	return divs, nil
}

// parseDivisionSections reads the original sidebar layout: one div.sub-section per division,
// its name in div.sub-title and its teams link in div.big-title.
func parseDivisionSections(doc *goquery.Document) []Division {
	var divs []Division

	doc.Find("div.sub-section").Each(func(_ int, sec *goquery.Selection) {
		name := strings.TrimSpace(sec.Find("div.sub-title").First().Text())
		if name == "" {
			return
		}

		var teamsPath string
		var tournamentID, divID string
		sec.Find("div.big-title a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			href, _ := a.Attr("href")
			m := reDivisionTeamsHref.FindStringSubmatch(href)
			if len(m) < 3 {
				return true // keep looking; this a wasn't the teams link
			}
			teamsPath = href
			tournamentID = m[1]
			divID = m[2]
			return false
		})
		if teamsPath == "" {
			return
		}

		divs = append(divs, Division{ID: divID, TournamentID: tournamentID, Name: name, TeamsPath: teamsPath})
	})

	return divs
}

// ParseDivisionLinks finds a tournament's divisions by their links alone: any
// /tournaments/{id}-{slug}/teams/{div_id} href on the page, named from the link's own text
// plus the label of the group it sits in.
//
// It makes no assumptions about wrappers, classes or ordering, which is what ParseDivisionSidebar
// falls back to when the markup it knows isn't there, and what tooling that must keep
// working across a layout change (cmd/fixturecapture) uses directly.
func ParseDivisionLinks(r io.Reader) ([]Division, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse division links html: %w", err)
	}
	divs := parseDivisionLinks(doc)
	if len(divs) == 0 {
		return nil, fmt.Errorf("no division team links found on the page")
	}
	return divs, nil
}

// genericTabLabels are link texts that name a page rather than a division — the tournament
// navigation labels a division's team list "Teams" or "Results" depending on whether it has
// been played, so those must not be mistaken for the division's name.
var genericTabLabels = map[string]bool{
	"teams": true, "results": true, "games": true, "teams / results": true,
}

func parseDivisionLinks(doc *goquery.Document) []Division {
	var divs []Division
	seen := map[string]bool{}
	current := currentDivisionName(doc)

	doc.Find("[href]").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		m := reDivisionTeamsHref.FindStringSubmatch(href)
		if len(m) < 3 || seen[m[2]] {
			return
		}
		seen[m[2]] = true

		name := strings.TrimSpace(a.Text())
		if genericTabLabels[strings.ToLower(name)] {
			// A navigation tab pointing at the current page's own division.
			name = current
		} else if group := divisionGroupLabel(a); group != "" && name != "" {
			// Grouped dropdowns split the name in two: the tier on the item ("Advanced"),
			// the category on the group ("Women"). Schedule pages spell the full name
			// tier-first, so join it in that order to keep the two matchable.
			name = name + " " + group
		}
		if name == "" {
			name = current
		}
		if name == "" {
			name = "division " + m[2]
		}

		divs = append(divs, Division{
			ID:           m[2],
			TournamentID: m[1],
			Name:         name,
			TeamsPath:    href,
		})
	})

	return divs
}

// divisionGroupLabel returns the category a division link is grouped under, e.g. "Women" for
// an "Advanced/Contender/Pro" entry. Only the tournament navigation's own grouping counts:
// any other labelled ancestor (the page's main nav, a landmark region) would otherwise
// contribute a meaningless prefix.
func divisionGroupLabel(a *goquery.Selection) string {
	group := a.Closest("div.rg-tournament-nav--division-group")
	if group.Length() == 0 {
		return ""
	}
	if label, ok := group.Attr("aria-label"); ok {
		if label = strings.TrimSpace(label); label != "" {
			return label
		}
	}
	return strings.TrimSpace(group.Find("span.rg-tournament-nav--division-group-label").First().Text())
}

// currentDivisionName reads the division the page is currently showing out of the tournament
// navigation — the highlighted dropdown entry, or the collapsed label that stands in for it.
// Returns "" when the page has no such navigation.
func currentDivisionName(doc *goquery.Document) string {
	context := doc.Find("div.rg-tournament-nav--context")
	if context.Length() == 0 {
		return ""
	}

	active := context.Find("rg-navigation-dropdown-item.active").First()
	if name := strings.TrimSpace(active.Text()); name != "" && !genericTabLabels[strings.ToLower(name)] {
		if group := divisionGroupLabel(active); group != "" {
			return name + " " + group
		}
		return name
	}

	// The collapsed navigation item renders the current division as its own text, with the
	// dropdown holding the alternatives — take only the text belonging to the item itself.
	item := context.Find("rg-navigation-item").First()
	return strings.TrimSpace(item.Contents().Not("rg-navigation-dropdown").Text())
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

	result.DivisionName = scheduleDivisionName(doc)
	result.TeamName = scheduleTeamName(doc)

	// Parse each player card on the page — these are our team's players with individual RGX.
	doc.Find("div.player-card").Each(func(_ int, card *goquery.Selection) {
		nameLink := card.Find("div.info-player-name a").First()
		name := strings.TrimSpace(nameLink.Text())
		// HTML-encoded quotes in names (like Joceline &quot;Jocie&quot;) come through decoded by goquery.
		name = cleanPlayerName(name)
		if name == "" {
			return
		}
		// Non-GRT tournaments (e.g. EURA-rated ones) don't render an RGX badge on
		// player cards at all, since RGX doesn't apply to them — firstField
		// returns "" rather than panicking, which strconv.Atoi below turns into
		// rgx 0, and the caller's proxy-RGX fallback (spRGX <= 0) takes it from there.
		href, _ := nameLink.Attr("href")
		numericID := ""
		if m := reProfileHref.FindStringSubmatch(href); len(m) >= 2 {
			numericID = m[1]
		}
		result.Players = append(result.Players, TeamPlayer{NumericID: numericID, Name: name, RGX: rgxFromBadge(card)})
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
		isBye := false

		teamDivs.Each(func(i int, t *goquery.Selection) {
			name := strings.TrimSpace(t.Find("span.name").Text())
			if isByeName(name) {
				// Bye rows render with an empty load_team_info(, ) onclick, so the
				// team-ID match below never fires for them and opponentName would
				// otherwise stay "" — misreporting as a draw against an unknown team.
				isBye = true
			}
			onclick, _ := t.Find("a.details").Attr("onclick")
			m := reTournamentTeamIDOnclick.FindStringSubmatch(onclick)
			if len(m) >= 2 {
				tid, _ := strconv.Atoi(m[1])
				if tid == ourTeamID {
					weAreTeamOne = (i == 0)
				} else {
					opponentName = name
				}
			}
		})

		// A "Bye"/"Freilos" row is an automatic advance, not a played game — omit
		// it from the schedule entirely instead of letting it fall through to
		// MatchP's 0:0 default (p=0.5), which would misreport it as a draw.
		if isBye {
			return
		}

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

// rgxFromBadge reads the RGX value out of a container holding an RGX badge — a player card
// on a schedule page, a row of the ranking table. The badge used to be a span holding the
// number as text (optionally followed by a trend arrow, e.g. "1963 ↙") and is now an
// rg-score-badge element carrying it in a score attribute; both are read, newest first.
//
// A missing badge is not an error: non-GRT tournaments don't render RGX at all, and a player
// with no RGX yet has none to render. Either way this returns 0, and the caller's
// proxy-RGX fallback takes over.
func rgxFromBadge(container *goquery.Selection) int {
	if score, ok := container.Find("rg-score-badge[score]").First().Attr("score"); ok {
		if rgx, err := strconv.Atoi(strings.TrimSpace(strings.ReplaceAll(score, ".", ""))); err == nil {
			return rgx
		}
	}
	// The DE locale writes a thousands separator, so strip it before parsing.
	rgxText := firstField(strings.TrimSpace(container.Find("span.rgx-value").First().Text()))
	rgx, _ := strconv.Atoi(strings.ReplaceAll(rgxText, ".", ""))
	return rgx
}

// scheduleDivisionName resolves the division a team schedule page belongs to. Playerzone
// renders it in a different place depending on whether the tournament has started:
// a "tournament-division" summary line before it does, the tournament navigation once it
// has. The page title ("Schedule {team} - {division}") carries it in both cases and is used
// as the fallback, ahead of the original layout's section heading.
func scheduleDivisionName(doc *goquery.Document) string {
	// "Advanced/Contender/Pro Women, 15 Teams" — the team count is not part of the name.
	if line := strings.TrimSpace(doc.Find("div.tournament-division").First().Text()); line != "" {
		if i := strings.Index(line, ","); i >= 0 {
			line = line[:i]
		}
		return strings.TrimSpace(line)
	}
	if name := currentDivisionName(doc); name != "" {
		return name
	}
	if _, division, ok := scheduleTitleParts(doc); ok {
		return division
	}
	return strings.TrimSpace(doc.Find("div.row.section-title h3.event-title").First().Text())
}

// scheduleTeamName resolves whose schedule the page shows, across the same layout variants.
func scheduleTeamName(doc *goquery.Document) string {
	if name := strings.TrimSpace(doc.Find("div.second-title span a").First().Text()); name != "" {
		return name
	}
	if name := strings.TrimSpace(doc.Find("div.tournament-team").First().Text()); name != "" {
		return name
	}
	if team, _, ok := scheduleTitleParts(doc); ok {
		return team
	}
	return strings.TrimSpace(doc.Find("div.row.section-title h3.event-title").First().Text())
}

// scheduleTitleParts splits a schedule page's title, "Schedule {team} - {division}", into
// its two halves. Reported as not ok for any other title shape, so a differently-titled page
// never yields a garbage name.
func scheduleTitleParts(doc *goquery.Document) (team, division string, ok bool) {
	title := strings.Join(strings.Fields(doc.Find("title").First().Text()), " ")
	rest, found := strings.CutPrefix(title, "Schedule ")
	if !found {
		return "", "", false
	}
	team, division, found = strings.Cut(rest, " - ")
	if !found {
		return "", "", false
	}
	team, division = strings.TrimSpace(team), strings.TrimSpace(division)
	if team == "" || division == "" {
		return "", "", false
	}
	return team, division, true
}

// firstField returns the first whitespace-separated token in s, or "" if s has
// no tokens — a panic-safe replacement for strings.Fields(s)[0], which
// indexes an empty slice (and panics) when s is blank.
func firstField(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// isByeName reports whether a team-slot name marks an automatic-advance row rather than
// a real opponent. Playerzone renders these as "Bye" in English; "Freilos" is the German
// term used in the product/domain docs, so it's matched defensively too.
func isByeName(name string) bool {
	lower := strings.ToLower(name)
	return lower == "bye" || lower == "freilos"
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
	B                  float64 // 1.0 for Best-of-3, 0.75 for single-set
	P                  float64 // match outcome value, see MatchP
	E1                 float64 // expected score for our team given r1/r2
	Delta              int     // rounded RGX delta for our team
	PerformanceE1      float64 // expected score using d=350
	PerformanceDelta   int     // rounded performance delta for our team
}

// ComputeDeltaBreakdown computes the RGX formula for a single game and returns every
// intermediate value alongside the final rounded delta. It also computes performance
// using d=350 instead of d=550.
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

	performanceE1 := 1.0 / (1.0 + math.Pow(10, float64(r2-r1)/350.0))
	performanceDelta := b * 50.0 * (p - performanceE1)

	return RGXBreakdown{
		B:                  b,
		P:                  p,
		E1:                 e1,
		Delta:              int(math.Round(delta)),
		PerformanceE1:      performanceE1,
		PerformanceDelta:   int(math.Round(performanceDelta)),
	}
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

// GameResultFromCard converts a parsed GameCard into a GameResult using the RGX formula
// and performance formula (d=350).
// A cancelled game never contributes to RGX or performance (delta 0) since it wasn't
// actually played to completion, but its (possibly partial) set scores are still reported.
func GameResultFromCard(game GameCard, r1, r2 int, hasProxy bool) GameResult {
	sets := orientSets(game)
	if game.WasCancelled {
		return GameResult{
			Round:                     game.Round,
			OpponentName:              game.OpponentName,
			TeamRGX:                   r1,
			OpponentRGX:               r2,
			Result:                    "C",
			HasProxy:                  hasProxy,
			WasCancelled:              true,
			Sets:                      sets,
			RGXDelta:                  0,
			RGXDeltaPerPlayer:         0,
			PerformanceDelta:          0,
			PerformanceDeltaPerPlayer: 0,
		}
	}

	setsWon, setsLost := countSets(game)
	breakdown := ComputeDeltaBreakdown(r1, r2, setsWon, setsLost)
	result := setResultChar(setsWon, setsLost)
	return GameResult{
		Round:                     game.Round,
		OpponentName:              game.OpponentName,
		TeamRGX:                   r1,
		OpponentRGX:               r2,
		Result:                    result,
		RGXDelta:                  breakdown.Delta,
		RGXDeltaPerPlayer:         0.5 * float32(breakdown.Delta),
		PerformanceDelta:          breakdown.PerformanceDelta,
		PerformanceDeltaPerPlayer: 0.5 * float32(breakdown.PerformanceDelta),
		HasProxy:                  hasProxy,
		Sets:                      sets,
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

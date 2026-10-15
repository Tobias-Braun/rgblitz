package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// The Deutsche Roundnet Liga (DRL) is a single playerzone tournament spanning a whole winter
// season. Its teams are club squads, each matchday is a "Begegnung" (club vs club) made of five
// pair games ("Partien": Open 1/2, Damen, Mixed 1/2), and the date and city of a matchday hang
// off the matchday group the Begegnung belongs to, not off the tournament.

// reLeagueSeasonName matches the season tournament's name, e.g. "Deutsche Roundnet Liga 2026/2027".
// Anchored so other national leagues on the platform (e.g. the Swiss one) never match.
var reLeagueSeasonName = regexp.MustCompile(`^Deutsche Roundnet Liga \d{4}/\d{4}$`)

// ActiveLeagueSeason picks the current DRL season out of the upcoming-tournaments list. The
// season stays listed until its final date, so it is found for the whole season. Reports false
// when no season is listed (off-season).
func ActiveLeagueSeason(entries []TournamentListEntry) (TournamentListEntry, bool) {
	for _, e := range entries {
		if reLeagueSeasonName.MatchString(strings.TrimSpace(e.Name)) {
			return e, true
		}
	}
	return TournamentListEntry{}, false
}

// LeagueFixture is one Begegnung on a league team's schedule page.
type LeagueFixture struct {
	GameID          int
	Matchday        string // tag text, e.g. "1. Matchday #2"
	MatchdayPath    string // the tag's link to the matchday group's page
	MatchdayGroupID int
	OpponentTeamID  int // the opponent's tournament registration ID
	OpponentName    string
	WeAreTeamOne    bool
}

// reSubgamesID matches the Begegnung's expand handler, e.g. show_subgames('392883').
var reSubgamesID = regexp.MustCompile(`show_subgames\('?(\d+)'?\)`)

// ParseLeagueFixtures reads the Begegnungen of a league team's schedule page
// (/tournaments/schedule/team/{ourTeamID}) — played and upcoming. Only regular-season
// Begegnungen carry a matchday group; play-off phases don't and are skipped.
func ParseLeagueFixtures(r io.Reader, ourTeamID int) ([]LeagueFixture, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse league fixtures html: %w", err)
	}

	var fixtures []LeagueFixture
	doc.Find("div[id^='game-'] rg-game-card[variant='game']").Each(func(_ int, card *goquery.Selection) {
		if f, ok := parseLeagueFixture(card, ourTeamID); ok {
			fixtures = append(fixtures, f)
		}
	})

	if len(fixtures) == 0 {
		return nil, fmt.Errorf("league schedule parsed 0 matchday fixtures — possible DOM change")
	}
	return fixtures, nil
}

func parseLeagueFixture(card *goquery.Selection, ourTeamID int) (LeagueFixture, bool) {
	tag := card.Find("div[slot='tags'] a.tag[href]").First()
	path, _ := tag.Attr("href")
	groupID := matchdayGroupID(path)
	if groupID == 0 {
		return LeagueFixture{}, false
	}

	onclick, _ := card.Find("[onclick*='show_subgames']").First().Attr("onclick")
	m := reSubgamesID.FindStringSubmatch(onclick)
	if len(m) < 2 {
		return LeagueFixture{}, false
	}
	gameID, _ := strconv.Atoi(m[1])

	f := LeagueFixture{
		GameID:          gameID,
		Matchday:        strings.Join(strings.Fields(tag.Text()), " "),
		MatchdayPath:    path,
		MatchdayGroupID: groupID,
	}
	foundUs := false
	card.Find("div.teams div.team").Each(func(i int, t *goquery.Selection) {
		onclick, _ := t.Find("a.details").Attr("onclick")
		m := reTournamentTeamIDOnclick.FindStringSubmatch(onclick)
		if len(m) < 2 {
			return
		}
		id, _ := strconv.Atoi(m[1])
		if id == ourTeamID {
			foundUs = true
			f.WeAreTeamOne = i == 0
			return
		}
		f.OpponentTeamID = id
		f.OpponentName = strings.TrimSpace(t.Find("span.name").Text())
	})
	return f, foundUs && f.OpponentTeamID != 0
}

// matchdayGroupID reads the md_group query parameter of a matchday tag's link, 0 if absent.
func matchdayGroupID(path string) int {
	u, err := url.Parse(path)
	if err != nil {
		return 0
	}
	id, _ := strconv.Atoi(u.Query().Get("md_group"))
	return id
}

// MatchdayGroup is where and when one group of a league matchday is played.
type MatchdayGroup struct {
	Name string // e.g. "Gruppe B"
	Date string // YYYY-MM-DD
	City string
}

// ParseMatchdayGroup reads one matchday group's header from its matchday page. The page renders
// only the requested group (?md_group=…) in full; the others are placeholders loaded later.
func ParseMatchdayGroup(r io.Reader, groupID int) (MatchdayGroup, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return MatchdayGroup{}, fmt.Errorf("parse matchday html: %w", err)
	}

	header := doc.Find(fmt.Sprintf("div.md-group-container.md-group-%d div.md-group-header", groupID)).First()
	if header.Length() == 0 {
		return MatchdayGroup{}, fmt.Errorf("matchday group %d has no header — possible DOM change", groupID)
	}
	date := germanDate(strings.TrimSpace(header.Find("span.md-group-date").Text()))
	if date == "" {
		return MatchdayGroup{}, fmt.Errorf("matchday group %d has no readable date", groupID)
	}
	return MatchdayGroup{
		Name: strings.TrimSpace(header.Find("span.md-group-name").Text()),
		Date: date,
		City: strings.TrimSpace(header.Find("span.md-group-city").Text()),
	}, nil
}

var reGermanDate = regexp.MustCompile(`^(\d{2})\.(\d{2})\.(\d{4})$`)

// germanDate converts DD.MM.YYYY to YYYY-MM-DD, "" if s has any other shape.
func germanDate(s string) string {
	m := reGermanDate.FindStringSubmatch(s)
	if len(m) < 4 {
		return ""
	}
	return m[3] + "-" + m[2] + "-" + m[1]
}

// Partie is one pair game of a Begegnung. Pairs hold each side's player names as rendered —
// surnames, with a first-name initial where needed to tell players apart ("F. Stadler").
type Partie struct {
	Label     string // "Open 1", "Damen", "Mixed 2", …
	Pairs     [2][]string
	SetScores []SetScore
}

// SubgamesPath is the AJAX endpoint the schedule page expands a Begegnung's Partien from.
func SubgamesPath(gameID int) string {
	return fmt.Sprintf("/ajax/get-subgames/%d", gameID)
}

// ParseSubgames reads the Partien of a Begegnung from the /ajax/get-subgames/{id} JSON
// response, whose "html" field carries one rg-game-card per Partie in the same side order as
// the Begegnung itself.
func ParseSubgames(r io.Reader) ([]Partie, error) {
	var payload struct {
		HTML string `json:"html"`
	}
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode subgames json: %w", err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(payload.HTML))
	if err != nil {
		return nil, fmt.Errorf("parse subgames html: %w", err)
	}

	var parties []Partie
	doc.Find("rg-game-card[variant='subgame']").Each(func(_ int, card *goquery.Selection) {
		if p, ok := parsePartie(card); ok {
			parties = append(parties, p)
		}
	})
	if len(parties) == 0 {
		return nil, fmt.Errorf("subgames parsed 0 Partien — possible DOM change")
	}
	return parties, nil
}

func parsePartie(card *goquery.Selection) (Partie, bool) {
	teams := card.Find("div.teams div.team")
	if teams.Length() < 2 {
		return Partie{}, false
	}
	p := Partie{Label: strings.Join(strings.Fields(card.Find("div[slot='tags'] span").First().Text()), " ")}
	for i := range p.Pairs {
		p.Pairs[i] = splitPair(teams.Eq(i).Find("span.name").Text())
	}
	card.Find("div.scores div.set").Each(func(_ int, set *goquery.Selection) {
		spans := set.Find("span.score")
		if spans.Length() < 2 {
			return
		}
		s1, err1 := strconv.Atoi(strings.TrimSpace(spans.Eq(0).Text()))
		s2, err2 := strconv.Atoi(strings.TrimSpace(spans.Eq(1).Text()))
		if err1 == nil && err2 == nil {
			p.SetScores = append(p.SetScores, SetScore{TeamOneScore: s1, TeamTwoScore: s2})
		}
	})
	return p, true
}

func splitPair(s string) []string {
	var names []string
	for n := range strings.SplitSeq(s, "/") {
		if n = strings.Join(strings.Fields(n), " "); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// PairNameMatches reports whether a name as rendered in a Partie ("Bottler", "F. Stadler")
// refers to the player with the given full name ("Jonas Bottler").
func PairNameMatches(pairName, fullName string) bool {
	full := strings.ToLower(strings.Join(strings.Fields(cleanPlayerName(fullName)), " "))
	pair := strings.ToLower(pairName)

	initial, surname, hasInitial := strings.Cut(pair, ". ")
	if !hasInitial || len(initial) != 1 {
		surname, initial = pair, ""
	}
	if !strings.HasSuffix(full, " "+surname) {
		return false
	}
	return strings.HasPrefix(full, initial)
}

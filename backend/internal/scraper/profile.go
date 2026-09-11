package scraper

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ProfileTournamentEntry is one tournament a player has played, as listed on
// their playerzone profile page.
type ProfileTournamentEntry struct {
	// Date is kept as the raw "DD.MM.YY" string from the page rather than
	// parsed, so callers compare it against time.Now().Format("02.01.06")
	// directly — the site uses a two-digit year, which is ambiguous to parse
	// into a full time.Time without guessing a century.
	Date             string
	TournamentTeamID int
}

// reLoadTournamentGames matches onclick="load_tournament_games(61549, ”)".
var reLoadTournamentGames = regexp.MustCompile(`load_tournament_games\((\d+)`)

// ParseProfilePage extracts every tournament a player has played from their
// profile page (/players/{numeric_id}). Tournaments without a published
// bracket yet (upcoming tournaments a player is registered for) render
// without a "Games" button and are skipped, since they have no team ID to
// fetch a schedule from.
func ParseProfilePage(r io.Reader) ([]ProfileTournamentEntry, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse profile html: %w", err)
	}

	var entries []ProfileTournamentEntry

	doc.Find("li.year-item").Each(func(_ int, li *goquery.Selection) {
		date := strings.TrimSpace(li.Find("div.team-tournament-date").First().Text())
		if date == "" {
			return
		}

		onclick, _ := li.Find("div.info-buttons rg-button[onclick]").First().Attr("onclick")
		m := reLoadTournamentGames.FindStringSubmatch(onclick)
		if len(m) < 2 {
			return
		}
		teamID, err := strconv.Atoi(m[1])
		if err != nil {
			return
		}

		entries = append(entries, ProfileTournamentEntry{Date: date, TournamentTeamID: teamID})
	})

	if len(entries) == 0 {
		return nil, fmt.Errorf("profile page parsed 0 tournament entries — possible DOM change")
	}
	return entries, nil
}

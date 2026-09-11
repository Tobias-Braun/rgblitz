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

	doc.Find("li.year-item, li.rg-player-tournaments--item").Each(func(_ int, li *goquery.Selection) {
		date := profileEntryDate(li)
		if date == "" {
			return
		}

		// The "Games" button is the only element carrying the tournament team ID. It used to
		// sit in a div.info-buttons wrapper and now sits in the card's actions row, so it's
		// matched by what it does rather than by where it is.
		onclick, _ := li.Find("rg-button[onclick*='load_tournament_games']").First().Attr("onclick")
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

// reProfileEntryDate matches the DD.MM.YY date a tournament card's subtitle starts with,
// e.g. "05.09.26, Bamberg".
var reProfileEntryDate = regexp.MustCompile(`^(\d{2}\.\d{2}\.\d{2})`)

// profileEntryDate reads the day a profile's tournament entry took place. It used to have
// its own element; it is now the first field of the tournament card's subtitle, alongside
// the location. Returns "" if neither yields a date in the expected DD.MM.YY shape, which
// skips the entry rather than matching "today" against something that isn't a date.
func profileEntryDate(li *goquery.Selection) string {
	if date := strings.TrimSpace(li.Find("div.team-tournament-date").First().Text()); date != "" {
		return date
	}
	subtitle, _ := li.Find("rg-expandable-card[subtitle]").First().Attr("subtitle")
	m := reProfileEntryDate.FindStringSubmatch(strings.TrimSpace(subtitle))
	if m == nil {
		return ""
	}
	return m[1]
}

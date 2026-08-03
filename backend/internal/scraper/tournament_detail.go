package scraper

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// TournamentDetail is the data extracted from a tournament's detail page
// (/tournaments/{id}-{slug}), used to classify a tournament as GRT or not before
// computing RGX changes for any player found registered in it.
type TournamentDetail struct {
	Name  string
	IsGRT bool
	Date  string // YYYY-MM-DD, first day of the tournament
}

// reGRTLabel matches the "German Roundnet Tour <year>" label playerzone renders (next to
// the GRT logo) on tournament detail pages that count towards the German Roundnet Tour.
// Non-GRT tournaments (e.g. EURA-rated ones) don't render this label at all. Matched
// against the whole page rather than a specific container, since the label's surrounding
// markup isn't reliably scoped to one wrapper element across tournament pages.
var reGRTLabel = regexp.MustCompile(`German Roundnet Tour \d{4}`)

// reTournamentStartDate matches the first date in the "Date: <b>...</b>" field, e.g.
// "Aug. 22, 2026" (single-day) or "July 18, 2026 - July 19, 2026" (multi-day) — playerzone
// renders the month both as a 3-letter abbreviation with a period and as the full name
// depending on the tournament, so the month token is matched loosely and resolved by its
// first 3 letters. Only the first (start) date is captured.
var reTournamentStartDate = regexp.MustCompile(`([A-Za-z]{3,9})\.?\s+(\d{1,2}),\s+(\d{4})`)

var monthAbbreviations = map[string]time.Month{
	"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April,
	"may": time.May, "jun": time.June, "jul": time.July, "aug": time.August,
	"sep": time.September, "oct": time.October, "nov": time.November, "dec": time.December,
}

// ParseTournamentDetail parses a tournament's detail page (/tournaments/{id}-{slug}).
func ParseTournamentDetail(r io.Reader) (TournamentDetail, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return TournamentDetail{}, fmt.Errorf("parse tournament detail html: %w", err)
	}

	name := strings.TrimSpace(doc.Find("h3.kf-title").First().Text())
	if name == "" {
		return TournamentDetail{}, fmt.Errorf("tournament detail parsed no title — possible DOM change")
	}

	// The "Date: <b>...</b>" field is always the first .tournament-info paragraph in the
	// tournament-details panel.
	dateText := doc.Find("p.tournament-info").First().Text()
	date, err := parseTournamentStartDate(dateText)
	if err != nil {
		return TournamentDetail{}, fmt.Errorf("tournament detail date: %w", err)
	}

	rankingText := doc.Find(".card-panel.tournament-details").First().Find(".info-row").Last().Text()
	if rankingText == "" {
		return TournamentDetail{}, fmt.Errorf("grt status parsing failed - possible DOM change")
	}

	return TournamentDetail{
		Name:  name,
		IsGRT: reGRTLabel.MatchString(rankingText),
		Date:  date,
	}, nil
}

// parseTournamentStartDate extracts the first calendar date found in text and returns it
// as YYYY-MM-DD.
func parseTournamentStartDate(text string) (string, error) {
	m := reTournamentStartDate.FindStringSubmatch(text)
	if m == nil {
		return "", fmt.Errorf("no date found in %q", strings.TrimSpace(text))
	}
	monthKey := strings.ToLower(m[1])
	if len(monthKey) > 3 {
		monthKey = monthKey[:3]
	}
	month, ok := monthAbbreviations[monthKey]
	if !ok {
		return "", fmt.Errorf("unrecognized month %q", m[1])
	}
	day, _ := strconv.Atoi(m[2])
	year, _ := strconv.Atoi(m[3])
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), nil
}

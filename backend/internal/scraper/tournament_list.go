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

// TournamentsUpcomingPath lists every tournament that hasn't finished yet — including the
// ones running right now, which is what makes it usable as a "what's on today" source.
const TournamentsUpcomingPath = "/tournaments/upcoming"

// TournamentListEntry is one tournament card from the upcoming list. It carries only what
// the card itself renders; GRT status still requires the tournament's own detail page
// (see ParseTournamentDetail).
type TournamentListEntry struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Location  string `json:"location"`
	StartDate string `json:"start_date"` // YYYY-MM-DD
	EndDate   string `json:"end_date"`   // YYYY-MM-DD; equal to StartDate for a single-day tournament
}

// RunsOn reports whether day falls inside this tournament's date range. Dates that failed
// to parse never match, so a malformed card can't pull an unrelated tournament into a
// capture.
func (e TournamentListEntry) RunsOn(day time.Time) bool {
	if e.StartDate == "" || e.EndDate == "" {
		return false
	}
	d := day.Format("2006-01-02")
	return e.StartDate <= d && d <= e.EndDate
}

// MightRunOn is the wider filter used to pick candidates worth confirming against a
// tournament's own detail page. Entries found in the navigation dropdown carry only the
// start date from their URL slug, so a tournament that began on Saturday and is still
// running on Sunday can't be recognised by RunsOn alone — a start date of day or the day
// before counts as a candidate. Tournaments run on weekends only (see docs/01-overview.md),
// so one day of slack covers every multi-day case.
func (e TournamentListEntry) MightRunOn(day time.Time) bool {
	if e.RunsOn(day) {
		return true
	}
	if e.StartDate == "" {
		return false
	}
	return e.StartDate == day.Format("2006-01-02") ||
		e.StartDate == day.AddDate(0, 0, -1).Format("2006-01-02")
}

// reTournamentBodyID matches the card's onclick handler, e.g. display_tournament_body(692).
var reTournamentBodyID = regexp.MustCompile(`display_tournament_body\((\d+)\)`)

// reFullDate matches a complete DD.MM.YYYY date as rendered in a card subtitle.
var reFullDate = regexp.MustCompile(`(\d{1,2})\.(\d{1,2})\.(\d{4})`)

// reLeadingDayOnly matches the abbreviated multi-day form where the start date omits the
// month and year because the end date supplies them, e.g. "02. - 06.09.2026".
var reLeadingDayOnly = regexp.MustCompile(`^\s*(\d{1,2})\.\s*[-–]`)

// reTournamentSlugHref matches a tournament link of the form
// /tournaments/{id}-{YYYY-MM-DD}-{slug}, capturing the ID and the start date the slug
// encodes.
var reTournamentSlugHref = regexp.MustCompile(`^/tournaments/(\d+)-(\d{4})-(\d{2})-(\d{2})-`)

// ParseTournamentList extracts every tournament it can find on a playerzone page, from two
// independent places:
//
//   - the expandable cards of /tournaments/upcoming, which carry a name, a full date range
//     and a location, but drop a tournament from the list as soon as it has started;
//   - the navigation dropdown rendered into every page, which *does* keep currently-running
//     tournaments and encodes their start date in the URL slug (no end date).
//
// The nav dropdown is what makes "which tournament is on right now" answerable at all;
// cards supply the richer data when a tournament hasn't started yet. Entries found in both
// keep the card's data. Individual unreadable cards or links are skipped rather than failing
// the page — playerzone mixes in tournaments from other national bodies that don't always
// render the same fields — but a page yielding nothing at all is reported as an error, since
// that means the layout changed under us.
func ParseTournamentList(r io.Reader) ([]TournamentListEntry, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse tournament list html: %w", err)
	}

	var entries []TournamentListEntry
	seen := map[int]bool{}

	doc.Find("rg-expandable-card[onclick]").Each(func(_ int, card *goquery.Selection) {
		onclick, _ := card.Attr("onclick")
		m := reTournamentBodyID.FindStringSubmatch(onclick)
		if len(m) < 2 {
			return
		}
		id, err := strconv.Atoi(m[1])
		if err != nil || seen[id] {
			return
		}

		title, _ := card.Attr("title")
		subtitle, _ := card.Attr("subtitle")
		dates, location := splitSubtitle(subtitle)
		start, end := parseDateRange(dates)

		seen[id] = true
		entries = append(entries, TournamentListEntry{
			ID:        id,
			Name:      strings.TrimSpace(title),
			Location:  location,
			StartDate: start,
			EndDate:   end,
		})
	})

	doc.Find(`a[href], [href]`).Each(func(_ int, link *goquery.Selection) {
		href, _ := link.Attr("href")
		m := reTournamentSlugHref.FindStringSubmatch(href)
		if len(m) < 5 {
			return
		}
		id, err := strconv.Atoi(m[1])
		if err != nil || seen[id] {
			return
		}
		seen[id] = true
		entries = append(entries, TournamentListEntry{
			ID:   id,
			Name: strings.TrimSpace(link.Text()),
			// The slug encodes the start day only; the tournament's own detail page is the
			// authority on how many days it runs (see TournamentDetail.EndDate).
			StartDate: fmt.Sprintf("%s-%s-%s", m[2], m[3], m[4]),
		})
	})

	if len(entries) == 0 {
		return nil, fmt.Errorf("tournament list parsed 0 tournaments — possible DOM change")
	}
	return entries, nil
}

// splitSubtitle splits a card subtitle like "18.07.2026 - 19.07.2026, Bochum" into its date
// part and its location. The location is everything after the last comma, since a date part
// never contains one but a location occasionally does.
func splitSubtitle(subtitle string) (dates, location string) {
	subtitle = strings.TrimSpace(subtitle)
	i := strings.LastIndex(subtitle, ",")
	if i < 0 {
		return subtitle, ""
	}
	return strings.TrimSpace(subtitle[:i]), strings.TrimSpace(subtitle[i+1:])
}

// parseDateRange reads the start and end date out of a card's date text. Playerzone renders
// the range inconsistently — "25.07.2026", "18.07.2026 - 19.07.2026", "15.08.2026-16.08.2026"
// and "02. - 06.09.2026" all occur — so every full date is collected and the abbreviated
// leading-day form is resolved against the end date's month and year. Returns empty strings
// if no date could be read at all.
func parseDateRange(text string) (start, end string) {
	matches := reFullDate.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return "", ""
	}

	full := make([]string, 0, len(matches))
	for _, m := range matches {
		day, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		year, _ := strconv.Atoi(m[3])
		if month < 1 || month > 12 || day < 1 || day > 31 {
			continue
		}
		full = append(full, fmt.Sprintf("%04d-%02d-%02d", year, month, day))
	}
	if len(full) == 0 {
		return "", ""
	}

	start, end = full[0], full[len(full)-1]

	// "02. - 06.09.2026": the only full date found is the end date, and the start day sits
	// in front of the separator.
	if len(full) == 1 {
		if m := reLeadingDayOnly.FindStringSubmatch(text); m != nil {
			day, _ := strconv.Atoi(m[1])
			if parsed, err := time.Parse("2006-01-02", end); err == nil && day >= 1 && day <= 31 {
				start = time.Date(parsed.Year(), parsed.Month(), day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
			}
		}
	}

	if start > end {
		start, end = end, start
	}
	return start, end
}

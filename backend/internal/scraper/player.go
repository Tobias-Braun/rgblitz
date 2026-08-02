package scraper

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

// Ranking page paths for the two tabs that make up a complete player index.
// Women play both Open and Women/Mixed tournaments with separate tracked RGX
// values, so both tabs must be scraped and merged — see MergeWomenRGX.
const (
	RankingOpenPath  = "/ranking/rg-index/1"
	RankingWomenPath = "/ranking/rg-index/2"
)

// FetchRankingPage fetches and parses a single ranking tab (RankingOpenPath or
// RankingWomenPath) via the given Fetcher.
func FetchRankingPage(fetcher Fetcher, path string) ([]db.Player, error) {
	body, err := fetcher.Fetch(path)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", path, err)
	}
	defer body.Close()

	players, err := ParseRankingPage(body)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return players, nil
}

// MergeWomenRGX combines the Open ranking with the Women ranking into a single
// player index containing the union of both: every Open entry, plus any Women
// entry that isn't also on the Open list (a woman who has never played an
// Open tournament). For any player who appears on both, her Open entry is
// kept (it usually carries richer data — club, avatar, games played) but her
// RGX is overridden with her Women RGX, since the persisted index must always
// reflect the Women value for players who have one — a specific tournament's
// live schedule page is the source of truth for which RGX actually applies on
// a given day (see ScrapeTodayResult), not this index.
//
// IDs alone aren't a reliable join key here: a player's ID is avatar-derived
// when the ranking row has an avatar and falls back to an internal numeric ID
// (see playerIDFromShowHistory) when it doesn't, and avatar presence depends
// on a player's rank position *within that specific tab* — a woman can easily
// crack the top of the (much shorter) Women tab and get an avatar-derived ID
// there while sitting well outside the Open tab's top ranks with a numeric
// fallback ID (or vice versa), giving her two different IDs across the two
// tabs. Matching falls back to an exact, case-insensitive name match — names
// are rendered identically on both ranking tabs (unlike the abbreviated names
// seen on team-schedule pages) — whenever the ID lookup misses.
func MergeWomenRGX(open, women []db.Player) []db.Player {
	womenIdxByID := make(map[string]int, len(women))
	womenIdxByName := make(map[string]int, len(women))
	for i, w := range women {
		womenIdxByID[w.ID] = i
		womenIdxByName[strings.ToLower(w.Name)] = i
	}

	matched := make([]bool, len(women))
	merged := make([]db.Player, len(open))
	for i, p := range open {
		idx, ok := womenIdxByID[p.ID]
		if !ok {
			idx, ok = womenIdxByName[strings.ToLower(p.Name)]
		}
		if ok {
			p.RGX = women[idx].RGX
			matched[idx] = true
		}
		merged[i] = p
	}

	// Women who never appear on the Open list (no Open tournament history)
	// are added as-is — discarding them would silently drop real players.
	for i, w := range women {
		if !matched[i] {
			merged = append(merged, w)
		}
	}
	return merged
}

// playerIDFromAvatarURL extracts the stable player ID from a playerzone avatar URL.
// URL format: .../profile_{ID}-{timestamp}.jpg
var rePlayerID = regexp.MustCompile(`profile_([^-]+)-\d{4}-\d{2}-\d{2}`)

func playerIDFromAvatarURL(rawURL string) string {
	base := filepath.Base(rawURL)
	m := rePlayerID.FindStringSubmatch(base)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// Past roughly the top 30, ranking rows drop the avatar entirely (no
// "player-img" div at all), so playerIDFromAvatarURL has nothing to parse.
// reShowHistory extracts the internal numeric player ID from the
// onclick="show_history(ID, tab)" handler on the name link instead, which
// every row has regardless of whether it also has an avatar.
var reShowHistory = regexp.MustCompile(`show_history\((\d+)\s*,`)

// playerIDFromShowHistory is a fallback for rows with no avatar image. It is
// prefixed so it can never collide with an avatar-derived ID (which is an
// alphanumeric code, not a bare number).
func playerIDFromShowHistory(onclick string) string {
	m := reShowHistory.FindStringSubmatch(onclick)
	if len(m) < 2 {
		return ""
	}
	return "PZ" + m[1]
}

// reTrailingDigits matches the numeric suffix of a DB player ID. Both ID
// shapes end in the raw playerzone numeric ID: avatar-derived IDs like
// "LUC2110" and show_history-derived IDs like "PZ8517".
var reTrailingDigits = regexp.MustCompile(`(\d+)$`)

// NumericPlayerID extracts the bare numeric playerzone ID from a DB player ID,
// for use in profile page URLs (https://playerzone.roundnetgermany.de/players/{numeric_id}).
func NumericPlayerID(id string) (string, error) {
	m := reTrailingDigits.FindStringSubmatch(id)
	if len(m) < 2 {
		return "", fmt.Errorf("player ID %q has no trailing numeric ID", id)
	}
	return m[1], nil
}

// ParseRankingPage reads the playerzone /ranking/rg-index/ HTML and returns all players.
// Callers should assert len(players) > 0 before upserting — zero results indicate a parse failure.
func ParseRankingPage(r io.Reader) ([]db.Player, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse ranking html: %w", err)
	}

	now := time.Now()
	var players []db.Player

	doc.Find("table#rgx-main-table tbody tr").Each(func(_ int, row *goquery.Selection) {
		nameLink := row.Find("td.player-col div.player-name a").First()
		name := strings.TrimSpace(nameLink.Text())
		if name == "" {
			return
		}

		avatarImg := row.Find("td.player-col div.player-img img")
		avatarURL, _ := avatarImg.Attr("src")
		playerID := playerIDFromAvatarURL(avatarURL)
		if playerID == "" {
			onclick, _ := nameLink.Attr("onclick")
			playerID = playerIDFromShowHistory(onclick)
		}
		if playerID == "" {
			return
		}

		// Prefer the long club name (hidden on small screens); fall back to abbreviated.
		clubName := strings.TrimSpace(row.Find("td.player-col span.player-club.hide-on-med-and-down").Text())
		if clubName == "" {
			clubName = strings.TrimSpace(row.Find("td.player-col span.player-club").First().Text())
		}

		clubLogoURL, _ := row.Find("td.player-col div.player-club-community img.club-img").Attr("src")

		// rgx-value may contain a trailing trend indicator (e.g. "1963  ↙"); take the first token.
		rgxText := strings.TrimSpace(row.Find("td.rgx-col span.rgx-value").First().Text())
		rgxText = strings.Fields(rgxText)[0]
		rgx, _ := strconv.Atoi(strings.ReplaceAll(rgxText, ".", "")) // DE locale uses . as thousands sep

		gamesText := strings.TrimSpace(row.Find("td.games-col").First().Text())
		games, _ := strconv.Atoi(strings.TrimSpace(gamesText))

		// 🆕 icon appears inside the name cell for brand-new players.
		isNew := strings.Contains(row.Find("td.player-col").Text(), "🆕")

		players = append(players, db.Player{
			ID:          playerID,
			Name:        name,
			ClubName:    clubName,
			AvatarURL:   avatarURL,
			ClubLogoURL: clubLogoURL,
			RGX:         rgx,
			GamesPlayed: games,
			IsNew:       isNew,
			ScrapedAt:   now,
		})
	})

	if len(players) == 0 {
		return nil, fmt.Errorf("ranking page parsed 0 players — possible DOM change")
	}
	return players, nil
}

package scraper

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ansiBrightYellow/ansiReset color the "Request to PZ" log line so live
// playerzone traffic stands out from other log output in the terminal.
const (
	ansiBrightYellow = "\033[93m"
	ansiReset        = "\033[0m"
)

// Fetcher retrieves the raw HTML body for a playerzone URL path (e.g. "/ranking/rg-index/").
// Implementations let the same pipeline code run against the live site or offline fixtures.
type Fetcher interface {
	Fetch(path string) (io.ReadCloser, error)
}

// DefaultUserAgent is sent on every live request to playerzone. Using a
// distinctly-named bot (rather than impersonating a browser) is a deliberate
// transparency choice: it makes RGBlitz trivially identifiable and blockable
// via robots.txt, which is what RESPECT_PERMISSION checks against.
const DefaultUserAgent = "RGBlitzBot"

// HTTPFetcher fetches pages from the real playerzone site, rate-limited through RL.
type HTTPFetcher struct {
	BaseURL   string
	RL        *RateLimiter
	Client    *http.Client
	UserAgent string
}

// NewHTTPFetcher builds an HTTPFetcher targeting the real playerzone host.
// rl must be shared with any other scraper (e.g. the weekly cron job) so the
// 1-request-per-2-seconds limit is enforced globally, not per fetcher.
func NewHTTPFetcher(rl *RateLimiter) *HTTPFetcher {
	return &HTTPFetcher{
		BaseURL:   "https://playerzone.roundnetgermany.de",
		RL:        rl,
		Client:    http.DefaultClient,
		UserAgent: DefaultUserAgent,
	}
}

func (f *HTTPFetcher) Fetch(path string) (io.ReadCloser, error) {
	f.RL.Wait()
	url := f.BaseURL + path
	log.Printf("%sRequest to PZ: %s%s", ansiBrightYellow, url, ansiReset)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	ua := f.UserAgent
	if ua == "" {
		ua = DefaultUserAgent
	}
	req.Header.Set("User-Agent", ua)
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("playerzone returned %d for %s", resp.StatusCode, path)
	}
	return resp.Body, nil
}

// FixtureFetcher serves pre-captured HTML snapshots from disk instead of hitting the
// network. It maps a playerzone URL path to a fixture file by shape rather than by exact
// ID/slug, since the fixture set is a single fixed snapshot (see scraper/fixtures/README.md).
type FixtureFetcher struct {
	Dir string
}

func NewFixtureFetcher(dir string) *FixtureFetcher {
	return &FixtureFetcher{Dir: dir}
}

func (f *FixtureFetcher) Fetch(path string) (io.ReadCloser, error) {
	file, err := fixtureFileForPath(path)
	if err != nil {
		return nil, err
	}
	full := filepath.Join(f.Dir, file)
	r, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("fixture fetch %q: %w", path, err)
	}
	return r, nil
}

// fixtureFileForPath maps a playerzone URL path to the fixture file that captures its
// page shape. Order matters: more specific patterns must be checked before the general
// "/tournaments/" fallback.
func fixtureFileForPath(path string) (string, error) {
	switch {
	case path == "/robots.txt":
		return "robots.txt", nil
	case path == "/ranking/rg-index/", path == RankingOpenPath:
		return "ranking.html", nil
	case path == RankingWomenPath:
		return "ranking-women.html", nil
	case path == "/tournaments/upcoming":
		return "tournaments-upcoming.html", nil
	case strings.HasPrefix(path, "/players/"):
		return "player-profile.html", nil
	case strings.HasPrefix(path, "/tournaments/schedule/team/"):
		return "team-schedule.html", nil
	case strings.Contains(path, "/teams/"):
		return "division-teams.html", nil
	case strings.HasPrefix(path, "/tournaments/"):
		return "tournament-detail.html", nil
	default:
		return "", fmt.Errorf("no fixture mapped for path %q", path)
	}
}

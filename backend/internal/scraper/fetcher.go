package scraper

import (
	"encoding/json"
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
	file, err := FixtureFileForPath(path)
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

// ManifestFileName is the index a capture writes next to its HTML files, mapping each
// fetched playerzone URL path to the file its response body was saved as.
const ManifestFileName = "manifest.json"

// ManifestFetcher replays a capture directory: unlike FixtureFetcher, which maps a path to
// a file by page *shape* (one snapshot per kind of page), it serves the exact response that
// was recorded for that exact path. That's what lets a capture containing many division and
// team pages — cmd/fixturecapture's whole-tournament mode — be replayed offline with full
// fidelity.
type ManifestFetcher struct {
	Dir   string
	Paths map[string]string
}

// NewManifestFetcher loads the manifest written by a capture. It fails if the manifest is
// missing or unreadable rather than silently falling back to shape-based lookup, since a
// partial replay would quietly produce different results than the capture did.
func NewManifestFetcher(dir string) (*ManifestFetcher, error) {
	raw, err := os.ReadFile(filepath.Join(dir, ManifestFileName))
	if err != nil {
		return nil, fmt.Errorf("read capture manifest: %w", err)
	}
	var paths map[string]string
	if err := json.Unmarshal(raw, &paths); err != nil {
		return nil, fmt.Errorf("parse capture manifest: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("capture manifest %s maps no paths", filepath.Join(dir, ManifestFileName))
	}
	return &ManifestFetcher{Dir: dir, Paths: paths}, nil
}

// HasManifest reports whether dir is a capture directory that can be replayed path-exactly.
func HasManifest(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ManifestFileName))
	return err == nil
}

func (f *ManifestFetcher) Fetch(path string) (io.ReadCloser, error) {
	file, ok := f.Paths[path]
	if !ok {
		return nil, fmt.Errorf("capture manifest has no recording for path %q", path)
	}
	r, err := os.Open(filepath.Join(f.Dir, file))
	if err != nil {
		return nil, fmt.Errorf("replay fetch %q: %w", path, err)
	}
	return r, nil
}

// FixtureFileForPath maps a playerzone URL path to the fixture file that captures its
// page shape. Order matters: more specific patterns must be checked before the general
// "/tournaments/" fallback. It is exported so tools that capture fresh snapshots from the
// live site (cmd/fixturecapture) name their files exactly the way FixtureFetcher will
// later look them up, making a capture directly usable as a FIXTURE_DIR.
func FixtureFileForPath(path string) (string, error) {
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

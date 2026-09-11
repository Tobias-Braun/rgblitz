package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// fetchRecord notes one request and the file its response body was written to.
type fetchRecord struct {
	Path      string `json:"path"`
	File      string `json:"file"`
	Bytes     int    `json:"bytes"`
	FetchedAt string `json:"fetched_at"`
	Error     string `json:"error,omitempty"`
	// Reused marks a page served from an earlier fetch in the same snapshot instead of a
	// second request.
	Reused bool `json:"reused,omitempty"`
}

// recordingFetcher wraps a Fetcher, writing every response body into the current snapshot
// directory and handing the buffered body on to the pipeline unchanged.
type recordingFetcher struct {
	inner scraper.Fetcher
	dir   string
	log   []fetchRecord
	// nameFn chooses the file name for a path. Player mode names files the way
	// FixtureFetcher looks them up, so a snapshot doubles as a drop-in fixture set; tournament
	// mode gives every page a distinct name, since it captures many pages of the same shape.
	// Either way manifest.json records the exact mapping.
	nameFn func(path string) string
	// seen counts how often each file name has been used in the current snapshot, so two
	// paths mapping to the same name can't silently overwrite each other.
	seen map[string]int
	// byPath remembers which file each already-fetched path was written to, so fetching the
	// same page twice within one snapshot costs no second request — at 1 req/2s that matters,
	// and a snapshot's manifest can only map a path to one file anyway.
	byPath map[string]string
	// total counts requests across the whole run, for the --max-requests budget.
	total int
}

// startSnapshot points the recorder at a new snapshot directory and clears the per-snapshot
// log; the run-wide request count deliberately survives.
func (r *recordingFetcher) startSnapshot(dir string, nameFn func(string) string) {
	r.dir = dir
	r.log = nil
	r.seen = map[string]int{}
	r.byPath = map[string]string{}
	r.nameFn = nameFn
}

func (r *recordingFetcher) records() []fetchRecord { return r.log }

// requestCount is the number of requests made so far in this run.
func (r *recordingFetcher) requestCount() int { return r.total }

func (r *recordingFetcher) Fetch(path string) (io.ReadCloser, error) {
	if file, ok := r.byPath[path]; ok {
		raw, err := os.ReadFile(filepath.Join(r.dir, file))
		if err == nil {
			r.log = append(r.log, fetchRecord{
				Path:      path,
				File:      file,
				Bytes:     len(raw),
				FetchedAt: time.Now().Format(time.RFC3339),
				Reused:    true,
			})
			return io.NopCloser(bytes.NewReader(raw)), nil
		}
		// Re-reading what we just wrote failed — fall through and fetch it again rather
		// than failing the capture over a local file problem.
	}

	rec := fetchRecord{Path: path, FetchedAt: time.Now().Format(time.RFC3339)}
	r.total++

	body, err := r.inner.Fetch(path)
	if err != nil {
		rec.Error = err.Error()
		r.log = append(r.log, rec)
		return nil, err
	}
	defer body.Close()

	raw, err := io.ReadAll(body)
	if err != nil {
		rec.Error = err.Error()
		r.log = append(r.log, rec)
		return nil, err
	}
	rec.Bytes = len(raw)
	rec.File = r.uniqueName(path)
	r.byPath[path] = rec.File

	if err := os.WriteFile(filepath.Join(r.dir, rec.File), raw, 0o644); err != nil {
		rec.Error = err.Error()
	}
	r.log = append(r.log, rec)

	return io.NopCloser(bytes.NewReader(raw)), nil
}

// uniqueName resolves the file name for a path, suffixing it if that name is already taken
// in this snapshot.
func (r *recordingFetcher) uniqueName(path string) string {
	nameFn := r.nameFn
	if nameFn == nil {
		nameFn = uniquePageName
	}
	name := nameFn(path)
	if r.seen == nil {
		r.seen = map[string]int{}
	}
	r.seen[name]++
	if n := r.seen[name]; n > 1 {
		ext := filepath.Ext(name)
		name = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), n, ext)
	}
	return name
}

// writeManifest saves the snapshot's path → file index, which scraper.ManifestFetcher replays.
// Failed fetches are left out: there's no file to serve for them, and a replay should fail
// the same way the capture did rather than find an empty page.
func (r *recordingFetcher) writeManifest(dir string, extra map[string]string) error {
	paths := map[string]string{}
	for k, v := range extra {
		paths[k] = v
	}
	for _, rec := range r.log {
		if rec.Error == "" && rec.File != "" {
			paths[rec.Path] = rec.File
		}
	}
	raw, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, scraper.ManifestFileName), raw, 0o644)
}

// fixturePageName names a file the way FixtureFetcher will look it up, so a single-player
// snapshot can be passed straight to --fixture-dir. Paths outside the fixture map still get
// captured, just under a derived name.
func fixturePageName(path string) string {
	if name, err := scraper.FixtureFileForPath(path); err == nil {
		return name
	}
	return uniquePageName(path)
}

var (
	reTeamSchedulePath = regexp.MustCompile(`^/tournaments/schedule/team/(\d+)$`)
	reDivisionPath     = regexp.MustCompile(`/tournaments/(\d+)-[^/]+/teams/(\d+)$`)
	reTournamentPath   = regexp.MustCompile(`^/tournaments/(\d+)(?:-|$)`)
	rePlayerPath       = regexp.MustCompile(`^/players/(\d+)$`)
)

// uniquePageName derives a distinct, self-describing file name from a URL path. A
// whole-tournament capture holds dozens of division and team pages, so unlike the fixture
// naming these have to carry their IDs.
func uniquePageName(path string) string {
	switch {
	case path == scraper.RankingOpenPath:
		return "ranking.html"
	case path == scraper.RankingWomenPath:
		return "ranking-women.html"
	case path == scraper.TournamentsUpcomingPath:
		return "tournaments-upcoming.html"
	case path == "/robots.txt":
		return "robots.txt"
	}
	if m := reTeamSchedulePath.FindStringSubmatch(path); m != nil {
		return fmt.Sprintf("team-%s-schedule.html", m[1])
	}
	if m := reDivisionPath.FindStringSubmatch(path); m != nil {
		return fmt.Sprintf("tournament-%s-division-%s-teams.html", m[1], m[2])
	}
	if m := rePlayerPath.FindStringSubmatch(path); m != nil {
		return fmt.Sprintf("player-%s-profile.html", m[1])
	}
	if m := reTournamentPath.FindStringSubmatch(path); m != nil {
		return fmt.Sprintf("tournament-%s-detail.html", m[1])
	}
	return sanitize(strings.Trim(path, "/")) + ".html"
}

// linkShared makes the run's shared pages (fetched once, used by every snapshot) available
// inside a snapshot directory, and reports the path → file entries to add to its manifest.
// Hard links keep a weekend of hourly captures from costing a multi-megabyte copy per
// snapshot; a plain copy is used where linking isn't possible.
func linkShared(from, to string, records []fetchRecord) (map[string]string, error) {
	paths := map[string]string{}
	for _, rec := range records {
		if rec.Error != "" || rec.File == "" {
			continue
		}
		src := filepath.Join(from, rec.File)
		dst := filepath.Join(to, rec.File)
		paths[rec.Path] = rec.File

		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.Link(src, dst); err == nil {
			continue
		}
		raw, err := os.ReadFile(src)
		if err != nil {
			return paths, err
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			return paths, err
		}
	}
	return paths, nil
}

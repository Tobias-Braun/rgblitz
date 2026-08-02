package scraper_test

import (
	"strings"
	"testing"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

const realRobotsTxt = `User-agent: *
Disallow: /players/*

User-agent: *
Disallow: /search/*

User-agent: *
Disallow: /teams/*

User-agent: *
Disallow: /tournaments/*

User-agent: *
Disallow: /ranking

User-agent: SemrushBot
Disallow: /
`

func TestParseRobotsTxt_RealFile(t *testing.T) {
	rules := scraper.ParseRobotsTxt(strings.NewReader(realRobotsTxt))

	// RGBlitzBot has no dedicated group, so it falls under "*" — which
	// disallows nearly every page this app needs to scrape.
	cases := []struct {
		path string
		want bool
	}{
		{"/ranking/rg-index/1", false},
		{"/ranking/rg-index/2", false},
		{"/tournaments/692-leguan-cup", false},
		{"/teams/foo", false},
		{"/players/PAULCHEN", false},
		{"/search/foo", false},
		{"/", true}, // not covered by any Disallow rule
	}
	for _, tc := range cases {
		if got := rules.Allowed("RGBlitzBot", tc.path); got != tc.want {
			t.Errorf("Allowed(RGBlitzBot, %q) = %v, want %v", tc.path, got, tc.want)
		}
	}

	// SemrushBot has its own group disallowing everything.
	if rules.Allowed("SemrushBot", "/") {
		t.Error("Allowed(SemrushBot, /) = true, want false (its group disallows /)")
	}
}

func TestCheckPermission_RealFile(t *testing.T) {
	rules := scraper.ParseRobotsTxt(strings.NewReader(realRobotsTxt))
	for _, p := range scraper.RequiredScrapePaths {
		if p == "/" {
			continue
		}
		if rules.Allowed(scraper.DefaultUserAgent, p) {
			t.Errorf("expected %q to be disallowed for %s per the real robots.txt", p, scraper.DefaultUserAgent)
		}
	}
}

func TestRobotsRules_AllowOverridesLongerDisallowTie(t *testing.T) {
	rules := scraper.ParseRobotsTxt(strings.NewReader(`
User-agent: *
Disallow: /foo
Allow: /foo
`))
	if !rules.Allowed("anybot", "/foo/bar") {
		t.Error("expected /foo/bar to be allowed when an equal-length Allow rule ties with Disallow")
	}
}

func TestRobotsRules_LongestPrefixWins(t *testing.T) {
	rules := scraper.ParseRobotsTxt(strings.NewReader(`
User-agent: *
Disallow: /foo
Allow: /foo/bar
`))
	if rules.Allowed("anybot", "/foo/baz") {
		t.Error("expected /foo/baz to be disallowed (only the shorter /foo rule matches)")
	}
	if !rules.Allowed("anybot", "/foo/bar/x") {
		t.Error("expected /foo/bar/x to be allowed (the longer, more specific Allow rule wins)")
	}
}

func TestRobotsRules_NoMatchingGroupDefaultsAllowed(t *testing.T) {
	rules := scraper.ParseRobotsTxt(strings.NewReader(`
User-agent: SemrushBot
Disallow: /
`))
	if !rules.Allowed("RGBlitzBot", "/anything") {
		t.Error("expected allow by default when neither a specific nor a wildcard group exists")
	}
}

func TestCheckPermission_UsesFetcher(t *testing.T) {
	fetcher := scraper.NewFixtureFetcher("../../../scraper/fixtures")
	allowed, err := scraper.CheckPermission(fetcher, scraper.DefaultUserAgent, scraper.RequiredScrapePaths)
	if err != nil {
		t.Fatalf("CheckPermission: %v", err)
	}
	if allowed {
		t.Error("expected the bundled robots.txt fixture to disallow scraping (mirrors the real site)")
	}
}

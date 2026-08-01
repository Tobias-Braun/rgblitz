package scraper

import (
	"bufio"
	"io"
	"strings"
)

// RequiredScrapePaths lists one representative path per page shape this app
// fetches from playerzone. A single "/tournaments/" prefix stands in for
// tournament detail, division-teams, and team-schedule pages alike, since
// they're all disallowed or allowed together under that prefix in robots.txt.
// RESPECT_PERMISSION evaluates all of these together and stores one overall
// verdict — if any of them is disallowed, the bot must not scrape at all.
var RequiredScrapePaths = []string{
	"/",
	RankingOpenPath,
	RankingWomenPath,
	"/tournaments/",
}

type robotsRule struct {
	prefix string
	allow  bool
}

// RobotsRules is a parsed robots.txt, grouped by (lowercased) user-agent token.
type RobotsRules struct {
	groups map[string][]robotsRule
}

// ParseRobotsTxt parses a robots.txt document into per-user-agent rule groups.
// It implements the common subset of the spec: "User-agent"/"Disallow"/"Allow"
// directives, comments (#...), and grouping (consecutive User-agent lines share
// the rules that follow, up to the next User-agent line). "*" in a path value
// is treated as a wildcard matching any suffix (i.e. only the text before the
// first "*" is used as a prefix) — sufficient for the simple prefix-style
// patterns real-world robots.txt files use.
func ParseRobotsTxt(r io.Reader) *RobotsRules {
	rules := &RobotsRules{groups: make(map[string][]robotsRule)}

	var currentAgents []string
	seenRule := false

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		sep := strings.IndexByte(line, ':')
		if sep < 0 {
			continue
		}
		field := strings.ToLower(strings.TrimSpace(line[:sep]))
		value := strings.TrimSpace(line[sep+1:])

		switch field {
		case "user-agent":
			// A User-agent line after a rule line starts a new group; consecutive
			// User-agent lines (no rule in between) share the rules that follow.
			if seenRule {
				currentAgents = nil
				seenRule = false
			}
			currentAgents = append(currentAgents, strings.ToLower(value))
		case "disallow", "allow":
			seenRule = true
			if value == "" && field == "disallow" {
				// An empty Disallow value means "allow everything" for this group.
				continue
			}
			prefix, _, _ := strings.Cut(value, "*")
			for _, ua := range currentAgents {
				rules.groups[ua] = append(rules.groups[ua], robotsRule{prefix: prefix, allow: field == "allow"})
			}
		}
	}
	return rules
}

// Allowed reports whether userAgent may fetch path, using the longest matching
// prefix rule (ties broken in favor of Allow) from the group whose token
// exactly matches userAgent (case-insensitive), falling back to the "*" group.
// With no matching rule at all, the path is allowed by default.
func (rr *RobotsRules) Allowed(userAgent, path string) bool {
	group, ok := rr.groups[strings.ToLower(userAgent)]
	if !ok {
		group, ok = rr.groups["*"]
		if !ok {
			return true
		}
	}

	bestLen := -1
	allowed := true
	for _, rule := range group {
		if !strings.HasPrefix(path, rule.prefix) {
			continue
		}
		if len(rule.prefix) > bestLen || (len(rule.prefix) == bestLen && rule.allow) {
			bestLen = len(rule.prefix)
			allowed = rule.allow
		}
	}
	return allowed
}

// FetchRobotsTxt fetches and parses "/robots.txt" via fetcher.
func FetchRobotsTxt(fetcher Fetcher) (*RobotsRules, error) {
	body, err := fetcher.Fetch("/robots.txt")
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return ParseRobotsTxt(body), nil
}

// CheckPermission fetches robots.txt and reports whether userAgent may fetch
// every path in paths. A single disallowed path makes the overall result false.
func CheckPermission(fetcher Fetcher, userAgent string, paths []string) (bool, error) {
	rules, err := FetchRobotsTxt(fetcher)
	if err != nil {
		return false, err
	}
	for _, p := range paths {
		if !rules.Allowed(userAgent, p) {
			return false, nil
		}
	}
	return true, nil
}

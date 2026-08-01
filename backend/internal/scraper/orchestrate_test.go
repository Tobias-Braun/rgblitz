package scraper

import "testing"

func TestFuzzyPlayerNameMatch(t *testing.T) {
	cases := []struct {
		pageName   string
		playerName string
		want       bool
	}{
		{"Lucas Christiani", "Lucas Christiani", true},
		{"lucas christiani", "Lucas Christiani", true},
		{"Lucas", "Lucas Christiani", true},
		{"Wanda Testfrau", "Wanda Testfrau", true},
		{"Bob", "Alice", false},
	}

	for _, tc := range cases {
		if got := fuzzyPlayerNameMatch(tc.pageName, tc.playerName); got != tc.want {
			t.Errorf("fuzzyPlayerNameMatch(%q, %q) = %v, want %v", tc.pageName, tc.playerName, got, tc.want)
		}
	}
}

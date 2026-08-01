// rgxcalc computes the RGX delta for a single game from the raw inputs, printing every
// intermediate value of the formula. It touches no network, database, or fixtures — useful
// for sanity-checking the formula in docs/03-data-requirements.md against hand-worked examples.
//
//	go run ./cmd/rgxcalc --r1 2347 --r2 2401 --won 2 --lost 0
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tobiasmarkusbraun/rgx-calc/internal/scraper"
)

func main() {
	r1 := flag.Int("r1", 0, "combined pre-tournament RGX of our team (sum of both players) — required")
	r2 := flag.Int("r2", 0, "combined pre-tournament RGX of the opponent team — required")
	won := flag.Int("won", 0, "sets won by our team")
	lost := flag.Int("lost", 0, "sets lost by our team")
	flag.Parse()

	if *r1 <= 0 || *r2 <= 0 {
		fmt.Fprintln(os.Stderr, "error: --r1 and --r2 are required and must be positive")
		flag.Usage()
		os.Exit(1)
	}
	if *won == 0 && *lost == 0 {
		fmt.Fprintln(os.Stderr, "error: give at least one of --won / --lost, e.g. --won 2 --lost 0")
		flag.Usage()
		os.Exit(1)
	}

	bd := scraper.ComputeDeltaBreakdown(*r1, *r2, *won, *lost)

	fmt.Printf("inputs:  r1=%d  r2=%d  sets=%d:%d\n\n", *r1, *r2, *won, *lost)
	fmt.Printf("b  (format factor)      = %.2f   %s\n", bd.B, formatNote(bd.B))
	fmt.Printf("p  (match outcome)      = %.2f\n", bd.P)
	fmt.Printf("e1 (expected score)     = 1 / (1 + 10^((r2-r1)/550)) = %.4f\n", bd.E1)
	fmt.Printf("delta = round(b * 50 * (p - e1))\n")
	fmt.Printf("      = round(%.2f * 50 * (%.2f - %.4f))\n", bd.B, bd.P, bd.E1)
	fmt.Printf("      = %+d\n", bd.Delta)
}

func formatNote(b float64) string {
	if b == 0.75 {
		return "(single-set format)"
	}
	return "(Best-of-3)"
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// The rendering helpers below are shared by both capture modes so a tournament report and a
// player report show the same numbers in the same layout.

// saveSnapshot writes a snapshot's manifest, capture.json and report.txt, reporting false when
// the capture itself couldn't be saved. A missing manifest only costs the offline replay.
func saveSnapshot(run runContext, dir string, sharedPaths map[string]string, capture any, report string) bool {
	if err := run.rec.writeManifest(dir, sharedPaths); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: write manifest: %v\n", err)
	}
	raw, err := json.MarshalIndent(capture, "", "  ")
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "capture.json"), raw, 0o644)
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "report.txt"), []byte(report), 0o644)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "  write capture: %v\n", err)
		return false
	}
	return true
}

// writeFetchedAndParseErrors opens every report: what was fetched, and which pages the parsers
// couldn't read.
func writeFetchedAndParseErrors(b *strings.Builder, records []fetchRecord, parseErrors map[string]string) {
	b.WriteString("\n--- fetched pages ---\n")
	writeFetchedPages(b, records)
	if len(parseErrors) == 0 {
		return
	}
	b.WriteString("\n--- parse errors ---\n")
	for file, msg := range parseErrors {
		fmt.Fprintf(b, "  %s: %s\n", file, msg)
	}
}

func writeFetchedPages(b *strings.Builder, records []fetchRecord) {
	for _, f := range records {
		fmt.Fprintf(b, "  %-52s -> %-40s %8d bytes %s\n", f.Path, f.File, f.Bytes, f.Error)
	}
}

// writeSchedulePlayers prints a team's players with the RGX read off the schedule page, and
// the r1 those values add up to — the single most useful line when a delta looks wrong,
// since every game of the day is computed against it.
func writeSchedulePlayers(b *strings.Builder, schedule scraper.TeamScheduleResult) {
	r1 := 0
	for _, pl := range schedule.Players {
		note := ""
		rgx := pl.RGX
		if rgx <= 0 {
			rgx = scraper.DivisionStartingRGX(schedule.DivisionName)
			note = fmt.Sprintf(" (no RGX on page → division proxy %d)", rgx)
		}
		fmt.Fprintf(b, "  player %-28s rgx=%d%s\n", pl.Name, pl.RGX, note)
		r1 += rgx
	}
	fmt.Fprintf(b, "  r1 (combined rgx from schedule page) = %d\n", r1)
}

func writeDivisionTeams(b *strings.Builder, teams []scraper.TeamEntry) {
	for _, t := range teams {
		fmt.Fprintf(b, "  %-34s team_id=%-7d combined_rgx=%-5d %s\n",
			t.Name, t.TournamentTeamID, t.CombinedRGX, strings.Join(t.PlayerNames, " + "))
	}
}

// writeGameLines prints one line per game with every input and intermediate of the RGX
// formula, recomputed from the recorded result so the printed delta and the printed b/p/e1
// can't drift apart.
func writeGameLines(b *strings.Builder, games []scraper.GameResult) {
	for _, g := range games {
		won, lost := setsFromResult(g)
		br := scraper.ComputeDeltaBreakdown(g.TeamRGX, g.OpponentRGX, won, lost)
		notes := ""
		if g.HasProxy {
			notes += " proxy"
		}
		if g.WasCancelled {
			notes += " cancelled"
		}
		fmt.Fprintf(b, "  %-14s vs %-30s %s %+4d  r1=%d r2=%d sets=%d:%d b=%.2f p=%.2f e1=%.4f%s\n",
			g.Round, g.OpponentName, g.Result, g.RGXDelta,
			g.TeamRGX, g.OpponentRGX, won, lost, br.B, br.P, br.E1, notes)
	}
}

// setsFromResult recounts sets won/lost from the oriented set scores in a GameResult, so
// the report shows the same (won, lost) pair the delta was computed from.
func setsFromResult(g scraper.GameResult) (won, lost int) {
	for _, s := range g.Sets {
		switch {
		case s.OurScore > s.OpponentScore:
			won++
		case s.OpponentScore > s.OurScore:
			lost++
		}
	}
	return won, lost
}

func formatSetScores(sets []scraper.SetScore) string {
	parts := make([]string, 0, len(sets))
	for _, s := range sets {
		parts = append(parts, strconv.Itoa(s.TeamOneScore)+":"+strconv.Itoa(s.TeamTwoScore))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}

func formatSetResults(sets []scraper.SetResult) string {
	parts := make([]string, 0, len(sets))
	for _, s := range sets {
		parts = append(parts, strconv.Itoa(s.OurScore)+":"+strconv.Itoa(s.OpponentScore))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}

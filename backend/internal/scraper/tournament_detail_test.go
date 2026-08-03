package scraper_test

import (
	"os"
	"testing"

	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

const (
	grtTournamentFixture    = "../../../scraper/fixtures/tournament-detail.html"
	nonGRTTournamentFixture = "../../../scraper/fixtures/tournament-detail-non-grt.html"
)

func TestParseTournamentDetail_GRT(t *testing.T) {
	f, err := os.Open(grtTournamentFixture)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	defer f.Close()

	detail, err := scraper.ParseTournamentDetail(f)
	if err != nil {
		t.Fatalf("ParseTournamentDetail: %v", err)
	}

	if detail.Name != "2. Leguan Cup" {
		t.Errorf("name: got %q, want %q", detail.Name, "2. Leguan Cup")
	}
	if !detail.IsGRT {
		t.Error("IsGRT = false, want true for a German Roundnet Tour tournament")
	}
	if detail.Date != "2026-07-18" {
		t.Errorf("date: got %q, want %q (first day of a two-day tournament)", detail.Date, "2026-07-18")
	}
}

func TestParseTournamentDetail_NonGRT(t *testing.T) {
	f, err := os.Open(nonGRTTournamentFixture)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	defer f.Close()

	detail, err := scraper.ParseTournamentDetail(f)
	if err != nil {
		t.Fatalf("ParseTournamentDetail: %v", err)
	}

	if detail.Name != "ETS Championship Basel 2026" {
		t.Errorf("name: got %q, want %q", detail.Name, "ETS Championship Basel 2026")
	}
	if detail.IsGRT {
		t.Error("IsGRT = true, want false for a non-GRT (EURA-rated) tournament")
	}
	if detail.Date != "2026-08-08" {
		t.Errorf("date: got %q, want %q (first day of a two-day tournament)", detail.Date, "2026-08-08")
	}
}

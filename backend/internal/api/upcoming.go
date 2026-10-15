package api

import (
	"log"
	"net/http"
	"sort"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

const upcomingTournamentsCacheKey = "upcoming_tournaments"

// UpcomingTournaments handles GET /api/tournaments/upcoming — tournaments that haven't
// started yet, soonest first, for the list on the start page.
func (h *Handler) UpcomingTournaments(w http.ResponseWriter, r *http.Request) {
	// Held across the fetch so concurrent cache misses share one playerzone request instead
	// of each queueing its own on the rate limiter that RGX jobs also wait on.
	h.upcomingMu.Lock()
	defer h.upcomingMu.Unlock()

	if cached, ok := h.cache.Get(upcomingTournamentsCacheKey); ok {
		jsonOK(w, cached)
		return
	}
	if !h.scrapePermitted() {
		log.Printf("upcoming tournaments: scraping not permitted, returning empty list")
		jsonOK(w, []scraper.TournamentListEntry{})
		return
	}

	entries, err := h.fetchUpcomingTournaments()
	if err != nil {
		log.Printf("upcoming tournaments: %v", err)
		jsonError(w, http.StatusBadGateway, "upcoming tournaments unavailable")
		return
	}
	h.cache.Set(upcomingTournamentsCacheKey, entries, cache.TTLUpcomingTournaments)
	jsonOK(w, entries)
}

func (h *Handler) fetchUpcomingTournaments() ([]scraper.TournamentListEntry, error) {
	body, err := h.fetcher.Fetch(scraper.TournamentsUpcomingPath)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	all, err := scraper.ParseTournamentList(body)
	if err != nil {
		return nil, err
	}

	// Entries without an end date come from the navigation dropdown, which lists tournaments
	// that are already running — not upcoming, and without a location to show.
	today := h.Now().In(berlin).Format("2006-01-02")
	upcoming := []scraper.TournamentListEntry{}
	for _, e := range all {
		if e.EndDate != "" && e.EndDate >= today {
			upcoming = append(upcoming, e)
		}
	}
	sort.SliceStable(upcoming, func(i, j int) bool { return upcoming[i].StartDate < upcoming[j].StartDate })
	return upcoming, nil
}

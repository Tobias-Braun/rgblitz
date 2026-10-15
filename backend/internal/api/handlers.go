package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tobiasmarkusbraun/rgblitz/internal/cache"
	"github.com/tobiasmarkusbraun/rgblitz/internal/config"
	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/jobqueue"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

// resultFreshWindow is how long a successfully finished job's result is
// served immediately without triggering a new scrape (see TodayResult).
const resultFreshWindow = 15 * time.Minute

var berlin *time.Location

func init() {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		log.Fatalf("cannot load Europe/Berlin timezone: %v", err)
	}
	berlin = loc
}

// Handler holds dependencies for all HTTP handlers.
type Handler struct {
	cfg         *config.Config
	players     *db.PlayerStore
	cache       *cache.Cache
	fetcher     scraper.Fetcher
	permissions *db.PermissionStore
	jobs        *db.JobStore
	stats       *db.JobStatsStore
	queue       *jobqueue.Queue
	feedback    *db.FeedbackStore
	notifier    FeedbackNotifier
	upcomingMu  sync.Mutex

	// Now returns the current time; defaults to time.Now. Overridable in tests so the
	// weekday/time-window guard in TodayResult can be exercised deterministically.
	Now func() time.Time
}

// FeedbackNotifier sends a best-effort notification for a newly created
// Feedback row. Implemented by *email.ResendClient in production; tests can
// substitute a fake instead of hitting the real Resend API.
type FeedbackNotifier interface {
	SendFeedbackNotification(fb *db.Feedback) error
}

func NewHandler(cfg *config.Config, ps *db.PlayerStore, c *cache.Cache, fetcher scraper.Fetcher, permissions *db.PermissionStore, jobs *db.JobStore, stats *db.JobStatsStore, queue *jobqueue.Queue, feedback *db.FeedbackStore, notifier FeedbackNotifier) *Handler {
	return &Handler{cfg: cfg, players: ps, cache: c, fetcher: fetcher, permissions: permissions, jobs: jobs, stats: stats, queue: queue, feedback: feedback, notifier: notifier, Now: time.Now}
}

// scrapePermitted reports whether live playerzone requests may be made.
func (h *Handler) scrapePermitted() bool {
	return h.permissions.Permitted(h.cfg.RespectPermission)
}

// --- /api/players/search ---

type playerSearchResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ClubName    string `json:"club_name"`
	AvatarURL   string `json:"avatar_url"`
	ClubLogoURL string `json:"club_logo_url"`
	RGX         int    `json:"rgx"`
}

func (h *Handler) SearchPlayers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		jsonError(w, http.StatusBadRequest, "q must be at least 2 characters")
		return
	}

	players, err := h.players.Search(q)
	if err != nil {
		log.Printf("search error: %v", err)
		jsonError(w, http.StatusInternalServerError, "search failed")
		return
	}

	results := make([]playerSearchResult, 0, len(players))
	for _, p := range players {
		results = append(results, playerSearchResult{
			ID:          p.ID,
			Name:        p.Name,
			ClubName:    p.ClubName,
			AvatarURL:   h.proxyURL(p.AvatarURL),
			ClubLogoURL: h.proxyURL(p.ClubLogoURL),
			RGX:         p.RGX,
		})
	}

	if h.cfg.DevMode {
		results = append(results, matchingDevSpecialPlayers(q)...)
	}

	jsonOK(w, results)
}

// devSpecialPlayer is a hardcoded, non-persisted player injected into search results
// and today-lookups only when DEV_MODE is enabled, letting frontend work exercise the
// ok/stale/error/loading ResultView states on demand without needing real playerzone
// data to line up. None of these ever reach the players.db.
type devSpecialPlayer struct {
	id       string
	name     string
	clubName string
	rgx      int
}

const devLoadingDelay = 10 * time.Second

var devSpecialPlayers = []devSpecialPlayer{
	{id: "dev-ok", name: "OK", clubName: "Dev Fixtures", rgx: 1400},
	{id: "dev-stale", name: "STALE", clubName: "Dev Fixtures", rgx: 1400},
	{id: "dev-error", name: "ERROR", clubName: "Dev Fixtures", rgx: 1400},
	{id: "dev-loading", name: "LOADING", clubName: "Dev Fixtures", rgx: 1400},
	{id: "dev-league", name: "LEAGUE", clubName: "Dev Fixtures", rgx: 1400},
}

// matchingDevSpecialPlayers returns the fixture players whose name (ok/stale/error/loading/league)
// occurs in q, case-insensitively.
func matchingDevSpecialPlayers(q string) []playerSearchResult {
	lower := strings.ToLower(q)
	var out []playerSearchResult
	for _, p := range devSpecialPlayers {
		if strings.Contains(lower, strings.ToLower(p.name)) {
			out = append(out, playerSearchResult{
				ID:       p.id,
				Name:     p.name,
				ClubName: p.clubName,
				RGX:      p.rgx,
			})
		}
	}
	return out
}

func findDevSpecialPlayer(id string) (devSpecialPlayer, bool) {
	for _, p := range devSpecialPlayers {
		if p.id == id {
			return p, true
		}
	}
	return devSpecialPlayer{}, false
}

// --- /api/players/{id}/today ---

// TodayResponse, PlayerInfo and GameResult are aliases for the scraper
// package's types so ScrapePlayerToday can run outside an HTTP request (e.g.
// from the background job worker) while API handlers keep their existing
// names and JSON tags.
type TodayResponse = scraper.TodayResult
type PlayerInfo = scraper.PlayerInfo
type GameResult = scraper.GameResult

// JobResponse is returned by both GET /api/players/{id}/today (get-or-create
// a job) and GET /api/players/{id}/today/jobs/{jobId} (poll a job's status).
type JobResponse struct {
	JobID         int64          `json:"job_id,omitempty"`
	Status        string         `json:"status"` // no_tournament | queued | running | done | error
	QueuePosition *int           `json:"queue_position,omitempty"`
	Result        *TodayResponse `json:"result,omitempty"`
	Error         string         `json:"error,omitempty"`
	FinishedAt    *time.Time     `json:"finished_at,omitempty"`
}

func (h *Handler) TodayResult(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "id")

	if h.cfg.DevMode {
		if sp, ok := findDevSpecialPlayer(playerID); ok {
			jsonOK(w, h.devSpecialJobResponse(sp, h.Now().In(berlin)))
			return
		}
	}

	player, err := h.players.GetByID(playerID)
	if err != nil {
		log.Printf("db get player %q: %v", playerID, err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if player == nil {
		jsonError(w, http.StatusNotFound, "player not found")
		return
	}

	pInfo := &PlayerInfo{
		ID:          player.ID,
		Name:        player.Name,
		ClubName:    player.ClubName,
		AvatarURL:   h.proxyURL(player.AvatarURL),
		ClubLogoURL: h.proxyURL(player.ClubLogoURL),
		RGX:         player.RGX,
	}

	now := h.Now().In(berlin)

	// In dev mode, bypass the weekday/time guard, the job queue and the scrape
	// pipeline entirely, returning synthetic tournament data immediately.
	if h.cfg.DevMode {
		resp := h.buildDevResponse(player, now)
		jsonOK(w, JobResponse{Status: "done", Result: &resp})
		return
	}

	noTournament := JobResponse{Status: "no_tournament", Result: &TodayResponse{Status: "no_tournament", Player: pInfo}}

	// Weekday guard — tournaments only happen on weekends. Neither this nor the
	// hour guard below ever touches the job queue.
	if wd := now.Weekday(); wd != time.Saturday && wd != time.Sunday {
		jsonOK(w, noTournament)
		return
	}
	if now.Hour() < 8 {
		jsonOK(w, noTournament)
		return
	}

	dateStr := now.Format("2006-01-02")
	job, err := h.jobs.LatestForPlayerDate(playerID, dateStr)
	if err != nil {
		log.Printf("load latest job for player %s: %v", playerID, err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if job != nil {
		switch job.Status {
		case db.JobStatusQueued, db.JobStatusRunning:
			// Already in flight — return its current status rather than enqueueing a duplicate.
			jsonOK(w, h.jobResponse(job))
			return
		case db.JobStatusDone:
			if job.FinishedAt.Valid && now.Sub(job.FinishedAt.Time) < resultFreshWindow {
				jsonOK(w, h.jobResponse(job))
				return
			}
			// Older than the freshness window — fall through and enqueue a refresh.
		case db.JobStatusError:
			// Always allow retrying a failed scrape — fall through and enqueue a new job.
		}
	}

	if !h.scrapePermitted() {
		log.Printf("robots.txt permission check denies RGBlitzBot — skipping live scrape for player %s", playerID)
		jsonOK(w, noTournament)
		return
	}

	newJob, err := h.jobs.Create(playerID, dateStr, now)
	if err != nil {
		log.Printf("create job for player %s: %v", playerID, err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := h.stats.Create(newJob.ID, playerID, now); err != nil {
		// Non-fatal — usage stats are best-effort and must never block serving the job.
		log.Printf("record job stats for job %d: %v", newJob.ID, err)
	}
	h.queue.Enqueue(newJob.ID)
	jsonOK(w, h.jobResponse(newJob))
}

// JobStatus handles GET /api/players/{id}/today/jobs/{jobId} — a pure status
// lookup with no side effects, used by the frontend to poll a job started by
// TodayResult until it reaches a final (done/error) state.
func (h *Handler) JobStatus(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "id")
	jobID, err := strconv.ParseInt(chi.URLParam(r, "jobId"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	job, err := h.jobs.GetByID(jobID)
	if err != nil {
		log.Printf("load job %d: %v", jobID, err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if job == nil || job.PlayerID != playerID {
		jsonError(w, http.StatusNotFound, "job not found")
		return
	}

	jsonOK(w, h.jobResponse(job))
}

// --- /api/stats/rgx-jobs ---

// RgxJobStatsResponse is the all-time usage summary returned by GET /api/stats/rgx-jobs.
type RgxJobStatsResponse struct {
	Created   int `json:"created"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// RgxJobStats handles GET /api/stats/rgx-jobs — all-time counts of RgxJobs
// created/succeeded/failed, backing the "Happily served N RGX changes"
// header stat on the frontend.
func (h *Handler) RgxJobStats(w http.ResponseWriter, r *http.Request) {
	totals, err := h.stats.Totals()
	if err != nil {
		log.Printf("load job stats totals: %v", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	jsonOK(w, RgxJobStatsResponse{Created: totals.Created, Succeeded: totals.Succeeded, Failed: totals.Failed})
}

// jobResponse builds the JobResponse for a job, decoding its stored result
// and computing its queue position as needed.
func (h *Handler) jobResponse(job *db.Job) JobResponse {
	resp := JobResponse{JobID: job.ID, Status: job.Status}

	switch job.Status {
	case db.JobStatusQueued:
		ahead, err := h.jobs.CountQueuedBefore(job.QueuedAt, job.ID)
		if err != nil {
			log.Printf("count queue position for job %d: %v", job.ID, err)
			ahead = 0
		}
		position := ahead + 1
		resp.QueuePosition = &position
	case db.JobStatusDone:
		var result TodayResponse
		if err := json.Unmarshal([]byte(job.ResultJSON), &result); err != nil {
			log.Printf("decode result for job %d: %v", job.ID, err)
		} else {
			resp.Result = &result
		}
		if job.FinishedAt.Valid {
			t := job.FinishedAt.Time
			resp.FinishedAt = &t
		}
	case db.JobStatusError:
		resp.Error = job.Error
		if job.FinishedAt.Valid {
			t := job.FinishedAt.Time
			resp.FinishedAt = &t
		}
	}

	return resp
}

// scrapeDeps bundles the dependencies scraper.ScrapePlayerToday needs, shared
// with the background job worker (internal/jobqueue) which cannot construct
// a Handler.
func (h *Handler) scrapeDeps() scraper.ScrapeDeps {
	return scraper.ScrapeDeps{Fetcher: h.fetcher, Cache: h.cache, ProxyURL: h.proxyURL}
}

// devSpecialJobResponse creates (or reuses/refreshes, via the same dedup and
// 15-min freshness rules as real players) an RgxJob for one of the magic
// dev-mode search results (dev-ok/dev-stale/dev-error/dev-loading/dev-league), entirely
// independent of the DB player table and the real scrape pipeline so it
// works even though the player was never indexed. dev-ok/dev-stale/dev-error/dev-league
// resolve the job synchronously before responding ("immediately completed");
// dev-loading resolves it in the background after devLoadingDelay, so the
// frontend exercises the real queued/running -> done polling flow.
func (h *Handler) devSpecialJobResponse(sp devSpecialPlayer, now time.Time) JobResponse {
	dateStr := now.Format("2006-01-02")

	job, err := h.jobs.LatestForPlayerDate(sp.id, dateStr)
	if err != nil {
		log.Printf("load latest dev job for %s: %v", sp.id, err)
	}
	if job != nil {
		switch job.Status {
		case db.JobStatusQueued, db.JobStatusRunning:
			return h.jobResponse(job)
		case db.JobStatusDone:
			if job.FinishedAt.Valid && now.Sub(job.FinishedAt.Time) < resultFreshWindow {
				return h.jobResponse(job)
			}
			// Older than the freshness window — fall through and create a new job.
		case db.JobStatusError:
			// Always allow retrying — fall through and create a new job.
		}
	}

	newJob, err := h.jobs.Create(sp.id, dateStr, now)
	if err != nil {
		log.Printf("create dev job for %s: %v", sp.id, err)
		return JobResponse{Status: "error", Error: "internal error"}
	}

	if sp.id == "dev-loading" {
		go h.resolveDevSpecialJob(sp, newJob.ID, now, devLoadingDelay)
		return h.jobResponse(newJob)
	}

	h.resolveDevSpecialJob(sp, newJob.ID, now, 0)
	resolved, err := h.jobs.GetByID(newJob.ID)
	if err != nil || resolved == nil {
		log.Printf("reload resolved dev job %d for %s: %v", newJob.ID, sp.id, err)
		return JobResponse{Status: "error", Error: "internal error"}
	}
	return h.jobResponse(resolved)
}

// resolveDevSpecialJob marks jobID running (if delay > 0), waits delay, then marks it
// done with the synthetic fixture result or, for dev-error, marks it error. Called
// synchronously (delay 0) for the immediately-completed fixtures, or in a goroutine
// (delay devLoadingDelay) for dev-loading.
func (h *Handler) resolveDevSpecialJob(sp devSpecialPlayer, jobID int64, now time.Time, delay time.Duration) {
	if delay > 0 {
		if err := h.jobs.MarkRunning(jobID, time.Now()); err != nil {
			log.Printf("mark dev job %d running: %v", jobID, err)
		}
		time.Sleep(delay)
	}

	if sp.id == "dev-error" {
		if err := h.jobs.MarkError(jobID, "upstream unavailable", time.Now()); err != nil {
			log.Printf("mark dev job %d error: %v", jobID, err)
		}
		return
	}

	resp := h.buildDevSpecialResult(sp, now)
	resultJSON, err := json.Marshal(resp)
	if err != nil {
		log.Printf("encode dev job %d result: %v", jobID, err)
		if err := h.jobs.MarkError(jobID, "encode result failed", time.Now()); err != nil {
			log.Printf("mark dev job %d error: %v", jobID, err)
		}
		return
	}
	if err := h.jobs.MarkDone(jobID, string(resultJSON), time.Now()); err != nil {
		log.Printf("mark dev job %d done: %v", jobID, err)
	}
}

// buildDevSpecialResult renders the hardcoded ok/stale games fixture shared by
// dev-ok/dev-stale/dev-loading, or a league matchday's Partien for dev-league.
func (h *Handler) buildDevSpecialResult(sp devSpecialPlayer, now time.Time) TodayResponse {
	pInfo := &PlayerInfo{
		ID:       sp.id,
		Name:     sp.name,
		ClubName: sp.clubName,
		RGX:      sp.rgx,
	}

	const ourTeamRGX = 2800
	games := []GameResult{
		computeDevGameResult("Gr. A #1", "Team Nord", "Dev Fixtures Team", ourTeamRGX, 2786, 2, 0),
		computeDevGameResult("Gr. A #2", "Team Süd", "Dev Fixtures Team", ourTeamRGX, 2818, 0, 2),
		computeDevGameResult("HF #1", "Team West", "Dev Fixtures Team", ourTeamRGX, 2778, 2, 1),
	}
	competition := ""
	if sp.id == "dev-league" {
		// One Partie per Begegnung, each with a different partner, as a league player plays them.
		competition = scraper.CompetitionLeague
		games = []GameResult{
			computeDevGameResult("Open 1 · 1. Matchday #1", "Nord/Süd (Team Nord)", "League/Partner", 2900, 3100, 2, 1),
			computeDevGameResult("Mixed 1 · 1. Matchday #2", "West/Ost (Team West)", "League/Partnerin", 2700, 2650, 0, 2),
		}
	}
	totalDelta := 0
	totalPerformanceDelta := 0
	for _, g := range games {
		totalDelta += g.RGXDelta
		totalPerformanceDelta += g.PerformanceDelta
	}

	return TodayResponse{
		Status:                "ok",
		Competition:           competition,
		Stale:                 sp.id == "dev-stale",
		Player:                pInfo,
		Date:                  now.Format("2006-01-02"),
		TotalDelta:            totalDelta,
		TotalPerformanceDelta: totalPerformanceDelta,
		Games:                 games,
	}
}

// buildDevResponse generates synthetic tournament data for DEV_MODE using real players from the DB.
func (h *Handler) buildDevResponse(player *db.Player, now time.Time) TodayResponse {
	pInfo := &PlayerInfo{
		ID:          player.ID,
		Name:        player.Name,
		ClubName:    player.ClubName,
		AvatarURL:   h.proxyURL(player.AvatarURL),
		ClubLogoURL: h.proxyURL(player.ClubLogoURL),
		RGX:         player.RGX,
	}

	opponents, err := h.players.RandomPlayers(player.ID, 8)
	if err != nil || len(opponents) < 2 {
		return TodayResponse{Status: "no_tournament", Player: pInfo}
	}

	partnerList, _ := h.players.RandomPlayers(player.ID, 1)
	partnerRGX := 1200
	partnerName := "Partner"
	if len(partnerList) > 0 {
		partnerRGX = partnerList[0].RGX
		partnerName = partnerList[0].Name
	}
	r1 := player.RGX + partnerRGX
	ourTeamName := player.Name + " / " + partnerName

	rng := rand.New(rand.NewSource(now.UnixNano()))
	rounds := []string{"Gr. A #1", "Gr. A #2", "Gr. A #3", "HF #1", "Finale"}

	numGames := 3 + rng.Intn(3)
	maxGames := len(opponents) / 2
	if numGames > maxGames {
		numGames = maxGames
	}
	if numGames < 1 {
		numGames = 1
	}

	var games []GameResult
	totalDelta := 0
	totalPerformanceDelta := 0

	for i := 0; i < numGames; i++ {
		team := opponents[i*2 : i*2+2]
		r2 := team[0].RGX + team[1].RGX

		setsWon, setsLost := randomOutcome(rng)
		card := scraper.GameCard{
			Round:        rounds[i%len(rounds)],
			OpponentName: team[0].Name + " / " + team[1].Name,
			WeAreTeamOne: true,
			Outcome:      outcomeWord(setsWon, setsLost),
		}

		breakdown := scraper.ComputeDeltaBreakdown(r1, r2, setsWon, setsLost)
		result := resultLetter(setsWon, setsLost)
		totalDelta += breakdown.Delta
		totalPerformanceDelta += breakdown.PerformanceDelta
		games = append(games, GameResult{
			Sets:                      fakeSetScores(setsWon, setsLost, rng),
			Round:                     card.Round,
			OpponentName:              card.OpponentName,
			OurTeamName:               ourTeamName,
			TeamRGX:                   r1,
			OpponentRGX:               r2,
			Result:                    result,
			RGXDelta:                  breakdown.Delta,
			RGXDeltaPerPlayer:         0.5 * float32(breakdown.Delta),
			PerformanceDelta:          breakdown.PerformanceDelta,
			PerformanceDeltaPerPlayer: 0.5 * float32(breakdown.PerformanceDelta),
		})
	}

	return TodayResponse{
		Status:                "ok",
		Player:                pInfo,
		Date:                  now.Format("2006-01-02"),
		TotalDelta:            totalDelta,
		TotalPerformanceDelta: totalPerformanceDelta,
		Games:                 games,
	}
}

func randomOutcome(rng *rand.Rand) (won, lost int) {
	choices := [][2]int{{2, 0}, {2, 1}, {1, 1}, {1, 2}, {0, 2}}
	c := choices[rng.Intn(len(choices))]
	return c[0], c[1]
}

func fakeSetScores(won, lost int, rng *rand.Rand) []scraper.SetResult {
	sets := make([]scraper.SetResult, 0, won+lost)
	for i := 0; i < won; i++ {
		sets = append(sets, scraper.SetResult{OurScore: 21, OpponentScore: 15 + rng.Intn(5)})
	}
	for i := 0; i < lost; i++ {
		sets = append(sets, scraper.SetResult{OurScore: 15 + rng.Intn(5), OpponentScore: 21})
	}
	return sets
}

func outcomeWord(won, lost int) string {
	if won > lost {
		return "win"
	}
	if lost > won {
		return "loss"
	}
	return "draw"
}

func resultLetter(won, lost int) string {
	if won > lost {
		return "W"
	}
	if lost > won {
		return "L"
	}
	return "D"
}

// --- /api/images/proxy ---

func (h *Handler) ProxyImage(w http.ResponseWriter, r *http.Request) {
	rawParam := r.URL.Query().Get("url")
	if rawParam == "" {
		jsonError(w, http.StatusBadRequest, "url parameter required")
		return
	}

	decoded, err := base64.URLEncoding.DecodeString(rawParam)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(rawParam)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "url must be base64-encoded")
			return
		}
	}

	originalURL := string(decoded)
	if !isAllowedImageOrigin(originalURL) {
		jsonError(w, http.StatusBadRequest, "disallowed image origin")
		return
	}

	cacheFile := h.imageCachePath(originalURL)
	if info, err := os.Stat(cacheFile); err == nil {
		if time.Since(info.ModTime()) < cache.TTLImageDisk {
			serveFile(w, cacheFile)
			return
		}
	}

	resp, err := imageProxyClient.Get(originalURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		jsonError(w, http.StatusBadGateway, "upstream image fetch failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		jsonError(w, http.StatusBadGateway, "read upstream image failed")
		return
	}

	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err == nil {
		_ = os.WriteFile(cacheFile, body, 0o644)
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=604800")
	_, _ = w.Write(body)
}

func (h *Handler) imageCachePath(originalURL string) string {
	safe := strings.NewReplacer("://", "_", "/", "_", "?", "_", "&", "_", "=", "_").Replace(originalURL)
	if len(safe) > 200 {
		safe = safe[:200]
	}
	return filepath.Join(h.cfg.ImageCacheDir, safe)
}

// imageProxyClient bounds every outbound image fetch so a slow or
// non-responding upstream can't tie up a handler goroutine indefinitely.
var imageProxyClient = &http.Client{Timeout: 10 * time.Second}

// isAllowedImageOrigin restricts the proxy to the two known image hosts.
// It requires an exact host match or a proper subdomain (".host" suffix) —
// a plain strings.HasSuffix(host, "pstaassets.blob.core.windows.net") would
// also match an attacker-registered Azure storage account such as
// "evilpstaassets.blob.core.windows.net", since Azure blob container names
// are self-service and HasSuffix doesn't require a label boundary.
func isAllowedImageOrigin(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	for _, host := range []string{"pstaassets.blob.core.windows.net", "playerzone.roundnetgermany.de"} {
		if u.Host == host || strings.HasSuffix(u.Host, "."+host) {
			return true
		}
	}
	return false
}

func serveFile(w http.ResponseWriter, path string) {
	f, err := os.Open(path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Cache-Control", "public, max-age=604800")
	_, _ = io.Copy(w, f)
}

// --- helpers ---

func (h *Handler) proxyURL(original string) string {
	return ProxyURL(original)
}

// ProxyURL rewrites a playerzone/CDN image URL into a same-origin
// /api/images/proxy URL. Exported so the background job worker
// (internal/jobqueue) can build the same URLs as the HTTP handler without
// needing a *Handler.
func ProxyURL(original string) string {
	if original == "" {
		return ""
	}
	encoded := base64.URLEncoding.EncodeToString([]byte(original))
	return "/api/images/proxy?url=" + encoded
}

// --- /api/feedback ---

type FeedbackRequest struct {
	Rating int    `json:"rating"`
	Text   string `json:"text"`
}

type FeedbackResponse struct {
	Status string `json:"status"`
	ID     int64  `json:"id"`
}

func (h *Handler) SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	var req FeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		jsonError(w, http.StatusBadRequest, "rating must be between 1 and 5")
		return
	}
	text := strings.TrimSpace(req.Text)
	if len(text) > 4000 {
		jsonError(w, http.StatusBadRequest, "text too long")
		return
	}

	fb, err := h.feedback.Create(req.Rating, text, h.Now())
	if err != nil {
		log.Printf("create feedback: %v", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Best-effort: the feedback is already saved, so a notification failure
	// (missing config, Resend outage) must never surface to the user.
	go func() {
		if err := h.notifier.SendFeedbackNotification(fb); err != nil {
			log.Printf("send feedback notification: %v", err)
		}
	}()

	jsonOK(w, FeedbackResponse{Status: "ok", ID: fb.ID})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func computeDevGameResult(round, opponentName, ourTeamName string, teamRGX, opponentRGX, setsWon, setsLost int) GameResult {
	breakdown := scraper.ComputeDeltaBreakdown(teamRGX, opponentRGX, setsWon, setsLost)
	result := "D"
	if setsWon > setsLost {
		result = "W"
	} else if setsLost > setsWon {
		result = "L"
	}
	return GameResult{
		Sets:                      devSetScores(setsWon, setsLost),
		Round:                     round,
		OpponentName:              opponentName,
		OurTeamName:               ourTeamName,
		TeamRGX:                   teamRGX,
		OpponentRGX:               opponentRGX,
		Result:                    result,
		RGXDelta:                  breakdown.Delta,
		RGXDeltaPerPlayer:         0.5 * float32(breakdown.Delta),
		PerformanceDelta:          breakdown.PerformanceDelta,
		PerformanceDeltaPerPlayer: 0.5 * float32(breakdown.PerformanceDelta),
		HasProxy:                  false,
	}
}

// devSetScores synthesizes a plausible set-by-set score line for the dev fixture
// result. Unlike a live scrape, the dev fixture has no real per-set scores — only
// the won/lost count — so this fills in placeholder scores consistent with that
// count for GameRow.vue's set-list rendering.
func devSetScores(setsWon, setsLost int) []scraper.SetResult {
	sets := make([]scraper.SetResult, 0, setsWon+setsLost)
	for i := 0; i < setsWon; i++ {
		sets = append(sets, scraper.SetResult{OurScore: 21, OpponentScore: 15})
	}
	for i := 0; i < setsLost; i++ {
		sets = append(sets, scraper.SetResult{OurScore: 15, OpponentScore: 21})
	}
	return sets
}

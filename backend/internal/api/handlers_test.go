package api_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tobiasmarkusbraun/rgx-calc/internal/api"
	"github.com/tobiasmarkusbraun/rgx-calc/internal/cache"
	"github.com/tobiasmarkusbraun/rgx-calc/internal/config"
	"github.com/tobiasmarkusbraun/rgx-calc/internal/db"
	"github.com/tobiasmarkusbraun/rgx-calc/internal/jobqueue"
	"github.com/tobiasmarkusbraun/rgx-calc/internal/scraper"
)

const (
	fixtureDir     = "../../../scraper/fixtures"
	rankingFixture = fixtureDir + "/ranking.html"
)

// fixtureDate matches the day the fixtures were captured on — the only day the
// Leguan Cup tournament in tournament-detail.html/division-teams.html/team-schedule.html
// is "active".
var fixtureDate = time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

// testPlayerName is a player from ranking.html whose first name ("Lucas") also appears in
// a team's player list in division-teams.html, so the pipeline's name-matching finds a team.
const (
	testPlayerID   = "LUC2110"
	testPlayerName = "Lucas Christiani"
)

// countingFetcher wraps another Fetcher and counts how many times Fetch is called, so tests
// can assert the cache is actually being hit (fetch count stays flat on a second request).
type countingFetcher struct {
	inner scraper.Fetcher
	calls int
}

func (f *countingFetcher) Fetch(path string) (io.ReadCloser, error) {
	f.calls++
	return f.inner.Fetch(path)
}

// noNetworkFetcher fails the test if it is ever called — used to prove the weekday/time-window
// guard in TodayResult short-circuits before touching the network.
type noNetworkFetcher struct {
	t *testing.T
}

func (f noNetworkFetcher) Fetch(path string) (io.ReadCloser, error) {
	f.t.Fatalf("fetcher must not be called, but Fetch(%q) was", path)
	return nil, nil
}

// newTestHandler builds a Handler backed by a temp SQLite DB seeded from the real ranking.html
// fixture, plus whatever fetcher/config the test needs. The returned job queue's single
// worker is already started, so a "queued" job transitions to "done"/"error" in the
// background exactly as it would in production — tests that need the final result must
// poll pollJobUntilFinal rather than expecting a synchronous response.
func newTestHandler(t *testing.T, fetcher scraper.Fetcher, cfg *config.Config) (*api.Handler, *db.PlayerStore, *db.PermissionStore, *db.JobStore) {
	t.Helper()

	f, err := os.Open(rankingFixture)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	defer f.Close()

	players, err := scraper.ParseRankingPage(f)
	if err != nil {
		t.Fatalf("parse ranking fixture: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	store := db.NewPlayerStore(conn)
	for _, p := range players {
		if err := store.Upsert(p); err != nil {
			t.Fatalf("upsert player %s: %v", p.ID, err)
		}
	}

	if cfg == nil {
		cfg = &config.Config{}
	}
	permissions := db.NewPermissionStore(conn)
	jobs := db.NewJobStore(conn)

	c := cache.New()
	scrapeDeps := scraper.ScrapeDeps{Fetcher: fetcher, Cache: c, ProxyURL: api.ProxyURL}
	queue := jobqueue.New(jobs, store, scrapeDeps, permissions, cfg.RespectPermission, time.Now)
	queue.Start()

	h := api.NewHandler(cfg, store, c, fetcher, permissions, jobs, queue)
	return h, store, permissions, jobs
}

// pollJobUntilFinal polls GET /api/players/{id}/today/jobs/{jobId} until the job reaches
// a final status (done/error), matching how the frontend polls, or fails the test after a
// generous timeout — the queue worker runs on a real goroutine/clock even in tests.
func pollJobUntilFinal(t *testing.T, router http.Handler, playerID string, jobID int64) api.JobResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/players/%s/today/jobs/%d", playerID, jobID), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("poll job status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var resp api.JobResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode job response: %v", err)
		}
		if resp.Status == "done" || resp.Status == "error" {
			return resp
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %d for player %s did not reach a final status within timeout", jobID, playerID)
	return api.JobResponse{}
}

func TestSearchPlayers_HappyPath(t *testing.T) {
	h, _, _, _ := newTestHandler(t, scraper.NewFixtureFetcher(fixtureDir), nil)
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/players/search?q=Lucas", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var results []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one search result for \"Lucas\"")
	}
	found := false
	for _, r := range results {
		if r["id"] == testPlayerID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %s in search results, got %+v", testPlayerID, results)
	}
}

func TestSearchPlayers_QueryTooShort(t *testing.T) {
	h, _, _, _ := newTestHandler(t, scraper.NewFixtureFetcher(fixtureDir), nil)
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/players/search?q=a", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestTodayResult_DevMode(t *testing.T) {
	cfg := &config.Config{DevMode: true}
	h, _, _, _ := newTestHandler(t, noNetworkFetcher{t}, cfg)
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/players/"+testPlayerID+"/today", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp api.JobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "done" {
		t.Fatalf("status = %q, want %q (dev mode should always synthesize a result immediately)", resp.Status, "done")
	}
	if resp.Result == nil || resp.Result.Status != "ok" {
		t.Errorf("result = %+v, want status %q", resp.Result, "ok")
	}
}

func TestTodayResult_DevSpecialOk_CreatesImmediatelyDoneJob(t *testing.T) {
	cfg := &config.Config{DevMode: true}
	h, _, _, jobs := newTestHandler(t, noNetworkFetcher{t}, cfg)
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/players/dev-ok/today", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp api.JobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "done" || resp.JobID == 0 {
		t.Fatalf("resp = %+v, want a done job with a job_id (dev-ok must resolve immediately)", resp)
	}
	if resp.Result == nil || resp.Result.Status != "ok" {
		t.Errorf("result = %+v, want status %q", resp.Result, "ok")
	}

	// A real RgxJob row must exist — this isn't just a synthesized response.
	job, err := jobs.GetByID(resp.JobID)
	if err != nil || job == nil {
		t.Fatalf("GetByID(%d) = %v, %v — want a persisted job", resp.JobID, job, err)
	}
	if job.PlayerID != "dev-ok" || job.Status != "done" {
		t.Errorf("job = %+v, want player_id=dev-ok status=done", job)
	}
}

func TestTodayResult_DevSpecialLoading_ResolvesInBackground(t *testing.T) {
	cfg := &config.Config{DevMode: true}
	h, _, _, jobs := newTestHandler(t, noNetworkFetcher{t}, cfg)
	router := api.NewRouter(h)

	start := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/api/players/dev-loading/today", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if elapsed > time.Second {
		t.Fatalf("request took %s — dev-loading must respond immediately, resolving in the background", elapsed)
	}

	var resp api.JobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "queued" && resp.Status != "running" {
		t.Fatalf("status = %q, want %q or %q immediately after creation", resp.Status, "queued", "running")
	}
	if resp.JobID == 0 {
		t.Fatal("expected a job_id for a created dev-loading job")
	}

	// Still not final well before the 10s devLoadingDelay elapses.
	job, err := jobs.GetByID(resp.JobID)
	if err != nil || job == nil {
		t.Fatalf("GetByID(%d) = %v, %v", resp.JobID, job, err)
	}
	if job.Status == db.JobStatusDone || job.Status == db.JobStatusError {
		t.Errorf("job status = %q, want still in-flight shortly after creation", job.Status)
	}

	// devLoadingDelay is a real 10s sleep, longer than pollJobUntilFinal's 5s
	// deadline, so poll inline with a longer timeout instead of reusing it.
	deadline := time.Now().Add(15 * time.Second)
	var final api.JobResponse
	for time.Now().Before(deadline) {
		pollReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/players/dev-loading/today/jobs/%d", resp.JobID), nil)
		pollRec := httptest.NewRecorder()
		router.ServeHTTP(pollRec, pollReq)
		if err := json.Unmarshal(pollRec.Body.Bytes(), &final); err != nil {
			t.Fatalf("decode poll response: %v", err)
		}
		if final.Status == "done" || final.Status == "error" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if final.Status != "done" || final.Result == nil || final.Result.Status != "ok" {
		t.Fatalf("final = %+v, want a done result", final)
	}
}

func TestTodayResult_WeekdayGuard_NoNetwork(t *testing.T) {
	h, _, _, _ := newTestHandler(t, noNetworkFetcher{t}, nil)
	h.Now = func() time.Time { return time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC) } // Wednesday
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/players/"+testPlayerID+"/today", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp api.JobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "no_tournament" {
		t.Errorf("status = %q, want %q on a weekday", resp.Status, "no_tournament")
	}
}

func TestTodayResult_WeekendBeforeWindow_NoNetwork(t *testing.T) {
	h, _, _, _ := newTestHandler(t, noNetworkFetcher{t}, nil)
	h.Now = func() time.Time { return time.Date(2026, 7, 19, 5, 0, 0, 0, time.UTC) } // Sunday 07:00 Europe/Berlin (CEST), before the 08:00 window
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/players/"+testPlayerID+"/today", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp api.JobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "no_tournament" {
		t.Errorf("status = %q, want %q before the 08:00 window", resp.Status, "no_tournament")
	}
}

func TestTodayResult_RespectPermission_BlocksWhenDisallowed(t *testing.T) {
	cfg := &config.Config{RespectPermission: true}
	h, _, permissions, _ := newTestHandler(t, noNetworkFetcher{t}, cfg)
	if err := permissions.Set(false, time.Now()); err != nil {
		t.Fatalf("seed permission: %v", err)
	}
	h.Now = func() time.Time { return fixtureDate } // a Sunday within the tournament window
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/players/"+testPlayerID+"/today", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp api.JobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "no_tournament" {
		t.Errorf("status = %q, want %q when robots.txt permission is stored as denied", resp.Status, "no_tournament")
	}
}

func TestTodayResult_RespectPermission_NoStoredCheckBlocks(t *testing.T) {
	cfg := &config.Config{RespectPermission: true}
	h, _, _, _ := newTestHandler(t, noNetworkFetcher{t}, cfg)
	h.Now = func() time.Time { return fixtureDate }
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/players/"+testPlayerID+"/today", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp api.JobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "no_tournament" {
		t.Errorf("status = %q, want %q — must fail closed when no permission check has ever run", resp.Status, "no_tournament")
	}
}

func TestTodayResult_LiveScrapeAndCache(t *testing.T) {
	fetcher := &countingFetcher{inner: scraper.NewFixtureFetcher(fixtureDir)}
	h, _, _, _ := newTestHandler(t, fetcher, nil)
	h.Now = func() time.Time { return fixtureDate }
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/players/"+testPlayerID+"/today", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var jobResp api.JobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &jobResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if jobResp.Status != "queued" || jobResp.JobID == 0 {
		t.Fatalf("first request status = %+v, want a queued job", jobResp)
	}

	final := pollJobUntilFinal(t, router, testPlayerID, jobResp.JobID)
	if final.Status != "done" || final.Result == nil {
		t.Fatalf("final job status = %+v, want done with a result", final)
	}
	resp := *final.Result
	if resp.Status != "ok" {
		t.Fatalf("status = %q, want %q; body = %+v", resp.Status, "ok", resp)
	}

	// Cross-check the game count against an independent parse of the same fixture.
	scheduleFile, err := os.Open(fixtureDir + "/team-schedule.html")
	if err != nil {
		t.Fatalf("open team schedule fixture: %v", err)
	}
	defer scheduleFile.Close()
	schedule, err := scraper.ParseTeamSchedule(scheduleFile, 0) // team ID doesn't affect game count
	if err != nil {
		t.Fatalf("parse team schedule fixture: %v", err)
	}
	if len(resp.Games) != len(schedule.Games) {
		t.Errorf("games = %d, want %d (from independent fixture parse)", len(resp.Games), len(schedule.Games))
	}

	callsAfterFirst := fetcher.calls
	if callsAfterFirst == 0 {
		t.Fatal("expected the fetcher to be called at least once on a cache miss")
	}

	// Second request within the freshness window must return the cached done job
	// immediately — no new job, no additional fetches.
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second request status = %d, body = %s", rec2.Code, rec2.Body.String())
	}
	var jobResp2 api.JobResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &jobResp2); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if jobResp2.Status != "done" {
		t.Fatalf("second request status = %q, want %q (cached, within freshness window)", jobResp2.Status, "done")
	}
	if fetcher.calls != callsAfterFirst {
		t.Errorf("fetch count changed on cached request: %d -> %d", callsAfterFirst, fetcher.calls)
	}
}

func TestTodayResult_UnknownPlayer(t *testing.T) {
	h, _, _, _ := newTestHandler(t, scraper.NewFixtureFetcher(fixtureDir), nil)
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/players/DOES-NOT-EXIST/today", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestProxyImage_MissingURL(t *testing.T) {
	h, _, _, _ := newTestHandler(t, scraper.NewFixtureFetcher(fixtureDir), nil)
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/images/proxy", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestProxyImage_DisallowedOrigin(t *testing.T) {
	h, _, _, _ := newTestHandler(t, scraper.NewFixtureFetcher(fixtureDir), nil)
	router := api.NewRouter(h)

	encoded := base64.URLEncoding.EncodeToString([]byte("https://evil.example.com/x.jpg"))
	req := httptest.NewRequest(http.MethodGet, "/api/images/proxy?url="+encoded, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

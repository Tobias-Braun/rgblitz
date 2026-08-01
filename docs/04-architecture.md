# rgx-calc — System Architecture

## High-Level Diagram

```
┌─────────────────────────────────────────┐
│           Browser (Vue / Nuxt)           │
│                                         │
│  SearchView  ──►  ResultView            │
│      │                │                 │
│  autocomplete     today's data          │
│      │                │                 │
└──────┼────────────────┼─────────────────┘
       │  HTTP          │  HTTP
       ▼                ▼
┌──────────────────────────────────────────┐
│             Go HTTP Server               │
│                                         │
│  /api/players/search                    │
│  /api/players/{id}/today (job create)   │
│  /api/players/{id}/today/jobs/{jobId}   │
│  /api/image (proxy)                     │
│                                         │
│  ┌──────────────┐  ┌──────────────────┐ │
│  │  Player DB   │  │  Result Cache    │ │
│  │  (SQLite /   │  │  (in-memory or   │ │
│  │   Postgres)  │  │   Redis)         │ │
│  └──────────────┘  └──────────────────┘ │
│                                         │
│  ┌──────────────────────────────────┐   │
│  │         Scraper Layer            │   │
│  │  - PlayerScraper (weekly cron)   │   │
│  │  - TournamentScraper (on-demand) │   │
│  │  - ImageProxy cache              │   │
│  └────────────────┬─────────────────┘   │
└───────────────────┼─────────────────────┘
                    │  HTTP (scraping)
                    ▼
        playerzone.roundngetgermany.de
```

---

## Component Breakdown

### Frontend — Vue 3 + Nuxt

| Component | Responsibility |
|-----------|---------------|
| `SearchBar.vue` | Animated search bar; manages focus state, shader toggle, autocomplete rendering |
| `AutocompleteItem.vue` | Single autocomplete row (avatar, name, club logo) |
| `ResultView.vue` | Orchestrates State 3; receives player + games data as props |
| `RgxCircle.vue` | Gradient glow circle with delta number; battery-aware animation |
| `MoodText.vue` | Selects and displays threshold-based phrase from config |
| `SpieleListe.vue` | Accordion wrapper + game rows |
| `GameRow.vue` | Single match row (opponent, W/L/D badge, delta) |
| `BatteryProvider.ts` | Composable: wraps `navigator.getBattery()`, exposes `isLowBattery` ref |
| `mood-phrases.ts` | Static config file: threshold brackets → German phrase strings |

**State management**: Simple `ref`/`reactive` composables — no Pinia/Vuex needed given the stateless nature.

**Routing**: Single route `/` — all view transitions are CSS/JS state changes within the page, not router navigations.

**SSR**: Nuxt SSR is enabled. The initial page load is server-rendered; the Go API is called from the Nuxt server on first render, not from the browser. After hydration, subsequent calls (e.g. re-polling during a tournament) are made directly from the browser to the Go API.

---

### Backend — Go

#### Package structure

```
cmd/
  server/         # main.go — wires up router, runs HTTP server and cron
internal/
  api/            # HTTP handlers (search, today job create/poll, image proxy)
  scraper/        # playerzone HTML scrapers + scrape orchestration
    player.go     # scrapes player list + profile pages
    tournament.go # parses a team schedule page into per-game RGX deltas
    orchestrate.go # ScrapePlayerToday — the full home→tournament→division→schedule hop, run off the request path
  jobqueue/       # single-worker queue draining RgxJob rows through the shared rate limiter
  cache/          # in-memory TTL cache for tournament results + images
  db/             # database access layer (player index, RgxJob store, permission store)
  cron/           # weekly player-index scrape + daily RgxJob cleanup scheduler
  config/         # env-based config (port, DB path, scrape interval, etc.)
```

#### Key Go decisions

- **HTTP router**: `net/http` stdlib + `gorilla/mux` or `chi` for route params
- **HTML scraping**: `golang.org/x/net/html` or `github.com/PuerkitoBio/goquery`
- **Database**: SQLite via `modernc.org/sqlite` (zero C deps, easy local dev) with option to swap to Postgres in production
- **In-memory cache**: simple `sync.Map` with expiry timestamps, or `github.com/patrickmn/go-cache`
- **Cron**: `github.com/robfig/cron/v3`
- **Image proxy**: `net/http` reverse-proxy with local disk cache under `./cache/images/`

---

## Data Flow: User Searches for a Player

```
1. User types ≥2 chars into SearchBar
2. SearchBar debounces 200ms, calls GET /api/players/search?q=max
3. Go handler queries local SQLite players table (LIKE '%max%')
4. Returns ≤10 matches with proxied image URLs
5. SearchBar renders AutocompleteItem list
6. User selects a player → player.id stored in reactive state
7. ResultView mounts, calls GET /api/players/{id}/today
8. Go handler:
   a. Checks current CET time — outside the weekend/08:00 window, or robots.txt
      disallows scraping → return {status: "no_tournament"} immediately, no
      RgxJob is created
   b. Otherwise, look up today's RgxJob for this player:
      - queued/running → return that job as-is (dedup)
      - done, finished < 15 min ago → return its cached result immediately
      - done (stale) or error → fall through
   c. Create a new RgxJob (status "queued"), enqueue its ID on the
      background jobqueue.Queue, return {job_id, status: "queued", queue_position}
9. The single jobqueue worker (FIFO, serialized through the existing
   playerzone rate limiter) picks up the job: marks it "running", runs the
   full home→tournament→division→schedule scrape, computes RGX deltas, and
   marks the job "done" (with the result) or "error"
10. ResultView polls GET /api/players/{id}/today/jobs/{jobId} every 2s,
    showing the queue position (as a German status phrase) until the job
    reaches "done" or "error"
11. ResultView renders RgxCircle + MoodText + SpieleListe from the final result
```

---

## Data Flow: Weekly Player Index Scrape

```
1. Cron fires Friday 22:00 CET
2. PlayerScraper fetches playerzone player list / ranking page(s)
3. For each player: parse id, name, avatar_url, club_name, club_logo_url, current_rgx
4. Upsert into SQLite players table
5. Log count of players updated / added / errors
```

---

## Deployment

- Single binary (Go server) + compiled Vue/Nuxt static assets served by the same binary or a reverse proxy (Caddy / nginx)
- SQLite database file on persistent volume
- Environment variables for configuration (no hardcoded secrets):
  - `PORT` (default 8080)
  - `DB_PATH` (default `./data/players.db`)
  - `IMAGE_CACHE_DIR` (default `./cache/images`)
  - `TZ` must be set to `Europe/Berlin`

---

## Resolved Decisions

| # | Question | Decision |
|---|----------|----------|
| OQ1 | Does playerzone block scrapers? | No known blocking. Standard `net/http` with a reasonable User-Agent is fine. Add 1 req/s rate limit as courtesy. |
| OQ2 | How to detect today's tournament? | Scrape the home page (`/`). Tournaments shown there are live/in-play. `/tournaments/upcoming` shows not-yet-started events only. |
| OQ3 | Is the player ID stable? | Yes. Derived from avatar filename prefix (e.g. `PAULCHEN`, `LUC2110`) — stable across time. |
| OQ4 | Search background shader? | Plain WebGL/canvas shader — vanilla JS only, no Three.js. Degrades to static gradient in low-battery mode. |
| OQ5 | Cache backend? | Single in-process `go-cache` or `sync.Map` with TTL — no Redis needed for single-server deployment. |
| OQ6 | RGX formula? | Fully documented. See `03-data-requirements.md §RGX Calculation Formula`. |
| OQ7 | Game-info pages accessible? | No — `/tournaments/game-info/{id}` requires login. RGX is computed from team schedule page + division teams page. |

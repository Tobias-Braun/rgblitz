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
│  /api/images/proxy (image cache)        │
│                                         │
│  ┌──────────────┐  ┌──────────────────┐ │
│  │  Player DB   │  │  In-memory TTL   │ │
│  │  (SQLite)    │  │  result cache    │ │
│  └──────────────┘  └──────────────────┘ │
│                                         │
│  ┌──────────────────────────────────┐   │
│  │         Scraper Layer            │   │
│  │  - player index scrape (cron)    │   │
│  │  - per-player scrape (job queue) │   │
│  │  - image proxy + disk cache      │   │
│  └────────────────┬─────────────────┘   │
└───────────────────┼─────────────────────┘
                    │  HTTP (scraping, rate-limited)
                    ▼
        playerzone.roundnetgermany.de
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
| `PlayerChip.vue` | Persistent header chip for the selected player (avatar, name, close button) |
| `NoTournamentCard.vue` | State 2 — player has no tournament today |
| `NoGamesPlayedYet.vue` | State 2d — registered for today's tournament, schedule has no games yet |
| `ResultLoading.vue` | State 2a — skeleton + queue-position phrase while a scrape job is in flight |
| `ResultStale.vue` | State 2b — offers retry vs. accept-stale-result when a live fetch fails but cached data exists |
| `ResultError.vue` | State 2c — hard failure with no cached fallback |
| `useBattery.ts` (composable) | Wraps `navigator.getBattery()` and `prefers-reduced-motion`, exposes `isFastMode` |
| `useTodayResult.ts` (composable) | Drives the job-create → poll-until-final-status flow behind `/api/players/:id/today` |
| `mood-phrases.ts` | Static config file: RGX-delta brackets → German phrase strings |
| `queue-phrases.ts` | Static config file: queue-position brackets → German phrase strings |

See `docs/02-ui-requirements.md` for the full state list and behavior of each.

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

- **HTTP router**: `github.com/go-chi/chi/v5`
- **HTML scraping**: `github.com/PuerkitoBio/goquery` (jQuery-style selectors over `golang.org/x/net/html`)
- **Database**: SQLite via `modernc.org/sqlite` — pure Go, no cgo, a single file on disk. No plan to swap to a client/server database; the whole dataset is small and single-writer
- **In-memory cache**: `github.com/patrickmn/go-cache` for TTL'd tournament results
- **Cron**: `github.com/robfig/cron/v3`
- **Image proxy**: plain `net/http` client fetch (not a reverse proxy — the handler reads the upstream response fully, then writes it out) with a local disk cache under `./cache/images/`

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
1. Cron fires Saturday 03:00 CET (or once at startup if the players table has < 1000 rows)
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

## Notable Implementation Details

- **Scraper courtesy**: no known blocking from playerzone, but every outbound request is still serialized through a single global rate limiter capped at **1 request per 2 seconds** (`internal/scraper/rate_limiter.go`), shared across the weekly index scrape and every per-player job.
- **Tournament detection**: rather than scanning the site for active tournaments, the backend scrapes the *player's own profile page* (`/players/{numeric_id}`) and looks for an entry dated today — see `05-scraping-algorithm.md §Tournament Detection`. There is no separate "is there a tournament right now" check.
- **Player ID stability**: derived from the avatar image filename prefix (e.g. `PAULCHEN`, `LUC2110`) — stable across time, used as the primary key everywhere in the backend.
- **Game-info pages are out of scope**: `/tournaments/game-info/{game_id}` requires a playerzone login, so RGX deltas are computed from the team schedule page + division teams page instead of scraped directly.

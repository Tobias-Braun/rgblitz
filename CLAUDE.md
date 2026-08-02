# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

The project is **implemented**: the Go backend and Nuxt frontend both exist and build. The `docs/` folder is still the spec of record — read it before changing behaviour, since it can drift from the code on details that aren't covered below.

| Doc | Contents |
|-----|----------|
| `docs/01-overview.md` | Goals, tech stack, domain assumptions (tournaments weekends-only, etc.) |
| `docs/02-ui-requirements.md` | All UI states, component behaviour, battery UX, mood text thresholds |
| `docs/03-data-requirements.md` | RGX formula, data entities, caching TTL rules, scraper robustness |
| `docs/04-architecture.md` | Component tree, Go package layout, data flow sequences, resolved decisions |
| `docs/05-scraping-algorithm.md` | Step-by-step scraping pseudocode, rate limiting, new-player proxy logic |
| `docs/06-api-spec.md` | Full REST API contract with request/response shapes and polling behaviour |

## Tech stack

- **Frontend**: Vue 3 + Nuxt (SSR enabled), vanilla CSS and JS only — no UI libraries, no animation libraries
- **Backend**: Go — `net/http` + chi/mux router, SQLite via `modernc.org/sqlite`, `robfig/cron/v3`, `go-cache` or `sync.Map` for in-memory TTL cache
- **Data source**: HTML scraping of `playerzone.roundnetgermany.de` — no API exists

## Architecture in one paragraph

The Go backend serves a REST API (`/api/*`) and is the sole client of playerzone. It maintains a SQLite player index (scraped every Saturday 03:00 CET, plus automatically once at startup if the table has fewer than 1000 rows — see `internal/cron/jobs.go` and `cmd/server/main.go`) that powers the autocomplete endpoint without any live playerzone requests. When a user selects a player on a weekend, the backend scrapes the home page to find active tournaments, walks the division team lists to locate the player's team, fetches that team's schedule page, then computes RGX deltas using the formula in `docs/03-data-requirements.md` against pre-tournament RGX values. The Nuxt frontend calls the Go API server-side on first render (SSR), then polls `/api/players/:id/today` every 60 seconds client-side during the tournament window.

Every scrape (cron-triggered, startup-triggered, or via `Scheduler.ScrapeNow()`) first checks `robots.txt` when `RESPECT_PERMISSION=true`, persisting the allow/deny result via `internal/db.PermissionStore` so it can also gate live API requests. If disallowed, no further playerzone requests are made for that run.

## Key domain rules (non-obvious, will cause bugs if missed)

- **Tournaments are Saturday/Sunday only.** Return `{status: "no_tournament"}` immediately on weekdays without touching playerzone.
- **RGX inputs are pre-tournament and fixed for the whole day.** Do not update `r1`/`r2` between games — all games use the same baseline. `total_delta` is the plain sum of per-game deltas — **no division caps apply anymore** (removed; see `docs/03-data-requirements.md`).
- **New players** (no RGX in DB) get their division's starting value as a proxy (Beginner/Basic 1000, Intermediate 1100, Advanced 1250, Contender 1400, Pro 1600, unknown 1200). Contender is distinct from Pro. Set `has_proxy: true` on affected game results.
- **Rate limit**: max 1 outbound request to playerzone every 2 seconds. Use a single global token bucket/ticker shared across all goroutines.
- **Game-info pages** (`/tournaments/game-info/{id}`) require login — they are out of scope. RGX is computed, not scraped.

## RGX formula (implement exactly)

```
e1 = 1 / (1 + 10^((r2 - r1) / 550))
x1 = b × 50 × (p - e1)        # rounded to nearest integer
x2 = -x1
```

- `r1`, `r2` = sum of both players' RGX on each team (pre-tournament, fixed)
- `b` = 1.0 for multi-set (Best-of-3 or Best-of-5), 0.75 for single-set
- `p` (set score → p): 1:0/2:0/3:0 → 1.0, 3:1 → 0.75, 2:1 → 0.67, 3:2 → 0.6, 1:1/2:2 → 0.5, 2:3 → 0.4, 1:2 → 0.33, 1:3 → 0.25, 0:1/0:2/0:3 → 0.0
- Scale constant `d = 550` (not the ELO standard 400)

## Playerzone page map

| Purpose | URL |
|---------|-----|
| Active/live tournaments | `https://playerzone.roundnetgermany.de/` (home) |
| Upcoming (not started) | `/tournaments/upcoming` |
| Player ranking + RGX | `/ranking/rg-index/` |
| Tournament detail + division links | `/tournaments/{id}-{slug}` |
| Division team list (with combined RGX) | `/tournaments/{id}-{slug}/teams/{div_id}` |
| Team game schedule | `/tournaments/schedule/team/{team_id}` |

Player avatars and club logos are on `pstaassets.blob.core.windows.net` — proxy them via `/api/images/proxy`, never serve the CDN URL directly to the browser.

## Go package layout (actual)

```
cmd/server/         # main.go — wires router, HTTP server, cron, startup index check
cmd/seed/           # one-off DB seeding tool
cmd/reindex/        # manual re-run of the player index scrape
cmd/scrapecheck/    # manual fixture/live scrape smoke test
cmd/checkpermission/ # manual robots.txt permission check
internal/api/       # HTTP handlers + router
internal/scraper/   # HTML parsers, fetcher, rate limiter, robots.txt permission check
internal/cache/     # in-memory TTL cache
internal/db/        # SQLite access layer (PlayerStore, PermissionStore)
internal/cron/      # weekly scrape job (Scheduler)
internal/config/    # env-based config
```

## Frontend component layout (actual)

```
components/
  SearchBar.vue       # animated bar + autocomplete, manages focus state
  AutocompleteItem.vue
  RgxCircle.vue       # gradient glow ring, battery-aware animation
  MoodText.vue        # threshold → German phrase lookup
  SpieleListe.vue     # accordion wrapper
  GameRow.vue         # opponent / W·L·D / ±delta
  AppHeader.vue / AppFooter.vue
  PlayerChip.vue
  NoTournamentCard.vue
  ResultView.vue / ResultLoading.vue / ResultStale.vue / ResultError.vue
composables/
  useBattery.ts       # wraps navigator.getBattery(), exposes isLowBattery ref
  useTodayResult.ts   # fetches /api/players/:id/today, manages 60s poll
config/
  mood-phrases.ts     # static threshold → phrase map (German)
types/
  api.ts              # shared API response types
```

## Environment variables (Go backend)

| Variable | Default | Notes |
|----------|---------|-------|
| `PORT` | `8080` | |
| `DB_PATH` | `./data/players.db` | |
| `IMAGE_CACHE_DIR` | `./cache/images` | |
| `TZ` | — | Must be set to `Europe/Berlin` |
| `DEV_MODE` | `false` | |
| `FIXTURE_DIR` | `""` | When set, scraper reads from local fixture HTML instead of playerzone |
| `RESPECT_PERMISSION` | `false` | When `true`, every scrape checks `robots.txt` before making any playerzone request |

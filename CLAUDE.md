# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project documentation

The `docs/` folder is the spec of record for behaviour not covered below — it's kept in sync with the code, but always prefer reading the actual code over the docs when the two disagree.

| Doc                             | Contents                                                                  |
| ------------------------------- | ------------------------------------------------------------------------- |
| `docs/01-overview.md`           | Goals, tech stack, domain assumptions (tournaments weekends-only, etc.)   |
| `docs/02-ui-requirements.md`    | All UI states, component behaviour, battery UX, mood text thresholds      |
| `docs/03-data-requirements.md`  | RGX formula, data entities, caching TTL rules, scraper robustness         |
| `docs/04-architecture.md`       | Component tree, Go package layout, data flow sequences                    |
| `docs/05-scraping-algorithm.md` | Step-by-step scraping pseudocode, rate limiting, new-player proxy logic   |
| `docs/06-api-spec.md`           | Full REST API contract with request/response shapes and polling behaviour |
| `docs/07-analytics.md`          | Umami events, session properties, privacy rule for tracked data           |

## Tech stack

- **Frontend**: Vue 3 + Nuxt (SSR enabled), vanilla CSS and JS only — no UI libraries, no animation libraries
- **Backend**: Go — `net/http` + `go-chi/chi/v5` router, SQLite via `modernc.org/sqlite`, `robfig/cron/v3`, `patrickmn/go-cache` for in-memory TTL cache, `PuerkitoBio/goquery` for HTML parsing
- **Data source**: HTML scraping of `playerzone.roundnetgermany.de` — no API exists

## Architecture in one paragraph

The Go backend serves a REST API (`/api/*`) and is the sole client of playerzone. It maintains a SQLite player index (scraped every Saturday 03:00 CET, plus automatically once at startup if the table has fewer than 1000 rows — see `internal/cron/jobs.go` and `cmd/server/main.go`) that powers the autocomplete endpoint without any live playerzone requests. When a user selects a player on a weekend, `GET /api/players/:id/today` creates (or reuses) a background `RgxJob` and returns immediately; a single worker (`internal/jobqueue`) drains queued jobs one at a time, scraping the player's profile page to find today's tournament entry, walking the division team list to find the opponent's combined RGX, fetching the team's schedule page, and computing RGX deltas using the formula in `docs/03-data-requirements.md` against pre-tournament RGX values. The Nuxt frontend calls the Go API server-side on first render (SSR) to create/check the job, then polls `GET /api/players/:id/today/jobs/:jobId` every 2 seconds client-side until the job reaches `done` or `error` (see `frontend/composables/useTodayResult.ts`).

Every scrape (cron-triggered, startup-triggered, or via `Scheduler.ScrapeNow()`) first checks `robots.txt` when `RESPECT_PERMISSION=true`, persisting the allow/deny result via `internal/db.PermissionStore` so it can also gate live API requests. If disallowed, no further playerzone requests are made for that run.

## Key domain rules (non-obvious, will cause bugs if missed)

- **Tournaments are Saturday/Sunday only.** Return `{status: "no_tournament"}` immediately on weekdays without touching playerzone.
- **RGX inputs are pre-tournament and fixed for the whole day.** Do not update `r1`/`r2` between games — all games use the same baseline. `total_delta` is the plain sum of per-game deltas — no division caps apply.
- **New players** (no RGX in DB) get their division's starting value as a proxy (Beginner/Basic 1000, Intermediate 1100, Advanced 1250, Contender 1400, Pro 1600, unknown 1200). Contender is distinct from Pro. Set `has_proxy: true` on affected game results.
- **Rate limit**: max 1 outbound request to playerzone every 2 seconds, enforced by a single shared `RateLimiter` (`internal/scraper/rate_limiter.go`) — every scraper and the job-queue worker go through the same instance.
- **Game-info pages** (`/tournaments/game-info/{id}`) require login — they are out of scope. RGX is computed, not scraped.
- **Stale-on-error is not wired up in production.** `TodayResult.Stale` exists and the frontend has a full UI for it (`ResultStale.vue`), but nothing in the real scrape path sets it — a failed job just becomes `status: "error"`. The only place `Stale: true` is produced is the `DEV_MODE` fixture player `dev-stale`. Don't assume a live stale-cache fallback exists; if you're asked to add one, it needs new logic in `internal/jobqueue`.
- **Image proxy is origin-restricted, not a generic passthrough.** `isAllowedImageOrigin` (`internal/api/handlers.go`) only allows `https://` URLs whose host is exactly `pstaassets.blob.core.windows.net`/`playerzone.roundnetgermany.de` or a proper subdomain of one — never loosen this to a `strings.HasSuffix` check without a label-boundary guard, since Azure blob container names are self-service and a naive suffix check is bypassable (e.g. `evilpstaassets.blob.core.windows.net`).
- **RgxJob usage stats (`rgx_jobs_stats`) persist independently of `rgx_jobs`.** `rgx_jobs` is purged daily by cron; `rgx_jobs_stats` (`internal/db/stats.go`) is a separate, never-purged table recording one row per _genuinely created_ job (never for a job reused from the dedup/freshness cache, and never for `DEV_MODE` synthetic jobs). `GET /api/stats/rgx-jobs` aggregates it for the frontend footer's all-time usage stat.
- **Only GRT tournaments produce RGX changes.** Other national bodies (e.g. EURA) run tournaments on the same platform; a tournament's own detail page (`/tournaments/{id}-{slug}`) states whether it's GRT via a "German Roundnet Tour {year}" label. `internal/scraper.ScrapePlayerToday` resolves this once per tournament, permanently caching it in the never-purged `tournaments` table (`internal/db/tournaments.go`) — GRT status is assumed fixed forever once a tournament has started, so it's never re-scraped. A non-GRT tournament produces `{status: "no_tournament"}`, identical to no tournament at all.

## Two playerzone layouts, one set of parsers

Playerzone rebuilt its tournament pages around web components in September 2026. Every
parser in `internal/scraper` reads **both** that layout and the older one, because the
fixtures in `scraper/fixtures/` (captured 2026-07-19) are still the offline test corpus —
don't "clean up" a fallback branch just because the live site no longer needs it. What
moved:

| Data                                       | Current layout                                                  | Pre-September-2026 layout                       |
| ------------------------------------------ | --------------------------------------------------------------- | ----------------------------------------------- |
| Any RGX value (ranking rows, player cards) | `rg-score-badge[score]`                                         | `span.rgx-value` text                           |
| Tournament name (detail page)              | `span.rg-tournament-nav--title`                                 | `h3.kf-title`                                   |
| Divisions of a tournament                  | `div.rg-tournament-nav--context` links to `/teams/{id}`         | `div.sub-section` blocks                        |
| Profile tournament entries                 | `li.rg-player-tournaments--item`, date in the card's `subtitle` | `li.year-item`, date in `.team-tournament-date` |
| Schedule page division/team name           | `div.tournament-division` / nav / page title                    | `h3.event-title` / `div.second-title`           |
| Team ID in a game card                     | `load_team_info(a, b, null, null, gameID)`                      | `load_team_info(a, b)`                          |

Two consequences worth knowing before touching the pipeline: a **played** team's schedule
page links only its own division (labelled "Results", not by division name — hence the
single-division fallback in `resolveDivisionTeamsPath`), and a schedule page for a
tournament that hasn't started links **no** division at all, which is why a team with no
games yet returns `ok` with an empty game list instead of failing.

`cmd/fixturecapture` is the tool for diagnosing the next such change: its whole-tournament
mode cross-checks both sides of every game, and a division whose deltas don't sum to 0 is
the signal that a parser is silently reading the wrong value.

## Scraping robustness policy

Every playerzone page is untrusted input. Treat the HTML as capable of changing at any
moment — a DOM tweak, an A/B test, a tournament type that renders a different layout (see
the non-GRT case below) — and never assume a selector, attribute, or text node will be
present or non-empty just because it usually is.

- **No unexpected page content may ever panic the process.** Indexing (`[0]`), a failed
  type assertion, or any other operation that can panic on absent/malformed data must be
  guarded. A parser encountering unexpected structure must return an `error` (see the
  existing "fail loudly" pattern in `internal/scraper` and its `robustness_test.go`
  suite — new parsing code should get a similar garbage/malformed/missing-field test) or,
  where a sensible default exists, degrade gracefully (e.g. a missing RGX badge on a
  player card is treated as "no RGX", which the existing new-player-proxy logic already
  handles — see `firstField` in `internal/scraper/tournament.go`). Never let a raw
  `slice[0]`, unchecked map/type assertion, or similar sit in a code path fed directly by
  scraped HTML.
- **Recover at every goroutine boundary that processes scraped data, as defense in
  depth.** An unrecovered panic in any goroutine crashes the _entire_ Go process (Go
  offers no per-goroutine isolation), not just that unit of work. Both long-running
  entry points that process untrusted playerzone data — `internal/jobqueue.Queue.process`
  (one RGX job) and the cron jobs registered in `internal/cron.Scheduler.Start`
  (`robfigcron.Recover` middleware) — recover from panics and convert them into a
  normal, terminal failure (`db.JobStatusError` for jobs; a logged error for cron runs)
  instead of taking down the server. Any new background worker that touches scraped data
  needs the same guarantee.
- **A recovered panic must never leave a job re-triable in a way that repeats itself.**
  `db.JobStore.ResetStuckRunning` only re-enqueues jobs still in `"running"` state after a
  crash-recovery restart — so a job must reach a terminal state (`done` or `error`)
  before the goroutine handling it returns, panic or not. This is what stops a
  reproducible bad-input panic from crash-looping the server (or, pre-recover, from
  poisoning the queue) on every retry against the same input.

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
- `go run ./cmd/rgxcalc --r1 <int> --r2 <int> --won <int> --lost <int>` prints every intermediate value for a single game — useful for checking a hand-worked example against the code.

## Playerzone page map

| Purpose                                                  | URL                                       |
| -------------------------------------------------------- | ----------------------------------------- |
| Player profile (tournament history, incl. today's entry) | `/players/{numeric_id}`                   |
| Upcoming tournaments (not yet started)                   | `/tournaments/upcoming`                   |
| Player ranking + RGX                                     | `/ranking/rg-index/`                      |
| Tournament detail + division links                       | `/tournaments/{id}-{slug}`                |
| Division team list (with combined RGX)                   | `/tournaments/{id}-{slug}/teams/{div_id}` |
| Team game schedule                                       | `/tournaments/schedule/team/{team_id}`    |

Player avatars and club logos are on `pstaassets.blob.core.windows.net` — proxy them via `/api/images/proxy`, never serve the CDN URL directly to the browser.

## Go package layout (actual)

```
cmd/server/         # main.go — wires router, HTTP server, cron, startup index check
cmd/seed/           # one-off DB seeding tool
cmd/reindex/        # manual re-run of the player index scrape
cmd/scrapecheck/    # manual fixture/live scrape smoke test
cmd/checkpermission/ # manual robots.txt permission check
cmd/rgxcalc/        # CLI: compute one game's RGX delta from raw inputs, no network/DB
cmd/fixturecapture/ # CLI: snapshot a whole tournament running today (or one player's scrape)
                    # into a replayable capture dir — HTML + parsed intermediates + deltas +
                    # cross-checks; driven by scripts/capture-fixtures.sh
internal/api/       # HTTP handlers + router
internal/scraper/   # HTML parsers, fetcher, rate limiter, robots.txt permission check
internal/jobqueue/  # single-worker queue that drains RgxJob rows through the scraper
internal/cache/     # in-memory TTL cache
internal/db/        # SQLite access layer (PlayerStore, JobStore, JobStatsStore, TournamentStore, PermissionStore)
internal/cron/      # weekly scrape job + daily RgxJob cleanup (Scheduler)
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
  NoTournamentCard.vue      # player has no tournament today
  NoGamesPlayedYet.vue      # registered for today, schedule has no games yet
  ResultView.vue            # normal result state
  ResultLoading.vue         # job queued/running, shows queue-position phrase
  ResultStale.vue           # retry vs. accept-stale-result (see production gap note above)
  ResultError.vue           # hard failure, no cached fallback
  UpcomingTournaments.vue   # not-yet-started tournaments under the focused, still-empty search bar
composables/
  useBattery.ts       # wraps navigator.getBattery() + prefers-reduced-motion, exposes isFastMode
  useTodayResult.ts   # drives job-create → poll-every-2s-until-final-status flow
  useRgxJobStats.ts   # non-blocking (lazy) fetch of /api/stats/rgx-jobs for the footer's served-count stat
  useTrack.ts         # no-op-safe Umami track/identify wrapper (events: docs/07-analytics.md)
  useUpcomingTournaments.ts # non-blocking (lazy) fetch of /api/tournaments/upcoming, rendered by UpcomingTournaments.vue
config/
  mood-phrases.ts     # RGX-delta bracket → German phrase map
  queue-phrases.ts    # queue-position bracket → German phrase map
types/
  api.ts              # shared API response types
```

## Environment variables (Go backend)

| Variable                | Default             | Notes                                                                                                                                                                                          |
| ----------------------- | ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PORT`                  | `8080`              |                                                                                                                                                                                                |
| `DB_PATH`               | `./data/players.db` |                                                                                                                                                                                                |
| `IMAGE_CACHE_DIR`       | `./cache/images`    |                                                                                                                                                                                                |
| `TZ`                    | —                   | Must be set to `Europe/Berlin`                                                                                                                                                                 |
| `DEV_MODE`              | `false`             | Enables synthetic `dev-ok`/`dev-stale`/`dev-error`/`dev-loading` players for exercising every result state without live scraping                                                               |
| `FIXTURE_DIR`           | `""`                | When set, scraper reads from local fixture HTML instead of playerzone                                                                                                                          |
| `RESPECT_PERMISSION`    | `false`             | When `true`, every scrape checks `robots.txt` before making any playerzone request                                                                                                             |
| `RESEND_API_KEY`        | `""`                | Resend API key for the feedback email notification. Optional — if unset (or `FEEDBACK_NOTIFY_EMAIL` is unset), the notification silently no-ops; feedback is still stored in SQLite either way |
| `RESEND_FROM_EMAIL`     | `""`                | Sender address for the feedback email notification, e.g. `RGBlitz <feedback@yourdomain>`                                                                                                       |
| `FEEDBACK_NOTIFY_EMAIL` | `""`                | Recipient address for the feedback email notification                                                                                                                                          |

# rgblitz — Go Backend

## Prerequisites

- Go 1.22+
- TZ must be set to `Europe/Berlin` at runtime (the server enforces CET for tournament-window logic)

---

## Fetch / restore dependencies

```sh
cd backend
go mod download   # download to module cache (no internet needed after this)
go mod tidy       # prune unused entries and add any missing ones
```

If `go.sum` is missing or stale:

```sh
go mod tidy
```

---

## Seed the database from the HTML fixture

Before running the server for the first time, populate the player index from the captured ranking page snapshot:

```sh
cd backend
go run ./cmd/seed \
  --fixture ../scraper/fixtures/ranking.html \
  --db      ../data/players.db
```

This parses all players from the fixture and upserts them into SQLite. Run it any time you want to reset or refresh the local DB without hitting playerzone.

---

## Run the server

```sh
cd backend
TZ=Europe/Berlin go run ./cmd/server
```

The server listens on `:8080` by default. Override with `PORT`.

### Dev mode (bypass weekend/time guard)

In dev mode every call to `/api/players/:id/today` returns a synthetic tournament result computed from real players in the DB — regardless of the current day or time. Useful when building the frontend on a weekday.

```sh
TZ=Europe/Berlin DEV_MODE=true go run ./cmd/server
```

---

## Manually verify the RGX formula

`cmd/rgxcalc` computes a single game's RGX delta from raw numbers and prints every intermediate value of the formula (`b`, `p`, `e1`, and the rounded `delta`). It touches no network, database, or fixtures — the fastest way to sanity-check the math in `docs/03-data-requirements.md` against a hand-worked example.

```sh
cd backend
go run ./cmd/rgxcalc --r1 2347 --r2 2401 --won 2 --lost 0
```

```
inputs:  r1=2347  r2=2401  sets=2:0

b  (format factor)      = 1.00   (Best-of-3)
p  (match outcome)      = 1.00
e1 (expected score)     = 1 / (1 + 10^((r2-r1)/550)) = 0.4437
delta = round(b * 50 * (p - e1))
      = round(1.00 * 50 * (1.00 - 0.4437))
      = +28
```

Flags:

| Flag | Description |
|------|-------------|
| `--r1` | Combined pre-tournament RGX of your team (sum of both players) — required |
| `--r2` | Combined pre-tournament RGX of the opponent team — required |
| `--won` | Sets won by your team |
| `--lost` | Sets lost by your team |

A single-set game (`--won 1 --lost 0` or `--won 0 --lost 1`, no `2:x` result) automatically uses `b = 0.75` instead of `1.0` — you'll see this reflected in the printed breakdown.

---

## Manually verify the scraping pipeline

`cmd/scrapecheck` dry-runs the *entire* scrape → RGX pipeline for one player — home page → active tournaments → GRT classification → divisions → division teams → team schedule → per-game RGX deltas — and prints every step as it happens. It never goes through the HTTP server. Since GRT classification is cached permanently in the `tournaments` DB table (see the root `CLAUDE.md`), it opens a small SQLite file under the OS temp dir on each run — no `--db-path` flag needed, this is just a throwaway cache shared across repeated invocations.

Step 1 fetches and merges both ranking tabs (Open + Women), same as the reindex job. Note that the RGX printed here is only the *indexed* value — a game's actual `r1`/`r2` and the player's own before/after RGX for the day always come live from the tournament's own pages (`ScrapeTodayResult`), since a woman playing an Open tournament uses her live Open RGX for that day even though her indexed value is her Women RGX.

### Dev mode (default) — safe, offline, no setup

Reads HTML fixtures from `../scraper/fixtures` instead of the network:

The bundled fixtures were captured on 2026-07-19, and the fixture tournament is only "active" on that date — so pass `--date` to match it (otherwise you'll correctly get `no tournament found`, since "today" is genuinely not 2026-07-19):

```sh
cd backend
go run ./cmd/scrapecheck --player LUC2110 --date 2026-07-19
# or match by name instead of ID:
go run ./cmd/scrapecheck --player "Lucas Christiani" --date 2026-07-19
```

Try a date outside the tournament window to see the other branch:

```sh
go run ./cmd/scrapecheck --player LUC2110 --date 2026-07-20   # a day after the tournament ended → no_tournament
```

### Prod mode — hits the real site

```sh
go run ./cmd/scrapecheck --mode prod --player "Paul Siemer"
```

This scrapes `playerzone.roundnetgermany.de` for real, respecting the standard 1-request-per-2-seconds rate limit. Only run this when you actually want live network traffic — dev mode covers everything you need for day-to-day development.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--mode` | `dev` | `dev` reads local fixtures, `prod` scrapes the live site |
| `--player` | — | Player ID (e.g. `LUC2110`) or full name (e.g. `"Lucas Christiani"`) — required |
| `--fixture-dir` | `../scraper/fixtures` (or `$FIXTURE_DIR`) | Directory of HTML fixtures (dev mode only) |
| `--date` | today | Override "today" as `YYYY-MM-DD` (Europe/Berlin) — useful for testing date-window matching against fixtures |

If the player isn't found, or matches more than one player, `scrapecheck` prints a clear error instead of guessing.

---

## Capture live fixtures during a tournament

`cmd/fixturecapture` is the tool for debugging *wrong* results: it fetches the pages the RGX pipeline reads and writes each one to disk together with what the parsers made of it. When a delta doesn't match reality, the snapshot taken at that hour contains the exact HTML that produced it.

#### Tournament mode (no arguments)

```sh
cd backend
go run ./cmd/fixturecapture
```

Finds every tournament running today and captures all of it — the detail page, every division's team list, every team's schedule — and computes each team's RGX deltas through `scraper.ComputeTeamDayResult`, the same function the API pipeline calls. No advance knowledge of who is playing is needed, which is what makes it usable unattended.

Seeing *both* sides of every game also makes the results auditable without knowing the right answer. `report.txt` ends with consistency checks:

- **delta sum per division** — RGX is zero-sum per game (`x2 = -x1`), so a division must sum to 0; anything else means deltas aren't mirroring, usually an `r1`/`r2` mix-up;
- **mirror mismatches** — games where the two teams' schedules disagree about the set scores, the combined RGX inputs, or deltas that don't cancel;
- **proxied RGX** — teams whose own `r1`, or whose opponent's `r2`, fell back to a division starting value, which is the usual explanation for a surprising number.

When a parser the *production* pipeline depends on fails (`ParseTournamentDetail`, `ParseDivisionSidebar`), the capture records it prominently and keeps going — falling back to `ParseDivisionLinks`, which finds divisions by their `/teams/{id}` links regardless of layout. A capture is most valuable exactly when the parsers are broken, so it must not abort with them.

#### Player mode

```sh
go run ./cmd/fixturecapture --players "LUC2110,Lucas Christiani"
```

Captures only what one player's `/today` request fetches, through the real `ScrapePlayerToday`. The ranking pages are fetched once per run and hard-linked into each snapshot, so a weekend of hourly captures doesn't cost a 3.8 MB ranking copy per player per hour.

#### What a snapshot holds

Snapshots land in `scraper/captures/<YYYY-MM-DD>/<HHMM>-t<tournament-id>/` (or `-<player-id>`):

| File | Contents |
|------|----------|
| `*.html` | Every fetched page |
| `manifest.json` | URL path → file, replayed by `scraper.ManifestFetcher` |
| `capture.json` | Fetched paths, parsed intermediates, computed results, full check findings |
| `report.txt` | The same rendered for reading: schedule players and their RGX, division teams with combined RGX, every game's `r1`, `r2`, sets, `b`, `p`, `e1` and delta, then the consistency checks |

A snapshot replays offline as often as you like while you fix a parser — `scrapecheck` and `fixturecapture` both pick up the manifest automatically:

```sh
go run ./cmd/scrapecheck --player LUC2110 \
  --fixture-dir ../scraper/captures/2026-09-12/1400-LUC2110 --date 2026-09-12

go run ./cmd/fixturecapture --tournaments 695 --date 2026-09-12 \
  --from-fixtures ../scraper/captures/2026-09-12/1400-t695
```

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--players` | — | Comma-separated player IDs or full names; omit for tournament mode |
| `--tournaments` | — | Numeric tournament IDs to capture instead of discovering today's |
| `--out` | `../scraper/captures` (or `$CAPTURE_DIR`) | Root directory for snapshots |
| `--divisions` | — | Only capture divisions whose name contains one of these substrings |
| `--include-non-grt` | `false` | Also capture non-GRT tournaments (the backend reports those as `no_tournament`) |
| `--max-requests` | `600` | Request budget per run, so one huge tournament can't run for hours at 1 req/2s |
| `--db-path` | temp dir | SQLite file backing the `tournaments` GRT-status cache |
| `--from-fixtures` | — | Capture from a fixture or capture directory instead of the live site |
| `--date` | today | Override "today" as `YYYY-MM-DD` |

A full tournament costs roughly `1 + divisions + teams` requests — about 110 for a nine-division tournament, so ~4 minutes at the 1-request-per-2-seconds limit.

### Capturing automatically all weekend

`scripts/capture-fixtures.sh` wraps the above with the domain's schedule: it captures only on Saturday/Sunday between 09:00 and 18:00 Europe/Berlin, and does nothing at any other time. Run it hourly from cron, or let it schedule itself:

```sh
# stays running, captures every tournament running today at each top of the hour
./scripts/capture-fixtures.sh --watch

# single window-gated run — the form to put in crontab
0 * * * * /path/to/rgblitz/scripts/capture-fixtures.sh >> /tmp/rgblitz-capture.log 2>&1
```

`--players` narrows a run to specific players, `--force` captures immediately regardless of the weekday/hour window, and `--start`/`--end` move the window. Anything after `--` is passed through to `fixturecapture` (e.g. `-- --divisions Advanced`). A failed capture never stops `--watch`; it's logged and the next hour is attempted normally.

---

## Manually re-run the player index scrape

`cmd/reindex` runs the exact same job the Saturday 03:00 CET cron trigger runs — `Scheduler.ScrapeNow()` — against the live `playerzone.roundnetgermany.de` ranking pages, and upserts the results into SQLite. Use it to check the ranking-page scraper still works after a change, or to manually refresh/repair the index without waiting for (or restarting) the weekly schedule.

Both ranking tabs are scraped and merged: `/ranking/rg-index/1` (Open) is the base list, and `/ranking/rg-index/2` (Women) overrides the RGX of any player who also appears there. Women play Open tournaments too, but their tracked Open RGX and Women RGX can diverge — the persisted index always stores the Women RGX for those players, so the DB stays a stable reference value. If the Women tab fails to fetch, the reindex still completes with Open-only data (logged as a warning, not fatal).

```sh
cd backend
go run ./cmd/reindex --db ../data/players.db
```

This hits the live site (respecting the standard 1-request-per-2-seconds rate limit) and prints the player count before and after. It reuses the same upsert path as the cron job, so existing players are updated in place and new players are added — nothing is deleted.

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | `../data/players.db` | Path to the SQLite database to update |

---

## robots.txt compliance (`RESPECT_PERMISSION`)

Every live request identifies itself as `User-Agent: RGBlitzBot` (see `scraper.DefaultUserAgent`) — a deliberately distinct, non-browser-impersonating name that playerzone can trivially block via robots.txt if they want to.

When `RESPECT_PERMISSION=true`, the app checks whether robots.txt allows `RGBlitzBot` to fetch every page shape it needs (`scraper.RequiredScrapePaths`: home, both ranking tabs, `/tournaments/`) before doing anything else:

- The weekly (or manually triggered via `cmd/reindex`) player index scrape re-checks robots.txt first. If disallowed, it makes **zero** further requests and just stores the result.
- The result is persisted in the `scrape_permission` SQLite table (`internal/db/permission.go`) and reused by live `/api/players/:id/today` requests — if the stored result says "disallowed" (or no check has ever run), the API returns `{"status": "no_tournament"}` without touching the network, same as the weekday/time guard.

When `RESPECT_PERMISSION=false` (the default), this is all skipped and the app behaves as before — always attempts to scrape.

Check (or re-check) permission on demand, independent of the weekly cron:

```sh
cd backend
go run ./cmd/checkpermission --db ./data/players.db
```

This fetches the live `robots.txt`, evaluates it for `RGBlitzBot`, and updates the stored permission — useful right after enabling the flag, or to pick up a robots.txt change without waiting for Friday.

---

## Environment variables

| Variable        | Default              | Description |
|-----------------|----------------------|-------------|
| `PORT`          | `8080`               | HTTP listen port |
| `DB_PATH`       | `./data/players.db`  | SQLite database file |
| `IMAGE_CACHE_DIR` | `./cache/images`   | Disk cache for proxied CDN images (7-day TTL) |
| `TZ`            | —                    | **Must** be `Europe/Berlin`; affects all time-window logic |
| `DEV_MODE`      | `false`              | Skip weekday/time guard; return fake tournament data |
| `RESPECT_PERMISSION` | `false`          | Honor robots.txt for `RGBlitzBot` — see above |

---

## Compile a binary

```sh
cd backend
go build -o ../bin/rgblitz-server ./cmd/server
TZ=Europe/Berlin ../bin/rgblitz-server
```

Cross-compile for Linux (e.g. for deployment):

```sh
GOOS=linux GOARCH=amd64 go build -o ../bin/rgblitz-server-linux ./cmd/server
```

---

## Run the tests

Tests run against the HTML fixtures in `../scraper/fixtures/` — no internet connection or running server required. This includes:

- `internal/scraper`: parser correctness for every playerzone page type (including tournament detail pages, for GRT/non-GRT classification), the RGX formula (`ComputeDeltaBreakdown`/`MatchP`) across all outcome types including single-set games, division RGX caps, `resolveTournamentGRT`'s scrape-once/cache-forever behaviour, the `Fetcher` abstraction (fixture routing + a live-HTTP path against `httptest`), and robustness checks that every parser fails loudly (never panics) on malformed or empty HTML.
- `internal/db`: `TournamentStore` CRUD (get-missing, create-then-get, idempotent create for an already-seen tournament ID).
- `internal/api`: HTTP handler behaviour via `httptest` — search, the weekday/time-window guard (asserted with a fetcher that fails the test if the network is touched), dev mode, a full fixture-backed scrape with result caching, unknown players, and the image proxy's input validation.

```sh
cd backend
go test ./...
```

Run a single package with verbose output:

```sh
go test -v ./internal/scraper/...
go test -v ./internal/api/...
```

If a fixture file is missing the test is skipped (not failed), so the suite stays green in CI even without the full fixture set.

---

## Package layout

```
cmd/
  server/      — main entry point: wires router, HTTP server, cron
  seed/        — one-shot DB seeder from ranking.html fixture
  scrapecheck/ — dry-run the scrape → RGX pipeline for one player (dev fixtures or live site)
  rgxcalc/     — pure RGX formula calculator, no network/DB involved
  fixturecapture/ — snapshot a whole tournament (or one player's scrape) from the live site:
                 the HTML, the parsed intermediates, the computed deltas and cross-checks
  reindex/     — manually re-run the weekly player index scrape against the live site
  checkpermission/ — manually re-check robots.txt and update the stored RESPECT_PERMISSION result
internal/
  api/      — HTTP handlers (search, today job create/poll, image proxy) + chi router
  jobqueue/ — single-worker queue draining RgxJob rows through the shared rate limiter
  cache/    — thin in-memory TTL cache wrapper (go-cache)
  config/   — env-based config struct
  cron/     — weekly Saturday 03:00 CET player-index scrape + daily RgxJob cleanup
  db/       — SQLite schema migration + player/job/tournament/permission CRUD
  scraper/  — HTML parsers for each playerzone page type (including a tournament's own
              detail page, for GRT classification), the RGX formula, and the Fetcher
              abstraction (HTTPFetcher for the live site, FixtureFetcher for dev mode /
              tests / the scrapecheck CLI, ManifestFetcher for replaying a capture
              request-for-request)
```

---

## API endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/players/search?q=` | Autocomplete from local player index (≥2 chars) |
| `GET` | `/api/players/{id}/today` | Gets or creates today's scrape job for a player; returns immediately |
| `GET` | `/api/players/{id}/today/jobs/{jobId}` | Polls the status/result of a job created above |
| `GET` | `/api/images/proxy?url=` | CDN image proxy (base64-encoded URL param) |
| `GET` | `/health` | Liveness check — returns 200 |

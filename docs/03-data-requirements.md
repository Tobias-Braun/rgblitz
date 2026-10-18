# RGBlitz — Data Requirements

## Source

All data originates from **playerzone.roundnetgermany.de** via HTML scraping.
No official API exists. The Go backend is the sole consumer of playerzone; the Vue frontend never contacts playerzone directly.

Images (player avatars, club logos) are hosted on Azure Blob Storage at `pstaassets.blob.core.windows.net` and must be proxied (see §Image Proxy).

> **Note for contributors**: playerzone's HTML structure isn't versioned or documented anywhere — it can change without notice. The scrapers (`internal/scraper/`) assert on known stable identifiers and are tested against saved HTML fixtures in `scraper/fixtures/`. If a live scrape starts failing, check those fixtures against the current page source before assuming the parsing logic is at fault.

---

## RGX Calculation Formula

RGX deltas are **not provided by playerzone** — they must be computed by the backend from game results and pre-game RGX values.

Source: https://playerzone.roundnetgermany.de/ranking/rg-index/faq (supersedes the 2023 rules at https://roundnetgermany.de/adjustment-rgx-division-limits-2023/ — starting values changed and division caps were removed; see below)

### Formula

```
x1 = b × k × (p - e1)

e1 = 1 / (1 + 10^((r2 - r1) / d))
x2 = -x1   (zero-sum: team 2 loses exactly what team 1 gains)
```

| Variable | Value / Meaning |
|----------|----------------|
| `d` | 550 (scale constant, replaces ELO's standard 400) |
| `k` | 50 (max points per game) |
| `b` | 1.0 for Best-of-3 matches; 0.75 for single-set matches |
| `r1` | Sum of both players' RGX on team 1 (pre-game values) |
| `r2` | Sum of both players' RGX on team 2 (pre-game values) |
| `e1` | Expected score for team 1 |
| `p` | Actual match result for team 1 (see table below) |
| `x1` | RGX delta applied to **each** player on team 1 |

### Match Result Values (p)

Covers single-set, best-of-3, and best-of-5 formats:

| Set score (team1 : team2) | p for team 1 |
|---------------------------|-------------|
| 1:0, 2:0, or 3:0 | 1.0 |
| 3:1 | 0.75 |
| 2:1 | 0.67 |
| 3:2 | 0.6 |
| 1:1 or 2:2 | 0.5 |
| 2:3 | 0.4 |
| 1:2 | 0.33 |
| 1:3 | 0.25 |
| 0:1, 0:2, or 0:3 | 0.0 |

### Application

- Both players on the **winning** team receive `+x1` each.
- Both players on the **losing** team receive `+x2` (= `-x1`) each.
- In a league Begegnung, each **Partie** is one such game between two pairs: `r1`/`r2` are the RGX sums of the two pairs who played that Partie, not of the whole squads (see §League Matchdays).
- All games in a tournament day use the **pre-game RGX values** as `r1`/`r2` inputs — the player's RGX does not change between games within the same day.
- The displayed daily delta is the **sum** of all per-game `x1` values computed against the fixed pre-tournament baseline.
- **No division caps apply.** Division RGX limits were removed; `total_delta` is a plain sum of per-game deltas with nothing clamped afterwards.

### New Players (no existing RGX)

Players without an existing RGX score (new players, shown with a 🆕 indicator on the ranking page) are assigned their **division's starting value** as a proxy for `r` in the formula:

| Division | Proxy RGX |
|----------|-----------|
| Beginner / Basic | 1,000 |
| Intermediate | 1,100 |
| Advanced | 1,250 |
| Contender | 1,400 |
| Pro | 1,600 |
| Unknown / no division | 1,200 |

Contender is a distinct division from Pro — it is **not** treated as a synonym for Pro.

In the real playerzone system, new players go through a pre-calculation round that produces their actual starting RGX. We omit this because it would require calculating all other players' scores simultaneously. The division default is a close enough approximation and this case is uncommon.

**UI requirement**: whenever a new player (ours or an opponent) contributes to a delta calculation using a proxy value, display a disclaimer next to the result:
> "Ein Spieler in diesem Spiel hat noch keine RGX. Der angezeigte Wert ist eine Schätzung."

---

## Data Entities

### Player (master data)

| Field | Source | Notes |
|-------|--------|-------|
| `id` | Derived from avatar filename (e.g. `PAULCHEN` prefix) or assigned sequentially | Stable identifier used in all cache keys |
| `name` | `/ranking/rg-index/` ranking page | Full name as displayed |
| `avatar_url` | `/ranking/rg-index/` — `pstaassets.blob.core.windows.net/assets/players/...` | Proxied through our backend (see §Image Proxy) |
| `club_name` | playerzone player page | |
| `club_logo_url` | playerzone club page | Proxied through our backend |
| `current_rgx` | playerzone player page | The player's current ELO rating |

### Tournament Result (daily data)

| Field | Source | Notes |
|-------|--------|-------|
| `tournament_id` | playerzone tournament page | |
| `tournament_date` | playerzone tournament page | `YYYY-MM-DD` |
| `player_id` | — | FK to Player |
| `games` | playerzone match list | Array; see Game below |
| `total_delta` | computed | Sum of `game.rgx_delta` for the day |

### Game

| Field | Source | Notes |
|-------|--------|-------|
| `opponent_name` | playerzone match row | |
| `result` | playerzone match row | `W` / `L` / `D`, or `C` if the game was cancelled |
| `rgx_delta` | computed | Signed integer; always `0` for a cancelled game |
| `rgx_delta_per_player` | computed | `rgx_delta / 2` |
| `has_proxy` | computed | True if either team involved a player with no RGX (division default used) |
| `round` | playerzone match row | Optional; include if available |
| `was_cancelled` | playerzone match row | True if playerzone marked the game as cancelled (forfeit, injury, etc.) |
| `sets` | playerzone match row | Array of `{our_score, opponent_score}`, oriented to our team, in play order; empty/partial for a cancelled game |

---

## Tournament Classification (GRT vs. non-GRT)

Not every tournament on playerzone is a **German Roundnet Tour (GRT)** tournament —
other national bodies (e.g. **EURA**) run tournaments on the same platform, and those
must never produce RGX changes. A tournament's own detail page
(`/tournaments/{id}-{slug}`) states this directly: GRT tournaments render a
"German Roundnet Tour {year}" label next to the GRT logo; non-GRT tournaments don't.

A permanent table, `tournaments`, records this classification the first time a
tournament is resolved (never purged, unlike `rgx_jobs` — see §Usage Statistics for
the analogous rationale):

| Field | Meaning |
|-------|---------|
| `id` | Numeric playerzone tournament ID |
| `name` | Tournament name (`span.rg-tournament-nav--title` on the detail page) |
| `is_grt` | Whether the tournament counts towards the German Roundnet Tour |
| `date` | First day of the tournament, `YYYY-MM-DD` |
| `scraped_at` | When this row was first written |

**Write rules**:

- Looked up by tournament ID as soon as `ScrapePlayerToday` resolves which tournament
  the player is registered in today (`internal/scraper/orchestrate.go`).
- If no row exists yet, `GET /tournaments/{id}` (playerzone redirects the numeric-only
  path to the full slug URL) is scraped once and the result inserted.
- **Never re-scraped or updated once inserted** — a tournament's GRT status is assumed
  fixed for good once the tournament has started, so there is no TTL and no refresh path.
- A non-GRT tournament produces `{status: "no_tournament"}` — identical to the player
  having no tournament at all today. No RGX deltas are computed for it.

---

## League Matchdays (Deutsche Roundnet Liga)

The Deutsche Roundnet Liga (DRL) runs on the same platform but differs from a tournament
in every dimension the tournament pipeline relies on:

| | Tournament | DRL |
|---|---|---|
| Tournament entity | one weekend | one per season (e.g. `793`, "Deutsche Roundnet Liga 2026/2027"), four divisions |
| Date | the profile entry's date | per **matchday group** (`.md-group-header` on the phase schedule page); the profile entry carries only one season date |
| Team | a pair | a club squad of 8–18 players |
| Game | pair vs pair | a **Begegnung** (club vs club) of five **Partien**: Open 1, Open 2, Women, Mixed 1, Mixed 2 |
| Who played | the pair | per Partie, from `/ajax/get-subgames/{game_id}` (surnames, with an initial where needed) |
| GRT label | yes | none — it is never classified as GRT, and it is not looked up in `tournaments` |

Every Partie counts towards RGX, which playerzone recalculates after each matchday, so the
pre-matchday RGX shown on the rosters is the fixed baseline for the day, as for a tournament.
Only the regular season is supported; the final phases (Final Six, Elite Eight, Champions Cup)
carry no matchday group and are skipped.

A roster index, `league_players`, maps playerzone's numeric player ID to the player's team
(tournament registration ID) in the current season:

| Field | Meaning |
|-------|---------|
| `player_id` | Bare numeric playerzone player ID (the only ID a roster links) |
| `team_id` | The team's registration ID — the `{team_id}` of its schedule page |
| `tournament_id` | The season's tournament ID |

**Write rules**: rebuilt Saturday and Sunday 03:00 CET right after the player index, and once
at startup (in the background) while empty. Rows of earlier seasons are dropped; a team whose
roster failed to load keeps its earlier rows.

**Read rules**: every job checks it first. A player on a roster whose team plays today gets
league results (`competition: "league"`), one game per Partie the player played. A player not on
any roster, or whose team doesn't play today, goes through the tournament pipeline unchanged.

---

## Caching Strategy

### Player Index (master data)

- **Schedule**: Every **Saturday and Sunday at 03:00 CET** (cron job in Go backend, `internal/cron/jobs.go`) — comfortably before either day's first match, and running on both days means a Saturday tournament's RGX changes are picked up before a Sunday tournament computes its own deltas
- **Startup fallback**: Also runs once at process startup if the `players` table has fewer than 1000 rows, so a fresh deployment doesn't wait up to a week for a usable autocomplete (`cmd/server/main.go`)
- **What**: Scrape full player list from playerzone — all active players with name, ID, avatar URL, club name, club logo URL
- **Storage**: Local database table `players` (upsert by player ID)
- **Purpose**: Powers the autocomplete; no live playerzone request per search keystroke
- **Staleness tolerance**: Up to a few days old is acceptable

### Tournament Data (daily, on-demand)

Tournaments run on a **single calendar day**, typically **08:00–20:00 CET**.

**Cache key**: `tournament:{player_id}:{date}` where `date` is `YYYY-MM-DD` in CET.

**Polling trigger**: A client requests data for a player on the current date.

**TTL logic**:

| Condition | Cache TTL |
|-----------|-----------|
| Weekday (Mon–Fri) | Return `{status: "no_tournament"}` immediately — no playerzone request, no cache entry needed |
| Weekend, before 08:00 CET | Return `{status: "no_tournament"}` immediately without hitting playerzone |
| Weekend, 08:00–20:00 CET (tournament window) | Cache for **60 seconds** — re-scrape on next request after expiry |
| Weekend, after 20:00 CET | Tournament considered finished; cache for **24 hours**; no further scrapes today |
| Weekend, no tournament found for player today | Cache negative result for **15 minutes** to avoid hammering playerzone |

**"Tournament active today" check**: Before scraping match data, check whether there is a tournament scheduled for today on playerzone. This avoids unnecessary scraping on non-tournament days.

**GRT check**: Once a tournament is found for today, its GRT classification is also checked (see §Tournament Classification) — a non-GRT tournament (e.g. EURA-rated) is treated the same as no tournament at all.

---

## Image Proxy

Player avatars and club logos point to the playerzone Azure CDN. The frontend must **not** load these directly (CORS, hot-linking). The Go backend proxies them via `GET /api/images/proxy`.

Full endpoint spec: see `06-api-spec.md`.

- Caches proxied images on disk with a TTL of 7 days
- Returns `Content-Type` matching the original image
- Issues a fresh, independent request to the CDN — no cookies or headers from the incoming client request are ever forwarded upstream
- Only `https://` URLs on the two allow-listed hosts (`pstaassets.blob.core.windows.net`, `playerzone.roundnetgermany.de`) are proxied; anything else is rejected with `400`

---

## Backend API

Full REST API specification is in `06-api-spec.md`. Summary of endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/players/search?q=` | Autocomplete from local player index |
| `GET` | `/api/players/:id/today` | Gets or creates today's scrape job for a player; returns immediately, never blocks on a live scrape |
| `GET` | `/api/players/:id/today/jobs/:jobId` | Polls the status/result of a job created above |
| `GET` | `/api/images/proxy?url=` | CDN image proxy with disk cache |
| `GET` | `/api/stats/rgx-jobs` | All-time RgxJob usage counts (created/succeeded/failed) — see §Usage Statistics |

Response time target for `/api/players/search`: < 100 ms (DB-only, no remote call).

The live scrape itself runs on a single background worker (`internal/jobqueue`) rather than inline in the request handler — this is what lets multiple concurrent users each get an instant response while still serializing every outbound playerzone request through one rate limiter. See `04-architecture.md §Data Flow` for the full request lifecycle.

---

## Usage Statistics

A separate table, `rgx_jobs_stats`, records permanent (never purged) usage counts, independent of the `rgx_jobs` table's daily cleanup (see §Backend API and `internal/cron`).

**Why a separate table**: `rgx_jobs` rows are purged daily by cron. All-time usage counts need to survive that purge, so they live in their own table rather than being derived from `rgx_jobs` at query time.

| Field | Meaning |
|-------|---------|
| `job_id` | The originating `rgx_jobs.id` — not a foreign key on purpose, since that row is purged daily while this one persists |
| `player_id` | Player the job was created for |
| `tournament_id` | Numeric playerzone tournament ID (from `/tournaments/{id}-{slug}/...`) the job resolved to; `NULL` if the job errored before resolving one, or the player had no tournament today |
| `status` | `queued` (set at creation) → `done` or `error` (set when the job finishes) |
| `created_at` / `finished_at` | Timestamps |

**Write rules**:

- A row is inserted **only** when a genuinely new `rgx_jobs` row is created (`internal/api/handlers.go TodayResult`) — never when an existing job is reused because it's still in flight or within the 15-minute freshness window (see `06-api-spec.md`).
- The row is updated to `done`/`error` by the background worker (`internal/jobqueue`) when the job finishes — the same place that marks the underlying `rgx_jobs` row done/error.
- Dev-mode synthetic jobs (`dev-ok`/`dev-stale`/`dev-error`/`dev-loading`, and the no-job-at-all dev-mode path for real players) never write to this table — only real production scrapes count toward usage stats.

**Reads**: `GET /api/stats/rgx-jobs` aggregates all-time totals with a single indexed `COUNT`/`SUM` query over `status` (see `06-api-spec.md`). If this table ever grows large enough for that aggregation to become slow, replace it with running counters updated alongside the writes above instead of re-aggregating the whole table on every read.

---

## Scraper Robustness

- Scrapers are tested with saved HTML fixtures (`scraper/fixtures/`) so they can be run offline
- If playerzone changes its HTML structure, the scraper should fail loudly (assert on known element presence) rather than silently returning empty data
- A failed scrape currently surfaces to the client as `status: "error"`, not a stale-cache fallback — see `05-scraping-algorithm.md §Not currently implemented` for the gap between this and the frontend's `ResultStale.vue` UI

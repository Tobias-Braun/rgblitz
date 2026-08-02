# rgx-calc — Data Requirements

## Source

All data originates from **playerzone.roundnetgermany.de** (note correct spelling) via HTML scraping.  
No official API exists. The Go backend is the sole consumer of playerzone; the Vue frontend never contacts playerzone directly.

Images (player avatars, club logos) are hosted on Azure Blob Storage at `pstaassets.blob.core.windows.net` and must be proxied (see §Image Proxy).

> **Important**: Before implementing scrapers, inspect the current DOM structure of playerzone to confirm element selectors. The site structure may change without notice — scraper tests should assert on known stable identifiers.

---

## Data Entities

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

## Caching Strategy

### Weekly Player Index (master data)

- **Schedule**: Every **Friday at 22:00 CET** (cron job in Go backend)
- **What**: Scrape full player list from playerzone — all active players with name, ID, avatar URL, club name, club logo URL
- **Storage**: Local database table `players` (upsert by player ID)
- **Purpose**: Powers the autocomplete; no live playerzone request per search keystroke
- **Staleness tolerance**: Up to 7 days old is acceptable

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

---

## Image Proxy

Player avatars and club logos point to the playerzone Azure CDN. The frontend must **not** load these directly (CORS, hot-linking). The Go backend proxies them via `GET /api/images/proxy`.

Full endpoint spec: see `06-api-spec.md`.

- Caches proxied images on disk with a TTL of 7 days
- Returns `Content-Type` matching the original image
- Strips cookies and auth headers before forwarding

---

## Backend API

Full REST API specification is in `06-api-spec.md`. Summary of endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/players/search?q=` | Autocomplete from local player index |
| `GET` | `/api/players/:id/today` | Today's tournament result for a player |
| `GET` | `/api/images/proxy?url=` | CDN image proxy with disk cache |

Response time target for `/api/players/search`: < 100 ms (DB-only, no remote call).

---

## Scraper Robustness

- All scrapers must handle HTTP errors from playerzone gracefully (return stale cache if available, surface error to client otherwise)
- Log all scrape attempts with timestamp, target URL, HTTP status, and parse success/failure
- Scrapers are tested with saved HTML fixtures so they can be run offline
- If playerzone changes its HTML structure, the scraper should fail loudly (assert on known element presence) rather than silently returning empty data

# RGBlitz — Scraping Algorithm

## Page Map

| Page | URL Pattern | Auth Required | Purpose |
|------|-------------|---------------|---------|
| Player profile | `https://playerzone.roundnetgermany.de/players/{numeric_id}` | No | List every tournament a player has played, each with its tournament-scoped team ID |
| Upcoming | `https://playerzone.roundnetgermany.de/tournaments/upcoming` | No | List future tournaments (not yet started) |
| Ranking | `https://playerzone.roundnetgermany.de/ranking/rg-index/` | No | Full player list with individual RGX |
| Division teams | `/tournaments/{id}-{slug}/teams/{div_id}` | No | All teams in a division with combined RGX + player names |
| Team schedule | `/tournaments/schedule/team/{team_id}` | No | A single team's games (scores, round, opponents) + a sidebar listing every division in that tournament (name + teams-page link) |
| Game info | `/tournaments/game-info/{game_id}` | **Yes** | Per-game detail — **behind login, out of scope** |

---

## Master Data Scrape (Saturday and Sunday, 03:00 CET)

Running on both tournament days (not just Saturday) means a Saturday tournament's RGX changes are picked up before a Sunday tournament computes its own deltas.

### Target
`https://playerzone.roundnetgermany.de/ranking/rg-index/`

### Algorithm

```
1. GET /ranking/rg-index/
2. For each row in the ranking table:
   a. Extract: rank, player_name, club_name, rgx_value, games_played, trend_indicator
   b. Extract avatar_url from <img> src attribute (pstaassets.blob.core.windows.net/assets/players/...)
   c. Extract club_logo_url from club <img> src attribute
   d. Derive player_id from avatar filename:
      - Filename pattern: profile_{ID}-{YYYY-MM-DD-...}.jpg
      - player_id = the {ID} segment (e.g. "PAULCHEN", "LUC2110")
3. UPSERT all players into `players` DB table (conflict on player_id → update all fields)
4. Log: total scraped, updated, new, errors
```

### Parsed fields per player

```go
type Player struct {
    ID         string  // from avatar filename (e.g. "PAULCHEN")
    Name       string
    ClubName   string
    AvatarURL  string  // original, un-proxied
    ClubLogoURL string
    RGX        int
    GamesPlayed int
    ScrapedAt  time.Time
}
```

### Failure handling
- If the page returns non-200: retain existing DB data, log error, retry once after 10 min
- If parsing yields 0 players: treat as a parse failure, do not overwrite DB

---

## Tournament Detection (on-demand, per request)

### Goal
Determine whether a given player has a tournament today, using the player's
own profile page instead of scanning the site for active tournaments.

### Algorithm

```
1. Derive numeric_id from player_id (the trailing digit run, e.g. "LUC2110" → "2110")
2. GET https://playerzone.roundnetgermany.de/players/{numeric_id}
3. Find all li.year-item entries — one per tournament this player has ever played
4. For each entry:
   a. Extract tournament_date from .team-tournament-date (format DD.MM.YY)
   b. Extract tournament_team_id from the onclick="load_tournament_games(id, '')"
      handler inside that entry's .info-buttons block
      - This is a tournament-scoped team ID, distinct from the persistent team ID
        under .tournament-team — only the load_tournament_games ID resolves
        correctly on /tournaments/schedule/team/{id}
   c. Skip entries missing either piece (e.g. an upcoming tournament with no
      bracket/Games button published yet)
5. Compare tournament_date against TODAY using an exact string match
   (now.Format("02.01.06")) — sidesteps two-digit-year parsing ambiguity
6. If no entry matches today:
   → return status "no_tournament" for this player
7. Otherwise proceed to §Tournament Data Flow using that entry's tournament_team_id
```

There is no separate "detect active tournaments site-wide" step and nothing is
cached per-tournament: detection is scoped to one player's profile page per
request (itself cached briefly, see §Caching Summary).

---

## Player Lookup Flow (on-demand, per user search)

### Goal
Given a player search query string, return matching players from the local DB.

```
1. Query DB: SELECT * FROM players WHERE LOWER(name) LIKE LOWER('%{q}%') LIMIT 10
2. Return player records (with proxied image URLs constructed server-side)
3. No playerzone request is made during search — fully served from local cache
```

---

## Tournament Data Flow (on-demand, when user selects a player)

### Goal
Given a `player_id`, return today's RGX delta and individual game breakdown.

### Prerequisites
- Weekly player DB must be populated (player's RGX is needed as starting point)

### Algorithm

```
FUNCTION GetTodayResult(player_id):

  1. player = DB.GetPlayer(player_id)
     IF player == nil: return {status: "not_found"}

  2. numeric_id = NumericPlayerID(player_id)
     entries = FetchProfileEntries(numeric_id)  # GET /players/{numeric_id}, cached 60s
     today_entry = entry IN entries WHERE entry.date == TODAY.Format("DD.MM.YY")
     IF today_entry == nil: return {status: "no_tournament"}
     # Player is not registered for anything today, or not registered at all

  3. GET /tournaments/schedule/team/{today_entry.tournament_team_id}
     → Parse twice from the same response body:
       a. TeamSchedule: our team's players + their individual RGX (pre-tournament
          values shown on page; use these in preference to the weekly cache as
          they are more current), division_name (h3.event-title), and all games
          in order: [{round, opponent_name, set_scores[]}]
       b. DivisionSidebar: every division in this tournament, each with its
          display name (div.sub-section .sub-title) and teams-page path
          (div.sub-section .big-title a[href])

  4. found_division = sidebar entry WHERE name equals TeamSchedule.division_name
     (case-insensitive)
     IF found_division == nil: fail loudly — DOM/parse mismatch, not "no tournament"

  5. pre_r1 = ResolveTeamRGX(team_players, found_division.name)
     # See ResolvePlayerRGX below — handles new players with no score

  6. division_teams = GET found_division.teams_path (cached 5min per teams_path)
     → Parse team list: [{team_name, team_id, player_names[], combined_rgx}]

  7. FOR each game IN games:
     a. opponent_team = FindTeamByName(opponent_name, division_teams)
        → opponent_combined_rgx = division_teams entry combined RGX
           (if opponent team contains a new player, this value already reflects the
            division proxy — the division teams page shows the combined value we use directly)
     b. Determine match format:
        - 2+ sets present → multi-set (Best-of-3 or Best-of-5) → b = 1.0
        - 1 set only → single-set → b = 0.75
     c. Determine p from set_scores:
        - Count sets won by each side
        - Map (our_sets : their_sets) → p value (see §RGX Formula table)
     d. e1 = 1 / (1 + 10^((opponent_combined_rgx - pre_r1) / 550))
     e. delta = round(b * 50 * (p - e1))
        (both players on our team get the same delta)
     f. Store game result: {round, opponent_name, result (W/L/D), rgx_delta: delta}

  8. total_delta = sum of all per-game deltas (no division caps applied)

  9. Cache result under key tournament:{player_id}:{today} with TTL:
     - Before 20:00 CET: 60 seconds
     - After 20:00 CET: 24 hours (tournament finished)

  10. Return {status: "ok", player, date, total_delta, games[]}
```

### ResolvePlayerRGX — handling new players

```
FUNCTION ResolvePlayerRGX(player_name, division_name) → (rgx int, is_proxy bool):

  player = DB.GetPlayerByName(player_name)

  IF player == nil OR player.RGX == 0 OR player.IsNew:
    # New player: no existing RGX on the ranking page (shown with 🆕)
    proxy_rgx = DivisionStartingRGX(division_name)
    return (proxy_rgx, true)

  return (player.RGX, false)

FUNCTION ResolveTeamRGX(players [], division_name) → (combined_rgx int, has_proxy bool):
  total = 0
  has_proxy = false
  FOR p IN players:
    rgx, is_proxy = ResolvePlayerRGX(p.name, division_name)
    total += rgx
    IF is_proxy: has_proxy = true
  return (total, has_proxy)
```

`has_proxy` propagates into the game result object:

```go
type SetResult struct {
    OurScore      int
    OpponentScore int
}

type GameResult struct {
    Round             string
    OpponentName      string
    Result            string  // "W", "L", "D", or "C" for a cancelled game
    RGXDelta          int     // always 0 for a cancelled game
    RGXDeltaPerPlayer float32
    HasProxy          bool    // true if any player in this game used a division default RGX
    WasCancelled      bool
    Sets              []SetResult
}
```

The `has_proxy` flag on **any** game in a day's results causes the frontend to display the estimation disclaimer (see `03-data-requirements.md §New Players`).

---

### Cancelled games

playerzone renders a cancelled game (forfeit, injury, etc.) the same way it renders a
not-yet-played game — `outcome=""` on the `rg-game-card` element, since neither has a
final win/loss outcome. The two are told apart by a `cancelled` class token on
`rg-game-card` (and/or a `<span class="tag game-cancelled">` inside the tags slot),
present only for actually-cancelled games:

```
IF outcome == "" AND NOT has "cancelled" marker:
    SKIP this game entirely (scheduled, not yet played)
ELSE IF outcome == "" AND has "cancelled" marker:
    KEEP the game, mark was_cancelled = true, RGXDelta = 0
    (whatever partial set scores exist are still reported, but never scored)
ELSE:
    KEEP the game, compute RGXDelta normally
```

---

### Determining W/L/D from set scores

```
Given set_scores = [(21, 18), (24, 25), (21, 14)] for our team:

sets_won_by_us   = count where score_pair.our > score_pair.their
sets_won_by_them = count where score_pair.their > score_pair.our

Result:
  2:0 → W → p = 1.0
  2:1 → W → p = 0.67
  1:1 → D → p = 0.5
  1:2 → L → p = 0.33
  0:2 → L → p = 0.0
  1:0 (single set) → W → p = 1.0, b = 0.75
  0:1 (single set) → L → p = 0.0, b = 0.75
```

---

## Caching Summary

| Cache Key | TTL | Storage | Contents |
|-----------|-----|---------|----------|
| `players` DB table | Updated weekly | SQLite | All player master data |
| `profile:{numeric_id}` | 60s | In-memory | Parsed tournament entries from a player's profile page |
| `division_teams:{teams_path}` | 5 min | In-memory | Parsed team list for a division |
| `tournament_result:{player_id}:{date}` | 60s (active) / 24h (finished) | In-memory | Computed game results + delta |
| image cache file, named from a sanitized original URL | 7 days | Disk (`./cache/images/`) | Proxied image bytes |

---

## Scraping Robustness Rules

1. **Never scrape playerzone faster than 1 request per 2 seconds** — enforced by a single global rate limiter (`internal/scraper/rate_limiter.go`, ticker-based) shared across the weekly index scrape and every per-player job. Parallel goroutines queue through it, not bypass it. **Implemented.**
2. **Fixture-based tests**: raw HTML snapshots for each page type live in `scraper/fixtures/`; parsers are tested against these offline (see `internal/scraper/*_test.go`). **Implemented.**
3. **robots.txt gate**: when `RESPECT_PERMISSION=true`, every scrape run checks `robots.txt` first and persists the allow/deny result; a disallowed result blocks further playerzone requests for that run (`internal/scraper/robots.go`, `internal/db/permission.go`). **Implemented.**

### Not currently implemented

The `TodayResult` type has a `stale` field (`internal/scraper/orchestrate.go`) and the frontend has a full UI for it (`ResultStale.vue` — retry vs. accept-stale-result), but nothing in the production code path ever sets `stale: true`. Today, when a job's scrape fails, it's simply marked `"error"` with no fallback to a previous cached result — the frontend then shows `ResultError.vue` instead. The only place `Stale: true` is ever produced is the `DEV_MODE` fixture player (`dev-stale`), used to exercise the `ResultStale.vue` UI in isolation. If you want real stale-on-error behavior (serve the last successful result when a live scrape fails, per the original design intent below), it needs wiring up in `internal/jobqueue` — flagged here so it isn't mistaken for a bug in existing behavior.

A circuit breaker (pausing all scraping after N consecutive playerzone errors) was discussed early on but was never built; the rate limiter above is the only backpressure mechanism today.

---

## Player Identification — Deduplication

The player's canonical ID is derived from their avatar image filename:
```
https://pstaassets.blob.core.windows.net/assets/players/profile_{ID}-{timestamp}.jpg
                                                                  ^^^^
                                                         This is the stable player_id
```

Examples: `PAULCHEN`, `LUC2110`, `RAS1811`

On the team schedule page, players are matched to the DB by name string comparison (normalized: lowercase, trim whitespace). This is required to link team page player entries back to our cached player records.

> **Edge case**: Two players with the same full name — flag in DB with a `name_collision: true` field. Surface this in logs; the autocomplete must still show both with their respective club names to distinguish them.

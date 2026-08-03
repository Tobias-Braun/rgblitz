# RGBlitz — REST API Specification

## General

- All endpoints are prefixed with `/api`
- All responses are `Content-Type: application/json`
- All timestamps are ISO 8601 in CET (`Europe/Berlin`)
- HTTP errors use standard status codes; the body always contains `{"error": "<message>"}`
- The Nuxt frontend calls this API both server-side (SSR) and client-side (after hydration)

---

## Endpoints

### `GET /api/players/search`

Returns players matching a name query from the local weekly-cached player index. No live playerzone request is made.

**Query parameters**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Name substring to search. Min 2 characters. Case-insensitive. |

**Response `200 OK`**

```json
[
  {
    "id": "PAULCHEN",
    "name": "Paul Siemer",
    "club_name": "1. Roundnet Club Köln",
    "avatar_url": "/api/images/proxy?url=<encoded>",
    "club_logo_url": "/api/images/proxy?url=<encoded>",
    "rgx": 1963
  }
]
```

- Returns at most **10** results, ordered by descending RGX.
- Returns an **empty array** `[]` when no players match — never `null`.
- Returns `400` if `q` is missing or fewer than 2 characters.

---

### `GET /api/players/:id/today`

Gets or creates today's `RgxJob` for this player and returns it immediately — it never blocks on a live playerzone scrape. The actual multi-hop scrape (home page → tournament detail → division teams → team schedule) runs in a background worker, serialized through the same global rate limiter as before. The frontend polls `GET /api/players/:id/today/jobs/:jobId` (below) until the job reaches a final status.

**Path parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Stable player ID (e.g. `PAULCHEN`) |

**Dedup / freshness rules**

- If a job for this player+day is already `queued` or `running`, that same job is returned (no duplicate is created).
- If the latest job is `done` and finished **less than 15 minutes ago**, its cached result is returned immediately — no new scrape.
- If the latest job is `done` but **older than 15 minutes**, or `error`, a new job is created and enqueued (refresh/retry).
- Outside the tournament window (weekday, or before 08:00 CET on a weekend) or when a robots.txt check disallows scraping, `status: "no_tournament"` is returned directly — no job is ever created.

**Response `200 OK` — job created or already in flight**

```json
{
  "job_id": 42,
  "status": "queued",
  "queue_position": 2
}
```

`status: "running"` (no `queue_position` — it's already being processed):

```json
{
  "job_id": 42,
  "status": "running"
}
```

**Response `200 OK` — cached/fresh result available immediately**

```json
{
  "job_id": 42,
  "status": "done",
  "finished_at": "2026-07-19T14:32:10+02:00",
  "result": {
    "status": "ok",
    "player": {
      "id": "PAULCHEN",
      "name": "Paul Siemer",
      "club_name": "1. Roundnet Club Köln",
      "avatar_url": "/api/images/proxy?url=<encoded>",
      "club_logo_url": "/api/images/proxy?url=<encoded>",
      "rgx": 1963
    },
    "date": "2026-07-19",
    "total_delta": 12,
    "has_proxy": false,
    "games": [
      {
        "round": "Gr. A #1",
        "opponent_name": "Heidelberger Heidelbärinnen",
        "result": "W",
        "rgx_delta": 7,
        "rgx_delta_per_player": 3.5,
        "has_proxy": false,
        "was_cancelled": false,
        "sets": [
          { "our_score": 21, "opponent_score": 15 },
          { "our_score": 21, "opponent_score": 18 }
        ]
      },
      {
        "round": "Gr. A #2",
        "opponent_name": "Hanni & Anni",
        "result": "L",
        "rgx_delta": -3,
        "rgx_delta_per_player": -1.5,
        "has_proxy": false,
        "was_cancelled": false,
        "sets": [
          { "our_score": 15, "opponent_score": 21 },
          { "our_score": 18, "opponent_score": 21 }
        ]
      },
      {
        "round": "Gr. A #3",
        "opponent_name": "no grace",
        "result": "C",
        "rgx_delta": 0,
        "rgx_delta_per_player": 0,
        "has_proxy": false,
        "was_cancelled": true,
        "sets": []
      }
    ]
  }
}
```

**Response `200 OK` — no tournament today** (weekday, outside time window, robots.txt disallows scraping, or player not registered)

```json
{
  "status": "no_tournament",
  "result": {
    "status": "no_tournament",
    "player": {
      "id": "PAULCHEN",
      "name": "Paul Siemer",
      "club_name": "1. Roundnet Club Köln",
      "avatar_url": "/api/images/proxy?url=<encoded>",
      "club_logo_url": "/api/images/proxy?url=<encoded>",
      "rgx": 1963
    }
  }
}
```

**Response `404 Not Found`** — player ID not in local DB

```json
{
  "error": "player not found"
}
```

---

### `GET /api/players/:id/today/jobs/:jobId`

Pure status lookup for a job created by `GET /api/players/:id/today` — no side effects, safe to poll. The frontend polls this **every 2 seconds** while `status` is `queued` or `running`, and stops once it becomes `done` or `error`.

**Path parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Stable player ID |
| `jobId` | integer | `job_id` returned by `GET /api/players/:id/today` |

**Response `200 OK`**

Same `JobResponse` shape as above — `status: "queued"` (with `queue_position`), `"running"`, `"done"` (with `result` and `finished_at`), or `"error"` (with `error`):

```json
{
  "job_id": 42,
  "status": "error",
  "finished_at": "2026-07-19T14:32:10+02:00",
  "error": "upstream unavailable"
}
```

**Response `404 Not Found`** — no such job for this player

```json
{
  "error": "job not found"
}
```

**`has_proxy` fields**

| Field | Meaning |
|-------|---------|
| `games[n].has_proxy` | This specific game involved at least one player with no RGX — division default was used |
| `total_delta.has_proxy` (top-level) | Any game in today's set used a proxy value |

The frontend displays a disclaimer whenever the top-level `has_proxy` is `true`.

**`was_cancelled` / `sets` fields**

| Field | Meaning |
|-------|---------|
| `games[n].was_cancelled` | The game was marked as cancelled on playerzone (forfeit, injury, etc.) and was never actually completed |
| `games[n].sets` | The set scores for this game, from our team's perspective (`our_score`/`opponent_score`), in the order played. Empty or partial for a cancelled game. |
| `games[n].result` | `"W"`/`"L"`/`"D"` as before, or `"C"` for a cancelled game |

A cancelled game always has `rgx_delta: 0` and `rgx_delta_per_player: 0` — it never contributes to the day's rating change, but is still listed so the player can see it happened.

---

### `GET /api/images/proxy`

Proxies an image from the playerzone Azure CDN (`pstaassets.blob.core.windows.net`). Exists to avoid CORS issues and hot-linking.

**Query parameters**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | Yes | Base64-encoded original image URL |

**Response `200 OK`**

- Body: raw image bytes
- `Content-Type`: matches original image (`image/jpeg`, `image/png`, etc.)
- `Cache-Control: public, max-age=604800` (7 days)
- Images are cached on disk server-side; playerzone is not re-fetched until the cache expires

**Response `400`** — `url` param missing or not valid base64

**Response `502`** — upstream image fetch failed

---

### `GET /api/stats/rgx-jobs`

All-time usage counts of `RgxJob`s created via `GET /api/players/:id/today`. Backed by the permanent `rgx_jobs_stats` table (see `03-data-requirements.md §Usage Statistics`), which survives the daily `rgx_jobs` purge. Powers the frontend footer's all-time usage stat (`AppFooter.vue`).

Counts only real, production-created jobs — cache hits (an in-flight or still-fresh job reused instead of creating a new one) and `DEV_MODE` synthetic jobs are never counted.

**Response `200 OK`**

```json
{
  "created": 4213,
  "succeeded": 4029,
  "failed": 184
}
```

| Field | Meaning |
|-------|---------|
| `created` | Total jobs ever created, regardless of current status (includes any still `queued`/`running`) |
| `succeeded` | Jobs that reached `done` |
| `failed` | Jobs that reached `error` |

---

## Response Field Reference

| Field | Type | Description |
|-------|------|-------------|
| `job_id` | integer | The `RgxJob` row ID; pass to `GET /api/players/:id/today/jobs/:jobId` to poll |
| `status` | `"no_tournament" \| "queued" \| "running" \| "done" \| "error"` | Job status. `"queued"`/`"running"` are non-final — keep polling |
| `queue_position` | integer | 1-based position among currently queued jobs; only present when `status: "queued"` |
| `result` | object | The scrape result (`TodayResult` shape); present when `status` is `"done"` or `"no_tournament"` |
| `result.status` | `"ok" \| "no_tournament"` | Result state inside `result` |
| `error` | string | Failure message; present when `status: "error"` |
| `finished_at` | string (ISO 8601) | When the job reached `done`/`error` |
| `has_proxy` | boolean | Any RGX value in the calculation was estimated from division default (new player) |
| `result.games[n].result` | `"W" \| "L" \| "D"` | Win / Loss / Draw for a single game |
| `rgx_delta` | integer (signed) | RGX change for a single game; can be negative |
| `total_delta` | integer (signed) | Sum of all `rgx_delta` values for the day |

---

## Polling Behaviour (client-side, post-hydration)

The frontend never blocks on `GET /api/players/:id/today` — it always returns immediately with a `JobResponse`.

1. Call `GET /api/players/:id/today`. If `status` is `"done"` or `"no_tournament"`, render the result — no polling needed.
2. If `status` is `"queued"` or `"running"`, start polling `GET /api/players/:id/today/jobs/:jobId` **every 2 seconds**, updating the displayed queue position each tick (rendered as a German status phrase, e.g. "Fast geschafft, du bist gleich dran...").
3. Stop polling once `status` becomes `"done"` or `"error"`.
4. To manually refresh an already-`"done"` result, call `GET /api/players/:id/today` again — the 15-minute freshness window (see above) decides whether that returns the cached result instantly or enqueues a new scrape.

This is implemented client-side by the `useTodayResult.ts` Vue composable. SSR renders the initial (immediate) job-creation response; polling only runs after hydration.

`RgxJob` rows (and their stored results) are purged daily by a cron job at 03:00 CET (same time as the player index scrape) — polling a `job_id` from a previous day will 404.

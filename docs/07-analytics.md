# RGBlitz — Analytics (Umami)

## General

- Self-hosted Umami, cookie-free. The script is injected in `frontend/app.vue` only when `NUXT_PUBLIC_UMAMI_SCRIPT_URL` and `NUXT_PUBLIC_UMAMI_WEBSITE_ID` are set.
- The app has a single route (`/`), so page views alone show almost nothing. Custom events cover the states inside it.
- All events go through `frontend/composables/useTrack.ts`. It is a silent no-op during SSR, without a configured script, and when the script is blocked.
- **Privacy rule:** no event or session property may contain a player name, player ID, club, or raw search query. `pages/datenschutz.vue` §2 describes what is collected; update it when you add an event.

## Session properties

Set once per page load via `umami.identify`:

| Property         | Meaning                                                   |
| ---------------- | --------------------------------------------------------- |
| `fast_mode`      | Battery/motion-aware fast mode active (`useBattery`)      |
| `reduced_motion` | `prefers-reduced-motion: reduce`                          |
| `standalone`     | Opened as an installed app (`display-mode: standalone`)   |

## Events

| Event               | Source                         | Properties                                                     |
| ------------------- | ------------------------------ | -------------------------------------------------------------- |
| `player-select`     | `SearchBar.vue`                | `rank` (1-based list position), `query_len`                    |
| `search-no-results` | `SearchBar.vue`                | `query_len`                                                    |
| `search-cancel`     | `SearchBar.vue` (attribute)    | —                                                              |
| `result-shown`      | `pages/index.vue`              | `state`, `games`, `delta_bucket`, `wait_s`, `queue_pos`        |
| `job-error`         | `pages/index.vue`              | `category` (backend error string, max. 50 chars)               |
| `retry`             | `pages/index.vue`              | `from`                                                         |
| `refresh`           | `pages/index.vue`              | `from`                                                         |
| `search-again`      | `pages/index.vue`              | `from`                                                         |
| `games-expand`      | `SpieleListe.vue`              | — (only on opening)                                            |
| `verdict-expand`    | `ResultView.vue`               | — (only on opening)                                            |
| `feedback-open`     | `ResultView.vue` (attribute)   | —                                                              |
| `feedback-submit`   | `FeedbackForm.vue`             | `rating` (only after a successful POST)                        |

- `result-shown` fires once each time a job request settles (select, retry or refresh). `state` is one of `ok`, `stale`, `no_games`, `no_tournament`, `error` (see `resultState` in `frontend/utils/analytics.ts`). `delta_bucket` is the per-player delta in 10-point buckets (`deltaBucket`), and is only set for `ok`/`stale`/`no_games`. `queue_pos` is the first queue position reported while polling, and is absent for a cached result.
- `from` names the view the button was in: `result`, `no_games`, `no_tournament`, `stale`, `error`, `chip`.

## Suggested Umami setup

These are configured by hand in the Umami UI:

- **Funnel:** page view `/` → `player-select` → `result-shown` (`state = ok`) → `games-expand`
- **Goal:** `feedback-submit`
- **City data:** Umami looks up the visitor's location using its bundled GeoLite2 database. If *Location → Cities* shows "Unknown", Umami is getting the ingress IP: forward `X-Forwarded-For`/`X-Real-IP` and set `CLIENT_IP_HEADER` on the Umami deployment.

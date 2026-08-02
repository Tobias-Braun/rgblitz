# RGBlitz — Project Overview

## What this is

**RGBlitz** is a single-page web app for **Roundnet Germany** players. Roundnet (branded internationally as Spikeball) is played in weekend tournaments run by roundnetgermany.de. Players are rated with an ELO-like score called **RGX**. This app lets a player search for themselves and see how their RGX changed over the course of today's tournament, updated after each of their games, without needing an account.

Roundnet Germany runs its own player portal at **playerzone.roundnetgermany.de**, but that site never shows a live, per-game RGX delta — only the final rating some time after the fact. **RGBlitz** fills that gap by scraping the same public pages a spectator would see, computing the delta itself, and presenting it in a purpose-built UI.

---

## Goals

| # | Goal |
|---|------|
| G1 | Let a player find themselves by name and see today's cumulative RGX delta at a glance |
| G2 | Show the individual games that produced that delta in a collapsible list |
| G3 | Adapt visual richness to device battery level (beautiful → fast) |
| G4 | Work fast on modern mobile phones; desktop uses the same layout |
| G5 | Require zero user accounts or persistent state |

---

## Non-Goals

- Historical RGX tracking / charts over multiple sessions or days
- Authentication or user accounts
- Support for browsers without the Battery Status API (graceful degrade to "beautiful" mode)
- Real-time push from the server — client-side polling while a scrape is in flight is sufficient

---

## Tech Stack (as implemented)

| Layer | Technology |
|-------|-----------|
| Frontend | Vue 3 + Nuxt, SSR enabled. Vanilla CSS/JS only — no UI or animation libraries |
| Backend | Go, `net/http` + chi router |
| Database | SQLite via `modernc.org/sqlite` — a single embedded file, no external DB server |
| Scheduling | `robfig/cron/v3` for the weekly player-index refresh |
| Deployment | Docker Compose: Go backend + Nuxt frontend + Caddy as reverse proxy/TLS terminator, see `docker-compose.yml` |

---

## Constraints

- All tournament and player data originates from `playerzone.roundnetgermany.de` via HTML scraping — there is no official API
- The app stores nothing beyond what is already publicly visible on playerzone (player names, club affiliation, RGX, avatar/logo images)
- Tournaments run on a single calendar day, roughly 08:00–20:00 CET
- The player index (used for autocomplete) is refreshed automatically once a week; searches never trigger a live playerzone request

## Domain Assumptions

- **Tournaments only occur on weekends (Saturday and Sunday).** There are no weekday tournaments — this is a hard domain rule with no known exceptions. Any request arriving on a weekday can immediately return `{status: "no_tournament"}` without contacting playerzone.
- The weekly player-index scrape runs **Saturday 03:00 CET** (see `internal/cron/jobs.go`) — early enough that it's done well before any Saturday tournament's matches start, so the scraped RGX values are a valid pre-tournament baseline for both days of the weekend.
- The backend also runs this same index scrape once at startup if the `players` table has fewer than 1000 rows, so a fresh deployment isn't stuck with an empty autocomplete for up to a week (see `cmd/server/main.go`).

# rgx-calc — Project Overview

## Purpose

rgx-calc is a single-page web application that lets competitive players of the "Round n' Get Germany" table-tennis league see their ELO rating change (called **RGX**) in real time on tournament days.

Data is scraped from the public site **playerzone.roundngetgermany.de** by a Go backend. The frontend is a Vue-based one-pager.

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

- Historical ELO tracking / charts over multiple sessions or days
- Authentication or user accounts
- Support for browsers without the Battery Status API (graceful degrade to "beautiful" mode)
- Real-time push from server — polling on demand is sufficient

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Frontend | Vue 3 + Nuxt (preferred) or Astro with Vue islands |
| Backend | Go (primary learning goal) |
| Database | Lightweight embedded or hosted store (e.g. SQLite or Postgres) for player index cache |
| Deployment | Single server or container; no serverless required |

---

## Constraints

- All data originates from `playerzone.roundnetgermany.de` via HTML scraping (no official API)
- The app must not store personal data beyond what is publicly visible on playerzone
- Tournament days run ~08:00–20:00 CET on a single calendar day; outside that window no live data is expected
- The weekly player-index scrape runs Friday night to minimise load on the source site

## Domain Assumptions

- **Tournaments only occur on weekends (Saturday and Sunday).** There are no weekday tournaments. This is a hard domain rule with no real exceptions.
- As a consequence, the Friday 22:00 CET player-index scrape always runs before any tournament of that weekend — the scraped RGX values are guaranteed to be the correct pre-tournament baseline.
- Any request arriving on a weekday (Monday–Friday) can immediately return `{status: "no_tournament"}` without contacting playerzone.

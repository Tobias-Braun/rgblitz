<div align="center">

<h1><img src="frontend/public/favicon.svg" width="32" height="32" valign="middle" alt="" /> RGBlitz</h1>

**See your Roundnet RGX rating change live, as your tournament happens.**

[![Docker images](https://img.shields.io/badge/ghcr.io-rgblitz-blue)](https://github.com/Tobias-Braun/rgblitz/pkgs/container/rgblitz-backend)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

</div>

---

## What is this?

If you play **Roundnet** (the sport also known internationally as *Spikeball*) in a German league tournament, your performance is tracked with a rating called **RGX** — similar in spirit to a chess ELO rating. Roundnet Germany's own player portal, *playerzone*, only shows you your RGX rating well after the fact. It never tells you, mid-tournament, "you just went up 7 points from that last game."

**RGBlitz** fills that gap. Search for your name, and during a tournament weekend you'll see your cumulative RGX change for the day updating live, plus a breakdown of exactly which games contributed what. No account, no sign-up, no tracking — just search and see your day.

The app also has its own in-app FAQ page (`frontend/pages/faq.vue`) covering the same ground as below.

---

## Frequently Asked Questions

**Do I need an account?**
No. Search your name, see your day. Nothing is saved between visits.

**Where does the data come from?**
From the same public pages anyone can already see on Roundnet Germany's `playerzone` site. RGBlitz doesn't have any special access — it reads the same tournament schedules and rankings a spectator would, and does the RGX math itself (since playerzone doesn't show it live).

**Why can't I find myself, or why does it say "no tournament"?**
A few likely reasons: tournaments only happen on Saturdays and Sundays, so any other day will correctly show "no tournament." Your name also needs to be in the weekly-refreshed player list — if you're brand new to the league, it can take up to a week to appear. If something looks wrong during an active tournament weekend, please [open an issue](../../issues/new/choose) — see below.

**Is my data safe / private?**
RGBlitz doesn't collect or store anything about *you* as a visitor — no accounts, no cookies, no analytics. It only displays information (name, club, rating) that's already publicly visible on playerzone for any competitive player.

**I found a bug, or want a feature added — what do I do?**
Please [open an issue](../../issues/new/choose)! Bug report and feature request templates are set up to make this quick — just pick the one that fits and fill in the blanks. Pull requests are welcome too.

**Is this an official Roundnet Germany product?**
No — it's an independent, unofficial companion project built by a fan of the sport.

---

## Screenshots / status

The project is in **active/alpha development** — see the in-app notice for the current state. Expect some rough edges; report anything that trips you up.

---

## For developers

### Quick facts

| | |
|---|---|
| Frontend | Vue 3 + Nuxt (SSR), no UI/animation libraries |
| Backend | Go, `chi` router, SQLite (`modernc.org/sqlite`), no external database server |
| Data source | HTML scraping of `playerzone.roundnetgermany.de` — there's no official API |
| Deployment | Docker Compose: backend + frontend + Caddy reverse proxy |

For the full technical breakdown — architecture, RGX formula, scraping algorithm, API contract, and UI state machine — see the [`docs/`](docs/) folder:

| Doc | What's in it |
|---|---|
| [`docs/01-overview.md`](docs/01-overview.md) | Goals, tech stack, domain rules (weekends-only tournaments, etc.) |
| [`docs/02-ui-requirements.md`](docs/02-ui-requirements.md) | Every UI state, battery-aware animation behaviour, mood text |
| [`docs/03-data-requirements.md`](docs/03-data-requirements.md) | The RGX formula, data entities, caching rules |
| [`docs/04-architecture.md`](docs/04-architecture.md) | Component tree, Go package layout, request/data flow |
| [`docs/05-scraping-algorithm.md`](docs/05-scraping-algorithm.md) | Step-by-step scraping logic, rate limiting, new-player handling |
| [`docs/06-api-spec.md`](docs/06-api-spec.md) | Full REST API contract |

Backend-specific setup, environment variables, and CLI tools are documented in [`backend/README.md`](backend/README.md).

### Scraping robustness policy

playerzone is a third-party site with no API and no stability contract — its HTML can
change shape at any time (a DOM tweak, or simply a page type the scraper hasn't seen
before, like a non-GRT tournament rendering without an RGX badge). Every scraper in
`internal/scraper` treats that HTML as untrusted input: unexpected, missing, or malformed
structure must never panic the process, only ever return a clear `error` or fall back to
a well-defined default. As defense in depth, every long-running goroutine that processes
scraped data (the job-queue worker, cron jobs) also recovers from any panic that slips
through anyway, turning it into a normal failure instead of crashing the server. See
`internal/scraper/robustness_test.go` for the parser-level test pattern this policy
expects new scrapers to follow.

### Running it locally

**Backend:**

```sh
cd backend
go mod download
TZ=Europe/Berlin go run ./cmd/server
```

The server listens on `:8080`. On first run with an empty database, it automatically scrapes the player index once at startup (see `docs/01-overview.md`) — or seed it offline from a bundled fixture:

```sh
go run ./cmd/seed --fixture ../scraper/fixtures/ranking.html --db ../data/players.db
```

Set `DEV_MODE=true` to bypass the weekend/tournament-window check and exercise every UI state (success, stale, error, loading) with synthetic data — see `backend/README.md` for the special player names this unlocks.

**Frontend:**

```sh
cd frontend
npm install
npm run dev
```

Nuxt runs on `:3000` and expects the Go API at `http://localhost:8080` by default (`frontend/.env`).

**Full stack via Docker Compose:**

```sh
cp .env.example .env   # then edit DOMAIN / NUXT_PUBLIC_API_BASE
docker compose up -d
```

See `docker-compose.yml` and `Caddyfile` for the production reverse-proxy setup.

### Running the tests

```sh
cd backend && go test ./...
cd frontend && npm run typecheck
```

Backend tests run entirely against HTML fixtures in `scraper/fixtures/` — no network access or live server required.

### Contributing

Issues and pull requests are welcome. Please use the provided [bug report](.github/ISSUE_TEMPLATE/bug_report.md) or [feature request](.github/ISSUE_TEMPLATE/feature_request.md) templates when opening an issue so we have what we need to help.

### License

Apache License 2.0 — see [`LICENSE`](LICENSE).

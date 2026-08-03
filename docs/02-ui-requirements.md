# RGBlitz — UI Requirements

## Layout Principles

- **Single page application (SPA)** — no route changes between views; state is managed client-side
- **Mobile-first, identical on desktop** — no responsive breakpoint divergence; the design is a centered column that caps at ~480 px on wide screens
- **No persistence** — page always starts in the search state; nothing is remembered across sessions

---

## Views / States

### State 1 — Search (initial / idle)

**Layout**
- Full-page dark background with an animated radial-gradient effect behind the search bar (plain CSS, no canvas/WebGL; must degrade gracefully in low-battery mode — see §Battery UX)
- Vertically centered search bar in the middle of the viewport

**Search bar behaviour**
- Placeholder text (German, e.g. "Spieler suchen…")
- On **focus**:
  - Bar animates to the **top of the page**, full viewport width
  - Autocomplete dropdown appears below it
  - Background gradient dims / reduces motion
- Autocomplete items each show:
  - Player avatar (image scraped from playerzone, lazy-loaded, 32 px circle)
  - Player full name
  - Club logo (image scraped from playerzone, 24 px)
- Autocomplete is driven by the weekly-cached player index (see Data doc); no live playerzone request per keystroke
- Minimum 2 characters before suggestions appear
- On **no match**: show inline message "Kein Spieler gefunden"

**Transition to result state**
- User taps/clicks an autocomplete suggestion
- Bar collapses, page transitions to State 2 or State 3

---

### State 2 — No-Tournament Day

Triggered when the selected player has no tournament registered for today (component: `NoTournamentCard.vue`).

**Layout**
- Player chip remains visible at top
- Centered message card:
  > "Es sieht so aus, als würdest du heute GRT kein Turnier spielen. Diese Seite funktioniert nur an Turnier-Tagen."
- No data displayed below

---

### State 2a — Loading (job queued/running)

Selecting a player kicks off a background scrape job on the backend (see `04-architecture.md`); the frontend shows a skeleton (component: `ResultLoading.vue`) while polling for the result.

- Skeleton circle + line, shimmer animation (disabled under `prefers-reduced-motion`)
- Below the skeleton, a German status phrase keyed off the job's queue position (`config/queue-phrases.ts`), e.g. "Fast geschafft, du bist gleich dran..." — brackets range from "being fetched right now" up to "quite a lot going on, please be patient"

### State 2b — Stale result available, live fetch failed

If a fetch attempt fails but a previously cached result exists, the user is offered a choice rather than being shown an error outright (component: `ResultStale.vue`):
- "Die Daten konnten gerade nicht live geladen werden. Du siehst einen zwischengespeicherten Stand vom letzten erfolgreichen Abruf."
- Two actions: **Erneut versuchen** (retry the live fetch) or **Trotzdem anzeigen** (accept the stale snapshot and proceed to the normal result view)

> **Current gap**: the backend doesn't yet produce `stale: true` outside of `DEV_MODE` fixtures — a real scrape failure today goes to State 2c (`ResultError.vue`) instead. See `05-scraping-algorithm.md §Not currently implemented`.

### State 2c — Hard error, no cached data

When a fetch fails and there's nothing to fall back on (component: `ResultError.vue`): retry button plus an option to search for a different player.

### State 2d — Registered for today, but no games yet

The player is confirmed to be in a tournament today, but the scraped schedule has no games in it yet — common right after the tournament starts (component: `NoGamesPlayedYet.vue`). Distinct from State 2 (no tournament at all): the player chip and a manual refresh button stay visible, since games can appear at any moment.
- "Noch keine Spiele heute" — "Du bist für ein Turnier eingetragen, aber es wurden noch keine Ergebnisse erfasst. Schau später noch mal rein."

---

### State 3 — Result (tournament day, data available)

**Layout (top to bottom)**

#### 3a. RGX Circle

- Dark inner circle (~200 px diameter on mobile)
- **Gradient glow ring** around the circle:
  - Positive delta (≥ 0): green / teal glow
  - Negative delta (< 0): red / orange glow
- Inside the circle: the **cumulative daily RGX delta** as a large number with explicit sign (`+12`, `-7`)
- Font: large, bold, high contrast against dark background

#### 3b. Mood Text

Displayed directly below the circle, centered, smaller font.  
Hard-coded German phrases selected by threshold (see §Mood Text Thresholds).

#### 3c. Spiele Accordion

- Label: **"Spiele"**
- Collapsed by default
- On expand: vertically stacked list of today's matches, each row showing:
  - Opponent name (left-aligned)
  - Match result badge: `W` (win) / `L` (loss) / `D` (draw) — color coded green / red / grey
  - RGX delta for that match (`+n` / `-n`) — color coded green (positive) / red (negative)
- Rows are ordered chronologically (earliest match first)

---

## Mood Text Thresholds

These are starter values; the exact phrases and bracket edges are configured in a static file (not hard-coded in components).

| RGX Delta Range | Suggested Phrase (German) |
|-----------------|--------------------------|
| ≥ +20 | "Heute läuft alles. Absoluter Baller-Tag." |
| +10 to +19 | "Starke Leistung. Der RGX steigt." |
| +1 to +9 | "Solider Tag. Ein paar Punkte mitgenommen." |
| 0 | "Ausgeglichen. Weder Fisch noch Fleisch." |
| -1 to -9 | "Nicht der beste Tag. Macht nichts." |
| -10 to -19 | "Heute war wohl nichts. Kommt wieder." |
| ≤ -20 | "Harter Tag. Aber morgen ist auch noch ein Tag." |

---

## Battery UX

Detected via the **Web Battery Status API** (`navigator.getBattery()`), implemented in `composables/useBattery.ts`.
Fast mode triggers when battery level **≤ 20 %**, OR the device is discharging (`charging === false`) with level **≤ 30 %**. It's also forced on whenever the OS-level `prefers-reduced-motion: reduce` media query matches, independent of battery state.

| Feature | Beautiful mode (high battery) | Fast mode (low battery) |
|---------|-------------------------------|------------------------|
| Background gradient animation | Enabled, full frame rate | Disabled — static solid color |
| Circle glow animation (pulse) | Enabled | Disabled — static border only |
| Number count-up animation on load | Enabled | Disabled — number shown immediately |
| Accordion open/close animation | CSS transition (150 ms ease) | Instant toggle, no transition |
| Autocomplete entrance animation | Fade-in slide | Instant appear |

Browsers that do not support the Battery Status API default to **beautiful mode**.

---

## Accessibility & Performance Targets

- Minimum touch target size: 44 × 44 px
- Color contrast ratio: WCAG AA (4.5:1) for all text
- First Contentful Paint target: < 1.5 s on a mid-range phone (4G)
- Images (player avatar, club logo): served with explicit `width`/`height`, lazy-loaded, WebP where possible
- Reduce motion: honour `prefers-reduced-motion` in addition to battery check (disables animations independently)

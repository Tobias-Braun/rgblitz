# rgx-calc — UI Requirements

## Layout Principles

- **Single page application (SPA)** — no route changes between views; state is managed client-side
- **Mobile-first, identical on desktop** — no responsive breakpoint divergence; the design is a centered column that caps at ~480 px on wide screens
- **No persistence** — page always starts in the search state; nothing is remembered across sessions

---

## Views / States

### State 1 — Search (initial / idle)

**Layout**
- Full-page dark background with a **shader / animated gradient effect** behind the search bar (CSS shader or canvas-based; must degrade gracefully in low-battery mode — see §Battery UX)
- Vertically centered search bar in the middle of the viewport

**Search bar behaviour**
- Placeholder text (German, e.g. "Spieler suchen…")
- On **focus**:
  - Bar animates to the **top of the page**, full viewport width
  - Autocomplete dropdown appears below it
  - Background shader dims / reduces motion
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

Triggered when the selected player has no tournament data for today.

**Layout**
- Search bar remains visible at top (collapsed, non-focused)
- Centered message card:
  > "Es sieht so aus, als würdest du heute kein Turnier spielen. Diese Seite funktioniert nur an Turnier-Tagen."
- No data displayed below

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

Detected via the **Web Battery Status API** (`navigator.getBattery()`).  
Threshold for switching to fast mode: battery level **≤ 20 %** OR `charging === false && level ≤ 0.3` (exact threshold TBD).

| Feature | Beautiful mode (high battery) | Fast mode (low battery) |
|---------|-------------------------------|------------------------|
| Background shader / animated gradient | Enabled, full frame rate | Disabled — static solid color |
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

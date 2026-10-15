export type DeltaTone = 'positive' | 'negative' | 'neutral'

export function deltaTone(delta: number): DeltaTone {
  if (delta > 0) return 'positive'
  if (delta < 0) return 'negative'
  return 'neutral'
}

// Unspaced, explicit sign, minus rendered as U+2212 to match the design's
// typographic minus rather than a hyphen. Rounds to the nearest .5 first to
// absorb float32->JS number noise, so per-player deltas always print as a
// clean integer or an exact X.5 half-integer.
export function formatDelta(delta: number): string {
  const rounded = Math.round(delta * 2) / 2
  if (rounded > 0) return `+${rounded}`
  if (rounded < 0) return `−${Math.abs(rounded)}`
  return '±0'
}

// Backend image URLs (avatar_url, club_logo_url) are relative paths pointing
// at the Go backend's own /api/images/proxy route, not the Nuxt server. Only
// prefix relative paths — absolute URLs (e.g. in dev fixtures) pass through.
export function withApiBase(path: string, apiBase: string): string {
  if (!path || /^https?:\/\//.test(path)) return path
  return `${apiBase}${path}`
}

// Abbreviates a non-negative integer to at most 4 characters for the header's
// "Happily served N RGX changes" stat, e.g. 1074 -> "1.1K", 999500 -> "1.0M".
// One decimal is used while the scaled value is still a single digit (keeps
// "1.1K" at 4 chars); above that the decimal is dropped ("12K"). Rounding can
// push a value across a unit boundary (999.95K -> "1000K"), so the result is
// re-escalated to the next unit whenever that happens.
export function formatCompactCount(n: number): string {
  if (n < 1000) return String(Math.round(n))

  const units = ['K', 'M', 'B', 'T']
  let value = n
  let unitIndex = -1
  while (value >= 1000 && unitIndex < units.length - 1) {
    value /= 1000
    unitIndex++
  }

  // toFixed(1) can itself round a value like 9.999 up into double digits
  // ("10.0") — fall back to the no-decimal form when that happens.
  const render = (v: number): string => {
    if (v < 10) {
      const withDecimal = v.toFixed(1)
      return parseFloat(withDecimal) < 10 ? withDecimal : String(Math.round(v))
    }
    return String(Math.round(v))
  }
  let display = render(value)

  if (parseFloat(display) >= 1000 && unitIndex < units.length - 1) {
    value /= 1000
    unitIndex++
    display = render(value)
  }

  return `${display}${units[unitIndex]}`
}

// Renders a YYYY-MM-DD range as a German date, stating the month and year only
// once when both ends share them: "25.–26.07.2026", "31.07.–01.08.2026".
export function formatDateRange(start: string, end: string): string {
  const [sy, sm, sd] = start.split('-')
  const [ey, em, ed] = end.split('-')
  const endText = `${ed}.${em}.${ey}`
  if (start === end) return endText
  if (sy !== ey) return `${sd}.${sm}.${sy}–${endText}`
  if (sm !== em) return `${sd}.${sm}.–${endText}`
  return `${sd}.–${endText}`
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return ''
  if (parts.length === 1) return parts[0]!.slice(0, 2).toUpperCase()
  return (parts[0]![0]! + parts[parts.length - 1]![0]!).toUpperCase()
}

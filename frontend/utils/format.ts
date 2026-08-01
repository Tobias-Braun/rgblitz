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

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return ''
  if (parts.length === 1) return parts[0]!.slice(0, 2).toUpperCase()
  return (parts[0]![0]! + parts[parts.length - 1]![0]!).toUpperCase()
}

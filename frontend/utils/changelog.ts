import changelog from '~/changelog.json'

// CI prepends one entry per merge to main (scripts/release.mjs), so the
// newest entry is always the deployed version.
export const appVersion = changelog[0]!.version

// Internal releases (CI, refactors, dev tooling) still get a version but
// aren't worth a user's attention.
export const visibleReleases = changelog.filter((release) => !release.internal)

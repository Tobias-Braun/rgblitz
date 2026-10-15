// Prepends a release to frontend/changelog.json and prints the new version.
// Usage: node release.mjs <claude-output.json> [--major]
import { readFileSync, writeFileSync } from 'node:fs'

const changelogPath = new URL('../../frontend/changelog.json', import.meta.url)
const [claudeOutputPath, ...flags] = process.argv.slice(2)

const { bump, internal, changes } = JSON.parse(readFileSync(claudeOutputPath, 'utf8')).structured_output ?? {}
if (!['minor', 'patch'].includes(bump) || typeof internal !== 'boolean' || !changes?.length) {
  throw new Error(`unexpected Claude output in ${claudeOutputPath}`)
}

const changelog = JSON.parse(readFileSync(changelogPath, 'utf8'))
const [major, minor, patch] = changelog[0].version.split('.').map(Number)
// Major is the owner's call (release:major marker), never Claude's.
const version = flags.includes('--major')
  ? `${major + 1}.0.0`
  : bump === 'minor'
    ? `${major}.${minor + 1}.0`
    : `${major}.${minor}.${patch + 1}`

const date = new Date().toLocaleDateString('sv-SE', { timeZone: 'Europe/Berlin' })
changelog.unshift({ version, date, internal, changes })
writeFileSync(changelogPath, JSON.stringify(changelog, null, 2) + '\n')
console.log(version)

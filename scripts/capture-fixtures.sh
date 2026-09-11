#!/usr/bin/env bash
#
# Captures fresh playerzone fixtures for the rgblitz backend during tournament hours.
#
# Tournaments run Saturday and Sunday only, so this script does nothing unless it is a
# weekend and the local (Europe/Berlin) hour is inside the capture window — by default
# 09:00 through 18:00 inclusive. That makes it safe to run from cron every hour, or to
# leave running in --watch mode, which sleeps until each top of the hour and captures
# itself (no cron entry needed).
#
#   ./scripts/capture-fixtures.sh --watch          # every tournament running today, hourly
#   ./scripts/capture-fixtures.sh                  # single window-gated run
#   ./scripts/capture-fixtures.sh --force          # ignore the window, capture now
#   ./scripts/capture-fixtures.sh --players LUC2110  # narrow it to one player's scrape
#
# With no --players it captures each tournament running today in full — every division,
# every team, every game, with the RGX deltas the backend computes for them and a set of
# consistency checks over both sides of every game. Each run writes a snapshot under
# scraper/captures/<date>/<HHMM>-t<tournament-id>/ (or -<player-id> in player mode):
# the fetched HTML, a manifest that replays it offline, capture.json, and a readable
# report.txt. See backend/cmd/fixturecapture for what exactly lands in a snapshot.
#
# Cron equivalent of --watch (hourly, the script's own window check handles the rest):
#   0 * * * * /path/to/rgblitz/scripts/capture-fixtures.sh >> /tmp/rgblitz-capture.log 2>&1

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PLAYERS="${CAPTURE_PLAYERS:-}"
OUT_DIR="${CAPTURE_DIR:-$REPO_ROOT/scraper/captures}"
START_HOUR="${CAPTURE_START_HOUR:-9}"
END_HOUR="${CAPTURE_END_HOUR:-18}"
WATCH=false
FORCE=false

usage() {
  sed -n '2,23p' "${BASH_SOURCE[0]}" | sed 's/^#\{0,1\} \{0,1\}//'
  cat <<'EOF'

Options:
  --players LIST   comma-separated player IDs or names (or $CAPTURE_PLAYERS); omit to
                   capture every tournament running today in full
  --out DIR        capture root directory (default: <repo>/scraper/captures)
  --start HOUR     first hour of the capture window, 0-23 (default: 9)
  --end HOUR       last hour of the capture window, inclusive, 0-23 (default: 18)
  --watch          stay running and capture at every top of the hour inside the window
  --force          capture now even outside the weekend/hour window
  -h, --help       show this help

Anything after -- is passed straight through to cmd/fixturecapture, e.g.
  ./scripts/capture-fixtures.sh --force -- --divisions "Advanced,Pro"
EOF
}

PASSTHROUGH=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --players) PLAYERS="$2"; shift 2 ;;
    --out) OUT_DIR="$2"; shift 2 ;;
    --start) START_HOUR="$2"; shift 2 ;;
    --end) END_HOUR="$2"; shift 2 ;;
    --watch) WATCH=true; shift ;;
    --force) FORCE=true; shift ;;
    -h|--help) usage; exit 0 ;;
    --) shift; PASSTHROUGH=("$@"); break ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

# No --players means whole-tournament mode, which is the point of running this unattended:
# it needs no advance knowledge of who is playing today.
CAPTURE_ARGS=(--out "$OUT_DIR")
if [[ -n "$PLAYERS" ]]; then
  CAPTURE_ARGS+=(--players "$PLAYERS")
fi
CAPTURE_ARGS+=("${PASSTHROUGH[@]+"${PASSTHROUGH[@]}"}")

# All scheduling decisions are made in Europe/Berlin, the timezone the backend and the
# tournaments themselves run in — the machine's own timezone is irrelevant here.
export TZ="Europe/Berlin"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S %Z')] $*"; }

# in_window reports whether now is a Saturday/Sunday inside [START_HOUR, END_HOUR].
in_window() {
  local dow hour
  dow="$(date +%u)"   # 1=Monday .. 7=Sunday
  hour="$(date +%-H)" # no leading zero, so 09 doesn't parse as octal
  [[ "$dow" -ge 6 ]] && [[ "$hour" -ge "$START_HOUR" ]] && [[ "$hour" -le "$END_HOUR" ]]
}

capture() {
  if [[ -n "$PLAYERS" ]]; then
    log "capturing players: $PLAYERS"
  else
    log "capturing every tournament running today"
  fi
  # A failed capture (playerzone down, player not in the index) must not kill --watch —
  # the next hour is very likely to succeed, and the failure is already logged by the tool.
  if (cd "$REPO_ROOT/backend" && go run ./cmd/fixturecapture "${CAPTURE_ARGS[@]}"); then
    log "capture finished"
  else
    log "capture FAILED (exit $?) — continuing"
  fi
}

# sleep_until_next_hour blocks until the next top of the hour, plus a few seconds of margin
# so the capture lands after the hour has definitely rolled over.
sleep_until_next_hour() {
  local secs
  secs=$(( 3600 - (10#$(date +%M) * 60 + 10#$(date +%S)) + 5 ))
  log "sleeping ${secs}s until the next hour"
  sleep "$secs"
}

if [[ "$WATCH" == true ]]; then
  log "watch mode: weekends ${START_HOUR}:00–${END_HOUR}:59 (Europe/Berlin), output → $OUT_DIR"
  while true; do
    if [[ "$FORCE" == true ]] || in_window; then
      capture
    fi
    sleep_until_next_hour
  done
fi

if [[ "$FORCE" == true ]] || in_window; then
  capture
else
  log "outside the capture window (weekends ${START_HOUR}:00–${END_HOUR}:59) — nothing to do"
fi

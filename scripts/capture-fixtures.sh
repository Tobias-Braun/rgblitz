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
#   ./scripts/capture-fixtures.sh --players "PAULCHEN,Anna Beispiel" --watch
#   ./scripts/capture-fixtures.sh --players PAULCHEN            # single run, window-gated
#   ./scripts/capture-fixtures.sh --players PAULCHEN --force    # ignore the window
#
# Each run writes a snapshot per player under scraper/captures/<date>/<HHMM>-<player>/ —
# the fetched HTML (named so the directory replays as a FIXTURE_DIR), capture.json, and a
# readable report.txt with every parsed intermediate and RGX formula value. See
# backend/cmd/fixturecapture for the details of what lands in a snapshot.
#
# Cron equivalent of --watch (hourly, the script's own window check handles the rest):
#   0 * * * * /path/to/rgblitz/scripts/capture-fixtures.sh --players PAULCHEN >> /tmp/rgblitz-capture.log 2>&1

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PLAYERS="${CAPTURE_PLAYERS:-}"
OUT_DIR="${CAPTURE_DIR:-$REPO_ROOT/scraper/captures}"
START_HOUR="${CAPTURE_START_HOUR:-9}"
END_HOUR="${CAPTURE_END_HOUR:-18}"
WATCH=false
FORCE=false

usage() {
  sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^#\{0,1\} \{0,1\}//'
  cat <<'EOF'

Options:
  --players LIST   comma-separated player IDs or names (or $CAPTURE_PLAYERS) — required
  --out DIR        capture root directory (default: <repo>/scraper/captures)
  --start HOUR     first hour of the capture window, 0-23 (default: 9)
  --end HOUR       last hour of the capture window, inclusive, 0-23 (default: 18)
  --watch          stay running and capture at every top of the hour inside the window
  --force          capture now even outside the weekend/hour window
  -h, --help       show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --players) PLAYERS="$2"; shift 2 ;;
    --out) OUT_DIR="$2"; shift 2 ;;
    --start) START_HOUR="$2"; shift 2 ;;
    --end) END_HOUR="$2"; shift 2 ;;
    --watch) WATCH=true; shift ;;
    --force) FORCE=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$PLAYERS" ]]; then
  echo "error: --players is required (or set CAPTURE_PLAYERS)" >&2
  usage >&2
  exit 2
fi

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
  log "capturing: $PLAYERS"
  # A failed capture (playerzone down, player not in the index) must not kill --watch —
  # the next hour is very likely to succeed, and the failure is already logged by the tool.
  if (cd "$REPO_ROOT/backend" && go run ./cmd/fixturecapture --players "$PLAYERS" --out "$OUT_DIR"); then
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

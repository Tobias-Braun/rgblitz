#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -f .env ]; then
  echo "Missing .env — copy .env.example to .env and fill in DOMAIN / NUXT_PUBLIC_API_BASE first." >&2
  exit 1
fi

docker compose up -d --build

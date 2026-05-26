#!/usr/bin/env bash
# Fetch free proxy list used by go-ingestion/proxy loader.
# Sources defaults from deploy/.env (canonical env file in repo root).
# Override any variable by setting it in the environment or deploy/.env.
#
# The proxy list is NOT committed because:
#   - it is a live upstream feed that should be refreshed before each run,
#   - it is ephemeral — most proxies have a short lifetime.
#
# Called by: make fetch-proxies, make bootstrap, or manually before make ingestion.

set -euo pipefail

# ── Source canonical .env ──────────────────────────────────────────
ENV_FILE="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/deploy/.env"
if [ -f "$ENV_FILE" ]; then
  set -a
  # shellcheck source=../../deploy/.env
  . "$ENV_FILE"
  set +a
fi

DEST="go-ingestion/config/proxy.txt"
URL="${URL:-https://raw.githubusercontent.com/iplocate/free-proxy-list/refs/heads/main/all-proxies.txt}"

mkdir -p "$(dirname "$DEST")"

echo "Fetching proxies into $DEST ..."
printf '  -> %s ... ' "$URL"

if curl -sS --connect-timeout 15 --max-time 60 "$URL" -o "$DEST.tmp"; then
  lines=$(wc -l < "$DEST.tmp")
  if [ "$lines" -eq 0 ]; then
    echo "EMPTY (no proxies returned)"
    rm -f "$DEST.tmp"
    exit 1
  fi
  mv "$DEST.tmp" "$DEST"
  echo "OK — $lines proxies written"
else
  echo "FAILED"
  rm -f "$DEST.tmp"
  exit 1
fi

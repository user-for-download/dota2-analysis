#!/usr/bin/env bash
# Fetch dotaconstants JSON files used by go-ingestion/enrich.
# Sources defaults from deploy/.env (canonical env file in repo root).
# Override any variable by setting it in the environment or deploy/.env.
#
# These files are NOT committed because:
#   - they are ~3 MB total,
#   - they are upstream artifacts that should be refreshed,
#   - they bloat clone time.
#
# Called by: make fetch-constants, Docker build

set -euo pipefail

# ── Source canonical .env ──────────────────────────────────────────
ENV_FILE="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/deploy/.env"
if [ -f "$ENV_FILE" ]; then
  set -a
  # shellcheck source=../../deploy/.env
  . "$ENV_FILE"
  set +a
fi

DEST="${DEST:-go-ingestion/assets/dotaconstants}"
BASE_URL="${BASE_URL:-https://raw.githubusercontent.com/odota/dotaconstants/master/build}"

FILES=(
  "abilities.json"
  "ability_ids.json"
  "game_mode.json"
  "hero_abilities.json"
  "heroes.json"
  "item_ids.json"
  "items.json"
  "lobby_type.json"
  "patch.json"
  "region.json"
)

mkdir -p "$DEST"

echo "Fetching dotaconstants into $DEST ..."
for f in "${FILES[@]}"; do
  url="$BASE_URL/$f"
  out="$DEST/$f"
  printf '  -> %s ... ' "$f"
  if curl -fsSL --connect-timeout 10 --max-time 30 "$url" -o "$out.tmp"; then
    if command -v jq &>/dev/null; then
      jq -e . "$out.tmp" >/dev/null 2>&1 || { echo "INVALID"; rm -f "$out.tmp"; exit 1; }
    fi
    mv "$out.tmp" "$out"
    echo "OK"
  else
    echo "FAILED"
    rm -f "$out.tmp"
    exit 1
  fi
done

echo "Done. $(find "$DEST" -maxdepth 1 -name '*.json' | wc -l) files written to $DEST/"

#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────
# publish-core.sh — Tag and publish go-dota2-core module
# ─────────────────────────────────────────────────────────────
# Usage: ./scripts/publish-core.sh <version>
#   version: semver tag like v1.0.0 or v1.1.0
#
# This script:
#   1. Validates the version format
#   2. Tags the go-core module in its git repo
#   3. Pushes the tag to origin
#   4. Prints instructions for downstream projects
# ─────────────────────────────────────────────────────────────
set -euo pipefail

CORE_DIR="$(cd "$(dirname "$0")/../go-core" && pwd)"
VERSION="${1:-}"

if [ -z "$VERSION" ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 v1.0.0"
    exit 1
fi

if ! echo "$VERSION" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "Error: version must be a semver tag (e.g., v1.0.0)"
    exit 1
fi

echo "=== Publishing go-dota2-core $VERSION ==="

cd "$CORE_DIR"

# Ensure we're in a git repo
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "Error: go-core is not a git repository"
    echo "Run: cd go-core && git init && git add . && git commit -m 'Initial commit'"
    exit 1
fi

# Ensure working tree is clean
if ! git diff-index --quiet HEAD --; then
    echo "Error: working tree is dirty. Commit or stash changes first."
    exit 1
fi

# Tag and push
echo "Tagging $VERSION..."
git tag -a "$VERSION" -m "go-dota2-core $VERSION"
git push origin "$VERSION"

echo ""
echo "=== Tag $VERSION pushed ==="
echo ""
echo "Next steps for downstream projects:"
echo ""
echo "  # In go-ingestion/:"
echo "  cd go-ingestion"
echo "  go get github.com/user-for-download/go-dota2-core@$VERSION"
echo "  # Remove 'replace' line from go.mod"
echo "  go mod vendor"
echo ""
echo "  # In go-analysis/:"
echo "  cd go-analysis"
echo "  go get github.com/user-for-download/go-dota2-core@$VERSION"
echo "  # Remove 'replace' line from go.mod"
echo "  go mod vendor"
echo ""
echo "Then commit the updated go.mod, go.sum, and vendor/ directories."
echo "Run 'make build' to verify Docker images build correctly."

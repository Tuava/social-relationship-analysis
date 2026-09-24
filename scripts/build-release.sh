#!/usr/bin/env bash
set -euo pipefail
# Build only on GitHub-hosted CI; the project owner's workstation is not a build host.
[ "${GITHUB_ACTIONS:-}" = true ] || { echo 'Run the Verify and release workflow on GitHub to build packages.' >&2; exit 1; }
: "${GOOS:?}" "${GOARCH:?}" "${VERSION:?}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="$ROOT/release/sra-$GOOS-$GOARCH"
mkdir -p "$DEST/backend/internal/collectors/napcat" "$DEST/frontend" "$DEST/scripts/lib" "$DEST/docs"
cd "$ROOT/backend"
export CGO_ENABLED=0
go build -trimpath -ldflags='-s -w' -o "$DEST/backend/server" ./cmd/server
go build -trimpath -ldflags='-s -w' -o "$DEST/backend/mcp-server" ./cmd/mcp
go build -trimpath -ldflags='-s -w' -o "$DEST/backend/backfill-text-window" ./cmd/backfill_text_window
cp -R migrations "$DEST/backend/"
cp internal/collectors/napcat/openapi-4.18.18.json "$DEST/backend/internal/collectors/napcat/"
cp .env.example "$DEST/backend/"
cp -R "$ROOT/frontend/dist" "$DEST/frontend/"
cp "$ROOT/scripts/run-release.sh" "$ROOT/scripts/configure-release.sh" "$ROOT/scripts/mcp-server.sh" "$DEST/scripts/"
cp "$ROOT/scripts/lib/env.sh" "$DEST/scripts/lib/"
cp "$ROOT/README.md" "$ROOT/SECURITY.md" "$ROOT/THIRD_PARTY_NOTICES.md" "$DEST/"
cp "$ROOT/docs/release-install.md" "$ROOT/docs/release-notes.md" "$ROOT/docs/sql-reader.md" "$ROOT/docs/release-review.md" "$DEST/docs/"
printf '%s\n' "$VERSION" > "$DEST/VERSION"
cd "$ROOT/release"
tar -czf "sra-$GOOS-$GOARCH.tar.gz" "sra-$GOOS-$GOARCH"

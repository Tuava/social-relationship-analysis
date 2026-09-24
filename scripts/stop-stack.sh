#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Each stop script is ownership-aware and leaves unrelated listeners alone.
"$ROOT_DIR/scripts/stop-qzone.sh" || true
"$ROOT_DIR/scripts/stop-frontend.sh" || true
"$ROOT_DIR/scripts/stop-server.sh" || true

echo "SRA local stack stopped. PostgreSQL and external NapCat were not changed."

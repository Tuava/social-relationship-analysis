#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[ -x "$ROOT/backend/server" ] || { echo 'Download and extract a platform release package first.' >&2; exit 1; }
[ -f "$ROOT/backend/.env" ] || { echo 'Run ./scripts/configure-release.sh and set DATABASE_URL first.' >&2; exit 1; }
source "$ROOT/scripts/lib/env.sh"
load_env_file "$ROOT/backend/.env"
export FRONTEND_DIST="$ROOT/frontend/dist"
export MIGRATIONS_DIR="$ROOT/backend/migrations"
export NAPCAT_OPENAPI_PATH="$ROOT/backend/internal/collectors/napcat/openapi-4.18.18.json"
export OBJECT_ROOT="${OBJECT_ROOT:-$ROOT/data/objects}"
cd "$ROOT/backend"
exec "$ROOT/backend/server"

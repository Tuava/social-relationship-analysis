#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT_DIR/backend/.env"

# shellcheck source=scripts/lib/env.sh
source "$ROOT_DIR/scripts/lib/env.sh"

if [ ! -f "$ENV_FILE" ]; then
  echo "backend/.env is missing; run scripts/configure-release.sh to initialize local credentials" >&2
  exit 1
fi

load_env_file "$ENV_FILE"

cd "$ROOT_DIR/backend"
exec "$ROOT_DIR/backend/mcp-server" "$@"

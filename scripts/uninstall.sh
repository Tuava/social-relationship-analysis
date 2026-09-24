#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT_DIR/backend/.env"
# shellcheck source=scripts/lib/env.sh
source "$ROOT_DIR/scripts/lib/env.sh"
REMOVE_CONFIG=0
REMOVE_DATA=0
REMOVE_DB=0
PURGE=0
YES=0
DRY_RUN=0

usage() {
  cat <<'EOF'
Usage: ./scripts/uninstall.sh [options]

Stop SRA and remove generated runtime artifacts. User data and PostgreSQL are
preserved by default.

Options:
  --remove-config   Also remove backend/.env (credentials and local settings).
  --remove-data     Also remove data/ (messages, media and raw evidence).
  --remove-db       Drop the configured PostgreSQL database after confirmation.
  --purge           Equivalent to all three destructive options.
  --yes             Confirm destructive actions (required with --purge in CI).
  --dry-run         List actions without stopping or deleting anything.
  -h, --help        Show this help.

Database deletion is never implicit. It affects the configured database only;
it does not uninstall PostgreSQL or touch other databases.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --remove-config) REMOVE_CONFIG=1 ;;
    --remove-data) REMOVE_DATA=1 ;;
    --remove-db) REMOVE_DB=1 ;;
    --purge) PURGE=1; REMOVE_CONFIG=1; REMOVE_DATA=1; REMOVE_DB=1 ;;
    --yes) YES=1 ;;
    --dry-run) DRY_RUN=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

if [ "$PURGE" -eq 1 ] && [ "$YES" -eq 0 ] && [ "$DRY_RUN" -eq 0 ]; then
  echo "--purge is intentionally non-interactive only with --yes." >&2
  exit 2
fi

DB_NAME=""
ADMIN_DATABASE_URL=""
if [ -f "$ENV_FILE" ]; then
  load_env_file "$ENV_FILE"
  DB_NAME="${SRA_DB_NAME:-}"
  if [ -z "$DB_NAME" ]; then
    case "${DATABASE_URL:-}" in
      postgres://*|postgresql://*)
        if [[ "$DATABASE_URL" =~ ^(postgres(ql)?://[^/]+)(/[^?]*)?(\?.*)?$ ]] && [ -n "${BASH_REMATCH[3]:-}" ]; then
          DB_NAME="${BASH_REMATCH[3]#/}"
        fi
        ;;
    esac
  fi
  if [ -z "$DB_NAME" ] && command -v psql >/dev/null 2>&1; then
    DB_NAME="$(psql --dbname="$DATABASE_URL" -Atqc 'SELECT current_database()' 2>/dev/null || true)"
  fi
  case "${DATABASE_URL:-}" in
    postgres://*|postgresql://*)
      if [[ "$DATABASE_URL" =~ ^(postgres(ql)?://[^/]+)(/[^?]*)?(\?.*)?$ ]]; then
        ADMIN_DATABASE_URL="${BASH_REMATCH[1]}/postgres${BASH_REMATCH[4]:-}"
      fi
      ;;
  esac
fi

echo "The following SRA-owned runtime artifacts will be removed:"
printf '  - backend/server, backend/mcp-server, frontend PID files\n'
printf '  - backend/server.log, frontend/frontend.log and data/logs/qzone.log\n'
[ "$REMOVE_CONFIG" -eq 1 ] && printf '  - backend/.env\n'
[ "$REMOVE_DATA" -eq 1 ] && printf '  - data/ (ALL local evidence, media and logs)\n'
[ "$REMOVE_DB" -eq 1 ] && printf '  - PostgreSQL database: %s\n' "${DB_NAME:-<could not resolve from .env>}"

if [ "$DRY_RUN" -eq 1 ]; then
  echo "Dry run only; nothing changed."
  exit 0
fi

if [ "$REMOVE_DB" -eq 1 ] && [ "$YES" -eq 0 ]; then
  printf 'Type DELETE to drop the database and continue: '
  read -r CONFIRM
  [ "$CONFIRM" = "DELETE" ] || { echo "Cancelled; no database was dropped."; REMOVE_DB=0; }
fi

"$ROOT_DIR/scripts/stop-server.sh" || true
"$ROOT_DIR/scripts/stop-frontend.sh" || true
"$ROOT_DIR/scripts/stop-qzone.sh" || true

rm -f "$ROOT_DIR/backend/server" "$ROOT_DIR/backend/mcp-server" \
  "$ROOT_DIR/backend/server.log" "$ROOT_DIR/backend/server.pid" \
  "$ROOT_DIR/frontend/frontend.log" "$ROOT_DIR/frontend/frontend.pid" \
  "$ROOT_DIR/data/logs/qzone.log" "$ROOT_DIR/data/run/qzone.pid"

if [ "$REMOVE_CONFIG" -eq 1 ]; then
  rm -f "$ENV_FILE"
fi
if [ "$REMOVE_DATA" -eq 1 ]; then
  rm -rf "$ROOT_DIR/data"
fi
if [ "$REMOVE_DB" -eq 1 ]; then
  [ -n "$DB_NAME" ] || { echo "Cannot resolve database name from backend/.env; database was not dropped." >&2; exit 1; }
  command -v psql >/dev/null 2>&1 || { echo "psql is required to drop $DB_NAME" >&2; exit 1; }
  [ -n "$ADMIN_DATABASE_URL" ] || { echo "Database deletion requires DATABASE_URL to be a postgres:// or postgresql:// URI." >&2; exit 1; }
  psql --dbname="$ADMIN_DATABASE_URL" -v ON_ERROR_STOP=1 -v db_name="$DB_NAME" -c \
    'SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = :'"'"'db_name'"'"' AND pid <> pg_backend_pid();' >/dev/null
  # PostgreSQL does not accept a variable as an identifier in DROP DATABASE;
  # generate the safely identifier-quoted statement inside psql instead.
  psql --dbname="$ADMIN_DATABASE_URL" -v ON_ERROR_STOP=1 -v db_name="$DB_NAME" -c \
    'SELECT format($$DROP DATABASE IF EXISTS %I$$, :'"'"'db_name'"'"') \gexec'
fi

echo "SRA runtime artifacts removed. PostgreSQL and user data were preserved unless explicitly requested above."

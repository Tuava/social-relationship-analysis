#!/usr/bin/env bash
set -euo pipefail

# Safe, repeatable local deployment helper. It prepares dependencies and build
# artifacts, but never starts collection or overwrites an existing .env.
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
FRONTEND_DIR="$ROOT_DIR/frontend"
ENV_FILE="$BACKEND_DIR/.env"
# shellcheck source=scripts/lib/env.sh
source "$ROOT_DIR/scripts/lib/env.sh"
CHECK_ONLY=0
CREATE_DB=0
SKIP_FRONTEND=0
SKIP_BACKEND=0
SHOW_CREDENTIALS=0
START_BACKEND=0
CHECK_STACK=0

usage() {
  cat <<'EOF'
Usage: ./scripts/deploy.sh [options]

Prepare a local SRA deployment without touching existing data or configuration.

Options:
  --check              Validate tools, configuration and PostgreSQL only.
  --check-stack        Read-only check of PostgreSQL, NapCat, QZone, backend,
                       frontend and MCP without building or starting anything.
  --create-db          Create the configured database if it does not exist.
  --skip-backend       Skip Go build and backend tests.
  --skip-frontend      Skip npm install, typecheck and frontend build.
  --start              Start the backend after a successful preparation.
  --show-credentials   Print the local admin username/password once.
  -h, --help           Show this help.

The script never starts QZone, starts no collection, and never overwrites
backend/.env. Use --create-db only when the configured PostgreSQL account is
allowed to create databases.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --check) CHECK_ONLY=1 ;;
    --check-stack) CHECK_STACK=1 ;;
    --create-db) CREATE_DB=1 ;;
    --skip-backend) SKIP_BACKEND=1 ;;
    --skip-frontend) SKIP_FRONTEND=1 ;;
    --show-credentials) SHOW_CREDENTIALS=1 ;;
    --start) START_BACKEND=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

if [ "$CHECK_STACK" -eq 1 ]; then
  exec "$ROOT_DIR/scripts/check-stack.sh"
fi

die() { echo "deploy: $*" >&2; exit 1; }
need_command() { command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"; }

admin_database_url() {
  # DATABASE_URL is normally a PostgreSQL URI. When the target database does
  # not exist, connect to the maintenance database without exposing the URI
  # or password in output. For non-URI libpq connection strings, the caller
  # must create the database beforehand because there is no safe generic way
  # to rewrite an arbitrary connection string here.
  case "$DATABASE_URL" in
    postgres://*|postgresql://*)
      if [[ "$DATABASE_URL" =~ ^(postgres(ql)?://[^/]+)(/[^?]*)?(\?.*)?$ ]]; then
        printf '%s/postgres%s\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[4]:-}"
        return 0
      fi
      ;;
  esac
  return 1
}

target_database_name() {
  if [ -n "${SRA_DB_NAME:-}" ]; then
    printf '%s\n' "$SRA_DB_NAME"
    return 0
  fi
  case "$DATABASE_URL" in
    postgres://*|postgresql://*)
      if [[ "$DATABASE_URL" =~ ^(postgres(ql)?://[^/]+)(/[^?]*)?(\?.*)?$ ]] && [ -n "${BASH_REMATCH[3]:-}" ]; then
        # The URI form used by this project has a plain database name. Keep
        # this deliberately conservative instead of attempting URL decoding.
        printf '%s\n' "${BASH_REMATCH[3]#/}"
        return 0
      fi
      ;;
  esac
  printf '%s\n' "social_relationship_analysis"
}

need_command go
need_command npm
need_command curl
need_command openssl
need_command psql

if [ ! -f "$ENV_FILE" ]; then
  [ "$CHECK_ONLY" -eq 0 ] || die "backend/.env is missing (run without --check once to create it)"
  umask 077
  cat > "$ENV_FILE" <<EOF
DATABASE_URL=postgres://localhost:5432/social_relationship_analysis?sslmode=disable
HTTP_ADDR=127.0.0.1:8000
JWT_SECRET=$(openssl rand -hex 32)
SRA_CONFIG_ENCRYPTION_KEY=$(openssl rand -hex 32)
ADMIN_USERNAME=admin
ADMIN_PASSWORD=$(openssl rand -hex 24)
OBJECT_ROOT=../data/objects
NAPCAT_MEDIA_ROOT=
NAPCAT_OPENAPI_PATH=internal/collectors/napcat/openapi-4.18.18.json
CORS_ORIGIN=http://localhost:5173
EOF
  chmod 600 "$ENV_FILE"
  echo "Created $ENV_FILE (permissions 600)."
fi

load_env_file "$ENV_FILE"
: "${DATABASE_URL:?DATABASE_URL is missing in backend/.env}"
: "${HTTP_ADDR:=127.0.0.1:8000}"
: "${JWT_SECRET:?JWT_SECRET is missing in backend/.env}"
: "${SRA_CONFIG_ENCRYPTION_KEY:?SRA_CONFIG_ENCRYPTION_KEY is missing in backend/.env}"
: "${ADMIN_USERNAME:=admin}"
: "${ADMIN_PASSWORD:?ADMIN_PASSWORD is missing in backend/.env}"

echo "Checking PostgreSQL connection..."
if ! psql --dbname="$DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc 'SELECT 1' >/dev/null 2>&1; then
  if [ "$CREATE_DB" -eq 0 ]; then
    die "cannot connect to DATABASE_URL; start PostgreSQL or rerun with --create-db after checking credentials"
  fi
  DB_NAME="$(target_database_name)"
  echo "Creating database '$DB_NAME' if needed..."
  ADMIN_DATABASE_URL="$(admin_database_url || true)"
  [ -n "$ADMIN_DATABASE_URL" ] || die "--create-db currently requires DATABASE_URL to be a postgres:// or postgresql:// URI"
  psql --dbname="$ADMIN_DATABASE_URL" -v ON_ERROR_STOP=1 -v db_name="$DB_NAME" -Atqc \
    'SELECT format($$CREATE DATABASE %I$$, :'"'"'db_name'"'"') WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'"'"'db_name'"'"')' \
    | while IFS= read -r statement; do [ -z "$statement" ] || psql --dbname="$ADMIN_DATABASE_URL" -v ON_ERROR_STOP=1 -c "$statement"; done
  psql --dbname="$DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc 'SELECT 1' >/dev/null \
    || die "database '$DB_NAME' was not reachable after creation"
fi

if [ "$CHECK_ONLY" -eq 1 ]; then
  echo "Configuration and PostgreSQL checks passed. No files were built or started."
  [ "$SHOW_CREDENTIALS" -eq 1 ] && printf 'Admin username: %s\nAdmin password: %s\n' "$ADMIN_USERNAME" "$ADMIN_PASSWORD"
  exit 0
fi

if [ "$SKIP_BACKEND" -eq 0 ]; then
  echo "Testing and building backend..."
  (cd "$BACKEND_DIR" && go test ./... \
    && go build -o "$BACKEND_DIR/server" ./cmd/server \
    && go build -o "$BACKEND_DIR/mcp-server" ./cmd/mcp)
fi

if [ "$SKIP_FRONTEND" -eq 0 ]; then
  echo "Installing and building frontend..."
  (cd "$FRONTEND_DIR" && npm ci && npm run typecheck && npm run build)
fi

if [ "$START_BACKEND" -eq 1 ]; then
  "$ROOT_DIR/scripts/start-server.sh"
fi

echo
echo "Deployment preparation complete."
echo "Stack:    ./scripts/start-stack.sh  (backend + frontend)"
echo "Backend:  ./scripts/start-server.sh  (health: http://127.0.0.1:8000/health)"
echo "Frontend: ./scripts/start-frontend.sh (http://127.0.0.1:5173/login)"
echo "Stop:     ./scripts/stop-stack.sh     (does not touch PostgreSQL/NapCat)"
echo "MCP:       ./scripts/mcp-server.sh"
echo "Config:    $ENV_FILE"
echo "Data:      $ROOT_DIR/data/ (kept outside Git)"
echo "QZone:     optional; start only when NapCat/QZone credentials are configured"
if [ "$SHOW_CREDENTIALS" -eq 1 ]; then
  printf 'Admin username: %s\nAdmin password: %s\n' "$ADMIN_USERNAME" "$ADMIN_PASSWORD"
else
  echo "Admin credentials were not printed. Use --show-credentials or read the protected .env file locally."
fi

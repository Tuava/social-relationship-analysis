#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR/backend"

# shellcheck source=scripts/lib/env.sh
source "$ROOT_DIR/scripts/lib/env.sh"

PID_FILE="$ROOT_DIR/backend/server.pid"
LOG_FILE="$ROOT_DIR/backend/server.log"
ENV_FILE="$ROOT_DIR/backend/.env"

ensure_local_env() {
  if [ -f "$ENV_FILE" ]; then
    if ! grep -q '^SRA_CONFIG_ENCRYPTION_KEY=' "$ENV_FILE"; then
      printf 'SRA_CONFIG_ENCRYPTION_KEY=%s\n' "$(openssl rand -hex 32)" >> "$ENV_FILE"
    fi
    chmod 600 "$ENV_FILE"
    return
  fi

  umask 077
  {
    printf 'DATABASE_URL=%s\n' "postgres://localhost:5432/social_relationship_analysis?sslmode=disable"
    printf 'HTTP_ADDR=%s\n' "127.0.0.1:8000"
    printf 'JWT_SECRET=%s\n' "$(openssl rand -hex 32)"
    printf 'SRA_CONFIG_ENCRYPTION_KEY=%s\n' "$(openssl rand -hex 32)"
    printf 'ADMIN_USERNAME=%s\n' "admin"
    printf 'ADMIN_PASSWORD=%s\n' "$(openssl rand -hex 24)"
  } > "$ENV_FILE"
  chmod 600 "$ENV_FILE"
  echo "Created local runtime credentials at $ENV_FILE"
}

load_env() {
  load_env_file "$ENV_FILE"
}

http_port() {
  case "${HTTP_ADDR:-127.0.0.1:8000}" in
    *:*) printf '%s\n' "${HTTP_ADDR##*:}" ;;
    *) printf '%s\n' "${HTTP_ADDR:-8000}" ;;
  esac
}

health_url() {
  local host="${HTTP_ADDR%%:*}"
  case "$HTTP_ADDR" in
    0.0.0.0:*|\:\:*|\[\:\]:*) host="127.0.0.1" ;;
    \[*\]:*) host="127.0.0.1" ;;
    '') host="127.0.0.1" ;;
  esac
  printf 'http://%s:%s/health\n' "$host" "$(http_port)"
}

listener_pid() {
  lsof -nP -t -iTCP:"$(http_port)" -sTCP:LISTEN 2>/dev/null | head -n 1 || true
}

process_command() {
  ps -p "$1" -o command= 2>/dev/null || true
}

process_cwd() {
  lsof -a -p "$1" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1 || true
}

is_sra_pid() {
  local pid="$1" command cwd
  command="$(process_command "$pid")"
  cwd="$(process_cwd "$pid")"
  case "$cwd" in
    "$ROOT_DIR/backend"|"$ROOT_DIR/backend"/*)
      case "$command" in
        *"$ROOT_DIR/backend/server"*) return 0 ;;
      esac
      ;;
  esac
  return 1
}

health_is_sra() {
  local response
  response="$(curl -fsS --max-time 2 "$(health_url)" 2>/dev/null || true)"
  printf '%s' "$response" | grep -Eq '"service"[[:space:]]*:[[:space:]]*"sra-backend"'
}

SERVICE_LABEL="social-relationship-analysis.backend"

ensure_local_env
load_env

LISTENER_PID="$(listener_pid)"
if [ -n "$LISTENER_PID" ]; then
  if is_sra_pid "$LISTENER_PID" && curl -fsS --max-time 2 "$(health_url)" >/dev/null \
    && health_is_sra; then
    echo "$LISTENER_PID" > "$PID_FILE"
    echo "Backend server is already running (PID: $LISTENER_PID)"
    exit 0
  fi
  echo "Port $(http_port) is occupied by a non-SRA process (PID: $LISTENER_PID)" >&2
  exit 1
fi

if [ -f "$PID_FILE" ]; then
  PID=$(cat "$PID_FILE")
  if kill -0 "$PID" 2>/dev/null && is_sra_pid "$PID" \
    && curl -fsS --max-time 2 "$(health_url)" >/dev/null \
    && health_is_sra; then
    echo "Backend server is already running (PID: $PID)"
    exit 0
  fi
  rm -f "$PID_FILE"
fi

go build -o "$ROOT_DIR/backend/server" ./cmd/server

if [ "$(uname -s)" = "Darwin" ] && command -v launchctl >/dev/null 2>&1; then
  launchctl remove "$SERVICE_LABEL" 2>/dev/null || true
  ROOT_DIR_Q="$(printf '%q' "$ROOT_DIR")"
  ENV_FILE_Q="$(printf '%q' "$ENV_FILE")"
  LOG_FILE_Q="$(printf '%q' "$LOG_FILE")"
  launchctl submit -l "$SERVICE_LABEL" -- /bin/bash -lc \
    "source $ROOT_DIR_Q/scripts/lib/env.sh; load_env_file $ENV_FILE_Q; cd $ROOT_DIR_Q/backend; exec $ROOT_DIR_Q/backend/server >> $LOG_FILE_Q 2>&1"
else
  nohup "$ROOT_DIR/backend/server" </dev/null >> "$LOG_FILE" 2>&1 &
  disown "$!" 2>/dev/null || true
fi

for _ in $(seq 1 20); do
  SERVER_PID="$(listener_pid)"
  if [ -n "$SERVER_PID" ] && is_sra_pid "$SERVER_PID" \
    && curl -fsS --max-time 1 "$(health_url)" >/dev/null \
    && health_is_sra; then
    echo "$SERVER_PID" > "$PID_FILE"
    echo "Backend server started in background (PID: $SERVER_PID, Log: $LOG_FILE)"
    exit 0
  fi
  sleep 0.5
done

rm -f "$PID_FILE"
tail -n 40 "$LOG_FILE" >&2 || true
echo "Backend server did not become healthy in time" >&2
exit 1

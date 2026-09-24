#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/frontend"
BACKEND_ENV="$ROOT_DIR/backend/.env"
PID_FILE="$FRONTEND_DIR/frontend.pid"
LOG_FILE="$FRONTEND_DIR/frontend.log"
SERVICE_LABEL="social-relationship-analysis.frontend"

# shellcheck source=scripts/lib/env.sh
source "$ROOT_DIR/scripts/lib/env.sh"

FRONTEND_HOST="$(env_file_value FRONTEND_HOST "$BACKEND_ENV")"
FRONTEND_HOST="${FRONTEND_HOST:-127.0.0.1}"
FRONTEND_PORT="$(env_file_value FRONTEND_PORT "$BACKEND_ENV")"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"

case "$FRONTEND_PORT" in
  ''|*[!0-9]*)
    echo "Frontend port must be numeric: $FRONTEND_PORT" >&2
    exit 2
    ;;
esac

command -v npm >/dev/null 2>&1 || { echo "npm is required" >&2; exit 1; }
[ -f "$FRONTEND_DIR/package.json" ] || { echo "frontend/package.json is missing" >&2; exit 1; }

process_command() {
  ps -p "$1" -o command= 2>/dev/null || true
}

process_cwd() {
  lsof -a -p "$1" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1 || true
}

is_frontend_pid() {
  local pid="$1"
  local command cwd
  command="$(process_command "$pid")"
  cwd="$(process_cwd "$pid")"
  case "$cwd" in
    "$FRONTEND_DIR"|"$FRONTEND_DIR"/*)
      case "$command" in
        *vite*|*npm*serve*|*npm*run*serve*) return 0 ;;
      esac
      ;;
  esac
  return 1
}

listener_pid() {
  lsof -nP -t -iTCP:"$FRONTEND_PORT" -sTCP:LISTEN 2>/dev/null | head -n 1 || true
}

health_host() {
  case "$FRONTEND_HOST" in
    0.0.0.0|::|\[::\]) printf '127.0.0.1' ;;
    *) printf '%s' "$FRONTEND_HOST" ;;
  esac
}

health_url="http://$(health_host):$FRONTEND_PORT/login"

if [ -f "$PID_FILE" ]; then
  candidate="$(tr -dc '0-9' < "$PID_FILE")"
  if [ -n "$candidate" ] && kill -0 "$candidate" 2>/dev/null && is_frontend_pid "$candidate"; then
    if curl -fsS --max-time 2 "$health_url" >/dev/null 2>&1; then
      echo "Frontend is already running (PID: $candidate)"
      exit 0
    fi
    echo "Frontend is already starting (PID: $candidate); waiting for health..."
  else
    rm -f "$PID_FILE"
  fi
fi

LISTENER_PID="$(listener_pid)"
if [ -n "$LISTENER_PID" ]; then
  if is_frontend_pid "$LISTENER_PID" && curl -fsS --max-time 2 "$health_url" >/dev/null 2>&1; then
    echo "$LISTENER_PID" > "$PID_FILE"
    echo "Frontend is already running (PID: $LISTENER_PID)"
    exit 0
  fi
  echo "Port $FRONTEND_PORT is occupied by a non-SRA frontend process; leaving it untouched." >&2
  exit 1
fi

cd "$FRONTEND_DIR"
if [ "$(uname -s)" = "Darwin" ] && command -v launchctl >/dev/null 2>&1; then
  # A plain nohup child can still be reaped with the invoking terminal's
  # process group on macOS. launchctl gives the dev server an independent
  # lifecycle, matching start-server.sh and surviving terminal closure.
  launchctl remove "$SERVICE_LABEL" 2>/dev/null || true
  FRONTEND_DIR_Q="$(printf '%q' "$FRONTEND_DIR")"
  FRONTEND_HOST_Q="$(printf '%q' "$FRONTEND_HOST")"
  FRONTEND_PORT_Q="$(printf '%q' "$FRONTEND_PORT")"
  LOG_FILE_Q="$(printf '%q' "$LOG_FILE")"
  launchctl submit -l "$SERVICE_LABEL" -- /bin/zsh -lc \
    "cd $FRONTEND_DIR_Q; exec npm run serve -- --host $FRONTEND_HOST_Q --port $FRONTEND_PORT_Q --strictPort >> $LOG_FILE_Q 2>&1"
  LAUNCH_PID=""
else
  nohup npm run serve -- --host "$FRONTEND_HOST" --port "$FRONTEND_PORT" --strictPort \
    >> "$LOG_FILE" 2>&1 < /dev/null &
  LAUNCH_PID="$!"
  disown "$LAUNCH_PID" 2>/dev/null || true
fi

for _ in $(seq 1 40); do
  LISTENER_PID="$(listener_pid)"
  if [ -n "$LISTENER_PID" ] && is_frontend_pid "$LISTENER_PID" \
    && curl -fsS --max-time 1 "$health_url" >/dev/null 2>&1; then
    echo "$LISTENER_PID" > "$PID_FILE"
    echo "Frontend started in background (PID: $LISTENER_PID, Log: $LOG_FILE)"
    exit 0
  fi
  if [ -n "$LAUNCH_PID" ] && ! kill -0 "$LAUNCH_PID" 2>/dev/null && [ -z "$LISTENER_PID" ]; then
    break
  fi
  sleep 0.5
done

rm -f "$PID_FILE"
if [ "$(uname -s)" = "Darwin" ] && command -v launchctl >/dev/null 2>&1; then
  launchctl remove "$SERVICE_LABEL" 2>/dev/null || true
fi
tail -n 40 "$LOG_FILE" >&2 || true
echo "Frontend did not become healthy in time" >&2
exit 1

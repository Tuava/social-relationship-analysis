#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/frontend"
BACKEND_ENV="$ROOT_DIR/backend/.env"
PID_FILE="$FRONTEND_DIR/frontend.pid"
SERVICE_LABEL="social-relationship-analysis.frontend"

# shellcheck source=scripts/lib/env.sh
source "$ROOT_DIR/scripts/lib/env.sh"

FRONTEND_PORT="$(env_file_value FRONTEND_PORT "$BACKEND_ENV")"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"

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

stop_tree() {
  local pid="$1" child
  for child in $(pgrep -P "$pid" 2>/dev/null || true); do
    stop_tree "$child"
  done
  kill -TERM "$pid" 2>/dev/null || true
}

stop_pid() {
  local pid="$1"
  kill -0 "$pid" 2>/dev/null || return 0
  stop_tree "$pid"
  for _ in $(seq 1 20); do
    kill -0 "$pid" 2>/dev/null || return 0
    sleep 0.25
  done
  kill -KILL "$pid" 2>/dev/null || true
}

PID=""
if [ -f "$PID_FILE" ]; then
  CANDIDATE="$(tr -dc '0-9' < "$PID_FILE")"
  if [ -n "$CANDIDATE" ] && kill -0 "$CANDIDATE" 2>/dev/null && is_frontend_pid "$CANDIDATE"; then
    PID="$CANDIDATE"
  fi
fi

if [ -z "$PID" ]; then
  CANDIDATE="$(listener_pid)"
  if [ -n "$CANDIDATE" ] && kill -0 "$CANDIDATE" 2>/dev/null && is_frontend_pid "$CANDIDATE"; then
    PID="$CANDIDATE"
  elif [ -n "$CANDIDATE" ]; then
    echo "Port $FRONTEND_PORT is occupied by a process that does not belong to SRA; leaving it untouched." >&2
  fi
fi

if [ -n "$PID" ]; then
  stop_pid "$PID"
  echo "Frontend (PID: $PID) stopped."
else
  echo "No SRA frontend is running."
fi
if [ "$(uname -s)" = "Darwin" ] && command -v launchctl >/dev/null 2>&1; then
  launchctl remove "$SERVICE_LABEL" 2>/dev/null || true
fi
rm -f "$PID_FILE"

#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PID_FILE="$ROOT/data/run/qzone.pid"

process_command() {
  ps -p "$1" -o command= 2>/dev/null || true
}

process_cwd() {
  lsof -a -p "$1" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1 || true
}

is_ours() {
  PID_TO_CHECK="$1"
  COMMAND=$(process_command "$PID_TO_CHECK")
  CWD=$(process_cwd "$PID_TO_CHECK")
  case "$COMMAND:$CWD" in
    *"$ROOT/integrations/onebot-qzone"*) return 0 ;;
    *) return 1 ;;
  esac
}

stop_tree() {
  PID_TO_STOP="$1"
  CHILDREN=$(pgrep -P "$PID_TO_STOP" 2>/dev/null || true)
  for CHILD in $CHILDREN; do
    stop_tree "$CHILD"
  done
  kill -TERM "$PID_TO_STOP" 2>/dev/null || true
}

if [ -f "$PID_FILE" ]; then
  PID=$(cat "$PID_FILE")
  if kill -0 "$PID" 2>/dev/null && is_ours "$PID"; then
    stop_tree "$PID"
    sleep 1
  fi
  rm -f "$PID_FILE"
fi

# Never kill an arbitrary process just because it happens to use port 5700.
# Only clean up a listener whose command/cwd identifies this checkout.
PIDS=$(lsof -tiTCP:5700 -sTCP:LISTEN 2>/dev/null || true)
for PID in $PIDS; do
  if kill -0 "$PID" 2>/dev/null && is_ours "$PID"; then
    stop_tree "$PID"
  fi
done

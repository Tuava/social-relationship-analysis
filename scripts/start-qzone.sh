#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=scripts/lib/env.sh
source "$ROOT/scripts/lib/env.sh"
INTEGRATION="$ROOT/integrations/onebot-qzone"
LOG_DIR="$ROOT/data/logs"
RUN_DIR="$ROOT/data/run"
PID_FILE="$RUN_DIR/qzone.pid"
QR_PATCH="$ROOT/patches/onebot-qzone-qr-session.patch"
SERVICE_LABEL="social-relationship-analysis.qzone"

mkdir -p "$LOG_DIR" "$RUN_DIR" "$ROOT/data/qzone-cache"

if [ ! -d "$INTEGRATION" ] || [ ! -f "$INTEGRATION/package.json" ]; then
  echo "QZone bridge source is missing or uninitialized: $INTEGRATION" >&2
  exit 1
fi

QZONE_ENV="$INTEGRATION/.env"
QZONE_HOST="$(env_file_value ONEBOT_HOST "$QZONE_ENV")"
QZONE_HOST="${QZONE_HOST:-127.0.0.1}"
QZONE_PORT="$(env_file_value ONEBOT_PORT "$QZONE_ENV")"
QZONE_PORT="${QZONE_PORT:-5700}"
case "$QZONE_PORT" in
  ''|*[!0-9]*) echo "ONEBOT_PORT must be numeric: $QZONE_PORT" >&2; exit 2 ;;
esac

process_command() {
  ps -p "$1" -o command= 2>/dev/null || true
}

process_cwd() {
  lsof -a -p "$1" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1 || true
}

is_qzone_pid() {
  local pid="$1" command cwd
  command="$(process_command "$pid")"
  cwd="$(process_cwd "$pid")"
  case "$cwd" in
    "$INTEGRATION"|"$INTEGRATION"/*)
      case "$command" in
        *node*|*npm*|*tsx*) return 0 ;;
      esac
      ;;
  esac
  return 1
}

listener_pid() {
  lsof -nP -t -iTCP:"$QZONE_PORT" -sTCP:LISTEN 2>/dev/null | head -n 1 || true
}

health_url="http://$QZONE_HOST:$QZONE_PORT/status"

if [ -f "$PID_FILE" ]; then
  candidate="$(tr -dc '0-9' < "$PID_FILE")"
  if [ -n "$candidate" ] && kill -0 "$candidate" 2>/dev/null && is_qzone_pid "$candidate" \
    && curl -fsS --max-time 2 "$health_url" >/dev/null 2>&1; then
    echo "QZone bridge is already running (PID: $candidate, port: $QZONE_PORT)"
    exit 0
  fi
  rm -f "$PID_FILE"
fi

listener="$(listener_pid)"
if [ -n "$listener" ]; then
  if is_qzone_pid "$listener" && curl -fsS --max-time 2 "$health_url" >/dev/null 2>&1; then
    echo "$listener" > "$PID_FILE"
    echo "QZone bridge is already running (PID: $listener, port: $QZONE_PORT)"
    exit 0
  fi
  echo "Port $QZONE_PORT is occupied by a non-SRA QZone process (PID: $listener); leaving it untouched." >&2
  exit 1
fi

cd "$INTEGRATION"

# Published sources include the QR-session patch; do not patch a vendored
# directory during startup. Developers can apply patches explicitly if needed.

if [ ! -d node_modules ]; then
  npm ci
fi

BACKEND_ENV="$ROOT/backend/.env"
NAPCAT_HTTP_URL="$(env_file_value NAPCAT_HTTP_URL "$BACKEND_ENV")"
NAPCAT_HTTP_URL="${NAPCAT_HTTP_URL:-http://127.0.0.1:3000}"
NAPCAT_HTTP_TOKEN="$(env_file_value NAPCAT_HTTP_TOKEN "$BACKEND_ENV")"

# Reuse NapCat's authenticated QQ session without persisting QZone cookies.
if [ -z "${QZONE_COOKIE_STRING:-}" ]; then
  CURL_ARGS=(-fsS --max-time 10 -X POST "$NAPCAT_HTTP_URL/get_cookies" \
    -H 'Content-Type: application/json' \
    -d '{"domain":"qzone.qq.com"}')
  if [ -n "$NAPCAT_HTTP_TOKEN" ]; then
    CURL_ARGS=(-fsS --max-time 10 -X POST "$NAPCAT_HTTP_URL/get_cookies" \
      -H "Authorization: Bearer $NAPCAT_HTTP_TOKEN" \
      -H 'Content-Type: application/json' \
      -d '{"domain":"qzone.qq.com"}')
  fi
  NAPCAT_COOKIES=$(curl "${CURL_ARGS[@]}" 2>/dev/null \
    | jq -r 'select(.status == "ok") | .data.cookies // empty' 2>/dev/null || true)
  if [ -n "$NAPCAT_COOKIES" ]; then
    export QZONE_COOKIE_STRING="$NAPCAT_COOKIES"
    export QZONE_ENABLE_QR=0
  fi
fi

if [ "$(uname -s)" = "Darwin" ] && command -v launchctl >/dev/null 2>&1; then
  launchctl remove "$SERVICE_LABEL" 2>/dev/null || true
  INTEGRATION_Q="$(printf '%q' "$INTEGRATION")"
  LOG_FILE_Q="$(printf '%q' "$LOG_DIR/qzone.log")"
  launchctl submit -l "$SERVICE_LABEL" -- /bin/bash -lc \
    "cd $INTEGRATION_Q; exec npm run dev >> $LOG_FILE_Q 2>&1"
else
  nohup npm run dev >> "$LOG_DIR/qzone.log" 2>&1 </dev/null &
  launch_pid="$!"
  disown "$launch_pid" 2>/dev/null || true
fi

for _ in $(seq 1 40); do
  listener="$(listener_pid)"
  if [ -n "$listener" ] && is_qzone_pid "$listener" \
    && curl -fsS --max-time 1 "$health_url" >/dev/null 2>&1; then
    echo "$listener" > "$PID_FILE"
    echo "QZone bridge started in background (PID: $listener, port: $QZONE_PORT, log: $LOG_DIR/qzone.log)"
    exit 0
  fi
  if [ -n "${launch_pid:-}" ] && ! kill -0 "$launch_pid" 2>/dev/null && [ -z "$listener" ]; then
    break
  fi
  sleep 0.5
done

rm -f "$PID_FILE"
tail -n 40 "$LOG_DIR/qzone.log" >&2 || true
if [ "$(uname -s)" = "Darwin" ] && command -v launchctl >/dev/null 2>&1; then
  launchctl remove "$SERVICE_LABEL" 2>/dev/null || true
fi
echo "QZone bridge did not become healthy in time" >&2
exit 1

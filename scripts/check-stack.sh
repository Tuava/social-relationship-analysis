#!/usr/bin/env bash
set -u

# Read-only component diagnostic for a local SRA checkout.  It deliberately
# does not start services, touch the database, or print credentials/cookies.
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_ENV="$ROOT_DIR/backend/.env"
QZONE_DIR="$ROOT_DIR/integrations/onebot-qzone"
QZONE_ENV="$QZONE_DIR/.env"
STRICT=0

# shellcheck source=scripts/lib/env.sh
source "$ROOT_DIR/scripts/lib/env.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/check-stack.sh [--strict]

Run a read-only diagnostic for the local SRA stack.

Required components:
  PostgreSQL, SRA backend, and frontend

Optional external components:
  NapCat and the QZone bridge. They are reported separately and are not
  treated as project failures unless --strict is supplied.

Options:
  --strict   Return non-zero when NapCat or QZone is unavailable too.
  -h, --help Show this help.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --strict) STRICT=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "check-stack: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

split_addr() {
  local addr="$1" default_host="$2" default_port="$3"
  if [ -z "$addr" ]; then
    printf '%s\t%s\n' "$default_host" "$default_port"
  elif [[ "$addr" == \[*\]:* ]]; then
    printf '%s\t%s\n' "${addr%%]:*}]" "${addr##*:}"
  elif [[ "$addr" == *:* ]]; then
    printf '%s\t%s\n' "${addr%:*}" "${addr##*:}"
  else
    printf '%s\t%s\n' "$default_host" "$addr"
  fi
}

tcp_listening() {
  local host="$1" port="$2"
  if command -v nc >/dev/null 2>&1; then
    # -w is supported by the macOS and OpenBSD netcat variants used by the
    # supported local environments. lsof remains the fallback below.
    nc -z -w 1 "$host" "$port" >/dev/null 2>&1
    return $?
  fi
  command -v lsof >/dev/null 2>&1 || return 1
  lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
}

http_probe() {
  local url="$1" token="${2:-}" status
  if [ -n "$token" ]; then
    status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 3 -H "Authorization: Bearer $token" "$url" 2>/dev/null || true)"
  else
    status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 3 "$url" 2>/dev/null || true)"
  fi
  [ -n "$status" ] || status=000
  case "$status" in
    2*|3*) printf 'online (%s)' "$status"; return 0 ;;
    401|403) printf 'online, auth rejected (%s)' "$status"; return 0 ;;
    000) printf 'offline'; return 1 ;;
    *) printf 'reachable, returned HTTP %s' "$status"; return 0 ;;
  esac
}

process_command() {
  ps -p "$1" -o command= 2>/dev/null || true
}

process_cwd() {
  lsof -a -p "$1" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1 || true
}

is_sra_backend_pid() {
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

print_status() {
  printf '%-18s %s\n' "$1" "$2"
}

required_failed=0
optional_failed=0

echo "SRA stack diagnostic (read-only)"
echo "Checkout: $ROOT_DIR"
echo

# PostgreSQL: use psql's exit status only; never echo DATABASE_URL.
database_url="$(env_file_value DATABASE_URL "$BACKEND_ENV")"
if [ -z "$database_url" ]; then
  print_status "PostgreSQL" "not configured"
  required_failed=1
elif command -v psql >/dev/null 2>&1 && psql --dbname="$database_url" -v ON_ERROR_STOP=1 -Atqc 'SELECT 1' >/dev/null 2>&1; then
  print_status "PostgreSQL" "online"
else
  print_status "PostgreSQL" "offline or unavailable"
  required_failed=1
fi

backend_addr="$(env_file_value HTTP_ADDR "$BACKEND_ENV")"
read -r backend_host backend_port < <(split_addr "$backend_addr" "127.0.0.1" "8000")
backend_health=""
backend_pid="$(lsof -nP -t -iTCP:"$backend_port" -sTCP:LISTEN 2>/dev/null | head -n 1 || true)"
backend_identity="$(curl -fsS --max-time 3 "http://$backend_host:$backend_port/health" 2>/dev/null || true)"
if [ -n "$backend_pid" ] && is_sra_backend_pid "$backend_pid" \
  && printf '%s' "$backend_identity" | grep -Eq '"service"[[:space:]]*:[[:space:]]*"sra-backend"'; then
  backend_health="$(http_probe "http://$backend_host:$backend_port/health")"
  print_status "SRA backend" "$backend_health ($backend_host:$backend_port)"
else
  if [ -n "$backend_identity" ]; then
    print_status "SRA backend" "port reachable but identity check failed ($backend_host:$backend_port)"
  else
    print_status "SRA backend" "offline ($backend_host:$backend_port)"
  fi
  required_failed=1
fi

frontend_host="$(env_file_value FRONTEND_HOST "$BACKEND_ENV")"
frontend_host="${frontend_host:-127.0.0.1}"
frontend_port="$(env_file_value FRONTEND_PORT "$BACKEND_ENV")"
frontend_port="${frontend_port:-5173}"
frontend_health=""
if frontend_health="$(http_probe "http://$frontend_host:$frontend_port/login")"; then
  print_status "SRA frontend" "$frontend_health ($frontend_host:$frontend_port)"
else
  print_status "SRA frontend" "offline ($frontend_host:$frontend_port)"
  required_failed=1
fi

# vue-mcp-next is a development/introspection sidecar started by the Vite
# server. It is useful to maintainers but is not a required production API;
# report it separately without making the stack fail when a built frontend is
# being served by another web server.
vue_mcp_host="$(env_file_value VUE_MCP_HOST "$BACKEND_ENV")"
vue_mcp_host="${vue_mcp_host:-127.0.0.1}"
vue_mcp_port="$(env_file_value VUE_MCP_PORT "$BACKEND_ENV")"
vue_mcp_port="${vue_mcp_port:-8890}"
if vue_mcp_health="$(http_probe "http://$vue_mcp_host:$vue_mcp_port/health")"; then
  print_status "Vue MCP" "$vue_mcp_health ($vue_mcp_host:$vue_mcp_port)"
else
  print_status "Vue MCP" "not running (dev-only sidecar; optional) ($vue_mcp_host:$vue_mcp_port)"
fi

if [ -x "$ROOT_DIR/backend/mcp-server" ] || [ -f "$ROOT_DIR/backend/mcp-server" ]; then
  print_status "SRA MCP" "binary present (stdio; no port)"
else
  print_status "SRA MCP" "binary missing (run deploy.sh)"
  required_failed=1
fi

echo
echo "External QQ components"

napcat_http_host="$(env_file_value NAPCAT_HTTP_HOST "$BACKEND_ENV")"
napcat_http_host="${napcat_http_host:-127.0.0.1}"
napcat_http_port="$(env_file_value NAPCAT_HTTP_PORT "$BACKEND_ENV")"
napcat_http_port="${napcat_http_port:-3000}"
napcat_http_token="$(env_file_value NAPCAT_HTTP_TOKEN "$BACKEND_ENV")"
if tcp_listening "$napcat_http_host" "$napcat_http_port"; then
  napcat_health=""
  if napcat_health="$(http_probe "http://$napcat_http_host:$napcat_http_port/get_login_info" "$napcat_http_token")"; then
    print_status "NapCat HTTP" "$napcat_health ($napcat_http_host:$napcat_http_port)"
  else
    print_status "NapCat HTTP" "listening but API probe failed ($napcat_http_host:$napcat_http_port)"
    optional_failed=1
  fi
else
  print_status "NapCat HTTP" "external dependency unavailable ($napcat_http_host:$napcat_http_port)"
  optional_failed=1
fi

napcat_ws_host="$(env_file_value NAPCAT_WS_HOST "$BACKEND_ENV")"
napcat_ws_host="${napcat_ws_host:-127.0.0.1}"
napcat_ws_port="$(env_file_value NAPCAT_WS_PORT "$BACKEND_ENV")"
napcat_ws_port="${napcat_ws_port:-3001}"
if tcp_listening "$napcat_ws_host" "$napcat_ws_port"; then
  print_status "NapCat WS" "listening ($napcat_ws_host:$napcat_ws_port)"
else
  print_status "NapCat WS" "external dependency unavailable ($napcat_ws_host:$napcat_ws_port)"
  optional_failed=1
fi

qzone_host="$(env_file_value ONEBOT_HOST "$QZONE_ENV")"
qzone_host="${qzone_host:-127.0.0.1}"
qzone_port="$(env_file_value ONEBOT_PORT "$QZONE_ENV")"
qzone_port="${qzone_port:-5700}"
qzone_token="$(env_file_value ONEBOT_ACCESS_TOKEN "$QZONE_ENV")"
qzone_health=""
if qzone_health="$(http_probe "http://$qzone_host:$qzone_port/status" "$qzone_token")"; then
  print_status "QZone bridge" "$qzone_health ($qzone_host:$qzone_port)"
elif [ -d "$QZONE_DIR" ]; then
  print_status "QZone bridge" "not running ($qzone_host:$qzone_port)"
  optional_failed=1
else
  print_status "QZone bridge" "submodule missing"
  optional_failed=1
fi

if [ -d "$QZONE_DIR" ]; then
  if [ -f "$QZONE_DIR/package.json" ]; then
    print_status "QZone source" "present"
  else
    print_status "QZone source" "directory exists but submodule is not initialized"
    optional_failed=1
  fi
  if [ -f "$QZONE_DIR/package.json" ] && [ -d "$QZONE_DIR/node_modules" ]; then
    print_status "QZone dependencies" "installed"
  elif [ -f "$QZONE_DIR/package.json" ]; then
    print_status "QZone dependencies" "missing (run npm ci in integrations/onebot-qzone)"
    optional_failed=1
  fi
else
  print_status "QZone source" "missing (initialize the submodule)"
  optional_failed=1
fi

echo
echo "Notes: NapCat is not shipped by this repository. QZone is optional and requires an authorized session."
if [ "$required_failed" -ne 0 ]; then
  exit 1
fi
if [ "$STRICT" -ne 0 ] && [ "$optional_failed" -ne 0 ]; then
  exit 1
fi
exit 0

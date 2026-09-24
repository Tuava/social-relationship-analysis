#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_FILE="$ROOT_DIR/backend/server.pid"
SERVICE_LABEL="social-relationship-analysis.backend"

# shellcheck source=scripts/lib/env.sh
source "$ROOT_DIR/scripts/lib/env.sh"

if [ -f "$ROOT_DIR/backend/.env" ]; then
	load_env_file "$ROOT_DIR/backend/.env"
fi

http_port() {
	case "${HTTP_ADDR:-127.0.0.1:8000}" in
		*:*) printf '%s\n' "${HTTP_ADDR##*:}" ;;
		*) printf '%s\n' "${HTTP_ADDR:-8000}" ;;
	esac
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
				*) return 1 ;;
			esac
			;;
	esac
	return 1
}

stop_pid() {
	local pid="$1"
	if ! kill -0 "$pid" 2>/dev/null; then
		return
	fi
	kill -TERM "$pid" 2>/dev/null || true
	for _ in $(seq 1 20); do
		if ! kill -0 "$pid" 2>/dev/null; then
			return
		fi
		sleep 0.25
	done
	kill -KILL "$pid" 2>/dev/null || true
}

PID=""
if [ -f "$PID_FILE" ]; then
	CANDIDATE="$(tr -dc '0-9' < "$PID_FILE")"
	if [ -n "$CANDIDATE" ] && kill -0 "$CANDIDATE" 2>/dev/null; then
		PID="$CANDIDATE"
	fi
fi
if [ -z "$PID" ]; then
	PID="$(listener_pid)"
fi

	if [ -n "$PID" ] && is_sra_pid "$PID"; then
	stop_pid "$PID"
	if [ "$(uname -s)" = "Darwin" ] && command -v launchctl >/dev/null 2>&1; then
		launchctl remove "$SERVICE_LABEL" 2>/dev/null || true
	fi
	rm -f "$PID_FILE"
	echo "Backend server (PID: $PID) stopped."
else
	if [ -n "$PID" ]; then
		echo "Port $(http_port) is occupied by a process that does not belong to SRA; leaving it untouched." >&2
	fi
	rm -f "$PID_FILE"
	echo "No SRA backend server is running."
fi

#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WITH_QZONE=0

usage() {
  cat <<'EOF'
Usage: ./scripts/start-stack.sh [--with-qzone]

Start the local SRA backend and frontend in the background.

Options:
  --with-qzone  Also start the optional QZone bridge. NapCat is external and
                must already be installed, logged in and listening.
  -h, --help    Show this help.

This script never installs or starts NapCat, never starts a collection task,
and never opens a browser or QR login flow unless QZone is explicitly enabled.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --with-qzone) WITH_QZONE=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

"$ROOT_DIR/scripts/start-server.sh"
"$ROOT_DIR/scripts/start-frontend.sh"

if [ "$WITH_QZONE" -eq 1 ]; then
  "$ROOT_DIR/scripts/start-qzone.sh"
else
  echo "QZone bridge not started (use --with-qzone after NapCat is ready)."
fi

echo "SRA local stack started. Run ./scripts/check-stack.sh to verify it."

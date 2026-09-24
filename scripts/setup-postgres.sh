#!/usr/bin/env bash
set -euo pipefail

brew install postgresql@16
brew services start postgresql@16
createdb "${SRA_DB_NAME:-social_relationship_analysis}" 2>/dev/null || true
echo "PostgreSQL database ready: ${SRA_DB_NAME:-social_relationship_analysis}"

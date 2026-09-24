#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT/backend/.env"
if [ -f "$ENV_FILE" ]; then
  echo "Configuration already exists: $ENV_FILE"
  exit 0
fi
command -v openssl >/dev/null || { echo 'openssl is required to generate credentials.' >&2; exit 1; }
umask 077
# noclobber also protects against simultaneous initializers.
set -o noclobber
{
  printf 'DATABASE_URL=%s\n' 'postgres://localhost:5432/social_relationship_analysis?sslmode=disable'
  printf 'HTTP_ADDR=%s\n' '127.0.0.1:8000'
  printf 'JWT_SECRET=%s\n' "$(openssl rand -hex 32)"
  printf 'SRA_CONFIG_ENCRYPTION_KEY=%s\n' "$(openssl rand -hex 32)"
  printf 'ADMIN_USERNAME=admin\nADMIN_PASSWORD=%s\n' "$(openssl rand -hex 24)"
} > "$ENV_FILE"
echo "Created $ENV_FILE (mode 600). Set DATABASE_URL there before starting."

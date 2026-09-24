#!/usr/bin/env bash
set -euo pipefail
[ "${GITHUB_ACTIONS:-}" = true ] || { echo 'CI-only smoke test.' >&2; exit 1; }
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT/backend"
server_pid=''
trap 'if [ -n "$server_pid" ]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi' EXIT
for attempt in 1 2; do
  ./server > "$RUNNER_TEMP/sra-smoke.log" 2>&1 &
  server_pid=$!
  ready=0
  for _ in $(seq 1 60); do
    if curl -fsS http://127.0.0.1:8000/health >/dev/null; then ready=1; break; fi
    if ! kill -0 "$server_pid" 2>/dev/null; then cat "$RUNNER_TEMP/sra-smoke.log"; exit 1; fi
    sleep 1
  done
  [ "$ready" = 1 ] || { cat "$RUNNER_TEMP/sra-smoke.log"; exit 1; }
  python3 - <<'PY'
import json, os, urllib.request, urllib.error
base='http://127.0.0.1:8000'
body=json.dumps({'username':os.environ['ADMIN_USERNAME'],'password':os.environ['ADMIN_PASSWORD']}).encode()
r=urllib.request.Request(base+'/api/v1/auth/login',data=body,headers={'Content-Type':'application/json'})
with urllib.request.urlopen(r) as response: token=json.load(response)['token']
for path in ['/api/v1/auth/me','/api/v1/accounts','/api/v1/conversations','/api/v1/system/settings','/api/v1/bot/instances','/api/v1/bot/audit']:
 with urllib.request.urlopen(urllib.request.Request(base+path,headers={'Authorization':'Bearer '+token})) as response:
  assert response.status==200, path
for path in ['/api/v1/accounts','/api/v1/media/chat-image?file=/etc/passwd']:
 try: urllib.request.urlopen(base+path)
 except urllib.error.HTTPError as e: assert e.code==401, (path,e.code)
 else: raise AssertionError('anonymous access: '+path)
print('Fresh database/authentication smoke passed')
PY
  kill "$server_pid"
  wait "$server_pid" || true
  server_pid=''
done

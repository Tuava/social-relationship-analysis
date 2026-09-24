#!/usr/bin/env python3
"""Check the Git publication inventory without executing or compiling source."""
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
SECRET = re.compile(rb'(?:gh[pousr]_[A-Za-z0-9]{30,}|sk-(?:proj-)?[A-Za-z0-9_-]{32,}|-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----)')
DENIED_SUFFIXES = ('.db', '.db-wal', '.db-shm', '.sqlite', '.sqlite3', '.dump', '.jsonl', '.log', '.pid', '.pyc', '.pyo')

def problems(path, content):
    issues = []
    if path == "docs/data-collection-known-issues.md" or path.startswith("docs/mcp-readonly-analysis-"):
        issues.append("private investigation report")
    parts = pathlib.PurePosixPath(path).parts
    name = parts[-1]
    if any(x in ('data', 'node_modules', '.agents', '.git', '__pycache__') for x in parts):
        issues.append('runtime/private directory')
    if (name.startswith('.env') and name != '.env.example') or name.endswith(DENIED_SUFFIXES):
        issues.append('runtime or secret file')
    if SECRET.search(content):
        issues.append('credential/private-key pattern')
    if content[:4] in (b'\x7fELF', b'\xcf\xfa\xed\xfe', b'\xfe\xed\xfa\xcf', b'\xca\xfe\xba\xbe'):
        issues.append('compiled executable')
    if len(content) > 10 * 1024 * 1024:
        issues.append('unexpected large source file')
    return issues

if __name__ == '__main__':
    files = subprocess.check_output(['git','ls-files','-z'],cwd=ROOT).decode().split('\0')
    failures = []
    for name in filter(None, files):
        path = ROOT / name
        if not path.is_file():
            failures.append((name, ['missing file or unresolved submodule']))
            continue
        found = problems(name,path.read_bytes())
        if found: failures.append((name,found))
    for name, reasons in failures:
        print(name + ': ' + ', '.join(reasons), file=sys.stderr)
    print(f'Checked {len(list(filter(None,files)))} tracked source files; {len(failures)} failures.')
    sys.exit(bool(failures))

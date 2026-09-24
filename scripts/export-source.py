#!/usr/bin/env python3
"""Export a complete working-source snapshot, without Git history or runtime data.
Does not compile, install dependencies, start services, or modify the source index.
"""
import importlib.util
import pathlib
import shutil
import subprocess
import sys

sys.dont_write_bytecode = True
ROOT = pathlib.Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('check_source',ROOT/'scripts/check-source.py')
check=importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)

def paths(base):
    raw=subprocess.check_output(['git','ls-files','--cached','--others','--exclude-standard','-z'],cwd=base)
    return sorted(set(filter(None,raw.decode().split('\0'))))

def export(destination):
    if destination.exists():
        raise SystemExit('Destination must not exist; use a fresh directory to preserve prior snapshots.')
    selected=[]
    for name in paths(ROOT):
        if name in ('.gitmodules','integrations/onebot-qzone','skills-lock.json','docs/data-collection-known-issues.md'):continue
        if name.startswith('integrations/onebot-qzone/'):continue
        if name.startswith(('.agents/','.claude/','docs/mcp-readonly-analysis-')):continue
        path=ROOT/name
        if path.is_file():selected.append((name,path))
    vendor=ROOT/'integrations/onebot-qzone'
    for name in paths(vendor):
        path=vendor/name
        if path.is_file() and not name.startswith('.claude/'):selected.append(('integrations/onebot-qzone/'+name,path))
    errors=[]
    for name,path in selected:
        issues=check.problems(name,path.read_bytes())
        if issues:errors.append(name+': '+', '.join(issues))
    if errors:raise SystemExit('\n'.join(errors))
    for name,path in selected:
        target=destination/name
        target.parent.mkdir(parents=True,exist_ok=True)
        shutil.copy2(path,target)
    print(f'Exported {len(selected)} source files to {destination}')

if __name__=='__main__':
    if len(sys.argv)!=2:raise SystemExit('Usage: python3 scripts/export-source.py NEW_DIRECTORY')
    export(pathlib.Path(sys.argv[1]).resolve())

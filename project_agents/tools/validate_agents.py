#!/usr/bin/env python3
from pathlib import Path
import hashlib, json, sys
repo=Path(__file__).resolve().parents[2]; pa=repo/'project_agents'
manifest=json.loads((pa/'manifest.json').read_text()); errors=[]
for m in manifest:
    c=pa/'canonical'/m['tier'].lower()/f"{m['name']}.md"; sha=hashlib.sha256(c.read_bytes()).hexdigest()
    if sha!=m['sha256']: errors.append(f"canonical changed without sync: {m['name']}")
    for f in [repo/'.cursor'/'agents'/f"{m['name']}.md",repo/'.codex'/'agents'/f"{m['name']}.toml",repo/'.agents'/'agents'/f"{m['name']}.md"]:
        if not f.exists() or sha not in f.read_text(): errors.append(f"adapter drift/missing: {f.relative_to(repo)}")
if errors:
    print('FAIL'); [print('-',x) for x in errors]; sys.exit(1)
print(f'PASS: {len(manifest)} Platform agents synchronized.')

#!/usr/bin/env python3
from pathlib import Path
import hashlib, json
repo=Path(__file__).resolve().parents[2]
pa=repo/'project_agents'
manifest=json.loads((pa/'manifest.json').read_text())
for m in manifest:
    canonical=pa/'canonical'/m['tier'].lower()/f"{m['name']}.md"
    body=canonical.read_text()
    sha=hashlib.sha256(body.encode()).hexdigest(); m['sha256']=sha
    name=m['name']; desc=m['description']; mode=m['mode']
    cp=repo/'.cursor'/'agents'/f'{name}.md'; cp.parent.mkdir(parents=True,exist_ok=True)
    cp.write_text(f"---\nname: {name}\ndescription: {desc}\n---\n\n<!-- generated from project_agents/{canonical.relative_to(pa)} sha256:{sha} -->\n\n{body}")
    tools=['view_file','grep_search','run_command'] + (['replace_file_content'] if mode=='write' else [])
    ty='\n'.join(f'  - {x}' for x in tools)
    ap=repo/'.agents'/'agents'/f'{name}.md'; ap.parent.mkdir(parents=True,exist_ok=True)
    ap.write_text(f"---\nname: {name}\ndescription: {desc}\ntools:\n{ty}\nmainAgent: false\nsubagent: true\nmodel: inherit\ncommandExecutionPolicy: sandbox\n---\n\n<!-- generated from project_agents/{canonical.relative_to(pa)} sha256:{sha} -->\n\n{body}")
    tb=body.replace('"""','\\"\\"\\"'); sandbox='read-only' if mode=='read' else 'workspace-write'
    xp=repo/'.codex'/'agents'/f'{name}.toml'; xp.parent.mkdir(parents=True,exist_ok=True)
    xp.write_text(f'name = "{name}"\ndescription = {json.dumps(desc)}\nsandbox_mode = "{sandbox}"\ndeveloper_instructions = """\nGenerated from project_agents/{canonical.relative_to(pa)} sha256:{sha}\n\n{tb}\n"""\n')
(pa/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
print(f'Synchronized {len(manifest)} Platform agents.')

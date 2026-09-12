#!/usr/bin/env python3
from pathlib import Path
import hashlib
import json

repo = Path(__file__).resolve().parents[2]
pa = repo / "project_agents"
manifest = json.loads((pa / "manifest.json").read_text())

for entry in manifest:
    canonical = pa / "canonical" / entry["tier"].lower() / f"{entry['name']}.md"
    body = canonical.read_text()
    sha = hashlib.sha256(body.encode()).hexdigest()
    entry["sha256"] = sha
    name = entry["name"]
    desc = entry["description"]
    mode = entry["mode"]

    cursor_path = repo / ".cursor" / "agents" / f"{name}.md"
    cursor_path.parent.mkdir(parents=True, exist_ok=True)
    cursor_path.write_text(
        f"---\nname: {name}\ndescription: {desc}\n---\n\n"
        f"<!-- generated from project_agents/{canonical.relative_to(pa)} sha256:{sha} -->\n\n{body}"
    )

    tools = ["view_file", "grep_search", "run_command"]
    if mode == "write":
        tools.append("replace_file_content")
    tool_yaml = "\n".join(f"  - {tool}" for tool in tools)
    antigravity_path = repo / ".agents" / "agents" / f"{name}.md"
    antigravity_path.parent.mkdir(parents=True, exist_ok=True)
    antigravity_path.write_text(
        f"---\nname: {name}\ndescription: {desc}\ntools:\n{tool_yaml}\n"
        "mainAgent: false\nsubagent: true\nmodel: inherit\ncommandExecutionPolicy: sandbox\n---\n\n"
        f"<!-- generated from project_agents/{canonical.relative_to(pa)} sha256:{sha} -->\n\n{body}"
    )

    escaped_body = body.replace('"""', '\\"\\"\\"')
    sandbox = "read-only" if mode == "read" else "workspace-write"
    codex_path = repo / ".codex" / "agents" / f"{name}.toml"
    codex_path.parent.mkdir(parents=True, exist_ok=True)
    codex_path.write_text(
        f"name = {json.dumps(name)}\n"
        f"description = {json.dumps(desc)}\n"
        f"sandbox_mode = {json.dumps(sandbox)}\n"
        'developer_instructions = """\n'
        f"Generated from project_agents/{canonical.relative_to(pa)} sha256:{sha}\n\n"
        f"{escaped_body}\n"
        '"""\n'
    )

(pa / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")

skills_root = pa / "skills"
skill_manifest_path = skills_root / "manifest.json"
if skill_manifest_path.exists():
    skill_manifest = json.loads(skill_manifest_path.read_text())
    declared_names = [entry["name"] for entry in skill_manifest]
    if len(declared_names) != len(set(declared_names)):
        raise SystemExit("duplicate skill names in project_agents/skills/manifest.json")

    generated_root = repo / "skills"
    generated_root.mkdir(parents=True, exist_ok=True)
    for skill in skill_manifest:
        name = skill["name"]
        canonical_skill = skills_root / name / "SKILL.md"
        if not canonical_skill.is_file():
            raise SystemExit(f"missing canonical skill: {canonical_skill.relative_to(repo)}")
        destination = generated_root / name / "SKILL.md"
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(canonical_skill.read_text())

    declared = set(declared_names)
    for directory in generated_root.iterdir():
        if directory.is_dir() and directory.name not in declared:
            raise SystemExit(f"unexpected generated root skill directory: {directory.relative_to(repo)}")

print(f"Synchronized {len(manifest)} Platform agents and canonical root skills.")

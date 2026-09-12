#!/usr/bin/env python3
from collections import Counter
from pathlib import Path
import hashlib
import json
import sys

repo = Path(__file__).resolve().parents[2]
pa = repo / "project_agents"
manifest = json.loads((pa / "manifest.json").read_text())
errors: list[str] = []


def canonical_path(entry: dict) -> Path:
    return pa / "canonical" / entry["tier"].lower() / f"{entry['name']}.md"


def source_ref(path: Path) -> str:
    return path.relative_to(pa).as_posix()


def render_cursor(entry: dict, body: str, sha: str, canonical: Path) -> str:
    return (
        f"---\nname: {entry['name']}\n"
        f"description: {entry['description']}\n---\n\n"
        f"<!-- generated from project_agents/{source_ref(canonical)} sha256:{sha} -->\n\n{body}"
    )


def render_antigravity(entry: dict, body: str, sha: str, canonical: Path) -> str:
    tools = ["view_file", "grep_search", "run_command"]
    if entry["mode"] == "write":
        tools.append("replace_file_content")
    tool_yaml = "\n".join(f"  - {tool}" for tool in tools)
    return (
        f"---\nname: {entry['name']}\n"
        f"description: {entry['description']}\n"
        f"tools:\n{tool_yaml}\n"
        "mainAgent: false\nsubagent: true\nmodel: inherit\ncommandExecutionPolicy: sandbox\n---\n\n"
        f"<!-- generated from project_agents/{source_ref(canonical)} sha256:{sha} -->\n\n{body}"
    )


def render_codex(entry: dict, body: str, sha: str, canonical: Path) -> str:
    escaped_body = body.replace('"""', '\\"\\"\\"')
    sandbox = "read-only" if entry["mode"] == "read" else "workspace-write"
    return (
        f"name = {json.dumps(entry['name'])}\n"
        f"description = {json.dumps(entry['description'])}\n"
        f"sandbox_mode = {json.dumps(sandbox)}\n"
        'developer_instructions = """\n'
        f"Generated from project_agents/{source_ref(canonical)} sha256:{sha}\n\n"
        f"{escaped_body}\n"
        '"""\n'
    )


names = [entry.get("name") for entry in manifest]
if not manifest:
    errors.append("manifest must declare at least one agent")
if len(set(names)) != len(names):
    duplicates = sorted(name for name, count in Counter(names).items() if count > 1)
    errors.append(f"duplicate manifest agent names: {', '.join(duplicates)}")

for entry in manifest:
    missing_fields = [field for field in ("name", "description", "mode", "tier", "sha256") if not entry.get(field)]
    if missing_fields:
        errors.append(f"manifest entry {entry.get('name', '<unnamed>')} missing fields: {', '.join(missing_fields)}")
    if entry.get("mode") not in {"read", "write"}:
        errors.append(f"manifest entry {entry.get('name', '<unnamed>')} has invalid mode {entry.get('mode')}")

expected_canonical = {
    canonical_path(entry).relative_to(pa).as_posix()
    for entry in manifest
    if entry.get("name") and entry.get("tier")
}
actual_canonical = {path.relative_to(pa).as_posix() for path in (pa / "canonical").rglob("*.md")}
if actual_canonical != expected_canonical:
    missing = sorted(expected_canonical - actual_canonical)
    extra = sorted(actual_canonical - expected_canonical)
    if missing:
        errors.append(f"missing canonical agents: {', '.join(missing)}")
    if extra:
        errors.append(f"unmanifested canonical agents: {', '.join(extra)}")

expected_adapter_paths = {
    repo / ".cursor" / "agents": {f"{entry['name']}.md" for entry in manifest},
    repo / ".codex" / "agents": {f"{entry['name']}.toml" for entry in manifest},
    repo / ".agents" / "agents": {f"{entry['name']}.md" for entry in manifest},
}
for directory, expected_names in expected_adapter_paths.items():
    actual_names = {path.name for path in directory.iterdir() if path.is_file()} if directory.exists() else set()
    if actual_names != expected_names:
        missing = sorted(expected_names - actual_names)
        extra = sorted(actual_names - expected_names)
        if missing:
            errors.append(f"missing adapters in {directory.relative_to(repo)}: {', '.join(missing)}")
        if extra:
            errors.append(f"unexpected adapters in {directory.relative_to(repo)}: {', '.join(extra)}")

for entry in manifest:
    canonical = canonical_path(entry)
    if not canonical.exists():
        continue
    body = canonical.read_text()
    sha = hashlib.sha256(body.encode()).hexdigest()
    if sha != entry.get("sha256"):
        errors.append(f"canonical changed without sync: {entry['name']}")
    expected_files = {
        repo / ".cursor" / "agents" / f"{entry['name']}.md": render_cursor(entry, body, sha, canonical),
        repo / ".codex" / "agents" / f"{entry['name']}.toml": render_codex(entry, body, sha, canonical),
        repo / ".agents" / "agents" / f"{entry['name']}.md": render_antigravity(entry, body, sha, canonical),
    }
    for path, expected_content in expected_files.items():
        if not path.exists():
            errors.append(f"adapter missing: {path.relative_to(repo)}")
        elif path.read_text() != expected_content:
            errors.append(f"adapter drift: {path.relative_to(repo)}")

skill_manifest_path = pa / "skills" / "manifest.json"
if not skill_manifest_path.is_file():
    errors.append("missing project_agents/skills/manifest.json")
else:
    skill_manifest = json.loads(skill_manifest_path.read_text())
    skill_names = [entry.get("name") for entry in skill_manifest]
    if not skill_manifest:
        errors.append("skill manifest must declare at least one skill")
    if any(not name for name in skill_names):
        errors.append("skill manifest contains unnamed skill")
    if len(set(skill_names)) != len(skill_names):
        duplicates = sorted(name for name, count in Counter(skill_names).items() if count > 1)
        errors.append(f"duplicate skill names: {', '.join(duplicates)}")

    declared = {name for name in skill_names if name}
    canonical_dirs = {path.name for path in (pa / "skills").iterdir() if path.is_dir()}
    if canonical_dirs != declared:
        missing = sorted(declared - canonical_dirs)
        extra = sorted(canonical_dirs - declared)
        if missing:
            errors.append(f"missing canonical skill directories: {', '.join(missing)}")
        if extra:
            errors.append(f"unmanifested canonical skill directories: {', '.join(extra)}")

    generated_root = repo / "skills"
    generated_dirs = {path.name for path in generated_root.iterdir() if path.is_dir()} if generated_root.exists() else set()
    if generated_dirs != declared:
        missing = sorted(declared - generated_dirs)
        extra = sorted(generated_dirs - declared)
        if missing:
            errors.append(f"missing generated root skill directories: {', '.join(missing)}")
        if extra:
            errors.append(f"unexpected generated root skill directories: {', '.join(extra)}")

    for name in sorted(declared):
        canonical_skill = pa / "skills" / name / "SKILL.md"
        generated_skill = repo / "skills" / name / "SKILL.md"
        if not canonical_skill.is_file():
            errors.append(f"canonical skill missing SKILL.md: project_agents/skills/{name}")
            continue
        if not generated_skill.is_file():
            errors.append(f"generated root skill missing SKILL.md: skills/{name}")
            continue
        if generated_skill.read_text() != canonical_skill.read_text():
            errors.append(f"generated root skill drift: skills/{name}/SKILL.md")

if errors:
    print("FAIL")
    for error in errors:
        print("-", error)
    sys.exit(1)

print(
    f"PASS: {len(manifest)} manifest-declared Platform agents and "
    f"{len(skill_manifest)} manifest-declared skills exactly synchronized."
)

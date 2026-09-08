#!/usr/bin/env python3
from collections import Counter
from pathlib import Path
import hashlib
import json
import sys

EXPECTED_TOTAL = 27
EXPECTED_TIERS = {"P0": 12, "P1": 15}

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
        f"---\n"
        f"name: {entry['name']}\n"
        f"description: {entry['description']}\n"
        f"---\n\n"
        f"<!-- generated from project_agents/{source_ref(canonical)} sha256:{sha} -->\n\n"
        f"{body}"
    )


def render_antigravity(entry: dict, body: str, sha: str, canonical: Path) -> str:
    tools = ["view_file", "grep_search", "run_command"]
    if entry["mode"] == "write":
        tools.append("replace_file_content")
    tool_yaml = "\n".join(f"  - {tool}" for tool in tools)
    return (
        f"---\n"
        f"name: {entry['name']}\n"
        f"description: {entry['description']}\n"
        f"tools:\n{tool_yaml}\n"
        f"mainAgent: false\n"
        f"subagent: true\n"
        f"model: inherit\n"
        f"commandExecutionPolicy: sandbox\n"
        f"---\n\n"
        f"<!-- generated from project_agents/{source_ref(canonical)} sha256:{sha} -->\n\n"
        f"{body}"
    )


def render_codex(entry: dict, body: str, sha: str, canonical: Path) -> str:
    escaped_body = body.replace('"""', '\\"\\"\\"')
    sandbox = "read-only" if entry["mode"] == "read" else "workspace-write"
    return (
        f"name = {json.dumps(entry['name'])}\n"
        f"description = {json.dumps(entry['description'])}\n"
        f"sandbox_mode = {json.dumps(sandbox)}\n"
        f"developer_instructions = \"\"\"\n"
        f"Generated from project_agents/{source_ref(canonical)} sha256:{sha}\n\n"
        f"{escaped_body}\n"
        f"\"\"\"\n"
    )


names = [entry.get("name") for entry in manifest]
if len(manifest) != EXPECTED_TOTAL:
    errors.append(f"manifest agent count {len(manifest)} != expected {EXPECTED_TOTAL}")

if len(set(names)) != len(names):
    duplicates = sorted(name for name, count in Counter(names).items() if count > 1)
    errors.append(f"duplicate manifest agent names: {', '.join(duplicates)}")

tier_counts = Counter(entry.get("tier") for entry in manifest)
for tier, expected in EXPECTED_TIERS.items():
    if tier_counts.get(tier, 0) != expected:
        errors.append(f"manifest {tier} count {tier_counts.get(tier, 0)} != expected {expected}")

expected_canonical = {canonical_path(entry).relative_to(pa).as_posix() for entry in manifest}
actual_canonical = {
    path.relative_to(pa).as_posix()
    for path in (pa / "canonical").rglob("*.md")
}
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
            continue
        if path.read_text() != expected_content:
            errors.append(f"adapter drift: {path.relative_to(repo)}")

if errors:
    print("FAIL")
    for error in errors:
        print("-", error)
    sys.exit(1)

print(f"PASS: {len(manifest)} Platform agents exactly synchronized.")

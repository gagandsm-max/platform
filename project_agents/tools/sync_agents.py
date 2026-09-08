#!/usr/bin/env python3
from pathlib import Path
import hashlib
import json

repo = Path(__file__).resolve().parents[2]
pa = repo / "project_agents"
manifest = json.loads((pa / "manifest.json").read_text())

READ_ANTIGRAVITY_WRAPPER = """> **READ-MODE NATIVE BOUNDARY:** This adapter is repository read-only by default. Do not modify repository/files; do not perform git writes; do not use shell redirection or in-place editing; do not install dependencies. `run_command` is limited to inspection or required test/evidence commands. Any command capable of changing workspace state requires explicit parent/user approval before invocation.

"""


def canonical_path(entry: dict) -> Path:
    return pa / "canonical" / entry["tier"].lower() / f"{entry['name']}.md"


def source_ref(path: Path) -> str:
    return path.relative_to(pa).as_posix()


def render_cursor(entry: dict, body: str, sha: str, canonical: Path) -> str:
    readonly = "true" if entry["mode"] == "read" else "false"
    return (
        f"---\n"
        f"name: {entry['name']}\n"
        f"description: {entry['description']}\n"
        f"model: inherit\n"
        f"readonly: {readonly}\n"
        f"---\n\n"
        f"<!-- generated from project_agents/{source_ref(canonical)} sha256:{sha} -->\n\n"
        f"{body}"
    )


def render_antigravity(entry: dict, body: str, sha: str, canonical: Path) -> str:
    tools = ["view_file", "grep_search", "run_command"]
    if entry["mode"] == "write":
        tools.append("replace_file_content")
    tool_yaml = "\n".join(f"  - {tool}" for tool in tools)
    policy = "off" if entry["mode"] == "read" else "sandbox"
    wrapper = READ_ANTIGRAVITY_WRAPPER if entry["mode"] == "read" else ""
    return (
        f"---\n"
        f"name: {entry['name']}\n"
        f"description: {entry['description']}\n"
        f"tools:\n{tool_yaml}\n"
        f"mainAgent: false\n"
        f"subagent: true\n"
        f"model: inherit\n"
        f"commandExecutionPolicy: {policy}\n"
        f"---\n\n"
        f"<!-- generated from project_agents/{source_ref(canonical)} sha256:{sha} -->\n\n"
        f"{wrapper}"
        f"{body}"
    )


def render_codex(entry: dict, body: str, sha: str, canonical: Path) -> str:
    escaped_body = body.replace('"""', '\\\"\\\"\\\"')
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


for entry in manifest:
    canonical = canonical_path(entry)
    body = canonical.read_text()
    sha = hashlib.sha256(body.encode()).hexdigest()
    entry["sha256"] = sha

    cursor_path = repo / ".cursor" / "agents" / f"{entry['name']}.md"
    cursor_path.parent.mkdir(parents=True, exist_ok=True)
    cursor_path.write_text(render_cursor(entry, body, sha, canonical))

    antigravity_path = repo / ".agents" / "agents" / f"{entry['name']}.md"
    antigravity_path.parent.mkdir(parents=True, exist_ok=True)
    antigravity_path.write_text(render_antigravity(entry, body, sha, canonical))

    codex_path = repo / ".codex" / "agents" / f"{entry['name']}.toml"
    codex_path.parent.mkdir(parents=True, exist_ok=True)
    codex_path.write_text(render_codex(entry, body, sha, canonical))

(pa / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
print(f"Synchronized {len(manifest)} Platform agents.")

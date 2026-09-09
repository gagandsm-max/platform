#!/usr/bin/env python3
"""Platform Codex Session Launcher.

Projects manifest-defined specialist agent roles into native Codex CLI execution sessions
with deterministic sandbox enforcement and canonical developer instructions.
"""
from pathlib import Path
import argparse
import json
import subprocess
import sys
import tomllib

REPO_ROOT = Path(__file__).resolve().parents[2]
MANIFEST_PATH = REPO_ROOT / "project_agents" / "manifest.json"
CODEX_AGENTS_DIR = REPO_ROOT / ".codex" / "agents"


def get_agent_entry(manifest: list[dict], name: str) -> dict:
    for entry in manifest:
        if entry.get("name") == name:
            return entry
    raise ValueError(f"Agent '{name}' is not manifested in {MANIFEST_PATH}")


def resolve_role_config(agent_name: str) -> dict:
    if not MANIFEST_PATH.is_file():
        raise FileNotFoundError(f"Manifest not found: {MANIFEST_PATH}")
    manifest = json.loads(MANIFEST_PATH.read_text())
    entry = get_agent_entry(manifest, agent_name)

    mode = entry.get("mode")
    if mode == "read":
        effective_sandbox = "read-only"
    elif mode == "write":
        effective_sandbox = "workspace-write"
    else:
        raise ValueError(f"Invalid mode '{mode}' for agent '{agent_name}'")

    role_file = CODEX_AGENTS_DIR / f"{agent_name}.toml"
    if not role_file.is_file():
        raise FileNotFoundError(f"Role file not found: {role_file}")

    role_toml = tomllib.loads(role_file.read_text())
    instructions = role_toml.get("developer_instructions", "")
    expected_marker = f"Generated from project_agents/canonical/{entry['tier'].lower()}/{entry['name']}.md sha256:{entry['sha256']}"
    if expected_marker not in instructions:
        raise ValueError(f"Canonical provenance marker missing or mismatched in {role_file}")

    return {
        "agent": agent_name,
        "tier": entry.get("tier"),
        "manifest_mode": mode,
        "effective_sandbox": effective_sandbox,
        "role_source": str(role_file.relative_to(REPO_ROOT)),
        "provenance_sha": entry.get("sha256"),
        "developer_instructions": instructions,
    }


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Launch a Platform specialist agent in an authoritative Codex CLI session."
    )
    parser.add_argument("agent", help="Name of the manifested Platform specialist agent")
    parser.add_argument("prompt", nargs="?", default=None, help="Optional user prompt / instructions")
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print structured projection plan without executing Codex",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Output plan as JSON (used with --dry-run)",
    )
    args = parser.parse_args()

    try:
        config = resolve_role_config(args.agent)
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        sys.exit(1)

    if args.dry_run:
        plan = {
            "agent": config["agent"],
            "tier": config["tier"],
            "manifest_mode": config["manifest_mode"],
            "effective_sandbox": config["effective_sandbox"],
            "role_source": config["role_source"],
            "provenance_sha": config["provenance_sha"],
        }
        if args.json:
            print(json.dumps(plan, indent=2))
        else:
            print(f"Agent:             {plan['agent']} ({plan['tier']})")
            print(f"Manifest Mode:     {plan['manifest_mode']}")
            print(f"Effective Sandbox: {plan['effective_sandbox']}")
            print(f"Role Source:       {plan['role_source']}")
            print(f"Provenance SHA:    {plan['provenance_sha']}")
        sys.exit(0)

    # Build codex exec command
    # Authoritative session sandbox is mapped strictly from manifest mode
    # Developer instructions are injected via -c developer_instructions="..."
    instructions_json = json.dumps(config["developer_instructions"])
    cmd = [
        "codex",
        "exec",
        "-C",
        str(REPO_ROOT),
        "-s",
        config["effective_sandbox"],
        "-c",
        f"developer_instructions={instructions_json}",
    ]

    prompt = args.prompt
    if prompt is None and not sys.stdin.isatty():
        stdin_content = sys.stdin.read().strip()
        if stdin_content:
            prompt = stdin_content

    if prompt is not None:
        cmd.append(prompt)

    try:
        proc = subprocess.run(cmd)
        sys.exit(proc.returncode)
    except FileNotFoundError:
        print("Error: 'codex' executable not found in PATH", file=sys.stderr)
        sys.exit(127)


if __name__ == "__main__":
    main()

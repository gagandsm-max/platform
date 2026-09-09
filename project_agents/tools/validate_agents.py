#!/usr/bin/env python3
from collections import Counter
from pathlib import Path
import hashlib
import json
import sys
import tomllib

EXPECTED_TOTAL = 27
EXPECTED_TIERS = {"P0": 12, "P1": 15}
EXPECTED_MODES = {"read": 15, "write": 12}
ALLOWED_MODES = {"read", "write"}
ANTIGRAVITY_READ_TOOLS = ["view_file", "grep_search", "run_command"]
ANTIGRAVITY_WRITE_TOOLS = ANTIGRAVITY_READ_TOOLS + ["replace_file_content"]
REQUIRED_ASSETS = [
    "AGENTS.md",
    "project_agents/AGENT_MATRIX.md",
    "project_agents/SOURCE_MAPPING.md",
    "project_agents/USAGE_PLAYBOOK.md",
    "project_agents/START_HERE.md",
    "project_agents/TASK_PROMPT_TEMPLATE.md",
    "project_agents/CHATGPT_AGENT_ROUTER.md",
    "project_agents/OPERATIONAL_SMOKE_TEST.md",
    "project_agents/tools/run_codex_agent.py",
    "project_agents/tools/test_codex_launcher.py",
    ".agents/rules/platform-authority.md",
]
ROOT_AGENT_ANCHORS = [
    "## Platform Specialist Agent Pack",
    "project_agents/canonical/",
    "project_agents/AGENT_MATRIX.md",
    "project_agents/USAGE_PLAYBOOK.md",
    ".cursor/agents/",
    ".codex/agents/",
    ".agents/agents/",
]
READ_ANTIGRAVITY_WRAPPER = """> **READ-MODE NATIVE BOUNDARY:** This adapter is repository read-only by default. Do not modify repository/files; do not perform git writes; do not use shell redirection or in-place editing; do not install dependencies. `run_command` is limited to inspection or required test/evidence commands. Any command capable of changing workspace state requires explicit parent/user approval before invocation.

"""

repo = Path(__file__).resolve().parents[2]
pa = repo / "project_agents"
manifest = json.loads((pa / "manifest.json").read_text())
errors: list[str] = []


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
    tools = ANTIGRAVITY_WRITE_TOOLS if entry["mode"] == "write" else ANTIGRAVITY_READ_TOOLS
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
    return (
        f"name = {json.dumps(entry['name'])}\n"
        f"description = {json.dumps(entry['description'])}\n"
        f"developer_instructions = \"\"\"\n"
        f"Generated from project_agents/{source_ref(canonical)} sha256:{sha}\n\n"
        f"{escaped_body}\n"
        f"\"\"\"\n"
    )


def parse_frontmatter(path: Path) -> dict:
    text = path.read_text()
    if not text.startswith("---\n"):
        raise ValueError("missing opening frontmatter delimiter")
    end = text.find("\n---\n", 4)
    if end < 0:
        raise ValueError("missing closing frontmatter delimiter")
    lines = text[4:end].splitlines()
    result: dict[str, object] = {}
    index = 0
    while index < len(lines):
        line = lines[index]
        if not line or line.startswith(" ") or ":" not in line:
            raise ValueError(f"unsupported frontmatter line: {line!r}")
        key, value = line.split(":", 1)
        value = value.strip()
        if key == "tools":
            if value:
                raise ValueError("tools must be a block list")
            tools: list[str] = []
            index += 1
            while index < len(lines) and lines[index].startswith("  - "):
                tools.append(lines[index][4:].strip())
                index += 1
            result[key] = tools
            continue
        if value == "true":
            parsed: object = True
        elif value == "false":
            parsed = False
        else:
            parsed = value
        result[key] = parsed
        index += 1
    return result


names = [entry.get("name") for entry in manifest]
if len(manifest) != EXPECTED_TOTAL:
    errors.append(f"manifest agent count {len(manifest)} != expected {EXPECTED_TOTAL}")

if len(set(names)) != len(names):
    duplicates = sorted(name for name, count in Counter(names).items() if count > 1)
    errors.append(f"duplicate manifest agent names: {', '.join(duplicates)}")

mode_counts = Counter(entry.get("mode") for entry in manifest)
unknown_modes = sorted(mode for mode in mode_counts if mode not in ALLOWED_MODES)
if unknown_modes:
    errors.append(f"unsupported manifest modes: {', '.join(str(mode) for mode in unknown_modes)}")
for mode, expected in EXPECTED_MODES.items():
    if mode_counts.get(mode, 0) != expected:
        errors.append(f"manifest {mode.upper()} count {mode_counts.get(mode, 0)} != expected {expected}")

for entry in manifest:
    required = {"name", "description", "tier", "mode", "canonical_path", "sha256"}
    missing = sorted(required - set(entry))
    if missing:
        errors.append(f"manifest entry missing fields {entry.get('name', '<unnamed>')}: {', '.join(missing)}")
        continue
    expected_manifest_path = f"canonical/{entry['tier'].lower()}/{entry['name']}.md"
    if entry.get("canonical_path") != expected_manifest_path:
        errors.append(
            f"manifest canonical_path mismatch {entry['name']}: "
            f"{entry.get('canonical_path')} != {expected_manifest_path}"
        )


tier_counts = Counter(entry.get("tier") for entry in manifest)
for tier, expected in EXPECTED_TIERS.items():
    if tier_counts.get(tier, 0) != expected:
        errors.append(f"manifest {tier} count {tier_counts.get(tier, 0)} != expected {expected}")

expected_canonical = {canonical_path(entry).relative_to(pa).as_posix() for entry in manifest}
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

for asset in REQUIRED_ASSETS:
    if not (repo / asset).is_file():
        errors.append(f"required agent governance asset missing: {asset}")

root_agents = (repo / "AGENTS.md").read_text() if (repo / "AGENTS.md").is_file() else ""
for anchor in ROOT_AGENT_ANCHORS:
    if anchor not in root_agents:
        errors.append(f"root AGENTS specialist-pack anchor missing: {anchor}")

for entry in manifest:
    canonical = canonical_path(entry)
    if not canonical.exists():
        continue

    body = canonical.read_text()
    sha = hashlib.sha256(body.encode()).hexdigest()
    if sha != entry.get("sha256"):
        errors.append(f"canonical changed without sync: {entry['name']}")

    cursor_path = repo / ".cursor" / "agents" / f"{entry['name']}.md"
    antigravity_path = repo / ".agents" / "agents" / f"{entry['name']}.md"
    codex_path = repo / ".codex" / "agents" / f"{entry['name']}.toml"

    expected_files = {
        cursor_path: render_cursor(entry, body, sha, canonical),
        codex_path: render_codex(entry, body, sha, canonical),
        antigravity_path: render_antigravity(entry, body, sha, canonical),
    }
    for path, expected_content in expected_files.items():
        if not path.exists():
            errors.append(f"adapter missing: {path.relative_to(repo)}")
            continue
        if path.read_text() != expected_content:
            errors.append(f"adapter drift: {path.relative_to(repo)}")

    if cursor_path.exists():
        try:
            cursor = parse_frontmatter(cursor_path)
            if cursor.get("name") != entry["name"]:
                errors.append(f"Cursor name mismatch: {entry['name']}")
            if cursor.get("description") != entry["description"]:
                errors.append(f"Cursor description mismatch: {entry['name']}")
            if cursor.get("model") != "inherit":
                errors.append(f"Cursor model must inherit: {entry['name']}")
            expected_readonly = entry["mode"] == "read"
            if cursor.get("readonly") is not expected_readonly:
                errors.append(f"Cursor readonly mismatch for {entry['mode']} agent: {entry['name']}")
        except ValueError as exc:
            errors.append(f"Cursor frontmatter invalid {entry['name']}: {exc}")

    if antigravity_path.exists():
        try:
            antigravity = parse_frontmatter(antigravity_path)
            expected_tools = ANTIGRAVITY_READ_TOOLS if entry["mode"] == "read" else ANTIGRAVITY_WRITE_TOOLS
            if antigravity.get("name") != entry["name"]:
                errors.append(f"Antigravity name mismatch: {entry['name']}")
            if antigravity.get("description") != entry["description"]:
                errors.append(f"Antigravity description mismatch: {entry['name']}")
            if antigravity.get("tools") != expected_tools:
                errors.append(f"Antigravity tools mismatch: {entry['name']}")
            if antigravity.get("mainAgent") is not False:
                errors.append(f"Antigravity mainAgent must be false: {entry['name']}")
            if antigravity.get("subagent") is not True:
                errors.append(f"Antigravity subagent must be true: {entry['name']}")
            if antigravity.get("model") != "inherit":
                errors.append(f"Antigravity model must inherit: {entry['name']}")
            expected_policy = "off" if entry["mode"] == "read" else "sandbox"
            if antigravity.get("commandExecutionPolicy") != expected_policy:
                errors.append(f"Antigravity command policy mismatch: {entry['name']}")
            if entry["mode"] == "read" and READ_ANTIGRAVITY_WRAPPER not in antigravity_path.read_text():
                errors.append(f"Antigravity READ boundary missing: {entry['name']}")
        except ValueError as exc:
            errors.append(f"Antigravity frontmatter invalid {entry['name']}: {exc}")

    if codex_path.exists():
        try:
            codex = tomllib.loads(codex_path.read_text())
            if codex.get("name") != entry["name"]:
                errors.append(f"Codex name mismatch: {entry['name']}")
            if codex.get("description") != entry["description"]:
                errors.append(f"Codex description mismatch: {entry['name']}")
            if "sandbox_mode" in codex:
                errors.append(f"Codex role TOML must not declare role-level sandbox_mode: {entry['name']}")
            if "default_permissions" in codex:
                errors.append(f"Codex role TOML must not declare default_permissions: {entry['name']}")
            if "permissions" in codex or "permission_profile" in codex:
                errors.append(f"Codex role TOML must not declare permissions: {entry['name']}")
            instructions = codex.get("developer_instructions", "")
            expected_marker = f"Generated from project_agents/{source_ref(canonical)} sha256:{sha}"
            if expected_marker not in instructions:
                errors.append(f"Codex canonical SHA marker mismatch: {entry['name']}")
        except tomllib.TOMLDecodeError as exc:
            errors.append(f"Codex TOML invalid {entry['name']}: {exc}")

launcher_path = pa / "tools" / "run_codex_agent.py"
if not launcher_path.is_file():
    errors.append("controlled Codex launcher missing: project_agents/tools/run_codex_agent.py")
else:
    launcher_text = launcher_path.read_text()
    if "danger-full-access" in launcher_text:
        errors.append("controlled Codex launcher must not contain danger-full-access")
    if "shell=True" in launcher_text:
        errors.append("controlled Codex launcher must not use shell=True")

    import subprocess

    for entry in manifest:
        proc = subprocess.run(
            [sys.executable, str(launcher_path), "--dry-run", "--json", entry["name"]],
            capture_output=True,
            text=True,
        )
        if proc.returncode != 0:
            errors.append(f"launcher dry-run failed for {entry['name']}: {proc.stderr.strip()}")
            continue
        try:
            plan = json.loads(proc.stdout)
            expected_sandbox = "read-only" if entry["mode"] == "read" else "workspace-write"
            if plan.get("manifest_mode") != entry["mode"]:
                errors.append(f"launcher dry-run manifest_mode mismatch for {entry['name']}")
            if plan.get("effective_sandbox") != expected_sandbox:
                errors.append(
                    f"launcher dry-run effective_sandbox mismatch for {entry['name']}: "
                    f"{plan.get('effective_sandbox')} != {expected_sandbox}"
                )
            if plan.get("role_source") != f".codex/agents/{entry['name']}.toml":
                errors.append(f"launcher dry-run role_source mismatch for {entry['name']}")
            if plan.get("provenance_sha") != entry["sha256"]:
                errors.append(f"launcher dry-run provenance SHA mismatch for {entry['name']}")
        except json.JSONDecodeError:
            errors.append(f"launcher dry-run returned invalid JSON for {entry['name']}")

    proc_unknown = subprocess.run(
        [sys.executable, str(launcher_path), "--dry-run", "platform-nonexistent-agent"],
        capture_output=True,
        text=True,
    )
    if proc_unknown.returncode == 0:
        errors.append("launcher dry-run must fail for unknown/unmanifested agent name")

if errors:
    print("FAIL")
    for error in errors:
        print("-", error)
    sys.exit(1)

print(
    f"PASS: {len(manifest)} Platform agents exactly synchronized "
    f"({mode_counts.get('read', 0)} read / {mode_counts.get('write', 0)} write) "
    f"[CONFIGURATION / PROJECTION VALID]."
)

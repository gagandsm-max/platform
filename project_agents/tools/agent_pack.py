"""Deterministic Platform agent-pack generation and validation.

This module does not author JSON schemas. Schemas live under
project_agents/schemas/ and are checked against the state machine by
validate_agents.py / control_plane.py.
"""

from __future__ import annotations

import hashlib
import json
import re
import sys
from collections import Counter
from pathlib import Path
from typing import Any

from control_plane import ControlPlaneError, assert_schema_state_enums_match_machine

AGENT_NAME_RE = re.compile(r"^[a-z][a-z0-9-]*$")
SKILL_NAME_RE = re.compile(r"^[a-z][a-z0-9_]*$")
CURSOR_MARK_RE = re.compile(
    r"<!-- generated from project_agents/canonical/[a-z0-9_./-]+ sha256:[0-9a-f]{64} -->"
)
CODEX_MARK_RE = re.compile(
    r"Generated from project_agents/canonical/[a-z0-9_./-]+ sha256:[0-9a-f]{64}"
)
SKILL_MARK_RE = re.compile(
    r"<!-- generated from project_agents/skills/[a-z0-9_]+/SKILL\.md sha256:[0-9a-f]{64} -->"
)
TMP_SUFFIX = ".platform-sync.tmp"
ALLOWED_FRONTMATTER_KEYS = frozenset({"name", "description"})
POLICY_PERMIT_RE = re.compile(
    r"(?i)\b(you may|you can|allowed to|should force-push|skip tests to|disable tls validation|"
    r"insecureskipverify\s*=\s*true|docker system prune|compose down -v|"
    r"llm (may|can|should) (directly )?(execute|perform) privileged|"
    r"writer (may|can) self-certify|claim (it|the work) is (production|ga)-ready)\b"
)
SECRET_RE = re.compile(
    r"BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY|AKIA[0-9A-Z]{16}|xox[baprs]-[0-9A-Za-z-]{10,}"
)
MATRIX_PATH = "skills/agent_skill_matrix.json"
ADAPTER_ROOTS = (
    (".cursor", "agents"),
    (".agents", "agents"),
    (".codex", "agents"),
)


class AgentPackError(ValueError):
    """Deterministic agent-pack failure."""


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def load_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def canonical_path(repo: Path, entry: dict[str, Any]) -> Path:
    return repo / "project_agents" / "canonical" / str(entry["tier"]).lower() / f"{entry['name']}.md"


def source_ref(repo: Path, path: Path) -> str:
    return path.relative_to(repo / "project_agents").as_posix()


def assert_safe_name(name: str, pattern: re.Pattern[str], label: str) -> str:
    if not pattern.match(name) or ".." in name or "/" in name or "\\" in name:
        raise AgentPackError(f"refusing unsafe {label} name {name!r}")
    return name


def owned_path(root: Path, name: str, pattern: re.Pattern[str], label: str) -> Path:
    assert_safe_name(name, pattern, label)
    root_resolved = root.resolve()
    candidate = (root / name).resolve()
    if candidate == root_resolved or root_resolved not in candidate.parents:
        raise AgentPackError(f"path traversal blocked for {label} {name!r}")
    return root / name


def refuse_agents_md(path: Path, repo: Path) -> None:
    forbidden = {
        (repo / "AGENTS.md").resolve(),
        (repo / "project_documentation" / "AGENTS.md").resolve(),
    }
    try:
        resolved = path.resolve()
    except OSError as exc:
        raise AgentPackError(f"cannot resolve path {path}: {exc}") from exc
    if resolved in forbidden or path.name == "AGENTS.md" and path.parent.resolve() == repo.resolve():
        raise AgentPackError("refusing to touch root AGENTS.md")


def atomic_write(path: Path, content: str, repo: Path) -> None:
    refuse_agents_md(path, repo)
    if path.is_symlink():
        raise AgentPackError(f"refusing to overwrite symlink {path}")
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(path.name + TMP_SUFFIX)
    refuse_agents_md(tmp, repo)
    tmp.write_text(content, encoding="utf-8")
    tmp.replace(path)


def safe_unlink(path: Path, owned_root: Path, repo: Path) -> None:
    refuse_agents_md(path, repo)
    if path.is_symlink():
        return
    owned = owned_root.resolve()
    resolved = path.resolve()
    if resolved != owned and owned not in resolved.parents:
        raise AgentPackError(f"refusing to delete outside owned root: {path}")
    if resolved.is_file():
        resolved.unlink()


def render_cursor(entry: dict[str, Any], body: str, sha: str, canonical: Path, repo: Path) -> str:
    return (
        f"---\nname: {entry['name']}\n"
        f"description: {entry['description']}\n---\n\n"
        f"<!-- generated from project_agents/{source_ref(repo, canonical)} sha256:{sha} -->\n\n{body}"
    )


def render_antigravity(entry: dict[str, Any], body: str, sha: str, canonical: Path, repo: Path) -> str:
    tools = ["view_file", "grep_search", "run_command"]
    if entry["mode"] == "write":
        tools.append("replace_file_content")
    tool_yaml = "\n".join(f"  - {tool}" for tool in tools)
    return (
        f"---\nname: {entry['name']}\n"
        f"description: {entry['description']}\n"
        f"tools:\n{tool_yaml}\n"
        "mainAgent: false\nsubagent: true\nmodel: inherit\ncommandExecutionPolicy: sandbox\n---\n\n"
        f"<!-- generated from project_agents/{source_ref(repo, canonical)} sha256:{sha} -->\n\n{body}"
    )


def render_codex(entry: dict[str, Any], body: str, sha: str, canonical: Path, repo: Path) -> str:
    escaped_body = body.replace('"""', '\\"\\"\\"')
    sandbox = "read-only" if entry["mode"] == "read" else "workspace-write"
    return (
        f"name = {json.dumps(entry['name'])}\n"
        f"description = {json.dumps(entry['description'])}\n"
        f"sandbox_mode = {json.dumps(sandbox)}\n"
        'developer_instructions = """\n'
        f"Generated from project_agents/{source_ref(repo, canonical)} sha256:{sha}\n\n"
        f"{escaped_body}\n"
        '"""\n'
    )


def render_generated_skill(name: str, body: str) -> str:
    """Project a canonical skill into generated mirrors.

    Codex skill discovery requires YAML frontmatter at byte 0 of SKILL.md.
    Provenance is therefore inserted immediately after the closing frontmatter
    delimiter, never before it.
    """
    sha = sha256_text(body)
    marker = f"<!-- generated from project_agents/skills/{name}/SKILL.md sha256:{sha} -->"
    if not body.startswith("---\n"):
        raise AgentPackError(f"canonical skill {name} missing YAML frontmatter at byte 0")
    end = body.find("\n---\n", 4)
    if end < 0:
        raise AgentPackError(f"canonical skill {name} frontmatter is not terminated")
    front = body[: end + 5]
    rest = body[end + 5 :]
    return f"{front}\n{marker}\n{rest}"


render_root_skill = render_generated_skill


def parse_frontmatter(text: str, label: str) -> tuple[dict[str, str], str]:
    if text.startswith("\ufeff"):
        raise AgentPackError(f"{label} YAML frontmatter must start at byte 0 (BOM present)")
    if not text.startswith("---\n"):
        raise AgentPackError(f"{label} missing YAML frontmatter")
    end = text.find("\n---\n", 4)
    if end < 0:
        raise AgentPackError(f"{label} frontmatter is not terminated")
    raw = text[4:end]
    data: dict[str, str] = {}
    for line in raw.splitlines():
        if not line.strip() or line.strip().startswith("#"):
            continue
        if ":" not in line:
            raise AgentPackError(f"{label} invalid frontmatter line: {line!r}")
        key, value = line.split(":", 1)
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        if key not in ALLOWED_FRONTMATTER_KEYS:
            raise AgentPackError(f"{label} unsupported frontmatter key {key!r}")
        data[key] = value
    missing = sorted(ALLOWED_FRONTMATTER_KEYS - set(data))
    if missing:
        raise AgentPackError(f"{label} frontmatter missing {missing}")
    body = text[end + 5 :]
    return data, body


def validate_generated_skill_mirror(
    rel: str,
    name: str,
    text: str,
    canonical_meta: dict[str, str],
) -> list[str]:
    """Independent Codex-compatible checks for a generated skill projection.

    Must not rely solely on equality with render_generated_skill(); a broken
    renderer could otherwise certify its own wrong output.
    """
    errors: list[str] = []
    if text.startswith("\ufeff") or not text.startswith("---\n"):
        errors.append(f"generated skill must begin with YAML frontmatter at byte 0: {rel}")
        return errors
    try:
        meta, _rest = parse_frontmatter(text, rel)
    except AgentPackError as exc:
        errors.append(str(exc))
        return errors
    if meta.get("name") != name:
        errors.append(f"generated skill name mismatch for {rel}: {meta.get('name')!r} != {name!r}")
    if meta.get("description") != canonical_meta.get("description"):
        errors.append(f"generated skill description drift: {rel}")
    if meta.get("name") != canonical_meta.get("name"):
        errors.append(f"generated skill name drift vs canonical: {rel}")
    end = text.find("\n---\n", 4)
    after = text[end + 5 :] if end >= 0 else ""
    if not SKILL_MARK_RE.search(text):
        errors.append(f"generated skill missing provenance marker: {rel}")
    elif not SKILL_MARK_RE.search(after):
        errors.append(f"generated skill provenance must follow closing frontmatter: {rel}")
    return errors


def policy_scan(path: Path, text: str) -> list[str]:
    errors: list[str] = []
    rel = path.as_posix()
    if SECRET_RE.search(text):
        errors.append(f"secret-like material in {rel}")
    for line_no, line in enumerate(text.splitlines(), 1):
        lowered = line.lower()
        if any(
            token in lowered
            for token in ("do not", "don't", "never ", "must not", "cannot", "forbidden", "prohibited")
        ):
            continue
        if POLICY_PERMIT_RE.search(line):
            errors.append(f"policy violation {rel}:{line_no}: {line.strip()}")
    return errors


def has_generated_adapter_marker(path: Path) -> bool:
    if not path.is_file() or path.is_symlink():
        return False
    text = path.read_text(encoding="utf-8")
    if path.suffix == ".toml":
        return bool(CODEX_MARK_RE.search(text))
    return bool(CURSOR_MARK_RE.search(text))


def has_generated_skill_marker(path: Path) -> bool:
    if not path.is_file() or path.is_symlink():
        return False
    return bool(SKILL_MARK_RE.search(path.read_text(encoding="utf-8")))


def load_agent_manifest(repo: Path) -> list[dict[str, Any]]:
    path = repo / "project_agents" / "manifest.json"
    data = load_json(path)
    if not isinstance(data, list):
        raise AgentPackError("project_agents/manifest.json must be an array")
    return data


def load_skill_manifest(repo: Path) -> list[dict[str, Any]]:
    path = repo / "project_agents" / "skills" / "manifest.json"
    data = load_json(path)
    if not isinstance(data, list):
        raise AgentPackError("project_agents/skills/manifest.json must be an array")
    return data


def expected_adapter_map(repo: Path, entry: dict[str, Any], body: str, sha: str, canonical: Path) -> dict[Path, str]:
    name = entry["name"]
    return {
        repo / ".cursor" / "agents" / f"{name}.md": render_cursor(entry, body, sha, canonical, repo),
        repo / ".agents" / "agents" / f"{name}.md": render_antigravity(entry, body, sha, canonical, repo),
        repo / ".codex" / "agents" / f"{name}.toml": render_codex(entry, body, sha, canonical, repo),
    }


def clean_tmp_files(root: Path, repo: Path) -> None:
    if not root.exists():
        return
    for path in root.rglob(f"*{TMP_SUFFIX}"):
        if path.is_symlink():
            continue
        refuse_agents_md(path, repo)
        if path.is_file():
            path.unlink()


def prune_retired_adapters(directory: Path, expected_names: set[str], repo: Path) -> list[str]:
    skipped: list[str] = []
    if not directory.exists():
        return skipped
    for path in list(directory.iterdir()):
        if path.name.endswith(TMP_SUFFIX):
            continue
        if path.name in expected_names:
            continue
        rel = path.relative_to(repo).as_posix()
        if path.is_symlink():
            skipped.append(f"unowned adapter requires review (symlink not deleted): {rel}")
            continue
        if path.is_file() and has_generated_adapter_marker(path):
            safe_unlink(path, directory, repo)
            continue
        skipped.append(f"unowned adapter requires review (not deleted): {rel}")
    return skipped


def prune_retired_skill_mirrors(generated_root: Path, declared: set[str], repo: Path) -> list[str]:
    skipped: list[str] = []
    if not generated_root.exists():
        return skipped
    for directory in list(generated_root.iterdir()):
        if not directory.is_dir() and not directory.is_symlink():
            continue
        rel = directory.relative_to(repo).as_posix()
        if directory.name in declared:
            continue
        if directory.is_symlink():
            skipped.append(f"unowned root skill requires review (symlink not deleted): {rel}")
            continue
        skill_md = directory / "SKILL.md"
        if skill_md.is_symlink() or directory.is_symlink():
            skipped.append(f"unowned root skill requires review (symlink not deleted): {rel}")
            continue
        if skill_md.is_file() and has_generated_skill_marker(skill_md):
            extras = [item for item in directory.iterdir() if item.name != "SKILL.md" and not item.name.endswith(TMP_SUFFIX)]
            if extras:
                skipped.append(
                    f"generated root skill leftover has extra files; not recursively deleted: {rel}"
                )
                continue
            safe_unlink(skill_md, generated_root, repo)
            try:
                directory.rmdir()
            except OSError:
                skipped.append(f"generated root skill directory not empty after SKILL.md prune: {rel}")
            continue
        skipped.append(f"unowned root skill requires review (not deleted): {rel}")
    return skipped


def sync_agent_pack(repo: Path) -> str:
    pa = repo / "project_agents"
    manifest_path = pa / "manifest.json"
    manifest = load_agent_manifest(repo)
    for entry in manifest:
        name = assert_safe_name(str(entry.get("name", "")), AGENT_NAME_RE, "agent")
        if entry.get("mode") not in {"read", "write"}:
            raise AgentPackError(f"manifest entry {name} has invalid mode {entry.get('mode')}")
        canonical = canonical_path(repo, entry)
        if not canonical.is_file() or canonical.is_symlink():
            raise AgentPackError(f"missing canonical agent: {canonical.relative_to(repo)}")
        body = canonical.read_text(encoding="utf-8")
        sha = sha256_text(body)
        entry["sha256"] = sha
        for path, content in expected_adapter_map(repo, entry, body, sha, canonical).items():
            owned_path(path.parent, path.name, re.compile(r"^[a-z][a-z0-9.-]*$"), "adapter")
            atomic_write(path, content, repo)

    skill_manifest = load_skill_manifest(repo)
    declared_names = [str(entry["name"]) for entry in skill_manifest]
    if len(declared_names) != len(set(declared_names)):
        raise AgentPackError("duplicate skill names in project_agents/skills/manifest.json")
    generated_roots = [repo / "skills", repo / ".agents" / "skills"]
    for generated_root in generated_roots:
        generated_root.mkdir(parents=True, exist_ok=True)
    for skill in skill_manifest:
        name = assert_safe_name(str(skill.get("name", "")), SKILL_NAME_RE, "skill")
        canonical_skill = pa / "skills" / name / "SKILL.md"
        if not canonical_skill.is_file() or canonical_skill.is_symlink():
            raise AgentPackError(f"missing canonical skill: {canonical_skill.relative_to(repo)}")
        rendered = render_generated_skill(name, canonical_skill.read_text(encoding="utf-8"))
        for generated_root in generated_roots:
            destination = owned_path(generated_root, name, SKILL_NAME_RE, "skill") / "SKILL.md"
            atomic_write(destination, rendered, repo)

    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")

    expected_adapter_names = {
        repo / ".cursor" / "agents": {f"{entry['name']}.md" for entry in manifest},
        repo / ".agents" / "agents": {f"{entry['name']}.md" for entry in manifest},
        repo / ".codex" / "agents": {f"{entry['name']}.toml" for entry in manifest},
    }
    review: list[str] = []
    for directory, expected_names in expected_adapter_names.items():
        review.extend(prune_retired_adapters(directory, expected_names, repo))
        clean_tmp_files(directory, repo)
    review.extend(prune_retired_skill_mirrors(generated_roots[0], set(declared_names), repo))
    review.extend(prune_retired_skill_mirrors(generated_roots[1], set(declared_names), repo))
    for generated_root in generated_roots:
        clean_tmp_files(generated_root, repo)
    if review:
        sys.stderr.write(
            "unowned generated-tree paths were preserved for review:\n- "
            + "\n- ".join(review)
            + "\n"
        )
    return (
        f"Synchronized {len(manifest)} manifest-declared Platform agents and "
        f"{len(skill_manifest)} manifest-declared skills."
    )


def validate_agent_pack(repo: Path) -> list[str]:
    errors: list[str] = []
    pa = repo / "project_agents"
    try:
        manifest = load_agent_manifest(repo)
    except (OSError, json.JSONDecodeError, AgentPackError) as exc:
        return [str(exc)]

    names = [entry.get("name") for entry in manifest]
    if not manifest:
        errors.append("manifest must declare at least one agent")
    if len(set(names)) != len(names):
        duplicates = sorted(str(name) for name, count in Counter(names).items() if count > 1)
        errors.append(f"duplicate manifest agent names: {', '.join(duplicates)}")

    for entry in manifest:
        missing_fields = [field for field in ("name", "description", "mode", "tier", "sha256") if not entry.get(field)]
        if missing_fields:
            errors.append(f"manifest entry {entry.get('name', '<unnamed>')} missing fields: {', '.join(missing_fields)}")
        if entry.get("mode") not in {"read", "write"}:
            errors.append(f"manifest entry {entry.get('name', '<unnamed>')} has invalid mode {entry.get('mode')}")
        name = str(entry.get("name", ""))
        if name and not AGENT_NAME_RE.match(name):
            errors.append(f"unsafe agent name {name!r}")

    expected_canonical = {
        canonical_path(repo, entry).relative_to(pa).as_posix()
        for entry in manifest
        if entry.get("name") and entry.get("tier")
    }
    canonical_root = pa / "canonical"
    actual_canonical = {
        path.relative_to(pa).as_posix()
        for path in canonical_root.rglob("*.md")
        if path.is_file()
    } if canonical_root.exists() else set()
    if actual_canonical != expected_canonical:
        missing = sorted(expected_canonical - actual_canonical)
        extra = sorted(actual_canonical - expected_canonical)
        if missing:
            errors.append(f"missing canonical agents: {', '.join(missing)}")
        if extra:
            errors.append(f"unmanifested canonical agents: {', '.join(extra)}")

    expected_adapter_paths = {
        repo / ".cursor" / "agents": {f"{entry['name']}.md" for entry in manifest if entry.get("name")},
        repo / ".codex" / "agents": {f"{entry['name']}.toml" for entry in manifest if entry.get("name")},
        repo / ".agents" / "agents": {f"{entry['name']}.md" for entry in manifest if entry.get("name")},
    }
    for directory, expected_names in expected_adapter_paths.items():
        actual_names = {path.name for path in directory.iterdir() if path.is_file() or path.is_symlink()} if directory.exists() else set()
        tmp_files = sorted(name for name in actual_names if name.endswith(TMP_SUFFIX))
        if tmp_files:
            errors.append(
                f"interrupted sync residue in {directory.relative_to(repo)}: {', '.join(tmp_files)}"
            )
        actual_names = {name for name in actual_names if not name.endswith(TMP_SUFFIX)}
        missing = sorted(expected_names - actual_names)
        extra = sorted(actual_names - expected_names)
        if missing:
            errors.append(f"missing adapters in {directory.relative_to(repo)}: {', '.join(missing)}")
        for name in extra:
            path = directory / name
            rel = path.relative_to(repo).as_posix()
            if path.is_symlink():
                errors.append(f"unowned adapter requires review (symlink not deleted): {rel}")
            elif has_generated_adapter_marker(path):
                errors.append(f"generated adapter leftover requires sync: {rel}")
            else:
                errors.append(f"unowned adapter requires review (not deleted): {rel}")

    for entry in manifest:
        if not entry.get("name") or not entry.get("tier"):
            continue
        canonical = canonical_path(repo, entry)
        if not canonical.exists():
            continue
        body = canonical.read_text(encoding="utf-8")
        errors.extend(policy_scan(canonical.relative_to(repo), body))
        sha = sha256_text(body)
        if sha != entry.get("sha256"):
            errors.append(f"canonical changed without sync: {entry['name']}")
        expected_files = expected_adapter_map(repo, entry, body, sha, canonical)
        for path, expected_content in expected_files.items():
            if not path.exists():
                errors.append(f"adapter missing: {path.relative_to(repo)}")
            elif path.is_symlink():
                errors.append(f"adapter is a symlink and was not followed: {path.relative_to(repo)}")
            elif path.read_text(encoding="utf-8") != expected_content:
                errors.append(f"adapter drift: {path.relative_to(repo)}")

    skill_manifest_path = pa / "skills" / "manifest.json"
    skill_manifest: list[dict[str, Any]] = []
    if not skill_manifest_path.is_file():
        errors.append("missing project_agents/skills/manifest.json")
    else:
        try:
            skill_manifest = load_skill_manifest(repo)
        except (OSError, json.JSONDecodeError, AgentPackError) as exc:
            errors.append(str(exc))
            skill_manifest = []
        skill_names = [entry.get("name") for entry in skill_manifest]
        if not skill_manifest:
            errors.append("skill manifest must declare at least one skill")
        if any(not name for name in skill_names):
            errors.append("skill manifest contains unnamed skill")
        if len(set(skill_names)) != len(skill_names):
            duplicates = sorted(str(name) for name, count in Counter(skill_names).items() if count > 1)
            errors.append(f"duplicate skill names: {', '.join(duplicates)}")
        for name in skill_names:
            if name and not SKILL_NAME_RE.match(str(name)):
                errors.append(f"unsafe skill name {name!r}")

        declared = {str(name) for name in skill_names if name}
        canonical_dirs = {path.name for path in (pa / "skills").iterdir() if path.is_dir()} if (pa / "skills").exists() else set()
        if canonical_dirs != declared:
            missing = sorted(declared - canonical_dirs)
            extra = sorted(canonical_dirs - declared)
            if missing:
                errors.append(f"missing canonical skill directories: {', '.join(missing)}")
            if extra:
                errors.append(f"unmanifested canonical skill directories: {', '.join(extra)}")

        for name in sorted(declared):
            canonical_skill = pa / "skills" / name / "SKILL.md"
            if not canonical_skill.is_file():
                errors.append(f"canonical skill missing SKILL.md: project_agents/skills/{name}")
                continue
            body = canonical_skill.read_text(encoding="utf-8")
            errors.extend(policy_scan(canonical_skill.relative_to(repo), body))
            try:
                meta, _rest = parse_frontmatter(body, f"project_agents/skills/{name}/SKILL.md")
            except AgentPackError as exc:
                errors.append(str(exc))
                continue
            if meta.get("name") != name:
                errors.append(
                    f"folder/name mismatch: directory {name} frontmatter name {meta.get('name')!r}"
                )

        for generated_root, label in (
            (repo / "skills", "legacy skills/"),
            (repo / ".agents" / "skills", "native .agents/skills/"),
        ):
            generated_dirs = {path.name for path in generated_root.iterdir() if path.is_dir() or path.is_symlink()} if generated_root.exists() else set()
            tmp_residue = []
            if generated_root.exists():
                tmp_residue = sorted(path.relative_to(repo).as_posix() for path in generated_root.rglob(f"*{TMP_SUFFIX}"))
            if tmp_residue:
                errors.append(f"interrupted sync residue in {label}: " + ", ".join(tmp_residue))
            missing = sorted(declared - generated_dirs)
            extra = sorted(generated_dirs - declared)
            if missing:
                errors.append(f"missing generated {label} directories: {', '.join(missing)}")
            for extra_name in extra:
                directory = generated_root / extra_name
                rel = directory.relative_to(repo).as_posix()
                skill_md = directory / "SKILL.md"
                if directory.is_symlink() or skill_md.is_symlink():
                    errors.append(f"unowned {label} requires review (symlink not deleted): {rel}")
                elif skill_md.is_file() and has_generated_skill_marker(skill_md):
                    errors.append(f"generated {label} leftover requires sync: {rel}")
                else:
                    errors.append(f"unowned {label} requires review (not deleted): {rel}")
            for name in sorted(declared):
                canonical_skill = pa / "skills" / name / "SKILL.md"
                generated_skill = generated_root / name / "SKILL.md"
                if not canonical_skill.is_file():
                    continue
                rel = generated_skill.relative_to(repo).as_posix()
                if generated_skill.is_symlink():
                    errors.append(f"generated skill is a symlink and was not followed: {rel}")
                    continue
                if not generated_skill.is_file():
                    errors.append(f"generated skill missing SKILL.md: {rel}")
                    continue
                canonical_text = canonical_skill.read_text(encoding="utf-8")
                try:
                    canonical_meta, _canonical_rest = parse_frontmatter(
                        canonical_text, f"project_agents/skills/{name}/SKILL.md"
                    )
                except AgentPackError as exc:
                    errors.append(str(exc))
                    continue
                generated_text = generated_skill.read_text(encoding="utf-8")
                errors.extend(
                    validate_generated_skill_mirror(rel, name, generated_text, canonical_meta)
                )
                expected = render_generated_skill(name, canonical_text)
                if generated_text != expected:
                    errors.append(f"generated skill drift: {rel}")

        matrix_path = pa / "skills" / "agent_skill_matrix.json"
        if matrix_path.is_file():
            try:
                matrix = load_json(matrix_path)
            except (OSError, json.JSONDecodeError) as exc:
                errors.append(f"agent skill matrix is not valid JSON: {exc}")
                matrix = {}
            if not isinstance(matrix, dict):
                errors.append("agent skill matrix must be an object")
            else:
                agent_names = {str(entry.get("name")) for entry in manifest if entry.get("name")}
                if set(matrix) != agent_names:
                    missing = sorted(agent_names - set(matrix))
                    extra = sorted(set(matrix) - agent_names)
                    if missing:
                        errors.append(f"agent skill matrix missing agents: {', '.join(missing)}")
                    if extra:
                        errors.append(f"agent skill matrix has unknown agents: {', '.join(extra)}")
                required_sizes: list[int] = []
                for agent_name, spec in matrix.items():
                    if not isinstance(spec, dict):
                        errors.append(f"agent skill matrix {agent_name} must be an object")
                        continue
                    required = spec.get("requiredSkills") or []
                    conditional = spec.get("conditionalSkills") or []
                    if not required:
                        errors.append(f"{agent_name} has no required Skills")
                    required_sizes.append(len(required))
                    for skill_name in list(required) + list(conditional):
                        if skill_name not in declared:
                            errors.append(f"{agent_name} references unknown skill {skill_name}")
                if declared and required_sizes and all(set(matrix[name]["requiredSkills"]) == declared for name in matrix if isinstance(matrix.get(name), dict)):
                    errors.append("every agent requires every skill; routing is not bounded")
                if declared and required_sizes and max(required_sizes) >= len(declared):
                    errors.append("an agent requires the entire skill inventory")

    agents_md = repo / "AGENTS.md"
    if agents_md.exists() or agents_md.is_symlink():
        if not agents_md.is_symlink():
            errors.append("root AGENTS.md must remain a symlink")

    machine_path = pa / "control_plane_state_machine.json"
    task_schema_path = pa / "schemas" / "task_manifest.schema.json"
    handoff_schema_path = pa / "schemas" / "handoff.schema.json"
    if machine_path.is_file() and task_schema_path.is_file() and handoff_schema_path.is_file():
        try:
            assert_schema_state_enums_match_machine(
                load_json(machine_path),
                load_json(task_schema_path),
                load_json(handoff_schema_path),
            )
        except ControlPlaneError as exc:
            errors.append(str(exc))
        except OSError as exc:
            errors.append(f"control-plane schema/state-machine check failed: {exc}")

    return errors

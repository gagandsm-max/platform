"""Canonical Skill resource projection (references/fixtures/evals).

Tranche-1 Capability Pack commercial-depth: project non-SKILL.md resources into
generated Cursor/Antigravity mirrors with exact-byte fidelity and hard safety
bounds. No network access. No execution. scripts/ is unsupported.
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import Iterable

ALLOWED_RESOURCE_DIRS = frozenset({"references", "fixtures", "evals"})
UNSUPPORTED_RESOURCE_DIRS = frozenset({"scripts"})

# Hard bounds (bytes / counts). Tuned for engineering guidance packs, not blobs.
MAX_RESOURCE_FILES_PER_SKILL = 64
MAX_RESOURCE_FILE_BYTES = 256 * 1024
MAX_RESOURCE_AGGREGATE_BYTES = 2 * 1024 * 1024
MAX_RELATIVE_PATH_LEN = 180


class SkillResourceError(ValueError):
    """Deterministic skill-resource failure."""


def _is_safe_relative_parts(parts: tuple[str, ...]) -> bool:
    if not parts:
        return False
    if parts[0] not in ALLOWED_RESOURCE_DIRS:
        return False
    for part in parts:
        if not part or part in {".", ".."}:
            return False
        if any(bad in part for bad in ("/", "\\", "\0")):
            return False
        if part.startswith(".") and part not in {".gitkeep"}:
            # Allow normal filenames; reject hidden/. and .. already handled.
            if part.startswith(".."):
                return False
    return True


def assert_safe_resource_rel(rel: str) -> Path:
    """Validate a repo-relative resource path under a skill package."""
    if not rel or rel.startswith("/") or rel.startswith("\\") or "\\" in rel or "\0" in rel:
        raise SkillResourceError(f"unsafe resource path: {rel!r}")
    if len(rel) > MAX_RELATIVE_PATH_LEN:
        raise SkillResourceError(f"resource path too long: {rel!r}")
    path = Path(rel)
    if path.is_absolute() or ".." in path.parts:
        raise SkillResourceError(f"unsafe resource path: {rel!r}")
    parts = path.parts
    if not _is_safe_relative_parts(parts):
        raise SkillResourceError(f"unsafe or unsupported resource path: {rel!r}")
    return path


def discover_canonical_resources(skill_dir: Path) -> list[Path]:
    """Return sorted relative Paths of projectable resources under skill_dir."""
    if not skill_dir.is_dir() or skill_dir.is_symlink():
        raise SkillResourceError(f"skill directory missing or symlink: {skill_dir}")

    for unsupported in sorted(UNSUPPORTED_RESOURCE_DIRS):
        candidate = skill_dir / unsupported
        if candidate.exists():
            raise SkillResourceError(
                f"unsupported resource directory in Tranche 1: "
                f"{skill_dir.name}/{unsupported}/"
            )

    found: list[Path] = []
    aggregate = 0
    for dirname in sorted(ALLOWED_RESOURCE_DIRS):
        root = skill_dir / dirname
        if not root.exists():
            continue
        if root.is_symlink():
            raise SkillResourceError(f"symlink resource root rejected: {skill_dir.name}/{dirname}")
        if not root.is_dir():
            raise SkillResourceError(f"resource root must be a directory: {skill_dir.name}/{dirname}")
        for path in sorted(root.rglob("*")):
            rel = path.relative_to(skill_dir)
            rel_s = rel.as_posix()
            if path.is_dir() and not path.is_symlink():
                continue
            if path.is_symlink() or path.is_dir():
                raise SkillResourceError(f"symlink resource rejected: {skill_dir.name}/{rel_s}")
            assert_safe_resource_rel(rel_s)
            if not path.is_file():
                raise SkillResourceError(f"non-file resource rejected: {skill_dir.name}/{rel_s}")
            size = path.stat().st_size
            if size > MAX_RESOURCE_FILE_BYTES:
                raise SkillResourceError(
                    f"resource exceeds per-file bound ({MAX_RESOURCE_FILE_BYTES} bytes): "
                    f"{skill_dir.name}/{rel_s} size={size}"
                )
            aggregate += size
            if aggregate > MAX_RESOURCE_AGGREGATE_BYTES:
                raise SkillResourceError(
                    f"resource aggregate exceeds bound ({MAX_RESOURCE_AGGREGATE_BYTES} bytes): "
                    f"{skill_dir.name}"
                )
            found.append(rel)

    if len(found) > MAX_RESOURCE_FILES_PER_SKILL:
        raise SkillResourceError(
            f"too many resource files for {skill_dir.name}: {len(found)} > "
            f"{MAX_RESOURCE_FILES_PER_SKILL}"
        )
    return found


def atomic_write_bytes(path: Path, data: bytes, repo: Path) -> None:
    """Write bytes atomically inside repo; refuse symlinks and AGENTS.md."""
    # Local import avoids circular dependency at module import time in tests.
    from agent_pack import AgentPackError, TMP_SUFFIX, refuse_agents_md

    refuse_agents_md(path, repo)
    if path.is_symlink():
        raise AgentPackError(f"refusing to overwrite symlink {path}")
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(path.name + TMP_SUFFIX)
    refuse_agents_md(tmp, repo)
    tmp.write_bytes(data)
    tmp.replace(path)


def expected_resource_map(skill_dir: Path) -> dict[str, bytes]:
    """Map relative posix path -> exact canonical bytes."""
    mapping: dict[str, bytes] = {}
    for rel in discover_canonical_resources(skill_dir):
        mapping[rel.as_posix()] = (skill_dir / rel).read_bytes()
    return mapping


def sync_skill_resources(
    skill_name: str,
    canonical_dir: Path,
    generated_roots: Iterable[Path],
    repo: Path,
    *,
    skill_name_re,
) -> list[str]:
    """Project skill resources into each generated root. Returns review notes."""
    from agent_pack import AgentPackError, TMP_SUFFIX, owned_path, safe_unlink

    review: list[str] = []
    try:
        expected = expected_resource_map(canonical_dir)
    except SkillResourceError as exc:
        raise AgentPackError(str(exc)) from exc

    for generated_root in generated_roots:
        skill_out = owned_path(generated_root, skill_name, skill_name_re, "skill")
        skill_out.mkdir(parents=True, exist_ok=True)

        # Write / refresh expected resources.
        for rel, data in expected.items():
            dest = skill_out / rel
            # Ensure parent path stays inside skill_out.
            dest_resolved_parent = dest.parent.resolve()
            skill_resolved = skill_out.resolve()
            if skill_resolved not in dest_resolved_parent.parents and dest_resolved_parent != skill_resolved:
                raise AgentPackError(f"path traversal blocked for resource {skill_name}/{rel}")
            if dest.is_symlink():
                raise AgentPackError(f"refusing to overwrite symlink resource {dest}")
            atomic_write_bytes(dest, data, repo)

        # Prune stale projected resources under allowed dirs only.
        for dirname in sorted(ALLOWED_RESOURCE_DIRS):
            root = skill_out / dirname
            if not root.exists():
                continue
            if root.is_symlink():
                review.append(
                    f"unowned generated skill resource requires review (symlink not deleted): "
                    f"{root.relative_to(repo).as_posix()}"
                )
                continue
            for path in sorted(root.rglob("*"), reverse=True):
                rel = path.relative_to(skill_out).as_posix()
                if path.name.endswith(TMP_SUFFIX):
                    continue
                if path.is_dir() and not path.is_symlink():
                    # Remove empty dirs after file prune.
                    try:
                        if not any(path.iterdir()):
                            path.rmdir()
                    except OSError:
                        pass
                    continue
                if rel in expected:
                    continue
                if path.is_symlink():
                    review.append(
                        f"unowned generated skill resource requires review (symlink not deleted): "
                        f"{path.relative_to(repo).as_posix()}"
                    )
                    continue
                if path.is_file():
                    safe_unlink(path, skill_out, repo)
            # Final empty-dir sweep.
            if root.is_dir() and not root.is_symlink():
                for dirpath, dirnames, filenames in os.walk(root, topdown=False):
                    if filenames:
                        continue
                    if dirnames:
                        continue
                    try:
                        Path(dirpath).rmdir()
                    except OSError:
                        pass
                if root.exists() and root.is_dir() and not any(root.iterdir()):
                    try:
                        root.rmdir()
                    except OSError:
                        pass

        # Unsupported scripts/ in generated tree is drift.
        scripts = skill_out / "scripts"
        if scripts.exists():
            review.append(
                f"unsupported generated scripts/ requires review: "
                f"{scripts.relative_to(repo).as_posix()}"
            )

    return review


def validate_skill_resources(
    skill_name: str,
    canonical_dir: Path,
    generated_roots: Iterable[Path],
    repo: Path,
) -> list[str]:
    """Validate resource discovery bounds and exact generated mirrors."""
    errors: list[str] = []
    try:
        expected = expected_resource_map(canonical_dir)
    except SkillResourceError as exc:
        return [f"{skill_name}: {exc}"]

    for generated_root in generated_roots:
        skill_out = generated_root / skill_name
        try:
            label = generated_root.relative_to(repo).as_posix()
        except ValueError:
            label = str(generated_root)
        if skill_out.is_symlink():
            errors.append(f"generated skill directory is a symlink: {label}/{skill_name}")
            continue
        for rel, data in expected.items():
            dest = skill_out / rel
            rel_label = f"{label}/{skill_name}/{rel}"
            if dest.is_symlink():
                errors.append(f"generated skill resource is a symlink: {rel_label}")
                continue
            if not dest.is_file():
                errors.append(f"generated skill resource missing: {rel_label}")
                continue
            actual = dest.read_bytes()
            if actual != data:
                errors.append(f"generated skill resource drift: {rel_label}")

        # Extra projected resources under allowed dirs must not remain.
        if skill_out.is_dir():
            for dirname in sorted(ALLOWED_RESOURCE_DIRS):
                root = skill_out / dirname
                if not root.exists():
                    continue
                if root.is_symlink():
                    errors.append(f"generated skill resource root is a symlink: {label}/{skill_name}/{dirname}")
                    continue
                for path in root.rglob("*"):
                    if path.is_dir() and not path.is_symlink():
                        continue
                    rel = path.relative_to(skill_out).as_posix()
                    if rel not in expected:
                        errors.append(f"unexpected generated skill resource: {label}/{skill_name}/{rel}")
            scripts = skill_out / "scripts"
            if scripts.exists():
                errors.append(f"unsupported generated scripts/ present: {label}/{skill_name}/scripts")

    return errors

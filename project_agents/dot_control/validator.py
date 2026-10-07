"""Manifest and path validation for the Dot Host-Control Bridge."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any, Mapping, Optional, Sequence

from .models import (
    ALLOWED_MANIFEST_KEYS,
    APPROVED_CONTROL_ROOT,
    APPROVED_REPO_EXACT,
    APPROVED_REPO_PREFIX,
    APPROVED_REPOSITORY,
    Action,
    BridgeError,
    DEFAULT_CONTROL_ROOT,
    DEFAULT_HOME,
    FORBIDDEN_MANIFEST_KEYS,
    MAX_COMMIT_MESSAGE_LEN,
    MAX_PROMPT_BYTES,
    MAX_TIMEOUT_SECONDS,
    SCHEMA_VERSION,
    SHA_HEX_LEN,
    TEST_PROFILES,
    TaskManifest,
    VerifierTool,
    WriterTool,
)

_SHA_RE = re.compile(r"^[0-9a-f]{40}$")
_TASK_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
_ISO_LIKE_RE = re.compile(r"^\d{4}-\d{2}-\d{2}T")


class PathPolicy:
    """Approved-root and symlink-escape policy (injectable for tests)."""

    def __init__(
        self,
        *,
        home: Path = DEFAULT_HOME,
        control_root: Path = DEFAULT_CONTROL_ROOT,
        approved_repo_exact: Path = APPROVED_REPO_EXACT,
        approved_repo_prefix: str = APPROVED_REPO_PREFIX,
        allow_test_roots: Optional[Sequence[Path]] = None,
    ):
        self.home = Path(home)
        self.control_root = Path(control_root)
        self.approved_repo_exact = Path(approved_repo_exact)
        self.approved_repo_prefix = approved_repo_prefix
        self.allow_test_roots = tuple(Path(p) for p in (allow_test_roots or ()))

    def reject_symlink_root(self, path: Path, label: str) -> Path:
        if path.exists() and path.is_symlink():
            raise BridgeError(f"{label} must not be a symlink: {path}")
        resolved = path.resolve(strict=False)
        return resolved

    def is_approved_worktree(self, path: Path) -> bool:
        resolved = path.resolve(strict=False)
        text = str(resolved)
        exact = str(self.approved_repo_exact.resolve(strict=False))
        if text == exact or text.startswith(exact + "/"):
            return True
        if text.startswith(self.approved_repo_prefix):
            return True
        for root in self.allow_test_roots:
            root_r = str(root.resolve(strict=False))
            if text == root_r or text.startswith(root_r + "/"):
                return True
        return False

    def is_approved_control_path(self, path: Path) -> bool:
        resolved = path.resolve(strict=False)
        text = str(resolved)
        ctrl = str(self.control_root.resolve(strict=False))
        if text == ctrl or text.startswith(ctrl + "/"):
            return True
        # Production absolute control root also accepted when policy uses defaults.
        prod = str(APPROVED_CONTROL_ROOT)
        if text == prod or text.startswith(prod + "/"):
            return True
        return False

    def assert_under(self, path: Path, root: Path, label: str) -> Path:
        resolved = path.resolve(strict=False)
        root_r = root.resolve(strict=False)
        try:
            resolved.relative_to(root_r)
        except ValueError as exc:
            raise BridgeError(f"{label} escapes approved root {root_r}: {path}") from exc
        return resolved

    def assert_no_forbidden_home(self, path: Path) -> None:
        resolved = path.resolve(strict=False)
        text = str(resolved)
        blocked_prefixes = (
            str(self.home / ".ssh"),
            str(self.home / ".gnupg"),
            str(self.home / ".password-store"),
            str(self.home / "Documents"),
            str(self.home / ".mozilla"),
            str(self.home / ".config" / "google-chrome"),
            str(self.home / ".config" / "chromium"),
        )
        for prefix in blocked_prefixes:
            if text == prefix or text.startswith(prefix + "/"):
                raise BridgeError(f"forbidden home path: {path}")


def _require(data: Mapping[str, Any], key: str) -> Any:
    if key not in data:
        raise BridgeError(f"missing required field: {key}")
    return data[key]


def _as_bool(value: Any, key: str) -> bool:
    if not isinstance(value, bool):
        raise BridgeError(f"{key} must be a boolean")
    return value


def _as_str(value: Any, key: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise BridgeError(f"{key} must be a non-empty string")
    return value


def _as_str_list(value: Any, key: str) -> tuple[str, ...]:
    if not isinstance(value, list) or not all(isinstance(x, str) for x in value):
        raise BridgeError(f"{key} must be an array of strings")
    return tuple(value)


def validate_sha(value: str, key: str) -> str:
    if not _SHA_RE.match(value):
        raise BridgeError(f"{key} must be a 40-char lowercase hex SHA")
    return value


def _reject_duplicate_json_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    """Fail closed on duplicate JSON object keys (json.loads otherwise keeps last)."""
    out: dict[str, Any] = {}
    for key, value in pairs:
        if key in out:
            raise BridgeError(f"duplicate JSON key rejected: {key}")
        out[key] = value
    return out


def load_json_object(path: Path) -> dict[str, Any]:
    try:
        raw = path.read_text(encoding="utf-8")
    except OSError as exc:
        raise BridgeError(f"cannot read manifest: {exc}") from exc
    try:
        data = json.loads(raw, object_pairs_hook=_reject_duplicate_json_keys)
    except BridgeError:
        raise
    except json.JSONDecodeError as exc:
        raise BridgeError(f"invalid JSON: {exc}") from exc
    if not isinstance(data, dict):
        raise BridgeError("manifest must be a JSON object")
    return data


def validate_manifest_keys(data: Mapping[str, Any]) -> None:
    """Reject forbidden command fields and every unknown key (schema additionalProperties:false)."""
    unknown = sorted(k for k in data.keys() if k not in ALLOWED_MANIFEST_KEYS)
    for key in data:
        if key in FORBIDDEN_MANIFEST_KEYS:
            raise BridgeError(f"arbitrary command field rejected: {key}")
    if unknown:
        # Prefer specific forbidden-field messages already raised; remaining are unknown.
        raise BridgeError(f"unknown manifest field(s) rejected: {', '.join(unknown)}")


def validate_manifest_dict(
    data: Mapping[str, Any],
    *,
    policy: PathPolicy,
    require_paths_exist: bool = True,
) -> TaskManifest:
    validate_manifest_keys(data)

    schema_version = _as_str(_require(data, "schemaVersion"), "schemaVersion")
    if schema_version != SCHEMA_VERSION:
        raise BridgeError(f"unsupported schemaVersion: {schema_version}")

    task_id = _as_str(_require(data, "taskId"), "taskId")
    if not _TASK_ID_RE.match(task_id):
        raise BridgeError(f"invalid taskId: {task_id}")

    governing_issue = _as_str(_require(data, "governingIssue"), "governingIssue")

    try:
        action = Action(_as_str(_require(data, "action"), "action"))
    except ValueError as exc:
        raise BridgeError(f"unknown action: {data.get('action')}") from exc

    try:
        writer_tool = WriterTool(_as_str(_require(data, "writerTool"), "writerTool"))
    except ValueError as exc:
        raise BridgeError(f"invalid writerTool: {data.get('writerTool')}") from exc

    try:
        verifier_tool = VerifierTool(_as_str(_require(data, "verifierTool"), "verifierTool"))
    except ValueError as exc:
        raise BridgeError(f"invalid verifierTool: {data.get('verifierTool')}") from exc

    if writer_tool.value == "dot" or verifier_tool.value == "dot":
        raise BridgeError("Dot is not writerTool or verifierTool")

    if (
        writer_tool != WriterTool.NONE
        and verifier_tool != VerifierTool.NONE
        and writer_tool.value == verifier_tool.value
    ):
        raise BridgeError("writerTool must not equal verifierTool")

    repository = _as_str(_require(data, "repository"), "repository")
    if repository != APPROVED_REPOSITORY:
        raise BridgeError(
            f"repository must be {APPROVED_REPOSITORY}; got {repository}"
        )
    worktree_raw = _as_str(_require(data, "worktree"), "worktree")
    if ".." in Path(worktree_raw).parts or worktree_raw.startswith("~"):
        raise BridgeError(f"traversal rejected in worktree: {worktree_raw}")
    worktree = Path(worktree_raw)
    if not worktree.is_absolute():
        raise BridgeError("worktree must be an absolute path")

    policy.assert_no_forbidden_home(worktree)
    if worktree.exists() and worktree.is_symlink():
        raise BridgeError(f"symlink worktree rejected: {worktree}")
    worktree = policy.reject_symlink_root(worktree, "worktree")
    if not policy.is_approved_worktree(worktree):
        raise BridgeError(f"worktree outside approved AmrudAi roots: {worktree}")

    if require_paths_exist and not worktree.is_dir():
        raise BridgeError(f"worktree is not a directory: {worktree}")

    branch = _as_str(_require(data, "branch"), "branch")
    authoritative_base = validate_sha(
        _as_str(_require(data, "authoritativeBase"), "authoritativeBase"),
        "authoritativeBase",
    )
    expected_head = validate_sha(
        _as_str(_require(data, "expectedHead"), "expectedHead"),
        "expectedHead",
    )

    allowed_files = _as_str_list(_require(data, "allowedFiles"), "allowedFiles")
    forbidden_files = _as_str_list(_require(data, "forbiddenFiles"), "forbiddenFiles")

    if "promptFile" not in data:
        raise BridgeError("missing required field: promptFile")
    prompt_raw = data["promptFile"]
    prompt_file: Optional[Path]
    if prompt_raw is None or prompt_raw == "":
        prompt_file = None
    else:
        prompt_file = Path(_as_str(prompt_raw, "promptFile"))
        if not prompt_file.is_absolute():
            raise BridgeError("promptFile must be absolute")
        if ".." in prompt_file.parts:
            raise BridgeError(f"traversal rejected in promptFile: {prompt_file}")
        policy.assert_no_forbidden_home(prompt_file)
        task_prompt_root = policy.control_root / "tasks" / task_id
        policy.assert_under(prompt_file, task_prompt_root, "promptFile")

    timeout_seconds = _require(data, "timeoutSeconds")
    if not isinstance(timeout_seconds, int) or isinstance(timeout_seconds, bool):
        raise BridgeError("timeoutSeconds must be an integer")
    if timeout_seconds < 1 or timeout_seconds > MAX_TIMEOUT_SECONDS:
        raise BridgeError(f"timeoutSeconds out of range 1..{MAX_TIMEOUT_SECONDS}")

    allow_commit = _as_bool(_require(data, "allowCommit"), "allowCommit")
    allow_push = _as_bool(_require(data, "allowPush"), "allowPush")
    allow_pull_request = _as_bool(_require(data, "allowPullRequest"), "allowPullRequest")

    if allow_push:
        raise BridgeError("allowPush must be false in v1")
    if allow_pull_request:
        raise BridgeError("allowPullRequest must be false in v1")

    created_at = _as_str(_require(data, "createdAt"), "createdAt")
    if not _ISO_LIKE_RE.match(created_at):
        raise BridgeError("createdAt must be an ISO-8601 timestamp")
    requested_by = _as_str(_require(data, "requestedBy"), "requestedBy")

    commit_message = data.get("commitMessage")
    if commit_message is not None:
        commit_message = _as_str(commit_message, "commitMessage")
        if len(commit_message) > MAX_COMMIT_MESSAGE_LEN:
            raise BridgeError("commitMessage exceeds bound")
        if "\x00" in commit_message:
            raise BridgeError("commitMessage must not contain NUL")

    test_profile = data.get("testProfile")
    if test_profile is not None:
        test_profile = _as_str(test_profile, "testProfile")

    _validate_action_tools(
        action=action,
        writer_tool=writer_tool,
        verifier_tool=verifier_tool,
        branch=branch,
        allow_commit=allow_commit,
        allowed_files=allowed_files,
        prompt_file=prompt_file,
        commit_message=commit_message,
        test_profile=test_profile,
    )

    if prompt_file is not None and require_paths_exist:
        validate_prompt_file(prompt_file, task_id=task_id, policy=policy)

    return TaskManifest(
        schema_version=schema_version,
        task_id=task_id,
        governing_issue=governing_issue,
        action=action,
        writer_tool=writer_tool,
        verifier_tool=verifier_tool,
        repository=repository,
        worktree=worktree,
        branch=branch,
        authoritative_base=authoritative_base,
        expected_head=expected_head,
        allowed_files=allowed_files,
        forbidden_files=forbidden_files,
        prompt_file=prompt_file,
        timeout_seconds=timeout_seconds,
        allow_commit=allow_commit,
        allow_push=allow_push,
        allow_pull_request=allow_pull_request,
        created_at=created_at,
        requested_by=requested_by,
        commit_message=commit_message,
        test_profile=test_profile,
    )


def _validate_action_tools(
    *,
    action: Action,
    writer_tool: WriterTool,
    verifier_tool: VerifierTool,
    branch: str,
    allow_commit: bool,
    allowed_files: Sequence[str],
    prompt_file: Optional[Path],
    commit_message: Optional[str],
    test_profile: Optional[str],
) -> None:
    protected = {"main", "master", "origin/main"}
    if action in {
        Action.GIT_COMMIT_PROOF,
        Action.CURSOR_TASK,
        Action.CODEX_TASK,
        Action.TEST_PROFILE,
    } and branch in protected:
        raise BridgeError(f"direct writes to protected branch prohibited: {branch}")

    if action == Action.REPO_PROBE:
        if writer_tool != WriterTool.NONE or verifier_tool != VerifierTool.NONE:
            raise BridgeError("REPO_PROBE requires writerTool=none and verifierTool=none")
        return

    if action == Action.GIT_COMMIT_PROOF:
        if writer_tool != WriterTool.NONE:
            raise BridgeError("GIT_COMMIT_PROOF is bridge-owned; writerTool must be none")
        if verifier_tool != VerifierTool.NONE:
            raise BridgeError("GIT_COMMIT_PROOF requires verifierTool=none")
        if not allow_commit:
            raise BridgeError("GIT_COMMIT_PROOF requires allowCommit=true")
        if len(allowed_files) != 1:
            raise BridgeError("GIT_COMMIT_PROOF requires exactly one allowedFiles entry")
        if not commit_message:
            raise BridgeError("GIT_COMMIT_PROOF requires commitMessage")
        return

    if action == Action.CURSOR_TASK:
        if writer_tool != WriterTool.CURSOR_CLI:
            raise BridgeError("CURSOR_TASK requires writerTool=cursor-cli")
        if verifier_tool != VerifierTool.NONE:
            raise BridgeError("CURSOR_TASK requires verifierTool=none")
        if prompt_file is None:
            raise BridgeError("CURSOR_TASK requires promptFile")
        return

    if action == Action.CODEX_TASK:
        if writer_tool != WriterTool.CODEX:
            raise BridgeError("CODEX_TASK requires writerTool=codex")
        if verifier_tool != VerifierTool.NONE:
            raise BridgeError("CODEX_TASK requires verifierTool=none")
        if prompt_file is None:
            raise BridgeError("CODEX_TASK requires promptFile")
        return

    if action == Action.ANTIGRAVITY_AUDIT:
        if writer_tool != WriterTool.NONE:
            raise BridgeError("ANTIGRAVITY_AUDIT requires writerTool=none")
        if verifier_tool != VerifierTool.ANTIGRAVITY:
            raise BridgeError("ANTIGRAVITY_AUDIT requires verifierTool=antigravity")
        if prompt_file is None:
            raise BridgeError("ANTIGRAVITY_AUDIT requires promptFile")
        return

    if action == Action.TEST_PROFILE:
        if writer_tool != WriterTool.NONE or verifier_tool != VerifierTool.NONE:
            raise BridgeError("TEST_PROFILE requires writerTool=none and verifierTool=none")
        if not test_profile:
            raise BridgeError("TEST_PROFILE requires testProfile")
        if test_profile not in TEST_PROFILES:
            raise BridgeError(f"unknown test profile: {test_profile}")
        return

    raise BridgeError(f"unknown action: {action}")


def validate_prompt_file(prompt_file: Path, *, task_id: str, policy: PathPolicy) -> Path:
    if prompt_file.is_symlink():
        raise BridgeError(f"symlink prompt rejected: {prompt_file}")
    if not prompt_file.is_file():
        raise BridgeError(f"promptFile is not a regular file: {prompt_file}")
    task_root = (policy.control_root / "tasks" / task_id).resolve(strict=False)
    resolved = policy.assert_under(prompt_file, task_root, "promptFile")
    size = prompt_file.stat().st_size
    if size > MAX_PROMPT_BYTES:
        raise BridgeError(f"oversized prompt ({size} > {MAX_PROMPT_BYTES})")
    raw = prompt_file.read_bytes()
    if b"\x00" in raw:
        raise BridgeError("promptFile must not contain NUL")
    try:
        raw.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise BridgeError("promptFile must be UTF-8") from exc
    return resolved


def normalize_plan(manifest: TaskManifest) -> dict[str, Any]:
    """Dry-run plan without executing."""
    plan: dict[str, Any] = {
        "taskId": manifest.task_id,
        "action": manifest.action.value,
        "writerTool": manifest.writer_tool.value,
        "verifierTool": manifest.verifier_tool.value,
        "worktree": str(manifest.worktree),
        "branch": manifest.branch,
        "expectedHead": manifest.expected_head,
        "authoritativeBase": manifest.authoritative_base,
        "timeoutSeconds": manifest.timeout_seconds,
        "allowCommit": manifest.allow_commit,
        "allowPush": manifest.allow_push,
        "allowPullRequest": manifest.allow_pull_request,
        "allowedFiles": list(manifest.allowed_files),
        "forbiddenFiles": list(manifest.forbidden_files),
        "promptFile": str(manifest.prompt_file) if manifest.prompt_file else None,
        "testProfile": manifest.test_profile,
        "constructedArgvPreview": preview_argv(manifest),
    }
    return plan


def preview_argv(manifest: TaskManifest) -> list[str]:
    from .executor import build_action_argv

    argv, meta = build_action_argv(manifest)
    if meta.get("unsupported"):
        return [f"UNSUPPORTED:{meta.get('reason', 'unknown')}"]
    return list(argv)

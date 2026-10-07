"""Action execution for the Dot Host-Control Bridge.

All argv arrays are constructed internally from allowlisted action types.
Manifests never supply free-form shell commands.
"""

from __future__ import annotations

import os
import re
import select
import signal
import subprocess
import time
from pathlib import Path
from typing import Any, Callable, Mapping, Optional, Sequence
from urllib.parse import unquote, urlparse

from .evidence import open_log_files, redact_argv, redact_text
from .models import (
    APPROVED_REPOSITORY,
    DIR_MODE,
    LIVE_ANTIGRAVITY_UNSUPPORTED_REASON,
    LIVE_TEST_PROFILE_UNSUPPORTED_REASON,
    LIVE_WRITER_UNSUPPORTED_REASON,
    PROCESS_KILL_GRACE_SECONDS,
    Action,
    BridgeError,
    CommandEvidence,
    Disposition,
    TEST_PROFILES,
    TaskManifest,
    WriterTool,
)

GitRunner = Callable[[Sequence[str], Path], subprocess.CompletedProcess]

_GITHUB_HTTPS_RE = re.compile(
    r"(?i)^https?://(?:[^/@]+@)?github\.com[/:]([^/]+)/([^/]+?)(?:\.git)?/?$"
)
_GITHUB_SSH_RE = re.compile(
    r"(?i)^(?:ssh://)?git@github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$"
)
_GITHUB_SSH_ALT_RE = re.compile(
    r"(?i)^ssh://(?:[^/@]+@)?github\.com/([^/]+)/([^/]+?)(?:\.git)?/?$"
)


def _git_isolated_env() -> dict[str, str]:
    """Controlled Git environment for bridge-native commit operations."""
    keep = (
        "PATH",
        "USER",
        "LOGNAME",
        "LANG",
        "LC_ALL",
        "LC_CTYPE",
        "TERM",
        "TMPDIR",
        "GIT_AUTHOR_NAME",
        "GIT_AUTHOR_EMAIL",
        "GIT_COMMITTER_NAME",
        "GIT_COMMITTER_EMAIL",
        "GIT_AUTHOR_DATE",
        "GIT_COMMITTER_DATE",
    )
    out = {k: os.environ[k] for k in keep if k in os.environ}
    out["GIT_CONFIG_NOSYSTEM"] = "1"
    out["GIT_CONFIG_GLOBAL"] = os.devnull
    out["GIT_TERMINAL_PROMPT"] = "0"
    out["GIT_ASKPASS"] = "/bin/false"
    out["GCM_INTERACTIVE"] = "never"
    out["GIT_EDITOR"] = ":"
    out["EDITOR"] = ":"
    out["VISUAL"] = ":"
    out["GIT_SEQUENCE_EDITOR"] = ":"
    # Explicitly drop credential / SSH side channels.
    for drop in (
        "SSH_AUTH_SOCK",
        "SSH_AGENT_PID",
        "GIT_SSH",
        "GIT_SSH_COMMAND",
        "GH_TOKEN",
        "GITHUB_TOKEN",
        "GIT_CONFIG_SYSTEM",
    ):
        out.pop(drop, None)
    return out


def _git_safe_config_args(hooks_path: Path) -> list[str]:
    """Per-invocation overrides that disable hooks, signing, filters side channels."""
    false_bin = "/bin/false"
    return [
        "-c",
        f"core.hooksPath={hooks_path}",
        "-c",
        "core.fsmonitor=",
        "-c",
        "core.fsmonitorHook=",
        "-c",
        "core.useBuiltinFSMonitor=false",
        "-c",
        "core.editor=:",
        "-c",
        "sequence.editor=:",
        "-c",
        "commit.gpgsign=false",
        "-c",
        "tag.gpgsign=false",
        "-c",
        f"gpg.program={false_bin}",
        "-c",
        f"gpg.ssh.program={false_bin}",
        "-c",
        "credential.helper=",
        "-c",
        "diff.external=",
        "-c",
        "interactive.diffFilter=",
        "-c",
        "merge.tool=",
        "-c",
        "i18n.commitEncoding=utf-8",
    ]


def _run_git(
    args: Sequence[str],
    cwd: Path,
    *,
    check: bool = False,
    timeout: int = 60,
    text: bool = True,
    env: Optional[Mapping[str, str]] = None,
    isolated: bool = False,
    hooks_path: Optional[Path] = None,
) -> subprocess.CompletedProcess:
    git_argv: list[str] = ["git"]
    run_env: Optional[dict[str, str]] = None
    if isolated:
        run_env = _git_isolated_env()
        if env:
            run_env.update({k: v for k, v in env.items() if v is not None})
        hp = hooks_path
        if hp is None:
            raise BridgeError("isolated git requires hooks_path")
        git_argv.extend(_git_safe_config_args(hp))
    elif env is not None:
        run_env = dict(env)
    git_argv.extend(args)
    return subprocess.run(
        git_argv,
        cwd=str(cwd),
        capture_output=True,
        text=text,
        shell=False,
        check=check,
        timeout=timeout,
        env=run_env,
    )


def git_is_repo(worktree: Path) -> bool:
    proc = _run_git(["rev-parse", "--is-inside-work-tree"], worktree)
    return proc.returncode == 0 and proc.stdout.strip() == "true"


def git_head(worktree: Path) -> str:
    proc = _run_git(["rev-parse", "HEAD"], worktree, check=True)
    return proc.stdout.strip()


def git_branch(worktree: Path) -> str:
    proc = _run_git(["branch", "--show-current"], worktree, check=True)
    return proc.stdout.strip()


def git_detached(worktree: Path) -> bool:
    return git_branch(worktree) == ""


def git_dirty(worktree: Path) -> bool:
    proc = _run_git(["status", "--porcelain", "-z"], worktree, check=True, text=False)
    return bool(proc.stdout)


def validate_repo_relative_path(path: str, worktree: Path) -> str:
    """Validate a Git pathspec as a safe repository-relative path."""
    if path is None:
        raise BridgeError("empty path rejected")
    if not isinstance(path, str):
        raise BridgeError("path must be a string")
    if path == "":
        raise BridgeError("empty path rejected")
    if "\x00" in path:
        raise BridgeError("NUL in path rejected")
    if path.startswith("/") or path.startswith("\\"):
        raise BridgeError(f"absolute path rejected: {path}")
    # Reject Windows drive / UNC and backslash ambiguity on Linux bridge.
    if "\\" in path:
        raise BridgeError(f"backslash path rejected: {path}")
    if re.match(r"^[A-Za-z]:", path):
        raise BridgeError(f"drive path rejected: {path}")
    parts = Path(path).parts
    if any(part == ".." for part in parts):
        raise BridgeError(f"path traversal rejected: {path}")
    if any(part == "" for part in parts):
        raise BridgeError(f"empty path segment rejected: {path}")

    worktree_r = worktree.resolve(strict=False)
    candidate = (worktree_r / path).resolve(strict=False)
    try:
        candidate.relative_to(worktree_r)
    except ValueError as exc:
        raise BridgeError(f"path escapes worktree: {path}") from exc

    # Symlink escape: if an existing path component is a symlink leaving the tree.
    cursor = worktree_r
    for part in Path(path).parts:
        cursor = cursor / part
        if cursor.is_symlink():
            resolved = cursor.resolve(strict=False)
            try:
                resolved.relative_to(worktree_r)
            except ValueError as exc:
                raise BridgeError(f"symlink escape rejected: {path}") from exc
    return path


def _nul_split_paths(raw: bytes) -> list[str]:
    if not raw:
        return []
    parts = raw.split(b"\x00")
    out: list[str] = []
    for part in parts:
        if not part:
            continue
        out.append(part.decode("utf-8", errors="surrogateescape"))
    return out


def git_nul_name_only(args: Sequence[str], worktree: Path, *, timeout: int = 60) -> list[str]:
    """Run a git command expected to emit NUL-terminated paths; never parse porcelain."""
    proc = _run_git(list(args), worktree, check=False, timeout=timeout, text=False)
    if proc.returncode != 0:
        err = (proc.stderr or b"").decode("utf-8", errors="replace").strip()
        raise BridgeError(f"git {' '.join(args)} failed: {err or proc.returncode}")
    return _nul_split_paths(proc.stdout or b"")


def collect_change_scope(
    worktree: Path,
    starting_head: str,
    ending_head: str,
) -> dict[str, list[str]]:
    """Union of committed, staged, unstaged, and untracked paths (NUL-safe)."""
    committed: list[str] = []
    if starting_head and ending_head and starting_head != ending_head:
        committed = git_nul_name_only(
            ["diff", "--name-only", "-z", f"{starting_head}..{ending_head}"],
            worktree,
        )
    elif starting_head and ending_head and starting_head == ending_head:
        committed = []

    staged = git_nul_name_only(["diff", "--cached", "--name-only", "-z"], worktree)
    unstaged = git_nul_name_only(["diff", "--name-only", "-z"], worktree)
    untracked = git_nul_name_only(
        ["ls-files", "--others", "--exclude-standard", "-z"],
        worktree,
    )

    def _clean(paths: list[str]) -> list[str]:
        cleaned: list[str] = []
        for p in paths:
            cleaned.append(validate_repo_relative_path(p, worktree))
        return cleaned

    return {
        "committed": _clean(committed),
        "staged": _clean(staged),
        "unstaged": _clean(unstaged),
        "untracked": _clean(untracked),
    }


def union_change_paths(scope: Mapping[str, Sequence[str]]) -> list[str]:
    seen: set[str] = set()
    ordered: list[str] = []
    for key in ("committed", "staged", "unstaged", "untracked"):
        for path in scope.get(key, ()):
            if path not in seen:
                seen.add(path)
                ordered.append(path)
    return ordered


def enforce_change_scope(
    manifest: TaskManifest,
    starting_head: str,
    ending_head: str,
    *,
    scope: Optional[Mapping[str, Sequence[str]]] = None,
) -> list[str]:
    """Validate allowCommit semantics and allowed/forbidden path sets."""
    scope_map = dict(scope) if scope is not None else collect_change_scope(
        manifest.worktree, starting_head, ending_head
    )
    changed = union_change_paths(scope_map)
    allowed = set(manifest.allowed_files)
    forbidden = set(manifest.forbidden_files)

    for path in changed:
        validate_repo_relative_path(path, manifest.worktree)
        if path in forbidden:
            raise BridgeError(f"changed file is forbidden: {path}")
        if allowed and path not in allowed:
            raise BridgeError(f"changed file outside allowedFiles: {path}")
        if not allowed and path:
            # Empty allow-list means no file mutations permitted for writer tasks.
            if manifest.action in {Action.CURSOR_TASK, Action.CODEX_TASK}:
                raise BridgeError(f"changed file outside allowedFiles: {path}")

    if manifest.action in {Action.CURSOR_TASK, Action.CODEX_TASK}:
        if not manifest.allow_commit:
            if ending_head != starting_head:
                raise BridgeError(
                    "allowCommit=false but HEAD moved "
                    f"({starting_head} -> {ending_head})"
                )
            if scope_map.get("staged"):
                raise BridgeError(
                    "allowCommit=false but staged files present: "
                    f"{list(scope_map['staged'])}"
                )
            # Uncommitted (unstaged/untracked) paths must still be allowed.
            for path in list(scope_map.get("unstaged", ())) + list(
                scope_map.get("untracked", ())
            ):
                if allowed and path not in allowed:
                    raise BridgeError(
                        f"uncommitted path outside allowedFiles with allowCommit=false: {path}"
                    )
                if path in forbidden:
                    raise BridgeError(f"changed file is forbidden: {path}")
        else:
            # allowCommit=true: HEAD may move; every committed/staged/uncommitted path allowed.
            for path in changed:
                if path in forbidden:
                    raise BridgeError(f"changed file is forbidden: {path}")
                if allowed and path not in allowed:
                    raise BridgeError(f"changed file outside allowedFiles: {path}")

    return changed


# Back-compat alias used by older tests.
def enforce_post_change_scope(manifest: TaskManifest, changed: Sequence[str]) -> None:
    for path in changed:
        validate_repo_relative_path(path, manifest.worktree)
        if path in set(manifest.forbidden_files):
            raise BridgeError(f"changed file is forbidden: {path}")
        if manifest.allowed_files and path not in set(manifest.allowed_files):
            raise BridgeError(f"changed file outside allowedFiles: {path}")


def git_changed_files(worktree: Path) -> list[str]:
    """Legacy helper — prefer collect_change_scope. Uses NUL-safe unstaged+untracked+staged."""
    scope = collect_change_scope(worktree, git_head(worktree), git_head(worktree))
    return union_change_paths(scope)


def sanitize_remote_url(url: str) -> str:
    """Strip credential userinfo before any logging or comparison display."""
    return redact_text(url.strip())


def parse_github_owner_repo(url: str) -> str:
    """Normalize accepted HTTPS/SSH GitHub remotes to owner/name."""
    raw = url.strip()
    if not raw:
        raise BridgeError("empty remote URL rejected")
    # Remove credential userinfo before parse without logging secrets.
    sanitized = raw
    if "://" in sanitized:
        scheme, rest = sanitized.split("://", 1)
        if "@" in rest and "/" in rest.split("@", 1)[1]:
            rest = rest.split("@", 1)[1]
            sanitized = f"{scheme}://{rest}"
    for pattern in (_GITHUB_HTTPS_RE, _GITHUB_SSH_RE, _GITHUB_SSH_ALT_RE):
        match = pattern.match(sanitized) or pattern.match(raw)
        if match:
            owner = unquote(match.group(1))
            name = unquote(match.group(2))
            if name.endswith(".git"):
                name = name[:-4]
            return f"{owner}/{name}"
    # urlparse fallback for https with credentials already stripped
    parsed = urlparse(sanitized if "://" in sanitized else f"ssh://{sanitized}")
    host = (parsed.hostname or "").lower()
    if host == "github.com" and parsed.path:
        parts = [p for p in parsed.path.strip("/").split("/") if p]
        if len(parts) >= 2:
            owner, name = parts[0], parts[1]
            if name.endswith(".git"):
                name = name[:-4]
            return f"{owner}/{name}"
    raise BridgeError(f"malformed or non-GitHub remote rejected: {sanitize_remote_url(raw)}")


def verify_repository_binding(manifest: TaskManifest) -> dict[str, Any]:
    if manifest.repository != APPROVED_REPOSITORY:
        raise BridgeError(
            f"repository must be {APPROVED_REPOSITORY}; got {manifest.repository}"
        )
    proc = _run_git(["remote", "get-url", "origin"], manifest.worktree, check=False)
    if proc.returncode != 0:
        raise BridgeError(
            f"origin remote missing or unreadable: {redact_text(proc.stderr.strip())}"
        )
    remote = proc.stdout.strip()
    owner_repo = parse_github_owner_repo(remote)
    if owner_repo != APPROVED_REPOSITORY:
        raise BridgeError(
            f"origin remote repository mismatch: expected {APPROVED_REPOSITORY}, "
            f"got {owner_repo} (url={sanitize_remote_url(remote)})"
        )
    return {
        "repository": APPROVED_REPOSITORY,
        "originSanitized": sanitize_remote_url(remote),
        "ownerRepo": owner_repo,
    }


def verify_authoritative_base(manifest: TaskManifest) -> dict[str, Any]:
    """Prove base exists as a commit and is an ancestor of expectedHead."""
    base = manifest.authoritative_base
    head = manifest.expected_head
    exists = _run_git(["cat-file", "-e", f"{base}^{{commit}}"], manifest.worktree, check=False)
    if exists.returncode != 0:
        raise BridgeError(f"authoritativeBase does not exist as commit: {base}")
    ancestor = _run_git(
        ["merge-base", "--is-ancestor", base, head],
        manifest.worktree,
        check=False,
    )
    if ancestor.returncode != 0:
        raise BridgeError(
            f"authoritativeBase {base} is not an ancestor of expectedHead {head}"
        )
    return {"authoritativeBase": base, "expectedHead": head, "ancestor": True}


def verify_git_preconditions(manifest: TaskManifest) -> dict[str, Any]:
    worktree = manifest.worktree
    if not git_is_repo(worktree):
        raise BridgeError(f"not a git repository: {worktree}")

    binding = verify_repository_binding(manifest)
    base_info = verify_authoritative_base(manifest)

    head = git_head(worktree)
    branch = git_branch(worktree)
    detached = branch == ""
    dirty = git_dirty(worktree)

    if head != manifest.expected_head:
        raise BridgeError(
            f"unexpected HEAD: have {head}, expected {manifest.expected_head}"
        )

    if manifest.action == Action.ANTIGRAVITY_AUDIT:
        if not detached:
            raise BridgeError("ANTIGRAVITY_AUDIT requires detached HEAD")
        if dirty:
            raise BridgeError("ANTIGRAVITY_AUDIT requires a clean worktree")
        if manifest.writer_tool != WriterTool.NONE:
            raise BridgeError("ANTIGRAVITY_AUDIT writerTool must be none")
    else:
        if detached and manifest.action in {
            Action.CURSOR_TASK,
            Action.CODEX_TASK,
            Action.GIT_COMMIT_PROOF,
            Action.TEST_PROFILE,
        }:
            raise BridgeError("writers may not use detached HEAD")
        if not detached and branch != manifest.branch:
            raise BridgeError(
                f"unexpected branch: have {branch or '(detached)'}, expected {manifest.branch}"
            )
        if manifest.action in {Action.CURSOR_TASK, Action.CODEX_TASK} and dirty:
            raise BridgeError("dirty worktree at start")
        if manifest.action == Action.GIT_COMMIT_PROOF:
            scope = collect_change_scope(worktree, head, head)
            changed = union_change_paths(scope)
            allowed = set(manifest.allowed_files)
            if not changed:
                raise BridgeError("GIT_COMMIT_PROOF requires the allowed file to be modified")
            if set(changed) != allowed:
                raise BridgeError(
                    f"GIT_COMMIT_PROOF dirty files must equal allowedFiles; have {changed}"
                )

    if branch in {"main", "master"} and manifest.action in {
        Action.GIT_COMMIT_PROOF,
        Action.CURSOR_TASK,
        Action.CODEX_TASK,
    }:
        raise BridgeError(f"direct writes to {branch} prohibited")

    return {
        "head": head,
        "branch": branch or "(detached)",
        "detached": detached,
        "dirty": dirty,
        "repository": binding,
        "authoritativeBase": base_info,
    }


def live_writer_unsupported(action: Action) -> Optional[str]:
    """v1: do not launch Cursor/Codex/Antigravity/TEST_PROFILE child processes."""
    if action in {Action.CURSOR_TASK, Action.CODEX_TASK}:
        return LIVE_WRITER_UNSUPPORTED_REASON
    if action == Action.ANTIGRAVITY_AUDIT:
        return LIVE_ANTIGRAVITY_UNSUPPORTED_REASON
    if action == Action.TEST_PROFILE:
        return LIVE_TEST_PROFILE_UNSUPPORTED_REASON
    return None


def build_action_argv(manifest: TaskManifest) -> tuple[list[str], dict[str, Any]]:
    """Construct allowlisted argv. Never accepts free-form command from manifest.

    For CURSOR_TASK/CODEX_TASK/ANTIGRAVITY_AUDIT/TEST_PROFILE this remains planning
    evidence only in v1 — live launch is blocked by live_writer_unsupported().
    """
    meta: dict[str, Any] = {"unsupported": False, "reason": None, "stdin": None}

    if manifest.action == Action.REPO_PROBE:
        return ["git", "repo-probe", "--fixed"], meta

    if manifest.action == Action.GIT_COMMIT_PROOF:
        return ["git", "commit-proof", "--internal"], meta

    if manifest.action == Action.CURSOR_TASK:
        prompt = manifest.prompt_file.read_text(encoding="utf-8") if manifest.prompt_file else ""
        argv = [
            "agent",
            "--print",
            "--trust",
            "--workspace",
            str(manifest.worktree),
            "--output-format",
            "text",
            prompt,
        ]
        return argv, meta

    if manifest.action == Action.CODEX_TASK:
        argv = [
            "codex",
            "exec",
            "--sandbox",
            "workspace-write",
            "--approve-for-me",
            "-C",
            str(manifest.worktree),
            "--ephemeral",
            "-",
        ]
        meta["stdin_path"] = str(manifest.prompt_file) if manifest.prompt_file else None
        return argv, meta

    if manifest.action == Action.ANTIGRAVITY_AUDIT:
        prompt = manifest.prompt_file.read_text(encoding="utf-8") if manifest.prompt_file else ""
        argv = [
            "agy",
            "--print",
            "--sandbox",
            "--mode",
            "plan",
            "--output-format",
            "text",
            prompt,
        ]
        return argv, meta

    if manifest.action == Action.TEST_PROFILE:
        assert manifest.test_profile is not None
        # Planning preview only — live child launch is UNSUPPORTED in v1.
        profile_argv = list(TEST_PROFILES[manifest.test_profile])
        return profile_argv, meta

    meta["unsupported"] = True
    meta["reason"] = f"unknown action {manifest.action}"
    return [], meta


def run_process(
    argv: Sequence[str],
    *,
    cwd: Path,
    timeout_seconds: int,
    logs_dir: Path,
    task_id: str,
    stdin_path: Optional[Path] = None,
    env: Optional[Mapping[str, str]] = None,
    on_start: Optional[Callable[[int], None]] = None,
) -> CommandEvidence:
    stdout_path, stderr_path, stdout_w, stderr_w = open_log_files(logs_dir, task_id)
    stdin_f = None
    proc: Optional[subprocess.Popen] = None
    evidence = CommandEvidence(
        argv=redact_argv(list(argv)),
        stdout_log=str(stdout_path),
        stderr_log=str(stderr_path),
    )
    child_env = _sanitized_env(env)
    try:
        if stdin_path is not None:
            stdin_f = open(stdin_path, "rb")
        proc = subprocess.Popen(
            list(argv),
            cwd=str(cwd),
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            stdin=stdin_f if stdin_f is not None else subprocess.DEVNULL,
            shell=False,
            start_new_session=True,
            env=child_env,
        )
        evidence.child_pid = proc.pid
        if on_start is not None:
            on_start(proc.pid)
        try:
            evidence.exit_code = _drain_bounded(
                proc,
                stdout_w,
                stderr_w,
                timeout_seconds=timeout_seconds,
                evidence=evidence,
            )
        except subprocess.TimeoutExpired:
            evidence.timed_out = True
            evidence.sigterm_sent = True
            _signal_group(proc.pid, signal.SIGTERM)
            try:
                evidence.exit_code = _drain_bounded(
                    proc,
                    stdout_w,
                    stderr_w,
                    timeout_seconds=PROCESS_KILL_GRACE_SECONDS,
                    evidence=evidence,
                )
            except subprocess.TimeoutExpired:
                evidence.sigkill_sent = True
                _signal_group(proc.pid, signal.SIGKILL)
                evidence.exit_code = _drain_bounded(
                    proc,
                    stdout_w,
                    stderr_w,
                    timeout_seconds=PROCESS_KILL_GRACE_SECONDS,
                    evidence=evidence,
                )
    finally:
        stdout_w.close()
        stderr_w.close()
        evidence.stdout_bytes_captured = stdout_w.bytes_captured
        evidence.stderr_bytes_captured = stderr_w.bytes_captured
        evidence.stdout_truncated = stdout_w.truncated
        evidence.stderr_truncated = stderr_w.truncated
        if proc is not None:
            for stream in (proc.stdout, proc.stderr):
                if stream is not None:
                    try:
                        stream.close()
                    except OSError:
                        pass
        if stdin_f is not None:
            stdin_f.close()
    return evidence


def _drain_bounded(
    proc: subprocess.Popen,
    stdout_w: Any,
    stderr_w: Any,
    *,
    timeout_seconds: int,
    evidence: CommandEvidence,
) -> int:
    """Read pipes with select until process exits or timeout; enforce log bounds."""
    deadline = time.monotonic() + timeout_seconds
    writers: dict[int, Any] = {}
    if proc.stdout is not None:
        writers[proc.stdout.fileno()] = stdout_w
    if proc.stderr is not None:
        writers[proc.stderr.fileno()] = stderr_w

    while writers and proc.poll() is None:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise subprocess.TimeoutExpired(proc.args, timeout_seconds)
        ready, _, _ = select.select(list(writers.keys()), [], [], min(remaining, 0.25))
        for fd in ready:
            chunk = os.read(fd, 65536)
            if not chunk:
                del writers[fd]
            else:
                writers[fd].write(chunk)

    for stream, writer in ((proc.stdout, stdout_w), (proc.stderr, stderr_w)):
        if stream is None:
            continue
        try:
            rest = stream.read()
            if rest:
                writer.write(rest)
        except (ValueError, OSError):
            pass

    rc = proc.poll()
    if rc is None:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise subprocess.TimeoutExpired(proc.args, timeout_seconds)
        return proc.wait(timeout=max(remaining, 0.01))
    return rc


def _signal_group(pid: int, sig: signal.Signals) -> None:
    try:
        os.killpg(pid, sig)
    except ProcessLookupError:
        try:
            os.kill(pid, sig)
        except ProcessLookupError:
            pass


def _sanitized_env(extra: Optional[Mapping[str, str]] = None) -> dict[str, str]:
    """Pass a minimal environment — never dump or log the full host env.

    Intentionally omits SSH_AUTH_SOCK and Git credential helpers so child
    processes cannot silently push. Writer CLIs remain UNSUPPORTED in v1
    until isolated HOME/auth is proven.
    """
    keep = (
        "PATH",
        "USER",
        "LOGNAME",
        "LANG",
        "LC_ALL",
        "LC_CTYPE",
        "TERM",
        "TMPDIR",
        "XDG_RUNTIME_DIR",
        "GIT_AUTHOR_NAME",
        "GIT_AUTHOR_EMAIL",
        "GIT_COMMITTER_NAME",
        "GIT_COMMITTER_EMAIL",
    )
    out = {k: os.environ[k] for k in keep if k in os.environ}
    out["GIT_TERMINAL_PROMPT"] = "0"
    out["GIT_ASKPASS"] = os.devnull
    out["GCM_INTERACTIVE"] = "never"
    # Do not pass HOME/SSH_AUTH_SOCK — credential isolation boundary for v1 children.
    if extra:
        blocked = {
            "SSH_AUTH_SOCK",
            "GIT_SSH",
            "GIT_SSH_COMMAND",
            "GH_TOKEN",
            "GITHUB_TOKEN",
            "GIT_ASKPASS",
        }
        out.update({k: v for k, v in extra.items() if v is not None and k not in blocked})
    return out


def execute_repo_probe(manifest: TaskManifest) -> tuple[CommandEvidence, dict[str, Any], Disposition]:
    """Run fixed git evidence commands; never force SUCCESS when required commands fail."""
    commands = [
        ("status", ["status", "--short", "--branch"], True),
        ("branch", ["branch", "--show-current"], True),
        ("head", ["rev-parse", "HEAD"], True),
        ("origin_main", ["rev-parse", "origin/main"], True),
        ("ahead_behind", ["rev-list", "--left-right", "--count", "origin/main...HEAD"], True),
    ]
    probe: dict[str, Any] = {"commands": {}}
    combined_argv: list[str] = []
    required_failed = 0
    for name, args, required in commands:
        combined_argv.extend(["git", *args, ";"])
        proc = _run_git(args, manifest.worktree, timeout=manifest.timeout_seconds)
        entry = {
            "argv": redact_argv(["git", *args]),
            "exitCode": proc.returncode,
            "stdout": redact_text(proc.stdout.strip()),
            "stderr": redact_text(proc.stderr.strip()),
            "required": required,
        }
        probe["commands"][name] = entry
        # Preserve legacy key for older readers.
        probe[" ".join(args)] = entry
        if proc.returncode != 0 and required:
            required_failed += 1

    if required_failed == 0:
        disposition = Disposition.SUCCESS
        exit_code = 0
    elif required_failed == len(commands):
        disposition = Disposition.FAILED
        exit_code = 1
    else:
        disposition = Disposition.PARTIAL
        exit_code = 1

    probe["summary"] = {
        "requiredFailed": required_failed,
        "requiredTotal": len(commands),
        "disposition": disposition.value,
    }
    evidence = CommandEvidence(
        argv=redact_argv(combined_argv),
        exit_code=exit_code,
        timed_out=False,
    )
    return evidence, probe, disposition


def _prepare_empty_hooks_dir(worktree: Path) -> Path:
    """Create an empty hooks directory inside .git for core.hooksPath override."""
    git_dir = _run_git(["rev-parse", "--git-dir"], worktree, check=True)
    git_path = Path(git_dir.stdout.strip())
    if not git_path.is_absolute():
        git_path = (worktree / git_path).resolve()
    hooks = git_path / "dot-control-empty-hooks"
    hooks.mkdir(mode=DIR_MODE, exist_ok=True)
    try:
        os.chmod(hooks, DIR_MODE)
    except OSError:
        pass
    # Ensure directory is empty — any executable hook file would be a hazard.
    for child in hooks.iterdir():
        raise BridgeError(
            f"safe hooksPath is not empty: {child.name}; refusing GIT_COMMIT_PROOF"
        )
    return hooks


def _reject_unsafe_commit_attributes(worktree: Path, path: str, *, hooks_path: Path) -> None:
    """Reject clean/smudge/filter attributes that would invoke external programs."""
    attr = _run_git(
        ["check-attr", "-a", "--", path],
        worktree,
        check=False,
        isolated=True,
        hooks_path=hooks_path,
    )
    if attr.returncode != 0:
        raise BridgeError(
            f"git check-attr failed: {redact_text(attr.stderr.strip())}"
        )
    dangerous = ("filter", "clean", "smudge", "working-tree-encoding")
    for line in (attr.stdout or "").splitlines():
        # format: path: attr: value
        parts = [p.strip() for p in line.split(":", 2)]
        if len(parts) < 3:
            continue
        _file, name, value = parts[0], parts[1], parts[2]
        if name in dangerous and value not in {"unspecified", "unset"}:
            raise BridgeError(
                f"GIT_COMMIT_PROOF refused: path {path} has attribute "
                f"{name}={value} that may invoke filters"
            )


def _reject_local_filter_config(worktree: Path, *, hooks_path: Path) -> None:
    """Reject local filter.*.clean/smudge definitions that cannot be trusted."""
    conf = _run_git(
        ["config", "--local", "--get-regexp", r"^filter\..*\.(clean|smudge)$"],
        worktree,
        check=False,
        isolated=True,
        hooks_path=hooks_path,
    )
    # exit 1 = no matches (safe); exit 0 = matches present (unsafe)
    if conf.returncode == 0 and (conf.stdout or "").strip():
        raise BridgeError(
            "GIT_COMMIT_PROOF refused: local filter clean/smudge config present"
        )
    fsmonitor = _run_git(
        ["config", "--local", "--get", "core.fsmonitor"],
        worktree,
        check=False,
        isolated=True,
        hooks_path=hooks_path,
    )
    if fsmonitor.returncode == 0 and (fsmonitor.stdout or "").strip():
        # Overridable via -c, but still reject if a custom hook path-like value looks external.
        value = fsmonitor.stdout.strip()
        if value not in {"", "false", "true"}:
            raise BridgeError(
                f"GIT_COMMIT_PROOF refused: core.fsmonitor={redact_text(value)}"
            )


def execute_git_commit_proof(manifest: TaskManifest) -> tuple[CommandEvidence, dict[str, Any]]:
    """Bridge-internal one-file commit with hooks/signing/filters disabled.

    Uses a controlled Git environment and rejects paths/config that would invoke
    external hooks, clean/smudge filters, fsmonitor hooks, or commit signing.
    """
    worktree = manifest.worktree
    allowed = manifest.allowed_files[0]
    validate_repo_relative_path(allowed, worktree)
    allowed_path = (worktree / allowed).resolve(strict=False)
    try:
        allowed_path.relative_to(worktree.resolve(strict=False))
    except ValueError as exc:
        raise BridgeError(f"allowed file escapes worktree: {allowed}") from exc
    if not allowed_path.is_file():
        raise BridgeError(f"allowed file missing: {allowed}")

    hooks_path = _prepare_empty_hooks_dir(worktree)
    _reject_unsafe_commit_attributes(worktree, allowed, hooks_path=hooks_path)
    _reject_local_filter_config(worktree, hooks_path=hooks_path)

    add = _run_git(
        ["add", "--", allowed],
        worktree,
        check=False,
        isolated=True,
        hooks_path=hooks_path,
    )
    if add.returncode != 0:
        raise BridgeError(f"git add failed: {redact_text(add.stderr.strip())}")

    staged_proc = _run_git(
        ["diff", "--cached", "--name-only", "-z"],
        worktree,
        check=False,
        text=False,
        isolated=True,
        hooks_path=hooks_path,
    )
    if staged_proc.returncode != 0:
        err = (staged_proc.stderr or b"").decode("utf-8", errors="replace").strip()
        raise BridgeError(f"git diff --cached --name-only failed: {redact_text(err)}")
    staged = _nul_split_paths(staged_proc.stdout or b"")
    for forbidden in manifest.forbidden_files:
        if forbidden in staged:
            raise BridgeError(f"commit stages forbidden file: {forbidden}")
    if staged != [allowed]:
        raise BridgeError(f"staged files mismatch: {staged}")

    check = _run_git(
        ["diff", "--cached", "--check"],
        worktree,
        check=False,
        isolated=True,
        hooks_path=hooks_path,
    )
    if check.returncode != 0:
        raise BridgeError(
            f"git diff --cached --check failed: {redact_text(check.stdout + check.stderr)}"
        )

    message = manifest.commit_message or ""
    # --no-verify is belt-and-suspenders; empty hooksPath is the primary control.
    # --no-gpg-sign disables signing even if local config requests it.
    commit = _run_git(
        ["commit", "--no-verify", "--no-gpg-sign", "-m", message],
        worktree,
        check=False,
        isolated=True,
        hooks_path=hooks_path,
    )
    if commit.returncode != 0:
        raise BridgeError(f"git commit failed: {redact_text(commit.stderr.strip())}")

    sha_proc = _run_git(
        ["rev-parse", "HEAD"],
        worktree,
        check=True,
        isolated=True,
        hooks_path=hooks_path,
    )
    sha = sha_proc.stdout.strip()
    evidence = CommandEvidence(
        argv=redact_argv(
            [
                "git",
                "-c",
                "core.hooksPath=<empty>",
                "add",
                "--",
                allowed,
                "git",
                "diff",
                "--cached",
                "--check",
                "git",
                "commit",
                "--no-verify",
                "--no-gpg-sign",
                "-m",
                message,
            ]
        ),
        exit_code=0,
        child_pid=None,
    )
    details = {
        "commitSha": sha,
        "committedFiles": [allowed],
        "pushStatus": "NOT_ATTEMPTED",
        "prStatus": "NOT_ATTEMPTED",
        "hooksDisabled": True,
        "signingDisabled": True,
    }
    return evidence, details


def sleep_interruptible(seconds: float) -> None:
    time.sleep(seconds)

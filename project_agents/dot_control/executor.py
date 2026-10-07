"""Action execution for the Dot Host-Control Bridge.

All argv arrays are constructed internally from allowlisted action types.
Manifests never supply free-form shell commands.
"""

from __future__ import annotations

import os
import signal
import subprocess
import time
from pathlib import Path
from typing import Any, Callable, Mapping, Optional, Sequence

from .evidence import open_log_files, redact_argv, utc_now_iso
from .models import (
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


def _run_git(
    args: Sequence[str],
    cwd: Path,
    *,
    check: bool = False,
    timeout: int = 60,
) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["git", *args],
        cwd=str(cwd),
        capture_output=True,
        text=True,
        shell=False,
        check=check,
        timeout=timeout,
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
    proc = _run_git(["status", "--porcelain"], worktree, check=True)
    return bool(proc.stdout.strip())


def git_changed_files(worktree: Path) -> list[str]:
    proc = _run_git(["status", "--porcelain"], worktree, check=True)
    files: list[str] = []
    for line in proc.stdout.splitlines():
        if not line.strip():
            continue
        # status format: XY PATH or XY ORIG -> PATH
        path_part = line[3:]
        if " -> " in path_part:
            path_part = path_part.split(" -> ", 1)[1]
        files.append(path_part.strip())
    return files


def verify_git_preconditions(manifest: TaskManifest) -> dict[str, Any]:
    worktree = manifest.worktree
    if not git_is_repo(worktree):
        raise BridgeError(f"not a git repository: {worktree}")

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
            # Clean except the single allowed file that will be staged/committed.
            changed = git_changed_files(worktree)
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
    }


def build_action_argv(manifest: TaskManifest) -> tuple[list[str], dict[str, Any]]:
    """Construct allowlisted argv. Never accepts free-form command from manifest."""
    meta: dict[str, Any] = {"unsupported": False, "reason": None, "stdin": None}

    if manifest.action == Action.REPO_PROBE:
        # REPO_PROBE runs multiple fixed git commands; argv is a marker.
        return ["git", "repo-probe", "--fixed"], meta

    if manifest.action == Action.GIT_COMMIT_PROOF:
        return ["git", "commit-proof", "--internal"], meta

    if manifest.action == Action.CURSOR_TASK:
        # Inspected: agent --print for non-interactive scripts; --workspace; --trust.
        # Intentionally omit --force/--yolo (unsafe blanket approval).
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
        # Inspected: codex exec --sandbox workspace-write --approve-for-me -C DIR -
        # Prompt via stdin ('-'); never danger-full-access.
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
        # Inspected: agy --print --sandbox --mode plan (read-only audit posture).
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
    stdout_path, stderr_path, stdout_f, stderr_f = open_log_files(logs_dir, task_id)
    stdin_f = None
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
            stdout=stdout_f,
            stderr=stderr_f,
            stdin=stdin_f if stdin_f is not None else subprocess.DEVNULL,
            shell=False,
            start_new_session=True,
            env=child_env,
        )
        evidence.child_pid = proc.pid
        if on_start is not None:
            on_start(proc.pid)
        try:
            evidence.exit_code = proc.wait(timeout=timeout_seconds)
        except subprocess.TimeoutExpired:
            evidence.timed_out = True
            evidence.sigterm_sent = True
            _signal_group(proc.pid, signal.SIGTERM)
            try:
                evidence.exit_code = proc.wait(timeout=PROCESS_KILL_GRACE_SECONDS)
            except subprocess.TimeoutExpired:
                evidence.sigkill_sent = True
                _signal_group(proc.pid, signal.SIGKILL)
                evidence.exit_code = proc.wait(timeout=PROCESS_KILL_GRACE_SECONDS)
    finally:
        stdout_f.close()
        stderr_f.close()
        if stdin_f is not None:
            stdin_f.close()
    return evidence


def _signal_group(pid: int, sig: signal.Signals) -> None:
    try:
        os.killpg(pid, sig)
    except ProcessLookupError:
        try:
            os.kill(pid, sig)
        except ProcessLookupError:
            pass


def _sanitized_env(extra: Optional[Mapping[str, str]] = None) -> dict[str, str]:
    """Pass a minimal environment — never dump or log the full host env."""
    keep = (
        "PATH",
        "HOME",
        "USER",
        "LOGNAME",
        "LANG",
        "LC_ALL",
        "LC_CTYPE",
        "TERM",
        "TMPDIR",
        "XDG_RUNTIME_DIR",
        "SSH_AUTH_SOCK",
        "GIT_AUTHOR_NAME",
        "GIT_AUTHOR_EMAIL",
        "GIT_COMMITTER_NAME",
        "GIT_COMMITTER_EMAIL",
    )
    out = {k: os.environ[k] for k in keep if k in os.environ}
    if extra:
        out.update({k: v for k, v in extra.items() if v is not None})
    return out


def execute_repo_probe(manifest: TaskManifest) -> tuple[CommandEvidence, dict[str, Any]]:
    commands = [
        ["status", "--short", "--branch"],
        ["branch", "--show-current"],
        ["rev-parse", "HEAD"],
        ["rev-parse", "origin/main"],
        ["rev-list", "--left-right", "--count", "origin/main...HEAD"],
    ]
    probe: dict[str, Any] = {}
    combined_argv: list[str] = []
    for args in commands:
        combined_argv.extend(["git", *args, ";"])
        proc = _run_git(args, manifest.worktree, timeout=manifest.timeout_seconds)
        key = " ".join(args)
        probe[key] = {
            "exitCode": proc.returncode,
            "stdout": proc.stdout.strip(),
            "stderr": proc.stderr.strip(),
        }
        if proc.returncode != 0 and args[0] != "rev-parse":
            # origin/main may be absent in temp fixtures; still record.
            pass
    evidence = CommandEvidence(
        argv=redact_argv(combined_argv),
        exit_code=0,
        timed_out=False,
    )
    return evidence, probe


def execute_git_commit_proof(manifest: TaskManifest) -> tuple[CommandEvidence, dict[str, Any]]:
    worktree = manifest.worktree
    allowed = manifest.allowed_files[0]
    allowed_path = (worktree / allowed).resolve(strict=False)
    try:
        allowed_path.relative_to(worktree.resolve(strict=False))
    except ValueError as exc:
        raise BridgeError(f"allowed file escapes worktree: {allowed}") from exc
    if any(part == ".." for part in Path(allowed).parts):
        raise BridgeError(f"traversal in allowedFiles: {allowed}")
    if not allowed_path.is_file():
        raise BridgeError(f"allowed file missing: {allowed}")

    # Stage only the allowed file — no arbitrary git flags from manifest.
    add = _run_git(["add", "--", allowed], worktree, check=False)
    if add.returncode != 0:
        raise BridgeError(f"git add failed: {add.stderr.strip()}")

    cached = _run_git(["diff", "--cached", "--name-only"], worktree, check=True)
    staged = [line for line in cached.stdout.splitlines() if line.strip()]
    for forbidden in manifest.forbidden_files:
        if forbidden in staged:
            raise BridgeError(f"commit stages forbidden file: {forbidden}")
    if staged != [allowed]:
        raise BridgeError(f"staged files mismatch: {staged}")

    check = _run_git(["diff", "--cached", "--check"], worktree, check=False)
    if check.returncode != 0:
        raise BridgeError(f"git diff --cached --check failed: {check.stdout}{check.stderr}")

    message = manifest.commit_message or ""
    commit = _run_git(["commit", "-m", message], worktree, check=False)
    if commit.returncode != 0:
        raise BridgeError(f"git commit failed: {commit.stderr.strip()}")

    sha = git_head(worktree)
    evidence = CommandEvidence(
        argv=redact_argv(
            [
                "git",
                "add",
                "--",
                allowed,
                "git",
                "diff",
                "--cached",
                "--check",
                "git",
                "commit",
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
    }
    return evidence, details


def enforce_post_change_scope(manifest: TaskManifest, changed: Sequence[str]) -> None:
    allowed = set(manifest.allowed_files)
    forbidden = set(manifest.forbidden_files)
    for path in changed:
        if path in forbidden:
            raise BridgeError(f"changed file is forbidden: {path}")
        if allowed and path not in allowed:
            raise BridgeError(f"changed file outside allowedFiles: {path}")


def sleep_interruptible(seconds: float) -> None:
    time.sleep(seconds)

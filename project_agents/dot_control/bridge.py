#!/usr/bin/env python3
"""AmrudAi Dot Host-Control Bridge — CLI and manifest lifecycle.

Runs as a Linux user-space process (no sudo/root). Consumes validated task
manifests from the control root inbox and executes allowlisted actions only.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import traceback
from pathlib import Path
from typing import Any, Optional

from .evidence import (
    append_bridge_log,
    atomic_write_json,
    redact_argv,
    utc_now_iso,
    write_result,
    write_state,
)
from .executor import (
    build_action_argv,
    enforce_change_scope,
    execute_git_commit_proof,
    execute_repo_probe,
    git_head,
    live_writer_unsupported,
    run_process,
    verify_git_preconditions,
)
from .locks import WorktreeLock, inspect_lock
from .models import (
    CONTROL_SUBDIRS,
    DAEMON_POLL_SECONDS,
    DEFAULT_CONTROL_ROOT,
    Action,
    BridgeError,
    Disposition,
    TaskManifest,
    TaskResult,
)
from .paths import init_control_root, validate_control_root_layout
from .validator import (
    PathPolicy,
    load_json_object,
    normalize_plan,
    validate_manifest_dict,
)


class DotControlBridge:
    def __init__(
        self,
        control_root: Path = DEFAULT_CONTROL_ROOT,
        *,
        policy: Optional[PathPolicy] = None,
        create_dirs: bool = False,
    ):
        self.control_root = Path(control_root)
        self.policy = policy or PathPolicy(control_root=self.control_root)
        if create_dirs:
            self.init_control_root()
        elif self.control_root.exists():
            validate_control_root_layout(self.control_root, require_subdirs=False)

    # --- layout -------------------------------------------------------------

    @property
    def inbox(self) -> Path:
        return self.control_root / "inbox"

    @property
    def processing(self) -> Path:
        return self.control_root / "processing"

    @property
    def results(self) -> Path:
        return self.control_root / "results"

    @property
    def logs(self) -> Path:
        return self.control_root / "logs"

    @property
    def locks(self) -> Path:
        return self.control_root / "locks"

    @property
    def state(self) -> Path:
        return self.control_root / "state"

    @property
    def quarantine(self) -> Path:
        return self.control_root / "quarantine"

    def init_control_root(self) -> list[str]:
        """Explicit init — creates required subdirectories with hardened modes."""
        return init_control_root(self.control_root)

    # --- lifecycle ----------------------------------------------------------

    def list_inbox_ready(self) -> list[Path]:
        if not self.inbox.is_dir() or self.inbox.is_symlink():
            return []
        return sorted(p for p in self.inbox.glob("*.json") if p.is_file() and not p.is_symlink())

    def task_id_known(self, task_id: str) -> bool:
        candidates = [
            self.processing / f"{task_id}.json",
            self.results / f"{task_id}.result.json",
            self.state / f"{task_id}.json",
            self.quarantine / f"{task_id}.json",
        ]
        return any(p.exists() for p in candidates)

    def quarantine_manifest(self, path: Path, reason: str) -> Path:
        if self.quarantine.is_symlink():
            raise BridgeError(f"quarantine must not be a symlink: {self.quarantine}")
        self.quarantine.mkdir(parents=True, exist_ok=True)
        dest = self.quarantine / path.name
        if dest.exists():
            dest = self.quarantine / f"{path.stem}.{int(time.time())}.json"
        os.replace(path, dest)
        meta = dest.with_suffix(dest.suffix + ".reason.json")
        atomic_write_json(meta, {"reason": reason, "at": utc_now_iso(), "source": str(path)})
        return dest

    def claim_manifest(self, inbox_path: Path) -> Path:
        if self.processing.is_symlink():
            raise BridgeError(f"processing must not be a symlink: {self.processing}")
        self.processing.mkdir(parents=True, exist_ok=True)
        dest = self.processing / inbox_path.name
        if dest.exists():
            raise BridgeError(f"duplicate processing entry: {dest.name}")
        os.replace(inbox_path, dest)
        return dest

    def validate_only(self, manifest_path: Path) -> dict[str, Any]:
        data = load_json_object(manifest_path)
        manifest = validate_manifest_dict(data, policy=self.policy, require_paths_exist=True)
        plan = normalize_plan(manifest)
        plan["disposition"] = Disposition.DRY_RUN.value
        unsupported = live_writer_unsupported(manifest.action)
        if unsupported:
            plan["liveExecution"] = "UNSUPPORTED"
            plan["liveExecutionReason"] = unsupported
            plan["constructedArgvPreview"] = redact_argv(
                plan.get("constructedArgvPreview") or []
            )
        return plan

    def process_one(self, manifest_path: Optional[Path] = None) -> TaskResult:
        validate_control_root_layout(self.control_root, require_subdirs=False)
        if manifest_path is None:
            ready = self.list_inbox_ready()
            if not ready:
                raise BridgeError("inbox empty", disposition=Disposition.REJECTED)
            manifest_path = ready[0]

        manifest_path = Path(manifest_path)
        if manifest_path.suffix != ".json":
            raise BridgeError("partial manifest ignored until atomic rename to .json")

        processing_path: Optional[Path] = None
        try:
            data = load_json_object(manifest_path)
            task_id_preview = data.get("taskId")
            if isinstance(task_id_preview, str) and self.task_id_known(task_id_preview):
                if manifest_path.parent.resolve() == self.inbox.resolve():
                    self.quarantine_manifest(manifest_path, f"duplicate taskId: {task_id_preview}")
                raise BridgeError(f"duplicate taskId: {task_id_preview}")

            if manifest_path.parent.resolve() == self.inbox.resolve():
                processing_path = self.claim_manifest(manifest_path)
            else:
                processing_path = manifest_path

            manifest = validate_manifest_dict(data, policy=self.policy, require_paths_exist=True)
        except BridgeError as exc:
            if processing_path is not None and processing_path.exists():
                self.quarantine_manifest(processing_path, str(exc))
            elif manifest_path.exists() and manifest_path.parent.resolve() == self.inbox.resolve():
                self.quarantine_manifest(manifest_path, str(exc))
            raise

        return self._execute_manifest(manifest, processing_path)

    def _execute_manifest(self, manifest: TaskManifest, processing_path: Path) -> TaskResult:
        started = utc_now_iso()
        lock = WorktreeLock(self.locks, manifest.worktree)
        lock_lifecycle: dict[str, Any] = {}
        starting_head = ""
        ending_head = ""
        branch_observed = manifest.branch
        command_argv: list[str] = []
        exit_code: Optional[int] = None
        timed_out = False
        stdout_log = None
        stderr_log = None
        changed: list[str] = []
        committed: list[str] = []
        commit_sha = None
        push_status = "NOT_ATTEMPTED"
        pr_status = "NOT_ATTEMPTED"
        probe: dict[str, Any] = {}
        notes: list[str] = []
        error: Optional[str] = None
        disposition = Disposition.FAILED
        stdout_bytes = 0
        stderr_bytes = 0
        stdout_truncated = False
        stderr_truncated = False
        bridge_log_truncated = False
        selected_tool = manifest.writer_tool.value
        if manifest.action == Action.ANTIGRAVITY_AUDIT:
            selected_tool = manifest.verifier_tool.value
        elif manifest.action in {Action.REPO_PROBE, Action.GIT_COMMIT_PROOF, Action.TEST_PROFILE}:
            selected_tool = "bridge"

        try:
            lock.acquire(
                task_id=manifest.task_id,
                action=manifest.action.value,
                writer_tool=manifest.writer_tool.value,
                branch=manifest.branch,
                expected_head=manifest.expected_head,
            )
            lock_lifecycle = dict(lock.lifecycle)

            pre = verify_git_preconditions(manifest)
            starting_head = pre["head"]
            branch_observed = pre["branch"]
            _, bridge_log_truncated = append_bridge_log(
                self.logs,
                manifest.task_id,
                [f"{utc_now_iso()} preconditions ok head={starting_head} branch={branch_observed}"],
            )

            if manifest.action == Action.REPO_PROBE:
                evidence, probe, disposition = execute_repo_probe(manifest)
                command_argv = evidence.argv
                exit_code = evidence.exit_code
                if disposition != Disposition.SUCCESS:
                    error = (
                        f"REPO_PROBE evidence incomplete: "
                        f"{probe.get('summary', {})}"
                    )

            elif manifest.action == Action.GIT_COMMIT_PROOF:
                evidence, details = execute_git_commit_proof(manifest)
                command_argv = evidence.argv
                exit_code = evidence.exit_code
                commit_sha = details["commitSha"]
                committed = list(details["committedFiles"])
                push_status = details["pushStatus"]
                pr_status = details["prStatus"]
                disposition = Disposition.SUCCESS

            elif manifest.action in {
                Action.CURSOR_TASK,
                Action.CODEX_TASK,
                Action.ANTIGRAVITY_AUDIT,
                Action.TEST_PROFILE,
            }:
                argv, meta = build_action_argv(manifest)
                live_block = live_writer_unsupported(manifest.action)
                if live_block:
                    # Construct argv for planning evidence; do not launch child.
                    command_argv = redact_argv(argv) if argv else []
                    disposition = Disposition.UNSUPPORTED
                    error = live_block
                    notes.append(f"UNSUPPORTED: {error}")
                    notes.append("child process not launched")
                    exit_code = None
                elif meta.get("unsupported"):
                    disposition = Disposition.UNSUPPORTED
                    error = meta.get("reason")
                    command_argv = []
                    notes.append(f"UNSUPPORTED: {error}")
                else:
                    command_argv = redact_argv(argv)
                    stdin_path = Path(meta["stdin_path"]) if meta.get("stdin_path") else None

                    def _on_start(pid: int) -> None:
                        lock.update_child_pid(pid)

                    evidence = run_process(
                        argv,
                        cwd=manifest.worktree,
                        timeout_seconds=manifest.timeout_seconds,
                        logs_dir=self.logs,
                        task_id=manifest.task_id,
                        stdin_path=stdin_path,
                        on_start=_on_start,
                    )
                    exit_code = evidence.exit_code
                    timed_out = evidence.timed_out
                    stdout_log = evidence.stdout_log
                    stderr_log = evidence.stderr_log
                    stdout_bytes = evidence.stdout_bytes_captured
                    stderr_bytes = evidence.stderr_bytes_captured
                    stdout_truncated = evidence.stdout_truncated
                    stderr_truncated = evidence.stderr_truncated
                    lock.update_child_pid(None)
                    if timed_out:
                        disposition = Disposition.TIMEOUT
                        error = "process timed out; SIGTERM/SIGKILL applied to process group"
                    elif exit_code == 0:
                        disposition = Disposition.SUCCESS
                    else:
                        disposition = Disposition.FAILED
                        error = f"child exited {exit_code}"

                    if manifest.action in {Action.CURSOR_TASK, Action.CODEX_TASK}:
                        ending_head = git_head(manifest.worktree)
                        changed = enforce_change_scope(
                            manifest, starting_head, ending_head
                        )
            else:
                raise BridgeError(f"unknown action: {manifest.action}")

            ending_head = git_head(manifest.worktree)

            # Scope accounting for writer actions even when UNSUPPORTED (HEAD must not move).
            if manifest.action in {Action.CURSOR_TASK, Action.CODEX_TASK}:
                if disposition == Disposition.UNSUPPORTED:
                    # Still prove allowCommit semantics against any accidental mutation.
                    try:
                        changed = enforce_change_scope(
                            manifest, starting_head, ending_head
                        )
                    except BridgeError as scope_exc:
                        disposition = Disposition.FAILED
                        error = str(scope_exc)
                        notes.append("scope enforcement failed after UNSUPPORTED writer action")

        except BridgeError as exc:
            error = str(exc)
            disposition = exc.disposition
            try:
                ending_head = git_head(manifest.worktree)
            except Exception:
                ending_head = starting_head
        except Exception as exc:  # pragma: no cover - defensive
            error = f"internal error: {exc}"
            disposition = Disposition.CRASH_RECOVERY
            _, bridge_log_truncated = append_bridge_log(
                self.logs,
                manifest.task_id,
                [traceback.format_exc()],
            )
            try:
                ending_head = git_head(manifest.worktree)
            except Exception:
                ending_head = starting_head
        finally:
            lock_lifecycle = dict(lock.lifecycle)
            if lock.acquired:
                lock.release()
                lock_lifecycle = dict(lock.lifecycle)

        ended = utc_now_iso()
        result = TaskResult(
            task_id=manifest.task_id,
            action=manifest.action.value,
            selected_tool=selected_tool,
            worktree=str(manifest.worktree),
            branch=branch_observed,
            starting_head=starting_head,
            ending_head=ending_head,
            authoritative_base=manifest.authoritative_base,
            command_argv=command_argv,
            started_at=started,
            ended_at=ended,
            exit_code=exit_code,
            timed_out=timed_out,
            stdout_log=stdout_log,
            stderr_log=stderr_log,
            changed_files=changed,
            committed_files=committed,
            commit_sha=commit_sha,
            push_status=push_status,
            pr_status=pr_status,
            lock_lifecycle=lock_lifecycle,
            disposition=disposition.value,
            error=error,
            probe=probe,
            notes=notes,
            stdout_bytes_captured=stdout_bytes,
            stderr_bytes_captured=stderr_bytes,
            stdout_truncated=stdout_truncated,
            stderr_truncated=stderr_truncated,
            bridge_log_truncated=bridge_log_truncated,
        )
        write_result(self.results, result)
        write_state(
            self.state,
            manifest.task_id,
            {
                "taskId": manifest.task_id,
                "disposition": disposition.value,
                "endedAt": ended,
                "endingHead": ending_head,
                "resultPath": str(self.results / f"{manifest.task_id}.result.json"),
            },
        )
        if processing_path.exists():
            try:
                processing_path.unlink()
            except OSError:
                notes.append("processing cleanup failed")
        return result

    def run_daemon(self, *, once: bool = False, poll_seconds: float = DAEMON_POLL_SECONDS) -> int:
        self.init_control_root()
        processed = 0
        while True:
            ready = self.list_inbox_ready()
            if ready:
                try:
                    self.process_one(ready[0])
                    processed += 1
                except BridgeError as exc:
                    append_bridge_log(
                        self.logs,
                        "_daemon",
                        [f"{utc_now_iso()} rejected: {exc}"],
                    )
            if once:
                break
            time.sleep(poll_seconds)
        return processed


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="amrudai-dot-control",
        description="AmrudAi Dot Host-Control Bridge v1 (user-space, no arbitrary shell)",
    )
    parser.add_argument(
        "--control-root",
        type=Path,
        default=DEFAULT_CONTROL_ROOT,
        help=f"control root (default: {DEFAULT_CONTROL_ROOT})",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("init", help="create control-root subdirectories")

    p_one = sub.add_parser("oneshot", help="process exactly one inbox manifest and exit")
    p_one.add_argument("--manifest", type=Path, help="optional explicit processing/inbox path")

    p_daemon = sub.add_parser("daemon", help="poll inbox with bounded sleep")
    p_daemon.add_argument("--poll", type=float, default=DAEMON_POLL_SECONDS)
    p_daemon.add_argument("--once", action="store_true", help="single poll cycle (testing)")

    p_val = sub.add_parser("validate", help="validate manifest and print dry-run plan")
    p_val.add_argument("manifest", type=Path)

    p_lock = sub.add_parser("inspect-lock", help="inspect lock record for a worktree")
    p_lock.add_argument("worktree", type=Path)

    return parser


def main(argv: Optional[list[str]] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    bridge = DotControlBridge(control_root=args.control_root)

    try:
        if args.command == "init":
            created = bridge.init_control_root()
            print(json.dumps({"created": created}, indent=2))
            return 0

        if args.command == "validate":
            plan = bridge.validate_only(args.manifest)
            print(json.dumps(plan, indent=2, sort_keys=True))
            return 0

        if args.command == "inspect-lock":
            print(json.dumps(inspect_lock(bridge.locks, args.worktree), indent=2, sort_keys=True))
            return 0

        if args.command == "oneshot":
            bridge.init_control_root()
            result = bridge.process_one(args.manifest)
            print(json.dumps(result.to_dict(), indent=2, sort_keys=True))
            return 0 if result.disposition == Disposition.SUCCESS.value else 1

        if args.command == "daemon":
            bridge.run_daemon(once=args.once, poll_seconds=args.poll)
            return 0

        parser.error(f"unknown command {args.command}")
        return 2
    except BridgeError as exc:
        print(json.dumps({"error": str(exc), "disposition": exc.disposition.value}), file=sys.stderr)
        return 1


if __name__ == "__main__":
    if __package__ is None or __package__ == "":
        sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
        __package__ = "dot_control"
        from dot_control.bridge import main as _main

        raise SystemExit(_main())
    raise SystemExit(main())

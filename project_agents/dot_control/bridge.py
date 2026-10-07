#!/usr/bin/env python3
"""AmrudAi Dot Host-Control Bridge — CLI and manifest lifecycle.

Runs as a Linux user-space process (no sudo/root). Consumes validated task
manifests from the control root inbox and executes allowlisted actions only.
"""

from __future__ import annotations

import argparse
import json
import os
import re
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
    git_inspect_for_recovery,
    live_writer_unsupported,
    run_process,
    verify_git_preconditions,
)
from .locks import (
    FinalizationBarrier,
    TaskIdLock,
    WorktreeLock,
    inspect_lock,
    recover_finalization_barrier,
)
from .models import (
    DAEMON_POLL_SECONDS,
    DEFAULT_CONTROL_ROOT,
    FILE_MODE,
    Action,
    BridgeError,
    Disposition,
    TaskManifest,
    TaskResult,
)
from .paths import (
    assert_durable_file_mode,
    init_control_root,
    validate_control_root_layout,
)
from .validator import (
    PathPolicy,
    load_json_object,
    normalize_plan,
    validate_manifest_dict,
)

_TASK_ID_FILE_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.json$")


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

    def assert_approved_manifest_path(self, manifest_path: Path) -> Path:
        """Accept only exact control-root inbox or processing paths."""
        path = Path(manifest_path)
        if path.is_symlink() or os.path.islink(path):
            raise BridgeError(f"manifest must not be a symlink: {path}")
        try:
            resolved = path.resolve(strict=True)
        except OSError as exc:
            raise BridgeError(f"manifest path unreadable: {path}: {exc}") from exc
        inbox_r = self.inbox.resolve(strict=False)
        processing_r = self.processing.resolve(strict=False)
        parent = resolved.parent
        if parent != inbox_r and parent != processing_r:
            raise BridgeError(
                f"manifest path must be under exact control-root inbox or processing; "
                f"got {path} (parent={parent})"
            )
        if not _TASK_ID_FILE_RE.match(resolved.name):
            raise BridgeError(f"manifest filename must be <taskId>.json; got {resolved.name}")
        return resolved

    # --- lifecycle ----------------------------------------------------------

    def list_inbox_ready(self) -> list[Path]:
        if not self.inbox.is_dir() or self.inbox.is_symlink():
            return []
        return sorted(
            p
            for p in self.inbox.glob("*.json")
            if p.is_file() and not p.is_symlink() and not p.name.endswith(".tmp")
        )

    def task_id_known(self, task_id: str) -> bool:
        candidates = [
            self.inbox / f"{task_id}.json",
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
        try:
            os.chmod(dest, FILE_MODE)
        except OSError:
            pass
        assert_durable_file_mode(dest, "quarantined manifest")
        meta = dest.with_suffix(dest.suffix + ".reason.json")
        atomic_write_json(meta, {"reason": reason, "at": utc_now_iso(), "source": str(path)})
        assert_durable_file_mode(meta, "quarantine reason")
        return dest

    def claim_manifest(self, inbox_path: Path, task_id: str) -> Path:
        """Atomically claim inbox/<taskId>.json → processing/<taskId>.json without overwrite."""
        if self.processing.is_symlink():
            raise BridgeError(f"processing must not be a symlink: {self.processing}")
        expected_name = f"{task_id}.json"
        if inbox_path.name != expected_name:
            raise BridgeError(
                f"inbox filename must be exactly {expected_name}; got {inbox_path.name}"
            )
        assert_durable_file_mode(inbox_path, "inbox manifest")
        if inbox_path.parent.resolve() != self.inbox.resolve():
            raise BridgeError("claim requires exact inbox parent")
        self.processing.mkdir(parents=True, exist_ok=True)
        dest = self.processing / expected_name
        if dest.exists():
            raise BridgeError(f"duplicate processing entry: {dest.name}")
        # Exclusive create of destination before rename to prevent overwrite races.
        flags = os.O_CREAT | os.O_EXCL | os.O_WRONLY | os.O_CLOEXEC | os.O_NOFOLLOW
        try:
            fd = os.open(str(dest), flags, FILE_MODE)
            os.close(fd)
        except FileExistsError as exc:
            raise BridgeError(f"duplicate processing entry: {dest.name}") from exc
        try:
            os.replace(inbox_path, dest)
        except Exception:
            try:
                dest.unlink(missing_ok=True)  # type: ignore[call-arg]
            except TypeError:
                if dest.exists():
                    dest.unlink()
            raise
            # Revalidate after atomic claim (no silent mode repair).
        assert_durable_file_mode(dest, "processing manifest")
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
            plan["notes"] = [
                "child process not launched",
                "dry-run / planning preview only",
            ]
        return plan

    def process_one(self, manifest_path: Optional[Path] = None) -> TaskResult:
        validate_control_root_layout(self.control_root, require_subdirs=True, check_modes=True)
        if manifest_path is None:
            ready = self.list_inbox_ready()
            if not ready:
                raise BridgeError("inbox empty", disposition=Disposition.REJECTED)
            manifest_path = ready[0]

        manifest_path = self.assert_approved_manifest_path(Path(manifest_path))
        if manifest_path.suffix != ".json":
            raise BridgeError("partial manifest ignored until atomic rename to .json")
        if manifest_path.name.endswith(".tmp"):
            raise BridgeError("partial .tmp manifest ignored")

        processing_path: Optional[Path] = None
        task_id_lock: Optional[TaskIdLock] = None
        owns_task_id_lock = False
        task_id_preview = manifest_path.stem

        try:
            # Admit by filename before any mutation; TaskIdLock losers must not touch the file.
            if not _TASK_ID_FILE_RE.match(manifest_path.name):
                raise BridgeError(f"invalid manifest filename: {manifest_path.name}")

            task_id_lock = TaskIdLock(self.locks, task_id_preview)
            try:
                task_id_lock.acquire()
            except BridgeError as lock_exc:
                if lock_exc.disposition == Disposition.LOCKED:
                    # Loser: LOCKED/retry evidence only — never mutate the manifest.
                    raise
                raise
            owns_task_id_lock = True

            # Revalidate path/filename/taskId under the lock before claim.
            manifest_path = self.assert_approved_manifest_path(manifest_path)
            if manifest_path.stem != task_id_preview or manifest_path.name != f"{task_id_preview}.json":
                raise BridgeError("taskId/filename revalidation failed after TaskIdLock")

            assert_durable_file_mode(manifest_path, "admission manifest")

            data = load_json_object(manifest_path)
            body_task_id = data.get("taskId")
            if not isinstance(body_task_id, str) or not body_task_id.strip():
                raise BridgeError("missing or invalid taskId before admission")
            if body_task_id != task_id_preview:
                raise BridgeError(
                    f"inbox/processing filename must equal taskId.json "
                    f"({task_id_preview}.json); body taskId={body_task_id}"
                )

            expected_name = f"{task_id_preview}.json"
            for label, path in (
                ("processing", self.processing / expected_name),
                ("results", self.results / f"{task_id_preview}.result.json"),
                ("state", self.state / f"{task_id_preview}.json"),
                ("quarantine", self.quarantine / expected_name),
            ):
                if path.exists() and path.resolve() != manifest_path.resolve():
                    raise BridgeError(f"duplicate taskId in {label}: {task_id_preview}")

            if manifest_path.parent.resolve() == self.inbox.resolve():
                processing_path = self.claim_manifest(manifest_path, task_id_preview)
            else:
                processing_path = manifest_path
                if processing_path.name != expected_name:
                    raise BridgeError(
                        f"processing filename must equal {expected_name}"
                    )
                assert_durable_file_mode(processing_path, "processing manifest")

            manifest = validate_manifest_dict(data, policy=self.policy, require_paths_exist=True)
            if manifest.task_id != task_id_preview:
                raise BridgeError("taskId mutated between admission and validation")
        except BridgeError as exc:
            # Only the TaskIdLock owner may claim/quarantine/mutate the manifest.
            if owns_task_id_lock:
                try:
                    if processing_path is not None and processing_path.exists():
                        self.quarantine_manifest(processing_path, str(exc))
                    elif (
                        manifest_path.exists()
                        and manifest_path.parent.resolve() == self.inbox.resolve()
                    ):
                        self.quarantine_manifest(manifest_path, str(exc))
                except BridgeError:
                    pass
            if task_id_lock is not None and task_id_lock.acquired:
                task_id_lock.release()
            raise

        try:
            return self._execute_manifest(manifest, processing_path)
        finally:
            if task_id_lock is not None and task_id_lock.acquired:
                task_id_lock.release()

    def _execute_manifest(self, manifest: TaskManifest, processing_path: Path) -> TaskResult:
        started = utc_now_iso()
        lock = WorktreeLock(self.locks, manifest.worktree)
        barrier = FinalizationBarrier(self.locks, manifest.worktree)
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

        provisional_written = False
        barrier_written = False
        result: Optional[TaskResult] = None

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
                    command_argv = redact_argv(argv) if argv else []
                    disposition = Disposition.UNSUPPORTED
                    error = live_block
                    notes.append(f"UNSUPPORTED: {error}")
                    notes.append("child process not launched")
                    if manifest.action == Action.TEST_PROFILE:
                        notes.append(
                            "TEST_PROFILE dry-run preview only; no capability-pack/"
                            "agent/static-contract evidence was executed"
                        )
                    exit_code = None
                elif meta.get("unsupported"):
                    disposition = Disposition.UNSUPPORTED
                    error = meta.get("reason")
                    command_argv = []
                    notes.append(f"UNSUPPORTED: {error}")
                else:
                    command_argv = redact_argv(argv)
                    disposition = Disposition.UNSUPPORTED
                    error = "v1 live child execution path disabled"
                    notes.append("child process not launched")
                    exit_code = None

                if manifest.action in {Action.CURSOR_TASK, Action.CODEX_TASK}:
                    ending_head = git_head(manifest.worktree)
                    try:
                        changed = enforce_change_scope(
                            manifest, starting_head, ending_head
                        )
                    except BridgeError as scope_exc:
                        disposition = Disposition.FAILED
                        error = str(scope_exc)
                        notes.append("scope enforcement failed after UNSUPPORTED writer action")
            else:
                raise BridgeError(f"unknown action: {manifest.action}")

            ending_head = git_head(manifest.worktree)

            if manifest.action in {Action.CURSOR_TASK, Action.CODEX_TASK}:
                if disposition == Disposition.UNSUPPORTED:
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

        # --- Durable finalization barrier lifecycle ------------------------
        # acquire flock → execute → provisional result/state → barrier →
        # release flock → final released result/state → remove barrier
        ended = utc_now_iso()
        lock_lifecycle = dict(lock.lifecycle)
        lock_lifecycle["evidencePending"] = True
        lock_lifecycle["released"] = False
        lock_lifecycle["finalizationBarrier"] = str(barrier.path)

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

        def _write_barrier(evidence_status: str, *, recovery_required: bool) -> None:
            nonlocal barrier_written
            barrier.write(
                task_id=manifest.task_id,
                expected_head=manifest.expected_head,
                ending_head=ending_head or starting_head or manifest.expected_head,
                evidence_status=evidence_status,
                recovery_required=recovery_required,
                bridge_pid=os.getpid(),
            )
            barrier_written = True
            lock_lifecycle["finalizationBarrierWritten"] = True
            lock_lifecycle["evidenceStatus"] = evidence_status

        # 1) Provisional result/state while flock held.
        try:
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
                    "lockHeld": True,
                    "evidencePending": True,
                    "finalizationBarrier": str(barrier.path),
                },
            )
            provisional_written = True
            lock_lifecycle["evidencePending"] = True
            lock_lifecycle["provisionalDurable"] = True
        except Exception as write_exc:
            disposition = Disposition.CRASH_RECOVERY
            error = f"provisional evidence write failed while lock held: {write_exc}"
            notes.append("provisional evidence-write failure")
            result.disposition = disposition.value
            result.error = error
            result.notes = notes
            # Fail closed: durable barrier before any flock release.
            try:
                _write_barrier("provisional_write_failed", recovery_required=True)
            except Exception as barrier_exc:
                notes.append(f"finalization barrier write also failed: {barrier_exc}")
                result.notes = notes
                # Last resort: keep flock if still held (one-shot exit still frees OS flock;
                # barrier is the durable mechanism — residual if both fail).
                lock_lifecycle["released"] = False
                lock_lifecycle["retainedForForensics"] = True
                result.lock_lifecycle = dict(lock_lifecycle)
                if processing_path.exists():
                    try:
                        processing_path.unlink()
                    except OSError:
                        pass
                return result

        # 2) Write/fsync finalization barrier while flock still held.
        if not barrier_written:
            try:
                _write_barrier("provisional_durable", recovery_required=True)
            except Exception as barrier_exc:
                disposition = Disposition.CRASH_RECOVERY
                error = f"finalization barrier write failed: {barrier_exc}"
                notes.append(str(error))
                result.disposition = disposition.value
                result.error = error
                result.notes = notes
                result.lock_lifecycle = dict(lock_lifecycle)
                if processing_path.exists():
                    try:
                        processing_path.unlink()
                    except OSError:
                        pass
                return result

        # 3) Release flock only after provisional evidence + barrier are durable.
        if lock.acquired:
            lock.release()
            lock_lifecycle = dict(lock.lifecycle)
            lock_lifecycle["finalizationBarrier"] = str(barrier.path)
            lock_lifecycle["finalizationBarrierWritten"] = True
            lock_lifecycle["provisionalDurable"] = provisional_written
            lock_lifecycle["released"] = True
            lock_lifecycle["evidencePending"] = True
            result.lock_lifecycle = lock_lifecycle

        # 4) Publish final released result/state; on failure leave barrier in place.
        final_ok = False
        try:
            lock_lifecycle["evidencePending"] = False
            lock_lifecycle["evidenceDurable"] = True
            lock_lifecycle["released"] = True
            result.lock_lifecycle = lock_lifecycle
            if result.disposition == Disposition.SUCCESS.value and not provisional_written:
                result.disposition = Disposition.CRASH_RECOVERY.value
            write_result(self.results, result)
            write_state(
                self.state,
                manifest.task_id,
                {
                    "taskId": manifest.task_id,
                    "disposition": result.disposition,
                    "endedAt": ended,
                    "endingHead": ending_head,
                    "resultPath": str(self.results / f"{manifest.task_id}.result.json"),
                    "lockHeld": False,
                    "evidencePending": False,
                    "lockReleased": True,
                    "finalizationBarrier": str(barrier.path),
                },
            )
            final_ok = True
        except Exception as final_exc:
            notes.append(f"final evidence write failed after lock release: {final_exc}")
            result.notes = notes
            if result.disposition == Disposition.SUCCESS.value:
                result.disposition = Disposition.PARTIAL.value
            else:
                result.disposition = Disposition.CRASH_RECOVERY.value
            result.error = (
                (result.error + "; " if result.error else "")
                + f"final evidence update failed: {final_exc}"
            )
            try:
                write_result(self.results, result)
            except Exception:
                pass
            # Leave barrier in place — next task blocked; manual recovery required.
            lock_lifecycle["evidenceDurable"] = False
            lock_lifecycle["finalEvidenceFailed"] = True
            result.lock_lifecycle = lock_lifecycle
            if processing_path.exists():
                try:
                    processing_path.unlink()
                except OSError:
                    notes.append("processing cleanup failed")
                    result.notes = notes
            return result

        # 5) Remove finalization barrier atomically only after final evidence success.
        if final_ok:
            try:
                barrier.remove_atomic()
                lock_lifecycle["finalizationBarrierCleared"] = True
                lock_lifecycle["evidencePending"] = False
                result.lock_lifecycle = lock_lifecycle
                # Refresh result/state with cleared-barrier truth (best effort).
                try:
                    write_result(self.results, result)
                    write_state(
                        self.state,
                        manifest.task_id,
                        {
                            "taskId": manifest.task_id,
                            "disposition": result.disposition,
                            "endedAt": ended,
                            "endingHead": ending_head,
                            "resultPath": str(
                                self.results / f"{manifest.task_id}.result.json"
                            ),
                            "lockHeld": False,
                            "evidencePending": False,
                            "lockReleased": True,
                            "finalizationBarrierCleared": True,
                        },
                    )
                except Exception as refresh_exc:
                    # Barrier already cleared; provisional+final evidence exist.
                    notes.append(f"post-clear lifecycle refresh failed: {refresh_exc}")
                    result.notes = notes
            except Exception as clear_exc:
                notes.append(f"barrier clear failed: {clear_exc}")
                result.notes = notes
                if result.disposition == Disposition.SUCCESS.value:
                    result.disposition = Disposition.PARTIAL.value
                result.error = (
                    (result.error + "; " if result.error else "")
                    + f"finalization barrier clear failed: {clear_exc}"
                )
                # Barrier still present → blocks next task (fail closed).
                try:
                    write_result(self.results, result)
                except Exception:
                    pass

        if processing_path.exists():
            try:
                processing_path.unlink()
            except OSError:
                notes.append("processing cleanup failed")
                result.notes = notes
        return result

    def recover_worktree_finalization(
        self,
        worktree: Path,
        *,
        confirm_task_id: str,
        action: str = "clear",
    ) -> dict[str, Any]:
        """Explicit recovery: clear finalization barrier after inspection gates."""
        validate_control_root_layout(self.control_root, require_subdirs=True, check_modes=True)
        git_inspect = git_inspect_for_recovery(Path(worktree))
        return recover_finalization_barrier(
            self.locks,
            Path(worktree),
            action=action,
            confirm_task_id=confirm_task_id,
            git_inspect=git_inspect,
        )

    def run_daemon(self, *, once: bool = False, poll_seconds: float = DAEMON_POLL_SECONDS) -> int:
        # Normal operation validates; does not repair modes (explicit init only).
        validate_control_root_layout(self.control_root, require_subdirs=True, check_modes=True)
        processed = 0
        while True:
            ready = self.list_inbox_ready()
            if ready:
                try:
                    self.process_one(ready[0])
                    processed += 1
                except BridgeError as exc:
                    # LOCKED losers must not mutate; log only.
                    append_bridge_log(
                        self.logs,
                        "_daemon",
                        [f"{utc_now_iso()} rejected: {exc} disposition={exc.disposition.value}"],
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
    p_one.add_argument(
        "--manifest",
        type=Path,
        help="optional explicit path under control-root inbox/ or processing/",
    )

    p_daemon = sub.add_parser("daemon", help="poll inbox with bounded sleep")
    p_daemon.add_argument("--poll", type=float, default=DAEMON_POLL_SECONDS)
    p_daemon.add_argument("--once", action="store_true", help="single poll cycle (testing)")

    p_val = sub.add_parser("validate", help="validate manifest and print dry-run plan")
    p_val.add_argument("manifest", type=Path)

    p_lock = sub.add_parser("inspect-lock", help="inspect lock/barrier record for a worktree")
    p_lock.add_argument("worktree", type=Path)

    p_rec = sub.add_parser(
        "recover-finalization",
        help="explicit operator clear of a durable worktree finalization barrier",
    )
    p_rec.add_argument("worktree", type=Path)
    p_rec.add_argument(
        "--confirm-task-id",
        required=True,
        help="predecessor taskId recorded in the barrier (explicit confirmation)",
    )
    p_rec.add_argument(
        "--action",
        default="clear",
        choices=["clear"],
        help="recovery action (only clear is supported)",
    )

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

        if args.command == "recover-finalization":
            out = bridge.recover_worktree_finalization(
                args.worktree,
                confirm_task_id=args.confirm_task_id,
                action=args.action,
            )
            print(json.dumps(out, indent=2, sort_keys=True))
            return 0 if out.get("cleared") else 1

        if args.command == "oneshot":
            # Require prior explicit init; do not silently repair modes.
            validate_control_root_layout(
                bridge.control_root, require_subdirs=True, check_modes=True
            )
            result = bridge.process_one(args.manifest)
            print(json.dumps(result.to_dict(), indent=2, sort_keys=True))
            return 0 if result.disposition == Disposition.SUCCESS.value else 1

        if args.command == "daemon":
            bridge.run_daemon(once=args.once, poll_seconds=args.poll)
            return 0

        parser.error(f"unknown command {args.command}")
        return 2
    except BridgeError as exc:
        print(
            json.dumps(
                {
                    "error": str(exc),
                    "disposition": exc.disposition.value,
                }
            ),
            file=sys.stderr,
        )
        return 1


if __name__ == "__main__":
    if __package__ is None or __package__ == "":
        sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
        __package__ = "dot_control"
        from dot_control.bridge import main as _main

        raise SystemExit(_main())
    raise SystemExit(main())

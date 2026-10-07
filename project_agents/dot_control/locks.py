"""Exclusive per-worktree file locks and durable finalization barriers."""

from __future__ import annotations

import hashlib
import json
import os
import stat
import time
from pathlib import Path
from typing import Any, Optional

from .evidence import utc_now_iso
from .models import FILE_MODE, BridgeError, Disposition, LockRecord

try:
    import fcntl
except ImportError:  # pragma: no cover - Linux-only bridge
    fcntl = None  # type: ignore


def worktree_lock_name(worktree: Path) -> str:
    digest = hashlib.sha256(str(worktree.resolve()).encode("utf-8")).hexdigest()[:16]
    safe = re_safe(worktree.name)
    return f"{safe}-{digest}.lock"


def finalization_barrier_name(worktree: Path) -> str:
    digest = hashlib.sha256(str(worktree.resolve()).encode("utf-8")).hexdigest()[:16]
    safe = re_safe(worktree.name)
    return f"finalize-{safe}-{digest}.barrier"


def re_safe(name: str) -> str:
    return "".join(ch if ch.isalnum() or ch in "-_." else "_" for ch in name)[:64]


def pid_alive(pid: Optional[int]) -> bool:
    if pid is None or pid <= 0:
        return False
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def _assert_lock_file_openable(path: Path, label: str) -> bool:
    """Return True if path already exists. Reject unsafe pre-existing modes (no repair)."""
    if path.is_symlink() or os.path.islink(path):
        raise BridgeError(f"{label} must not be a symlink: {path}", disposition=Disposition.FAILED)
    existed = os.path.lexists(path)
    if existed:
        if not path.is_file():
            raise BridgeError(f"{label} is not a regular file: {path}", disposition=Disposition.FAILED)
        st = path.stat()
        if not stat.S_ISREG(st.st_mode):
            raise BridgeError(f"{label} is not a regular file: {path}", disposition=Disposition.FAILED)
        mode = stat.S_IMODE(st.st_mode)
        if mode != FILE_MODE:
            raise BridgeError(
                f"{label} mode {oct(mode)} != required {oct(FILE_MODE)}; "
                "explicit recovery/init required (no silent repair)",
                disposition=Disposition.FAILED,
            )
        if st.st_uid != os.geteuid():
            raise BridgeError(
                f"{label} owner uid {st.st_uid} != bridge uid {os.geteuid()}",
                disposition=Disposition.FAILED,
            )
    return existed


class FinalizationBarrier:
    """Durable per-worktree finalization marker under the locks/ directory.

    Survives one-shot process exit. While present, WorktreeLock.acquire rejects
    new tasks even when the previous flock/PID is gone.
    """

    def __init__(self, locks_dir: Path, worktree: Path):
        self.locks_dir = locks_dir
        self.worktree = worktree.resolve()
        self.path = locks_dir / finalization_barrier_name(self.worktree)

    def exists(self) -> bool:
        return self.path.exists() and not self.path.is_symlink()

    def read(self) -> Optional[dict[str, Any]]:
        if not self.exists():
            return None
        try:
            return json.loads(self.path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError, TypeError, ValueError):
            return {
                "taskId": None,
                "worktree": str(self.worktree),
                "evidenceStatus": "unreadable",
                "recoveryRequired": True,
                "error": "barrier unreadable",
            }

    def write(
        self,
        *,
        task_id: str,
        expected_head: str,
        ending_head: str,
        evidence_status: str,
        recovery_required: bool = True,
        bridge_pid: Optional[int] = None,
    ) -> Path:
        if self.locks_dir.is_symlink():
            raise BridgeError(
                f"locks directory must not be a symlink: {self.locks_dir}",
                disposition=Disposition.FAILED,
            )
        self.locks_dir.mkdir(parents=True, exist_ok=True)
        if self.path.is_symlink():
            raise BridgeError(
                f"finalization barrier must not be a symlink: {self.path}",
                disposition=Disposition.FAILED,
            )
        payload = {
            "taskId": task_id,
            "predecessorTaskId": task_id,
            "worktree": str(self.worktree),
            "expectedHead": expected_head,
            "endingHead": ending_head,
            "evidenceStatus": evidence_status,
            "createdAt": utc_now_iso(),
            "recoveryRequired": recovery_required,
            "bridgePid": bridge_pid if bridge_pid is not None else os.getpid(),
        }
        data = json.dumps(payload, indent=2, sort_keys=True) + "\n"
        flags = os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_CLOEXEC | os.O_NOFOLLOW
        fd = os.open(str(self.path), flags, FILE_MODE)
        try:
            os.fchmod(fd, FILE_MODE)
            with os.fdopen(fd, "w", encoding="utf-8") as fh:
                fh.write(data)
                fh.flush()
                os.fsync(fh.fileno())
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            raise
        # Fsync directory entry for durability of the create/rename-like write.
        try:
            dir_fd = os.open(str(self.locks_dir), os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
            try:
                os.fsync(dir_fd)
            finally:
                os.close(dir_fd)
        except OSError:
            pass
        return self.path

    def remove_atomic(self) -> None:
        """Remove barrier only via rename-away then unlink (no follow)."""
        if not os.path.lexists(self.path):
            return
        if self.path.is_symlink() or os.path.islink(self.path):
            raise BridgeError(
                f"refusing to clear symlink barrier: {self.path}",
                disposition=Disposition.FAILED,
            )
        tmp = self.path.with_suffix(self.path.suffix + ".cleared")
        if tmp.exists() or tmp.is_symlink():
            try:
                tmp.unlink()
            except OSError:
                pass
        os.rename(self.path, tmp)
        try:
            tmp.unlink()
        except OSError:
            pass
        try:
            dir_fd = os.open(str(self.locks_dir), os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
            try:
                os.fsync(dir_fd)
            finally:
                os.close(dir_fd)
        except OSError:
            pass


def inspect_finalization_barrier(locks_dir: Path, worktree: Path) -> dict[str, Any]:
    barrier = FinalizationBarrier(locks_dir, worktree)
    result: dict[str, Any] = {
        "barrierPath": str(barrier.path),
        "barrierExists": barrier.exists(),
        "barrierInspectedAt": utc_now_iso(),
    }
    data = barrier.read()
    if data is not None:
        result["barrier"] = data
        pred_pid = data.get("bridgePid")
        result["predecessorPidAlive"] = pid_alive(
            int(pred_pid) if pred_pid not in (None, "", 0, "0") else None
        )
    return result


class WorktreeLock:
    """One exclusive lock per canonical worktree path."""

    def __init__(self, locks_dir: Path, worktree: Path):
        if fcntl is None:
            raise BridgeError("fcntl.flock unavailable; Linux required", disposition=Disposition.FAILED)
        self.locks_dir = locks_dir
        self.worktree = worktree.resolve()
        self.path = locks_dir / worktree_lock_name(self.worktree)
        self.barrier = FinalizationBarrier(locks_dir, self.worktree)
        self._fh: Any = None
        self.record: Optional[LockRecord] = None
        self.acquired = False
        self.lifecycle: dict[str, Any] = {
            "lockPath": str(self.path),
            "barrierPath": str(self.barrier.path),
            "acquired": False,
            "released": False,
            "staleReclaimed": False,
            "liveConflict": False,
            "finalizationBarrierBlocked": False,
        }

    def acquire(
        self,
        *,
        task_id: str,
        action: str,
        writer_tool: str,
        branch: str,
        expected_head: str,
        allow_stale_reclaim: bool = True,
    ) -> LockRecord:
        if self.locks_dir.is_symlink() or (
            self.locks_dir.exists() and self.locks_dir.is_symlink()
        ):
            raise BridgeError(
                f"locks directory must not be a symlink: {self.locks_dir}",
                disposition=Disposition.FAILED,
            )
        # Durable finalization barrier blocks even when previous flock/PID is gone.
        if self.barrier.exists():
            existing_barrier = self.barrier.read() or {}
            self.lifecycle["finalizationBarrierBlocked"] = True
            self.lifecycle["existingBarrier"] = existing_barrier
            raise BridgeError(
                f"unresolved finalization barrier for worktree {self.worktree}: "
                f"predecessor={existing_barrier.get('predecessorTaskId') or existing_barrier.get('taskId')}; "
                f"evidenceStatus={existing_barrier.get('evidenceStatus')}; "
                "manual recovery/inspection required",
                disposition=Disposition.LOCKED,
            )

        existed = _assert_lock_file_openable(self.path, "worktree lock")
        self.locks_dir.mkdir(parents=True, exist_ok=True)
        flags = os.O_RDWR | os.O_CREAT | os.O_CLOEXEC | os.O_NOFOLLOW
        fd = os.open(str(self.path), flags, FILE_MODE)
        try:
            st = os.fstat(fd)
            if not stat.S_ISREG(st.st_mode):
                os.close(fd)
                raise BridgeError(
                    f"lock path is not a regular file: {self.path}",
                    disposition=Disposition.FAILED,
                )
            mode = stat.S_IMODE(st.st_mode)
            if existed and mode != FILE_MODE:
                os.close(fd)
                raise BridgeError(
                    f"worktree lock mode {oct(mode)} != required {oct(FILE_MODE)}; "
                    "explicit recovery required (no silent repair)",
                    disposition=Disposition.FAILED,
                )
            if not existed:
                os.fchmod(fd, FILE_MODE)
            self._fh = os.fdopen(fd, "a+", encoding="utf-8")
        except BridgeError:
            raise
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            raise
        try:
            fcntl.flock(self._fh.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            existing = self._read_unlocked_snapshot()
            self.lifecycle["liveConflict"] = True
            self.lifecycle["existing"] = existing.to_wire() if existing else None
            self._fh.close()
            self._fh = None
            raise BridgeError(
                f"live lock held for worktree {self.worktree}: {existing}",
                disposition=Disposition.LOCKED,
            ) from exc

        # Re-check barrier after acquiring flock (race with predecessor finalization).
        if self.barrier.exists():
            existing_barrier = self.barrier.read() or {}
            fcntl.flock(self._fh.fileno(), fcntl.LOCK_UN)
            self._fh.close()
            self._fh = None
            self.lifecycle["finalizationBarrierBlocked"] = True
            self.lifecycle["existingBarrier"] = existing_barrier
            raise BridgeError(
                f"unresolved finalization barrier for worktree {self.worktree}",
                disposition=Disposition.LOCKED,
            )

        existing = self._read_from_fh()
        if existing is not None:
            bridge_live = pid_alive(existing.bridge_pid)
            child_live = pid_alive(existing.child_pid)
            if bridge_live or child_live:
                fcntl.flock(self._fh.fileno(), fcntl.LOCK_UN)
                self._fh.close()
                self._fh = None
                self.lifecycle["liveConflict"] = True
                self.lifecycle["existing"] = existing.to_wire()
                raise BridgeError(
                    f"live lock PIDs still active for {self.worktree} "
                    f"(bridge={existing.bridge_pid} child={existing.child_pid})",
                    disposition=Disposition.LOCKED,
                )
            if not allow_stale_reclaim:
                fcntl.flock(self._fh.fileno(), fcntl.LOCK_UN)
                self._fh.close()
                self._fh = None
                raise BridgeError(
                    f"stale lock present for {self.worktree}; reclaim not allowed",
                    disposition=Disposition.LOCKED,
                )
            self.lifecycle["staleReclaimed"] = True
            self.lifecycle["stale"] = existing.to_wire()

        now = utc_now_iso()
        record = LockRecord(
            task_id=task_id,
            action=action,
            writer_tool=writer_tool,
            worktree=str(self.worktree),
            branch=branch,
            expected_head=expected_head,
            bridge_pid=os.getpid(),
            child_pid=None,
            created_at=now,
            heartbeat_at=now,
        )
        self._write_record(record)
        self.record = record
        self.acquired = True
        self.lifecycle["acquired"] = True
        self.lifecycle["record"] = record.to_wire()
        return record

    def update_child_pid(self, child_pid: Optional[int]) -> None:
        if not self.acquired or self.record is None:
            return
        self.record.child_pid = child_pid
        self.record.heartbeat_at = utc_now_iso()
        self._write_record(self.record)
        self.lifecycle["record"] = self.record.to_wire()

    def heartbeat(self) -> None:
        if not self.acquired or self.record is None:
            return
        self.record.heartbeat_at = utc_now_iso()
        self._write_record(self.record)

    def release(self) -> None:
        if self._fh is None:
            return
        try:
            # Preserve lock file contents for forensics; clear PIDs to mark released.
            if self.record is not None:
                released = LockRecord(
                    task_id=self.record.task_id,
                    action=self.record.action,
                    writer_tool=self.record.writer_tool,
                    worktree=self.record.worktree,
                    branch=self.record.branch,
                    expected_head=self.record.expected_head,
                    bridge_pid=0,
                    child_pid=None,
                    created_at=self.record.created_at,
                    heartbeat_at=utc_now_iso(),
                )
                self._write_record(released)
                self.lifecycle["releasedRecord"] = released.to_wire()
            fcntl.flock(self._fh.fileno(), fcntl.LOCK_UN)
        finally:
            self._fh.close()
            self._fh = None
            self.acquired = False
            self.lifecycle["released"] = True

    def _write_record(self, record: LockRecord) -> None:
        assert self._fh is not None
        self._fh.seek(0)
        self._fh.truncate()
        payload = json.dumps(record.to_wire(), indent=2, sort_keys=True) + "\n"
        self._fh.write(payload)
        self._fh.flush()
        os.fsync(self._fh.fileno())

    def _read_from_fh(self) -> Optional[LockRecord]:
        assert self._fh is not None
        self._fh.seek(0)
        raw = self._fh.read().strip()
        if not raw:
            return None
        try:
            return LockRecord.from_dict(json.loads(raw))
        except (json.JSONDecodeError, KeyError, TypeError, ValueError):
            return None

    def _read_unlocked_snapshot(self) -> Optional[LockRecord]:
        if not self.path.exists():
            return None
        try:
            data = json.loads(self.path.read_text(encoding="utf-8"))
            return LockRecord.from_dict(data)
        except (OSError, json.JSONDecodeError, KeyError, TypeError, ValueError):
            return None


def inspect_lock(locks_dir: Path, worktree: Path) -> dict[str, Any]:
    path = locks_dir / worktree_lock_name(worktree.resolve())
    result: dict[str, Any] = {"lockPath": str(path), "exists": path.exists()}
    if path.exists():
        try:
            record = LockRecord.from_dict(json.loads(path.read_text(encoding="utf-8")))
            result["record"] = record.to_wire()
            result["bridgePidAlive"] = pid_alive(record.bridge_pid)
            result["childPidAlive"] = pid_alive(record.child_pid)
            result["stale"] = not (pid_alive(record.bridge_pid) or pid_alive(record.child_pid))
            result["mtime"] = time.ctime(path.stat().st_mtime)
        except (OSError, json.JSONDecodeError, KeyError, TypeError, ValueError) as exc:
            result["error"] = str(exc)
    result["inspectedAt"] = utc_now_iso()
    result.update(inspect_finalization_barrier(locks_dir, worktree))
    return result


def recover_finalization_barrier(
    locks_dir: Path,
    worktree: Path,
    *,
    action: str,
    confirm_task_id: str,
    git_inspect: Optional[dict[str, Any]] = None,
) -> dict[str, Any]:
    """Explicit operator recovery: clear barrier only after inspection gates pass."""
    if action != "clear":
        raise BridgeError(
            f"unsupported recovery action: {action}; only 'clear' is allowed",
            disposition=Disposition.REJECTED,
        )
    barrier = FinalizationBarrier(locks_dir, worktree)
    if not barrier.exists():
        return {"cleared": False, "reason": "no barrier present", "worktree": str(worktree.resolve())}

    data = barrier.read() or {}
    pred_id = str(data.get("predecessorTaskId") or data.get("taskId") or "")
    if confirm_task_id != pred_id:
        raise BridgeError(
            f"confirm taskId {confirm_task_id!r} does not match barrier predecessor {pred_id!r}",
            disposition=Disposition.REJECTED,
        )

    pred_pid = data.get("bridgePid")
    try:
        pred_pid_i = int(pred_pid) if pred_pid not in (None, "", 0, "0") else None
    except (TypeError, ValueError):
        pred_pid_i = None
    if pid_alive(pred_pid_i):
        raise BridgeError(
            f"predecessor bridge PID {pred_pid_i} still alive; refuse barrier clear",
            disposition=Disposition.LOCKED,
        )

    # Ensure no live worktree lock holder.
    lock_path = locks_dir / worktree_lock_name(worktree.resolve())
    if lock_path.exists() and not lock_path.is_symlink():
        try:
            rec = LockRecord.from_dict(json.loads(lock_path.read_text(encoding="utf-8")))
            if pid_alive(rec.bridge_pid) or pid_alive(rec.child_pid):
                raise BridgeError(
                    "live worktree lock holder exists; refuse barrier clear",
                    disposition=Disposition.LOCKED,
                )
        except BridgeError:
            raise
        except (OSError, json.JSONDecodeError, KeyError, TypeError, ValueError):
            pass

    # Attempt non-blocking flock to prove no holder; release immediately.
    if fcntl is not None and lock_path.exists() and not lock_path.is_symlink():
        flags = os.O_RDWR | os.O_CLOEXEC | os.O_NOFOLLOW
        try:
            fd = os.open(str(lock_path), flags)
        except OSError:
            fd = -1
        if fd >= 0:
            try:
                try:
                    fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
                except BlockingIOError as exc:
                    raise BridgeError(
                        "worktree flock still held; refuse barrier clear",
                        disposition=Disposition.LOCKED,
                    ) from exc
                fcntl.flock(fd, fcntl.LOCK_UN)
            finally:
                os.close(fd)

    if git_inspect is None:
        raise BridgeError(
            "recovery requires worktree branch/HEAD/status inspection evidence",
            disposition=Disposition.REJECTED,
        )
    for key in ("branch", "head", "dirty", "inspectedAt"):
        if key not in git_inspect:
            raise BridgeError(
                f"recovery git inspection missing field: {key}",
                disposition=Disposition.REJECTED,
            )

    barrier.remove_atomic()
    return {
        "cleared": True,
        "worktree": str(worktree.resolve()),
        "predecessorTaskId": pred_id,
        "gitInspect": git_inspect,
        "clearedAt": utc_now_iso(),
    }


def task_id_lock_name(task_id: str) -> str:
    digest = hashlib.sha256(task_id.encode("utf-8")).hexdigest()[:16]
    safe = re_safe(task_id)
    return f"taskid-{safe}-{digest}.lock"


class TaskIdLock:
    """Process-wide exclusive admission lock for a taskId (cross-worktree)."""

    def __init__(self, locks_dir: Path, task_id: str):
        if fcntl is None:
            raise BridgeError("fcntl.flock unavailable; Linux required", disposition=Disposition.FAILED)
        self.locks_dir = locks_dir
        self.task_id = task_id
        self.path = locks_dir / task_id_lock_name(task_id)
        self._fh: Any = None
        self.acquired = False

    def acquire(self) -> None:
        if self.locks_dir.is_symlink():
            raise BridgeError(
                f"locks directory must not be a symlink: {self.locks_dir}",
                disposition=Disposition.FAILED,
            )
        existed = _assert_lock_file_openable(self.path, "task-id lock")
        self.locks_dir.mkdir(parents=True, exist_ok=True)
        flags = os.O_RDWR | os.O_CREAT | os.O_CLOEXEC | os.O_NOFOLLOW
        fd = os.open(str(self.path), flags, FILE_MODE)
        try:
            st = os.fstat(fd)
            if not stat.S_ISREG(st.st_mode):
                os.close(fd)
                raise BridgeError(
                    f"task-id lock is not a regular file: {self.path}",
                    disposition=Disposition.FAILED,
                )
            mode = stat.S_IMODE(st.st_mode)
            if existed and mode != FILE_MODE:
                os.close(fd)
                raise BridgeError(
                    f"task-id lock mode {oct(mode)} != required {oct(FILE_MODE)}; "
                    "explicit recovery required (no silent repair)",
                    disposition=Disposition.FAILED,
                )
            if not existed:
                os.fchmod(fd, FILE_MODE)
            self._fh = os.fdopen(fd, "a+", encoding="utf-8")
        except BridgeError:
            raise
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            raise
        try:
            fcntl.flock(self._fh.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            self._fh.close()
            self._fh = None
            raise BridgeError(
                f"live task-id lock held for {self.task_id}",
                disposition=Disposition.LOCKED,
            ) from exc
        self.acquired = True
        self._fh.seek(0)
        self._fh.truncate()
        self._fh.write(
            json.dumps(
                {"taskId": self.task_id, "bridgePid": os.getpid(), "at": utc_now_iso()},
                indent=2,
                sort_keys=True,
            )
            + "\n"
        )
        self._fh.flush()
        os.fsync(self._fh.fileno())

    def release(self) -> None:
        if self._fh is None:
            return
        try:
            fcntl.flock(self._fh.fileno(), fcntl.LOCK_UN)
        finally:
            self._fh.close()
            self._fh = None
            self.acquired = False

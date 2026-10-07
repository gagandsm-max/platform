"""Exclusive per-worktree file locks using fcntl.flock."""

from __future__ import annotations

import hashlib
import json
import os
import time
from pathlib import Path
from typing import Any, Optional

from .evidence import utc_now_iso
from .models import BridgeError, Disposition, LockRecord

try:
    import fcntl
except ImportError:  # pragma: no cover - Linux-only bridge
    fcntl = None  # type: ignore


def worktree_lock_name(worktree: Path) -> str:
    digest = hashlib.sha256(str(worktree.resolve()).encode("utf-8")).hexdigest()[:16]
    safe = re_safe(worktree.name)
    return f"{safe}-{digest}.lock"


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


class WorktreeLock:
    """One exclusive lock per canonical worktree path."""

    def __init__(self, locks_dir: Path, worktree: Path):
        if fcntl is None:
            raise BridgeError("fcntl.flock unavailable; Linux required", disposition=Disposition.FAILED)
        self.locks_dir = locks_dir
        self.worktree = worktree.resolve()
        self.path = locks_dir / worktree_lock_name(self.worktree)
        self._fh: Any = None
        self.record: Optional[LockRecord] = None
        self.acquired = False
        self.lifecycle: dict[str, Any] = {
            "lockPath": str(self.path),
            "acquired": False,
            "released": False,
            "staleReclaimed": False,
            "liveConflict": False,
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
        if self.path.is_symlink():
            raise BridgeError(
                f"lock file must not be a symlink: {self.path}",
                disposition=Disposition.FAILED,
            )
        self.locks_dir.mkdir(parents=True, exist_ok=True)
        import stat as _stat

        from .models import DIR_MODE, FILE_MODE

        try:
            os.chmod(self.locks_dir, DIR_MODE)
        except OSError:
            pass
        flags = os.O_RDWR | os.O_CREAT | os.O_CLOEXEC | os.O_NOFOLLOW
        fd = os.open(str(self.path), flags, FILE_MODE)
        try:
            os.fchmod(fd, FILE_MODE)
            st = os.fstat(fd)
            if not _stat.S_ISREG(st.st_mode):
                os.close(fd)
                raise BridgeError(
                    f"lock path is not a regular file: {self.path}",
                    disposition=Disposition.FAILED,
                )
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

        existing = self._read_from_fh()
        if existing is not None:
            bridge_live = pid_alive(existing.bridge_pid)
            child_live = pid_alive(existing.child_pid)
            if bridge_live or child_live:
                # Another holder wrote the record but we somehow got the flock —
                # treat as live conflict and release.
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
    if not path.exists():
        return result
    try:
        record = LockRecord.from_dict(json.loads(path.read_text(encoding="utf-8")))
    except (OSError, json.JSONDecodeError, KeyError, TypeError, ValueError) as exc:
        result["error"] = str(exc)
        return result
    result["record"] = record.to_wire()
    result["bridgePidAlive"] = pid_alive(record.bridge_pid)
    result["childPidAlive"] = pid_alive(record.child_pid)
    result["stale"] = not (pid_alive(record.bridge_pid) or pid_alive(record.child_pid))
    result["inspectedAt"] = utc_now_iso()
    result["mtime"] = time.ctime(path.stat().st_mtime)
    return result

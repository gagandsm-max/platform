"""Evidence writing and secret redaction for the Dot Host-Control Bridge."""

from __future__ import annotations

import json
import os
import re
import stat
from pathlib import Path
from typing import Any, Iterable, Mapping, Optional, Sequence, TextIO

from .models import (
    DIR_MODE,
    FILE_MODE,
    MAX_BRIDGE_LOG_BYTES,
    MAX_STDERR_LOG_BYTES,
    MAX_STDOUT_LOG_BYTES,
    REDACTION_OVERLAP_BYTES,
    TRUNCATION_MARK,
    TaskResult,
)

# Credential-bearing URL userinfo and common token shapes.
_URL_USERINFO_RE = re.compile(
    r"(?i)\b([a-z][a-z0-9+.-]*://)([^/\s:@]+):([^/\s@]+)@"
)
_TOKEN_RE = re.compile(
    r"(?i)\b(ghp_|gho_|ghu_|ghs_|ghr_|github_pat_|sk-|xox[baprs]-|AKIA)[A-Za-z0-9_\-./+=]{8,}"
)
_BEARER_RE = re.compile(r"(?i)(authorization\s*[:=]\s*bearer\s+)\S+")
_PEM_RE = re.compile(
    r"-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----",
    re.DOTALL,
)


def redact_text(text: str) -> str:
    """Redact tokens, private keys and credential-bearing URL userinfo."""
    if not text:
        return text
    out = _PEM_RE.sub("-----BEGIN PRIVATE KEY-----[REDACTED]-----END PRIVATE KEY-----", text)
    out = _URL_USERINFO_RE.sub(r"\1[REDACTED]:[REDACTED]@", out)
    out = _TOKEN_RE.sub("[REDACTED_TOKEN]", out)
    out = _BEARER_RE.sub(r"\1[REDACTED]", out)
    return out


def redact_bytes(data: bytes) -> bytes:
    return redact_text(data.decode("utf-8", errors="replace")).encode("utf-8")


def redact_argv(argv: Sequence[str]) -> list[str]:
    return [redact_text(part) for part in argv]


def utc_now_iso() -> str:
    from datetime import datetime, timezone

    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def ensure_real_dir(path: Path, *, label: str) -> Path:
    """Reject symlink directories; require a real directory under an already-checked root."""
    if path.exists() or path.is_symlink():
        if path.is_symlink():
            raise ValueError(f"{label} must not be a symlink: {path}")
        if not path.is_dir():
            raise ValueError(f"{label} is not a directory: {path}")
    return path


def open_exclusive_file(
    path: Path,
    *,
    mode: str = "wb",
    file_mode: int = FILE_MODE,
    append: bool = False,
) -> Any:
    """Open a regular file without following symlinks (O_NOFOLLOW on Linux)."""
    if path.is_symlink():
        raise OSError(f"refusing to open symlink: {path}")
    flags = os.O_CLOEXEC | os.O_NOFOLLOW
    if "b" not in mode:
        # Text mode wrappers still open via binary fd then wrap.
        pass
    if append:
        flags |= os.O_RDWR | os.O_CREAT | os.O_APPEND
    elif "w" in mode:
        flags |= os.O_WRONLY | os.O_CREAT | os.O_TRUNC
    else:
        flags |= os.O_RDONLY
    fd = os.open(str(path), flags, file_mode)
    try:
        os.fchmod(fd, file_mode)
        st = os.fstat(fd)
        if stat.S_ISLNK(st.st_mode):
            os.close(fd)
            raise OSError(f"refusing symlink after open: {path}")
        if not stat.S_ISREG(st.st_mode):
            os.close(fd)
            raise OSError(f"refusing non-regular file: {path}")
        if "b" in mode:
            return os.fdopen(fd, mode)
        # Binary capture then TextIO wrapper is unnecessary; open text via fd.
        return os.fdopen(fd, mode, encoding="utf-8")
    except Exception:
        try:
            os.close(fd)
        except OSError:
            pass
        raise


def atomic_write_json(path: Path, payload: Mapping[str, Any]) -> None:
    """Write JSON via tmp + fsync + rename; never follow destination symlink."""
    parent = path.parent
    if parent.is_symlink():
        raise OSError(f"refusing write under symlink directory: {parent}")
    parent.mkdir(parents=True, exist_ok=True)
    if path.is_symlink():
        raise OSError(f"refusing to overwrite symlink result: {path}")
    tmp = path.with_suffix(path.suffix + ".tmp")
    if tmp.is_symlink():
        raise OSError(f"refusing symlink temp path: {tmp}")
    data = json.dumps(payload, indent=2, sort_keys=True) + "\n"
    data = redact_text(data)
    flags = os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_CLOEXEC | os.O_NOFOLLOW
    fd = os.open(str(tmp), flags, FILE_MODE)
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
    os.replace(tmp, path)
    try:
        os.chmod(path, FILE_MODE)
    except OSError:
        pass


def write_result(results_dir: Path, result: TaskResult) -> Path:
    if results_dir.is_symlink():
        raise OSError(f"results directory must not be a symlink: {results_dir}")
    out = results_dir / f"{result.task_id}.result.json"
    payload = result.to_dict()
    payload["commandArgv"] = redact_argv(payload.get("commandArgv") or [])
    if payload.get("error"):
        payload["error"] = redact_text(str(payload["error"]))
    atomic_write_json(out, payload)
    return out


def write_state(state_dir: Path, task_id: str, state: Mapping[str, Any]) -> Path:
    if state_dir.is_symlink():
        raise OSError(f"state directory must not be a symlink: {state_dir}")
    out = state_dir / f"{task_id}.json"
    atomic_write_json(out, dict(state))
    return out


class BoundedRedactingWriter:
    """Capture child output with hard byte bounds and cross-chunk redaction.

    Raw input is retained in a bounded memory buffer (max_bytes + overlap) so
    secrets split across pipe reads are redacted against the full window before
    any durable write. The durable file size, including the truncation marker,
    never exceeds ``max_bytes``.
    """

    def __init__(self, path: Path, *, max_bytes: int, overlap: int = REDACTION_OVERLAP_BYTES):
        if path.parent.is_symlink():
            raise OSError(f"log directory must not be a symlink: {path.parent}")
        path.parent.mkdir(parents=True, exist_ok=True)
        if path.is_symlink():
            raise OSError(f"refusing symlink log path: {path}")
        self.path = path
        self.max_bytes = max_bytes
        self.overlap = max(0, min(overlap, max(64, max_bytes // 4) if max_bytes else 0))
        self.bytes_captured = 0
        self.truncated = False
        self._raw = bytearray()
        self._raw_cap = max_bytes + self.overlap
        self._fh = open_exclusive_file(path, mode="wb", file_mode=FILE_MODE)
        self._closed = False
        self._durable_flushed = False

    def write(self, data: bytes) -> int:
        if self._closed or self.truncated:
            return len(data)
        if not data:
            return 0
        room = self._raw_cap - len(self._raw)
        if room <= 0:
            self.truncated = True
            return len(data)
        self._raw.extend(data[:room])
        if len(data) > room:
            self.truncated = True
        return len(data)

    def _emit_durable(self) -> None:
        """Redact the full raw window and write a bounded durable representation."""
        if self._durable_flushed:
            return
        self._durable_flushed = True
        redacted = redact_bytes(bytes(self._raw))
        self._raw.clear()
        mark = TRUNCATION_MARK
        if self.truncated or len(redacted) > self.max_bytes:
            self.truncated = True
            if len(mark) >= self.max_bytes:
                out = mark[: self.max_bytes]
            else:
                body = redacted[: self.max_bytes - len(mark)]
                out = body + mark
        else:
            out = redacted
        if len(out) > self.max_bytes:
            out = out[: self.max_bytes]
            self.truncated = True
        self._fh.write(out)
        self._fh.flush()
        self.bytes_captured = len(out)

    def fileno(self) -> int:
        return self._fh.fileno()

    def close(self) -> None:
        if not self._closed:
            try:
                self._emit_durable()
                self._fh.flush()
                os.fsync(self._fh.fileno())
            finally:
                self._fh.close()
                self._closed = True
            try:
                os.chmod(self.path, FILE_MODE)
            except OSError:
                pass
            try:
                if self.path.stat().st_size > self.max_bytes:
                    with open(self.path, "r+b") as fh:
                        fh.truncate(self.max_bytes)
                    self.bytes_captured = min(self.bytes_captured, self.max_bytes)
                    self.truncated = True
            except OSError:
                pass

    def __enter__(self) -> "BoundedRedactingWriter":
        return self

    def __exit__(self, *args: object) -> None:
        self.close()


def open_log_files(
    logs_dir: Path,
    task_id: str,
    *,
    max_stdout: int = MAX_STDOUT_LOG_BYTES,
    max_stderr: int = MAX_STDERR_LOG_BYTES,
) -> tuple[Path, Path, BoundedRedactingWriter, BoundedRedactingWriter]:
    if logs_dir.is_symlink():
        raise OSError(f"logs directory must not be a symlink: {logs_dir}")
    logs_dir.mkdir(parents=True, exist_ok=True)
    stdout_path = logs_dir / f"{task_id}.stdout.log"
    stderr_path = logs_dir / f"{task_id}.stderr.log"
    stdout_w = BoundedRedactingWriter(stdout_path, max_bytes=max_stdout)
    stderr_w = BoundedRedactingWriter(stderr_path, max_bytes=max_stderr)
    return stdout_path, stderr_path, stdout_w, stderr_w


def append_bridge_log(
    logs_dir: Path,
    task_id: str,
    lines: Iterable[str],
    *,
    max_bytes: int = MAX_BRIDGE_LOG_BYTES,
) -> tuple[Path, bool]:
    """Append redacted bridge log lines; truncate when bound reached."""
    if logs_dir.is_symlink():
        raise OSError(f"logs directory must not be a symlink: {logs_dir}")
    logs_dir.mkdir(parents=True, exist_ok=True)
    path = logs_dir / f"{task_id}.bridge.log"
    if path.is_symlink():
        raise OSError(f"refusing symlink bridge log: {path}")
    truncated = False
    existing = path.stat().st_size if path.exists() and path.is_file() else 0
    if existing >= max_bytes:
        return path, True
    flags = os.O_WRONLY | os.O_CREAT | os.O_APPEND | os.O_CLOEXEC | os.O_NOFOLLOW
    fd = os.open(str(path), flags, FILE_MODE)
    try:
        os.fchmod(fd, FILE_MODE)
        with os.fdopen(fd, "a", encoding="utf-8") as fh:
            written = existing
            for line in lines:
                red = redact_text(line.rstrip("\n")) + "\n"
                encoded = red.encode("utf-8")
                if written + len(encoded) > max_bytes:
                    remain = max_bytes - written
                    mark = "[TRUNCATED: bridge log byte bound reached]\n"
                    mark_b = mark.encode("utf-8")
                    # Ensure marker fits within bound when possible.
                    if remain >= len(mark_b):
                        payload_space = remain - len(mark_b)
                        if payload_space > 0:
                            fh.write(encoded[:payload_space].decode("utf-8", errors="ignore"))
                            written += payload_space
                        fh.write(mark)
                        written += len(mark_b)
                    elif remain > 0:
                        fh.write(mark_b[:remain].decode("utf-8", errors="ignore"))
                        written += remain
                    truncated = True
                    break
                fh.write(red)
                written += len(encoded)
            fh.flush()
            os.fsync(fh.fileno())
    except Exception:
        try:
            os.close(fd)
        except OSError:
            pass
        raise
    try:
        os.chmod(path, FILE_MODE)
    except OSError:
        pass
    return path, truncated


def safe_read_text(path: Path, *, max_bytes: int = 1_000_000) -> str:
    """Read at most ``max_bytes`` from a regular file without loading the whole file."""
    if path.is_symlink():
        raise OSError(f"refusing to read symlink: {path}")
    flags = os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW
    fd = os.open(str(path), flags)
    try:
        st = os.fstat(fd)
        if not stat.S_ISREG(st.st_mode):
            raise OSError(f"refusing non-regular file: {path}")
        raw = os.read(fd, max_bytes)
    finally:
        os.close(fd)
    return redact_text(raw.decode("utf-8", errors="replace"))

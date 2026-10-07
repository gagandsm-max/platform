"""Evidence writing and secret redaction for the Dot Host-Control Bridge."""

from __future__ import annotations

import json
import os
import re
import stat
from pathlib import Path
from typing import Any, Iterable, Mapping, Optional, Sequence

from .models import (
    FILE_MODE,
    MAX_BRIDGE_LOG_BYTES,
    MAX_STDERR_LOG_BYTES,
    MAX_STDOUT_LOG_BYTES,
    REDACTION_OVERLAP_BYTES,
    TRUNCATION_MARK,
    BridgeError,
    Disposition,
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
_PEM_BEGIN_RE = re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----")
_PARTIAL_TOKEN_PREFIX_RE = re.compile(
    r"(?i)\b(ghp_|gho_|ghu_|ghs_|ghr_|github_pat_|sk-|xox[baprs]-|AKIA)[A-Za-z0-9_\-./+=]{0,7}$"
)
_PARTIAL_URL_USERINFO_RE = re.compile(
    r"(?i)\b([a-z][a-z0-9+.-]*://)([^/\s:@]+)(?::([^/\s@]*))?$"
)
_PARTIAL_BEARER_RE = re.compile(r"(?i)(authorization\s*[:=]\s*bearer\s+)\S*$")

TRUNCATION_BOUNDARY_MARK = b"\n[REDACTED: truncation-boundary indeterminate]\n"


def redact_text(text: str) -> str:
    """Redact tokens, private keys and credential-bearing URL userinfo."""
    if not text:
        return text
    out = _PEM_RE.sub("-----BEGIN PRIVATE KEY-----[REDACTED]-----END PRIVATE KEY-----", text)
    out = _URL_USERINFO_RE.sub(r"\1[REDACTED]:[REDACTED]@", out)
    out = _TOKEN_RE.sub("[REDACTED_TOKEN]", out)
    out = _BEARER_RE.sub(r"\1[REDACTED]", out)
    return out


def redact_unterminated_at_boundary(text: str) -> str:
    """Fail-closed scrub of secrets that lack a terminator before end-of-buffer."""
    if not text:
        return text
    # Unterminated PEM: redact from last BEGIN with no matching END after it.
    begin_matches = list(_PEM_BEGIN_RE.finditer(text))
    if begin_matches:
        last = begin_matches[-1]
        after = text[last.start() :]
        if "-----END " not in after or "PRIVATE KEY-----" not in after.split("-----END ", 1)[-1][:80]:
            # No complete END after this BEGIN — redact through EOF.
            if "-----END " not in after or not re.search(
                r"-----END [A-Z ]*PRIVATE KEY-----", after
            ):
                text = text[: last.start()] + "[REDACTED: unterminated private key]"

    # Partial credential URL without trailing @ (userinfo incomplete at boundary).
    m = _PARTIAL_URL_USERINFO_RE.search(text)
    if m and "@" not in text[m.start() :]:
        text = text[: m.start()] + m.group(1) + "[REDACTED: partial credential URL]"

    # Partial token prefix with fewer than required token characters at EOF.
    m = _PARTIAL_TOKEN_PREFIX_RE.search(text)
    if m:
        text = text[: m.start()] + "[REDACTED_TOKEN]"

    # Partial bearer at EOF.
    m = _PARTIAL_BEARER_RE.search(text)
    if m:
        # Only scrub if this didn't already get a full bearer redact with trailing content.
        tail = text[m.end() :]
        if not tail.strip() or m.end() == len(text):
            text = text[: m.start()] + m.group(1) + "[REDACTED]"

    return text


def redact_bytes(data: bytes) -> bytes:
    return redact_text(data.decode("utf-8", errors="replace")).encode("utf-8")


def redact_bytes_fail_closed(data: bytes, *, truncated: bool) -> bytes:
    text = data.decode("utf-8", errors="replace")
    text = redact_text(text)
    if truncated:
        text = redact_unterminated_at_boundary(text)
    return text.encode("utf-8")


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
    allow_existing: bool = True,
) -> Any:
    """Open a regular file without following symlinks (O_NOFOLLOW on Linux).

    Pre-existing files with unsafe modes are rejected (no silent repair).
    Newly created files receive ``file_mode``.
    """
    if path.is_symlink():
        raise OSError(f"refusing to open symlink: {path}")
    existed = os.path.lexists(path)
    if existed:
        if not allow_existing:
            raise OSError(f"refusing to open existing file: {path}")
        if not path.is_file():
            raise OSError(f"refusing non-regular file: {path}")
        st_exist = path.stat()
        if not stat.S_ISREG(st_exist.st_mode):
            raise OSError(f"refusing non-regular file: {path}")
        existing_mode = stat.S_IMODE(st_exist.st_mode)
        if existing_mode != file_mode:
            raise BridgeError(
                f"durable file mode {oct(existing_mode)} != required {oct(file_mode)}: {path}; "
                "explicit recovery required (no silent repair)",
                disposition=Disposition.FAILED,
            )
        if st_exist.st_uid != os.geteuid():
            raise BridgeError(
                f"durable file owner uid {st_exist.st_uid} != bridge uid {os.geteuid()}: {path}",
                disposition=Disposition.FAILED,
            )
    flags = os.O_CLOEXEC | os.O_NOFOLLOW
    if append:
        flags |= os.O_RDWR | os.O_CREAT | os.O_APPEND
    elif "w" in mode:
        flags |= os.O_WRONLY | os.O_CREAT | os.O_TRUNC
    else:
        flags |= os.O_RDONLY
    fd = os.open(str(path), flags, file_mode)
    try:
        st = os.fstat(fd)
        if stat.S_ISLNK(st.st_mode):
            os.close(fd)
            raise OSError(f"refusing symlink after open: {path}")
        if not stat.S_ISREG(st.st_mode):
            os.close(fd)
            raise OSError(f"refusing non-regular file: {path}")
        if not existed:
            os.fchmod(fd, file_mode)
        else:
            # Never silently repair pre-existing modes.
            if stat.S_IMODE(st.st_mode) != file_mode:
                os.close(fd)
                raise BridgeError(
                    f"durable file mode {oct(stat.S_IMODE(st.st_mode))} != required "
                    f"{oct(file_mode)}: {path}; explicit recovery required (no silent repair)",
                    disposition=Disposition.FAILED,
                )
        if "b" in mode:
            return os.fdopen(fd, mode)
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
    # Destination was just created/replaced by us at FILE_MODE; verify without repair loops.
    st = path.stat()
    if stat.S_IMODE(st.st_mode) != FILE_MODE:
        try:
            os.chmod(path, FILE_MODE)
        except OSError as exc:
            raise OSError(f"result mode not 0600 after write: {path}") from exc


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
    """Capture child output with hard byte bounds and fail-closed truncation redaction.

    Raw input is retained in a bounded memory buffer (max_bytes + overlap). When
    capture truncates, the final overlap is treated as sensitive/indeterminate and
    is never emitted raw; unterminated secrets at the boundary are scrubbed.
    The durable file size, including markers, never exceeds ``max_bytes``.
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
        """Redact the raw window; on truncation, discard final overlap fail-closed."""
        if self._durable_flushed:
            return
        self._durable_flushed = True
        raw = bytes(self._raw)
        self._raw.clear()
        mark = TRUNCATION_MARK
        boundary = TRUNCATION_BOUNDARY_MARK

        if self.truncated:
            # Never emit the raw final overlap/tail directly.
            if self.overlap > 0 and len(raw) > self.overlap:
                head = raw[: -self.overlap]
            else:
                head = b""
            redacted = redact_bytes_fail_closed(head, truncated=True)
            # Explicit indeterminate-zone marker replaces discarded overlap.
            body = redacted + boundary + mark
            self.truncated = True
        else:
            redacted = redact_bytes_fail_closed(raw, truncated=False)
            # Still scrub unterminated PEM that ends mid-stream without truncation.
            redacted = redact_unterminated_at_boundary(
                redacted.decode("utf-8", errors="replace")
            ).encode("utf-8")
            if len(redacted) > self.max_bytes:
                self.truncated = True
                # Treat overflow like truncation: drop final overlap of redacted window.
                if self.overlap > 0 and len(redacted) > self.overlap:
                    head = redacted[: -self.overlap]
                else:
                    head = b""
                body = head + boundary + mark
            else:
                body = redacted

        if len(body) > self.max_bytes:
            self.truncated = True
            if len(mark) >= self.max_bytes:
                out = mark[: self.max_bytes]
            else:
                # Prefer keeping truncation markers; drop from front of body if needed.
                keep_tail = boundary + mark if self.truncated else mark
                if len(keep_tail) >= self.max_bytes:
                    out = keep_tail[-self.max_bytes :]
                else:
                    prefix_budget = self.max_bytes - len(keep_tail)
                    # body may already include markers; take safe prefix only.
                    prefix = body[:prefix_budget]
                    out = prefix + keep_tail
                    if len(out) > self.max_bytes:
                        out = out[: self.max_bytes]
        else:
            out = body

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
    """Append redacted bridge log lines; truncate when bound reached.

    Pre-existing unsafe modes are rejected (no silent repair).
    """
    if logs_dir.is_symlink():
        raise OSError(f"logs directory must not be a symlink: {logs_dir}")
    logs_dir.mkdir(parents=True, exist_ok=True)
    path = logs_dir / f"{task_id}.bridge.log"
    if path.is_symlink():
        raise OSError(f"refusing symlink bridge log: {path}")
    truncated = False
    existed = os.path.lexists(path)
    if existed:
        if not path.is_file():
            raise OSError(f"bridge log is not a regular file: {path}")
        st_exist = path.stat()
        mode = stat.S_IMODE(st_exist.st_mode)
        if mode != FILE_MODE:
            raise BridgeError(
                f"bridge log mode {oct(mode)} != required {oct(FILE_MODE)}; "
                "explicit recovery required (no silent repair)",
                disposition=Disposition.FAILED,
            )
        if st_exist.st_uid != os.geteuid():
            raise BridgeError(
                f"bridge log owner uid {st_exist.st_uid} != bridge uid {os.geteuid()}",
                disposition=Disposition.FAILED,
            )
    existing = path.stat().st_size if existed and path.is_file() else 0
    if existing >= max_bytes:
        return path, True
    flags = os.O_WRONLY | os.O_CREAT | os.O_APPEND | os.O_CLOEXEC | os.O_NOFOLLOW
    fd = os.open(str(path), flags, FILE_MODE)
    try:
        st = os.fstat(fd)
        if not existed:
            os.fchmod(fd, FILE_MODE)
        elif stat.S_IMODE(st.st_mode) != FILE_MODE:
            os.close(fd)
            raise BridgeError(
                f"bridge log mode {oct(stat.S_IMODE(st.st_mode))} != required {oct(FILE_MODE)}; "
                "explicit recovery required (no silent repair)",
                disposition=Disposition.FAILED,
            )
        with os.fdopen(fd, "a", encoding="utf-8") as fh:
            written = existing
            for line in lines:
                red = redact_text(line.rstrip("\n")) + "\n"
                encoded = red.encode("utf-8")
                if written + len(encoded) > max_bytes:
                    remain = max_bytes - written
                    mark = "[TRUNCATED: bridge log byte bound reached]\n"
                    mark_b = mark.encode("utf-8")
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
    except BridgeError:
        raise
    except Exception:
        try:
            os.close(fd)
        except OSError:
            pass
        raise
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

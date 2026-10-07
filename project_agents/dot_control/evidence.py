"""Evidence writing and secret redaction for the Dot Host-Control Bridge."""

from __future__ import annotations

import json
import os
import re
from pathlib import Path
from typing import Any, Iterable, Mapping, Optional, Sequence

from .models import TaskResult

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


def redact_argv(argv: Sequence[str]) -> list[str]:
    return [redact_text(part) for part in argv]


def utc_now_iso() -> str:
    from datetime import datetime, timezone

    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def atomic_write_json(path: Path, payload: Mapping[str, Any]) -> None:
    """Write JSON via tmp + fsync + rename."""
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    data = json.dumps(payload, indent=2, sort_keys=True) + "\n"
    data = redact_text(data)
    with open(tmp, "w", encoding="utf-8") as fh:
        fh.write(data)
        fh.flush()
        os.fsync(fh.fileno())
    os.replace(tmp, path)


def write_result(results_dir: Path, result: TaskResult) -> Path:
    out = results_dir / f"{result.task_id}.result.json"
    payload = result.to_dict()
    payload["commandArgv"] = redact_argv(payload.get("commandArgv") or [])
    if payload.get("error"):
        payload["error"] = redact_text(str(payload["error"]))
    atomic_write_json(out, payload)
    return out


def write_state(state_dir: Path, task_id: str, state: Mapping[str, Any]) -> Path:
    out = state_dir / f"{task_id}.json"
    atomic_write_json(out, dict(state))
    return out


def open_log_files(logs_dir: Path, task_id: str) -> tuple[Path, Path, Any, Any]:
    logs_dir.mkdir(parents=True, exist_ok=True)
    stdout_path = logs_dir / f"{task_id}.stdout.log"
    stderr_path = logs_dir / f"{task_id}.stderr.log"
    stdout_f = open(stdout_path, "w", encoding="utf-8")
    stderr_f = open(stderr_path, "w", encoding="utf-8")
    return stdout_path, stderr_path, stdout_f, stderr_f


def append_bridge_log(logs_dir: Path, task_id: str, lines: Iterable[str]) -> Path:
    logs_dir.mkdir(parents=True, exist_ok=True)
    path = logs_dir / f"{task_id}.bridge.log"
    with open(path, "a", encoding="utf-8") as fh:
        for line in lines:
            fh.write(redact_text(line.rstrip("\n")) + "\n")
    return path


def safe_read_text(path: Path, *, max_bytes: int = 1_000_000) -> str:
    raw = path.read_bytes()[:max_bytes]
    return redact_text(raw.decode("utf-8", errors="replace"))

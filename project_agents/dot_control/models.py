"""Typed models and constants for the Dot Host-Control Bridge."""

from __future__ import annotations

from dataclasses import asdict, dataclass, field
from enum import Enum
from pathlib import Path
from typing import Any, Mapping, Optional, Sequence


SCHEMA_VERSION = "dot-control-v1"

DEFAULT_CONTROL_ROOT = Path("/home/gagan/nms-dot-control")
DEFAULT_HOME = Path("/home/gagan")
APPROVED_REPO_EXACT = Path("/home/gagan/nms")
APPROVED_REPO_PREFIX = "/home/gagan/nms-"
APPROVED_CONTROL_ROOT = Path("/home/gagan/nms-dot-control")

CONTROL_SUBDIRS = (
    "inbox",
    "processing",
    "results",
    "logs",
    "locks",
    "state",
    "quarantine",
    "tasks",
)

MAX_PROMPT_BYTES = 256 * 1024
MAX_COMMIT_MESSAGE_LEN = 200
MAX_TIMEOUT_SECONDS = 6 * 60 * 60
DEFAULT_TIMEOUT_SECONDS = 1800
PROCESS_KILL_GRACE_SECONDS = 10
DAEMON_POLL_SECONDS = 2.0

SHA_HEX_LEN = 40

FORBIDDEN_MANIFEST_KEYS = frozenset(
    {
        "command",
        "commands",
        "shell",
        "shellCommand",
        "argv",
        "args",
        "executable",
        "exec",
        "script",
        "env",
        "environment",
        "sudo",
        "eval",
    }
)


class Action(str, Enum):
    REPO_PROBE = "REPO_PROBE"
    GIT_COMMIT_PROOF = "GIT_COMMIT_PROOF"
    CURSOR_TASK = "CURSOR_TASK"
    CODEX_TASK = "CODEX_TASK"
    ANTIGRAVITY_AUDIT = "ANTIGRAVITY_AUDIT"
    TEST_PROFILE = "TEST_PROFILE"


class WriterTool(str, Enum):
    CURSOR_CLI = "cursor-cli"
    CODEX = "codex"
    NONE = "none"


class VerifierTool(str, Enum):
    ANTIGRAVITY = "antigravity"
    NONE = "none"


class Disposition(str, Enum):
    SUCCESS = "SUCCESS"
    FAILED = "FAILED"
    TIMEOUT = "TIMEOUT"
    REJECTED = "REJECTED"
    QUARANTINED = "QUARANTINED"
    UNSUPPORTED = "UNSUPPORTED"
    DRY_RUN = "DRY_RUN"
    LOCKED = "LOCKED"
    CRASH_RECOVERY = "CRASH_RECOVERY"


# Hard-coded test profiles — manifest supplies a name only.
TEST_PROFILES: Mapping[str, Sequence[str]] = {
    "capability-pack": ("python3", "project_agents/tools/validate_agents.py"),
    "agent": (
        "python3",
        "-m",
        "unittest",
        "tests.engineering_orchestrator.test_capability_pack_v12",
    ),
    "static-contracts": ("make", "test-static-contracts"),
}


@dataclass(frozen=True)
class TaskManifest:
    schema_version: str
    task_id: str
    governing_issue: str
    action: Action
    writer_tool: WriterTool
    verifier_tool: VerifierTool
    repository: str
    worktree: Path
    branch: str
    authoritative_base: str
    expected_head: str
    allowed_files: tuple[str, ...]
    forbidden_files: tuple[str, ...]
    prompt_file: Optional[Path]
    timeout_seconds: int
    allow_commit: bool
    allow_push: bool
    allow_pull_request: bool
    created_at: str
    requested_by: str
    commit_message: Optional[str] = None
    test_profile: Optional[str] = None

    def to_public_dict(self) -> dict[str, Any]:
        return {
            "schemaVersion": self.schema_version,
            "taskId": self.task_id,
            "governingIssue": self.governing_issue,
            "action": self.action.value,
            "writerTool": self.writer_tool.value,
            "verifierTool": self.verifier_tool.value,
            "repository": self.repository,
            "worktree": str(self.worktree),
            "branch": self.branch,
            "authoritativeBase": self.authoritative_base,
            "expectedHead": self.expected_head,
            "allowedFiles": list(self.allowed_files),
            "forbiddenFiles": list(self.forbidden_files),
            "promptFile": str(self.prompt_file) if self.prompt_file else None,
            "timeoutSeconds": self.timeout_seconds,
            "allowCommit": self.allow_commit,
            "allowPush": self.allow_push,
            "allowPullRequest": self.allow_pull_request,
            "createdAt": self.created_at,
            "requestedBy": self.requested_by,
            "commitMessage": self.commit_message,
            "testProfile": self.test_profile,
        }


@dataclass
class LockRecord:
    task_id: str
    action: str
    writer_tool: str
    worktree: str
    branch: str
    expected_head: str
    bridge_pid: int
    child_pid: Optional[int]
    created_at: str
    heartbeat_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    @classmethod
    def from_dict(cls, data: Mapping[str, Any]) -> "LockRecord":
        return cls(
            task_id=str(data["taskId"] if "taskId" in data else data["task_id"]),
            action=str(data.get("action", "")),
            writer_tool=str(data.get("writerTool", data.get("writer_tool", ""))),
            worktree=str(data.get("worktree", "")),
            branch=str(data.get("branch", "")),
            expected_head=str(data.get("expectedHead", data.get("expected_head", ""))),
            bridge_pid=int(data.get("bridgePid", data.get("bridge_pid", 0))),
            child_pid=(
                None
                if data.get("childPid", data.get("child_pid")) in (None, "")
                else int(data.get("childPid", data.get("child_pid")))
            ),
            created_at=str(data.get("createdAt", data.get("created_at", ""))),
            heartbeat_at=str(data.get("heartbeatAt", data.get("heartbeat_at", ""))),
        )

    def to_wire(self) -> dict[str, Any]:
        return {
            "taskId": self.task_id,
            "action": self.action,
            "writerTool": self.writer_tool,
            "worktree": self.worktree,
            "branch": self.branch,
            "expectedHead": self.expected_head,
            "bridgePid": self.bridge_pid,
            "childPid": self.child_pid,
            "createdAt": self.created_at,
            "heartbeatAt": self.heartbeat_at,
        }


@dataclass
class CommandEvidence:
    argv: list[str]
    exit_code: Optional[int] = None
    timed_out: bool = False
    sigterm_sent: bool = False
    sigkill_sent: bool = False
    child_pid: Optional[int] = None
    stdout_log: Optional[str] = None
    stderr_log: Optional[str] = None
    unsupported_reason: Optional[str] = None


@dataclass
class TaskResult:
    task_id: str
    action: str
    selected_tool: str
    worktree: str
    branch: str
    starting_head: str
    ending_head: str
    authoritative_base: str
    command_argv: list[str]
    started_at: str
    ended_at: str
    exit_code: Optional[int]
    timed_out: bool
    stdout_log: Optional[str]
    stderr_log: Optional[str]
    changed_files: list[str]
    committed_files: list[str]
    commit_sha: Optional[str]
    push_status: str
    pr_status: str
    lock_lifecycle: dict[str, Any]
    disposition: str
    error: Optional[str] = None
    probe: dict[str, Any] = field(default_factory=dict)
    notes: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "taskId": self.task_id,
            "action": self.action,
            "selectedTool": self.selected_tool,
            "worktree": self.worktree,
            "branch": self.branch,
            "startingHead": self.starting_head,
            "endingHead": self.ending_head,
            "authoritativeBase": self.authoritative_base,
            "commandArgv": self.command_argv,
            "startedAt": self.started_at,
            "endedAt": self.ended_at,
            "exitCode": self.exit_code,
            "timedOut": self.timed_out,
            "stdoutLog": self.stdout_log,
            "stderrLog": self.stderr_log,
            "changedFiles": self.changed_files,
            "committedFiles": self.committed_files,
            "commitSha": self.commit_sha,
            "pushStatus": self.push_status,
            "prStatus": self.pr_status,
            "lockLifecycle": self.lock_lifecycle,
            "disposition": self.disposition,
            "error": self.error,
            "probe": self.probe,
            "notes": self.notes,
        }


class BridgeError(Exception):
    """Validation or precondition failure (non-execution)."""

    def __init__(self, message: str, *, disposition: Disposition = Disposition.REJECTED):
        super().__init__(message)
        self.disposition = disposition

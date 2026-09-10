"""Core task/evidence models and enums for Engineering Orchestrator Phase 1."""

from __future__ import annotations

from dataclasses import asdict, dataclass, field
from enum import Enum
from typing import Any, Optional


class State(str, Enum):
    TASK_ACCEPTED = "TASK_ACCEPTED"
    SOURCE_INSPECTION = "SOURCE_INSPECTION"
    AGENT_ROUTING = "AGENT_ROUTING"
    RCA_COMPLETE = "RCA_COMPLETE"
    IMPLEMENTATION = "IMPLEMENTATION"
    FOCUSED_VALIDATION = "FOCUSED_VALIDATION"
    FULL_VALIDATION = "FULL_VALIDATION"
    BASE_RECONCILIATION = "BASE_RECONCILIATION"
    INDEPENDENT_REVIEW = "INDEPENDENT_REVIEW"
    EXACT_HEAD_CI = "EXACT_HEAD_CI"
    REALITY_GATE = "REALITY_GATE"
    REWORK = "REWORK"
    READY_FOR_MERGE_GATE = "READY_FOR_MERGE_GATE"
    BLOCKED = "BLOCKED"
    FAILED = "FAILED"
    CANCELLED = "CANCELLED"
    NOT_MERGED = "NOT_MERGED"
    MERGED = "MERGED"


class Actor(str, Enum):
    WRITER = "WRITER"
    REALITY_CHECKER = "REALITY_CHECKER"
    MERGE_AUTHORITY = "MERGE_AUTHORITY"
    FOUNDER_CTO = "FOUNDER_CTO"
    SYSTEM = "SYSTEM"


class FailureClass(str, Enum):
    SOURCE_DEFECT = "SOURCE_DEFECT"
    TEST_DEFECT = "TEST_DEFECT"
    CONFIG_DEFECT = "CONFIG_DEFECT"
    ENVIRONMENT_DEFECT = "ENVIRONMENT_DEFECT"
    TRANSIENT_EXTERNAL_FAILURE = "TRANSIENT_EXTERNAL_FAILURE"
    SECURITY_BLOCKER = "SECURITY_BLOCKER"
    ARCHITECTURE_BLOCKER = "ARCHITECTURE_BLOCKER"
    MIGRATION_COLLISION = "MIGRATION_COLLISION"
    EVIDENCE_GAP = "EVIDENCE_GAP"
    BUSINESS_DECISION_REQUIRED = "BUSINESS_DECISION_REQUIRED"


class EvidenceType(str, Enum):
    SOURCE = "SOURCE"
    LOCAL_TEST = "LOCAL_TEST"
    LOCAL_RUNTIME = "LOCAL_RUNTIME"
    DATABASE = "DATABASE"
    SECURITY = "SECURITY"
    CI = "CI"
    INFERENCE = "INFERENCE"
    RECOMMENDATION = "RECOMMENDATION"


class EvidenceResult(str, Enum):
    PASS = "PASS"
    FAIL = "FAIL"
    NOT_RUN = "NOT_RUN"


class RcaStatus(str, Enum):
    KNOWN = "KNOWN"
    UNKNOWN = "UNKNOWN"
    INCONCLUSIVE = "INCONCLUSIVE"


class HandoffStatus(str, Enum):
    PENDING = "PENDING"
    RETURNED = "RETURNED"
    REJECTED = "REJECTED"
    EXPIRED = "EXPIRED"


class MigrationReservationState(str, Enum):
    RESERVED = "RESERVED"
    RELEASED = "RELEASED"
    COLLIDED = "COLLIDED"


TERMINAL_STATES = frozenset(
    {
        State.FAILED,
        State.CANCELLED,
        State.NOT_MERGED,
        State.MERGED,
    }
)

HAPPY_PATH: tuple[State, ...] = (
    State.TASK_ACCEPTED,
    State.SOURCE_INSPECTION,
    State.AGENT_ROUTING,
    State.RCA_COMPLETE,
    State.IMPLEMENTATION,
    State.FOCUSED_VALIDATION,
    State.FULL_VALIDATION,
    State.BASE_RECONCILIATION,
    State.INDEPENDENT_REVIEW,
    State.EXACT_HEAD_CI,
    State.REALITY_GATE,
    State.READY_FOR_MERGE_GATE,
    State.MERGED,
)


def enum_value(value: Enum | str) -> str:
    return value.value if isinstance(value, Enum) else str(value)


@dataclass
class Evidence:
    evidence_id: str
    type: EvidenceType
    head_sha: str
    claim_scope: str
    result: EvidenceResult
    collected_at: str
    payload_ref: str = ""
    impact_known: bool = True
    invalidated: bool = False
    invalidation_reason: str = ""

    def to_dict(self) -> dict[str, Any]:
        d = asdict(self)
        d["type"] = enum_value(self.type)
        d["result"] = enum_value(self.result)
        return d

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "Evidence":
        return cls(
            evidence_id=str(data["evidence_id"]),
            type=EvidenceType(data["type"]),
            head_sha=str(data["head_sha"]),
            claim_scope=str(data.get("claim_scope", "")),
            result=EvidenceResult(data["result"]),
            collected_at=str(data.get("collected_at", "")),
            payload_ref=str(data.get("payload_ref", "")),
            impact_known=bool(data.get("impact_known", True)),
            invalidated=bool(data.get("invalidated", False)),
            invalidation_reason=str(data.get("invalidation_reason", "")),
        )


@dataclass
class Task:
    task_id: str
    title: str
    state: State = State.TASK_ACCEPTED
    head_sha: str = ""
    base_sha: str = ""
    actor_last: Optional[Actor] = None
    rca_status: RcaStatus = RcaStatus.UNKNOWN
    failure_class: Optional[FailureClass] = None
    resume_state: Optional[State] = None
    rework_target: Optional[State] = None
    readiness: bool = False
    evidence_ids: list[str] = field(default_factory=list)
    lane_id: str = ""
    path_prefixes: list[str] = field(default_factory=list)
    notes: str = ""
    # retry_key -> count for TRANSIENT_EXTERNAL_FAILURE only
    retry_counts: dict[str, int] = field(default_factory=dict)
    required_scopes: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "task_id": self.task_id,
            "title": self.title,
            "state": enum_value(self.state),
            "head_sha": self.head_sha,
            "base_sha": self.base_sha,
            "actor_last": enum_value(self.actor_last) if self.actor_last else None,
            "rca_status": enum_value(self.rca_status),
            "failure_class": enum_value(self.failure_class) if self.failure_class else None,
            "resume_state": enum_value(self.resume_state) if self.resume_state else None,
            "rework_target": enum_value(self.rework_target) if self.rework_target else None,
            "readiness": self.readiness,
            "evidence_ids": list(self.evidence_ids),
            "lane_id": self.lane_id,
            "path_prefixes": list(self.path_prefixes),
            "notes": self.notes,
            "retry_counts": dict(self.retry_counts),
            "required_scopes": list(self.required_scopes),
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "Task":
        actor_last = data.get("actor_last")
        failure_class = data.get("failure_class")
        resume_state = data.get("resume_state")
        rework_target = data.get("rework_target")
        return cls(
            task_id=str(data["task_id"]),
            title=str(data.get("title", "")),
            state=State(data.get("state", State.TASK_ACCEPTED.value)),
            head_sha=str(data.get("head_sha", "")),
            base_sha=str(data.get("base_sha", "")),
            actor_last=Actor(actor_last) if actor_last else None,
            rca_status=RcaStatus(data.get("rca_status", RcaStatus.UNKNOWN.value)),
            failure_class=FailureClass(failure_class) if failure_class else None,
            resume_state=State(resume_state) if resume_state else None,
            rework_target=State(rework_target) if rework_target else None,
            readiness=bool(data.get("readiness", False)),
            evidence_ids=list(data.get("evidence_ids") or []),
            lane_id=str(data.get("lane_id", "")),
            path_prefixes=list(data.get("path_prefixes") or []),
            notes=str(data.get("notes", "")),
            retry_counts=dict(data.get("retry_counts") or {}),
            required_scopes=list(data.get("required_scopes") or []),
        )

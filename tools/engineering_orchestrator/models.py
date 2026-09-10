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
    INDEPENDENT_REVIEWER = "INDEPENDENT_REVIEWER"
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
    INDEPENDENT_REVIEW = "INDEPENDENT_REVIEW"
    INFERENCE = "INFERENCE"
    RECOMMENDATION = "RECOMMENDATION"


class EvidenceResult(str, Enum):
    PASS = "PASS"
    FAIL = "FAIL"
    NOT_RUN = "NOT_RUN"
    SKIPPED = "SKIPPED"
    UNKNOWN = "UNKNOWN"
    PARTIAL = "PARTIAL"


class EvidenceAttestation(str, Enum):
    """Phase 1 attestation boundary.

    Only UNATTESTED exists. TRUSTED is intentionally absent (Phase 2).
    Hash/state integrity ≠ evidence attestation.
    """

    UNATTESTED = "UNATTESTED"


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


class GateApplicability(str, Enum):
    REQUIRED = "REQUIRED"
    NOT_APPLICABLE = "NOT_APPLICABLE"


class ReadinessTarget(str, Enum):
    IMPLEMENTED = "IMPLEMENTED"
    COMPONENT_TESTED = "COMPONENT_TESTED"
    FOUNDATION_SAFE_TO_LAND = "FOUNDATION_SAFE_TO_LAND"
    PILOT_READY = "PILOT_READY"
    PRODUCTION_READY = "PRODUCTION_READY"
    ENTERPRISE_GA_READY = "ENTERPRISE_GA_READY"


# Cumulative order: higher includes all lower applicable required gates.
READINESS_TARGET_ORDER: tuple[ReadinessTarget, ...] = (
    ReadinessTarget.IMPLEMENTED,
    ReadinessTarget.COMPONENT_TESTED,
    ReadinessTarget.FOUNDATION_SAFE_TO_LAND,
    ReadinessTarget.PILOT_READY,
    ReadinessTarget.PRODUCTION_READY,
    ReadinessTarget.ENTERPRISE_GA_READY,
)

FOUNDATION_OR_ABOVE = frozenset(
    {
        ReadinessTarget.FOUNDATION_SAFE_TO_LAND,
        ReadinessTarget.PILOT_READY,
        ReadinessTarget.PRODUCTION_READY,
        ReadinessTarget.ENTERPRISE_GA_READY,
    }
)

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

BUNDLE_SCHEMA_VERSION = 1


def enum_value(value: Enum | str) -> str:
    return value.value if isinstance(value, Enum) else str(value)


def is_structurally_valid_sha(value: str) -> bool:
    """Non-empty SHA-like token: no whitespace/control characters."""
    if not isinstance(value, str) or not value:
        return False
    if any(ch.isspace() or ord(ch) < 32 for ch in value):
        return False
    return True


def readiness_rank(target: ReadinessTarget) -> int:
    return READINESS_TARGET_ORDER.index(target)


@dataclass
class RequiredGate:
    gate_id: str
    claim_scope: str
    required_evidence_type: EvidenceType
    readiness_target: ReadinessTarget
    applicability: GateApplicability = GateApplicability.REQUIRED
    not_applicable_reason: str = ""

    def __post_init__(self) -> None:
        if self.applicability == GateApplicability.NOT_APPLICABLE:
            if not str(self.not_applicable_reason).strip():
                raise ValueError(
                    "NOT_APPLICABLE gate requires non-empty not_applicable_reason"
                )

    def to_dict(self) -> dict[str, Any]:
        return {
            "gate_id": self.gate_id,
            "claim_scope": self.claim_scope,
            "required_evidence_type": enum_value(self.required_evidence_type),
            "readiness_target": enum_value(self.readiness_target),
            "applicability": enum_value(self.applicability),
            "not_applicable_reason": self.not_applicable_reason,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "RequiredGate":
        applicability = GateApplicability(
            data.get("applicability", GateApplicability.REQUIRED.value)
        )
        reason = str(data.get("not_applicable_reason", ""))
        return cls(
            gate_id=str(data["gate_id"]),
            claim_scope=str(data.get("claim_scope", "")),
            required_evidence_type=EvidenceType(data["required_evidence_type"]),
            readiness_target=ReadinessTarget(data["readiness_target"]),
            applicability=applicability,
            not_applicable_reason=reason,
        )


@dataclass
class Evidence:
    evidence_id: str
    type: EvidenceType
    claim_scope: str
    head_sha: str
    result: EvidenceResult
    collected_at: str
    repository: str = ""
    branch: str = ""
    base_sha: str = ""
    paths: list[str] = field(default_factory=list)
    dependencies: list[str] = field(default_factory=list)
    command_identity: str = ""
    exit_code: Optional[int] = None
    worker_logical_identity: str = ""
    reviewer_logical_identity: str = ""
    ci_run_id: str = ""
    ci_job_id: str = ""
    reused_from_evidence_id: str = ""
    reuse_reason: str = ""
    attestation: EvidenceAttestation = EvidenceAttestation.UNATTESTED
    payload_ref: str = ""
    impact_known: bool = True
    invalidated: bool = False
    invalidation_reason: str = ""

    def to_dict(self) -> dict[str, Any]:
        return {
            "evidence_id": self.evidence_id,
            "type": enum_value(self.type),
            "claim_scope": self.claim_scope,
            "repository": self.repository,
            "branch": self.branch,
            "head_sha": self.head_sha,
            "base_sha": self.base_sha,
            "paths": list(self.paths),
            "dependencies": list(self.dependencies),
            "result": enum_value(self.result),
            "command_identity": self.command_identity,
            "exit_code": self.exit_code,
            "worker_logical_identity": self.worker_logical_identity,
            "reviewer_logical_identity": self.reviewer_logical_identity,
            "ci_run_id": self.ci_run_id,
            "ci_job_id": self.ci_job_id,
            "reused_from_evidence_id": self.reused_from_evidence_id,
            "reuse_reason": self.reuse_reason,
            "collected_at": self.collected_at,
            "attestation": enum_value(self.attestation),
            "payload_ref": self.payload_ref,
            "impact_known": self.impact_known,
            "invalidated": self.invalidated,
            "invalidation_reason": self.invalidation_reason,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "Evidence":
        attestation_raw = data.get("attestation", EvidenceAttestation.UNATTESTED.value)
        if attestation_raw != EvidenceAttestation.UNATTESTED.value:
            raise ValueError(
                f"Phase 1 rejects attestation={attestation_raw!r}; only UNATTESTED allowed"
            )
        exit_code = data.get("exit_code")
        return cls(
            evidence_id=str(data["evidence_id"]),
            type=EvidenceType(data["type"]),
            claim_scope=str(data.get("claim_scope", "")),
            repository=str(data.get("repository", "")),
            branch=str(data.get("branch", "")),
            head_sha=str(data["head_sha"]),
            base_sha=str(data.get("base_sha", "")),
            paths=list(data.get("paths") or []),
            dependencies=list(data.get("dependencies") or []),
            result=EvidenceResult(data["result"]),
            command_identity=str(data.get("command_identity", "")),
            exit_code=None if exit_code is None else int(exit_code),
            worker_logical_identity=str(data.get("worker_logical_identity", "")),
            reviewer_logical_identity=str(data.get("reviewer_logical_identity", "")),
            ci_run_id=str(data.get("ci_run_id", "")),
            ci_job_id=str(data.get("ci_job_id", "")),
            reused_from_evidence_id=str(data.get("reused_from_evidence_id", "")),
            reuse_reason=str(data.get("reuse_reason", "")),
            collected_at=str(data.get("collected_at", "")),
            attestation=EvidenceAttestation.UNATTESTED,
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
    repository: str = ""
    branch: str = ""
    actor_last: Optional[Actor] = None
    rca_status: RcaStatus = RcaStatus.UNKNOWN
    failure_class: Optional[FailureClass] = None
    resume_state: Optional[State] = None
    rework_target: Optional[State] = None
    readiness: bool = False
    readiness_target: ReadinessTarget = ReadinessTarget.IMPLEMENTED
    evidence_ids: list[str] = field(default_factory=list)
    required_gates: list[RequiredGate] = field(default_factory=list)
    lane_id: str = ""
    path_prefixes: list[str] = field(default_factory=list)
    notes: str = ""
    # retry_key -> count for TRANSIENT_EXTERNAL_FAILURE only
    retry_counts: dict[str, int] = field(default_factory=dict)
    required_scopes: list[str] = field(default_factory=list)
    migration_reservation_allowed: bool = False

    def to_dict(self) -> dict[str, Any]:
        return {
            "task_id": self.task_id,
            "title": self.title,
            "state": enum_value(self.state),
            "head_sha": self.head_sha,
            "base_sha": self.base_sha,
            "repository": self.repository,
            "branch": self.branch,
            "actor_last": enum_value(self.actor_last) if self.actor_last else None,
            "rca_status": enum_value(self.rca_status),
            "failure_class": enum_value(self.failure_class) if self.failure_class else None,
            "resume_state": enum_value(self.resume_state) if self.resume_state else None,
            "rework_target": enum_value(self.rework_target) if self.rework_target else None,
            "readiness": self.readiness,
            "readiness_target": enum_value(self.readiness_target),
            "evidence_ids": list(self.evidence_ids),
            "required_gates": [g.to_dict() for g in self.required_gates],
            "lane_id": self.lane_id,
            "path_prefixes": list(self.path_prefixes),
            "notes": self.notes,
            "retry_counts": dict(self.retry_counts),
            "required_scopes": list(self.required_scopes),
            "migration_reservation_allowed": self.migration_reservation_allowed,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "Task":
        actor_last = data.get("actor_last")
        failure_class = data.get("failure_class")
        resume_state = data.get("resume_state")
        rework_target = data.get("rework_target")
        gates_raw = data.get("required_gates") or []
        return cls(
            task_id=str(data["task_id"]),
            title=str(data.get("title", "")),
            state=State(data.get("state", State.TASK_ACCEPTED.value)),
            head_sha=str(data.get("head_sha", "")),
            base_sha=str(data.get("base_sha", "")),
            repository=str(data.get("repository", "")),
            branch=str(data.get("branch", "")),
            actor_last=Actor(actor_last) if actor_last else None,
            rca_status=RcaStatus(data.get("rca_status", RcaStatus.UNKNOWN.value)),
            failure_class=FailureClass(failure_class) if failure_class else None,
            resume_state=State(resume_state) if resume_state else None,
            rework_target=State(rework_target) if rework_target else None,
            readiness=bool(data.get("readiness", False)),
            readiness_target=ReadinessTarget(
                data.get("readiness_target", ReadinessTarget.IMPLEMENTED.value)
            ),
            evidence_ids=list(data.get("evidence_ids") or []),
            required_gates=[RequiredGate.from_dict(g) for g in gates_raw],
            lane_id=str(data.get("lane_id", "")),
            path_prefixes=list(data.get("path_prefixes") or []),
            notes=str(data.get("notes", "")),
            retry_counts=dict(data.get("retry_counts") or {}),
            required_scopes=list(data.get("required_scopes") or []),
            migration_reservation_allowed=bool(
                data.get("migration_reservation_allowed", False)
            ),
        )

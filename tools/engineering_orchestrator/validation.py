"""Stdlib-only strict structure validation for Engineering Orchestrator Phase 1.

No jsonschema dependency. Rejects unknown keys where schemas declare
additionalProperties=false. Keep aligned with schemas/*.json.
"""

from __future__ import annotations

from typing import Any

from .models import (
    Actor,
    EvidenceAttestation,
    EvidenceResult,
    EvidenceType,
    GateApplicability,
    MigrationReservationState,
    ReadinessTarget,
    State,
)


class ValidationError(ValueError):
    """Raised when persisted structure fails strict validation."""


def _require_object(data: Any, label: str) -> dict[str, Any]:
    if not isinstance(data, dict):
        raise ValidationError(f"{label} must be a JSON object")
    return data


def _reject_unknown(data: dict[str, Any], allowed: frozenset[str], label: str) -> None:
    unknown = set(data.keys()) - allowed
    if unknown:
        raise ValidationError(
            f"{label} has unknown keys: {sorted(unknown)}"
        )


def _require_keys(data: dict[str, Any], required: frozenset[str], label: str) -> None:
    missing = required - set(data.keys())
    if missing:
        raise ValidationError(f"{label} missing required keys: {sorted(missing)}")


BUNDLE_KEYS = frozenset(
    {
        "schema_version",
        "generation",
        "task",
        "journal",
        "evidence",
        "lanes",
        "migrations",
        "handoffs",
    }
)
BUNDLE_REQUIRED = frozenset(
    {
        "schema_version",
        "generation",
        "task",
        "journal",
        "evidence",
        "lanes",
        "migrations",
        "handoffs",
    }
)

TASK_KEYS = frozenset(
    {
        "task_id",
        "title",
        "state",
        "head_sha",
        "base_sha",
        "repository",
        "branch",
        "actor_last",
        "rca_status",
        "failure_class",
        "resume_state",
        "rework_target",
        "readiness",
        "readiness_target",
        "evidence_ids",
        "required_gates",
        "lane_id",
        "path_prefixes",
        "notes",
        "retry_counts",
        "required_scopes",
        "migration_reservation_allowed",
    }
)
TASK_REQUIRED = frozenset({"task_id", "title", "state"})

EVENT_KEYS = frozenset(
    {
        "sequence",
        "event_id",
        "task_id",
        "from_state",
        "to_state",
        "head_sha",
        "base_sha",
        "evidence_ids",
        "actor",
        "timestamp",
        "previous_event_hash",
        "event_hash",
        "resume_state",
        "notes",
        "expected_head_sha",
        "expected_base_sha",
        "changed_paths",
    }
)
EVENT_REQUIRED = frozenset(
    {
        "sequence",
        "event_id",
        "task_id",
        "from_state",
        "to_state",
        "head_sha",
        "base_sha",
        "evidence_ids",
        "actor",
        "timestamp",
        "previous_event_hash",
        "event_hash",
    }
)

EVIDENCE_KEYS = frozenset(
    {
        "evidence_id",
        "type",
        "claim_scope",
        "repository",
        "branch",
        "head_sha",
        "base_sha",
        "paths",
        "dependencies",
        "result",
        "command_identity",
        "exit_code",
        "worker_logical_identity",
        "reviewer_logical_identity",
        "ci_run_id",
        "ci_job_id",
        "reused_from_evidence_id",
        "reuse_reason",
        "collected_at",
        "attestation",
        "payload_ref",
        "impact_known",
        "invalidated",
        "invalidation_reason",
    }
)
EVIDENCE_REQUIRED = frozenset(
    {
        "evidence_id",
        "type",
        "claim_scope",
        "head_sha",
        "result",
        "collected_at",
        "attestation",
    }
)

HANDOFF_KEYS = frozenset(
    {
        "handoff_id",
        "task_id",
        "from_state",
        "expected_return_state",
        "actor_role",
        "instructions_ref",
        "inputs",
        "outputs",
        "codex_enabled",
        "status",
    }
)
HANDOFF_REQUIRED = frozenset(
    {
        "handoff_id",
        "task_id",
        "from_state",
        "expected_return_state",
        "actor_role",
        "codex_enabled",
        "status",
    }
)

LANE_KEYS = frozenset({"lane_id", "task_id", "path_prefixes", "active"})
LANE_REQUIRED = frozenset({"lane_id", "task_id", "path_prefixes"})

MIGRATION_KEYS = frozenset(
    {"reservation_id", "migration_id", "lane_id", "task_id", "state", "head_sha"}
)
MIGRATION_REQUIRED = frozenset(
    {"reservation_id", "migration_id", "task_id", "state"}
)

GATE_KEYS = frozenset(
    {
        "gate_id",
        "claim_scope",
        "required_evidence_type",
        "readiness_target",
        "applicability",
        "not_applicable_reason",
    }
)
GATE_REQUIRED = frozenset(
    {
        "gate_id",
        "claim_scope",
        "required_evidence_type",
        "readiness_target",
        "applicability",
    }
)


def validate_required_gate(data: Any) -> dict[str, Any]:
    obj = _require_object(data, "required_gate")
    _reject_unknown(obj, GATE_KEYS, "required_gate")
    _require_keys(obj, GATE_REQUIRED, "required_gate")
    try:
        EvidenceType(obj["required_evidence_type"])
        ReadinessTarget(obj["readiness_target"])
        applicability = GateApplicability(obj["applicability"])
    except ValueError as exc:
        raise ValidationError(f"required_gate enum invalid: {exc}") from exc
    if applicability == GateApplicability.NOT_APPLICABLE:
        reason = str(obj.get("not_applicable_reason", "")).strip()
        if not reason:
            raise ValidationError(
                "NOT_APPLICABLE required_gate needs not_applicable_reason"
            )
    return obj


def validate_task(data: Any) -> dict[str, Any]:
    obj = _require_object(data, "task")
    _reject_unknown(obj, TASK_KEYS, "task")
    _require_keys(obj, TASK_REQUIRED, "task")
    try:
        State(obj["state"])
    except ValueError as exc:
        raise ValidationError(f"task.state invalid: {exc}") from exc
    if "readiness_target" in obj:
        try:
            ReadinessTarget(obj["readiness_target"])
        except ValueError as exc:
            raise ValidationError(f"task.readiness_target invalid: {exc}") from exc
    if "actor_last" in obj and obj["actor_last"] is not None:
        try:
            Actor(obj["actor_last"])
        except ValueError as exc:
            raise ValidationError(f"task.actor_last invalid: {exc}") from exc
    gates = obj.get("required_gates")
    if gates is not None:
        if not isinstance(gates, list):
            raise ValidationError("task.required_gates must be an array")
        for g in gates:
            validate_required_gate(g)
    return obj


def validate_event(data: Any) -> dict[str, Any]:
    obj = _require_object(data, "event")
    _reject_unknown(obj, EVENT_KEYS, "event")
    _require_keys(obj, EVENT_REQUIRED, "event")
    try:
        Actor(obj["actor"])
        State(obj["from_state"])
        State(obj["to_state"])
    except ValueError as exc:
        raise ValidationError(f"event enum invalid: {exc}") from exc
    if not isinstance(obj["evidence_ids"], list):
        raise ValidationError("event.evidence_ids must be an array")
    if "changed_paths" in obj and not isinstance(obj["changed_paths"], list):
        raise ValidationError("event.changed_paths must be an array")
    return obj


def validate_evidence(data: Any) -> dict[str, Any]:
    obj = _require_object(data, "evidence")
    _reject_unknown(obj, EVIDENCE_KEYS, "evidence")
    _require_keys(obj, EVIDENCE_REQUIRED, "evidence")
    try:
        EvidenceType(obj["type"])
        EvidenceResult(obj["result"])
        attestation = EvidenceAttestation(obj["attestation"])
    except ValueError as exc:
        raise ValidationError(f"evidence enum invalid: {exc}") from exc
    if attestation != EvidenceAttestation.UNATTESTED:
        raise ValidationError("Phase 1 evidence attestation must be UNATTESTED")
    return obj


def validate_handoff(data: Any) -> dict[str, Any]:
    obj = _require_object(data, "handoff")
    _reject_unknown(obj, HANDOFF_KEYS, "handoff")
    _require_keys(obj, HANDOFF_REQUIRED, "handoff")
    if not isinstance(obj["codex_enabled"], bool):
        raise ValidationError("handoff.codex_enabled must be a boolean")
    return obj


def validate_lane(data: Any) -> dict[str, Any]:
    obj = _require_object(data, "lane")
    _reject_unknown(obj, LANE_KEYS, "lane")
    _require_keys(obj, LANE_REQUIRED, "lane")
    if not isinstance(obj["path_prefixes"], list):
        raise ValidationError("lane.path_prefixes must be an array")
    return obj


def validate_migration(data: Any) -> dict[str, Any]:
    obj = _require_object(data, "migration")
    _reject_unknown(obj, MIGRATION_KEYS, "migration")
    _require_keys(obj, MIGRATION_REQUIRED, "migration")
    try:
        MigrationReservationState(obj["state"])
    except ValueError as exc:
        raise ValidationError(f"migration.state invalid: {exc}") from exc
    return obj


def validate_bundle(data: Any) -> dict[str, Any]:
    obj = _require_object(data, "bundle")
    _reject_unknown(obj, BUNDLE_KEYS, "bundle")
    _require_keys(obj, BUNDLE_REQUIRED, "bundle")
    if not isinstance(obj["schema_version"], int) or obj["schema_version"] < 1:
        raise ValidationError("bundle.schema_version must be a positive integer")
    if not isinstance(obj["generation"], int) or obj["generation"] < 0:
        raise ValidationError("bundle.generation must be a non-negative integer")
    if obj["task"] is not None:
        validate_task(obj["task"])
    if not isinstance(obj["journal"], list):
        raise ValidationError("bundle.journal must be an array")
    for event in obj["journal"]:
        validate_event(event)
    if not isinstance(obj["evidence"], list):
        raise ValidationError("bundle.evidence must be an array")
    for ev in obj["evidence"]:
        validate_evidence(ev)
    if not isinstance(obj["lanes"], list):
        raise ValidationError("bundle.lanes must be an array")
    for lane in obj["lanes"]:
        validate_lane(lane)
    if not isinstance(obj["migrations"], list):
        raise ValidationError("bundle.migrations must be an array")
    for mig in obj["migrations"]:
        validate_migration(mig)
    if not isinstance(obj["handoffs"], list):
        raise ValidationError("bundle.handoffs must be an array")
    for ho in obj["handoffs"]:
        validate_handoff(ho)
    return obj

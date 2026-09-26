"""Shared helpers for Engineering Orchestrator Phase 1 tests."""

from __future__ import annotations

from tools.engineering_orchestrator.engine import OrchestratorEngine
from tools.engineering_orchestrator.models import (
    Actor,
    EvidenceResult,
    EvidenceType,
    GateApplicability,
    ReadinessTarget,
    RequiredGate,
    State,
)
from tools.engineering_orchestrator.store import OrchestratorStore

# B4 strict Skill headings for pack-safety fixtures (must match skill_structure.py).
FIXTURE_SKILL_HEADINGS = (
    "Purpose",
    "Use when",
    "Authoritative inputs",
    "Invariants",
    "Required procedure",
    "Negative / failure cases",
    "Security / tenant implications",
    "Required evidence",
    "Stop conditions",
    "Anti-patterns",
    "Handoff / output",
    "Non-ownership",
)


def fixture_skill_markdown(name: str, description: str) -> str:
    """Minimal structurally valid canonical Skill body for temporary pack fixtures."""
    parts = [f"---\nname: {name}\ndescription: {description}\n---\n", f"# {name}\n"]
    for heading in FIXTURE_SKILL_HEADINGS:
        parts.append(f"## {heading}\n\nFixture {heading} body for `{name}`.\n")
    return "\n".join(parts)


def engine(tmp: str) -> OrchestratorEngine:
    return OrchestratorEngine(OrchestratorStore(tmp))


def default_gates(
    *,
    target: ReadinessTarget = ReadinessTarget.IMPLEMENTED,
) -> list[RequiredGate]:
    return [
        RequiredGate(
            gate_id="g-local",
            claim_scope="default",
            required_evidence_type=EvidenceType.LOCAL_TEST,
            readiness_target=ReadinessTarget.IMPLEMENTED,
            applicability=GateApplicability.REQUIRED,
        ),
    ]


def foundation_gates() -> list[RequiredGate]:
    return [
        RequiredGate(
            gate_id="g-local",
            claim_scope="default",
            required_evidence_type=EvidenceType.LOCAL_TEST,
            readiness_target=ReadinessTarget.IMPLEMENTED,
        ),
        RequiredGate(
            gate_id="g-ir",
            claim_scope="default",
            required_evidence_type=EvidenceType.INDEPENDENT_REVIEW,
            readiness_target=ReadinessTarget.FOUNDATION_SAFE_TO_LAND,
        ),
    ]


def advance_to_full_validation(eng: OrchestratorEngine) -> None:
    order = [
        State.SOURCE_INSPECTION,
        State.AGENT_ROUTING,
        State.RCA_COMPLETE,
        State.IMPLEMENTATION,
        State.FOCUSED_VALIDATION,
        State.FULL_VALIDATION,
    ]
    for state in order:
        assert eng.task is not None
        eng.transition(state, Actor.WRITER, expected_state=eng.task.state)


def reconcile_to_independent_review(
    eng: OrchestratorEngine,
    *,
    new_head: str | None = None,
) -> None:
    assert eng.task is not None
    old_head = eng.task.head_sha
    old_base = eng.task.base_sha
    eng.record_evidence(
        evidence_id="src-verify",
        evidence_type=EvidenceType.SOURCE,
        head_sha=old_head,
        claim_scope="default",
        result=EvidenceResult.PASS,
        collected_at="2026-01-01T00:00:00Z",
        command_identity="source-inspect",
    )
    nh = new_head or old_head
    nb = old_base or "b1"
    eng.reconcile_base(
        expected_state=State.FULL_VALIDATION,
        expected_head_sha=old_head,
        expected_base_sha=old_base,
        new_head_sha=nh,
        new_base_sha=nb,
        changed_paths=["tools/engineering_orchestrator/engine.py"],
        verification_evidence_ids=["src-verify"],
        impact_known=True,
    )
    assert eng.task is not None
    eng.transition(
        State.INDEPENDENT_REVIEW,
        Actor.SYSTEM,
        expected_state=State.BASE_RECONCILIATION,
    )


def advance_through_ir_and_ci(eng: OrchestratorEngine) -> None:
    assert eng.task is not None
    head = eng.task.head_sha
    eng.record_evidence(
        evidence_id="ir1",
        evidence_type=EvidenceType.INDEPENDENT_REVIEW,
        head_sha=head,
        claim_scope="default",
        result=EvidenceResult.PASS,
        collected_at="2026-01-01T00:00:00Z",
        reviewer_logical_identity="reviewer-1",
    )
    eng.transition(
        State.EXACT_HEAD_CI,
        Actor.INDEPENDENT_REVIEWER,
        expected_state=State.INDEPENDENT_REVIEW,
    )
    eng.record_evidence(
        evidence_id="ci1",
        evidence_type=EvidenceType.CI,
        head_sha=head,
        claim_scope="default",
        result=EvidenceResult.PASS,
        collected_at="2026-01-01T00:00:00Z",
        ci_run_id="run-1",
        ci_job_id="job-1",
        command_identity="platform-ci",
    )
    eng.transition(
        State.REALITY_GATE,
        Actor.SYSTEM,
        expected_state=State.EXACT_HEAD_CI,
    )

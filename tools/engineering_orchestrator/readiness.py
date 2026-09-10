"""Readiness and RequiredGate evaluation.

100%-pass ≠ readiness. CI alone never promotes readiness.
Empty applicable gate set ≠ vacuous 100% PASS.
ENTERPRISE_GA_READY is a logical target, not product GA certification.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Iterable

from .evidence import has_independent_review_pass, is_usable
from .models import (
    Actor,
    Evidence,
    EvidenceResult,
    EvidenceType,
    FOUNDATION_OR_ABOVE,
    GateApplicability,
    ReadinessTarget,
    RequiredGate,
    Task,
    readiness_rank,
)


@dataclass
class PassEvaluation:
    all_required_pass: bool
    missing_scopes: list[str]
    stale_or_invalid: list[str]
    details: str


@dataclass
class ReadinessEvaluation:
    readiness: bool
    hundred_percent_pass: bool
    reason: str


NON_PASS_RESULTS = frozenset(
    {
        EvidenceResult.FAIL,
        EvidenceResult.NOT_RUN,
        EvidenceResult.SKIPPED,
        EvidenceResult.UNKNOWN,
        EvidenceResult.PARTIAL,
    }
)


def applicable_gates_for_target(
    gates: list[RequiredGate],
    target: ReadinessTarget,
) -> list[RequiredGate]:
    """Gates whose readiness_target rank is <= task target, and REQUIRED."""
    rank = readiness_rank(target)
    result: list[RequiredGate] = []
    for g in gates:
        if g.applicability != GateApplicability.REQUIRED:
            continue
        if readiness_rank(g.readiness_target) <= rank:
            result.append(g)
    return result


def evaluate_hundred_percent_pass(
    task: Task,
    evidence: Iterable[Evidence],
) -> PassEvaluation:
    """Every applicable RequiredGate has PASS evidence at exact current head.

    No implicit LOCAL_TEST-per-scope default. Empty applicable set is NOT 100%.
    """
    evidence_list = list(evidence)
    applicable = applicable_gates_for_target(task.required_gates, task.readiness_target)
    if not applicable:
        return PassEvaluation(
            all_required_pass=False,
            missing_scopes=["<no applicable RequiredGate>"],
            stale_or_invalid=[],
            details="empty applicable gate set is not 100% PASS",
        )

    missing: list[str] = []
    stale: list[str] = []

    for gate in applicable:
        matches = [
            ev
            for ev in evidence_list
            if ev.claim_scope == gate.claim_scope
            and ev.type == gate.required_evidence_type
        ]
        usable = [ev for ev in matches if is_usable(ev, task.head_sha)]
        non_pass = [
            ev
            for ev in matches
            if not ev.invalidated
            and ev.head_sha == task.head_sha
            and ev.result in NON_PASS_RESULTS
        ]
        if usable:
            continue
        if non_pass:
            stale.append(
                f"{gate.gate_id}:{gate.claim_scope}:{gate.required_evidence_type.value}"
                f"={non_pass[0].result.value}"
            )
        elif matches:
            stale.append(
                f"{gate.gate_id}:{gate.claim_scope}:{gate.required_evidence_type.value}"
            )
        else:
            missing.append(
                f"{gate.gate_id}:{gate.claim_scope}:{gate.required_evidence_type.value}"
            )

    ok = not missing and not stale
    details = "all applicable RequiredGates PASS at exact head" if ok else "gaps present"
    return PassEvaluation(
        all_required_pass=ok,
        missing_scopes=missing,
        stale_or_invalid=stale,
        details=details,
    )


def evaluate_readiness(
    task: Task,
    evidence: Iterable[Evidence],
    *,
    actor: Actor,
    reality_gate_pass: bool,
    independent_review_completed: bool = False,
) -> ReadinessEvaluation:
    """Readiness requires Reality Checker gate + RequiredGates + target rules.

    CI alone never promotes readiness. Writer cannot set readiness.
    FOUNDATION_SAFE_TO_LAND+ needs IR evidence, IR completion, Reality PASS,
    and at least one qualifying non-CI gate.
    """
    if actor != Actor.REALITY_CHECKER:
        return ReadinessEvaluation(
            readiness=False,
            hundred_percent_pass=False,
            reason="only REALITY_CHECKER may promote readiness",
        )

    pass_eval = evaluate_hundred_percent_pass(task, evidence)
    if not reality_gate_pass:
        return ReadinessEvaluation(
            readiness=False,
            hundred_percent_pass=pass_eval.all_required_pass,
            reason="Reality Checker gate PASS required",
        )

    evidence_list = list(evidence)
    applicable = applicable_gates_for_target(task.required_gates, task.readiness_target)
    # CI alone never promotes: every applicable RequiredGate is CI-typed,
    # or the only usable evidence bag is CI (caller passed CI-only list).
    usable = [ev for ev in evidence_list if is_usable(ev, task.head_sha)]
    applicable_all_ci = bool(applicable) and all(
        g.required_evidence_type == EvidenceType.CI for g in applicable
    )
    evidence_only_ci = bool(usable) and all(ev.type == EvidenceType.CI for ev in usable)
    if applicable_all_ci or evidence_only_ci:
        return ReadinessEvaluation(
            readiness=False,
            hundred_percent_pass=pass_eval.all_required_pass,
            reason="CI alone never promotes readiness",
        )

    if not pass_eval.all_required_pass:
        return ReadinessEvaluation(
            readiness=False,
            hundred_percent_pass=False,
            reason=f"100%-pass not met: missing={pass_eval.missing_scopes} "
            f"stale={pass_eval.stale_or_invalid}",
        )

    if task.readiness_target in FOUNDATION_OR_ABOVE:
        if not has_independent_review_pass(evidence_list, task.head_sha):
            return ReadinessEvaluation(
                readiness=False,
                hundred_percent_pass=pass_eval.all_required_pass,
                reason="FOUNDATION_SAFE_TO_LAND+ requires current INDEPENDENT_REVIEW PASS",
            )
        if not independent_review_completed:
            return ReadinessEvaluation(
                readiness=False,
                hundred_percent_pass=pass_eval.all_required_pass,
                reason="FOUNDATION_SAFE_TO_LAND+ requires INDEPENDENT_REVIEWER completion",
            )
        # Qualifying non-CI *gate* (not incidental SOURCE/IR evidence).
        non_ci_gates = [
            g for g in applicable if g.required_evidence_type != EvidenceType.CI
        ]
        if not non_ci_gates:
            return ReadinessEvaluation(
                readiness=False,
                hundred_percent_pass=pass_eval.all_required_pass,
                reason="FOUNDATION_SAFE_TO_LAND+ requires ≥1 qualifying non-CI gate",
            )
        non_ci_gate_satisfied = False
        for gate in non_ci_gates:
            matches = [
                ev
                for ev in evidence_list
                if ev.claim_scope == gate.claim_scope
                and ev.type == gate.required_evidence_type
                and is_usable(ev, task.head_sha)
            ]
            if matches:
                non_ci_gate_satisfied = True
                break
        if not non_ci_gate_satisfied:
            return ReadinessEvaluation(
                readiness=False,
                hundred_percent_pass=pass_eval.all_required_pass,
                reason="FOUNDATION_SAFE_TO_LAND+ requires ≥1 qualifying non-CI gate",
            )

    if task.failure_class is not None and task.state.value == "BLOCKED":
        return ReadinessEvaluation(
            readiness=False,
            hundred_percent_pass=True,
            reason="open blocker present",
        )

    return ReadinessEvaluation(
        readiness=True,
        hundred_percent_pass=True,
        reason="Reality Checker gate PASS with required evidence at exact head",
    )


def ci_alone_promotes_readiness() -> bool:
    """Invariant helper for tests/docs."""
    return False


def validate_gate_definitions(gates: list[RequiredGate]) -> None:
    for g in gates:
        if g.applicability == GateApplicability.NOT_APPLICABLE:
            if not g.not_applicable_reason.strip():
                raise ValueError(
                    f"gate {g.gate_id}: NOT_APPLICABLE requires not_applicable_reason"
                )

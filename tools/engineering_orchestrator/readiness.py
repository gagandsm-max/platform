"""Readiness and 100%-pass evaluation.

100%-pass ≠ readiness. CI alone never promotes readiness.
Reality Checker gate is required for readiness=true.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Iterable

from .evidence import is_usable
from .models import Actor, Evidence, EvidenceType, Task


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


def evaluate_hundred_percent_pass(
    task: Task,
    evidence: Iterable[Evidence],
    required_types_by_scope: dict[str, set[EvidenceType]] | None = None,
) -> PassEvaluation:
    """Every required check for declared claim_scope has PASS at exact current head.

    This alone does NOT promote readiness.
    """
    evidence_list = list(evidence)
    scopes = list(task.required_scopes) or ["default"]
    missing: list[str] = []
    stale: list[str] = []

    # Default: require LOCAL_TEST PASS at current head for each required scope.
    type_map = required_types_by_scope or {
        scope: {EvidenceType.LOCAL_TEST} for scope in scopes
    }

    for scope, types in type_map.items():
        for etype in types:
            matches = [
                ev
                for ev in evidence_list
                if ev.claim_scope == scope and ev.type == etype
            ]
            usable = [ev for ev in matches if is_usable(ev, task.head_sha)]
            if not matches:
                missing.append(f"{scope}:{etype.value}")
            elif not usable:
                stale.append(f"{scope}:{etype.value}")

    ok = not missing and not stale
    details = "all required checks PASS at exact head" if ok else "gaps present"
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
) -> ReadinessEvaluation:
    """Readiness requires Reality Checker gate + 100%-pass + no open blockers.

    CI alone never promotes readiness. Writer cannot set readiness.
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

    # CI-only evidence set is insufficient even if CI PASS at head.
    evidence_list = list(evidence)
    usable = [ev for ev in evidence_list if is_usable(ev, task.head_sha)]
    only_ci = bool(usable) and all(ev.type == EvidenceType.CI for ev in usable)
    if only_ci:
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

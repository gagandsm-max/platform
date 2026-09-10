"""Evidence types, forbidden promotions, and invalidation rules.

New HEAD always invalidates CI. Uncertain impact → conservative invalidation.
"""

from __future__ import annotations

from .models import Evidence, EvidenceResult, EvidenceType

# Forbidden promotions: (from_type, to_type) or semantic tags.
FORBIDDEN_PROMOTIONS: frozenset[tuple[EvidenceType, EvidenceType]] = frozenset(
    {
        (EvidenceType.LOCAL_TEST, EvidenceType.CI),
        (EvidenceType.SOURCE, EvidenceType.LOCAL_RUNTIME),
        (EvidenceType.INFERENCE, EvidenceType.LOCAL_TEST),
        (EvidenceType.INFERENCE, EvidenceType.LOCAL_RUNTIME),
        (EvidenceType.INFERENCE, EvidenceType.CI),
        (EvidenceType.INFERENCE, EvidenceType.SECURITY),
        (EvidenceType.INFERENCE, EvidenceType.DATABASE),
        (EvidenceType.RECOMMENDATION, EvidenceType.LOCAL_TEST),
        (EvidenceType.RECOMMENDATION, EvidenceType.LOCAL_RUNTIME),
        (EvidenceType.RECOMMENDATION, EvidenceType.CI),
        (EvidenceType.RECOMMENDATION, EvidenceType.SECURITY),
        (EvidenceType.RECOMMENDATION, EvidenceType.DATABASE),
        (EvidenceType.RECOMMENDATION, EvidenceType.SOURCE),
        (EvidenceType.INFERENCE, EvidenceType.SOURCE),
    }
)

# Types that may be reused across head change only when impact_known=True.
SAFE_REUSE_ACROSS_HEAD: frozenset[EvidenceType] = frozenset(
    {
        EvidenceType.SOURCE,  # still scoped; rarely reused without impact_known
    }
)

CONSERVATIVE_INVALIDATE_TYPES: frozenset[EvidenceType] = frozenset(
    {
        EvidenceType.LOCAL_TEST,
        EvidenceType.LOCAL_RUNTIME,
        EvidenceType.CI,
        EvidenceType.DATABASE,
        EvidenceType.SECURITY,
    }
)


class EvidenceError(ValueError):
    """Raised for illegal evidence promotion or misuse."""


def assert_promotion_allowed(from_type: EvidenceType, to_type: EvidenceType) -> None:
    if (from_type, to_type) in FORBIDDEN_PROMOTIONS:
        raise EvidenceError(
            f"forbidden evidence promotion: {from_type.value} -> {to_type.value}"
        )
    # Semantic bans expressed as type tags.
    if from_type == EvidenceType.CI and to_type in {
        # CI cannot become a "readiness" surrogate type; readiness is not an
        # evidence type — callers must not treat CI as readiness proof.
        EvidenceType.CI,
    }:
        # same-type re-label is fine; readiness gate is elsewhere
        pass
    if from_type in {EvidenceType.INFERENCE, EvidenceType.RECOMMENDATION}:
        if to_type not in {EvidenceType.INFERENCE, EvidenceType.RECOMMENDATION}:
            raise EvidenceError(
                f"{from_type.value} cannot become execution evidence ({to_type.value})"
            )


def assert_not_static_config_as_runtime(claim: str, evidence_type: EvidenceType) -> None:
    claim_l = claim.lower()
    if evidence_type == EvidenceType.LOCAL_RUNTIME and (
        "static_config" in claim_l or "config_only" in claim_l
    ):
        raise EvidenceError(
            "static config cannot be promoted to runtime enforcement evidence"
        )


def assert_ci_not_readiness(evidence_type: EvidenceType, claim_scope: str) -> None:
    if evidence_type == EvidenceType.CI and "production_readiness" in claim_scope.lower():
        raise EvidenceError("CI evidence cannot claim production readiness")


def invalidate_for_new_head(
    evidence_list: list[Evidence],
    new_head: str,
    *,
    impact_known: bool = True,
) -> list[Evidence]:
    """Return updated evidence with invalidation applied for a new HEAD.

    Rule: new HEAD always invalidates CI.
    Uncertain impact → conservative invalidation of overlapping types.
    Safe reuse only when same head OR (impact_known and type allowlisted).
    """
    updated: list[Evidence] = []
    for ev in evidence_list:
        if ev.invalidated:
            updated.append(ev)
            continue
        if ev.head_sha == new_head:
            updated.append(ev)
            continue

        reason = ""
        if ev.type == EvidenceType.CI:
            reason = "new HEAD invalidates CI evidence"
        elif not impact_known and ev.type in CONSERVATIVE_INVALIDATE_TYPES:
            reason = "uncertain impact — conservative invalidation"
        elif impact_known and ev.type in SAFE_REUSE_ACROSS_HEAD:
            # allow reuse of SOURCE when impact known
            updated.append(ev)
            continue
        else:
            reason = "head_sha changed — evidence stale for current HEAD"

        ev.invalidated = True
        ev.invalidation_reason = reason
        updated.append(ev)
    return updated


def is_usable(ev: Evidence, current_head: str) -> bool:
    if ev.invalidated:
        return False
    if ev.result != EvidenceResult.PASS:
        return False
    if ev.type == EvidenceType.CI and ev.head_sha != current_head:
        return False
    if ev.head_sha != current_head and ev.type in CONSERVATIVE_INVALIDATE_TYPES:
        return False
    return True


def has_fresh_ci(evidence_list: list[Evidence], head_sha: str) -> bool:
    return any(
        is_usable(ev, head_sha) and ev.type == EvidenceType.CI and ev.head_sha == head_sha
        for ev in evidence_list
    )

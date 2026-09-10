"""Evidence types, forbidden promotions, and invalidation rules.

New HEAD always invalidates CI. Uncertain impact → conservative invalidation.
Path/dependency metadata drives known-impact invalidation. Explicit reuse
requires reused_from_evidence_id + reuse_reason.
"""

from __future__ import annotations

from .models import Evidence, EvidenceResult, EvidenceType
from .paths import paths_overlap, validate_relative_path

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
        (EvidenceType.INFERENCE, EvidenceType.INDEPENDENT_REVIEW),
        (EvidenceType.RECOMMENDATION, EvidenceType.LOCAL_TEST),
        (EvidenceType.RECOMMENDATION, EvidenceType.LOCAL_RUNTIME),
        (EvidenceType.RECOMMENDATION, EvidenceType.CI),
        (EvidenceType.RECOMMENDATION, EvidenceType.SECURITY),
        (EvidenceType.RECOMMENDATION, EvidenceType.DATABASE),
        (EvidenceType.RECOMMENDATION, EvidenceType.SOURCE),
        (EvidenceType.RECOMMENDATION, EvidenceType.INDEPENDENT_REVIEW),
        (EvidenceType.INFERENCE, EvidenceType.SOURCE),
    }
)

EXECUTION_OR_REVIEW_TYPES: frozenset[EvidenceType] = frozenset(
    {
        EvidenceType.LOCAL_TEST,
        EvidenceType.LOCAL_RUNTIME,
        EvidenceType.CI,
        EvidenceType.DATABASE,
        EvidenceType.SECURITY,
        EvidenceType.INDEPENDENT_REVIEW,
    }
)

DEPENDENCY_CATEGORIES: frozenset[str] = frozenset(
    {
        "@build_descriptor",
        "@test_harness",
        "@workflow",
        "@runtime_config",
        "@migration_manifest",
        "@shared_contract",
    }
)


class EvidenceError(ValueError):
    """Raised for illegal evidence promotion or misuse."""


def assert_promotion_allowed(from_type: EvidenceType, to_type: EvidenceType) -> None:
    if (from_type, to_type) in FORBIDDEN_PROMOTIONS:
        raise EvidenceError(
            f"forbidden evidence promotion: {from_type.value} -> {to_type.value}"
        )
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


def assert_provenance(ev: Evidence) -> None:
    """Require meaningful provenance for execution/review evidence types."""
    if ev.type == EvidenceType.CI:
        if not ev.ci_run_id or not ev.ci_job_id:
            raise EvidenceError("CI evidence requires ci_run_id and ci_job_id")
        if not ev.command_identity:
            raise EvidenceError("CI evidence requires command_identity")
    if ev.type == EvidenceType.INDEPENDENT_REVIEW:
        if not ev.reviewer_logical_identity:
            raise EvidenceError(
                "INDEPENDENT_REVIEW evidence requires reviewer_logical_identity"
            )
    if ev.type in {
        EvidenceType.LOCAL_TEST,
        EvidenceType.LOCAL_RUNTIME,
        EvidenceType.DATABASE,
        EvidenceType.SECURITY,
    }:
        if not ev.command_identity and not ev.worker_logical_identity:
            raise EvidenceError(
                f"{ev.type.value} evidence requires command_identity or "
                "worker_logical_identity"
            )
    if ev.reused_from_evidence_id and not ev.reuse_reason.strip():
        raise EvidenceError("explicit reuse requires reuse_reason")
    if ev.reuse_reason and not ev.reused_from_evidence_id:
        raise EvidenceError("reuse_reason requires reused_from_evidence_id")


def path_to_dependency_categories(path: str) -> set[str]:
    """Map a repository-relative path to closed dependency categories."""
    try:
        norm = validate_relative_path(path)
    except ValueError:
        return set(DEPENDENCY_CATEGORIES)  # conservative
    lower = norm.lower()
    cats: set[str] = set()
    if lower.startswith(".github/workflows/") or "/.github/workflows/" in f"/{lower}":
        cats.add("@workflow")
    if (
        lower == "makefile"
        or lower.endswith("/makefile")
        or lower.endswith("dockerfile")
        or "dockerfile" in lower.split("/")[-1]
        or lower.endswith(".mk")
        or "build." in lower.split("/")[-1]
    ):
        cats.add("@build_descriptor")
    if (
        "test" in lower.split("/")
        or lower.startswith("tests/")
        or "run_tests" in lower
        or lower.endswith("/conftest.py")
    ):
        cats.add("@test_harness")
    if (
        "docker-compose" in lower
        or lower.endswith(".env.example")
        or lower.startswith("deploy/")
        or "compose" in lower.split("/")[-1]
    ):
        cats.add("@runtime_config")
    if "migration" in lower or lower.startswith("migrations/"):
        cats.add("@migration_manifest")
    if (
        lower.endswith(".schema.json")
        or "/schemas/" in f"/{lower}"
        or lower.endswith(".proto")
        or "contract" in lower
    ):
        cats.add("@shared_contract")
    return cats


def changed_paths_to_categories(changed_paths: list[str]) -> set[str]:
    cats: set[str] = set()
    for p in changed_paths:
        cats |= path_to_dependency_categories(p)
    return cats


def _path_intersects(evidence_paths: list[str], changed_paths: list[str]) -> bool:
    for ep in evidence_paths:
        for cp in changed_paths:
            try:
                if paths_overlap(ep, cp) or paths_overlap(cp, ep):
                    return True
            except ValueError:
                # Conservative: bad path metadata → treat as intersect.
                return True
            # Also simple prefix/substring for file-level equality.
            if ep == cp:
                return True
    return False


def _deps_intersect(evidence_deps: list[str], changed_cats: set[str]) -> bool:
    for dep in evidence_deps:
        if dep in DEPENDENCY_CATEGORIES and dep in changed_cats:
            return True
    return False


def invalidate_for_new_head(
    evidence_list: list[Evidence],
    new_head: str,
    *,
    impact_known: bool = True,
    changed_paths: list[str] | None = None,
) -> list[Evidence]:
    """Return updated evidence with invalidation applied for a new HEAD.

    Rules:
    - CI always invalidates on new HEAD.
    - Unknown impact or missing changed_paths → conservative invalidation of
      execution/review evidence.
    - Known impact → invalidate on path intersection or dependency category hit.
    - Evidence lacking dependency metadata (when known impact) → conservative.
    - Old-head evidence is never auto-promoted; reuse must be explicit.
    """
    paths = list(changed_paths) if changed_paths is not None else []
    changed_cats = changed_paths_to_categories(paths) if paths else set()
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
        elif not impact_known or not paths:
            if ev.type in EXECUTION_OR_REVIEW_TYPES:
                reason = "uncertain impact — conservative invalidation"
            else:
                # SOURCE / INFERENCE / RECOMMENDATION: still stale for head
                # unless explicit reuse later.
                reason = "head_sha changed — evidence stale for current HEAD"
        else:
            # Known impact with paths.
            insufficient_meta = (
                ev.type in EXECUTION_OR_REVIEW_TYPES
                and not ev.paths
                and not ev.dependencies
            )
            if insufficient_meta:
                reason = "insufficient dependency metadata — conservative invalidation"
            elif _path_intersects(ev.paths, paths) or _deps_intersect(
                ev.dependencies, changed_cats
            ):
                reason = "changed path/dependency intersects evidence scope"
            elif ev.type in EXECUTION_OR_REVIEW_TYPES:
                # No intersection: still not auto-current; mark stale without
                # allowing unsafe old-head promotion via is_usable.
                reason = "head_sha changed — evidence stale for current HEAD"
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
    if ev.head_sha != current_head:
        return False
    return True


def has_fresh_ci(evidence_list: list[Evidence], head_sha: str) -> bool:
    return any(
        is_usable(ev, head_sha) and ev.type == EvidenceType.CI and ev.head_sha == head_sha
        for ev in evidence_list
    )


def has_independent_review_pass(
    evidence_list: list[Evidence], head_sha: str
) -> bool:
    return any(
        is_usable(ev, head_sha)
        and ev.type == EvidenceType.INDEPENDENT_REVIEW
        and bool(ev.reviewer_logical_identity)
        for ev in evidence_list
    )


def create_reuse(
    source: Evidence,
    *,
    new_evidence_id: str,
    new_head: str,
    reuse_reason: str,
    collected_at: str,
) -> Evidence:
    """Explicit reuse: new record referencing prior evidence (never silent)."""
    if not reuse_reason.strip():
        raise EvidenceError("reuse_reason required")
    return Evidence(
        evidence_id=new_evidence_id,
        type=source.type,
        claim_scope=source.claim_scope,
        repository=source.repository,
        branch=source.branch,
        head_sha=new_head,
        base_sha=source.base_sha,
        paths=list(source.paths),
        dependencies=list(source.dependencies),
        result=source.result,
        command_identity=source.command_identity,
        exit_code=source.exit_code,
        worker_logical_identity=source.worker_logical_identity,
        reviewer_logical_identity=source.reviewer_logical_identity,
        ci_run_id=source.ci_run_id,
        ci_job_id=source.ci_job_id,
        reused_from_evidence_id=source.evidence_id,
        reuse_reason=reuse_reason,
        collected_at=collected_at,
        attestation=source.attestation,
        payload_ref=source.payload_ref,
        impact_known=True,
    )

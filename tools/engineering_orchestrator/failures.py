"""Failure classes and bounded retry policy.

Only TRANSIENT_EXTERNAL_FAILURE may auto-retry, with MAX=2.
No retry-until-green. Unknown/INCONCLUSIVE RCA never auto-REWORK.
"""

from __future__ import annotations

from .models import FailureClass, RcaStatus, State

MAX_TRANSIENT_RETRIES = 2

REWORK_ELIGIBLE = frozenset(
    {
        FailureClass.SOURCE_DEFECT,
        FailureClass.TEST_DEFECT,
        FailureClass.CONFIG_DEFECT,
    }
)

ALWAYS_BLOCK = frozenset(
    {
        FailureClass.ENVIRONMENT_DEFECT,
        FailureClass.EVIDENCE_GAP,
        FailureClass.SECURITY_BLOCKER,
        FailureClass.ARCHITECTURE_BLOCKER,
        FailureClass.MIGRATION_COLLISION,
        FailureClass.BUSINESS_DECISION_REQUIRED,
    }
)


class FailurePolicyError(ValueError):
    """Raised when a failure disposition is illegal."""


def retry_key(task_id: str, state: State, failure_class: FailureClass) -> str:
    return f"{task_id}|{state.value}|{failure_class.value}"


def allows_bounded_retry(failure_class: FailureClass) -> bool:
    return failure_class == FailureClass.TRANSIENT_EXTERNAL_FAILURE


def can_auto_rework(failure_class: FailureClass, rca_status: RcaStatus) -> bool:
    if rca_status != RcaStatus.KNOWN:
        return False
    return failure_class in REWORK_ELIGIBLE


def next_retry_count(current: int, failure_class: FailureClass) -> int:
    if not allows_bounded_retry(failure_class):
        raise FailurePolicyError(
            f"{failure_class.value} is not retryable (no retry-until-green)"
        )
    nxt = current + 1
    if nxt > MAX_TRANSIENT_RETRIES:
        raise FailurePolicyError(
            f"transient retry budget exhausted (MAX={MAX_TRANSIENT_RETRIES})"
        )
    return nxt


def disposition_for_failure(
    failure_class: FailureClass,
    rca_status: RcaStatus,
    retry_count: int,
) -> str:
    """Return recommended next disposition: RETRY | REWORK | BLOCKED | FAILED.

    RETRY means remain in the same state after recording the transient failure
    (engine increments counter; does not invent a new FSM state).
    """
    if failure_class == FailureClass.TRANSIENT_EXTERNAL_FAILURE:
        if retry_count < MAX_TRANSIENT_RETRIES:
            return "RETRY"
        return "BLOCKED"

    if failure_class in ALWAYS_BLOCK:
        return "BLOCKED"

    if can_auto_rework(failure_class, rca_status):
        return "REWORK"

    # Unknown / INCONCLUSIVE RCA: never automatic REWORK.
    if rca_status in {RcaStatus.UNKNOWN, RcaStatus.INCONCLUSIVE}:
        return "BLOCKED"

    return "BLOCKED"

"""Founder / CTO status projection for Engineering Orchestrator Phase 1."""

from __future__ import annotations

from typing import Any

from .models import FailureClass, State, Task, TERMINAL_STATES


def project_founder_status(task: Task, *, journal_len: int = 0) -> dict[str, Any]:
    """Compact status projection for Founder/CTO review gates."""
    waiting_on = _waiting_on(task)
    return {
        "task_id": task.task_id,
        "title": task.title,
        "state": task.state.value,
        "head_sha": task.head_sha,
        "base_sha": task.base_sha,
        "readiness": task.readiness,
        "rca_status": task.rca_status.value,
        "failure_class": task.failure_class.value if task.failure_class else None,
        "resume_state": task.resume_state.value if task.resume_state else None,
        "terminal": task.state in TERMINAL_STATES,
        "blocked": task.state == State.BLOCKED,
        "waiting_on": waiting_on,
        "founder_action_required": waiting_on in {
            "FOUNDER_CTO",
            "BUSINESS_DECISION",
            "SECURITY_REVIEW",
            "ARCHITECTURE_REVIEW",
            "MIGRATION_COORDINATION",
        },
        "journal_events": journal_len,
        "evidence_count": len(task.evidence_ids),
        "lane_id": task.lane_id,
    }


def _waiting_on(task: Task) -> str:
    if task.state == State.MERGED:
        return "NONE"
    if task.state == State.READY_FOR_MERGE_GATE:
        return "MERGE_AUTHORITY"
    if task.state == State.REALITY_GATE:
        return "REALITY_CHECKER"
    if task.state == State.INDEPENDENT_REVIEW:
        return "INDEPENDENT_REVIEWER"
    if task.state == State.BLOCKED:
        if task.failure_class == FailureClass.BUSINESS_DECISION_REQUIRED:
            return "BUSINESS_DECISION"
        if task.failure_class == FailureClass.SECURITY_BLOCKER:
            return "SECURITY_REVIEW"
        if task.failure_class == FailureClass.ARCHITECTURE_BLOCKER:
            return "ARCHITECTURE_REVIEW"
        if task.failure_class == FailureClass.MIGRATION_COLLISION:
            return "MIGRATION_COORDINATION"
        return "FOUNDER_CTO"
    if task.state in TERMINAL_STATES:
        return "NONE"
    return "WRITER"

"""Orchestration engine: transitions, evidence, resume, failure disposition."""

from __future__ import annotations

from typing import Any, Optional

from .authority import AuthorityError, authorize_transition, founder_may_unblock
from .evidence import (
    EvidenceError,
    assert_ci_not_readiness,
    assert_not_static_config_as_runtime,
    assert_promotion_allowed,
    has_fresh_ci,
    invalidate_for_new_head,
)
from .failures import (
    FailurePolicyError,
    can_auto_rework,
    disposition_for_failure,
    next_retry_count,
    retry_key,
)
from .fsm import REWORK_REENTRY_TARGETS, TransitionError, is_terminal
from .handoff import Handoff, create_handoff
from .journal import Journal, JournalError
from .migration_model import MigrationCollisionError, MigrationReservationBook
from .models import (
    Actor,
    Evidence,
    EvidenceResult,
    EvidenceType,
    FailureClass,
    RcaStatus,
    State,
    Task,
)
from .paths import Lane, LaneOwnershipError, LaneRegistry, validate_relative_path
from .readiness import evaluate_hundred_percent_pass, evaluate_readiness
from .status import project_founder_status
from .store import OrchestratorStore


class EngineError(ValueError):
    """Raised for orchestration policy violations."""


class OrchestratorEngine:
    def __init__(self, store: OrchestratorStore) -> None:
        self.store = store
        self.task: Optional[Task] = store.load_task()
        self.journal: Journal = store.load_journal()
        self.evidence: list[Evidence] = store.load_evidence()
        self.lanes: LaneRegistry = store.load_lanes()
        self.migrations: MigrationReservationBook = store.load_migrations()
        self.handoffs: list[Handoff] = store.load_handoffs()

    def persist(self) -> None:
        if self.task is not None:
            self.store.save_task(self.task)
        self.store.save_journal(self.journal)
        self.store.save_evidence(self.evidence)
        self.store.save_lanes(self.lanes)
        self.store.save_migrations(self.migrations)
        self.store.save_handoffs(self.handoffs)

    def init_task(
        self,
        *,
        task_id: str,
        title: str,
        head_sha: str = "",
        base_sha: str = "",
        lane_id: str = "",
        path_prefixes: list[str] | None = None,
        required_scopes: list[str] | None = None,
    ) -> Task:
        if self.task is not None:
            raise EngineError("task already initialized in this store")
        prefixes = [validate_relative_path(p) for p in (path_prefixes or [])]
        task = Task(
            task_id=task_id,
            title=title,
            state=State.TASK_ACCEPTED,
            head_sha=head_sha,
            base_sha=base_sha,
            lane_id=lane_id,
            path_prefixes=prefixes,
            required_scopes=list(required_scopes or ["default"]),
        )
        if lane_id and prefixes:
            self.lanes.claim(
                Lane(lane_id=lane_id, task_id=task_id, path_prefixes=prefixes, active=True)
            )
        self.task = task
        self.persist()
        return task

    def record_evidence(
        self,
        *,
        evidence_id: str,
        evidence_type: EvidenceType,
        head_sha: str,
        claim_scope: str,
        result: EvidenceResult,
        collected_at: str,
        payload_ref: str = "",
        impact_known: bool = True,
        promote_from: EvidenceType | None = None,
    ) -> Evidence:
        task = self._require_task()
        if payload_ref:
            validate_relative_path(payload_ref)
        if promote_from is not None:
            assert_promotion_allowed(promote_from, evidence_type)
        assert_not_static_config_as_runtime(claim_scope, evidence_type)
        assert_ci_not_readiness(evidence_type, claim_scope)

        ev = Evidence(
            evidence_id=evidence_id,
            type=evidence_type,
            head_sha=head_sha,
            claim_scope=claim_scope,
            result=result,
            collected_at=collected_at,
            payload_ref=payload_ref,
            impact_known=impact_known,
        )
        # Replace same id if re-recorded.
        self.evidence = [e for e in self.evidence if e.evidence_id != evidence_id]
        self.evidence.append(ev)
        if evidence_id not in task.evidence_ids:
            task.evidence_ids.append(evidence_id)
        self.persist()
        return ev

    def set_head(
        self,
        new_head: str,
        *,
        impact_known: bool = True,
    ) -> None:
        task = self._require_task()
        if new_head == task.head_sha:
            return
        self.evidence = invalidate_for_new_head(
            self.evidence,
            new_head,
            impact_known=impact_known,
        )
        task.head_sha = new_head
        task.readiness = False
        self.persist()

    def transition(
        self,
        to_state: State,
        actor: Actor,
        *,
        evidence_ids: list[str] | None = None,
        resume_state: State | None = None,
        rework_target: State | None = None,
        failure_class: FailureClass | None = None,
        rca_status: RcaStatus | None = None,
        reality_gate_pass: bool = False,
        notes: str = "",
        head_sha: str | None = None,
        base_sha: str | None = None,
        impact_known: bool = True,
    ) -> Task:
        task = self._require_task()
        from_state = task.state

        if head_sha is not None and head_sha != task.head_sha:
            self.set_head(head_sha, impact_known=impact_known)
        if base_sha is not None:
            task.base_sha = base_sha
        if rca_status is not None:
            task.rca_status = rca_status
        if failure_class is not None:
            task.failure_class = failure_class

        # Special: BLOCKED resume to recorded resume_state.
        if from_state == State.BLOCKED and to_state not in {
            State.CANCELLED,
            State.FAILED,
        }:
            return self._resume_blocked(to_state, actor, notes=notes)

        # Policy gates before authority table.
        if to_state == State.REWORK:
            self._assert_rework_allowed(task, failure_class or task.failure_class)

        if to_state in REWORK_REENTRY_TARGETS and from_state == State.REWORK:
            target = rework_target or to_state
            if target not in REWORK_REENTRY_TARGETS:
                raise EngineError(f"illegal rework target {target.value}")
            to_state = target

        if to_state == State.BLOCKED:
            task.resume_state = resume_state or from_state

        if to_state == State.READY_FOR_MERGE_GATE:
            self._assert_ready_for_merge(task, actor, reality_gate_pass=reality_gate_pass)

        if from_state == State.INDEPENDENT_REVIEW and to_state == State.EXACT_HEAD_CI:
            if not has_fresh_ci(self.evidence, task.head_sha):
                # Allow entering EXACT_HEAD_CI to *run* CI; freshness checked
                # leaving EXACT_HEAD_CI → REALITY_GATE.
                pass

        if from_state == State.EXACT_HEAD_CI and to_state == State.REALITY_GATE:
            if not has_fresh_ci(self.evidence, task.head_sha):
                raise EngineError("EXACT_HEAD_CI requires fresh CI evidence for current HEAD")

        try:
            authorize_transition(from_state, to_state, actor)
        except (AuthorityError, TransitionError):
            raise

        # Clear readiness unless Reality Checker just granted it.
        if to_state != State.READY_FOR_MERGE_GATE:
            if to_state not in {State.MERGED, State.NOT_MERGED}:
                if actor != Actor.REALITY_CHECKER:
                    pass

        if to_state == State.READY_FOR_MERGE_GATE:
            task.readiness = True
        if to_state in {State.REWORK, State.BLOCKED, State.FAILED, State.CANCELLED}:
            task.readiness = False

        if from_state == State.REWORK and to_state in REWORK_REENTRY_TARGETS:
            task.rework_target = None
            task.failure_class = None

        # Reset transient retry counter when leaving a state successfully.
        if to_state not in {State.BLOCKED, State.FAILED, State.REWORK, State.CANCELLED}:
            keys = [k for k in task.retry_counts if k.startswith(f"{task.task_id}|{from_state.value}|")]
            for k in keys:
                del task.retry_counts[k]

        ids = list(evidence_ids) if evidence_ids is not None else list(task.evidence_ids)
        self.journal.append(
            task_id=task.task_id,
            from_state=from_state,
            to_state=to_state,
            head_sha=task.head_sha,
            base_sha=task.base_sha,
            evidence_ids=ids,
            actor=actor,
            resume_state=task.resume_state if to_state == State.BLOCKED else None,
            notes=notes,
        )
        task.state = to_state
        task.actor_last = actor
        self.persist()
        return task

    def apply_failure(
        self,
        failure_class: FailureClass,
        actor: Actor,
        *,
        rca_status: RcaStatus | None = None,
        rework_target: State | None = None,
        notes: str = "",
    ) -> dict[str, Any]:
        """Apply failure disposition: RETRY (same state), REWORK, BLOCKED, or FAILED."""
        task = self._require_task()
        if rca_status is not None:
            task.rca_status = rca_status
        task.failure_class = failure_class

        key = retry_key(task.task_id, task.state, failure_class)
        current = int(task.retry_counts.get(key, 0))
        disposition = disposition_for_failure(failure_class, task.rca_status, current)

        if disposition == "RETRY":
            try:
                task.retry_counts[key] = next_retry_count(current, failure_class)
            except FailurePolicyError:
                disposition = "BLOCKED"
            else:
                self.persist()
                return {
                    "disposition": "RETRY",
                    "retry_count": task.retry_counts[key],
                    "state": task.state.value,
                }

        if disposition == "REWORK":
            if rework_target:
                task.rework_target = rework_target
            self.transition(State.REWORK, actor, notes=notes or f"failure={failure_class.value}")
            return {"disposition": "REWORK", "state": task.state.value}

        if disposition == "BLOCKED":
            self.transition(
                State.BLOCKED,
                actor,
                resume_state=task.state,
                failure_class=failure_class,
                notes=notes or f"failure={failure_class.value}",
            )
            return {"disposition": "BLOCKED", "state": task.state.value}

        self.transition(State.FAILED, actor, notes=notes or f"failure={failure_class.value}")
        return {"disposition": "FAILED", "state": task.state.value}

    def reserve_migration(
        self,
        *,
        reservation_id: str,
        migration_id: str,
        lane_id: str = "",
        head_sha: str = "",
    ) -> dict[str, Any]:
        task = self._require_task()
        try:
            reservation = self.migrations.reserve(
                reservation_id=reservation_id,
                migration_id=migration_id,
                lane_id=lane_id or task.lane_id,
                task_id=task.task_id,
                head_sha=head_sha or task.head_sha,
            )
            self.persist()
            return reservation.to_dict()
        except MigrationCollisionError:
            self.apply_failure(
                FailureClass.MIGRATION_COLLISION,
                Actor.SYSTEM,
                rca_status=RcaStatus.KNOWN,
                notes=f"migration collision on {migration_id}",
            )
            raise

    def create_handoff(
        self,
        *,
        handoff_id: str,
        actor_role: str,
        expected_return_state: str,
        instructions_ref: str = "",
        inputs: dict[str, Any] | None = None,
        codex_enabled: bool = False,
    ) -> Handoff:
        task = self._require_task()
        handoff = create_handoff(
            handoff_id=handoff_id,
            task_id=task.task_id,
            from_state=task.state.value,
            expected_return_state=expected_return_state,
            actor_role=actor_role,
            instructions_ref=instructions_ref,
            inputs=inputs,
            codex_enabled=codex_enabled,
        )
        self.handoffs.append(handoff)
        self.persist()
        return handoff

    def status(self) -> dict[str, Any]:
        task = self._require_task()
        pass_eval = evaluate_hundred_percent_pass(task, self.evidence)
        projection = project_founder_status(task, journal_len=len(self.journal.events))
        projection["hundred_percent_pass"] = pass_eval.all_required_pass
        projection["pass_details"] = {
            "missing": pass_eval.missing_scopes,
            "stale_or_invalid": pass_eval.stale_or_invalid,
            "details": pass_eval.details,
        }
        return projection

    def replay(self) -> dict[str, Any]:
        self.journal.validate(enforce_transitions=True)
        if not self.journal.events:
            return {"ok": True, "state": None, "events": 0}
        last = self.journal.events[-1]
        return {
            "ok": True,
            "task_id": last.task_id,
            "state": last.to_state,
            "events": len(self.journal.events),
            "head_sha": last.head_sha,
            "base_sha": last.base_sha,
        }

    def validate_journal(self) -> dict[str, Any]:
        try:
            self.journal.validate(enforce_transitions=True)
            return {"ok": True, "events": len(self.journal.events)}
        except JournalError as exc:
            return {"ok": False, "error": str(exc), "events": len(self.journal.events)}

    def claim_lane(self, lane_id: str, path_prefixes: list[str]) -> None:
        task = self._require_task()
        prefixes = [validate_relative_path(p) for p in path_prefixes]
        try:
            self.lanes.claim(
                Lane(lane_id=lane_id, task_id=task.task_id, path_prefixes=prefixes, active=True)
            )
        except LaneOwnershipError:
            self.apply_failure(
                FailureClass.MIGRATION_COLLISION,
                Actor.SYSTEM,
                notes="lane overlap",
            )
            raise
        task.lane_id = lane_id
        task.path_prefixes = prefixes
        self.persist()

    def _resume_blocked(self, to_state: State, actor: Actor, *, notes: str = "") -> Task:
        task = self._require_task()
        if not founder_may_unblock(actor):
            raise AuthorityError("only FOUNDER_CTO or SYSTEM may resume from BLOCKED")
        if task.resume_state is None:
            raise EngineError("BLOCKED task missing resume_state")
        if to_state != task.resume_state:
            raise EngineError(
                f"resume target {to_state.value} != recorded resume_state "
                f"{task.resume_state.value}"
            )
        if is_terminal(to_state):
            raise EngineError("cannot resume into terminal state")

        from_state = State.BLOCKED
        self.journal.append(
            task_id=task.task_id,
            from_state=from_state,
            to_state=to_state,
            head_sha=task.head_sha,
            base_sha=task.base_sha,
            evidence_ids=list(task.evidence_ids),
            actor=actor,
            resume_state=to_state,
            notes=notes or "resume from BLOCKED",
        )
        task.state = to_state
        task.actor_last = actor
        task.failure_class = None
        # Keep resume_state for audit; clear open blocker.
        self.persist()
        return task

    def _assert_rework_allowed(
        self,
        task: Task,
        failure_class: FailureClass | None,
    ) -> None:
        if task.rca_status in {RcaStatus.UNKNOWN, RcaStatus.INCONCLUSIVE}:
            raise EngineError(
                "unknown/INCONCLUSIVE RCA — no automatic REWORK"
            )
        if failure_class is None:
            raise EngineError("REWORK requires a failure_class")
        if not can_auto_rework(failure_class, task.rca_status):
            raise EngineError(
                f"failure class {failure_class.value} is not rework-eligible "
                f"with rca_status={task.rca_status.value}"
            )

    def _assert_ready_for_merge(
        self,
        task: Task,
        actor: Actor,
        *,
        reality_gate_pass: bool,
    ) -> None:
        if actor != Actor.REALITY_CHECKER:
            raise AuthorityError("only REALITY_CHECKER may enter READY_FOR_MERGE_GATE")
        evaluation = evaluate_readiness(
            task,
            self.evidence,
            actor=actor,
            reality_gate_pass=reality_gate_pass,
        )
        if not evaluation.readiness:
            raise EngineError(f"readiness denied: {evaluation.reason}")

    def _require_task(self) -> Task:
        if self.task is None:
            raise EngineError("no task initialized")
        return self.task

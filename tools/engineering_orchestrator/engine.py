"""Orchestration engine: transitions, evidence, resume, failure disposition."""

from __future__ import annotations

from typing import Any, Optional

from .authority import AuthorityError, authorize_transition, may_resume_blocked
from .evidence import (
    EvidenceError,
    assert_ci_not_readiness,
    assert_not_static_config_as_runtime,
    assert_promotion_allowed,
    assert_provenance,
    has_fresh_ci,
    has_independent_review_pass,
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
    EvidenceAttestation,
    EvidenceResult,
    EvidenceType,
    FailureClass,
    RcaStatus,
    ReadinessTarget,
    RequiredGate,
    State,
    Task,
    is_structurally_valid_sha,
)
from .paths import Lane, LaneOwnershipError, LaneRegistry, validate_relative_path
from .readiness import evaluate_hundred_percent_pass, evaluate_readiness, validate_gate_definitions
from .status import project_founder_status
from .store import BundleState, OrchestratorStore, StoreError


class EngineError(ValueError):
    """Raised for orchestration policy violations."""


class OrchestratorEngine:
    def __init__(self, store: OrchestratorStore) -> None:
        self.store = store
        bundle = store.load_bundle()
        self._apply_loaded_bundle(bundle)

    def _apply_loaded_bundle(self, bundle: BundleState) -> None:
        """Validate journal first, replay projection, require task agreement."""
        self.generation = bundle.generation
        self.journal: Journal = bundle.journal
        try:
            self.journal.validate(enforce_transitions=True)
        except JournalError as exc:
            raise EngineError(f"journal validation failed on load: {exc}") from exc

        projection = self.journal.replay_projection()
        task = bundle.task

        if projection is None:
            if task is None:
                self.task = None
            else:
                if task.state != State.TASK_ACCEPTED:
                    raise EngineError(
                        "empty journal only permits TASK_ACCEPTED task projection"
                    )
                self.task = task
        else:
            if task is None:
                raise EngineError("journal present but task projection missing")
            if task.task_id != projection.task_id:
                raise EngineError("task/journal task_id mismatch")
            if task.state != projection.state:
                raise EngineError(
                    f"task/journal state mismatch: task={task.state.value} "
                    f"journal={projection.state.value}"
                )
            if task.head_sha != projection.head_sha:
                raise EngineError(
                    f"task/journal HEAD mismatch: task={task.head_sha!r} "
                    f"journal={projection.head_sha!r}"
                )
            if task.base_sha != projection.base_sha:
                raise EngineError(
                    f"task/journal base mismatch: task={task.base_sha!r} "
                    f"journal={projection.base_sha!r}"
                )
            # Resume field must agree with journal when BLOCKED.
            if task.state == State.BLOCKED:
                journal_resume = self.journal.journal_resume_target()
                if journal_resume is None:
                    raise EngineError("BLOCKED task missing journal resume target")
                if task.resume_state != journal_resume:
                    raise EngineError(
                        "task resume_state does not match journal resume target"
                    )
            self.task = task

        self.evidence: list[Evidence] = bundle.evidence
        self.lanes: LaneRegistry = bundle.lanes
        self.migrations: MigrationReservationBook = bundle.migrations
        self.handoffs: list[Handoff] = bundle.handoffs

        # Validate evidence/lane/migration/handoff references after agreement.
        if self.task is not None:
            known = {ev.evidence_id for ev in self.evidence}
            for eid in self.task.evidence_ids:
                if eid not in known:
                    raise EngineError(f"task references missing evidence_id {eid}")
            try:
                validate_gate_definitions(self.task.required_gates)
            except ValueError as exc:
                raise EngineError(str(exc)) from exc

    def _candidate(self) -> BundleState:
        return BundleState(
            schema_version=1,
            generation=self.generation,
            task=self.task,
            journal=self.journal,
            evidence=self.evidence,
            lanes=self.lanes,
            migrations=self.migrations,
            handoffs=self.handoffs,
        ).deep_copy()

    def _commit(self, candidate: BundleState) -> None:
        published = self.store.commit_bundle(
            candidate, expected_generation=self.generation
        )
        self._apply_loaded_bundle(published)

    def persist(self) -> None:
        """Commit current in-memory state as a new generation (legacy helper)."""
        candidate = self._candidate()
        self._commit(candidate)

    def init_task(
        self,
        *,
        task_id: str,
        title: str,
        head_sha: str = "",
        base_sha: str = "",
        repository: str = "",
        branch: str = "",
        lane_id: str = "",
        path_prefixes: list[str] | None = None,
        required_scopes: list[str] | None = None,
        required_gates: list[RequiredGate] | None = None,
        readiness_target: ReadinessTarget = ReadinessTarget.IMPLEMENTED,
        migration_reservation_allowed: bool = False,
    ) -> Task:
        if self.task is not None:
            raise EngineError("task already initialized in this store")
        if self.journal.events:
            raise EngineError("cannot init_task when journal is non-empty")
        prefixes = [validate_relative_path(p) for p in (path_prefixes or [])]
        gates = list(required_gates or [])
        validate_gate_definitions(gates)
        task = Task(
            task_id=task_id,
            title=title,
            state=State.TASK_ACCEPTED,
            head_sha=head_sha,
            base_sha=base_sha,
            repository=repository,
            branch=branch,
            lane_id=lane_id,
            path_prefixes=prefixes,
            required_scopes=list(required_scopes or []),
            required_gates=gates,
            readiness_target=readiness_target,
            migration_reservation_allowed=migration_reservation_allowed,
        )
        candidate = self._candidate()
        if lane_id and prefixes:
            candidate.lanes.claim(
                Lane(lane_id=lane_id, task_id=task_id, path_prefixes=prefixes, active=True)
            )
        candidate.task = task
        self._commit(candidate)
        return self._require_task()

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
        paths: list[str] | None = None,
        dependencies: list[str] | None = None,
        command_identity: str = "",
        exit_code: int | None = None,
        worker_logical_identity: str = "",
        reviewer_logical_identity: str = "",
        ci_run_id: str = "",
        ci_job_id: str = "",
        reused_from_evidence_id: str = "",
        reuse_reason: str = "",
        base_sha: str = "",
    ) -> Evidence:
        task = self._require_task()
        if payload_ref:
            validate_relative_path(payload_ref)
        if promote_from is not None:
            assert_promotion_allowed(promote_from, evidence_type)
        assert_not_static_config_as_runtime(claim_scope, evidence_type)
        assert_ci_not_readiness(evidence_type, claim_scope)

        path_list = [validate_relative_path(p) for p in (paths or [])]
        ev = Evidence(
            evidence_id=evidence_id,
            type=evidence_type,
            claim_scope=claim_scope,
            repository=task.repository,
            branch=task.branch,
            head_sha=head_sha,
            base_sha=base_sha or task.base_sha,
            paths=path_list,
            dependencies=list(dependencies or []),
            result=result,
            command_identity=command_identity,
            exit_code=exit_code,
            worker_logical_identity=worker_logical_identity,
            reviewer_logical_identity=reviewer_logical_identity,
            ci_run_id=ci_run_id,
            ci_job_id=ci_job_id,
            reused_from_evidence_id=reused_from_evidence_id,
            reuse_reason=reuse_reason,
            collected_at=collected_at,
            attestation=EvidenceAttestation.UNATTESTED,
            payload_ref=payload_ref,
            impact_known=impact_known,
        )
        assert_provenance(ev)

        candidate = self._candidate()
        assert candidate.task is not None
        candidate.evidence = [e for e in candidate.evidence if e.evidence_id != evidence_id]
        candidate.evidence.append(ev)
        if evidence_id not in candidate.task.evidence_ids:
            candidate.task.evidence_ids.append(evidence_id)
        self._commit(candidate)
        return next(e for e in self.evidence if e.evidence_id == evidence_id)

    def transition(
        self,
        to_state: State,
        actor: Actor,
        *,
        expected_state: State,
        evidence_ids: list[str] | None = None,
        rework_target: State | None = None,
        failure_class: FailureClass | None = None,
        rca_status: RcaStatus | None = None,
        reality_gate_pass: bool = False,
        notes: str = "",
    ) -> Task:
        """Ordinary transition with expected_state CAS. No HEAD/base mutation."""
        task = self._require_task()
        from_state = task.state

        # CAS: mismatch → zero mutation.
        if from_state != expected_state:
            raise EngineError(
                f"expected_state CAS mismatch: expected={expected_state.value} "
                f"actual={from_state.value}"
            )

        if to_state == State.BASE_RECONCILIATION:
            raise EngineError(
                "generic transition cannot enter BASE_RECONCILIATION; "
                "use reconcile_base()"
            )

        # Special: BLOCKED resume to journal-recorded resume_state.
        if from_state == State.BLOCKED and to_state not in {
            State.CANCELLED,
            State.FAILED,
        }:
            return self._resume_blocked(to_state, actor, notes=notes)

        # Authorize BEFORE any mutation.
        if to_state == State.REWORK:
            self._assert_rework_allowed(task, failure_class or task.failure_class)

        if to_state in REWORK_REENTRY_TARGETS and from_state == State.REWORK:
            target = rework_target or to_state
            if target not in REWORK_REENTRY_TARGETS:
                raise EngineError(f"illegal rework target {target.value}")
            to_state = target

        if to_state == State.READY_FOR_MERGE_GATE:
            self._assert_ready_for_merge(task, actor, reality_gate_pass=reality_gate_pass)

        if from_state == State.INDEPENDENT_REVIEW and to_state == State.EXACT_HEAD_CI:
            if not has_independent_review_pass(self.evidence, task.head_sha):
                raise EngineError(
                    "INDEPENDENT_REVIEW → EXACT_HEAD_CI requires current "
                    "INDEPENDENT_REVIEW PASS with reviewer_logical_identity"
                )

        if from_state == State.EXACT_HEAD_CI and to_state == State.REALITY_GATE:
            if not has_fresh_ci(self.evidence, task.head_sha):
                raise EngineError(
                    "EXACT_HEAD_CI requires fresh CI evidence for current HEAD"
                )

        try:
            authorize_transition(from_state, to_state, actor)
        except (AuthorityError, TransitionError):
            raise

        # Build candidate only after all checks pass.
        candidate = self._candidate()
        assert candidate.task is not None
        ctask = candidate.task

        if rca_status is not None:
            ctask.rca_status = rca_status
        if failure_class is not None:
            ctask.failure_class = failure_class

        if to_state == State.BLOCKED:
            # Force resume_state = from_state; ignore caller-supplied target.
            ctask.resume_state = from_state

        if to_state == State.READY_FOR_MERGE_GATE:
            ctask.readiness = True
        if to_state in {State.REWORK, State.BLOCKED, State.FAILED, State.CANCELLED}:
            ctask.readiness = False

        if from_state == State.REWORK and to_state in REWORK_REENTRY_TARGETS:
            ctask.rework_target = None
            ctask.failure_class = None

        if to_state not in {State.BLOCKED, State.FAILED, State.REWORK, State.CANCELLED}:
            keys = [
                k
                for k in ctask.retry_counts
                if k.startswith(f"{ctask.task_id}|{from_state.value}|")
            ]
            for k in keys:
                del ctask.retry_counts[k]

        ids = list(evidence_ids) if evidence_ids is not None else list(ctask.evidence_ids)
        candidate.journal.append(
            task_id=ctask.task_id,
            from_state=from_state,
            to_state=to_state,
            head_sha=ctask.head_sha,
            base_sha=ctask.base_sha,
            evidence_ids=ids,
            actor=actor,
            resume_state=ctask.resume_state if to_state == State.BLOCKED else None,
            notes=notes,
        )
        ctask.state = to_state
        ctask.actor_last = actor
        self._commit(candidate)
        return self._require_task()

    def reconcile_base(
        self,
        *,
        expected_state: State,
        expected_head_sha: str,
        expected_base_sha: str,
        new_head_sha: str,
        new_base_sha: str,
        changed_paths: list[str],
        verification_evidence_ids: list[str],
        impact_known: bool,
        actor: Actor = Actor.SYSTEM,
    ) -> Task:
        """Dedicated FULL_VALIDATION → BASE_RECONCILIATION with SHA updates.

        Contract A–M: CAS on state/head/base, SOURCE verification, candidate
        invalidate+update, one journal event, atomic commit. Zero mutation on fail.
        """
        task = self._require_task()

        # A–D
        if expected_state != State.FULL_VALIDATION:
            raise EngineError("reconcile_base expected_state must be FULL_VALIDATION")
        if task.state != expected_state:
            raise EngineError(
                f"reconcile CAS state mismatch: expected={expected_state.value} "
                f"actual={task.state.value}"
            )
        if task.head_sha != expected_head_sha:
            raise EngineError("reconcile expected_head_sha mismatch")
        if task.base_sha != expected_base_sha:
            raise EngineError("reconcile expected_base_sha mismatch")

        # E
        if not is_structurally_valid_sha(new_head_sha):
            raise EngineError("new_head_sha structurally invalid")
        if not is_structurally_valid_sha(new_base_sha):
            raise EngineError("new_base_sha structurally invalid")

        if actor != Actor.SYSTEM:
            raise AuthorityError("reconcile_base requires SYSTEM actor")

        # F — matching SOURCE verification evidence
        if not verification_evidence_ids:
            raise EngineError("reconcile_base requires verification_evidence_ids")
        by_id = {ev.evidence_id: ev for ev in self.evidence}
        for eid in verification_evidence_ids:
            ev = by_id.get(eid)
            if ev is None:
                raise EngineError(f"verification evidence missing: {eid}")
            if ev.type != EvidenceType.SOURCE:
                raise EngineError(f"verification evidence {eid} must be SOURCE")
            if ev.invalidated or ev.result != EvidenceResult.PASS:
                raise EngineError(f"verification evidence {eid} not usable PASS SOURCE")
            if ev.head_sha != task.head_sha:
                raise EngineError(
                    f"verification evidence {eid} head_sha must match current HEAD"
                )

        path_list = [validate_relative_path(p) for p in changed_paths]

        # G–J candidate only
        candidate = self._candidate()
        assert candidate.task is not None
        candidate.evidence = invalidate_for_new_head(
            candidate.evidence,
            new_head_sha,
            impact_known=impact_known,
            changed_paths=path_list,
        )
        candidate.task.head_sha = new_head_sha
        candidate.task.base_sha = new_base_sha
        candidate.task.readiness = False
        candidate.task.state = State.BASE_RECONCILIATION
        candidate.task.actor_last = Actor.SYSTEM

        # K — exactly one reconciliation journal event
        candidate.journal.append(
            task_id=candidate.task.task_id,
            from_state=State.FULL_VALIDATION,
            to_state=State.BASE_RECONCILIATION,
            head_sha=new_head_sha,
            base_sha=new_base_sha,
            evidence_ids=list(verification_evidence_ids),
            actor=Actor.SYSTEM,
            notes="reconcile_base",
            expected_head_sha=expected_head_sha,
            expected_base_sha=expected_base_sha,
            changed_paths=path_list,
        )

        # L–M atomic commit then publish memory
        self._commit(candidate)
        return self._require_task()

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
        expected = task.state
        eval_rca = rca_status if rca_status is not None else task.rca_status
        key = retry_key(task.task_id, task.state, failure_class)
        current = int(task.retry_counts.get(key, 0))
        disposition = disposition_for_failure(failure_class, eval_rca, current)

        if disposition == "RETRY":
            try:
                new_count = next_retry_count(current, failure_class)
            except FailurePolicyError:
                disposition = "BLOCKED"
            else:
                candidate = self._candidate()
                assert candidate.task is not None
                if rca_status is not None:
                    candidate.task.rca_status = rca_status
                candidate.task.failure_class = failure_class
                candidate.task.retry_counts[key] = new_count
                self._commit(candidate)
                return {
                    "disposition": "RETRY",
                    "retry_count": new_count,
                    "state": self._require_task().state.value,
                }

        if disposition == "REWORK":
            self.transition(
                State.REWORK,
                actor,
                expected_state=expected,
                failure_class=failure_class,
                rca_status=rca_status,
                rework_target=rework_target,
                notes=notes or f"failure={failure_class.value}",
            )
            return {"disposition": "REWORK", "state": self._require_task().state.value}

        if disposition == "BLOCKED":
            self.transition(
                State.BLOCKED,
                actor,
                expected_state=expected,
                failure_class=failure_class,
                rca_status=rca_status,
                notes=notes or f"failure={failure_class.value}",
            )
            return {"disposition": "BLOCKED", "state": self._require_task().state.value}

        self.transition(
            State.FAILED,
            actor,
            expected_state=expected,
            failure_class=failure_class,
            rca_status=rca_status,
            notes=notes or f"failure={failure_class.value}",
        )
        return {"disposition": "FAILED", "state": self._require_task().state.value}
    def reserve_migration(
        self,
        *,
        reservation_id: str,
        migration_id: str,
        lane_id: str = "",
        head_sha: str = "",
    ) -> dict[str, Any]:
        task = self._require_task()
        if not task.migration_reservation_allowed:
            raise EngineError(
                "migration_reservation_allowed is false — reservation denied"
            )
        candidate = self._candidate()
        assert candidate.task is not None
        try:
            reservation = candidate.migrations.reserve(
                reservation_id=reservation_id,
                migration_id=migration_id,
                lane_id=lane_id or candidate.task.lane_id,
                task_id=candidate.task.task_id,
                head_sha=head_sha or candidate.task.head_sha,
            )
        except MigrationCollisionError:
            # No reservation persisted; apply failure on current state.
            self.apply_failure(
                FailureClass.MIGRATION_COLLISION,
                Actor.SYSTEM,
                rca_status=RcaStatus.KNOWN,
                notes=f"migration collision on {migration_id}",
            )
            raise
        self._commit(candidate)
        return reservation.to_dict()

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
        candidate = self._candidate()
        candidate.handoffs.append(handoff)
        self._commit(candidate)
        return next(h for h in self.handoffs if h.handoff_id == handoff_id)

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
        projection["readiness_target"] = task.readiness_target.value
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
        candidate = self._candidate()
        assert candidate.task is not None
        try:
            candidate.lanes.claim(
                Lane(
                    lane_id=lane_id,
                    task_id=candidate.task.task_id,
                    path_prefixes=prefixes,
                    active=True,
                )
            )
        except LaneOwnershipError:
            self.apply_failure(
                FailureClass.MIGRATION_COLLISION,
                Actor.SYSTEM,
                notes="lane overlap",
            )
            raise
        candidate.task.lane_id = lane_id
        candidate.task.path_prefixes = prefixes
        self._commit(candidate)

    def _resume_blocked(self, to_state: State, actor: Actor, *, notes: str = "") -> Task:
        task = self._require_task()
        if not may_resume_blocked(actor, task.failure_class):
            raise AuthorityError(
                "resume from BLOCKED denied for actor "
                f"{actor.value} (business decisions are FOUNDER_CTO only)"
            )
        journal_target = self.journal.journal_resume_target()
        if journal_target is None:
            raise EngineError("BLOCKED task missing journal resume target")
        if to_state != journal_target:
            raise EngineError(
                f"resume target {to_state.value} != journal resume target "
                f"{journal_target.value}"
            )
        if is_terminal(to_state):
            raise EngineError("cannot resume into terminal state")

        from_state = State.BLOCKED
        candidate = self._candidate()
        assert candidate.task is not None
        candidate.journal.append(
            task_id=candidate.task.task_id,
            from_state=from_state,
            to_state=to_state,
            head_sha=candidate.task.head_sha,
            base_sha=candidate.task.base_sha,
            evidence_ids=list(candidate.task.evidence_ids),
            actor=actor,
            resume_state=to_state,
            notes=notes or "resume from BLOCKED",
        )
        candidate.task.state = to_state
        candidate.task.actor_last = actor
        candidate.task.failure_class = None
        candidate.task.resume_state = None  # clear after successful resume
        self._commit(candidate)
        return self._require_task()

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
        # IR completion: journal shows INDEPENDENT_REVIEW → EXACT_HEAD_CI by IR.
        ir_completed = any(
            e.from_state == State.INDEPENDENT_REVIEW.value
            and e.to_state == State.EXACT_HEAD_CI.value
            and e.actor == Actor.INDEPENDENT_REVIEWER.value
            for e in self.journal.events
        )
        evaluation = evaluate_readiness(
            task,
            self.evidence,
            actor=actor,
            reality_gate_pass=reality_gate_pass,
            independent_review_completed=ir_completed,
        )
        if not evaluation.readiness:
            raise EngineError(f"readiness denied: {evaluation.reason}")

    def _require_task(self) -> Task:
        if self.task is None:
            raise EngineError("no task initialized")
        return self.task

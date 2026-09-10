"""FSM and authority tests for Engineering Orchestrator Phase 1 (P0 hardened)."""

from __future__ import annotations

import tempfile
import unittest

from tools.engineering_orchestrator.authority import AuthorityError, authorize_transition
from tools.engineering_orchestrator.engine import EngineError, OrchestratorEngine
from tools.engineering_orchestrator.fsm import TransitionError, happy_path_next, validate_transition
from tools.engineering_orchestrator.models import (
    Actor,
    EvidenceResult,
    EvidenceType,
    FailureClass,
    RcaStatus,
    ReadinessTarget,
    State,
)
from tools.engineering_orchestrator.store import OrchestratorStore

from helpers import (
    advance_through_ir_and_ci,
    advance_to_full_validation,
    default_gates,
    engine,
    reconcile_to_independent_review,
)


class FsmAuthorityTests(unittest.TestCase):
    def test_valid_happy_path_to_reality_gate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                base_sha="b1",
                required_gates=default_gates(),
            )
            advance_to_full_validation(eng)
            reconcile_to_independent_review(eng)
            advance_through_ir_and_ci(eng)
            self.assertEqual(eng.task.state, State.REALITY_GATE)

    def test_invalid_skip_transition(self) -> None:
        with self.assertRaises(TransitionError):
            validate_transition(State.TASK_ACCEPTED, State.IMPLEMENTATION, Actor.WRITER)

    def test_writer_self_certification_rejected(self) -> None:
        with self.assertRaises(AuthorityError):
            authorize_transition(
                State.REALITY_GATE, State.READY_FOR_MERGE_GATE, Actor.WRITER
            )

    def test_writer_merge_rejected(self) -> None:
        with self.assertRaises(AuthorityError):
            authorize_transition(
                State.READY_FOR_MERGE_GATE, State.MERGED, Actor.WRITER
            )

    def test_reality_checker_gate(self) -> None:
        authorize_transition(
            State.REALITY_GATE, State.READY_FOR_MERGE_GATE, Actor.REALITY_CHECKER
        )

    def test_merge_authority_gate(self) -> None:
        authorize_transition(
            State.READY_FOR_MERGE_GATE, State.MERGED, Actor.MERGE_AUTHORITY
        )

    def test_writer_cannot_complete_independent_review(self) -> None:
        with self.assertRaises(TransitionError):
            validate_transition(
                State.INDEPENDENT_REVIEW, State.EXACT_HEAD_CI, Actor.WRITER
            )

    def test_system_cannot_complete_independent_review(self) -> None:
        with self.assertRaises(TransitionError):
            validate_transition(
                State.INDEPENDENT_REVIEW, State.EXACT_HEAD_CI, Actor.SYSTEM
            )

    def test_independent_reviewer_path(self) -> None:
        validate_transition(
            State.INDEPENDENT_REVIEW, State.EXACT_HEAD_CI, Actor.INDEPENDENT_REVIEWER
        )

    def test_engine_writer_cannot_ready(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                base_sha="b1",
                required_gates=default_gates(),
            )
            advance_to_full_validation(eng)
            reconcile_to_independent_review(eng)
            advance_through_ir_and_ci(eng)
            with self.assertRaises((AuthorityError, EngineError)):
                eng.transition(
                    State.READY_FOR_MERGE_GATE,
                    Actor.WRITER,
                    expected_state=State.REALITY_GATE,
                )

    def test_full_happy_path_with_gates(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                base_sha="b1",
                required_gates=default_gates(),
                readiness_target=ReadinessTarget.IMPLEMENTED,
            )
            advance_to_full_validation(eng)
            reconcile_to_independent_review(eng)
            advance_through_ir_and_ci(eng)
            eng.record_evidence(
                evidence_id="lt1",
                evidence_type=EvidenceType.LOCAL_TEST,
                head_sha=eng.task.head_sha,
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="2026-01-01T00:00:00Z",
                command_identity="unittest",
            )
            eng.transition(
                State.READY_FOR_MERGE_GATE,
                Actor.REALITY_CHECKER,
                expected_state=State.REALITY_GATE,
                reality_gate_pass=True,
            )
            self.assertTrue(eng.task.readiness)
            eng.transition(
                State.MERGED,
                Actor.MERGE_AUTHORITY,
                expected_state=State.READY_FOR_MERGE_GATE,
            )
            self.assertEqual(eng.task.state, State.MERGED)

    def test_happy_path_next_helper(self) -> None:
        self.assertEqual(happy_path_next(State.TASK_ACCEPTED), State.SOURCE_INSPECTION)
        self.assertEqual(happy_path_next(State.REALITY_GATE), State.READY_FOR_MERGE_GATE)

    def test_founder_cancel(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            eng.transition(
                State.CANCELLED,
                Actor.FOUNDER_CTO,
                expected_state=State.TASK_ACCEPTED,
            )
            self.assertEqual(eng.task.state, State.CANCELLED)

    def test_expected_state_mismatch_side_effect_free(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            gen = eng.generation
            with self.assertRaises(EngineError):
                eng.transition(
                    State.SOURCE_INSPECTION,
                    Actor.WRITER,
                    expected_state=State.IMPLEMENTATION,
                )
            self.assertEqual(eng.task.state, State.TASK_ACCEPTED)
            self.assertEqual(eng.generation, gen)
            self.assertEqual(len(eng.journal.events), 0)

    def test_rejected_transition_cannot_mutate_head_or_base(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            with self.assertRaises(TransitionError):
                eng.transition(
                    State.IMPLEMENTATION,
                    Actor.WRITER,
                    expected_state=State.TASK_ACCEPTED,
                )
            self.assertEqual(eng.task.head_sha, "h1")
            self.assertEqual(eng.task.base_sha, "b1")

    def test_generic_transition_cannot_enter_reconciliation(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            advance_to_full_validation(eng)
            with self.assertRaises(EngineError):
                eng.transition(
                    State.BASE_RECONCILIATION,
                    Actor.SYSTEM,
                    expected_state=State.FULL_VALIDATION,
                )

    def test_independent_reviewer_requires_review_evidence(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            advance_to_full_validation(eng)
            reconcile_to_independent_review(eng)
            with self.assertRaises(EngineError):
                eng.transition(
                    State.EXACT_HEAD_CI,
                    Actor.INDEPENDENT_REVIEWER,
                    expected_state=State.INDEPENDENT_REVIEW,
                )

    def test_independent_reviewer_current_pass_path(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            advance_to_full_validation(eng)
            reconcile_to_independent_review(eng)
            eng.record_evidence(
                evidence_id="ir1",
                evidence_type=EvidenceType.INDEPENDENT_REVIEW,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                reviewer_logical_identity="r1",
            )
            eng.transition(
                State.EXACT_HEAD_CI,
                Actor.INDEPENDENT_REVIEWER,
                expected_state=State.INDEPENDENT_REVIEW,
            )
            self.assertEqual(eng.task.state, State.EXACT_HEAD_CI)

    def test_business_blocker_founder_only_resume(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            eng.transition(
                State.SOURCE_INSPECTION,
                Actor.WRITER,
                expected_state=State.TASK_ACCEPTED,
            )
            eng.apply_failure(
                FailureClass.BUSINESS_DECISION_REQUIRED,
                Actor.WRITER,
                rca_status=RcaStatus.KNOWN,
            )
            self.assertEqual(eng.task.state, State.BLOCKED)
            with self.assertRaises(AuthorityError):
                eng.transition(
                    State.SOURCE_INSPECTION,
                    Actor.SYSTEM,
                    expected_state=State.BLOCKED,
                )
            eng.transition(
                State.SOURCE_INSPECTION,
                Actor.FOUNDER_CTO,
                expected_state=State.BLOCKED,
            )
            self.assertEqual(eng.task.state, State.SOURCE_INSPECTION)
            self.assertIsNone(eng.task.resume_state)


if __name__ == "__main__":
    unittest.main()

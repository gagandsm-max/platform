"""FSM and authority tests for Engineering Orchestrator Phase 1."""

from __future__ import annotations

import tempfile
import unittest

from tools.engineering_orchestrator.authority import AuthorityError, authorize_transition
from tools.engineering_orchestrator.engine import EngineError, OrchestratorEngine
from tools.engineering_orchestrator.fsm import TransitionError, happy_path_next, validate_transition
from tools.engineering_orchestrator.models import Actor, EvidenceResult, EvidenceType, State
from tools.engineering_orchestrator.store import OrchestratorStore


def _engine(tmp: str) -> OrchestratorEngine:
    return OrchestratorEngine(OrchestratorStore(tmp))


def _advance_to(engine: OrchestratorEngine, target: State) -> None:
    """Advance along happy path until target (writer-owned states only)."""
    order = [
        State.SOURCE_INSPECTION,
        State.AGENT_ROUTING,
        State.RCA_COMPLETE,
        State.IMPLEMENTATION,
        State.FOCUSED_VALIDATION,
        State.FULL_VALIDATION,
        State.BASE_RECONCILIATION,
        State.INDEPENDENT_REVIEW,
        State.EXACT_HEAD_CI,
        State.REALITY_GATE,
    ]
    for state in order:
        if engine.task is None:
            raise AssertionError("missing task")
        if engine.task.state == target:
            return
        if state == State.REALITY_GATE and engine.task.state == State.EXACT_HEAD_CI:
            engine.record_evidence(
                evidence_id="ci-fresh",
                evidence_type=EvidenceType.CI,
                head_sha=engine.task.head_sha,
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="2026-01-01T00:00:00Z",
            )
        engine.transition(state, Actor.WRITER)
        if engine.task.state == target:
            return


class FsmAuthorityTests(unittest.TestCase):
    def test_valid_happy_path_to_reality_gate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = _engine(tmp)
            engine.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            _advance_to(engine, State.REALITY_GATE)
            self.assertEqual(engine.task.state, State.REALITY_GATE)

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

    def test_merge_authority_cannot_enter_ready(self) -> None:
        with self.assertRaises(AuthorityError):
            authorize_transition(
                State.REALITY_GATE, State.READY_FOR_MERGE_GATE, Actor.MERGE_AUTHORITY
            )

    def test_reality_checker_cannot_merge(self) -> None:
        with self.assertRaises(AuthorityError):
            authorize_transition(
                State.READY_FOR_MERGE_GATE, State.MERGED, Actor.REALITY_CHECKER
            )

    def test_engine_writer_cannot_ready(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = _engine(tmp)
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            _advance_to(engine, State.REALITY_GATE)
            with self.assertRaises((AuthorityError, EngineError)):
                engine.transition(State.READY_FOR_MERGE_GATE, Actor.WRITER)

    def test_full_happy_path_with_gates(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = _engine(tmp)
            engine.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                required_scopes=["default"],
            )
            _advance_to(engine, State.REALITY_GATE)
            engine.record_evidence(
                evidence_id="lt1",
                evidence_type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="2026-01-01T00:00:00Z",
            )
            engine.record_evidence(
                evidence_id="src1",
                evidence_type=EvidenceType.SOURCE,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="2026-01-01T00:00:00Z",
            )
            engine.transition(
                State.READY_FOR_MERGE_GATE,
                Actor.REALITY_CHECKER,
                reality_gate_pass=True,
            )
            self.assertTrue(engine.task.readiness)
            engine.transition(State.MERGED, Actor.MERGE_AUTHORITY)
            self.assertEqual(engine.task.state, State.MERGED)

    def test_happy_path_next_helper(self) -> None:
        self.assertEqual(happy_path_next(State.TASK_ACCEPTED), State.SOURCE_INSPECTION)
        self.assertEqual(happy_path_next(State.REALITY_GATE), State.READY_FOR_MERGE_GATE)

    def test_founder_cancel(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = _engine(tmp)
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            engine.transition(State.CANCELLED, Actor.FOUNDER_CTO)
            self.assertEqual(engine.task.state, State.CANCELLED)


if __name__ == "__main__":
    unittest.main()

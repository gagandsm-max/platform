"""Failure classification and retry policy tests."""

from __future__ import annotations

import tempfile
import unittest

from tools.engineering_orchestrator.engine import EngineError, OrchestratorEngine
from tools.engineering_orchestrator.failures import (
    MAX_TRANSIENT_RETRIES,
    can_auto_rework,
    disposition_for_failure,
    next_retry_count,
)
from tools.engineering_orchestrator.failures import FailurePolicyError
from tools.engineering_orchestrator.models import (
    Actor,
    FailureClass,
    RcaStatus,
    State,
)
from tools.engineering_orchestrator.store import OrchestratorStore


class FailureRetryTests(unittest.TestCase):
    def test_failure_classifications_exist(self) -> None:
        expected = {
            "SOURCE_DEFECT",
            "TEST_DEFECT",
            "CONFIG_DEFECT",
            "ENVIRONMENT_DEFECT",
            "TRANSIENT_EXTERNAL_FAILURE",
            "SECURITY_BLOCKER",
            "ARCHITECTURE_BLOCKER",
            "MIGRATION_COLLISION",
            "EVIDENCE_GAP",
            "BUSINESS_DECISION_REQUIRED",
        }
        self.assertEqual({f.value for f in FailureClass}, expected)

    def test_retry_budget_transient_only(self) -> None:
        self.assertEqual(next_retry_count(0, FailureClass.TRANSIENT_EXTERNAL_FAILURE), 1)
        self.assertEqual(next_retry_count(1, FailureClass.TRANSIENT_EXTERNAL_FAILURE), 2)
        with self.assertRaises(FailurePolicyError):
            next_retry_count(2, FailureClass.TRANSIENT_EXTERNAL_FAILURE)
        with self.assertRaises(FailurePolicyError):
            next_retry_count(0, FailureClass.SOURCE_DEFECT)

    def test_max_transient_is_two(self) -> None:
        self.assertEqual(MAX_TRANSIENT_RETRIES, 2)

    def test_unknown_rca_no_retry_rework(self) -> None:
        self.assertFalse(
            can_auto_rework(FailureClass.SOURCE_DEFECT, RcaStatus.UNKNOWN)
        )
        self.assertFalse(
            can_auto_rework(FailureClass.SOURCE_DEFECT, RcaStatus.INCONCLUSIVE)
        )
        self.assertEqual(
            disposition_for_failure(
                FailureClass.SOURCE_DEFECT, RcaStatus.UNKNOWN, 0
            ),
            "BLOCKED",
        )

    def test_known_source_defect_rework(self) -> None:
        self.assertTrue(can_auto_rework(FailureClass.SOURCE_DEFECT, RcaStatus.KNOWN))
        self.assertEqual(
            disposition_for_failure(FailureClass.SOURCE_DEFECT, RcaStatus.KNOWN, 0),
            "REWORK",
        )

    def test_security_blocker_blocks(self) -> None:
        self.assertEqual(
            disposition_for_failure(
                FailureClass.SECURITY_BLOCKER, RcaStatus.KNOWN, 0
            ),
            "BLOCKED",
        )

    def test_engine_transient_retries_then_block(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            engine.transition(State.SOURCE_INSPECTION, Actor.WRITER)

            r1 = engine.apply_failure(
                FailureClass.TRANSIENT_EXTERNAL_FAILURE, Actor.WRITER
            )
            self.assertEqual(r1["disposition"], "RETRY")
            self.assertEqual(engine.task.state, State.SOURCE_INSPECTION)

            r2 = engine.apply_failure(
                FailureClass.TRANSIENT_EXTERNAL_FAILURE, Actor.WRITER
            )
            self.assertEqual(r2["disposition"], "RETRY")

            r3 = engine.apply_failure(
                FailureClass.TRANSIENT_EXTERNAL_FAILURE, Actor.WRITER
            )
            self.assertEqual(r3["disposition"], "BLOCKED")
            self.assertEqual(engine.task.state, State.BLOCKED)

    def test_engine_unknown_rca_rejects_rework_transition(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            engine.transition(State.SOURCE_INSPECTION, Actor.WRITER)
            with self.assertRaises(EngineError):
                engine.transition(
                    State.REWORK,
                    Actor.WRITER,
                    failure_class=FailureClass.SOURCE_DEFECT,
                    rca_status=RcaStatus.UNKNOWN,
                )

    def test_founder_cto_business_gate_status(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            engine.apply_failure(
                FailureClass.BUSINESS_DECISION_REQUIRED,
                Actor.WRITER,
                rca_status=RcaStatus.KNOWN,
            )
            status = engine.status()
            self.assertTrue(status["founder_action_required"])
            self.assertEqual(status["waiting_on"], "BUSINESS_DECISION")


if __name__ == "__main__":
    unittest.main()

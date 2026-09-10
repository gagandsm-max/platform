"""Evidence promotion and invalidation tests."""

from __future__ import annotations

import tempfile
import unittest

from tools.engineering_orchestrator.engine import OrchestratorEngine
from tools.engineering_orchestrator.evidence import (
    EvidenceError,
    assert_promotion_allowed,
    has_fresh_ci,
    invalidate_for_new_head,
    is_usable,
)
from tools.engineering_orchestrator.models import (
    Actor,
    Evidence,
    EvidenceResult,
    EvidenceType,
    State,
)
from tools.engineering_orchestrator.store import OrchestratorStore


class EvidenceTests(unittest.TestCase):
    def test_promotion_local_test_to_ci_rejected(self) -> None:
        with self.assertRaises(EvidenceError):
            assert_promotion_allowed(EvidenceType.LOCAL_TEST, EvidenceType.CI)

    def test_promotion_source_to_runtime_rejected(self) -> None:
        with self.assertRaises(EvidenceError):
            assert_promotion_allowed(EvidenceType.SOURCE, EvidenceType.LOCAL_RUNTIME)

    def test_inference_to_execution_rejected(self) -> None:
        with self.assertRaises(EvidenceError):
            assert_promotion_allowed(EvidenceType.INFERENCE, EvidenceType.LOCAL_TEST)
        with self.assertRaises(EvidenceError):
            assert_promotion_allowed(EvidenceType.RECOMMENDATION, EvidenceType.CI)

    def test_engine_rejects_forbidden_promotion(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            with self.assertRaises(EvidenceError):
                engine.record_evidence(
                    evidence_id="e1",
                    evidence_type=EvidenceType.CI,
                    head_sha="h1",
                    claim_scope="default",
                    result=EvidenceResult.PASS,
                    collected_at="2026-01-01T00:00:00Z",
                    promote_from=EvidenceType.LOCAL_TEST,
                )

    def test_ci_cannot_claim_production_readiness(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            with self.assertRaises(EvidenceError):
                engine.record_evidence(
                    evidence_id="e1",
                    evidence_type=EvidenceType.CI,
                    head_sha="h1",
                    claim_scope="production_readiness",
                    result=EvidenceResult.PASS,
                    collected_at="2026-01-01T00:00:00Z",
                )

    def test_static_config_not_runtime(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            with self.assertRaises(EvidenceError):
                engine.record_evidence(
                    evidence_id="e1",
                    evidence_type=EvidenceType.LOCAL_RUNTIME,
                    head_sha="h1",
                    claim_scope="static_config_check",
                    result=EvidenceResult.PASS,
                    collected_at="2026-01-01T00:00:00Z",
                )

    def test_new_head_invalidates_ci(self) -> None:
        items = [
            Evidence(
                evidence_id="ci1",
                type=EvidenceType.CI,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
            ),
            Evidence(
                evidence_id="lt1",
                type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
            ),
        ]
        updated = invalidate_for_new_head(items, "h2", impact_known=True)
        ci = next(e for e in updated if e.evidence_id == "ci1")
        self.assertTrue(ci.invalidated)
        self.assertIn("CI", ci.invalidation_reason)

    def test_unknown_impact_conservative_invalidation(self) -> None:
        items = [
            Evidence(
                evidence_id="lt1",
                type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
            ),
            Evidence(
                evidence_id="src1",
                type=EvidenceType.SOURCE,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                impact_known=True,
            ),
        ]
        updated = invalidate_for_new_head(items, "h2", impact_known=False)
        lt = next(e for e in updated if e.evidence_id == "lt1")
        self.assertTrue(lt.invalidated)
        self.assertIn("conservative", lt.invalidation_reason)

    def test_safe_evidence_reuse_same_head(self) -> None:
        ev = Evidence(
            evidence_id="lt1",
            type=EvidenceType.LOCAL_TEST,
            head_sha="h1",
            claim_scope="default",
            result=EvidenceResult.PASS,
            collected_at="t",
        )
        self.assertTrue(is_usable(ev, "h1"))
        updated = invalidate_for_new_head([ev], "h1", impact_known=True)
        self.assertFalse(updated[0].invalidated)
        self.assertTrue(is_usable(updated[0], "h1"))

    def test_engine_set_head_invalidates_ci(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            engine.record_evidence(
                evidence_id="ci1",
                evidence_type=EvidenceType.CI,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
            )
            self.assertTrue(has_fresh_ci(engine.evidence, "h1"))
            engine.set_head("h2", impact_known=True)
            self.assertFalse(has_fresh_ci(engine.evidence, "h2"))
            ci = next(e for e in engine.evidence if e.evidence_id == "ci1")
            self.assertTrue(ci.invalidated)

    def test_exact_head_ci_required_before_reality_gate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            for state in [
                State.SOURCE_INSPECTION,
                State.AGENT_ROUTING,
                State.RCA_COMPLETE,
                State.IMPLEMENTATION,
                State.FOCUSED_VALIDATION,
                State.FULL_VALIDATION,
                State.BASE_RECONCILIATION,
                State.INDEPENDENT_REVIEW,
                State.EXACT_HEAD_CI,
            ]:
                engine.transition(state, Actor.WRITER)
            with self.assertRaises(Exception):
                engine.transition(State.REALITY_GATE, Actor.WRITER)


if __name__ == "__main__":
    unittest.main()

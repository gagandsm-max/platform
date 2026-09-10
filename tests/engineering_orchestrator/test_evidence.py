"""Evidence promotion, provenance, invalidation, and reuse tests."""

from __future__ import annotations

import tempfile
import unittest

from tools.engineering_orchestrator.engine import OrchestratorEngine
from tools.engineering_orchestrator.evidence import (
    EvidenceError,
    assert_promotion_allowed,
    create_reuse,
    has_fresh_ci,
    invalidate_for_new_head,
    is_usable,
)
from tools.engineering_orchestrator.models import (
    Evidence,
    EvidenceAttestation,
    EvidenceResult,
    EvidenceType,
)
from tools.engineering_orchestrator.store import OrchestratorStore
from tools.engineering_orchestrator.validation import ValidationError, validate_evidence


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
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            with self.assertRaises(EvidenceError):
                eng.record_evidence(
                    evidence_id="e1",
                    evidence_type=EvidenceType.CI,
                    head_sha="h1",
                    claim_scope="default",
                    result=EvidenceResult.PASS,
                    collected_at="2026-01-01T00:00:00Z",
                    promote_from=EvidenceType.LOCAL_TEST,
                    ci_run_id="r",
                    ci_job_id="j",
                    command_identity="ci",
                )

    def test_ci_cannot_claim_production_readiness(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            with self.assertRaises(EvidenceError):
                eng.record_evidence(
                    evidence_id="e1",
                    evidence_type=EvidenceType.CI,
                    head_sha="h1",
                    claim_scope="production_readiness",
                    result=EvidenceResult.PASS,
                    collected_at="2026-01-01T00:00:00Z",
                    ci_run_id="r",
                    ci_job_id="j",
                    command_identity="ci",
                )

    def test_static_config_not_runtime(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            with self.assertRaises(EvidenceError):
                eng.record_evidence(
                    evidence_id="e1",
                    evidence_type=EvidenceType.LOCAL_RUNTIME,
                    head_sha="h1",
                    claim_scope="static_config_check",
                    result=EvidenceResult.PASS,
                    collected_at="2026-01-01T00:00:00Z",
                    command_identity="run",
                )

    def test_phase1_evidence_always_unattested(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            ev = eng.record_evidence(
                evidence_id="e1",
                evidence_type=EvidenceType.SOURCE,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                command_identity="src",
            )
            self.assertEqual(ev.attestation, EvidenceAttestation.UNATTESTED)

    def test_trusted_persisted_attestation_rejected(self) -> None:
        with self.assertRaises(ValidationError):
            validate_evidence(
                {
                    "evidence_id": "e1",
                    "type": "SOURCE",
                    "claim_scope": "default",
                    "head_sha": "h1",
                    "result": "PASS",
                    "collected_at": "t",
                    "attestation": "TRUSTED",
                }
            )
        with self.assertRaises(ValueError):
            Evidence.from_dict(
                {
                    "evidence_id": "e1",
                    "type": "SOURCE",
                    "claim_scope": "default",
                    "head_sha": "h1",
                    "result": "PASS",
                    "collected_at": "t",
                    "attestation": "TRUSTED",
                }
            )

    def test_ci_provenance_required(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            with self.assertRaises(EvidenceError):
                eng.record_evidence(
                    evidence_id="e1",
                    evidence_type=EvidenceType.CI,
                    head_sha="h1",
                    claim_scope="default",
                    result=EvidenceResult.PASS,
                    collected_at="t",
                )

    def test_execution_provenance_required(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            with self.assertRaises(EvidenceError):
                eng.record_evidence(
                    evidence_id="e1",
                    evidence_type=EvidenceType.LOCAL_TEST,
                    head_sha="h1",
                    claim_scope="default",
                    result=EvidenceResult.PASS,
                    collected_at="t",
                )

    def test_review_provenance_required(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            with self.assertRaises(EvidenceError):
                eng.record_evidence(
                    evidence_id="e1",
                    evidence_type=EvidenceType.INDEPENDENT_REVIEW,
                    head_sha="h1",
                    claim_scope="default",
                    result=EvidenceResult.PASS,
                    collected_at="t",
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
                ci_run_id="r",
                ci_job_id="j",
                command_identity="ci",
            ),
            Evidence(
                evidence_id="lt1",
                type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                command_identity="t",
                paths=["tools/foo.py"],
            ),
        ]
        updated = invalidate_for_new_head(
            items, "h2", impact_known=True, changed_paths=["tools/foo.py"]
        )
        by_id = {e.evidence_id: e for e in updated}
        self.assertTrue(by_id["ci1"].invalidated)
        self.assertTrue(by_id["lt1"].invalidated)

    def test_dependency_path_intersection_invalidates(self) -> None:
        items = [
            Evidence(
                evidence_id="lt1",
                type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                command_identity="t",
                paths=["tools/engineering_orchestrator/engine.py"],
            )
        ]
        updated = invalidate_for_new_head(
            items,
            "h2",
            impact_known=True,
            changed_paths=["tools/engineering_orchestrator/engine.py"],
        )
        self.assertTrue(updated[0].invalidated)

    def test_all_sensitive_categories_invalidate(self) -> None:
        items = [
            Evidence(
                evidence_id="lt1",
                type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                command_identity="t",
                dependencies=[
                    "@workflow",
                    "@build_descriptor",
                    "@test_harness",
                    "@runtime_config",
                    "@migration_manifest",
                    "@shared_contract",
                ],
            )
        ]
        for path in [
            ".github/workflows/platform-ci.yml",
            "Makefile",
            "tests/run_unit.sh",
            "docker-compose.yml",
            "migrations/001.sql",
            "tools/engineering_orchestrator/schemas/task.schema.json",
        ]:
            copy = [
                Evidence(
                    evidence_id="lt1",
                    type=EvidenceType.LOCAL_TEST,
                    head_sha="h1",
                    claim_scope="default",
                    result=EvidenceResult.PASS,
                    collected_at="t",
                    command_identity="t",
                    dependencies=list(items[0].dependencies),
                )
            ]
            updated = invalidate_for_new_head(
                copy, "h2", impact_known=True, changed_paths=[path]
            )
            self.assertTrue(updated[0].invalidated, msg=path)

    def test_unknown_impact_invalidates(self) -> None:
        items = [
            Evidence(
                evidence_id="lt1",
                type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                command_identity="t",
            )
        ]
        updated = invalidate_for_new_head(items, "h2", impact_known=False)
        self.assertTrue(updated[0].invalidated)

    def test_explicit_evidence_reuse(self) -> None:
        source = Evidence(
            evidence_id="old",
            type=EvidenceType.SOURCE,
            head_sha="h1",
            claim_scope="default",
            result=EvidenceResult.PASS,
            collected_at="t",
            command_identity="src",
        )
        reused = create_reuse(
            source,
            new_evidence_id="new",
            new_head="h2",
            reuse_reason="impact known — source unchanged",
            collected_at="t2",
        )
        self.assertEqual(reused.reused_from_evidence_id, "old")
        self.assertEqual(reused.head_sha, "h2")
        self.assertTrue(is_usable(reused, "h2"))

    def test_no_unsafe_old_head_promotion(self) -> None:
        ev = Evidence(
            evidence_id="lt1",
            type=EvidenceType.LOCAL_TEST,
            head_sha="h1",
            claim_scope="default",
            result=EvidenceResult.PASS,
            collected_at="t",
            command_identity="t",
        )
        self.assertFalse(is_usable(ev, "h2"))

    def test_has_fresh_ci(self) -> None:
        items = [
            Evidence(
                evidence_id="ci1",
                type=EvidenceType.CI,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                ci_run_id="r",
                ci_job_id="j",
                command_identity="ci",
            )
        ]
        self.assertTrue(has_fresh_ci(items, "h1"))
        self.assertFalse(has_fresh_ci(items, "h2"))


if __name__ == "__main__":
    unittest.main()

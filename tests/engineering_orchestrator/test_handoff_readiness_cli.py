"""Handoff, readiness, CLI, store generation, and schema tests."""

from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from tools.engineering_orchestrator.cli import main as cli_main
from tools.engineering_orchestrator.engine import EngineError, OrchestratorEngine
from tools.engineering_orchestrator.handoff import create_handoff, redact_sensitive
from tools.engineering_orchestrator.models import (
    Actor,
    Evidence,
    EvidenceResult,
    EvidenceType,
    GateApplicability,
    ReadinessTarget,
    RequiredGate,
    State,
)
from tools.engineering_orchestrator.readiness import (
    ci_alone_promotes_readiness,
    evaluate_hundred_percent_pass,
    evaluate_readiness,
)
from tools.engineering_orchestrator.store import BundleState, OrchestratorStore, StoreError
from tools.engineering_orchestrator.validation import ValidationError, validate_bundle

from helpers import (
    advance_through_ir_and_ci,
    advance_to_full_validation,
    default_gates,
    foundation_gates,
    reconcile_to_independent_review,
)


ROOT = Path(__file__).resolve().parents[2]
SCHEMAS = ROOT / "tools" / "engineering_orchestrator" / "schemas"


class HandoffReadinessCliTests(unittest.TestCase):
    def test_codex_disabled_by_default(self) -> None:
        handoff = create_handoff(
            handoff_id="h1",
            task_id="T1",
            from_state="IMPLEMENTATION",
            expected_return_state="FOCUSED_VALIDATION",
            actor_role="writer",
        )
        self.assertFalse(handoff.codex_enabled)

    def test_handoff_data_safety_redacts_secrets(self) -> None:
        redacted = redact_sensitive(
            {
                "token": "supersecret",
                "note": "ok",
                "nested": {"api_key": "x"},
                "items": [{"token": "nested-secret"}, {"note": "safe"}],
            }
        )
        self.assertEqual(redacted["token"], "[REDACTED]")
        self.assertEqual(redacted["note"], "ok")
        self.assertEqual(redacted["nested"]["api_key"], "[REDACTED]")
        self.assertEqual(redacted["items"][0]["token"], "[REDACTED]")
        self.assertEqual(redacted["items"][1]["note"], "safe")

    def test_codex_enabled_string_false_rejected_on_reload(self) -> None:
        from tools.engineering_orchestrator.handoff import Handoff, HandoffError

        with self.assertRaises(HandoffError):
            Handoff.from_dict(
                {
                    "handoff_id": "h1",
                    "task_id": "T1",
                    "from_state": "IMPLEMENTATION",
                    "expected_return_state": "FOCUSED_VALIDATION",
                    "actor_role": "writer",
                    "codex_enabled": "false",
                }
            )

    def test_engine_handoff_create_defaults_codex_false(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            handoff = eng.create_handoff(
                handoff_id="h1",
                actor_role="manual-worker",
                expected_return_state="IMPLEMENTATION",
                inputs={"password": "nope", "task": "fix"},
            )
            self.assertFalse(handoff.codex_enabled)
            self.assertEqual(handoff.inputs["password"], "[REDACTED]")

    def test_empty_gates_not_hundred_percent(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            result = evaluate_hundred_percent_pass(eng.task, eng.evidence)
            self.assertFalse(result.all_required_pass)

    def test_required_gate_matrix_and_non_pass(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                base_sha="b1",
                required_gates=default_gates(),
            )
            for result in (
                EvidenceResult.FAIL,
                EvidenceResult.PARTIAL,
                EvidenceResult.SKIPPED,
                EvidenceResult.UNKNOWN,
                EvidenceResult.NOT_RUN,
            ):
                eng.record_evidence(
                    evidence_id=f"lt-{result.value}",
                    evidence_type=EvidenceType.LOCAL_TEST,
                    head_sha="h1",
                    claim_scope="default",
                    result=result,
                    collected_at="t",
                    command_identity="unittest",
                )
                evaluation = evaluate_hundred_percent_pass(eng.task, eng.evidence)
                self.assertFalse(evaluation.all_required_pass, msg=result.value)

    def test_na_requires_reason(self) -> None:
        with self.assertRaises(ValueError):
            RequiredGate(
                gate_id="g1",
                claim_scope="default",
                required_evidence_type=EvidenceType.LOCAL_TEST,
                readiness_target=ReadinessTarget.IMPLEMENTED,
                applicability=GateApplicability.NOT_APPLICABLE,
                not_applicable_reason="",
            )
        # from_dict path
        with self.assertRaises(ValueError):
            RequiredGate.from_dict(
                {
                    "gate_id": "g1",
                    "claim_scope": "default",
                    "required_evidence_type": "LOCAL_TEST",
                    "readiness_target": "IMPLEMENTED",
                    "applicability": "NOT_APPLICABLE",
                    "not_applicable_reason": "",
                }
            )

    def test_readiness_targets_cumulative_and_foundation(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                base_sha="b1",
                required_gates=foundation_gates(),
                readiness_target=ReadinessTarget.FOUNDATION_SAFE_TO_LAND,
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
                collected_at="t",
                command_identity="unittest",
            )
            # IR evidence already recorded in advance_through_ir_and_ci
            evaluation = evaluate_readiness(
                eng.task,
                eng.evidence,
                actor=Actor.REALITY_CHECKER,
                reality_gate_pass=True,
                independent_review_completed=True,
            )
            self.assertTrue(evaluation.readiness)

            # Without IR completion flag
            evaluation2 = evaluate_readiness(
                eng.task,
                eng.evidence,
                actor=Actor.REALITY_CHECKER,
                reality_gate_pass=True,
                independent_review_completed=False,
            )
            self.assertFalse(evaluation2.readiness)

    def test_ci_alone_never_readiness(self) -> None:
        self.assertFalse(ci_alone_promotes_readiness())
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                base_sha="b1",
                required_gates=default_gates(),
            )
            eng.record_evidence(
                evidence_id="ci1",
                evidence_type=EvidenceType.CI,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                ci_run_id="r",
                ci_job_id="j",
                command_identity="ci",
            )
            # Also add local to satisfy gate, then evaluate with CI-only list
            eng.record_evidence(
                evidence_id="lt1",
                evidence_type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                command_identity="t",
            )
            ci_only = [e for e in eng.evidence if e.type == EvidenceType.CI]
            evaluation = evaluate_readiness(
                eng.task,
                ci_only,
                actor=Actor.REALITY_CHECKER,
                reality_gate_pass=True,
            )
            self.assertFalse(evaluation.readiness)
            self.assertIn("CI alone", evaluation.reason)

    def test_foundation_ci_only_gates_never_readiness(self) -> None:
        """FOUNDATION+ with only CI RequiredGates must not promote."""
        ci_only_gates = [
            RequiredGate(
                gate_id="g-ci",
                claim_scope="default",
                required_evidence_type=EvidenceType.CI,
                readiness_target=ReadinessTarget.IMPLEMENTED,
                applicability=GateApplicability.REQUIRED,
            ),
            RequiredGate(
                gate_id="g-ci-f",
                claim_scope="default",
                required_evidence_type=EvidenceType.CI,
                readiness_target=ReadinessTarget.FOUNDATION_SAFE_TO_LAND,
                applicability=GateApplicability.REQUIRED,
            ),
        ]
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                base_sha="b1",
                required_gates=ci_only_gates,
                readiness_target=ReadinessTarget.FOUNDATION_SAFE_TO_LAND,
            )
            advance_to_full_validation(eng)
            reconcile_to_independent_review(eng)
            advance_through_ir_and_ci(eng)
            # Incidental SOURCE + IR evidence present from helpers; still CI-only gates.
            evaluation = evaluate_readiness(
                eng.task,
                eng.evidence,
                actor=Actor.REALITY_CHECKER,
                reality_gate_pass=True,
                independent_review_completed=True,
            )
            self.assertFalse(evaluation.readiness)
            self.assertIn("CI alone", evaluation.reason)

    def test_readiness_requires_reality_checker(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            eng.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                base_sha="b1",
                required_gates=default_gates(),
            )
            eng.record_evidence(
                evidence_id="lt1",
                evidence_type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                command_identity="t",
            )
            evaluation = evaluate_readiness(
                eng.task,
                eng.evidence,
                actor=Actor.WRITER,
                reality_gate_pass=True,
            )
            self.assertFalse(evaluation.readiness)

    def test_cli_help(self) -> None:
        with self.assertRaises(SystemExit) as ctx:
            cli_main(["--help"])
        self.assertEqual(ctx.exception.code, 0)

    def test_cli_init_status_validate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            code = cli_main(
                [
                    "init-task",
                    "--store",
                    tmp,
                    "--task-id",
                    "T1",
                    "--title",
                    "demo",
                    "--head-sha",
                    "h1",
                    "--base-sha",
                    "b1",
                ]
            )
            self.assertEqual(code, 0)
            code = cli_main(
                [
                    "transition",
                    "--store",
                    tmp,
                    "--to-state",
                    "SOURCE_INSPECTION",
                    "--expected-state",
                    "TASK_ACCEPTED",
                    "--actor",
                    "WRITER",
                ]
            )
            self.assertEqual(code, 0)
            code = cli_main(["status", "--store", tmp])
            self.assertEqual(code, 0)
            code = cli_main(["validate-journal", "--store", tmp])
            self.assertEqual(code, 0)
            code = cli_main(["replay", "--store", tmp])
            self.assertEqual(code, 0)
            code = cli_main(
                [
                    "handoff-create",
                    "--store",
                    tmp,
                    "--handoff-id",
                    "h1",
                    "--actor-role",
                    "writer",
                    "--expected-return-state",
                    "IMPLEMENTATION",
                    "--inputs-json",
                    '{"token":"x","ok":"y"}',
                ]
            )
            self.assertEqual(code, 0)

    def test_successful_mutation_exactly_one_generation(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = OrchestratorEngine(OrchestratorStore(tmp))
            self.assertEqual(eng.generation, 0)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            self.assertEqual(eng.generation, 1)
            eng.transition(
                State.SOURCE_INSPECTION,
                Actor.WRITER,
                expected_state=State.TASK_ACCEPTED,
            )
            self.assertEqual(eng.generation, 2)

    def test_stale_generation_write_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            store = OrchestratorStore(tmp)
            eng = OrchestratorEngine(store)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            candidate = BundleState.empty()
            with self.assertRaises(StoreError):
                store.commit_bundle(candidate, expected_generation=0)

    def test_atomic_publish_failure_leaves_previous_bundle(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            store = OrchestratorStore(tmp)
            eng = OrchestratorEngine(store)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            before = (Path(tmp) / "bundle.json").read_text(encoding="utf-8")
            candidate = eng._candidate()
            assert candidate.task is not None
            candidate.task.title = "mutated"
            with mock.patch("os.replace", side_effect=OSError("boom")):
                with self.assertRaises(OSError):
                    store.commit_bundle(candidate, expected_generation=eng.generation)
            after = (Path(tmp) / "bundle.json").read_text(encoding="utf-8")
            self.assertEqual(before, after)

    def test_unknown_bundle_field_rejected(self) -> None:
        with self.assertRaises(ValidationError):
            validate_bundle(
                {
                    "schema_version": 1,
                    "generation": 0,
                    "task": None,
                    "journal": [],
                    "evidence": [],
                    "lanes": [],
                    "migrations": [],
                    "handoffs": [],
                    "extra": True,
                }
            )

    def test_schemas_parse_as_json(self) -> None:
        for name in ("task", "event", "evidence", "handoff", "bundle", "required_gate"):
            path = SCHEMAS / f"{name}.schema.json"
            with path.open(encoding="utf-8") as fh:
                data = json.load(fh)
            self.assertEqual(data["type"], "object")
            self.assertIn("properties", data)
            self.assertFalse(data.get("additionalProperties", True))


if __name__ == "__main__":
    unittest.main()

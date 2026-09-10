"""Handoff, readiness, CLI, and schema tests."""

from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from tools.engineering_orchestrator.cli import main as cli_main
from tools.engineering_orchestrator.engine import OrchestratorEngine
from tools.engineering_orchestrator.handoff import create_handoff, redact_sensitive
from tools.engineering_orchestrator.models import (
    Actor,
    EvidenceResult,
    EvidenceType,
    State,
)
from tools.engineering_orchestrator.readiness import (
    ci_alone_promotes_readiness,
    evaluate_hundred_percent_pass,
    evaluate_readiness,
)
from tools.engineering_orchestrator.store import OrchestratorStore


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
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            handoff = engine.create_handoff(
                handoff_id="h1",
                actor_role="manual-worker",
                expected_return_state="IMPLEMENTATION",
                inputs={"password": "nope", "task": "fix"},
            )
            self.assertFalse(handoff.codex_enabled)
            self.assertEqual(handoff.inputs["password"], "[REDACTED]")

    def test_hundred_percent_pass_positive(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                required_scopes=["default"],
            )
            engine.record_evidence(
                evidence_id="lt1",
                evidence_type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
            )
            result = evaluate_hundred_percent_pass(engine.task, engine.evidence)
            self.assertTrue(result.all_required_pass)

    def test_hundred_percent_pass_negative_missing(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                required_scopes=["default"],
            )
            result = evaluate_hundred_percent_pass(engine.task, engine.evidence)
            self.assertFalse(result.all_required_pass)
            self.assertTrue(result.missing_scopes)

    def test_hundred_percent_pass_stale_after_head_change(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                required_scopes=["default"],
            )
            engine.record_evidence(
                evidence_id="lt1",
                evidence_type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
            )
            engine.set_head("h2", impact_known=False)
            result = evaluate_hundred_percent_pass(engine.task, engine.evidence)
            self.assertFalse(result.all_required_pass)
            self.assertTrue(result.stale_or_invalid)

    def test_readiness_not_promoted_by_ci_alone(self) -> None:
        self.assertFalse(ci_alone_promotes_readiness())
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                required_scopes=["default"],
            )
            engine.record_evidence(
                evidence_id="ci1",
                evidence_type=EvidenceType.CI,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
            )
            # Satisfy 100%-pass with local test too, but usable set only CI for
            # the CI-alone path: remove local and evaluate with CI-only usable.
            evaluation = evaluate_readiness(
                engine.task,
                engine.evidence,
                actor=Actor.REALITY_CHECKER,
                reality_gate_pass=True,
            )
            self.assertFalse(evaluation.readiness)
            self.assertIn("CI alone", evaluation.reason)

    def test_readiness_requires_reality_checker(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                required_scopes=["default"],
            )
            engine.record_evidence(
                evidence_id="lt1",
                evidence_type=EvidenceType.LOCAL_TEST,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
            )
            evaluation = evaluate_readiness(
                engine.task,
                engine.evidence,
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

    def test_schemas_parse_as_json(self) -> None:
        for name in ("task", "event", "evidence", "handoff"):
            path = SCHEMAS / f"{name}.schema.json"
            with path.open(encoding="utf-8") as fh:
                data = json.load(fh)
            self.assertEqual(data["type"], "object")
            self.assertIn("properties", data)


if __name__ == "__main__":
    unittest.main()

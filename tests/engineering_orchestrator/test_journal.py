"""Journal continuity, reconcile SHA rules, resume, and bundle load agreement."""

from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from tools.engineering_orchestrator.engine import EngineError, OrchestratorEngine
from tools.engineering_orchestrator.journal import Journal, JournalError
from tools.engineering_orchestrator.models import (
    Actor,
    EvidenceResult,
    EvidenceType,
    FailureClass,
    RcaStatus,
    State,
)
from tools.engineering_orchestrator.store import OrchestratorStore

from helpers import advance_to_full_validation, engine, reconcile_to_independent_review


class JournalTests(unittest.TestCase):
    def test_hash_chain_tamper_detection(self) -> None:
        j = Journal()
        j.append(
            task_id="T1",
            from_state=State.TASK_ACCEPTED,
            to_state=State.SOURCE_INSPECTION,
            head_sha="h1",
            base_sha="b1",
            evidence_ids=[],
            actor=Actor.WRITER,
        )
        j._events[0].notes = "tampered"
        with self.assertRaises(JournalError):
            j.validate()

    def test_ordinary_event_rejects_head_change(self) -> None:
        j = Journal()
        j.append(
            task_id="T1",
            from_state=State.TASK_ACCEPTED,
            to_state=State.SOURCE_INSPECTION,
            head_sha="h1",
            base_sha="b1",
            evidence_ids=[],
            actor=Actor.WRITER,
        )
        j.append(
            task_id="T1",
            from_state=State.SOURCE_INSPECTION,
            to_state=State.AGENT_ROUTING,
            head_sha="h2",
            base_sha="b1",
            evidence_ids=[],
            actor=Actor.WRITER,
        )
        with self.assertRaises(JournalError):
            j.validate()

    def test_ordinary_event_rejects_base_change(self) -> None:
        j = Journal()
        j.append(
            task_id="T1",
            from_state=State.TASK_ACCEPTED,
            to_state=State.SOURCE_INSPECTION,
            head_sha="h1",
            base_sha="b1",
            evidence_ids=[],
            actor=Actor.WRITER,
        )
        j.append(
            task_id="T1",
            from_state=State.SOURCE_INSPECTION,
            to_state=State.AGENT_ROUTING,
            head_sha="h1",
            base_sha="b2",
            evidence_ids=[],
            actor=Actor.WRITER,
        )
        with self.assertRaises(JournalError):
            j.validate()

    def test_reconcile_expected_sha_validation(self) -> None:
        j = Journal()
        # Build minimal path to FULL_VALIDATION via writer edges — skip for unit:
        # directly append reconcile-like after a seed event is invalid without path.
        # Use engine path instead.
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            advance_to_full_validation(eng)
            eng.record_evidence(
                evidence_id="src-verify",
                evidence_type=EvidenceType.SOURCE,
                head_sha="h1",
                claim_scope="default",
                result=EvidenceResult.PASS,
                collected_at="t",
                command_identity="src",
            )
            with self.assertRaises(EngineError):
                eng.reconcile_base(
                    expected_state=State.FULL_VALIDATION,
                    expected_head_sha="WRONG",
                    expected_base_sha="b1",
                    new_head_sha="h2",
                    new_base_sha="b2",
                    changed_paths=["tools/engineering_orchestrator/engine.py"],
                    verification_evidence_ids=["src-verify"],
                    impact_known=True,
                )
            with self.assertRaises(EngineError):
                eng.reconcile_base(
                    expected_state=State.FULL_VALIDATION,
                    expected_head_sha="h1",
                    expected_base_sha="WRONG",
                    new_head_sha="h2",
                    new_base_sha="b2",
                    changed_paths=["tools/engineering_orchestrator/engine.py"],
                    verification_evidence_ids=["src-verify"],
                    impact_known=True,
                )
            # Successful reconcile then journal validates
            eng.reconcile_base(
                expected_state=State.FULL_VALIDATION,
                expected_head_sha="h1",
                expected_base_sha="b1",
                new_head_sha="h2",
                new_base_sha="b2",
                changed_paths=["tools/engineering_orchestrator/engine.py"],
                verification_evidence_ids=["src-verify"],
                impact_known=True,
            )
            eng.journal.validate(enforce_transitions=True)
            self.assertEqual(eng.task.head_sha, "h2")
            self.assertEqual(eng.task.base_sha, "b2")

    def test_task_journal_state_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            eng.transition(
                State.SOURCE_INSPECTION,
                Actor.WRITER,
                expected_state=State.TASK_ACCEPTED,
            )
            bundle_path = Path(tmp) / "bundle.json"
            data = json.loads(bundle_path.read_text(encoding="utf-8"))
            data["task"]["state"] = "IMPLEMENTATION"
            bundle_path.write_text(json.dumps(data), encoding="utf-8")
            with self.assertRaises(EngineError):
                OrchestratorEngine(OrchestratorStore(tmp))

    def test_task_journal_head_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            eng.transition(
                State.SOURCE_INSPECTION,
                Actor.WRITER,
                expected_state=State.TASK_ACCEPTED,
            )
            bundle_path = Path(tmp) / "bundle.json"
            data = json.loads(bundle_path.read_text(encoding="utf-8"))
            data["task"]["head_sha"] = "TAMPERED"
            bundle_path.write_text(json.dumps(data), encoding="utf-8")
            with self.assertRaises(EngineError):
                OrchestratorEngine(OrchestratorStore(tmp))

    def test_task_journal_base_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            eng.transition(
                State.SOURCE_INSPECTION,
                Actor.WRITER,
                expected_state=State.TASK_ACCEPTED,
            )
            bundle_path = Path(tmp) / "bundle.json"
            data = json.loads(bundle_path.read_text(encoding="utf-8"))
            data["task"]["base_sha"] = "TAMPERED"
            bundle_path.write_text(json.dumps(data), encoding="utf-8")
            with self.assertRaises(EngineError):
                OrchestratorEngine(OrchestratorStore(tmp))

    def test_blocked_resume_tampering_and_journal_target(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            eng.transition(
                State.SOURCE_INSPECTION,
                Actor.WRITER,
                expected_state=State.TASK_ACCEPTED,
            )
            eng.transition(
                State.AGENT_ROUTING,
                Actor.WRITER,
                expected_state=State.SOURCE_INSPECTION,
            )
            eng.apply_failure(
                FailureClass.EVIDENCE_GAP,
                Actor.WRITER,
                rca_status=RcaStatus.KNOWN,
            )
            self.assertEqual(eng.task.state, State.BLOCKED)
            self.assertEqual(eng.task.resume_state, State.AGENT_ROUTING)
            # Tamper resume to wrong state
            with self.assertRaises(EngineError):
                eng.transition(
                    State.SOURCE_INSPECTION,
                    Actor.FOUNDER_CTO,
                    expected_state=State.BLOCKED,
                )
            eng.transition(
                State.AGENT_ROUTING,
                Actor.FOUNDER_CTO,
                expected_state=State.BLOCKED,
            )
            self.assertIsNone(eng.task.resume_state)

    def test_replay_and_validate_cli_paths(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            eng = engine(tmp)
            eng.init_task(task_id="T1", title="t", head_sha="h1", base_sha="b1")
            advance_to_full_validation(eng)
            reconcile_to_independent_review(eng)
            result = eng.validate_journal()
            self.assertTrue(result["ok"])
            replay = eng.replay()
            self.assertEqual(replay["state"], State.INDEPENDENT_REVIEW.value)


if __name__ == "__main__":
    unittest.main()

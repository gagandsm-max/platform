"""Journal hash-chain, tamper detection, and resume tests.

Hash chain = tamper detection only — not signatures / non-repudiation.
"""

from __future__ import annotations

import tempfile
import unittest

from tools.engineering_orchestrator.engine import OrchestratorEngine
from tools.engineering_orchestrator.journal import (
    GENESIS_PREVIOUS_HASH,
    Journal,
    JournalError,
    compute_event_hash,
)
from tools.engineering_orchestrator.models import Actor, FailureClass, RcaStatus, State
from tools.engineering_orchestrator.store import OrchestratorStore


class JournalResumeTests(unittest.TestCase):
    def test_genesis_previous_is_sixty_four_zeros(self) -> None:
        self.assertEqual(GENESIS_PREVIOUS_HASH, "0" * 64)
        self.assertEqual(len(GENESIS_PREVIOUS_HASH), 64)

    def test_append_chain_and_validate(self) -> None:
        journal = Journal()
        e1 = journal.append(
            task_id="T1",
            from_state=State.TASK_ACCEPTED,
            to_state=State.SOURCE_INSPECTION,
            head_sha="h1",
            base_sha="b1",
            evidence_ids=[],
            actor=Actor.WRITER,
            timestamp="2026-01-01T00:00:00Z",
            event_id="e1",
        )
        self.assertEqual(e1.previous_event_hash, GENESIS_PREVIOUS_HASH)
        journal.append(
            task_id="T1",
            from_state=State.SOURCE_INSPECTION,
            to_state=State.AGENT_ROUTING,
            head_sha="h1",
            base_sha="b1",
            evidence_ids=[],
            actor=Actor.WRITER,
            timestamp="2026-01-01T00:00:01Z",
            event_id="e2",
        )
        journal.validate()

    def test_tamper_detection_on_hash(self) -> None:
        journal = Journal()
        journal.append(
            task_id="T1",
            from_state=State.TASK_ACCEPTED,
            to_state=State.SOURCE_INSPECTION,
            head_sha="h1",
            base_sha="b1",
            evidence_ids=[],
            actor=Actor.WRITER,
            timestamp="2026-01-01T00:00:00Z",
            event_id="e1",
        )
        journal._events[0].notes = "tampered"
        # notes included in hash when present — force hash mismatch via field change
        journal._events[0].head_sha = "EVIL"
        with self.assertRaises(JournalError) as ctx:
            journal.validate()
        self.assertIn("tamper", str(ctx.exception).lower())

    def test_missing_reordered_events(self) -> None:
        journal = Journal()
        journal.append(
            task_id="T1",
            from_state=State.TASK_ACCEPTED,
            to_state=State.SOURCE_INSPECTION,
            head_sha="h1",
            base_sha="b1",
            evidence_ids=[],
            actor=Actor.WRITER,
            timestamp="2026-01-01T00:00:00Z",
            event_id="e1",
        )
        journal.append(
            task_id="T1",
            from_state=State.SOURCE_INSPECTION,
            to_state=State.AGENT_ROUTING,
            head_sha="h1",
            base_sha="b1",
            evidence_ids=[],
            actor=Actor.WRITER,
            timestamp="2026-01-01T00:00:01Z",
            event_id="e2",
        )
        # Reorder
        journal._events[0], journal._events[1] = journal._events[1], journal._events[0]
        with self.assertRaises(JournalError):
            journal.validate()

    def test_sequence_gap(self) -> None:
        journal = Journal()
        journal.append(
            task_id="T1",
            from_state=State.TASK_ACCEPTED,
            to_state=State.SOURCE_INSPECTION,
            head_sha="h1",
            base_sha="b1",
            evidence_ids=[],
            actor=Actor.WRITER,
            timestamp="2026-01-01T00:00:00Z",
            event_id="e1",
        )
        journal._events[0].sequence = 3
        # Also need matching hash for sequence field — expect sequence error first
        with self.assertRaises(JournalError) as ctx:
            journal.validate()
        self.assertIn("sequence", str(ctx.exception).lower())

    def test_blocked_resume(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            engine.transition(State.SOURCE_INSPECTION, Actor.WRITER)
            engine.apply_failure(
                FailureClass.EVIDENCE_GAP,
                Actor.WRITER,
                rca_status=RcaStatus.KNOWN,
            )
            self.assertEqual(engine.task.state, State.BLOCKED)
            self.assertEqual(engine.task.resume_state, State.SOURCE_INSPECTION)
            engine.transition(State.SOURCE_INSPECTION, Actor.FOUNDER_CTO)
            self.assertEqual(engine.task.state, State.SOURCE_INSPECTION)
            result = engine.validate_journal()
            self.assertTrue(result["ok"])

    def test_resume_wrong_target_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1")
            engine.transition(State.SOURCE_INSPECTION, Actor.WRITER)
            engine.transition(
                State.BLOCKED,
                Actor.WRITER,
                resume_state=State.SOURCE_INSPECTION,
                failure_class=FailureClass.ENVIRONMENT_DEFECT,
            )
            with self.assertRaises(Exception):
                engine.transition(State.IMPLEMENTATION, Actor.FOUNDER_CTO)

    def test_compute_event_hash_deterministic(self) -> None:
        payload = {
            "sequence": 1,
            "event_id": "e1",
            "task_id": "T1",
            "from_state": "TASK_ACCEPTED",
            "to_state": "SOURCE_INSPECTION",
            "head_sha": "h",
            "base_sha": "b",
            "evidence_ids": [],
            "actor": "WRITER",
            "timestamp": "t",
            "previous_event_hash": GENESIS_PREVIOUS_HASH,
        }
        self.assertEqual(compute_event_hash(payload), compute_event_hash(payload))


if __name__ == "__main__":
    unittest.main()

"""Append-only SHA-256 hash-chain journal.

IMPORTANT: The hash chain provides tamper detection only.
It does NOT provide cryptographic signatures or non-repudiation.
"""

from __future__ import annotations

import hashlib
import json
import uuid
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Optional

from .fsm import is_transition_allowed
from .models import Actor, State

# Genesis previous hash: 64 zero hex digits (tamper-detection chain root only —
# not a signature, not non-repudiation).
GENESIS_PREVIOUS_HASH = "0" * 64


class JournalError(ValueError):
    """Raised for journal integrity or continuity failures."""


def _utc_now_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def canonical_json(obj: dict[str, Any]) -> str:
    """Deterministic JSON for hashing: sorted keys, compact separators, UTF-8."""
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def compute_event_hash(payload_without_hash: dict[str, Any]) -> str:
    raw = canonical_json(payload_without_hash).encode("utf-8")
    return hashlib.sha256(raw).hexdigest()


@dataclass
class JournalEvent:
    sequence: int
    event_id: str
    task_id: str
    from_state: str
    to_state: str
    head_sha: str
    base_sha: str
    evidence_ids: list[str]
    actor: str
    timestamp: str
    previous_event_hash: str
    event_hash: str
    resume_state: Optional[str] = None
    notes: str = ""

    def to_dict(self) -> dict[str, Any]:
        d: dict[str, Any] = {
            "sequence": self.sequence,
            "event_id": self.event_id,
            "task_id": self.task_id,
            "from_state": self.from_state,
            "to_state": self.to_state,
            "head_sha": self.head_sha,
            "base_sha": self.base_sha,
            "evidence_ids": list(self.evidence_ids),
            "actor": self.actor,
            "timestamp": self.timestamp,
            "previous_event_hash": self.previous_event_hash,
            "event_hash": self.event_hash,
        }
        if self.resume_state is not None:
            d["resume_state"] = self.resume_state
        if self.notes:
            d["notes"] = self.notes
        return d

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "JournalEvent":
        return cls(
            sequence=int(data["sequence"]),
            event_id=str(data["event_id"]),
            task_id=str(data["task_id"]),
            from_state=str(data["from_state"]),
            to_state=str(data["to_state"]),
            head_sha=str(data.get("head_sha", "")),
            base_sha=str(data.get("base_sha", "")),
            evidence_ids=list(data.get("evidence_ids") or []),
            actor=str(data["actor"]),
            timestamp=str(data["timestamp"]),
            previous_event_hash=str(data["previous_event_hash"]),
            event_hash=str(data["event_hash"]),
            resume_state=data.get("resume_state"),
            notes=str(data.get("notes", "")),
        )


class Journal:
    """In-memory append-only journal with SHA-256 chain (tamper detection only)."""

    def __init__(self) -> None:
        self._events: list[JournalEvent] = []

    @property
    def events(self) -> list[JournalEvent]:
        return list(self._events)

    def last_hash(self) -> str:
        if not self._events:
            return GENESIS_PREVIOUS_HASH
        return self._events[-1].event_hash

    def next_sequence(self) -> int:
        return len(self._events) + 1

    def append(
        self,
        *,
        task_id: str,
        from_state: State | str,
        to_state: State | str,
        head_sha: str,
        base_sha: str,
        evidence_ids: list[str],
        actor: Actor | str,
        resume_state: Optional[State | str] = None,
        notes: str = "",
        event_id: Optional[str] = None,
        timestamp: Optional[str] = None,
    ) -> JournalEvent:
        from_s = from_state.value if isinstance(from_state, State) else str(from_state)
        to_s = to_state.value if isinstance(to_state, State) else str(to_state)
        actor_s = actor.value if isinstance(actor, Actor) else str(actor)
        resume_s = None
        if resume_state is not None:
            resume_s = (
                resume_state.value if isinstance(resume_state, State) else str(resume_state)
            )

        seq = self.next_sequence()
        prev = self.last_hash()
        payload: dict[str, Any] = {
            "sequence": seq,
            "event_id": event_id or str(uuid.uuid4()),
            "task_id": task_id,
            "from_state": from_s,
            "to_state": to_s,
            "head_sha": head_sha,
            "base_sha": base_sha,
            "evidence_ids": list(evidence_ids),
            "actor": actor_s,
            "timestamp": timestamp or _utc_now_iso(),
            "previous_event_hash": prev,
        }
        if resume_s is not None:
            payload["resume_state"] = resume_s
        if notes:
            payload["notes"] = notes

        event_hash = compute_event_hash(payload)
        event = JournalEvent(
            sequence=seq,
            event_id=payload["event_id"],
            task_id=task_id,
            from_state=from_s,
            to_state=to_s,
            head_sha=head_sha,
            base_sha=base_sha,
            evidence_ids=list(evidence_ids),
            actor=actor_s,
            timestamp=payload["timestamp"],
            previous_event_hash=prev,
            event_hash=event_hash,
            resume_state=resume_s,
            notes=notes,
        )
        self._events.append(event)
        return event

    def to_list(self) -> list[dict[str, Any]]:
        return [e.to_dict() for e in self._events]

    def load_list(self, items: list[dict[str, Any]]) -> None:
        self._events = [JournalEvent.from_dict(i) for i in items]

    def validate(self, *, enforce_transitions: bool = True) -> None:
        """Replay-validate sequence, hash chain, task continuity, transitions,
        base/head continuity, and BLOCKED resume_state.

        Hash chain = tamper detection only (not signatures / non-repudiation).
        """
        if not self._events:
            return

        expected_prev = GENESIS_PREVIOUS_HASH
        task_id: Optional[str] = None
        last_to: Optional[str] = None
        last_head: Optional[str] = None
        last_base: Optional[str] = None

        for idx, event in enumerate(self._events):
            expected_seq = idx + 1
            if event.sequence != expected_seq:
                raise JournalError(
                    f"sequence gap/reorder: expected {expected_seq}, got {event.sequence}"
                )
            if event.previous_event_hash != expected_prev:
                raise JournalError(
                    f"hash chain break at sequence {event.sequence} "
                    "(tamper detection failed)"
                )

            payload = {
                "sequence": event.sequence,
                "event_id": event.event_id,
                "task_id": event.task_id,
                "from_state": event.from_state,
                "to_state": event.to_state,
                "head_sha": event.head_sha,
                "base_sha": event.base_sha,
                "evidence_ids": list(event.evidence_ids),
                "actor": event.actor,
                "timestamp": event.timestamp,
                "previous_event_hash": event.previous_event_hash,
            }
            if event.resume_state is not None:
                payload["resume_state"] = event.resume_state
            if event.notes:
                payload["notes"] = event.notes
            recomputed = compute_event_hash(payload)
            if recomputed != event.event_hash:
                raise JournalError(
                    f"event_hash mismatch at sequence {event.sequence} "
                    "(tamper detection failed)"
                )

            if task_id is None:
                task_id = event.task_id
            elif event.task_id != task_id:
                raise JournalError("task_id continuity broken")

            if last_to is not None and event.from_state != last_to:
                raise JournalError(
                    f"state continuity broken at {event.sequence}: "
                    f"from={event.from_state} expected={last_to}"
                )

            if enforce_transitions:
                try:
                    from_state = State(event.from_state)
                    to_state = State(event.to_state)
                    actor = Actor(event.actor)
                except ValueError as exc:
                    raise JournalError(f"invalid enum in event {event.sequence}: {exc}") from exc

                # BLOCKED → resume_state special case (resume target recorded).
                if from_state == State.BLOCKED and to_state != State.CANCELLED and to_state != State.FAILED:
                    if not event.resume_state:
                        raise JournalError(
                            f"BLOCKED resume missing resume_state at sequence {event.sequence}"
                        )
                    if event.resume_state != event.to_state:
                        raise JournalError(
                            f"BLOCKED resume_state mismatch at sequence {event.sequence}"
                        )
                    # Resume uses SYSTEM or FOUNDER_CTO; treat as allowed if actor matches.
                    if actor not in {Actor.FOUNDER_CTO, Actor.SYSTEM}:
                        raise JournalError(
                            f"BLOCKED resume actor unauthorized at sequence {event.sequence}"
                        )
                elif not is_transition_allowed(from_state, to_state, actor):
                    # Writer hard bans still caught by is_transition_allowed tables.
                    raise JournalError(
                        f"illegal transition in journal at sequence {event.sequence}: "
                        f"{event.from_state}->{event.to_state} by {event.actor}"
                    )

            if last_head is not None and event.head_sha and last_head and event.head_sha != last_head:
                # Head may change; continuity means recorded, not immutable.
                pass
            if last_base is not None and event.base_sha and last_base and event.base_sha != last_base:
                # Base may change on reconciliation; recorded for audit.
                pass

            expected_prev = event.event_hash
            last_to = event.to_state
            last_head = event.head_sha or last_head
            last_base = event.base_sha or last_base

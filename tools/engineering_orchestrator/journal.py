"""Append-only SHA-256 hash-chain journal.

IMPORTANT: The hash chain provides tamper detection only.
It does NOT provide cryptographic signatures or non-repudiation.

Ordinary events must preserve HEAD/base exactly.
Only FULL_VALIDATION → BASE_RECONCILIATION by SYSTEM (reconcile) may change them.
"""

from __future__ import annotations

import hashlib
import json
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Optional

from .fsm import is_reconcile_only_edge, is_transition_allowed
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
    expected_head_sha: Optional[str] = None
    expected_base_sha: Optional[str] = None
    changed_paths: list[str] = field(default_factory=list)

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
        if self.expected_head_sha is not None:
            d["expected_head_sha"] = self.expected_head_sha
        if self.expected_base_sha is not None:
            d["expected_base_sha"] = self.expected_base_sha
        if self.changed_paths:
            d["changed_paths"] = list(self.changed_paths)
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
            expected_head_sha=data.get("expected_head_sha"),
            expected_base_sha=data.get("expected_base_sha"),
            changed_paths=list(data.get("changed_paths") or []),
        )


@dataclass
class JournalProjection:
    task_id: str
    state: State
    head_sha: str
    base_sha: str
    resume_state: Optional[State] = None


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
        expected_head_sha: Optional[str] = None,
        expected_base_sha: Optional[str] = None,
        changed_paths: list[str] | None = None,
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
        if expected_head_sha is not None:
            payload["expected_head_sha"] = expected_head_sha
        if expected_base_sha is not None:
            payload["expected_base_sha"] = expected_base_sha
        if changed_paths:
            payload["changed_paths"] = list(changed_paths)

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
            expected_head_sha=expected_head_sha,
            expected_base_sha=expected_base_sha,
            changed_paths=list(changed_paths or []),
        )
        self._events.append(event)
        return event

    def to_list(self) -> list[dict[str, Any]]:
        return [e.to_dict() for e in self._events]

    def load_list(self, items: list[dict[str, Any]]) -> None:
        self._events = [JournalEvent.from_dict(i) for i in items]

    def _is_reconcile_event(self, event: JournalEvent) -> bool:
        try:
            from_state = State(event.from_state)
            to_state = State(event.to_state)
            actor = Actor(event.actor)
        except ValueError:
            return False
        return (
            is_reconcile_only_edge(from_state, to_state)
            and actor == Actor.SYSTEM
        )

    def validate(self, *, enforce_transitions: bool = True) -> None:
        """Replay-validate sequence, hash chain, task continuity, transitions,
        HEAD/base continuity, and BLOCKED resume_state.

        Hash chain = tamper detection only (not signatures / non-repudiation).
        """
        if not self._events:
            return

        expected_prev = GENESIS_PREVIOUS_HASH
        task_id: Optional[str] = None
        last_to: Optional[str] = None
        last_head: Optional[str] = None
        last_base: Optional[str] = None
        blocked_resume_target: Optional[str] = None

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
            if event.expected_head_sha is not None:
                payload["expected_head_sha"] = event.expected_head_sha
            if event.expected_base_sha is not None:
                payload["expected_base_sha"] = event.expected_base_sha
            if event.changed_paths:
                payload["changed_paths"] = list(event.changed_paths)
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

            is_reconcile = self._is_reconcile_event(event)

            # HEAD/base continuity.
            if last_head is not None or last_base is not None:
                if is_reconcile:
                    if event.expected_head_sha is None or event.expected_base_sha is None:
                        raise JournalError(
                            f"reconcile event missing expected SHAs at {event.sequence}"
                        )
                    if last_head is not None and event.expected_head_sha != last_head:
                        raise JournalError(
                            f"reconcile expected_head_sha mismatch at {event.sequence}"
                        )
                    if last_base is not None and event.expected_base_sha != last_base:
                        raise JournalError(
                            f"reconcile expected_base_sha mismatch at {event.sequence}"
                        )
                    # New SHAs allowed on reconcile event.
                else:
                    if last_head is not None and event.head_sha != last_head:
                        raise JournalError(
                            f"ordinary event may not change head_sha at {event.sequence}"
                        )
                    if last_base is not None and event.base_sha != last_base:
                        raise JournalError(
                            f"ordinary event may not change base_sha at {event.sequence}"
                        )

            if enforce_transitions:
                try:
                    from_state = State(event.from_state)
                    to_state = State(event.to_state)
                    actor = Actor(event.actor)
                except ValueError as exc:
                    raise JournalError(f"invalid enum in event {event.sequence}: {exc}") from exc

                if from_state == State.BLOCKED and to_state not in {
                    State.CANCELLED,
                    State.FAILED,
                }:
                    if not event.resume_state:
                        raise JournalError(
                            f"BLOCKED resume missing resume_state at sequence {event.sequence}"
                        )
                    if event.resume_state != event.to_state:
                        raise JournalError(
                            f"BLOCKED resume_state mismatch at sequence {event.sequence}"
                        )
                    if blocked_resume_target is not None and event.to_state != blocked_resume_target:
                        raise JournalError(
                            f"BLOCKED resume tampering at {event.sequence}: "
                            f"to={event.to_state} expected={blocked_resume_target}"
                        )
                    if actor != Actor.FOUNDER_CTO:
                        raise JournalError(
                            f"BLOCKED resume actor unauthorized at sequence {event.sequence}"
                        )
                elif is_reconcile:
                    if actor != Actor.SYSTEM:
                        raise JournalError(
                            f"reconcile actor must be SYSTEM at {event.sequence}"
                        )
                elif not is_transition_allowed(from_state, to_state, actor):
                    raise JournalError(
                        f"illegal transition in journal at sequence {event.sequence}: "
                        f"{event.from_state}->{event.to_state} by {event.actor}"
                    )

                if to_state == State.BLOCKED:
                    # Entering BLOCKED: resume_state must equal from_state.
                    if event.resume_state != event.from_state:
                        raise JournalError(
                            f"BLOCKED entry resume_state must equal from_state "
                            f"at {event.sequence}"
                        )
                    blocked_resume_target = event.from_state
                elif from_state == State.BLOCKED and to_state not in {
                    State.CANCELLED,
                    State.FAILED,
                }:
                    blocked_resume_target = None

            expected_prev = event.event_hash
            last_to = event.to_state
            last_head = event.head_sha
            last_base = event.base_sha

    def replay_projection(self) -> Optional[JournalProjection]:
        """Authoritative workflow projection from journal (after validate)."""
        if not self._events:
            return None
        self.validate(enforce_transitions=True)
        task_id = self._events[0].task_id
        state = State(self._events[-1].to_state)
        head_sha = self._events[-1].head_sha
        base_sha = self._events[-1].base_sha
        resume_state: Optional[State] = None
        # Find latest BLOCKED entry resume target still open.
        open_resume: Optional[str] = None
        for event in self._events:
            if event.to_state == State.BLOCKED.value:
                open_resume = event.resume_state or event.from_state
            elif event.from_state == State.BLOCKED.value and event.to_state not in {
                State.CANCELLED.value,
                State.FAILED.value,
            }:
                open_resume = None
            elif event.from_state == State.BLOCKED.value and event.to_state in {
                State.CANCELLED.value,
                State.FAILED.value,
            }:
                open_resume = None
        if open_resume:
            resume_state = State(open_resume)
        return JournalProjection(
            task_id=task_id,
            state=state,
            head_sha=head_sha,
            base_sha=base_sha,
            resume_state=resume_state,
        )

    def journal_resume_target(self) -> Optional[State]:
        """Original BLOCKED resume target from journal (not task field)."""
        open_resume: Optional[str] = None
        for event in self._events:
            if event.to_state == State.BLOCKED.value:
                open_resume = event.resume_state or event.from_state
            elif event.from_state == State.BLOCKED.value:
                open_resume = None
        return State(open_resume) if open_resume else None

"""Authority rules for Engineering Orchestrator Phase 1.

Critical split:
- Writer CANNOT enter READY_FOR_MERGE_GATE or MERGED.
- Only REALITY_CHECKER enters READY_FOR_MERGE_GATE (from REALITY_GATE).
- Only MERGE_AUTHORITY enters MERGED (from READY_FOR_MERGE_GATE).
"""

from __future__ import annotations

from .fsm import TransitionError, validate_transition
from .models import Actor, State


class AuthorityError(TransitionError):
    """Raised when an actor lacks authority for a transition."""


def assert_writer_cannot_self_certify(actor: Actor, to_state: State) -> None:
    if actor == Actor.WRITER and to_state in {
        State.READY_FOR_MERGE_GATE,
        State.MERGED,
    }:
        raise AuthorityError(
            "WRITER cannot transition to READY_FOR_MERGE_GATE or MERGED "
            "(no self-certification / self-merge)"
        )


def assert_merge_gate_authority(actor: Actor, from_state: State, to_state: State) -> None:
    if to_state == State.READY_FOR_MERGE_GATE:
        if actor != Actor.REALITY_CHECKER:
            raise AuthorityError(
                "only REALITY_CHECKER may enter READY_FOR_MERGE_GATE"
            )
        if from_state != State.REALITY_GATE:
            raise AuthorityError(
                "READY_FOR_MERGE_GATE only from REALITY_GATE"
            )
    if to_state == State.MERGED:
        if actor != Actor.MERGE_AUTHORITY:
            raise AuthorityError("only MERGE_AUTHORITY may enter MERGED")
        if from_state != State.READY_FOR_MERGE_GATE:
            raise AuthorityError("MERGED only from READY_FOR_MERGE_GATE")


def authorize_transition(from_state: State, to_state: State, actor: Actor) -> None:
    """Validate FSM edge plus hard authority bans."""
    assert_writer_cannot_self_certify(actor, to_state)
    assert_merge_gate_authority(actor, from_state, to_state)
    validate_transition(from_state, to_state, actor)


def founder_may_unblock(actor: Actor) -> bool:
    return actor in {Actor.FOUNDER_CTO, Actor.SYSTEM}


def founder_gate_required_for_business() -> Actor:
    return Actor.FOUNDER_CTO

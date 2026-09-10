"""Finite-state machine tables for Engineering Orchestrator Phase 1.

Transition tables encode allowed (from, to, actors). Authority-critical
edges are also enforced in authority.py.
"""

from __future__ import annotations

from .models import Actor, State, TERMINAL_STATES

# Happy-path sequential edges (writer/system may advance until REALITY_GATE).
_HAPPY_EDGES: list[tuple[State, State]] = [
    (State.TASK_ACCEPTED, State.SOURCE_INSPECTION),
    (State.SOURCE_INSPECTION, State.AGENT_ROUTING),
    (State.AGENT_ROUTING, State.RCA_COMPLETE),
    (State.RCA_COMPLETE, State.IMPLEMENTATION),
    (State.IMPLEMENTATION, State.FOCUSED_VALIDATION),
    (State.FOCUSED_VALIDATION, State.FULL_VALIDATION),
    (State.FULL_VALIDATION, State.BASE_RECONCILIATION),
    (State.BASE_RECONCILIATION, State.INDEPENDENT_REVIEW),
    (State.INDEPENDENT_REVIEW, State.EXACT_HEAD_CI),
    (State.EXACT_HEAD_CI, State.REALITY_GATE),
]

REWORK_REENTRY_TARGETS = frozenset(
    {
        State.SOURCE_INSPECTION,
        State.RCA_COMPLETE,
        State.IMPLEMENTATION,
        State.FOCUSED_VALIDATION,
    }
)

# Explicit allowed (from_state, to_state) -> frozenset of actors.
ALLOWED_TRANSITIONS: dict[tuple[State, State], frozenset[Actor]] = {}

for _src, _dst in _HAPPY_EDGES:
    ALLOWED_TRANSITIONS[(_src, _dst)] = frozenset({Actor.WRITER, Actor.SYSTEM})

# Reality Checker owns ready-for-merge entry.
ALLOWED_TRANSITIONS[(State.REALITY_GATE, State.READY_FOR_MERGE_GATE)] = frozenset(
    {Actor.REALITY_CHECKER}
)

# Merge Authority owns MERGED.
ALLOWED_TRANSITIONS[(State.READY_FOR_MERGE_GATE, State.MERGED)] = frozenset(
    {Actor.MERGE_AUTHORITY}
)
ALLOWED_TRANSITIONS[(State.READY_FOR_MERGE_GATE, State.NOT_MERGED)] = frozenset(
    {Actor.MERGE_AUTHORITY, Actor.FOUNDER_CTO}
)

# Post-ready reopen before merge.
ALLOWED_TRANSITIONS[(State.READY_FOR_MERGE_GATE, State.REWORK)] = frozenset(
    {Actor.REALITY_CHECKER, Actor.FOUNDER_CTO}
)
ALLOWED_TRANSITIONS[(State.READY_FOR_MERGE_GATE, State.BLOCKED)] = frozenset(
    {Actor.REALITY_CHECKER, Actor.FOUNDER_CTO}
)
ALLOWED_TRANSITIONS[(State.READY_FOR_MERGE_GATE, State.CANCELLED)] = frozenset(
    {Actor.FOUNDER_CTO}
)

# REWORK re-entry.
for _target in REWORK_REENTRY_TARGETS:
    ALLOWED_TRANSITIONS[(State.REWORK, _target)] = frozenset({Actor.WRITER, Actor.SYSTEM})

# BLOCKED resume / terminal decisions — resume_state validated at engine layer.
ALLOWED_TRANSITIONS[(State.BLOCKED, State.CANCELLED)] = frozenset({Actor.FOUNDER_CTO})
ALLOWED_TRANSITIONS[(State.BLOCKED, State.FAILED)] = frozenset({Actor.FOUNDER_CTO})

# Non-terminal work states that can enter REWORK / BLOCKED / FAILED / CANCELLED.
_NON_TERMINAL_WORK = [
    s
    for s in State
    if s not in TERMINAL_STATES and s not in {State.BLOCKED, State.READY_FOR_MERGE_GATE}
]

_REWORK_FROM_ACTORS = frozenset({Actor.WRITER, Actor.REALITY_CHECKER})
_BLOCK_ACTORS = frozenset(
    {Actor.WRITER, Actor.REALITY_CHECKER, Actor.FOUNDER_CTO, Actor.SYSTEM}
)
_FAIL_ACTORS = frozenset({Actor.WRITER, Actor.REALITY_CHECKER, Actor.FOUNDER_CTO})
_CANCEL_WRITER_ALLOWED = frozenset({Actor.FOUNDER_CTO, Actor.WRITER})

for _s in _NON_TERMINAL_WORK:
    ALLOWED_TRANSITIONS[(_s, State.REWORK)] = _REWORK_FROM_ACTORS
    ALLOWED_TRANSITIONS[(_s, State.BLOCKED)] = _BLOCK_ACTORS
    ALLOWED_TRANSITIONS[(_s, State.FAILED)] = _FAIL_ACTORS
    ALLOWED_TRANSITIONS[(_s, State.CANCELLED)] = _CANCEL_WRITER_ALLOWED

# REWORK itself can block/fail/cancel.
ALLOWED_TRANSITIONS[(State.REWORK, State.BLOCKED)] = _BLOCK_ACTORS
ALLOWED_TRANSITIONS[(State.REWORK, State.FAILED)] = _FAIL_ACTORS
ALLOWED_TRANSITIONS[(State.REWORK, State.CANCELLED)] = _CANCEL_WRITER_ALLOWED


class TransitionError(ValueError):
    """Raised when a requested FSM transition is illegal."""


def is_terminal(state: State) -> bool:
    return state in TERMINAL_STATES


def allowed_actors(from_state: State, to_state: State) -> frozenset[Actor]:
    return ALLOWED_TRANSITIONS.get((from_state, to_state), frozenset())


def is_transition_allowed(from_state: State, to_state: State, actor: Actor) -> bool:
    actors = allowed_actors(from_state, to_state)
    return actor in actors


def validate_transition(from_state: State, to_state: State, actor: Actor) -> None:
    if from_state == to_state:
        raise TransitionError(f"no-op transition rejected: {from_state.value}")
    if is_terminal(from_state):
        raise TransitionError(f"terminal state {from_state.value} cannot transition")
    actors = allowed_actors(from_state, to_state)
    if not actors:
        raise TransitionError(
            f"transition not defined: {from_state.value} -> {to_state.value}"
        )
    if actor not in actors:
        raise TransitionError(
            f"actor {actor.value} cannot transition "
            f"{from_state.value} -> {to_state.value}; "
            f"allowed={sorted(a.value for a in actors)}"
        )


def happy_path_next(state: State) -> State | None:
    for src, dst in _HAPPY_EDGES:
        if src == state:
            return dst
    if state == State.REALITY_GATE:
        return State.READY_FOR_MERGE_GATE
    if state == State.READY_FOR_MERGE_GATE:
        return State.MERGED
    return None

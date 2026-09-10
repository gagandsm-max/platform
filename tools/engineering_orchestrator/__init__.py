"""Platform Engineering Orchestrator — Phase 1 (deterministic FSM + evidence).

Hash-chain journaling provides tamper detection only — not signatures or
non-repudiation.
"""

__version__ = "0.1.0"
PHASE = 1

from .models import Actor, EvidenceType, FailureClass, RcaStatus, State, Task

__all__ = [
    "Actor",
    "EvidenceType",
    "FailureClass",
    "PHASE",
    "RcaStatus",
    "State",
    "Task",
    "__version__",
]

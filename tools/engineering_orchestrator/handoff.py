"""Structured manual handoffs. Codex handoff default DISABLED.

Phase 1 does not invoke AI, spawn subprocesses, or network-call workers.
"""

from __future__ import annotations

from dataclasses import asdict, dataclass, field
from typing import Any

from .models import HandoffStatus
from .paths import validate_relative_path


class HandoffError(ValueError):
    """Raised for handoff policy violations."""


@dataclass
class Handoff:
    handoff_id: str
    task_id: str
    from_state: str
    expected_return_state: str
    actor_role: str
    instructions_ref: str = ""
    inputs: dict[str, Any] = field(default_factory=dict)
    outputs: dict[str, Any] = field(default_factory=dict)
    # Codex handoff default DISABLED for Phase 1.
    codex_enabled: bool = False
    status: HandoffStatus = HandoffStatus.PENDING

    def to_dict(self) -> dict[str, Any]:
        d = asdict(self)
        d["status"] = self.status.value
        return d

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "Handoff":
        instructions_ref = str(data.get("instructions_ref", "") or "")
        if instructions_ref:
            validate_relative_path(instructions_ref)
        # Strict boolean only — strings like "false" must not become True.
        codex_enabled = _parse_strict_bool(data.get("codex_enabled", False), default=False)
        return cls(
            handoff_id=str(data["handoff_id"]),
            task_id=str(data["task_id"]),
            from_state=str(data["from_state"]),
            expected_return_state=str(data["expected_return_state"]),
            actor_role=str(data.get("actor_role", "")),
            instructions_ref=instructions_ref,
            inputs=dict(data.get("inputs") or {}),
            outputs=dict(data.get("outputs") or {}),
            codex_enabled=codex_enabled,
            status=HandoffStatus(data.get("status", HandoffStatus.PENDING.value)),
        )


def create_handoff(
    *,
    handoff_id: str,
    task_id: str,
    from_state: str,
    expected_return_state: str,
    actor_role: str,
    instructions_ref: str = "",
    inputs: dict[str, Any] | None = None,
    codex_enabled: bool = False,
) -> Handoff:
    """Create a structured handoff. codex_enabled defaults to False."""
    if codex_enabled:
        # Phase 1 allows recording the flag only if explicitly set; execution
        # is still forbidden. Prefer default false in CLI.
        pass
    if instructions_ref:
        validate_relative_path(instructions_ref)
    # Strip obviously sensitive keys from inputs (data safety).
    safe_inputs = redact_sensitive(dict(inputs or {}))
    return Handoff(
        handoff_id=handoff_id,
        task_id=task_id,
        from_state=from_state,
        expected_return_state=expected_return_state,
        actor_role=actor_role,
        instructions_ref=instructions_ref,
        inputs=safe_inputs,
        outputs={},
        codex_enabled=bool(codex_enabled),
        status=HandoffStatus.PENDING,
    )


_SENSITIVE_KEYS = frozenset(
    {
        "password",
        "secret",
        "token",
        "api_key",
        "apikey",
        "private_key",
        "credential",
        "authorization",
    }
)


def _parse_strict_bool(value: Any, *, default: bool = False) -> bool:
    """Parse JSON-safe booleans only. Reject truthy non-bool strings."""
    if value is None:
        return default
    if isinstance(value, bool):
        return value
    if isinstance(value, int) and value in (0, 1) and not isinstance(value, bool):
        # JSON has no distinct int/bool for 0/1 in some paths; treat only exact bool.
        # Reject integers to avoid accidental enablement from "1".
        raise HandoffError("codex_enabled must be a JSON boolean")
    if isinstance(value, str):
        raise HandoffError("codex_enabled must be a JSON boolean, not a string")
    raise HandoffError("codex_enabled must be a JSON boolean")


def redact_sensitive(data: Any) -> Any:
    """Recursively redact sensitive keys in dict/list structures."""
    if isinstance(data, dict):
        redacted: dict[str, Any] = {}
        for key, value in data.items():
            key_l = str(key).lower()
            if key_l in _SENSITIVE_KEYS or any(s in key_l for s in _SENSITIVE_KEYS):
                redacted[key] = "[REDACTED]"
            else:
                redacted[key] = redact_sensitive(value)
        return redacted
    if isinstance(data, list):
        return [redact_sensitive(item) for item in data]
    return data


def assert_codex_default_disabled(handoff: Handoff) -> None:
    if handoff.codex_enabled:
        raise HandoffError(
            "Codex handoff is disabled by default in Phase 1; "
            "execution of AI/subprocess workers is not implemented"
        )

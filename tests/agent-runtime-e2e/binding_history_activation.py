#!/usr/bin/env python3
"""Shared E2E-only helpers for post-commit Agent↔entity binding history activation.

Mirrors production AgentControlRepository.afterCommit scheduling of
platform.activate_entity_agent_binding_history(...). Fixtures that mutate
platform.entity_agent_binding via direct SQL must activate in a *separate*
committed transaction after the write commits — never inside the creating txn.
"""

from __future__ import annotations

# Intentional fixture classes for callers that classify mutations before activation.
COMMITTED_LEGITIMATE = "committed_legitimate"
INTENTIONALLY_UNCOMMITTED = "intentionally_uncommitted"
NEGATIVE_MISSING_OR_AMBIGUOUS = "negative_missing_or_ambiguous"
REJECTED_OR_ROLLED_BACK = "rejected_or_rolled_back"

_ACTIVATABLE = frozenset({COMMITTED_LEGITIMATE})


def may_activate(mutation_class: str) -> bool:
    """Only committed legitimate binding changes may call the activation procedure."""
    return mutation_class in _ACTIVATABLE


def activate_pending_agent_binding_sql(tenant_id: str, agent_id: str) -> str:
    """SQL that activates every PENDING_COMMIT_VISIBILITY row for tenant/agent.

    Must run only after the binding write has already committed. Returns one
    boolean column per pending row (PostgreSQL 't'/'f').
    """
    return (
        "SELECT platform.activate_entity_agent_binding_history("
        "h.history_version, h.tenant_id, h.agent_id, h.entity_id, "
        "h.certificate_identity, h.binding_state, h.reinstall_generation) "
        "FROM platform.entity_agent_binding_history h WHERE h.tenant_id='"
        + tenant_id
        + "' AND h.agent_id='"
        + agent_id
        + "' AND h.activation_state='PENDING_COMMIT_VISIBILITY'"
    )


def seed_post_commit_activation_sql(tenant_id: str, agent_id: str, identifier_id: str) -> str:
    """Post-COMMIT activation for seed_authority: identifier then Agent binding."""
    return f"""
SELECT CASE WHEN platform.activate_entity_identifier_binding_history(
    h.history_version, h.tenant_id, h.entity_identifier_id, h.entity_id,
    h.namespace, h.identifier_type, h.identifier_value)
    THEN 'HISTORY_ACTIVE' ELSE 'HISTORY_UNPROVEN' END
  FROM platform.entity_identifier_binding_history h
 WHERE h.tenant_id = '{tenant_id}' AND h.entity_identifier_id = '{identifier_id}'
   AND h.activation_state = 'PENDING_COMMIT_VISIBILITY';
SELECT CASE WHEN platform.activate_entity_agent_binding_history(
    h.history_version, h.tenant_id, h.agent_id, h.entity_id,
    h.certificate_identity, h.binding_state, h.reinstall_generation)
    THEN 'AGENT_HISTORY_ACTIVE' ELSE 'AGENT_HISTORY_UNPROVEN' END
  FROM platform.entity_agent_binding_history h
 WHERE h.tenant_id = '{tenant_id}' AND h.agent_id = '{agent_id}'
   AND h.activation_state = 'PENDING_COMMIT_VISIBILITY';
"""


def confirm_active_open_agent_binding_sql(tenant_id: str, agent_id: str) -> str:
    """Count open ACTIVE Agent binding history rows (valid_to IS NULL)."""
    return (
        "SELECT count(*) FROM platform.entity_agent_binding_history "
        "WHERE tenant_id='"
        + tenant_id
        + "' AND agent_id='"
        + agent_id
        + "' AND activation_state='ACTIVE' AND valid_from IS NOT NULL "
        "AND valid_to IS NULL AND binding_state='ACTIVE'"
    )


def confirm_pending_agent_binding_sql(tenant_id: str, agent_id: str) -> str:
    """Count PENDING_COMMIT_VISIBILITY Agent binding history rows."""
    return (
        "SELECT count(*) FROM platform.entity_agent_binding_history "
        "WHERE tenant_id='"
        + tenant_id
        + "' AND agent_id='"
        + agent_id
        + "' AND activation_state='PENDING_COMMIT_VISIBILITY'"
    )


def interpret_activation_result(raw: str) -> str:
    """Classify psql -At activation output for fixture assertions."""
    text = (raw or "").strip()
    if not text:
        return "NO_PENDING"
    lines = [line.strip() for line in text.splitlines() if line.strip()]
    if any(line in ("f", "false", "AGENT_HISTORY_UNPROVEN") for line in lines):
        return "UNPROVEN"
    if all(line in ("t", "true", "AGENT_HISTORY_ACTIVE") for line in lines):
        return "ACTIVATED"
    return "UNEXPECTED:" + repr(text)


def seed_activation_ok(output: str) -> bool:
    """True when seed post-commit activation proved both identifier and Agent history."""
    return (
        "HISTORY_ACTIVE" in output
        and "AGENT_HISTORY_ACTIVE" in output
        and "HISTORY_UNPROVEN" not in output
        and "AGENT_HISTORY_UNPROVEN" not in output
    )

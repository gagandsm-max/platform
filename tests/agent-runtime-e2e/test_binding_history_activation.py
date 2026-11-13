#!/usr/bin/env python3
"""Focused unit tests for V022 E2E Agent binding history activation helpers."""

from __future__ import annotations

import unittest

from binding_history_activation import (
    COMMITTED_LEGITIMATE,
    INTENTIONALLY_UNCOMMITTED,
    NEGATIVE_MISSING_OR_AMBIGUOUS,
    REJECTED_OR_ROLLED_BACK,
    activate_pending_agent_binding_sql,
    confirm_active_open_agent_binding_sql,
    confirm_pending_agent_binding_sql,
    interpret_activation_result,
    may_activate,
    seed_activation_ok,
    seed_post_commit_activation_sql,
)


TENANT = "018f0e0a-0000-7000-8000-000000000001"
AGENT = "agent-e2e-01"
IDENTIFIER = "018f0e0a-0000-7000-8000-00000000a101"


class BindingHistoryActivationHelperTest(unittest.TestCase):
    def test_only_committed_legitimate_may_activate(self):
        self.assertTrue(may_activate(COMMITTED_LEGITIMATE))
        for forbidden in (
            INTENTIONALLY_UNCOMMITTED,
            NEGATIVE_MISSING_OR_AMBIGUOUS,
            REJECTED_OR_ROLLED_BACK,
        ):
            with self.subTest(forbidden=forbidden):
                self.assertFalse(may_activate(forbidden))

    def test_activate_sql_targets_pending_only_and_uses_procedure(self):
        sql = activate_pending_agent_binding_sql(TENANT, AGENT)
        self.assertIn("platform.activate_entity_agent_binding_history(", sql)
        self.assertIn("activation_state='PENDING_COMMIT_VISIBILITY'", sql)
        self.assertIn(f"tenant_id='{TENANT}'", sql)
        self.assertIn(f"agent_id='{AGENT}'", sql)
        # Never mutate ACTIVE history rows directly.
        self.assertNotIn("UPDATE platform.entity_agent_binding_history", sql)
        self.assertNotIn("DISABLE TRIGGER", sql)

    def test_confirm_sql_requires_active_open_interval(self):
        sql = confirm_active_open_agent_binding_sql(TENANT, AGENT)
        self.assertIn("activation_state='ACTIVE'", sql)
        self.assertIn("valid_from IS NOT NULL", sql)
        self.assertIn("valid_to IS NULL", sql)
        self.assertIn("binding_state='ACTIVE'", sql)

    def test_pending_confirm_preserves_intentionally_missing_or_pending(self):
        sql = confirm_pending_agent_binding_sql(TENANT, AGENT)
        self.assertIn("PENDING_COMMIT_VISIBILITY", sql)
        self.assertNotIn("activate_entity_agent_binding_history", sql)

    def test_interpret_committed_activation_and_duplicate_no_pending(self):
        self.assertEqual(interpret_activation_result("t"), "ACTIVATED")
        self.assertEqual(interpret_activation_result("t\nt"), "ACTIVATED")
        self.assertEqual(interpret_activation_result(""), "NO_PENDING")
        self.assertEqual(interpret_activation_result("f"), "UNPROVEN")
        self.assertEqual(interpret_activation_result("AGENT_HISTORY_UNPROVEN"), "UNPROVEN")

    def test_seed_post_commit_activates_identifier_then_agent_separately(self):
        sql = seed_post_commit_activation_sql(TENANT, AGENT, IDENTIFIER)
        self.assertIn("activate_entity_identifier_binding_history", sql)
        self.assertIn("activate_entity_agent_binding_history", sql)
        # Seed helper is post-COMMIT only; it must not wrap activation in a txn.
        self.assertNotRegex(sql, r"(?i)\bBEGIN\b")
        self.assertNotRegex(sql, r"(?i)\bCOMMIT\b")

    def test_seed_activation_ok_requires_both_histories(self):
        good = "HISTORY_ACTIVE\nAGENT_HISTORY_ACTIVE\nseeded-agent=agent-e2e-01"
        self.assertTrue(seed_activation_ok(good))
        self.assertFalse(seed_activation_ok("HISTORY_ACTIVE\nseeded-agent=x"))
        self.assertFalse(seed_activation_ok(
            "HISTORY_ACTIVE\nAGENT_HISTORY_UNPROVEN\nseeded-agent=x"))
        self.assertFalse(seed_activation_ok(
            "HISTORY_UNPROVEN\nAGENT_HISTORY_ACTIVE\nseeded-agent=x"))

    def test_spoofed_agent_sql_is_scoped_to_requested_agent_only(self):
        foreign = activate_pending_agent_binding_sql(TENANT, "agent-spoofed-99")
        self.assertIn("agent_id='agent-spoofed-99'", foreign)
        self.assertNotIn(f"agent_id='{AGENT}'", foreign)


if __name__ == "__main__":
    unittest.main()

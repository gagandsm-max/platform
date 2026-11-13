import copy
import json
import math
import unittest
from readback import entity_binding_result, expected_observation, verify, verify_health, float64_within_one_ulp, counter64_exact


class ReadbackTests(unittest.TestCase):
    def test_exact_fields_and_explicit_timestamp_precision(self):
        obj = {"tenantId": "t", "entityId": "e", "metricId": "cpu", "sensorId": "s", "value": 42,
               "sequenceId": "seq", "sourceProtocol": "AGENT_HOST", "agentId": "agent-e2e-01",
               "timestamp": "2026-11-08T08:00:00.123456789Z"}
        expected = expected_observation("key|" + json.dumps(obj), "agent-e2e-01")
        self.assertFalse(expected["timestamp_exact_at_storage_precision"])
        self.assertEqual(expected["stored_fields"]["ts"], "2026-11-08 08:00:00.123")
        verify(expected, json.dumps(expected["stored_fields"]))
        for field in expected["stored_fields"]:
            changed = dict(expected["stored_fields"], **{field: "forged"})
            with self.assertRaises(ValueError): verify(expected, json.dumps(changed))
        with self.assertRaises(ValueError): verify(expected, "")
        with self.assertRaises(ValueError): verify(expected, "\n".join([json.dumps(expected["stored_fields"])] * 2))

    def test_float64_semantics_are_bounded_to_one_ulp(self):
        for value in (0.0, -0.0, 1e-300, -1e-300, 1.0, -1.0, 1e300, -1e300):
            adjacent = math.nextafter(value, math.inf)
            farther = math.nextafter(adjacent, math.inf)
            self.assertTrue(float64_within_one_ulp(value, adjacent))
            self.assertFalse(float64_within_one_ulp(value, farther))
        self.assertTrue(float64_within_one_ulp(-0.0, 0.0))
        for bad in (math.inf, -math.inf, math.nan, True, "1"):
            self.assertFalse(float64_within_one_ulp(bad, bad))

    def test_counter64_never_uses_float_tolerance(self):
        for value in (0, 2**53 + 1, 2**64 - 1):
            self.assertTrue(counter64_exact(value, value))
            self.assertFalse(counter64_exact(value, float(value)))
            self.assertFalse(counter64_exact(value, value - 1))
        self.assertFalse(counter64_exact(True, 1))
        self.assertFalse(counter64_exact(-1, -1))
        self.assertFalse(counter64_exact(2**64, 2**64))

    def test_missing_agent_rejected(self):
        with self.assertRaises(ValueError): expected_observation("{}", "agent-e2e-01")

    def test_entity_binding_checks_canonical_identity_not_just_presence(self):
        row = {"metricId": "probe", "tenantId": "t", "agentId": "a", "entityId": "e"}
        def result(value):
            return entity_binding_result("key|" + json.dumps(value), "probe", "t", "a", "e")["status"]
        self.assertEqual(result(row), "PASS")
        for field in ("tenantId", "agentId", "entityId"):
            self.assertEqual(result(dict(row, **{field: "forged"})), "FAIL")
        self.assertEqual(result({}), "NOT_RUN")

    def test_health_preserves_local_safety_contract_and_checks_body(self):
        ready = {"status": "DEGRADED", "ready": True, "http_semantic": 200}
        health = {"component_state": "DEGRADED", "queue_depth": 2, "delivery_status": "FAILED",
                  "dependency_status": {"sensor_transport": "DOWN"}, "reason_codes": ["SENSOR_TRANSPORT_UNAVAILABLE"]}
        self.assertTrue(verify_health(ready, health, 200, 2, True))
        self.assertFalse(verify_health(ready, health, 503, 2, True))
        self.assertFalse(verify_health(ready, health, 200, 0, True))
        for key, value in (("status", "UP"), ("ready", False), ("http_semantic", 503)):
            self.assertFalse(verify_health(dict(ready, **{key: value}), health, 200, 2, True))
        for key, value in (("component_state", "UP"), ("queue_depth", 0), ("delivery_status", "OK"),
                           ("dependency_status", {"sensor_transport": "UP"}), ("reason_codes", [])):
            changed = copy.deepcopy(health); changed[key] = value
            self.assertFalse(verify_health(ready, changed, 200, 2, True))
        ready["status"] = "UP"
        health.update(component_state="UP", delivery_status="OK", dependency_status={"sensor_transport": "UP"})
        self.assertTrue(verify_health(ready, health, 200, 0, False))
        health["delivery_status"] = "FAILED"
        self.assertFalse(verify_health(ready, health, 200, 0, False))

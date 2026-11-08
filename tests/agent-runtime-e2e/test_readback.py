import json
import unittest
from readback import expected_observation, verify


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

    def test_missing_agent_rejected(self):
        with self.assertRaises(ValueError): expected_observation("{}", "agent-e2e-01")

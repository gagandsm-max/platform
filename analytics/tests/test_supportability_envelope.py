"""BUILD-005 Slice 4 Analytics bounded envelope / redaction contract tests."""

from __future__ import annotations

import importlib.util
import io
import json
import logging
import os
import sys
import unittest
from io import BytesIO
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src" / "main.py"


def load_main():
    os.environ["PLATFORM_VERSION"] = "0.1.0-test"
    os.environ["PLATFORM_SERVICE_INSTANCE_ID"] = "analytics-test-instance"
    spec = importlib.util.spec_from_file_location("analytics_main", SRC)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class AnalyticsEnvelopeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.main = load_main()

    def _capture_log(self, **extra):
        buf = io.StringIO()
        self.main.logger.handlers.clear()
        handler = logging.StreamHandler(buf)
        handler.setFormatter(self.main.JsonFormatter())
        self.main.logger.addHandler(handler)
        self.main.logger.info("probe", extra=extra)
        return json.loads(buf.getvalue().strip().splitlines()[-1])

    def _get(self, path: str):
        handler = self.main.HealthHandler.__new__(self.main.HealthHandler)
        handler.path = path
        body = BytesIO()
        status = {"code": None}

        def send_response(code):
            status["code"] = code

        handler.wfile = body
        handler.send_response = send_response
        handler.send_header = lambda *_: None
        handler.end_headers = lambda: None
        handler.do_GET()
        return status["code"], json.loads(body.getvalue().decode("utf-8"))

    def test_canonical_log_envelope(self):
        obj = self._capture_log(safe_context={"mode": "ok"})
        for key in ("ts", "level", "component", "version", "service_instance_id", "message"):
            self.assertIn(key, obj)
        self.assertEqual(obj["component"], "platform-analytics")
        self.assertNotIn("service", obj)
        self.assertNotIn("timestamp", obj)
        self.assertEqual(obj["safe_context"], {"mode": "ok"})

    def test_safe_context_rejects_secret_keys_and_values(self):
        obj = self._capture_log(
            safe_context={
                "mode": "ok",
                "community": "public",
                "passphrase": "hunter2",
                "pwd": "x",
                "auth_pass": "snmp-auth",
                "priv_key": "opaque-priv",
                "access_key": "AKIA_TEST",
                "token_value": "Bearer abc",
                "Authorization": "Bearer leaked-token",
                "private_material": "-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----",
                "note": "Bearer authentication failed",
            }
        )
        self.assertEqual(
            obj.get("safe_context"),
            {"mode": "ok", "note": "Bearer authentication failed"},
        )

    def test_invalid_correlation_id_omitted(self):
        obj = self._capture_log(correlation_id="bad/id with space")
        self.assertNotIn("correlation_id", obj)

    def test_public_overall_health_routes(self):
        for path in ("/health", "/healthz"):
            code, body = self._get(path)
            self.assertEqual(code, 200)
            self.assertEqual(body["status"], "UP")
            self.assertEqual(body["component"], "platform-analytics")
            self.assertIn("version", body)
            self.assertIn("ts", body)
            self.assertIn("service_instance_id", body)
            self.assertNotIn("document_type", body)
            self.assertNotIn("service", body)

    def test_live_and_ready_envelopes(self):
        live_code, live = self._get("/live")
        self.assertEqual(live_code, 200)
        self.assertEqual(live["status"], "UP")
        ready_code, ready = self._get("/ready")
        self.assertEqual(ready_code, 200)
        self.assertTrue(ready["ready"])
        self.assertEqual(ready["http_semantic"], 200)


if __name__ == "__main__":
    sys.path.insert(0, str(ROOT / "src"))
    unittest.main()

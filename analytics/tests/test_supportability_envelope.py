"""BUILD-005 Slice 4 Analytics bounded envelope / redaction contract tests."""

from __future__ import annotations

import importlib.util
import io
import json
import logging
import math
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
        return buf.getvalue().strip().splitlines()[-1], json.loads(
            buf.getvalue().strip().splitlines()[-1]
        )

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
        payload = body.getvalue()
        parsed = json.loads(payload.decode("utf-8")) if payload else None
        return status["code"], parsed

    def test_canonical_log_envelope(self):
        _, obj = self._capture_log(safe_context={"mode": "ok"})
        for key in ("ts", "level", "component", "version", "service_instance_id", "message"):
            self.assertIn(key, obj)
        self.assertEqual(obj["component"], "platform-analytics")
        self.assertNotIn("service", obj)
        self.assertNotIn("timestamp", obj)
        self.assertEqual(obj["safe_context"], {"mode": "ok"})

    def test_safe_context_rejects_secret_keys_and_values(self):
        raw, obj = self._capture_log(
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
                "private_material": "-----BEGIN PRIVATE KEY-----MIIE-----END PRIVATE KEY-----",
                "note": "Bearer authentication failed",
                "note_basic": "Basic authentication rejected",
                "outcome": "token validation failed",
            }
        )
        self.assertEqual(
            obj.get("safe_context"),
            {
                "mode": "ok",
                "note": "Bearer authentication failed",
                "note_basic": "Basic authentication rejected",
                "outcome": "token validation failed",
            },
        )
        lowered = raw.lower()
        for sentinel in ("hunter2", "leaked-token", "-----begin private key-----"):
            self.assertNotIn(sentinel, lowered)

    def test_safe_context_rejects_embedded_authorization_and_mixed_case_pem(self):
        raw, obj = self._capture_log(
            safe_context={
                "status": "ok",
                "detail": "request failed; Authorization: Bearer abc123-token",
                "upstream": "upstream Authorization: Basic Zm9vOmJhcg==",
                "pem_lower": "-----begin private key----- SECRET-SENTINEL -----end private key-----",
                "prose": "Bearer authentication failed",
            }
        )
        self.assertEqual(
            obj.get("safe_context"),
            {"status": "ok", "prose": "Bearer authentication failed"},
        )
        lowered = raw.lower()
        for sentinel in (
            "abc123-token",
            "zm9vomjhcg==",
            "secret-sentinel",
            "authorization: bearer",
            "authorization: basic",
        ):
            self.assertNotIn(sentinel, lowered)

    def test_safe_context_omits_non_finite_floats(self):
        _, obj = self._capture_log(
            safe_context={
                "ok_int": 7,
                "ok_float": 1.5,
                "bad_nan": math.nan,
                "bad_pos_inf": math.inf,
                "bad_neg_inf": -math.inf,
            }
        )
        self.assertEqual(obj.get("safe_context"), {"ok_float": 1.5, "ok_int": 7})

    def test_invalid_correlation_id_omitted(self):
        _, obj = self._capture_log(correlation_id="bad/id with space")
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

    def test_unauthorized_probe_aliases_absent(self):
        for path in ("/live", "/livez", "/ready", "/readyz"):
            code, body = self._get(path)
            self.assertEqual(code, 404, path)
            self.assertIsNone(body)

    def test_emitted_json_has_no_duplicate_top_level_keys(self):
        raw, obj = self._capture_log(safe_context={"mode": "ok"})

        def no_duplicate_keys(pairs):
            out = {}
            for key, value in pairs:
                if key in out:
                    raise AssertionError(f"duplicate JSON key: {key}")
                out[key] = value
            return out

        parsed = json.loads(raw, object_pairs_hook=no_duplicate_keys)
        self.assertEqual(parsed["component"], "platform-analytics")
        self.assertEqual(obj["component"], "platform-analytics")

    def test_identity_bounds_valid_and_invalid_env(self):
        # Valid bounds
        self.assertEqual(self.main._sanitize_version("0.1.0-test"), "0.1.0-test")
        self.assertEqual(
            self.main._sanitize_service_instance_id("analytics-test-01"),
            "analytics-test-01",
        )
        # Empty / oversize version falls back
        self.assertEqual(self.main._sanitize_version(""), "0.1.0-dev")
        self.assertEqual(self.main._sanitize_version("v" * 65), "0.1.0-dev")
        # Invalid service_instance_id charset / length falls back to analytics-<hex>
        bad_chars = self.main._sanitize_service_instance_id("analytics/bad id")
        self.assertTrue(bad_chars.startswith("analytics-"))
        self.assertNotEqual(bad_chars, "analytics/bad id")
        self.assertRegex(bad_chars, r"^analytics-[0-9a-f]{32}$")
        oversize = self.main._sanitize_service_instance_id("a" * 129)
        self.assertRegex(oversize, r"^analytics-[0-9a-f]{32}$")


if __name__ == "__main__":
    sys.path.insert(0, str(ROOT / "src"))
    unittest.main()

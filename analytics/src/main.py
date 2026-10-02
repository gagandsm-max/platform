import http.server
import json
import logging
import math
import os
import re
import signal
import sys
import threading
import time
import uuid
from datetime import datetime, timezone


COMPONENT = "platform-analytics"
_SERVICE_INSTANCE_RE = re.compile(r"^[A-Za-z0-9._:-]{1,128}$")


def _sanitize_version(raw):
    value = (raw or "").strip()
    if value and len(value) <= 64 and all(ord(c) >= 32 for c in value):
        return value[:64]
    return "0.1.0-dev"


def _sanitize_service_instance_id(raw):
    value = (raw or "").strip()
    if value and _SERVICE_INSTANCE_RE.fullmatch(value):
        return value
    return f"analytics-{uuid.uuid4().hex}"


VERSION = _sanitize_version(os.getenv("PLATFORM_VERSION", "0.1.0-dev"))
SERVICE_INSTANCE_ID = _sanitize_service_instance_id(
    os.getenv("PLATFORM_SERVICE_INSTANCE_ID")
)


def utc_ts() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


class JsonFormatter(logging.Formatter):
    """BUILD-005 Slice 4 canonical Analytics structured-log envelope."""

    ALLOWED = {
        "ts",
        "level",
        "component",
        "version",
        "service_instance_id",
        "message",
        "event_code",
        "correlation_id",
        "request_id",
        "safe_context",
    }

    def format(self, record):
        level = record.levelname.upper()
        if level == "WARNING":
            level = "WARN"
        log_obj = {
            "ts": utc_ts(),
            "level": level,
            "component": COMPONENT,
            "version": VERSION,
            "service_instance_id": SERVICE_INSTANCE_ID,
            "message": (record.getMessage() or "-")[:1024],
        }
        event_code = getattr(record, "event_code", None)
        if isinstance(event_code, str) and event_code.strip():
            cleaned_code = event_code.strip()[:64]
            if all(c.isalnum() or c in "._:-" for c in cleaned_code):
                log_obj["event_code"] = cleaned_code
        correlation_id = getattr(record, "correlation_id", None)
        if isinstance(correlation_id, str) and correlation_id.strip():
            cleaned_corr = correlation_id.strip()[:128]
            if all(c.isalnum() or c in "._:-" for c in cleaned_corr):
                log_obj["correlation_id"] = cleaned_corr
        safe_context = getattr(record, "safe_context", None)
        if isinstance(safe_context, dict) and safe_context:
            cleaned = {}
            for key in sorted(safe_context.keys())[:16]:
                if not isinstance(key, str) or not key or len(key) > 64:
                    continue
                if not key[0].isalpha():
                    continue
                if not all(c.isalnum() or c == "_" for c in key):
                    continue
                if self._secret_key(key):
                    continue
                value = safe_context[key]
                if isinstance(value, bool):
                    cleaned[key] = value
                elif isinstance(value, int):
                    cleaned[key] = value
                elif isinstance(value, float):
                    if math.isfinite(value):
                        cleaned[key] = value
                elif isinstance(value, str) and len(value) <= 256 and value.isprintable():
                    if self._looks_credential_like(value):
                        continue
                    cleaned[key] = value
            if cleaned:
                log_obj["safe_context"] = cleaned
        # Enforce closed top-level shape.
        return json.dumps({k: v for k, v in log_obj.items() if k in self.ALLOWED})

    _BEARER_FULL = re.compile(r"^\s*Bearer\s+[A-Za-z0-9._~+/=-]+\s*$", re.IGNORECASE)
    _BASIC_FULL = re.compile(r"^\s*Basic\s+([A-Za-z0-9+/]+=*)\s*$", re.IGNORECASE)
    _AUTH_BEARER_EMBEDDED = re.compile(
        r"Authorization\s*[:=]\s*Bearer\s+[A-Za-z0-9._~+/=-]+",
        re.IGNORECASE,
    )
    _AUTH_BASIC_EMBEDDED = re.compile(
        r"Authorization\s*[:=]\s*Basic\s+[A-Za-z0-9+/]+=*",
        re.IGNORECASE,
    )

    @staticmethod
    def _normalize_key(key: str) -> str:
        return "".join(c for c in key.lower() if c.isalnum())

    @classmethod
    def _secret_key(cls, key: str) -> bool:
        token = cls._normalize_key(key)
        if token.endswith(
            (
                "authpass",
                "privpass",
                "privacypass",
                "authkey",
                "privkey",
                "privacykey",
                "accesskey",
            )
        ) or token == "bearer":
            return True
        for forbidden in (
            "password",
            "passwd",
            "community",
            "token",
            "secret",
            "privatekey",
            "apikey",
            "clientsecret",
            "authorization",
            "cookie",
            "credential",
            "passphrase",
            "pwd",
        ):
            if forbidden in token:
                return True
        return False

    @classmethod
    def _looks_credential_like(cls, value: str) -> bool:
        # Targeted safe_context sanitization only: full credential blobs,
        # embedded Authorization header credentials, and PEM private-key
        # markers (case-insensitive). Operational prose such as
        # "Bearer authentication failed" remains permitted.
        trimmed = value.strip()
        if cls._BEARER_FULL.fullmatch(trimmed):
            return True
        basic = cls._BASIC_FULL.fullmatch(trimmed)
        if basic is not None and len(basic.group(1)) >= 4:
            return True
        if cls._AUTH_BEARER_EMBEDDED.search(trimmed):
            return True
        if cls._AUTH_BASIC_EMBEDDED.search(trimmed):
            return True
        upper = trimmed.upper()
        return "-----BEGIN" in upper and "PRIVATE KEY" in upper


handler = logging.StreamHandler(sys.stdout)
handler.setFormatter(JsonFormatter())
logger = logging.getLogger("platform.analytics")
logger.setLevel(os.getenv("PLATFORM_LOG_LEVEL", "INFO"))
logger.addHandler(handler)
logger.propagate = False


class HealthHandler(http.server.BaseHTTPRequestHandler):
    shutting_down = False

    def _write_json(self, code: int, body: dict) -> None:
        payload = json.dumps(body).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def do_GET(self):
        if self.path in ("/health", "/healthz"):
            # PUBLIC_OVERALL only — Slice-1 certified Analytics baseline.
            # Do not add /live|/ready aliases without a concrete consumer.
            state = "SHUTTING_DOWN" if self.shutting_down else "UP"
            code = 503 if self.shutting_down else 200
            self._write_json(
                code,
                {
                    "status": state,
                    "component": COMPONENT,
                    "version": VERSION,
                    "ts": utc_ts(),
                    "service_instance_id": SERVICE_INSTANCE_ID,
                },
            )
            return

        self.send_response(404)
        self.end_headers()

    def log_message(self, format, *args):
        # Suppress standard logging to stdout to keep JSON clean
        pass


def start_health_server(port):
    server = http.server.HTTPServer(("0.0.0.0", port), HealthHandler)
    server_thread = threading.Thread(target=server.serve_forever, daemon=True)
    server_thread.start()
    return server


def main():
    logger.info("Platform Analytics engine initializing")
    health_port = int(os.getenv("ANALYTICS_HEALTH_PORT", "8090"))
    server = start_health_server(health_port)
    logger.info(
        "Health server listening",
        extra={"safe_context": {"listen_configured": True, "port": health_port}},
    )

    running = True

    def signal_handler(signum, frame):
        nonlocal running
        HealthHandler.shutting_down = True
        logger.info(
            "Received signal, initiating graceful shutdown",
            extra={
                "event_code": "ANALYTICS_SHUTDOWN_SIGNAL",
                "safe_context": {"signal": int(signum)},
            },
        )
        running = False

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    logger.info("Platform Analytics worker loop started")
    while running:
        try:
            # Synthetic background computation cycle
            time.sleep(5)
        except Exception as e:
            logger.error(
                "Error in analytics worker loop",
                extra={
                    "event_code": "ANALYTICS_WORKER_LOOP_FAILED",
                    "safe_context": {"reason_class": "worker_loop_failed"},
                },
            )
            _ = e

    server.shutdown()
    logger.info("Platform Analytics engine shutdown cleanly")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""BUILD-005 supportability stress harness for Analytics (hostile safe_context / redaction).

Run via tests/stress/run-build005-supportability-stress.sh — not ordinary unit CI.
"""

from __future__ import annotations

import importlib.util
import io
import json
import logging
import math
import os
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src" / "main.py"

SENTINELS = (
    "STRESS-BEARER-SENTINEL",
    "U1RSRVNTLUJBU0lD",
    "STRESS-PEM-SENTINEL",
    "stress-password-sentinel",
)


def load_main():
    os.environ["PLATFORM_VERSION"] = "0.1.0-stress"
    os.environ["PLATFORM_SERVICE_INSTANCE_ID"] = "analytics-stress-01"
    spec = importlib.util.spec_from_file_location("analytics_main_stress", SRC)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def capture(main, **extra):
    buf = io.StringIO()
    main.logger.handlers.clear()
    handler = logging.StreamHandler(buf)
    handler.setFormatter(main.JsonFormatter())
    main.logger.addHandler(handler)
    main.logger.info("stress", extra=extra)
    raw = buf.getvalue().strip().splitlines()[-1]
    return raw, json.loads(raw)


def main() -> int:
    started = time.time()
    mod = load_main()
    cases = 0
    leaks = 0
    malformed = 0
    prose_ok = 0
    nonfinite = 0

    hostile = [
        {"detail": "Bearer STRESS-BEARER-SENTINEL"},
        {"detail": "Basic U1RSRVNTLUJBU0lD"},
        {"detail": "Authorization: Bearer STRESS-BEARER-SENTINEL"},
        {"detail": "Authorization = Bearer STRESS-BEARER-SENTINEL"},
        {"detail": "Authorization: Basic U1RSRVNTLUJBU0lD"},
        {"detail": "Authorization = Basic U1RSRVNTLUJBU0lD"},
        {"detail": "authorization=bearer STRESS-BEARER-SENTINEL"},
        {"detail": "authorization=basic U1RSRVNTLUJBU0lD"},
        {"pem": "-----begin private key-----\nSTRESS-PEM-SENTINEL\n-----end private key-----"},
        {"password": "stress-password-sentinel"},
        {"community": "stress-password-sentinel"},
        {"api_token": "stress-password-sentinel"},
        {"cookie": "stress-password-sentinel"},
        {"passphrase": "stress-password-sentinel"},
        {"auth_key": "stress-password-sentinel"},
        {"privacy_key": "stress-password-sentinel"},
        {"nested": {"x": 1}},
        {"arr": [1, 2, 3]},
        {"nan": math.nan, "pinf": math.inf, "ninf": -math.inf, "ok": 1.5},
        {"ctrl": "bad\x00value"},
    ]
    # pad to >16 keys
    big = {f"k{i}": i for i in range(20)}
    big["mode"] = "ok"
    hostile.append(big)

    for i in range(10000):
        ctx = dict(hostile[i % len(hostile)])
        ctx["status"] = "ok"
        try:
            raw, obj = capture(mod, safe_context=ctx)
        except Exception:
            malformed += 1
            continue
        cases += 1
        lowered = raw.lower()
        for s in SENTINELS:
            if s.lower() in lowered:
                leaks += 1
        if "nan" in lowered or "infinity" in lowered:
            nonfinite += 1
        sc = obj.get("safe_context") or {}
        if isinstance(sc, dict) and len(sc) > 16:
            malformed += 1

    raw, obj = capture(
        mod,
        safe_context={
            "a": "Bearer authentication failed",
            "b": "Basic authentication rejected",
            "c": "token validation failed",
            "d": "Authorization validation failed",
            "e": "authentication scheme Bearer rejected",
        },
    )
    sc = obj.get("safe_context") or {}
    if all(k in sc for k in ("a", "b", "c", "d", "e")):
        prose_ok = 1

    duration = time.time() - started
    print(
        f"ANALYTICS_STRESS cases={cases} leaks={leaks} malformed={malformed} "
        f"nonfinite={nonfinite} prose_ok={prose_ok} duration_s={duration:.2f}"
    )
    if leaks or malformed or nonfinite or not prose_ok:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

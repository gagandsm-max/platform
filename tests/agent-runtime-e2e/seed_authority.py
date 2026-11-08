#!/usr/bin/env python3
"""Seed Core Agent authority rows for the test-only E2E path. No product API exists yet."""

from __future__ import annotations

import json
import os
import subprocess
import sys
from datetime import datetime, timezone, timedelta
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
META = Path(os.environ["PLATFORM_E2E_CERT_DIR"]) / "metadata.json"
PREFIX = os.environ["PLATFORM_E2E_PREFIX"]

TENANT = "018f0e0a-0000-7000-8000-000000000001"
AGENT_ID = "agent-e2e-01"
SENSOR_ID = os.environ.get("SENSOR_INFRA_ID", "sensor-infra-01")
HOST_ID = "host-e2e-agent-01"
ENTITY_ID = "018f0e0a-0000-7000-8000-00000000ae01"
CREDENTIAL_ID = "018f0e0a-0000-7000-8000-00000000ac01"
ASSIGNMENT_ID = "018f0e0a-0000-7000-8000-00000000aa01"
IDENTIFIER_ID = "018f0e0a-0000-7000-8000-00000000a101"

if sys.argv[1:] == ["--probe"]:
    META = META.with_name("probe-metadata.json")
    AGENT_ID = "agent-e2e-probe-01"
    HOST_ID = "host-e2e-probe-01"
    ENTITY_ID = ENTITY_ID[:-1] + "2"
    CREDENTIAL_ID = CREDENTIAL_ID[:-1] + "2"
    ASSIGNMENT_ID = ASSIGNMENT_ID[:-1] + "2"
    IDENTIFIER_ID = IDENTIFIER_ID[:-1] + "2"
elif sys.argv[1:]:
    raise SystemExit("Only --probe is supported")


def psql(sql: str) -> str:
    env = os.environ
    user = env.get("POSTGRES_USER", "platform_app")
    db = env.get("POSTGRES_DB", "platform")
    cmd = [
        "docker",
        "exec",
        "-i",
        PREFIX + "-postgres",
        "psql",
        "-U",
        user,
        "-d",
        db,
        "-v",
        "ON_ERROR_STOP=1",
        "-t",
        "-A",
    ]
    return subprocess.check_output(cmd, input=sql, text=True)


def main() -> int:
    meta = json.loads(META.read_text())
    fp = meta["fingerprint_sha256"]
    issuer_fp = meta["issuer_fingerprint_sha256"]
    serial = meta["serial_number"]
    issuer_dn = meta["issuer_dn"].replace("'", "''")
    now = datetime.now(timezone.utc)
    not_before = (now - timedelta(days=1)).strftime("%Y-%m-%dT%H:%M:%SZ")
    not_after = (now + timedelta(days=365)).strftime("%Y-%m-%dT%H:%M:%SZ")

    sensor = psql(
        f"SELECT count(*) FROM platform.sensor WHERE tenant_id = '{TENANT}' AND sensor_id = '{SENSOR_ID}';"
    ).strip()
    if sensor != "1":
        print(f"[ERROR] sensor {SENSOR_ID} not enrolled for tenant {TENANT}: count={sensor}", file=sys.stderr)
        return 1

    sql = f"""
BEGIN;
DELETE FROM platform.agent_sensor_assignment
 WHERE tenant_id = '{TENANT}' AND agent_id = '{AGENT_ID}';
DELETE FROM platform.agent_certificate_credential
 WHERE tenant_id = '{TENANT}'
   AND (agent_id = '{AGENT_ID}' OR credential_id = '{CREDENTIAL_ID}' OR fingerprint_sha256 = '{fp}');
DELETE FROM platform.entity_identifier
 WHERE tenant_id = '{TENANT}'
   AND (
        identifier_id = '{IDENTIFIER_ID}'
        OR (namespace = 'platform.agent' AND identifier_type = 'HOST_ID' AND identifier_value = '{HOST_ID}')
   );
DELETE FROM platform.entity WHERE entity_id = '{ENTITY_ID}';
DELETE FROM platform.agent WHERE agent_id = '{AGENT_ID}';

INSERT INTO platform.agent (agent_id, tenant_id, site_id, status, created_at, updated_at)
VALUES ('{AGENT_ID}', '{TENANT}', NULL, 'ACTIVE', now(), now());

INSERT INTO platform.agent_certificate_credential (
    credential_id, tenant_id, agent_id, fingerprint_sha256, spki_sha256,
    serial_number, issuer_dn, issuer_fingerprint_sha256,
    not_before, not_after, status, activated_at, revoked_at, created_at, updated_at
) VALUES (
    '{CREDENTIAL_ID}', '{TENANT}', '{AGENT_ID}', '{fp}', NULL,
    '{serial}', '{issuer_dn}', '{issuer_fp}',
    '{not_before}', '{not_after}', 'ACTIVE', now(), NULL, now(), now()
);

INSERT INTO platform.agent_sensor_assignment (
    assignment_id, tenant_id, agent_id, sensor_id, generation, status, created_at, updated_at, revoked_at
) VALUES (
    '{ASSIGNMENT_ID}', '{TENANT}', '{AGENT_ID}', '{SENSOR_ID}', 1, 'ACTIVE', now(), now(), NULL
);

INSERT INTO platform.entity (
    entity_id, tenant_id, entity_type, display_name, lifecycle_state,
    first_seen_at, last_seen_at, created_at, updated_at
) VALUES (
    '{ENTITY_ID}', '{TENANT}', 'HOST', 'agent-e2e-01', 'ACTIVE',
    now(), now(), now(), now()
);

INSERT INTO platform.entity_identifier (
    identifier_id, tenant_id, entity_id, identifier_type, namespace, identifier_value,
    confidence, first_seen_at, last_seen_at
) VALUES (
    '{IDENTIFIER_ID}', '{TENANT}', '{ENTITY_ID}', 'HOST_ID', 'platform.agent', '{HOST_ID}',
    1.0, now(), now()
);
COMMIT;
SELECT 'seeded-agent=' || '{AGENT_ID}' || ' fingerprint=' || '{fp}';
"""
    out = psql(sql)
    print(out.strip())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

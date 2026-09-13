#!/usr/bin/env python3
"""Build Agent→Sensor delivery envelopes with checksums matching agent/common/queue."""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
import uuid
from datetime import datetime, timezone


def record_checksum(sequence: int, record_id: str, created_at: str, payload: str) -> str:
    h = hashlib.sha256()
    h.update(f"{sequence}\n{record_id}\n{created_at}\n".encode("utf-8"))
    h.update(payload.encode("utf-8"))
    return h.hexdigest()


def batch_id(agent_id: str, first: int, last: int, records_json_checksums: list[str]) -> str:
    h = hashlib.sha256()
    for item in records_json_checksums:
        h.update(f"{item}\n".encode("utf-8"))
    # Agent BuildBatchID hashes sequence, id, checksum — keep a stable test batch id instead.
    digest = h.hexdigest()[:16]
    return f"batch-{agent_id}-{first:020d}-{last:020d}-{digest}"


def build_payload(
    *,
    tenant_id: str,
    agent_id: str,
    host_id: str,
    metric_key: str,
    value: float,
    observation_id: str,
) -> str:
    now = "2026-09-12T19:00:00Z"
    batch = {
        "schemaVersion": 1,
        "sourceType": "AGENT",
        "tenantId": tenant_id,
        "agentId": agent_id,
        "hostId": host_id,
        "hostname": "agent-e2e",
        "os": "linux",
        "arch": "amd64",
        "collectedAt": now,
        "durationMs": 1,
        "observations": [
            {
                "schemaVersion": 1,
                "observationId": observation_id,
                "tenantId": tenant_id,
                "agentId": agent_id,
                "hostId": host_id,
                "observedAt": now,
                "metricKey": metric_key,
                "value": value,
                "unit": "count",
                "source": "AGENT_HOST",
                "collector": "e2e",
            }
        ],
    }
    return json.dumps(batch, separators=(",", ":"), ensure_ascii=True)


def build_envelope(
    *,
    envelope_tenant: str,
    payload_tenant: str,
    agent_id: str,
    host_id: str,
    sequence: int,
    metric_key: str,
    value: float,
    batch_id_value: str | None,
    correlation_id: str,
) -> str:
    created_at = "2026-09-12T19:00:00Z"
    record_id = f"rec-{sequence}"
    observation_id = f"obs-e2e-{sequence}"
    payload = build_payload(
        tenant_id=payload_tenant,
        agent_id=agent_id,
        host_id=host_id,
        metric_key=metric_key,
        value=value,
        observation_id=observation_id,
    )
    checksum = record_checksum(sequence, record_id, created_at, payload)
    if not batch_id_value:
        batch_id_value = f"batch-{agent_id}-{sequence:020d}-{sequence:020d}-{checksum[:16]}"
    record = (
        f'{{"sequence":{sequence},"id":"{record_id}","createdAt":"{created_at}",'
        f'"checksum":"{checksum}","payload":{payload}}}'
    )
    return (
        f'{{"schemaVersion":1,"agentId":"{agent_id}","tenantId":"{envelope_tenant}",'
        f'"correlationId":"{correlation_id}","batchId":"{batch_id_value}",'
        f'"firstSequence":{sequence},"lastSequence":{sequence},"recordCount":1,'
        f'"records":[{record}]}}'
    )


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--envelope-tenant", required=True)
    p.add_argument("--payload-tenant", required=True)
    p.add_argument("--agent-id", required=True)
    p.add_argument("--host-id", required=True)
    p.add_argument("--sequence", type=int, required=True)
    p.add_argument("--metric-key", required=True)
    p.add_argument("--value", type=float, default=1.0)
    p.add_argument("--batch-id", default="")
    p.add_argument("--correlation-id", default="")
    args = p.parse_args()
    corr = args.correlation_id or f"corr-e2e-{uuid.uuid4().hex}"
    body = build_envelope(
        envelope_tenant=args.envelope_tenant,
        payload_tenant=args.payload_tenant,
        agent_id=args.agent_id,
        host_id=args.host_id,
        sequence=args.sequence,
        metric_key=args.metric_key,
        value=args.value,
        batch_id_value=args.batch_id or None,
        correlation_id=corr,
    )
    sys.stdout.write(body)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

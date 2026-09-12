# Agent runtime E2E (test-only)

Bounded overlay for Issue #46 / PR #47. It does **not** change the production
`compose.yaml` default `SENSOR_AGENT_RECEIVER_ENABLED=false`.

Paths in `compose.override.yaml` are relative to the repository root (Compose
project directory), not this folder.

```bash
make certs
make test-agent-runtime-e2e
```

Path proven:

Agent process → local durable queue → mTLS `POST /api/v1/agent/telemetry` →
Sensor Agent Receiver (infrastructure Sensor) → dedicated Agent spool →
drain `POST /api/v1/sensor/agent-telemetry` → Core → Kafka `telemetry.numeric.v1`.

Agent enrollment uses test-only SQL against the existing Agent Control tables
because no public enroll/assign HTTP API exists on this branch.

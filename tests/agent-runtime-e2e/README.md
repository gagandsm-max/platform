# Agent runtime E2E (test-only)

Bounded overlay for Issue #46 / PR #47. It does **not** change the production
`compose.yaml` default `SENSOR_AGENT_RECEIVER_ENABLED=false`.

Paths in `compose.override.yaml` are relative to the repository root (Compose
project directory), not this folder.

```bash
make certs
make test-agent-runtime-e2e
```

`make test-agent-runtime-e2e` does **not** regenerate `deploy/docker/certs`
(that would rotate the Root CA while Core still holds the previous keystore in
memory). If CA files are missing, `run.sh` calls `make certs` once. Do not run
`make certs` against a live stack.

Each run deletes only the test-only named volume `platform_agent_e2e_state`.
The Sensor Agent spool volume is retained so Core agent-ingress sequences do
not rewind (`ERR_SEQUENCE_REGRESSION`). Oversized leftover spool records are
parked as terminal poison on HTTP 413.

Kafka assertions consume **all partitions** from a saved end-offset snapshot.
`telemetry.numeric.v1` is partitioned; a single `--from-beginning` consumer
with `--timeout-ms` can miss later Agent records.

Agent `PLATFORM_AGENT_MAX_BATCH_BYTES` defaults to 384 KiB so one delivery
envelope stays under Core `AgentIngestionValidator.MAX_RECORD_BYTES` (512 KiB).
The overlay also sets that cap explicitly. Sensor Agent Receiver default max
body is 512 KiB. Core 413 `ERR_RECORD_TOO_LARGE` is a terminal drain poison
(not an infinite retry).

```bash
# only if ca.crt is missing, and only while Compose is down:
# make certs
make test-agent-runtime-e2e
```

Path proven:

Agent process → local durable queue → mTLS `POST /api/v1/agent/telemetry` →
Sensor Agent Receiver (infrastructure Sensor) → dedicated Agent spool →
drain `POST /api/v1/sensor/agent-telemetry` → Core → Kafka `telemetry.numeric.v1`.

Agent enrollment uses test-only SQL against the existing Agent Control tables
because no public enroll/assign HTTP API exists on this branch.

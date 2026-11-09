# Agent runtime E2E (test-only)

## Isolated optional runtime gate

Run only while holding the admitted Issue #211 runtime allocation. The harness is
optional and does not confer BUILD/module/native certification.

```bash
PLATFORM_E2E_RUN_DIR=/absolute/durable/evidence/new-run make test-agent-runtime-e2e
```

The directory must be new. `run-isolated.sh` renders the existing Compose contract
into a fresh `agent-e2e-*` project: unique container/image/volume/network names,
ephemeral loopback receiver port, and private test PKI. It verifies isolation
before any mutation and uses the existing guarded Kafka fresh-volume procedure.
`run.sh` rejects invocation without the verified isolated configuration.

The run builds current Core, Sensor and Agent sources, then exercises the existing
scalar path. Enrollment is explicitly **SQL-seeded test authority**, not proof of
a production Agent enrollment API. Process/service/inventory/eBPF typed evidence
is not carried by this scalar harness. The canonical ClickHouse readback checks
stored identity, time, value and replay count; Agent observation provenance is
currently Kafka-only and must not be claimed as persisted in ClickHouse.

No old shared Agent volume, Sensor acceptance receipt or shared PKI is reset.
Cleanup captures logs and removes only this fresh project and its provenance-
checked Kafka volume. Interrupted cleanup is a HOLD requiring resource inspection.
Native installation, systemd, physical reboot and power-loss are separate gates.

Kafka assertions consume **all partitions** from a saved end-offset snapshot.
`telemetry.numeric.v1` is partitioned; a single `--from-beginning` consumer
with `--timeout-ms` can miss later Agent records.

Agent `PLATFORM_AGENT_MAX_BATCH_BYTES` defaults to 384 KiB so one delivery
envelope stays under Core `AgentIngestionValidator.MAX_RECORD_BYTES` (512 KiB).
The overlay also sets that cap explicitly. Operator overrides above 384 KiB are
rejected at Agent config load (`CONFIG_MAX_BATCH_BYTES_OUT_OF_BOUNDS`) so the
process cannot start a deterministic HTTP 413 retry loop. Sensor Agent Receiver
default max body is 512 KiB. Agent HTTP 413 `ERR_REQUEST_TOO_LARGE` is classified
`OVERSIZED` (queue retained, no hot retry). Core 413 `ERR_RECORD_TOO_LARGE`
retains the frozen Sensor spool head and reports degraded delivery until an
exact durable Core disposition permits removal.

Post-drain duplicate replay uses the durable batch-index acceptance receipt. The
pending Agent spool record may be gone after downstream drain; an identical
retry must still return the original `accepted_through_sequence`. Mutated reuse
of the same sequence range returns `ERR_REPLAY_CONFLICT`.

Path exercised when the run passes (inspect exact run receipts):

Agent process → local durable queue → mTLS `POST /api/v1/agent/telemetry` →
Sensor Agent Receiver (infrastructure Sensor) → dedicated Agent spool →
drain `POST /api/v1/sensor/agent-telemetry` → Core → Kafka `telemetry.numeric.v1`.

Agent enrollment uses test-only SQL against the existing Agent Control tables
because no public enroll/assign HTTP API exists on this branch.

## Canonical authority and numeric semantics

V019 fixtures explicitly bind A (`agent-e2e-01`, entity suffix `ae01`,
`host-e2e-agent-01`) and B (`agent-e2e-probe-01`, entity suffix `ae02`,
`host-e2e-probe-01`) to separate certificate fingerprints. Legitimate scalar
readback verifies each canonical entity. `identity_probe.py` authenticates both
spoof directions at the Sensor, then verifies each forged delivery has an exact
durable Core terminal disposition and advanced settled continuity. The harness
also checks Kafka and ClickHouse absence for both metrics. Sensor local-custody
ACK remains distinct from Core settlement.

The Master ruling permits at most one ULP for ordinary finite Float64 operational
values. `readback.py` uses an explicit IEEE-754 ordered-bit distance, never a loose
absolute/relative epsilon. Counter64 uses exact unsigned integer comparison and
rejects float coercion. The scalar readback does not establish a raw-Counter64
physical path. Timestamp truncation to existing DateTime64(3) is recorded
separately and never described as nanosecond preservation.

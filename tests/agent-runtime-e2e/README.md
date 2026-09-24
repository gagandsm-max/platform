# Agent runtime E2E (test-only)

## BUILD-001 classification — OPTIONAL NON-CI / ISOLATION-PENDING

This harness is **not** part of BUILD-001 certification.

- It uses the shared Compose project/resource namespace `platform`.
- It uses fixed host/resource assumptions (default receiver host port `28444`, named volumes/containers).
- It can `docker pause` / restart `platform-core` and mutate Sensor Agent spool state.
- It **MUST NOT** be run concurrently with a developer Platform stack or normal Platform CI.
- Isolation/hardening is deferred; do not treat green local runs as a canonical CI gate.
- Shared-host CI/container contention remains Issue **#107** (do not duplicate here).

BUILD-001 certifies source-only transport harness capability (including the production
`SpoolAcceptanceSink` → `agentdrain` → mTLS Mock Core pipeline slice under `-tags=pipeline`).
This Compose overlay remains an optional, isolation-pending real-path tool only.

---

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
not rewind (`ERR_SEQUENCE_REGRESSION`). Drained `batch-index` receipts from a
previous run are removed at start because the Agent queue reset restarts source
sequences at 1; that wipe is test-only. Mid-run Sensor restart keeps receipts so
lost-ACK replay after drain remains valid. Oversized leftover spool records are
parked as terminal poison on HTTP 413.

Kafka assertions consume **all partitions** from a saved end-offset snapshot.
`telemetry.numeric.v1` is partitioned; a single `--from-beginning` consumer
with `--timeout-ms` can miss later Agent records.

Agent `PLATFORM_AGENT_MAX_BATCH_BYTES` defaults to 384 KiB so one delivery
envelope stays under Core `AgentIngestionValidator.MAX_RECORD_BYTES` (512 KiB).
The overlay also sets that cap explicitly. Operator overrides above 384 KiB are
rejected at Agent config load (`CONFIG_MAX_BATCH_BYTES_OUT_OF_BOUNDS`) so the
process cannot start a deterministic HTTP 413 retry loop. Sensor Agent Receiver
default max body is 512 KiB. Agent HTTP 413 `ERR_REQUEST_TOO_LARGE` is classified
`OVERSIZED` (queue retained, no hot retry). Core 413 `ERR_RECORD_TOO_LARGE` is a
terminal drain poison (not an infinite retry).

Post-drain duplicate replay uses the durable batch-index acceptance receipt. The
pending Agent spool record may be gone after downstream drain; an identical
retry must still return the original `accepted_through_sequence`. Mutated reuse
of the same sequence range returns `ERR_REPLAY_CONFLICT`.

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

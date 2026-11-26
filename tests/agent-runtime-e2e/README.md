# Agent runtime E2E (test-only)

## Isolated optional runtime gate

Run only while holding the admitted Issue #211 runtime allocation. The harness is
optional and does not confer BUILD/module/native certification.

```bash
PLATFORM_E2E_RUN_DIR=/absolute/durable/evidence/new-run bash tests/agent-runtime-e2e/run-isolated.sh
```

Use the isolated entry point directly. In this checkout, the Make target calls
the inner workload script and does not establish custody. Choose a new canonical
evidence directory outside the checkout. The runner uses the local Unix Docker
endpoint and a private CLI configuration. Compose 5.5.0 must be available to
that configuration through a system plugin location.

The runner checks the rendered Compose model against the fixture's service,
network, named-volume, image, build-context and read-only bind allowlists. Only
the Sensor receiver publishes an ephemeral loopback port. Source mounts are
limited to the specific fixture and product paths. Private PKI is generated in
the evidence directory. The dedicated keytool helper can write only to that PKI
mount. Kafka keeps its guarded fresh-volume provisioning and storage checks.

The PostgreSQL bootstrap bind uses `postgres-init.sql` inside the mode
`0700` evidence directory. The runner copies the public checkout SQL as-is,
sets the copy to mode `0644`, and verifies the exact read-only bind, file mode
and byte equality before workload operations. This gives the container's
PostgreSQL user read access even when the checkout uses a private umask.

A successful inventory of containers, volumes, networks and image tags must
show a collision-free project before the runner creates its private reservation.
Subsequent mutations require the same reservation ID and exact project and
ownership evidence. Docker helpers and one-off containers carry the run owner
label. The runner retains run-owned logs in `stack.log` and container inspect
receipts under `container-state/`.

The PATH shim checks the Docker command forms used by the pinned fixture.
Container reads use verified full IDs, volume reads use verified exact names,
and lists emit only owned entries. Health polling returns a retryable status
while the expected service is absent. The shim assumes trusted local fixture
code with access to the same host environment.

The run builds current Core, Sensor and Agent sources, then exercises the existing
scalar path. Enrollment is explicitly **SQL-seeded test authority**, not proof of
a production Agent enrollment API. Process/service/inventory/eBPF typed evidence
is not carried by this scalar harness. The canonical ClickHouse readback checks
stored identity, time, value and replay count; Agent observation provenance is
currently Kafka-only and must not be claimed as persisted in ClickHouse.

Work has one aggregate deadline including preparation, builds and fixture execution
(`PLATFORM_E2E_WORK_SECONDS`, default 3600, maximum 7200). Teardown has its own
aggregate deadline (`PLATFORM_E2E_CLEANUP_SECONDS`, default 180, maximum 600).
TERM and INT stop the workload process group before teardown. The recorded
workload exit is 124 for a work timeout, 143 for TERM or 130 for INT. See
`exits.txt` for both workload and teardown results. A workload failure takes
exit-status precedence. A teardown failure makes a successful workload exit
nonzero.

Teardown checks exact ownership before removal, uses object IDs where Docker
exposes them, and verifies absence with fresh successful inventories. Docker
addresses volumes by name, so the runner re-inspects their provenance immediately
before removal. This assumes an admitted local daemon where other actors do not
concurrently replace the private run's resources or edit its source and evidence
files. Shared base images and builder cache follow daemon policy.

A failed or interrupted Docker client can leave a daemon operation in flight.
The runner atomically records a private pending receipt before submission. It
contains the admitted operation category, verb and applicable fixture service
names. A positive client exit status is recorded as `returned-nonzero` with its
exit code. A signal result is `uncertain`. A receipt still marked `submitted`
means no outcome was recorded. All three states and legacy receipts require
completion evidence for clearance. The invalid-config helper is created from
the owned Agent image by ID and overrides its inherited Compose service label
with `agent-runtime-invalid-config`. The runner checks the helper's exact
name, returned ID, ownership, image, mounts and network before start. It
verifies application exit 1 on that ID, removes the container, and confirms
its absence before clearing the receipt.
Unresolved receipts retain the reservation and make teardown fail even when
owned-resource cleanup succeeds. Retain the evidence directory and use
`cleanup.log`, the pending and custody receipts, and a fresh inventory for
operator reconciliation under the admitted runtime allocation. An empty
inventory alone cannot resolve an in-flight request.
Native installation, systemd, physical reboot and power-loss are separate gates.

The isolation and custody tests can run without contacting a Docker daemon.

```bash
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests/agent-runtime-e2e
```

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

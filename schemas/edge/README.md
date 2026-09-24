# Platform Edge Transport Contracts (BUILD-000)

**Scope:** Agent↔Sensor and Sensor↔Core JSON transport contracts, sequence bounds, ACK durability, heartbeat v1, standard errors.
**Mother Reference:** Platform v3.2.2.
**Issue:** #110.
**Canonical base SHA:** `315a20980e89683666a64362f0ed3827bf613d4a`.

`SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED`.

## Maturity labels (mandatory)

| Label | Meaning |
|---|---|
| `CURRENT_IMPLEMENTED_V1` | Matches current runtime wire behavior at the BUILD-000 base SHA |
| `FROZEN_BUILD_000_INVARIANT` | Approved BUILD-000 decision; must not silently diverge |
| `BUILD_002_TARGET` | Required for Agent↔Sensor hardening; not implemented in BUILD-000 |
| `BUILD_003_TARGET` | Required for Sensor↔Core hardening; not implemented in BUILD-000 |
| `BUILD_004_TARGET` | Desired/observed control reconciliation; not heartbeat v1 authority |

## Directory layout

```text
schemas/edge/
  README.md
  common-definitions.schema.json
  agent-telemetry-batch.schema.json
  agent-telemetry-ack.schema.json
  sensor-telemetry-batch.schema.json
  sensor-telemetry-ack.schema.json
  sensor-agent-ingress-batch.schema.json
  sensor-agent-ingress-ack.schema.json
  sensor-heartbeat-request.schema.json
  sensor-heartbeat-response.schema.json
  standard-error.schema.json
  tools/validate.mjs
  examples/
  fixtures/cases.json
```

JSON Schema Draft 2020-12 documents structure. `tools/validate.mjs` performs dependency-free semantic validation including `Number.isSafeInteger` for sequence/control counters. No npm packages. No package.json.

## MAX_SAFE_SEQUENCE (B000-D001)

`MAX_SAFE_SEQUENCE = 9007199254740991` (`2^53 - 1`).

Applies to JSON-carried durable delivery/replay sequences and JSON-carried control counters/generations/versions.

- Minimum sequence on wire: **1**
- Zero is prohibited for record/first/last/ingress sequences
- Zero may exist only internally as “nothing acknowledged yet”
- Sequences MUST NOT wrap; exhaustion fails closed; fresh lineage requires explicit new stream/generation

**Counter64 metric values are a different domain (B000-D002)** and MUST NOT use `MAX_SAFE_SEQUENCE`.

## Exact v1 field naming (B000-D003)

Do not rename for consistency. Dual casing is frozen:

| Surface | ACK fields |
|---|---|
| Agent→Sensor | `status`, `accepted_through_sequence`, `agent_id`, `batch_id`, `correlation_id` |
| Sensor→Core telemetry | `status`, `accepted_through_sequence`, `sensor_identity` |
| Sensor→Core agent-ingress | `status`, `sensorId`, `batchId`, `acceptedThroughIngressSequence`, `correlationId` |

## ACK / durability (B000-D004 / D007)

| Boundary | ACK means | Delete/advance requires |
|---|---|---|
| Agent→Sensor | Authz + contract checks + durable Sensor Agent spool accept + durable replay accounting | Validated batch-bound application ACK (HTTP 2xx alone insufficient) |
| Sensor→Core telemetry | PG pending reserve → Kafka txn commit → PG finalize watermark → ACK | Validated `accepted_through_sequence` |
| Sensor→Core agent-ingress | Kafka publish + PG ingress watermark (**at-least-once**; no V009 reservation yet) | Validated `acceptedThroughIngressSequence` |

ClickHouse completion is asynchronous and **MUST NOT** gate Sensor ACK.

## bootEpoch (B000-D005)

Wire field remains `bootEpoch` for v1 compatibility.

Canonical meaning: durable Sensor spool **stream/generation** identity matching `^stream-[0-9a-f]{32}$`.

It survives Sensor process restart, changes when durable spool lineage is recreated, and is **not** OS boot time. The name is historical/misleading.

## Heartbeat v1 (B000-D008)

Core heartbeat response authority:

`status`, `authenticated`, `sensor_identity`, `tenant_id`, `received_at`

`desiredConfigVersion` / `assignmentVersion` / `policyVersionWatermark` are **not** authoritative Core desired/applied-state values in heartbeat v1. Sensor request fields of that form are non-authoritative compatibility fields. **BUILD_004_TARGET** owns desired/received/applied/rejected/stale/fenced reconciliation.

## Entity authority (B000-D009)

Agent `hostId` is source evidence. Agent-supplied `entityId` is not authoritative. Canonical tenant-scoped entity mapping remains Core-owned.

## CURRENT V1 LIMITATION — Agent stream generation (B000-D015)

Sensor durable Agent replay authority is keyed by `tenantId + agentId`.

If the same Agent identity loses/recreates its durable queue and sequences restart, current Sensor correctly fails closed because it cannot distinguish legitimate fresh lineage from replay.

**BUILD_002_TARGET:** introduce durable Agent stream/generation identifier (working name `agentStreamId`) on a versioned Agent→Sensor contract:

- durable across Agent process restart;
- changes only when durable delivery lineage is intentionally recreated;
- included in Sensor replay identity;
- no automatic watermark reset for same `agentId`;
- migration from v1 must fail safely;
- old v1 replay state must not be silently reinterpreted as a new lineage.

BUILD-000 does **not** implement this migration.

## Agent-ingress hardening ownership (B000-D013)

Current Sensor→Core agent-ingress remains **at-least-once**.

**BUILD_003_TARGET:** pending replay reservation, conflicting retry protection, first-batch sequence conformance (`start at 1`), retry amplification hardening.

## Monitoring correctness (B000-D012)

Collection / transport / credential / Agent / Sensor / Core / queue / ingestion failure, no telemetry, and stale telemetry MUST NOT automatically assert monitored target DOWN.

`No Data != Device Down`.

## Semantic error classes (B000-D010)

Wire remains `{status, code, message}`. Detailed `ERR_*` codes remain valid.

Semantic classes:

`INVALID_CONTRACT`, `AUTHENTICATION`, `AUTHORIZATION`, `STALE_OR_FENCED`, `UNSUPPORTED_VERSION`, `INTEGRITY`, `REPLAY_OR_DUPLICATE`, `CAPACITY_OR_RATE_LIMIT`, `TIMEOUT_OR_DEADLINE`, `CANCELLED`, `TRANSPORT`, `SERVER_UNAVAILABLE`

Deterministic mapping examples:

| Detailed code prefix / example | Class |
|---|---|
| `ERR_MALFORMED_JSON`, `ERR_INVALID_FIELD`, `ERR_EMPTY_*`, `ERR_BATCH_*` | INVALID_CONTRACT |
| `ERR_AUTH_*`, cert required/invalid | AUTHENTICATION |
| `ERR_AUTH_IDENTITY_MISMATCH`, tenant/entity scope deny | AUTHORIZATION |
| assignment/generation/pack stale/fence codes | STALE_OR_FENCED |
| `ERR_UNSUPPORTED_SCHEMA_VERSION` | UNSUPPORTED_VERSION |
| checksum mismatches | INTEGRITY |
| `ERR_REPLAY_*`, `ERR_IDEMPOTENCY_*`, `ERR_SEQUENCE_*` | REPLAY_OR_DUPLICATE |
| busy / too large / rate | CAPACITY_OR_RATE_LIMIT |
| deadline/timeout local codes | TIMEOUT_OR_DEADLINE |
| cancelled local codes | CANCELLED |
| `DELIVERY_*` transport, `CORECLIENT_*` | TRANSPORT |
| `ERR_KAFKA_*`, authority unavailable | SERVER_UNAVAILABLE |

## Time (B000-D017)

Cross-boundary evidence timestamps: UTC RFC3339/ISO-8601 with `Z`. Go `time.Time` UTC; Java `Instant`; PostgreSQL `TIMESTAMPTZ`. Monotonic time is internal-only for retry/backoff. No hard clock-skew maximum is frozen in BUILD-000.

## Filename width (B000-D014)

No queue/spool filename padding migration. `MAX_SAFE_SEQUENCE` fits existing Agent 19-digit and Sensor 20-digit representations.

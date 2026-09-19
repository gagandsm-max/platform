# Platform — API, Event, Data Model & Schema Contracts

**Document ID:** PLATFORM-CONTRACTDATA-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.2 document preserves the normative content of the listed v3.1 source documents (via v3.2/v3.2.1) and adds the approved v3.2.2 NOC troubleshooting API/event/data contracts plus the 2026-09-19 Sensor communication-integrity contract hardening below. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

## 1. Purpose

Defines public/internal API compatibility, Sensor/Agent control contracts, Kafka event rules,
idempotency/versioning, PostgreSQL/ClickHouse/Kafka/Valkey ownership, canonical identity, schema migration,
retention classes and ingestion/data-integrity rules.

## 2. Source-document provenance

- `Platform_19_API_and_Event_Contract_Master.md` (v3.1) — fully incorporated below.
- `Platform_20_Data_Model_and_Schema_Master.md` (v3.1) — fully incorporated below.

## 3. Platform — API & Event Contract Master

> Consolidated from `Platform_19_API_and_Event_Contract_Master.md` v3.1. Requirements in this section remain normative.

## 1. Purpose

Defines compatibility rules for UI APIs, public automation APIs, Sensor/Agent control channels, internal gRPC/HTTPS ingestion and Kafka events.

## 2. API principles

Versioned paths/contracts, server-side RBAC/tenant enforcement, idempotency keys for mutating bulk/job operations, pagination, query limits, correlation IDs, stable error codes and machine-readable remediation fields where appropriate.

### 2.1 NOC troubleshooting API contracts

The versioned API surface shall expose typed resource families for **Diagnostic Jobs, Live Evidence, Investigation Sessions and CLI Sessions**. Exact route strings are frozen in the generated OpenAPI contract before implementation/GA; clients must not depend on undocumented routes.

**Diagnostic Job create request** contains at minimum: origin type/id (entity/interface/alert/incident/investigation), canonical tenant-scoped target entity, diagnostic type, bounded parameters, requested vantage mode (`ASSIGNED`, authorized `SELECTED`, or `MULTI`), optional selected vantage IDs, optional investigation/incident reference and client correlation/idempotency context where applicable.

**Diagnostic Job response/state** contains: `diagnostic_job_id`, correlation ID, authorization/result state (`QUEUED|RUNNING|PARTIAL|SUCCEEDED|FAILED|CANCELLED|TIMED_OUT|DENIED`), effective bounded parameters, resolved vantage set, per-vantage typed results/errors, start/end/result timestamps, evidence quality/provenance and stable remediation/error codes. Cancellation is an explicit authorized operation. A retry is a new execution unless the same idempotency key is replayed.

**Live Evidence** returns monitored evidence separately from latest applicable `ON_DEMAND` evidence. Each item carries evidence type, source/vantage, observed/result time, freshness/quality, correlation/evidence reference and qualification state. The contract must not overwrite monitored history simply because an on-demand test succeeded or failed.

**Investigation Session** create/read/update/close contracts expose investigation ID, tenant, origin, owner, state, optional incident link, reason/title, timestamps and an ordered index of authorized evidence references. Evidence attachment references existing authoritative diagnostic/CLI/query/config/topology/Flow/log/change/RCA artifacts where possible rather than copying full payloads.

**CLI Session** creation accepts target, requested privilege, reason/ticket/approval context and credential mode (`STORED_PROFILE`, policy-permitted `PERSONAL`, or `ONE_TIME`). Reusable credential values are never returned to the browser. One-time secret material is accepted only on the protected session-creation request, is never echoed, persisted to normal job/audit/event stores or included in support/observability payloads, and is discarded when no longer required. Successful creation returns only short-lived session metadata/token needed to establish the authorized terminal channel.

All contracts are tenant/resource scoped server-side, use stable error codes/correlation IDs, and expose `DENIED`, `CAPACITY_LIMITED`, `TARGET_UNREACHABLE`, `CREDENTIAL_FAILURE`, `TIMEOUT`, `CANCELLED` and other protocol-specific normalized outcomes without leaking secrets.

## 3. Sensor/Agent control contract

Enrollment establishes identity/certificate. Subsequent sessions authenticate with mTLS. Core sends versioned desired configuration/assignment; endpoint acknowledges applied version/checksum. Telemetry ingestion is asynchronous and replay-safe.

For on-demand diagnostics, Core sends an authorized bounded execution request containing job/correlation ID, canonical target, diagnostic type, effective parameters, timeout/cancellation/fencing context and no reusable secret material beyond the minimum execution reference/token required by the worker. Sensor/Agent returns a typed per-vantage result with timestamps, evidence quality and normalized error/result code. Expired/replayed/fenced diagnostic jobs are rejected.

Sensor assignment/control contracts carry a monotonically advancing assignment/fencing version and lease validity. A Sensor rejects work superseded by a newer version it has observed. The exact disconnected-grace, reassignment hold-off and any extra epoch/token metadata remain a P0 engineering spike so transient Core loss does not automatically stop polling and stale disconnected ownership cannot persist indefinitely.

### 3.1 Durable telemetry acknowledgement contract

Each replayable Sensor stream/batch carries deterministic source identity, sequence/range identity and checksum/idempotency metadata sufficient to detect gaps, regressions, duplicates and conflicting replay.

Core durable acknowledgement exposes an accepted-through watermark. Under normal acknowledgement-driven compaction, Sensor may advance its durable acknowledged watermark and reclaim covered spool data only after a valid acknowledgement consistent with the certified HA recoverability boundary. Missing, malformed, out-of-range or unsupported acknowledgement semantics fail closed and retain spool data. The mandatory 80% emergency storage-protection path is separate: unavoidable shedding of eligible oldest lower-priority unacknowledged data must not advance the acknowledged watermark or be represented as delivery, and the resulting loss/gap must be explicitly accounted for. When such shedding creates a non-contiguous sequence gap, neither Sensor nor Core may infer acknowledgement across the missing range. The exact loss-range representation/reconciliation and rule for resuming later accepted-through progress are part of the existing `ACK_DURABLE` **TBD/P0-SPIKE**; until certified, acknowledgement-driven reclamation across that gap fails closed while bounded collection continues under the storage-protection policy.

The exact `ACK_DURABLE` recoverability boundary across the two-Core Kafka/PostgreSQL/ClickHouse failure model is **TBD/P0-SPIKE** until crash/failover tests prove it. Cross-system distributed 2PC/XA is not assumed by declaration. At-least-once transport is acceptable when duplicate authoritative effects are prevented by deterministic identity/idempotency.


## 4. Kafka event rules

Kafka is internal. Events include schema version, tenant, source identity, event ID/sequence where required, event time, ingest time, quality/provenance and payload. Producers/consumers must tolerate compatible schema evolution.

Diagnostic/investigation event families, where carried internally, include job/session ID, origin reference, canonical target/evidence reference, source/vantage, state transition, timestamps, quality/provenance, normalized result/error code and correlation ID. **Secrets, one-time credentials, reusable credential material and terminal keystroke payloads are prohibited from general Kafka/audit/analytics events.** Browser/UI live updates use an authenticated Platform UI channel; Kafka is never exposed directly to the browser.

## 5. Compatibility

Breaking API/event changes require major contract version and migration window. Core must expose compatibility matrix for Sensor/Agent versions.

## 4. Platform — Data Model & Schema Master

> Consolidated from `Platform_20_Data_Model_and_Schema_Master.md` v3.1. Requirements in this section remain normative.

## 1. Ownership

PostgreSQL owns transactional/control state: tenants, users/RBAC metadata, inventory/current entity state, policies, credentials metadata/references, licensing, **diagnostic job metadata, Investigation Session state/evidence references**, other job metadata and configuration. ClickHouse owns high-volume historical metrics/flow/events/log/audit analytics and normalized diagnostic-result analytics where retention/query volume justifies it. Kafka transports events; Valkey stores cache/ephemeral state only. No separate diagnostics database is introduced.

## 2. Canonical identity

Every monitored object maps to canonical tenant-scoped entity identity independent of protocol. IP is an attribute, not sole identity. Provenance and source protocol are retained.

## 3. Schema migration

Migrations are versioned, automated, tested from supported upgrade origins and idempotent where practical. Destructive migrations require backup/rollback plan and explicit approval.

## 4. Retention classes

Raw, rollup, incident, audit, configuration, **diagnostic/investigation evidence** and protected/legal-hold classes use different lifecycle policies. Investigation evidence linked to an incident/legal hold follows the applicable protected lifecycle; ephemeral one-time credentials are never a retention class. Emergency purge never treats all tables equally.

## 5. Data integrity

Ingestion records support deduplication/idempotency and late/out-of-order data handling. Pipeline reconciliation detects produced/Sensor-durable/Sensor-acknowledged/Core-accepted/Kafka-produced/consumer-processed/storage-inserted discrepancies and explicitly accounts for rejected, duplicate, late and intentionally dropped records.

Reconciliation may use sequence ranges, offsets, compact watermarks and counters appropriate to each stream/transformation; it does not require duplicate per-stage storage of every observation solely for accounting. Any residual unexplained discrepancy outside the certified tolerance/transform contract emits `PIPELINE_INTEGRITY_GAP` with affected scope and audit evidence.

Diagnostic/investigation integrity requires tenant-scoped canonical targets, immutable result/evidence provenance, per-vantage source identity, result/observed timestamps, correlation IDs and explicit state transitions. Evidence references must not silently retarget another tenant/entity after creation. Cancellation/timeout/partial results remain visible. No secret or one-time credential value may be persisted in diagnostic result, investigation evidence, audit, event or analytics rows.

---

## 5. v3.2.2 UI/dashboard/query contract supplement (2026-09-19 amendment)

### Dashboard definition/version contract

Dashboard APIs operate on tenant-scoped, schema-versioned declarative definitions. Published versions are immutable and include dashboard ID, version/revision, actor, timestamp, optional change note and integrity metadata. Draft and publish operations are separate authorization decisions. Rollback creates a new published version derived from a prior one.

Definitions may reference approved panel types, Query ASTs, variables, links, layout and presentation options. They must not contain plaintext credentials, executable customer code or cross-tenant object references.

### Query request contract

A panel/query request carries:
- query schema version;
- source/data domain;
- namespace/resource selectors;
- metric/field;
- aggregation/group/time bucket;
- typed filters/variables;
- requested time range/timezone/comparison context;
- bounded output hints such as panel width/resolution;
- correlation/request ID.

Server derives authoritative tenant/resource scope from authentication and permissions, not from client claims. Query services enforce budgets and may choose a coarser safe resolution.

Raw-query APIs, where present, are separate expert contracts with read-only source-specific validation and all normal tenant/budget controls.

### Shared context and saved views

Saved views/context serialization may persist compatible tenant-safe filters, time preferences, entity/service/site/tag selections, layout and visualization state. Context payloads cannot grant authority. Resource IDs are re-authorized on use.

### Provisioning/preflight

Provisioning APIs use credential-reference IDs, not secrets. Preflight is an asynchronous or bounded synchronous job returning structured checks, evidence status and remediation codes. The browser never receives reusable secret material from preflight responses.

### Error/data safety

Dashboard, query, import and connector APIs return stable safe error codes. SQL text, stack traces, credentials, private data-source URLs and unredacted raw payloads are not returned merely for debugging. Large raw response inspection is bounded and escaped.

---

## 6. v3.2.2 operational-view contract additions (2026-09-19 amendment)

### Synthetic/API evidence

Where a synthetic monitor captures phase timing, responses may expose structured bounded timing phases such as DNS, connect, TLS, first-byte and transfer plus assertion results and location identity. Raw/response inspection is separately authorized, size bounded, escaped and redacted. Authentication material and secret headers are never returned.

### Database operational evidence

Database query/workload responses use normalized query identity where possible and bounded samples. Query text, lock/session evidence and explain-plan payloads are tenant scoped, permission checked and redacted. Explain plans should expose a normalized tree when parser support exists and a bounded escaped raw representation otherwise.

### Threshold provenance

Panel definitions distinguish visualization/presentation thresholds from references to effective monitoring policy. A dashboard-definition write cannot mutate alert policy unless it calls the separate authorized policy API/workflow.

### Faceplate metadata

Physical faceplate coordinates/relationships are optional evidence supplied by certified device/hardware metadata. Clients must tolerate absence and fall back to logical interface lists/density representations rather than inventing physical placement.

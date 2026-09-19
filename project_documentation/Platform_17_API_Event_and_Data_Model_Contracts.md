# Platform — API, Event, Data Model & Schema Contracts

**Document ID:** PLATFORM-CONTRACTDATA-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.2 document preserves the normative content of the listed v3.1 source documents (via v3.2/v3.2.1) and adds the 2026-09-19 Sensor communication-integrity contract hardening below. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

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

## 3. Sensor/Agent control contract

Enrollment establishes identity/certificate. Subsequent sessions authenticate with mTLS. Core sends versioned desired configuration/assignment; endpoint acknowledges applied version/checksum. Telemetry ingestion is asynchronous and replay-safe.

Sensor assignment/control contracts carry a monotonically advancing assignment/fencing version and lease validity. A Sensor rejects work superseded by a newer version it has observed. The exact disconnected-grace, reassignment hold-off and any extra epoch/token metadata remain a P0 engineering spike so transient Core loss does not automatically stop polling and stale disconnected ownership cannot persist indefinitely.

### 3.1 Durable telemetry acknowledgement contract

Each replayable Sensor stream/batch carries deterministic source identity, sequence/range identity and checksum/idempotency metadata sufficient to detect gaps, regressions, duplicates and conflicting replay.

Core durable acknowledgement exposes an accepted-through watermark. Under normal acknowledgement-driven compaction, Sensor may advance its durable acknowledged watermark and reclaim covered spool data only after a valid acknowledgement consistent with the certified HA recoverability boundary. Missing, malformed, out-of-range or unsupported acknowledgement semantics fail closed and retain spool data. The mandatory 80% emergency storage-protection path is separate: unavoidable shedding of eligible oldest lower-priority unacknowledged data must not advance the acknowledged watermark or be represented as delivery, and the resulting loss/gap must be explicitly accounted for. When such shedding creates a non-contiguous sequence gap, neither Sensor nor Core may infer acknowledgement across the missing range. The exact loss-range representation/reconciliation and rule for resuming later accepted-through progress are part of the existing `ACK_DURABLE` **TBD/P0-SPIKE**; until certified, acknowledgement-driven reclamation across that gap fails closed while bounded collection continues under the storage-protection policy.

The exact `ACK_DURABLE` recoverability boundary across the two-Core Kafka/PostgreSQL/ClickHouse failure model is **TBD/P0-SPIKE** until crash/failover tests prove it. Cross-system distributed 2PC/XA is not assumed by declaration. At-least-once transport is acceptable when duplicate authoritative effects are prevented by deterministic identity/idempotency.

## 4. Kafka event rules

Kafka is internal. Events include schema version, tenant, source identity, event ID/sequence where required, event time, ingest time, quality/provenance and payload. Producers/consumers must tolerate compatible schema evolution.

## 5. Compatibility

Breaking API/event changes require major contract version and migration window. Core must expose compatibility matrix for Sensor/Agent versions.

## 4. Platform — Data Model & Schema Master

> Consolidated from `Platform_20_Data_Model_and_Schema_Master.md` v3.1. Requirements in this section remain normative.

## 1. Ownership

PostgreSQL owns transactional/control state: tenants, users/RBAC metadata, inventory/current entity state, policies, credentials metadata/references, licensing, job metadata and configuration. ClickHouse owns high-volume historical metrics/flow/events/log/audit analytics as defined. Kafka transports events; Valkey stores cache/ephemeral state only.

## 2. Canonical identity

Every monitored object maps to canonical tenant-scoped entity identity independent of protocol. IP is an attribute, not sole identity. Provenance and source protocol are retained.

## 3. Schema migration

Migrations are versioned, automated, tested from supported upgrade origins and idempotent where practical. Destructive migrations require backup/rollback plan and explicit approval.

## 4. Retention classes

Raw, rollup, incident, audit, configuration and protected/legal-hold classes use different lifecycle policies. Emergency purge never treats all tables equally.

## 5. Data integrity

Ingestion records support deduplication/idempotency and late/out-of-order data handling. Pipeline reconciliation detects produced/Sensor-durable/Sensor-acknowledged/Core-accepted/Kafka-produced/consumer-processed/storage-inserted discrepancies and explicitly accounts for rejected, duplicate, late and intentionally dropped records.

Reconciliation may use sequence ranges, offsets, compact watermarks and counters appropriate to each stream/transformation; it does not require duplicate per-stage storage of every observation solely for accounting. Any residual unexplained discrepancy outside the certified tolerance/transform contract emits `PIPELINE_INTEGRITY_GAP` with affected scope and audit evidence.

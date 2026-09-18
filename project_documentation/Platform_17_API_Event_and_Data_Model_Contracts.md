# Platform — API, Event, Data Model & Schema Contracts

**Document ID:** PLATFORM-CONTRACTDATA-001
**Version:** 3.2.3
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.1 document preserves the normative content of the listed v3.1 source documents (via v3.2). Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

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

Ingestion records support deduplication/idempotency and late/out-of-order data handling. Pipeline reconciliation detects produced/accepted/normalized/inserted/rejected discrepancies.

---

## 5. v3.2.3 UI/dashboard/query contract supplement

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

## 6. v3.2.3 operational-view contract additions

### Synthetic/API evidence

Where a synthetic monitor captures phase timing, responses may expose structured bounded timing phases such as DNS, connect, TLS, first-byte and transfer plus assertion results and location identity. Raw/response inspection is separately authorized, size bounded, escaped and redacted. Authentication material and secret headers are never returned.

### Database operational evidence

Database query/workload responses use normalized query identity where possible and bounded samples. Query text, lock/session evidence and explain-plan payloads are tenant scoped, permission checked and redacted. Explain plans should expose a normalized tree when parser support exists and a bounded escaped raw representation otherwise.

### Threshold provenance

Panel definitions distinguish visualization/presentation thresholds from references to effective monitoring policy. A dashboard-definition write cannot mutate alert policy unless it calls the separate authorized policy API/workflow.

### Faceplate metadata

Physical faceplate coordinates/relationships are optional evidence supplied by certified device/hardware metadata. Clients must tolerate absence and fall back to logical interface lists/density representations rather than inventing physical placement.

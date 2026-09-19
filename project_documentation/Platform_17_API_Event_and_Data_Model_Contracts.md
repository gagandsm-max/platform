# Platform — API, Event, Data Model & Schema Contracts

**Document ID:** PLATFORM-CONTRACTDATA-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.2 document preserves the normative content of the listed v3.1 source documents (via v3.2) and adds the approved v3.2.2 NOC troubleshooting API/event/data contracts. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

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

### 2.2 Cloud and full-stack observability API contracts

The versioned API surface shall expose typed resource families for cloud provider connections, hierarchy/scope discovery, credential/permission preflight, cloud resource/service query, connector health/rate budget, observability coverage/blind spots, traces/cross-signal pivots, service graph, cloud changes/events, quotas/autoscaling and SLO/service state as governed by `Platform_24_Cloud_and_Full_Stack_Observability_Master.md`.

Cloud connection create/update requests carry provider, tenant-scoped hierarchy scope, credential reference/auth mode, selected Cloud/API Sensor pool, selected Capability/Cloud Service Packs, include/exclude filters, collection policy and parent-scope ephemeral activation policy. Secret values are accepted only through protected credential-binding operations and are never returned by normal read APIs.

Hierarchy/resource queries are paginated and expose provider source IDs as aliases/evidence plus canonical Platform entity IDs. Trace/log/metric/event queries enforce tenant/resource scope and query/cardinality limits; trace/span IDs never authorize access by themselves.

Coverage and connector-health responses expose freshness, permission gaps, unsupported/unavailable signals, throttle/rate state, last successful reconciliation and normalized states such as `CONNECTED`, `PARTIAL`, `AUTHORIZATION_DEGRADED`, `THROTTLED`, `STALE`, `RECONCILING`, `MISCONFIGURED`, `DISCONNECTED` and `SUSPENDED`.

## 3. Sensor/Agent control contract

Enrollment establishes identity/certificate. Subsequent sessions authenticate with mTLS. Core sends versioned desired configuration/assignment; endpoint acknowledges applied version/checksum. Telemetry ingestion is asynchronous and replay-safe.

For on-demand diagnostics, Core sends an authorized bounded execution request containing job/correlation ID, canonical target, diagnostic type, effective parameters, timeout/cancellation/fencing context and no reusable secret material beyond the minimum execution reference/token required by the worker. Sensor/Agent returns a typed per-vantage result with timestamps, evidence quality and normalized error/result code. Expired/replayed/fenced diagnostic jobs are rejected.

## 4. Kafka event rules

Kafka is internal. Events include schema version, tenant, source identity, event ID/sequence where required, event time, ingest time, quality/provenance and payload. Producers/consumers must tolerate compatible schema evolution.

Diagnostic/investigation event families, where carried internally, include job/session ID, origin reference, canonical target/evidence reference, source/vantage, state transition, timestamps, quality/provenance, normalized result/error code and correlation ID. **Secrets, one-time credentials, reusable credential material and terminal keystroke payloads are prohibited from general Kafka/audit/analytics events.** Browser/UI live updates use an authenticated Platform UI channel; Kafka is never exposed directly to the browser.

Cloud/full-stack logical event families include telemetry metric/log/trace, cloud inventory/event/change/network-flow, connector-health/coverage and SLO evaluation. Exact topic names follow repository schema governance, but every schema carries tenant, source/provider, canonical entity/service references where known, schema version, event/observation time, ingest time, quality/freshness, provenance and idempotency/correlation identity where required. Provider events and trace/log attributes are untrusted payload data until validated; credentials and prohibited sensitive fields never enter general events.

## 5. Compatibility

Breaking API/event changes require major contract version and migration window. Core must expose compatibility matrix for Sensor/Agent versions.

## 4. Platform — Data Model & Schema Master

> Consolidated from `Platform_20_Data_Model_and_Schema_Master.md` v3.1. Requirements in this section remain normative.

## 1. Ownership

PostgreSQL owns transactional/control state: tenants, users/RBAC metadata, inventory/current entity state, policies, credentials metadata/references, licensing, **diagnostic job metadata, Investigation Session state/evidence references**, cloud provider connections and hierarchy metadata, Cloud Service Pack bindings, durable reconciliation cursors where correctness requires them, canonical cloud/service identity aliases, ephemeral-parent activation policy, SLO definitions, coverage policy/expectations, connector desired state and compatibility/certification references. ClickHouse owns high-volume historical metrics/flow/events/log/audit analytics, trace spans/derived trace metrics, cloud audit/control-plane events, normalized cloud changes, cloud network-flow facts/summaries, provider-health observations, historical quota/autoscaling observations, coverage-history facts, normalized diagnostic-result analytics and, when P1 is enabled, high-volume RUM/profile/cloud-cost facts. Kafka transports versioned events; Valkey stores only reconstructable cache/ephemeral state such as bounded correlation windows and rate/token buckets. No separate diagnostics, cloud or trace database is introduced by Platform_24.

## 2. Canonical identity

Every monitored object maps to canonical tenant-scoped entity identity independent of protocol. IP is an attribute, not sole identity. Provenance and source protocol are retained. The same runtime/resource/service must not become duplicate canonical entities merely because it is observed through provider API, Kubernetes, Agent, OpenTelemetry, logs, flow or synthetics. Provider resource IDs/ARNs/URIs, Kubernetes UIDs and OTel resource/service attributes are tenant-scoped source identifiers/aliases and never override Platform tenant authority.

## 3. Schema migration

Migrations are versioned, automated, tested from supported upgrade origins and idempotent where practical. Destructive migrations require backup/rollback plan and explicit approval.

## 4. Retention classes

Raw, rollup, incident, audit, configuration, **diagnostic/investigation evidence**, trace, cloud-change/event/network-flow, coverage-history and protected/legal-hold classes use different lifecycle policies. P1 RUM/profile/cost classes are independently configurable. Investigation evidence linked to an incident/legal hold follows the applicable protected lifecycle; ephemeral one-time credentials are never a retention class. Emergency purge never treats all tables equally. Trace/log/RUM/profile attributes are subject to sensitive-field and cardinality policy before long-term retention/indexing.

## 5. Data integrity

Ingestion records support deduplication/idempotency and late/out-of-order data handling. Pipeline reconciliation detects produced/accepted/normalized/inserted/rejected discrepancies.

Diagnostic/investigation integrity requires tenant-scoped canonical targets, immutable result/evidence provenance, per-vantage source identity, result/observed timestamps, correlation IDs and explicit state transitions. Evidence references must not silently retarget another tenant/entity after creation. Cancellation/timeout/partial results remain visible. No secret or one-time credential value may be persisted in diagnostic result, investigation evidence, audit, event or analytics rows.

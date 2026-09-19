# Platform — API, Event, Data Model & Schema Contracts

**Document ID:** PLATFORM-CONTRACTDATA-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.2 document preserves the normative content of the listed v3.1 source documents (via v3.2) and adds the approved v3.2.2 NOC troubleshooting plus enterprise-Flow/dedicated-Sensor API/event/data contracts. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

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

### 2.2 Flow and dedicated Sensor API contracts

Versioned API resource families shall expose, as authorized:

- Sensor role/capability/readiness and governed drain/change-role state;
- Flow listeners and configured exporters/source mappings;
- exporter health/data quality;
- v9/IPFIX template inventory/diagnostics;
- Flow search/query, Top-N, conversations, matrices and bounded Sankey data;
- Flow Coverage evidence;
- enrichment/provenance detail where permitted.

Flow query APIs accept bounded tenant/time/scope plus filters such as exporter, observation domain, device/interface/site, source/destination IP/entity/FQDN/user, protocol/port/application, ASN, VRF/VLAN, QoS, sampling and quality. User-attribution fields require separate server-side permission; field-level omission is enforced before response serialization/export.

Exporter configuration binds tenant, eligible FLOW Sensor/pool/listener, protocol/profile, source policy, canonical exporter entity and optional expected observation domains/rate/capacity metadata. Client-supplied packet fields never override this authority.

Sensor role APIs expose effective role and allowed capabilities. A production role transition is a stateful drain/reassignment operation with explicit validation/results, not an immediate boolean toggle.

## 3. Sensor/Agent control contract

Enrollment establishes identity/certificate. Subsequent sessions authenticate with mTLS. Core sends versioned desired configuration/assignment; endpoint acknowledges applied version/checksum. Telemetry ingestion is asynchronous and replay-safe.

For on-demand diagnostics, Core sends an authorized bounded execution request containing job/correlation ID, canonical target, diagnostic type, effective parameters, timeout/cancellation/fencing context and no reusable secret material beyond the minimum execution reference/token required by the worker. Sensor/Agent returns a typed per-vantage result with timestamps, evidence quality and normalized error/result code. Expired/replayed/fenced diagnostic jobs are rejected.

Sensor desired state also carries the effective production role/capability allowlist. A FLOW Sensor rejects Network/Log assignments/listeners; a LOG Sensor rejects Flow/Network assignments. Role-generation changes are applied only after the governed drain/reassignment preconditions are satisfied and are reported through desired/applied generation/checksum semantics.

## 4. Kafka event rules

Kafka is internal. Events include schema version, tenant, source identity, event ID/sequence where required, event time, ingest time, quality/provenance and payload. Producers/consumers must tolerate compatible schema evolution.

`network.flow.v1` logical events carry stable `flow_record_id`, tenant, FLOW Sensor/listener, configured exporter/canonical entity, protocol/version, observation-domain/engine identity where applicable, source sequence/record index, export/observed/ingest/start/end times with quality, directional 5-tuple/protocol fields, observed and separately estimated counters, sampling metadata, raw interface/VRF/VLAN/QoS/NAT/application/template fields, quality and bounded extensions. Enrichment may be embedded or referenced, but source/time/version/confidence/provenance remain explicit.

Platform transport replay uses stable Flow record identity for deduplication. Records from different valid observation points are not deduplicated merely because 5-tuples/time overlap.

Diagnostic/investigation event families, where carried internally, include job/session ID, origin reference, canonical target/evidence reference, source/vantage, state transition, timestamps, quality/provenance, normalized result/error code and correlation ID. **Secrets, one-time credentials, reusable credential material and terminal keystroke payloads are prohibited from general Kafka/audit/analytics events.** Browser/UI live updates use an authenticated Platform UI channel; Kafka is never exposed directly to the browser.

## 5. Compatibility

Breaking API/event changes require major contract version and migration window. Core must expose compatibility matrix for Sensor/Agent versions.

## 4. Platform — Data Model & Schema Master

> Consolidated from `Platform_20_Data_Model_and_Schema_Master.md` v3.1. Requirements in this section remain normative.

## 1. Ownership

PostgreSQL owns transactional/control state: tenants, users/RBAC metadata, inventory/current entity state, policies, credentials metadata/references, licensing, **diagnostic job metadata, Investigation Session state/evidence references**, Sensor role/source assignment, Flow listener/exporter/profile configuration, saved Flow views/policies and current identity/session mapping metadata where applicable, other job metadata and configuration. ClickHouse owns high-volume historical metrics/flow/events/log/audit analytics, Flow raw/common observations, quality history and selected Flow conversation/Top-N/matrix/coverage rollups where retention/query volume justifies it. Kafka transports events; Valkey stores cache/ephemeral state only. No separate diagnostics, Flow or identity database is introduced.

## 2. Canonical identity

Every monitored object maps to canonical tenant-scoped entity identity independent of protocol. IP is an attribute, not sole identity. Provenance and source protocol are retained.

Flow enrichment resolves IP/address observations against temporal canonical identity. Historical endpoint/system/site/user/domain/Geo/cloud attribution records the applicable evidence time/version/source/confidence so current address ownership or directory state cannot silently rewrite old Flow.

User identity is a subject/session attribution, not part of the Flow record's authorization boundary. Directory identity (AD/LDAP/Entra) may enrich a session only when time-bounded evidence connects the subject to the endpoint/network attachment. Shared/NAT/multi-user/conflicting evidence can remain unresolved.

## 3. Schema migration

Migrations are versioned, automated, tested from supported upgrade origins and idempotent where practical. Destructive migrations require backup/rollback plan and explicit approval.

## 4. Retention classes

Raw, rollup, incident, audit, configuration, **diagnostic/investigation evidence** and protected/legal-hold classes use different lifecycle policies. Investigation evidence linked to an incident/legal hold follows the applicable protected lifecycle; ephemeral one-time credentials are never a retention class. Emergency purge never treats all tables equally.

## 5. Data integrity

Ingestion records support deduplication/idempotency and late/out-of-order data handling. Pipeline reconciliation detects produced/accepted/normalized/inserted/rejected discrepancies.

Flow integrity additionally distinguishes pre-accept UDP loss from post-accept durable Platform responsibility; tracks protocol-specific sequence/exporter reset, template/sampling quality and Sensor socket/queue/spool drops; and reconciles received datagrams/decoded records/accepted observations/published events/ClickHouse inserts. A missing template, unknown sampling or enrichment failure must not be silently represented as zero traffic or authoritative endpoint/user identity.

Flow source replay deduplication and multi-observation-point correlation are distinct: identical-looking traffic at different exporters/interfaces can be valid separate evidence.

Diagnostic/investigation integrity requires tenant-scoped canonical targets, immutable result/evidence provenance, per-vantage source identity, result/observed timestamps, correlation IDs and explicit state transitions. Evidence references must not silently retarget another tenant/entity after creation. Cancellation/timeout/partial results remain visible. No secret or one-time credential value may be persisted in diagnostic result, investigation evidence, audit, event or analytics rows.

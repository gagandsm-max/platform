# Platform — API, Event, Data Model & Schema Contracts

**Document ID:** PLATFORM-CONTRACTDATA-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.2 document preserves the normative content of the listed v3.1 source documents (via v3.2) and adds the approved v3.2.2 NOC troubleshooting plus enterprise Flow/NTA and dedicated Sensor-role API/event/data contracts. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

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

### 2.2 Flow/NTA API and resource contracts

The versioned API surface shall expose typed resource families for Flow Exporter Profiles, Flow listeners/source admission, exporter health/data quality, template status, Flow search/analytics, Flow Coverage/consistency and authorized enrichment provenance. Exact public route strings are frozen in generated OpenAPI before implementation/GA.

Exporter configuration includes tenant/resource scope, assigned `FLOW_SENSOR`/pool, listener, allowed source mapping/policy, protocol set, optional observation-domain/engine constraints, rate/resource policy, enrichment policy, redundancy profile and revision. Unknown/quarantined exporters are visible to authorized administrators but cannot self-activate.

Flow query contracts are bounded by time/cardinality/result size and support IPv4/IPv6, exporter/observation point/interface, source/destination entity/IP/subnet/site, domain/application, authorized user attribution, ASN/cloud/Geo, protocol/ports, VRF/VLAN/QoS, sampling and quality filters.

User/session enrichment fields require distinct authorization from ordinary Flow viewing where policy requires. API responses include provenance/confidence/effective time and shall not represent directory membership as historical session proof, public Geo as precise user location, or a domain/application as a full URL.

Template/API diagnostics expose metadata/status/counters rather than arbitrary unbounded exporter payloads.

## 3. Sensor/Agent control contract

Enrollment establishes identity/certificate. Subsequent sessions authenticate with mTLS. Core sends versioned desired configuration/assignment; endpoint acknowledges applied version/checksum. Telemetry ingestion is asynchronous and replay-safe.

Sensor desired configuration includes the governed role. A production `FLOW_SENSOR` or `LOG_SENSOR` rejects incompatible workload assignments/module activation. Role change is versioned and requires drain/spool accounting before acknowledgement of the new effective role.

For on-demand diagnostics, Core sends an authorized bounded execution request containing job/correlation ID, canonical target, diagnostic type, effective parameters, timeout/cancellation/fencing context and no reusable secret material beyond the minimum execution reference/token required by the worker. Sensor/Agent returns a typed per-vantage result with timestamps, evidence quality and normalized error/result code. Expired/replayed/fenced diagnostic jobs are rejected.

## 4. Kafka event rules

Kafka is internal. Events include schema version, tenant, source identity, event ID/sequence where required, event time, ingest time, quality/provenance and payload. Producers/consumers must tolerate compatible schema evolution.

For `network.flow.v1`, the canonical envelope includes trusted tenant/Sensor/exporter/listener identity, protocol/version, observation-domain/engine context where applicable, stable Platform flow-record/ingestion identity, source sequence/record index where available, export/observed/ingest and optional flow-start/end times with quality, sampling state, template/generation reference, directional tuple/value fields, observation point, bounded enrichment provenance and Flow Data Quality.

Kafka realization may use compatible typed substreams only where benchmark/consumer-isolation evidence justifies them. Packet-provided tenant identity is never authoritative.

Diagnostic/investigation event families, where carried internally, include job/session ID, origin reference, canonical target/evidence reference, source/vantage, state transition, timestamps, quality/provenance, normalized result/error code and correlation ID. **Secrets, one-time credentials, reusable credential material and terminal keystroke payloads are prohibited from general Kafka/audit/analytics events.** Browser/UI live updates use an authenticated Platform UI channel; Kafka is never exposed directly to the browser.

## 5. Compatibility

Breaking API/event changes require major contract version and migration window. Core must expose compatibility matrix for Sensor/Agent versions.

## 4. Platform — Data Model & Schema Master

> Consolidated from `Platform_20_Data_Model_and_Schema_Master.md` v3.1. Requirements in this section remain normative.

## 1. Ownership

PostgreSQL owns transactional/control state: tenants, users/RBAC metadata, inventory/current entity state, policies, credentials metadata/references, licensing, **diagnostic job metadata, Investigation Session state/evidence references**, Flow exporter/listener/source-admission/enrichment/redundancy policy and saved-view metadata, Sensor role/configuration state, other job metadata and configuration. ClickHouse owns high-volume historical metrics/flow/events/log/audit analytics, normalized Flow records/quality and benchmark-selected Flow rollups plus normalized diagnostic-result analytics where retention/query volume justifies it. Kafka transports events; Valkey stores cache/ephemeral state only. No separate diagnostics or NTA database is introduced.

## 2. Canonical identity

Every monitored object maps to canonical tenant-scoped entity identity independent of protocol. IP is an attribute, not sole identity. Provenance and source protocol are retained.

Flow uses temporal identity. Source/destination IP at an observation time may resolve to canonical endpoint/device/system/FQDN, site/subnet and user/session identity only through provenance-bearing history. AD/LDAP/Entra directory identity enriches the canonical user/person/account model but is not sufficient historical session evidence by itself. Current address/hostname/user/site state must not silently rewrite historical Flow.

The canonical temporal identity-session/association contract is tenant scoped and reusable across Flow, logs, alerts, RCA, topology and endpoint monitoring. It carries stable session/association identity, user/account identity, endpoint entity identity, observed address binding where available, effective/session interval, source, quality/confidence and provenance. Shared or multiplexed endpoints may legitimately return ambiguous attribution; APIs must preserve ambiguity rather than fabricate one user.

Flow enrichment fields are derived evidence, not raw source truth. API/event responses preserve source observation identity separately from normalized Flow and enrichment context. Enrichment changes are versioned/effective-time aware; a present-day identity lookup must not silently reinterpret historical observations.


Flow observation identity and analytical conversation identity are distinct. Stable source record/Platform ingestion identity supports replay/deduplication; a derived conversation may reference multiple valid observation points without destroying directional provenance.

## 3. Schema migration

Migrations are versioned, automated, tested from supported upgrade origins and idempotent where practical. Destructive migrations require backup/rollback plan and explicit approval.

## 4. Retention classes

Raw, rollup, incident, audit, configuration, **diagnostic/investigation evidence** and protected/legal-hold classes use different lifecycle policies. Flow distinguishes raw normalized Flow from longer-lived benchmark-selected conversation/site/interface/application aggregates and incident-protected Flow evidence. Investigation evidence linked to an incident/legal hold follows the applicable protected lifecycle; ephemeral one-time credentials are never a retention class. Emergency purge never treats all tables equally.

## 5. Data integrity

Ingestion records support deduplication/idempotency and late/out-of-order data handling. Pipeline reconciliation detects produced/accepted/normalized/inserted/rejected discrepancies.

Flow integrity additionally requires: protocol-specific exporter/template/sampling/sequence accounting; stable record identity after certified local durable acceptance; replay-safe Sensor→Core delivery; separate socket/queue/template/sequence/rate-limit/spool/downstream gap counters; observed-vs-estimated sampling truth; and produced→durably accepted→uploaded→Core accepted→Kafka processed→ClickHouse inserted reconciliation. An acknowledgement/watermark shall not advance over unaccounted accepted Flow records.

Raw Flow observations are not globally deduplicated by five-tuple across different exporters/interfaces/observation points. Historical enrichment changes use versioned/temporal provenance rather than destructive reinterpretation.

Diagnostic/investigation integrity requires tenant-scoped canonical targets, immutable result/evidence provenance, per-vantage source identity, result/observed timestamps, correlation IDs and explicit state transitions. Evidence references must not silently retarget another tenant/entity after creation. Cancellation/timeout/partial results remain visible. No secret or one-time credential value may be persisted in diagnostic result, investigation evidence, audit, event or analytics rows.

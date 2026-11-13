# Platform — API, Event, Data Model & Schema Contracts

**Document ID:** PLATFORM-CONTRACTDATA-001
**Version:** 3.3.0
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract (TRANSITIONAL_LEGACY)**
**Normalization state:** TRANSITIONAL_LEGACY. BUILD-000 freezes a bounded edge/control contract subsection below; the remainder awaits later normalization.
**Product:** Platform (internal engineering/build identifier) / AmrudAi (commercial/customer-facing product name)
**Consolidation rule:** This v3.3.0 document preserves the normative content of the listed v3.1 source documents (via v3.2) and adds the approved v3.2.2 NOC troubleshooting plus enterprise-SNMP correctness API/event/data contracts. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.
**Last reconciliation:** 2026-10-12 — Issue #200/#203 Tranche B Phase 1/1.5 certification-envelope reconciliation (API/event/ACK/ICMP/tenant contracts); prior Issue #142 Phase-1 observability closure retained; document remains TRANSITIONAL_LEGACY.

## 1. Purpose

Defines public/internal API compatibility, Sensor/Agent control contracts, Kafka event rules,
idempotency/versioning, PostgreSQL/ClickHouse/Kafka/Valkey ownership, canonical identity, schema migration,
retention classes and ingestion/data-integrity rules.

## 2. Source-document provenance

- `Platform_19_API_and_Event_Contract_Master.md` (v3.1) — fully incorporated below.
- `Platform_20_Data_Model_and_Schema_Master.md` (v3.1) — fully incorporated below.

## 3. Platform — API & Event Contract Master

> Consolidated from `Platform_19_API_and_Event_Contract_Master.md` v3.1. Requirements in this section remain normative.

### 3.1 Purpose

Defines compatibility rules for UI APIs, public automation APIs, Sensor/Agent control channels, internal gRPC/HTTPS ingestion and Kafka events.

### 3.2 API principles

Versioned paths/contracts, server-side RBAC/tenant enforcement, idempotency keys for mutating bulk/job operations, pagination, query limits, correlation IDs, stable error codes and machine-readable remediation fields where appropriate.

#### 3.2.1 NOC troubleshooting API contracts

The versioned API surface shall expose typed resource families for **Diagnostic Jobs, Live Evidence, Investigation Sessions and CLI Sessions**. Exact route strings are frozen in the generated OpenAPI contract before implementation/GA; clients must not depend on undocumented routes.

**Diagnostic Job create request** contains at minimum: origin type/id (entity/interface/alert/incident/investigation), canonical tenant-scoped target entity, diagnostic type, bounded parameters, requested vantage mode (`ASSIGNED`, authorized `SELECTED`, or `MULTI`), optional selected vantage IDs, optional investigation/incident reference and client correlation/idempotency context where applicable.

**Diagnostic Job response/state** contains: `diagnostic_job_id`, correlation ID, authorization/result state (`QUEUED|RUNNING|PARTIAL|SUCCEEDED|FAILED|CANCELLED|TIMED_OUT|DENIED`), effective bounded parameters, resolved vantage set, per-vantage typed results/errors, start/end/result timestamps, evidence quality/provenance and stable remediation/error codes. Cancellation is an explicit authorized operation. A retry is a new execution unless the same idempotency key is replayed.

**Live Evidence** returns monitored evidence separately from latest applicable `ON_DEMAND` evidence. Each item carries evidence type, source/vantage, observed/result time, freshness/quality, correlation/evidence reference and qualification state. The contract must not overwrite monitored history simply because an on-demand test succeeded or failed.

**Investigation Session** create/read/update/close contracts expose investigation ID, tenant, origin, owner, state, optional incident link, reason/title, timestamps and an ordered index of authorized evidence references. Evidence attachment references existing authoritative diagnostic/CLI/query/config/topology/Flow/log/change/RCA artifacts where possible rather than copying full payloads.

**CLI Session** creation accepts target, requested privilege, reason/ticket/approval context and credential mode (`STORED_PROFILE`, policy-permitted `PERSONAL`, or `ONE_TIME`). Reusable credential values are never returned to the browser. One-time secret material is accepted only on the protected session-creation request, is never echoed, persisted to normal job/audit/event stores or included in support/observability payloads, and is discarded when no longer required. Successful creation returns only short-lived session metadata/token needed to establish the authorized terminal channel.

All contracts are tenant/resource scoped server-side, use stable error codes/correlation IDs, and expose `DENIED`, `CAPACITY_LIMITED`, `TARGET_UNREACHABLE`, `CREDENTIAL_FAILURE`, `TIMEOUT`, `CANCELLED` and other protocol-specific normalized outcomes without leaking secrets.

#### 3.2.2 SNMP polling, diagnostic and typed-observation contracts

SNMP scheduled-control contracts carry at minimum: tenant/entity, assignment key/version/fence, Sensor target, monitoring-profile/Capability-Pack version, operation/poll-plan reference or bounded operation set, Credential Profile reference/authorization context, timeout/retry/resource limits and correlation/job identity. **Reusable SNMP community/auth/privacy secret values are not part of public/customer-visible API schemas or general durable job-history records.**

On-demand SNMP Diagnostic Job parameters support bounded GET/WALK/BULKWALK operations, OID/root, result limits and authorized vantage. Response items preserve raw numeric OID, optional resolved symbol, BER/value kind, typed raw value, normalized/decoded value where available, source Sensor/vantage, result time, quality, error/exception code and remediation. Secret-bearing fields are prohibited from the response.

Canonical SNMP source observations are a typed union, not a numeric-only object:

- `NUMERIC` — signed/unsigned gauge/measurement appropriate for numeric analytics;
- `COUNTER` — exact unsigned raw counter value with counter width/source identity and optional derived delta/rate quality;
- `STATE` — enum/boolean/text state with canonical state mapping;
- `INVENTORY` — identity/description/model/interface/component facts;
- `TOPOLOGY` — relationship/neighbor/index facts;
- `EVENT` — trap/inform/notification evidence;
- `QUALITY` — timeout/auth/authz/noSuch/endOfMibView/partial/stale/discontinuity and other collection outcomes.

A poll may yield multiple types. Unsupported optional OIDs can yield typed `QUALITY` outcomes while valid sibling observations continue through their applicable contracts; acceptance policy must not silently acknowledge a partially discarded source record unless the record-level accounting contract proves every observation is represented or intentionally classified.

For counters, the canonical contract preserves exact `uint32/uint64` source value plus source metric/OID, timestamp, sequence/observation identity and discontinuity/reset evidence. Derived floating-point rate/utilization is a separate field/observation and never replaces authoritative raw Counter64. Counter64 must not be forced through Java `double`, JSON unsafe-number semantics or ClickHouse `Float64` before exact delta/reset processing.

Inform notification contracts distinguish durable local receiver acceptance/accounting from downstream enrichment and Core/Kafka/ClickHouse/alert/incident completion. Successful Inform ACK is represented only after bounded parse, configured security validation, sufficient source/tenant attribution, bounded pre-ACK storm admission/accounting and durable local acceptance (accepted raw event or durable storm-aggregation/accounting record). MIB/Pack semantic enrichment, final entity resolution and semantic dedup/correlation may proceed after ACK; they are not ACK prerequisites.


### 3.3 Sensor/Agent control contract

Enrollment establishes identity/certificate. Subsequent sessions authenticate with mTLS. Core sends versioned desired configuration/assignment; endpoint acknowledges applied version/checksum. Telemetry ingestion is asynchronous and replay-safe.

For on-demand diagnostics, Core sends an authorized bounded execution request containing job/correlation ID, canonical target, diagnostic type, effective parameters, timeout/cancellation/fencing context and no reusable secret material beyond the minimum execution reference/token required by the worker. Sensor/Agent returns a typed per-vantage result with timestamps, evidence quality and normalized error/result code. Expired/replayed/fenced diagnostic jobs are rejected.

### 3.4 Kafka event rules

Kafka is internal. Events include schema version, tenant, source identity, event ID/sequence where required, event time, ingest time, quality/provenance and payload. Producers/consumers must tolerate compatible schema evolution.

For SNMP, logical Kafka families distinguish at least numeric measurements, exact counters, state, inventory/topology, notifications and telemetry quality. The architecture may realize the exact-counter family with a dedicated versioned topic (for example `telemetry.counter.v1`) or compatible typed payloads only where ordering, precision, consumer isolation and schema evolution remain provable. A consumer must never deserialize Counter64 into a representation that cannot preserve the full unsigned 64-bit source range before counter processing.

Diagnostic/investigation event families, where carried internally, include job/session ID, origin reference, canonical target/evidence reference, source/vantage, state transition, timestamps, quality/provenance, normalized result/error code and correlation ID. **Secrets, one-time credentials, reusable credential material and terminal keystroke payloads are prohibited from general Kafka/audit/analytics events.** Browser/UI live updates use an authenticated Platform UI channel; Kafka is never exposed directly to the browser.

### 3.5 Compatibility

Breaking API/event changes require major contract version and migration window. Core must expose compatibility matrix for Sensor/Agent versions.

## 4. Platform — Data Model & Schema Master

> Consolidated from `Platform_20_Data_Model_and_Schema_Master.md` v3.1. Requirements in this section remain normative.

### 4.1 Ownership

PostgreSQL owns transactional/control state: tenants, users/RBAC metadata, inventory/current entity state, policies, credentials metadata/references, licensing, **diagnostic job metadata, Investigation Session state/evidence references**, SNMP monitoring definitions/MIB metadata/Pack mappings/interface-reconciliation current state where applicable, other job metadata and configuration. ClickHouse owns high-volume historical metrics/flow/events/log/audit analytics, exact raw counter history/derived counter analytics, state history, trap/event history, telemetry-quality history and normalized diagnostic-result analytics where retention/query volume justifies it. Kafka transports events; Valkey stores cache/ephemeral state only. No separate diagnostics or SNMP database is introduced.

### 4.2 Canonical identity

Every monitored object maps to canonical tenant-scoped entity identity independent of protocol. IP is an attribute, not sole identity. Provenance and source protocol are retained.

For interfaces, `ifIndex` is a protocol observation/index and may change. Canonical interface identity is reconciled from available stable evidence such as ifName/ifDescr/ifAlias, MAC, physical slot/port/entPhysical relationships, Device Pack stable identifiers and authorized operator reconciliation. Ambiguity is preserved as reconciliation/quality state rather than silent merge or duplicate creation.

### 4.3 Schema migration

Migrations are versioned, automated, tested from supported upgrade origins and idempotent where practical. Every applied migration is immutable; fixes use new forward-only migrations. Destructive migrations require backup/rollback plan and explicit approval. The current ordered PostgreSQL source authority is `core/src/main/resources/db/migrations/manifest.txt` (V001–V018 at `36e1513f`); §4.7 maps existing physical artifacts. This manifest does not itself certify any upgrade origin or authorize a new migration number.

### 4.4 Retention classes

Raw, rollup, incident, audit, configuration, **diagnostic/investigation evidence** and protected/legal-hold classes use different lifecycle policies. Investigation evidence linked to an incident/legal hold follows the applicable protected lifecycle; ephemeral one-time credentials are never a retention class. Emergency purge never treats all tables equally.

### 4.5 Data integrity

Ingestion records support deduplication/idempotency and late/out-of-order data handling. Pipeline reconciliation detects produced/accepted/normalized/inserted/rejected discrepancies.

Diagnostic/investigation integrity requires tenant-scoped canonical targets, immutable result/evidence provenance, per-vantage source identity, result/observed timestamps, correlation IDs and explicit state transitions. Evidence references must not silently retarget another tenant/entity after creation. Cancellation/timeout/partial results remain visible. No secret or one-time credential value may be persisted in diagnostic result, investigation evidence, audit, event or analytics rows.

SNMP integrity additionally requires exact Counter64 preservation before derivation, explicit counter reset/wrap/discontinuity quality, typed handling of non-numeric varbinds/exceptions, stable observation identity for replay/deduplication, and produced→accepted→typed-normalized→persisted accounting by source record/observation class. A failed/unsupported conversion must not be hidden by advancing an acknowledgement over unaccounted SNMP evidence. Inform ACK integrity requires pre-ACK storm admission/accounting plus durable local acceptance/accounting evidence and must not imply enrichment completion or Core/Kafka/ClickHouse/alert/incident completion.

---

### 4.6 Endpoint Diagnostic Snapshot contracts — v3.2.2 reconciliation

**EDS is a specialized diagnostic type/profile of the existing Diagnostic Job architecture**, not a second diagnostics framework, registry, scheduler, authorization system, investigation state machine or evidence store. Product-facing language may continue to use **Endpoint Diagnostic Snapshot (EDS)**; the authoritative API/event/data identity remains the Diagnostic Job contract in §3.2.1 with:

```text
diagnostic_type / profile = ENDPOINT_DIAGNOSTIC_SNAPSHOT
execution_mode            = AGENT | SSH | WINRM_WMI_CIM
```

EDS create/request/result/cancellation/expiry/idempotency/fencing reuse the existing Diagnostic Job, Live Evidence and Investigation Session contracts. Free-form script stdout is not the canonical interface.

#### 4.6.1 Diagnostic Job profile extension for EDS

An EDS request is a Diagnostic Job create whose typed parameters extend the existing job contract. Minimum EDS-profile fields (in addition to canonical `diagnostic_job_id`, tenant, target entity, vantage, authorization, correlation/idempotency, timeout/expiry and investigation/incident association):

```text
schema_version
diagnostic_job_id
diagnostic_type           ENDPOINT_DIAGNOSTIC_SNAPSHOT
tenant_id
entity_id
request_source            AUTO_POLICY | OPERATOR | INCIDENT | RCA
requested_at
expires_at
definition_id
definition_version
authority:
  assignment_generation     and/or equivalent Core-issued control generation
  effective_policy_id
  effective_policy_version
  sensor_ownership_generation optional where Sensor assignment fences execution
idempotency_key
correlation_id
trigger:                    local capture-eligibility provenance only
  policy_id
  policy_version
  metric_or_state_key
  capture_eligibility_state INELIGIBLE | QUALIFYING | CAPTURE_ELIGIBLE | COOLDOWN
  qualified_at
  observed_value/unit       optional
  threshold/context         policy-safe normalized representation
budget:
  deadline_ms
  max_evidence_bytes
  max_items
  max_log_window
  priority
parent_alert_id             optional
parent_incident_id          optional
investigation_session_id    optional
```

The receiving Agent/Sensor validates tenant/entity/assignment authority, **current Core-issued generation/fencing**, expiry, supported definition/version, idempotency and local resource policy before execution. Stale, revoked, expired, replayed or superseded authority must fail closed and must not publish evidence under obsolete authority.

#### 4.6.2 EDS capture result as subordinate Diagnostic Job evidence

EDS capture outcome is reported through the existing Diagnostic Job response/state and Live Evidence association. The job retains the canonical authorization/result vocabulary:

```text
QUEUED | RUNNING | PARTIAL | SUCCEEDED | FAILED | CANCELLED | TIMED_OUT | DENIED
```

Capture completeness detail is a subordinate typed field on the EDS profile result (not a competing lifecycle vocabulary):

```text
capture_status            COMPLETE | PARTIAL | FAILED | SUPPRESSED
```

Minimum EDS-profile result fields (in addition to canonical Diagnostic Job / Live Evidence provenance):

```text
schema_version
diagnostic_job_id
tenant_id
entity_id
source_mode               AGENT | SSH | WINRM_WMI_CIM
agent_id                  optional
sensor_id
vantage_id
definition_id
definition_version
authority_context         generation/policy/assignment identity that authorized capture
started_at
completed_at
duration_ms
job_state                 canonical Diagnostic Job state
capture_status            COMPLETE | PARTIAL | FAILED | SUPPRESSED
trigger_provenance
evidence_items[]
missing_evidence[]
collector_errors[]
resource_observation
loss_or_shed_reason       optional explicit incomplete/suppressed/expired/shed evidence
correlation_id
parent_alert_id           optional
parent_incident_id        optional
investigation_session_id  optional
```

Each evidence item includes a stable evidence type/key, typed value/state, unit where applicable, labels/dimensions within cardinality limits, observed time, source/provenance, quality and optional raw-artifact reference only where separately permitted.

`missing_evidence` identifies expected evidence that could not be obtained and the reason (`UNSUPPORTED|PRIVILEGE_DENIED|TIMEOUT|RESOURCE_GUARD|COLLECTOR_ERROR|POLICY_DENIED|NOT_AVAILABLE` or compatible versioned code). Suppressed, shortened, dropped, expired or storage-shed diagnostic evidence must remain explicit, observable and auditable; diagnostic protection must never silently destroy routine telemetry durability.

#### 4.6.3 EDS lifecycle events

EDS does not invent a second event family for job lifecycle. Canonical Diagnostic Job / Live Evidence / Investigation events remain authoritative. EDS-profile events are profile annotations or compatible extensions of that family and carry `diagnostic_job_id`, tenant/entity/correlation/schema/timestamp fields plus definition/policy/authority provenance. Compatible names may include:

- `diagnostic.job.requested` / `endpoint.diagnostic.requested` (alias)
- `diagnostic.job.started` / `endpoint.diagnostic.started` (alias)
- `diagnostic.job.completed` / `endpoint.diagnostic.completed` (alias)
- `diagnostic.job.partial` / `endpoint.diagnostic.partial` (alias)
- `diagnostic.job.failed` / `endpoint.diagnostic.failed` (alias)
- `diagnostic.job.suppressed` / `endpoint.diagnostic.suppressed` (alias)

Replayed duplicate requests/results are idempotently recognized and do not create duplicate authoritative evidence. New aliases must not fork an incompatible status machine.

#### 4.6.4 Causal boundary

EDS evidence may contain deterministic indicators but not an authoritative root-cause declaration. Core RCA references Diagnostic Job / Live Evidence IDs for EDS captures alongside topology, telemetry, changes, dependency and contradiction evidence. AI/LLM narrative output is a separate derived artifact with its own provenance and cannot mutate EDS facts.


### 4.7 Current physical contract artifact map — source `36e1513f`

This bounded source inventory binds existing implementation, not the complete product roadmap. Controller annotations establish source routes, not authentication success, feature activation, UI wiring, deployment or certification. Exact method/subpath/body/error/authorization behavior remains in each listed controller and its DTO/service/validator/test sources; future logical API examples elsewhere in this document are not automatically implemented endpoints. No generated OpenAPI artifact or new DDL is invented by this map.

| Current controller base path | Exact source authority |
|---|---|
| `/api/v1/tenants/{tenantId}/capability-packs` | `core/src/main/java/com/platform/core/api/CapabilityPackController.java` |
| `/api/v1/tenants/{tenantId}/discovery` | `core/src/main/java/com/platform/core/api/DiscoveryController.java` |
| `/api/v1/tenants/{tenantId}/entities` | `core/src/main/java/com/platform/core/api/EntityController.java` |
| `/api/v1/tenants/{tenantId}/entities/{entityId}/management-endpoints` | `core/src/main/java/com/platform/core/api/EntityManagementEndpointController.java` |
| `/api/v1/tenants/{tenantId}/entities/{entityId}/metric-observations` | `core/src/main/java/com/platform/core/api/EntityMetricObservationController.java` |
| `/api/v1/tenants/{tenantId}/groups` | `core/src/main/java/com/platform/core/api/GroupController.java` |
| `/api/v1/tenants/{tenantId}/sites` | `core/src/main/java/com/platform/core/api/SiteController.java` |
| `/api/v1/tenants/{tenantId}/tags` | `core/src/main/java/com/platform/core/api/TagController.java` |
| `/api/v1/tenants` | `core/src/main/java/com/platform/core/api/TenantController.java` |
| `/api/v1/sensor/agent-authority` | `core/src/main/java/com/platform/core/controller/SensorAgentAuthorityController.java` |
| `/api/v1/sensor/capability-packs` | `core/src/main/java/com/platform/core/controller/SensorCapabilityPackController.java` |
| `/api/v1/sensor/control` | `core/src/main/java/com/platform/core/controller/SensorControlDesiredStateController.java` |
| `/api/v1` | `core/src/main/java/com/platform/core/controller/SensorIngestionController.java` |
| `/api/v1` | `core/src/main/java/com/platform/core/controller/VersionController.java` |
| `/api/v1` | `core/src/main/java/com/platform/core/ingestion/agentingress/SensorAgentIngestionController.java` |

| Surface | Schema/configuration/physical authority | Evidence boundary |
|---|---|---|
| Agent↔Sensor and Sensor↔Core telemetry, ACK, error, heartbeat and desired/applied control | `schemas/edge/` and `schemas/edge/tools/validate.mjs`; exact runtime validators/DTOs in `core/src/main/java/com/platform/core/ingestion/` and `core/src/main/java/com/platform/core/controller/`; Agent/Sensor boundaries in `agent/common/client/https.go` and `sensor/internal/agentreceiver/validator.go` | §10 reconciles current behavior. Schema fixture validation alone does not prove runtime durability. |
| Internal numeric/exact-counter telemetry publication | `core/src/main/resources/application.yml`: `platform.ingestion.kafka.topic`, environment `TELEMETRY_NUMERIC_TOPIC`, default `telemetry.numeric.v1`; `core/src/main/java/com/platform/core/ingestion/KafkaTelemetryProducer.java` | Current producer serializes canonical observations as JSON within a Kafka transaction. Apicurio/Avro/Protobuf remain approved target technologies; new physical families are BUILD-063 decisions. |
| PostgreSQL control and replay schema | `core/src/main/resources/db/migrations/manifest.txt` and the adjacent listed `V001`–`V018` SQL files | Exact ordered migration authority; logical table names elsewhere are not substitute DDL. All applied migrations remain immutable; supported-origin upgrades require evidence. |
| Current ClickHouse initialization and writer/reader contracts | `deploy/docker/clickhouse/init.sql`; `core/src/main/java/com/platform/core/ingestion/ClickHouseClient.java` | Existing initialization/source contract; not a certification of all retention, rollup, HA or upgrade scenarios. Future process-network/path DDL remains BUILD-064-owned. |
| Presentation definitions | `schemas/presentation/` and `schemas/presentation/tools/validate.mjs` | Definition/fixture contract; does not prove backend query or UI integration. |
| Provisioning import/readiness/connectivity definitions | `schemas/provisioning/` and `schemas/provisioning/tools/validate.mjs` | Definition/fixture contract; does not prove complete enrollment/install flow. |
| Runtime supportability definitions | `schemas/supportability/` and `schemas/supportability/tools/validate.mjs` | Health/log/evidence shape; does not certify every deployment probe or recovery path. |

Relevant source tests include `core/src/test/java/com/platform/core/ingestion/agentingress/AgentIngestionServiceTest.java`, `AgentIngestionValidatorTest.java` in the same directory, `core/src/test/java/com/platform/core/controller/SensorControlDesiredStateControllerTest.java` and `core/src/test/java/com/platform/core/security/http/SecurityDirectoryConfigurationTest.java`. Their existence is not a new passing-test claim. Exact-head execution records and active compatibility/upgrade matrices remain required before certification.

Shared six-area acceptance and unresolved-gate disposition remain `Platform_00` §10.1, `Platform_08` §4.1–4.3 and `Platform_12` §29; this artifact map is supporting evidence rather than a competing acceptance checklist. For a capability to authorize implementation, its existing 83-field contract (`Platform_00` §8 / `AGENTS.md`) MUST bind required UI/API/event/data objects, authority/failure semantics, physical artifacts where selected, version/upgrade behavior and test/BUILD owners. A not-yet-selected essential physical contract stays an explicit technical gate under its existing owner; logical examples must not be copied into a fabricated migration or advertised route.

### 4.8 Storage assurance contract binding — accepted Issue #292

Capacity/retention assessment is a logical extension of existing Storage Lifecycle, Platform Operations and Notification Center/ITSM/email ownership. Exact arithmetic, physical-volume provenance, uncertainty/`OBSERVED`/`ESTIMATED`/`UNKNOWN`/`PROJECTED`, protected/reclaimable accounting, version-bound simulation/activation, stale-preview/concurrent-change rejection, integer-byte API authority (safe JSON exact-integer encoding where Number is unsafe), GB/TB versus GiB/TiB display semantics and delivery lifecycle are normative in `Platform_11` §9 and `Platform_05` §37.6–37.7/§38. This document allocates no new route, schema file, migration number or service; the responsible BUILD-061/064/074/086/094/097/130 contract gates must bind concrete API/schema/test artifacts before implementation claims.

## 5. Display timezone and calendar-query contracts — v3.2.2 handoff baseline

Authoritative source timestamps in public/internal APIs, Kafka events and datastore facts remain absolute UTC-capable RFC3339/ISO-8601 instants. Clients MUST NOT treat a rendered wall-clock string as source truth.

Where calendar bucketing depends on local day/week/month/business-day boundaries, query requests MAY accept an explicit IANA timezone identifier. Responses that return local-calendar buckets MUST include:

- bucket start/end as absolute instants;
- the IANA timezone used for grouping;
- enough metadata that a second viewer can reproduce the same buckets.

Logical user preference field `display_timezone` (nullable IANA identifier) is an implementation requirement on the existing user preference/profile surface. This documentation baseline does **not** allocate a PostgreSQL migration number. Existing `tenant.timezone` remains the tenant fallback. Host OS timezone is non-authoritative.

## 6. Counter64 / exact-counter implementation truth — current main after PR #84 + PR #304 (BUILD-042)

Architecture requires exact Counter64 end-to-end. Current merged implementation truth:

- Sensor preserves Counter64 as an exact unsigned decimal source value;
- Core canonicalization uses a dedicated exact counter representation separate from the ordinary floating-point numeric observation path;
- accepted wire form is a validated decimal string and non-canonical/ambiguous representations fail closed;
- Kafka serializes raw Counter64 without JSON/JavaScript Number semantics;
- ClickHouse persists raw exact values in a dedicated `UInt64` path and exposes replay-safe canonical reads;
- tests cover values around `2^53`, the Java signed-long boundary and through `18446744073709551615` (`2^64-1`), plus restart/replay/idempotent migration behavior;
- therefore the **raw exact Counter64 transport/persistence path is IMPLEMENTED and TESTED** (PR #84), but remains **NOT BENCHMARKED / NOT CERTIFIED / NOT DEPLOYED**.

After PR #304 (`main` `e207be47372236aadade3d452167bc03f4e5025e`), the BUILD-042 **exact Counter32/Counter64 delta/rate/reset/wrap/discontinuity leaf engine** is implemented and merged under `sensor/internal/protocol/snmp/counter/` (identity-complete derivation, continuity-aware wrap/reset, elapsed-time rate). Bounded source-truth status:

`PARTIAL — exact raw Counter64 + counter derivation leaf implemented; integration/benchmark/certification pending`

Non-claims (mandatory):

- leaf engine is **not** yet complete Network Sensor integration;
- no whole BUILD-042 certification;
- no BUILD-048 Network Sensor certification;
- no deployment claim.

The ordinary numeric measurement path may remain floating-point because it is semantically separate from the authoritative raw counter family. Derived delta/rate/utilization may use floating-point only AFTER exact delta/reset/discontinuity processing. A decrease is not automatically a wrap; discontinuity/reboot/source evidence is evaluated first.

## 7. Telemetry durability / ACK contract note — PR #81 canonical

Canonical Sensor→Core telemetry durability remains:

```text
validate complete source batch
→ PostgreSQL durable pending reservation
→ claim bounded PostgreSQL publisher lease (owner/epoch)
→ Kafka transactional publication and commit
→ PostgreSQL finalize accepted watermark for matching reservation/owner/epoch
→ Sensor ACK
```

Semantics are **at-least-once**, not exactly-once. Duplicate Kafka publication of the same stable logical observation identity is tolerated; canonical ClickHouse reads must not expose duplicate logical numeric observations. **Current source reconciliation (`36e1513f`):** `SensorIngestionService` and `AgentIngestionService` now claim a bounded PostgreSQL publisher lease with owner/epoch after reservation, publish transactionally and require the matching fence for finalization (V012). This bounds concurrent publisher ownership; it does not make Kafka/PostgreSQL one XA transaction, eliminate the commit/finalize replay window or solve HA fencing. Both Core ingress paths use reservation/lease/commit/finalize before ACK. When another publisher already holds the exact pending-batch lease, Core returns HTTP **429 Too Many Requests** with stable code `ERR_BATCH_PUBLICATION_IN_PROGRESS` (see `PublisherClaimResult.busy()`). Clients MUST retry with bounded exponential backoff + jitter and MUST NOT treat 429 as an ACK or as a permanent conflict; `Retry-After` MAY be supplied when implemented and MUST be honoured when present. Physical consumer/read deduplication and durability certification remain separate gates.

**2026-10-30 transport audit contract clarification (additive, no v1 wire break):** For authenticated Sensor→Core telemetry ingress, capacity/congestion status (including HTTP 429/503) without a verified durable application ACK MUST NOT authorize spool deletion. When present, `Retry-After` MUST be standards-compatible (delta-seconds or HTTP-date), capped by existing Sensor retry policy, and ignored when absent/invalid with fallback to bounded exponential backoff/jitter. A prior `ALREADY_ACKED` exact replay may reuse the durable ACK without Kafka re-publication, but **post-Kafka-commit / pre-PostgreSQL-finalize** failure or publisher-lease expiry can require physical republish under `RETRY_PENDING`; downstream committed-only consumption and effect-level idempotency remain mandatory. For Phase-1.5 HA, stable endpoint/mTLS and versioned `active_epoch`/session are complementary to a **separately certified exclusive authority and persistence fencing mechanism**. Epoch/session metadata alone cannot fence storage writes or satisfy `ACKED-FOR-DELETION TELEMETRY RPO = 0`.

**Deletion-authorizing ACK durability (Issue #200 — shared contract shape, envelope-specific matrices):**

1. **Phase 1 standalone:** If Core issues an ACK authorizing Sensor durable-spool deletion, that telemetry must survive the certified standalone failure matrix (application/process/service crash/restart, governed host reboot and other explicitly certified recoverable failures). Catastrophic destruction/loss of the single Core/storage is **not** Phase-1 zero-RPO HA; recovery uses backup/restore and the declared Phase-1 DR profile.
2. **Phase 1.5 HA:** If Core issues an ACK authorizing Sensor durable-spool deletion, that telemetry must survive every failure in the certified permanent single-Core HA failure matrix (`ACKED-FOR-DELETION TELEMETRY RPO = 0` for those HA scenarios; not site/DR RPO).

Conceptual lifecycle: Sensor durable spool → Core acceptance → certified durability barrier for the active envelope (standalone matrix or HA matrix) → deletion-authorizing ACK → Sensor may delete acknowledged segment. Physical HA mechanisms remain unselected; closure belongs to `HA_DURABILITY_DESIGN_GATE` / BUILD-120–124. Tenant-aware identity on the ACK path remains mandatory in Phase 1; MSP/shared-tenant adversarial isolation certification remains Phase 1.5.

### 7.1 Agent-ingress pending-reservation and pre-publication rejection safety

**Authority.** This subsection is the authoritative normative contract for Agent-ingress (and the shared Sensor→Core ingress durability shape where the same reserve/publish/finalize machinery applies). LLD/Dataflow/Build Register bind and reference it; they MUST NOT restate a competing complete contract.

**FROZEN invariant.** A deterministic pre-publication non-terminal rejection MUST NOT leave an unpublished durable pending reservation blocking continuity.

Observed production defect class (documentation prohibition; not an implementation claim of current main closure): reserving before mutable assignment validation, then rejecting with `ERR_AGENT_ASSIGNMENT_UNKNOWN` (or equivalent stale/unknown/revoked/fenced assignment authority), can leave `pending_first_sequence` set while `settled_through_sequence` / accepted continuity remains behind that pending range — permanently wedging the Agent-ingress stream. Mother Reference wording MUST prohibit that outcome.

#### 7.1.1 Disposition classes

For Agent-ingress durable disposition semantics:

| Disposition | Meaning | Continuity effect |
|---|---|---|
| `ACCEPTED` | Exact range/checksum was durably accepted after successful publication/finalize (or exact prior ACCEPTED replay) | Accepted watermark may advance; Sensor deletion-authorizing ACK only after verified durable acceptance |
| `TERMINALLY_REJECTED` | Exact range/checksum was durably settled as a governed terminal rejection | Settled continuity advances; accepted watermark MUST NOT falsely advance; no customer telemetry publication |
| Non-terminal deterministic rejection | Pre-publication semantic/authority rejection that is not a governed terminal disposition | No ACCEPTED; no TERMINALLY_REJECTED unless explicitly governed; no watermark advance; no publication; no lingering unpublished pending reservation after safe rejection completion |
| Retryable failure | Infrastructure/publication/contention/crash ambiguity | MAY retain pending/publisher state required for at-least-once retry; MUST NOT authorize Sensor deletion |

Current governed terminal rejection case remains `ERR_AGENT_IDENTITY_CONFLICT` only. Do not generalize every validation error into terminal settlement.

#### 7.1.2 Normative processing order (historical replay preserved)

Logical ordering for authenticated Agent-ingress admission:

1. Authenticate Sensor.
2. Derive server-authoritative tenant and Sensor identity.
3. Validate immutable request structure, stream identity, sequence range and checksum/body identity.
4. Perform exact durable prior-disposition / replay lookup for the request identity.
5. If an exact previous `ACCEPTED` or `TERMINALLY_REJECTED` disposition exists, return that exact durable result. Current mutable state MUST NOT rewrite historical durable outcomes.
6. For genuinely new/unsettled work only, perform mutable assignment / entity / binding validation (including current assignment generation, Sensor binding, revocation/fencing and entity resolution). New reservations begin `UNVALIDATED`; only after this full exact-request validation **and** atomic `PendingCanonicalBindingV1` persistence may the durable authority checkpoint become `AUTHORITY_VALIDATED` (§7.1.10). Historical `UNVALIDATED` / `AUTHORITY_VALIDATED` / `LEGACY_UNKNOWN` recovery paths follow §7.1.11–§7.1.13 (qualified `SensorAcceptanceEvidenceV1` + reproducible canonical identity) and MUST NOT invent a reusable Agent-wide grant.
7. Proceed to publication and finalize (`ACCEPTED`), governed terminal settlement (`TERMINALLY_REJECTED` where explicitly authorized), or deterministic non-terminal rejection with pending-reservation safety below. Publisher claim is permitted only when exact pending authority state is `AUTHORITY_VALIDATED` (never while `UNVALIDATED` or `LEGACY_UNKNOWN`).

Do **not** move historical disposition lookup behind mutable current-identity validation. Cross-tenant or unauthorized callers MUST fail before they can learn disposition state.

#### 7.1.3 Stale / unknown / revoked / fenced assignment authority

When mutable assignment authority is stale, unknown, revoked or fenced for a genuinely new/unsettled request, Core MUST produce a deterministic pre-publication non-terminal rejection unless a separately governed terminal disposition applies. After that rejection has safely completed:

- no `ACCEPTED` disposition;
- no `TERMINALLY_REJECTED` disposition merely because assignment authority failed;
- no accepted-watermark advance;
- no settled-watermark advance;
- no Kafka publication;
- no customer telemetry publication;
- no lingering publisher lease;
- no lingering unpublished durable pending reservation.

A corrected/current authorized retry of the **same unsettled sequence** remains eligible. The rejection MUST NOT create a sequence-gap wedge that blocks later valid authority for that sequence.

#### 7.1.4 Pending reservation safety and conditional release

If an implementation reserves before mutable semantic validation, then every deterministic pre-publication rejection that occurs after reservation MUST conditionally release that unpublished pending reservation before returning a definitive non-terminal rejection.

Pending release is allowed **only** when the authoritative database proves the pending reservation still belongs to this exact request and is unpublished/unfinalized. The conditional identity MUST include the applicable fields among:

- tenant;
- Sensor;
- stream;
- first/last sequence;
- checksum;
- batch identity;
- request digest / correlation identity where persisted;
- reservation / publisher state (unpublished, not publisher-owned for an in-flight publish, not finalized/settled).

A release MUST NOT remove:

- another request's pending reservation;
- another batch;
- accepted / finalized state;
- terminally settled state;
- a publisher-owned reservation;
- a reservation where Kafka publication may already have occurred.

Use transactional / row-lock semantics as required by the implementation. Do not prescribe an unnecessary new service. Prefer validating mutable authority before reservation when that preserves historical replay order above; conditional release is the mandatory safety valve when reserve-before-mutable-validation remains in the implementation path.

#### 7.1.5 Retryable infrastructure / publication failure (distinct)

Retryable failures include Kafka unavailable, temporary database failure, process crash, publisher-lease contention (`ERR_BATCH_PUBLICATION_IN_PROGRESS` / HTTP 429) and post-commit / pre-finalize ambiguity. A retryable failure MAY retain pending state where required for at-least-once semantics. Do **not** require pending deletion for all failures. Retryable failure MUST NOT authorize Sensor spool deletion.

#### 7.1.6 Cleanup failure

If a deterministic rejection requires pending release but the system cannot prove safe release succeeded:

- the operation becomes retryable;
- no false definitive rejection/ACK may authorize Sensor deletion;
- Sensor retains custody of the unsettled segment;
- no accepted or settled watermark advance;
- state remains observable and reconcilable.

#### 7.1.7 Terminal disposition remains distinct — `ERR_AGENT_IDENTITY_CONFLICT`

`ERR_AGENT_IDENTITY_CONFLICT` remains the governed terminal rejection case for Agent-ingress identity conflict. Its semantics remain:

- immutable `TERMINALLY_REJECTED` disposition for the exact request identity;
- settled continuity advances;
- accepted watermark does not falsely advance;
- no customer telemetry publication;
- durable typed response;
- Sensor durable quarantine of the terminally rejected segment;
- active-spool deletion only after exact disposition verification.

Do not treat stale/unknown assignment, ordinary contract validation failures or retryable infrastructure errors as terminal settlement by implication.

#### 7.1.8 Mandatory acceptance tests / fixtures

Applicable Agent / shared-ingress BUILD owners (principally BUILD-062 ingestion gateway with BUILD-024 Sensor Agent-gateway custody; additive to historical BUILD-003 transport certification without rewriting that certified record) MUST provide fixtures proving:

1. **Stale assignment rejection.** Submit sequence `N` under stale/unknown/fenced assignment authority. Prove deterministic non-terminal rejection; accepted watermark unchanged; settled watermark unchanged; no terminal disposition; no Kafka/customer telemetry; `pending_first_sequence IS NULL`; no active publisher lease.
2. **Corrected authority, same sequence.** Then submit the same `N` under current valid authority. Prove normal reservation; no sequence-gap wedge; normal `ACCEPTED` succeeds.
3. **Repeated stale request.** Repeated stale rejection does not create a pending wedge.
4. **Cleanup race.** Conditional cleanup cannot delete publisher-owned, finalized or settled state.
5. **Cleanup failure.** Cleanup failure remains retryable, preserves Sensor custody and advances neither accepted nor settled watermarks.
6. **Crash after mutable validation failure, before pending release.** Core crashes after a deterministic mutable-validation failure has been decided but before conditional unpublished pending release commits. Prove: no deletion-authorizing response exists so Sensor retains custody; exact retry after Core restart reconciles/releases that exact unpublished pending reservation; corrected authority may then use the same unsettled sequence; no Kafka/customer duplicate and no fabricated watermark advance.
7. **Authority checkpoint negatives.** Changed body/checksum/tenant/Sensor/stream cannot consume a checkpoint; `AUTHORITY_VALIDATED` cannot be created before validation; publisher cannot claim `UNVALIDATED`/`LEGACY_UNKNOWN`; checkpoint cannot authorize new sequences; raw `acceptedAt` alone is insufficient historical authority; canonical binding snapshot is required before `AUTHORITY_VALIDATED` publication.
8. **J7 publication-uncertain + later authority/binding change.** Prove §7.1.14: Entity A snapshot survives later Entity B binding change and optional revocation; physical Kafka at-least-once republish with once-only canonical customer-visible Entity A observation; changed body cannot consume Entity A snapshot.
9. **J8_UNVALIDATED_CRASH_THEN_REVOCATION.** Prove §7.1.11.5–§7.1.11.6: UNVALIDATED reservation + Core crash + later assignment revocation → qualified historical acceptance recovers to `AUTHORITY_VALIDATED` then publish/finalize; unqualified/uncertain acceptance → no publication, no ACK, no permanent DB wedge, conditional exact release permitted, Sensor retains custody.

These tests are documentation-acceptance obligations for implementation closure; their presence here does not claim current main already passes them. Fixture 6 is a required BUILD-062 failure fixture. Fixtures 7–9, J8 and U1–U7 (§7.1.15) remain `AUTHORITY_CHECKPOINT_RECOVERY = SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED` until runtime evidence exists.

#### 7.1.9 Schema migration / release-guard upgrade precondition

**Authority.** This subsection freezes upgrade/schema-migration safety for Agent-ingress pending-release semantics. Runtime pending/replay, authority-checkpoint, and cutover-fence admission behavior remain BUILD-062; upgrade/canary/rollback acceptance of the race-safe cutover (active-writer drain + dormant `LEGACY_UNKNOWN` preserve) remains BUILD-127. Operations sequence lives in `Platform_16` §5.1; application-flow transitions live in `Platform_06` §119.21.

```text
AUTHORITY_CHECKPOINT_RECOVERY = SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED
SENSOR_ACCEPTANCE_EVIDENCE_V1 = SPECIFIED / PENDING_IMPLEMENTATION
PENDING_CANONICAL_BINDING_V1 = SPECIFIED / PENDING_IMPLEMENTATION
```

Current canonical `main` does **NOT** implement authority-checkpoint recovery, `SensorAcceptanceEvidenceV1`, or `PendingCanonicalBindingV1`. Docs must not claim runtime/test completion until a runtime candidate merges and independent evidence exists.

**SUPERSEDED contract — zero-pending as mandatory migration blocker.** The prior contract that all dormant Agent-ingress pending rows must be zero before a V021-like schema evolution is **superseded**. Reason: a legacy pending request may be impossible to drain through the old runtime after mutable authority changes, causing one Sensor stream to block the entire Core schema upgrade indefinitely.

**New rule (frozen):**

- active publisher/writer ownership MUST drain before schema migration;
- dormant legacy pending reservations MAY cross the schema migration;
- migration preserves them explicitly as `LEGACY_UNKNOWN` (authority) with publication history `UNKNOWN`;
- no accepted/settled watermark is advanced;
- no pending row is deleted;
- publication history is not guessed (`TRUE`/`FALSE` fabrication is forbidden).

**Root problem (retained).** A schema migration cannot safely infer whether a pre-migration pending Agent-ingress batch:

- was never published;
- was publisher-claimed;
- was published to Kafka but not finalized;
- is merely a stale-assignment orphan.

Therefore an upgrade MUST NOT automatically clear or invent terminal settlement for ambiguous historical pending state. Historical pending publication state is **not inferable** from schema alone. Migration MUST NOT label an unknown pending row as safe-to-release or as never-published. Migration MUST mark unknown publication history `UNKNOWN` and unknown authority checkpoint `LEGACY_UNKNOWN`.

**Race class (forbidden).** A sequence that only (1) quiesces/drains, (2) observes active writers drained, (3) leaves a window where an old-version exact retry creates a new reservation or publisher claim, then (4) commits migration, is forbidden. Apparent quiescence before hard fence is not a migration authorization. The earlier zero-pending race wording remains historically informative; the superseding fence still forbids check-then-unlock-then-migrate gaps against concurrent writers.

**FROZEN two-phase cutover precondition.** Any Agent-ingress schema migration that introduces or changes pending-release / pending-replay / authority-checkpoint semantics (a release-guard schema transition; cross-reference the current V021-equivalent Flyway migration when present in the catalogue, without treating that filename as eternal product terminology) MUST apply the race-safe two-phase protocol:

**STAGE 1 — DRAIN MODE** under the pre-upgrade Core:

- reject creation of NEW Agent-ingress work/reservations;
- permit only bounded reconciliation/retry necessary to resolve ALREADY-EXISTING pending reservations that the old runtime can still settle safely;
- allow existing publisher/finalize/release work to complete;
- continue until active publisher/writer ownership approaches zero;
- dormant legacy pending that cannot safely settle under old mutable authority MAY remain;
- a retry that would create a brand-new reservation MUST NOT be admitted;
- if the current/previous GA pair cannot enforce drain mode, that upgrade origin is NOT certified and requires a compatible intermediate release.

**STAGE 2 — HARD CUTOVER FENCE** before the final active-writer / publisher-lease check:

##### 7.1.9.1 Global fence authority — `AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1`

Use **one** PostgreSQL transaction-scoped advisory lock namespace dedicated to Agent-ingress schema cutover.

Stable symbolic identity (frozen):

```text
AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1
```

Do **not** freeze a numeric lock value in documentation unless existing source already owns one. Numeric/hash encoding remains an implementation detail, but every participant MUST derive the **same** lock identity from this symbolic name.

This lock is distinct from, and complementary to:

- per-Sensor ingress serialization locks;
- ordinary migration-runner serialization;
- publisher lease ownership.

A per-Sensor lock alone does **not** fence all Agent-ingress writers.

```text
AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1 = SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED
```

This global acquisition protocol is now **SPECIFIED**. Current canonical `main` does **NOT** yet implement it. Docs must not claim runtime closure until source/tests implement it.

##### 7.1.9.2 Required participants

Every transaction capable of creating or mutating Agent-ingress durable state MUST participate in the global cutover protocol. At minimum:

1. new reservation creation;
2. exact pending retry/reservation reconciliation;
3. publisher lease claim;
4. publisher lease release;
5. accepted finalization;
6. conditional pending release;
7. terminal disposition creation;
8. migration/cutover transaction.

##### 7.1.9.3 Normal runtime lock order

For ordinary Agent-ingress state-changing transactions:

1. acquire `AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1` in **SHARED** mode;
2. acquire existing per-Sensor advisory/row locks as currently required;
3. perform reservation/claim/finalize/release/terminal operation;
4. commit/rollback;
5. transaction automatically releases locks.

Lock order is therefore always:

```text
GLOBAL CUTOVER SHARED → PER-SENSOR / ROW
```

Never reverse this order. This avoids lock-order inversion.

##### 7.1.9.4 HARD CUTOVER order

The migration/cutover path:

1. enter product-level HARD CUTOVER state so HTTP admission rejects all new Agent-ingress requests retryably;
2. wait for existing request handlers to drain;
3. acquire `AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1` in **EXCLUSIVE** mode;
4. exclusive acquisition waits until all transactions holding SHARED mode have completed;
5. after EXCLUSIVE is acquired: no new state-changing Agent-ingress transaction can enter;
6. inside the same database transaction / exclusive fenced interval:
   - verify active publisher ownership/leases = 0;
   - verify no in-flight Agent-ingress writers / unresolved active publication-attempt ownership;
   - inventory dormant legacy pending reservations (may be non-zero);
   - verify migration preconditions (including request-identity support for supported upgrade origins);
7. apply migration, preserving each dormant pending row as authority=`LEGACY_UNKNOWN` and publication history=`UNKNOWN` without deleting rows or advancing watermarks;
8. commit schema transaction;
9. release EXCLUSIVE lock only via transaction completion;
10. start/activate upgraded runtime;
11. exit HARD CUTOVER state.

There MUST be no release between final active-writer/publisher check and migration commit. The final active-ownership check and migration MUST share one uninterrupted fenced cutover interval. There must be no check-then-unlock-then-migrate gap. The previously frozen phrase “final zero-state check” for pending rows is superseded for dormant legacy pending; it remains conceptually applicable only to active publisher/writer ownership (must be zero).

##### 7.1.9.5 HTTP admission fence

During HARD CUTOVER:

all Agent-ingress HTTP requests, including exact retries, fail **before** any durable reservation/disposition lookup or mutation with retryable:

```text
ERR_PLATFORM_UPGRADE_IN_PROGRESS
```

(or repository-canonical equivalent). Agent/Sensor retains custody. MUST create no reservation, no terminal disposition, no deletion authorization, and no watermark change.

Response:

- does not ACK deletion;
- does not create reservation;
- does not create terminal disposition;
- does not advance accepted/settled watermark;
- Sensor retains custody.

##### 7.1.9.6 Old-version participation (supported-origin preflight)

A direct supported upgrade origin is allowed **ONLY** when **both**:

1. the pre-upgrade Core version already implements `AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1` participation; and
2. the pre-upgrade version/schema already contains exact request body digest / request identity and enough legacy pending identity (tenant/Sensor/stream/sequence/checksum/batch/reservation) for safe post-upgrade historical replay under §7.1.10–§7.1.13.

If the installed old version does **NOT** participate in the shared global fence, or lacks exact request identity for safe post-upgrade replay:

```text
PROFILE_BLOCKED / UNSUPPORTED_DIRECT_SCHEMA_CUTOVER
```

That origin requires a certified intermediate release that introduces the shared-lock protocol and/or request-identity pending fields first. Do not pretend a new migration lock can fence binaries that never acquire it. Do not claim safe migration from arbitrary historical schemas lacking request identity. Supported-origin/preflight semantics are owned by `Platform_16` §5 / §5.1.

##### 7.1.9.7 Phase 1.5 / two-Core participation

For Phase 1.5:

- both Cores must honor the same PostgreSQL global cutover lock identity;
- no Core may remain capable of Agent-ingress mutation while the other migrates;
- standby-first application/package sequencing does **not** override shared database fencing;
- active/standby authority/fencing must first ensure only the governed upgrade actor enters hard cutover;
- database migration is single-authority;
- migration EXCLUSIVE lock fences state-changing Agent-ingress transactions from either Core.

Do **not** introduce a third witness/coordinator.

##### 7.1.9.8 Abort

**Fail-closed migration behavior.**

- If the active-writer / active-publisher-lease drain precondition is violated, migration MUST abort; no partial-apply is valid.
- Dormant legacy pending alone MUST NOT abort migration when active ownership is drained and the migration can preserve those rows as `LEGACY_UNKNOWN` / publication `UNKNOWN`.
- Migration apply MUST be transactional such that failure leaves the prior schema and data authoritative.

If active publisher/writer ownership remains; migration validation fails; exclusive cutover lock cannot be acquired within bounded operation deadline; or the origin lacks required request identity for supported cutover:

- migration does not commit;
- old schema remains authoritative;
- cutover lock is released by rollback;
- operator returns to compatible old-version recovery/drain flow;
- no manual DB deletion/reset.

No guessed publication state, fabricated Kafka/customer duplicate, or fabricated watermark advance is permitted. Operator manual pending-row deletion to bypass the gate is prohibited (`Platform_16` §5.1). Upgrade may resume after active ownership is drained (dormant legacy pending may remain for post-upgrade `LEGACY_UNKNOWN` recovery), or after a separately governed recovery procedure under BUILD-127.

Finite acceptance owned across BUILD-062 (runtime fence/admission + SHARED participation + authority-checkpoint recovery) and BUILD-127 (upgrade/cutover certification + EXCLUSIVE fence + U1–U7):

1. U1 clean migration (no pending / no active leases) under hard fence → succeeds.
2. U2 dormant legacy pending migration → succeeds; rows preserved as `LEGACY_UNKNOWN` / publication `UNKNOWN`; watermarks unchanged; no row deleted.
3. U3 multi-tenant legacy pending → same preserve semantics; no cross-tenant bleed.
4. U4 active publisher blocks migration → abort with old schema/data intact.
5. U5A `LEGACY_UNKNOWN` + qualified historical authority + reproducible canonical identity after upgrade → `LEGACY_UNKNOWN → AUTHORITY_VALIDATED →` publisher claim → publish/re-publish → finalize → ACK.
6. U5B historically valid authority but canonical mapping not reproducible → `LEGACY_PENDING_RECONCILIATION_REQUIRED`; no ACK/republish/pending deletion/terminal settlement; only affected stream blocked.
7. U6 insufficient historical proof → remains `LEGACY_PENDING_RECONCILIATION_REQUIRED`; no ACK/release/terminal settlement; only affected stream blocked; schema upgrade itself not globally blocked.
8. U7 accepted/settled history unchanged across migration.
9. Manual pending deletion/reset is **not** accepted remediation.
10. Failure is observable/audited (stable error code, correlation ID, pending/lease diagnostic evidence without secrets).

The superseded historical fixtures “V020-equivalent zero-pending → migration succeeds” and “One pre-existing pending row → migration aborts” are replaced by U1–U4 above. Do not reinstate zero-pending-as-mandatory-migration-blocker.

Mandatory race fixtures (BUILD-127 acceptance; BUILD-062 runtime proof points):

**CUTOVER-RACE-1 — exact retry at cutover transition.** Setup: preflight/drain reaches apparent active-ownership drained; exact retry races at the transition into hard cutover. Prove either (A) retry enters before hard fence and becomes visible active pending/writer work, causing final fenced active-ownership check to fail and migration to abort; OR (B) hard fence wins first and retry receives retryable `ERR_PLATFORM_UPGRADE_IN_PROGRESS` without creating a reservation. Forbidden: migration commits while the retry creates or owns Agent-ingress pending state as an active writer.

**CUTOVER-RACE-2 — in-flight writer.** An Agent-ingress writer is already active when hard fence begins. Prove: cutover waits for it to terminate/commit/rollback; final active-ownership check occurs afterward; migration does not race the writer.

**CUTOVER-RACE-3 — publisher lease.** Publisher lease exists at cutover. Migration must not proceed.

**CUTOVER-RACE-4 — post-commit resume.** After migration commit and fence release: exact retained Sensor request retries normally under the upgraded runtime (including `LEGACY_UNKNOWN` historical recovery where applicable); no duplicate deletion-authorizing ACK or false sequence gap for customer-visible canonical telemetry.

Mandatory global-lock fixtures (SPECIFIED / NOT YET EXECUTED; BUILD-062 runtime SHARED participation + BUILD-127 EXCLUSIVE cutover certification):

**CUTOVER-LOCK-1:** ordinary reservation holds SHARED; migration EXCLUSIVE waits.

**CUTOVER-LOCK-2:** migration EXCLUSIVE held; new reservation cannot acquire SHARED and HTTP admission remains upgrade-in-progress.

**CUTOVER-LOCK-3:** publisher claim/finalize transaction drains before EXCLUSIVE acquisition succeeds.

**CUTOVER-LOCK-4:** conditional release participates in SHARED lock and cannot race migration.

**CUTOVER-LOCK-5:** terminal disposition transaction participates in SHARED lock.

**CUTOVER-LOCK-6:** lock-order test proves GLOBAL-before-PER-SENSOR and no inversion.

**CUTOVER-LOCK-7:** old-version binary without global-lock support is rejected as unsupported direct upgrade origin (`PROFILE_BLOCKED / UNSUPPORTED_DIRECT_SCHEMA_CUTOVER`).

**CUTOVER-LOCK-8:** two-Core profile proves neither Core can mutate Agent-ingress while EXCLUSIVE migration fence is held.

**CUTOVER-LOCK-9:** migration failure rolls back schema and releases EXCLUSIVE lock.

**CUTOVER-LOCK-10:** post-commit upgraded retry proceeds normally.

#### 7.1.10 Durable authority checkpoint

Freeze explicit pending authority state (exact field names may follow implementation conventions; persisted semantics are mandatory):

| State | Meaning |
|---|---|
| `NONE` | No pending authority checkpoint is present |
| `UNVALIDATED` | New reservation created; request authority/entity validation not yet completed |
| `AUTHORITY_VALIDATED` | Exact-request authority/entity validation completed **and** `PendingCanonicalBindingV1` snapshot committed; publisher claim is eligible |
| `LEGACY_UNKNOWN` | Pre-checkpoint legacy pending preserved across schema migration; historical validation **and** reproducible canonical identity required before claim |

**New reservations** MUST start as `UNVALIDATED`.

After full request authority validation, canonical entity resolution, and atomic `PendingCanonicalBindingV1` persistence — and **before** publisher claim — state MUST become `AUTHORITY_VALIDATED`.

Publisher claim MUST NOT occur unless exact pending state is `AUTHORITY_VALIDATED`. Publisher claim before `AUTHORITY_VALIDATED` is forbidden.

##### 7.1.10.1 Exact-request scope

The authority checkpoint is bound to the exact:

- tenant;
- authenticated Sensor;
- stream;
- sequence range;
- checksum;
- batch/request identity;
- request body digest / request SHA-256;
- reservation identity.

It is **NOT**:

- an Agent-wide permission;
- a reusable assignment grant;
- authority for different telemetry;
- permission for future sequences;
- permission for another Sensor or tenant.

Security negatives (mandatory):

- changed body/checksum/tenant/Sensor/stream cannot consume the checkpoint;
- checkpoint cannot be created before validation (`AUTHORITY_VALIDATED` before validation is forbidden);
- publisher cannot claim `UNVALIDATED` or `LEGACY_UNKNOWN` state;
- authority checkpoint cannot authorize new sequences;
- canonical binding snapshot cannot be reused across batches/reservations.

##### 7.1.10.2 PendingCanonicalBindingV1

The exact request checkpoint binds request identity **and** the resolved canonical entity identity. Freeze typed `PendingCanonicalBindingV1`. Do **not** use a generic unbounded JSON authority blob.

Logical child-record contract (minimum):

- tenant ID;
- Sensor ID;
- reservation ID;
- Sensor ingress sequence;
- Agent ID;
- assignment generation;
- host ID / authoritative resolution key;
- resolved `entity_identifier_id`;
- resolved canonical `entity_id`;
- canonical-binding version;
- created/validated time.

Also persist exact `pending_canonical_batch_sha256` over the deterministic canonicalized batch projection.

The canonical snapshot belongs to the exact reservation only:

- exists only for that pending reservation;
- is protected by tenant/Sensor/reservation identity;
- cannot be reused across batches;
- clears atomically only when the pending reservation is successfully finalized/settled or through another explicitly governed safe terminal cleanup;
- MUST survive Core restart.

```text
PENDING_CANONICAL_BINDING_V1 = SPECIFIED / PENDING_IMPLEMENTATION
```

##### 7.1.10.3 AUTHORITY_VALIDATED atomic transaction invariant

The following is **one atomic PostgreSQL transaction**:

1. request authority validation succeeds;
2. canonical entity resolution succeeds;
3. canonical binding snapshot rows are written;
4. canonical batch digest (`pending_canonical_batch_sha256`) is written;
5. pending authority state becomes `AUTHORITY_VALIDATED`.

If any step fails, authority state remains `UNVALIDATED` and publisher claim remains impossible.

##### 7.1.10.4 Stable Agent observation identity (V1)

Freeze V1 logical observation identity components to the existing deterministic contract and ordering:

```text
tenant_id
authoritative_sensor_id
agent_id
assignment_generation
agent_record_sequence
observation_ordinal
metric_id
```

Representation may remain implementation-owned, but these identity inputs and their ordering are immutable for V1. Entity reassignment does **not** change the logical observation ID of an already validated exact request.

#### 7.1.11 Historical temporal authority

Telemetry durably accepted by the Sensor under a then-valid assignment remains recoverable after later reassignment/revocation.

Historical validation uses:

- tenant;
- `agentId`;
- `assignmentGeneration`;
- authoritative Sensor;
- qualified `SensorAcceptanceEvidenceV1` (not raw wall-clock alone);
- retained assignment created/revoked interval;
- reproducible historical canonical entity identity when transitioning to `AUTHORITY_VALIDATED`.

A `REVOKED` assignment generation may remain valid **historical** authority when qualified Sensor acceptance proves the complete acceptance uncertainty interval lies inside that assignment's authoritative interval.

Current `ACTIVE` state MUST NOT retroactively discard already-custodied telemetry.

Current `ACTIVE` assignment is **not** required when qualified historical evidence proves the exact request was already durably accepted during a valid historical assignment interval.

New telemetry after revocation remains prohibited.

##### 7.1.11.1 SensorAcceptanceEvidenceV1

Raw `acceptedAt = Sensor time.Now()` is **NOT** sufficient authority for retrospective revocation ordering.

Freeze typed `SensorAcceptanceEvidenceV1`, produced when the Sensor durably appends the Agent batch to its spool. The evidence is part of the durable Sensor spool record and exact request identity supplied to Core.

Minimum typed fields:

- `evidence_version = 1`;
- tenant ID;
- Sensor ID;
- Sensor incarnation / service-instance identity;
- Agent ID;
- assignment generation;
- Sensor ingress sequence/range;
- batch ID;
- exact batch/request digest;
- durable Sensor spool sequence;
- `accepted_at_utc`;
- `clock_quality`;
- `clock_max_error_ms`;
- `clock_sync_age_ms`;
- `clock_source`.

```text
SENSOR_ACCEPTANCE_EVIDENCE_V1 = SPECIFIED / PENDING_IMPLEMENTATION
```

##### 7.1.11.2 Phase-1 clock qualification

For V1, historical temporal authority requires `clock_quality = QUALIFIED` **and** typed clock-field validation below. Upper bounds alone are insufficient: unsigned/nonnegative semantics and a valid ordered uncertainty interval are mandatory.

###### Typed field validation (`SensorAcceptanceEvidenceV1`)

`clock_max_error_ms` MUST be:

- present when `clock_quality = QUALIFIED`;
- an integer;
- `0 <= clock_max_error_ms <= 5000`.

`clock_sync_age_ms` MUST be:

- present when `clock_quality = QUALIFIED`;
- an integer;
- `0 <= clock_sync_age_ms <= 300000`.

```text
clock_quality = QUALIFIED
0 <= clock_max_error_ms <= 5000
0 <= clock_sync_age_ms <= 300000
```

Reject (evidence becomes `UNQUALIFIED / INVALID`; historical authority MUST NOT be established):

- negative values;
- floating-point values;
- NaN/infinity representations;
- values above the bound;
- malformed numeric strings;
- integer overflow/underflow;
- null/missing values when `QUALIFIED`.

A value of exactly `0` is valid for either clock field when all other evidence is valid.

###### Acceptance timestamp

`accepted_at_utc` MUST:

- be present;
- parse as a valid UTC instant;
- use the existing canonical timestamp format (absolute UTC-capable RFC3339/ISO-8601);
- be representable for checked duration arithmetic.

Malformed or out-of-range timestamps fail validation.

The Sensor obtains clock quality from the host's disciplined system-clock state; unsynchronised/unknown clock state is `UNQUALIFIED`.

Do **not** silently substitute raw wall-clock time when clock qualification is missing.

###### Checked uncertainty arithmetic

Calculate using checked duration arithmetic:

```text
acceptance_lower = accepted_at_utc - clock_max_error_ms
acceptance_upper = accepted_at_utc + clock_max_error_ms
```

The implementation contract MUST explicitly require:

```text
acceptance_lower <= accepted_at_utc
accepted_at_utc <= acceptance_upper
acceptance_lower <= acceptance_upper
```

Arithmetic overflow or underflow MUST fail closed as invalid acceptance evidence. No wrapping, saturation or silent coercion.

###### Historical assignment ordering

Historical assignment interval is `[assignment.created_at, assignment.revoked_at)` for revoked assignments.

Historical acceptance is valid **only** when the **complete** acceptance uncertainty interval lies inside the authoritative assignment interval:

```text
acceptance_lower >= created_at
acceptance_upper < revoked_at
```

Exact equality `acceptance_upper == revoked_at` is **NOT** authorized.

If the uncertainty interval overlaps the revocation boundary, return `ERR_AGENT_ACCEPTANCE_ORDER_UNCERTAIN` (or repository-canonical equivalent).

For an assignment with no `revoked_at`, `acceptance_lower >= created_at` remains mandatory.

Boundary behavior is fail-closed. If the acceptance uncertainty interval overlaps `revoked_at`, authority ordering is `UNKNOWN`. Exact equality with the revocation boundary is **not** historically authorized.

###### Security invariant

Malformed clock evidence MUST NEVER become more permissive than missing clock evidence.

Any malformed, negative, overflowing or otherwise invalid clock field causes `SensorAcceptanceEvidenceV1` to be `UNQUALIFIED / INVALID` and MUST NOT establish historical authority. In particular this attack MUST fail closed:

- actual `accepted_at_utc` is after revocation;
- caller supplies negative `clock_max_error_ms`;
- calculated upper bound would otherwise move backward before revocation.

Result: historical authority DENIED. No transition to `AUTHORITY_VALIDATED`.

###### Required negative / boundary fixtures

Mandatory documentation/runtime acceptance fixtures (at least):

1. `clock_max_error_ms = -1` → reject.
2. `clock_sync_age_ms = -1` → reject.
3. `clock_max_error_ms = 5001` → reject.
4. `clock_sync_age_ms = 300001` → reject.
5. `clock_max_error_ms = 0` → valid where all other evidence is valid.
6. `clock_sync_age_ms = 0` → valid.
7. malformed non-integer clock error → reject.
8. malformed non-integer sync age → reject.
9. uncertainty interval exact upper boundary = `revoked_at` → not authorized.
10. interval overlaps `revoked_at` by 1 ms → not authorized.
11. `accepted_at` after `revoked_at` + negative error attack → reject before interval authority evaluation.
12. timestamp arithmetic overflow/underflow → reject.
13. valid interval wholly before `revoked_at` → historical authority remains valid.
14. `accepted_at` before assignment `created_at`, even with otherwise qualified clock → reject.
15. missing `QUALIFIED` clock fields → reject.

Additional negatives (mandatory):

- raw `acceptedAt` alone is insufficient;
- `clock_max_error_ms > 5000` fails qualification;
- `clock_sync_age_ms > 300000` fails qualification;
- uncertainty overlapping revocation fails historical authority.
##### 7.1.11.3 Legacy V020 raw acceptedAt rule

Legacy requests that contain only raw `acceptedAt` but do **not** contain qualified `SensorAcceptanceEvidenceV1` MUST NOT be automatically accepted merely because their raw timestamp appears earlier than `revoked_at`.

They require another authoritative historical proof source or become:

```text
LEGACY_PENDING_RECONCILIATION_REQUIRED
```

stream-locally. This does **NOT** block schema migration globally.

##### 7.1.11.4 UNVALIDATED exact retry

An exact pending reservation in `pending_authority_state = UNVALIDATED` MAY establish historical authority after restart/retry even when the assignment is now `REVOKED`, but **ONLY** through qualified historical acceptance evidence defined by `SensorAcceptanceEvidenceV1`.

Sequence:

1. authenticate current Sensor mTLS identity;
2. verify tenant/Sensor scope;
3. match exact reservation identity (tenant; Sensor; stream; sequence range; checksum; batch identity; request SHA-256; reservation ID);
4. validate `SensorAcceptanceEvidenceV1`;
5. validate the exact historical assignment generation;
6. validate historical canonical entity mapping;
7. atomically persist canonical identity snapshot (`PendingCanonicalBindingV1`);
8. transition `UNVALIDATED → AUTHORITY_VALIDATED`;
9. only after that transition may publisher claim occur.

This applies only to the exact already-custodied request. It MUST NOT authorize a different body, different checksum, different sequence, new telemetry, future batches, another Sensor, or another tenant.

##### 7.1.11.5 UNVALIDATED failure outcomes

If historical evidence clearly proves acceptance was after revocation, return a stable non-terminal authority rejection such as:

```text
ERR_AGENT_ASSIGNMENT_REVOKED_AT_ACCEPTANCE
```

Because `UNVALIDATED` guarantees publication has not started, the exact pending reservation may be conditionally released using the existing safe exact-release invariant. Sensor retains original spool custody.

If historical ordering cannot be proven, return:

```text
ERR_AGENT_ACCEPTANCE_ORDER_UNCERTAIN
```

The exact `UNVALIDATED` reservation may likewise be conditionally released because publisher claim is forbidden before `AUTHORITY_VALIDATED`.

In both failure outcomes:

- no terminal settlement;
- no ACK;
- no accepted/settled watermark advance;
- no permanent DB wedge.

BUILD-062 acceptance MUST include `J8_UNVALIDATED_CRASH_THEN_REVOCATION`:

- **Positive:** qualified historical acceptance → `AUTHORITY_VALIDATED` → publish/finalize.
- **Negative:** unqualified/uncertain acceptance → no publication, no ACK, no permanent DB wedge.

#### 7.1.12 AUTHORITY_VALIDATED retry

If exact pending request state is `AUTHORITY_VALIDATED`:

- exact retry does **not** require current `ACTIVE` assignment/binding as a continuity prerequisite;
- server Sensor/tenant authentication remains mandatory;
- exact body/reservation identity remains mandatory;
- use the persisted `PendingCanonicalBindingV1` snapshot — do **NOT** re-resolve entity identity against the current live entity binding;
- regenerate canonical observations from the exact body plus persisted canonical bindings;
- recompute canonical batch digest and require exact digest equality with `pending_canonical_batch_sha256`;
- use the same stable logical observation IDs (§7.1.10.4);
- then claim / re-publish / finalize.

A current entity-binding change cannot redirect already validated pending telemetry to another entity.

At-least-once physical Kafka duplication is allowed in the Kafka-commit / PostgreSQL-finalize uncertainty window.

Customer-visible canonical telemetry must remain deduplicated by stable logical observation identity.

#### 7.1.13 LEGACY_UNKNOWN migration and recovery

##### Migration preserve contract

For pre-checkpoint legacy pending rows, migration SHALL preserve:

- sequence;
- stream;
- checksum;
- batch/correlation identity;
- request body digest;
- reservation time;
- tenant/Sensor identity.

Migration marks the pending state `LEGACY_UNKNOWN` and publication history `UNKNOWN`.

Migration MUST NOT fabricate `FALSE` or `TRUE` publication history.

Migration MUST NOT clear the row.

Legacy V020 pending rows do **not** automatically possess a `PendingCanonicalBindingV1` snapshot.

##### Post-upgrade recovery

Exact Sensor retry after upgrade performs historical temporal authority validation (§7.1.11) **and** requires reproducible historical canonical entity identity.

Automatic `LEGACY_UNKNOWN` recovery requires **BOTH**:

1. qualified historical Agent authority (`SensorAcceptanceEvidenceV1` or another authoritative historical proof source — raw `acceptedAt` alone is insufficient);
2. reproducible historical canonical entity identity.

Allowed historical canonical proof includes a durable authoritative prior mapping or already-persisted canonical evidence that unambiguously identifies the original entity mapping. Current mutable entity resolution alone is **NOT** sufficient proof after a binding change.

**U5A — recover:**

`LEGACY_UNKNOWN` + qualified historical authority + reproducible canonical identity ⇒

```text
LEGACY_UNKNOWN → AUTHORITY_VALIDATED → publisher claim → transactional publish/re-publish → PostgreSQL finalize → ACK
```

This is safe regardless of unknown prior Kafka publication because delivery remains at-least-once and logical identity/dedupe is authoritative downstream.

**U5B — stream-local reconciliation:**

Historically valid authority but canonical mapping not reproducible ⇒

```text
LEGACY_PENDING_RECONCILIATION_REQUIRED
```

- no ACK;
- no republish;
- no pending deletion;
- no terminal settlement;
- Sensor retains custody;
- only the affected stream remains blocked.

If historical authority itself cannot be proven, the same stream-local `LEGACY_PENDING_RECONCILIATION_REQUIRED` outcome applies (U6).

The schema upgrade itself is not globally blocked by such a stream.

#### 7.1.14 Publication-uncertain authority change — mandatory case J7

Freeze mandatory J7 binding-change replay:

```text
1. exact reservation created
2. Sensor acceptance evidence qualified
3. historical/current Agent authority validates
4. canonical entity resolution produces Entity A
5. PendingCanonicalBindingV1 for Entity A is durably committed
6. state becomes AUTHORITY_VALIDATED
7. publisher claim
8. Kafka commit
9. PostgreSQL finalize fails
10. entity identifier/binding is changed to Entity B
11. Agent assignment may also be revoked/reassigned
12. exact original request retries
```

Required:

- replay uses persisted Entity A;
- does **not** resolve to Entity B;
- same logical observation ID (§7.1.10.4);
- physical Kafka replay may occur (at-least-once);
- canonical customer-visible result remains exactly one logical observation associated with Entity A;
- PostgreSQL finalize succeeds;
- pending state and binding snapshot clear.

Negative: changed request body cannot use Entity A snapshot.

#### 7.1.15 Authority-checkpoint / upgrade acceptance matrix (U1–U7 + J7 + J8)

```text
AUTHORITY_CHECKPOINT_RECOVERY = SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED
SENSOR_ACCEPTANCE_EVIDENCE_V1 = SPECIFIED / PENDING_IMPLEMENTATION
PENDING_CANONICAL_BINDING_V1 = SPECIFIED / PENDING_IMPLEMENTATION
```

until runtime candidate merges. Do not claim runtime/test completion on current canonical `main`.

| ID | Obligation | Owner |
|---|---|---|
| U1 | Clean migration (no pending / no active leases) succeeds under hard fence | BUILD-127 |
| U2 | Dormant legacy pending migrates preserved as `LEGACY_UNKNOWN` / publication `UNKNOWN` | BUILD-127 |
| U3 | Multi-tenant legacy pending preserve without cross-tenant bleed | BUILD-127 |
| U4 | Active publisher blocks migration | BUILD-127 |
| U5A | `LEGACY_UNKNOWN` + qualified historical authority + reproducible canonical identity ⇒ recover | BUILD-062 + BUILD-127 |
| U5B | Historically valid authority but canonical mapping not reproducible ⇒ stream-local `LEGACY_PENDING_RECONCILIATION_REQUIRED` | BUILD-062 + BUILD-127 |
| U6 | Insufficient historical proof → `LEGACY_PENDING_RECONCILIATION_REQUIRED`; stream-local block only | BUILD-062 + BUILD-127 |
| U7 | Accepted/settled history unchanged across migration | BUILD-127 |
| J7 | Publication uncertain + later authority/binding change remains Entity A recoverable | BUILD-062 |
| J8_UNVALIDATED_CRASH_THEN_REVOCATION | UNVALIDATED crash then revocation: qualified → `AUTHORITY_VALIDATED`; unqualified/uncertain → no publish/ACK/wedge | BUILD-062 |

## 8. Unified correlation / RCA contract supplement — v3.2.2

The existing Incident/Graph/RCA authority remains singular. Domain-specific rules normalize into the same contracts; they do not create separate network/storage/infrastructure RCA truth stores.

A versioned RCA result exposes at minimum:

```text
incident_id
rca_revision_id
revision_number
status PENDING | PROBABLE | DETERMINED | INCONCLUSIVE | SUPERSEDED
root_entity_id optional
first_causal_evidence_at optional
first_alert_at optional
trust_level CONFIRMED | HIGH | MEDIUM | LOW | INCONCLUSIVE
machine_confidence optional calibrated value
evidence_coverage
blast_radius_ref optional
supporting_evidence_refs[]
contradictory_evidence_refs[]
missing_evidence[]
rejected_hypotheses[]
recovery_validation optional
rule_model_provenance[]
created_at
supersedes_revision_id optional
recompute_reason optional
```

Each hypothesis is independently addressable and carries candidate entity/change, causal role, state, support/contradiction/missing evidence, expected effects/counterfactual checks, rejection reason where applicable, trust/confidence and rule/model version provenance. Causal-role vocabulary is:

```text
PRIMARY_CAUSE | CONTRIBUTING_CAUSE | AMPLIFIER | SYMPTOM |
CONSEQUENCE | MONITORING_FAILURE | UNKNOWN
```

Late evidence, topology/change updates and recovery may create a new immutable RCA revision. Consumers must never mutate historical evidence/reasoning in place merely to present the newest conclusion.

RCA events carry tenant, incident, revision, schema version, event time, ingest time and correlation/provenance. Replayed identical revision events are idempotent. Alert/incident correlation must preserve child-event references rather than deleting evidence after compression.

## 8.1 Presentation query/metric and visualization authority supplement — v3.2.2

Presentation definitions may reference governed query/metric/visualization metadata but do not themselves authorize arbitrary database execution.

A logical **Presentation Query & Metric Catalogue** exposes versioned metadata sufficient for Dashboard/Report Studio and OOB manifests to discover safe query choices:

```text
query_or_metric_id
version
domain
supported_entity_types[]
value_type / semantic_type
unit
dimensions[]
parameters[]
allowed_aggregations[]
allowed_groupings[]
supported_rollups_or_resolutions[]
time_semantics
freshness_quality_fields[]
permission_refs[]
resource_scope_behavior
entitlement_refs[]
query_bounds:
  max_time_range
  max_rows
  max_series_or_cardinality
  scan_or_cost_class
  timeout_class
supported_transformations[]
deprecation_or_compatibility
```

Exact physical storage is not frozen by this contract. The preferred starting point is schema-backed/versioned metadata consumed by existing Core/Query APIs. A separate catalogue microservice/database is not selected.

A logical **Presentation Capability Registry** defines panel-type capabilities, visualization-settings vocabulary, field overrides, value/status mappings, presentation thresholds, accessibility constraints, responsive/wallboard capabilities and allowed action/drill-down classes. Presentation thresholds never mutate Alert/Validation Policy.

The common workspace-manifest envelope references a typed class-specific definition. Dashboard-like classes may reference the current `DASHBOARD_DEFINITION`; console/entity/investigation/map/configuration/governance classes use their own bounded definitions when implemented.

Reference integrity is fail-closed. Shipped System definitions fail CI validation, and customer-authored definitions fail create/update validation, when a required `queryRef`, `dataSourceClassRef`, visualization/settings/override/value-mapping reference, action/drill-down target, permission reference or entitlement reference cannot be resolved by its authoritative registry. A panel/settings combination not permitted by the VIS capability matrix is invalid. Runtime authorization/entitlement re-evaluation remains mandatory even after definition validation; an unresolved security reference never defaults to allow.

Current `TEAM` scope values in presentation schemas are not sufficient ownership authority. A Team-owned artifact requires an authoritative tenant-scoped Team binding from the future Identity/Teams/RBAC reconciliation; authorization is always re-evaluated server-side.

## 8.2 Service assurance and reporting contract supplement — 2026-09-27

Logical API/data contracts are versioned and tenant scoped:

```text
ServiceTimeProfile
ServiceAssurancePolicy
ServiceAssurancePolicyVersion
ServiceAssuranceSimulation
ServiceAssuranceMeasurementPeriod
ServiceAssuranceLedgerEntry
ServiceAssuranceEvidenceRef
ReportDefinition
ReportObligation
ReportRun
ReportArtifact
ReportAttestation
```

`ServiceAssurancePolicy.type` supports at least `SLA`, `OLA`, `PROVIDER_SLA`, `SLO`. Policies reference governed scope, measurement basis, target/expected, optional minimum, service-time/calendar, formula version, inclusion/exclusion, degraded treatment, ownership/provider/dependency context and effective version.

A measurement period snapshots policy/formula/calendar/scope versions and preserves raw, adjusted and final result plus measurement quality/completeness. Finalized periods are immutable; restatement creates a new linked revision rather than overwriting prior truth.

Evidence references carry source, time range, quality/provenance and applicable incident/ticket/entity/service references. Monitoring-path degradation is distinguishable from monitored-service degradation.

Simulation endpoints are read-only and return candidate result/delta/explanation against a specified historical evidence window; they cannot activate policy or modify finalized periods.

Report artifacts use an explicit format enum restricted to `PDF` and `CSV`. All generated PDF reports prominently display the effective commercial product logo (defaulting to the governed AmrudAi logo: matching light/dark background contrast, preserving aspect ratio, or the tenant/MSP logo where a governed white-label configuration is active) and effective customer-facing commercial product branding. CSV reports contain data only and do not require image assets. JSON is an API payload encoding, not a report artifact format. XLSX/PPTX are not report artifact formats.

Report obligation/run/attestation APIs preserve schedule/due rule, timezone/calendar, recipients, authorization context, delivery state, artifact checksum/reference and review/dispute history. Report execution always re-evaluates current tenant/resource authorization.

Formula contracts are typed and bounded; arbitrary SQL/Python/JavaScript/eval text is not accepted as executable report/service-assurance logic.

## 8.3 Central branding configuration contract — 2026-09-27

Platform defines a central, tenant-scoped branding configuration contract to separate the internal engineering identity from external customer-facing commercial branding:

```json
{
  "internalProductId": "platform",
  "defaultCommercialProductName": "AmrudAi",
  "defaultLightLogoRef": "/brand/amrud_blk.png",
  "defaultDarkLogoRef": "/brand/amrud_whi.png",
  "faviconRef": null,
  "browserTitleTemplate": "{pageTitle} | {effectiveCommercialProductName}",
  "reportBranding": {
    "organizationName": null,
    "footerText": null,
    "confidentialityNotice": null
  },
  "whiteLabel": {
    "enabled": false,
    "overrideCommercialName": null,
    "customLightLogoRef": null,
    "customDarkLogoRef": null,
    "customFaviconRef": null,
    "customCssThemeRef": null
  }
}
```

- `internalProductId`: `"platform"` (immutable technical/engineering identifier for packages, schemas, services, builds, and internal architecture).
- `defaultCommercialProductName`: `"AmrudAi"` (default commercial/customer-facing product name).
- `defaultLightLogoRef` / `defaultDarkLogoRef`: governed relative asset references pointing to product-owned static assets (`amrud_blk.png` for light background / PDF reports; `amrud_whi.png` for dark background / UI navigation). Absolute developer filesystem paths (e.g. `/home/gagan/nms/logo/...`) must never be exposed or referenced.
- `faviconRef`: optional / `null` by default. No concrete default favicon asset is shipped in the repository; custom favicon references may only be supplied through optional white-label configuration overrides (`customFaviconRef`).
- **Single central branding resolver for UI and reports:** PDF rendering does not maintain an independent or duplicate brand authority. Instead, PDF rendering resolves `effectiveCommercialProductName`, `effectiveLightLogo`, and `effectiveDarkLogo` directly from the central branding resolver:
  - **Default:** `effectiveCommercialProductName = "AmrudAi"`, `effectiveLightLogo = "/brand/amrud_blk.png"`, `effectiveDarkLogo = "/brand/amrud_whi.png"`.
  - **White-label override:** Where an authorized, licensed tenant or MSP white-label configuration is active (`whiteLabel.enabled == true`), effective values resolve to the tenant/MSP configured commercial name (`overrideCommercialName`) and custom logo references (`customLightLogoRef` / `customDarkLogoRef`).
  - **Technical identity preservation:** Internal technical identity always remains `"platform"`.
  - **Report rendering & CSV integrity:** PDF reports embed the effective commercial logo appropriate to template/page background contrast, preserving aspect ratio without stretching. CSV reports contain tabular data only without embedded imagery.
- `reportBranding`: Contains only report-template metadata not owned by central brand resolution:
  - `organizationName`: optional / `null`. Publisher or legal entity name is distinct from the product commercial name and is never defaulted to `"AmrudAi"`.
  - `footerText`: optional / `null` / template-configurable. Report Studio documents do not force a universal footer string.
  - `confidentialityNotice`: optional / `null` / template-policy-controlled. Legal classification and document sensitivity markings are controlled per template/policy rather than universally imposing `"CONFIDENTIAL"`.
- White-label/MSP tenants may configure custom brand overrides without altering internal Platform schemas, service names, or API contracts.

## 8.4 Typed neighbor fact, dependency edge and suppression provenance — REQ-NET-007

This is a **logical versioned event/data contract**, not a claim that a physical schema/topic/API is deployed. Reuse the canonical tenant/entity/interface ID, existing topology.observation history, event, alert, incident, policy, RCA and audit authorities. Do not add a second graph source or publish a typed record as an unaccounted numeric observation.

- **Neighbor evidence:** tenant_id; source_entity_id; source_interface_ref plus local protocol index/port; advertised neighbor chassis/device-ID and port with ID subtype; optional management-address/capabilities; protocol LLDP|CDP; reporting Sensor/vantage; observed_at/received_at; TTL/first_seen/last_seen; quality/status; raw evidence reference, codec/Pack version and stable idempotency key. Remote advertisements are untrusted claims until canonical tenant-scoped identity corroboration.
- **Graph relationship projection:** stable tenant-scoped endpoint/interface refs when resolved; edge layer/type PHYSICAL_ADJACENCY versus ROUTING_PATH or directed DEPENDS_ON/HOSTED_ON where independently qualified; validity interval, confidence, direction, evidence/protocol/source, conflict state and graph revision/as_of. Missing neighbor evidence may expire a projection but is not a proven link or device failure.
- **Correlated alert/suppression decision:** tenant, incident ID, original immutable child alert/event IDs, hypothesized root reference, rule/codebook version, temporal window, graph snapshot/revision, evidence/contradiction/quality, explicit suppression reason and notification scope, actor/policy, created/expires/withdrawn time, history and idempotency key. Muting notification fan-out cannot erase source alerts or mark child devices DOWN.
- **RCA/fault signature:** candidate cause, TRUE/FALSE/UNKNOWN expected symptom vector, matched/conflicting/missing evidence, rule/codebook version and independent path/redundancy tests; emits existing revisioned hypothesis/root or INCONCLUSIVE. AI anomaly score is a separate advisory evidence reference.

Contract compatibility and physical tests must cover LLDP/CDP replay, duplicate and late topology facts, port renumbering, cross-tenant spoofed advertisement, contradictory edges, as-of replay, redundant-path counterfactual, suppression unmute, source/child-alert conservation and fail-closed decode/ACK. Specific implementation field names, topics, migrations and endpoint routes require subsequent approved BUILD-072/083/084 implementation review. These are **SPECIFIED, NOT IMPLEMENTED/CERTIFIED**.

## 9. Region, Sensor Pool and enrollment-control contract supplement — v3.2.2


Logical control resources are additive to the existing tenant/site/Sensor/Agent model:

```text
region:
  region_id
  tenant_id
  display_name
  parent/metadata optional
  lifecycle_state
  created_at / updated_at

sensor_pool:
  pool_id
  tenant_id
  region_id optional
  site_id optional
  display_name
  allowed_roles[]
  capability_constraints[]
  placement_policy
  failure_domain_policy
  capacity_policy_ref
  lifecycle_state
  revision

sensor_pool_membership / placement:
  sensor_id
  pool_id
  generation
  state
  effective_at
  revoked_at optional

agent_sensor_assignment:
  agent_id
  sensor_id
  generation
  state
  effective_at
  revoked_at optional
```

Region is customer-defined and optional; it is not a fixed geography enum. A Sensor Pool may be site-local or broader when policy explicitly allows, but every placement remains tenant-scoped.

Sensor bootstrap uses a short-lived single-use enrollment authority scoped to tenant/deployment and optional Region/Site/Pool/role constraints. The Sensor generates its keypair locally and submits proof/CSR material; Core never receives reusable Sensor private-key material. Successful enrollment binds Sensor identity/certificate and returns only authorized endpoint/control metadata.

Agent bootstrap follows the same local-key/short-lived-authority model and results in an authoritative generation-fenced Agent→Sensor assignment. Normal telemetry remains Agent→assigned Sensor→Core.

Stable Core endpoint contracts may use DNS or IP/VIP only with normal certificate trust/identity verification. APIs/installers must not require disabling hostname/SAN validation. Production Platform-owned Sensor↔Core and Agent↔Sensor control/telemetry contracts have no plaintext compatibility mode under this baseline.

## 10. BUILD-000 edge/control contract freeze (Issue #110)

> **Bounded subsection.** BUILD-000 froze the edge contract against historical base SHA `315a20980e89683666a64362f0ed3827bf613d4a`. The B000 decisions and exact ACK spelling remain; implementation statements below are reconciled to source `36e1513f25265d0bcd45bdfe5156ee02fdbf5238`. Historical `CURRENT_IMPLEMENTED_V1` and BUILD target labels do not imply that later completed hardening is still absent. Machine-readable authority remains `schemas/edge/`; where older prose uses illustrative naming (`boot_epoch`, uniform casing), **this section + `schemas/edge/` + Platform_05 §7.0 prevail for edge wire truth**. Source implementation is not certification or deployment.

### 10.1 Maturity labels

`CURRENT_IMPLEMENTED_V1` · `FROZEN_BUILD_000_INVARIANT` · `BUILD_002_TARGET` · `BUILD_003_TARGET` · `BUILD_004_TARGET`

`SPECIFIED != IMPLEMENTED != TESTED != CERTIFIED`. The five labels above describe the original freeze/ownership; current source status is stated explicitly below and must be rechecked at a later source revision.

### 10.2 Sequence / Counter64

- **FROZEN_BUILD_000_INVARIANT:** JSON-carried delivery/replay sequence maximum = `9007199254740991` (`2^53−1`); sequences ∈ `[1, MAX_SAFE_SEQUENCE]`; no wrap; fail closed on exhaustion. Schemas/edge validator encode this ceiling now.
- **FROZEN_BUILD_000_INVARIANT:** Counter64 metric values remain full unsigned 64-bit exact-counter domain (not sequences).
- **Current source (`36e1513f`):** implemented transport/control paths enforce the JSON-safe ceiling through Agent queue ceiling guards and delivery-worker ACK range binding, Sensor Agent-receiver validation, Core `TransportSequenceLimits`/validators and the V012/V013 constraints. Historical BUILD-002/003 enforcement targets are no longer accurate absence claims for these paths. Future typed-envelope/control extensions MUST preserve the same ceiling and receive their own conformance evidence.

### 10.3 Exact v1 ACK naming (do not “normalize” casing)

| Path | ACK fields (exact) | Durability |
|---|---|---|
| Agent→Sensor | `status`, `accepted_through_sequence`, `agent_id`, `batch_id`, `correlation_id` | Durable Sensor Agent spool accept |
| Sensor→Core telemetry | `status`, `accepted_through_sequence`, `sensor_identity` | PG reserve → fenced publisher lease → Kafka txn commit → fenced PG finalize → ACK; CH async; at-least-once |
| Sensor→Core agent-ingress | `status`, `sensorId`, `batchId`, `acceptedThroughIngressSequence`, `correlationId` | Current source: PG reserve → fenced publisher lease → Kafka txn commit → fenced PG finalize → ACK; **at-least-once**, not structured-evidence or HA certification |

### 10.4 bootEpoch

**CURRENT_IMPLEMENTED_V1 / FROZEN:** wire `bootEpoch` = Sensor stream/generation id `^stream-[0-9a-f]{32}$`. Historical LLD `boot_epoch` wording is an alias; not OS boot time.

### 10.5 Heartbeat v1 vs desired-state

**CURRENT_IMPLEMENTED_V1 Core response (wire authority):** `status`, `authenticated`, `sensor_identity`, `tenant_id`, `received_at`.

**Compatibility:** current Sensor client models additional desired-state fields and ignores unknown Core fields (`tenant_id` / `received_at`) via `doJSON` strict-decode fallback — heartbeat is not currently broken. Desired-state response cleanup/versioning is **BUILD_004_TARGET**.

Heartbeat remains liveness/connectivity authority only; do not read optional request version fields as Core-applied truth. Separately, current `SensorControlDesiredStateController` implements `GET /api/v1/sensor/control/desired-state` and `PUT /api/v1/sensor/control/applied-state` with V014 persistence and versioned schemas. That bounded desired-state/observation path does not establish complete production scheduler wiring or make heartbeat fields authoritative.

### 10.6 Entity authority

Agent `hostId` is evidence. Agent-supplied `entityId` is not authoritative. Core owns canonical entity mapping (**FROZEN_BUILD_000_INVARIANT** / B000-D009).

### 10.7 Agent stream generation

**B000-D015 / legacy schemaVersion 1 limitation:** replay without a stream generation cannot safely accept the same Agent identity restarting delivery sequences after durable queue loss.

**Current source truth (`92f0fb37`; stream identity frozen since historical reconciliation at `36e1513f`):** schemaVersion 2 Agent→Sensor batches carry durable `agentStreamId` matching `^stream-[0-9a-f]{32}$`; v1 batches must omit it. Queue metadata migration preserves watermarks and a bounded legacy cutover, and batches must not span that cutover. The stream survives process restart, changes only with governed durable-lineage recreation, and sequence exhaustion fails closed. Current Sensor→Core Agent-ingress uses schemaVersion 2 and `sensorAgentSpoolStreamId`; these are distinct Agent and Sensor spool identities. See actual schemas and `schemas/edge/README.md`; no ACK field is renamed.

### 10.8 Monitoring correctness

Collection/transport/credential/Agent/Sensor/Core/queue/ingestion failure, no telemetry, stale telemetry MUST NOT automatically assert monitored target DOWN. `No Data != Device Down` (**FROZEN_BUILD_000_INVARIANT** / B000-D012). Phase 1 monitoring-quality contracts preserve `UNKNOWN` / `PARTIAL` / degraded semantic states rather than collapsing them into DOWN.

### 10.9 Standard error

Wire `{status, code, message}` remains. Semantic classes: `INVALID_CONTRACT`, `AUTHENTICATION`, `AUTHORIZATION`, `STALE_OR_FENCED`, `UNSUPPORTED_VERSION`, `INTEGRITY`, `REPLAY_OR_DUPLICATE`, `CAPACITY_OR_RATE_LIMIT`, `TIMEOUT_OR_DEADLINE`, `CANCELLED`, `TRANSPORT`, `SERVER_UNAVAILABLE`. Detailed `ERR_*` codes remain; mapping is documented in `schemas/edge/README.md`.

### 10.10 Classification of conflicting transitional wording

| Older Platform_17 wording | Classification |
|---|---|
| §3.3 desired config ACK via control channel | Bounded desired-state/applied-state endpoint implemented; not heartbeat v1 and not full scheduler certification |
| §3.2.1 OpenAPI frozen before GA | FUTURE |
| §7 telemetry ACK pipeline | Current reservation/lease/transaction/finalize on both Core ingress paths; at-least-once; structured evidence and HA certification remain separate |
| Uniform identifier casing assumptions | STALE vs frozen dual naming |
| Illustrative `boot_epoch` naming | Alias of wire `bootEpoch` stream id |

## 11. Network & Infrastructure Observability data model and event contracts supplement — v3.2.2

> **Reconciliation reference:** Issue #142 (Master Chat comments `5909076167` and `5909787672`). Governed by `REQ-INF-013` (bounded Linux Agent eBPF process-to-network telemetry), `REQ-NET-016` (active path hop/loss/latency/fingerprint observability), `REQ-INF-008` (cloud network entities/relationships/change evidence), and `REQ-INF-010` (OTLP phase boundary: metrics/logs in Phase 1, bounded trace/dependency summaries in Phase 1.5). Target Phase-1 capabilities are subject to applicable implementation, benchmark, and certification gates.

### 11.1 Purpose and architectural constraints

This section supplements `Platform_17` with logical evidence contracts and target event families. Physical wire/storage bindings remain gated by BUILD-016/062/063/064; this section does not assert current end-to-end transport for:
1. **Linux Agent eBPF process-to-network telemetry**: local aggregation around `process/service + remote endpoint + protocol/port + time bucket`, transported via existing durable Agent queue → Sensor spool → Core ingestion → ClickHouse, with zero separate eBPF datastore or service.
2. **Active path hop-by-hop telemetry and change events**: Sensor-executed multi-vantage path probing, hop metrics, path fingerprinting, and divergence events.
3. **Distributed trace reference contract**: `trace_id` / `span_id` as future-compatible reference attributes across logs, events, alerts, and investigations, without Phase-1 trace storage or APM backend.

### 11.2 Typed Linux Agent process-to-network evidence contract

#### 11.2.1 Transport envelope and identity

**Logical target, not current structured delivery:** process-network facts are collected/aggregated in the Linux Agent source, but the structured evidence sink is not wired into the implemented scalar transport at `36e1513f`. BUILD-016/062 must bind the typed family to the existing versioned Agent→Sensor mTLS path and Sensor spool → Core route `POST /api/v1/sensor/agent-telemetry` (`SensorAgentIngestionController`) without weakening authority or durability. The existing route currently extracts supported numeric observations; its presence does not prove acceptance or persistence of `ProcessNetworkEvidence`.

**Authority and Identity Model:**
- **Sensor Certificate Authority:** Core authenticates the Sensor mTLS certificate. `AgentIngestionValidator.resolveAuthority(clientCert)` derives authoritative `SensorScope` (tenant and sensor). Core then validates incoming Agent records against that authority.
- **Tenant Authority:** The Agent-origin tenant claim is not independently authoritative at Core. Core tenant authority derives from authenticated `SensorScope`. The Agent-origin claim/envelope tenant MUST equal the authenticated Sensor tenant. Downstream canonical evidence receives `tenant_id` only after this validation succeeds.
- **Sensor & Assignment Provenance:** The accepting Sensor identifier (`acceptingSensorId`) must equal the authenticated Sensor. The tuple `agentId + assignmentGeneration` must resolve to an authoritative `AgentSensorAssignment`, and the assignment Sensor must match the authenticated Sensor.
- **Agent Identity:** `agent_id` is a bounded canonical Agent identifier string (e.g. `"agent-01"`), validated against tenant + authenticated Sensor + `assignment_generation` before downstream canonicalization. It is not redefined as UUID-only and remains compatible with existing Agent identifier naming.
- **Host Identifier & Entity Resolution:** `host_id` is a bounded Agent-observed host identifier string (e.g. `"host-1"`). It is corroborating identity evidence, not canonical entity authority (`Agent hostId != canonical entityId`). Core's approved `entity_agent_binding` supplies canonical `entity_id`; a conflicting HOST_ID identifier causes an identity conflict. Agent-supplied `entityId` is never accepted as authoritative.
- **Agent-ingress pending authority checkpoint (V021 local candidate):** Each Sensor ingress record carries `SensorAcceptanceEvidenceV1`: version, UTC acceptance time sampled after durable spool append/fsync, QUALIFIED/UNQUALIFIED clock state, bounded integer maximum error and synchronization age when QUALIFIED, clock source, Sensor service-instance identity, and actual durable spool sequence. The Sensor persists this evidence in an exact spool-sequence/payload-bound sidecar before delivery; missing evidence after a crash is durably recovered only as UNQUALIFIED. The containing exact request binds tenant, Sensor, Agent, assignment generation, ingress range, batch identity and digest. Revoked assignment authority requires a checked qualified interval wholly inside `[createdAt, revokedAt)`; equality or overlap at revocation is uncertain. An exact Sensor request reserves `UNVALIDATED`, then one PostgreSQL transaction validates authority, resolves canonical identity and checkpoints `AUTHORITY_VALIDATED` with normalized per-record `PendingCanonicalBindingV1` rows and `pending_canonical_batch_sha256` binding observations plus complete security-relevant binding metadata before publisher claim. Exact validated retries check Agent/generation/host against those rows, reconstruct the persisted Entity A identity, and require the same canonical digest without mutable live resolution. Migrated V020 pending remains `LEGACY_UNKNOWN` with unknown publication history; exact retry must prove historical assignment and an authoritative `entity_identifier_binding_history` interval covering the complete qualified Sensor acceptance interval before checkpoint and republish, or retain custody in reconciliation-required state. V021 does not infer history from pre-existing mutable identifier rows. A later Entity B mapping does not rewrite a proven Entity A interval. No state authorizes a different request or an Agent-wide bypass. The global `AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1` protocol remains SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED. This source candidate remains subject to exact-head testing and independent acceptance; it is not certification.
- **Transport Inheritance:** Authority, assignment, replay and delivery provenance are inherited from and validated by the enclosing Agent/Sensor transport envelope (including authenticated sensor scope, `sensor_id`, `accepting_sensor_id`, `agent_id`, `assignment_generation`, agent stream identity, sensor ingress sequence, agent sequence, batch/idempotency identity, and correlation identity) before process-network evidence is canonicalized or persisted.
- **Workload Aggregation Identity Invariant:** Every single-valued workload/local identity attribute emitted on an aggregated `ProcessNetworkEvidence` record must be common to every event represented by that record. If multiple processes or workloads are deliberately merged (e.g. across worker processes under a service or container scope), workload-specific attributes (such as `pid` or specific local socket ports) must not pretend to identify one member; such fields must be omitted or null, or represented through explicitly aggregated dimensions, and the record must expose aggregate scope/quality appropriately. Applicable identity dimensions may include executable/process identity, service identity, container identity, cgroup identity, network namespace, and local endpoint scope. PID remains an ephemeral runtime observation only and must NEVER become durable entity authority. Exact implementation and cardinality balance are owned by BUILD-011 and certified by BUILD-019 / BUILD-130 without freezing premature cardinality constants.

Future extension and versioning of the physical envelope belongs to BUILD-016 / BUILD-062 and must preserve authority, idempotency, ACK, and durability contracts. The `uint64` logical fields below do not select a JSON number encoding; their exact physical representation and end-to-end precision/overflow tests MUST be frozen at that contract gate before transport implementation. `MAX_SAFE_SEQUENCE` remains a transport/control sequence bound, not a blanket conversion rule for raw measurement values. Ingestion validation and envelope acceptance are owned by BUILD-062, physical Kafka streaming (topic naming, partitions, serialization) is owned by BUILD-063, and physical ClickHouse storage (table DDL, engine selection, TTL) is owned by BUILD-064.

```text
ProcessNetworkEvidence:
  schema_version:           "1.0"
  tenant_id:                UUID (canonicalized at Core from authenticated SensorScope after Agent-origin tenant claim and assignment provenance validate)
  agent_id:                 string (bounded canonical Agent identifier string; validated against tenant + authenticated Sensor + assignment_generation before downstream canonicalization)
  host_id:                  string (bounded Agent-observed host identifier string; corroborating identity evidence only, not canonical entity authority)
  entity_id:                UUID (nullable, Core-resolved canonical entity identity; Agent hostId is not authoritative entityId)
  bucket_start_at:          RFC3339UTC instant
  bucket_end_at:            RFC3339UTC instant
  bucket_duration_sec:      uint32 (bounded configurable aggregation interval)
  process_metadata:
    process_name:           string (bounded string, e.g. "nginx", "envoy", "mysqld")
    executable_path:        string (bounded string, e.g. "/usr/sbin/nginx"; truncated display paths must not alter canonical identity)
    service_name:           string (bounded string, systemd service or container service name)
    pid:                    uint32 (nullable/optional runtime observation only, non-authoritative ephemeral identifier; null if aggregated across multiple worker PIDs)
    container_id:           string (nullable bounded string, e.g. Docker/containerd container ID)
    cgroup_path:            string (nullable bounded string, e.g. "/system.slice/nginx.service")
    network_namespace:      uint64 (nullable netns inode number)
  endpoint_pair:
    local_ip:               string (nullable/optional canonical IPv4 or IPv6; absent when not common to all represented events)
    local_port:             uint16 (nullable/optional; absent when not common to all represented events)
    remote_ip:              string (IPv4 or IPv6 canonical string)
    remote_port:            uint16
    remote_entity_id:       UUID (nullable, Core-resolved canonical target entity)
    protocol:               string ("TCP" | "UDP")
    ip_version:             string ("IPv4" | "IPv6")
  traffic_metrics:
    bytes_tx:               uint64 (raw bytes sent, where observable)
    bytes_rx:               uint64 (raw bytes received, where observable)
    active_connections:     uint32 (nullable/optional gauge at bucket end, applicable to stateful protocols like TCP)
    new_connections:        uint32 (nullable/optional counter within bucket)
    closed_connections:     uint32 (nullable/optional counter within bucket)
    duration_total_ms:      uint64 (nullable/optional aggregate duration of closed connections)
    mean_rtt_us:            uint32 (nullable/optional microsecond RTT from TCP info/SRTT when technically available and certified)
    retransmit_count:       uint32 (nullable/optional TCP segment retransmissions when technically available and certified)
    connection_failures:    uint32 (nullable/optional TCP RST, connection timeouts, SYN retries exceeded when technically available and certified)
  quality_and_shedding:
    quality_state:          "VALID" | "PARTIAL" | "SAMPLED" | "RESOURCE_GUARDED" | "UNSUPPORTED"
    dropped_records_count:  uint32 (records dropped due to queue or buffer saturation)
    shed_records_count:     uint32 (records shed due to memory/CPU guard thresholds)
    sampled_flag:           boolean
    partial_flag:           boolean
    resource_guarded_flag:  boolean
```

Protocol-dependent evidence rules: TCP-specific metrics (such as `mean_rtt_us`, `retransmit_count`, `connection_failures`, `active_connections`, `new_connections`, `closed_connections`, `duration_total_ms`) are applicable only when the selected implementation and protocol can measure them reliably. For non-stateful or unsupported protocols (such as UDP), or on platforms where specific kernel metrics are unavailable, these fields must be omitted or null rather than emitting fabricated zero values. The record distinguishes measured zero from unavailable, unsupported, partial, or resource-guarded status.

#### 11.2.2 Hard data-minimization boundaries

In accordance with Platform security threat models (`Platform_18`), the following data categories are **STRICTLY PROHIBITED** from process-network payloads:
- Application payload / packet data;
- TLS / SSL plaintext or decrypted payloads;
- HTTP request/response headers or bodies;
- SQL query text, parameters, or database statements;
- Command-line arguments or environment variables (eBPF process-network evidence excludes command-line text and correlates strictly using safe identity attributes: process name, executable identity, service identity, container/cgroup, network namespace, and ephemeral PID);
- Arbitrary kernel syscall argument streams;
- Kernel or user-space stack traces;
- Continuous profiler / flame-graph sample streams.

### 11.3 Active-path telemetry and change event contracts

#### 11.3.1 Active path hop-by-hop observation contract

Sensor-executed active path probes emit structured hop sequences and path-level performance facts. `path_summary.status` remains the probe-completion outcome. Path **identity/quality** and **comparison eligibility** are a separate machine-readable dimension (orthogonal to completion status) so AMBIGUOUS/UNPINNED/INSUFFICIENT/UNQUALIFIED observations are first-class evidence without overloading `COMPLETE|PARTIAL|TARGET_UNREACHABLE|FAILED` or inventing a breaking `schema_version` migration.

```text
ActivePathObservation:
  schema_version:           "1.0"
  path_id:                  UUID (canonical path test configuration identity)
  tenant_id:                UUID
  source_sensor_id:         string (bounded opaque canonical Sensor identifier; same identity contract as schemas/edge/common-definitions.schema.json sensorId, e.g. "sensor-1")
  source_vantage_id:        string (vantage location tag, e.g. "aws-eu-west-1", "dc-onprem-tor")
  target_entity_id:         UUID (canonical target entity identity)
  target_ip:                string (IPv4 or IPv6 canonical string)
  target_port:              uint16 (nullable for ICMP)
  protocol:                 "ICMP" | "TCP" | "UDP"
  probed_at:                RFC3339UTC instant
  path_summary:
    total_hops:             uint8
    responsive_hops:        uint8
    end_to_end_rtt_min_ms:  float32 (nullable, null if target unreachable or unmeasured)
    end_to_end_rtt_mean_ms: float32 (nullable, null if target unreachable or unmeasured)
    end_to_end_rtt_max_ms:  float32 (nullable, null if target unreachable or unmeasured)
    end_to_end_loss_pct:    float32 (nullable, null if unmeasured)
    jitter_ms:              float32 (nullable, null if insufficient samples)
    path_fingerprint:       string (opaque, versioned, deterministic fingerprint of the observed candidate hop sequence; algorithm selected and implemented by BUILD-040. Describes a candidate observation identity only. MUST NOT be used alone to assert topology change, RTT/loss change or PathChangeEvidence.)
    fingerprint_version:    string (e.g. "1")
    status:                 "COMPLETE" | "PARTIAL" | "TARGET_UNREACHABLE" | "FAILED"
    # Optional identity/quality metadata — backward-compatible additions under schema_version "1.0".
    # Absent metadata MUST be treated conservatively (see §11.3.3). Do not claim mixed-version
    # change-emission safety for consumers that still emit PathChangeEvidence from fingerprint deltas alone.
    path_identity_quality:  "PINNED" | "AMBIGUOUS" | "UNPINNED" | "INSUFFICIENT" | "UNQUALIFIED" (optional)
    comparison_eligible:    boolean (optional; true ONLY when the producer asserts this observation is qualified and comparable under BUILD-040 rules for topology and latency/loss comparison)
  hops:
    - hop_index:            uint8 (bounded hop index; max/probe limits governed by BUILD-040 policy and certification)
      hop_ip:               string (nullable IPv4 or IPv6 canonical string; null if unresponsive or hidden)
      hop_state:            "RESPONSIVE" | "UNRESPONSIVE" | "HIDDEN" | "UNKNOWN"
      hop_hostname:         string (nullable, reverse DNS PTR)
      canonical_entity_id:  UUID (nullable, Core-resolved entity ID if internal infrastructure)
      asn:                  uint32 (nullable, BGP autonomous system number when resolvable)
      rtt_min_ms:           float32 (nullable, null if unresponsive/hidden/unmeasured)
      rtt_mean_ms:          float32 (nullable, null if unresponsive/hidden/unmeasured)
      rtt_max_ms:           float32 (nullable, null if unresponsive/hidden/unmeasured)
      loss_pct:             float32 (nullable, null if unmeasured)
      jitter_ms:            float32 (nullable, null if insufficient samples)
      mpls_labels:          uint32[] (nullable, when observable from supported protocol extensions)
      is_unresponsive:      boolean (true if unresponsive or timeout)
      is_hidden:            boolean (true if router suppresses TTL expiry)
```

**Identity/quality and comparison eligibility (machine-readable):**
- `path_identity_quality = PINNED` means the observation is sufficiently flow-stable/pinned for BUILD-040-qualified geometry comparison.
- `AMBIGUOUS` / `UNPINNED` / `INSUFFICIENT` / `UNQUALIFIED` mean the fingerprint may still describe the observed candidate path but the observation is **not** eligible to assert topology or latency/loss change.
- `comparison_eligible = true` requires `path_identity_quality = PINNED` and BUILD-040-qualified sample sufficiency for the comparison class being considered. Producers MUST set `comparison_eligible = false` (or omit it) whenever quality is not `PINNED`.
- When `path_identity_quality` or `comparison_eligible` is **absent** (older producer): consumers MUST treat the observation as **not comparison-eligible** (`UNQUALIFIED` / `comparison_eligible = false`). Fingerprint presence alone never upgrades eligibility.
- When an older consumer cannot evaluate these fields: it MUST NOT emit `PathChangeEvidence` from fingerprint deltas. Claiming “schema compatible” while still creating false change events from fingerprint-only logic is prohibited.

#### 11.3.2 Path change event contract

`PathChangeEvidence` may be emitted **only** when **both** the baseline (previous) and current `ActivePathObservation` are comparison-eligible (`comparison_eligible = true` and `path_identity_quality = PINNED`) under the same `fingerprint_version` / BUILD-040 comparison class, and a qualified topology or latency/loss delta is observed. A fingerprint difference, incomplete path, AMBIGUOUS/UNPINNED/INSUFFICIENT/UNQUALIFIED observation, or absent quality metadata MUST NOT authorize `PATH_DIVERGENCE`, `HOP_ADDED`, `HOP_REMOVED`, `RTT_SHIFT`, `LOSS_ANOMALY` or fabricated `DEVICE_DOWN`. Physical Kafka topic naming, partitioning and serialization remain owned by BUILD-063:

```text
PathChangeEvidence:
  schema_version:           "1.0"
  event_id:                 UUID
  tenant_id:                UUID
  path_id:                  UUID
  source_sensor_id:         string (bounded opaque canonical Sensor identifier; same identity contract as schemas/edge/common-definitions.schema.json sensorId, e.g. "sensor-1")
  target_entity_id:         UUID
  detected_at:              RFC3339UTC instant
  previous_fingerprint:     string
  current_fingerprint:      string
  fingerprint_version:      string
  change_type:              "PATH_DIVERGENCE" | "HOP_ADDED" | "HOP_REMOVED" | "RTT_SHIFT" | "LOSS_ANOMALY"
  divergence_hop_index:     uint8 (nullable, first hop index where topology divergence was observed; null for RTT_SHIFT or LOSS_ANOMALY without topology change)
  previous_hop_ip:          string (nullable)
  current_hop_ip:           string (nullable)
  delta_rtt_ms:             float32 (nullable)
  delta_loss_pct:           float32 (nullable)
  correlation_id:           string
  # Optional emission provenance — absent on older producers; consumers must still enforce the dual-observation eligibility gate above.
  baseline_path_identity_quality: "PINNED" (optional; when present must be PINNED for a valid emission)
  current_path_identity_quality:  "PINNED" (optional; when present must be PINNED for a valid emission)
```

#### 11.3.3 Operational and causal integrity rules

- **Incomplete Path != Target Down:** An unresponsive intermediate hop or incomplete traceroute (e.g. intermediate ICMP suppression, CoPP or timeouts) MUST NOT assert target entity DOWN when corroborating ICMP echo, TCP port check, or SNMP polling indicates reachability.
- **Ambiguous / unpinned path quality (machine-readable):** Flow-stable probing is an evaluation technique under BUILD-040 ownership; it does not guarantee identical physical paths. When ECMP/per-packet balancing, NAT/middleboxes, MPLS, IPv6 Flow Label behaviour, ICMP CoPP or return-path asymmetry prevent a pinned observation, emit `path_identity_quality` of `AMBIGUOUS` / `UNPINNED` / `INSUFFICIENT` / `UNQUALIFIED` as applicable and set `comparison_eligible = false` (or omit it). A `path_fingerprint` may describe the observed candidate path but MUST NOT be used to assert topology or latency/loss change, and MUST NOT produce false `PATH_DIVERGENCE`, `RTT_SHIFT`, `LOSS_ANOMALY` or `DEVICE_DOWN`. Existing `schema_version` `"1.0"` and mandatory field requirements are preserved; optional quality/eligibility fields are additive. Mixed-version safety is conservative: absent optional metadata ⇒ not comparison-eligible; fingerprint-only change emitters are not safe.
- **PathChangeEvidence dual-qualification gate:** Emit only when **both** baseline and current observations are qualified and comparable. Otherwise retain quality/uncertainty evidence on the observations themselves.
- **Multi-Vantage Contradiction Preservation:** Divergence between Sensors is evidence to investigate, not proof of upstream routing/peering failure or target failure. A silent transit hop can reflect ICMP filtering/rate-limiting even when forwarding succeeds. Preserve source/vantage, protocol/target, time/freshness, collector health, path identity/comparison quality and each destination result; independently test plausible path, policy, source and endpoint alternatives. Only qualified corroborating evidence may support a fault-domain hypothesis; otherwise retain contradiction/uncertainty and `INCONCLUSIVE` under the owning RCA contract.

### 11.4 Distributed trace reference contract

To support cross-signal correlation with application observability without storing high-volume trace spans, Platform defines a trace reference contract across all event and log schemas:

```text
TraceContextReference:
  trace_id:                 string (32-hex character W3C Trace Context ID, or null)
  span_id:                  string (16-hex character W3C Trace Context Span ID, or null)
  sampled_flag:             boolean (trace sampling bit)
```

- **Scope:** Carried as optional reference metadata in syslog, flow records, diagnostic captures, alert evidence bundles, and Investigation Session items.
- **Phase-1 Architectural Boundary:** AmrudAi Phase 1 supports OTLP metrics and logs with trace-ready contracts and data models (`trace_id`, `span_id` reference fields). AmrudAi Phase 1 does NOT store trace spans, does NOT operate a trace datastore (such as Jaeger or Tempo), and does NOT construct distributed trace waterfall graphs.
- **Phase 1.5 Scope:** Phase 1.5 may add bounded customer-provided trace and dependency evidence (such as `trace_id`, `span_id`, service identity, dependency edges, latency/error summaries, selected exemplars) under separate implementation and certification. Full distributed trace storage, APM bytecode instrumentation, and dedicated trace backends remain excluded and require future explicit architecture change control.

---

## Phase-1 software inventory, monitor-selection and EDS-definition contracts — Issues #169/#170

### Software inventory

Canonical logical structured evidence is a versioned bounded authoritative snapshot/envelope. Item rows alone are insufficient for absence/removal inference.

```text
software_inventory_snapshot
  schema_version
  snapshot_id
  source_mode AGENT|SSH|WINRM_WMI_CIM
  source_type
  # authoritative endpoint/Agent/host context inherited from transport/Core authority
  inventory_scope
  snapshot_mode FULL|TARGETED
  started_at
  completed_at
  capture_status COMPLETE|PARTIAL|FAILED
  item_count
  truncated
  incomplete_or_loss_reason?
  source_provenance
  quality
  items[]
    native_package_id
    product_name
    version
    vendor_publisher?
    architecture?
    installation_scope MACHINE|USER|UNKNOWN
    authoritative_install_or_update_time?
    normalized_product_id?
    normalization_version?
    normalization_confidence?
```

Tenant/entity authority fields are inherited from existing transport/Core authority conventions and must not be duplicated as endpoint-authored truth that overrides Core identity.

`snapshot_id` identifies one inventory capture. It MUST remain stable across retries/replay of that same capture; a genuinely new capture MUST receive a new `snapshot_id`. Core reconciliation is idempotent on authoritative snapshot identity.

`inventory_scope` is the stable/versioned canonical comparison-scope identity for the certified collector/source definition (opaque/versioned fingerprint of what that source enumerates). Distinct certified collector/source-definition versions that change enumerated authority MUST use distinct `inventory_scope` values. Absence/removal may compare only the same endpoint authority + same native source + same `inventory_scope`.

For Windows default machine inventory, `source_provenance` unambiguously declares the native source/registry views covered (for example, the 64-bit and 32-bit `HKLM` Uninstall views) and their collection status, while each item's `native_package_id` is qualified by native source/hive/view and stable entry identity. The logical `inventory_scope` fingerprints the declared source set, privilege/scope and compatible collector definition. If each view is a separate native source/snapshot, its comparison key remains separate; if a snapshot combines multiple declared views, it can be `COMPLETE`/`FULL` only when all required views completed. Missing or denied required views must produce `PARTIAL`, not an authoritative absence signal. Raw dual-view/bundle registrations must not collide in Core native keys. Any cross-record product deduplication belongs to a versioned normalized identity resolver and must not rewrite source-native lifecycle evidence. Per-user inventory is never implicitly `FULL` from a Session-0 service; it requires a separately authorized/certified source scope, without offline hive mounting by default.


Replay / ordering / stale-snapshot authority:

- Duplicate replay of an identical `snapshot_id` must not create duplicate authoritative state, duplicate `SOFTWARE_*` events, incorrect `first_seen_at`/`last_seen_at` mutation, or false lifecycle history.
- An older authoritative snapshot arriving after a newer authoritative snapshot may be retained as historical/late evidence if policy permits, but MUST NOT replace newer current-state authority or generate reverse false change events. Later Core arrival alone does not confer authority.
- Ordering authority MUST preferentially inherit trustworthy enclosing execution/transport ordering: Agent path uses Agent stream/durable-envelope sequence, assignment generation and Sensor ingress/delivery provenance under BUILD-016/BUILD-062 replay/idempotency; agentless path uses authoritative collection/Diagnostic Job identity, Sensor/vantage execution identity and governed job ordering/generation where available. Wall-clock `started_at`/`completed_at` alone MUST NOT become sole ordering authority when stronger transport/job ordering exists. Late/out-of-order evidence retains original capture time/provenance.

Current inventory authority is Core-reconciled state. `first_seen_at` and `last_seen_at` are Core-derived lifecycle fields and are not accepted as endpoint-authored historical truth. Replay, stale snapshot delivery or normalization-resolver changes must not fabricate lifecycle changes or masquerade as package uninstall/install/version-change. Native source/package identity remains authoritative for reconciliation; normalized product identity is optional/versioned/provenance-bearing.

Normalized change event classes remain:

`SOFTWARE_INSTALLED | SOFTWARE_REMOVED | SOFTWARE_VERSION_CHANGED`.

Removal/absence safety:

- only a `COMPLETE`, non-truncated, authoritative `FULL` reconciliation for the same endpoint authority + native source + `inventory_scope` may infer general absence/removal against the prior authoritative FULL snapshot for that same comparison key;
- `PARTIAL`, `FAILED` or truncated capture must never generate mass removal;
- `TARGETED` refresh cannot infer absence of unrelated packages;
- source permission/access failure is monitoring-quality evidence, not uninstall evidence;
- a source/`inventory_scope` definition change must not appear as mass uninstall/install; incompatible FULL snapshots must not be compared as identical authority scope.

Software inventory is structured evidence and shall not be encoded as high-cardinality scalar metric labels on the current scalar Agent metric path (`HostCollector → Sample → Observation → CollectionBatch`).

Physical ownership boundary (logical target vs current implementation):

- BUILD-011/014 collect/normalize bounded typed inventory snapshots and expose an internal typed evidence boundary; they do not fake inventory through scalar metric labels and do not claim downstream persistence;
- BUILD-016 owns physical Agent structured-envelope / durable-delivery evolution required for typed inventory evidence;
- BUILD-062 owns Core envelope acceptance, authority validation and ingestion;
- BUILD-063 owns physical streaming realization;
- BUILD-064 owns physical historical/fact storage where appropriate;
- BUILD-098 consumes canonical structured inventory evidence and owns reconciliation/current-state/change-evidence/query behavior.

Public/internal query contracts are tenant/RBAC scoped and bounded.

### Process/service monitor definition

API/data contracts carry a stable selector and effective policy reference. PID may be returned as an observation but is prohibited as the durable selector key. Supported selector classes are service/unit identity, executable identity/path and bounded exact process name according to certified OS behavior.

Create/update/delete/Stop Monitoring operations are audited, tenant/resource authorized and versioned. Remediation authority is a separate contract.

### Criticality/Profile assignment

Expose business Criticality, suggested profile, effective profile/version, inheritance source and assignment mode `AUTO_FROM_CRITICALITY|EXPLICIT_PROFILE`. API updates preserve explicit override semantics and return capacity/admission failures without partially mutating effective policy.

### Trigger-specific EDS definitions

Existing EDS `definition_id/definition_version` is the authoritative extension point. Phase-1 built-ins include:

`EDS_CPU_PRESSURE`, `EDS_MEMORY_PRESSURE`, `EDS_FILESYSTEM_CAPACITY`, `EDS_BLOCK_IO_PRESSURE`, `EDS_PROCESS_FAILURE`, `EDS_PROCESS_RESTART_LOOP`, `EDS_PROCESS_UNRESPONSIVE`, `EDS_NETWORK_DEGRADATION`.

No second Fast-RCA bundle schema is introduced.

EDS/Diagnostic Job evidence may carry optional `evidence_window_id` and/or canonical context/relationship snapshot reference when associated with an Incident Flight Recorder / Investigation evidence window. Related OOB/virtualization/storage/path evidence remains separate evidence references with its own source/vantage/provenance; it is not copied into the endpoint EDS payload as fabricated local context.

Factual indicator values are versioned evidence fields, not free-form root-cause prose. Core RCA references evidence IDs and preserves revisions.


---

## 13. v3.3.0 Integration, CMDB and Change Assurance contract supplement

### 13.1 Canonical external-object API model

Versioned connector APIs normalize vendor objects to:

- `Incident` (Major Incident normally classification/extension);
- `Problem`;
- `Change`;
- `ChangeTask`;
- `Approval`;
- `ExternalActivity{PUBLIC_COMMENT|INTERNAL_WORK_NOTE}`;
- `MaintenanceWindow`;
- `AssignmentRef`, `ExternalPrincipalRef`, `BusinessServiceRef`;
- `CIRef`, `CIRelationshipRef`;
- `AttachmentRef`, `ExternalBinding`, `SyncConflict`.

Vendor-specific fields remain adapter extensions/mappings and must not leak into unrelated domain models as mandatory fields. Service Request may be represented for adapter interoperability but service-catalog-triggered privileged operation is Phase 2.

### 13.2 Integration event envelope

Inbound normalized events include server-derived `tenant_id`, `connector_id/version`, external event/object/type identifiers, external version/ETag where present, event time + received time, correlation/operation identity, origin/lineage, normalized object reference, mapping version, payload hash and authentication/verification result. Webhook payload tenant fields are never authoritative.

Outbound operation records include stable operation ID/hash, connector/version, canonical object/reference, external binding, mapping version, correlation ID, retry class/attempt/not-before, rate-budget state and lineage. Delivery is at-least-once/idempotent; exactly-once is not claimed.

### 13.3 ExternalBinding and relationship provenance

ExternalBinding semantics are defined in Platform_05 §94.2. `CIRelationshipRef` maps external relationship ID/type to an existing canonical graph edge representation while preserving external source, authority/declaration class, confidence where relevant, first/last observation, freshness, lifecycle and supporting binding. Normalized relationship vocabulary may include `CONNECTED_TO`, `RUNS_ON`, `HOSTED_ON`, `DEPENDS_ON`, `USES`, `SERVES`, `MEMBER_OF`, `BACKED_BY` and `MANAGED_BY` where the external/product semantics are supportable. Vendor-specific relation types may remain namespaced rather than being falsely normalized. Imported declared relationships must remain distinguishable from independently observed technical evidence.

### 13.4 Change Assurance API

Change Plan API uses immutable/versioned plan semantics and optimistic concurrency/ETag. Approval validity references exact plan version, target-set hash, rendered-change/template hash and approved window. Mutation returns/causes `STALE_REAPPROVAL_REQUIRED` where any approval-bound material property changes. Change Assurance query returns references to risk/blast snapshots, precheck/canary/batch/postcheck/stabilization/rollback/outcome evidence and audit rather than duplicating underlying source records.

### 13.5 OTLP Phase-1 data contract

Metrics/logs preserve OpenTelemetry Resource and Instrumentation Scope metadata needed for correct semantics and compatibility, plus schema/semantic-convention provenance where supplied. Metric handling preserves instrument/temporality/monotonic/unit/histogram semantics where certified; logs preserve timestamps, severity, bounded body/attributes and trace/span correlation identifiers. No Phase-1 schema may require distributed span persistence. Phase-1.5 sampled span contracts, if activated, preserve trace/span/parent identity, kind/timing/status, bounded attributes/events/links and sampling provenance without changing canonical entity/service identity.

### 13.6 Endpoint/access-port relationship evidence — Phase 1.5

Attachment observations map mutable MAC/IP/user-when-available/AP/switch/port/VLAN evidence to canonical entities/graph with source, observed time, confidence/quality and history. PID/MAC/IP/user labels are not silently promoted to permanent global identity without reconciliation evidence.

### 13.7 ICMP typed observation contract and Issue #218 gap

Phase 1 API/event contracts require ICMP availability evidence as a versioned typed observation family (Sensor→Core durable batch) carrying target, IP version, sent/received, loss, RTT samples, error/quality codes and provenance. Quality values such as `GOOD` / `PARTIAL` / `UNKNOWN` are first-class. Incomplete or rejected ICMP evidence must not authorize DEVICE_DOWN solely by absence of numeric projection.

**Issue #218 (SOURCE_FACT / NOT CLOSED):** Current Core `IngestionValidator` accepts SNMP `protocolPayload.observations` arrays or generic `metric_id`+numeric `value` shapes and rejects other protocol payloads with `ERR_UNSUPPORTED_RECORD_TYPE` before durable Kafka/ACK. Sensor `icmp_observation_v1` therefore fails end-to-end acceptance today. Architecture/contracts remain typed-ICMP; unresolved Core acceptance must not be documented as implemented, benchmarked or certified. Mixed-batch starvation risk from whole-batch rejection is a recorded integrity concern for gated Core remediation.

### 13.8 Change Assurance / NCM phase envelope

Change Assurance APIs and NCM privileged-execution contracts retain maker-checker, exact plan/hash/target/window binding and audit. Phase 1 independently certifiable surfaces are backup/version/diff/compliance/golden-state plus controlled manual CLI when security gates pass. Mass push/canary/automated remediation API enablement defaults to **Phase 1.5** unless Founder change control promotes them.

---

### 13.9 Flow ingress, template validity and sampling data contract — 2026-10-30 Network Sensor audit

This refines the existing Flow/NTA/tenant/data-integrity contracts and reconciles `Platform_05` §5.4 `network_flow` field semantics; it does not authorize a new ingest protocol, database, storage dependency, shared-MSP deployment or breaking schema migration. Every canonical Flow observation must bind independently verified ingress context, routing domain and authorization plus resolved exporter/observation-domain identity separately from untrusted packet source attributes. A bare UDP port is not a trusted isolation boundary. The raw event retains protocol/version, source and destination collector transport context where applicable, source ID/Observation Domain, template/options identity and accepted version/lifetime/refresh/epoch evidence, sequence/gap/duplicate/unknown-template accounting, and raw capture/export times. An ambiguous tenant/exporter binding must fail closed or quarantine without misattributing a record to another customer. Quarantined/unmapped traffic must not produce canonical Flow analytics. Phase 1 covers intra-tenant overlapping RFC1918/VRF spaces on verified ingress contexts; MSP/shared-customer isolation certification remains Phase 1.5.

**First bounded Phase-1 UDP receiver profile (specification authority).** The first executable receiver profile is `FLOW_UDP_P1_PROFILE_V1` owned by BUILD-051 in the Master Build Sequence Register (protocol/security notes also in `Platform_10`). It is a specification profile (`SPECIFIED_NOT_IMPLEMENTED`) until code and evidence exist; documentation of the profile MUST NOT change BUILD-051 source-reality maturity away from `NOT_IMPLEMENTED`. Profile scope: UDP collection for NetFlow v5, NetFlow v9, IPFIX v10 over UDP, and sFlow v5. J-Flow is not a proprietary parser in this profile — a J-Flow exporter is supported only when its wire format is certified as one of those NetFlow/IPFIX formats. IPFIX over TCP/SCTP and other transports require a later bounded profile. Tenant identity MUST NEVER come from a Flow packet field; resolve through current Core-issued/configured exporter binding using transport provenance (source IP/UDP port, collector local address/port, receive time, Sensor identity). Source IP alone is insufficient where NAT/overlap can make binding ambiguous. Unknown/ambiguous exporter binding ⇒ no tenant guess, no customer Flow persistence, typed quarantine/quality outcome, observable counters. Activated profile MUST supply `template_refresh_timeout_seconds > 0` (no invented universal timeout in code). IPFIX V1 untemplated Data Sets use `NO_BUFFER_UNTEMPLATED_DATA`. Sequence/loss uses protocol-native semantics (not a universal `sequence+1` rule) and never asserts Device Down. GoFlow2 (`OSS-022`) import for BUILD-051 is blocked until non-BUILD gate `FLOW_PARSER_DEPENDENCY_ADMISSION_GATE` freezes exact version/commit, licence revalidation, transitive inventory, SBOM/CVE, API surface used, reproducible checksum, fixture compatibility and rollback — dependency admission is not Flow certification.

**sFlow v5 sequence quality envelope (canonical typed outcomes).** Detailed datagram-sequence / gap / duplicate / reorder / restart semantics are owned by `Platform_10` (sFlow v5 datagram sequence contract; BUILD-051 S5). Canonical Flow quality/envelope fields MUST be able to carry the following typed outcomes without collapsing them into Device Down or inventing missing Flow rows: `SEQUENCE_GAP` (with `missing_datagrams` where applicable), `DUPLICATE_DATAGRAM`, `SEQUENCE_COLLISION`, `OUT_OF_ORDER_OR_OLD_DATAGRAM`, `EXPORTER_RESTART_OR_DISCONTINUITY`, `SEQUENCE_DISCONTINUITY_UNKNOWN`, and supplemental sample-family outcomes `SAMPLE_SEQUENCE_GAP` / `SAMPLE_DUPLICATE` / `SAMPLE_OUT_OF_ORDER`. Primary UDP datagram-loss authority for sFlow v5 is the datagram header `sequence_number` scoped by authoritative exporter binding + agent address + `sub_agent_id` (+ collector/vantage where required) — not source IP alone and not flow/counter sample sequence numbers. Sample-sequence anomalies remain supplemental and MUST NOT mutate primary datagram-loss counts.

**Raw flow evidence (observed ≠ estimated).** Canonical raw facts MUST preserve OBSERVED values separately from derived estimates. Minimum logical fields: exporter/source identity; observation domain; interface/source vantage; source/destination addresses and ports; protocol; **observed** bytes; **observed** packets; flow count where applicable; event/start/end time; sampling mode; sampling algorithm; sampling selector/id where supplied; sampling rate/probability where known; direction `INGRESS | EGRESS | UNKNOWN`; application/classification identity nullable; application/classification provenance; quality/provenance. Do **not** fabricate direction or application. If direction cannot be derived from a certified observation point/interface relation ⇒ `direction=UNKNOWN`. If application classification is unavailable ⇒ classification remains `UNKNOWN` (not a guessed application).

**Sampling state.** Explicit sampling state is one of `UNSAMPLED | SAMPLED_KNOWN | SAMPLED_UNKNOWN`. Estimation is eligible only when the exporter/protocol supplies a valid certified sampling model (`SAMPLED_KNOWN` with usable algorithm/rate/probability/selector evidence). Unknown/unsupported sampling (`SAMPLED_UNKNOWN` or missing certified model) MUST NOT silently scale observed traffic. Never treat `SAMPLED_UNKNOWN` as `UNSAMPLED`. Never silently average different sampling rates/models.

**Sampling cohort identity.** Aggregation MUST identify a deterministic `sampling_cohort_id` for the effective sampling model comprising: `sampling_state`; `sampling_algorithm`; `sampling_selector_id`; sampling rate/probability; exporter + observation domain; model/version; validity window. `UNSAMPLED` has an explicit unsampled cohort. `SAMPLED_UNKNOWN` remains a distinct non-estimable cohort. Rollup keys MUST retain `sampling_cohort_id` (or equivalent fields sufficient to prevent incompatible models being irreversibly merged).

The template used for decoding must be valid for the exporter transport/session/observation-domain and event window. Data Templates and Options Templates share one Template ID namespace per Observation Domain/exporter scope. Under UDP, enforce refresh/redefinition semantics and **ignore UDP Template Withdrawal messages**. After collector restart, do not decode UDP Data Sets using an unvalidated persisted template; require appropriate live template validation/session continuity. Unknown, expired, conflicting or missing templates produce explicit loss/quality evidence rather than fabricated decoded flow rows. Sampling metadata preserves selector/algorithm, rate/interval/probability, scope, observation-domain identity and valid time window where available. Distinguish **sFlow sampled-frame evidence**, **sFlow interface/device counter samples**, and IPFIX/NetFlow flow counters: an sFlow Counter sample is not an ordinary per-flow `network_flow` fact and must use existing typed metric/device-counter evidence paths (no new database). sFlow sampled header **capture length** is not automatically original packet/frame octet length; preserve original frame length only where the protocol supplies it and it is qualified. Never interpret cumulative sFlow interface counters as per-flow `bytes`/`packets`, nor scale those counters by packet sample rate. Raw exported `bytes`/`packets` (or sFlow qualified sampled-frame length evidence) are immutable **observed** source facts and must not be silently overloaded with estimates; any sampling-normalized/estimated bytes, packets, rates or utilization use separately identified fields (`observed_bytes`/`observed_packets` retained; `estimated_bytes`/`estimated_packets` nullable; `estimate_method`; cohort/`sampling_breakdown`; quality), never one unlabeled total. No blanket multiplication or duplicate normalization is permitted. Authenticated Sensor→Core mTLS does not make unauthenticated exporter UDP data intrinsically trusted.

**Query/rollup semantics.** DAF-FLW (§15.7) and rollups MUST return/maintain observed aggregates separately from any estimates. Observed values MAY aggregate across sampling cohorts because they remain labelled observed source evidence. Estimated values may be computed only per certified `SAMPLED_KNOWN` cohort; no estimate is generated for `SAMPLED_UNKNOWN`. Do not irreversibly mix sampling cohorts before the query layer has enough information to disclose their semantics. Replay/dedupe occurs before rollup, or rollups are rebuilt/reconciled from deduped raw facts so replay does not double-count (CC-CH rollup rule).

**DAF-FLW aggregate result coverage.** A Flow aggregate result MUST expose conceptually:

```text
observed_bytes
observed_packets
estimated_bytes | null
estimated_packets | null
estimate_coverage: FULL | PARTIAL | NONE
sampling_breakdown[]
```

Each `sampling_breakdown[]` entry identifies: `sampling_cohort_id`; `sampling_state`; algorithm/model; selector; rate/probability where known; validity interval; observed bytes/packets; estimated bytes/packets nullable; `estimate_method` nullable; quality.

Rules:

- `FULL`: every contributing record/cohort has a valid certified estimate.
- `PARTIAL`: some contributing evidence is estimable and some is not.
- `NONE`: no valid estimate exists.
- Top-level `estimated_bytes`/`estimated_packets` may be populated ONLY for `FULL`.
- For `PARTIAL`, top-level estimate is null; cohort breakdown exposes the estimable portions without pretending they are whole-traffic totals.
- Observed totals always remain explicitly labelled observed.

## 14. Phase 1 / Phase 1.5 certification envelopes — Issue #200 / #203

Architecture target ≠ implementation ≠ tested ≠ benchmarked ≠ certified ≠ deployed. Mother Reference set version remains **3.3.0** through Tranche B; v3.4.0 is reserved for gated Tranche D. This document remains TRANSITIONAL_LEGACY except where BUILD-000 edge freeze applies.

| Envelope | API / event / schema emphasis | ACK / tenancy | Explicit nonclaims |
|---|---|---|---|
| **Phase 1** | Tenant-scoped APIs/events; Sensor/Agent mTLS identity; assignment/fencing generations; SNMP typed + ICMP typed observation contracts; `No Data != Device Down` | Standalone deletion-ACK durability matrix; tenant-aware contracts without MSP isolation cert claim | Issue #218 Core ICMP acceptance open; no Core-HA continuity claim; NCM automation not Phase 1 default |
| **Phase 1.5** | Same contracts under two-Core HA continuity and MSP adversarial isolation certification | `ACKED-FOR-DELETION TELEMETRY RPO = 0` for certified permanent one-Core HA failures | Physical HA mechanisms unselected; no new journal/witness/database invented by this contract note |

## Phase 1.5 Serverless API/event/data contracts — Issue #268

Governing requirements: `REQ-SLS-003`, `REQ-SLS-004`, `REQ-SLS-005`. Status: SPECIFIED / NOT IMPLEMENTED / NOT CERTIFIED.

### Logical entity contract

Tenant-scoped canonical entities for serverless hierarchy:

`cloud_account|subscription|project` → `cloud_region` → `serverless_service` → `serverless_function|serverless_run_service` → `serverless_version|revision|alias` → `serverless_trigger|event_source` → dependency targets.

Required common fields (logical): `tenant_id`, `entity_id`, provider identity attributes, lifecycle state, provenance/source, first_seen/last_seen, connector quality/freshness, Capability Pack/version references. Provider ARNs/IDs/names are attributes only.

### Observation / event classes

Normalize provider-exposed aggregate metrics, logs, events, change facts and connector-health observations into existing metric/state/event/log envelopes where possible. Do not invent metrics the provider does not expose. Do not force high-cardinality per-invocation labels. Optional governed OTel enrichment reuses `REQ-INF-010` / `REQ-OTEL-*` boundaries; OTel resource attributes never select tenant authority.

### Alert Evidence Contract note

Class-specific serverless operational assertions require provider/resource or corroborated workload evidence. Connector/API/auth/quota/stale/no-data produce monitoring/connector quality states only (`REQ-SLS-005`).

No new database or event bus is authorized; PostgreSQL/ClickHouse/Kafka/Valkey roles remain unchanged.

---

## Phase 2 Enterprise RFP Readiness Reconciliation — Issue #269

Phase-2 external-observability evidence contracts shall preserve:
`tenant_id, source_system_id, source_object_id, entity/binding reference, evidence_type, event_time, ingest_time, quality, provenance, correlation_id, schema/version, bounded payload/reference`.

External APM trace summaries, external NPM/DPI packet/flow metadata, SIEM/security facts, CMDB CIs/relationships and third-party monitoring events are evidence classes, not independent Platform identity or RCA authority. Raw external payload retention is bounded and policy-controlled.

No Phase-2 contract requires unlimited span storage or packet-payload warehousing; those physical models are Phase 2.5+ change-control items.

---

## Issue #272 data-contract planning

Phase-2 closure records SHALL carry source/product/version/collection-mode/provenance for certified enterprise integrations.

Phase 2.5 SHALL define canonical contracts for trace/span/transaction identity, service/resource/process/runtime identity, profile sample/stack/symbol references, RUM/mobile/session evidence with privacy classification, packet/capture-source/conversation/decoded-transaction identity, packet-to-flow/service/trace correlation references, redaction/privacy classification and retention/evidence-custody metadata.

Raw packet payload and session-replay data SHALL remain separately protected evidence classes rather than generic telemetry attributes.

---

## 2026-10-30 — Infrastructure/Log Sensor edge contracts (architecture target, not implemented truth)

This is additive logical contract guidance under existing REQ-INF/REQ-EMC/REQ-MIA and BUILD-016/062/063/064/098 ownership. **No current v1 wire schema, frozen ACK field, storage engine, new BUILD ID or Phase-1 MSP certification is changed.**

### Logical `LogIngressProfile` and source binding

A versioned server-authoritative ingress configuration declares `profile_id`, owning Sensor/site/tenant binding, transport and bound listener/interface/trusted route, source/client identity allow/rules, optional TLS trust and **verified client-certificate-to-profile mapping**, parser version, redaction/security policy, rate/size/cardinality quotas and an ambiguous-source handling policy. These are *logical fields* for configuration design; exact API keys and backing tables remain an implementation/contract-gate decision. Source-originated hostname, IP, message content or self-declared tenant is never Core tenant authority. Shared listeners are permitted only if mapping to one authorized tenant is deterministic; Phase-1.5 overlapping-private-IP/NAT tests must prove this. Unmatched/ambiguous input is counted and rejected or held as bounded **sanitized metadata only** without tenant-content contamination. A claimed mTLS identity is valid only if chain validation and server-side certificate/profile binding succeed.

Normalized accepted log/evidence records retain trusted tenant/Sensor/profile/source binding, transport/framing, observed/received timestamps, parser/redaction version/outcome, bounded body/attributes, source correlation, and explicit `GOOD|PARTIAL|UNKNOWN` (or compatible) quality. Incomplete, malformed, redacted, sampled or shed records never silently become complete evidence. Separate receiving, durable spool, Sensor→Core ingestion and external SIEM-forwarding outcomes; a TCP socket write is not a durable application-level ACK.

### Typed Agent evidence, compatibility, assembly and ACK truth

Existing scalar `HostCollector → Sample → Observation → CollectionBatch` is **current** and not overloaded with inventory/process tables. BUILD-016 may introduce additive versioned typed evidence families (software inventory snapshots, ProcessNetworkEvidence and EDS references) with size/item/source bounds and provenance. An evidence-kind discriminator, fragmentation/assembly and physical encoding are **TBD implementation choices** under normal schema review; there is no mandated new polymorphic union, Valkey/PG staging table or replacement of frozen wire. If fragmented, fragments share a stable capture identity and are retry/replay-idempotent; only an integrity-checked, fully assembled, complete authoritative FULL snapshot may authorize general absence/removal. Older/partial/truncated/failed/TARGETED snapshots remain non-destructive evidence.

Keep exact existing Platform_17 §10.3 ACK fields and boundaries: Agent→Sensor local durable spool; current Sensor→Core telemetry and Agent-ingress both reserve/claim fenced publisher lease/commit Kafka transaction/finalize PostgreSQL before ACK. Both remain at-least-once. This source-bound parity does not implement typed structured evidence or certify physical storage, runtime deployment or HA; those claims require their own exact-head evidence.

### OTLP series admission and SIEM export accounting

OTLP receiver budgets apply to exact tenant/source/metric-series identity and decoded resource cost **before spooling**, with deterministic oversize/invalid/overbudget rejection and protocol-correct `partial_success.rejected_data_points` / `rejected_log_records` where supported. Preserve Resource/Scope/attributes, Sum monotonicity, temporality and histogram semantics; **never** make a rejected series into a Gauge, silently merge distinct series after stripping identity dimensions or retain excluded PII in an unindexed raw JSON overflow column. Estimates/sketches may inform a pre-admission budget but cannot prove exact new-series membership by themselves. Accounting separates accepted, rejected, sampled/shed and upstream continuity gaps; no sensitive values appear in reason details/metrics.

An outbound SIEM destination has independently bounded tenant-scoped queue/backlog/retry/dead-letter and terminal delivered-or-expired status, with message/batch identity and acknowledged delivery recorded only when the sink protocol actually supplies it. Destination status does not change primary Core ingest ACK. After expiry/quota exhaustion, expose irreversible export loss and relevant evidence uncertainty; do not claim lossless forwarding, infinite legal-hold quarantine, or "100% unaffected" shared resource operation.

## 15. Physical design contracts — PostgreSQL, ClickHouse, API operations, events and migration plan

**Status:** `SPEC_CONTRACT=FROZEN` (effective on independent exact-head acceptance) · `IMPLEMENTATION=PENDING` · every table, topic, route and migration below is **`SPECIFIED_NOT_IMPLEMENTED`**. Current physical authority remains §4.7 (`V001–V018`, `deploy/docker/clickhouse/init.sql`, existing controllers). This section does **not** create SQL files, route code or manifest entries and does **not** select a V019+ filename: migration numbers are assigned at merge time from the then-current `manifest.txt` tail. Plans are identified as `MIG-PLAN-*`. No new service, database, queue or OSS dependency is introduced (PostgreSQL = control state; ClickHouse = history; Kafka = internal transport; Valkey = cache only).

### 15.1 Conventions

**CC-PG (applies to every PostgreSQL table unless stated).** Schema `platform`. `tenant_id UUID NOT NULL REFERENCES platform.tenant` (existing pattern); PK `<x>_id UUID`; `UNIQUE (tenant_id, <x>_id)` to enable composite same-tenant FKs (`FOREIGN KEY (tenant_id, y_id) REFERENCES … (tenant_id, y_id)`, as V001/V003). Mutable aggregates add `version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1)`. Timestamps `TIMESTAMPTZ NOT NULL` (`created_at`, `updated_at`). Actor columns `created_by/updated_by VARCHAR(128)` = authenticated principal id. `correlation_id VARCHAR(128) NULL`. State columns `VARCHAR(32) NOT NULL` with a named `CHECK … IN (…)`. JSONB columns are bounded by `CHECK (octet_length(col::text) <= N)` (N owned by the table's BUILD; V003 uses 32768 for attributes). Secrets/keystrokes/one-time credentials are prohibited columns everywhere. Deletes of governed objects are soft (state) unless stated. Same-tenant integrity is a DB constraint, not application-only. Table text below omits these CC-PG columns; `+CC` marks "all CC-PG columns apply".

**CC-IDEM.** `platform.api_idempotency_record(tenant_id, principal_id VARCHAR(128), operation VARCHAR(64), idempotency_key VARCHAR(128), request_hash BYTEA NOT NULL, status VARCHAR(16) CHECK IN ('IN_PROGRESS','COMPLETED'), response_ref TEXT NULL, resource_type, resource_id UUID NULL, expires_at TIMESTAMPTZ NOT NULL, PRIMARY KEY (tenant_id, principal_id, operation, idempotency_key))`; expiry sweep by housekeeping; retention config `api.idempotency.retention` (`NUMERIC_BUDGET_OWNER=Platform_11/BUILD-130`).

**CC-OUTBOX.** `platform.event_outbox(event_id UUID PK, tenant_id, topic VARCHAR(96), schema_version VARCHAR(16), partition_key VARCHAR(256), payload JSONB (size-bounded), event_time TIMESTAMPTZ, created_at, state IN ('PENDING','PUBLISHED','DEAD'), attempts INT, not_before TIMESTAMPTZ)`; written in the same transaction as the state change; publisher marks `PUBLISHED` after Kafka acknowledgement; index `(state, not_before)`.

**CC-CH (ClickHouse).** Database `platform` (existing). `tenant_id String` first in `ORDER BY` (existing convention); `event_time DateTime64(3,'UTC')` (when it happened at the source) and `ingest_time DateTime64(3,'UTC')` (Core acceptance) are separate columns; durable logical identity (`sequence_id`/`event_id`/`tenant_seq`) is part of a physically replay-safe replacement/dedupe key — **`ORDER BY` alone does not create uniqueness** and MergeTree must not be claimed unique merely because `tenant_seq` appears in `ORDER BY`. Replay/backfill-sensitive history tables use `ReplacingMergeTree` (or an explicit rebuild/dedupe path) with readers on a canonical `FINAL` view (existing `metric_numeric` pattern); append-only MergeTree is permitted only when an explicit rebuild/dedupe contract is stated for that table. `LowCardinality(String)` for bounded enums; quality column `UInt8`; TTL follows the data's retention class (Platform_11 classes; numeric days = `NUMERIC_BUDGET_OWNER=Platform_11`); partition granularity chosen by volume (daily for flow/log/trap, monthly otherwise). **Rollup rule:** materialized views into `AggregatingMergeTree`/`SummingMergeTree` may consume only already-deduped raw inputs (or run as rebuildable reconcile jobs after replay). Irreversible incremental aggregation that would double-count under Kafka/PostgreSQL replay is forbidden; after a raw-table replay or backfill, dependent rollups are rebuilt/reconciled from the deduped raw authority before serving analytical truth. Per-tenant cardinality budgets enforced at ingestion (cardinality-bound column in each table below). ClickHouse is never control authority.

**CC-API.** Base `/api/v1/tenants/{tenantId}/…` (existing controller convention). Every POST requires `Idempotency-Key`; mutable-resource writes require `If-Match` (G-03/G-04 in `Platform_06` §119.0); every response carries `X-Correlation-Id`; errors use §10.9; list endpoints use cursor/keyset pagination via **CC-CURSOR** (existing paged APIs: default 50, max 200 — other maxima belong to `Platform_11`/BUILD-130); retry is safe only with the same idempotency key. Logical routes below are the canonical names the generated OpenAPI must emit (§3.2.1 requires OpenAPI to carry exact strings); changing one needs contract change control.

**CC-ASYNC (shared `async_operation` admission vs lifecycle — maps `Platform_06` G-05).** HTTP `202` means admission outcome `ACCEPTED_FOR_ASYNC_EXECUTION` only; it is **not** a persisted lifecycle state. A `202` response SHALL include `operation_id`, persisted initial `state=QUEUED`, correlation id, and status URL/ref where applicable. Persisted shared lifecycle for `async_operation` is exactly `QUEUED → RUNNING → SUCCEEDED | PARTIAL | FAILED | CANCELLED | TIMED_OUT`. `SUCCEEDED` is the persisted successful terminal state; UI MAY render it as **Completed**, but `COMPLETED` is not a second persisted `async_operation` state. Cancellation request sets `cancel_requested=true` while state remains `QUEUED` or `RUNNING`; only executor confirmation transitions to `CANCELLED`. Domain-specific machines (`platform_operation`, NCM, report/storage kinds with their own enums) keep their governed vocabularies.

**CC-CURSOR (Query Service cursor envelope — BUILD-067).** List/query pages return an opaque server-generated cursor token. Clients MUST treat the token as opaque; client modification is invalid. Do **not** add a growing `seen_ids` list to cursors. Do **not** invent heavyweight server session state merely to satisfy UI pagination. Two explicit consistency modes are frozen:

###### `LIVE_KEYSET` (default for ordinary operational UI lists)

Cursor binds (integrity-protected inside the token; not client-editable JSON):

| Binding field | Requirement |
|---|---|
| `cursor_schema_version` | envelope version (initial `"1.0"`) |
| `query_operation_id` | owning list/query operation identity |
| `tenant_id` | issuing tenant |
| `filter_fingerprint` | normalized filter/query fingerprint |
| `resource_scope_fingerprint` | authorized resource-scope fingerprint at issue |
| `sort_definition` | requested sort keys |
| `last_sort_tuple` | last returned sort key values |
| `tie_break_resource_id` | canonical unique resource identifier of the last row |
| `issued_at` / `expires_at` | issue and expiry timestamps |
| `consistency_mode` | literal `LIVE_KEYSET` |

Guarantees:

- Stable dataset ⇒ no duplicates or omissions across pages.
- Keyset order uses requested sort keys + immutable unique resource ID tie-break.
- Each page re-authorizes; the cursor never grants authorization.
- Filter/scope/sort/token mutation fails closed (`CURSOR_INVALID` / `CURSOR_SCOPE_CHANGED`).
- Concurrent insert/update/delete: a row whose sort tuple changes during paging MAY move before/after the cursor and therefore MAY be omitted or reappear. This is explicitly LIVE/evolving-list behavior, **not** snapshot consistency.
- UI MAY expose refresh/new-data indication where applicable.
- `LIVE_KEYSET` MUST NOT be used as an implicit closed "select all pages" membership set.
- Destructive bulk operation uses explicit IDs or a separately frozen server-side selection/snapshot contract.
- Remove any reading that promises mutation-proof no-duplicate behavior in `LIVE_KEYSET` mode — that guarantee is not implementable for arbitrary mutable sort keys with ordinary stateless keyset pagination without snapshotting or unbounded seen-row tracking.

###### `SNAPSHOT`

Use only where the owning query/storage contract can supply a stable immutable snapshot/`as_of` revision. Cursor additionally binds:

| Binding field | Requirement |
|---|---|
| `consistency_mode` | literal `SNAPSHOT` |
| `snapshot_ref` or authoritative `as_of` boundary | required; omitted ⇒ query MUST NOT advertise SNAPSHOT |

Guarantees within that snapshot:

- Stable membership; stable sort values; no duplicates; no omissions across pages.

If the owner cannot provide a real snapshot, the query MUST NOT advertise SNAPSHOT consistency. Expired/unavailable snapshot produces `CURSOR_EXPIRED` or `CURSOR_STALE`. Not every query is required to support SNAPSHOT.

Stable cursor errors (both modes): `CURSOR_INVALID`, `CURSOR_EXPIRED`, `CURSOR_STALE`, `CURSOR_SCOPE_CHANGED`. Permission/resource-scope change between pages MUST fail closed or re-evaluate safely and MUST NOT expose rows no longer authorized. Measured scale remains BUILD-130 — this envelope invents no capacity number.

**CC-EVT.** Topics follow `<domain>.<family>.v<major>` (existing `telemetry.numeric.v1`). Common envelope (§3.4): `schema_version, tenant_id, source identity, event_id (UUID) + sequence where ordered, event_time, ingest_time, quality/provenance, correlation_id, payload`. Physical topic/partition/serialization selection is BUILD-063; names below are logical. Breaking change = new major topic + migration window (§3.5). Secrets, one-time credentials and keystrokes never appear.

### 15.2 PostgreSQL planned schemas (`SPECIFIED_NOT_IMPLEMENTED`)

Legend: NN = NOT NULL, N = nullable. "Idx" = indexes. "Ver/Idem/Conc" = versioning, idempotency, concurrency. "Ret/Aud" = retention class and audit. "Mig" = owning migration plan.

#### Platform substrate

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `api_idempotency_record` | see CC-IDEM | PK per CC-IDEM | `(expires_at)` | n/a | short TTL sweep / none | MIG-PLAN-001 |
| `event_outbox` | see CC-OUTBOX | PK event_id | `(state,not_before)`,`(tenant_id,topic)` | publisher lease `FOR UPDATE SKIP LOCKED` | purge after PUBLISHED per ops class / none | MIG-PLAN-001 |
| `async_operation` +CC | operation_id UUID NN; kind VARCHAR(48) NN (ONBOARDING_BULK_DECIDE/PROVISIONING_IMPORT/ALERT_BULK/REPORT_RUN/AUDIT_EXPORT/STORAGE_ACTION/INTEGRATION_TEST/… bounded allowlist); state VARCHAR(16) NN CHECK IN (QUEUED,RUNNING,SUCCEEDED,PARTIAL,FAILED,CANCELLED,TIMED_OUT) (**persisted lifecycle only** — HTTP `202` admission = `ACCEPTED_FOR_ASYNC_EXECUTION`, not a state; `COMPLETED` is not persisted here); requested_by NN; resource_scope JSONB N; total_count/success_count/failed_count/skipped_count INT N; cursor JSONB N (resume); result_ref VARCHAR(512) N; error_code VARCHAR(48) N; cancel_requested BOOL NN DEFAULT false (request flag while QUEUED/RUNNING; `CANCELLED` only after executor confirmation); started_at/ended_at/expires_at N | PK; partial UNIQUE (tenant_id, kind, (resource_scope->>'batch_id')) WHERE state IN ('QUEUED','RUNNING') AND resource_scope ? 'batch_id' | `(tenant_id,state,created_at)`,`(tenant_id,kind,created_at DESC)` | version; CC-IDEM on create; cancel via If-Match sets `cancel_requested` | ops class (retention `api.operation.retention`, BUILD-130) / audit create/cancel/complete | MIG-PLAN-001 |
| `audit_event` +CC (append-only; **Phase-1 non-partitioned**) | audit_id UUID NN; tenant_seq BIGINT NN; occurred_at TIMESTAMPTZ NN; actor_type VARCHAR(16) NN (USER/SERVICE/SYSTEM); actor_id VARCHAR(128) NN; action VARCHAR(96) NN; resource_type VARCHAR(48) NN; resource_id VARCHAR(128) N; resource_version BIGINT N; result VARCHAR(16) NN CHECK IN ('SUCCESS','DENIED','FAILED'); reason VARCHAR(1000) N; before_hash/after_hash BYTEA N; detail JSONB N (bounded, no secrets); integrity_ref VARCHAR(256) N (reserved for tamper-evidence; construction deferred to architecture/security review) | **PRIMARY KEY (tenant_id, audit_id)**; **UNIQUE (tenant_id, tenant_seq)**; CHECK result IN ('SUCCESS','DENIED','FAILED') | `(tenant_id, occurred_at DESC)`,`(tenant_id, resource_type, resource_id)`,`(tenant_id, actor_id, occurred_at)` | immutable: `REVOKE UPDATE, DELETE`; append-only / n/a | **non-partitioned** Phase-1 PostgreSQL table; retention class AUDIT (legal-hold aware) / self. Scalable analytical/history access is ClickHouse `audit_history` (derived). PostgreSQL partitioning may be reconsidered only under BUILD-130 benchmark/migration evidence; do not invent a second audit authority. | MIG-PLAN-001 (BUILD-073 completes durability/export) |
| `platform_operation` +CC | operation_id UUID NN; kind VARCHAR(24) NN (BACKUP/RESTORE/UPGRADE/ROLLBACK/RETENTION_APPLY/PURGE/HOUSEKEEPING); scope VARCHAR(128) NN; state VARCHAR(24) NN (REQUESTED/PREFLIGHT/RUNNING/VERIFYING/COMPLETED/FAILED/CANCELLED/ROLLING_BACK/ROLLED_BACK/ROLLBACK_FAILED/ROLLBACK_UNSUPPORTED); requested_by NN; approver_id N; artifact_ref VARCHAR(512) N; artifact_hash BYTEA N; plan JSONB N; result JSONB N; started_at/ended_at N | PK; CHECK state/kind; partial UNIQUE (tenant_id, scope) WHERE state IN ('REQUESTED','PREFLIGHT','RUNNING','VERIFYING','ROLLING_BACK') (one active op per scope) | `(tenant_id,kind,state)` | version+If-Match; key via CC-IDEM | ops-history class / audit each transition | MIG-PLAN-012 |
| `platform_operation_step` | step_id UUID NN; operation_id NN; seq INT NN; name VARCHAR(64) NN; state VARCHAR(16) NN (PENDING/OK/FAILED/SKIPPED); detail JSONB N; started_at/ended_at N | PK; FK (tenant_id, operation_id); UNIQUE (operation_id, seq) | `(operation_id,seq)` | idempotent step resume by seq | with parent | MIG-PLAN-012 |

#### Inventory/onboarding extensions (reconciles V001/V003 vocabulary)

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `provisioning_request` +CC | request_id UUID NN; source VARCHAR(16) NN (MANUAL/BULK/ONBOARDING); state VARCHAR(16) NN (DRAFT/VALIDATING/PREFLIGHT/READY/PROVISIONING/ACTIVE/FAILED/CANCELLED/CANCEL_REQUESTED); entity_id UUID N; candidate_id UUID N; site_id UUID N; input JSONB NN (bounded; secret-free; credential_profile_ref only); assignment_mode VARCHAR(24) NN CHECK IN (AUTO_FROM_CRITICALITY,EXPLICIT_PROFILE); attempt_no INT NN; failure_class VARCHAR(32) N; batch_id UUID N | PK; FK entity/site/candidate same-tenant | `(tenant_id,state)`,`(tenant_id,batch_id)` | version/If-Match; CC-IDEM | provisioning-history class / `entity.provision.*` | MIG-PLAN-002 |
| `onboarding_candidate` +CC | per Platform_20 §6.1 (candidate_id, source_type, source_ref, entity_id N, agent_id N, lifecycle_state, identity_conflict_state, discovered_name, proposed_* columns, first/last_seen_at, revision BIGINT NN, dedupe_key VARCHAR(256) NN) | PK; UNIQUE (tenant_id, dedupe_key); CHECK lifecycle_state IN (DISCOVERED,ENROLLED,PENDING_APPROVAL,PROVISIONING,ACTIVE,UNMANAGED,REJECTED,EXCLUDED,DECOMMISSION_REQUESTED,DECOMMISSIONED,RETIRED); CHECK identity_conflict_state IN (NONE,REVIEW_REQUIRED,SECURITY_HOLD) | `(tenant_id,lifecycle_state)`,`(tenant_id,source_type,last_seen_at)` | `revision` is the concurrency token (If-Match); decisions via CC-IDEM | candidate-history class / `onboarding.*` | MIG-PLAN-002 |
| `onboarding_decision` (append-only) | decision_id UUID NN; candidate_id NN; decision VARCHAR(12) NN (APPROVE/UNMANAGED/REJECT/EXCLUDE/REOPEN); actor_id NN; reason VARCHAR(1000) NN; before_revision/after_revision BIGINT NN; effective JSONB N (group/profile/policy/assignment); result VARCHAR(16) NN (APPLIED/FAILED/CONFLICT); batch_id N; decided_at NN | PK; FK candidate same-tenant | `(candidate_id,decided_at)`,`(tenant_id,batch_id)` | idempotency via CC-IDEM + UNIQUE (candidate_id, before_revision, decision) | permanent with candidate / self | MIG-PLAN-002 |
| `entity_agent_binding` +CC | entity_id NN; agent_id VARCHAR(128) NN; certificate_identity VARCHAR(256) NN; system_name VARCHAR(255) NN; os_identity JSONB N; binding_state VARCHAR(20) NN CHECK IN (PENDING_APPROVAL,ACTIVE,UNMANAGED,REVOKED,SECURITY_HOLD); reinstall_generation INT NN; first_enrolled_at, last_heartbeat_at | PK (tenant_id, entity_id); UNIQUE (agent_id); UNIQUE (certificate_identity) (deployment-wide per Platform_20 §6.3) | `(tenant_id,binding_state)` | version; reinstall only through explicit reconcile | binding class / `agent.*` | MIG-PLAN-002 |
| `entity` (existing V001) | **change:** expand `chk_entity_lifecycle` to the Platform_20/MIG-PLAN-002 vocabulary (adds PROVISIONING, UNMANAGED, DECOMMISSION_REQUESTED, DECOMMISSIONED; keeps DISCOVERED, ACTIVE, QUIESCED, RETIRED) using the §15.6 MIG-PLAN-002 executable CHECK cutover sequence — **do not** leave dual restrictive+superset CHECKs as the steady state | existing | existing | existing | existing | MIG-PLAN-002 (forward-only; sequenced CHECK cutover) |
| `discovery_candidate` (existing V003) | **change:** expand `chk_candidate_review_status` to the Platform_20 vocabulary via the same §15.6 MIG-PLAN-002 CHECK cutover sequence. **Backfill mapping (exact):** `PENDING_REVIEW`→`PENDING_APPROVAL`; `APPROVED_FOR_PROVISIONING`→`PROVISIONING` when no activated `entity_id` / provisioning not yet ACTIVE, else `ACTIVE` when linked entity lifecycle is ACTIVE (or provisioning_request.state=`ACTIVE`); `LEFT_UNMANAGED`→`UNMANAGED`; `IGNORED`→`REJECTED`; `EXCLUDED_PERMANENTLY`→`EXCLUDED`. Rows already in Platform_20 vocabulary pass through unchanged | existing | existing | existing | existing | MIG-PLAN-002 |

#### Policies, profiles, maintenance

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `alert_policy` +CC | policy_id NN; name VARCHAR(160) NN; evidence_class VARCHAR(48) NN; scope JSONB NN (bounded); lifecycle_state VARCHAR(12) NN CHECK IN (DRAFT,BACKTEST,SHADOW,ACTIVE,RETIRED); current_version_no INT N | PK; UNIQUE (tenant_id,name) | `(tenant_id,lifecycle_state)` | version/If-Match | policy class / `policy.*` | MIG-PLAN-003 |
| `alert_policy_version` (immutable) | policy_id NN; version_no INT NN; **definition JSONB NN (bounded) = frozen `AlertPolicyDefinitionV1` below**; evidence_contract_ref VARCHAR(128) NN; content_hash BYTEA NN; state_at_creation VARCHAR(12) NN; backtest_ref/shadow_ref N; activated_at/by N | PK (tenant_id, policy_id, version_no); FK policy same-tenant; CHECK definition validates as `AlertPolicyDefinitionV1` at write time | `(tenant_id,policy_id)` | immutable; content_hash dedupes identical versions; never mutate definition in place | permanent / `policy.version.*` | MIG-PLAN-003 |

##### `AlertPolicyDefinitionV1` (frozen logical JSON schema; owned by this document; evaluation semantics owned by `Platform_22` §6.1)

`alert_policy_version.definition` SHALL store exactly one versioned `AlertPolicyDefinitionV1` document. `Platform_22` §6.1 owns threshold **evaluation** semantics (including tri-state TRUE/FALSE/UNKNOWN, Counter64 rate authority, GROUP membership evidence and business-time calendar behavior); this schema owns the persisted/authoring shape. Arbitrary executable code, SQL, JavaScript, Python, regex-based estate selectors or opaque free-form predicate/expression strings are **forbidden**. Server-side validation returns typed field/path errors and **never** silently normalizes an invalid definition.

```text
AlertPolicyDefinitionV1 {
  schema_version: "1.0"                         # required literal
  subject_selector: SubjectSelectorV1            # inventory/entity/metric/monitor scope
  evidence_class: string                         # class for Alert Evidence Contract binding
  root_condition: ThresholdExprV1                # required
  recovery_condition: ThresholdExprV1 | null     # nullable clear/recovery expression
  severity_mapping: SeverityMappingV1            # maps evaluation outcomes → severity
  evaluation_cadence: EvaluationCadenceV1        # tick/schedule parameters
  evidence_contract_ref: string                  # required; versioned evidence-contract identity
  metadata: object                               # bounded, non-authoritative labels/notes only
}
```

Business-time gating MUST NOT be represented by a root calendar field. Schema V1 forbids `business_calendar_ref` on `AlertPolicyDefinitionV1`; all business-time / blackout gating uses an explicit `BUSINESS_TIME` `ThresholdExprV1` node (`BusinessTimeThresholdV1`).

###### `SubjectSelectorV1`

```text
SubjectSelectorV1 {
  scope_kind: TENANT | REGION | SITE | GROUP | SERVICE | ENTITY
  scope_id: UUID | null
  metric_ref: string | null
  monitor_ref: string | null
  include_descendants: boolean
}
```

Validation:

- `scope_id` MUST be null only for `scope_kind=TENANT`; every other `scope_kind` requires a canonical same-tenant `scope_id`.
- At least one applicable metric/evidence input MUST ultimately be referenced by the threshold expression tree (via `metric_ref` / `monitor_ref` and/or descendant `input_ref` values).
- `metric_ref` and `monitor_ref` are canonical governed catalogue references, not arbitrary query/expression text.
- No embedded SQL, regex-based estate selector, JavaScript, Python or free-form predicate language.
- Server validates referenced scope/metric/monitor is authorized and belongs to the policy tenant.

###### `SeverityMappingV1`

```text
SeverityMappingV1 {
  on_true: INFO | WARNING | MINOR | MAJOR | CRITICAL
  on_false: NO_FIRE | CLEAR
  on_unknown: NO_FIRE | PRESERVE_ACTIVE
}
```

`UNKNOWN` never automatically becomes a firing state. Severity remains separate from evidence/causal confidence (`Platform_22`).

###### `EvaluationCadenceV1`

```text
EvaluationCadenceV1 {
  mode: ON_EVIDENCE | FIXED_INTERVAL
  interval_seconds: uint32 | null
  max_staleness_seconds: uint32
}
```

Rules:

- `FIXED_INTERVAL` requires `interval_seconds > 0`.
- `ON_EVIDENCE` requires `interval_seconds = null`.
- `max_staleness_seconds > 0`.
- Exact capacity/estate scheduling ceilings remain owned by `Platform_11` / BUILD-130 — do not invent a commercial/performance number here.
- Cadence is not a wall-clock business calendar. All business-time gating in schema V1 MUST use an explicit `BUSINESS_TIME` `ThresholdExprV1` node (`BusinessTimeThresholdV1`). Root-level calendar fields are forbidden.

###### `CalendarRefV1`

```text
CalendarRefV1 {
  calendar_id: UUID
  version_no: uint32
}
```

Rules:

- `version_no >= 1`.
- Calendar is same-tenant and immutable/version-addressed.
- IANA timezone identity and calendar contents remain owned by the business-calendar contract (`Platform_22` §6.1.10).
- Configurable DST gap/fold policy choices for a threshold are persisted on `BusinessTimeThresholdV1` (not left as unwritten implementation defaults).
- Evaluation evidence records the exact calendar version used.

###### `ComparisonV1` (reusable persisted comparison)

```text
ComparisonV1 {
  operator: GT | GE | LT | LE | EQ | NE | INSIDE | OUTSIDE
  threshold: number
  threshold_high: number | null
  lower_inclusive: boolean | null
  upper_inclusive: boolean | null
  unit: string
}
```

Validation:

- `INSIDE` / `OUTSIDE` require non-null `threshold_high`, non-null `lower_inclusive` and non-null `upper_inclusive`, and `threshold < threshold_high`.
- Non-range operators require `threshold_high = null`, `lower_inclusive = null` and `upper_inclusive = null`.
- `unit` MUST match the governed input metric unit unless an explicit versioned unit-conversion contract is referenced by the policy binding.
- Inclusive/exclusive bound flags close `Platform_22` §6.1.2 range semantics; writers MUST NOT invent bound openness.

###### `BaselineRefV1` / `PolicyVersionRefV1`

```text
BaselineRefV1 {
  baseline_id: string
  version_no: uint32
}

PolicyVersionRefV1 {
  policy_id: UUID
  version_no: uint32
}
```

Rules:

- `baseline_id` + `version_no` address an immutable published baseline artifact (`version_no >= 1`).
- `PolicyVersionRefV1` addresses an immutable same-tenant alert-policy version (`version_no >= 1`).
- Baseline promotion/update MUST create a new baseline version; it MUST NOT silently mutate an existing policy version or an already-referenced baseline version.

###### `SemanticStateV1`

```text
SemanticStateV1 =
  UP | DOWN | DEGRADED | UNKNOWN | NO_DATA | STALE |
  MONITORING_DEGRADED | REDUNDANCY_LOST | SLA_DEGRADED | EXPECTED_STATE
```

`SemanticStateV1` reuses the existing canonical Platform semantic-state vocabulary already persisted on `alert_current.semantic_state` / governed by `Platform_22` §5. It MUST NOT invent a second state model.

###### `DependencyRefV1`

```text
DependencyRefV1 {
  relationship_id: UUID
  min_confidence: LOW | MEDIUM | HIGH
  max_age_seconds: uint32
}
```

Rules:

- `relationship_id` is the canonical temporal-operational-graph / topology relationship identity for the policy tenant. Writers MUST NOT invent alternate free-form lookup keys, SQL selectors or opaque strings.
- Qualified dependency evidence MUST satisfy all of: canonical relationship identity; confidence ≥ `min_confidence`; freshness within `max_age_seconds`; and current `as_of` graph authority.
- Resolution uses Platform topology/dependency authority at the evaluation `as_of` instant.
- `max_age_seconds > 0`. Stale, low-confidence, No Data, or otherwise unqualified dependency evidence remains `UNKNOWN` / no dependency action per `Platform_22` §6.1.9 — never proof of child technical health/failure and never activation of `SUPPRESS`/`DEFER`.

###### `DependencyActivationV1`

```text
DependencyActivationV1 {
  semantic_states: SemanticStateV1[]
  maintenance_active: boolean
  combine: ANY | ALL
}
```

Rules:

- The semantic-state criterion is ENABLED when `semantic_states` is non-empty. It MATCHES when the current qualified dependency semantic state is a member of `semantic_states`.
- The maintenance criterion is ENABLED when `maintenance_active = true`. It MATCHES when the dependency is currently under qualified active maintenance according to the authoritative maintenance contract.
- At least one criterion MUST be enabled. An empty `semantic_states` array with `maintenance_active = false` is invalid.
- `combine` is required and MUST be persisted explicitly as `ANY` or `ALL`. Writers MUST NOT invent an unpersisted default.
- `ANY` — activation is TRUE when at least one ENABLED criterion matches.
- `ALL` — activation is TRUE only when every ENABLED criterion matches.
- If only one criterion is enabled, `combine` MUST be `ANY`. `ALL` with only one enabled criterion is invalid (no meaningful conjunction).
- No hidden/default combination behavior is permitted.
- `semantic_states` values MUST be members of `SemanticStateV1` (canonical vocabulary only).
- Before either combination rule is evaluated, dependency evidence MUST satisfy canonical relationship identity, `min_confidence`, freshness/`max_age_seconds`, and current `as_of` graph authority (`DependencyRefV1`). Stale, missing, low-confidence or unqualified dependency evidence ⇒ `UNKNOWN` / no dependency action and cannot activate `SUPPRESS`/`DEFER` irrespective of match fields.
- The combination operator determines only whether dependency activation is TRUE. Once TRUE, persisted `DependencyThresholdV1.action ∈ {SUPPRESS, DEFER, ANNOTATE}` governs behavior under `Platform_22` §6.1.9; action semantics are not redefined here.

**Finite activation fixtures (schema/evaluation contract):**

1. semantic criterion only matches (maintenance disabled) ⇒ activation TRUE when `combine=ANY` (required);
2. maintenance criterion only matches (`semantic_states` empty) ⇒ activation TRUE when `combine=ANY` (required);
3. both criteria enabled and both match ⇒ TRUE for `ANY` and for `ALL`;
4. both criteria enabled and neither matches ⇒ FALSE (not UNKNOWN when evidence is qualified);
5. `combine=ANY`, both enabled, only semantic matches ⇒ TRUE;
6. `combine=ANY`, both enabled, only maintenance matches ⇒ TRUE;
7. `combine=ALL`, both enabled, only one matches ⇒ FALSE;
8. `combine=ALL`, both enabled, both match ⇒ TRUE;
9. `combine=ALL` configured with only one enabled criterion ⇒ validation failure;
10. stale/unqualified dependency evidence ⇒ `UNKNOWN` / no dependency action irrespective of match fields.

###### `ThresholdExprV1` (discriminated union)

```text
ThresholdExprV1 = discriminated union on kind ∈ {
  STATIC | DURATION | CONSECUTIVE | RATE | BASELINE | MISSING |
  COMPOSITE | GROUP | DEPENDENCY | BUSINESS_TIME
}
```

Every variant carries a stable `expr_id` (string) unique within the policy version for evidence/audit references. Each `kind` contains **only** its frozen fields below. For every configurable behavior required by `Platform_22` §6.1, either (1) the exact choice is persisted in `AlertPolicyDefinitionV1` / `ThresholdExprV1`, or (2) it is represented by an exact immutable versioned reference whose contract owns that choice. There MUST be no implementation-significant default that an engineer invents at runtime. `Platform_22` §6.1 remains evaluation authority; this schema owns the persisted shape only.

**STATIC — `StaticThresholdV1`**

```text
StaticThresholdV1 {
  expr_id: string
  kind: STATIC
  input_ref: string
  fire: ComparisonV1
  clear: ComparisonV1 | null
}
```

Rules:

- `clear = null` means ordinary FALSE evaluation of `fire` clears the condition (no hidden hysteresis number).
- Non-null `clear` is the explicit hysteresis/clear comparison (`Platform_22` §6.1.2).
- `fire.unit` and `clear.unit` (when present) MUST be compatible with each other and with the governed `input_ref` metric.
- No unpersisted default clear band, percentage or absolute delta is permitted.

**DURATION — `DurationThresholdV1`**

```text
DurationThresholdV1 {
  expr_id: string
  kind: DURATION
  child: ThresholdExprV1
  duration_seconds: uint32
  unknown_behavior: PAUSE | RESET
  unknown_grace_seconds: uint32
}
```

Validation / semantics (persisted representation of `Platform_22` §6.1.3):

- `duration_seconds > 0`.
- `unknown_grace_seconds >= 0`.
- `RESET` requires `unknown_grace_seconds = 0`.
- `PAUSE` preserves elapsed TRUE duration only while the UNKNOWN/stale interval is `<= unknown_grace_seconds`; if grace is exceeded, duration resets.
- Qualified FALSE always resets. No unpersisted default UNKNOWN behavior.

**CONSECUTIVE — `ConsecutiveThresholdV1`**

```text
ConsecutiveThresholdV1 {
  expr_id: string
  kind: CONSECUTIVE
  child: ThresholdExprV1
  required_true_count: uint32
  unknown_behavior: RESET | SKIP
}
```

Rules:

- `required_true_count >= 1`.
- `RESET` — UNKNOWN/stale resets the consecutive count to zero.
- `SKIP` — UNKNOWN/stale does not increment and preserves the current count.
- Qualified FALSE always resets.
- No implicit default is left to implementation (`Platform_22` §6.1.4).

**RATE — `RateThresholdV1`**

```text
RateThresholdV1 {
  expr_id: string
  kind: RATE
  input_ref: string
  comparison: ComparisonV1
  minimum_interval_seconds: uint32
}
```

Rules:

- `minimum_interval_seconds > 0`.
- Exact Counter64 delta/reset/wrap/discontinuity processing occurs before floating-point rate derivation (`Platform_22` §6.1.5). Authoritative Counter64 input MUST never be converted through Float64 before delta calculation.
- `comparison.unit` is the rate unit. Do **not** add a separate ambiguous `unit_per_second` plus standalone threshold-operator fields; `ComparisonV1` is authoritative.
- Rate `ComparisonV1` uses non-range operators only in V1 (`GT`/`GE`/`LT`/`LE`/`EQ`/`NE`); `INSIDE`/`OUTSIDE` on RATE are rejected.

**BASELINE — `BaselineThresholdV1`**

```text
BaselineThresholdV1 {
  expr_id: string
  kind: BASELINE
  input_ref: string

  baseline_ref: BaselineRefV1

  reference_window_seconds: uint32
  aggregation: MEAN | MEDIAN | MIN | MAX | P50 | P95 | P99
  seasonality_key: string | null
  minimum_qualified_samples: uint32

  deviation_mode: ABSOLUTE | RELATIVE_PERCENT | SIGMA_MULTIPLE
  comparison: ABOVE | BELOW | OUTSIDE_BAND
  deviation_threshold: number
  deviation_threshold_high: number | null
  deviation_unit: string | null

  max_baseline_age_seconds: uint32
  stale_behavior: UNKNOWN | USE_FALLBACK_POLICY
  fallback_policy_ref: PolicyVersionRefV1 | null
}
```

Validation (aligns exactly with `Platform_22` §6.1.6):

- `baseline_ref.version_no >= 1`.
- `reference_window_seconds > 0`.
- `minimum_qualified_samples >= 1`.
- `max_baseline_age_seconds > 0`.
- `OUTSIDE_BAND` requires non-null `deviation_threshold_high` with `deviation_threshold < deviation_threshold_high`; other comparisons require `deviation_threshold_high = null`.
- `ABSOLUTE` requires non-null `deviation_unit` compatible with `input_ref`.
- `RELATIVE_PERCENT` uses percentage semantics and requires `deviation_unit = null`.
- `SIGMA_MULTIPLE` uses dimensionless σ multiples and requires `deviation_unit = null`.
- `USE_FALLBACK_POLICY` requires non-null immutable `fallback_policy_ref`; `UNKNOWN` requires `fallback_policy_ref = null`.
- Fallback policy MUST resolve same-tenant to an immutable compatible policy version and MUST NOT recursively create a cycle (direct or transitive baseline-fallback cycles are rejected).
- Baseline update/promotion never silently mutates an existing policy version.

**MISSING — `MissingThresholdV1`**

```text
MissingThresholdV1 {
  expr_id: string
  kind: MISSING
  input_ref: string
  expected_cadence_seconds: uint32
  freshness_deadline_seconds: uint32
}
```

Both `expected_cadence_seconds` and `freshness_deadline_seconds` MUST be `> 0`. Per `Platform_22` §6.1.7, missing/no-data is evidence-quality / semantic-state information and MUST NOT imply Device Down.

**COMPOSITE — `CompositeThresholdV1`**

```text
CompositeThresholdV1 {
  expr_id: string
  kind: COMPOSITE
  operator: AND | OR | NOT
  children: ThresholdExprV1[]
}
```

Validation:

- `NOT` requires exactly one child; `AND` / `OR` require `>= 2` children.
- Recursive validation applies to the complete expression tree.
- No UNKNOWN-coercion field is permitted in V1; `Platform_22` §6.1.8 fixes the tri-state truth table. UNKNOWN MUST NOT be coerced to FALSE/TRUE by default.

**GROUP — `GroupThresholdV1`**

```text
GroupThresholdV1 {
  expr_id: string
  kind: GROUP
  member_selector: SubjectSelectorV1
  child: ThresholdExprV1
  aggregation: ANY | ALL | COUNT | PERCENT
  min_qualified_members: uint32
  count_threshold: uint32 | null
  percent_threshold: number | null
  evaluation_window_seconds: uint32
  alignment_tolerance_seconds: uint32
}
```

Rules:

- Canonical membership selector, membership snapshot/revision/hash evidence, minimum qualified member behavior, COUNT/PERCENT thresholds, evaluation window/alignment and UNKNOWN semantics remain as `Platform_22` §6.1.11.
- `COUNT` requires non-null `count_threshold`; `PERCENT` requires non-null `percent_threshold`; other aggregations require both thresholds null.
- `min_qualified_members >= 1`; `evaluation_window_seconds > 0`; `alignment_tolerance_seconds` is non-negative.
- Insufficient qualified membership ⇒ UNKNOWN. Ad-hoc non-inventory member lists and rewriting historical membership evidence are forbidden.
- **Recursive nested-GROUP prohibition (V1):** validation MUST recursively walk the complete `child` expression tree. **No descendant at any depth may have `kind=GROUP`.** Direct-child-only checks are insufficient. Invalid forms include `GROUP → GROUP`, `GROUP → COMPOSITE → GROUP`, `GROUP → DURATION → COMPOSITE → GROUP`, and any other nested form. The deterministic validator error is `NESTED_GROUP_NOT_SUPPORTED_V1` and MUST include the exact offending JSON path.

**DEPENDENCY — `DependencyThresholdV1`**

```text
DependencyThresholdV1 {
  expr_id: string
  kind: DEPENDENCY
  dependency_ref: DependencyRefV1
  activation: DependencyActivationV1
  action: SUPPRESS | DEFER | ANNOTATE
  child: ThresholdExprV1
}
```

Rules (persists the dependency action required by `Platform_22` §6.1.9):

- `dependency_ref` is the typed canonical versioned/topology authority reference above.
- `activation` is required and MUST satisfy `DependencyActivationV1` (at least one activation criterion; required explicit `combine ∈ {ANY, ALL}`; `ALL` forbidden when only one criterion is enabled).
- `action` is required and immutable within a policy version; changing `action` or `activation` requires a new policy version.
- Exact SUPPRESS / DEFER / ANNOTATE evaluation semantics are owned by `Platform_22` §6.1.9 and MUST NOT be reimplemented as writer-invented defaults. `combine` only decides whether activation is TRUE.
- Stale/unqualified/No-Data dependency evidence ⇒ `UNKNOWN` / no dependency action; stale topology is never treated as proof of child health/failure and cannot activate `SUPPRESS`/`DEFER`.
- Dependency disappearance/revocation removes the dependency effect reversibly; historical child evidence is never deleted by dependency action alone.

**BUSINESS_TIME — `BusinessTimeThresholdV1`**

```text
BusinessTimeThresholdV1 {
  expr_id: string
  kind: BUSINESS_TIME

  calendar_ref: CalendarRefV1
  blackout_calendar_ref: CalendarRefV1 | null

  applicability: IN_WINDOW | OUTSIDE_WINDOW

  gap_behavior: OUTSIDE_WINDOW
  fold_behavior: EARLIER_OFFSET | LATER_OFFSET

  child: ThresholdExprV1
}
```

Rules (single business-time representation + configurable DST choices required by `Platform_22` §6.1.10):

- Schema V1 has **exactly one** business-time representation: an explicit `BUSINESS_TIME` node. Root `AlertPolicyDefinitionV1.business_calendar_ref` is **not** a valid V1 field and MUST fail validation (`unknown fields in schema V1 MUST fail validation`).
- `calendar_ref` is required: exact immutable same-tenant calendar ID + `version_no`; IANA timezone is owned by that calendar version.
- `blackout_calendar_ref` is nullable. When present it MUST resolve to an exact immutable same-tenant calendar ID + `version_no` and is the optional freeze/blackout policy identity required by `Platform_22` §6.1.10.
- V1 blackout behavior is **FIXED BY EVALUATION CONTRACT — NOT CONFIGURABLE IN V1**: `HOLD_STATE` (do not newly fire; do not clear an already-active alert solely because blackout started; preserve prior alert state; retain auditable `BLACKOUT_HOLD`; re-evaluate current qualified child evidence after blackout ends without backdating).
- `gap_behavior` has the only valid V1 value `OUTSIDE_WINDOW` (required; stored explicitly).
- `fold_behavior` is required and MUST be persisted as `EARLIER_OFFSET` or `LATER_OFFSET` on the immutable version. Evaluation of an immutable policy MUST NEVER depend on an unwritten implementation default.
- Missing/invalid primary `calendar_ref` ⇒ `UNKNOWN` (never fall back to always-on evaluation).
- Missing/invalid configured `blackout_calendar_ref` ⇒ `UNKNOWN`, not silently “no blackout”.
- Free-form offset-without-zone fields are forbidden.

###### `ThresholdExprV1` ↔ `Platform_22` §6.1.1 common-envelope persistence coverage matrix

| §6.1.1 configurable requirement | Persisted field / immutable reference | Notes |
|---|---|---|
| Input identity | `SubjectSelectorV1.metric_ref` / `monitor_ref`; per-node `input_ref` | Canonical catalogue refs only |
| Evaluation type | `ThresholdExprV1.kind` (+ COMPOSITE/GROUP composition) | Discriminated union above |
| Evaluation window | `EvaluationCadenceV1`; `GroupThresholdV1.evaluation_window_seconds` / `alignment_tolerance_seconds`; `DurationThresholdV1.duration_seconds`; `BaselineThresholdV1.reference_window_seconds`; `MissingThresholdV1.expected_cadence_seconds` / `freshness_deadline_seconds`; `RateThresholdV1.minimum_interval_seconds` | Family-specific windows; not a second DSL |
| Qualified-sample requirement | `BaselineThresholdV1.minimum_qualified_samples`; `GroupThresholdV1.min_qualified_members` | Quality-state membership itself is **FIXED BY EVALUATION CONTRACT — NOT CONFIGURABLE IN V1** (`Platform_22` §5 / §6.1) |
| Evidence-quality prerequisite | `evidence_contract_ref` + `AlertPolicyDefinitionV1.evidence_class` | Contract owns class-specific gates; not per-tick free-form predicates |
| Threshold parameters | `ComparisonV1` / family-specific fields (`fire`/`clear`, counts, deviations, calendars, children, group aggregation) | See variants above |
| Clear / reset / hysteresis | `StaticThresholdV1.clear`; `DurationThresholdV1.unknown_behavior` + `unknown_grace_seconds`; `ConsecutiveThresholdV1.unknown_behavior`; optional root `recovery_condition` | No hidden numeric hysteresis default |
| Missing / stale behavior | `MissingThresholdV1.*`; `BaselineThresholdV1.stale_behavior` + `fallback_policy_ref`; RATE/STATIC UNKNOWN on unqualified input | Device Down coercion **FIXED BY EVALUATION CONTRACT — NOT CONFIGURABLE IN V1** |
| Dependency behavior | `DependencyThresholdV1.action` + `DependencyThresholdV1.activation` (+ `dependency_ref`) | `SUPPRESS`/`DEFER`/`ANNOTATE` immutable per policy version; `activation.combine ∈ {ANY, ALL}` required (no hidden default); stale/UNKNOWN ⇒ no action |
| Business-time applicability | `BusinessTimeThresholdV1.calendar_ref` / `applicability` / `gap_behavior` / `fold_behavior` ONLY | No root `business_calendar_ref`; gap/fold stored explicitly |
| Blackout / freeze identity | `BusinessTimeThresholdV1.blackout_calendar_ref` | Optional exact same-tenant calendar version; V1 behavior `HOLD_STATE` **FIXED BY EVALUATION CONTRACT — NOT CONFIGURABLE IN V1** |
| Version authority | enclosing `alert_policy_version (policy_id, version_no)`; `BaselineRefV1`; `PolicyVersionRefV1`; `CalendarRefV1`; `DependencyRefV1.relationship_id` | Immutable references only |
| Tri-state result `TRUE\|FALSE\|UNKNOWN` | **FIXED BY EVALUATION CONTRACT — NOT CONFIGURABLE IN V1** | No UNKNOWN-coercion knob on COMPOSITE |
| Deterministic evaluation / no silent self-learning | **FIXED BY EVALUATION CONTRACT — NOT CONFIGURABLE IN V1** | Baseline/policy versions immutable once published |
| Counter64-before-float rate authority | **FIXED BY EVALUATION CONTRACT — NOT CONFIGURABLE IN V1** | Retained on RATE path |
| Nested GROUP | **FORBIDDEN IN V1** — recursive `NESTED_GROUP_NOT_SUPPORTED_V1` | Not a configurable knob |

This matrix exists to prevent persistence-versus-evaluation divergence. Do not invent knobs merely to fill cells.

###### `AlertPolicyDefinitionV1` recursive validation contract

The same server-side validator used by **OP-POLICY-CREATE**, **OP-POLICY-VERSION-CREATE** and **OP-POLICY-VERSION-VALIDATE** MUST perform recursive schema and semantic validation of the complete `ThresholdExprV1` tree, including:

- unique `expr_id` across the full tree;
- maximum expression depth / item count owned by existing security/capacity bounds — if an exact numeric bound is not yet frozen here, reference **BUILD-130** rather than inventing one;
- illegal field-for-kind rejection (only the frozen fields for that `kind` are legal);
- required-nullability enforcement (`ComparisonV1` range vs non-range; baseline deviation/fallback nullability; GROUP aggregation thresholds; DURATION RESET grace = 0; etc.);
- recursive nested-GROUP rejection with error code `NESTED_GROUP_NOT_SUPPORTED_V1` and the exact offending JSON path;
- baseline fallback-cycle rejection (direct and transitive);
- same-tenant reference checks for calendars, baselines, fallback policies, dependency relationships, subject scope and metric/monitor refs;
- unit compatibility (`ComparisonV1.unit`, STATIC fire/clear, ABSOLUTE baseline `deviation_unit`);
- calendar/version existence and immutable reference validity (`CalendarRefV1`, `BaselineRefV1`, `PolicyVersionRefV1`, `DependencyRefV1`);
- **root `business_calendar_ref` is not a valid V1 field** and MUST fail as an unknown/forbidden field; no alternate calendar representation may exist elsewhere on `AlertPolicyDefinitionV1` outside `BusinessTimeThresholdV1`;
- `BUSINESS_TIME` requires `calendar_ref`, required `gap_behavior`/`fold_behavior`, and nullable `blackout_calendar_ref`; when `blackout_calendar_ref` is present it MUST resolve same-tenant to an exact immutable calendar version; unknown/missing calendar references fail validation (`UNKNOWN` at evaluation — never silent always-on / silent “no blackout”);
- `DEPENDENCY` requires `dependency_ref`, `activation` (`DependencyActivationV1` with ≥1 activation criterion, required `combine ∈ {ANY, ALL}`, and `ALL` only when ≥2 criteria are enabled) and immutable `action ∈ {SUPPRESS, DEFER, ANNOTATE}`;
- unknown fields in schema V1 MUST fail validation unless a documented compatibility policy explicitly allows additive fields — never silently ignore an unknown policy field that could change intended behavior;
- typed field/path errors; **never** silently normalize or rewrite an invalid definition.

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `monitoring_profile` / `monitoring_profile_version` | profile_id, name, version_no, definition JSONB, content_hash, capacity_estimate JSONB N | PK/UNIQUE as policy | `(tenant_id,name)` | immutable versions | permanent / `profile.*` | MIG-PLAN-003 |
| `monitoring_profile_assignment` +CC | entity_id NN; profile_id NN; profile_version_no INT NN; assignment_mode VARCHAR(24) NN CHECK IN (AUTO_FROM_CRITICALITY,EXPLICIT_PROFILE); assigned_by NN; effective_from NN | PK (tenant_id, entity_id); FK entity/profile same-tenant | `(tenant_id,profile_id)` | version/If-Match; **an EXPLICIT_PROFILE row is never replaced by a criticality change** | permanent / `profile.assign.*` | MIG-PLAN-003 |
| `maintenance_window` +CC | window_id NN; scope JSONB NN; timezone VARCHAR(64) NN; recurrence VARCHAR(256) N; starts_at/ends_at TIMESTAMPTZ NN; effect VARCHAR(28) NN CHECK IN (SUPPRESS_NOTIFICATIONS,SUPPRESS_ALERTING,EXPECTED_STATE); state VARCHAR(12) NN CHECK IN (SCHEDULED,ACTIVE,ENDED,CANCELLED,EXPIRED); reason VARCHAR(1000) NN; ticket_ref VARCHAR(128) N; ended_early_at N; CHECK ends_at > starts_at | PK | `(tenant_id,state,starts_at)` | version/If-Match; CC-IDEM | maint-history class (never hard-deleted once ACTIVE) / `maintenance.*` | MIG-PLAN-003 |
| `suppression_rule` +CC | rule_id; type VARCHAR(12) NN CHECK IN (DEPENDENCY,DEDUP,FLAP); scope JSONB; definition JSONB bounded; enabled BOOL | PK | `(tenant_id,type)` | version | policy class / `suppression.*` | MIG-PLAN-003 |
| `alert_mute` +CC | mute_id; principal_id N (personal) / team_ref N; target_type (ALERT/SCOPE) NN; target_ref; muted_until TIMESTAMPTZ NN; reason | PK; CHECK muted_until > created_at | `(tenant_id,muted_until)` | version | short / `alert.mute.*` | MIG-PLAN-003 |

#### Alerts and incidents

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `alert_current` +CC | alert_id UUID NN; alert_key VARCHAR(256) NN; entity_id NN; policy_id NN; policy_version_no INT NN; semantic_state VARCHAR(24) NN (UP/DOWN/DEGRADED/UNKNOWN/NO_DATA/STALE/MONITORING_DEGRADED/REDUNDANCY_LOST/SLA_DEGRADED/EXPECTED_STATE); severity VARCHAR(12) NN; confidence VARCHAR(12) NN (separate dimension); lifecycle_state VARCHAR(20) NN CHECK IN (NEW,ACKNOWLEDGED,ASSIGNED,INVESTIGATING,MONITORING_RECOVERY,RESOLVED,CLOSED); assignee_ref N; acked_by/acked_at N; overlay_flags VARCHAR[] NN; occurrence_count INT NN; first_fired_at/last_evaluated_at NN; evidence_ref VARCHAR(256) NN; incident_id UUID N | PK alert_id; partial UNIQUE (tenant_id, alert_key) WHERE lifecycle_state <> 'CLOSED'; FK entity/policy/incident same-tenant | `(tenant_id,lifecycle_state,severity)`,`(tenant_id,entity_id)`,`(tenant_id,incident_id)` | version/If-Match; engine upsert on alert_key | alert-active class (history to ClickHouse) / `alert.*` via lifecycle event | MIG-PLAN-004 |
| `alert_lifecycle_event` (append-only) | event_id UUID NN; alert_id NN; seq BIGINT NN; from_state/to_state NN; actor_type/actor_id NN; reason N; detail JSONB N; occurred_at NN | PK event_id; UNIQUE (alert_id, seq); FK alert same-tenant | `(alert_id,seq)` | seq assigned under row lock of alert_current | follows alert history class / is the audit trail | MIG-PLAN-004 |
| `incident` +CC | incident_id NN; correlation_key VARCHAR(256) NN; title VARCHAR(240) NN; state VARCHAR(16) NN CHECK IN (OPEN,ACKNOWLEDGED,INVESTIGATING,RESOLVED,CLOSED); priority VARCHAR(8) NN; owner_ref N; root_scope JSONB N; rca_status VARCHAR(16) NN CHECK IN (PENDING,CONCLUDED,INCONCLUSIVE); itsm_sync_state VARCHAR(16) NN (NONE/PENDING/SYNCED/CONFLICT/FAILED); opened_at/resolved_at/closed_at N; reopen_of UUID N | PK; partial UNIQUE (tenant_id, correlation_key) WHERE state IN ('OPEN','ACKNOWLEDGED','INVESTIGATING'); FK reopen_of same-tenant | `(tenant_id,state,priority)`,`(tenant_id,owner_ref)` | version/If-Match | incident class (protected if legal-hold linked) / incident events | MIG-PLAN-004 |
| `incident_alert_link` | incident_id NN; alert_id NN; linked_at NN; linked_by NN; link_state VARCHAR(8) NN (ACTIVE/REMOVED) | PK (incident_id, alert_id); FK both same-tenant | `(alert_id)` | UNIQUE active link per alert (an alert is linked to at most one active incident) | with incident / `incident.link` | MIG-PLAN-004 |
| `incident_event` (append-only) | event_id; incident_id NN; seq BIGINT NN; type VARCHAR(24) NN; actor; reason; detail JSONB bounded; occurred_at | PK; UNIQUE (incident_id, seq) | `(incident_id,seq)` | seq under incident row lock | with incident / is the audit trail | MIG-PLAN-004 |

#### RCA and evidence metadata

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `rca_investigation` +CC | rca_id NN; incident_id NN; state VARCHAR(12) NN CHECK IN (OPEN,CONCLUDED,INCONCLUSIVE,SUPERSEDED); current_revision_no INT N | PK; UNIQUE (tenant_id, incident_id) ; FK incident | `(tenant_id,state)` | version | with incident / `rca.*` | MIG-PLAN-004 |
| `rca_revision` (immutable) | rca_id NN; revision_no INT NN; outcome VARCHAR(16) NN (CONCLUDED/INCONCLUSIVE/PARTIAL_EVIDENCE); likely_cause JSONB N; confidence VARCHAR(12) NN; hypotheses JSONB NN (bounded); supporting_refs/contradicting_refs/missing_refs JSONB NN; engine_version VARCHAR(64) NN; rule_set_version VARCHAR(64) NN; evidence_set_hash BYTEA NN; created_at | PK (tenant_id, rca_id, revision_no); UNIQUE (tenant_id, rca_id, evidence_set_hash, rule_set_version) | `(rca_id,revision_no DESC)` | append-only; the unique key dedupes replays | permanent / `rca.revision.created` | MIG-PLAN-004 |
| `rca_feedback` (append-only) | feedback_id; rca_id; revision_no; verdict VARCHAR(8) NN; reason_code VARCHAR(48) NN; note VARCHAR(1000) N; actor_id; created_at | PK; FK revision same-tenant | `(rca_id,revision_no)` | idempotent per (rca_id,revision_no,actor_id) | feeds evaluation only / `rca.feedback.*` | MIG-PLAN-004 |
| `evidence_ref` | evidence_id NN; kind VARCHAR(32) NN; source_system VARCHAR(48) NN; source_ref VARCHAR(512) NN; entity_id N; observed_from/observed_to TIMESTAMPTZ N; quality VARCHAR(12) NN; content_hash BYTEA N; retention_class VARCHAR(16) NN | PK; UNIQUE (tenant_id, source_system, source_ref, content_hash) | `(tenant_id,kind)`,`(tenant_id,entity_id,observed_from)` | immutable (references, never payloads) | per class (legal-hold aware) / none | MIG-PLAN-004 |

#### NCM

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `ncm_job` +CC | job_id NN; kind VARCHAR(12) NN CHECK IN (BACKUP,CHANGE,ROLLBACK,COMPLIANCE); state VARCHAR(20) NN (QUEUED/RUNNING/SUCCEEDED/PARTIAL/FAILED/CANCELLED/TIMED_OUT/COMPLETED_UNVERIFIED/ABORTED/ROLLED_BACK/ROLLBACK_FAILED/UNSUPPORTED); mode VARCHAR(16) NN (MANUAL_CLI/CHANGE_PLAN/SCHEDULED/ON_DEMAND); plan_version_id UUID N; **cli_session_id UUID N (column only in MIG-PLAN-005 — no FK until MIG-PLAN-009)**; reason VARCHAR(1000) N; ticket_ref N; requested_by NN; lease_generation BIGINT N | PK; FK `plan_version_id` same-tenant when Phase-1.5 plan tables exist (**no** FK to `cli_session` in MIG-PLAN-005 — `cli_session` does not exist until MIG-PLAN-009; FK `(tenant_id, cli_session_id)→cli_session` is added in MIG-PLAN-009) | `(tenant_id,kind,state)` | version; per-target lease; CC-IDEM | ncm-job class / `ncm.job.*` | MIG-PLAN-005 (column); MIG-PLAN-009 (FK) |
| `ncm_job_target` | job_id NN; entity_id NN; state VARCHAR(16) NN (PENDING/CAPTURING/STORED/UNCHANGED/FAILED/SKIPPED/EXECUTING/VALIDATING/…); error_code VARCHAR(48) N; result_config_version_id N; pre_config_version_id N; post_config_version_id N; sensor_id N | PK (job_id, entity_id); FK entity/job same-tenant | `(tenant_id,entity_id)` | one non-terminal target per entity (partial UNIQUE) | with job / via job audit | MIG-PLAN-005 |
| `ncm_config_version` (immutable) | config_version_id NN; entity_id NN; scope VARCHAR(10) NN (RUNNING/STARTUP); content_hash BYTEA NN; content_ref VARCHAR(512) NN (governed encrypted config store reference; no new database); size_bytes BIGINT NN; redaction_state VARCHAR(24) NN (REDACTED/UNREDACTED_RESTRICTED/FAILED); diff_base_id UUID N; golden BOOL NN; captured_at NN; capture_job_id N; sensor_id N | PK; UNIQUE (tenant_id, entity_id, scope, content_hash) | `(tenant_id,entity_id,captured_at DESC)` | immutable; hash dedupe → UNCHANGED | ncm-config class (legal-hold aware) / `ncm.config.version.stored` | MIG-PLAN-005 |
| `ncm_change_plan` / `ncm_change_plan_version` / `ncm_approval` (**Phase 1.5, not active**) | plan_id; version_no; target_set_hash; rendered_hash; window_start/end; approvals(approver_id, version_no, decision, decided_at) | immutable versions; UNIQUE (plan_id, version_no); approval FK exact version | `(tenant_id,plan_id)` | `STALE_REAPPROVAL_REQUIRED` on any bound-field change | permanent / `ncm.plan.*` | MIG-PLAN-005 (deferred activation) |

#### IPAM

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `ipam_vrf` +CC | vrf_id; name VARCHAR(128) NN; rd VARCHAR(32) N | PK; UNIQUE (tenant_id,name) | – | version | permanent / `ipam.vrf.*` | MIG-PLAN-006 |
| `ipam_subnet` +CC | subnet_id; vrf_id NN; cidr CIDR NN; site_id N; parent_subnet_id N; purpose VARCHAR(160) N | PK; UNIQUE (tenant_id, vrf_id, cidr); FK vrf/site same-tenant; overlap prevention in-transaction under `pg_advisory_xact_lock(hash(tenant,vrf))` (no extension introduced) | `(tenant_id,vrf_id)` | version/If-Match | permanent / `ipam.subnet.*` | MIG-PLAN-006 |
| `ipam_address` +CC | address_id; subnet_id NN; address INET NN; declared_state VARCHAR(12) NN (FREE/RESERVED/ASSIGNED/RELEASED); observed_state VARCHAR(12) NN (UNSEEN/OBSERVED/CONFLICT); observed_by JSONB N (source, MAC, entity_id, last_seen); last_observed_at N | PK; UNIQUE (tenant_id, subnet_id, address) | `(tenant_id,subnet_id,declared_state)` | version; observed vs declared kept separate | history class / `ipam.address.*` | MIG-PLAN-006 |
| `ipam_reservation` +CC | reservation_id; address_id NN; owner_ref VARCHAR(160) NN; purpose; state VARCHAR(10) NN CHECK IN (ACTIVE,RELEASED,EXPIRED); expires_at N; released_by/at N | PK; **partial UNIQUE (tenant_id, address_id) WHERE state='ACTIVE'** | `(tenant_id,state,expires_at)` | version/If-Match; CC-IDEM | permanent / `ipam.reserve|release|expire` | MIG-PLAN-006 |
| `ipam_conflict` +CC | conflict_id; address_id NN; kind VARCHAR(24) NN (DUPLICATE_OBSERVED/RESERVED_BY_OTHER/OVERLAP); evidence JSONB bounded; state VARCHAR(12) NN (OPEN/ACKNOWLEDGED/RESOLVED/DISMISSED); resolved_reason N | PK; partial UNIQUE (tenant_id,address_id,kind) WHERE state IN ('OPEN','ACKNOWLEDGED') | `(tenant_id,state)` | version | permanent / `ipam.conflict.*` | MIG-PLAN-006 |

#### Reports and schedules

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `report_definition` +CC / `report_definition_version` | report_def_id; name NN; type NN; scope JSONB; period_spec JSONB; format VARCHAR(3) NN **CHECK IN ('PDF','CSV')**; branding_ref N; current_version_no | PK; UNIQUE (tenant_id,name); versions immutable (definition JSONB bounded; typed formula only — no executable text) | `(tenant_id,type)` | version/If-Match | permanent / `report.define|update` | MIG-PLAN-007 |
| `report_obligation` +CC | obligation_id; report_def_id NN; rrule VARCHAR(256) NN; timezone VARCHAR(64) NN; recipients JSONB bounded; delivery VARCHAR(16) NN; state VARCHAR(10) NN (ACTIVE/PAUSED/CANCELLED); owner_principal NN | PK; FK def same-tenant | `(tenant_id,state)` | version/If-Match | permanent / `report.schedule.*` | MIG-PLAN-007 |
| `report_run` +CC | run_id; report_def_id NN; definition_version_no NN; obligation_id N; planned_at N; requested_by NN; state VARCHAR(10) NN (QUEUED/RUNNING/SUCCEEDED/PARTIAL/FAILED/CANCELLED/EXPIRED); quality JSONB N; failure_code N; lease_expires_at N | PK; **UNIQUE (tenant_id, obligation_id, planned_at)** (one run per occurrence) | `(tenant_id,state)` | version; CC-IDEM for manual | run-history class / `report.run|download` | MIG-PLAN-007 |
| `report_artifact` | artifact_id; run_id NN; format VARCHAR(3) NN CHECK IN ('PDF','CSV'); checksum BYTEA NN; size_bytes BIGINT NN; storage_ref VARCHAR(512) NN; branding_ref N; state VARCHAR(10) NN (GENERATING/AVAILABLE/EXPIRED/REVOKED); expires_at NN | PK; FK run same-tenant | `(tenant_id,expires_at)` | immutable payload; revoke flips state | artifact class (TTL) / `report.download` | MIG-PLAN-007 |
| `report_delivery` | delivery_id; run_id; channel; state (QUEUED/SENT/FAILED); attempts INT; not_before | PK | `(state,not_before)` | idempotent per (run_id, channel, recipient_hash) | ops class / `report.deliver` | MIG-PLAN-007 |

#### Notifications (aligns Platform_20 §6.4)

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `user_notification` | notification_id; category VARCHAR(24) NN CHECK IN (INCIDENT,NETWORK,INFRASTRUCTURE,DATABASE,SDWAN_CLOUD,DISCOVERY_NEW_ASSET,APPROVAL,PLATFORM_HEALTH,OTHER); notification_type VARCHAR(64) NN; source_object_type NN; source_object_id NN; priority VARCHAR(8) NN; message_key/message_fields JSONB bounded; dedupe_key VARCHAR(256) NN; expires_at N; action_required BOOL NN | PK; UNIQUE (tenant_id, dedupe_key) | `(tenant_id,category,created_at)`,`(tenant_id,source_object_type,source_object_id)` | idempotent by dedupe_key | notification class (expiry) / none | MIG-PLAN-008 |
| `user_notification_recipient` | notification_id NN; user_id NN; delivered_at NN; read_at/dismissed_at N; action_state VARCHAR(10) N (PENDING/DONE) | PK (notification_id,user_id); FK same-tenant | `(tenant_id,user_id,read_at)` partial on unread | per-recipient row update; not an authorization grant | with notification / admin actions audited | MIG-PLAN-008 |
| `notification_subscription` +CC | principal_type (USER/TEAM) NN; principal_id NN; filters JSONB bounded; resource_scope_id N; delivery_mode VARCHAR(10) NN (IMMEDIATE/DIGEST/MUTED); quiet_window JSONB N | PK (tenant_id,principal_type,principal_id,filter_hash) | – | version/If-Match | permanent / `notification.subscription.*` | MIG-PLAN-008 |
| `notification_delivery` | delivery_id; notification_id; channel VARCHAR(16) NN; state VARCHAR(12) NN (QUEUED/SENT/DELIVERED/FAILED/SUPPRESSED); attempts INT; not_before; last_error_code N | PK; UNIQUE (notification_id, channel, recipient_hash) | `(state,not_before)` | resumable worker | ops class / none | MIG-PLAN-008 |

#### Diagnostics, Investigation, CLI

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `diagnostic_job` +CC | job_id; origin_type VARCHAR(16) NN; origin_id NN; target_entity_id NN; diagnostic_type VARCHAR(48) NN; params JSONB NN (bounded; secret-free); vantage_mode VARCHAR(10) NN (ASSIGNED/SELECTED/MULTI); state VARCHAR(12) NN (QUEUED/RUNNING/PARTIAL/SUCCEEDED/FAILED/CANCELLED/TIMED_OUT/DENIED); investigation_id N; incident_id N; trigger VARCHAR(12) NN (OPERATOR/EDS_POLICY); lease_generation N; deadline_at NN | PK; FK entity/investigation/incident same-tenant | `(tenant_id,state)`,`(tenant_id,target_entity_id,created_at)` | version; CC-IDEM; EDS trigger dedupe by partial UNIQUE (tenant_id,target_entity_id,diagnostic_type) WHERE state IN ('QUEUED','RUNNING') | diagnostic-evidence class (incident-linked = protected) / `diagnostic.job.*` | MIG-PLAN-009 |
| `diagnostic_job_vantage_result` (immutable) | job_id NN; vantage_id VARCHAR(128) NN; state VARCHAR(12) NN (RESULT/ERROR/SUPPRESSED/PENDING/RUNNING); result JSONB bounded N; error_code VARCHAR(48) N; quality VARCHAR(12) NN; observed_at/result_at NN; sensor_or_agent_id NN | PK (job_id,vantage_id); FK job same-tenant | `(tenant_id,observed_at)` | idempotent upsert by (job_id,vantage_id) when state advances only | with job / none | MIG-PLAN-009 |
| `investigation_session` +CC | investigation_id; origin_type NN; origin_id NN; title VARCHAR(240) NN; state VARCHAR(6) NN (OPEN/CLOSED); owner_ref NN; incident_id N; close_reason N | PK | `(tenant_id,state,owner_ref)` | version/If-Match | incident-linked = protected / `investigation.*` | MIG-PLAN-009 |
| `investigation_evidence_link` | investigation_id NN; seq INT NN; evidence_id NN (→ evidence_ref); link_state VARCHAR(8) NN (ACTIVE/REMOVED); evidence_snapshot_hash BYTEA NN; added_by NN | PK (investigation_id, seq); UNIQUE (investigation_id, evidence_id) | – | idempotent attach | with session / `investigation.attach|detach` | MIG-PLAN-009 |
| `cli_session` +CC | session_id; entity_id NN; requested_privilege VARCHAR(16) NN; credential_mode VARCHAR(14) NN (STORED_PROFILE/PERSONAL/ONE_TIME); state VARCHAR(12) NN (REQUESTED/AUTHORIZED/CONNECTING/ACTIVE/CLOSED/TERMINATED/EXPIRED/DENIED/FAILED); reason NN; ticket_ref N; mfa_outcome VARCHAR(10) N; recording_ref VARCHAR(512) N; expires_at NN; closed_reason N — **no credential, token or keystroke columns** | PK; FK entity same-tenant. **Also in this plan:** `ALTER TABLE ncm_job ADD CONSTRAINT fk_ncm_job_cli_session FOREIGN KEY (tenant_id, cli_session_id) REFERENCES cli_session (tenant_id, session_id)` (column already nullable from MIG-PLAN-005; no forward-reference FK) | `(tenant_id,state)`,`(tenant_id,entity_id)` | version; CC-IDEM | cli-audit class / `cli.session.*` | MIG-PLAN-009 |

#### Integration outbox/inbox

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `integration_connector` +CC | connector_id; type VARCHAR(32) NN; endpoint_ref VARCHAR(512) NN; auth_ref (Credential Profile id) NN; mapping_version INT NN; state VARCHAR(14) NN (DRAFT/SANDBOX_PASSED/ENABLED/DEGRADED/DISABLED); health JSONB N | PK; UNIQUE (tenant_id,type,endpoint_ref) | `(tenant_id,state)` | version/If-Match | permanent / `integration.connector.*` | MIG-PLAN-010 |
| `integration_outbox` | operation_id UUID NN; connector_id NN; object_type/object_id NN; operation_hash BYTEA NN; mapping_version INT NN; payload JSONB bounded; state VARCHAR(12) NN (PENDING/SENDING/DELIVERED/RETRY/DEAD); attempts INT; not_before NN; lease_until N; last_error_code N; created_in_txn_event_id UUID NN | PK; **UNIQUE (tenant_id, connector_id, operation_hash)** | `(state,not_before)` | lease `SKIP LOCKED`; external call carries operation_hash as external idempotency reference | ops class; DEAD kept until replay/expiry / `integration.outbound.*` | MIG-PLAN-010 |
| `integration_inbox` | inbox_id; connector_id NN; external_event_id VARCHAR(256) NN; external_object_ref NN; external_version/etag N; payload_hash BYTEA NN; verification VARCHAR(10) NN (VERIFIED/FAILED); state VARCHAR(10) NN (RECEIVED/PROCESSED/REJECTED/DEAD); received_at NN; tenant derived from authenticated ingress (never from payload) | PK; **UNIQUE (tenant_id, connector_id, external_event_id)** | `(state,received_at)` | dedupe by unique key | ops class / `integration.inbound.*` | MIG-PLAN-010 |
| `external_binding` +CC | binding_id; connector_id; entity_or_incident_ref NN; external_ref NN; external_version N; authority VARCHAR(12) NN (PLATFORM/EXTERNAL/SHARED_FIELD); state VARCHAR(8) NN (ACTIVE/CONFLICT/BROKEN) | PK; UNIQUE (tenant_id, connector_id, external_ref) | – | version | permanent / `integration.binding.*` | MIG-PLAN-010 |
| `sync_conflict` +CC | conflict_id; binding_id; field_path; platform_value_hash; external_value_hash; state (OPEN/RESOLVED); resolution N | PK | `(tenant_id,state)` | version | permanent / `integration.conflict.*` | MIG-PLAN-010 |

#### Licensing

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `licence_entitlement` +CC | licence_id; scope_type VARCHAR(10) NN (DEPLOYMENT/TENANT); artifact_hash BYTEA NN; signature_state VARCHAR(10) NN (VALID/INVALID); issued_at NN; valid_from NN; expires_at TIMESTAMPTZ NN (absolute UTC); entitlements JSONB NN (opaque to this contract; quantities/SKUs `FOUNDER-RESERVED`); extended_until N; extended_reason N | PK; UNIQUE (scope_type, tenant_id, artifact_hash); **tenant_id is N when scope_type=DEPLOYMENT** (CHECK) | – | version/If-Match | permanent (no expiry deletion) / `licence.*` | MIG-PLAN-011 |
| `licence_state_history` (append-only) | licence_id NN; seq BIGINT NN; state VARCHAR(24) NN CHECK IN (VALID,EXPIRING_30D,EXPIRING_7D_BANNER,EXPIRED_GRACE_15D,POLLING_SUSPENDED); entered_at NN; derived_from_expiry TIMESTAMPTZ NN; actor VARCHAR(128) N | PK (licence_id,seq); UNIQUE (licence_id,state,derived_from_expiry) | – | evaluator idempotent | permanent / `licence.state_change` | MIG-PLAN-011 |
| `licence_usage_snapshot` | snapshot_id; captured_at NN; module VARCHAR(48) NN; licensed BIGINT N; used BIGINT NN | PK | `(tenant_id,captured_at)` | append-only | usage class / none | MIG-PLAN-011 |

#### Storage lifecycle

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `retention_policy_revision` +CC | revision_id; data_class VARCHAR(32) NN; requested_retention JSONB NN; effective_retention JSONB N; state VARCHAR(10) NN (DRAFT/PREVIEWED/ACTIVE/REJECTED); preview_snapshot_hash BYTEA N; preview_policy_version BIGINT N; reason NN | PK; partial UNIQUE (tenant_id,data_class) WHERE state='ACTIVE' | – | activation requires matching preview version (`STALE_PREVIEW`) | permanent / `storage.retention.*` | MIG-PLAN-012 |
| `storage_action` +CC | action_id; kind VARCHAR(24) NN; scope JSONB NN; state VARCHAR(10) NN (REQUESTED/RUNNING/COMPLETED/PARTIAL/FAILED/CANCELLED); cursor JSONB N; bytes_reclaimed BIGINT N (exact integer) | PK; partial UNIQUE (tenant_id,(scope->>'volume'),(scope->>'data_class')) WHERE state IN ('REQUESTED','RUNNING') | – | version; resumable cursor | ops class / `storage.purge.*` | MIG-PLAN-012 |
| `legal_hold` +CC | hold_id; scope JSONB NN; reason NN; state (ACTIVE/RELEASED); placed_by NN | PK | `(tenant_id,state)` | version | permanent / `legalhold.*` | MIG-PLAN-012 |

### 15.3 ClickHouse planned tables (`SPECIFIED_NOT_IMPLEMENTED`; BUILD-064 owns the DDL)

All engines follow CC-CH (`ReplacingMergeTree` with a `FINAL` view unless an append-only fact table declares an explicit rebuild/dedupe contract). Types: `tenant_id String`, bounded enums `LowCardinality(String)`.

| Table | Columns (key) | Engine / Partition | ORDER BY | event vs ingest time | Replay/dedupe | TTL / Rollup | Cardinality bound |
|---|---|---|---|---|---|---|---|
| `flow_record_raw` | tenant_id; exporter_id / source identity; observation_domain; ingress_context_id (interface/source vantage); event_time (flow start/export) + optional flow_end_time; ingest_time; src_ip, dst_ip (IPv6-capable); src_port, dst_port; proto; in_if, out_if; **observed_bytes UInt64**; **observed_packets UInt64**; flow_count UInt32 N; tcp_flags; **sampling_state** LowCardinality (`UNSAMPLED`\|`SAMPLED_KNOWN`\|`SAMPLED_UNKNOWN`); sampling_algorithm N; sampling_selector_id N; sampling_rate/probability N; sampling_model_version N; sampling_validity_from/to N; **sampling_cohort_id** String (deterministic cohort identity per §13.9); **direction** LowCardinality (`INGRESS`\|`EGRESS`\|`UNKNOWN`); app_id N; app_classification_provenance N; **estimated_bytes UInt64 N**; **estimated_packets UInt64 N**; estimate_method N; template_id; sequence_id; vrf_id; src_entity_id/dst_entity_id N; quality UInt8 | ReplacingMergeTree / daily `toYYYYMMDD(event_time)` | `(tenant_id, exporter_id, observation_domain, event_time, sequence_id, src_ip, dst_ip, src_port, dst_port, proto)` | both stored; export/capture time = event_time | key includes exporter/domain/sequence; unknown-template/ambiguous-tenant records are **quarantined, never inserted** (§13.9); observed columns never overwritten by estimates; `SAMPLED_UNKNOWN` never treated as `UNSAMPLED` | TTL raw class; MV → `flow_rollup_1m/1h` maintaining **separate observed vs estimated aggregates** (`SummingMergeTree` keyed by tenant, exporter, interface, app, direction, **`sampling_cohort_id`** [or equivalent cohort fields]) **only after** raw FINAL/dedupe; estimated aggregates computed only inside certified `SAMPLED_KNOWN` cohorts; post-replay rebuild/reconcile of rollups is mandatory (CC-CH rollup rule) — replay must not double-count; do not irreversibly merge incompatible sampling cohorts | heavy-hitter + top-N caps per tenant/exporter; rollup dimension budget per tenant; never fabricate direction/app (`UNKNOWN` when uncertified); never silently average different sampling rates/models |
| `log_event_raw` (syslog/OTLP logs/Agent logs) | tenant_id, source_id, source_type, host/entity_id N, event_time, ingest_time, severity UInt8, facility, program, message String (bounded), attributes Map(String,String) (bounded keys), trace_id/span_id N, event_id/sequence_id, quality UInt8, parse_state | ReplacingMergeTree / daily | `(tenant_id, source_id, event_time, sequence_id)` | source timestamp-quality flag when untrusted | event_id replay-collapse; dedupe-after-ACK counter | TTL log class; MV → `log_volume_1m` (counts by severity/program) after raw dedupe; rebuild/reconcile after replay | message/attribute size caps; attribute-key allowlist; per-source rate budget with drop counters |
| `trap_event_raw` | tenant_id, source_ip, entity_id N, engine_id, trap_oid, varbinds String (bounded JSON), event_time, ingest_time, inform_acked UInt8, storm_aggregate_count UInt32, quality, sequence_id | ReplacingMergeTree / daily | `(tenant_id, source_ip, trap_oid, event_time, sequence_id)` | device time vs receive time | sequence_id; storm aggregation recorded | TTL event class | per-source storm aggregation; varbind size cap |
| `audit_history` | tenant_id, audit_id, tenant_seq, occurred_at, ingest_time, actor_type, actor_id, action, resource_type, resource_id N, result, correlation_id N, integrity_ref N, _version UInt64 (backfill/generation watermark; higher wins) | **ReplacingMergeTree(_version)** / monthly `toYYYYMM(occurred_at)` | `(tenant_id, tenant_seq)` (**not** uniqueness by ORDER BY alone) | occurred_at / ingest_time | **Physically replay-safe:** replacement key `(tenant_id, tenant_seq)` + `_version`; readers use `audit_history_final` (`FINAL`). Repeated PostgreSQL backfill upserts the same `(tenant_id, tenant_seq)` and must **not** create duplicate authoritative analytical rows. Cutover watermark = max `(tenant_id, tenant_seq)` successfully copied + recorded in migration/job state; overlap = re-copy `[watermark−overlap_window, max]` then collapse via ReplacingMergeTree; restart/resume continues from persisted watermark. Alternate path: drop+rebuild from PostgreSQL `audit_event` (always authoritative). | TTL AUDIT class (legal-hold aware: partitions under hold are not dropped); **no incremental Summing/Aggregating rollup of audit counts that would double-count under replay** — any audit rollup is rebuild/reconcile-from-FINAL only | bounded fields; no detail blobs |
| `telemetry_quality_event` | tenant_id, source_id, source_protocol, signal_class, event_time, ingest_time, state (FRESH/STALE/PARTIAL/GAP/DROP/TIME_SKEW), counters (produced, accepted, normalized, inserted, rejected UInt64), quality_code, sequence_id | ReplacingMergeTree / monthly | `(tenant_id, source_id, signal_class, event_time, sequence_id)` | both | sequence_id | TTL quality class; MV → `quality_rollup_5m` | one row per source/signal/interval (bounded by source count) |
| `topology_observation_history` | tenant_id, edge_id, src_entity_id, dst_entity_id, relationship, provenance (LLDP/CDP/FLOW/ROUTING/DECLARED/PATH), confidence UInt8, event_time (observed), ingest_time, first_seen/last_seen, state (OBSERVED/STALE/WITHDRAWN), sequence_id | ReplacingMergeTree / monthly | `(tenant_id, edge_id, event_time, sequence_id)` | observation vs ingest | collapse by key; spoofed/cross-tenant advertisements rejected pre-insert | TTL topology class (as-of replay needs retention ≥ RCA window) | edge-count cap per tenant; per-edge history sampling policy |
| `process_network_evidence` (REQ-INF-013 / §11.2) | tenant_id, agent_id, entity_id N, bucket_start UInt32/DateTime64, bucket_end, bucket_duration_sec, process_name, executable_path_hash, service_name, container_id N, cgroup_hash N, local_ip N, local_port N, remote_ip, remote_port, protocol, ip_version, bytes_tx UInt64, bytes_rx UInt64, active/new/closed connections UInt32 N, mean_rtt_us N, retransmit_count N, connection_failures N, quality_state, dropped/shed counts, remote_entity_id N, ingest_time, sequence_id | ReplacingMergeTree / daily | `(tenant_id, entity_id, bucket_start, process_name, remote_ip, remote_port, protocol, sequence_id)` | bucket_start/end = event; ingest separate | sequence_id + agent_id; the Agent bucket is the unit | TTL evidence class; MV rollup → `process_edge_1h` | local aggregation at the Agent bounds cardinality (process/service + canonical target + port + bucket); per-tenant row/byte budget; **no payload/TLS/HTTP/SQL/env/cmdline columns exist** |
| `active_path_observation` + `active_path_hop` | observation: tenant_id, path_id, source_sensor_id, source_vantage_id, target_entity_id, protocol, probed_at (event), ingest_time, total_hops, responsive_hops, rtt/loss/jitter Float32 N, path_fingerprint, fingerprint_version, status, path_identity_quality, comparison_eligible UInt8, sequence_id. hop: observation key + hop_index, hop_ip N, hop_state, asn N, rtt/loss/jitter N, mpls_labels Array(UInt32) | ReplacingMergeTree / daily | obs: `(tenant_id, path_id, source_vantage_id, probed_at, sequence_id)`; hop: `(tenant_id, path_id, source_vantage_id, probed_at, sequence_id, hop_index)` | probe time vs ingest | sequence_id; absent quality fields stored as `UNQUALIFIED/0` | TTL path class | bounded target count and probe rate at Sensor (non-O(N²) fan-out); hop ≤ policy max |
| `alert_history` / `incident_history` | tenant_id, alert_id/incident_id, seq, event_time, ingest_time, from_state, to_state, semantic_state, severity, confidence, entity_id, policy_id+version, actor_type, reason_code, event_id | ReplacingMergeTree / monthly | `(tenant_id, alert_id, seq)` / `(tenant_id, incident_id, seq)` | both | key (tenant, id, seq) from the PostgreSQL lifecycle seq; backfill/replay collapses duplicates; watermark + overlap same pattern as `audit_history` | TTL alert/incident class; MV → daily counts by policy/severity/state **only from FINAL/deduped history**; after PG lifecycle backfill/replay, rebuild/reconcile daily count MVs (no irreversible double-counting incremental aggregation) | bounded; derived from events, never authority |

### 15.4 API operations for the Phase-1 journeys (`SPEC_CONTRACT=FROZEN`)

Auth column = permission; every operation also enforces resource scope, entitlement, CC-API (correlation ID, errors, retry). **Async** = HTTP `202` admission `ACCEPTED_FOR_ASYNC_EXECUTION` + persisted `async_operation` lifecycle per **CC-ASYNC** / G-05 (initial `QUEUED`; success terminal = `SUCCEEDED`). DTOs: `Req`/`Res` named after the contract objects in `Platform_06` §119. List pagination uses **CC-CURSOR**.

| Op ID | Method + logical path | Auth | Request → Response | Pagination / filters / limits | Idempotency | Concurrency | Async | Errors (stable codes, besides `PERMISSION_DENIED`, `VALIDATION_FAILED`, `STALE_VERSION`) | Journey |
|---|---|---|---|---|---|---|---|---|---|
| OP-PROV-CREATE | `POST /provisioning-requests` | inventory.provision | ProvisioningRequestCreate → ProvisioningRequest | – | key required | – | 202 | IDENTITY_CONFLICT, TARGET_EXCLUDED, CAPACITY_BLOCKED, LICENCE_BLOCKED | 119.1 |
| OP-PROV-GET/LIST | `GET /provisioning-requests[/{id}]` | inventory.read | → ProvisioningRequest / page | state, batch, site; cursor | – | ETag | – | RESOURCE_NOT_FOUND | 119.1 |
| OP-PROV-EDIT-FAILED | `PATCH /provisioning-requests/{id}` | inventory.provision | ProvisioningRequestEdit{input patch (secret-free; credential_profile_ref only), expected_version, reason} → ProvisioningRequest | only when state=`FAILED` (or `DRAFT` pre-submit); clears failure_class on accept | key | If-Match | sync | INVALID_TRANSITION (not FAILED/DRAFT), VALIDATION_FAILED, STALE_VERSION | 119.1 |
| OP-PROV-RESUBMIT | `POST /provisioning-requests/{id}:resubmit` | inventory.provision | {expected_version, reason?} → ProvisioningRequest (attempt_no+1; same request_id/entity) | only from `FAILED` after edit or with unchanged valid input | key | If-Match | 202 | INVALID_TRANSITION, IDENTITY_CONFLICT, CAPACITY_BLOCKED, LICENCE_BLOCKED | 119.1 |
| OP-PROV-CANCEL | `POST /provisioning-requests/{id}:cancel` | inventory.provision | {reason} → state | – | key | If-Match | 202 | INVALID_TRANSITION | 119.1 |
| OP-PROV-IMPORT | `POST /provisioning-requests:import` | inventory.provision | multipart CSV/XLSX → AsyncOperation{operation_id, batch_id} | file size/row bound (owner: BUILD-130) | key | – | 202 | IMPORT_ROW_ERRORS (per-row in AsyncOperation result) | 119.1 |
| OP-ASYNC-GET/LIST | `GET /operations[/{operationId}]` | inherits from creating op permission (or operations.read) | → AsyncOperation / page | kind, state, batch_id; cursor; retention-bounded | – | ETag | – | RESOURCE_NOT_FOUND, OPERATION_EXPIRED | shared (G-05) |
| OP-ASYNC-CANCEL | `POST /operations/{operationId}:cancel` | same as create | {reason} → AsyncOperation | items already committed stay committed; remainder `CANCELLED_NOT_STARTED` | key | If-Match | 202 | INVALID_TRANSITION, CANCEL_NOT_SUPPORTED | shared |
| OP-DISC-REQ-CREATE | `POST /discovery/requests` | discovery.run | DiscoveryRequestCreate → DiscoveryRequest | – | key required | – | 202 | SCOPE_INVALID, CAPACITY_BLOCKED | 119.2 |
| OP-DISC-REQ-GET/LIST | `GET /discovery/requests[/{id}]` | discovery.run | → DiscoveryRequest / `PageResult` (CC-API default 50/max 200) | state, site, protocol; cursor | – | ETag | – | RESOURCE_NOT_FOUND | 119.2 |
| OP-DISC-REQ-CANCEL | `POST /discovery/requests/{id}:cancel` | discovery.run | {reason} → DiscoveryRequest | – | key | If-Match | 202 | INVALID_TRANSITION | 119.2 |
| OP-ONB-CAND-LIST/GET | `GET /onboarding/candidates[/{id}]` | onboarding.review | → OnboardingCandidate(+revision) | state, source_type, site, conflict; cursor (default 50/max 200) | – | ETag=revision | – | – | 119.2/3 |
| OP-ONB-CAND-DECIDE | `POST /onboarding/candidates/{id}:decide` | onboarding.review | OnboardingDecision → OnboardingDecisionResult | – | key | `expected_revision` (If-Match) | sync (APPROVE → provisioning async) | IDENTITY_CONFLICT_REVIEW_REQUIRED, CAPACITY_BLOCKED, LICENCE_BLOCKED, INVALID_TRANSITION | 119.2 |
| OP-ONB-CAND-BULK-DECIDE | `POST /onboarding/candidates:bulk-decide` | onboarding.review | OnboardingBulkDecideRequest{items[{id,expected_revision}], decision, reason, enrichment?} → AsyncOperation{operation_id} (202) | item bound (BUILD-130) | key | per-item revision | 202 | PARTIAL per item (in AsyncOperation result) | 119.2 |
| OP-ONB-BULK-GET/STATUS | `GET /operations/{operationId}` (OP-ASYNC-GET) **and** `GET /onboarding/bulk-decisions/{operationId}` (alias → same AsyncOperation + typed per-item results[]) | onboarding.review | → AsyncOperation + OnboardingBulkItemResult[{candidate_id, result: APPLIED\|FAILED\|SKIPPED\|CONFLICT\|CANCELLED_NOT_STARTED, error_code?, after_revision?}] | cursor over items when large | – | ETag | – | RESOURCE_NOT_FOUND, OPERATION_EXPIRED | 119.2 |
| OP-ONB-BULK-CANCEL | `POST /operations/{operationId}:cancel` (OP-ASYNC-CANCEL) **or** `POST /onboarding/bulk-decisions/{operationId}:cancel` | onboarding.review | {reason} → AsyncOperation | decided items retained; undecided → CANCELLED_NOT_STARTED | key | If-Match | 202 | INVALID_TRANSITION | 119.2 |
| OP-EXCL-REOPEN | `POST /onboarding/candidates/{id}:reopen` | onboarding.exclusion.reopen | {reason} → candidate | – | key | If-Match | sync | NOT_EXCLUDED | 119.2 |
| OP-AGENT-REASSIGN | `POST /agents/{agentId}:reassign` | sensor.assign | {expected_assignment_generation, target, reason} → assignment | – | key | generation | 202 | NO_CAPACITY, STALE_VERSION | 119.3 |
| OP-AGENT-REVOKE | `POST /agents/{agentId}:revoke` | agent.revoke | {reason} → binding | – | key | If-Match | sync | – | 119.3 |
| OP-ALERT-LIST/GET | `GET /alerts[/{id}]` | alerts.read | → AlertCurrent / page | state, severity, confidence, entity, policy, assignee, overlay; cursor | – | ETag | – | – | 119.4 |
| OP-ALERT-ACK | `POST /alerts/{id}:acknowledge` | alerts.ack | {note?} → AlertCurrent | – | key | If-Match | sync | ALREADY_ACKNOWLEDGED(200 no-op), INVALID_TRANSITION, ALERT_CLOSED | 119.4 |
| OP-ALERT-ASSIGN/NOTE/CLOSE | `POST /alerts/{id}:assign` `:note` `:close` | alerts.assign / alerts.note / alerts.close | {assignee_ref}/{note}/{reason} | – | key | If-Match | sync | INVALID_TRANSITION | 119.4 |
| OP-ALERT-BULK | `POST /alerts:bulk-action` | alerts.ack | {ids[], action} → per-alert results | item bound | key | per-item version | 202 | PARTIAL | 119.4 |
| OP-ALERT-MUTE | `POST /alerts/{id}:mute` / `DELETE …/mute` | alerts.mute | {until, reason} → AlertMute | – | key | If-Match | sync | – | 119.5 |
| OP-MNT-CREATE/UPDATE/CANCEL/END-EARLY | `POST /maintenance-windows`, `PATCH /…/{id}`, `POST …:cancel`, `POST …:end-early`, `POST /maintenance-windows:preview` | maintenance.manage | MaintenanceWindow → window (+preview {affected_count}) | preview query bound | key | If-Match | sync | WINDOW_CONFLICT, QUERY_LIMITED | 119.5 |
| OP-INC-LIST/GET | `GET /incidents[/{id}]` | incidents.read | → Incident / page | state, priority, owner, service; cursor | – | ETag | – | – | 119.6 |
| OP-INC-ACK | `POST /incidents/{id}:acknowledge` | incidents.manage | IncidentAck{expected_version, reason?} → Incident | – | key | If-Match | sync | INVALID_TRANSITION | 119.6 |
| OP-INC-ASSIGN | `POST /incidents/{id}:assign` | incidents.manage | IncidentAssign{expected_version, assignee_ref, reason?} → Incident | – | key | If-Match | sync | INVALID_TRANSITION | 119.6 |
| OP-INC-PRIORITY | `POST /incidents/{id}:priority` | incidents.manage | IncidentSetPriority{expected_version, priority VARCHAR(8) NN, reason} → Incident | priority enum bounded to incident.priority vocabulary | key | If-Match | sync | VALIDATION_FAILED, INVALID_TRANSITION | 119.6 |
| OP-INC-NOTE | `POST /incidents/{id}:note` | incidents.manage | IncidentNote{expected_version, note VARCHAR(4000) NN, reason?} → Incident | – | key | If-Match | sync | VALIDATION_FAILED | 119.6 |
| OP-INC-LINK | `POST /incidents/{id}:link-alert` | incidents.manage | IncidentLinkAlert{expected_version, alert_id UUID NN, reason?} → Incident (+link) | – | key | If-Match | sync | ALERT_ALREADY_LINKED_ELSEWHERE, RESOURCE_NOT_FOUND, INVALID_TRANSITION | 119.6 |
| OP-INC-UNLINK | `POST /incidents/{id}:unlink-alert` | incidents.manage | IncidentUnlinkAlert{expected_version, alert_id UUID NN, reason?} → Incident | – | key | If-Match | sync | LINK_NOT_FOUND, INVALID_TRANSITION | 119.6 |
| OP-INC-RESOLVE | `POST /incidents/{id}:resolve` | incidents.manage | IncidentResolve{expected_version, reason, evidence_refs[]?} → Incident | recovery evidence required unless authorized manual-resolve | key | If-Match | sync | RECOVERY_NOT_QUALIFIED, INVALID_TRANSITION | 119.6 |
| OP-INC-CLOSE | `POST /incidents/{id}:close` | incidents.manage | IncidentClose{expected_version, reason} → Incident | – | key | If-Match | sync | INVALID_TRANSITION | 119.6 |
| OP-INC-REOPEN | `POST /incidents/{id}:reopen` | incidents.reopen | IncidentReopen{expected_version, reason} → Incident | – | key | If-Match | sync | REOPEN_WINDOW_EXPIRED, INVALID_TRANSITION | 119.6 |
| OP-INC-MERGE | `POST /incidents/{id}:merge` | incidents.manage | IncidentMerge{expected_version, source_incident_ids UUID[] NN (1..N bound BUILD-130), reason, evidence_refs[]?} → Incident (survivor=`{id}`; sources CLOSED linked) | sources must be same-tenant, open-or-resolved per policy; alert links move to survivor | key | If-Match on survivor + optimistic checks on sources | sync | MERGE_SOURCE_INVALID, ALERT_ALREADY_LINKED_ELSEWHERE, STALE_VERSION, INVALID_TRANSITION | 119.6 |
| OP-INC-SPLIT | `POST /incidents/{id}:split` | incidents.manage | IncidentSplit{expected_version, alert_ids UUID[] NN, new_title?, reason, evidence_refs[]?} → {survivor: Incident, created: Incident} | listed ACTIVE links move to created incident; restores from merge event log when reversing a merge | key | If-Match | sync | SPLIT_ALERT_NOT_LINKED, INVALID_TRANSITION, STALE_VERSION | 119.6 |
| OP-INC-CREATE | `POST /incidents` | incidents.manage | {title, scope, linked_alert_ids?, reason} → Incident | – | key | – | sync | – | 119.6 |
| OP-RCA-GET/LIST-REVISIONS | `GET /incidents/{id}/rca`, `GET /rca/{rcaId}/revisions[/{n}]` | rca.view | → RcaInvestigation / RcaRevision | cursor | – | ETag=revision_no | – | – | 119.7 |
| OP-RCA-FEEDBACK / OP-RCA-RERUN | `POST /rca/{rcaId}/revisions/{n}:feedback`, `POST /rca/{rcaId}:rerun` | rca.feedback | RcaFeedback / {reason} | rerun rate-limited | key | revision pinned | rerun 202 | RERUN_RATE_LIMITED | 119.7 |
| OP-NCM-JOB-CREATE | `POST /ncm/jobs` | ncm.backup / ncm.cli.change | NcmJobCreate → NcmJob | targets bound (BUILD-130) | key | per-target lease | 202 | CREDENTIAL_FAILURE, UNSUPPORTED_ADAPTER, FREEZE_ACTIVE | 119.8 |
| OP-NCM-JOB-GET/LIST/CANCEL | `GET /ncm/jobs[/{id}]`, `POST /ncm/jobs/{id}:cancel` | ncm.read / ncm.backup | → NcmJob (+targets) | kind, state, entity; cursor | key (cancel) | If-Match | – | – | 119.8 |
| OP-NCM-CONFIG-LIST/GET/DIFF | `GET /entities/{id}/configs[/{versionId}]`, `GET /ncm/diffs?from=&to=` | ncm.read | → ConfigVersion / Diff | cursor; diff size bound | – | immutable | – | DIFF_TOO_LARGE | 119.8 |
| OP-NCM-CHANGE-CREATE | `POST /ncm/changes` | ncm.cli.change (+ncm.change.approve 1.5) | NcmManualChange (P1) / ChangePlan (P1.5) → NcmJob | – | key | If-Match (plan) | 202 | PRE_SNAPSHOT_FAILED, STALE_REAPPROVAL_REQUIRED (P1.5) | 119.9 |
| OP-NCM-ROLLBACK-CREATE | `POST /ncm/rollbacks` | ncm.rollback | NcmRollback → NcmJob | – | key | to_config_version | 202 | ROLLBACK_UNSUPPORTED, HASH_MISMATCH | 119.10 |
| OP-IPAM-RESERVE | `POST /ipam/reservations` | ipam.reserve | IpamReserve → IpamReservation | cidr expansion bound | key | SERIALIZABLE+lock | sync (bulk 202) | ADDRESS_RESERVED_BY_OTHER, OUTSIDE_MANAGED_SUBNET, SUBNET_OVERLAP, RETRY_LATER | 119.11 |
| OP-IPAM-RELEASE | `POST /ipam/reservations/{id}:release` | ipam.release | {reason} → reservation | – | key | If-Match | sync | – | 119.11 |
| OP-IPAM-CONFLICT-ACTION | `POST /ipam/conflicts/{id}:acknowledge|:resolve|:dismiss` | ipam.conflict.manage | {reason} → conflict | – | key | If-Match | sync | – | 119.11 |
| OP-IPAM-READ | `GET /ipam/vrfs|subnets|addresses|conflicts` | ipam.read | → pages | vrf, subnet, state; cursor | – | ETag | – | – | 119.11 |
| OP-RPT-DEF-* | `POST/PATCH/GET /report-definitions` | reports.manage | ReportDefinition → ReportDefinition | cursor | key | If-Match | sync | UNSUPPORTED_FORMAT (only PDF/CSV) | 119.12 |
| OP-RPT-RUN | `POST /report-runs` | reports.run | ReportRunRequest{definition_id, version, period?} → ReportRun (also linked AsyncOperation when 202) | query bounds (timeout/row/scan) | key | version snapshot | 202 | QUERY_LIMITED, INSUFFICIENT_DATA, RENDER_FAILED | 119.12 |
| OP-RPT-RUN-GET/LIST | `GET /report-runs[/{id}]` | reports.run / reports.download | → ReportRun / page (state, definition_version_no, quality, failure_code, planned_at, artifact summary) | state, definition_id, obligation_id, time window; cursor | – | ETag | – | RESOURCE_NOT_FOUND | 119.12 |
| OP-RPT-RUN-STATUS | `GET /report-runs/{id}` (same resource; clients may poll) **or** `GET /operations/{operationId}` when create returned AsyncOperation | reports.run | → ReportRun.state / AsyncOperation.state | – | – | ETag | – | RESOURCE_NOT_FOUND, OPERATION_EXPIRED | 119.12 |
| OP-RPT-RUN-HISTORY | `GET /report-runs` with `obligation_id` / `definition_id` filters (LIST) | reports.run | → page of historical runs (succeeded/failed/cancelled/expired) | obligation_id, definition_id, from/to planned_at; cursor; max window BUILD-130 | – | – | – | QUERY_LIMITED | 119.12 |
| OP-RPT-RUN-CANCEL | `POST /report-runs/{id}:cancel` | reports.run | {reason?} → ReportRun | – | key | If-Match / version | 202 | INVALID_TRANSITION | 119.12 |
| OP-RPT-ARTIFACT-GET | `GET /report-runs/{id}/artifact` | reports.download | → binary (PDF/CSV) + checksum header | – | – | – | – | ARTIFACT_EXPIRED, PERMISSION_DENIED_AT_EXECUTION | 119.12 |
| OP-RPT-SCHEDULE | `POST/PATCH /report-obligations`, `:pause`, `:resume`, `:cancel` | reports.manage | ReportObligation → obligation | – | key | If-Match | sync | – | 119.12 |
| OP-DIAG-JOB-CREATE | `POST /diagnostic-jobs` | diagnostics.run | DiagnosticJobCreate → DiagnosticJob | params bounded; tenant/target concurrency & rate budget | key | – | 202 | CAPACITY_LIMITED, TARGET_UNREACHABLE, CREDENTIAL_FAILURE, UNSUPPORTED_DIAGNOSTIC | 119.13 |
| OP-DIAG-JOB-GET/LIST/CANCEL | `GET /diagnostic-jobs[/{id}]`, `POST …:cancel` | diagnostics.view / diagnostics.run | → DiagnosticJob (+vantage results) | origin, entity, state; cursor | key (cancel) | If-Match | cancel 202 | – | 119.13 |
| OP-LIVE-EVIDENCE-GET | `GET /entities/{id}/live-evidence` (also `/incidents/{id}/live-evidence`) | evidence.view | → LiveEvidence{monitored[], on_demand[]} | types, include_on_demand; cursor | – | – | – | per-section QUERY_LIMITED/UNAVAILABLE (200 partial) | 119.14 |
| OP-INV-SESSION-CREATE/GET/LIST/CLOSE/REOPEN | `POST/GET /investigation-sessions`, `:close`, `:reopen` | investigation.manage | → InvestigationSession | state, owner, incident; cursor | key | If-Match | sync | SESSION_CLOSED | 119.15 |
| OP-INV-SESSION-ATTACH/DETACH | `POST /investigation-sessions/{id}/evidence`, `DELETE …/{seq}` | investigation.manage | {evidence_ref} → link | batch bound | key (hash of ref) | If-Match | sync | EVIDENCE_NOT_AUTHORIZED, EVIDENCE_REF_NOT_FOUND | 119.15 |
| OP-CLI-SESSION-CREATE | `POST /cli-sessions` | cli.session | CliSessionCreate → CliSessionMeta{session_id, ws_endpoint, short_lived_token, expires_at} (never a reusable secret) | concurrent-session cap | key | – | sync | MFA_REQUIRED, CAPACITY_LIMITED, POLICY_COMMAND_BLOCKED | 119.16 |
| OP-CLI-SESSION-TERMINATE / GET / LIST | `POST /cli-sessions/{id}:terminate`, `GET …` | cli.session / cli.admin | → CliSessionMeta | user, target, state; cursor | key | If-Match | sync | – | 119.16 |
| OP-NTF-LIST/COUNT | `GET /notifications`, `GET /notifications:count` | notifications.read | → page / {unread} | category, state; cursor | – | – | – | – | 119.17 |
| OP-NTF-ACTION | `POST /notifications/{id}:read|:unread|:dismiss|:action` | notifications.read | {} → recipient state | – | idempotent by nature | – | sync | SOURCE_NOT_AUTHORIZED | 119.17 |
| OP-NTF-SUBSCRIPTION | `GET/PUT /notification-subscriptions` | notifications.read / notifications.admin | → NotificationSubscription | – | key | If-Match | sync | – | 119.17 |
| OP-ITSM-CONNECTOR-* | `POST/PATCH/GET /integrations/connectors`, `:test`, `:enable`, `:disable` | integration.manage | ConnectorConfig → connector | – | key | If-Match | `:test` 202 | AUTH_EXPIRED, MAPPING_CONFLICT | 119.18 |
| OP-ITSM-RECONCILE / OP-ITSM-REPLAY | `POST /integrations/bindings/{id}:reconcile`, `POST /integrations/outbox/{opId}:replay` | integration.manage | {} → operation | rate-limited | key | If-Match | 202 | DEAD_NOT_REPLAYABLE | 119.18 |
| OP-INBOUND-WEBHOOK | `POST /integrations/inbound/{connectorId}` (tenant derived from authenticated ingress) | connector credential | external event → 202 | payload size bound | external_event_id | – | 202 | VERIFICATION_FAILED (401/403), DUPLICATE (200) | 119.18 |
| OP-STO-RETENTION-PREVIEW/ACTIVATE | `POST /storage/retention-revisions:preview`, `POST …/{id}:activate` | storage.manage | {data_class, requested_retention} → preview{model, headroom, shortfall, protected, uncertainty} | – | key | preview snapshot version | sync | STALE_PREVIEW, PROTECTED_DATA_CONFLICT, INSUFFICIENT_INPUT_UNKNOWN | 119.19 |
| OP-STO-ACTION | `POST /storage/actions` / `GET` / `:cancel` | storage.manage | StorageActionRequest → StorageAction | – | key | single active per volume/class | 202 | PURGE_FAILED | 119.19 |
| OP-BKP-RUN / OP-BKP-RESTORE | `POST /platform/backups`, `POST /platform/restores` | platform.backup / platform.restore (+approver) | → PlatformOperation | – | key | single active per scope | 202 | PREFLIGHT_FAILED, INTEGRITY_MISMATCH, INCOMPATIBLE_ARTIFACT | 119.20 |
| OP-UPG-RUN / OP-UPG-ROLLBACK | `POST /platform/upgrades`, `POST /platform/upgrades/{id}:rollback` | platform.upgrade (+approver) | → PlatformOperation | – | key | single active per scope | 202 | PREFLIGHT_FAILED, MIGRATION_FAILED, ROLLBACK_UNSUPPORTED, STALE_ARTIFACT | 119.21 |
| OP-LIC-LOAD / OP-LIC-EXTEND / OP-LIC-GET | `POST /licence`, `POST /licence:extend`, `GET /licence` | licence.manage / licence.read | {artifact}/{until, reason} → LicenceState | – | artifact hash | If-Match | sync | LICENCE_INVALID | 119.22 |
| OP-PROFILE-ASSIGN | `PUT /entities/{id}/monitoring-profile` | profile.assign | {profile_id, mode} → assignment | – | key | If-Match | sync | PROFILE_CAPACITY_BLOCKED | 119.1/UI-032 |
| OP-POLICY-CREATE | `POST /alert-policies` | policy.manage | AlertPolicyCreate{name, evidence_class, scope, definition:`AlertPolicyDefinitionV1`} → AlertPolicy(+version_no=1, lifecycle=DRAFT) | server-side `AlertPolicyDefinitionV1` validator runs **before persist**; same typed field/path errors as OP-POLICY-VERSION-VALIDATE | key | – | sync | VALIDATION_FAILED (typed field/path errors; never silent normalize) | UI-059 Policies / Dashboard→Alert Policy |
| OP-POLICY-LIST | `GET /alert-policies` | policy.read | → `PageResult<AlertPolicySummary>` | lifecycle, evidence_class, name; cursor (CC-CURSOR) | – | ETag | – | – | UI-059/091 |
| OP-POLICY-GET | `GET /alert-policies/{policyId}` | policy.read | → AlertPolicy(+current version summary) | – | – | ETag | – | RESOURCE_NOT_FOUND | UI-059/091 |
| OP-POLICY-VERSION-CREATE | `POST /alert-policies/{policyId}/versions` | policy.manage | {definition:`AlertPolicyDefinitionV1`, change_reason?} → immutable AlertPolicyVersion | create/version requests are themselves the pre-persist validation boundary (same validator as OP-POLICY-VERSION-VALIDATE) | key | If-Match on policy aggregate | sync | VALIDATION_FAILED (typed field/path); STALE_VERSION | UI-059 Policies |
| OP-POLICY-VERSION-LIST | `GET /alert-policies/{policyId}/versions` | policy.read | → page of versions | cursor | – | – | – | – | UI-059/091 |
| OP-POLICY-VERSION-GET | `GET /alert-policies/{policyId}/versions/{versionNo}` | policy.read | → AlertPolicyVersion{definition:`AlertPolicyDefinitionV1`, content_hash, state_at_creation, …} | – | – | immutable | – | RESOURCE_NOT_FOUND | UI-059/091 |
| OP-POLICY-VERSION-VALIDATE | `POST /alert-policies/{policyId}/versions/{versionNo}:validate` | policy.manage | → ValidationResult{ok, errors[{path, code, message_key}]} for an **already persisted immutable version** | **ONLY** standalone validation operation — do **not** expose a second public "validate arbitrary body" route; OP-POLICY-CREATE / OP-POLICY-VERSION-CREATE invoke the same server-side validator before persistence | key | – | sync | VALIDATION_FAILED with typed paths; never silently rewrite invalid threshold | UI-059 Policies |
| OP-POLICY-LIFECYCLE | `POST /alert-policies/{policyId}:advance` (DRAFT→BACKTEST→SHADOW→ACTIVE) | policy.manage | **must identify** `{version_no, to_state, evidence_ref?, approval_ref?}` → policy | advances the named immutable version only | key | If-Match | backtest/shadow may 202 (`ACCEPTED_FOR_ASYNC_EXECUTION` → AsyncOperation `QUEUED`) | EVIDENCE_REQUIRED, INVALID_TRANSITION, VERSION_NOT_FOUND | UI-059/091 |
| OP-AUDIT-EXPORT | `POST /audit/exports` | audit.export | {filters, window, format CSV} → export job | bounded window/rows | key | – | 202 | QUERY_LIMITED | UI-083 |
| OP-SENSOR-REBALANCE-PREVIEW | `POST /sensors/rebalance:preview` | sensor.assign | RebalancePreviewRequest → RebalancePreview | candidate bound (BUILD-130) | key | – | sync | NO_CAPACITY, STALE_ASSIGNMENT | UI-074; DF-009 |
| OP-SENSOR-DRAIN | `POST /sensors/{sensorId}:drain` | sensor.assign | {reason, drain_mode} → SensorDrainJob | – | key | If-Match | 202 | INVALID_TRANSITION | UI-074; DF-010 |
| OP-ACCESS-REQUEST-DECIDE | `POST /access-requests/{id}:decide` | access.approve | AccessDecision → AccessRequest | – | key | If-Match | sync | INVALID_TRANSITION, MFA_REQUIRED | UI-084; `Platform_20` §3 |
| OP-SLA-DISPUTE | `POST /service-assurance/disputes` | sla.dispute | SlaDisputeCreate → SlaDispute | – | key | – | sync | PERIOD_LOCKED, INSUFFICIENT_EVIDENCE | UI-071; `Platform_21` / §8.2 |
| OP-HANDOVER-CREATE | `POST /shift-handovers` | operations.handover | HandoverCreate → ShiftHandover | – | key | – | sync | – | UI-094; DF-061 |
| OP-PIR-CREATE | `POST /post-incident-reviews` | pir.manage | PirCreate → PostIncidentReview | – | key | – | sync | INCIDENT_NOT_CLOSED | UI-066; DF-059 |
| OP-RUNBOOK-UPSERT | `PUT /runbooks/{id}` | runbook.manage | RunbookUpsert → Runbook | – | key | If-Match | sync | STALE_VERSION | UI-065; DF-065 |
| OP-MIGRATION-IMPORT | `POST /migrations/imports` | migration.manage | MigrationImport → MigrationJob | file/row bound (BUILD-130) | key | – | 202 | IMPORT_ROW_ERRORS | UI-077; DF-068 |
| OP-NCM-COMPLIANCE-ACK | `POST /ncm/compliance/{findingId}:acknowledge` | ncm.compliance.ack | {reason, until?} → ComplianceFinding | – | key | If-Match | sync | ALREADY_ACKNOWLEDGED | UI-043; NCM compliance |

### 15.5 Events (logical topics; physical selection BUILD-063)

Common: CC-EVT envelope; producer writes via CC-OUTBOX in the source transaction; consumers are idempotent on `event_id`; DLQ = parked topic `<topic>.dlq` (poison parking with reason/offset; never silently dropped); compatibility = additive optional fields only within a major version.

| Topic / name | Schema ver | Payload contract | Producer | Consumers | Identity / partition key | Times | Dedupe | Quality | Failure / DLQ |
|---|---|---|---|---|---|---|---|---|---|
| `entity.lifecycle.v1` | 1.0 | `EntityLifecyclePayload` v1 (§15.4 inventory/provisioning DTOs; required: tenant_id, entity_id, lifecycle_state, revision, event_time; optional: site_id, profile_id, reason; authority: PostgreSQL entity; ordering: revision; compat: additive optional within major) | Inventory/Provisioning | Policy, Sensor control, Licensing, Timeline, Notifications | entity_id / `tenant_id|entity_id` | event_time, ingest_time | event_id | n/a | DLQ + replay from outbox |
| `onboarding.candidate.v1` | 1.0 | `OnboardingCandidatePayload` v1 (`Platform_20` §6.1 / §15.2 onboarding tables; required: tenant_id, candidate_id, dedupe_key, lifecycle_state, revision; optional: entity_id, agent_id, proposed_*; authority: onboarding_candidate; ordering: revision; compat: additive) | Onboarding | Notifications, Timeline | candidate_id / `tenant_id|dedupe_key` | event_time | event_id + candidate revision | n/a | DLQ |
| `agent.assignment.v1` | 1.0 | `AgentAssignmentPayload` v1 (required: tenant_id, agent_id, assignment_generation, sensor_id/target, state; optional: reason; authority: Agent control; ordering: generation; compat: additive) | Agent control | Sensor control, UI channel | agent_id / `tenant_id|agent_id` | event_time | (agent_id, generation) | n/a | stale-generation consumer rejects |
| `policy.change.v1` / `maintenance.window.v1` | 1.0 | `PolicyChangePayload` / `MaintenanceWindowPayload` v1 — see §15.5.1 (typed `policy_id`+`version_no`+`lifecycle_seq`+`lifecycle_state` / `window_id`+`version`+`state`; lifecycle ordering is `lifecycle_seq`, independent of immutable definition `version_no`; no ambiguous untyped version_no/state) | Policy | Alert engine, Notifications, Service Assurance | policy/window id / `tenant_id|scope_hash` | event_time | event_id + `(policy_id, lifecycle_seq)` / `(window_id, version)` | n/a | DLQ |
| `alert.lifecycle.v1` | 1.0 | `AlertLifecyclePayload` v1 — see §15.5.1 (required from_state/to_state, entity, policy/policy_version, semantic_state, severity, confidence, evidence reference) | Alert engine | Incident engine, Notifications, ITSM outbox, ClickHouse `alert_history`, Timeline | alert_id / `tenant_id|alert_id` (ordered per alert) | event_time (state change), ingest_time | (alert_id, seq) | semantic_state + confidence + evidence quality | DLQ; consumer fail-closed ACK |
| `incident.lifecycle.v1` | 1.0 | `IncidentLifecyclePayload` v1 (§15.2 incident tables; required: tenant_id, incident_id, seq, state, event_time; optional: priority, owner, linked_alert_ids; authority: incident; ordering: (incident_id, seq); compat: additive) | Incident service | RCA, Notifications, ITSM, `incident_history`, Timeline | incident_id / `tenant_id|incident_id` | event_time | (incident_id, seq) | n/a | DLQ |
| `rca.revision.v1` | 1.0 | `RcaRevisionPayload` v1 (§15.2 rca_*; required: tenant_id, rca_id, revision_no, outcome, confidence, event_time; optional: contradictions, missing_evidence; authority: rca_investigation; ordering: revision_no; compat: additive) | RCA engine | UI channel, Timeline, evaluation | rca_id / `tenant_id|rca_id` | event_time | (rca_id, revision_no) | confidence, outcome | DLQ |
| `ncm.job.v1` | 1.0 | `NcmJobPayload` v1 (§15.2 ncm_job*; required: tenant_id, job_id, entity_id, kind, state, event_time; optional: target results; authority: ncm_job; ordering: (job_id, target, state); compat: additive) | NCM | Timeline, Incident, Notifications | job_id / `tenant_id|entity_id` | event_time | (job_id, target, state) | n/a | DLQ |
| `ipam.change.v1` | 1.0 | `IpamChangePayload` v1 (§15.2 IPAM; required: tenant_id, change_type, object_id, event_time; optional: vrf_id, address; authority: IPAM tables; ordering: event_id; compat: additive) | IPAM | Timeline, Discovery, CMDB sync | address_id / `tenant_id|vrf_id` | event_time | event_id | n/a | DLQ |
| `report.run.v1` | 1.0 | `ReportRunPayload` v1 (§15.2 report_run; required: tenant_id, run_id, state, definition_version_no, event_time; optional: quality, failure_code; authority: report_run; ordering: (run_id, state); compat: additive) | Reporting | Notifications, Delivery | run_id / `tenant_id|run_id` | event_time | (run_id, state) | quality summary | DLQ |
| `notification.request.v1` | 1.0 | `NotificationRequestPayload` v1 (§15.2 user_notification; required: tenant_id, notification_id, category, dedupe_key, event_time; optional: action_required; authority: user_notification; ordering: event_id; compat: additive) | Notification module | Integration Service delivery | notification_id / `tenant_id|recipient_hash` | event_time | dedupe_key | n/a | DLQ + delivery retry state in PostgreSQL |
| `diagnostic.job.v1` | 1.0 | `DiagnosticJobPayload` v1 (§15.2 diagnostic_job*; required: tenant_id, job_id, target_entity_id, state, event_time; optional: vantage results (**no secrets**); authority: diagnostic_job; ordering: (job_id, vantage_id, state); compat: additive) | Diagnostics | Timeline, Investigation, RCA, analytics | job_id / `tenant_id|target_entity_id` | event_time | (job_id, vantage_id, state) | per-vantage quality | DLQ; **no secrets** |
| `investigation.session.v1` | 1.0 | `InvestigationSessionPayload` v1 (§15.2 investigation_*; required: tenant_id, investigation_id, seq, state, event_time; optional: evidence links; authority: investigation_session; ordering: (investigation_id, seq); compat: additive) | Investigation | Timeline, Incident | investigation_id / `tenant_id|investigation_id` | event_time | (investigation_id, seq) | n/a | DLQ |
| `cli.session.v1` | 1.0 | `CliSessionPayload` v1 (§15.2 cli_session; required: tenant_id, session_id, state, entity_id, event_time; **forbidden:** credentials/keystrokes; authority: cli_session; ordering: (session_id, state); compat: additive) | CLI Broker | Audit, Timeline | session_id / `tenant_id|session_id` | event_time | (session_id, state) | n/a | DLQ; **lifecycle metadata only — no keystrokes, no secrets** |
| `integration.outbound.v1` / `integration.inbound.v1` | 1.0 | `IntegrationOutboundPayload` / `IntegrationInboundPayload` v1 — see §15.5.1 (**operation_hash required** on outbound; inbound uses external_event_id; no ambiguous ordering token) | Integration Service | Incident/Alert, Timeline, UI channel | operation_id / external_event_id; `tenant_id|connector_id` | event_time, received_time | operation_hash / external_event_id | verification result | DLQ + replayable DEAD |
| `audit.event.v1` | 1.0 | `AuditEventPayload` v1 — see §15.5.1 (authoritative actor/resource fields required, not only audit_id/action/result) | all (via outbox) | Audit durable store, SIEM export, ClickHouse `audit_history` | audit_id / `tenant_id` (ordered by tenant_seq) | occurred_at | (tenant_id, tenant_seq) | n/a | DLQ; **authoritative copy is PostgreSQL** |
| `licence.state.v1` | 1.0 | `LicenceStatePayload` v1 (§15.2 licence; required: tenant_id, licence_id, state, expiry_at, event_time; optional: grace_until; authority: licence tables; ordering: (licence_id, state, expiry); compat: additive) | Licensing | Scheduler, UI channel, Notifications | licence_id / `tenant_id` | derived_from_expiry | (licence_id, state, expiry) | n/a | DLQ |
| `platform.operation.v1` | 1.0 | `PlatformOperationPayload` v1 (§15.2 platform_operation*; required: tenant_id, operation_id, kind, state, event_time; optional: step, artifact_ref; authority: platform_operation; ordering: (operation_id, step, state); compat: additive) | Platform Operations | UI channel, Notifications, Audit | operation_id / `tenant_id|scope` | event_time | (operation_id, step, state) | n/a | DLQ |
| `telemetry.flow.v1` / `telemetry.log.v1` / `telemetry.event.v1` (traps) | 1.0 | Typed telemetry payloads per §11 / BUILD-062 envelope (required: tenant_id, source identity, sequence_id, event_time, observation family fields; optional: quality; authority: authenticated ingest admission; ordering: sequence_id; compat: additive typed-record versions) | Ingestion gateway (after authenticated admission) | ClickHouse writers, Alert engine, Flow/Log analytics | source/exporter id / `tenant_id|exporter_id|observation_domain` etc. | event_time, ingest_time | sequence_id | quality code, quarantine class | committed-only consumer; poison parking; **ambiguous tenant → quarantine, never produced** |
| `telemetry.quality.v1` | 1.0 | `TelemetryQualityPayload` v1 (§15.3 `telemetry_quality_event`; required: tenant_id, source_id, signal_class, state, event_time, sequence_id; optional: counters; authority: quality writers; ordering: sequence_id; compat: additive) | Sensor/Core | `telemetry_quality_event`, Alert qualification | source_id / `tenant_id|source_id` | event_time | sequence_id | state | DLQ |
| `telemetry.processnet.v1` / `telemetry.path.v1` | 1.0 | `ProcessNetworkEvidencePayload` / `ActivePathObservationPayload` v1 (§11.2 / §11.3; required typed fields + sequence_id + quality; authority: typed ingest; ordering: sequence_id; compat: pinned typed-record versions) | Ingestion gateway (typed Agent/Sensor evidence) | `process_network_evidence`, `active_path_*`, RCA | agent/path id / `tenant_id|entity_id` or `tenant_id|path_id` | bucket/probe time, ingest time | sequence_id | quality_state / path_identity_quality | DLQ; unsupported typed record is rejected explicitly, never silently ACKed |
| `topology.observation.v1` | 1.0 | `TopologyObservationPayload` v1 (§15.3 `topology_observation_history`; required: tenant_id, edge_id, src/dst, relationship, provenance, confidence, event_time, sequence_id; authority: topology writers; ordering: (edge_id, sequence); compat: additive) | Topology | `topology_observation_history`, graph | edge_id / `tenant_id|edge_id` | observed time | (edge_id, sequence) | provenance+confidence | spoof/cross-tenant rejected pre-publish |

#### 15.5.1 Touched event payload field contracts (v1)

Compatibility for all rows below: **additive optional fields only within major**; unknown fields ignored by forward-compatible consumers; rename/type/enum removal = new major topic. Ordering/version tokens are authoritative for consumer dedupe/order; `event_id` remains the transport idempotency key (CC-EVT).

**`AuditEventPayload` v1** (`audit.event.v1`) — authority = PostgreSQL `audit_event`

| Field | Type | Req | Enum / nested | Ordering / version token | Notes |
|---|---|---|---|---|---|
| tenant_id | UUID/string | R | – | partition with tenant_seq | server-derived |
| audit_id | UUID | R | – | – | PK fragment |
| tenant_seq | INT64 | R | – | **ordering + dedupe `(tenant_id, tenant_seq)`** | monotonic per tenant |
| occurred_at | TIMESTAMPTZ | R | – | event_time | |
| actor_type | string | R | USER\|SERVICE\|SYSTEM | – | authoritative actor |
| actor_id | string(≤128) | R | – | – | authoritative actor |
| action | string(≤96) | R | – | – | |
| resource_type | string(≤48) | R | – | – | authoritative resource |
| resource_id | string(≤128) | O | – | – | null only for non-resource actions |
| resource_version | INT64 | O | – | – | when resource is versioned |
| result | string | R | SUCCESS\|DENIED\|FAILED | – | |
| reason | string(≤1000) | O | – | – | |
| before_hash / after_hash | bytes | O | – | – | |
| correlation_id | string | O | – | – | |
| integrity_ref | string | O | – | – | reserved |
| detail | object (bounded JSON) | O | secret-free | – | never credentials |

**`AlertLifecyclePayload` v1** (`alert.lifecycle.v1`) — authority = `alert_current` + `alert_lifecycle_event`

| Field | Type | Req | Enum / nested | Ordering / version token | Notes |
|---|---|---|---|---|---|
| tenant_id | UUID/string | R | – | with alert_id | |
| alert_id | UUID | R | – | – | |
| seq | INT64 | R | – | **ordering/dedupe `(alert_id, seq)`** | under alert row lock |
| from_state | string | R | NEW\|ACKNOWLEDGED\|ASSIGNED\|INVESTIGATING\|MONITORING_RECOVERY\|RESOLVED\|CLOSED (lifecycle) | – | explicit; not implied |
| to_state | string | R | same lifecycle enum | – | explicit |
| entity_id | UUID | R | – | – | monitored entity |
| policy_id | UUID | R | – | – | or immutable snapshot ref below |
| policy_version_no | INT32 | R | – | – | exact version that qualified |
| semantic_state | string | R | UP\|DOWN\|DEGRADED\|UNKNOWN\|NO_DATA\|STALE\|MONITORING_DEGRADED\|REDUNDANCY_LOST\|SLA_DEGRADED\|EXPECTED_STATE | – | separate from severity |
| severity | string | R | bounded severity vocab | – | impact dimension |
| confidence | string | R | bounded confidence vocab | – | evidence dimension; never collapsed into severity |
| evidence_ref | string | R | – | – | immutable reference into evidence_ref / contract id |
| event_time | TIMESTAMPTZ | R | – | state-change time | |
| ingest_time | TIMESTAMPTZ | O | – | – | Core accept time |
| assignee_ref | string | O | – | – | |
| actor_type / actor_id | string | R | USER\|SERVICE\|SYSTEM | – | who drove the transition |
| reason | string | O | – | – | |
| overlay_flags | string[] | O | – | – | SUPPRESSED_*/MUTED etc. |

**`PolicyChangePayload` v1** / **`MaintenanceWindowPayload` v1** — no untyped `version_no/state` pair

| Payload | Required typed fields | Ordering token | Compat |
|---|---|---|---|
| PolicyChangePayload | tenant_id, policy_id UUID, version_no INT32, **lifecycle_seq INT64**, lifecycle_state DRAFT\|BACKTEST\|SHADOW\|ACTIVE\|RETIRED, event_time; optional: scope, evidence_ref, content_hash | **`(policy_id, lifecycle_seq)`** | additive within major for optional fields; `lifecycle_seq` is required and independent of immutable definition `version_no` |
| MaintenanceWindowPayload | tenant_id, window_id UUID, version INT64, state SCHEDULED\|ACTIVE\|ENDED\|CANCELLED\|EXPIRED, event_time; optional: scope, effect, timezone | `(window_id, version)` | additive |

`lifecycle_seq` is a per-`policy_id` monotonically increasing aggregate revision assigned on every lifecycle transition (including DRAFT→BACKTEST→SHADOW→ACTIVE on the same immutable `version_no`). Consumers MUST order and dedupe by `(policy_id, lifecycle_seq)`; a lower `lifecycle_seq` after a higher applied seq is stale and MUST NOT roll lifecycle state backward. `version_no` remains the immutable definition identity and MUST NOT be treated alone as the lifecycle ordering/dedupe token.

**`IntegrationOutboundPayload` v1** / **`IntegrationInboundPayload` v1**

| Field | Outbound | Inbound | Req | Notes |
|---|---|---|---|---|
| tenant_id | ✓ | ✓ | R | server-derived |
| connector_id | ✓ | ✓ | R | |
| operation_id | ✓ | – | R outbound | outbox PK |
| source_event_id | ✓ | – | **R outbound** | immutable originating domain-event identity; persisted as `integration_outbox.created_in_txn_event_id`; aligns with Platform_06 §119.18 `event_seq` / source-event sequencing. Stable across retries of the same outbox operation; distinct for each new source revision (so A→B→A cannot collide with the earlier A). |
| operation_hash | ✓ | – | **R outbound** | BYTEA; **included in contract** (not merely derived off-band). Immutable hash of (tenant_id, connector_id, object_type, object_id, mapping_version, source_event_id). Canonical mapped payload is carried for delivery but is **not** part of the identity hash. Used for UNIQUE dedupe and external idempotency. |
| external_event_id | – | ✓ | R inbound | UNIQUE with connector |
| object_type / object_id | ✓ | O | R outbound | |
| mapping_version | ✓ | O | R outbound | |
| state | ✓ | ✓ | R | outbound PENDING\|SENDING\|DELIVERED\|RETRY\|DEAD; inbound RECEIVED\|PROCESSED\|REJECTED\|DEAD |
| event_time / received_time | ✓ | ✓ | R | |
| verification | – | ✓ | R inbound | VERIFIED\|FAILED |
| mapping_refs / error_code | O | O | O | additive |

Ordering/dedupe: outbound **`operation_hash`** (and operation_id for transport); inbound **`external_event_id`**. Compatibility: additive optional fields within major; **changing hash canonicalization = major**. Pre-implementation correction (Issue #308): the defective content-only hash inputs are replaced by source-event identity inputs above before any shipped consumer; post-ship changes to hash inputs remain a new major.

### 15.6 Migration plan (`MIG-PLAN-*`; no V019+ filenames chosen)

Ordering rule: plans execute in the listed order; PostgreSQL plans precede dependent ClickHouse plans; one numbered SQL file may realize one or more plans (numbering assigned at merge from the then-current `manifest.txt`). Every applied migration stays immutable (§4.3); fixes are new forward migrations.

**Supported upgrade origins (`UPGRADE_ORIGIN`; authoritative with `Platform_16` §5):** `EMPTY_INSTALL→CURRENT`, `PREVIOUS_GA_PACKAGE→CURRENT` (Phase-1 standalone), and (Phase 1.5 only) `HA_STANDBY_FIRST_ROLLING→CURRENT`. Mixed Sensor/Agent versions are a reported compatibility state, not a Core partial-schema origin. “Every supported origin” means exactly this matrix — not an unbounded set.

**Non-trivial migration phases (mandatory columns below):** expand → backfill → validate → cutover → feature activation → contract/removal (where applicable). Record lock/downtime, mixed-version behavior, retry/resume after partial application, irreversible boundary, and rollback-versus-restore. **Rollback** = reversible expand/cutover undo where listed; otherwise restore-from-verified-backup plus post-restore validation.

| Plan | Order | Owner BUILD | Content | Expand | Backfill | Validate | Cutover / feature gate | Lock / mixed-version / resume | Irreversible boundary | Rollback vs restore | Verification |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MIG-PLAN-001 | 1 | 061/073 | `api_idempotency_record`, `event_outbox`, `async_operation`, **non-partitioned** `audit_event` PK `(tenant_id, audit_id)` + UNIQUE `(tenant_id, tenant_seq)` + time/resource/actor indexes; same-tenant helpers | CREATE new tables/indexes (`IF NOT EXISTS`); **no** monthly partition parent/children for Phase-1 `audit_event` | none | REVOKE UPDATE/DELETE on `audit_event`; PK/UNIQUE present; append-only insert probe; async_operation state CHECK | writers enabled after validate; no partition-creation claim | online DDL preferred; old app ignores new tables; retry safe (`IF NOT EXISTS`) | once any `audit_event` row exists, DROP is restore-only | DOWN drops empty new tables only; populated → restore pre-migration backup + post-restore row/constraint checks | origins EMPTY/PREVIOUS_GA; constraint + append-only tests; **no partition creation** |
| MIG-PLAN-002 | 2 | 066/068 | `provisioning_request`, `onboarding_candidate/decision`, `entity_agent_binding`; CHECK vocabulary expansion on `entity` / `discovery_candidate` via **executable cutover sequence below** (not dual-CHECK steady state) | CREATE new tables; then execute §15.6.1 steps 1–8 for entity + discovery_candidate CHECKs | map `discovery_candidate` → `onboarding_candidate` idempotent by exact `dedupe_key` (§15.6.1); no entity rewrite | parity counts; no duplicate `(tenant_id, dedupe_key)`; mapping criteria tests; CHECK holds only the intended single constraint after step 5 | **new-state writer feature flag activates only after step 6 succeeds** | online expand; mixed-version: app continues WRITE old states through step 3; quiesce/fence writers at step 4; resume re-runs backfill from recorded cursor | one-way after any new-state row exists **and** legacy CHECK removed (step 5) | failure/resume + restore boundary per §15.6.1 step 8 | origins EMPTY/PREVIOUS_GA; state-map + concurrency + dedupe collision tests |
| MIG-PLAN-003 | 3 | 070/083 | policy/version, profile/version/assignment, maintenance, suppression, mute | CREATE tables/indexes | none (no pre-existing policy data) | immutability + EXPLICIT_PROFILE-not-replaced | activate writers after validate | online; old app ignores; retry `IF NOT EXISTS` | one-way after first policy/version row | DOWN while empty; else restore | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-004 | 4 | 081/084/100 | `alert_current`, `alert_lifecycle_event`, `incident*`, `rca_*`, `evidence_ref` | CREATE tables/indexes/partial UNIQUEs | none | partial-unique concurrency + seq-under-lock | enable alert/incident writers after validate | online; mixed-version ignores new tables; resume safe | one-way after first alert/incident row | DOWN while empty; else restore | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-005 | 5 | 090/091 | `ncm_job*` (**cli_session_id UUID NULL column without FK**), `ncm_config_version`; plan/approval tables created **inactive** (Phase 1.5) | CREATE tables incl. nullable `ncm_job.cli_session_id` **with no FK** (cli_session absent until MIG-PLAN-009); Phase-1.5 objects inert | none | hash-dedupe + one-active-target; assert **no** FK to cli_session exists yet | Phase-1 NCM writers after validate; Phase-1.5 plans remain feature-gated; MANUAL_CLI jobs may store cli_session_id only after MIG-PLAN-009 FK exists (application must not require FK before 009) | online; resume safe; no forward-reference FK | one-way after first ncm_job/config row | DOWN while empty; else restore | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-006 | 6 | 092 | IPAM tables | CREATE tables/indexes | optional observed_state import from discovery/DHCP (idempotent) | reservation partial-unique + overlap-lock | enable IPAM reserve/release after validate | online; mixed-version safe; resume re-imports | one-way after first reservation/address row | DOWN while empty; else restore | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-007 | 7 | 095 | report tables (`format CHECK PDF/CSV`) | CREATE tables/checks | none | UNIQUE (obligation, planned_at) + format negative | enable report define/run after validate | online; resume safe | one-way after first definition/run | DOWN while empty; else restore | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-008 | 8 | 086 | notification tables | CREATE tables/indexes | none | dedupe + per-recipient auth re-check | enable Notification Center writers after validate | online; resume safe | one-way after first notification row | DOWN while empty; else restore | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-009 | 9 | 012/015/091 | diagnostics, investigation, CLI session tables; **add** `ncm_job` → `cli_session` same-tenant FK on existing nullable `cli_session_id` | CREATE diagnostic/investigation/cli_session tables/indexes; then `ALTER TABLE ncm_job ADD CONSTRAINT fk_ncm_job_cli_session …` (nullable FK; existing NULL rows ok) | none (orphan non-NULL cli_session_id values before FK are rejected at ADD CONSTRAINT — pre-clean or NULL them in expand) | no-secret-column + EDS partial-unique + FK validates | enable diagnostic/CLI APIs after validate; NCM MANUAL_CLI may reference sessions after FK | online; resume safe; **FK ownership coherent here, not in 005** | one-way after first job/session | DOWN while empty; else restore | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-010 | 10 | 086 | integration tables | CREATE tables/indexes | none | outbox/inbox uniqueness + lease concurrency | enable connectors after validate; inbound gated per connector | online; resume safe | one-way after first connector/outbox row | DOWN while empty; else restore | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-011 | 11 | 096 | licence tables | CREATE tables | none (no entitlement guessing; quantities `FOUNDER-RESERVED`) | state-history idempotency; T-7 vs `EXPIRED_GRACE_15D` | enable licence load/extend after validate | online; resume safe | one-way after first licence artifact | DOWN while empty; else restore | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-012 | 12 | 097/125/132 | `platform_operation*`, retention/storage/legal-hold tables | CREATE tables/indexes | none | single-active-operation partial-unique; STALE_PREVIEW | enable backup/retention ops after validate | online; resume by operation step seq | one-way after first platform_operation row | DOWN while empty; else restore | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-013 | 13 | 064 | ClickHouse `flow_record_raw` (+rollups), `log_event_raw`, `trap_event_raw` + `FINAL` views | CREATE tables/views (`IF NOT EXISTS`); never alters existing `metric_*` | **none for historical CH rows** — analytical history begins at cutover; **Kafka replay is not a substitute for CH history cutover** (replay only covers retained Kafka window per BUILD-063) | replay-collapse, quarantine-not-inserted, cardinality-budget on fixture | dual-write/shadow validate optional; cutover consumers to new tables; feature-gate new Flow/Log writers | online DDL; mixed-version readers use FINAL views; resume `IF NOT EXISTS` | empty DROP allowed; populated recreate+selective Kafka replay ≠ full history restore | recreate tables; replay only within Kafka retention; document history gap | origins EMPTY/PREVIOUS_GA; CH+Kafka retention matrix |
| MIG-PLAN-014 | 14 | 064/074 | `audit_history` (**ReplacingMergeTree**, not uniqueness-by-ORDER-BY), `telemetry_quality_event`, `topology_observation_history` | CREATE CH tables + `audit_history_final` FINAL view | backfill `audit_history` from PostgreSQL `audit_event` with persisted cutover watermark `(tenant_id, tenant_seq)`, overlap re-copy, `_version` bump; **repeated backfill must not create duplicate authoritative analytical rows** | PG↔CH parity on `(tenant_id, tenant_seq)` counts + sample hash; legal-hold retention behavior | enable CH audit consumers after parity; PG remains authoritative; any audit rollup rebuilt from FINAL after backfill | online; resume from watermark; mixed-version safe | CH drop/rebuild allowed; PG authoritative copy retained | rebuild CH from PostgreSQL; not Kafka replay | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-015 | 15 | 064 | `process_network_evidence`, `active_path_observation/hop` | CREATE typed CH tables | none | no-prohibited-column; comparison-eligibility defaults | feature-gate typed writers after validate | online; resume safe; pinned schema versions | recreate allowed | recreate; Kafka replay within retention only | origins EMPTY/PREVIOUS_GA |
| MIG-PLAN-016 | 16 | 064/081 | `alert_history`, `incident_history` + MVs | CREATE CH tables/MVs | backfill from PostgreSQL lifecycle (idempotent by (id, seq)) | parity counts | enable CH history readers after parity; PG lifecycle remains control authority | online; resume re-backfill | CH rebuild allowed | rebuild from PostgreSQL | origins EMPTY/PREVIOUS_GA |

#### 15.6.1 MIG-PLAN-002 executable CHECK cutover and backfill identity

**CHECK cutover (applies to `entity.chk_entity_lifecycle` and `discovery_candidate.chk_candidate_review_status`).** Do **not** design a steady state of “add broader CHECK while retaining old CHECK”. Executable sequence:

1. **Add** new superset CHECK constraint `NOT VALID` (new constraint name; includes legacy + Platform_20 vocabulary).
2. **Validate** the new constraint (`VALIDATE CONSTRAINT`) until it holds on all existing rows.
3. **Deploy** mixed-version-compatible application that continues to **WRITE old states only** (reads may understand both vocabularies; writers must not emit new-only states yet).
4. **Quiesce or fence** affected writers for cutover (no concurrent inserts/updates that could race the DDL window).
5. In a **short controlled DDL window**, **drop/replace** the restrictive legacy CHECK so only the validated superset remains (or rename/swap atomically per PostgreSQL capability); do not leave dual restrictive+superset CHECKs.
6. **Activate** the new-state writer feature flag **only AFTER** that constraint change succeeds.
7. **Verify** new-state write/read (fixture insert/select of PROVISIONING/UNMANAGED/… as applicable; negative test that legacy-invalid values still fail).
8. **Failure/resume point and restore boundary:** resume is safe through step 2 (re-validate) and through step 3 (app still old-state writers). If step 5 fails mid-window, restore from the verified pre-step-5 backup (or re-add legacy CHECK before allowing writers) — do not enable the feature flag. After step 5 succeeds, rollback of vocabulary is **restore-from-verified-backup + re-backfill**, not DOWN migration of populated new-state rows. Record the last successful step id in upgrade job state.

**Exact backfill identity (`discovery_candidate` → `onboarding_candidate`).**

- **`dedupe_key` derivation (deterministic, UTF-8, hex SHA-256 truncated/stored as VARCHAR(256) = full hex of SHA-256 over the canonical string below):**
  - `NETWORK_DISCOVERY` / non-agent sources: `sha256_hex("v1|" || source_type || "|" || canonical_source_ref)` where `canonical_source_ref` is the durable discovery identity already stored on V003 `discovery_candidate` / evidence (`source_ref` normalized: trim, lower-case scheme/host where URI, stable sort of multi-id components). **IP/MAC alone must not form the key** when a stronger durable identifier exists on the candidate evidence.
  - `AGENT_ENROLLMENT`: `sha256_hex("v1|AGENT_ENROLLMENT|" || agent_id)` (certificate_identity collision routes to `SECURITY_HOLD` / review — never silent merge).
  - If both agent_id and network source_ref exist on one logical candidate, Agent form wins and network evidence attaches as observation.
- **Collision handling:** `UNIQUE (tenant_id, dedupe_key)`. On conflict during backfill/rediscovery: **update** the existing `onboarding_candidate` (revision-safe merge of observations / last_seen_at); **do not** insert a second row. If identity evidence contradicts (different durable agent_id / certificate for same key, or same agent_id mapping to two keys), set `identity_conflict_state=REVIEW_REQUIRED` (or `SECURITY_HOLD` for cert clone) and **do not** auto-APPROVE.
- **`APPROVED_FOR_PROVISIONING` → `PROVISIONING` vs `ACTIVE`:** map to `PROVISIONING` when the candidate has no activated monitored entity yet (no `entity_id`, or linked `provisioning_request.state` ∈ {READY,PROVISIONING} / entity lifecycle ≠ ACTIVE). Map to `ACTIVE` when linked `entity.lifecycle_state=ACTIVE` **or** `provisioning_request.state=ACTIVE`. Ambiguous rows without entity/request evidence default to `PROVISIONING` and are listed in the validation report for operator review (not silently ACTIVE).
- **Parity checks (must pass before feature flag):** count of **distinct derived `dedupe_key` values** from eligible `discovery_candidate` rows = count(`onboarding_candidate`) backfilled for that tenant (± explicit exclusion tombstones). Raw source-row count MUST NOT be required to equal destination-row count when de-duplication/merge on `(tenant_id, dedupe_key)` collapses multiple source rows. Separately verify **source-row → destination mapping coverage**: every eligible source row maps to exactly one destination candidate via its derived `dedupe_key` (many-to-one merge is expected and accounted). Zero duplicate destination `dedupe_key`. Identity-conflict rows set `identity_conflict_state` (`REVIEW_REQUIRED` / `SECURITY_HOLD`) and remain mapped/quarantined without silent merge or auto-APPROVE. Every `APPROVED_FOR_PROVISIONING` row maps to exactly one of {PROVISIONING, ACTIVE} per criteria above. Deterministic sample hash of `(dedupe_key, lifecycle_state)` matches over the distinct destination set. Old writers still cannot insert new-only states until step 6.

**One-way treatment:** a plan past its irreversible boundary requires, before application: verified backup, explicit operator approval (`Platform_16` §5/§6), an `UPGRADE_ORIGIN` from the matrix above, and a documented restore + post-restore validation path; the upgrade journey (`Platform_06` §119.21) reports `ROLLBACK_UNSUPPORTED` for it. Phase-1.5 objects (`ncm_change_plan*`, `ncm_approval`) are created inactive and carry no behavior until BUILD-090/091 Phase-1.5 certification. MIG-PLAN-013 history cutover is distinct from Kafka replay: replay cannot reconstruct CH history older than Kafka retention.

### 15.7 Shared UI query contracts (DAF) — Phase-1 workspaces

Preserves all **98** accepted UI manifests in `Platform_09` (UI-001..). This section defines **shared read/query contracts** for DAF families used by Phase-1 workspaces. **Do not** create per-screen endpoints; screens bind via `queryRef` / `dataSourceClassRef` (§8.1) to these operation IDs. Auth = listed permission + server-side resource scope; CC-API pagination/errors/correlation apply. All are `SPECIFIED_NOT_IMPLEMENTED` until owning BUILDs land. Units and quality fields are mandatory in responses so UNKNOWN/STALE/MONITORING_DEGRADED are never rendered as DOWN.

| Op ID | Method + logical path | DAF / Auth | Filters / scope / time | Projection (DTO) | Units / quality / freshness | Pagination / bounds / ordering | Errors |
|---|---|---|---|---|---|---|---|
| OP-DAF-HLT-SUMMARY | `GET /query/health:summary` | DAF-HLT; health.read | scope=`site_id`\|`group_id`\|`service_id`\|`region_id`\|tenant; `as_of?`; window optional for trend | `HealthSummaryDto{scope_id, semantic_state, counts_by_state, top_risks[], data_quality}` | semantic_state enum per alert_current; freshness=`as_of`/`evaluated_at`; quality banner fields required | max children/top_risks BUILD-130; order by severity then scope_name | QUERY_LIMITED, SCOPE_NOT_AUTHORIZED, STALE_AS_OF |
| OP-DAF-HLT-GRID | `GET /query/health:grid` | DAF-HLT; health.read | same scope; `domain?` (network/infra/…); cursor | `HealthGridPage{items[{id, name, semantic_state, open_alerts, open_incidents, collector_health}]}` | per-cell freshness + collector_health separate from target state | cursor; default 50/max 200; order name\|state\|alerts | QUERY_LIMITED |
| OP-DAF-TOP-GRAPH | `GET /query/topology:graph` | DAF-TOP; topology.read | `site_id`\|`entity_id` seed; `depth` bound; `as_of` TIMESTAMPTZ; relationship filter | `TopologyGraphDto{nodes[], edges[{edge_id, src, dst, relationship, provenance, confidence, state}]}` | confidence UInt; provenance enum; edge freshness `observed_at`/`as_of` | node/edge caps BUILD-130; no O(N²) expansion; order stable by edge_id | QUERY_LIMITED, AS_OF_TOO_OLD |
| OP-DAF-TOP-NEIGHBORS | `GET /query/topology/entities/{entityId}/neighbors` | DAF-TOP; topology.read | `as_of?`; relationship types | `NeighborListDto` | same quality fields | cursor; max neighbors | RESOURCE_NOT_FOUND |
| OP-DAF-FLW-TOPN | `GET /query/flow:topn` | DAF-FLW; flow.read | exporter_id\|interface_id\|site_id; window `[from,to]`; dimension=talker\|app\|conversation\|site_pair; direction? | `FlowTopNDto{rows[{key, observed_bytes, observed_packets, estimated_bytes\|null, estimated_packets\|null, estimate_coverage:FULL\|PARTIAL\|NONE, sampling_breakdown[{sampling_cohort_id, sampling_state, algorithm/model, selector, rate/probability?, validity_interval, observed_bytes, observed_packets, estimated_bytes\|null, estimated_packets\|null, estimate_method\|null, quality}], flows, quality}], window, truncated}` | **observed and estimated returned separately** — never one unlabeled total; top-level estimated_* populated ONLY when `estimate_coverage=FULL`; for `PARTIAL` top-level estimated_*=null and estimable portions appear only in `sampling_breakdown`; never treat `SAMPLED_UNKNOWN` as `UNSAMPLED`; never silently average rates/models; quality + exporter_health; **cardinality-bounded top-N** | `n` ≤ policy max; scan/timeout class; order by observed_bytes DESC (estimates never silently substitute) | QUERY_LIMITED, EXPORTER_UNAVAILABLE, CARDINALITY_TRUNCATED |
| OP-DAF-FLW-TIMESERIES | `GET /query/flow:timeseries` | DAF-FLW; flow.read | same scope; window; step (1m/5m/1h from rollup) | `FlowTimeseriesDto{series[{t, observed_*, estimated_*\|null, estimate_coverage, sampling_breakdown[]}], step, unit, quality}` | unit series declare observed vs estimated explicitly; same FULL/PARTIAL/NONE coverage rules as TOPN; freshness of rollup bucket; from FINAL/deduped raw→rollup only; rollups retain `sampling_cohort_id` (or equivalent) so incompatible cohorts are not irreversibly merged | max series/points BUILD-130; order time ASC | QUERY_LIMITED, UNSUPPORTED_STEP |

**Binding rule:** Phase-1 manifests that list DAF-HLT / DAF-TOP / DAF-FLW (e.g. UI-001..006, UI-009..012, UI-017/018) MUST resolve to the shared ops above (or existing owned list/get ops already in §15.4 such as OP-ALERT-LIST, OP-INC-LIST, OP-LIVE-EVIDENCE-GET). Additional Phase-1 DAF families already bound by §15.4 resource APIs (DAF-ALR, DAF-INC, DAF-INV, DAF-DIAG, DAF-NCM, DAF-QLT, DAF-SEN, …) do not receive per-screen endpoints here. New DAF query ops require contract change control; they must remain family-scoped, not UI-id-scoped.

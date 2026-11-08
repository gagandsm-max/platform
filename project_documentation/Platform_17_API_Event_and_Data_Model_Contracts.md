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

## 6. Counter64 / exact-counter implementation truth — v3.2.2 current main after PR #84

Architecture requires exact Counter64 end-to-end. Current merged implementation truth:

- Sensor preserves Counter64 as an exact unsigned decimal source value;
- Core canonicalization uses a dedicated exact counter representation separate from the ordinary floating-point numeric observation path;
- accepted wire form is a validated decimal string and non-canonical/ambiguous representations fail closed;
- Kafka serializes raw Counter64 without JSON/JavaScript Number semantics;
- ClickHouse persists raw exact values in a dedicated `UInt64` path and exposes replay-safe canonical reads;
- tests cover values around `2^53`, the Java signed-long boundary and through `18446744073709551615` (`2^64-1`), plus restart/replay/idempotent migration behavior;
- therefore the **raw exact Counter64 transport/persistence path is IMPLEMENTED and TESTED**, but remains **NOT BENCHMARKED / NOT CERTIFIED / NOT DEPLOYED**.

The ordinary numeric measurement path may remain floating-point because it is semantically separate from the authoritative raw counter family. Derived delta/rate/utilization may use floating-point only AFTER exact delta/reset/discontinuity processing. That derived Counter32/Counter64 reset/wrap/discontinuity/rate layer remains **SPECIFIED but NOT IMPLEMENTED/CERTIFIED**. A decrease is not automatically a wrap; discontinuity/reboot/source evidence is evaluated first.

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
- **Host Identifier & Entity Resolution:** `host_id` is a bounded Agent-observed host identifier string (e.g. `"host-1"`). It represents corroborating identity evidence only, not canonical entity authority (`Agent hostId != canonical entityId`). Core resolves `tenant + namespace="platform.agent" + identifier_type="HOST_ID" + host_id` into canonical `entity_id`. If resolution fails, the platform follows the existing governed unresolved-entity behavior rather than fabricating an entity. Agent-supplied `entityId` is never accepted as authoritative.
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

The template used for decoding must be valid for the exporter transport/session/observation-domain and event window. Data Templates and Options Templates share one Template ID namespace per Observation Domain/exporter scope. Under UDP, enforce refresh/redefinition semantics and **ignore UDP Template Withdrawal messages**. After collector restart, do not decode UDP Data Sets using an unvalidated persisted template; require appropriate live template validation/session continuity. Unknown, expired, conflicting or missing templates produce explicit loss/quality evidence rather than fabricated decoded flow rows. Sampling metadata preserves selector/algorithm, rate/interval/probability, scope, observation-domain identity and valid time window where available. Distinguish **sFlow sampled-frame evidence**, **sFlow interface/device counter samples**, and IPFIX/NetFlow flow counters: an sFlow Counter sample is not an ordinary per-flow `network_flow` fact and must use existing typed metric/device-counter evidence paths (no new database). sFlow sampled header **capture length** is not automatically original packet/frame octet length; preserve original frame length only where the protocol supplies it and it is qualified. Never interpret cumulative sFlow interface counters as per-flow `bytes`/`packets`, nor scale those counters by packet sample rate. Raw exported `bytes`/`packets` (or sFlow qualified sampled-frame length evidence) are immutable source facts and must not be silently overloaded with estimates; any sampling-normalized/estimated bytes, packets, rates or utilization use separately identified fields (`bytes_estimated`/`packets_estimated` or equivalent), estimation method, source sampling provenance and a separate estimation indicator alongside the applicable existing quality state (such as `UNKNOWN` or `PARTIAL`). No blanket multiplication or duplicate normalization is permitted. Authenticated Sensor→Core mTLS does not make unauthenticated exporter UDP data intrinsically trusted.

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

**CC-CH (ClickHouse).** Database `platform` (existing). `tenant_id String` first in `ORDER BY` (existing convention); `event_time DateTime64(3,'UTC')` (when it happened at the source) and `ingest_time DateTime64(3,'UTC')` (Core acceptance) are separate columns; durable logical identity (`sequence_id`/`event_id`) is part of the replacement key so Kafka/Core crash-window replays collapse; readers use a canonical `FINAL` view (existing `metric_numeric` pattern); `LowCardinality(String)` for bounded enums; quality column `UInt8`; TTL follows the data's retention class (Platform_11 classes; numeric days = `NUMERIC_BUDGET_OWNER=Platform_11`); partition granularity chosen by volume (daily for flow/log/trap, monthly otherwise); rollups via materialized views into `AggregatingMergeTree`/`SummingMergeTree` tables; per-tenant cardinality budgets enforced at ingestion (cardinality-bound column in each table below). ClickHouse is never control authority.

**CC-API.** Base `/api/v1/tenants/{tenantId}/…` (existing controller convention). Every POST requires `Idempotency-Key`; mutable-resource writes require `If-Match` (G-03/G-04 in `Platform_06` §119.0); every response carries `X-Correlation-Id`; errors use §10.9; list endpoints use cursor/keyset pagination (existing paged APIs: default 50, max 200 — other maxima belong to `Platform_11`/BUILD-130); retry is safe only with the same idempotency key. Logical routes below are the canonical names the generated OpenAPI must emit (§3.2.1 requires OpenAPI to carry exact strings); changing one needs contract change control.

**CC-EVT.** Topics follow `<domain>.<family>.v<major>` (existing `telemetry.numeric.v1`). Common envelope (§3.4): `schema_version, tenant_id, source identity, event_id (UUID) + sequence where ordered, event_time, ingest_time, quality/provenance, correlation_id, payload`. Physical topic/partition/serialization selection is BUILD-063; names below are logical. Breaking change = new major topic + migration window (§3.5). Secrets, one-time credentials and keystrokes never appear.

### 15.2 PostgreSQL planned schemas (`SPECIFIED_NOT_IMPLEMENTED`)

Legend: NN = NOT NULL, N = nullable. "Idx" = indexes. "Ver/Idem/Conc" = versioning, idempotency, concurrency. "Ret/Aud" = retention class and audit. "Mig" = owning migration plan.

#### Platform substrate

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `api_idempotency_record` | see CC-IDEM | PK per CC-IDEM | `(expires_at)` | n/a | short TTL sweep / none | MIG-PLAN-001 |
| `event_outbox` | see CC-OUTBOX | PK event_id | `(state,not_before)`,`(tenant_id,topic)` | publisher lease `FOR UPDATE SKIP LOCKED` | purge after PUBLISHED per ops class / none | MIG-PLAN-001 |
| `audit_event` +CC (append-only) | audit_id UUID NN; tenant_seq BIGINT NN; occurred_at TIMESTAMPTZ NN; actor_type VARCHAR(16) NN (USER/SERVICE/SYSTEM); actor_id VARCHAR(128) NN; action VARCHAR(96) NN; resource_type VARCHAR(48) NN; resource_id VARCHAR(128) N; resource_version BIGINT N; result VARCHAR(16) NN (SUCCESS/DENIED/FAILED); reason VARCHAR(1000) N; before_hash/after_hash BYTEA N; detail JSONB N (bounded, no secrets); integrity_ref VARCHAR(256) N (reserved for the tamper-evidence mechanism; no cryptographic construction assumed before architecture/security review) | PK audit_id; UNIQUE (tenant_id, tenant_seq); CHECK result IN … | `(tenant_id, occurred_at DESC)`,`(tenant_id, resource_type, resource_id)`,`(tenant_id, actor_id, occurred_at)` | immutable: `REVOKE UPDATE, DELETE`; append only / n/a | declarative range partition by month on `occurred_at`; retention class AUDIT (legal-hold aware) / self | MIG-PLAN-001 (BUILD-073 completes durability/export) |
| `platform_operation` +CC | operation_id UUID NN; kind VARCHAR(24) NN (BACKUP/RESTORE/UPGRADE/ROLLBACK/RETENTION_APPLY/PURGE/HOUSEKEEPING); scope VARCHAR(128) NN; state VARCHAR(24) NN (REQUESTED/PREFLIGHT/RUNNING/VERIFYING/COMPLETED/FAILED/CANCELLED/ROLLING_BACK/ROLLED_BACK/ROLLBACK_FAILED/ROLLBACK_UNSUPPORTED); requested_by NN; approver_id N; artifact_ref VARCHAR(512) N; artifact_hash BYTEA N; plan JSONB N; result JSONB N; started_at/ended_at N | PK; CHECK state/kind; partial UNIQUE (tenant_id, scope) WHERE state IN ('REQUESTED','PREFLIGHT','RUNNING','VERIFYING','ROLLING_BACK') (one active op per scope) | `(tenant_id,kind,state)` | version+If-Match; key via CC-IDEM | ops-history class / audit each transition | MIG-PLAN-012 |
| `platform_operation_step` | step_id UUID NN; operation_id NN; seq INT NN; name VARCHAR(64) NN; state VARCHAR(16) NN (PENDING/OK/FAILED/SKIPPED); detail JSONB N; started_at/ended_at N | PK; FK (tenant_id, operation_id); UNIQUE (operation_id, seq) | `(operation_id,seq)` | idempotent step resume by seq | with parent | MIG-PLAN-012 |

#### Inventory/onboarding extensions (reconciles V001/V003 vocabulary)

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `provisioning_request` +CC | request_id UUID NN; source VARCHAR(16) NN (MANUAL/BULK/ONBOARDING); state VARCHAR(16) NN (DRAFT/VALIDATING/PREFLIGHT/READY/PROVISIONING/ACTIVE/FAILED/CANCELLED/CANCEL_REQUESTED); entity_id UUID N; candidate_id UUID N; site_id UUID N; input JSONB NN (bounded; secret-free; credential_profile_ref only); assignment_mode VARCHAR(24) NN CHECK IN (AUTO_FROM_CRITICALITY,EXPLICIT_PROFILE); attempt_no INT NN; failure_class VARCHAR(32) N; batch_id UUID N | PK; FK entity/site/candidate same-tenant | `(tenant_id,state)`,`(tenant_id,batch_id)` | version/If-Match; CC-IDEM | provisioning-history class / `entity.provision.*` | MIG-PLAN-002 |
| `onboarding_candidate` +CC | per Platform_20 §6.1 (candidate_id, source_type, source_ref, entity_id N, agent_id N, lifecycle_state, identity_conflict_state, discovered_name, proposed_* columns, first/last_seen_at, revision BIGINT NN, dedupe_key VARCHAR(256) NN) | PK; UNIQUE (tenant_id, dedupe_key); CHECK lifecycle_state IN (DISCOVERED,ENROLLED,PENDING_APPROVAL,PROVISIONING,ACTIVE,UNMANAGED,REJECTED,EXCLUDED,DECOMMISSION_REQUESTED,DECOMMISSIONED,RETIRED); CHECK identity_conflict_state IN (NONE,REVIEW_REQUIRED,SECURITY_HOLD) | `(tenant_id,lifecycle_state)`,`(tenant_id,source_type,last_seen_at)` | `revision` is the concurrency token (If-Match); decisions via CC-IDEM | candidate-history class / `onboarding.*` | MIG-PLAN-002 |
| `onboarding_decision` (append-only) | decision_id UUID NN; candidate_id NN; decision VARCHAR(12) NN (APPROVE/UNMANAGED/REJECT/EXCLUDE/REOPEN); actor_id NN; reason VARCHAR(1000) NN; before_revision/after_revision BIGINT NN; effective JSONB N (group/profile/policy/assignment); result VARCHAR(16) NN (APPLIED/FAILED/CONFLICT); batch_id N; decided_at NN | PK; FK candidate same-tenant | `(candidate_id,decided_at)`,`(tenant_id,batch_id)` | idempotency via CC-IDEM + UNIQUE (candidate_id, before_revision, decision) | permanent with candidate / self | MIG-PLAN-002 |
| `entity_agent_binding` +CC | entity_id NN; agent_id VARCHAR(128) NN; certificate_identity VARCHAR(256) NN; system_name VARCHAR(255) NN; os_identity JSONB N; binding_state VARCHAR(20) NN CHECK IN (PENDING_APPROVAL,ACTIVE,UNMANAGED,REVOKED,SECURITY_HOLD); reinstall_generation INT NN; first_enrolled_at, last_heartbeat_at | PK (tenant_id, entity_id); UNIQUE (agent_id); UNIQUE (certificate_identity) (deployment-wide per Platform_20 §6.3) | `(tenant_id,binding_state)` | version; reinstall only through explicit reconcile | binding class / `agent.*` | MIG-PLAN-002 |
| `entity` (existing V001) | **change:** extend `chk_entity_lifecycle` additively to include PROVISIONING, UNMANAGED, DECOMMISSION_REQUESTED, DECOMMISSIONED (keeps DISCOVERED, ACTIVE, QUIESCED, RETIRED) | existing | existing | existing | existing | MIG-PLAN-002 (forward-only; additive) |
| `discovery_candidate` (existing V003) | **change:** extend `chk_candidate_review_status` additively to accept the Platform_20 vocabulary while retaining the existing values; mapping: PENDING_REVIEW≡PENDING_APPROVAL, APPROVED_FOR_PROVISIONING≡PROVISIONING/ACTIVE, LEFT_UNMANAGED≡UNMANAGED, IGNORED≡REJECTED, EXCLUDED_PERMANENTLY≡EXCLUDED | existing | existing | existing | existing | MIG-PLAN-002 |

#### Policies, profiles, maintenance

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `alert_policy` +CC | policy_id NN; name VARCHAR(160) NN; evidence_class VARCHAR(48) NN; scope JSONB NN (bounded); lifecycle_state VARCHAR(12) NN CHECK IN (DRAFT,BACKTEST,SHADOW,ACTIVE,RETIRED); current_version_no INT N | PK; UNIQUE (tenant_id,name) | `(tenant_id,lifecycle_state)` | version/If-Match | policy class / `policy.*` | MIG-PLAN-003 |
| `alert_policy_version` (immutable) | policy_id NN; version_no INT NN; definition JSONB NN (bounded); evidence_contract_ref VARCHAR(128) NN; content_hash BYTEA NN; state_at_creation VARCHAR(12) NN; backtest_ref/shadow_ref N; activated_at/by N | PK (tenant_id, policy_id, version_no); FK policy same-tenant | `(tenant_id,policy_id)` | immutable; content_hash dedupes identical versions | permanent / `policy.version.*` | MIG-PLAN-003 |
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
| `rca_revision` (immutable) | rca_id NN; revision_no INT NN; outcome VARCHAR(14) NN (CONCLUDED/INCONCLUSIVE/PARTIAL_EVIDENCE); likely_cause JSONB N; confidence VARCHAR(12) NN; hypotheses JSONB NN (bounded); supporting_refs/contradicting_refs/missing_refs JSONB NN; engine_version VARCHAR(64) NN; rule_set_version VARCHAR(64) NN; evidence_set_hash BYTEA NN; created_at | PK (tenant_id, rca_id, revision_no); UNIQUE (tenant_id, rca_id, evidence_set_hash, rule_set_version) | `(rca_id,revision_no DESC)` | append-only; the unique key dedupes replays | permanent / `rca.revision.created` | MIG-PLAN-004 |
| `rca_feedback` (append-only) | feedback_id; rca_id; revision_no; verdict VARCHAR(8) NN; reason_code VARCHAR(48) NN; note VARCHAR(1000) N; actor_id; created_at | PK; FK revision same-tenant | `(rca_id,revision_no)` | idempotent per (rca_id,revision_no,actor_id) | feeds evaluation only / `rca.feedback.*` | MIG-PLAN-004 |
| `evidence_ref` | evidence_id NN; kind VARCHAR(32) NN; source_system VARCHAR(48) NN; source_ref VARCHAR(512) NN; entity_id N; observed_from/observed_to TIMESTAMPTZ N; quality VARCHAR(12) NN; content_hash BYTEA N; retention_class VARCHAR(16) NN | PK; UNIQUE (tenant_id, source_system, source_ref, content_hash) | `(tenant_id,kind)`,`(tenant_id,entity_id,observed_from)` | immutable (references, never payloads) | per class (legal-hold aware) / none | MIG-PLAN-004 |

#### NCM

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `ncm_job` +CC | job_id NN; kind VARCHAR(12) NN CHECK IN (BACKUP,CHANGE,ROLLBACK,COMPLIANCE); state VARCHAR(20) NN (QUEUED/RUNNING/SUCCEEDED/PARTIAL/FAILED/CANCELLED/TIMED_OUT/COMPLETED_UNVERIFIED/ABORTED/ROLLED_BACK/ROLLBACK_FAILED/UNSUPPORTED); mode VARCHAR(16) NN (MANUAL_CLI/CHANGE_PLAN/SCHEDULED/ON_DEMAND); plan_version_id UUID N; cli_session_id N; reason VARCHAR(1000) N; ticket_ref N; requested_by NN; lease_generation BIGINT N | PK; FK plan/session same-tenant | `(tenant_id,kind,state)` | version; per-target lease; CC-IDEM | ncm-job class / `ncm.job.*` | MIG-PLAN-005 |
| `ncm_job_target` | job_id NN; entity_id NN; state VARCHAR(16) NN (PENDING/CAPTURING/STORED/UNCHANGED/FAILED/SKIPPED/EXECUTING/VALIDATING/…); error_code VARCHAR(48) N; result_config_version_id N; pre_config_version_id N; post_config_version_id N; sensor_id N | PK (job_id, entity_id); FK entity/job same-tenant | `(tenant_id,entity_id)` | one non-terminal target per entity (partial UNIQUE) | with job / via job audit | MIG-PLAN-005 |
| `ncm_config_version` (immutable) | config_version_id NN; entity_id NN; scope VARCHAR(10) NN (RUNNING/STARTUP); content_hash BYTEA NN; content_ref VARCHAR(512) NN (governed encrypted config store reference; no new database); size_bytes BIGINT NN; redaction_state VARCHAR(12) NN (REDACTED/UNREDACTED_RESTRICTED/FAILED); diff_base_id UUID N; golden BOOL NN; captured_at NN; capture_job_id N; sensor_id N | PK; UNIQUE (tenant_id, entity_id, scope, content_hash) | `(tenant_id,entity_id,captured_at DESC)` | immutable; hash dedupe → UNCHANGED | ncm-config class (legal-hold aware) / `ncm.config.version.stored` | MIG-PLAN-005 |
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
| `cli_session` +CC | session_id; entity_id NN; requested_privilege VARCHAR(16) NN; credential_mode VARCHAR(14) NN (STORED_PROFILE/PERSONAL/ONE_TIME); state VARCHAR(12) NN (REQUESTED/AUTHORIZED/CONNECTING/ACTIVE/CLOSED/TERMINATED/EXPIRED/DENIED/FAILED); reason NN; ticket_ref N; mfa_outcome VARCHAR(10) N; recording_ref VARCHAR(512) N; expires_at NN; closed_reason N — **no credential, token or keystroke columns** | PK; FK entity same-tenant | `(tenant_id,state)`,`(tenant_id,entity_id)` | version; CC-IDEM | cli-audit class / `cli.session.*` | MIG-PLAN-009 |

#### Integration outbox/inbox

| Table | Columns | PK / FK / Unique / Check | Idx | Ver/Idem/Conc | Ret / Aud | Mig |
|---|---|---|---|---|---|---|
| `integration_connector` +CC | connector_id; type VARCHAR(32) NN; endpoint_ref VARCHAR(512) NN; auth_ref (Credential Profile id) NN; mapping_version INT NN; state VARCHAR(14) NN (DRAFT/SANDBOX_PASSED/ENABLED/DEGRADED/DISABLED); health JSONB N | PK; UNIQUE (tenant_id,type,endpoint_ref) | `(tenant_id,state)` | version/If-Match | permanent / `integration.connector.*` | MIG-PLAN-010 |
| `integration_outbox` | operation_id UUID NN; connector_id NN; object_type/object_id NN; operation_hash BYTEA NN; mapping_version INT NN; payload JSONB bounded; state VARCHAR(8) NN (PENDING/SENDING/DELIVERED/RETRY/DEAD); attempts INT; not_before NN; lease_until N; last_error_code N; created_in_txn_event_id UUID NN | PK; **UNIQUE (tenant_id, connector_id, operation_hash)** | `(state,not_before)` | lease `SKIP LOCKED`; external call carries operation_hash as external idempotency reference | ops class; DEAD kept until replay/expiry / `integration.outbound.*` | MIG-PLAN-010 |
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

All engines follow CC-CH (`ReplacingMergeTree` with a `FINAL` view unless an append-only fact table is explicitly deduplicated by its key). Types: `tenant_id String`, bounded enums `LowCardinality(String)`.

| Table | Columns (key) | Engine / Partition | ORDER BY | event vs ingest time | Replay/dedupe | TTL / Rollup | Cardinality bound |
|---|---|---|---|---|---|---|---|
| `flow_record_raw` | tenant_id, exporter_id, observation_domain, ingress_context_id, event_time (flow start/export), ingest_time, src_ip, dst_ip (IPv6-capable), src_port, dst_port, proto, in_if, out_if, bytes UInt64, packets UInt64, tcp_flags, sampling_rate UInt32, template_id, sequence_id, vrf_id, src_entity_id/dst_entity_id (nullable resolved), quality UInt8 | ReplacingMergeTree / daily `toYYYYMMDD(event_time)` | `(tenant_id, exporter_id, observation_domain, event_time, sequence_id, src_ip, dst_ip, src_port, dst_port, proto)` | both stored; export/capture time = event_time | key includes exporter/domain/sequence; unknown-template/ambiguous-tenant records are **quarantined, never inserted** (§13.9) | TTL raw class; MV → `flow_rollup_1m/1h` (`SummingMergeTree` keyed by tenant, exporter, interface, app, direction) | heavy-hitter + top-N caps per tenant/exporter; rollup dimension budget per tenant |
| `log_event_raw` (syslog/OTLP logs/Agent logs) | tenant_id, source_id, source_type, host/entity_id N, event_time, ingest_time, severity UInt8, facility, program, message String (bounded), attributes Map(String,String) (bounded keys), trace_id/span_id N, event_id/sequence_id, quality UInt8, parse_state | ReplacingMergeTree / daily | `(tenant_id, source_id, event_time, sequence_id)` | source timestamp-quality flag when untrusted | event_id replay-collapse; dedupe-after-ACK counter | TTL log class; MV → `log_volume_1m` (counts by severity/program) | message/attribute size caps; attribute-key allowlist; per-source rate budget with drop counters |
| `trap_event_raw` | tenant_id, source_ip, entity_id N, engine_id, trap_oid, varbinds String (bounded JSON), event_time, ingest_time, inform_acked UInt8, storm_aggregate_count UInt32, quality, sequence_id | ReplacingMergeTree / daily | `(tenant_id, source_ip, trap_oid, event_time, sequence_id)` | device time vs receive time | sequence_id; storm aggregation recorded | TTL event class | per-source storm aggregation; varbind size cap |
| `audit_history` | tenant_id, audit_id, tenant_seq, occurred_at, actor_type/id, action, resource_type/id, result, correlation_id, integrity_ref N | MergeTree / monthly | `(tenant_id, occurred_at, tenant_seq)` | occurred_at / ingest_time | key (tenant_id, tenant_seq); **a derived analytical copy — PostgreSQL `audit_event` remains authoritative** | TTL AUDIT class (legal-hold aware: partitions under hold are not dropped) | bounded fields; no detail blobs |
| `telemetry_quality_event` | tenant_id, source_id, source_protocol, signal_class, event_time, ingest_time, state (FRESH/STALE/PARTIAL/GAP/DROP/TIME_SKEW), counters (produced, accepted, normalized, inserted, rejected UInt64), quality_code, sequence_id | ReplacingMergeTree / monthly | `(tenant_id, source_id, signal_class, event_time, sequence_id)` | both | sequence_id | TTL quality class; MV → `quality_rollup_5m` | one row per source/signal/interval (bounded by source count) |
| `topology_observation_history` | tenant_id, edge_id, src_entity_id, dst_entity_id, relationship, provenance (LLDP/CDP/FLOW/ROUTING/DECLARED/PATH), confidence UInt8, event_time (observed), ingest_time, first_seen/last_seen, state (OBSERVED/STALE/WITHDRAWN), sequence_id | ReplacingMergeTree / monthly | `(tenant_id, edge_id, event_time, sequence_id)` | observation vs ingest | collapse by key; spoofed/cross-tenant advertisements rejected pre-insert | TTL topology class (as-of replay needs retention ≥ RCA window) | edge-count cap per tenant; per-edge history sampling policy |
| `process_network_evidence` (REQ-INF-013 / §11.2) | tenant_id, agent_id, entity_id N, bucket_start UInt32/DateTime64, bucket_end, bucket_duration_sec, process_name, executable_path_hash, service_name, container_id N, cgroup_hash N, local_ip N, local_port N, remote_ip, remote_port, protocol, ip_version, bytes_tx UInt64, bytes_rx UInt64, active/new/closed connections UInt32 N, mean_rtt_us N, retransmit_count N, connection_failures N, quality_state, dropped/shed counts, remote_entity_id N, ingest_time, sequence_id | ReplacingMergeTree / daily | `(tenant_id, entity_id, bucket_start, process_name, remote_ip, remote_port, protocol, sequence_id)` | bucket_start/end = event; ingest separate | sequence_id + agent_id; the Agent bucket is the unit | TTL evidence class; MV rollup → `process_edge_1h` | local aggregation at the Agent bounds cardinality (process/service + canonical target + port + bucket); per-tenant row/byte budget; **no payload/TLS/HTTP/SQL/env/cmdline columns exist** |
| `active_path_observation` + `active_path_hop` | observation: tenant_id, path_id, source_sensor_id, source_vantage_id, target_entity_id, protocol, probed_at (event), ingest_time, total_hops, responsive_hops, rtt/loss/jitter Float32 N, path_fingerprint, fingerprint_version, status, path_identity_quality, comparison_eligible UInt8, sequence_id. hop: observation key + hop_index, hop_ip N, hop_state, asn N, rtt/loss/jitter N, mpls_labels Array(UInt32) | ReplacingMergeTree / daily | obs: `(tenant_id, path_id, source_vantage_id, probed_at, sequence_id)`; hop: `(tenant_id, path_id, source_vantage_id, probed_at, sequence_id, hop_index)` | probe time vs ingest | sequence_id; absent quality fields stored as `UNQUALIFIED/0` | TTL path class | bounded target count and probe rate at Sensor (non-O(N²) fan-out); hop ≤ policy max |
| `alert_history` / `incident_history` | tenant_id, alert_id/incident_id, seq, event_time, ingest_time, from_state, to_state, semantic_state, severity, confidence, entity_id, policy_id+version, actor_type, reason_code, event_id | ReplacingMergeTree / monthly | `(tenant_id, alert_id, seq)` / `(tenant_id, incident_id, seq)` | both | key (tenant, id, seq) from the PostgreSQL lifecycle seq | TTL alert/incident class; MV → daily counts by policy/severity/state (feeds MTTR/false-alert metrics with data-quality columns) | bounded; derived from events, never authority |

### 15.4 API operations for the Phase-1 journeys (`SPEC_CONTRACT=FROZEN`)

Auth column = permission; every operation also enforces resource scope, entitlement, CC-API (correlation ID, errors, retry). **Async** = 202 + resource state machine. DTOs: `Req`/`Res` named after the contract objects in `Platform_06` §119.

| Op ID | Method + logical path | Auth | Request → Response | Pagination / filters / limits | Idempotency | Concurrency | Async | Errors (stable codes, besides `PERMISSION_DENIED`, `VALIDATION_FAILED`, `STALE_VERSION`) | Journey |
|---|---|---|---|---|---|---|---|---|---|
| OP-PROV-CREATE | `POST /provisioning-requests` | inventory.provision | ProvisioningRequestCreate → ProvisioningRequest | – | key required | – | 202 | IDENTITY_CONFLICT, TARGET_EXCLUDED, CAPACITY_BLOCKED, LICENCE_BLOCKED | 119.1 |
| OP-PROV-GET/LIST | `GET /provisioning-requests[/{id}]` | inventory.read | → ProvisioningRequest / page | state, batch, site; cursor | – | ETag | – | RESOURCE_NOT_FOUND | 119.1 |
| OP-PROV-CANCEL | `POST /provisioning-requests/{id}:cancel` | inventory.provision | {reason} → state | – | key | If-Match | 202 | INVALID_TRANSITION | 119.1 |
| OP-PROV-IMPORT | `POST /provisioning-requests:import` | inventory.provision | multipart CSV/XLSX → batch_id | file size/row bound (owner: BUILD-130) | key | – | 202 | IMPORT_ROW_ERRORS (per-row in result) | 119.1 |
| OP-DISC-REQ-* | existing `/discovery` create/read/list/cancel | discovery.run | existing DTOs | existing `PageResult` | existing | existing | existing | existing | 119.2 |
| OP-ONB-CAND-LIST/GET | `GET /onboarding/candidates[/{id}]` | onboarding.review | → OnboardingCandidate(+revision) | state, source_type, site, conflict; cursor (default 50/max 200) | – | ETag=revision | – | – | 119.2/3 |
| OP-ONB-CAND-DECIDE | `POST /onboarding/candidates/{id}:decide` | onboarding.review | OnboardingDecision → OnboardingDecisionResult | – | key | `expected_revision` (If-Match) | sync (APPROVE → provisioning async) | IDENTITY_CONFLICT_REVIEW_REQUIRED, CAPACITY_BLOCKED, LICENCE_BLOCKED, INVALID_TRANSITION | 119.2 |
| OP-ONB-CAND-BULK-DECIDE | `POST /onboarding/candidates:bulk-decide` | onboarding.review | {items[{id,expected_revision}], decision, reason} → per-item results | item bound (BUILD-130) | key | per-item revision | 202 | PARTIAL per item | 119.2 |
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
| OP-INC-ACK/ASSIGN/PRIORITY/NOTE/LINK/UNLINK/RESOLVE/CLOSE/REOPEN/MERGE/SPLIT | `POST /incidents/{id}:<action>` | incidents.manage (reopen: incidents.reopen) | IncidentAction → Incident | – | key | If-Match | sync | RECOVERY_NOT_QUALIFIED, ALERT_ALREADY_LINKED_ELSEWHERE, INVALID_TRANSITION | 119.6 |
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
| OP-RPT-RUN / OP-RPT-RUN-CANCEL | `POST /report-runs`, `POST /report-runs/{id}:cancel` | reports.run | ReportRunRequest → ReportRun | query bounds (timeout/row/scan) | key | version | 202 | QUERY_LIMITED, INSUFFICIENT_DATA, RENDER_FAILED | 119.12 |
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
| OP-POLICY-LIFECYCLE | `POST /alert-policies/{id}:advance` (DRAFT→BACKTEST→SHADOW→ACTIVE) | policy.manage | {to_state, evidence_ref} → policy | – | key | If-Match | backtest 202 | EVIDENCE_REQUIRED | UI-059/091 |
| OP-AUDIT-EXPORT | `POST /audit/exports` | audit.export | {filters, window, format CSV} → export job | bounded window/rows | key | – | 202 | QUERY_LIMITED | UI-083 |
| OP-SENSOR-REBALANCE-PREVIEW/DRAIN, OP-ACCESS-REQUEST-DECIDE, OP-SLA-DISPUTE, OP-HANDOVER-CREATE, OP-PIR-CREATE, OP-RUNBOOK-UPSERT, OP-MIGRATION-IMPORT, OP-NCM-COMPLIANCE-ACK | existing/owned by `Platform_20` (§3, DF-009/010/039), `Platform_21`/`Platform_17` §8.2, DF-061, DF-059/065, DF-068 respectively; same CC-API rules | per owner | per owner contract | – | key | If-Match | per owner | per owner | UI bindings |

### 15.5 Events (logical topics; physical selection BUILD-063)

Common: CC-EVT envelope; producer writes via CC-OUTBOX in the source transaction; consumers are idempotent on `event_id`; DLQ = parked topic `<topic>.dlq` (poison parking with reason/offset; never silently dropped); compatibility = additive optional fields only within a major version.

| Topic / name | Schema ver | Producer | Consumers | Identity / partition key | Times | Dedupe | Quality | Failure / DLQ |
|---|---|---|---|---|---|---|---|---|
| `entity.lifecycle.v1` | 1.0 | Inventory/Provisioning | Policy, Sensor control, Licensing, Timeline, Notifications | entity_id / `tenant_id|entity_id` | event_time, ingest_time | event_id | n/a | DLQ + replay from outbox |
| `onboarding.candidate.v1` | 1.0 | Onboarding | Notifications, Timeline | candidate_id / `tenant_id|dedupe_key` | event_time | event_id + candidate revision | n/a | DLQ |
| `agent.assignment.v1` | 1.0 | Agent control | Sensor control, UI channel | agent_id / `tenant_id|agent_id` | event_time | (agent_id, generation) | n/a | stale-generation consumer rejects |
| `policy.change.v1` / `maintenance.window.v1` | 1.0 | Policy | Alert engine, Notifications, Service Assurance | policy/window id / `tenant_id|scope_hash` | event_time | event_id + version | n/a | DLQ |
| `alert.lifecycle.v1` | 1.0 | Alert engine | Incident engine, Notifications, ITSM outbox, ClickHouse `alert_history`, Timeline | alert_id / `tenant_id|alert_id` (ordered per alert) | event_time (state change), ingest_time | (alert_id, seq) | semantic_state + confidence + evidence quality | DLQ; consumer fail-closed ACK |
| `incident.lifecycle.v1` | 1.0 | Incident service | RCA, Notifications, ITSM, `incident_history`, Timeline | incident_id / `tenant_id|incident_id` | event_time | (incident_id, seq) | n/a | DLQ |
| `rca.revision.v1` | 1.0 | RCA engine | UI channel, Timeline, evaluation | rca_id / `tenant_id|rca_id` | event_time | (rca_id, revision_no) | confidence, outcome | DLQ |
| `ncm.job.v1` | 1.0 | NCM | Timeline, Incident, Notifications | job_id / `tenant_id|entity_id` | event_time | (job_id, target, state) | n/a | DLQ |
| `ipam.change.v1` | 1.0 | IPAM | Timeline, Discovery, CMDB sync | address_id / `tenant_id|vrf_id` | event_time | event_id | n/a | DLQ |
| `report.run.v1` | 1.0 | Reporting | Notifications, Delivery | run_id / `tenant_id|run_id` | event_time | (run_id, state) | quality summary | DLQ |
| `notification.request.v1` | 1.0 | Notification module | Integration Service delivery | notification_id / `tenant_id|recipient_hash` | event_time | dedupe_key | n/a | DLQ + delivery retry state in PostgreSQL |
| `diagnostic.job.v1` | 1.0 | Diagnostics | Timeline, Investigation, RCA, analytics | job_id / `tenant_id|target_entity_id` | event_time | (job_id, vantage_id, state) | per-vantage quality | DLQ; **no secrets** |
| `investigation.session.v1` | 1.0 | Investigation | Timeline, Incident | investigation_id / `tenant_id|investigation_id` | event_time | (investigation_id, seq) | n/a | DLQ |
| `cli.session.v1` | 1.0 | CLI Broker | Audit, Timeline | session_id / `tenant_id|session_id` | event_time | (session_id, state) | n/a | DLQ; **lifecycle metadata only — no keystrokes, no secrets** |
| `integration.outbound.v1` / `integration.inbound.v1` | 1.0 | Integration Service | Incident/Alert, Timeline, UI channel | operation_id / external_event_id; `tenant_id|connector_id` | event_time, received_time | operation_hash / external_event_id | verification result | DLQ + replayable DEAD |
| `audit.event.v1` | 1.0 | all (via outbox) | Audit durable store, SIEM export, ClickHouse `audit_history` | audit_id / `tenant_id` (ordered by tenant_seq) | occurred_at | (tenant_id, tenant_seq) | n/a | DLQ; **authoritative copy is PostgreSQL** |
| `licence.state.v1` | 1.0 | Licensing | Scheduler, UI channel, Notifications | licence_id / `tenant_id` | derived_from_expiry | (licence_id, state, expiry) | n/a | DLQ |
| `platform.operation.v1` | 1.0 | Platform Operations | UI channel, Notifications, Audit | operation_id / `tenant_id|scope` | event_time | (operation_id, step, state) | n/a | DLQ |
| `telemetry.flow.v1` / `telemetry.log.v1` / `telemetry.event.v1` (traps) | 1.0 | Ingestion gateway (after authenticated admission) | ClickHouse writers, Alert engine, Flow/Log analytics | source/exporter id / `tenant_id|exporter_id|observation_domain` etc. | event_time, ingest_time | sequence_id | quality code, quarantine class | committed-only consumer; poison parking; **ambiguous tenant → quarantine, never produced** |
| `telemetry.quality.v1` | 1.0 | Sensor/Core | `telemetry_quality_event`, Alert qualification | source_id / `tenant_id|source_id` | event_time | sequence_id | state | DLQ |
| `telemetry.processnet.v1` / `telemetry.path.v1` | 1.0 | Ingestion gateway (typed Agent/Sensor evidence) | `process_network_evidence`, `active_path_*`, RCA | agent/path id / `tenant_id|entity_id` or `tenant_id|path_id` | bucket/probe time, ingest time | sequence_id | quality_state / path_identity_quality | DLQ; unsupported typed record is rejected explicitly, never silently ACKed |
| `topology.observation.v1` | 1.0 | Topology | `topology_observation_history`, graph | edge_id / `tenant_id|edge_id` | observed time | (edge_id, sequence) | provenance+confidence | spoof/cross-tenant rejected pre-publish |

### 15.6 Migration plan (`MIG-PLAN-*`; no V019+ filenames chosen)

Ordering rule: plans execute in the listed order; PostgreSQL plans precede dependent ClickHouse plans; one numbered SQL file may realize one or more plans (numbering assigned at merge from the then-current `manifest.txt`). Every applied migration stays immutable (§4.3); fixes are new forward migrations. **Rollback** here = restore-from-verified-backup unless a reversible `DOWN` is explicitly listed. Verification = migration runs from every supported upgrade origin on an empty and a populated fixture, idempotent re-run, tenant-isolation constraint tests and compatibility test with the previous application version.

| Plan | Order | Owner BUILD | Content | Forward | Rollback / one-way | Backfill | Compatibility | Verification | Failure / restore |
|---|---|---|---|---|---|---|---|---|---|
| MIG-PLAN-001 | 1 | 061/073 | `api_idempotency_record`, `event_outbox`, `audit_event` (+partitioning), same-tenant helper indexes | create-only | DOWN drops new tables (empty-only); once populated one-way | none | additive; old app ignores | constraint/append-only (`REVOKE`) tests; partition creation | restore pre-migration backup |
| MIG-PLAN-002 | 2 | 066/068 | `provisioning_request`, `onboarding_candidate/decision`, `entity_agent_binding`; additive CHECK extension on `entity` and `discovery_candidate` | create + `ALTER … DROP/ADD CONSTRAINT` (superset) | **one-way** after any new-state row exists | map `discovery_candidate.review_status` → `onboarding_candidate` rows (idempotent by `dedupe_key`); no entity state rewritten | old app reading old values unaffected; new values appear only after cutover flag | backfill count parity; no duplicate candidates; state-map tests | restore; re-run backfill (idempotent) |
| MIG-PLAN-003 | 3 | 070/083 | policy/version, profile/version/assignment, maintenance, suppression, mute | create-only | DOWN while empty; else one-way | none (no pre-existing policy data) | additive | immutability + EXPLICIT_PROFILE-not-replaced tests | restore |
| MIG-PLAN-004 | 4 | 081/084/100 | `alert_current`, `alert_lifecycle_event`, `incident*`, `rca_*`, `evidence_ref` | create-only | DOWN while empty; else one-way | none | additive | partial-unique (alert_key/correlation_key) concurrency tests; seq-under-lock tests | restore |
| MIG-PLAN-005 | 5 | 090/091 | `ncm_job*`, `ncm_config_version`; plan/approval tables created **inactive** (Phase 1.5) | create-only | DOWN while empty | none | additive | hash-dedupe + one-active-target tests | restore |
| MIG-PLAN-006 | 6 | 092 | IPAM tables | create-only | DOWN while empty | optional import from discovery/DHCP observations as `observed_state` only | additive | partial-unique reservation concurrency; overlap-lock test | restore |
| MIG-PLAN-007 | 7 | 095 | report tables (`format CHECK PDF/CSV`) | create-only | DOWN while empty | none | additive | UNIQUE (obligation, planned_at) test; format-check negative test | restore |
| MIG-PLAN-008 | 8 | 086 | notification tables | create-only | DOWN while empty | none | additive | dedupe, per-recipient auth re-check tests | restore |
| MIG-PLAN-009 | 9 | 012/015/091 | diagnostics, investigation, CLI session tables | create-only | DOWN while empty | none | additive | no-secret-column schema test; EDS dedupe partial-unique test | restore |
| MIG-PLAN-010 | 10 | 086 | integration tables | create-only | DOWN while empty | none | additive | outbox/inbox uniqueness; lease concurrency | restore |
| MIG-PLAN-011 | 11 | 096 | licence tables | create-only | DOWN while empty | none (no entitlement guessing; quantities `FOUNDER-RESERVED`) | additive | state-history idempotency; expiry/grace boundary tests (T-7 vs +15d) | restore |
| MIG-PLAN-012 | 12 | 097/125/132 | `platform_operation*`, retention/storage/legal-hold tables | create-only | DOWN while empty | none | additive | single-active-operation partial-unique; STALE_PREVIEW test | restore |
| MIG-PLAN-013 | 13 | 064 | ClickHouse `flow_record_raw` (+rollups), `log_event_raw`, `trap_event_raw` + `FINAL` views | create-only (`IF NOT EXISTS`) | DROP new tables while empty; never modifies existing tables | none (history starts at cutover) | additive; existing `metric_*` untouched | replay-collapse, quarantine-not-inserted, cardinality-budget tests | recreate; replay from Kafka retention |
| MIG-PLAN-014 | 14 | 064/074 | `audit_history`, `telemetry_quality_event`, `topology_observation_history` | create-only | as 013 | backfill `audit_history` from PostgreSQL `audit_event` (idempotent by tenant_seq) | additive | parity counts; legal-hold partition test | rebuild from PostgreSQL |
| MIG-PLAN-015 | 15 | 064 | `process_network_evidence`, `active_path_observation/hop` | create-only | as 013 | none | additive; typed-record schema versions pinned | no-prohibited-column test; comparison-eligibility default test | recreate |
| MIG-PLAN-016 | 16 | 064/081 | `alert_history`, `incident_history` + MVs | create-only | as 013 | backfill from PostgreSQL lifecycle tables (idempotent by (id, seq)) | additive | parity counts | rebuild from PostgreSQL |

**One-way treatment:** a plan marked "one-way after population" requires, before application: verified backup, explicit operator approval (`Platform_16`), an `UPGRADE_ORIGIN` entry and a documented restore path; the upgrade journey (`Platform_06` §119.21) reports `ROLLBACK_UNSUPPORTED` for it. Phase-1.5 objects (`ncm_change_plan*`, `ncm_approval`) are created inactive and carry no behavior until BUILD-090/091 Phase-1.5 certification.

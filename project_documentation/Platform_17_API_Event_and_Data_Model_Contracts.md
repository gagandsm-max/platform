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

Migrations are versioned, automated, tested from supported upgrade origins and idempotent where practical. Destructive migrations require backup/rollback plan and explicit approval.

### 4.4 Retention classes

Raw, rollup, incident, audit, configuration, **diagnostic/investigation evidence** and protected/legal-hold classes use different lifecycle policies. Investigation evidence linked to an incident/legal hold follows the applicable protected lifecycle; ephemeral one-time credentials are never a retention class. Emergency purge never treats all tables equally.

### 4.5 Data integrity

Ingestion records support deduplication/idempotency and late/out-of-order data handling. Pipeline reconciliation detects produced/accepted/normalized/inserted/rejected discrepancies.

Diagnostic/investigation integrity requires tenant-scoped canonical targets, immutable result/evidence provenance, per-vantage source identity, result/observed timestamps, correlation IDs and explicit state transitions. Evidence references must not silently retarget another tenant/entity after creation. Cancellation/timeout/partial results remain visible. No secret or one-time credential value may be persisted in diagnostic result, investigation evidence, audit, event or analytics rows.

SNMP integrity additionally requires exact Counter64 preservation before derivation, explicit counter reset/wrap/discontinuity quality, typed handling of non-numeric varbinds/exceptions, stable observation identity for replay/deduplication, and produced→accepted→typed-normalized→persisted accounting by source record/observation class. A failed/unsupported conversion must not be hidden by advancing an acknowledgement over unaccounted SNMP evidence. Inform ACK integrity requires pre-ACK storm admission/accounting plus durable local acceptance/accounting evidence and must not imply enrichment completion or Core/Kafka/ClickHouse/alert/incident completion.

---

### 4.6 Endpoint Diagnostic Snapshot contracts — v3.2.2 reconciliation

**EDS is a specialized diagnostic type/profile of the existing Diagnostic Job architecture**, not a second diagnostics framework, registry, scheduler, authorization system, investigation state machine or evidence store. Product-facing language may continue to use **Endpoint Diagnostic Snapshot (EDS)**; the authoritative API/event/data identity remains the Diagnostic Job contract in §1 with:

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
→ Kafka transactional publication
→ PostgreSQL finalize accepted watermark
→ Sensor ACK
```

Semantics are **at-least-once**, not exactly-once. Duplicate Kafka publication of the same stable logical observation identity is tolerated; canonical ClickHouse reads must not expose duplicate logical numeric observations. Concurrent exact HTTP retries while a reservation is `RETRY_PENDING` may amplify republish volume; that is a recorded **P1** hardening concern under the normal serialized Sensor uploader path. No distributed publisher lease/service is introduced by this documentation baseline.

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

> **Bounded subsection.** This section freezes the edge transport contract against exact base SHA `315a20980e89683666a64362f0ed3827bf613d4a` and machine-readable schemas under `schemas/edge/`. The remainder of Platform_17 remains **TRANSITIONAL_LEGACY** pending later normalization. Where older prose above is vaguer or uses illustrative naming (`boot_epoch`, uniform casing), **this section + `schemas/edge/` + Platform_05 §7.0 prevail for edge wire truth**.

### 10.1 Maturity labels

`CURRENT_IMPLEMENTED_V1` · `FROZEN_BUILD_000_INVARIANT` · `BUILD_002_TARGET` · `BUILD_003_TARGET` · `BUILD_004_TARGET`

`SPECIFIED != IMPLEMENTED != TESTED != CERTIFIED`.

### 10.2 Sequence / Counter64

- **FROZEN_BUILD_000_INVARIANT:** JSON-carried delivery/replay sequence maximum = `9007199254740991` (`2^53−1`); sequences ∈ `[1, MAX_SAFE_SEQUENCE]`; no wrap; fail closed on exhaustion. Schemas/edge validator encode this ceiling now.
- **FROZEN_BUILD_000_INVARIANT:** Counter64 metric values remain full unsigned 64-bit exact-counter domain (not sequences).
- **CURRENT_IMPLEMENTED_V1:** Agent/Sensor/Core use Go `uint64` / Java `long` / SQL `BIGINT` and current positive/range behavior. Explicit enforcement of `9007199254740991` at all hops is **not** yet present in runtime.
- **BUILD_002_TARGET:** Agent→Sensor runtime ceiling enforcement.
- **BUILD_003_TARGET:** Sensor→Core runtime ceiling enforcement.

### 10.3 Exact v1 ACK naming (do not “normalize” casing)

| Path | ACK fields (exact) | Durability |
|---|---|---|
| Agent→Sensor | `status`, `accepted_through_sequence`, `agent_id`, `batch_id`, `correlation_id` | Durable Sensor Agent spool accept |
| Sensor→Core telemetry | `status`, `accepted_through_sequence`, `sensor_identity` | PG reserve → Kafka txn → PG finalize → ACK; CH async |
| Sensor→Core agent-ingress | `status`, `sensorId`, `batchId`, `acceptedThroughIngressSequence`, `correlationId` | **at-least-once** (Kafka + PG watermark); **BUILD_003_TARGET** reservation parity |

### 10.4 bootEpoch

**CURRENT_IMPLEMENTED_V1 / FROZEN:** wire `bootEpoch` = Sensor stream/generation id `^stream-[0-9a-f]{32}$`. Historical LLD `boot_epoch` wording is an alias; not OS boot time.

### 10.5 Heartbeat v1 vs desired-state

**CURRENT_IMPLEMENTED_V1 Core response (wire authority):** `status`, `authenticated`, `sensor_identity`, `tenant_id`, `received_at`.

**Compatibility:** current Sensor client models additional desired-state fields and ignores unknown Core fields (`tenant_id` / `received_at`) via `doJSON` strict-decode fallback — heartbeat is not currently broken. Desired-state response cleanup/versioning is **BUILD_004_TARGET**.

Older §3.3 “versioned desired configuration/assignment” language is **CORRECT_BUT_UNIMPLEMENTED / BUILD_004_TARGET** for heartbeat authority. Do not read Sensor optional request version fields as Core-applied truth.

### 10.6 Entity authority

Agent `hostId` is evidence. Agent-supplied `entityId` is not authoritative. Core owns canonical entity mapping (**FROZEN_BUILD_000_INVARIANT** / B000-D009).

### 10.7 Agent stream generation

**CURRENT V1 LIMITATION:** Sensor Agent replay keyed by `tenantId+agentId`; lineage restart fails closed.

**BUILD_002_TARGET:** durable `agentStreamId` (working name) on versioned Agent→Sensor contract (B000-D015). Not implemented in BUILD-000.

### 10.8 Monitoring correctness

Collection/transport/credential/Agent/Sensor/Core/queue/ingestion failure, no telemetry, stale telemetry MUST NOT automatically assert monitored target DOWN. `No Data != Device Down` (**FROZEN_BUILD_000_INVARIANT** / B000-D012). Phase 1 monitoring-quality contracts preserve `UNKNOWN` / `PARTIAL` / degraded semantic states rather than collapsing them into DOWN.

### 10.9 Standard error

Wire `{status, code, message}` remains. Semantic classes: `INVALID_CONTRACT`, `AUTHENTICATION`, `AUTHORIZATION`, `STALE_OR_FENCED`, `UNSUPPORTED_VERSION`, `INTEGRITY`, `REPLAY_OR_DUPLICATE`, `CAPACITY_OR_RATE_LIMIT`, `TIMEOUT_OR_DEADLINE`, `CANCELLED`, `TRANSPORT`, `SERVER_UNAVAILABLE`. Detailed `ERR_*` codes remain; mapping is documented in `schemas/edge/README.md`.

### 10.10 Classification of conflicting transitional wording

| Older Platform_17 wording | Classification |
|---|---|
| §3.3 desired config ACK via control channel | BUILD_004_TARGET / not heartbeat v1 |
| §3.2.1 OpenAPI frozen before GA | FUTURE |
| §7 telemetry ACK pipeline | CURRENT_IMPLEMENTED_V1 for main telemetry; agent-ingress weaker (BUILD_003_TARGET) |
| Uniform identifier casing assumptions | STALE vs frozen dual naming |
| Illustrative `boot_epoch` naming | Alias of wire `bootEpoch` stream id |

## 11. Network & Infrastructure Observability data model and event contracts supplement — v3.2.2

> **Reconciliation reference:** Issue #142 (Master Chat comments `5909076167` and `5909787672`). Governed by `REQ-INF-013` (bounded Linux Agent eBPF process-to-network telemetry), `REQ-NET-016` (active path hop/loss/latency/fingerprint observability), `REQ-INF-008` (cloud network entities/relationships/change evidence), and `REQ-INF-010` (OTLP phase boundary: metrics/logs in Phase 1, bounded trace/dependency summaries in Phase 1.5). Target Phase-1 capabilities are subject to applicable implementation, benchmark, and certification gates.

### 11.1 Purpose and architectural constraints

This section supplements `Platform_17` with authoritative wire schemas, event families, and logical data model contracts for:
1. **Linux Agent eBPF process-to-network telemetry**: local aggregation around `process/service + remote endpoint + protocol/port + time bucket`, transported via existing durable Agent queue → Sensor spool → Core ingestion → ClickHouse, with zero separate eBPF datastore or service.
2. **Active path hop-by-hop telemetry and change events**: Sensor-executed multi-vantage path probing, hop metrics, path fingerprinting, and divergence events.
3. **Distributed trace reference contract**: `trace_id` / `span_id` as future-compatible reference attributes across logs, events, alerts, and investigations, without Phase-1 trace storage or APM backend.

### 11.2 Typed Linux Agent process-to-network evidence contract

#### 11.2.1 Transport envelope and identity

Process-network facts are produced by the Linux Agent eBPF collector, aggregated locally, and delivered over the existing versioned Agent→Sensor HTTPS/gRPC mTLS transport. The assigned Sensor buffers records in its durable spool and forwards batches to the Core via the canonical route `POST /api/v1/sensor/agent-telemetry` (handled by `SensorAgentIngestionController`).

**Authority and Identity Model:**
- **Sensor Certificate Authority:** Core authenticates the Sensor mTLS certificate. `AgentIngestionValidator.resolveAuthority(clientCert)` derives authoritative `SensorScope` (tenant and sensor). Core then validates incoming Agent records against that authority.
- **Tenant Authority:** The Agent-origin tenant claim is not independently authoritative at Core. Core tenant authority derives from authenticated `SensorScope`. The Agent-origin claim/envelope tenant MUST equal the authenticated Sensor tenant. Downstream canonical evidence receives `tenant_id` only after this validation succeeds.
- **Sensor & Assignment Provenance:** The accepting Sensor identifier (`acceptingSensorId`) must equal the authenticated Sensor. The tuple `agentId + assignmentGeneration` must resolve to an authoritative `AgentSensorAssignment`, and the assignment Sensor must match the authenticated Sensor.
- **Agent Identity:** `agent_id` is a bounded canonical Agent identifier string (e.g. `"agent-01"`), validated against tenant + authenticated Sensor + `assignment_generation` before downstream canonicalization. It is not redefined as UUID-only and remains compatible with existing Agent identifier naming.
- **Host Identifier & Entity Resolution:** `host_id` is a bounded Agent-observed host identifier string (e.g. `"host-1"`). It represents corroborating identity evidence only, not canonical entity authority (`Agent hostId != canonical entityId`). Core resolves `tenant + namespace="platform.agent" + identifier_type="HOST_ID" + host_id` into canonical `entity_id`. If resolution fails, the platform follows the existing governed unresolved-entity behavior rather than fabricating an entity. Agent-supplied `entityId` is never accepted as authoritative.
- **Transport Inheritance:** Authority, assignment, replay and delivery provenance are inherited from and validated by the enclosing Agent/Sensor transport envelope (including authenticated sensor scope, `sensor_id`, `accepting_sensor_id`, `agent_id`, `assignment_generation`, agent stream identity, sensor ingress sequence, agent sequence, batch/idempotency identity, and correlation identity) before process-network evidence is canonicalized or persisted.
- **Workload Aggregation Identity Invariant:** Every single-valued workload/local identity attribute emitted on an aggregated `ProcessNetworkEvidence` record must be common to every event represented by that record. If multiple processes or workloads are deliberately merged (e.g. across worker processes under a service or container scope), workload-specific attributes (such as `pid` or specific local socket ports) must not pretend to identify one member; such fields must be omitted or null, or represented through explicitly aggregated dimensions, and the record must expose aggregate scope/quality appropriately. Applicable identity dimensions may include executable/process identity, service identity, container identity, cgroup identity, network namespace, and local endpoint scope. PID remains an ephemeral runtime observation only and must NEVER become durable entity authority. Exact implementation and cardinality balance are owned by BUILD-011 and certified by BUILD-019 / BUILD-130 without freezing premature cardinality constants.

Future extension and versioning of the physical envelope belongs to BUILD-016 / BUILD-062 and must preserve authority, idempotency, ACK, and durability contracts. Ingestion validation and envelope acceptance are owned by BUILD-062, physical Kafka streaming (topic naming, partitions, serialization) is owned by BUILD-063, and physical ClickHouse storage (table DDL, engine selection, TTL) is owned by BUILD-064.

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
    local_ip:               string (IPv4 or IPv6 canonical string)
    local_port:             uint16
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

Sensor-executed active path probes emit structured hop sequences and path-level performance facts:

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
    path_fingerprint:       string (opaque, versioned, deterministic path fingerprint; algorithm selected and implemented by BUILD-040)
    fingerprint_version:    string (e.g. "1")
    status:                 "COMPLETE" | "PARTIAL" | "TARGET_UNREACHABLE" | "FAILED"
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

#### 11.3.2 Path change event contract

When the path fingerprint changes between consecutive probing intervals or deviates from the established baseline, the Sensor or Core generates a versioned internal change event (physical Kafka topic naming, partitioning, and serialization owned by BUILD-063):

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
```

#### 11.3.3 Operational and causal integrity rules

- **Incomplete Path != Target Down:** An unresponsive intermediate hop or incomplete traceroute (e.g. intermediate ICMP suppression or timeouts) MUST NOT assert target entity DOWN when corroborating ICMP echo, TCP port check, or SNMP polling indicates reachability.
- **Multi-Vantage Contradiction Preservation:** Divergence between probes from different Sensors (e.g. Sensor-A sees 100% loss at transit hop 4, Sensor-B reaches target normally) represents an upstream routing or peering anomaly, not target failure. Contradictions must be preserved and surfaced to RCA as evidence.

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

## 14. Phase 1 / Phase 1.5 certification envelopes — Issue #200 / #203

Architecture target ≠ implementation ≠ tested ≠ benchmarked ≠ certified ≠ deployed. Mother Reference set version remains **3.3.0** through Tranche B; v3.4.0 is reserved for gated Tranche D. This document remains TRANSITIONAL_LEGACY except where BUILD-000 edge freeze applies.

| Envelope | API / event / schema emphasis | ACK / tenancy | Explicit nonclaims |
|---|---|---|---|
| **Phase 1** | Tenant-scoped APIs/events; Sensor/Agent mTLS identity; assignment/fencing generations; SNMP typed + ICMP typed observation contracts; `No Data != Device Down` | Standalone deletion-ACK durability matrix; tenant-aware contracts without MSP isolation cert claim | Issue #218 Core ICMP acceptance open; no Core-HA continuity claim; NCM automation not Phase 1 default |
| **Phase 1.5** | Same contracts under two-Core HA continuity and MSP adversarial isolation certification | `ACKED-FOR-DELETION TELEMETRY RPO = 0` for certified permanent one-Core HA failures | Physical HA mechanisms unselected; no new journal/witness/database invented by this contract note |


## Issue #252 — delivery, report and formula contract supplement

Governing decision: [Issue #252](https://github.com/gagandsm-max/platform/issues/252), 21 October 2026. This supplement is SPECIFIED only. It preserves version **3.3.0**, the existing formal IDs, the 25-file Mother Reference set and the frozen Issue #200 Phase 1 / Phase 1.5 envelopes. It neither certifies implementation nor removes an existing release obligation.

Extend §8.1/§8.2/§13.2 rather than create a second query or delivery authority. Reusable profiles carry tenant ownership, version, endpoint/TLS/auth policy and write-only secret references. Activation-test evidence binds profile/secret version, destination/provider policy, result and validity; material changes stale the test. Typed event and report operations reference governed targets and authorized recipients. Per-target delivery attempts preserve stable operation/incident-generation identity, attempt, deadline, retry classification, sanitized provider reference, explicit ambiguous outcome and final disposition; provider acceptance is not generic exactly-once recipient delivery.

A report run binds definition/formula/catalogue versions, authorized scope, schedule occurrence/generation, execution timezone distinct from render timezone, evidence quality and bounded artifacts/checksums. Formats remain PDF/CSV. Execution, dispatch and artifact download recheck current tenant/resource/export authority. Download references expire and revoke; a token or prior recipient row is not an authorization grant. Encoded message/artifact limits and partial/cancelled/failure states are explicit.

The Issue #252 detailed formula contract owns the versioned declarative/checked typed AST and compile/preview interfaces. Persist definitions and provenance in PostgreSQL; approved ClickHouse query plans are derived, version-bound artifacts. No user executable SQL, identifiers, unrestricted joins/UDFs or arbitrary code. Preview shares production authorization/compiler/semantics, exposes sampling/quality and cannot mutate operational state. Recovery uses durable occurrence/operation identity, current boot/process identity and version/generation fencing, never an assertion that an old process, flock or ephemeral authentication survived reboot.

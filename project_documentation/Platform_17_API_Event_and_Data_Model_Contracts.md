# Platform — API, Event, Data Model & Schema Contracts

**Document ID:** PLATFORM-CONTRACTDATA-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract (TRANSITIONAL_LEGACY)**
**Normalization state:** TRANSITIONAL_LEGACY. BUILD-000 freezes a bounded edge/control contract subsection below; the remainder awaits later normalization.
**Product:** Platform (internal engineering/build identifier) / AmrudAi (commercial/customer-facing product name)
**Consolidation rule:** This v3.2.2 document preserves the normative content of the listed v3.1 source documents (via v3.2) and adds the approved v3.2.2 NOC troubleshooting plus enterprise-SNMP correctness API/event/data contracts. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.
**Last reconciliation:** 2026-09-30 — Unified Network & Infrastructure Observability Phase-1 closure and eBPF integration (Issue #142); document remains TRANSITIONAL_LEGACY.

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

Collection/transport/credential/Agent/Sensor/Core/queue/ingestion failure, no telemetry, stale telemetry MUST NOT automatically assert monitored target DOWN. `No Data != Device Down` (**FROZEN_BUILD_000_INVARIANT** / B000-D012).

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

Process-network facts are produced by the Linux Agent eBPF collector, aggregated locally, and delivered over the existing versioned Agent→Sensor HTTPS/gRPC mTLS transport (`/api/v1/agent/telemetry`). The logical wire record adheres to the following contract (physical topic naming, serialization, and ClickHouse tables are owned by BUILD-062/064):

```text
ProcessNetworkEvidence:
  schema_version:           "1.0"
  tenant_id:                UUID (authoritative from Agent mTLS certificate)
  agent_id:                 UUID (authoritative Agent identity)
  host_id:                  UUID (Agent-observed host ID, corroborating evidence)
  entity_id:                UUID (nullable, Core-resolved canonical entity identity; Agent hostId is not authoritative entityId)
  bucket_start_at:          RFC3339UTC instant
  bucket_end_at:            RFC3339UTC instant
  bucket_duration_sec:      uint32 (bounded configurable aggregation interval)
  process_metadata:
    process_name:           string (max 64 chars, e.g. "nginx", "envoy", "mysqld")
    executable_path:        string (max 255 chars, e.g. "/usr/sbin/nginx")
    service_name:           string (max 128 chars, systemd service or container service name)
    cmdline_sanitized:      string (max 255 chars, sanitized argv without secrets/tokens/env vars)
    pid:                    uint32 (runtime observation only, non-authoritative ephemeral identifier)
    container_id:           string (nullable, max 64 chars, e.g. Docker/containerd container ID)
    cgroup_path:            string (max 255 chars, e.g. "/system.slice/nginx.service")
    network_namespace:      uint64 (netns inode number)
  endpoint_pair:
    local_ip:               string (IPv4 or IPv6 canonical string)
    local_port:             uint16
    remote_ip:              string (IPv4 or IPv6 canonical string)
    remote_port:            uint16
    remote_entity_id:       UUID (nullable, Core-resolved canonical target entity)
    protocol:               string ("TCP" | "UDP")
    ip_version:             string ("IPv4" | "IPv6")
  traffic_metrics:
    active_connections:     uint32 (gauge at bucket end)
    new_connections:        uint32 (counter within bucket)
    closed_connections:     uint32 (counter within bucket)
    bytes_tx:               uint64 (raw bytes sent)
    bytes_rx:               uint64 (raw bytes received)
    duration_total_ms:      uint64 (aggregate duration of closed sockets)
    mean_rtt_us:            uint32 (microsecond RTT from TCP info/SRTT)
    retransmit_count:       uint32 (TCP segment retransmissions)
    connection_failures:    uint32 (TCP RST, connection timeouts, SYN retries exceeded)
  quality_and_shedding:
    quality_state:          "VALID" | "PARTIAL" | "SAMPLED" | "RESOURCE_GUARDED"
    dropped_records_count:  uint32 (records dropped due to queue or buffer saturation)
    shed_records_count:     uint32 (records shed due to memory/CPU guard thresholds)
    sampled_flag:           boolean
    partial_flag:           boolean
    resource_guarded_flag:  boolean
```

#### 11.2.2 Hard data-minimization boundaries

In accordance with Platform security threat models (`Platform_18`), the following data categories are **STRICTLY PROHIBITED** from process-network payloads:
- Application payload / packet data;
- TLS / SSL plaintext or decrypted payloads;
- HTTP request/response headers or bodies;
- SQL query text, parameters, or database statements;
- Environment variables or full unscrubbed command-line strings;
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
  source_sensor_id:         UUID (Sensor executing the probe)
  source_vantage_id:        string (vantage location tag, e.g. "aws-eu-west-1", "dc-onprem-tor")
  target_entity_id:         UUID (canonical target entity identity)
  target_ip:                string (IPv4 or IPv6)
  target_port:              uint16 (nullable for ICMP)
  protocol:                 "ICMP" | "TCP" | "UDP"
  probed_at:                RFC3339UTC instant
  path_summary:
    total_hops:             uint8
    responsive_hops:        uint8
    end_to_end_rtt_min_ms:  float32
    end_to_end_rtt_mean_ms: float32
    end_to_end_rtt_max_ms:  float32
    end_to_end_loss_pct:    float32
    jitter_ms:              float32
    path_fingerprint:       string (SHA-256 hash of ordered responsive hop IPs + ASNs)
    status:                 "COMPLETE" | "PARTIAL" | "TARGET_UNREACHABLE" | "FAILED"
  hops:
    - hop_index:            uint8 (1..64)
      hop_ip:               string (IPv4 or IPv6, or "*" if unresponsive)
      hop_hostname:         string (nullable, reverse DNS PTR)
      canonical_entity_id:  UUID (nullable, Core-resolved entity ID if internal infrastructure)
      asn:                  uint32 (nullable, BGP autonomous system number)
      rtt_min_ms:           float32
      rtt_mean_ms:          float32
      rtt_max_ms:           float32
      loss_pct:             float32
      jitter_ms:            float32
      mpls_labels:          uint32[] (nullable, extracted from ICMP time-exceeded extensions)
      is_unresponsive:      boolean (true if ICMP time exceeded not received)
      is_hidden:            boolean (true if middlebox/router suppresses TTL expiry)
```

#### 11.3.2 Path change event contract

When the path fingerprint changes between consecutive probing intervals or deviates from the established baseline, the Sensor or Core generates a versioned internal change event (physical Kafka topic and serialization owned by BUILD-062):

```text
PathChangeEvidence:
  schema_version:           "1.0"
  event_id:                 UUID
  tenant_id:                UUID
  path_id:                  UUID
  source_sensor_id:         UUID
  target_entity_id:         UUID
  detected_at:              RFC3339UTC instant
  previous_fingerprint:     string
  current_fingerprint:      string
  change_type:              "PATH_DIVERGENCE" | "HOP_ADDED" | "HOP_REMOVED" | "RTT_SHIFT" | "LOSS_ANOMALY"
  divergence_hop_index:     uint8 (first hop index where divergence was observed)
  previous_hop_ip:          string (nullable)
  current_hop_ip:           string (nullable)
  delta_rtt_ms:             float32 (nullable)
  delta_loss_pct:           float32 (nullable)
  correlation_id:           string
```

#### 11.3.3 Operational and causal integrity rules

- **Incomplete Path != Target Down:** An unresponsive intermediate hop (`* * *`) or incomplete traceroute MUST NOT assert target entity DOWN when corroborating ICMP echo, TCP port check, or SNMP polling indicates reachability.
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

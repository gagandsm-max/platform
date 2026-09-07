# Platform — Operational Intelligence, MTTR Reduction & Customer Value Master

**Document ID:** PLATFORM-INTELVALUE-001  
**Version:** 3.2.2  
**Status:** **Authoritative Mother Reference — Operational Intelligence / MTTR / Customer Value**  
**Product:** Platform  
**Change class:** Additive approved product scope; does not remove or weaken any v3.2.1 requirement

---

## 1. Authority and purpose

This document is the authoritative v3.2.2 specialized master for the operational-intelligence and customer-value scope approved by the product owner on 2026-09-07.

It is intentionally consolidated rather than duplicating the same requirement text across BRD, PRD, Architecture, HLD, LLD, Dataflow, UI, data-contract, quality and threat-model files. For this capability family, the sections below carry the corresponding logical-layer authority:

- §3 = business/product requirements (BRD/PRD supplement);
- §4 = governing architecture;
- §5 = HLD ownership and subsystem boundaries;
- §6 = LLD/data contracts;
- §7 = end-to-end dataflows;
- §8 = UI/UX requirements;
- §9 = protocol/connector requirements;
- §10 = security/threat requirements;
- §11 = performance/capacity/storage requirements;
- §12 = quality/release gates;
- §13 = operational/runbook requirements;
- §14 = formal requirements traceability supplement.

`Platform_00_Document_Index.md` defines precedence. Where an older v3.2.1 document is silent, this document adds normative v3.2.2 behavior. Where an actual contradiction is found, the higher-precedence governing layer wins and change control must reconcile the conflict before implementation.

After this branch is merged to canonical `main`, `PRD-OIV-*` and `REQ-OIV-*` requirements are implementation-authoritative. Before merge, this branch remains proposed change-control state only.

The purpose is not to add decorative AI. The purpose is to reduce operational noise, shorten time to diagnosis and restoration, preserve transient evidence, correlate cross-domain changes, enable bounded controlled remediation, and prove customer value using evidence rather than unsupported claims.

---

## 2. Non-negotiable principles

1. **Evidence before AI.** Protocol, topology, telemetry, change, path and operator-confirmed evidence remain authoritative for causal conclusions.
2. **`INCONCLUSIVE` remains valid.** GNN/ML/LLM must not invent a root cause when evidence is insufficient.
3. **No silent DPI scope expansion.** Triggered packet evidence is bounded diagnostic capture; full always-on DPI remains outside initial product scope.
4. **No direct LLM privileged execution.** Controlled remediation requires deterministic action definitions, authorization, guardrails, approval/policy, validation and rollback/fail-safe handling.
5. **Customer value must be auditable.** Platform shall distinguish measured, derived, estimated and customer-supplied values.
6. **No false causality in value reporting.** `Capability participated` is not the same as `capability caused the improvement`.
7. **No fabricated financial savings.** Monetary benefit is shown only from customer-approved assumptions and is labelled as an estimate unless independently audited.
8. **Tenant isolation applies to evidence and actions.** Packet evidence, incident evidence, similarity results, changes, remediation and value facts are tenant-scoped.
9. **No new database/service merely for fashion.** Use the existing PostgreSQL/ClickHouse/Kafka/Valkey architecture and modular Core unless benchmark/security evidence requires another component.
10. **Self-observability is mandatory.** These intelligence capabilities must expose freshness, completeness, lag, failure and resource impact.

---

## 3. Formal product requirements

The following IDs are authoritative PRD supplement requirements for v3.2.2.

| PRD ID | Requirement |
|---|---|
| `PRD-OIV-001` | Platform shall provide first-class SaaS monitoring through reusable certified SaaS Packs/connectors so common SaaS services do not require every customer to build a custom connector. |
| `PRD-OIV-002` | Platform shall provide privileged, bounded, incident-triggered packet evidence without making full always-on DPI an initial product requirement. |
| `PRD-OIV-003` | Platform shall discover and maintain cloud-native runtime service dependencies from approved Kubernetes, OpenTelemetry, service-mesh and cloud sources where evidence exists. |
| `PRD-OIV-004` | Platform shall ingest and normalize external deployment/change telemetry from CI/CD, Git/release, IaC, cloud, Kubernetes, database, certificate, patch/reboot and ITSM sources. |
| `PRD-OIV-005` | Platform shall enrich circuit/path/SLA investigations with carrier/ISP operational evidence where provider/customer integrations permit it. |
| `PRD-OIV-006` | Platform shall provide an Incident Flight Recorder that preserves a bounded before/during/after evidence window for qualifying incidents and investigations. |
| `PRD-OIV-007` | Platform shall provide Similar Incident Memory that surfaces relevant historical incidents and outcomes without silently copying an old root cause into the current incident. |
| `PRD-OIV-008` | Platform shall provide cross-domain Change Intelligence that answers `What changed before this incident?` across network, cloud, application, database, Kubernetes, infrastructure and Platform changes. |
| `PRD-OIV-009` | Platform shall support controlled remediation using the workflow `diagnose -> recommend -> precondition -> impact preview -> authorization/approval -> execute -> validate -> rollback/escalate`. |
| `PRD-OIV-010` | Platform shall provide an evidence-backed Platform Value / MTTR Improvement Engine measuring detection, qualification, acknowledgement, root-cause, remediation and restoration timing. |
| `PRD-OIV-011` | Each qualifying incident shall show where Platform capabilities contributed evidence or actions and shall distinguish participation from proven causal benefit. |
| `PRD-OIV-012` | Platform shall provide a four-layer customer-value dashboard covering operational noise reduction, diagnosis effectiveness, incident improvement and evidence-supported operational/economic benefit. |
| `PRD-OIV-013` | Platform shall expose health, lag, completeness, capacity and failure telemetry for all OIV capabilities themselves. |
| `PRD-OIV-014` | Platform shall provide role-aware APIs and UI for OIV capabilities with tenant scope, audit, query bounds and safe error behavior. |

### 3.1 First-class SaaS monitoring — `PRD-OIV-001`

A SaaS Pack is a versioned Platform connector/monitoring package with provider-specific authentication, collection, normalization, topology/business-service mapping, dashboards, health and diagnostics.

Where provider APIs permit, packs should support:

- provider/service status;
- customer tenant/account/subscription health;
- API latency, errors, throttling and quotas;
- authentication/session failure signals;
- service-specific operational KPIs;
- audit/change events;
- synthetic validation from selected Sensors/Agents;
- dependency edges into Platform business services;
- provider-incident correlation;
- connector health and rate-limit diagnostics.

Provider API entitlement, commercial access, rate limits, data residency and telemetry limitations must be explicit. Platform must not claim data a provider does not expose.

### 3.2 Triggered packet evidence — `PRD-OIV-002`

Capture may be initiated by an authorized operator, qualifying incident policy, anomaly/path/routing trigger, approved diagnostic runbook, remediation precondition or support diagnostic.

Mandatory controls:

- explicit source/interface/target/filter scope where technically possible;
- maximum duration, bytes, packets and rate;
- no unbounded background capture;
- privileged RBAC/resource scope;
- tenant isolation;
- encryption in transit/at rest;
- retention TTL and legal-hold policy;
- audit of actor/service identity, scope, reason and access;
- payload minimization/redaction policy;
- capture CPU/memory/disk/network telemetry;
- fail-safe termination at configured bounds.

Headers/metadata should be retained instead of payload whenever that is sufficient for diagnosis.

### 3.3 Cloud-native runtime dependency discovery — `PRD-OIV-003`

Platform shall enrich the temporal operational graph using runtime evidence such as:

- ingress/gateway -> service;
- service -> workload;
- workload -> pod;
- pod -> node;
- service/workload -> database/cache/queue/external API;
- cloud load balancer -> target/service;
- namespace/application ownership;
- service-mesh observed calls where approved;
- OpenTelemetry resource/service relationships;
- network/security-policy relationships where evidence exists.

Every inferred edge stores provenance, confidence, first/last observation and validity period. Runtime inference may not silently override customer-defined authoritative dependencies.

### 3.4 External deployment/change telemetry — `PRD-OIV-004`

Supported source classes may include:

- CI/CD deployments;
- Git/release revisions;
- Terraform/IaC apply events;
- Kubernetes/Helm/GitOps rollouts;
- cloud network/security/control-plane changes;
- load-balancer changes;
- database deployment/schema-change metadata;
- certificate rotation;
- patch/reboot events;
- application/feature/configuration release metadata;
- ITSM change records.

Normalized change evidence must retain source, event time, ingest time, actor/service identity where available, affected entity/service references, change type, version/revision, result, ticket/release reference, provenance and evidence reference.

### 3.5 Carrier/ISP operational integration — `PRD-OIV-005`

Where provider/customer APIs permit, Platform may ingest:

- provider outage/status events;
- circuit/service IDs;
- maintenance notices;
- provider incident/ticket state;
- outage/restore timestamps;
- SLA facts;
- escalation/contact metadata;
- ticket create/update synchronization.

Provider status is supporting evidence, not causal proof. Platform shall reconcile it with observed path, routing, synthetic and reachability evidence.

### 3.6 Incident Flight Recorder — `PRD-OIV-006`

For configured qualified incidents or authorized investigations, Platform shall pin/capture a bounded evidence window containing, where available:

- topology/graph state before, during and after the event;
- interface/link state/errors;
- BGP/OSPF/IS-IS/BFD/HSRP/VRRP and related routing/redundancy changes;
- traceroute/path and synthetic results;
- Flow summaries;
- syslog/traps/events;
- relevant metrics/anomalies;
- NCM and external change facts;
- Sensor/telemetry-quality state;
- server/VM/container/database/storage state;
- business-service state;
- triggered packet evidence when authorized.

Event time, ingest time, quality, provenance and correlation IDs shall be retained. UI shall provide a replayable timeline and historical graph snapshot.

### 3.7 Similar Incident Memory — `PRD-OIV-007`

Resolved incidents may be indexed using deterministic and learned features including root entity/class, temporal subgraph pattern, symptoms/events, protocol/change signatures, business services, site/technology/vendor context and remediation outcome.

Each candidate shall expose:

- similarity score;
- major matching factors;
- accepted historical root cause/outcome;
- remediation/runbook used;
- time to root candidate;
- service restoration time;
- outcome evidence link.

Similarity is advisory. Low-quality, stale, contradictory or poisoned historical evidence must reduce trust or exclude the candidate.

### 3.8 Cross-domain Change Intelligence — `PRD-OIV-008`

The unified change model shall include where available:

- NCM and device configuration changes;
- firewall/load-balancer changes;
- cloud control-plane changes;
- CI/CD deployments;
- IaC changes;
- Kubernetes rollouts;
- database deployment/schema metadata;
- certificate/secret-rotation metadata that is safe to retain;
- patch/reboot/maintenance;
- Platform policy/credential/monitoring changes;
- ITSM change records.

Investigations shall distinguish temporal correlation from proven causation.

### 3.9 Controlled remediation — `PRD-OIV-009`

Initial action classes shall prioritize bounded/reversible operations:

- retry/re-run monitor or synthetic test;
- restart/reconnect an authorized Platform Agent or bounded Platform component;
- move/retry Sensor work under assignment/fencing policy;
- execute an approved diagnostic runbook;
- rollback a Platform-controlled NCM change when a verified rollback artifact exists;
- restore an approved known-good configuration through maker-checker controls.

Guardrails:

- no direct LLM privileged execution;
- explicit action definition and version;
- tenant/resource-scoped authorization;
- maker-checker where policy requires;
- allowlisted command/action semantics;
- parameter validation;
- blast-radius/concurrency limits;
- preflight/dry-run where possible;
- idempotency;
- timeout/cancellation;
- immutable audit;
- post-action validation;
- rollback/fail-safe escalation;
- automatic action-family suspension after repeated unsafe failure according to policy.

### 3.10 Platform Value / MTTR Improvement Engine — `PRD-OIV-010`

Where evidence exists, retain at least:

- `failure_evidence_at`;
- `platform_detected_at`;
- `qualified_at`;
- `incident_created_at`;
- `operator_acknowledged_at`;
- `root_candidate_available_at`;
- `investigation_started_at`;
- `remediation_started_at`;
- `service_restored_at`;
- `incident_closed_at`.

Derived values shall retain formula/version/provenance/confidence where applicable.

Calculate at minimum:

- MTTD;
- qualification latency;
- correlation latency;
- MTTA;
- time to root-cause candidate;
- diagnosis-to-action time;
- service-restoration MTTR;
- closure/admin time;
- reopen rate;
- raw-event -> qualified-alert -> incident compression;
- RCA accepted/false-root/inconclusive rates;
- recommendation adoption/success;
- remediation success/rollback rates.

Comparable cohorts shall be filterable/matched by severity, business service, incident/root-cause class, technology/domain, region/site, impact band, business-hours/after-hours and remediation class where data permits.

Show median and P75/P90 where useful. Disclose sample size, baseline period, matching dimensions and insufficient-sample conditions. A simple unmatched before/after average shall not be presented as causal proof of Platform impact.

### 3.11 Per-incident contribution evidence — `PRD-OIV-011`

Each qualifying incident shall show where available:

- raw events observed;
- qualified alerts;
- symptoms correlated;
- first detection time;
- root-candidate time;
- acknowledgement time;
- restoration time;
- whether Platform RCA preceded accepted human diagnosis;
- whether Similar Incident Memory was used;
- whether Change Intelligence or Flight Recorder contributed evidence;
- whether recommendation/runbook was used;
- whether controlled remediation was executed and validated;
- actual MTTR;
- comparable historical/customer baseline where supportable;
- absolute minutes difference and percentage difference with methodology label;
- data gaps and limitations.

### 3.12 Four-layer value dashboard — `PRD-OIV-012`

**Layer 1 — Operational noise reduction**

- raw events;
- qualified alerts;
- incidents;
- compression ratio;
- duplicate/storm symptoms workflow-suppressed while evidence remains preserved;
- avoidable false-device-down reduction where benchmarkable.

**Layer 2 — Diagnosis effectiveness**

- median/P75/P90 time to root candidate;
- accepted RCA rate;
- false-root rate;
- inconclusive rate;
- Similar Incident Memory usage/success;
- incidents where Platform RCA preceded accepted human diagnosis;
- Flight Recorder evidence completeness.

**Layer 3 — Incident improvement**

- MTTD/MTTA/MTTR trends;
- matched-cohort baseline vs Platform-assisted periods;
- absolute incident minutes difference;
- percentage difference;
- reopen rate;
- severity/business-service breakdown;
- recommendation/remediation outcomes.

**Layer 4 — Operational/economic benefit**

Only evidence-supported values may be shown:

- incident minutes avoided/difference;
- engineer-hours avoided/redirected from customer-supplied staffing assumptions;
- duplicate tickets/escalations avoided;
- user-hours of disruption avoided when affected-user data is available;
- SLA penalty or outage-cost estimates only from customer-approved cost assumptions.

Every economic panel shall show assumptions and calculation basis.

---

## 4. Governing architecture

### 4.1 Architecture rule

These capabilities extend the existing modular Core/Sensor architecture. Do not create a monolithic `AI service` or a new database by default.

Logical responsibility placement:

- **Integration Service / connector framework:** SaaS, carrier, CI/CD, IaC and external change ingestion;
- **Graph Service:** cloud-native runtime dependency enrichment and temporal-edge provenance;
- **Incident/RCA modules:** Flight Recorder orchestration and evidence-window selection;
- **Analytics/RCA module:** Similar Incident Memory feature extraction/scoring;
- **Automation/NCM domain:** controlled-remediation definitions, approvals, execution orchestration and validation;
- **Reporting/SLA/analytics domain:** Platform Value cohorting, metrics and customer-value reporting;
- **Sensor Runtime:** bounded packet capture and edge-side evidence generation where local placement is required;
- **AI Gateway:** explanation, summarization and recommendation assistance only within authorization/data-boundary policy.

### 4.2 Data ownership

- PostgreSQL remains authoritative for transactional definitions, approvals, workflow/current state, connector configuration, remediation definitions/executions and baseline/cohort definitions.
- ClickHouse stores high-volume historical evidence and analytical facts where appropriate.
- Kafka remains the internal event backbone.
- Valkey remains cache/ephemeral state only.
- Packet binary artifacts are not stored indiscriminately in PostgreSQL/ClickHouse. Use a bounded encrypted artifact-storage abstraction with metadata references; implementation must not introduce an unapproved mandatory new storage dependency.

### 4.3 No vector-database assumption

Similar Incident Memory may use deterministic features, SQL/ClickHouse analytical similarity and approved ML embeddings. A dedicated vector database is not part of the v3.2.2 architecture unless a later benchmark/architecture decision justifies it and OSS governance approves it.

---

## 5. HLD subsystem contracts

### 5.1 Evidence Window Orchestrator

Logical module within Incident/RCA domain. Owns:

- incident trigger eligibility;
- before/during/after time window;
- evidence-source fan-out;
- completeness status;
- legal hold/retention reference;
- evidence manifest;
- replay timeline coordination.

It must tolerate missing sources and return `PARTIAL` rather than silently claiming completeness.

### 5.2 Similar Incident Service

Logical analytics/RCA module. Owns:

- feature extraction;
- bounded candidate search;
- similarity scoring/version;
- quality/provenance filtering;
- explanation of matching factors;
- feedback from accepted/rejected suggestions.

### 5.3 Change Intelligence Ingestion

Extends Integration Service. Owns connector health, authentication references, polling/webhook state, normalization, idempotency, clock/provenance fields and change/entity correlation.

### 5.4 Remediation Orchestrator

Logical Automation/NCM module. Owns action definitions, eligibility, precondition checks, blast-radius preview, approval state, execution lease/idempotency, validation, rollback/escalation and immutable audit.

Execution remains on the correct trusted execution boundary (Sensor/Core/Agent/connector) according to action type; Core remains authority.

### 5.5 Value Analytics

Logical Reporting/SLA/analytics module. Owns timestamp interpretation, metric formulas, cohort definitions, comparison methodology, completeness/quality warnings and export/report semantics.

### 5.6 Packet Evidence Worker

Sensor-side bounded worker with independent concurrency/resource controls. It must not starve polling, Flow/event receiving, NCM or Sensor control.

---

## 6. LLD and data contracts

### 6.1 Canonical change fact extension

```text
change_fact
  tenant_id
  change_id
  source_type
  source_instance_id
  event_time
  ingest_time
  actor_or_service_ref nullable
  affected_entity_ids[] bounded/reference model
  affected_service_ids[] bounded/reference model
  change_type
  revision_or_version nullable
  result
  ticket_or_release_ref nullable
  provenance
  quality
  payload_ref nullable
  correlation_id
```

Duplicate/replayed source events shall not generate duplicate causal evidence.

### 6.2 Incident evidence window

```text
incident_evidence_window
  evidence_window_id
  tenant_id
  incident_id
  trigger_type
  trigger_reference
  requested_by nullable
  window_start
  window_end
  state REQUESTED|COLLECTING|COMPLETE|PARTIAL|FAILED|EXPIRED
  completeness_score nullable
  retention_class
  legal_hold_state
  created_at
  finalized_at nullable
```

```text
incident_evidence_ref
  evidence_ref_id
  evidence_window_id
  evidence_type
  source_entity_id nullable
  source_system
  event_time_start nullable
  event_time_end nullable
  storage_reference
  checksum nullable
  quality
  provenance
  sensitive_classification
```

### 6.3 Similarity fact

```text
incident_similarity
  tenant_id
  incident_id
  candidate_incident_id
  model_or_rule_version
  score
  matching_factors
  quality
  calculated_at
```

Similarity results are derived data and may be rebuilt.

### 6.4 Remediation contracts

```text
remediation_action
  action_id
  tenant_id nullable for system template
  action_version
  action_type
  execution_boundary
  required_permission
  approval_policy
  parameter_schema
  precondition_definition
  validation_definition
  rollback_definition nullable
  blast_radius_policy
  status
```

```text
remediation_execution
  execution_id
  tenant_id
  incident_id nullable
  action_id/action_version
  requested_by
  approved_by nullable
  idempotency_key
  state REQUESTED|PREFLIGHT|AWAITING_APPROVAL|APPROVED|EXECUTING|VALIDATING|SUCCEEDED|FAILED|ROLLING_BACK|ROLLED_BACK|ESCALATED|CANCELLED
  target_scope
  parameter_hash/redacted_parameters
  started_at nullable
  completed_at nullable
  result_code nullable
  evidence_ref nullable
  correlation_id
```

### 6.5 Value fact

```text
incident_value_fact
  tenant_id
  incident_id
  metric_version
  failure_evidence_at nullable
  platform_detected_at nullable
  qualified_at nullable
  incident_created_at
  operator_acknowledged_at nullable
  root_candidate_available_at nullable
  investigation_started_at nullable
  remediation_started_at nullable
  service_restored_at nullable
  incident_closed_at nullable
  mttd_ms nullable
  mtta_ms nullable
  time_to_root_candidate_ms nullable
  service_restoration_mttr_ms nullable
  event_count nullable
  qualified_alert_count nullable
  correlated_symptom_count nullable
  platform_capabilities_used
  data_completeness
  calculated_at
```

### 6.6 Baseline cohort definition

```text
value_baseline_cohort
  cohort_id
  tenant_id
  name
  version
  baseline_period
  comparison_period nullable
  match_dimensions
  exclusion_rules
  minimum_sample_policy
  created_by
  created_at
```

### 6.7 SaaS and carrier facts

Versioned contracts shall exist for `saas_service_observation` and `carrier_event`, preserving tenant, provider/source, event/observation time, ingest time, service/circuit references, state, quality, provenance and correlation identifiers.

### 6.8 Packet evidence metadata

```text
packet_evidence_metadata
  packet_evidence_id
  tenant_id
  evidence_window_id nullable
  requested_by
  capture_sensor_id
  interface_or_source_scope
  target_filter
  started_at
  stopped_at
  packet_count
  byte_count
  payload_mode HEADERS_ONLY|BOUNDED_PAYLOAD
  encrypted_artifact_ref
  checksum
  retention_until
  legal_hold_state
  access_classification
```

Raw payload must never be logged or exposed through ordinary telemetry APIs.

---

## 7. End-to-end dataflows

### DF-OIV-001 — External change ingestion

```text
External system
 -> authenticated connector/webhook
 -> schema/source validation
 -> tenant/source ownership validation
 -> dedupe/idempotency
 -> canonical change_fact
 -> Kafka internal event
 -> ClickHouse/history + graph/entity correlation
 -> RCA/Change Intelligence timeline
```

### DF-OIV-002 — Incident Flight Recorder

```text
Qualified incident / authorized investigation
 -> Evidence Window Orchestrator
 -> select bounded before/during/after window
 -> request/query topology + telemetry + events + changes + paths + quality
 -> optional privileged packet evidence trigger
 -> evidence manifest + completeness
 -> immutable incident evidence references
 -> replay timeline / RCA context
```

Failure of one evidence source must result in `PARTIAL` with missing-source disclosure, not silent success.

### DF-OIV-003 — Similar Incident Memory

```text
Current incident
 -> bounded feature extraction
 -> tenant-scoped historical candidate search
 -> quality/provenance filters
 -> rule/ML similarity score
 -> ranked candidates + matching factors
 -> operator/RCA consumption
 -> accept/reject feedback
```

### DF-OIV-004 — Controlled remediation

```text
Incident/RCA recommendation or authorized manual action
 -> resolve versioned action definition
 -> authorization + tenant/resource scope
 -> precondition checks
 -> blast-radius preview
 -> policy/maker-checker approval
 -> idempotent execution lease
 -> trusted executor
 -> validation
 -> success OR rollback/escalation
 -> audit + incident timeline + value fact
```

### DF-OIV-005 — Value computation

```text
Incident lifecycle + RCA + remediation + ITSM evidence
 -> timestamp/provenance normalization
 -> completeness check
 -> metric calculation/version
 -> cohort matching
 -> baseline comparison
 -> dashboard/report
```

If baseline or sample quality is insufficient, UI shall show `INSUFFICIENT EVIDENCE` rather than an invented percentage improvement.

### DF-OIV-006 — Triggered packet evidence

```text
Authorized trigger
 -> RBAC/resource scope
 -> Sensor eligibility/resource check
 -> filter + duration/byte/packet bounds
 -> capture
 -> automatic stop at first bound
 -> encrypted artifact + metadata/checksum
 -> evidence manifest
 -> TTL/legal-hold lifecycle
```

---

## 8. UI/UX requirements

### 8.1 Incident workspace additions

Add role-aware tabs/panels:

- **Evidence / Flight Recorder** — replayable timeline, evidence completeness, historical topology snapshot;
- **Similar Incidents** — ranked candidates, matching factors, accepted historical outcome and links;
- **Changes** — unified cross-domain change timeline with source/filtering and causality disclaimer;
- **Remediation** — recommendation, preconditions, blast-radius preview, approval state, execution/validation/rollback;
- **Value / Contribution** — incident timing, Platform capability participation, baseline comparison and limitations.

### 8.2 Customer Value workspace

Provide a dedicated `Operational Value` / `Platform Value` dashboard accessible according to RBAC. It shall implement the four layers in §3.12 and allow filters for time, tenant, region/site, business service, severity, incident class, technology domain and assisted/remediation mode.

Every improvement figure shall provide a drill-down to cohort definition, sample size and calculation basis.

### 8.3 States

All OIV views shall explicitly support `Loading`, `Empty`, `Partial Data`, `Stale Data`, `Permission Restricted`, `Insufficient Evidence`, `Error` and `Normal` states.

---

## 9. Protocol and connector requirements

1. Reuse the existing versioned Connector framework for SaaS, carrier and external-change integrations.
2. Credentials use existing Credential Profiles and `USE`/`REVEAL` separation.
3. Webhook receivers require authenticated source validation, replay protection where provider semantics permit, schema bounds and rate limits.
4. Polling connectors require timeouts, bounded pagination, throttling/backoff and explicit provider rate-limit health.
5. Provider-specific packs require OSS/licence/API terms review before certification.
6. No connector may turn provider status text directly into a causal root cause without independent evidence.
7. Packet capture requires OS capabilities/privilege minimization and must be isolated from ordinary monitoring workers.

---

## 10. Security and threat requirements

Threat-model and test at minimum:

- unauthorized packet capture or artifact access;
- sensitive payload leakage through logs/support bundles/UI;
- cross-tenant evidence leakage;
- forged/replayed external change facts;
- malicious SaaS/carrier/provider responses;
- connector credential theft;
- poisoned Similar Incident Memory;
- similarity-based tenant data leakage;
- remediation command/parameter injection;
- approval bypass;
- replayed remediation requests;
- excessive remediation blast radius;
- action execution on stale/incorrect topology;
- rollback artifact tampering;
- false MTTR/value attribution;
- manipulated timestamps/cohorts;
- untrusted customer-supplied financial assumptions represented as audited savings.

Mandatory mitigations include tenant-scoped authorization, immutable audit, idempotency, signed/versioned action definitions where applicable, bounded inputs, schema validation, redaction, artifact encryption, TTL, checksums, approval policy, blast-radius limits and explicit data-quality labels.

---

## 11. Performance, capacity and storage requirements

1. Flight Recorder evidence queries/capture must be bounded so incident storms cannot exhaust Core query/storage capacity.
2. Packet capture worker has hard CPU/memory/disk/network/concurrency limits and lower priority than live monitoring continuity.
3. Similar Incident candidate search is bounded by tenant, time/class filters and result count; no unbounded all-history scan on every incident.
4. External connectors expose ingest lag, queue depth, rate-limit state and dropped/rejected events.
5. Evidence retention uses appropriate retention class; packet payload TTL should be short by default and separate from long-lived incident metadata.
6. Legal hold may protect selected evidence, but storage-protection rules and explicit capacity warnings still apply.
7. Value analytics must use rollups/aggregates rather than repeatedly scanning all raw telemetry.
8. OIV workload is included in SNWP/performance certification for applicable scenarios; the feature must not invalidate the Core footprint silently.

---

## 12. Test and release gates

Before any OIV capability is represented as production-ready, applicable tests shall cover:

- tenant isolation and resource scope;
- stale/partial topology;
- missing telemetry/evidence sources;
- clock skew and out-of-order events;
- duplicate/replayed change events;
- provider API timeout/throttling/outage/malformed response;
- Flight Recorder partial/failure/replay correctness;
- packet capture permission, bounds, auto-stop, encryption, TTL and resource protection;
- Similar Incident false positives, adversarial/poisoned history and low-quality evidence;
- remediation deny/precondition-fail/approval/timeout/cancel/partial/validation/rollback paths;
- idempotent remediation replay;
- blast-radius limit enforcement;
- value timestamp ambiguity;
- insufficient cohort size;
- mismatched cohort prevention;
- prevention of unsupported savings claims;
- formula/version migration behavior;
- performance/resource regression;
- upgrade/rollback compatibility;
- audit completeness.

### 12.1 GA claim restrictions

Platform may not externally claim a specific percentage MTTR reduction until reproducible customer/pilot evidence exists for a disclosed population/methodology. Internal engineering targets are not customer claims.

A dashboard may show a customer's measured/matched comparison if the data and methodology meet §3.10 requirements.

---

## 13. Operational/runbook requirements

Add runbooks for:

- Flight Recorder capture failure/partial evidence;
- similarity index stale/corrupt/low-quality state;
- external change connector lag/outage;
- SaaS/carrier connector throttling/auth failure;
- packet evidence worker failure/storage pressure;
- remediation stuck/partial/rollback failure;
- remediation emergency disable/suspension;
- value/cohort data quality issue;
- packet artifact access/retention/legal hold;
- support-bundle generation that excludes sensitive packet payload by default.

Operational teams should use Platform UI/Audit/Support Bundle first; shell access remains exceptional.

---

## 14. Requirements traceability supplement

These `REQ-OIV-*` IDs are authoritative additions to the Platform requirements catalogue. The logical-layer mappings refer to sections of this consolidated master; existing v3.2.1 files are supplemented, not silently rewritten.

| Requirement ID | PRD ID | Capability | Product | Arch | HLD | LLD/Data | Dataflow | UI | Security | Quality |
|---|---|---|---|---|---|---|---|---|---|---|
| `REQ-OIV-001` | `PRD-OIV-001` | First-class SaaS monitoring | §3.1 | §4 | §5.3 | §6.7 | DF-OIV-001 | §8 | §10 | §12 |
| `REQ-OIV-002` | `PRD-OIV-002` | Triggered packet evidence | §3.2 | §4.2 | §5.6 | §6.8 | DF-OIV-006 | §8.1 | §10 | §12 |
| `REQ-OIV-003` | `PRD-OIV-003` | Cloud-native dependency discovery | §3.3 | §4.1 | Graph Service §4.1 | existing graph contracts + §6 | DF-OIV-002/003 | §8.1 | §10 | §12 |
| `REQ-OIV-004` | `PRD-OIV-004` | External change/deployment telemetry | §3.4 | §4.1 | §5.3 | §6.1 | DF-OIV-001 | §8.1 | §10 | §12 |
| `REQ-OIV-005` | `PRD-OIV-005` | Carrier/ISP operational integration | §3.5 | §4.1 | §5.3 | §6.7 | DF-OIV-001 | §8.1 | §10 | §12 |
| `REQ-OIV-006` | `PRD-OIV-006` | Incident Flight Recorder | §3.6 | §4.1 | §5.1 | §6.2 | DF-OIV-002 | §8.1 | §10 | §12 |
| `REQ-OIV-007` | `PRD-OIV-007` | Similar Incident Memory | §3.7 | §4.3 | §5.2 | §6.3 | DF-OIV-003 | §8.1 | §10 | §12 |
| `REQ-OIV-008` | `PRD-OIV-008` | Cross-domain Change Intelligence | §3.8 | §4.1 | §5.3 | §6.1 | DF-OIV-001/002 | §8.1 | §10 | §12 |
| `REQ-OIV-009` | `PRD-OIV-009` | Controlled remediation | §3.9 | §4.1 | §5.4 | §6.4 | DF-OIV-004 | §8.1 | §10 | §12 |
| `REQ-OIV-010` | `PRD-OIV-010` | Platform Value / MTTR Engine | §3.10 | §4.1 | §5.5 | §6.5/6.6 | DF-OIV-005 | §8.2 | §10 | §12 |
| `REQ-OIV-011` | `PRD-OIV-011` | Per-incident Platform contribution | §3.11 | §4.1 | §5.5 | §6.5 | DF-OIV-005 | §8.1 | §10 | §12 |
| `REQ-OIV-012` | `PRD-OIV-012` | Four-layer customer value dashboard | §3.12 | §4.1 | §5.5 | §6.5/6.6 | DF-OIV-005 | §8.2 | §10 | §12 |
| `REQ-OIV-013` | `PRD-OIV-013` | OIV self-observability | §3 | §4 | §5 | §6 | all | §8.3 | §10 | §12 |
| `REQ-OIV-014` | `PRD-OIV-014` | Role-aware APIs/UI and audit | §3 | §4 | §5 | §6 | all | §8 | §10 | §12 |

### 14.1 Affected legacy mother documents

The following legacy files remain governing for their existing scope and are explicitly supplemented by this master for OIV requirements:

- `Platform_01_BRD.md` — business outcomes/value;
- `Platform_02_PRD.md` — customer-visible requirements;
- `Platform_03_Architecture.md` — modular Core/graph/AI/data architecture;
- `Platform_04_HLD.md` — Incident/RCA/Graph/Integration/Reporting/NCM ownership;
- `Platform_05_LLD.md` — data and action contracts;
- `Platform_06_Dataflow.md` — incident/change/RCA/control sequences;
- `Platform_08_Requirements_Traceability.md` — base requirements catalogue;
- `Platform_09_UI_UX_Master.md` — incident/value UI conventions;
- `Platform_10_Protocol_Security_Connector_Master.md` — connector/credential rules;
- `Platform_11_Sizing_Capacity_Storage_Master.md` — workload/storage protection;
- `Platform_14_Test_Performance_and_Release_Quality.md` — quality/benchmark gates;
- `Platform_16_Installation_Upgrade_Rollback_and_Operations.md` — runbooks;
- `Platform_17_API_Event_and_Data_Model_Contracts.md` — API/event/data ownership;
- `Platform_18_Security_Threat_Model.md` — threats/mitigations.

For implementation tasks in this capability family, engineers must read this document in addition to those base documents.

---

## 15. Implementation sequence

These are engineering sequencing decisions, not permission to defer the existing v1 scope.

1. canonical external change/carrier/SaaS connector normalization;
2. Flight Recorder evidence contracts/orchestration and incident UI;
3. Platform Value timestamp/attribution model and value dashboard foundation;
4. Similar Incident Memory;
5. cloud-native runtime graph enrichment;
6. controlled remediation for bounded action classes;
7. triggered packet evidence after security/resource controls pass;
8. expand certified SaaS/carrier packs based on customer demand/provider API viability.

Implement each as a coherent vertical slice with tests, security, observability and upgrade behavior rather than creating one monolithic future branch.

---

## 16. Definition of done

The existence of this document is specification evidence only.

A capability is not complete until implementation, tests, security, tenant isolation, data integrity, observability, capacity/resource bounds, upgrade/migration behavior, operational runbooks, UI behavior, traceability and applicable performance evidence are complete.

No MTTR-reduction or financial-value percentage becomes a product claim merely because the Value Engine can calculate a number.
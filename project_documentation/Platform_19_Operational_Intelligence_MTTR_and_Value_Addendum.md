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

Where provider APIs permit, packs should support provider/service status, customer tenant/account/subscription health, API latency/errors/throttling/quotas, authentication/session failure signals, service-specific KPIs, audit/change events, synthetic validation, business-service dependency edges, provider-incident correlation and connector/rate-limit health.

Provider API entitlement, commercial access, rate limits, data residency and telemetry limitations must be explicit. Platform must not claim data a provider does not expose.

### 3.2 Triggered packet evidence — `PRD-OIV-002`

Capture may be initiated by an authorized operator, qualifying incident policy, anomaly/path/routing trigger, approved diagnostic runbook, remediation precondition or support diagnostic.

Mandatory controls: explicit source/interface/target/filter scope where technically possible; maximum duration/bytes/packets/rate; no unbounded background capture; privileged RBAC/resource scope; tenant isolation; encryption in transit/at rest; retention TTL/legal hold; actor/scope/reason/access audit; payload minimization/redaction; capture resource telemetry; fail-safe termination at configured bounds.

Headers/metadata should be retained instead of payload whenever sufficient.

### 3.3 Cloud-native runtime dependency discovery — `PRD-OIV-003`

Platform shall enrich the temporal operational graph using runtime evidence such as ingress/gateway -> service, service -> workload, workload -> pod, pod -> node, service/workload -> database/cache/queue/external API, cloud load balancer -> target/service, namespace/application ownership, service-mesh observed calls, OpenTelemetry resource/service relationships and network/security-policy relationships where evidence exists.

Every inferred edge stores provenance, confidence, first/last observation and validity period. Runtime inference may not silently override customer-defined authoritative dependencies.

### 3.4 External deployment/change telemetry — `PRD-OIV-004`

Supported source classes may include CI/CD deployments, Git/release revisions, Terraform/IaC applies, Kubernetes/Helm/GitOps rollouts, cloud network/security/control-plane changes, load-balancer changes, DB deployment/schema metadata, certificate rotation, patch/reboot, application/feature/config release metadata and ITSM changes.

Normalized change evidence retains source, event/ingest time, actor/service identity where available, affected entity/service references, change type, version/revision, result, ticket/release reference, provenance and evidence reference.

### 3.5 Carrier/ISP operational integration — `PRD-OIV-005`

Where provider/customer APIs permit, Platform may ingest provider outage/status events, circuit/service IDs, maintenance notices, provider incident/ticket state, outage/restore timestamps, SLA facts, escalation/contact metadata and ticket synchronization.

Provider status is supporting evidence, not causal proof. Reconcile it with observed path/routing/synthetic/reachability evidence.

### 3.6 Incident Flight Recorder — `PRD-OIV-006`

For configured qualified incidents or authorized investigations, Platform shall pin/capture a bounded evidence window containing, where available: topology/graph state before/during/after; interface/link state/errors; BGP/OSPF/IS-IS/BFD/HSRP/VRRP changes; traceroute/path and synthetic results; **bounded Flow summaries such as top conversations/talkers, application/site/QoS mix, material traffic shifts, Flow Coverage/exporter-quality state and observation scope**; syslog/traps/events; relevant metrics/anomalies; NCM/external change facts; Sensor/telemetry quality; server/VM/container/database/storage state; business-service state; and authorized triggered packet evidence. Incident state references bounded Flow evidence/aggregates rather than copying millions of raw Flow rows.

Event time, ingest time, quality, provenance and correlation IDs shall be retained. UI provides a replayable timeline and historical graph snapshot.

### 3.7 Similar Incident Memory — `PRD-OIV-007`

Resolved incidents may be indexed using deterministic/learned features including root entity/class, temporal subgraph pattern, symptoms/events, protocol/change signatures, business services, site/technology/vendor context and remediation outcome.

Each candidate shows similarity score, major matching factors, accepted historical outcome, remediation/runbook used, time to root candidate, restoration time and evidence link.

Similarity is advisory. Low-quality/stale/contradictory/poisoned history reduces trust or is excluded.

### 3.8 Cross-domain Change Intelligence — `PRD-OIV-008`

The unified change model includes where available NCM/device config, firewall/LB, cloud control plane, CI/CD, IaC, Kubernetes, database deployment/schema, safe certificate/secret rotation metadata, patch/reboot/maintenance, Platform policy/credential/monitoring and ITSM changes. Change Intelligence may correlate material Flow traffic shifts with these changes when time/entity/path evidence aligns, but traffic shift/temporal proximity remains supporting or contradictory evidence and is not causal proof.

Investigations distinguish temporal correlation from proven causation.

### 3.9 Controlled remediation — `PRD-OIV-009`

Initial action classes prioritize bounded/reversible operations: retry monitors/synthetics; restart/reconnect an authorized Platform Agent/component; move/retry Sensor work under fencing policy; execute approved diagnostic runbook; rollback Platform-controlled NCM change with verified rollback artifact; restore approved known-good configuration through maker-checker.

Mandatory guardrails: no direct LLM privileged execution; versioned action definition; tenant/resource authorization; maker-checker where required; allowlisted semantics; parameter validation; blast-radius/concurrency limits; preflight/dry-run where possible; idempotency; timeout/cancellation; immutable audit; post-action validation; rollback/fail-safe escalation; policy-driven suspension after repeated unsafe failure.

### 3.10 Platform Value / MTTR Improvement Engine — `PRD-OIV-010`

Where evidence exists, retain `failure_evidence_at`, `platform_detected_at`, `qualified_at`, `incident_created_at`, `operator_acknowledged_at`, `root_candidate_available_at`, `investigation_started_at`, `remediation_started_at`, `service_restored_at`, `incident_closed_at`.

Calculate at minimum MTTD, qualification latency, correlation latency, MTTA, time to root-cause candidate, diagnosis-to-action, service-restoration MTTR, closure/admin time, reopen rate, event->alert->incident compression, RCA accepted/false-root/inconclusive rates, recommendation adoption/success and remediation success/rollback rates.

Comparable cohorts are filterable/matched by severity, business service, incident/root-cause class, technology/domain, region/site, impact band, business-hours/after-hours and remediation class where data permits. Show median and P75/P90 where useful and disclose sample size/baseline/matching/limitations. Unmatched before/after averages are not causal proof.

### 3.11 Per-incident contribution evidence — `PRD-OIV-011`

Each qualifying incident shows where available: raw events, qualified alerts, correlated symptoms, first detection/root-candidate/ack/restoration times, whether Platform RCA preceded accepted human diagnosis, Similar Incident/Change Intelligence/Flight Recorder participation, recommendation/runbook/remediation use, actual MTTR, comparable baseline where supportable, absolute/percentage difference with methodology, and data gaps.

### 3.12 Four-layer value dashboard — `PRD-OIV-012`

**Layer 1 — Operational noise reduction:** raw events, qualified alerts, incidents, compression ratio, preserved-but-suppressed storm symptoms and avoidable false-device-down reduction where benchmarkable.

**Layer 2 — Diagnosis effectiveness:** median/P75/P90 time to root candidate, accepted RCA, false-root, inconclusive, Similar Incident usage/success, incidents where Platform RCA preceded accepted human diagnosis and Flight Recorder completeness.

**Layer 3 — Incident improvement:** MTTD/MTTA/MTTR trends, matched-cohort baseline vs Platform-assisted periods, absolute/percentage difference, reopen rate, severity/service breakdown and recommendation/remediation outcomes.

**Layer 4 — Operational/economic benefit:** evidence-supported incident minutes difference, engineer-hours from customer staffing assumptions, duplicate tickets/escalations avoided, user-hours where data exists, and SLA/outage-cost estimates only from customer-approved assumptions. Every economic panel shows calculation basis.

---

## 4. Governing architecture

Extend the existing modular Core/Sensor architecture. Do not create a monolithic AI service or a new database by default.

Logical placement:

- Integration Service / connector framework: SaaS, carrier, CI/CD, IaC and external changes;
- Graph Service: runtime dependency enrichment/temporal provenance;
- Incident/RCA modules: Flight Recorder;
- Analytics/RCA: Similar Incident scoring;
- Automation/NCM: controlled remediation;
- Reporting/SLA/analytics: Value analytics;
- Sensor Runtime: bounded packet capture where local placement is required;
- AI Gateway: explanation/summarization/recommendation within authorization/data policy.

PostgreSQL remains transactional authority; ClickHouse historical analytics; Kafka internal event backbone; Valkey ephemeral/cache. Packet binaries use bounded encrypted artifact storage with metadata references and no unapproved mandatory new storage dependency.

No dedicated vector database is selected; use existing analytical capabilities first unless later benchmark/OSS review justifies otherwise.

---

## 5. HLD subsystem contracts

- **Evidence Window Orchestrator:** trigger eligibility, bounded window, evidence fan-out, completeness, retention/legal hold references, manifest and replay; missing sources yield `PARTIAL`.
- **Similar Incident Service:** feature extraction, bounded search, scoring/version, quality/provenance filters, matching-factor explanation, feedback.
- **Change Intelligence Ingestion:** connector health/auth references, polling/webhook state, normalization, idempotency, clock/provenance and entity correlation.
- **Remediation Orchestrator:** action definitions, eligibility, preconditions, blast-radius preview, approval, idempotent execution, validation, rollback/escalation and audit; Core remains authority.
- **Value Analytics:** timestamp interpretation, formula versions, cohorts, methodology, completeness warnings and reporting.
- **Packet Evidence Worker:** bounded Sensor worker that cannot starve live polling/events/NCM/control.

---

## 6. LLD and data contracts

### 6.1 Canonical change fact

```text
change_fact
  tenant_id, change_id, source_type, source_instance_id
  event_time, ingest_time, actor_or_service_ref?
  affected_entity/service references
  change_type, revision_or_version?, result
  ticket_or_release_ref?, provenance, quality, payload_ref?, correlation_id
```

Duplicate/replayed source events shall not create duplicate causal evidence.

### 6.2 Incident evidence

```text
incident_evidence_window
  evidence_window_id, tenant_id, incident_id
  trigger_type/reference, requested_by?
  window_start, window_end
  state REQUESTED|COLLECTING|COMPLETE|PARTIAL|FAILED|EXPIRED
  completeness_score?, retention_class, legal_hold_state
  created_at, finalized_at?
```

```text
incident_evidence_ref
  evidence_ref_id, evidence_window_id, evidence_type
  source_entity_id?, source_system
  event_time_start?, event_time_end?
  storage_reference, checksum?, quality, provenance, sensitive_classification
```

### 6.3 Similarity

```text
incident_similarity
  tenant_id, incident_id, candidate_incident_id
  model_or_rule_version, score, matching_factors, quality, calculated_at
```

### 6.4 Remediation

```text
remediation_action
  action_id, tenant_id?, action_version, action_type, execution_boundary
  required_permission, approval_policy, parameter_schema
  precondition_definition, validation_definition, rollback_definition?
  blast_radius_policy, status
```

```text
remediation_execution
  execution_id, tenant_id, incident_id?, action_id/action_version
  requested_by, approved_by?, idempotency_key
  state REQUESTED|PREFLIGHT|AWAITING_APPROVAL|APPROVED|EXECUTING|VALIDATING|SUCCEEDED|FAILED|ROLLING_BACK|ROLLED_BACK|ESCALATED|CANCELLED
  target_scope, parameter_hash/redacted_parameters
  started_at?, completed_at?, result_code?, evidence_ref?, correlation_id
```

### 6.5 Value fact

```text
incident_value_fact
  tenant_id, incident_id, metric_version
  failure_evidence_at?, platform_detected_at?, qualified_at?, incident_created_at
  operator_acknowledged_at?, root_candidate_available_at?, investigation_started_at?
  remediation_started_at?, service_restored_at?, incident_closed_at?
  mttd_ms?, mtta_ms?, time_to_root_candidate_ms?, service_restoration_mttr_ms?
  event_count?, qualified_alert_count?, correlated_symptom_count?
  platform_capabilities_used, data_completeness, calculated_at
```

`value_baseline_cohort` defines tenant, version, periods, match dimensions, exclusions, minimum-sample policy and creator/time.

Versioned `saas_service_observation`, `carrier_event` and `packet_evidence_metadata` contracts are required. Raw packet payload shall never be logged or exposed through ordinary telemetry APIs.

---

## 7. End-to-end dataflows

- **DF-OIV-001 External change:** authenticated connector/webhook -> validation -> tenant/source ownership -> dedupe -> canonical change -> Kafka -> history/graph -> RCA/Change Intelligence.
- **DF-OIV-002 Flight Recorder:** incident/investigation -> bounded window -> topology/telemetry/events/changes/path/quality -> optional packet trigger -> manifest/completeness -> evidence refs -> replay/RCA. Missing source => `PARTIAL`.
- **DF-OIV-003 Similar Incident:** current incident -> bounded features -> tenant-scoped history -> quality filters -> score -> ranked candidates/matching factors -> operator/RCA -> feedback.
- **DF-OIV-004 Remediation:** recommendation/manual -> versioned action -> authorization -> preconditions -> blast-radius preview -> approval/policy -> idempotent execution -> trusted executor -> validation -> success or rollback/escalation -> audit/value fact.
- **DF-OIV-005 Value:** lifecycle/RCA/remediation/ITSM evidence -> normalized timestamps -> completeness -> metric version -> cohort matching -> comparison -> dashboard. Insufficient evidence => `INSUFFICIENT EVIDENCE`.
- **DF-OIV-006 Packet evidence:** authorized trigger -> RBAC/scope -> Sensor resource check -> bounded capture -> auto-stop -> encrypted artifact/metadata/checksum -> manifest -> TTL/legal hold.

---

## 8. UI/UX requirements

Incident workspace adds **Evidence / Flight Recorder**, **Similar Incidents**, **Changes**, **Remediation**, and **Value / Contribution** panels/tabs.

Provide a dedicated `Platform Value` / `Operational Value` dashboard implementing §3.12 with time, tenant, region/site, business service, severity, incident class, technology and assisted/remediation filters. Every improvement figure drills into cohort/sample/calculation basis.

Required states: `Loading`, `Empty`, `Partial Data`, `Stale Data`, `Permission Restricted`, `Insufficient Evidence`, `Error`, `Normal`.

---

## 9. Protocol and connector requirements

Reuse the versioned Connector framework and Credential Profiles. Webhooks require authenticated source validation/replay protection/schema/rate bounds. Polling connectors require timeouts/bounded pagination/throttling/backoff. Provider packs require OSS/licence/API review. Provider text alone is not causal proof. Packet capture uses privilege minimization and isolation from ordinary workers.

---

## 10. Security and threat requirements

Threat-model/test unauthorized packet capture/access, payload leakage, cross-tenant evidence leakage, forged/replayed changes, malicious provider responses, connector credential theft, poisoned similarity, similarity tenant leakage, remediation injection/approval bypass/replay/excessive blast radius/stale topology, rollback tampering, false value attribution, manipulated timestamps/cohorts and untrusted financial assumptions represented as audited savings.

Mitigate with tenant-scoped authorization, immutable audit, idempotency, versioned actions, bounded inputs, schema validation, redaction, artifact encryption/TTL/checksums, approval policy, blast-radius limits and data-quality labels.

---

## 11. Performance, capacity and storage requirements

Bound Flight Recorder work, packet resources, similarity searches and connector queues. Packet evidence has lower priority than live monitoring. External connectors expose lag/rate-limit/drop health. Packet payload retention is short by default. Value analytics uses rollups/aggregates. OIV workload participates in applicable performance certification and may not silently invalidate the Core footprint.

---

## 12. Test and release gates

Applicable tests cover tenant isolation, stale/partial topology, missing evidence, clock skew/out-of-order events, duplicate/replayed changes, provider timeout/throttling/malformed response, Flight Recorder partial/failure/replay, packet permission/bounds/auto-stop/encryption/TTL/resources, similarity false positives/poisoning, remediation deny/precondition/approval/timeout/cancel/partial/validation/rollback/idempotency/blast-radius, value ambiguity/insufficient or mismatched cohorts/unsupported savings prevention, formula migration, performance regression, upgrade/rollback and audit completeness.

Platform may not externally claim a generic MTTR-reduction percentage without reproducible customer/pilot evidence for a disclosed population/methodology. A customer's own measured/matched dashboard comparison is allowed when data quality is sufficient.

---

## 13. Operational/runbook requirements

Add runbooks for Flight Recorder partial/failure, stale similarity, connector lag/throttling/auth, packet worker/storage pressure, remediation stuck/partial/rollback/emergency suspension, value/cohort quality, packet retention/legal hold and support-bundle generation excluding sensitive packet payload by default.

---

## 14. Requirements traceability supplement

| Requirement ID | PRD ID | Capability | Product | Arch | HLD | LLD/Data | Dataflow | UI | Security | Quality |
|---|---|---|---|---|---|---|---|---|---|---|
| `REQ-OIV-001` | `PRD-OIV-001` | First-class SaaS monitoring | §3.1 | §4 | §5 | §6 | DF-OIV-001 | §8 | §10 | §12 |
| `REQ-OIV-002` | `PRD-OIV-002` | Triggered packet evidence | §3.2 | §4 | §5 | §6 | DF-OIV-006 | §8 | §10 | §12 |
| `REQ-OIV-003` | `PRD-OIV-003` | Cloud-native dependency discovery | §3.3 | §4 | §5 | §6 | DF-OIV-002/003 | §8 | §10 | §12 |
| `REQ-OIV-004` | `PRD-OIV-004` | External change/deployment telemetry | §3.4 | §4 | §5 | §6 | DF-OIV-001 | §8 | §10 | §12 |
| `REQ-OIV-005` | `PRD-OIV-005` | Carrier/ISP integration | §3.5 | §4 | §5 | §6 | DF-OIV-001 | §8 | §10 | §12 |
| `REQ-OIV-006` | `PRD-OIV-006` | Incident Flight Recorder | §3.6 | §4 | §5 | §6 | DF-OIV-002 | §8 | §10 | §12 |
| `REQ-OIV-007` | `PRD-OIV-007` | Similar Incident Memory | §3.7 | §4 | §5 | §6 | DF-OIV-003 | §8 | §10 | §12 |
| `REQ-OIV-008` | `PRD-OIV-008` | Cross-domain Change Intelligence | §3.8 | §4 | §5 | §6 | DF-OIV-001/002 | §8 | §10 | §12 |
| `REQ-OIV-009` | `PRD-OIV-009` | Controlled remediation | §3.9 | §4 | §5 | §6 | DF-OIV-004 | §8 | §10 | §12 |
| `REQ-OIV-010` | `PRD-OIV-010` | Platform Value / MTTR Engine | §3.10 | §4 | §5 | §6 | DF-OIV-005 | §8 | §10 | §12 |
| `REQ-OIV-011` | `PRD-OIV-011` | Per-incident contribution | §3.11 | §4 | §5 | §6 | DF-OIV-005 | §8 | §10 | §12 |
| `REQ-OIV-012` | `PRD-OIV-012` | Four-layer value dashboard | §3.12 | §4 | §5 | §6 | DF-OIV-005 | §8 | §10 | §12 |
| `REQ-OIV-013` | `PRD-OIV-013` | OIV self-observability | §3 | §4 | §5 | §6 | all | §8 | §10 | §12 |
| `REQ-OIV-014` | `PRD-OIV-014` | Role-aware APIs/UI/audit | §3 | §4 | §5 | §6 | all | §8 | §10 | §12 |

This master supplements legacy `Platform_01`, `02`, `03`, `04`, `05`, `06`, `08`, `09`, `10`, `11`, `14`, `16`, `17` and `18` for this additive capability family. Those files remain authoritative for existing scope.

---

## 15. Implementation sequence

1. external change/carrier/SaaS normalization;
2. Flight Recorder;
3. Value Engine foundation;
4. Similar Incident Memory;
5. cloud-native graph enrichment;
6. bounded controlled remediation;
7. triggered packet evidence after security/resource gates;
8. expand certified SaaS/carrier packs based on demand/API viability.

---

## 16. Definition of done

This document is specification evidence only. Completion requires implementation, tests, security, tenant isolation, data integrity, observability, resource bounds, upgrade/migration behavior, runbooks, UI, traceability and applicable performance evidence.

No MTTR-reduction or financial-value percentage becomes a product claim merely because the Value Engine can calculate a number.

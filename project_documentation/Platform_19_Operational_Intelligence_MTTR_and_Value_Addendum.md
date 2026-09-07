# Platform — Operational Intelligence, MTTR Reduction & Customer Value Master

**Document ID:** PLATFORM-INTELVALUE-001  
**Version:** 3.2.2  
**Status:** **Authoritative Mother Reference — Operational Intelligence / MTTR / Customer Value**  
**Product:** Platform  
**Change class:** Additive approved product scope; does not remove or weaken any v3.2.1 requirement

## 1. Authority and purpose

This document is the authoritative v3.2.2 specialized master for the operational-intelligence and customer-value scope approved by the product owner on 2026-09-07.

For this capability family: §3 is the BRD/PRD supplement; §4 architecture; §5 HLD; §6 LLD/data; §7 dataflow; §8 UI/UX; §9 connector/protocol; §10 security/threat; §11 performance/capacity/storage; §12 quality/release; §13 operations/runbooks; §14 formal requirements traceability.

`Platform_00_Document_Index.md` defines precedence. Where older v3.2.1 files are silent, this document adds normative v3.2.2 behavior. It may not silently override an explicit higher-precedence existing requirement.

After this branch is merged to canonical `main`, `PRD-OIV-*` and `REQ-OIV-*` requirements are implementation-authoritative. Before merge, this branch remains proposed change-control state only.

## 2. Non-negotiable principles

1. Evidence before AI; deterministic/protocol/topology/change evidence remains authoritative for causal conclusions.
2. `INCONCLUSIVE` remains a valid RCA result.
3. Triggered packet evidence is bounded diagnostics; full always-on DPI remains outside initial scope.
4. No direct LLM privileged execution.
5. Customer value must distinguish measured, derived, estimated and customer-supplied values.
6. Capability participation is not automatically causal benefit.
7. No fabricated financial savings.
8. Tenant isolation covers evidence, similarity, changes, remediation and value facts.
9. Reuse existing modular Core/Sensor and PostgreSQL/ClickHouse/Kafka/Valkey architecture unless evidence justifies change.
10. OIV self-observability is mandatory.

## 3. Formal product requirements

| PRD ID | Requirement |
|---|---|
| `PRD-OIV-001` | First-class SaaS monitoring through reusable certified SaaS Packs/connectors. |
| `PRD-OIV-002` | Privileged, bounded, incident-triggered packet evidence without full always-on DPI. |
| `PRD-OIV-003` | Cloud-native runtime service-dependency discovery from approved Kubernetes, OpenTelemetry, service-mesh and cloud evidence. |
| `PRD-OIV-004` | Normalize external deployment/change telemetry from CI/CD, Git/release, IaC, cloud, Kubernetes, DB, certificate, patch/reboot and ITSM sources. |
| `PRD-OIV-005` | Carrier/ISP operational evidence integrated into circuit/path/SLA investigations where integrations permit. |
| `PRD-OIV-006` | Incident Flight Recorder preserving bounded before/during/after evidence. |
| `PRD-OIV-007` | Similar Incident Memory surfacing relevant historical incidents/outcomes without copying prior root cause. |
| `PRD-OIV-008` | Cross-domain Change Intelligence answering `What changed before this incident?`. |
| `PRD-OIV-009` | Controlled remediation: diagnose -> recommend -> precondition -> impact preview -> authorize/approve -> execute -> validate -> rollback/escalate. |
| `PRD-OIV-010` | Evidence-backed Platform Value / MTTR Improvement Engine. |
| `PRD-OIV-011` | Per-incident Platform contribution evidence distinguishing participation from causal benefit. |
| `PRD-OIV-012` | Four-layer customer-value dashboard: noise reduction, diagnosis effectiveness, incident improvement, operational/economic benefit. |
| `PRD-OIV-013` | OIV self-observability for health, lag, completeness, capacity and failure. |
| `PRD-OIV-014` | Role-aware, tenant-scoped, audited OIV APIs/UI with bounded queries and safe errors. |

### 3.1 SaaS monitoring

A SaaS Pack is a versioned provider connector/monitoring package. Where APIs permit, support service status, tenant/subscription health, API errors/latency/throttling/quotas, auth failures, operational KPIs, audit/change events, synthetics, business-service dependencies, provider-incident correlation and connector/rate-limit health. Provider API/entitlement/data limits must be explicit.

### 3.2 Triggered packet evidence

Capture may be initiated by an authorized operator, severe qualified incident policy, anomaly/path/routing trigger, approved runbook, remediation precondition or support diagnostic. Enforce source/interface/target/filter scope where possible, maximum duration/bytes/packets/rate, privileged RBAC, tenant isolation, encryption, TTL/legal hold, audit, payload minimization/redaction, resource telemetry and automatic stop at bounds. Prefer header/metadata retention when sufficient.

### 3.3 Cloud-native dependencies

Enrich temporal graph relationships such as ingress->service, service->workload, workload->pod, pod->node, service/workload->database/cache/queue/external API, cloud LB->target/service, namespace/application ownership, service-mesh calls, OTel service/resource relationships and supported security-policy relationships. Every inferred edge stores provenance/confidence/history and may not silently override customer-defined authoritative dependencies.

### 3.4 External changes

Normalize CI/CD, Git/release, IaC, Kubernetes/GitOps, cloud control-plane/network/security, LB, DB deployment/schema, certificate, patch/reboot, app/config release and ITSM changes. Retain event/ingest time, source, actor/service identity where available, affected entity/service references, change type, revision, result, ticket/release ref, provenance and evidence ref.

### 3.5 Carrier/ISP

Where APIs permit, ingest provider outage/status, circuit/service IDs, maintenance, provider ticket/incident state, outage/restore time, SLA and escalation metadata. Provider status is supporting evidence, not causal proof; correlate with observed path/routing/reachability evidence.

### 3.6 Incident Flight Recorder

For qualifying incidents/investigations, pin/capture a bounded window of topology, interface/link state/errors, routing/redundancy transitions, path/synthetic results, Flow summaries, logs/traps/events, metrics/anomalies, NCM/external changes, telemetry quality, infra/DB/storage/container state, business-service state and authorized packet evidence. Preserve event time vs ingest time, provenance, quality and correlation IDs. UI provides replayable timeline and historical graph snapshot.

### 3.7 Similar Incident Memory

Index resolved incidents using root entity/class, temporal subgraph, symptoms/events, protocol/change signatures, business services, site/technology/vendor context and remediation outcome. Show score, matching factors, accepted outcome, remediation/runbook, time-to-root and restoration time. Advisory only; stale/low-quality/contradictory evidence reduces trust.

### 3.8 Change Intelligence

Unify NCM/device, firewall/LB, cloud, CI/CD, IaC, Kubernetes, DB, certificate/secret metadata, patch/reboot, Platform and ITSM changes. Investigations distinguish temporal correlation from proven causation.

### 3.9 Controlled remediation

Prioritize bounded/reversible actions such as retry monitors/synthetics, restart/reconnect authorized Platform components, move/retry Sensor jobs under fencing, run approved diagnostics, rollback Platform-controlled NCM changes with verified artifacts and restore approved known-good config. Mandatory guardrails: versioned actions, tenant/resource authorization, maker-checker where required, allowlisted semantics, parameter validation, blast-radius/concurrency limits, preflight, idempotency, timeout/cancel, immutable audit, validation and rollback/fail-safe escalation. No direct LLM privileged execution.

### 3.10 Platform Value / MTTR Engine

Where evidence exists, retain failure, detection, qualification, incident creation, acknowledgement, root-candidate, investigation, remediation, restoration and closure timestamps with provenance/quality. Calculate MTTD, qualification/correlation latency, MTTA, time-to-root, diagnosis-to-action, service-restoration MTTR, closure time, reopen rate, event->alert->incident compression, RCA accepted/false-root/inconclusive rates, recommendation outcomes and remediation success/rollback.

Comparable cohorts should match/filter by severity, service, incident/root class, technology/domain, region/site, impact band, business hours and remediation class when data permits. Show median and P75/P90 where useful. Disclose baseline/sample/matching limitations. Unmatched before/after averages are not causal proof.

### 3.11 Per-incident contribution

Show raw events, qualified alerts, correlated symptoms, detection/root/ack/restoration times, whether Platform RCA preceded accepted human diagnosis, Similar Incident/Change Intelligence/Flight Recorder participation, recommendation/runbook/remediation usage, actual MTTR, comparable baseline where supportable, absolute/percentage difference with methodology and data gaps.

### 3.12 Four-layer value dashboard

1. **Noise reduction:** raw events, qualified alerts, incidents, compression and false-device-down reduction where benchmarkable.
2. **Diagnosis effectiveness:** median/P75/P90 time-to-root, accepted RCA, false-root, inconclusive, Similar Incident performance, RCA-before-human and Flight Recorder completeness.
3. **Incident improvement:** MTTD/MTTA/MTTR trends, matched baseline comparisons, absolute/percentage difference, reopen rate and remediation outcomes.
4. **Operational/economic benefit:** incident minutes, engineer-hours from customer staffing assumptions, ticket/escalation reduction, user-hours where data exists, SLA/outage-cost estimates only from customer-approved assumptions. Show calculation basis.

## 4. Governing architecture

Extend existing modular Core/Sensor architecture. Logical placement: Integration Service for SaaS/carrier/external changes; Graph Service for runtime dependencies; Incident/RCA for Flight Recorder; Analytics/RCA for Similar Incident scoring; Automation/NCM for remediation; Reporting/SLA/analytics for Value Engine; Sensor Runtime for bounded packet capture; AI Gateway for explanation/recommendation only.

PostgreSQL remains transactional authority, ClickHouse historical analytics, Kafka internal event backbone, Valkey cache/ephemeral. Packet binaries use bounded encrypted artifact storage with metadata refs; do not add an unapproved mandatory storage product. No dedicated vector database is selected without benchmark/OSS justification.

## 5. HLD subsystem contracts

- Evidence Window Orchestrator: trigger eligibility, bounded evidence window, fan-out, completeness, retention/legal-hold refs, manifest, replay; missing sources => `PARTIAL`.
- Similar Incident Service: bounded feature extraction/search, score/version, quality/provenance filters, matching-factor explanation, feedback.
- Change Intelligence Ingestion: connector state/health/auth refs, normalization, idempotency, clock/provenance, entity correlation.
- Remediation Orchestrator: action definitions, eligibility/preconditions, blast-radius preview, approval, idempotent execution, validation, rollback/escalation, audit; Core remains authority.
- Value Analytics: timestamp interpretation, formula versions, cohorts, methodology, quality warnings, reports.
- Packet Evidence Worker: bounded Sensor worker isolated from live monitoring resources.

## 6. LLD/data contracts

Versioned contracts are required for `change_fact`, `incident_evidence_window`, `incident_evidence_ref`, `incident_similarity`, `remediation_action`, `remediation_execution`, `incident_value_fact`, `value_baseline_cohort`, `saas_service_observation`, `carrier_event`, `packet_evidence_metadata`. All preserve tenant scope/provenance/quality as applicable. Remediation requests use idempotency and explicit state transitions. Raw packet payload is never logged or exposed through ordinary telemetry APIs.

## 7. Dataflows

- `DF-OIV-001`: authenticated external connector/webhook -> validation/tenant ownership/dedupe -> canonical change -> Kafka -> history/graph -> RCA/Change Intelligence.
- `DF-OIV-002`: incident/investigation -> bounded evidence window -> topology/telemetry/events/changes/path/quality -> optional packet trigger -> manifest/completeness -> replay/RCA; missing source => `PARTIAL`.
- `DF-OIV-003`: current incident -> bounded features -> tenant-scoped history -> quality filters -> score -> ranked candidates -> feedback.
- `DF-OIV-004`: recommendation/manual action -> versioned action -> authorization -> preconditions -> blast radius -> approval -> idempotent execution -> validation -> success or rollback/escalation -> audit/value fact.
- `DF-OIV-005`: incident/RCA/remediation/ITSM evidence -> timestamps/completeness -> metric version -> cohort -> comparison -> dashboard; insufficient data => `INSUFFICIENT EVIDENCE`.
- `DF-OIV-006`: authorized packet trigger -> RBAC/scope/resource check -> bounded capture -> auto-stop -> encrypted artifact/metadata/checksum -> TTL/legal hold.

## 8. UI/UX

Incident workspace adds **Evidence / Flight Recorder**, **Similar Incidents**, **Changes**, **Remediation**, and **Value / Contribution**. Dedicated `Platform Value` / `Operational Value` dashboard implements §3.12 with time/tenant/site/service/severity/class/domain/remediation filters. Every improvement figure drills into cohort/sample/calculation basis. Required states include `Partial Data`, `Stale Data`, `Permission Restricted`, `Insufficient Evidence`, `Error`, and `Normal`.

## 9. Connector/protocol rules

Reuse Connector framework and Credential Profiles. Webhooks require source authentication, replay/schema/rate safeguards. Pollers require bounded pagination/timeouts/throttling/backoff. Provider packs require API/licence/OSS review. Provider text alone is not causal proof. Packet capture uses least privilege and isolated bounded workers.

## 10. Security/threat requirements

Cover unauthorized packet capture/access, payload leakage, cross-tenant evidence, forged/replayed changes, malicious provider responses, credential theft, poisoned similarity, similarity-based tenant leakage, remediation injection/approval bypass/replay/excessive blast radius/stale topology, rollback tampering, false value attribution, manipulated timestamps/cohorts and untrusted financial assumptions. Mitigate using tenant authorization, immutable audit, idempotency, bounded inputs, schema validation, redaction, encryption/TTL/checksums, approval policy, blast-radius limits and data-quality labels.

## 11. Performance/capacity/storage

Bound Flight Recorder work, packet resources, similarity searches and connector queues. Packet evidence is lower priority than live monitoring. Expose connector lag/rate/drop health. Packet retention is short by default. Value analytics uses aggregates/rollups. OIV workload is part of applicable performance certification and may not silently invalidate Core resource targets.

## 12. Test/release gates

Applicable tests cover tenant isolation, stale/partial topology/evidence, clock skew/out-of-order data, duplicate/replayed changes, provider timeout/throttling/malformed response, Flight Recorder partial/replay, packet permissions/bounds/auto-stop/encryption/TTL/resources, similarity false positives/poisoning, remediation denied/precondition/approval/timeout/cancel/partial/validation/rollback/idempotency/blast radius, ambiguous value timestamps, insufficient/mismatched cohorts, unsupported savings prevention, formula migration, performance, upgrade/rollback and audit completeness.

Platform may not externally claim a generic MTTR-reduction percentage without reproducible customer/pilot evidence for a disclosed population/methodology. Customer-specific measured/matched dashboard comparisons are allowed when data quality is sufficient.

## 13. Operational/runbooks

Add runbooks for Flight Recorder partial/failure, stale similarity, external connector lag/auth/throttling, packet worker/storage, remediation stuck/partial/rollback/emergency suspension, value/cohort quality, packet retention/legal hold and support-bundle behavior excluding sensitive payload by default.

## 14. Requirements traceability supplement

| REQ | PRD | Capability |
|---|---|---|
| `REQ-OIV-001` | `PRD-OIV-001` | First-class SaaS monitoring |
| `REQ-OIV-002` | `PRD-OIV-002` | Triggered packet evidence |
| `REQ-OIV-003` | `PRD-OIV-003` | Cloud-native dependency discovery |
| `REQ-OIV-004` | `PRD-OIV-004` | External change/deployment telemetry |
| `REQ-OIV-005` | `PRD-OIV-005` | Carrier/ISP operational integration |
| `REQ-OIV-006` | `PRD-OIV-006` | Incident Flight Recorder |
| `REQ-OIV-007` | `PRD-OIV-007` | Similar Incident Memory |
| `REQ-OIV-008` | `PRD-OIV-008` | Cross-domain Change Intelligence |
| `REQ-OIV-009` | `PRD-OIV-009` | Controlled remediation |
| `REQ-OIV-010` | `PRD-OIV-010` | Platform Value / MTTR Engine |
| `REQ-OIV-011` | `PRD-OIV-011` | Per-incident contribution |
| `REQ-OIV-012` | `PRD-OIV-012` | Four-layer customer value dashboard |
| `REQ-OIV-013` | `PRD-OIV-013` | OIV self-observability |
| `REQ-OIV-014` | `PRD-OIV-014` | Role-aware APIs/UI/audit |

For all rows, governing logical layers are §§3–13 of this master as applicable. This master supplements `Platform_01`, `02`, `03`, `04`, `05`, `06`, `08`, `09`, `10`, `11`, `14`, `16`, `17` and `18`; those files remain authoritative for existing scope.

## 15. Implementation sequence

1. external change/carrier/SaaS normalization;
2. Flight Recorder;
3. Value Engine foundation;
4. Similar Incident Memory;
5. cloud-native graph enrichment;
6. bounded controlled remediation;
7. triggered packet evidence after security/resource gates;
8. expand SaaS/carrier packs based on demand/API viability.

## 16. Definition of done

This document is specification evidence only. Completion requires implementation, tests, security, tenant isolation, data integrity, observability, resource bounds, upgrade/migration behavior, runbooks, UI, traceability and applicable performance evidence.

No MTTR-reduction or financial-value percentage becomes a product claim merely because the Value Engine can calculate a number.

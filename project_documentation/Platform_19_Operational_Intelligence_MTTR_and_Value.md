# Platform — Operational Intelligence, MTTR Reduction & Customer Value Master

**Document ID:** PLATFORM-INTELVALUE-001  
**Version:** 3.2.2  
**Status:** **Authoritative Mother Reference — Cross-Cutting Operational Intelligence & Value Contract**  
**Product:** Platform  
**Change basis:** USER-FIXED scope expansion approved 2026-09-07

## 1. Authority and purpose

This document is an additive authoritative mother reference for the operational-intelligence, MTTR-reduction and customer-value capabilities approved after Platform v3.2.1. It supplements the BRD, PRD, Architecture, HLD, LLD, Dataflow and specialized masters without weakening any existing requirement.

For requirements introduced here, this document is the normative source until a later consolidation moves the same requirement text into older mother files. Where a conflict exists with a higher-precedence mother document, the normal precedence in `Platform_00_Document_Index.md` applies and implementation must stop until change control reconciles the conflict.

This document is specification authority only. It does not claim that the capabilities are implemented, validated, benchmarked, customer-proven or GA-ready.

## 2. Added V1 capability scope

Platform shall add the following to V1 scope:

| ID | Capability | Governing requirement |
|---|---|---|
| OI-SAA-001 | First-class SaaS monitoring | Certified/reusable SaaS Packs and connectors for operational health, KPIs, dependencies and change evidence. |
| OI-PKT-001 | Triggered packet evidence | Bounded incident diagnostic capture; full always-on DPI remains excluded. |
| OI-GRAPH-001 | Cloud-native service dependency discovery | Runtime-derived service/workload/dependency edges with provenance/confidence/history. |
| OI-CHG-001 | External change/deployment telemetry | Normalize CI/CD, IaC, cloud, K8s, DB, certificate, patch and ITSM changes into RCA evidence. |
| OI-ISP-001 | Carrier/ISP operational integration | Enrich circuits/path/SLA with provider outage, maintenance and ticket evidence. |
| OI-FR-001 | Incident Flight Recorder | Preserve bounded pre/during/post-incident evidence and historical graph/timeline replay. |
| OI-SIM-001 | Similar Incident Memory | Find comparable historical incidents and surface accepted outcomes without copying causality. |
| OI-CI-001 | Cross-domain Change Intelligence | Unified `What changed?` timeline beyond NCM. |
| OI-REM-001 | Controlled remediation | Policy/RBAC/approval-gated, bounded, validated remediation with rollback/fail-safe behavior. |
| OI-VAL-001 | Platform Value / MTTR Improvement Engine | Evidence-backed measurement of MTTD/MTTA/MTTR, time-to-RCA and comparable-cohort improvement. |
| OI-VAL-002 | Per-incident Platform contribution | Show exactly where Platform contributed to detection, diagnosis and restoration. |
| OI-VAL-003 | Four-layer customer value dashboard | Noise reduction, diagnosis effectiveness, incident improvement and evidence-supported operational/economic value. |

### 2.1 Existing scope boundaries preserved

- Full always-on DPI / packet-level Layer-7 observability remains outside initial product scope. `OI-PKT-001` is bounded diagnostic evidence only.
- Full APM product positioning remains outside initial scope. Cloud-native dependency discovery may use existing Kubernetes/OpenTelemetry/service-mesh evidence where available without turning Platform into a full APM product.
- Unsupervised autonomous closed-loop remediation and direct LLM-to-device privileged execution remain prohibited. Controlled remediation is explicitly authorized only through the guarded workflow in §12.
- Exact initial SaaS vendors/carriers are not invented here; pack priority depends on customer demand, provider API viability, entitlement, licensing and security review.

## 3. Governing principles

1. Each capability must improve detection, diagnosis, restoration, noise reduction or evidence of customer value; breadth alone is not sufficient.
2. Deterministic/protocol/topology/change evidence remains authoritative for causal RCA. ML/GNN/LLM may rank, summarize, explain and recommend but must not fabricate causality.
3. `INCONCLUSIVE` remains a valid RCA outcome.
4. Every value metric shall distinguish measured facts, derived values, inference and customer-supplied estimates.
5. Platform shall not claim MTTR reduction from an unmatched before/after average.
6. Financial savings shall be shown only when customer-approved cost inputs exist and shall remain labelled estimates unless independently audited.
7. Controlled remediation shall be policy-gated, resource-scoped, auditable, bounded, idempotent where applicable and validated after execution.
8. Evidence capture and similarity mechanisms must preserve tenant isolation, provenance, timestamps, quality/confidence and legal/retention controls.

## 4. First-class SaaS monitoring

### OI-SAA-001 — SaaS Pack model

Platform shall provide reusable, versioned SaaS Packs so common SaaS services can be monitored without forcing every customer to build a custom API Studio connector.

Where provider APIs permit, a pack should support:

- provider/service health;
- customer tenant/account/subscription health;
- API availability, latency, errors and throttling;
- authentication/session failure signals;
- quota/capacity/licence-consumption indicators;
- service-specific operational KPIs;
- provider/customer change and audit events where permitted;
- synthetic validation from selected Sensors/Agents;
- dependency edges into Platform business services;
- provider-incident correlation;
- connector health, token expiry and rate-limit diagnostics.

Provider API limitations, paid API entitlements, rate limits, data residency and redistribution terms shall be explicit. Platform shall not claim telemetry a provider does not expose.

## 5. Triggered packet evidence

### OI-PKT-001 — Bounded incident evidence

Platform shall support on-demand or incident-triggered packet evidence for diagnostics without introducing unbounded always-on packet capture.

Permitted trigger sources:

- authorized operator;
- severe qualified incident policy;
- anomaly/path/routing trigger;
- approved diagnostic runbook;
- remediation precondition;
- support diagnostic workflow.

Mandatory controls:

- explicit target/interface/host/filter scope where technically possible;
- strict duration, byte, packet and rate bounds;
- privileged RBAC/resource scope and tenant isolation;
- encryption in transit/at rest;
- retention TTL and legal-hold integration;
- immutable audit of actor, request, scope, reason and evidence access;
- payload minimization/redaction policy;
- headers/metadata preferred when sufficient;
- resource telemetry, CPU/disk/network guardrails and automatic abort on safety limits;
- no capture secrets or customer traffic sent to an LLM by default.

Raw packet payload shall not be stored indiscriminately in normal telemetry tables.

## 6. Cloud-native service dependency discovery

### OI-GRAPH-001 — Runtime dependency enrichment

Platform shall enrich the temporal operational graph using approved runtime evidence such as Kubernetes APIs, OpenTelemetry and service-mesh telemetry where available.

Target relationships include:

- ingress/gateway -> service;
- service -> workload;
- workload -> pod;
- pod -> node;
- service/workload -> database/cache/queue/external API;
- cloud load balancer -> target/service;
- namespace/application ownership;
- network/security-policy relationship where direct evidence exists.

Every derived edge shall retain source/provenance, confidence/quality, `first_seen`, `last_seen`, `valid_from`/`valid_to` and discovery mechanism. Runtime inference shall not silently overwrite customer-defined authoritative dependencies.

Introduction of eBPF or any additional privileged collector requires separate architecture, OSS, security, performance and kernel-compatibility approval; this requirement does not silently select eBPF.

## 7. External change/deployment telemetry

### OI-CHG-001 — Normalized external change facts

Platform shall ingest and normalize operational changes into the existing change/RCA model from approved sources including, where available:

- CI/CD deployments and release systems;
- Git/revision release metadata;
- Terraform/IaC apply events;
- Kubernetes/Helm/Argo-style rollouts;
- cloud network/security/control-plane changes;
- load-balancer/firewall changes;
- database deployment/schema-change metadata;
- certificate rotation;
- infrastructure patch/reboot events;
- feature/configuration release metadata;
- ITSM change records.

Normalized change facts shall include tenant, source, event/change time, ingest time, affected entity/service references, actor/service identity where available, change type, version/revision, result, ticket/release reference, quality/provenance and evidence pointer.

Change proximity is evidence, not automatic proof of causality.

## 8. Carrier/ISP operational integration

### OI-ISP-001 — Provider operational enrichment

Platform shall enrich circuit, path and SLA models using provider operational evidence when supported by provider APIs/customer integrations.

Supported evidence may include:

- provider outage/status events;
- circuit/service IDs;
- maintenance notices;
- provider ticket state;
- outage start/restore timestamps;
- SLA facts;
- escalation/contact metadata;
- ticket create/update synchronization.

Provider status remains one evidence source and shall be reconciled with observed routing, reachability, path and telemetry evidence.

## 9. Incident Flight Recorder

### OI-FR-001 — Evidence preservation

For configured qualified incidents or authorized manual investigations, Platform shall preserve a bounded evidence window around the incident so transient evidence remains available after recovery.

Where available, the window shall contain:

- historical topology/graph state before, during and after the incident;
- link/interface state, errors/discards/optics;
- BGP/OSPF/IS-IS/BFD/HSRP/VRRP and other relevant protocol/redundancy changes;
- path/traceroute and synthetic results;
- Flow summaries;
- syslog/traps/events;
- relevant metrics/anomaly facts;
- NCM/configuration and external change facts;
- Sensor/telemetry-quality state;
- server/VM/container/database/storage state;
- business-service state;
- triggered packet evidence when authorized;
- actions, acknowledgements, recommendations and remediation evidence.

Evidence shall retain event time, ingest time, provenance, quality, sequence/event IDs and correlation IDs.

The incident UI shall provide a replayable timeline and historical graph snapshot with explicit `complete`, `partial`, `stale` or `missing` evidence states.

## 10. Similar Incident Memory

### OI-SIM-001 — Historical precedent

Platform shall index resolved incidents and accepted outcomes so current incidents can surface relevant historical precedents.

Similarity may use:

- accepted root entity/root-cause class;
- temporal topology/subgraph pattern;
- symptoms/alerts/events;
- protocol/change signatures;
- affected business services;
- anomaly features;
- site/technology/vendor/Device Pack context;
- remediation/runbook outcome.

For each candidate, show similarity score and principal matching factors, accepted outcome, remediation/runbook, time-to-root-cause, restoration time, outcome/success state and evidence link.

Similar Incident Memory is advisory. It shall never silently copy a previous root cause into the current incident, and its score shall not be presented as causal proof.

## 11. Cross-domain Change Intelligence

### OI-CI-001 — Unified change timeline

Platform shall provide a unified Change Intelligence view combining:

- NCM changes;
- firewall/load-balancer changes;
- cloud control-plane changes;
- CI/CD deployments;
- IaC changes;
- Kubernetes rollouts;
- database deployment metadata;
- certificate/secret-rotation metadata where safely recordable;
- infrastructure patch/reboot/maintenance;
- Platform policy/credential/monitoring changes;
- ITSM change records.

RCA and investigations shall answer `What changed immediately before this incident?` while distinguishing temporal correlation, supporting evidence, contradiction and proven causality.

## 12. Controlled remediation

### OI-REM-001 — Mandatory workflow

Controlled remediation is now V1 scope for bounded, approved action classes using:

`diagnose -> recommend -> precondition -> impact/blast-radius preview -> policy/approval gate -> execute -> validate -> rollback/escalate`

Initial priority actions shall be safe/reversible where practical, for example:

- retry/re-run a monitor or synthetic test;
- restart/reconnect a Platform Agent or bounded Platform component where authorized;
- move/retry a Sensor job under placement/fencing policy;
- execute an approved diagnostic/runbook action;
- rollback a Platform-controlled NCM change when a verified rollback artifact exists;
- restore an approved known-good configuration through maker-checker governance.

Mandatory guardrails:

- no direct LLM privileged execution;
- explicit action identity/version and immutable audit;
- tenant/resource-scoped authorization;
- maker-checker when policy requires;
- allowlisted action/command semantics;
- schema/parameter validation and secret redaction;
- concurrency and blast-radius limits;
- dry-run/preflight where practical;
- idempotency or replay protection;
- timeout/cancellation;
- post-action validation against service/alert/telemetry state;
- rollback/fail-safe escalation;
- circuit breaker/automatic suspension after repeated unsafe failure.

## 13. Platform Value / MTTR Improvement Engine

### OI-VAL-001 — Incident timestamps

Where evidence exists, Platform shall retain at least:

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

Derived timestamps shall retain provenance/confidence and shall not silently replace missing source timestamps.

### OI-VAL-002 — Derived metrics

Calculate at minimum:

- MTTD;
- qualification latency;
- correlation latency;
- MTTA;
- time-to-root-cause candidate;
- diagnosis-to-action time;
- service-restoration MTTR;
- closure/admin time;
- reopen rate;
- event/alert compression ratio;
- RCA accepted/false-root/inconclusive rate;
- recommendation adoption/success rate;
- remediation success/rollback rate.

### OI-VAL-003 — Comparable cohorts

Platform shall not present unrelated incidents as a causal before/after comparison. Cohorts shall be matchable/filterable by severity, business service, incident/root-cause class, technology/domain, region/site, impact band, business-hours/after-hours and remediation class where sufficient data exists.

Show median and, where sample size permits, P75/P90. Disclose baseline period, sample size, filters/matching dimensions, exclusions and insufficient-sample limitations.

## 14. Per-incident Platform contribution

### OI-VAL-004 — Evidence-backed contribution summary

Each qualifying incident shall show:

- raw events observed;
- alerts qualified;
- child symptoms correlated;
- first detection time;
- root-candidate time;
- acknowledgement time;
- service-restoration time;
- whether Platform RCA preceded the accepted human diagnosis;
- whether Similar Incident Memory was consulted;
- whether Change Intelligence/Flight Recorder materially contributed evidence;
- whether a recommendation/runbook was used;
- whether controlled remediation was used and validated;
- actual MTTR;
- matched historical/customer baseline MTTR where statistically supportable;
- absolute minutes saved and percentage improvement with methodology label;
- data gaps and attribution limitations.

The UI shall distinguish `capability participated` from `capability proven to have caused the improvement`.

## 15. Customer Value dashboard — four layers

### Layer 1 — Operational noise reduction

Show raw events, qualified alerts, incidents, compression ratio, duplicate/storm symptoms suppressed while preserving evidence, and avoidable false-device-down reduction where benchmarkable.

### Layer 2 — Diagnosis effectiveness

Show median/P75/P90 time-to-root-cause, accepted RCA rate, false-root rate, inconclusive rate, Similar Incident Memory usage/outcome, percentage of incidents where Platform RCA preceded accepted human diagnosis, and Flight Recorder evidence completeness.

### Layer 3 — Incident improvement

Show MTTD/MTTA/MTTR trends, matched-cohort baseline versus Platform-assisted period, absolute incident minutes saved, percentage improvement, reopen rate, severity/business-service breakdown, recommendation success and controlled-remediation outcome.

### Layer 4 — Operational/economic benefit

Show only evidence-supported values such as incident minutes avoided, engineer-hours avoided/redirected, duplicate tickets/escalations avoided, user-hours of disruption avoided when customer impact data is supplied, and SLA penalty/outage-cost estimates only when customer-approved cost assumptions exist.

All economic values shall expose assumptions and shall not be represented as audited savings unless independently verified.

## 16. Data ownership and contracts

PostgreSQL owns transactional/control definitions and workflow state including connector definitions, evidence-window metadata, approved baselines, remediation approval/execution state and value methodology definitions.

ClickHouse owns high-volume historical evidence/value facts where appropriate. Existing Kafka event rules and replay/idempotency requirements continue to apply.

Versioned contracts are required for at least:

- external `change_fact` extensions;
- `incident_evidence_window`;
- `incident_evidence_ref`;
- `incident_similarity`;
- `remediation_action`;
- `remediation_execution`;
- `incident_value_fact`;
- `value_baseline_cohort`;
- `carrier_event`;
- `saas_service_observation`;
- `packet_evidence_metadata`.

All records are tenant-scoped and preserve event time, ingest time, provenance and quality/confidence where relevant.

## 17. API and UI contract

Provide versioned, tenant-scoped, RBAC-enforced APIs and role-aware UI for:

- Flight Recorder timeline/replay;
- Similar Incidents;
- Change Intelligence;
- remediation preview/approval/execution/validation/rollback status;
- incident value attribution;
- customer Value/MTTR dashboard and export;
- SaaS Pack configuration/status;
- carrier/circuit enrichment;
- privileged packet-evidence request/status/access.

APIs shall apply query/resource limits, correlation IDs, audit and stable error codes. Long-running actions shall be asynchronous jobs with inspectable progress/failure state.

## 18. Self-observability

Platform shall monitor:

- evidence capture success/failure/partial state and completeness;
- similarity index freshness/build failures;
- change/SaaS/carrier ingestion lag, API throttling and errors;
- remediation queue/action/validation/rollback state;
- packet-capture CPU/disk/network usage and aborts;
- value-metric data completeness, cohort quality and calculation failures.

These states feed Platform Operations and cannot fail silently.

## 19. Security and abuse cases

Threat-model and test at least:

- unauthorized packet capture/access;
- sensitive payload leakage;
- malicious/malformed connector/provider responses;
- forged/replayed change facts;
- remediation command/parameter injection;
- approval bypass or replay;
- excessive remediation blast radius;
- poisoned Similar Incident Memory;
- false value/financial attribution;
- cross-tenant evidence/value leakage;
- resource exhaustion through capture, similarity, connector or value queries.

## 20. Quality and release gates

Before any capability is represented as production-ready, tests shall cover as applicable:

- tenant isolation and RBAC;
- stale/partial topology and missing telemetry;
- clock skew/out-of-order evidence;
- alert storms and duplicate/replayed changes;
- packet-capture privilege/bounds/retention/resource protection;
- similarity false-positive/adversarial cases;
- remediation denied/partial/timeout/replay/validation/rollback paths;
- provider API throttling/outage/malformed response;
- ambiguous/missing value timestamps;
- insufficient cohort size and comparison bias;
- prevention of unsupported MTTR/financial claims;
- Flight Recorder replay correctness;
- migration/upgrade/backward-compatibility behavior;
- workload/resource regression and storage impact.

No MTTR percentage or economic saving becomes a contractual/public product claim solely because internal benchmark or customer data exists; external claims require appropriate commercial/legal review.

## 21. Implementation sequencing

Integrate these capabilities into existing modules rather than create one monolithic AI service:

1. normalized external change, SaaS and carrier connector contracts;
2. Incident Flight Recorder evidence contracts/storage/replay;
3. Similar Incident Memory;
4. cloud-native runtime graph enrichment;
5. Platform Value timestamp, cohort and attribution engine plus dashboard;
6. controlled remediation for bounded action classes;
7. triggered packet evidence after security/resource controls pass;
8. expand certified SaaS/carrier packs according to customer demand and provider API viability.

Existing Alert Qualification, Incident Correlation, Graph, RCA, Integration, NCM, Query, Audit, Storage Lifecycle and Platform Operations services shall be extended where they own the relevant state; a new process boundary requires the normal modularity justification.

## 22. Definition of done

A requirement in this master is done only when implementation, deterministic and negative tests, security, tenant isolation, observability, failure handling, migration/upgrade impact, data retention, performance/resource validation, operational runbooks, UI behavior, documentation and traceability are complete.

The presence or merge of this document is specification evidence only and must never be represented as feature implementation, MTTR reduction proof or GA evidence.

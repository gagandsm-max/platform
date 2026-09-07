# Platform — Operational Intelligence, MTTR Reduction & Value Addendum

**Document ID:** PLATFORM-INTELVALUE-001  
**Version:** 3.2.1-addendum-1  
**Status:** Additive Product/Engineering Requirement — subordinate to Platform Mother Reference v3.2.1 until formal consolidation  
**Product:** Platform

## 1. Purpose

This addendum turns the approved MTTR/value enhancements into implementable product requirements without weakening any existing Platform v3.2.1 requirement. If any statement conflicts with the Mother Reference, the Mother Reference wins and change control must reconcile the conflict before implementation.

## 2. Added capability scope

Platform shall add:

- first-class SaaS monitoring through certified SaaS Packs/connectors;
- triggered packet evidence for bounded incident diagnostics, without making always-on full DPI a V1 requirement;
- cloud-native runtime service-dependency discovery;
- external change/deployment telemetry;
- carrier/ISP operational integration;
- Incident Flight Recorder;
- Similar Incident Memory;
- cross-domain Change Intelligence beyond NCM;
- controlled remediation with guardrails, approval, validation and rollback;
- Platform Value / MTTR Improvement measurement;
- per-incident evidence showing where Platform contributed to faster diagnosis/restoration;
- a four-layer customer value dashboard covering operational noise reduction, diagnosis effectiveness, incident improvement and evidence-supported operational/economic benefit.

## 3. Governing principles

1. Breadth is not the objective by itself. Each capability must materially improve detection, diagnosis, restoration, operational noise or evidence of customer value.
2. Deterministic/protocol/topology/change evidence remains authoritative for causal conclusions. ML/GNN/LLM may rank, summarize, explain and recommend but must not fabricate causality.
3. `INCONCLUSIVE` remains a valid RCA outcome.
4. Every value claim must distinguish measured, inferred and estimated values.
5. Platform must not claim MTTR reduction from a simple unmatched before/after average.
6. Financial savings are shown only when customer-approved cost assumptions exist and must be labelled as estimates unless independently audited.
7. Controlled remediation must be policy-gated, resource-scoped, auditable, bounded and validated after execution.

## 4. First-class SaaS monitoring

### 4.1 Product requirement

Platform shall provide a reusable SaaS Pack model so high-value SaaS services can be monitored without requiring every customer to build a custom API Studio connector.

### 4.2 Standard SaaS Pack capabilities

Where provider APIs permit, a pack should support:

- provider/service health;
- customer tenant/account/subscription health;
- API latency, errors and throttling;
- authentication/session failure signals;
- quota/capacity/license-consumption indicators;
- service-specific operational KPIs;
- audit/change events;
- synthetic validation from selected Sensors/Agents;
- dependency edges into Platform business services;
- provider-incident correlation;
- connector health/rate-limit diagnostics.

### 4.3 Scope control

Provider API limitations, commercial API entitlement, rate limits and redistribution terms must be explicit. Platform must not pretend to expose telemetry the SaaS provider does not make available.

## 5. Triggered packet evidence

### 5.1 Decision

Platform shall add bounded, incident-triggered packet evidence. This does not change the current decision excluding full always-on DPI from initial scope.

### 5.2 Trigger sources

Capture may be requested by:

- an authorized operator;
- a severe qualified incident policy;
- an anomaly/path/routing trigger;
- an approved diagnostic runbook;
- a remediation precondition;
- support diagnostics.

### 5.3 Mandatory safety controls

- explicit interface/host/BPF-like filter scope where technically supported;
- strict duration, bytes, packets and rate bounds;
- no unbounded background capture;
- privileged RBAC/resource scope;
- tenant isolation;
- encryption at rest/in transit;
- retention TTL and legal-hold behavior;
- audit of request, actor, scope, reason and access;
- sensitive payload minimization/redaction policy;
- capture resource telemetry and self-protection.

Derived headers/metadata should be retained instead of payload when that is sufficient for diagnosis.

## 6. Cloud-native service dependency discovery

Platform shall enrich the temporal operational graph with runtime-derived relationships from approved sources such as Kubernetes APIs, OpenTelemetry and service-mesh telemetry where available.

Examples:

- ingress/gateway -> service;
- service -> workload;
- workload -> pod;
- pod -> node;
- service/workload -> database/cache/queue/external API;
- cloud load balancer -> target/service;
- namespace/application ownership;
- network/security-policy relationships when evidence exists.

Every edge must preserve provenance, confidence, `first_seen`, `last_seen`, `valid_from`/`valid_to` and discovery source. Runtime inference may not silently overwrite customer-defined authoritative dependencies.

## 7. External change/deployment telemetry

Platform shall normalize external operational changes into the existing change/RCA model.

Sources may include approved integrations for:

- CI/CD deployment systems;
- Git-based release metadata;
- Terraform/IaC apply events;
- Kubernetes/Helm/Argo-style rollouts;
- cloud network/security/control-plane changes;
- load-balancer changes;
- database deployment/schema-change metadata;
- certificate rotation;
- patch/reboot events;
- feature/configuration release metadata;
- ITSM change records.

A normalized change fact should contain tenant, source, timestamp, affected entity/service references, actor/service identity where available, change type, version/revision, result, ticket/release reference and evidence pointer.

## 8. Carrier/ISP operational integration

Platform shall enrich its circuit/path/SLA model with provider operational evidence when supported by provider APIs or customer integrations.

Support may include:

- provider outage/status events;
- circuit IDs and service IDs;
- maintenance notices;
- provider incident/ticket status;
- outage start/restore timestamps;
- SLA facts;
- provider escalation/contact metadata;
- ticket create/update synchronization.

Provider status is an evidence source, not causal proof; Platform must reconcile it with observed path, routing and reachability evidence.

## 9. Incident Flight Recorder

### 9.1 Purpose

Preserve short-lived evidence that commonly disappears before an operator starts investigating.

### 9.2 Evidence window

For configured qualified incidents or an authorized manual investigation, Platform shall pin or collect a bounded evidence window around the incident containing, where available:

- topology/graph state before, during and after the incident;
- interface/link state/errors;
- routing/adjacency/redundancy changes including BGP, OSPF, IS-IS, BFD, HSRP/VRRP where relevant;
- path/traceroute and synthetic results;
- Flow summaries;
- syslog/traps/events;
- relevant metrics/anomalies;
- NCM/configuration and external change facts;
- Sensor/telemetry-quality state;
- server/VM/container/database/storage state;
- business-service state;
- triggered packet evidence where policy authorizes it.

Event time, ingest time, provenance, evidence quality and correlation IDs must be retained.

### 9.3 User experience

The incident shall expose a replayable timeline and historical graph snapshot so the operator can reconstruct what happened even after the environment recovers.

## 10. Similar Incident Memory

Platform shall index resolved incidents and their accepted outcomes so current incidents can surface relevant historical precedents.

Similarity may use:

- root entity and root-cause class;
- temporal topology/subgraph pattern;
- symptoms/alerts/events;
- protocol and change signatures;
- affected business services;
- anomaly features;
- site/technology/vendor/device-pack context;
- remediation/runbook outcome.

For each candidate show:

- similarity score plus major matching factors;
- accepted root cause/outcome;
- remediation/runbook used;
- time to root cause;
- service-restoration time;
- recommendation/remediation outcome;
- evidence link.

Similar Incident Memory is advisory; it must never silently copy a previous incident's root cause into the current incident.

## 11. Cross-domain Change Intelligence

Create a unified Change Intelligence view combining:

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

RCA and investigations must be able to answer `What changed immediately before this incident?` while clearly distinguishing temporal correlation from proven causation.

## 12. Controlled remediation

### 12.1 Mandatory workflow

`diagnose -> recommend -> precondition check -> impact/blast-radius preview -> policy/approval gate -> execute -> validate -> rollback/escalate`

### 12.2 Initial bounded action classes

Prioritize safe/reversible actions such as:

- retry/re-run a monitor or synthetic test;
- restart/reconnect a Platform Agent or bounded Platform component where authorized;
- move/retry a Sensor job under placement/fencing policy;
- execute an approved diagnostic runbook;
- rollback a Platform-controlled NCM change when a verified rollback artifact exists;
- restore an approved known-good configuration through the NCM maker-checker workflow.

### 12.3 Guardrails

- no direct LLM privileged execution;
- explicit action identity and immutable audit;
- tenant/resource-scoped authorization;
- maker-checker when policy requires it;
- allowlisted action/command semantics;
- parameter validation;
- concurrency and blast-radius limits;
- dry-run/preflight where possible;
- idempotency;
- timeout/cancellation;
- post-action validation;
- rollback/fail-safe escalation;
- automatic suspension after repeated unsafe failure.

## 13. Platform Value / MTTR Improvement Engine

### 13.1 Purpose

Quantify operational improvement using evidence-backed timestamps and comparable incident cohorts.

### 13.2 Incident timestamps

Where evidence exists, retain:

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

Derived timestamps must retain provenance/confidence.

### 13.3 Metrics

Calculate at minimum:

- MTTD;
- qualification latency;
- correlation latency;
- MTTA;
- time to root-cause candidate;
- diagnosis-to-action time;
- service-restoration MTTR;
- closure/admin time;
- incident reopen rate;
- event/alert compression ratio;
- RCA accepted/false-root/inconclusive rate;
- recommendation adoption/success rate;
- remediation success/rollback rate.

### 13.4 Comparable cohorts

Platform must not present unrelated incidents as a causal before/after comparison. When sufficient data exists, cohorts should be matched/filterable by:

- severity;
- business service;
- incident/root-cause class;
- technology/domain;
- region/site;
- impact/blast-radius band;
- business-hours vs after-hours;
- automated vs manual remediation class.

Show median plus P75/P90 where useful, not mean alone. Disclose baseline period, sample sizes, matching dimensions and insufficient-sample limitations.

## 14. Per-incident Platform contribution

Each qualifying incident shall show an evidence-backed contribution summary including:

- raw events observed;
- alerts qualified;
- symptoms correlated into the incident;
- first detection time;
- root-candidate time;
- acknowledgement time;
- service-restoration time;
- whether Platform's RCA preceded the accepted human diagnosis;
- whether Similar Incident Memory was used;
- whether Change Intelligence or Flight Recorder materially contributed;
- whether a recommendation/runbook was used;
- whether controlled remediation was used and validated;
- actual MTTR;
- matched historical/customer baseline MTTR where statistically supportable;
- absolute minutes saved and percentage improvement with methodology label;
- data gaps/limitations.

Platform must distinguish `capability participated` from `capability proven to have caused the improvement`.

## 15. Customer Value dashboard — four layers

### 15.1 Layer 1 — Operational noise reduction

Show:

- raw events;
- qualified alerts;
- incidents;
- compression ratio;
- duplicate/storm symptoms suppressed while preserving evidence;
- avoidable false-device-down reduction where benchmarkable.

### 15.2 Layer 2 — Diagnosis effectiveness

Show:

- median/P75/P90 time to root-cause candidate;
- accepted RCA rate;
- false-root rate;
- inconclusive rate;
- Similar Incident Memory usage/success;
- percentage of incidents where Platform RCA preceded accepted human diagnosis;
- Flight Recorder usage and evidence completeness.

### 15.3 Layer 3 — Incident improvement

Show:

- MTTD/MTTA/MTTR trend;
- matched-cohort baseline vs Platform-assisted period;
- absolute incident minutes saved;
- percentage improvement;
- reopen rate;
- severity/business-service breakdown;
- recommendation/remediation success.

### 15.4 Layer 4 — Operational/economic benefit

Only show evidence-supported values such as:

- incident minutes avoided;
- engineer-hours avoided/redirected;
- duplicate tickets/escalations avoided;
- user-hours of disruption avoided when customer supplies affected-user data;
- SLA penalties or outage-cost estimate only when customer supplies approved cost assumptions.

Economic values must display their assumptions and must not be represented as audited savings unless independently verified.

## 16. Data ownership additions

Use PostgreSQL for transactional/control definitions and workflow state such as baseline definitions, remediation approval/execution metadata and connector configuration.

Use ClickHouse for high-volume historical evidence/value facts where appropriate.

Add versioned contracts for at least:

- normalized external `change_fact` source extensions;
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

Raw packet payloads must not be placed indiscriminately in normal telemetry tables.

## 17. API/UI requirements

Provide versioned APIs and role-aware UI for:

- Flight Recorder timeline/replay;
- Similar Incidents;
- Change Intelligence timeline/filtering;
- remediation preview/approval/execution/validation;
- incident value attribution;
- customer value/MTTR dashboards and export;
- SaaS Pack configuration/status;
- carrier/circuit operational enrichment;
- privileged packet-evidence request/status/access.

All APIs enforce tenant scope, RBAC, pagination/query limits, correlation IDs and audit.

## 18. Self-observability

Platform shall monitor these capabilities themselves:

- evidence capture success/failure/partial state;
- evidence window completeness;
- similarity index freshness;
- external change ingestion lag/errors;
- remediation queue/action state;
- SaaS/carrier connector health and throttling;
- packet-capture resource usage;
- value-metric data completeness and cohort quality.

## 19. Security/threat requirements

Threat-model at minimum:

- unauthorized packet capture/access;
- sensitive payload leakage;
- malicious connector/provider responses;
- forged change facts;
- remediation command injection;
- approval bypass;
- replayed remediation requests;
- excessive blast radius;
- poisoned Similar Incident Memory;
- false value/financial attribution;
- cross-tenant incident evidence leakage.

## 20. Test and release gates

Before a capability may be represented as production-ready, test at minimum:

- tenant isolation;
- stale/partial topology;
- missing telemetry;
- clock skew and out-of-order evidence;
- alert storms;
- duplicate/replayed change events;
- packet-capture privilege/limit/retention enforcement;
- similarity false-positive/adversarial cases;
- remediation denied/partial/timeout/rollback paths;
- provider API throttling/outage/malformed response;
- ambiguous/missing value timestamps;
- insufficient cohort sample size;
- prevention of unsupported savings claims;
- Flight Recorder replay correctness;
- workload/resource regression.

## 21. Implementation sequence

Integrate these capabilities with the existing programme rather than creating one monolithic AI service:

1. external change/carrier/SaaS connector normalization;
2. Flight Recorder evidence contracts and UI timeline;
3. Similar Incident Memory;
4. cloud-native runtime graph enrichment;
5. Platform Value timestamp/attribution model and dashboard;
6. controlled remediation for bounded action classes;
7. triggered packet evidence after security/resource controls pass;
8. expand certified SaaS/carrier packs according to customer demand/provider API viability.

## 22. Definition of done

The presence of this document is specification evidence only. Each capability remains incomplete until implementation, tests, security, tenant isolation, observability, migration/upgrade behavior, performance/resource validation, operational runbooks, UI behavior and traceability are complete.

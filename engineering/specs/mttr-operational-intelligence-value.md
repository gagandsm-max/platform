# Platform Operational Intelligence, MTTR Reduction & Value Enhancements

**Status:** IMPLEMENTATION DESIGN ADDENDUM — subordinate to Platform Mother Reference v3.2.1 until merged into the Mother Reference

This file defines additive product requirements requested for implementation. It must not override any conflicting Mother Reference requirement. Engineering must update the governing BRD/PRD/Architecture/HLD/LLD/Dataflow/UI/RTM documents before declaring any capability production-complete.

## 1. Scope

Add the following capabilities to Platform:

1. First-class SaaS monitoring.
2. Triggered packet evidence.
3. Cloud-native service dependency discovery.
4. External change/deployment telemetry.
5. Carrier/ISP operational integration.
6. Incident Flight Recorder.
7. Similar Incident Memory.
8. Cross-domain Change Intelligence beyond NCM.
9. Controlled remediation with authorization, guardrails, validation and rollback.
10. Platform Value / MTTR Improvement measurement.
11. Per-incident attribution showing where Platform saved time.
12. Executive/customer dashboard showing four layers of value: operational-noise reduction, diagnosis effectiveness, incident improvement, and economic/operational benefit.

## 2. Product principles

- These capabilities must reduce operational noise, diagnosis time, restoration time, or provide credible evidence of business value.
- Deterministic evidence remains authoritative for causal conclusions. ML/GNN/LLM may rank, summarize, explain or recommend, but must not fabricate causality.
- Every value claim must be reproducible from timestamped incident/audit/ITSM evidence and clearly distinguish measured, inferred and estimated values.
- Platform must never claim MTTR reduction from a simple before/after average when incident populations are not comparable.
- Financial savings are shown only when customer-supplied or contractually defined cost inputs exist.
- Controlled remediation must always enforce RBAC/resource scope, approval policy, blast-radius limits, preconditions, idempotency, audit, post-action validation and rollback where technically possible.

## 3. First-class SaaS monitoring

### 3.1 Goal

Provide certified, product-owned monitoring packs for high-value SaaS platforms so customers do not need to build every connector manually in API Studio.

### 3.2 V1 implementation model

Start with an extensible SaaS pack framework and a prioritized certified set. Each SaaS pack may expose, where the provider API permits:

- service availability and provider status;
- tenant/account/subscription health;
- API latency/errors/throttling;
- authentication/session failures;
- capacity/quota/license-consumption signals;
- critical service-specific operational KPIs;
- audit/change events;
- dependency relationships into Platform business services;
- synthetic validation from selected Sensors/Agents;
- provider-incident correlation.

Provider-specific functionality remains bounded by documented API permissions, rate limits, licensing and data availability.

## 4. Triggered packet evidence

### 4.1 Decision

Do not introduce always-on full DPI as part of this enhancement. Add bounded, triggered diagnostic packet evidence for supported Sensors.

### 4.2 Trigger sources

Capture may be invoked by:

- authorized manual investigation;
- policy-driven severe incident;
- qualified anomaly;
- path/routing state change;
- runbook/remediation precondition;
- support diagnostic workflow.

### 4.3 Safety requirements

- strict duration/size/rate limits;
- explicit interface/host/filter scope;
- encryption at rest and in transit;
- tenant isolation;
- sensitive payload minimization/redaction policy;
- privileged RBAC and audit;
- retention TTL and legal-hold handling;
- no unbounded capture;
- no silent background capture outside configured policy.

Derived metadata should be preferred over retaining payload when sufficient for diagnosis.

## 5. Cloud-native service dependency discovery

Augment the canonical operational graph with runtime-derived cloud-native relationships from Kubernetes APIs, OpenTelemetry, service mesh data and other approved sources where available.

The graph may include:

- ingress/gateway -> service;
- service -> workload;
- workload -> pod;
- pod -> node;
- service/workload -> database/cache/queue/external API;
- namespace/application ownership;
- network path/security-policy relationships where evidence exists;
- cloud load balancer -> target/service relationships.

Every discovered edge must retain provenance, confidence, first/last observed and temporal validity. Runtime inference must never silently overwrite customer-defined authoritative dependencies.

## 6. External change/deployment telemetry

Platform must ingest normalized change facts from approved integrations such as CI/CD, IaC, cloud control planes, Kubernetes deployment systems and enterprise change-management systems.

Examples include:

- application release/deployment;
- container/image rollout;
- Kubernetes manifest/Helm change;
- Terraform/IaC apply;
- cloud network/security change;
- load-balancer change;
- database deployment/schema change metadata;
- certificate rotation;
- patch/reboot event;
- feature/configuration release metadata.

Each change fact should include tenant, affected entities/services, source, actor/service identity where available, timestamp, change type, version/revision, result, ticket/release reference and evidence link/identifier.

## 7. Carrier/ISP operational integration

Add circuit/provider operational enrichment around Platform's existing circuit/SLA/path monitoring.

Where provider APIs or approved connectors permit, support:

- provider outage/status events;
- circuit/ticket identifiers;
- maintenance notices;
- provider incident status;
- escalation/contact metadata;
- provider SLA facts;
- outage start/restore timestamps;
- ticket creation/update synchronization.

Provider reports are evidence sources, not unquestioned root-cause proof. Platform must reconcile provider information with observed telemetry/path evidence.

## 8. Incident Flight Recorder

### 8.1 Goal

Preserve the short-lived evidence that operators normally lose before investigation begins.

### 8.2 Trigger

A Flight Recorder snapshot begins for configured severe/qualified incidents or an authorized manual investigation.

### 8.3 Evidence window

The recorder must collect or pin references for a configurable bounded window around the incident, including where available:

- topology/graph state before, during and after the incident;
- interface/link state and errors;
- BGP/OSPF/IS-IS/BFD/HSRP/VRRP and other relevant protocol changes;
- traceroute/path changes and synthetic results;
- Flow summaries;
- syslog/traps/events;
- relevant metrics/anomalies;
- configuration and external change facts;
- Sensor/telemetry-quality state;
- server/VM/container/database/storage state;
- business-service state;
- triggered packet evidence when policy authorizes it.

The recorder must preserve event-time and ingest-time distinction, provenance, correlation IDs and evidence quality.

### 8.4 Output

An incident exposes a replayable evidence timeline and graph snapshot so an operator can reconstruct what changed even when the environment has recovered.

## 9. Similar Incident Memory

Platform must index resolved incidents using normalized evidence and outcomes so operators can see relevant historical precedents.

Similarity may use:

- root entity/root-cause class;
- topology subgraph pattern;
- symptoms/alerts/events;
- protocol/change signatures;
- affected business services;
- temporal propagation pattern;
- anomaly features;
- environment/site/vendor/device-pack context.

For each similar incident, show:

- similarity score with explanation;
- verified/accepted root cause;
- remediation/runbook used;
- time to root cause;
- restoration time;
- whether the recommendation worked;
- links to evidence.

Similarity is advisory and must not automatically copy a prior root cause into a new incident.

## 10. Cross-domain Change Intelligence

Create one normalized Change Intelligence model combining:

- NCM/configuration changes;
- cloud control-plane changes;
- CI/CD deployments;
- IaC changes;
- Kubernetes rollouts;
- database deployment metadata;
- firewall/load-balancer policy changes;
- certificate/secret rotations where metadata may be safely recorded;
- infrastructure maintenance/patch/reboot;
- Platform policy/credential/monitoring changes;
- ITSM change records.

RCA must be able to answer: "What changed immediately before this incident?" while distinguishing correlation from causation.

## 11. Controlled remediation

### 11.1 Workflow

`diagnose -> recommend -> precondition check -> impact/blast-radius preview -> policy/approval gate -> execute -> validate -> rollback/escalate`

### 11.2 Initial safe action classes

Prioritize bounded Platform-controlled or diagnostically reversible actions, for example:

- retry/re-run monitoring or synthetic checks;
- restart/reconnect a Platform Agent or bounded Platform component where authorized;
- move/retry a Sensor job according to placement/fencing policy;
- execute an approved diagnostic runbook;
- rollback a Platform-controlled NCM change when a verified rollback artifact exists;
- restore a previously approved known-good configuration through NCM maker-checker workflow.

### 11.3 Guardrails

- no LLM-direct privileged execution;
- explicit action identity and immutable audit;
- tenant/resource-scoped authorization;
- maker-checker where required;
- command/action allowlist and parameter validation;
- concurrency and blast-radius limits;
- dry-run/preflight where possible;
- idempotency key;
- timeout/cancellation;
- post-action validation;
- rollback or fail-safe escalation plan;
- automatic suspension after repeated/unsafe failure.

## 12. Platform Value / MTTR Improvement Engine

### 12.1 Purpose

Provide evidence-backed measurement of operational improvement attributable to Platform-assisted workflows without making unsupported causal or financial claims.

### 12.2 Required incident timestamps

Where evidence exists, record:

- `failure_evidence_at` — earliest credible event-time for the operational failure;
- `platform_detected_at`;
- `qualified_at`;
- `incident_created_at`;
- `operator_acknowledged_at`;
- `root_candidate_available_at`;
- `investigation_started_at`;
- `remediation_started_at`;
- `service_restored_at`;
- `incident_closed_at`.

Source/provenance and confidence must be retained for derived timestamps.

### 12.3 Derived metrics

At minimum:

- mean/median time to detect;
- qualification latency;
- correlation latency;
- MTTA;
- time to root-cause candidate;
- diagnosis-to-action time;
- service-restoration MTTR;
- administrative closure time;
- incident reopen rate;
- alert/event compression ratio;
- false-root and inconclusive rates;
- recommendation adoption/success rate;
- controlled-remediation success/rollback rate.

### 12.4 Comparable-cohort methodology

Platform must not calculate claimed improvement by comparing unrelated incident populations.

When adequate data exists, compare cohorts using dimensions such as:

- severity;
- business service;
- incident/root-cause class;
- technology/domain;
- region/site;
- impact/blast-radius band;
- business-hours vs after-hours;
- automated vs manual remediation class.

Prefer median, P75 and P90 in addition to mean.

The UI must disclose the baseline period, sample sizes, matching/filter dimensions and confidence/limitations.

### 12.5 Attribution categories

For each incident, Platform should record which capabilities materially participated:

- qualification;
- correlation;
- anomaly detection;
- graph/path analysis;
- deterministic RCA;
- GNN scoring;
- Similar Incident Memory;
- Change Intelligence;
- Incident Flight Recorder;
- AI explanation;
- recommendation/runbook;
- controlled remediation.

This does not by itself prove causation; the dashboard must distinguish participation from measured improvement.

## 13. Per-incident "Platform helped" evidence

Every qualifying incident should expose a value summary such as:

- raw events observed;
- alerts qualified;
- child symptoms correlated into the incident;
- first detection time;
- root-candidate time;
- operator acknowledgement time;
- service restoration time;
- whether Platform produced the accepted root candidate before the human diagnosis;
- whether Similar Incident Memory was used;
- whether a recommendation/runbook was used;
- whether controlled remediation was used and validated;
- actual MTTR;
- matched historical/customer baseline MTTR if statistically supportable;
- absolute and percentage improvement with methodology label;
- known limitations/data gaps.

## 14. Customer value dashboard — four layers

### Layer 1 — Operational noise reduction

Show:

- raw events;
- qualified alerts;
- incidents;
- alert/event compression ratio;
- duplicate/storm alerts suppressed while preserving evidence;
- avoidable false device-down reduction where benchmarkable.

### Layer 2 — Diagnosis effectiveness

Show:

- median/P75/P90 time to root-cause candidate;
- accepted RCA rate;
- false-root rate;
- inconclusive rate;
- Similar Incident Memory usage and success;
- percentage of incidents where Platform RCA preceded accepted human diagnosis;
- Flight Recorder usage/evidence completeness.

### Layer 3 — Incident improvement

Show:

- MTTD/MTTA/MTTR trend;
- matched-cohort baseline vs Platform-assisted period;
- absolute minutes saved and percentage improvement;
- reopen rate;
- severity/business-service breakdown;
- recommendation/remediation success.

### Layer 4 — Operational/economic benefit

Show only evidence-supported values, such as:

- incident minutes avoided;
- engineer-hours avoided/redirected;
- duplicate tickets/escalations avoided;
- user-hours of disruption avoided where customer provides affected-user data;
- SLA penalties or outage-cost estimate only when customer provides approved cost assumptions.

Economic estimates must show the customer-supplied assumptions and never be represented as audited savings.

## 15. Data model additions

Introduce versioned contracts for at least:

- `change_fact` extensions / normalized external change source;
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

High-volume historical evidence belongs in ClickHouse where appropriate; transactional workflow/approval/baseline definitions belong in PostgreSQL. Raw packet data must not be placed indiscriminately into normal telemetry tables.

## 16. API and UI requirements

Provide versioned APIs and UI workflows for:

- Flight Recorder evidence timeline/replay;
- Similar Incidents;
- Change Intelligence timeline/filtering;
- remediation preview/approval/execution/validation;
- incident value attribution;
- MTTR/value dashboards and export;
- SaaS pack status/configuration;
- carrier/circuit operational enrichment;
- packet-evidence request/status/access with privileged authorization.

All APIs enforce tenant scope, RBAC, pagination/query bounds and audit requirements.

## 17. Observability and audit

Platform must monitor these features themselves, including:

- evidence-capture success/failure and dropped/partial evidence;
- similarity-index freshness;
- change-ingestion lag/errors;
- remediation queue/action state;
- SaaS/carrier connector health/rate-limit state;
- packet-capture resource consumption;
- value-metric data completeness and cohort sample quality.

## 18. Quality gates

Before GA claims for these capabilities, test at minimum:

- cross-tenant isolation;
- stale/partial topology and telemetry;
- clock skew and out-of-order evidence;
- high alert storms;
- change-event duplication/replay;
- packet-capture limits, privilege and retention;
- similarity false-positive cases;
- remediation denied/failed/partial/timeout/rollback paths;
- malformed or malicious connector responses;
- provider API throttling/outage;
- value metric missing/ambiguous timestamps;
- cohort sample-size insufficiency;
- prevention of unsupported savings claims;
- incident replay correctness;
- resource/performance impact at reference workloads.

## 19. Implementation sequence

These enhancements should be integrated with the existing programme rather than built as one monolith:

1. Normalize external change facts and carrier/SaaS connector framework extensions.
2. Add Flight Recorder evidence contracts and timeline.
3. Add Similar Incident Memory over resolved incident evidence.
4. Add cloud-native runtime graph enrichment.
5. Add Platform Value timestamp/attribution model and dashboards.
6. Add controlled remediation for bounded action classes.
7. Add triggered packet evidence after security/resource controls are validated.
8. Expand certified SaaS/carrier packs based on customer demand and provider API viability.

## 20. Definition of done

No capability in this addendum is complete because the requirement exists here. Completion requires implementation, security, tenant isolation, tests, observability, upgrade/migration behavior, operational runbooks, documentation/traceability, performance evidence and customer-visible behavior consistent with the Mother Reference.

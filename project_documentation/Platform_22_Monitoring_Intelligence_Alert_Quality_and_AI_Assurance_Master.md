# Platform — Monitoring Intelligence, Alert Quality and AI Assurance Master

**Document ID:** PLATFORM-MIA-001  
**Version:** 3.2.2  
**Status:** **Authoritative Mother Reference — Monitoring Intelligence / Alert Quality / AI Assurance (after merge)**  
**Product:** Platform  
**Change class:** Additive v3.2.2 product/engineering requirements; no runtime implementation, model selection, dependency selection or readiness claim

---

## 1. Authority and purpose

This document is the consolidated specialized master for monitoring intelligence, alert quality, evidence qualification, RCA confidence, AI/ML assurance and safe Incident Copilot behavior. `Platform_21_Enterprise_Gap_Closure_Master.md` owns the separate enterprise-gap capability family and must not be duplicated here.

`Platform_00_Document_Index.md` defines precedence. This master supplements existing v3.2.2 requirements and must not silently override a higher-precedence explicit requirement. Contradictions require change control before implementation.

Nothing in this document selects a new model, model provider, vector database, browser/runtime, OSS dependency, database, mandatory microservice, commercial SKU, price, compliance certification or vendor commitment. Capability statements are requirements/targets, not implementation or production-readiness claims.

---

## 2. Non-negotiable intelligence principles

1. **Evidence precedes assertion.** Monitoring state must flow through `evidence → qualification → correlation → causality → assistance → controlled action → measured value`.
2. **Telemetry quality is a first-class trust gate.** Collector/Sensor/Agent/path/time/schema degradation must be distinguished from monitored-environment failure.
3. **Severity and confidence are independent.** A high-impact incident may still have low causal confidence.
4. **Deterministic logic remains authoritative where safer.** GNN/ML/LLM outputs augment evidence and investigation; they are not causal proof.
5. **`INCONCLUSIVE` is valid.** Platform must prefer uncertainty over unsupported causal claims.
6. **No direct generative privileged execution.** LLM output cannot directly execute shell, SQL, NCM, remediation, credential, network or infrastructure actions.
7. **Core monitoring does not depend on AI availability.** Deterministic collection, alerting and incident functions continue when model/provider/internet access is unavailable.
8. **No silent learning across tenants.** Tenant data, operator feedback and incident evidence remain tenant scoped unless an explicitly approved privacy/security/governance process authorizes another use.
9. **No invented universal thresholds.** Alert evidence and release gates are class-specific and policy/evidence driven.
10. **AI claims require provenance.** Every causal or recommended-action claim must trace to authoritative evidence and disclose uncertainty/contradictions.

---

## 3. Formal product requirements

| PRD ID | Release | Requirement |
|---|---|---|
| `PRD-MIA-001` | P0 | Platform shall implement class-specific **Alert Evidence Contracts** defining sufficient evidence, contradictory evidence, telemetry-quality prerequisites, suppression/degradation behavior and recovery evidence for each critical alert class. |
| `PRD-MIA-002` | P0 | Before asserting monitored operational failure, Platform shall evaluate telemetry trust/coverage and distinguish monitored-object failure from collection, Sensor, Agent, path, time-quality or schema degradation. |
| `PRD-MIA-003` | P0 | Alert/incident severity and evidence/causal confidence shall be represented independently and shall not be conflated in UI, API, automation or AI output. |
| `PRD-MIA-004` | P0 | Alert policies shall support governed lifecycle `DRAFT → BACKTEST → SHADOW → ACTIVE`, with versioning, approval, rollback and evidence before promotion. |
| `PRD-MIA-005` | P0 | Production alert-policy promotion shall require measurable class-appropriate false-positive/false-negative and missed-detection evidence; no universal numeric threshold is fixed by this master. |
| `PRD-MIA-006` | P1 | Platform may perform bounded **Active Evidence Acquisition** to reduce uncertainty, subject to RBAC, safety, rate/budget limits, tenant policy, audit and protocol-specific controls. |
| `PRD-MIA-007` | P0 | Alert/incident evidence shall expose explicit **Evidence Coverage** showing expected signals, available signals, missing/stale/degraded signals and resulting confidence impact. |
| `PRD-MIA-008` | P0 | Operational state shall support semantic states including `MONITORING_DEGRADED`, `REDUNDANCY_LOST`, `SLA_DEGRADED`, `EXPECTED_STATE`, `UNKNOWN` and `STALE` where applicable instead of collapsing all abnormality into DOWN/CRITICAL. |
| `PRD-MIA-009` | P1 | Change/convergence intelligence shall correlate bounded configuration, deployment, routing, policy and infrastructure changes with telemetry while preventing temporal proximity from being presented as causal proof. |
| `PRD-MIA-010` | P1 | Structured operator feedback shall capture useful/not-useful, confirmed/rejected cause, suppression reason and remediation outcome without silently mutating production policy or model behavior. |
| `PRD-MIA-011` | P0 | AI/ML/GNN intelligence shall have a reproducible evaluation harness with versioned datasets/fixtures, provenance, expected outcomes, regressions, uncertainty cases and deterministic baseline comparison. |
| `PRD-MIA-012` | P0 | AI-produced factual, causal and remediation claims shall be evidence-bound; unsupported claims shall be rejected or emitted as `INCONCLUSIVE`/hypothesis with missing evidence explicitly identified. |
| `PRD-MIA-013` | P0 | Intelligence trust shall use explicit levels `CONFIRMED`, `HIGH`, `MEDIUM`, `LOW`, `INCONCLUSIVE`, distinct from operational severity. |
| `PRD-MIA-014` | P0 | LLM/tool integrations shall defend against prompt/instruction injection and use a deterministic tool-use firewall enforcing allowed tools, parameters, tenant/RBAC scope, preconditions, approval and audit. |
| `PRD-MIA-015` | P0 | Learning/evaluation inputs shall remain tenant scoped; Platform shall not silently train across tenants or send tenant data to an external provider outside configured data-egress policy. |
| `PRD-MIA-016` | P0 | AI/model/provider degradation shall not disable deterministic monitoring, alerting, incident evidence, topology, runbooks or safe manual workflows. |
| `PRD-MIA-017` | P1 | Platform may calculate peer/cohort baselines using explicitly governed comparable populations and shall disclose cohort scope, sample sufficiency and fallback when the cohort is weak. |
| `PRD-MIA-018` | P1 | Platform may perform multivariate anomaly detection across related telemetry, but anomaly score alone shall not assert causality or operational failure. |
| `PRD-MIA-019` | P1 | Investigation planning may rank next evidence by expected information gain, cost, safety and latency, while preserving deterministic policy limits and operator control. |
| `PRD-MIA-020` | P1 | RCA confidence shall evolve as evidence arrives and shall retain the evidence/contradiction history that caused confidence changes. |
| `PRD-MIA-021` | P1 | RCA shall support explicit causal contradiction testing and downgrade/reject hypotheses when expected effects are absent or contradictory evidence is stronger. |
| `PRD-MIA-022` | P1 | Production models/rules shall expose data/model/prompt/policy drift indicators and alert when evaluation quality materially departs from certified baselines. |
| `PRD-MIA-023` | P1 | New model/rule versions shall support champion/challenger or equivalent controlled comparison before promotion to authoritative assistance. |
| `PRD-MIA-024` | V1.x | Predictive operations may forecast capacity, degradation or failure risk only where backtesting/calibration supports the stated horizon and confidence; speculative predictions must not be represented as certainty. |
| `PRD-MIA-025` | P1 | Incident Copilot shall operate over authoritative Platform evidence and structure answers as `Observed → Impact → Likely cause → Confidence → Evidence → Contradictions → Missing evidence → Next validation → Recommended action`. |
| `PRD-MIA-026` | P1 | AI access shall support a provider abstraction capable of hosted, private-hosted, local/on-prem and no-LLM modes, subject to approved dependency/security/performance decisions. |
| `PRD-MIA-027` | P0 | AI data-egress policy shall govern what tenant data classes may leave the deployment, provider/region destinations, retention, masking, opt-out and audit. |
| `PRD-MIA-028` | P1 | Platform shall model shared-service dependencies including DNS, DHCP, NTP/PTP, RADIUS, TACACS+, LDAP and AD so common dependency failure can explain downstream symptoms without duplicating specialized products. |
| `PRD-MIA-029` | P1 | PKI assurance shall extend beyond expiry to chain/path validity, revocation/validation health where available, key/certificate mismatch, trust-store/configuration drift and service-impact evidence. |
| `PRD-MIA-030` | P1 | Facilities/power evidence may be ingested to explain infrastructure incidents, but Platform shall not expand into BMS/DCIM control-plane scope by implication. |
| `PRD-MIA-031` | P0 | AI/analytics workloads shall have explicit CPU/memory/GPU/concurrency/queue/time budgets and fail-safe degradation so intelligence cannot starve core monitoring. |
| `PRD-MIA-032` | P0 | AI/ML operations shall expose model/prompt/policy/version provenance, latency, error/timeout/fallback rates, token/compute usage where relevant, cache use, grounding/evidence coverage and decision audit. |

---

## 4. Alert Evidence Contracts

An Alert Evidence Contract is versioned policy for a specific alert class. Each contract shall define: monitored subject and scope; required primary evidence; corroborating evidence where applicable; contradictory evidence; telemetry freshness/quality prerequisites; maintenance/expected-state handling; severity logic; confidence logic; suppression/dedup/storm behavior; recovery evidence; active-validation options; and audit/version metadata.

The following classes require explicit contracts before production activation:

- **Device Down:** distinguish target unreachability from SNMP/credential/collector/path failure; use available independent reachability/path/neighbor evidence appropriate to device class.
- **Link Down:** use interface administrative/operational state, peer/neighbor/topology evidence, carrier/underlay evidence where available, and expected maintenance state.
- **CPU:** distinguish sustained resource pressure from collection gaps, transient bursts and platform/OS-specific semantics; tie severity to duration/service impact policy rather than one sample.
- **Memory:** account for platform-specific memory/cache semantics, sustained pressure, reclaim/swap/OOM/process evidence where available.
- **Service/Database:** distinguish process/listener/query/synthetic failure from host/network/dependency failure and from monitoring credential/query failure.
- **Routing adjacency:** use protocol neighbor state, hold/transition evidence, interface/path evidence and convergence context; expected maintenance/convergence may map to `EXPECTED_STATE` or `SLA_DEGRADED` rather than immediate device failure.
- **SD-WAN:** separate underlay circuit, overlay/tunnel, controller/fabric, SLA-class and application-path degradation using the canonical Platform_21 SD-WAN model.
- **Sensor failure:** distinguish Sensor process/node failure, Core reachability loss, queue/backpressure, credential/cert failure and partial collector degradation; monitored targets are not automatically DOWN when their Sensor is unavailable.
- **Event storms:** detect volume/cardinality/duplicate bursts, preserve root evidence, suppress fan-out safely and expose storm state rather than silently dropping incidents.
- **TLS/certificate:** distinguish certificate expiry, invalid chain, hostname/SAN mismatch, trust-store drift, revocation/validation failure and underlying service unreachability.

No contract may encode a universal “two signals always required” rule. Required evidence depends on alert class and available authoritative signals.

---

## 5. Telemetry quality, evidence coverage and semantic state

Before operational assertion, the evaluation path shall score whether expected evidence is present, fresh, schema-valid, time-aligned and produced by healthy collectors. Missing or degraded evidence lowers confidence and may change the state to `MONITORING_DEGRADED`, `UNKNOWN` or `STALE` instead of asserting target failure.

**Evidence Coverage** shall expose enough machine-readable information to answer: what evidence was expected; what arrived; what is missing/stale/degraded; whether active validation ran; which contradictions exist; and how the evidence affected confidence.

Semantic states are additive to existing alert/incident lifecycle. They describe operational meaning, not merely workflow status. `REDUNDANCY_LOST` means service may still be available but resilience is reduced. `SLA_DEGRADED` means service/path remains up but violates a governed objective. `EXPECTED_STATE` records a known/authorized condition. `UNKNOWN` means insufficient trustworthy evidence. `STALE` means state depends on evidence outside freshness policy.

---

## 6. Alert-policy lifecycle and false-alert quality gates

Alert policy changes follow `DRAFT → BACKTEST → SHADOW → ACTIVE`.

- **DRAFT:** authoring/validation only; no production customer alert effect.
- **BACKTEST:** replay against deterministic historical/synthetic fixtures and labelled incidents where available.
- **SHADOW:** evaluate live evidence without authoritative customer alerting; compare to active policy and operator outcomes.
- **ACTIVE:** only after required review/evidence gates; version remains rollback-capable.

Promotion evidence is class-specific. It shall include representative positive/negative cases, maintenance/expected-state cases, collection-degradation cases, storm conditions where relevant and known difficult edge cases. Metrics may include false-positive rate, false-negative/missed-event rate, precision/recall, detection latency, alert volume/cardinality, correlation compression and operator-confirmation rate. This document does not invent a universal numeric acceptance threshold; release criteria must be justified per class and deployment risk.

---

## 7. Active Evidence Acquisition

Active evidence acquisition is an investigation capability, not unrestricted probing. Each action must specify purpose, allowed target/scope, expected information gain, protocol/tool, credential need, rate/time/resource budget, potential impact and stop conditions. RBAC, tenant policy, maintenance constraints and audit apply. Unsafe or disruptive evidence collection requires explicit approval or is forbidden.

The investigation planner may rank candidate evidence actions, but deterministic policy decides whether an action is allowed. Failure to acquire evidence must reduce confidence rather than be interpreted as confirmation of the hypothesis.

---

## 8. RCA, change intelligence and causal discipline

Correlation orders evidence; it does not itself prove causation. Change intelligence may raise or lower a hypothesis based on topology/dependency relevance, timing, blast radius, expected effects and contradictions. A recent change is not automatically the cause.

Every non-trivial RCA hypothesis shall retain: supporting evidence IDs; contradictory evidence; missing expected evidence; dependency/path rationale; confidence; time window; candidate alternatives; and validation actions. Confidence may increase or decrease as evidence changes.

Causal contradiction testing should ask whether expected downstream/upstream effects are present. If predicted effects are absent or another dependency better explains the observation set, confidence must fall or the hypothesis must be rejected.

---

## 9. Operator feedback and learning governance

Operator feedback is structured evidence, not an automatic production-policy mutation. Supported feedback may include confirmed/rejected cause, alert useful/not useful, false-positive reason, suppression reason, remediation success/failure and missing evidence. Feedback records actor, tenant, incident/policy/model version and timestamp.

Use of feedback for model/policy training requires a governed evaluation/promotion process. Production policy, prompts or model weights must not silently self-modify from individual operator actions.

Cross-tenant learning is not assumed. Any future pooled/anonymized learning requires explicit privacy, contractual, security and Founder/change-control approval plus demonstrable tenant-isolation safeguards.

---

## 10. AI/ML/GNN evaluation and trust

A reproducible evaluation harness shall support deterministic fixtures, historical incidents where lawfully available, ambiguous cases, adversarial inputs, incomplete evidence, contradictory evidence, prompt-injection attempts, tenant-boundary cases, outage/provider failures and baseline deterministic RCA comparison.

AI trust levels:

- `CONFIRMED`: deterministic/authoritative evidence establishes the claim under the relevant contract.
- `HIGH`: strong corroborated evidence with no material unresolved contradiction.
- `MEDIUM`: plausible and supported but material alternatives/missing evidence remain.
- `LOW`: weak hypothesis useful only to guide investigation.
- `INCONCLUSIVE`: evidence does not support a defensible ranking/claim.

These labels are not substitutes for calibrated probabilities unless a separately validated model explicitly supplies them.

Model/prompt/policy/data drift monitoring shall compare production behavior with certified evaluation baselines. Challenger versions do not become active simply because they are newer; they require measured improvement without unacceptable regression in safety, grounding, tenant isolation, latency or resource cost.

---

## 11. Incident Copilot contract

Incident Copilot is an assistance layer over authoritative Platform state, not an independent incident database or causal authority. It shall answer using the following contract where applicable:

`Observed → Impact → Likely cause → Confidence → Evidence → Contradictions → Missing evidence → Next validation → Recommended action`

Every evidence reference must resolve to an authoritative incident/alert/change/topology/telemetry/runbook artifact available to the requesting tenant/user. When evidence is insufficient, Copilot returns `INCONCLUSIVE` and identifies the missing evidence or safest validation path.

Recommendations remain non-executable text/plans until they pass deterministic remediation/tool policy, RBAC, parameter validation, approval requirements, blast-radius controls, audit and post-action validation. The Copilot cannot directly execute privileged tools.

---

## 12. Prompt-injection and deterministic tool-use firewall

All external text — logs, SNMP/syslog payloads, tickets, configuration text, web/API content, user notes, vendor messages and model-retrieved documents — is untrusted data, not privileged instruction.

Tool invocation must use a deterministic allowlist/schema and enforce tenant scope, identity/RBAC, action class, parameter bounds, target constraints, preconditions, approval policy, secret isolation, rate/resource limits and audit. Model output is proposed structured input to this firewall; it cannot override policy or create new tool authority.

Security testing shall include direct and indirect prompt injection, encoded/obfuscated instructions, tool-parameter exfiltration attempts, cross-tenant resource references, malicious log/ticket content and attempts to reveal system prompts, credentials or hidden context.

---

## 13. Provider abstraction, egress and graceful degradation

AI capability shall be designed behind an interface that can support approved hosted, private-hosted and local/on-prem providers plus a **no-LLM** operating mode. This requirement does not approve a specific model/runtime.

Data-egress policy shall classify allowed/blocked data types, provider/region destination, masking/redaction, retention, opt-out and audit. Secrets, raw credentials and unnecessarily sensitive payloads must not be included in prompts/model context.

If AI/provider connectivity fails, deterministic monitoring, alert evaluation, incident evidence, topology, search/runbooks and manual remediation remain available. UI/API must identify AI unavailability/degraded mode rather than silently fabricating assistance.

---

## 14. Shared-service, PKI and facilities evidence

Shared services such as DNS, DHCP, NTP/PTP, RADIUS, TACACS+, LDAP and AD shall be represented as dependencies where evidence supports the relationship. Dependency health can explain broad symptom patterns, but service-specific failure evidence must remain distinct from collector failure.

PKI assurance may use certificate/chain metadata, endpoint validation, trust-store/configuration state and protocol evidence. It shall not overclaim revocation assurance where OCSP/CRL/provider evidence is unavailable.

Facilities/power telemetry may enter the evidence graph through supported APIs/SNMP/connectors to explain site/rack/device symptoms. Platform does not become a BMS/DCIM configuration/control system under this requirement.

---

## 15. Resource, reliability and observability controls

AI/analytics workloads shall have configurable CPU/memory/GPU/concurrency/queue/time budgets. Resource exhaustion, model timeout or queue backlog must degrade assistance before starving collection, ingestion, alerting, storage durability or control-plane functions.

Operational telemetry shall expose at minimum: request volume; provider/model/prompt/policy versions; latency; timeout/error/fallback counts; queue depth; resource consumption; evidence coverage/grounding status; `INCONCLUSIVE` rate; tool proposals/allow/deny outcomes; evaluation drift; and tenant-safe cost/token/compute metrics where relevant.

No raw customer secrets or unsafe payloads may be written to AI observability logs, traces, support bundles or evaluation artifacts.

---

## 16. Security, privacy and air-gap requirements

- Tenant identity and evidence authorization are enforced before retrieval, correlation, prompting or tool proposal.
- Model/provider context is least-data/minimized and governed by egress policy.
- AI-generated content is treated as untrusted until validated by deterministic output/schema/safety checks.
- Evaluation and support bundles are secret scanned and access controlled.
- Air-gapped deployments retain deterministic monitoring and may use only locally approved models if configured; no internet dependency is introduced for core monitoring.
- External provider failure or denial must not block core incident workflows.
- Operator feedback, prompts and model outputs follow configured retention/export/deletion policy.
- No employee/user-surveillance inference is authorized by this intelligence master.

---

## 17. Formal traceability requirements

| REQ ID | Release | Requirement / acceptance evidence |
|---|---|---|
| `REQ-MIA-001` | P0 | Each production critical alert class has a versioned Alert Evidence Contract with positive, negative, contradictory, degraded-telemetry, maintenance/expected-state and recovery fixtures. |
| `REQ-MIA-002` | P0 | Tests prove Sensor/Agent/collector/path/schema/time degradation does not automatically become monitored-object DOWN/CRITICAL and maps to appropriate semantic state/confidence. |
| `REQ-MIA-003` | P0 | API/UI/event tests prove severity and confidence are separate fields/concepts and are not silently derived from each other. |
| `REQ-MIA-004` | P0 | Policy lifecycle tests prove DRAFT/BACKTEST/SHADOW/ACTIVE versioning, approval, rollback and prevention of bypass promotion. |
| `REQ-MIA-005` | P0 | Release evidence reports class-appropriate alert quality metrics on reproducible datasets/fixtures and documents the approved acceptance rationale without invented universal thresholds. |
| `REQ-MIA-006` | P1 | Active-evidence tests prove RBAC/tenant scope, bounded rate/time/resource use, audit, unsafe-action denial and confidence reduction when acquisition fails. |
| `REQ-MIA-007` | P0 | Evidence Coverage is machine-readable and tests prove expected/present/missing/stale/degraded/contradictory evidence changes confidence/state as designed. |
| `REQ-MIA-008` | P0 | Semantic-state tests cover `MONITORING_DEGRADED`, `REDUNDANCY_LOST`, `SLA_DEGRADED`, `EXPECTED_STATE`, `UNKNOWN` and `STALE` where relevant. |
| `REQ-MIA-009` | P1 | Change/RCA tests prove temporal proximity alone cannot establish cause and contradictions/missing expected effects reduce confidence. |
| `REQ-MIA-010` | P1 | Operator-feedback tests prove feedback is audited/version bound and cannot directly mutate active alert policy/model/prompt behavior. |
| `REQ-MIA-011` | P0 | AI evaluation harness reproduces positive, ambiguous, contradictory, incomplete, adversarial, tenant-boundary and provider-failure cases against deterministic baselines. |
| `REQ-MIA-012` | P0 | Unsupported factual/causal/remediation claims are rejected or returned as hypothesis/`INCONCLUSIVE`; every supported claim includes evidence provenance. |
| `REQ-MIA-013` | P0 | Trust labels `CONFIRMED/HIGH/MEDIUM/LOW/INCONCLUSIVE` remain distinct from severity and tests cover downgrade on contradiction/missing evidence. |
| `REQ-MIA-014` | P0 | Prompt-injection suite covers untrusted logs/tickets/config/API text, obfuscation, cross-tenant references and tool-exfiltration attempts; deterministic firewall denies unauthorized actions. |
| `REQ-MIA-015` | P0 | Tenant/privacy tests prove no silent cross-tenant training/retrieval and enforce configured AI data-egress/provider/region/masking policy. |
| `REQ-MIA-016` | P0 | Provider/model outage tests prove deterministic monitoring/alert/incident workflows continue and AI degraded/unavailable state is explicit. |
| `REQ-MIA-017` | P1 | Cohort/anomaly tests disclose comparison population, sufficiency/fallback and prevent anomaly/cohort score from being represented as causal proof. |
| `REQ-MIA-018` | P1 | Information-gain planner tests prove candidate actions are ranked within deterministic safety/cost/latency policy and cannot self-authorize privileged tools. |
| `REQ-MIA-019` | P1 | RCA history preserves confidence changes, supporting/contradictory evidence and validation actions reproducibly. |
| `REQ-MIA-020` | P1 | Drift/champion-challenger tests prevent promotion when grounding, safety, tenant isolation, latency or resource regressions exceed the approved release envelope. |
| `REQ-MIA-021` | V1.x | Predictive claims are backtested/calibrated for stated horizon/population and expose error/confidence; unsupported failure prediction is prohibited. |
| `REQ-MIA-022` | P1 | Incident Copilot output follows the governed answer contract, cites authorized evidence, surfaces contradictions/missing evidence and returns `INCONCLUSIVE` when warranted. |
| `REQ-MIA-023` | P0 | Tool-use tests prove model output cannot directly invoke privileged remediation and all actions pass deterministic RBAC/scope/parameter/precondition/approval/audit controls. |
| `REQ-MIA-024` | P1 | Provider abstraction tests prove no-LLM mode and at least the configured approved provider modes fail/degrade without blocking core monitoring. |
| `REQ-MIA-025` | P1 | Shared-service dependency tests prove DNS/DHCP/time/AAA/directory evidence can explain dependent symptoms while preserving separate service-vs-monitoring failure semantics. |
| `REQ-MIA-026` | P1 | PKI tests distinguish expiry, chain/name/trust/configuration/revocation-evidence conditions without claiming unavailable validation certainty. |
| `REQ-MIA-027` | P1 | Facilities/power integration tests prove evidence can contribute to incident explanation without granting BMS/DCIM control authority. |
| `REQ-MIA-028` | P0 | Resource-exhaustion tests prove AI/analytics degrades before core monitoring and exposes queue/resource/fallback telemetry. |
| `REQ-MIA-029` | P0 | AI observability records version/provenance/latency/error/fallback/evidence coverage/tool decisions while secret-scanning logs/traces/support/evaluation bundles. |
| `REQ-MIA-030` | P0 | Air-gap tests prove core deterministic monitoring requires no external AI/internet service and locally configured AI can be disabled without functional collapse. |

---

## 18. Quality and release gates

P0/P1 implementation under this master is not complete until the relevant requirements have source traceability, deterministic unit/component tests, negative security tests, tenant-isolation tests, adversarial prompt/tool tests where AI is involved, reproducible evaluation evidence, failure/degradation tests, resource-bound tests and operational telemetry/runbook evidence.

A model, prompt, policy or rule version may not be promoted solely because aggregate accuracy improved. Safety, tenant isolation, evidence grounding, false-alert behavior, latency/resource consumption and graceful-degradation regressions are independent blockers.

`V1.x` requirements are not v1 GA blockers unless separately promoted through change control. A capability may not be advertised as implemented merely because this master defines it.

---

## 19. Explicit exclusions and unresolved engineering spikes

This master does **not** approve autonomous unrestricted remediation, direct LLM privileged execution, cross-tenant learning, employee surveillance, model-provider internet dependency for core monitoring, a specific GNN/LLM/vector/runtime dependency, universal alert thresholds, or AI-generated causal claims without evidence.

Engineering decisions still requiring bounded spikes before implementation include: first Alert Evidence Contract schemas and class-specific acceptance envelopes; evaluation dataset governance; model/provider compatibility and air-gap footprint; local/private model resource sizing; tool-firewall schema; cohort-definition strategy; drift metrics; predictive calibration method; and storage/retention for AI provenance/evaluation artifacts.

---

## 20. Founder-reserved decisions

Founder/change-control approval remains required for external AI/model-provider commercial commitments, material cloud/GPU spend, cross-tenant pooled learning, contractual AI accuracy/MTTR/prediction claims, new external data-egress destinations, surveillance-adjacent product behavior, and any new model/runtime/dependency not admitted through normal OSS/security/performance governance.

---

## 21. Implementation-status disclaimer

At creation, this document is a requirements/architecture authority only. It does not assert that any `PRD-MIA-*` or `REQ-MIA-*` capability has been implemented, benchmarked, certified or released. Implementation status must be established from exact source, executable tests, exact-head CI/runtime evidence and release records.
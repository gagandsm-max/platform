# Platform — Monitoring Intelligence, Alert Quality and AI Assurance Master

**Document ID:** PLATFORM-MIA-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Monitoring Intelligence / Alert Quality / AI Assurance (after merge)**
**Product:** Platform
**Change class:** Additive v3.2.2 product/engineering requirements; no runtime implementation, commercial provider commitment or readiness claim

---

## 1. Authority and purpose

This document is the consolidated specialized master for monitoring intelligence, alert quality, evidence qualification, RCA confidence, AI/ML assurance and safe Incident Copilot behavior. `Platform_21_Enterprise_Gap_Closure_Master.md` owns the separate enterprise-gap capability family and must not be duplicated here.

`Platform_00_Document_Index.md` defines precedence. This master supplements existing v3.2.2 requirements and must not silently override a higher-precedence explicit requirement. Contradictions require change control before implementation.

Nothing in this document selects a commercial model/provider, vector database, mandatory microservice, commercial SKU, price, compliance certification or vendor commitment. Runtime/library candidates named below remain subject to `Platform_07_OSS_Catalog.md`, exact-version security/licence review and benchmark admission before shipment. Capability statements and sizing values are requirements/engineering targets, not implementation or production-readiness claims.

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
11. **Local intelligence is lightweight by design.** Local AI/ML/LLM shall run inside the existing certified Core tiers, shall not create a mandatory GPU requirement and shall yield resources before live monitoring is degraded.
12. **Small model, strong tools.** Platform shall reduce/query/correlate evidence deterministically before LLM inference; the local LLM is an explanation/copilot layer, not the hot-path monitoring or causal engine.

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
| `PRD-MIA-033` | P0 | Platform shall implement the Local Intelligence capability as bounded logical modules under the existing modular Core/analytics architecture; the module map in §15.1 is authoritative and does not imply one process/microservice per module. |
| `PRD-MIA-034` | P0 | Supported Local AI/ML/LLM shall fit inside the existing Core sizing tiers in `Platform_11`; standard deployment shall be CPU-first with **no mandatory GPU** and shall preserve monitoring/ingestion/storage/alert/RCA objectives under certified AI load. |
| `PRD-MIA-035` | P0 | On the 8 vCPU/16 GB tier the default local LLM target is a **0.5B–1.5B parameter 4-bit-quantized** model with one concurrent request; larger models, including up to approximately 3B, require explicit benchmark certification and may not silently raise the minimum Core requirement. |
| `PRD-MIA-036` | P0 | Customer production Core shall not train or fine-tune foundation LLMs. Local LLM intelligence shall use approved pretrained quantized artifacts plus Platform evidence retrieval/tools; lightweight statistical/ML recalibration remains permitted when bounded and interruptible. |
| `PRD-MIA-037` | P0 | Local LLM input shall be produced by a bounded context/evidence builder; high-volume raw telemetry shall be aggregated/queried by Platform first, and the supported small-Core operating context shall normally remain within a 2K–4K token envelope unless benchmarked otherwise. |
| `PRD-MIA-038` | P0 | GNN inference shall use bounded incident-relevant temporal subgraphs and shall not evaluate the complete enterprise graph for every observation; GNN remains optional assistance behind deterministic RCA and may be deferred under resource pressure. |
| `PRD-MIA-039` | P0 | Local intelligence shall implement a Resource Governor/circuit breaker with the protection order `live collection/ingestion/storage → alert/incident/deterministic RCA → ML/GNN → interactive LLM → training/recalibration`; lower-priority work shall queue, defer, unload or stop first. |
| `PRD-MIA-040` | P0 | Local model/runtime artifacts shall be offline-packageable, signed/checksummed, versioned and governed by a Platform Model Registry with provenance, evaluation status, compatibility and rollback metadata. |
| `PRD-MIA-041` | P1 | Similar-incident, runbook and knowledge retrieval shall prefer Platform-owned structured/graph/keyword retrieval plus bounded embeddings where useful; no dedicated vector database is required unless benchmark evidence demonstrates a material gap. |
| `PRD-MIA-042` | P0 | Runtime/library selection shall remain dependency-governed: existing approved NumPy/Polars/scikit-learn may be used; PyTorch/PyG remain GNN candidates; **llama.cpp** is the preferred P0 candidate for CPU/GGUF local-LLM inference and **ONNX Runtime CPU** the preferred P0 candidate for compact embedding/ML inference, but neither may ship until admitted by `Platform_07` exact-version licence/CVE/SBOM and performance review. |

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

For Flow evidence, trust additionally considers configured exporter mapping, template validity, sampling-known/change state, protocol sequence gaps, receiver/socket/queue drops, observation-point scope, enrichment confidence and downstream pipeline health. `NO_RECENT_FLOW`, `TEMPLATE_MISSING`, `SAMPLING_UNKNOWN` or degraded FLOW Sensor health are monitoring-evidence conditions; they do not prove zero network traffic, link failure or application outage.

Flow volume/anomaly evidence may explain or prioritize a congestion/traffic hypothesis but does not independently prove congestion or causal path. Corroborating interface utilization/queue/discard/latency/path/topology/change evidence remains subject to the applicable Alert Evidence Contract.

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

The base PRD/UI additionally requires **operator-triggered** bounded diagnostics (for example Ping, traceroute/path, SNMP/TCP/DNS/SSH/API tests), multi-vantage verification, Live Evidence and deterministic `Verify Now` packs. Those manual NOC actions use the same evidence/safety model but are not dependent on an AI/planner. Planner-selected/autonomous evidence acquisition remains governed by this section and its release priority; manual safe diagnostics may be implemented earlier as base product workflow.

Multi-vantage acquisition must preserve independent source/vantage results and contradictions rather than average disagreement into a false operational conclusion. On-demand evidence remains provenance-distinct from continuously monitored evidence even when both contribute to qualification/RCA.

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

Within the Troubleshoot/Investigation experience, Copilot may explain current evidence and rank **Suggested Next Tests** from the deterministic permitted action/diagnostic-pack catalogue. Suggestions must identify why the test is useful, expected information gain where available, and material safety/cost considerations. Selecting a suggestion invokes the normal deterministic authorization/bounds flow; Copilot wording never grants privilege or fabricates a test result.

---

## 12. Prompt-injection and deterministic tool-use firewall

All external text — logs, SNMP/syslog payloads, tickets, configuration text, web/API content, user notes, vendor messages and model-retrieved documents — is untrusted data, not privileged instruction.

Tool invocation must use a deterministic allowlist/schema and enforce tenant scope, identity/RBAC, action class, parameter bounds, target constraints, preconditions, approval policy, secret isolation, rate/resource limits and audit. Model output is proposed structured input to this firewall; it cannot override policy or create new tool authority.

Security testing shall include direct and indirect prompt injection, encoded/obfuscated instructions, tool-parameter exfiltration attempts, cross-tenant resource references, malicious log/ticket content and attempts to reveal system prompts, credentials or hidden context.

---

## 13. Provider abstraction, egress and graceful degradation

AI capability shall be designed behind an interface that can support approved hosted, private-hosted and local/on-prem providers plus a **no-LLM** operating mode. This requirement does not approve a commercial model/provider.

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

### 15.1 Authoritative Local Intelligence module map

These are **logical modules/capabilities**. They shall be grouped into the existing modular Core, `platform-ai`/analytics worker and existing Incident/RCA/Graph/Sensor domains as appropriate; Platform shall not create thirty mandatory microservices.

| # | Logical module | Ownership | Approved/expected foundation | Responsibility |
|---:|---|---|---|---|
| 1 | Local Intelligence Orchestrator | **BUILD** | Java/Python within modular Core | Coordinates evidence, ML/GNN/LLM invocation, model selection, queues and graceful degradation. |
| 2 | Telemetry Quality Engine | **BUILD** | Deterministic Core logic | Scores freshness/source health/time/schema/sequence quality before operational assertions. |
| 3 | Feature & Evidence Builder | **BUILD** | ClickHouse/PostgreSQL + Java/Python | Converts high-volume raw observations into bounded typed features/evidence. |
| 4 | Baseline Engine | **BUILD on OSS** | NumPy/scikit-learn/SQL | Rolling, robust, seasonal and learned baselines with version/provenance. |
| 5 | Anomaly Detection Engine | **BUILD on OSS** | scikit-learn/approved inference runtime | Produces anomaly candidates; never directly declares causality/failure. |
| 6 | Multivariate Anomaly Engine | **BUILD on OSS** | statistical/scikit-learn methods | Detects correlated abnormal patterns across related metrics. |
| 7 | Peer/Cohort Intelligence Engine | **BUILD** | statistical/ML | Compares entities with governed comparable cohorts and sufficiency controls. |
| 8 | Forecast & Capacity Prediction Engine | **BUILD on OSS** | regression/statistical ML | Defensible capacity/exhaustion/degradation forecasts with calibration. |
| 9 | Graph Intelligence Engine | **BUILD** | Platform temporal graph | Deterministic dependency traversal, common-cause, blast-radius and temporal graph reasoning. |
| 10 | GNN Inference Engine | **BUILD on candidate OSS** | PyTorch/PyG candidate | Bounded root-candidate/propagation/impact/relationship scoring on incident subgraphs. |
| 11 | Causal RCA Fusion Engine | **BUILD** | deterministic evidence + graph + ML/GNN | Fuses evidence into explainable hypotheses/confidence; preserves `INCONCLUSIVE`. |
| 12 | Causal Contradiction Engine | **BUILD** | deterministic rules/graph | Searches for observations that disprove or materially weaken hypotheses. |
| 13 | Active Evidence Acquisition Engine | **BUILD** | existing Sensor/Core diagnostics | Selects bounded read-only validation actions by information gain/safety/resource cost. |
| 14 | Similar Incident Memory Engine | **BUILD** | PostgreSQL/ClickHouse/graph + optional embeddings | Finds relevant historical incidents without copying their cause. |
| 15 | Change Intelligence Engine | **BUILD** | Platform_19 change facts | Correlates NCM/cloud/K8s/CI/CD/DB/cert/ITSM changes with incident windows. |
| 16 | Incident Flight Recorder | **BUILD** | Platform_19 evidence windows | Preserves bounded before/during/after operational evidence and graph state. |
| 17 | Embedding Inference Service | **BUILD wrapper; runtime candidate** | preferred ONNX Runtime CPU candidate | Compact local embeddings for incidents/runbooks/knowledge where justified. |
| 18 | Local Retrieval / RAG Engine | **BUILD** | PostgreSQL/ClickHouse/graph/search + optional embeddings | Hybrid structured/graph/keyword/embedding retrieval; no dedicated vector DB by default. |
| 19 | Local LLM Runtime Adapter | **BUILD wrapper; runtime candidate** | preferred llama.cpp/GGUF CPU candidate | Bounded quantized local text-generation inference behind AI Gateway. |
| 20 | Local Incident Copilot | **BUILD** | Local LLM + Platform tools | Evidence-grounded operator Q&A, explanation, investigation and PIR/report assistance. |
| 21 | LLM Context Builder | **BUILD** | deterministic evidence summarization | Converts large evidence sets into small typed/cited context; enforces token/data minimization. |
| 22 | LLM Tool Firewall | **BUILD** | structured tool schemas + RBAC/policy | Validates tool proposal, tenant/resource scope, parameters, rate, approval and audit. |
| 23 | Prompt-Injection Defense Layer | **BUILD** | policy/input classification | Treats logs/tickets/config/docs/API text as untrusted data and blocks instruction escalation. |
| 24 | AI Model Registry | **BUILD** | PostgreSQL metadata + signed artifacts | Model/runtime version, checksum/signature, compatibility, approval, evaluation and rollback. |
| 25 | AI Resource Governor | **BUILD** | process/cgroup/service quotas + Core health | CPU/RAM/concurrency/queue/time limits and circuit-breaker degradation. |
| 26 | AI Evaluation & Benchmark Engine | **BUILD** | Python test/evaluation harness | Reproducible alert/anomaly/RCA/GNN/LLM quality and resource regression gates. |
| 27 | Model/Data Drift Engine | **BUILD** | statistics/Platform telemetry | Detects population/feature/model quality drift and raises review state. |
| 28 | Champion/Challenger Manager | **BUILD** | Model Registry + evaluation | Shadow comparison and controlled promotion/rollback of model/rule versions. |
| 29 | AI Provenance & Explainability Engine | **BUILD** | PostgreSQL/audit/evidence IDs | Persists model/prompt/policy/evidence/tool/confidence provenance for consequential outputs. |
| 30 | AI Self-Monitoring | **BUILD using Platform monitoring** | existing Platform telemetry/health | Monitors inference latency, queue, resources, errors, fallback, drift and model health. |

### 15.2 Local runtime/dependency boundary

Current approved foundations remain NumPy, Polars and scikit-learn for lightweight analytics/ML. PyTorch and PyTorch Geometric remain candidate GNN foundations subject to the existing P0 benchmark/licence decision.

For the new local inference functions, the preferred engineering candidates are:

- **llama.cpp / GGUF** for CPU-first quantized local LLM inference because the desired deployment is small, offline-capable and does not require a GPU;
- **ONNX Runtime CPU** for compact embedding and portable ML inference where it materially reduces resident framework footprint.

These are **candidate selections, not shipment approval**. Before merge of a runtime implementation they must be entered/admitted in `Platform_07_OSS_Catalog.md` at an exact pinned version after transitive/bundled licence, CVE, supply-chain, air-gap packaging and performance review. Model weights have their own licences/provenance and are not licensed merely because the inference runtime is permissive.

Do not add a mandatory LangChain/LlamaIndex/Ray/Spark/MLflow/vector-database/Kubernetes-AI stack merely to orchestrate these capabilities. A new dependency requires evidence that existing Platform code and approved libraries cannot meet the requirement safely within the resource envelope.

### 15.3 Local LLM role and model contract

The local LLM is responsible for natural-language explanation, evidence summarization, operator Q&A, structured tool proposal, runbook/similar-incident explanation, report/PIR assistance and other bounded language tasks. It is **not** responsible for polling, metric calculation, threshold/baseline evaluation, authoritative alert qualification, graph traversal, raw incident correlation or independent causal proof.

Standard smallest-Core target:

```text
Core tier                  8 vCPU / 16 GB RAM
Local LLM target           0.5B–1.5B parameters, 4-bit quantized
Concurrent local requests  1
Operating context          normally 2K–4K tokens
LLM CPU burst              within the Platform_11 ≤2 vCPU AI burst envelope
AI peak RAM                within the Platform_11 ≤3 GB aggregate AI envelope
Mandatory GPU              none
Foundation-model training  prohibited on customer production Core
```

A model up to approximately 3B may be enabled on a tier only after that exact model/runtime/context combination passes the tier's resource and monitoring-regression certification. Larger model size is not a default scaling objective; higher Core tiers should preferentially improve monitoring and AI concurrency.

### 15.4 Local ML/GNN operating contract

- Baseline/anomaly/cohort/forecast functions shall favor incremental/statistical/classical models over deep networks where accuracy is sufficient.
- Normal continuous ML activity on the 8/16 Core targets a sub-core average CPU footprint and bounded memory; expensive fitting/recalibration is scheduled, interruptible and may be postponed indefinitely while the Core is under pressure.
- GNN inference is incident/event driven over bounded temporal subgraphs. Training/experimentation is not part of the live monitoring hot path.
- Optional embedding computation is batch/on-demand and shall not become a prerequisite for Similar Incident Memory or runbook retrieval; deterministic/structured search remains the fallback.
- Customer-specific behavior is expressed through Platform data/evidence retrieval, policy, feedback and governed lightweight models—not through continuous foundation-model fine-tuning.

### 15.5 Resource Governor and circuit breaker

The Local Intelligence Orchestrator and deployment controls enforce this priority:

```text
1  Collection / Sensor communication / ingestion / Kafka / PostgreSQL / ClickHouse durability
2  Alert qualification / incident correlation / deterministic RCA
3  Lightweight ML / bounded GNN inference
4  Local LLM interactive assistance / embeddings
5  Recalibration / challenger evaluation / training jobs
```

When CPU, memory, swap, ingestion lag, Kafka/ClickHouse backlog, query latency, incident-storm load or storage pressure crosses the certified guard band, Platform shall progressively postpone training/recalibration, reduce or queue LLM concurrency, defer optional embeddings/GNN, unload the local LLM and finally disable optional AI assistance before protected monitoring functions are allowed to miss objectives.

The 8/16 profile shall normally unload/release the local LLM after a bounded idle period unless a benchmarked warm-model profile proves sufficient headroom. AI queue depth and degraded/suspended state are visible to the user and to Platform self-monitoring.

### 15.6 Local model lifecycle and offline packaging

Every shipped local model/embedding/GNN artifact records at least: model ID/type/version, source/provenance, model-specific licence, cryptographic checksum/signature, supported runtime and CPU architecture, quantization, context limit, feature/schema version, evaluation dataset/version, measured quality, resource envelope, approval state, activation time and rollback predecessor.

Air-gapped installation/upgrade includes the exact approved runtime/model artifacts in signed offline media; runtime may not fetch model weights from the Internet. Untrusted or customer-supplied models are disabled by default and require a governed import/security/evaluation workflow.

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
- Local model weights and runtimes are supply-chain artifacts: checksum/signature/provenance/licence/CVE policy applies before activation.
- Model files and retrieved context are never treated as trusted instructions; imported models execute only inside the constrained inference boundary and receive no direct credentials or unrestricted filesystem/network/tool authority.

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
| `REQ-MIA-031` | P0 | Architecture/component tests map the §15.1 logical modules to bounded deployment units and prove no module bypasses authoritative telemetry/evidence, tenant scope or deterministic RCA/tool policy. |
| `REQ-MIA-032` | P0 | SNWP-2500-v1 resource certification proves the supported 8 vCPU/16 GB local-intelligence profile remains within **≤2 vCPU AI burst and ≤3 GB aggregate AI peak RAM**, with no regression beyond approved monitoring/ingestion/query/failover tolerances. |
| `REQ-MIA-033` | P0 | CPU-only certification proves all mandatory Local AI/ML/LLM capabilities can operate without a GPU; optional accelerator failure/removal does not disable core monitoring or supported CPU fallback. |
| `REQ-MIA-034` | P0 | Local-LLM tests prove default small-Core 0.5B–1.5B Q4 operation, single-request concurrency, bounded 2K–4K context, idle unload/release behavior and refusal to enable an uncertified larger model/profile. |
| `REQ-MIA-035` | P0 | Production packaging/tests prove foundation-model training/fine-tuning is unavailable on customer Core while lightweight ML recalibration remains bounded, interruptible and lower priority than monitoring. |
| `REQ-MIA-036` | P0 | GNN/embedding tests prove bounded incident/subgraph and batch/on-demand behavior; full-graph-per-observation or embedding-dependent monitoring hot paths are rejected by design/performance tests. |
| `REQ-MIA-037` | P0 | Resource-governor failure/load tests prove the documented degradation order, AI queue visibility and local-model unload/suspension before protected collection/ingestion/storage/alert/RCA objectives are violated. |
| `REQ-MIA-038` | P0 | Model-registry/offline-package tests verify model/runtime checksum/signature, licence/provenance, compatibility, evaluation/resource metadata, no runtime Internet download and rollback to a previously approved artifact. |
| `REQ-MIA-039` | P0 | Dependency admission tests prove llama.cpp/ONNX Runtime or any replacement runtime cannot ship until exact-version `Platform_07` admission, transitive/bundled licence review, CVE/SBOM scan and resource/security benchmark evidence exist. |

---

## 18. Quality and release gates

P0/P1 implementation under this master is not complete until the relevant requirements have source traceability, deterministic unit/component tests, negative security tests, tenant-isolation tests, adversarial prompt/tool tests where AI is involved, reproducible evaluation evidence, failure/degradation tests, resource-bound tests and operational telemetry/runbook evidence.

A model, prompt, policy or rule version may not be promoted solely because aggregate accuracy improved. Safety, tenant isolation, evidence grounding, false-alert behavior, latency/resource consumption and graceful-degradation regressions are independent blockers.

The Local Intelligence profile is additionally release-blocked if it raises the certified minimum Core requirement, requires a GPU for mandatory capabilities, exceeds the applicable `Platform_11` envelope, performs foundation-model training on production Core, performs unbounded full-graph/raw-telemetry inference, downloads model artifacts at runtime in air-gap mode, or starves protected monitoring functions.

`V1.x` requirements are not v1 GA blockers unless separately promoted through change control. A capability may not be advertised as implemented merely because this master defines it.

---

## 19. Explicit exclusions and unresolved engineering spikes

This master does **not** approve autonomous unrestricted remediation, direct LLM privileged execution, cross-tenant learning, employee surveillance, model-provider internet dependency for core monitoring, a dedicated vector database, universal alert thresholds, AI-generated causal claims without evidence, mandatory GPU/NPU hardware, foundation-model training/fine-tuning on customer Core, or a separate microservice per Local Intelligence logical module.

Engineering decisions still requiring bounded spikes before implementation include: first Alert Evidence Contract schemas and class-specific acceptance envelopes; evaluation dataset governance; exact local LLM model artifact/licence/quality; exact llama.cpp or replacement runtime version/build options; exact ONNX Runtime or replacement inference package/build options; exact GNN model architecture/training labels; CPU architecture-specific throughput; model cold/warm load latency; certified context/concurrency per Core tier; tool-firewall schema; cohort-definition strategy; drift metrics; predictive calibration method; and storage/retention for AI provenance/evaluation artifacts.

The **resource envelopes and module boundaries in this master and `Platform_11` are fixed engineering targets**; benchmark spikes certify or reduce supported profiles. They may not silently raise the minimum Core sizing. A material increase to minimum Core compute requires explicit architecture/product change control rather than treating benchmark failure as permission to add hardware.

---

## 20. Founder-reserved decisions

Founder/change-control approval remains required for external AI/model-provider commercial commitments, material cloud/GPU spend, cross-tenant pooled learning, contractual AI accuracy/MTTR/prediction claims, new external data-egress destinations, surveillance-adjacent product behavior, and any commercial/licensing commitment around model weights or hosted inference.

Engineering dependency/runtime admission remains subject to normal architecture/OSS/security/performance governance and does not itself constitute a commercial provider commitment.

---

## 21. Implementation-status disclaimer

At creation/update, this document is a requirements/architecture authority only. It does not assert that any `PRD-MIA-*` or `REQ-MIA-*` capability, local model, llama.cpp/ONNX runtime, GNN model or resource envelope has been implemented, benchmarked, certified or released. Implementation status must be established from exact source, executable tests, exact-head CI/runtime evidence and release records.
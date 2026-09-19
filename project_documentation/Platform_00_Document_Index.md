# Platform — Mother Reference Documentation Set

**Document ID:** PLATFORM-INDEX-001  
**Version:** 3.2.2  
**Status:** **Authoritative Mother Reference — 25-File Consolidated Engineering Source of Truth**
**Product:** Platform  
**Project independence:** Clean-room product; separate from ThoughtData, Enterprise360 and NetAI  
**Last reconciliation:** 2026-09-19 — v3.2.2 Sensor communication-integrity and reliability contract reconciliation

## 1. Authority

**v3.2.2 supersedes v3.2.1 as the authoritative Platform mother-reference set after merge to the canonical GitHub `main`.**

v3.2.2 is an additive product/engineering scope revision. It includes five specialized Mother Reference masters: `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` for operational intelligence/MTTR/customer value; `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md` for governed discovery/onboarding, Agent identity continuity and the global Notification Center; `Platform_21_Enterprise_Gap_Closure_Master.md` for enterprise adoption gaps such as normalized SD-WAN, SCIM lifecycle target, SLI/SLO/error budgets, tamper-evident audit, governed offboarding/data portability and connector families; `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` for evidence-qualified alerting, monitoring-state semantics, RCA/AI assurance, bounded AI assistance and related shared-service evidence; and `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` for enterprise monitoring-domain coverage, the single Capability Pack architecture, dimensional certification and Real Customer Requirements normalization. These specialized masters carry formal logical-layer supplements without weakening any v3.2.1 requirement. Existence of `Platform_23` is EMC-0 specification only and does not imply pack runtime, vendor certification or contractual coverage.

v3.2.1 superseded v3.2 as a corrective/governance patch. v3.2 superseded v3.1 as a documentation-packaging revision. Earlier v1/v2/v3.0/v3.1 generated master packages and the legacy `Platform_Master_Architecture_v1.0.docx` are superseded as Platform engineering sources of truth.

Original ThoughtData/Enterprise360, NetAI, SolarWinds, ManageEngine and other supplied reference material remains reference/evidence only and never overrides this set.

## 2. No-assumption rule

- Explicit product-owner decisions remain fixed until change control changes them.
- Delegated engineering choices are architecture decisions and must be recorded.
- Unknown values remain `TBD — Decision Required` or `P0 ENGINEERING SPIKE`; they are not guessed.
- Capacity statements remain engineering targets until benchmark-certified.
- Security/compliance readiness must never be represented as achieved certification until independently achieved.
- Customer-value/MTTR percentages are not product claims until supported by disclosed reproducible customer/pilot evidence.

## 3. Exact 25-file mother-reference set

| # | File | Governing scope |
|---:|---|---|
| 1 | `AGENTS.md` | Coding-agent/engineer navigation and non-negotiable repository rules |
| 2 | `Platform_00_Document_Index.md` | Authority, precedence, file map, change/completeness rules |
| 3 | `Platform_01_BRD.md` | Business intent, outcomes, scope, principles, commercial/security/continuity requirements |
| 4 | `Platform_02_PRD.md` | Base customer-visible functional and non-functional product requirements |
| 5 | `Platform_03_Architecture.md` | Governing architecture, technology, deployment, HA/data/Sensor/AI/security decisions |
| 6 | `Platform_04_HLD.md` | Subsystems, deployment units, interfaces, ownership and failure boundaries |
| 7 | `Platform_05_LLD.md` | Concrete schemas, topics, state machines, adapters, algorithms and implementation rules |
| 8 | `Platform_06_Dataflow.md` | End-to-end telemetry/control/HA/RCA/NCM/storage/integration sequences |
| 9 | `Platform_07_OSS_Catalog.md` | Authoritative OSS allow/candidate list, licences, custom boundaries and governance |
| 10 | `Platform_08_Requirements_Traceability.md` | Base completeness and requirement traceability control |
| 11 | `Platform_09_UI_UX_Master.md` | UI shell, navigation, Studios, RBAC UX and out-of-box dashboards/workspaces |
| 12 | `Platform_10_Protocol_Security_Connector_Master.md` | Protocols, authentication, credentials, ports, connector and provisioning diagnostics |
| 13 | `Platform_11_Sizing_Capacity_Storage_Master.md` | Core/Sensor sizing, MWU, placement, retention, rollups and storage protection |
| 14 | `Platform_12_Decision_Source_Register.md` | Fixed/delegated/reference/TBD decision provenance |
| 15 | `Platform_13_Engineering_Development_and_Docker_Standards.md` | Engineering execution + Docker Compose development/test standard |
| 16 | `Platform_14_Test_Performance_and_Release_Quality.md` | Test strategy + benchmarks/performance + Alpha/Beta/RC/GA quality gates |
| 17 | `Platform_15_CI_CD_Packaging_and_Distribution.md` | CI/CD + SBOM/signing + `.deb`/`.rpm`/Windows packaging/distribution |
| 18 | `Platform_16_Installation_Upgrade_Rollback_and_Operations.md` | Install/HA enrollment/upgrade/rollback + operational runbooks |
| 19 | `Platform_17_API_Event_and_Data_Model_Contracts.md` | API/event compatibility + PostgreSQL/ClickHouse/Kafka/Valkey data contracts |
| 20 | `Platform_18_Security_Threat_Model.md` | Trust boundaries, threats, abuse cases and mitigations |
| 21 | `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` | Authoritative OIV specialized master: SaaS/carrier/change intelligence, Flight Recorder, Similar Incident Memory, triggered packet evidence, controlled remediation, MTTR/value engine, formal `PRD-OIV-*` + `REQ-OIV-*` supplement |
| 22 | `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md` | Authoritative onboarding specialized master: discovery review, Agent enrollment/identity continuity, approval/audit, grouping/policy preview, Notification Center, formal `PRD-ONB-*` + `REQ-ONB-*` supplement |
| 23 | `Platform_21_Enterprise_Gap_Closure_Master.md` | Authoritative enterprise gap-closure specialized master: normalized SD-WAN, SCIM lifecycle target, SLI/SLO/error budgets, audit tamper evidence, enterprise connector/offboarding/licensing/policy-as-code requirements, formal `PRD-EGC-*` + `REQ-EGC-*` supplement |
| 24 | `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` | Authoritative monitoring-intelligence specialized master: class-specific Alert Evidence Contracts, telemetry trust, semantic monitoring states, alert-quality lifecycle, RCA/AI assurance, Incident Copilot, AI safety/provenance, formal `PRD-MIA-*` + `REQ-MIA-*` supplement |
| 25 | `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` | Authoritative enterprise-monitoring-coverage specialized master: Capability Pack product contract, L1/L2/L3 dimensional certification, technology-family catalogue, Customer Acceptance Profiles, Real Customer Requirements normalization, formal `PRD-EMC-*` + `REQ-EMC-*` supplement |

## 4. Consolidation and version map

### 4.1 v3.1 → v3.2 consolidation

| v3.1 document(s) | v3.2 destination |
|---|---|
| `Platform_13_Engineering_Execution_Standards.md` + `Platform_14_Docker_Development_Environment.md` | `Platform_13_Engineering_Development_and_Docker_Standards.md` |
| `Platform_15_Test_Strategy_and_Quality_Gates.md` + `Platform_22_Performance_and_Benchmark_Plan.md` + `Platform_24_Release_Acceptance_Criteria.md` | `Platform_14_Test_Performance_and_Release_Quality.md` |
| `Platform_16_CI_CD_and_Release_Engineering.md` + `Platform_18_Packaging_and_Distribution.md` | `Platform_15_CI_CD_Packaging_and_Distribution.md` |
| `Platform_17_Installation_Upgrade_Rollback_Runbook.md` + `Platform_23_Operational_Runbooks.md` | `Platform_16_Installation_Upgrade_Rollback_and_Operations.md` |
| `Platform_19_API_and_Event_Contract_Master.md` + `Platform_20_Data_Model_and_Schema_Master.md` | `Platform_17_API_Event_and_Data_Model_Contracts.md` |
| `Platform_21_Security_Threat_Model.md` | `Platform_18_Security_Threat_Model.md` |

Every source section is retained in the destination document. Consolidation is not permission to drop a requirement.

### 4.2 v3.2.1 → v3.2.2 additions

v3.2.2 adds specialized masters under a consolidated-amendment pattern instead of duplicating the same normative text across many base files.

`Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` carries OIV business/product, architecture, HLD, LLD/data, dataflow, UI, security, capacity, quality, operations and RTM authority. Its formal `PRD-OIV-*` and `REQ-OIV-*` IDs are authoritative supplements to the base PRD/RTM catalogues.

The 2026-09-11 reconciliation adds `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md` under the same pattern. Its §§3–12 carry product/architecture/HLD/LLD/dataflow/UI/security/quality/operations authority for the onboarding capability family, and §13 provides formal `REQ-ONB-*` traceability.

The enterprise gap-closure reconciliation adds `Platform_21_Enterprise_Gap_Closure_Master.md` under the same pattern. Its `PRD-EGC-*` and `REQ-EGC-*` requirements govern the enterprise capability family while explicitly reserving monitoring-intelligence/alert-quality/AI-assurance requirements for a separate specialized master.

The 2026-09-12 monitoring-intelligence reconciliation adds `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md`. Its `PRD-MIA-*` and `REQ-MIA-*` requirements govern alert evidence/quality, semantic monitoring states, RCA/AI assurance, bounded AI assistance and related dependency evidence. This remains **v3.2.2**; no v3.2.3 product-version number is created.

The 2026-09-13 enterprise-monitoring-coverage reconciliation adds `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` under the same pattern. Its `PRD-EMC-*` and `REQ-EMC-*` requirements govern the Capability Pack architecture, dimensional certification, enterprise technology coverage and Customer Acceptance Profiles. Technology Monitoring Pack, Device / Network Pack, SaaS Monitoring Pack, Integration Pack and optional Customer Extension Pack are classifications/profiles of Capability Pack, not a competing framework. This remains **v3.2.2**; no v3.2.3 product-version number is created. `Platform_23` does not replace Device Pack contracts, `Platform_10` connector/security mechanics, `Platform_19` OIV, `Platform_20` onboarding, `Platform_21` enterprise-gap or `Platform_22` alert/AI-assurance authority.

The older files remain authoritative for their existing scope and are explicitly supplemented by specialized masters; they are not considered obsolete. This packaging choice is deliberate change control, not a waiver of traceability.

## 5. Precedence

When requirements appear to conflict:

1. BRD — business intent/non-negotiable outcomes.
2. PRD — customer-visible behavior.
3. Architecture — technology/deployment/security constraints.
4. HLD — subsystem boundaries.
5. LLD — implementation contracts.
6. Dataflow — end-to-end sequencing consistent with the above.
7. Specialized masters — detailed normative contracts in their assigned domain.
8. Decision Register — provenance/TBD control.

**OIV special case:** `Platform_19` is a consolidated specialized amendment whose product requirements act as authoritative PRD supplements and whose remaining sections carry corresponding architecture/HLD/LLD/dataflow/UI/security/quality/operations authority for that additive capability family. It may add behavior where existing files are silent, but may not silently override an explicit higher-precedence requirement.

**Onboarding special case:** `Platform_20` is a consolidated specialized amendment. Its `PRD-ONB-*` requirements act as authoritative BRD/PRD supplements; its logical-layer sections carry the corresponding architecture/HLD/LLD/dataflow/UI/security/quality/operations authority; its `REQ-ONB-*` catalogue is the formal RTM supplement.

**Enterprise gap-closure special case:** `Platform_21` is a consolidated specialized amendment. Its `PRD-EGC-*` requirements act as authoritative product supplements for its assigned enterprise capability family; its architecture/security/operability sections govern that scope and its `REQ-EGC-*` catalogue is the formal RTM supplement.

**Monitoring-intelligence special case:** `Platform_22` is a consolidated specialized amendment. Its `PRD-MIA-*` requirements act as authoritative product supplements for alert quality, evidence qualification and AI/RCA assurance; its architecture/security/operability sections govern that capability family and its `REQ-MIA-*` catalogue is the formal RTM supplement. It does not silently override Platform_19 OIV or Platform_21 enterprise-gap authority outside its assigned scope.

**Enterprise-monitoring-coverage special case:** `Platform_23` is a consolidated specialized amendment. Its `PRD-EMC-*` requirements act as authoritative product supplements for enterprise monitoring coverage, Capability Pack contract, dimensional certification and Customer Acceptance Profiles; its `REQ-EMC-*` catalogue is the formal RTM supplement. It does not silently override Platform_10 connector/security, Platform_19 OIV, Platform_20 onboarding, Platform_21 enterprise-gap or Platform_22 alert/AI-assurance authority outside its assigned scope.

Conflicts require documented change control; implementation must not silently choose a convenient interpretation.

## 6. Fixed product/architecture decisions

- Product name: **Platform**.
- Clean-room implementation; no ThoughtData/NetAI proprietary implementation reuse.
- Core/Sensor: **Ubuntu 24.x LTS family** and **RHEL 9.x family** first (subject to certification matrix); Windows Core/Sensor not initial scope.
- Standalone: one Core. HA: exactly two full Core nodes, Active/Standby, target <60s Core-service restoration.
- Sensors continue polling and locally buffer through Core failure/WAN loss while capacity permits; 80% storage protection may reclaim/shed eligible oldest lower-priority data and must expose data-loss risk. Absolute zero-loss is not promised.
- Sensor/Agent external channel: HTTPS/gRPC over mTLS; Kafka stays internal.
- Data stack: PostgreSQL authoritative control state, ClickHouse high-volume telemetry analytics, Kafka streaming backbone, Valkey cache/ephemeral state.
- Go Sensors/Agents; Java 21 Core; React/TypeScript UI; Python for selected NCM/AI/analytics workers.
- Core ≤2,500 reference devices: 8 vCPU/16 GB mandatory engineering target under SNWP-2500-v1.
- UI-driven provisioning: individual, CSV/XLSX bulk and network auto-discovery.
- Placement is locality + reachability + capacity + HA + security aware.
- Full always-on DPI/full APM remain excluded initially; **bounded triggered packet evidence** is an approved diagnostic capability and does not reopen the full-DPI decision.
- Graph-grounded hybrid RCA; GNN is an augmenting scorer, not causal proof; `INCONCLUSIVE` is required.
- Native Platform dashboard/UI; no embedded Grafana.
- Core and Sensor managed-storage 80% protection watermark is mandatory.
- Customer controls raw retention, rollups, archive and allowed purge priority.
- DR capability required in v1.0; contractual RPO/RTO remain TBD until frozen.
- Gemini through AI Gateway initially; deterministic/causal RCA remains independent.
- HTTPS-only Platform UI; no application listener on TCP/80.
- Development/test: Docker Compose standard; host laptop kept free of project-specific runtimes.
- Production packaging: Core/Sensor/Linux Agent `.deb` + `.rpm`; Windows Agent signed service EXE + MSI + BAT wrappers.
- Core, Sensors and Agents auto-start after reboot and emit structured diagnostics/logs.
- Canonical source repository: `https://github.com/gagandsm-max/platform`.
- First-class SaaS monitoring, cloud-native runtime dependency discovery, external deployment/change telemetry and carrier/ISP operational integration are approved product scope.
- Incident Flight Recorder, Similar Incident Memory and cross-domain Change Intelligence are approved MTTR-reduction capabilities.
- Controlled remediation is approved only with deterministic action definitions, authorization, preconditions, blast-radius controls, approval/policy, audit, post-action validation and rollback/fail-safe handling; no direct LLM privileged execution.
- Platform Value / MTTR Improvement measurement and the four-layer customer-value dashboard are approved product scope; unsupported causal or financial claims are prohibited.
- Network auto-discovery and Windows/Linux Agent enrollment use a governed review lifecycle; full monitoring/licence activation requires review by default unless authorized trusted auto-approval policy applies.
- Approve/reject/unmanaged/exclude onboarding decisions require operator comment/reason and audit; pre-activation review supports metadata, group/sub-group and monitoring/alert-policy preview.
- Agent-backed endpoint identity is anchored to canonical `entity_id` plus unique `agent_id`/mTLS identity; system/host name is human-readable technical identity and dynamic IP/MAC values are mutable observations.
- Platform exposes one global per-user Notification Center/bell with server-side tenant/RBAC/resource scoping; operational Alerts/Incidents remain distinct from platform/workflow notifications.
- Enterprise integrations normalize vendor-specific evidence into Platform-owned canonical contracts; vendor identifiers do not override tenant/entity authority.
- Existing seven-day licence grace remains governed behavior. Commercial quantities, packaging and pricing remain Founder-reserved.
- Declarative policy/configuration inputs cannot bypass Platform RBAC, validation, approvals, audit or runtime safety.
- Endpoint DEX is bounded operational-experience monitoring, not employee surveillance.
- Future regional federation does not reopen the v1 exactly-two-Core Active/Standby topology by implication.
- Material alert classes use class-specific evidence contracts and telemetry-quality trust checks before authoritative operational-failure assertions.
- Alert severity and evidentiary confidence are independent; semantic states such as `MONITORING_DEGRADED`, `REDUNDANCY_LOST`, `SLA_DEGRADED`, `EXPECTED_STATE`, `UNKNOWN` and `STALE` are preserved where applicable rather than collapsed into generic DOWN.
- GNN/LLM outputs are evidence/hypothesis assistance, never causal authority; `INCONCLUSIVE` remains valid and prompt/model text cannot grant privileged execution authority.
- Core monitoring and deterministic RCA/alerting continue without Internet/hosted AI availability; hosted/private/local/no-LLM operation is governed by explicit data-egress and fallback controls.
- Capability Pack is the canonical enterprise extension architecture. Technology Monitoring Pack, Device / Network Pack, SaaS Monitoring Pack, Integration Pack and optional Customer Extension Pack are classifications/profiles of that architecture, not independent frameworks. Existing Device Pack contracts remain valid under this model.
- Monitoring-technology certification is dimensional (Discovery/L1/L2/L3/Topology/NCM/Security/version). A family listed in `Platform_23` is a coverage target, not an implemented or certified pack.

## 7. OSS governance

Only components admitted by `Platform_07_OSS_Catalog.md` at an approved pinned version may ship.
Top-level licence alone is insufficient; transitive/bundled licences, notices, vulnerabilities and exact artifacts must be scanned.
Every release produces SPDX and CycloneDX SBOMs, third-party notices/licence texts, version locks, checksums and provenance.
Implementation dependencies must match the catalogue; catalogue omissions discovered in implementation require catalogue/change-control updates before milestone/release.

`Platform_19`, `Platform_20`, `Platform_21`, `Platform_22` and `Platform_23` do not by themselves select new runtime dependencies. Any new vector database, packet-analysis product, SaaS SDK, browser automation engine, probe runtime, AI/model runtime, vendor SDK, pack runtime or other dependency requires normal OSS catalogue/security/performance approval.

## 8. Completeness control

`Platform_08_Requirements_Traceability.md` remains the base formal guard against lost requirements.

For v3.2.2 OIV scope, `Platform_19` contains the authoritative `REQ-OIV-*` traceability supplement.
For v3.2.2 onboarding scope, `Platform_20` contains the authoritative `REQ-ONB-*` traceability supplement.
For v3.2.2 enterprise gap-closure scope, `Platform_21` contains the authoritative `REQ-EGC-*` traceability supplement mapped to `PRD-EGC-*` requirements and their governing architecture/security/operability clauses.
For v3.2.2 monitoring-intelligence/alert-quality/AI-assurance scope, `Platform_22` contains the authoritative `REQ-MIA-*` traceability supplement mapped to `PRD-MIA-*` requirements and their governing evidence/security/operability clauses.
For v3.2.2 enterprise-monitoring-coverage / Capability Pack certification scope, `Platform_23` contains the authoritative `REQ-EMC-*` traceability supplement mapped to `PRD-EMC-*` requirements. Documentation mapping is not implementation evidence.

A new requirement is considered incorporated only when it has an ID, affected-layer mapping and sufficient normative behavior for implementation without inventing a load-bearing decision. Specialized masters satisfy this requirement only within their explicitly assigned capability family.

## 9. Explicit unresolved/P0 items

Authoritative list is `Platform_12_Decision_Source_Register.md` §5. Restated here for navigation:

- exact two-Core Kafka implementation/failover while honoring no third full Core;
- exact two-Core ClickHouse replication/promotion/fencing design;
- exact PostgreSQL two-Core failover/fencing/rewind mechanics;
- exact Core service fencing/VIP implementation and failure matrix;
- final contractual DR RPO/RTO and data-loss objectives;
- final SNWP-2500-v1 metric/interface/event mix and certified performance results;
- Streaming Sensor certified Flow records/sec and Syslog EPS;
- exact GNN model architecture, training labels, inference latency, CPU/GPU footprint and production enablement threshold;
- final compliance certification program/scope and achieved certifications;
- exact product SKUs/licensing quantities beyond the agreed behavioral rules.

These remain unresolved until formally approved; no coding tool may invent them. Specialized-master additions do not resolve these P0/TBD items by declaration.

## 10. Development-stage readiness

Before coding a module, an engineer or coding agent reads `AGENTS.md`, this Index, the relevant BRD/PRD requirement, Architecture/HLD/LLD/Dataflow sections, OSS Catalogue, **all task-relevant specialized masters discovered from this Index**, and the Decision Register. Do not hard-code an upper specialized-master document number in agent/tooling logic.

For any OIV task, `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` is mandatory preflight reading.
For discovery review, Agent onboarding/enrollment identity, dynamic endpoint identity/reconciliation or in-product Notification Center work, `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md` is mandatory preflight reading.
For normalized SD-WAN, SCIM lifecycle, SLI/SLO/error budgets, audit tamper evidence, middleware/backup/SASE connectors, tenant offboarding/data portability, licensing safety, declarative configuration/policy-as-code, Incident Command Workspace, privacy-bounded endpoint DEX, browser synthetics, customer-operated probes or future regional federation, `Platform_21_Enterprise_Gap_Closure_Master.md` is mandatory preflight reading.
For alert evidence/qualification, semantic monitoring states, alert-policy lifecycle/quality, active evidence acquisition, RCA confidence/contradictions, AI evaluation/safety, Incident Copilot, AI provider/egress controls, prompt-injection/tool-use protection, tenant-scoped learning, shared-service dependency evidence, PKI assurance or AI observability, `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` is mandatory preflight reading.
For enterprise monitoring-domain catalogue, Capability Pack contract/lifecycle, dimensional L1/L2/L3 certification, Customer Acceptance Profiles, Real Customer Requirements normalization, operational-graph vs governed-CMDB semantics, or coverage-measurement work, `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` is mandatory preflight reading.

A feature is not complete when it only compiles: tests, observability, error/failure behavior, upgrade/migration impact, security/OSS implications, traceability and acceptance evidence are part of Definition of Done.

## 11. Mother-document change control (READ-MOSTLY)

Mother documents are **READ-MOSTLY** and authoritative.
Coding agents must **not casually** modify them while implementing features.

A mother document **MUST** be updated when:

1. a factual/documentation error is identified;
2. two mother documents contradict one another;
3. an approved product decision changes;
4. an approved architecture decision changes;
5. an implementation dependency becomes formally selected;
6. a P0/TBD item is resolved;
7. a requirement was accidentally omitted;
8. an implemented behavior would otherwise conflict with documented truth.

Such changes require deliberate change control, cross-document reconciliation, Decision Register update where a decision changes, and review through the canonical GitHub repository. No implementation is allowed to permanently diverge from the mother-reference truth. If implementation discovers an error in the mother documents, engineering must correct the mother documents rather than knowingly preserve a false specification.

## 12. Canonical repository and working clone

**USER-FIXED:** Canonical Platform source repository is `https://github.com/gagandsm-max/platform`.

- GitHub is the authoritative repository/system of record.
- Local `~/nms` (or any other clone path) is a **working clone**, not an independent source of truth.
- Code, build, test and mother-document changes become authoritative only after being pushed and merged into the canonical GitHub repository.
- Normal development uses short-lived branches.
- Material changes use pull requests.
- Do not directly push normal development changes to the protected/default branch.
- Commits must be traceable to the implementation/change being made.
- Do not force-push shared/default branches.
- GitHub history must preserve architecture/document decision provenance.
- Selecting GitHub as source of record does **not** by itself freeze a CI provider. GitHub Actions may be evaluated later; it is not a fixed architecture decision unless separately approved.

## 13. Repository placement

For ChatGPT Project use, these **25 Markdown mother-reference files** are sufficient as the authoritative source set.
For code-agent enforcement, `AGENTS.md` should also be available at repository root (`~/nms/AGENTS.md` in a working clone), preferably as the existing symlink to `project_documentation/AGENTS.md`, so tools that auto-discover root instructions see the same content.
Authority of the clone follows §12 after merge to the canonical GitHub repository.

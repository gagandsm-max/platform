# Platform — Mother Reference Documentation Set

**Document ID:** PLATFORM-INDEX-001  
**Version:** 3.2.2  
**Status:** **Authoritative Mother Reference — 21-File Consolidated Engineering Source of Truth**  
**Product:** Platform  
**Project independence:** Clean-room product; separate from ThoughtData, Enterprise360 and NetAI  
**Last reconciliation:** 2026-09-07 — v3.2.2 additive operational-intelligence/MTTR/customer-value scope expansion

## 1. Authority

**v3.2.2 supersedes v3.2.1 as the authoritative Platform mother-reference set.**

v3.2.2 is an additive product/engineering scope revision. It introduces `Platform_19_Operational_Intelligence_MTTR_and_Value.md` as an authoritative cross-cutting master for approved operational-intelligence, MTTR-reduction and customer-value capabilities. Existing v3.2.1 requirements remain normative unless explicitly changed by v3.2.2 change control.

v3.2.1 superseded v3.2 as a corrective/governance patch. v3.2 superseded v3.1 as a documentation-packaging revision. No product requirement from earlier authoritative revisions is intentionally removed by this update.

Earlier v1/v2/v3.0/v3.1 generated master packages and the legacy `Platform_Master_Architecture_v1.0.docx` are superseded as Platform engineering sources of truth. Original ThoughtData/Enterprise360, NetAI, SolarWinds, ManageEngine and other supplied reference material remains reference/evidence only and never overrides this set.

### 1.1 v3.2.2 supplement rule

`Platform_19_Operational_Intelligence_MTTR_and_Value.md` is normative for requirements introduced by the 2026-09-07 approved scope expansion. It intentionally contains product, architecture, HLD/LLD/dataflow, security, API/data and quality requirements for its cross-cutting domain so engineering may implement without inventing missing contracts.

If Platform_19 conflicts with a higher-precedence existing BRD/PRD/Architecture statement, implementation stops and normal change control resolves the conflict. Absence of older text is not a conflict where Platform_19 is purely additive.

## 2. No-assumption rule

- Explicit product-owner decisions remain fixed until change control changes them.
- Delegated engineering choices are architecture decisions and must be recorded.
- Unknown values remain `TBD — Decision Required` or `P0 ENGINEERING SPIKE`; they are not guessed.
- Capacity statements remain engineering targets until benchmark-certified.
- Security/compliance readiness must never be represented as achieved certification until independently achieved.
- MTTR, percentage-improvement and economic-benefit claims are evidence claims, not marketing defaults; methodology and customer-supplied assumptions must be disclosed.

## 3. Exact 21-file mother-reference set

| # | File | Governing scope |
|---:|---|---|
| 1 | `AGENTS.md` | Coding-agent/engineer navigation and non-negotiable repository rules |
| 2 | `Platform_00_Document_Index.md` | Authority, precedence, file map, change/completeness rules |
| 3 | `Platform_01_BRD.md` | Business intent, outcomes, scope, principles, commercial/security/continuity requirements |
| 4 | `Platform_02_PRD.md` | Complete customer-visible functional and non-functional product requirements |
| 5 | `Platform_03_Architecture.md` | Governing architecture, technology, deployment, HA/data/Sensor/AI/security decisions |
| 6 | `Platform_04_HLD.md` | Subsystems, deployment units, interfaces, ownership and failure boundaries |
| 7 | `Platform_05_LLD.md` | Concrete schemas, topics, state machines, adapters, algorithms and implementation rules |
| 8 | `Platform_06_Dataflow.md` | End-to-end telemetry/control/HA/RCA/NCM/storage/integration sequences |
| 9 | `Platform_07_OSS_Catalog.md` | Authoritative OSS allow/candidate list, licences, custom boundaries and governance |
| 10 | `Platform_08_Requirements_Traceability.md` | Completeness and requirement traceability control |
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
| 21 | `Platform_19_Operational_Intelligence_MTTR_and_Value.md` | SaaS/carrier/change enrichment, Flight Recorder, Similar Incident Memory, controlled remediation, MTTR/value evidence and customer value dashboard |

## 4. Consolidation and revision map

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

### 4.2 v3.2.1 → v3.2.2

- Existing 20 v3.2.1 mother files remain carried forward unless explicitly edited by this revision.
- `Platform_19_Operational_Intelligence_MTTR_and_Value.md` is added as file 21 because the approved scope is sufficiently cross-cutting that compressing it into one older specialized master would obscure implementation/security/value contracts.
- A later packaging-only consolidation may fold Platform_19 into older masters without relaxing its requirements.

## 5. Precedence

When requirements appear to conflict:

1. BRD — business intent/non-negotiable outcomes.
2. PRD — customer-visible behavior.
3. Architecture — technology/deployment/security constraints.
4. HLD — subsystem boundaries.
5. LLD — implementation contracts.
6. Dataflow — end-to-end sequencing consistent with the above.
7. Specialized masters — detailed normative contracts in their assigned domain, including Platform_19 for its new cross-cutting scope.
8. Decision Register — provenance/TBD control.

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
- Full always-on DPI and full APM product positioning remain excluded initially; bounded incident-triggered packet evidence and cloud-native dependency discovery are now included under Platform_19.
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
- First-class SaaS monitoring, carrier/ISP enrichment, external deployment/change telemetry, Incident Flight Recorder, Similar Incident Memory and cross-domain Change Intelligence are V1 requirements under Platform_19.
- Controlled remediation is V1 for bounded, policy/RBAC/approval-gated action classes. Unsupervised autonomous closed-loop and direct LLM privileged execution remain prohibited.
- Platform Value/MTTR measurement and the four-layer customer-value dashboard are V1 requirements; unsupported customer-benefit percentages or economic claims are prohibited.

## 7. OSS governance

Only components admitted by `Platform_07_OSS_Catalog.md` at an approved pinned version may ship. Top-level licence alone is insufficient; transitive/bundled licences, notices, vulnerabilities and exact artifacts must be scanned. Every release produces SPDX and CycloneDX SBOMs, third-party notices/licence texts, version locks, checksums and provenance.

The v3.2.2 scope does not silently approve eBPF runtimes, packet-capture libraries, new SaaS SDKs or carrier SDKs. Any new runtime/build dependency follows normal catalogue and security review before merge.

## 8. Completeness control

`Platform_08_Requirements_Traceability.md` is the formal guard against lost requirements.

A new requirement is incorporated only when it has an ID, authoritative document mapping and necessary implementation/security/quality coverage. Platform_19 deliberately contains the product-to-quality contract for its requirement family; RTM maps these requirements explicitly.

Documentation mapping is not implementation, validation or GA evidence.

## 9. Explicit unresolved/P0 items

Authoritative legacy list remains `Platform_12_Decision_Source_Register.md` §5:

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

v3.2.2 does not resolve those P0s. It also deliberately leaves exact initial SaaS/provider/carrier pack catalogue, packet-capture engine/library and optional eBPF mechanism to implementation/OSS/security selection; engineering must not invent external support claims before certification.

## 10. Development-stage readiness

Before coding a module, an engineer or coding agent reads `AGENTS.md`, this Index, relevant BRD/PRD requirements, Architecture/HLD/LLD/Dataflow sections, OSS Catalogue, relevant specialized masters, Platform_19 when the task touches operational intelligence/value, and the Decision Register.

A feature is not complete when it only compiles: tests, observability, error/failure behavior, upgrade/migration impact, security/OSS implications, traceability and acceptance evidence are part of Definition of Done.

## 11. Mother-document change control (READ-MOSTLY)

Mother documents are **READ-MOSTLY** and authoritative. Coding agents must not casually modify them while implementing features.

A mother document **MUST** be updated when:

1. a factual/documentation error is identified;
2. two mother documents contradict one another;
3. an approved product decision changes;
4. an approved architecture decision changes;
5. an implementation dependency becomes formally selected;
6. a P0/TBD item is resolved;
7. a requirement was accidentally omitted;
8. an implemented behavior would otherwise conflict with documented truth.

Such changes require deliberate change control, cross-document reconciliation, Decision Register update where a decision changes, and review through the canonical GitHub repository. No implementation is allowed to permanently diverge from the mother-reference truth.

## 12. Canonical repository and working clone

**USER-FIXED:** Canonical Platform source repository is `https://github.com/gagandsm-max/platform`.

- GitHub is the authoritative repository/system of record.
- Local `~/nms` (or any other clone path) is a working clone, not an independent source of truth.
- Code, build, test and mother-document changes become authoritative only after being pushed and merged into the canonical GitHub repository.
- Normal development uses short-lived branches.
- Material changes use pull requests.
- Do not directly push normal development changes to the protected/default branch.
- Commits must be traceable to the implementation/change being made.
- Do not force-push shared/default branches.
- GitHub history must preserve architecture/document decision provenance.
- Selecting GitHub as source of record does not by itself freeze a CI provider.

## 13. Repository placement

For ChatGPT Project use, these 21 Markdown files are the authoritative source set for v3.2.2.

For code-agent enforcement, `AGENTS.md` should also be available at repository root of the working clone, preferably as a copy or symlink to `project_documentation/AGENTS.md`, so tools that auto-discover root instructions see the same content. Authority of the clone follows §12 after merge to the canonical GitHub repository.

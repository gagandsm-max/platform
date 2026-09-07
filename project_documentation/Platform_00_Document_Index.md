# Platform — Mother Reference Documentation Set

**Document ID:** PLATFORM-INDEX-001  
**Version:** 3.2.2  
**Status:** **Authoritative Mother Reference — 21-File Consolidated Engineering Source of Truth**  
**Product:** Platform  
**Project independence:** Clean-room product; separate from ThoughtData, Enterprise360 and NetAI  
**Last reconciliation:** 2026-09-07 — v3.2.2 operational-intelligence, MTTR and customer-value scope addition

## 1. Authority

**v3.2.2 supersedes v3.2.1 as the authoritative Platform mother-reference set after merge to the canonical GitHub `main`.**

v3.2.2 is an additive product/engineering scope revision. It introduces one new specialized mother document, `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md`, promoted to authoritative status for operational intelligence, MTTR reduction and customer-value requirements. It does not intentionally remove or weaken any v3.2.1 requirement.

v3.2.1 superseded v3.2 as a corrective/governance patch. v3.2 superseded v3.1 as a documentation-packaging revision. Earlier v1/v2/v3.0/v3.1 generated master packages and the legacy `Platform_Master_Architecture_v1.0.docx` are superseded as Platform engineering sources of truth.

Original ThoughtData/Enterprise360, NetAI, SolarWinds, ManageEngine and other supplied reference material remains reference/evidence only and never overrides this set.

## 2. No-assumption rule

- Explicit product-owner decisions remain fixed until change control changes them.
- Delegated engineering choices are architecture decisions and must be recorded.
- Unknown values remain `TBD — Decision Required` or `P0 ENGINEERING SPIKE`; they are not guessed.
- Capacity statements remain engineering targets until benchmark-certified.
- Security/compliance readiness must never be represented as achieved certification until independently achieved.
- Customer-value/MTTR percentages are not product claims until supported by disclosed reproducible customer/pilot evidence.

## 3. Exact 21-file mother-reference set

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

### 4.2 v3.2.1 → v3.2.2 addition

v3.2.2 adds `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` as an authoritative specialized master instead of duplicating the same OIV text across many legacy files.

For the OIV capability family, `Platform_19` contains logical business/product, architecture, HLD, LLD/data, dataflow, UI, security, capacity, quality, operations and RTM sections. Its formal `PRD-OIV-*` and `REQ-OIV-*` IDs are authoritative supplements to the base PRD/RTM catalogues.

The older files remain authoritative for their existing scope and are explicitly supplemented by `Platform_19`; they are not considered obsolete.

This packaging choice is deliberate change control, not a waiver of traceability. `Platform_19` §14 maps every OIV requirement to its governing logical layers, and `AGENTS.md` makes the specialized master mandatory preflight for OIV work.

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

**OIV special case:** `Platform_19` is a consolidated specialized amendment that explicitly labels the logical layer of each section. Its §3 `PRD-OIV-*` requirements act as authoritative PRD supplements; §§4–13 carry the corresponding architecture/HLD/LLD/dataflow/UI/security/quality/operations authority for that additive capability family. It may add behavior where v3.2.1 files are silent, but may not silently override an explicit higher-precedence existing requirement.

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
- Full always-on DPI/full APM remain excluded initially; **bounded triggered packet evidence** is now an approved diagnostic capability and does not reopen the full-DPI decision.
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

## 7. OSS governance

Only components admitted by `Platform_07_OSS_Catalog.md` at an approved pinned version may ship.
Top-level licence alone is insufficient; transitive/bundled licences, notices, vulnerabilities and exact artifacts must be scanned.
Every release produces SPDX and CycloneDX SBOMs, third-party notices/licence texts, version locks, checksums and provenance.
Implementation dependencies must match the catalogue; catalogue omissions discovered in implementation require catalogue/change-control updates before milestone/release.

`Platform_19` deliberately does not select a new vector database, packet-analysis product or SaaS SDK dependency. Any such dependency requires normal OSS catalogue approval.

## 8. Completeness control

`Platform_08_Requirements_Traceability.md` remains the base formal guard against lost requirements.

For v3.2.2 OIV scope, `Platform_19` §14 is an **authoritative requirements-traceability supplement** containing formal `REQ-OIV-*` IDs mapped to formal `PRD-OIV-*` IDs and the product/architecture/HLD/LLD/dataflow/UI/security/quality sections that govern them.

A new requirement is considered incorporated only when it has an ID, affected-layer mapping and sufficient normative behavior for implementation without inventing a load-bearing decision. `Platform_19` satisfies this for the approved OIV scope through its consolidated logical-layer contract.

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

These remain unresolved until formally approved; no coding tool may invent them.
The new OIV scope does not resolve any of these P0/TBD items by declaration.

## 10. Development-stage readiness

Before coding a module, an engineer or coding agent reads `AGENTS.md`, this Index, the relevant BRD/PRD requirement, Architecture/HLD/LLD/Dataflow sections, OSS Catalogue, relevant specialized master (`09` through `19`) and Decision Register.

For any OIV task, `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` is mandatory preflight reading.

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

Such changes require deliberate change control, cross-document reconciliation, Decision Register update where a decision changes, and review through the canonical GitHub repository.
No implementation is allowed to permanently diverge from the mother-reference truth.
If implementation discovers an error in the mother documents, engineering must correct the mother documents rather than knowingly preserve a false specification.

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

For ChatGPT Project use, these **21 Markdown mother-reference files** are sufficient as the authoritative source set.
For code-agent enforcement, `AGENTS.md` should also be available at repository root (`~/nms/AGENTS.md` in a working clone), preferably as a copy or symlink to `project_documentation/AGENTS.md`, so tools that auto-discover root instructions see the same content.
Authority of the clone follows §12 after merge to the canonical GitHub repository.

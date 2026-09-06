# Platform — Mother Reference Documentation Set

**Document ID:** PLATFORM-INDEX-001  
**Version:** 3.2  
**Status:** **Authoritative Mother Reference — 20-File Consolidated Engineering Source of Truth**  
**Product:** Platform  
**Project independence:** Clean-room product; separate from ThoughtData, Enterprise360 and NetAI  
**Last reconciliation:** 2026-09-06 — v3.2 file-count consolidation for engineering/ChatGPT Project use

## 1. Authority

This v3.2 set supersedes v3.1 as a **documentation-packaging revision**. No product requirement from v3.1 is intentionally removed.
The reduction from the v3.1 file set to 20 Markdown files is achieved only by consolidating closely related engineering
governance documents.

Earlier v1/v2/v3.0/v3.1 generated master packages and the legacy
`Platform_Master_Architecture_v1.0.docx` are superseded as Platform engineering sources of truth.
Original ThoughtData/Enterprise360, NetAI, SolarWinds, ManageEngine and other supplied reference material remains
reference/evidence only and never overrides this set.

## 2. No-assumption rule

- Explicit product-owner decisions remain fixed until change control changes them.
- Delegated engineering choices are architecture decisions and must be recorded.
- Unknown values remain `TBD — Decision Required` or `P0 ENGINEERING SPIKE`; they are not guessed.
- Capacity statements remain engineering targets until benchmark-certified.
- Security/compliance readiness must never be represented as achieved certification until independently achieved.

## 3. Exact 20-file mother-reference set

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

## 4. v3.1 → v3.2 consolidation map

| v3.1 document(s) | v3.2 destination |
|---|---|
| `Platform_13_Engineering_Execution_Standards.md` + `Platform_14_Docker_Development_Environment.md` | `Platform_13_Engineering_Development_and_Docker_Standards.md` |
| `Platform_15_Test_Strategy_and_Quality_Gates.md` + `Platform_22_Performance_and_Benchmark_Plan.md` + `Platform_24_Release_Acceptance_Criteria.md` | `Platform_14_Test_Performance_and_Release_Quality.md` |
| `Platform_16_CI_CD_and_Release_Engineering.md` + `Platform_18_Packaging_and_Distribution.md` | `Platform_15_CI_CD_Packaging_and_Distribution.md` |
| `Platform_17_Installation_Upgrade_Rollback_Runbook.md` + `Platform_23_Operational_Runbooks.md` | `Platform_16_Installation_Upgrade_Rollback_and_Operations.md` |
| `Platform_19_API_and_Event_Contract_Master.md` + `Platform_20_Data_Model_and_Schema_Master.md` | `Platform_17_API_Event_and_Data_Model_Contracts.md` |
| `Platform_21_Security_Threat_Model.md` | `Platform_18_Security_Threat_Model.md` |

Every source section is retained in the destination document. Consolidation is not permission to drop a requirement.

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

Conflicts require documented change control; implementation must not silently choose a convenient interpretation.

## 6. Fixed product/architecture decisions

- Product name: **Platform**.
- Clean-room implementation; no ThoughtData/NetAI proprietary implementation reuse.
- Core/Sensor: Ubuntu LTS and RHEL first; Windows Core/Sensor not initial scope.
- Standalone: one Core. HA: exactly two full Core nodes, Active/Standby, target <60s Core-service restoration.
- Sensors continue polling and locally buffer through Core failure/WAN loss.
- Sensor/Agent external channel: HTTPS/gRPC over mTLS; Kafka stays internal.
- Data stack: PostgreSQL authoritative control state, ClickHouse high-volume telemetry analytics, Kafka streaming backbone, Valkey cache/ephemeral state.
- Go Sensors/Agents; Java 21 Core; React/TypeScript UI; Python for selected NCM/AI/analytics workers.
- Core ≤2,500 reference devices: 8 vCPU/16 GB mandatory engineering target under SNWP-2500-v1.
- UI-driven provisioning: individual, CSV/XLSX bulk and network auto-discovery.
- Placement is locality + reachability + capacity + HA + security aware.
- DPI/full APM excluded initially; OTel metrics/logs, synthetics, DB, cloud, K8s, network and infrastructure included.
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

## 7. OSS governance

Only components admitted by `Platform_07_OSS_Catalog.md` at an approved pinned version may ship.
Top-level licence alone is insufficient; transitive/bundled licences, notices, vulnerabilities and exact artifacts must be scanned.
Every release produces SPDX and CycloneDX SBOMs, third-party notices/licence texts, version locks, checksums and provenance.

## 8. Completeness control

`Platform_08_Requirements_Traceability.md` is the formal guard against lost requirements.
A new requirement is not considered incorporated until it has an ID, affected-document mapping and necessary updates to
product, architecture, implementation/dataflow and test/acceptance documentation.

## 9. Explicit unresolved/P0 items

- Exact two-Core Kafka failover implementation without a third full Core.
- Exact two-Core ClickHouse replication/promotion/fencing implementation.
- Exact PostgreSQL two-Core failover/fencing/rewind mechanics.
- Exact HA endpoint/VIP/fencing failure matrix.
- Contractual DR RPO/RTO/data-loss objectives.
- Final SNWP-2500-v1 workload constants and certified benchmark results.
- Certified Streaming Sensor Flow records/sec and Syslog EPS.
- Final production GNN runtime/model/compute profile.
- Achieved compliance certifications and final certification scope.

These remain unresolved until formally approved; no coding tool may invent them.

## 10. Development-stage readiness

Before coding a module, an engineer or coding agent reads `AGENTS.md`, this Index, the relevant PRD requirement,
Architecture/HLD/LLD/Dataflow sections, OSS Catalogue, specialized master and Decision Register.
A feature is not complete when it only compiles: tests, observability, error/failure behavior, upgrade/migration impact,
security/OSS implications, traceability and acceptance evidence are part of Definition of Done.

## 11. Repository placement

For ChatGPT Project use, these 20 Markdown files are sufficient as the authoritative source set.
For code-agent enforcement, `AGENTS.md` should also be available at repository root (`~/nms/AGENTS.md`), preferably as a
copy or symlink to `project_documentation/AGENTS.md`, so tools that auto-discover root instructions see the same content.

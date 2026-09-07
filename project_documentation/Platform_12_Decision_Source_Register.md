# Platform — Decision, Source & Assumption-Control Register

**Document ID:** PLATFORM-DECISION-001  
**Version:** 3.2.2  
**Status:** **Authoritative Mother Reference — Decision Provenance**

## 1. Purpose

This register exists to enforce the instruction **do not assume**. Every material statement in the mother-reference set must be one of `USER-FIXED`, `ARCH-DELEGATED`, `REFERENCE-LEARNING`, or `TBD/P0-SPIKE`.

Platform is a clean-room product independent of ThoughtData/Enterprise360 and NetAI. Supplied material is learning/reference only; proprietary code/assets are not reused.

## 2. Governing USER-FIXED decisions

| Decision | Classification | Governing statement |
|---|---|---|
| Product name | USER-FIXED | New product/project is **Platform**. |
| Independence | USER-FIXED | Build from scratch; separate from ThoughtData and NetAI. |
| Scope | USER-FIXED | Network + Infrastructure + DB + Cloud + Kubernetes + OTel metrics/logs + synthetics; full always-on DPI and full APM excluded initially. |
| Core HA | USER-FIXED | Exactly two full Core nodes; one Active, one Standby; continuous sync; failover target <60 seconds. |
| Core install | USER-FIXED | Separate Core build; installer chooses Standalone/HA and pairs two Cores. |
| Sensor/Agent install | USER-FIXED | Separate Sensor and Agent builds; enroll against Core; Core directs Agent to appropriate Sensor. |
| OS | USER-FIXED | Core/Sensor Ubuntu and RHEL; no Windows Core/Sensor initially. Windows/Linux Agents supported. |
| Sensor locality | USER-FIXED | Sensors should be close to devices but placement must also avoid overload and respect capacity/security. |
| Provisioning | USER-FIXED | UI-driven individual, CSV/XLSX bulk and auto-discovery with actionable diagnostics. |
| Security protocols | USER-FIXED | Enterprise protocol/security choices exposed through Platform. |
| Storage protection | USER-FIXED | Core and Sensors protect at 80%; live monitoring is prioritized over eligible historical data. |
| Retention | USER-FIXED | Admin controls retention/rollups. |
| DR | USER-FIXED | DR architecture mandatory in v1.0. |
| AI | USER-FIXED | Gemini via AI Gateway initially; deterministic/causal RCA remains independent. |
| Data stack | USER-FIXED | Kafka + ClickHouse + PostgreSQL + Valkey. |
| Languages | USER-FIXED | Go Sensors, Java 21 Core, React/TypeScript UI, Python where appropriate. |
| Licence expiry | USER-FIXED | 30-day warning; 7-day banner; 7-day default grace; historical data remains after polling stop. |
| Security posture | USER-FIXED | Design for banks, defence and global enterprises; no false certification claims. |
| UI | USER-FIXED | Excellent/easy/fast/readable UX; HTTPS only; no app port 80. |
| Master docs | USER-FIXED | Split mother-reference set authoritative; no silent assumptions. |
| Canonical source repository | USER-FIXED | `https://github.com/gagandsm-max/platform` is system of record; local clones are working copies; material changes use PRs; no force-push of shared/default branches. |
| Mother-document change control | USER-FIXED | Mother documents are READ-MOSTLY but must be corrected under deliberate change control when decisions/requirements/errors/conflicts require it. |
| First-class SaaS monitoring | USER-FIXED | Add reusable/certified SaaS monitoring packs/connectors. |
| Triggered packet evidence | USER-FIXED | Add bounded privileged incident-triggered packet diagnostics without reopening full always-on DPI. |
| Cloud-native runtime dependencies | USER-FIXED | Add runtime dependency discovery from Kubernetes/OTel/service-mesh/cloud evidence with provenance/confidence. |
| External change/deployment telemetry | USER-FIXED | Add normalized CI/CD, release, IaC, cloud, Kubernetes, DB, certificate, patch/reboot and ITSM change evidence. |
| Carrier/ISP operational integration | USER-FIXED | Add provider outage/maintenance/circuit/ticket/SLA evidence where integrations permit; provider status is evidence, not causal proof. |
| Incident Flight Recorder | USER-FIXED | Add bounded before/during/after incident evidence preservation. |
| Similar Incident Memory | USER-FIXED | Add tenant-scoped historical incident similarity/outcomes; advisory only. |
| Cross-domain Change Intelligence | USER-FIXED | Expand change intelligence beyond NCM to network, cloud, app, DB, Kubernetes, platform and ITSM changes. |
| Controlled remediation | USER-FIXED | Add diagnose → recommend → precondition → impact preview → authorize/approve → execute → validate → rollback/escalate. No LLM-direct privileged execution. |
| Platform Value / MTTR Engine | USER-FIXED | Add evidence-backed MTTD/MTTA/time-to-root/MTTR measurement and matched-cohort comparison; no unsupported causal/financial claims. |
| Four-layer customer value dashboard | USER-FIXED | Show noise reduction, diagnosis effectiveness, incident improvement and evidence-supported operational/economic benefit with methodology. |

## 3. ARCH-DELEGATED decisions

| Decision | Architecture selection | Status |
|---|---|---|
| Core target ≤2,500 devices | 8 vCPU / 16 GB under versioned SNWP-2500-v1 | Mandatory engineering target pending benchmark certification |
| Core modularity | Modular Core rather than excessive microservices | Fixed architecture direction |
| Sensor implementation language | Go; Rust only for benchmark-proven hotspot | Fixed architecture direction |
| Sensor planning | 4C/16G/150GB ~800 normal devices baseline; 8C/16G ~2,000–2,500 benchmark target | Baseline + objective |
| Sensor transport | Sensor/Agent → HTTPS/gRPC mTLS ingestion → Kafka internal | Fixed architecture direction |
| High-rate events | Dedicated Streaming/Event Sensor when volume threatens polling | Fixed architecture direction |
| Telemetry DB | ClickHouse primary high-volume telemetry/analytics store | Fixed architecture direction |
| Optional search | OpenSearch only later if proven need | Optional |
| Identity foundation | Keycloak with Platform-owned resource authorization | Selected subject to OSS scans |
| GNN | PyTorch/PyG candidates; GNN is hypothesis/impact scorer, not causal oracle | Candidate pending benchmark/license review |
| OIV packaging | `Platform_19` carries consolidated product/architecture/HLD/LLD/dataflow/UI/security/quality/RTM additions | Fixed v3.2.2 documentation architecture |
| OIV service shape | Extend existing modular Core/Sensor domains; no monolithic AI service/new DB by default | Fixed architecture direction |
| Similar Incident storage/search | Use existing PostgreSQL/ClickHouse/analytics first; no dedicated vector DB without evidence/OSS approval | Fixed architecture direction |
| Packet artifact storage | Bounded encrypted artifact abstraction with metadata refs; raw packets not indiscriminately stored in PG/CH | Fixed architecture direction |

## 4. REFERENCE-LEARNING register

### 4.1 NetAI / graph-first learning

Retained conceptual lessons: topology as reasoning graph; temporal/topological/protocol correlation; root cause + evidence + causal chain + blast radius; anomaly context; LLM as interaction/explanation rather than causal authority. Platform implements independently.

### 4.2 SolarWinds learning

Retained lessons: horizontal collection scale, two application Core nodes for HA, separation of collection/control, future federation.

### 4.3 ManageEngine learning

Retained lessons: central Core + distributed Sensors, multiple Sensors per site, centralized policy, local continuity and asynchronous ingestion.

### 4.4 Legacy ThoughtData/Enterprise360 learning

Legacy material informed broad monitoring coverage only. Platform is clean-room and does not inherit proprietary source or claims.

## 5. Explicit TBD/P0 Engineering Spikes

- exact two-Core Kafka implementation/failover;
- exact two-Core ClickHouse replication/promotion/fencing;
- exact PostgreSQL two-Core failover/fencing/rewind;
- exact Core service fencing/VIP failure matrix;
- final DR RPO/RTO/data-loss objectives;
- final SNWP-2500-v1 workload constants/performance results;
- Streaming Sensor certified Flow/Syslog performance;
- exact GNN production model/runtime/labels/latency/compute threshold;
- final compliance certification programme/scope/achievements;
- exact product SKUs/licensing quantities beyond agreed behavior.

No implementation may invent values for these.

## 6. v3.1 development decisions

Docker-only development/testing, Docker Compose, reboot-safe named volumes, `.deb`+`.rpm` Linux packaging, signed Windows Service/MSI/BAT wrappers, auto-start, supervised recovery/logging, and isolated Windows certification remain fixed as previously recorded.

## 7. v3.2.1 governance decisions

Canonical GitHub source-of-record, READ-MOSTLY mother-document correction governance and bootstrap OSS catalogue admissions remain fixed as previously recorded.

## 8. v3.2.2 OIV decisions

| Decision | Classification | Governing statement |
|---|---|---|
| OIV product scope | USER-FIXED | `PRD-OIV-001` through `PRD-OIV-014` in `Platform_19` are approved additive requirements. |
| OIV traceability | ARCH-DELEGATED | `REQ-OIV-001` through `REQ-OIV-014` in `Platform_19` §14 are the authoritative v3.2.2 RTM supplement. |
| Packet evidence vs DPI | USER-FIXED | Bounded incident diagnostics in scope; full always-on DPI still excluded initially. |
| Remediation safety | USER-FIXED | No LLM-direct privileged execution; authorization, preconditions, blast radius, approval/policy, audit, validation and rollback/fail-safe are mandatory. |
| Value claims | USER-FIXED | Evidence-backed customer-specific metrics allowed; unsupported generic MTTR/financial-savings percentages prohibited. |
| OIV architecture reuse | ARCH-DELEGATED | Extend existing Incident/RCA/Graph/Integration/NCM/Reporting/Sensor modules and data stack before adding mandatory services/databases. |

These additions do not resolve §5 P0/TBD items.

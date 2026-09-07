# Platform — Decision, Source & Assumption-Control Register

**Document ID:** PLATFORM-DECISION-001  
**Version:** 3.2.2  
**Status:** **Authoritative Mother Reference — Decision Provenance**

## 1. Purpose

This register exists to enforce the instruction **do not assume**. Every material statement in the mother-reference set must be one of:

- **USER-FIXED** — explicitly decided by the product owner in Platform design conversations;
- **ARCH-DELEGATED** — product owner delegated the decision to the CTO/architecture function;
- **REFERENCE-LEARNING** — lesson learned from supplied competitor/legacy material; useful for design but not copied and not itself a Platform requirement unless separately adopted;
- **TBD/P0-SPIKE** — unresolved and explicitly not invented.

Platform is a clean-room product independent of ThoughtData/Enterprise360 and NetAI. Supplied material is learning/reference only; proprietary code/assets are not reused.

## 2. Governing USER-FIXED decisions

| Decision | Classification | Governing statement |
|---|---|---|
| Product name | USER-FIXED | New product/project is **Platform**; do not call it Enterprise360 except when discussing legacy/reference material. |
| Independence | USER-FIXED | Build from scratch; separate from ThoughtData and NetAI. |
| Scope | USER-FIXED | Network + Infrastructure + DB + Cloud + Kubernetes + OTel metrics/logs + synthetics + first-class SaaS operational monitoring + carrier/ISP operational enrichment + cross-domain operational intelligence. Full always-on DPI and full APM product positioning remain excluded initially. Bounded incident-triggered packet evidence is included. |
| Core HA | USER-FIXED | Exactly two full Core nodes; one Active, one Standby; continuous sync; failover target <60 seconds. |
| Core install | USER-FIXED | Separate Core build; installer chooses Standalone/HA and pairs two Cores through supplied peer/HA addressing. |
| Sensor/Agent install | USER-FIXED | Separate Sensor and Agent builds; install against current master Core, handshake, download effective config; Core directs Agent to appropriate Sensor. |
| OS | USER-FIXED | Core/Sensor Ubuntu and RHEL; no Windows Core/Sensor initially. Windows/Linux Agents supported. |
| Sensor locality | USER-FIXED | Pollers/Sensors should be close to devices, but assignment must also avoid overload and respect available nearby capacity. |
| Provisioning | USER-FIXED | UI-driven individual, CSV/XLSX bulk and network auto-discovery; actionable failure/remediation; optional name/description with auto-derived name when blank. |
| Security protocols | USER-FIXED | Network provisioning exposes and supports configurable enterprise protocol/security options through Platform. |
| Storage protection | USER-FIXED | Core and Sensors protect at 80%; monitoring must continue during storage pressure and eligible oldest data/logs are reclaimed. |
| Retention | USER-FIXED | Admin controls raw retention, DB retention, rollup interval/resolution and retention. |
| DR | USER-FIXED | DR architecture is mandatory in v1.0. |
| AI | USER-FIXED | Gemini initially through AI Gateway; meaningful AI only; deterministic/causal RCA independent; local/private/offline later. |
| Data stack | USER-FIXED | Kafka + ClickHouse + PostgreSQL + Valkey. |
| Languages | USER-FIXED | Go Sensors, Java 21 Core, React/TypeScript UI, Python for AI/analytics/NCM where required. |
| Licence expiry | USER-FIXED | 30-day warning; dashboard banner from 7 days before; 7-day default grace after expiry; after grace polling stops unless manually extended; historical data remains. |
| Security posture | USER-FIXED | Design from day 1 for banks, defence and global enterprises; standards/certification readiness required, but no false certification claims. |
| UI | USER-FIXED | Excellent/easy/fast/readable UX; SSL/TLS only; no application port 80. |
| Master docs | USER-FIXED | Split mother-reference set is authoritative; no silent assumptions. |
| Canonical source repository | USER-FIXED | Canonical Platform source repository is `https://github.com/gagandsm-max/platform`. GitHub is the system of record; local clones (including `~/nms`) are working copies. Code/build/test/mother-document changes become authoritative only after push and merge to GitHub. Short-lived branches and pull requests are required for material changes; do not push normal development to the protected/default branch; do not force-push shared/default branches; preserve decision provenance in Git history. Selecting GitHub does **not** freeze a CI provider. |
| Mother-document change control | USER-FIXED | Mother documents are READ-MOSTLY. Agents must not casually modify them during feature work, but **must** update them under deliberate change control when factual errors, contradictions, approved decision changes, formally selected dependencies, resolved P0/TBD items, omitted requirements, or implementation/document conflicts are found. Implementation must not permanently diverge from mother-reference truth. |
| SaaS monitoring | USER-FIXED | First-class SaaS monitoring through reusable/certified SaaS Packs is V1 scope. Exact provider pack list is not invented; pack support depends on customer priority, provider APIs, entitlements, security and licensing. |
| Triggered packet evidence | USER-FIXED | Bounded, incident-triggered/on-demand packet evidence is V1 diagnostic scope with strict RBAC, duration/volume/resource/retention controls. This does not reintroduce full always-on DPI. |
| Cloud-native dependency discovery | USER-FIXED | Platform shall enrich the temporal operational graph from approved Kubernetes/OpenTelemetry/service-mesh runtime evidence. eBPF is not silently selected and requires separate approval if proposed. |
| External change intelligence | USER-FIXED | CI/CD, IaC, cloud, Kubernetes, DB, certificate, patch/reboot and ITSM change/deployment evidence shall normalize into cross-domain Change Intelligence and RCA. |
| Carrier/ISP integration | USER-FIXED | Carrier/ISP outage, maintenance, circuit/ticket and SLA evidence is a first-class enrichment where provider/customer APIs permit; provider status is supporting evidence, not causal proof. |
| Incident Flight Recorder | USER-FIXED | Platform shall preserve bounded pre/during/post-incident topology, protocol, telemetry, change, path, service and authorized packet evidence so transient incidents remain reconstructable after recovery. |
| Similar Incident Memory | USER-FIXED | Resolved incidents and accepted outcomes shall be searchable/rankable as historical precedents; similarity is advisory and may not silently copy causality. |
| Controlled remediation | USER-FIXED | Controlled remediation is V1 for bounded approved action classes using diagnose → recommend → precondition/blast-radius preview → policy/approval → execute → validate → rollback/escalate. Unsupervised autonomous closed-loop and direct LLM privileged execution remain prohibited. |
| Platform Value / MTTR | USER-FIXED | Platform shall measure evidence-backed MTTD/MTTA/MTTR/time-to-RCA, matched-cohort improvement and per-incident contribution, and expose a four-layer customer value dashboard. Unsupported MTTR percentages or financial-savings claims are prohibited. |

## 3. ARCH-DELEGATED decisions

| Decision | Architecture selection | Status |
|---|---|---|
| Core target ≤2,500 devices | 8 vCPU / 16 GB under versioned SNWP-2500-v1 | Mandatory engineering target pending benchmark certification |
| Core modularity | Modular Core rather than excessive microservices to meet footprint | Fixed architecture direction |
| Sensor implementation language | Go for Universal Sensor Runtime; Rust only if benchmark proves a hot-spot need | Fixed architecture direction |
| Sensor planning | 4C/16G/150GB ~800 normal network devices retained as safe planning baseline; 8C/16G ~2,000–2,500 target benchmark | Baseline + performance objective |
| Sensor transport | Sensor/Agent → HTTPS/gRPC mTLS ingestion endpoint → Kafka internal | Fixed architecture direction |
| High-rate events | Dedicated Streaming/Event Sensor when volume threatens polling | Fixed architecture direction |
| Telemetry DB | ClickHouse, not Elasticsearch, as primary high-volume telemetry/analytics DB | Fixed architecture direction |
| Optional search | OpenSearch only later if Lucene-class full-text need is proven | Optional |
| Identity foundation | Keycloak with Platform-owned resource authorization | Selected subject to OSS scans |
| GNN | PyTorch + PyTorch Geometric are P0 candidates; GNN is hypothesis/propagation/blast-radius scorer, not causal oracle | Candidate pending benchmark/license review |
| Operational-intelligence modularity | Extend existing Alert/Incident/Graph/RCA/Integration/NCM/Query/Audit/Storage/Platform Operations ownership; no monolithic AI service | Fixed architecture direction |
| Packet evidence implementation | Mechanism/library is not selected by requirement; choose only after privilege, kernel, performance, storage, OSS and security evaluation | Architecture selection pending implementation review |
| Value methodology | Use evidence-backed timestamps and comparable cohorts; median plus P75/P90 where sample size supports; disclose baseline/sample/matching/limitations | Fixed architecture direction |

## 4. REFERENCE-LEARNING register

### 4.1 NetAI / graph-first learning

Supplied NetAI material was reviewed for product/architecture learning. Retained lessons: topology must be an operational reasoning graph rather than only a picture; correlation should use time + topology + protocol/service relationships; incident outputs should expose root cause, child symptoms, causal chain, evidence timeline and blast radius; anomaly/baseline context improves proactive operations; LLM is a human interaction/explanation layer, not the source of causal truth. Platform independently implements these ideas with its own canonical graph, deterministic evidence engine and optional GNN scoring. NetAI proprietary code/assets are not reused.

### 4.2 SolarWinds learning

Retained design lessons: central application/UI with horizontally scalable polling; two application nodes for customer-facing HA; separate collection scale from UI/control scale; future higher-level federation/enterprise console model for large distributed estates. Platform does not copy SolarWinds implementation or assume its database/HA mechanics.

### 4.3 ManageEngine learning

Retained design lessons: Central–Probe model, probes close to monitored networks, multiple probes per site for scale, centralized configuration, remote probe deployment and local continuity. Platform intentionally avoids making every Sensor a full mini-NMS with its own long-term database, avoids WAN-sensitive synchronous central DB access, and uses local bounded spool + asynchronous secure ingestion.

### 4.4 Legacy ThoughtData/Enterprise360 learning

Legacy material was used as capability/reference input for broad monitoring coverage such as SNMP/Flow, infrastructure, virtualization, DB, cloud, custom MIB, dashboards, NCM/IPAM/topology/synthetics. Platform is clean-room and does not inherit legacy proprietary source or claims. Cyber-threat, full always-on DPI and full APM scope are not carried into current Platform product scope.

## 5. Explicit TBD/P0 Engineering Spikes

The following must remain visibly unresolved until engineering evidence exists:

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

Any implementation document that fills one of these with a guessed value is non-conformant until the decision is formally approved and this register updated.

The v3.2.2 scope also leaves the following implementation selections open without creating new product-level P0 claims: exact initial SaaS/carrier certification list, exact packet-capture library/engine, whether eBPF is justified, and customer-specific economic-cost assumptions.

## 6. v3.1 USER-FIXED / ARCH-DELEGATED development decisions

| Decision | Classification | Governing statement |
|---|---|---|
| Docker-only development/testing | USER-FIXED | Development and testing must be isolated from the primary laptop to avoid project conflicts. |
| Local container runtime | ARCH-DELEGATED | Standardize on Docker Compose; Podman compatibility is not a v1 requirement. |
| Reboot-safe local state | USER-FIXED + ARCH-DELEGATED implementation | Laptop is frequently shut down/rebooted; named volumes, daemon auto-start, restart policies and deterministic recovery are mandatory. |
| RHEL package format | USER-FIXED | Core, Sensor and Linux Agent must ship `.rpm` in addition to `.deb`. |
| Windows Agent packaging | ARCH-DELEGATED | Runtime is compiled code-signed Windows Service. GA package is MSI; BAT install/uninstall wrappers are also shipped for scripted deployment. |
| Auto boot | USER-FIXED | Core, Sensor and Agents must start automatically after host reboot. |
| Service recovery | USER-FIXED + ARCH-DELEGATED implementation | Recoverable failures should restart automatically, but repeated failure/crash loops must be visible and diagnosable. |
| System/application logs | USER-FIXED | Every component must retain actionable logs/health evidence so engineering/support can determine what broke or is degrading. |
| Windows release testing | ARCH-DELEGATED | Local development remains Docker-first, but Windows Service/MSI/reboot certification requires isolated Windows VM/runner because Linux Docker cannot validate SCM behavior. |

These decisions supersede any earlier wording implying `.bat` is itself the Windows Agent binary or that Podman compatibility is required.

## 7. v3.2.1 USER-FIXED governance decisions

| Decision | Classification | Governing statement |
|---|---|---|
| Canonical GitHub repository | USER-FIXED | `https://github.com/gagandsm-max/platform` is authoritative; see §2. |
| Mother-document controlled correction | USER-FIXED | READ-MOSTLY with mandatory correction triggers; see §2 and `Platform_00_Document_Index.md` §11. |
| Bootstrap UI/build/logging OSS admissions | ARCH-DELEGATED + catalogue update | NGINX (dev UI proxy), Node.js/npm/Vite/@vitejs/plugin-react/TypeScript (UI tooling), and logstash-logback-encoder (Core JSON logging) recorded in `Platform_07_OSS_Catalog.md` at pinned bootstrap versions. Top-level licence is insufficient; lockfile/SBOM transitive review remains mandatory. NGINX is not silently promoted to mandatory customer production Core dependency solely because Docker development uses it. |

## 8. v3.2.2 USER-FIXED operational-intelligence/value decisions

The v3.2.2 decisions are the USER-FIXED rows in §2 covering SaaS monitoring, triggered packet evidence, cloud-native dependency discovery, external change/deployment telemetry, carrier/ISP integration, Incident Flight Recorder, Similar Incident Memory, cross-domain Change Intelligence, controlled remediation and Platform Value/MTTR.

`Platform_19_Operational_Intelligence_MTTR_and_Value.md` is the normative implementation contract for those decisions.

v3.2.2 does not reopen fixed HA, data-stack, AI-causality, security, storage or packaging decisions and does not convert an implementation candidate into an approved OSS dependency.

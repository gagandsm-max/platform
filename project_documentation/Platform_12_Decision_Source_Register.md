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
| Scope | USER-FIXED | Network + Infrastructure + DB + Cloud + Kubernetes + OTel metrics/logs + synthetics; full always-on DPI and full APM excluded initially. |
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
| Canonical source repository | USER-FIXED | Canonical Platform source repository is `https://github.com/gagandsm-max/platform`. GitHub is the system of record; local clones (including `~/nms`) are working copies. Code/build/test/mother-document changes become authoritative only after push and merge to GitHub. Short-lived branches and pull requests are required for material changes; do not push normal development to the protected/default branch; do not force-push shared/default branches; preserve decision provenance in Git history. Selecting GitHub does **not** freeze a CI provider (GitHub Actions remains unevaluated unless separately approved). |
| Mother-document change control | USER-FIXED | Mother documents are READ-MOSTLY. Agents must not casually modify them during feature work, but **must** update them under deliberate change control when factual errors, contradictions, approved decision changes, formally selected dependencies, resolved P0/TBD items, omitted requirements, or implementation/document conflicts are found. Implementation must not permanently diverge from mother-reference truth. |
| First-class SaaS monitoring | USER-FIXED | Add first-class reusable/certified SaaS monitoring packs/connectors rather than requiring every customer to build every SaaS integration from scratch. |
| Triggered packet evidence | USER-FIXED | Add bounded, privileged incident-triggered packet evidence for diagnostics. This does **not** reopen full always-on DPI as initial scope. |
| Cloud-native runtime dependencies | USER-FIXED | Add runtime service-dependency discovery/enrichment from Kubernetes, OpenTelemetry, service-mesh/cloud evidence where available, with provenance/confidence and no silent overwrite of customer-defined authoritative dependencies. |
| External change/deployment telemetry | USER-FIXED | Add normalized change/deployment evidence from CI/CD, Git/release, IaC, cloud, Kubernetes, database, certificate, patch/reboot and ITSM sources for Change Intelligence/RCA. |
| Carrier/ISP operational integration | USER-FIXED | Add carrier/ISP outage, maintenance, circuit/ticket/SLA operational evidence where provider/customer integrations permit it; provider status remains evidence rather than causal proof. |
| Incident Flight Recorder | USER-FIXED | Add a bounded incident evidence window preserving before/during/after topology, routing, telemetry, events, changes, paths, service state and authorized packet evidence so transient failures remain diagnosable. |
| Similar Incident Memory | USER-FIXED | Add tenant-scoped historical incident similarity and prior-outcome evidence. Similarity is advisory and must not silently copy a previous root cause. |
| Cross-domain Change Intelligence | USER-FIXED | Expand change intelligence beyond NCM to network, firewall/LB, cloud, CI/CD, IaC, Kubernetes, DB, certificate, patch/reboot, Platform and ITSM changes. |
| Controlled remediation | USER-FIXED | Add controlled remediation using diagnose → recommend → precondition → impact/blast-radius preview → authorization/approval → execute → validate → rollback/escalate. No direct LLM privileged execution. |
| Platform Value / MTTR Engine | USER-FIXED | Add evidence-backed MTTD/MTTA/time-to-root-candidate/MTTR measurement, matched-cohort comparisons and per-incident Platform contribution evidence. Unsupported causal or financial claims are prohibited. |
| Four-layer customer value dashboard | USER-FIXED | Show operational noise reduction, diagnosis effectiveness, incident/MTTR improvement and evidence-supported operational/economic benefit with assumptions, sample sizes and methodology. |
| NOC no-context-switch troubleshooting | USER-FIXED | Make Platform the primary NOC troubleshooting workspace: persistent contextual action bar/Troubleshoot drawer; embedded governed CLI; on-demand and multi-vantage protocol verification; Verify Now; Live Evidence; Why/What Changed/Compare; Investigation Sessions; deterministic diagnostic packs and risk-proportionate controls. Grafana is inspiration for dashboard flexibility only, not a product/UI to copy. |

## 3. ARCH-DELEGATED decisions

| Decision | Architecture selection | Status |
|---|---|---|
| Core target ≤2,500 devices | 8 vCPU / 16 GB under versioned SNWP-2500-v1 | Mandatory engineering target pending benchmark certification |
| Core modularity | Modular Core rather than excessive microservices to meet footprint | Fixed architecture direction |
| Sensor implementation language | Go for Universal Sensor Runtime; Rust only if benchmark proves a hot-spot need | Fixed architecture direction |
| Sensor planning | 4C/16G/150GB ~800 normal network devices retained as safe planning baseline; 8C/16G ~2,000–2,500 target benchmark | Baseline + performance objective |
| Sensor transport | Sensor/Agent → HTTPS/gRPC mTLS ingestion endpoint → Kafka internal | Fixed architecture direction |
| High-rate events | Production Flow and Log use dedicated exclusive roles per §13; exact implementation/capacity details remain delegated | Superseded/refined by §13 USER-FIXED role decision |
| Telemetry DB | ClickHouse, not Elasticsearch, as primary high-volume telemetry/analytics DB | Fixed architecture direction |
| Optional search | OpenSearch only later if Lucene-class full-text need is proven | Optional |
| Identity foundation | Keycloak with Platform-owned resource authorization | Selected subject to OSS scans |
| GNN | PyTorch + PyTorch Geometric are P0 candidates; GNN is hypothesis/propagation/blast-radius scorer, not causal oracle | Candidate pending benchmark/license review |
| OIV packaging | One authoritative specialized OIV mother master (`Platform_19`) carries consolidated product/architecture/HLD/LLD/dataflow/UI/security/quality/RTM additions instead of duplicating the same text across legacy files | Fixed v3.2.2 documentation architecture |
| OIV service shape | Extend existing modular Core/Sensor domains; do not create a monolithic AI service or new database by default | Fixed architecture direction |
| Similar Incident storage/search | Use existing PostgreSQL/ClickHouse/analytics capabilities first; no dedicated vector database without benchmark/OSS justification | Fixed architecture direction |
| Packet artifact storage | Use bounded encrypted artifact-storage abstraction and metadata references; do not store raw packet payload indiscriminately in PostgreSQL/ClickHouse or add an unapproved mandatory storage product | Fixed architecture direction |

## 4. REFERENCE-LEARNING register

### 4.1 NetAI / graph-first learning

Supplied NetAI material was reviewed for product/architecture learning. Retained lessons: topology must be an operational reasoning graph rather than only a picture; correlation should use time + topology + protocol/service relationships; incident outputs should expose root cause, child symptoms, causal chain, evidence timeline and blast radius; anomaly/baseline context improves proactive operations; LLM is a human interaction/explanation layer, not the source of causal truth. Platform independently implements these ideas with its own canonical graph, deterministic evidence engine and optional GNN scoring. NetAI proprietary code/assets are not reused.

### 4.2 SolarWinds learning

Retained design lessons: central application/UI with horizontally scalable polling; two application nodes for customer-facing HA; separate collection scale from UI/control scale; future higher-level federation/enterprise console model for large distributed estates. Platform does not copy SolarWinds implementation or assume its database/HA mechanics.

### 4.3 ManageEngine learning

Retained design lessons: Central–Probe model, probes close to monitored networks, multiple probes per site for scale, centralized configuration, remote probe deployment and local continuity. Platform intentionally avoids making every Sensor a full mini-NMS with its own long-term database, avoids WAN-sensitive synchronous central DB access, and uses local bounded spool + asynchronous secure ingestion.

### 4.4 Legacy ThoughtData/Enterprise360 learning

Legacy material was used as capability/reference input for broad monitoring coverage such as SNMP/Flow, infrastructure, virtualization, DB, cloud, custom MIB, dashboards, NCM/IPAM/topology/synthetics. Platform is clean-room and does not inherit legacy proprietary source or claims. Cyber-threat, full DPI and full APM scope are not carried into current Platform product scope.

## 5. Explicit TBD/P0 Engineering Spikes

The following must remain visibly unresolved until engineering evidence exists:

- exact two-Core Kafka implementation/failover while honoring no third full Core;
- exact two-Core ClickHouse replication/promotion/fencing design;
- exact PostgreSQL two-Core failover/fencing/rewind mechanics;
- exact Core service fencing/VIP implementation and failure matrix;
- final contractual DR RPO/RTO and data-loss objectives;
- final SNWP-2500-v1 metric/interface/event mix and certified performance results;
- Dedicated Flow Sensor certified datagrams/records/bytes/sec and Dedicated Log Sensor certified EPS/bytes/sec;
- exact GNN model architecture, training labels, inference latency, CPU/GPU footprint and production enablement threshold;
- final compliance certification program/scope and achieved certifications;
- exact product SKUs/licensing quantities beyond the agreed behavioral rules.

Any implementation document that fills one of these with a guessed value is non-conformant until the decision is formally approved and this register updated.

The v3.2.2 OIV additions do not resolve any of these P0/TBD items by declaration.

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

v3.2.1 does not reopen fixed product/architecture decisions and does not replace §5 P0/TBD items with implementation-option lists.

## 8. v3.2.2 USER-FIXED / ARCH-DELEGATED OIV decisions

| Decision | Classification | Governing statement |
|---|---|---|
| OIV product scope | USER-FIXED | `PRD-OIV-001` through `PRD-OIV-014` in `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` are approved additive product requirements. |
| OIV traceability | ARCH-DELEGATED | `REQ-OIV-001` through `REQ-OIV-014` in `Platform_19` §14 are the authoritative v3.2.2 RTM supplement, mapped to consolidated logical-layer sections. |
| Triggered packet evidence vs DPI | USER-FIXED | Bounded incident diagnostic capture is in scope; full always-on DPI remains excluded initially. |
| Remediation safety | USER-FIXED | No direct LLM privileged execution; controlled remediation must preserve authorization, preconditions, blast-radius controls, approval/policy, audit, validation and rollback/fail-safe behavior. |
| Value claims | USER-FIXED | Platform shall calculate evidence-backed customer-specific operational metrics but shall not present unsupported generic MTTR-reduction or financial-savings percentages as fact. |
| OIV architecture reuse | ARCH-DELEGATED | Extend existing Incident/RCA/Graph/Integration/NCM/Reporting/Sensor modules and existing data stack before introducing new mandatory services/databases. |

These decisions are additive and do not reopen §5 P0/TBD items.

## 9. v3.2.2 USER-FIXED discovery/onboarding/notification decisions — 2026-09-11

| Decision | Classification | Governing statement |
|---|---|---|
| Unified onboarding review | USER-FIXED | Network auto-discovery and Windows/Linux Agent enrollment shall surface in one governed customer review lifecycle. Review before full monitoring/licence is the safe default; trusted scoped auto-approval may be configured by authorized policy. |
| Review enrichment and decision evidence | USER-FIXED | Before activation authorized users may edit Display Name/Alias/Description, Site, Group/Sub-group, Tags, Criticality and applicable monitoring/alert-policy assignment. Approve/reject/unmanaged/exclude decisions require user comment/reason and audit evidence. |
| Agent endpoint identity | USER-FIXED | Canonical `entity_id` plus unique `agent_id`/mTLS identity is the durable Agent-backed endpoint identity. System/host name is the preferred human-readable technical name; dynamic IP/MAC values are mutable observations and do not create a new endpoint by themselves. |
| Identity reconciliation | USER-FIXED | Reinstall/reimage/clone/hostname/stable-identifier conflicts must be explicitly reconciled; Platform must not silently merge distinct endpoints or create silent duplicate active bindings when durable identity evidence conflicts. |
| Global Notification Center | USER-FIXED | Platform shall expose one global per-user Notification Center/bell with categories, unread state, deep links and preferences. Recipient visibility is server-side tenant/RBAC/resource scoped. Operational Alerts/Incidents remain semantically distinct from platform/workflow notifications. |
| Discovery drift notification | USER-FIXED | Meaningful new/disappeared/conflicting/materially moved discovery facts may notify according to policy; routine rediscovery churn, dynamic-IP changes and scan/reconnect storms must be deduplicated/rate-limited. |
| Onboarding specification packaging | ARCH-DELEGATED | `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md` is the consolidated authoritative v3.2.2 specialized master for this capability family, following the approved Platform_19 logical-layer pattern and avoiding duplicated normative text across many base documents. |

## 10. v3.2.2 USER-FIXED / ARCH-DELEGATED enterprise gap-closure decisions — 2026-09-11

| Decision | Classification | Governing statement |
|---|---|---|
| Enterprise gap-closure product scope | USER-FIXED | `PRD-EGC-001` through `PRD-EGC-015` in `Platform_21_Enterprise_Gap_Closure_Master.md` are approved additive v3.2.2 product requirements/targets within their stated release classification. |
| Enterprise gap-closure traceability | ARCH-DELEGATED | `REQ-EGC-001` through `REQ-EGC-015` in `Platform_21` are the authoritative RTM supplement for that capability family. |
| Normalized vendor integrations | USER-FIXED + ARCH-DELEGATED implementation | SD-WAN, SASE/SSE/ZTNA, middleware and backup integrations normalize into Platform-owned canonical contracts; vendor identifiers never override tenant/entity authority. |
| SCIM lifecycle target | USER-FIXED | SCIM v2-compatible lifecycle integration is an enterprise target; specification or implementation must not be represented as independent SCIM certification/compliance until validated. |
| SLI/SLO/error-budget capability | USER-FIXED | Platform shall support governed SLI/SLO/error-budget and burn-rate evaluation. Exact customer/contractual targets remain policy or Founder-reserved unless separately fixed. |
| Audit tamper evidence | USER-FIXED + ARCH-DELEGATED implementation | Security-relevant audit requires independently verifiable tamper evidence; this decision does not preselect hash chaining, signing, external anchoring or key-custody construction. |
| Tenant offboarding/data portability | USER-FIXED | Offboarding distinguishes governed export, active-data deletion, backup/archive expiry, legal hold and final destruction evidence; Platform must not promise immediate physical erasure from immutable backups. |
| Licence safety | USER-FIXED | Existing 30-day warning, 7-day banner and seven-day default post-expiry grace remain governed behavior. Commercial quantities, packaging, pricing and SKUs remain Founder-reserved. |
| Declarative policy/configuration | USER-FIXED | Git/policy-as-code may express desired state but cannot bypass Platform RBAC, validation, approvals, audit, bounded rollout or runtime safety authority. |
| Incident Command Workspace | USER-FIXED | Platform may provide a lean incident evidence/action workspace integrated with ITSM/collaboration systems; it does not become an independent chat product. |
| Endpoint DEX privacy boundary | USER-FIXED | Optional DEX is bounded operational-experience telemetry, not employee surveillance. Keystroke/content/screenshot/microphone/camera/productivity-scoring collection is outside approved scope. |
| Future browser synthetics / outside-in probes | USER-FIXED + ARCH-DELEGATED implementation | These are future capability requirements only; no browser/runtime/vendor/dependency or Platform-operated global probe SaaS is selected by declaration. |
| Regional federation | USER-FIXED | Future regional federation requires separate architecture/change control and does not reopen v1 exactly-two-Core Active/Standby by implication. |
| Enterprise gap-closure packaging | ARCH-DELEGATED | `Platform_21_Enterprise_Gap_Closure_Master.md` is the consolidated authoritative v3.2.2 specialized master for enterprise adoption gaps; monitoring-intelligence/alert-quality/AI-assurance remains reserved for a separate specialized master. |

## 11. v3.2.2 USER-FIXED / ARCH-DELEGATED monitoring-intelligence and AI-assurance decisions — 2026-09-12

| Decision | Classification | Governing statement |
|---|---|---|
| Monitoring-intelligence product scope | USER-FIXED | The formal `PRD-MIA-*` requirements in `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` are approved additive v3.2.2 requirements within their stated release classification. |
| Monitoring-intelligence traceability | ARCH-DELEGATED | The `REQ-MIA-*` catalogue in `Platform_22` is the authoritative RTM supplement for alert quality, evidence qualification, RCA/AI assurance and related shared-service evidence. |
| Alert Evidence Contracts | USER-FIXED + ARCH-DELEGATED implementation | Material alert/failure classes use class-specific required/contradictory/missing evidence semantics; a single monitor failure is not authoritative when credible corroborating evidence contradicts the asserted state. |
| Telemetry-quality trust gate | USER-FIXED | Collector/source health, freshness, timestamp quality and evidence gaps are evaluated before Platform converts weak telemetry into an operational-failure assertion. |
| Severity versus confidence | USER-FIXED | Operational severity and evidentiary confidence are separate dimensions. High severity does not imply high confidence and vice versa. |
| Semantic operational states | USER-FIXED | Platform supports explicit non-DOWN states including `MONITORING_DEGRADED`, `REDUNDANCY_LOST`, `SLA_DEGRADED`, `EXPECTED_STATE`, `UNKNOWN` and `STALE` where applicable. |
| Alert-policy lifecycle | USER-FIXED + ARCH-DELEGATED implementation | New/changed alert policy follows governed `DRAFT -> BACKTEST -> SHADOW -> ACTIVE` progression where applicable; promotion requires reproducible evidence rather than invented universal thresholds. |
| Active evidence acquisition | USER-FIXED + ARCH-DELEGATED implementation | Additional diagnostic evidence may be gathered only through bounded, authorized, tenant-scoped and resource-limited actions that cannot bypass protocol/security policy. |
| RCA / AI epistemic contract | USER-FIXED | GNN/LLM are evidence/hypothesis assistants, never causal authority. `INCONCLUSIVE` remains valid and Incident Copilot must expose evidence, contradictions, missing evidence and next validation rather than fabricate certainty. |
| AI tool-use firewall | USER-FIXED + ARCH-DELEGATED implementation | Prompt/model output cannot directly execute privileged actions. Deterministic allowlists, schema validation, tenant/resource authorization, policy, audit and post-action verification remain authoritative. |
| Tenant-scoped learning | USER-FIXED | No silent cross-tenant training, retrieval or evidence mixing. Any cross-tenant learning requires explicit approved governance and privacy controls. |
| Graceful AI degradation | USER-FIXED | Core monitoring, deterministic alerting and deterministic RCA functions must continue when hosted/private/local AI providers are unavailable; Internet/LLM/GNN availability is not a core-monitoring prerequisite. |
| AI evaluation and drift | USER-FIXED + ARCH-DELEGATED implementation | AI/ML releases require reproducible evaluation, provenance, model/data-drift visibility, resource-budget evidence and measured champion/challenger comparison before promotion. |
| AI provider abstraction and data egress | USER-FIXED + ARCH-DELEGATED implementation | Hosted/private/local/no-LLM operation must preserve explicit data-egress policy and deterministic fallback. A model/vendor/runtime dependency is not selected merely by this requirement. |
| Shared-service / PKI evidence | USER-FIXED | DNS/DHCP/NTP/PTP/RADIUS/TACACS+/LDAP/AD and PKI/certificate state are valid dependency evidence for incident reasoning; facilities/power evidence may explain infrastructure incidents but does not expand Platform into BMS/DCIM scope. |
| Monitoring-intelligence packaging | ARCH-DELEGATED | `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` is the consolidated authoritative v3.2.2 specialized master for this capability family and does not replace Platform_19 OIV or Platform_21 enterprise-gap authority outside its assigned scope. |

## 12. v3.2.2 USER-FIXED / ARCH-DELEGATED enterprise monitoring coverage and Capability Pack decisions — 2026-09-13

| Decision | Classification | Governing statement |
|---|---|---|
| Capability Pack is the single extensibility architecture | USER-FIXED | Technology Monitoring Pack, Device / Network Pack, SaaS Monitoring Pack, Integration Pack and optional Customer Extension Pack are classifications/profiles under Capability Pack. An independent Technology Pack framework is prohibited. Existing Device Pack contracts remain valid under this model. |
| Dimensional certification | USER-FIXED | Certification is dimensional (Discovery, L1, L2, L3, Topology, NCM where relevant, Security, version compatibility, profile, evidence). One scalar support level must not imply capabilities that have not been certified. |
| Operational Graph vs governed CMDB | USER-FIXED + ARCH-DELEGATED implementation | Operational Graph and governed CMDB are related but distinct states/uses of the same graph technology. Provenance, confidence, age and provisional/governed status must be preserved. A second graph database is not selected by this distinction. |
| Compatibility/certification source of truth | ARCH-DELEGATED | Product/version compatibility and certification use one machine-readable source of truth. Future UI/documentation matrices derive from that source. The source is specified in `Platform_23`; this decision does not claim the registry is implemented. |
| Real-customer requirements normalization | USER-FIXED | Recurring enterprise ITOM/AIOps needs are normalized as Real Customer Enterprise Benchmark — Automotive / ITOM-AIOps with classification A–G. Confidential customer names, inventories, locations, quantities, commercial clauses, penalties, unique SLAs and contract language must not be embedded in Mother References. |
| Enterprise-monitoring-coverage product scope | USER-FIXED | `PRD-EMC-001` through `PRD-EMC-018` in `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` are approved additive v3.2.2 product requirements/targets. They are EMC-0 specification only and do not claim runtime, pack certification or contractual coverage. |
| Enterprise-monitoring-coverage packaging | ARCH-DELEGATED | `Platform_23` is the consolidated authoritative v3.2.2 specialized master for this family. `REQ-EMC-001` through `REQ-EMC-018` are the RTM supplement. It does not replace Platform_10, 19, 20, 21 or 22 outside its assigned scope. |
## 13. v3.2.2 enterprise Flow/NTA and dedicated Sensor-role decisions — 2026-09-19

| Decision | Classification | Governing statement |
|---|---|---|
| Dedicated production Flow/Log Sensor roles | USER-FIXED | Founder explicitly requires Flow Monitoring and Log Monitoring to run on separate dedicated Sensor instances so high-rate workloads do not contend with other monitoring. One Universal Sensor software/build remains; `FLOW_SENSOR` and `LOG_SENSOR` are mutually exclusive production roles and do not run ordinary polling/discovery/NCM/synthetics or each other's workload. |
| Dedicated does not mean saturation | ARCH-DELEGATED | Role exclusivity reserves the Sensor for that workload but certification retains CPU/RAM/disk/network safety headroom for bursts, spool/replay, runtime, health and upgrade. |
| Combined high-rate Sensor | ARCH-DELEGATED | Combined Flow+Log+polling may exist for development/CI/demo/explicit non-production lab use only; it is not a production-certified role. |
| Role conversion | ARCH-DELEGATED | Production role changes require drain/unassignment, durable-queue disposition, readiness validation and restart/reconfiguration; no live mixed-role transition. |
| Flow identity enrichment | USER-FIXED + ARCH-DELEGATED implementation | Founder requires IP-to-system/hostname/user/geo/AD-context association wherever reliable evidence exists. Platform implements this as temporal, provenance/confidence-bearing enrichment; IP or AD membership alone is not user identity. |
| URL/domain boundary | ARCH-DELEGATED | Ordinary Flow may enrich domain/FQDN/application from DNS/exporter/security-device evidence but does not imply full URL. URL path/query is separately governed sensitive content and is not inferred from IP/port traffic. |
| User attribution/privacy | USER-FIXED + ARCH-DELEGATED implementation | User-attributed Flow uses time-bounded authenticated session evidence (Agent/AD-Entra auth/VPN/RADIUS/NAC/WLAN/firewall/proxy or equivalent), separate authorization and audit. NTA does not become employee browsing/content surveillance. |
| Geo semantics | ARCH-DELEGATED | Internal site geography uses governed inventory. Public-IP GeoIP is approximate provider/database evidence and is not precise physical-user location. |
| Flow source trust | ARCH-DELEGATED | UDP source/observation fields are evidence, not tenant authority. Exporters/listeners are configured and tenant scoped; unknown exporters reject/quarantine. |
| Template/sampling/sequence correctness | ARCH-DELEGATED | NetFlow v9/IPFIX template lifecycle, sampling observed-vs-estimated truth, protocol-specific sequence/gap/reset and timestamp quality are mandatory before analytics can be trusted. |
| Flow durability | ARCH-DELEGATED | Pre-acceptance UDP is best-effort; after certified Sensor durable acceptance, common replay/idempotency applies. Known gaps/drops are explicit; no zero-loss claim is made before acceptance. |
| Flow storage | ARCH-DELEGATED | PostgreSQL remains control/configuration, ClickHouse Flow/history/rollups and Kafka internal transport. No new NTA database is selected; raw retention and aggregates are distinct policy classes. |
| Flow Sensor redundancy | ARCH-DELEGATED | Flow Sensor Pool HA is exporter/profile aware. Multiple collectors, active/standby/VIP or other certified mechanisms are selected per exporter capability; generic UDP load balancing is not assumed, and duplicate/gap/template recovery semantics remain explicit. |
| Cloud-native flow sources | ARCH-DELEGATED | AWS/Azure/GCP flow-log sources may later normalize through Cloud/Capability Packs into the same canonical Flow model; they do not create a second NTA architecture by declaration. |
| GoFlow2 boundary | ARCH-DELEGATED | Reuse approved GoFlow2 for protocol decoding when implementation begins; Platform owns exporter/template/sampling/sequence/enrichment/quality/durability semantics. Catalogue status is not implementation status. |
| Flow analytics authority | ARCH-DELEGATED | Raw directional observations keep provenance; conversations/Top-N/matrices/Sankey/anomalies are derived. Flow supports RCA/capacity explanation but alone does not prove congestion, path or target failure. |
| Flow specialized master | ARCH-DELEGATED | No new specialized Flow Mother Reference is created; requirements are reconciled across existing base masters and Platform_23 NTA/Capability Pack authority. |

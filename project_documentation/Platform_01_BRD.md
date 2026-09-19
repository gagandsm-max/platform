# Platform — Business Requirements Document (BRD)

**Document ID:** PLATFORM-BRD-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference — Engineering Source of Truth
**Product name:** Platform  
**Implementation model:** Clean-room implementation; independent project, not part of ThoughtData or NetAI  
**Audience:** Founders, product leadership, engineering, architecture, QA, security, implementation, sales engineering, support  

---

## 1. Document authority and no-assumption rule

This BRD is one document in the authoritative Platform engineering set. It must be read with:

- `Platform_02_PRD.md`
- `Platform_03_Architecture.md`
- `Platform_04_HLD.md`
- `Platform_05_LLD.md`
- `Platform_06_Dataflow.md`

This BRD defines **why Platform exists, who it serves, the business outcomes required, the product principles that may not be compromised, the scope boundaries, commercial expectations and business acceptance criteria**.

Where a value has not been explicitly agreed, the document uses **TBD — Decision Required**. Engineering must not convert a TBD into a hidden assumption.

Earlier Enterprise360, NetAI, SolarWinds, ManageEngine or other product material is reference/learning material only. Platform is a clean-room product and shall not copy proprietary source code, algorithms, device packs, schemas, UI assets or confidential implementation details from those products.

---

## 2. Business vision

Platform shall be an enterprise-grade unified network and infrastructure operations platform designed for organizations that need one operational system for discovery, monitoring, topology, performance, traffic, configuration, IP address management, infrastructure, cloud, Kubernetes, databases, synthetic monitoring, alert qualification, incident correlation, causal root-cause analysis, reporting, SLA, automation readiness and operational governance.

Platform is intended to compete in a crowded market. Therefore feature presence alone is insufficient. Platform must win on **quality, usability, low operational noise, intelligence, deployment simplicity, resource efficiency and responsiveness**.

---

## 3. Non-negotiable product principles

| ID | Principle | Business requirement |
|---|---|---|
| BR-P01 | Excellent product | Features must be production-grade, coherent, supportable and testable; no demo-only behavior may be represented as complete. |
| BR-P02 | Ease of use | Normal provisioning, administration, monitoring configuration, troubleshooting and reporting must be achievable through the UI without editing files, YAML, database rows or source code. |
| BR-P03 | Very low false alerts | Platform must qualify failures using multiple evidence sources, hysteresis, dependency awareness, recovery validation and correlation before escalating avoidable noise. Absolute zero false alerts cannot be guaranteed, so the measurable goal is to minimize and continuously measure them. |
| BR-P04 | Meaningful AI | AI must help users diagnose, explain, search, summarize, recommend and operate. AI must not be decorative. It must be evidence-grounded, uncertainty-aware and able to say that root cause cannot be determined. |
| BR-P05 | Friendly UI/UX | Navigation must be role-aware, readable, fast and consistent. The product must support light/dark theme and avoid unreadably small dashboard text. |
| BR-P06 | Easy implementation | Installation, Sensor deployment, Agent enrollment, discovery, provisioning, credential testing and integration testing must be guided and diagnosable. |
| BR-P07 | Light resource footprint | Core, Sensors and Agents must be engineered against explicit CPU/RAM/storage targets and regression-tested for resource efficiency. |
| BR-P08 | Low latency / fast UI | Common dashboards, search, tables and operational actions must remain responsive at enterprise scale. Performance is a release gate. |
| BR-P09 | HTTPS-only UI | Customer UI is served only over HTTPS/TLS. Platform shall not expose an application listener on TCP/80. |
| BR-P10 | No silent failure | Material collection, storage, integration, HA, provisioning, authentication or workflow failures must expose what failed, evidence, impact and remediation guidance. |
| BR-P10A | Trustworthy traffic context | Flow analytics must preserve source/provenance, sampling and data-quality limitations and may enrich IPs with endpoint/system/domain/user/site/Geo/cloud context only when evidence supports the association. Unknown/conflicting attribution is preferable to invented certainty. |
| BR-P11 | Safe by default, flexible by choice | Secure/safe defaults are enabled initially; customers may customize operational behavior through UI where doing so does not violate tenant isolation, integrity or mandatory safety controls. |
| BR-P12 | No-context-switch operations | An authorized NOC/operator shall be able to verify, diagnose and investigate a monitored entity from its current Platform context using safe on-demand diagnostics, browser CLI and related evidence without requiring routine external ping/traceroute/SSH/SNMP/flow/config-comparison tools. Low-risk actions shall be easy; privileged or destructive actions shall retain proportionate controls and audit. |

---

## 4. Target customers and deployment environments

Platform shall support the following customer classes without requiring separate products:

- Small and medium enterprises.
- Large enterprises and global corporations.
- Banks and financial institutions.
- Defence and highly regulated organizations.
- Government/PSU environments.
- Managed Service Providers and Global/System Integrators.
- Organizations with one data center, multiple data centers, branches, regional sites, hybrid cloud and globally distributed environments.

Platform shall support these deployment models:

- On-premises VM.
- Bare-metal Linux.
- Private cloud VM.
- Public cloud VM.
- Air-gapped deployment.
- Future container/Kubernetes deployment for selected scenarios; Kubernetes is not required for initial customer deployment.

Initial certified Core and Sensor operating systems:

- Ubuntu 24.x LTS family, subject to certification matrix.
- RHEL 9.x family, subject to certification matrix.
- No Windows Core or Windows Sensor in the initial certified architecture.
- Windows and Linux Agents are separate builds from Core and Sensor.

---

## 5. Business capability scope

### 5.1 In scope

Platform shall provide, directly or through approved extensibility frameworks:

- Network discovery and inventory.
- SNMP v1/v2c/v3 monitoring and traps/informs.
- ICMP and traceroute.
- NetFlow, IPFIX, sFlow and vendor-compatible flow ingestion with exporter/template/sampling/data-quality visibility and operational traffic analytics.
- Syslog/event ingestion.
- NETCONF, RESTCONF, gNMI/gRPC and streaming telemetry.
- SSH/CLI operations with privileged session governance.
- Contextual NOC troubleshooting with on-demand ICMP/traceroute/SNMP/TCP/DNS/SSH/API verification, multi-vantage checks, Investigation Sessions and risk-tiered operational actions from the relevant entity/alert/incident workspace.
- NCM: backup, versioning, diff, compliance, controlled push, rollback, approval and audit.
- Deep network monitoring for routing, switching, overlays and WAN technologies including BGP, OSPF, IS-IS, MPLS, VXLAN/EVPN, VLAN/STP, VRF, HSRP/VRRP, LACP, QoS/CBQoS, multicast, IP SLA, VPN, SD-WAN and WLAN, subject to Device Pack/vendor telemetry availability.
- Infrastructure monitoring using SSH, WMI/WinRM, agents and APIs.
- Hardware lifecycle/health via Redfish/IPMI/iDRAC/iLO/CIMC and related interfaces.
- VMware, Hyper-V, Proxmox and Nutanix monitoring.
- Storage and SAN/fabric monitoring.
- Database monitoring.
- Kubernetes/container monitoring.
- AWS, Azure and GCP monitoring.
- OpenTelemetry metrics and logs; trace-ready architecture, without marketing full APM initially.
- Synthetic ICMP/TCP/DNS/HTTP/HTTPS/API/TLS tests.
- DNS/DHCP/NTP monitoring.
- Identity infrastructure monitoring such as AD/LDAP/DC replication/Kerberos/RADIUS/TACACS+ where supported by secure probes/APIs, and governed use of time-bounded identity/session evidence to enrich operational Flow where available.
- UPS/PDU/environment/power/cooling monitoring.
- IPAM with IPv4/IPv6, subnet hierarchy, VRF/VLAN context, DNS/DHCP integration, utilization, conflicts, history and forecasting.
- Flexible topology and maps: physical, L2, L3, routing, MPLS, VXLAN/EVPN, SD-WAN, traffic, infrastructure, service, causal and geographic.
- Customer-defined logical/business-service relationships.
- Dashboard Studio, Map Studio, Report Studio, MIB Studio, API Studio and Device Pack Studio.
- Multi-tenancy/MSP mode or single-customer mode.
- Role-based and resource-scoped authorization.
- Audit and governance.
- Alert qualification, event/alert/incident separation, correlation and RCA.
- Gemini through an AI Gateway initially, with future private/local/offline model support.
- ITSM and notification integrations.
- HA, DR capability, backup/restore, upgrade/rollback and support diagnostics.
- Customer-configurable retention, rollups, archives and storage protection.

### 5.2 Out of initial product scope

- Full DPI / packet-level Layer-7 observability as a primary product capability.
- Full APM transaction tracing product positioning.
- Autonomous closed-loop remediation in initial release; assisted recommendations and operator-controlled workflows come first.
- Future Master Core / Regional Core federation is architecturally anticipated but is not part of the initial implementation scope.

---

## 6. Competitive architectural learnings to incorporate without copying

Platform shall incorporate **industry lessons**, not proprietary implementations.

### 6.1 SolarWinds lessons

- Scale collection horizontally using pollers/sensors instead of making the central UI/control server poll every device.
- Use two application Core nodes for HA, one active and one standby.
- Keep the primary Core focused on visualization, control, alerting, reporting and administration as collection scale grows.
- For future very large/global deployments, allow multiple autonomous regional Cores with a higher-level visualization/federation layer.

### 6.2 ManageEngine lessons

- Use Central/Core plus distributed Probe/Sensor architecture.
- Place Sensors close to monitored devices.
- Allow multiple Sensors at one site for capacity, not only one Sensor per geography.
- Centralize configuration and provisioning; normal users should not administer Sensors locally.
- Give remote Sensors enough autonomy and buffering to survive WAN interruption.
- Learn from LiteCentral-style principles for future regional federation: keep high-volume raw history regional and federate summaries upward instead of copying every raw sample globally.
- Provide UI-driven remote Sensor deployment and readiness diagnostics.

### 6.3 NetAI/graph-intelligence lessons

Platform shall incorporate the strongest validated conceptual lessons from graph-first network operations:

- Treat topology as an operational reasoning model, not merely a visual map.
- Maintain a live, temporal, multi-layer graph spanning physical, L2, L3, routing, MPLS, VXLAN/EVPN, tunnels, infrastructure dependencies and business services.
- Correlate events using temporal proximity, topological relationships, protocol semantics, configuration changes, telemetry anomalies and dependency models.
- Preserve child/symptom evidence even when alert storms are compressed into one root incident.
- Expose root cause, evidence timeline, causal chain, causal graph, blast radius, contradictions, missing evidence and recommendations.
- Separate graph/causal reasoning from natural-language interaction. LLMs explain and interact; they are not the sole RCA authority.
- Add GNN-based graph learning as an **augmenting inference layer**, not as proof of causality. GNN scores/hypotheses must be constrained by protocol rules, topology provenance, temporal order and source evidence.
- Require an explicit **INCONCLUSIVE** outcome when evidence is insufficient.
- Use learned baselines/anomaly detection to surface deviations before hard thresholds where appropriate.
- Design future remediation as: diagnose → recommend → policy gate → approve → execute → validate → rollback.

Platform shall not claim that a GNN is inherently deterministic or causal. Repeatability, confidence and correctness are separate engineering concerns.

---

## 7. Core deployment business requirements

### 7.1 Standalone

A standalone installation shall use one Core node.

### 7.2 HA

A HA installation shall use exactly two full Core nodes:

- Core-A and Core-B.
- One Active, one Standby at any point in normal operation.
- Both continuously synchronize required persistent state.
- Failover target: **less than 60 seconds** for Core UI/control service restoration under certified failure scenarios.
- Sensor polling must continue independently through Core failover; data is buffered and replayed as required.
- No third customer-visible Core/governance node is part of the required HA topology.

The implementation must prevent dual-active/split-brain behavior. Automatic promotion must be fail-safe; ambiguous network partitions may not result in two authoritative active Cores.

### 7.3 Installation experience

Core, Sensor and Agent are separate builds.

Core installer shall ask whether deployment is:

- Standalone, or
- HA.

For HA, installer collects the HA endpoint/VIP and peer information. The first Core enters waiting/ready-for-peer state; the second Core completes secure pairing/handshake and synchronization.

In HA deployments, Sensors and Agents should use the stable **Core HA endpoint** rather than remain permanently bound to a single physical Core address.

---

## 8. Sensor and Agent business requirements

- Platform uses one Universal Sensor Runtime with selectable roles rather than separate Sensor codebases.
- Production Flow monitoring uses a dedicated **Flow Sensor** instance whose allocated monitoring capacity is not shared with Network polling, discovery, NCM, synthetics or Log monitoring.
- Production Log monitoring uses a dedicated **Log Sensor** instance whose allocated monitoring capacity is not shared with Flow monitoring, Network polling, discovery, NCM or synthetics.
- Network and Infrastructure Sensor roles remain separately scoped. A mixed/combined Sensor may exist for development, CI, demonstration or explicitly non-production lab use but is not production certified for Flow/Log capacity.
- Sensor role selection must be visible, governed and capacity-aware; changing a production role requires workload drain/reassignment and health validation rather than a casual live toggle.
- Sensor placement must prefer network locality while also respecting reachability, capacity, HA, protocol access and policy.
- Flow and Log availability/redundancy must be protocol/source aware; the product shall not assume an unvalidated generic load balancer or duplicate-collector behavior works for every exporter/log source.
- Customers may choose automatic placement, recommendation/approval, manual placement or Sensor Pool assignment.
- Sensor capacity must be represented by workload, not device count alone.
- Customer may deploy multiple Sensor Pools by region/country/site/security zone.
- Sensors must continue polling during transient Core/WAN loss and buffer data locally.
- Sensor disk must have the same 80% safety-protection philosophy as Core storage.
- Agents connect initially to Core; Core directs each Agent to an appropriate available Sensor based on location, capability and capacity.
- Branch/edge Agents support branch-to-DC network tests to expose asymmetric path issues.

---

## 9. Provisioning business requirements

All normal provisioning is UI-driven.

Supported workflows:

- Individual device/entity provisioning.
- Bulk CSV/XLSX import with staging/validation.
- Automatic discovery of network devices.
- Agent enrollment.
- API/REST connector provisioning.
- Cloud, virtualization, storage, database and Kubernetes connector provisioning.

Provisioning must support optional customer-entered **Display Name** and **Description**. If Display Name is blank, Platform automatically selects the best discovered name/hostname/alias and falls back to IP/address only when better identity is unavailable. Customer-entered display names are never silently overwritten by later discovery.

Bulk onboarding must normalize safe fields, detect duplicates and expose exact failures. It must not merely return “failed.” The customer must see what failed, evidence and what to fix.

Examples include:

- leading/trailing whitespace;
- invalid IP/hostname;
- missing credential profile;
- wrong SNMP details;
- SNMP disabled or suspected disabled;
- authentication failure;
- authorization failure;
- firewall/ACL/port/path issue;
- no eligible Sensor;
- Sensor capacity exceeded;
- TLS/certificate failure;
- duplicate device;
- unsupported/unknown device requiring a MIB/Device Pack.

Sensitive credential strings must not be silently trimmed because spaces may be legitimate. Credential Profiles are preferred over secrets embedded in spreadsheets.

---

## 10. Security business requirements

Platform is intended for banks, defence and global enterprises. Security architecture must therefore be designed from Day 1 for strong enterprise controls, including:

- TLS everywhere appropriate and mTLS between trusted Platform components.
- HTTPS-only UI.
- OIDC/SAML/LDAP/AD/Entra integration.
- MFA.
- Resource-scoped RBAC and explicit deny semantics.
- Secrets encrypted at rest and never sent to browsers unnecessarily.
- Certificate/PKI lifecycle management.
- Scoped API tokens/service accounts.
- Signed software/packages and Device Packs.
- SBOM and dependency provenance.
- Vulnerability and licence scanning.
- Tamper-evident audit capabilities.
- Break-glass and temporary elevation controls.
- NCM/CLI maker-checker and command guardrails.
- Sensitive-data masking before storage/AI where configured.

Architecture must support alignment with high-assurance expectations such as ISO 27001 controls, SOC 2 control families, PCI-oriented controls, RBI/BFSI expectations, defence hardening and FIPS-capable cryptography where required. **No certification may be claimed until formally achieved.** Exact certification roadmap is TBD.

---

## 11. Commercial/licensing requirements

Platform licensing may support annual, perpetual, device, element and module-based commercial models.

Licence behavior shall include:

- 30-day advance expiry warning.
- From 7 days before expiry, every dashboard displays a prominent banner with days remaining.
- Default 7-day grace period after expiry.
- During grace, polling continues.
- After the grace period, polling stops unless licence is manually extended/renewed.
- Historical data remains available and must not be deleted merely because the licence expires.
- Licence administration must remain available so authorized users can apply an extension/new licence.
- Slight over-entitlement should generate clear commercial warnings rather than randomly dropping devices; exact overage policy is TBD.

---

## 12. Storage, retention and continuity business requirements

Customers control normal retention and rollups through UI by data class and scope.

They must be able to configure:

- raw retention duration;
- rollup resolutions;
- rollup retention;
- archive destination and duration;
- emergency purge priority within safe limits;
- inheritance/overrides by tenant/region/site/group/device/data type where appropriate.

Platform must show estimated storage impact before material changes.

A mandatory Storage Protection Engine shall ensure monitoring does not stop because historical storage fills.

- 80% utilization is the non-disableable emergency safety watermark for Core and Sensor managed data volumes.
- Platform should warn and forecast before reaching the watermark.
- At/above the watermark, eligible oldest raw telemetry/log/flow data is automatically reclaimed.
- Rollups/aggregates should be preserved longer than high-resolution raw data where possible.
- Protected configuration, identity, current inventory, security and active operational state are not indiscriminately deleted.
- Audit/compliance data follows protected retention/archival policy.
- Customer is shown exactly what was removed and why.
- Reclamation should target safe headroom rather than merely reducing 80.1% to 79.9%.
- Minimum recent operational data window is configurable within safety limits.
- Legal/investigation hold can protect incident evidence from normal purge, subject to emergency capacity constraints and explicit warning.

---

## 13. Business continuity requirements

- HA is distinct from DR.
- DR architecture is mandatory in v1.0.
- Exact contractual RPO/RTO values are **TBD — Decision Required** and must not be invented.
- Backup/restore must cover configuration, PostgreSQL state, ClickHouse data/metadata policy, Device Packs, dashboards, reports, integrations, policies, certificates/keys according to secure process and other required state.
- Restore testing/integrity verification is required.
- Upgrades require preflight checks, drain/rolling behavior where applicable, verification and rollback strategy.
- Sensors should be upgradable without unnecessarily interrupting monitoring through pool drain/reassignment.

---

## 14. User/persona requirements

Platform must serve at least these role archetypes while allowing custom roles:

- Platform Super Admin.
- MSP Super Admin.
- Tenant Admin.
- Security Admin.
- NOC Manager.
- NOC Operator L1.
- Network Engineer L2/L3.
- Infrastructure Engineer.
- Cloud Engineer.
- Database Engineer.
- NCM Administrator.
- IPAM Administrator.
- Integration Administrator.
- Report Administrator.
- Auditor.
- Executive Viewer.
- Read Only.
- API Service Account.

Role templates are starting points only. Authorization is evaluated as **Who × Resource × Action × Where × Conditions** and enforced server-side.

---

## 15. Success metrics

Product effectiveness must be measurable. Platform should expose or internally track metrics including:

- provisioning success rate;
- time to onboard a standard device;
- Sensor utilization and polling completion;
- telemetry freshness/completeness;
- false-alert/qualification metrics;
- alert compression ratio;
- mean time to detect;
- mean time to qualify;
- time to root cause;
- RCA accepted/partially accepted/rejected/inconclusive rate;
- blast-radius accuracy where ground truth exists;
- incident reopen/flapping rate;
- dashboard/query latency;
- Core/Sensor CPU/RAM/storage footprint;
- storage compression and growth;
- ITSM synchronization success;
- upgrade/failover success;
- data-pipeline rejection/duplicate/late-data rates.

---

## 16. Performance and sizing business targets

### 16.1 Core

For the agreed reference workload, **8 vCPU / 16 GB RAM for up to 2,500 devices is a mandatory engineering optimization target**. It must be validated against a defined Standard Network Workload Profile; it is not a claim that every possible 2,500-device workload fits that footprint.

### 16.2 Sensor

Current safe planning baseline:

- Network Sensor: 4 vCPU / 16 GB RAM / 150 GB local SSD for approximately 800 normally monitored network devices under the agreed reference conditions.

Engineering density target to benchmark:

- 8 vCPU / 16 GB Sensor profile targeting approximately 2,000–2,500 normal network devices where workload permits.

Device count is never the sole sizing dimension. Monitoring Work Units must account for metrics, interfaces, polling frequency, protocol, RTT, timeouts, Flow, syslog, gNMI, NCM and other work.

---

## 17. Clean-room and OSS business requirement

Platform is built from scratch. Reuse is limited to approved open-source components and newly developed Platform code.

Permissive OSS should be reused for commodity plumbing wherever practical, for example SNMP parsing, flow parsing, gNMI, SSH, virtualization SDKs, Kubernetes clients, UI chart/table libraries and document generation.

Platform shall build its own differentiated logic where generic OSS would create conflicting product models or fail to meet requirements, especially:

- canonical entity/relationship model;
- Device Pack/MIB/API studios;
- workload-aware Sensor scheduler/placement;
- policy inheritance;
- alert qualification;
- incident correlation;
- graph/causal RCA;
- governance/RBAC semantics;
- unified timeline;
- dashboard/report UX;
- storage-protection policy;
- licensing/entitlements.

All dependencies must pass the Platform OSS Catalogue process, SBOM, transitive licence scan, vulnerability scan and third-party-notice generation.

---

## 18. Business acceptance gates

The product may not be considered enterprise-ready unless all applicable gates are demonstrated:

1. UI-only normal provisioning works for individual, bulk and discovery workflows.
2. Provisioning errors are actionable and evidence-based.
3. Core and Sensor remain within certified resource envelopes for reference workloads.
4. HA pair fails over within the certified target without stopping Sensor polling.
5. Storage pressure at the 80% watermark does not stop live monitoring and produces an auditable reclamation event.
6. Alert qualification demonstrably reduces avoidable device-down false positives.
7. RCA can return INCONCLUSIVE and every root-cause conclusion links to inspectable evidence.
8. GNN/ML output cannot bypass protocol/evidence guardrails or remediation approval.
9. Tenant isolation is enforced at API/query/data layers, not only UI.
10. NCM/CLI privileged operations are authenticated, scoped, auditable and safely controlled.
11. Upgrade, backup/restore and DR procedures are tested.
12. Security, dependency and licence scans are part of release engineering.
13. UI is HTTPS-only and common workflows meet certified responsiveness targets.

---

## 19. Explicit TBD / decision register

The following are intentionally not assumed:

| ID | Item | Status |
|---|---|---|
| TBD-01 | Final external product/brand name beyond working name “Platform” | TBD |
| TBD-02 | Contractual DR RPO/RTO and automated-vs-manual DR failover policy | TBD |
| TBD-03 | Exact compliance certification roadmap/timeline | TBD |
| TBD-04 | Final certified Core scale beyond 15,000 devices and exact multi-Core trigger | Benchmark required |
| TBD-05 | Exact Standard Network Workload Profile values after lab characterization | Engineering benchmark required |
| TBD-06 | Final GNN implementation framework and optional GPU profile | Architecture benchmark/licence review required |
| TBD-07 | Exact licensing overage policy before polling suspension | Commercial decision |
| TBD-08 | Exact default retention durations/rollup periods | Product defaults to be decided; UI remains configurable |
| TBD-09 | Exact numeric UI/API SLOs after performance baseline | Benchmark required |
| TBD-10 | Future Master Core/Regional Core release scope | Future architecture only |

---

## 20. Business architecture summary

Platform is to be built as a lightweight central control and intelligence plane with distributed Sensors close to monitored resources. Sensors perform collection and local buffering; Core provides centralized control, UI, policies, alerting, incidents, RCA, reporting and administration. High-volume telemetry uses Kafka internally and ClickHouse for analytical history; PostgreSQL stores authoritative transactional/configuration state. A temporal multi-layer operational graph is a first-class platform construct. Deterministic/protocol-aware correlation is augmented by anomaly detection and graph learning/GNN techniques; Gemini is an interaction/explanation layer through an AI Gateway, not the primary source of truth.

The commercial proposition depends on the platform being easier to deploy, quieter, faster, lighter and more operationally useful than incumbent alternatives—not simply broader in feature count.


---

# Version 3.0 Reconciliation — Requirements That Must Not Be Lost

This section is part of the authoritative document. It was added after a complete reconciliation of the Platform design discussions because the earlier split-document version compressed or omitted details. The canonical requirement IDs are in `Platform_08_Requirements_Traceability.md`; OSS decisions are in `Platform_07_OSS_Catalog.md`.

The following cross-cutting capabilities are mandatory where applicable and must be represented in design, UI, APIs, security, audit and QA:

- asset lifecycle/reconciliation, stale/disappeared/replaced device handling and decommission workflow;
- technology lifecycle/EOL/EOS/firmware risk visibility without pretending to be a vulnerability scanner;
- backup, restore verification, v1.0 DR, low-downtime upgrade, rollback, canary and feature-flag controls;
- desired-vs-actual monitoring state, drift detection, change impact preview and dry-run;
- path analysis, device/time comparison, circuit inventory, business/service ownership and shift handover;
- learned baselines/seasonality, alert storm protection, flapping detection, event compression and alert-policy effectiveness;
- unified Operations Timeline combining telemetry, events, topology, NCM, audit, user action, sensor activity, ITSM and RCA;
- data provenance/quality/confidence, topology relationship provenance/confidence and historical topology;
- global search, command palette, contextual troubleshooting actions, support bundle and customer-visible Platform Health;
- licensing/usage dashboard, data-growth forecast and entitlement-aware UI;
- sensitive-field classification, masking/drop/hash/store policy, AI data boundaries and AI audit;
- Device Pack certification/test simulator, import/export, template library, onboarding wizard and contextual help;
- API-first implementation, webhooks/outbound events, API governance, rate limits and query protection;
- integration test sandbox, timezone/site timezone handling, internationalization foundations and accessibility;
- telemetry/cardinality explosion protection, tenant/noisy-neighbor quotas, ingestion dedupe, late/out-of-order/replay handling and legal hold;
- Migration Studio and Environment Readiness Scanner;
- change-freeze/business calendars, runbook/knowledge library and post-incident review/problem management;
- RCA accuracy/feedback metrics, monitoring optimization recommendations and Sensor placement learning;
- certificate/PKI lifecycle, proxy/no-proxy and data-sovereignty/regional-placement readiness;
- IPv6/dual-stack first-class behavior;
- scheduled housekeeping and end-to-end pipeline-integrity visibility;
- stable customer-facing error codes/correlation IDs and **No Silent Failure**.


## BRD-OSS — Open-source and commercial-product requirements

### Business requirement

Platform shall aggressively reuse mature, commercially suitable OSS for commodity protocol/data/UI functions **only through the approved OSS Catalogue**, while retaining Platform-owned code for differentiated product behavior. The objective is faster time-to-market, lower defect/security risk in standard protocols, better resource efficiency and a sustainable commercial distribution model.

### Business rules

- Approved OSS must not force Platform to disclose proprietary Platform source code or prevent commercial redistribution under the intended business model.
- A component with a permissive top-level license is not automatically approved; all transitive dependencies and bundled assets require review.
- Vendor MIB/YANG/SDK/API examples/icons are separate intellectual property from the OSS library used to parse them.
- Customers must receive the notices/licenses required by shipped dependencies.
- The project shall maintain an automated license/security watch and may freeze/replace an OSS component if relicensing/security materially changes its suitability.
- Platform must not make a customer deploy a heavyweight nested third-party product when a library can provide the primitive more efficiently.

### OSS strategy by business capability

| Business capability | OSS strategy | Platform-owned value |
|---|---|---|
| Network collection | Reuse protocol libraries (GoSNMP, GoFlow2, OpenConfig, SSH, WinRM, etc.) | unified device identity, policy, placement, diagnostics and normalization |
| Data platform | Kafka + PostgreSQL + ClickHouse + Valkey | schemas, lifecycle, HA integration, query model, storage protection |
| UI | React/ECharts/TanStack/Cytoscape/MapLibre/xterm.js | complete role-aware product experience and Studios |
| NCM | reuse network automation libraries | safe governance, approval, compliance, RCA evidence |
| AI/analytics | permissive analytics/GNN libraries where approved | evidence model, training labels, causal guardrails, accuracy measurement |
| Identity/security | Keycloak/OPA/CEL where approved | tenant/resource authorization, audit and business policy |
| OSS governance | ORT/Syft/Trivy/ScanCode | company allow/deny policy and release approval |

## BRD-COMP — Enterprise security/compliance readiness from day one

Platform is intended for banks, defence, government and large global enterprises. Engineering shall therefore implement controls needed to support future alignment/certification efforts including ISO 27001/SOC 2-style control evidence, PCI DSS-relevant controls where customers require them, RBI/BFSI security expectations, FIPS-capable cryptographic deployment options, STIG/CIS hardening and strong supply-chain evidence. **The BRD does not claim that Platform is certified to any of these standards.** Certification/validation requires the applicable formal process and is outside a software-design assertion.

## BRD-LIC — Commercial-license continuity

- 30 days before expiry: administrator/customer warning begins.
- 7 days before expiry: every dashboard shows a persistent `License expires in X days` banner to authorized users.
- At expiry: 7-day default grace begins; polling continues.
- End of grace: new polling/collection stops unless the license is manually extended/renewed. Existing historical data remains accessible subject to normal RBAC/retention.
- The system shall never randomly drop only the number of devices over entitlement. Entitlement exceptions and overage behavior must be deterministic, visible and auditable.

## BRD-STOR — Storage continuity principle

Historical telemetry is less important than continuity of live monitoring. Core and Sensor managed volumes implement mandatory 80% safety protection. Customers control normal raw retention, rollup resolution/retention and archive behavior; safety logic may reclaim oldest eligible data when needed while preserving protected configuration/security/current-state data and respecting legal hold where possible.


## Mother-reference cross-document contract (v3.1)

The following supporting mother references are normative and must be read together with this document:

- `Platform_07_OSS_Catalog.md` — approved/candidate OSS by module, license class, usage, rationale and restrictions.
- `Platform_08_Requirements_Traceability.md` — completeness control; a missing mapped requirement is a documentation defect.
- `Platform_09_UI_UX_Master.md` — detailed global shell, Studios, 98 out-of-box dashboard/workspace screen contracts, filters/actions/states/RBAC behavior.
- `Platform_10_Protocol_Security_Connector_Master.md` — protocol/security/authentication/credential/connectivity matrix and provisioning diagnostics.
- `Platform_11_Sizing_Capacity_Storage_Master.md` — Core/Sensor compute targets, MWU, placement, retention/storage and 80% protection.
- `Platform_12_Decision_Source_Register.md` — USER-FIXED vs ARCH-DELEGATED vs REFERENCE-LEARNING vs TBD/P0 decisions.

**OSS rule:** implementation teams may not add a runtime/build dependency merely because it is open source. It must be admitted to the Platform OSS Catalogue by exact version, transitive license/CVE scan and release policy.

## 21. Development isolation, installability and service-continuity business requirements — v3.1

### 21.1 Development isolation

Platform engineering must not interfere with other software projects on an engineer's workstation. All routine development, integration environments, stateful dependencies and automated tests therefore run under Docker Compose. The project must not require engineers to install project-specific Java, Go, Python, Node, Kafka, PostgreSQL, ClickHouse or Valkey runtimes directly on their host OS.

The local environment must tolerate normal laptop shutdown/reboot. Persistent developer data must survive through named volumes, while disposable test environments must be reproducible from source and pinned images.

### 21.2 Production packaging

Platform customers must not be forced to install Docker to run the production product. Certified native packages are required:

- Core: `.deb` for Ubuntu/Debian-family and `.rpm` for RHEL-family.
- Sensor: `.deb` and `.rpm`.
- Linux Agent: `.deb` and `.rpm`.
- Windows Agent: compiled and code-signed Windows Service executable, GA MSI installer, plus simple BAT wrappers for scripted install/uninstall.

Core, Sensor and Agents are separate release artifacts with independent versions, compatibility rules and rollback capability.

### 21.3 Reboot and failure continuity

Core, Sensor and Linux Agent must start automatically at OS boot and recover from recoverable process failures through `systemd`. Windows Agent must register as an automatically starting Windows Service and use Service Control Manager recovery actions. A machine restart must not require manual application startup.

### 21.4 Diagnostic transparency

Every component must maintain enough structured operational evidence for support and engineering to determine what failed, when, where, why, what was retried, whether recovery succeeded and what customer action is required. Logging must be useful but bounded, protected by retention/rotation and the 80% storage-safety policy. Secrets and credentials must not be logged.

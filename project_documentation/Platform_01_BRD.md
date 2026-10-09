# Platform — Business Requirements Document (BRD)

**Document ID:** PLATFORM-BRD-001
**Version:** 3.3.0
**Status:** Authoritative Mother Reference — Business Requirements Authority (**A2.1 NORMALIZED_V1**)
**Product name:** Platform (internal engineering/build identifier) / AmrudAi (commercial/customer-facing product name)
**Implementation model:** Clean-room implementation; independent project, not part of ThoughtData, Enterprise360 or NetAI
**Audience:** Founders, product leadership, engineering, architecture, QA, security, sales engineering, support
**Normalization state:** NORMALIZED_V1
**Last reconciliation:** 2026-09-30 — Unified Network & Infrastructure Observability Phase-1 closure and eBPF integration (Issue #142)

---

## 1. Purpose and authority

This BRD is the **business requirements authority** for Platform. It defines why Platform exists, who it serves, the outcomes customers must achieve, the product principles that may not be compromised, commercial/continuity expectations and business acceptance criteria.

It must be read with:

- `Platform_00_Document_Index.md` — documentation constitution, maturity model and precedence
- `Platform_02_PRD.md` — customer-visible product capability and behavior
- `Platform_03_Architecture.md` — system structure, trust boundaries and technical invariants
- `Platform_08_Requirements_Traceability.md` — global formal `REQ-*` / `PRD-*` index
- Specialized masters `Platform_19`..`Platform_23` where those domains apply
- `Platform_12_Decision_Source_Register.md` for USER-FIXED / ARCH-DELEGATED / TBD provenance

**Ownership boundary:** BRD owns business outcomes and scope. It does **not** own implementation algorithms, schema indexes, protocol byte formats or UI pixel contracts. Those belong to PRD, Architecture and downstream masters.

Where a value has not been explicitly agreed, this document uses **TBD — Decision Required** or **Founder-reserved**. Engineering must not convert a TBD into a hidden assumption.

Earlier Enterprise360, NetAI, SolarWinds, ManageEngine or other product material is reference/learning material only. Platform shall not copy proprietary source code, algorithms, device packs, schemas, UI assets or confidential implementation details from those products.

**Maturity discipline (mandatory):**

`SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED`

Documentation alone never advances maturity. A business requirement that is SPECIFIED is not evidence of implementation.

---

## 2. Business vision

Platform shall be an enterprise-grade unified network and infrastructure operations platform for organizations that need **one operational system** for discovery, monitoring, topology, performance, traffic, configuration, IP address management, infrastructure, cloud, Kubernetes, databases, synthetic monitoring, alert qualification, incident correlation, causal root-cause analysis, reporting, SLA, automation readiness and operational governance.

Platform competes in a crowded market. Feature presence alone is insufficient. Platform must win on **quality, usability, low operational noise, intelligence, deployment simplicity, resource efficiency and responsiveness**.

### 2.1 Product positioning: Unified Network & Infrastructure Observability (Phase 1)

Platform’s commercial positioning for Phase 1 is **Unified Network & Infrastructure Observability**, unifying network infrastructure, systems, virtualization, cloud networks, operational topology, and endpoint process interactions into a coherent, high-trust operational model. This position is supported by the target Phase-1 capability set (subject to applicable implementation, benchmark and certification gates), including:

1. **Network monitoring:** Deep protocol coverage (SNMP v1/v2c/v3, traps/informs, gNMI, NETCONF, streaming telemetry) with authoritative telemetry trust.
2. **Infrastructure monitoring:** Physical servers, virtualization (VMware, Hyper-V, Proxmox, Nutanix), storage fabrics, and OS-level telemetry.
3. **Temporal operational topology:** Time-aware L2/L3 topology and dependency graphs reflecting historical truth during incident windows.
4. **Telemetry quality and trust:** Explicit scoring of freshness, reachability, timestamp jitter, and evidence completeness before operational failure is asserted.
5. **Cross-signal investigation:** Zero-context-switch investigation across metrics, logs, flows, topology, process telemetry, and active paths.
6. **Active path observability (`REQ-NET-016`):** Multi-vantage, active path tracing and hop-by-hop latency/loss measurement with flow-stable multipath awareness (such as Paris-traceroute-style techniques evaluated by BUILD-040) to eliminate false multipath anomalies where supported.
7. **Flow, log, and event correlation:** Unified correlation of NetFlow/IPFIX/sFlow and syslog without drowning polling capacity.
8. **Bounded Linux process-to-network evidence (`REQ-INF-013`):** In-kernel eBPF socket-level telemetry capturing process, container, remote endpoint, bytes, duration, and TCP health without modifying application code or deploying heavyweight APM.
9. **Service Assurance / SLI / SLO / Error Budgets:** Service-level objectives, error-budget burn rates, and SLA/OLA assurance derived from authoritative operational evidence.
10. **Cloud and Kubernetes monitoring (`REQ-INF-008`):** Multi-cloud and Kubernetes estate observability with normalized entity identity and collector health separation.
11. **Deterministic evidence-led RCA:** Explainable root-cause analysis that identifies contradictions, missing evidence, and supports `INCONCLUSIVE` as a valid outcome.
12. **Incident and change intelligence:** Correlating ITSM change records, deployment events, and configuration diffs as supporting context.

### 2.2 Hard product boundaries

Phase 1 Unified Network & Infrastructure Observability is intentionally bounded. It does **not** include or imply:
- Full APM (code-level transaction tracing, Java/.NET bytecode manipulation/instrumentation, method-level timing);
- Continuous profiling or flame graphs;
- Real User Monitoring (RUM) or session replay;
- Full DPI (Deep Packet Inspection), continuous always-on packet capture, TLS decryption, or packet payload reconstruction;
- Mandatory GPU/NPU requirements;
- A mandatory third Core node;
- Dedicated external graph databases, dedicated trace databases, or dedicated Elasticsearch/OpenSearch clusters;
- A separate eBPF collector backend or pipeline.

Platform preserves existing bounded triggered packet evidence under Operational Intelligence rules (`Platform_19`), which is privileged, bounded, tenant-scoped, and resource-limited.

Business outcome: an authorized operator can discover, monitor, diagnose and govern their estate from Platform without routine reliance on disconnected ping/traceroute/SSH/SNMP/flow/config-comparison toolchains, while privileged actions remain controlled and auditable.

---

## 3. Product principles

| ID | Principle | Business requirement |
|---|---|---|
| BR-P01 | Excellent product | Features must be production-grade, coherent, supportable and testable; no demo-only behavior may be represented as complete. |
| BR-P02 | Ease of use | Normal provisioning, administration, monitoring configuration, troubleshooting and reporting must be achievable through the UI without editing files, YAML, database rows or source code. |
| BR-P03 | Very low false alerts | Platform must qualify failures using multiple evidence sources, hysteresis, dependency awareness, recovery validation and correlation before escalating avoidable noise. Absolute zero false alerts cannot be guaranteed; the measurable goal is to minimize and continuously measure them. |
| BR-P04 | Meaningful AI | AI must help users diagnose, explain, search, summarize, recommend and operate. AI must be evidence-grounded, uncertainty-aware and able to say that root cause cannot be determined. |
| BR-P05 | Friendly UI/UX | Navigation must be role-aware, readable, fast and consistent. The product must support light/dark theme and avoid unreadably small dashboard text. |
| BR-P06 | Easy implementation | Installation, Sensor deployment, Agent enrollment, discovery, provisioning, credential testing and integration testing must be guided and diagnosable. |
| BR-P07 | Light resource footprint | Core, Sensors and Agents must be engineered against explicit CPU/RAM/storage targets and regression-tested for resource efficiency. |
| BR-P08 | Low latency / fast UI | Common dashboards, search, tables and operational actions must remain responsive at enterprise scale. Performance is a release gate. |
| BR-P09 | HTTPS-only UI | Customer UI is served only over HTTPS/TLS. Platform shall not expose an application listener on TCP/80. |
| BR-P10 | No silent failure | Material collection, storage, integration, HA, provisioning, authentication or workflow failures must expose what failed, evidence, impact and remediation guidance. |
| BR-P11 | Safe by default, flexible by choice | Secure/safe defaults are enabled initially; customers may customize operational behavior through UI where doing so does not violate tenant isolation, integrity or mandatory safety controls. |
| BR-P12 | No-context-switch operations | An authorized NOC/operator shall verify, diagnose and investigate a monitored entity from its current Platform context using safe on-demand diagnostics, browser CLI and related evidence without requiring routine external tools. Low-risk actions shall be easy; privileged or destructive actions shall retain proportionate controls and audit. |
| BR-P13 | Commercial product branding | Customer-facing UI surfaces display effective commercial branding (defaulting to AmrudAi, or licensed tenant/MSP branding where white-label is active) with appropriate light/dark logo contrast and aspect ratio preservation. Generated PDF reports prominently embed the effective commercial logo (defaulting to the governed AmrudAi logo, or licensed tenant/MSP logo where white-label is active); CSV reports are data-only. Internal engineering, architecture, code packages, and schemas retain Platform identity. |

**Industry lessons (business intent, not proprietary copy):**

- Scale collection horizontally with Sensors/pollers rather than making the central UI/control server poll every device.
- Use exactly two application Core nodes for HA (Active/Standby).
- Place Sensors close to monitored resources; allow multiple Sensors per site for capacity.
- Centralize configuration; Sensors remain headless and continue buffering during WAN/Core interruption.
- Treat topology as an operational reasoning model, not merely a visual map.
- Separate graph/causal reasoning from natural-language interaction; require explicit **INCONCLUSIVE** when evidence is insufficient.
- Design remediation as diagnose → recommend → policy gate → approve → execute → validate → rollback.

---

## 4. Customer types and personas

### 4.1 Customer classes

Platform shall support, without requiring separate products:

- Small and medium enterprises
- Large enterprises and global corporations
- Banks and financial institutions
- Defence and highly regulated organizations
- Government/PSU environments
- Managed Service Providers and Global/System Integrators
- Organizations with one data center, multiple data centers, branches, regional sites, hybrid cloud and globally distributed environments

### 4.2 Personas (role archetypes)

Platform must serve at least these role archetypes while allowing custom roles:

- Platform Super Admin; MSP Super Admin; Tenant Admin; Security Admin
- NOC Manager; NOC Operator L1; Network Engineer L2/L3
- Infrastructure Engineer; Cloud Engineer; Database Engineer
- NCM Administrator; IPAM Administrator; Integration Administrator; Report Administrator
- Auditor; Executive Viewer; Read Only; API Service Account

Role templates are starting points only. Authorization is evaluated as **Who × Resource × Action × Where × Conditions** and enforced server-side (product detail in PRD; architecture in Platform_03).

---

## 5. MSP versus customer deployment context

- **Single-customer mode:** one tenant/customer boundary; no MSP multi-customer switcher required.
- **MSP / multi-tenant mode:** one Platform deployment may operate many customers with hard isolation at API, authorization, query and persistence layers—not UI-only separation.
- MSP operators may scope across authorized customers; a customer user must never browse another customer’s estate.
- Overlapping IP spaces across customers are expected and must be handled by tenant-aware identity, not IP-alone identity.
- Noisy-neighbor protection (quotas, query/cardinality limits) is a business continuity requirement for MSP viability.

---

## 6. Supported deployment environments

### 6.1 Infrastructure targets

- On-premises VM; bare-metal Linux; private cloud VM; public cloud VM; air-gapped deployment
- Future container/Kubernetes deployment for selected scenarios; Kubernetes is **not** required for initial customer deployment

### 6.2 Certified OS targets (initial)

- Ubuntu 24.x LTS family (subject to certification matrix)
- RHEL 9.x family (subject to certification matrix)
- No Windows Core or Windows Sensor in the initial certified architecture
- Windows and Linux Agents are separate builds from Core and Sensor

### 6.3 Deployment topologies (business)

- **Standalone:** one Core node
- **HA:** exactly two full Core nodes (Active/Standby); no customer-visible third Core/governance node in the required HA topology
- **DR:** site-level disaster recovery capability is mandatory in v1.0 business scope; contractual RPO/RTO remain TBD
- **Future federation:** Master Core / Regional Core is architecturally anticipated and out of initial implementation scope

---

## 7. Business capability catalogue

Platform shall provide the following capability families. Each family depends on entity identity, credential governance, Sensor/Agent placement and monitoring policy where applicable. Detailed product behavior is owned by `Platform_02_PRD.md`; specialized supplements by `Platform_19`..`Platform_23`.

| # | Capability family | Business outcome | Key dependencies |
|---|---|---|---|
| 1 | Discovery / onboarding | New entities enter a governed review lifecycle before licensed monitoring | Credentials, Sensor capacity, identity/dedup |
| 2 | Network monitoring | Operators see network health, path and protocol state with low false downs | SNMP/ICMP/flow/path, Device Packs, qualification |
| 3 | Infrastructure monitoring | Servers/hardware/virt/storage/endpoints are operable | Agents/agentless protocols, Sensor roles |
| 4 | Applications / services | Critical service checks and synthetics expose user-facing health | Synthetics, deps graph |
| 5 | Databases | DB availability and health are visible without unsafe SQL by default | Packs, credentials, host deps |
| 6 | Cloud / Kubernetes | Cloud and K8s estates appear as first-class inventory and health | Connectors, OTel, identity normalization |
| 7 | Telemetry / log / flow | High-volume telemetry is usable without drowning polling | FLOW/LOG roles, cardinality controls |
| 8 | NCM | Config backup, compliance and controlled change with audit | Privileged credentials, maker-checker |
| 9 | IPAM | Address space is authoritative and linked to topology | Discovery, DNS/DHCP where available |
| 10 | Topology / dependency | Operators reason over relationships, not siloed devices | Graph provenance, temporal history |
| 11 | Alert / incident / RCA | Noise is reduced; causes are evidenced or INCONCLUSIVE | Freshness, identity, topology, maintenance |
| 12 | AI-assisted operations | Operators get grounded help without AI becoming authority | Evidence, tool firewall, audit |
| 13 | Reporting / SLA | Stakeholders get trustworthy reports and SLA evidence | Timezone policy, maintenance exclusions |
| 14 | Integrations | Tickets/notifications sync without becoming truth | Durable outbound, RBAC |
| 15 | Administration | Day-2 operations are UI-driven | RBAC, audit |
| 16 | IAM / security | Regulated customers can meet control expectations | mTLS, SSO/MFA, secrets |
| 17 | Audit / governance | Actions are attributable and exportable | Durable audit target |
| 18 | Data retention / protection | Live monitoring survives storage pressure | 80% protection, legal hold |
| 19 | Backup / restore / DR | Recoverability is demonstrated | HA≠DR separation |
| 20 | HA / continuity | Control plane restores quickly; Sensors keep collecting | Exactly-two-Core topology |
| 21 | Install / upgrade | Customers install without Docker mandate; upgrades are safe | Native packages, preflight |
| 22 | Supportability | Failures are diagnosable with support bundles | Structured logs, correlation IDs |
| 23 | Operational diagnostics | NOC verifies without context switch | Sensor/Agent paths, RBAC |
| 24 | Licensing | Commercial continuity is deterministic and visible | Founder-reserved grace conflict |
| 25 | UX / accessibility | Product is usable at enterprise scale | HTTPS-only, a11y |
| 26 | Extensibility | Customers extend via packs/studios without Core forks | Capability Pack architecture |

---

## 8. Monitoring coverage (business)

Platform shall cover, directly or through approved extensibility:

- Network discovery and inventory
- SNMP v1/v2c/v3 monitoring and traps/informs
- ICMP, standard traceroute, and multi-vantage active path tracing (flow-stable active path tracing and multipath hop attribution under `REQ-NET-016`)
- NetFlow, IPFIX, sFlow and vendor-compatible flow ingestion
- Syslog/event ingestion
- NETCONF, RESTCONF, gNMI/gRPC and streaming telemetry
- SSH/CLI operations with privileged session governance
- Contextual NOC troubleshooting (on-demand verification, Investigation Sessions, risk-tiered actions)
- Deep network monitoring for routing, switching, overlays and WAN technologies (BGP, OSPF, IS-IS, MPLS, VXLAN/EVPN, VLAN/STP, VRF, HSRP/VRRP, LACP, QoS/CBQoS, multicast, IP SLA, VPN, SD-WAN, WLAN) subject to Device Pack/vendor telemetry availability
- Infrastructure monitoring (SSH, WMI/WinRM, agents, APIs) and bounded Linux process-to-network socket telemetry using eBPF (`REQ-INF-013`)
- Hardware lifecycle/health (Redfish/IPMI/iDRAC/iLO/CIMC and related)
- VMware, Hyper-V, Proxmox and Nutanix monitoring
- Storage and SAN/fabric monitoring
- Database monitoring
- Kubernetes/container monitoring
- AWS, Azure and GCP monitoring (and OCI where product requirements already authorize)
- OpenTelemetry metrics and logs; trace-context reference boundary (`REQ-INF-010`) without marketing full APM or storing distributed traces in Phase 1
- Synthetic ICMP/TCP/DNS/HTTP/HTTPS/API/TLS tests
- DNS/DHCP/NTP monitoring
- Identity infrastructure monitoring (AD/LDAP/DC replication/Kerberos/RADIUS/TACACS+) where supported by secure probes/APIs
- UPS/PDU/environment/power/cooling monitoring (facilities evidence may explain incidents; Platform is not BMS/DCIM)

**Business invariant:** monitoring-path failure must not automatically become a device-down business conclusion. Reliable alert qualification depends on monitoring freshness, entity identity, topology/dependency context and maintenance/suppression state.

---

## 9. Sensor business model

- Platform uses **one Universal Sensor Runtime** with selectable roles (not multiple Sensor products/codebases).
- **Standard production roles (product):** NETWORK, INFRASTRUCTURE, FLOW, LOG.
- **Conditional product roles:** SYNTHETIC, CLOUD_API where already justified.
- **COMBINED:** only for lab/dev/small explicitly governed deployment profiles unless separately certified.
- Current implementation role `STREAMING_EVENT` is an **implementation mismatch / migration gap**, not the long-term customer-facing role taxonomy (see PRD/Architecture). Business documentation must not treat `STREAMING_EVENT` as the future product role.
- Placement prefers network locality while respecting reachability, capacity, HA, protocol access and policy.
- Customers may choose automatic placement, recommendation/approval, manual placement or Sensor Pool assignment.
- Capacity is represented by workload (Monitoring Work Units), not device count alone.
- Customers may deploy multiple Sensor Pools by region/country/site/security zone.
- Sensors continue polling during transient Core/WAN loss and buffer locally; Sensor disk follows the same **80%** safety-protection philosophy as Core.
- During Core failover, Sensors continue collecting/buffering while capacity permits (absolute zero-loss is not promised).

---

## 10. Agent business model

- Windows and Linux Agents are separate builds from Core and Sensor.
- Agents connect initially to Core; Core directs each Agent to an appropriate available Sensor based on location, capability and capacity.
- Agent enrollment feeds a **governed onboarding/review lifecycle**. Newly enrolled Agents do not silently become licensed/actively monitored unless an explicit authorized trusted auto-approval policy applies.
- Agent-backed endpoint identity is anchored to canonical entity identity plus unique agent/mTLS identity. IP/MAC are mutable observations and must not alone create a new endpoint.
- Branch/edge Agents support branch-to-DC network tests to expose asymmetric path issues.
- Agents buffer locally and support multi-endpoint failover behavior appropriate to their deployment.

---

## 11. Discovery and onboarding

All normal provisioning is UI-driven.

Supported business workflows:

- Individual device/entity provisioning
- Bulk CSV/XLSX import with staging/validation
- Automatic discovery of network devices
- Agent enrollment
- API/REST connector provisioning
- Cloud, virtualization, storage, database and Kubernetes connector provisioning

**Onboarding outcomes:**

- Optional customer-entered Display Name and Description; blank Display Name uses best discovered name/hostname/alias and falls back to IP only when better identity is unavailable. Customer-entered display names are never silently overwritten by later discovery.
- Bulk onboarding normalizes safe fields, detects duplicates and exposes exact failures with evidence and remediation—not merely “failed.”
- Sensitive credential strings must not be silently trimmed; Credential Profiles are preferred over secrets in spreadsheets.
- Approve / reject / unmanaged / exclude decisions are tenant-scoped, authorized and audited.

Discovery/onboarding depends on Credential Profiles, eligible Sensors, capacity and identity/deduplication. It must not auto-activate licensed monitoring without policy.

---

## 12. Network monitoring (business)

Business outcome: operators trust network health signals.

Dependencies: Device Packs, SNMP/flow/path evidence, Sensor NETWORK (and FLOW when volume warrants), Alert Qualification Engine, maintenance calendars.

Must support deep protocol coverage subject to packs (see §8). Flow analytics must not treat “No Flow” as proof of no traffic, nor treat flow anomaly alone as device-down.

---

## 13. Infrastructure monitoring (business)

Business outcome: server, hardware, virtualization and storage health are operable from the same Platform as network.

Dependencies: INFRASTRUCTURE Sensor role, Agents and/or agentless protocols, Credential Profiles, host↔service dependency graph for RCA.

Collection failure is a **monitoring-health** outcome unless corroborating evidence supports host failure.

---

## 14. Applications and services (business)

Business outcome: service assurance via synthetics and dependency-aware views without requiring full APM initially.

Dependencies: SYNTHETIC role (where used), service relationships, alert qualification.

---

## 15. Databases (business)

Business outcome: database availability and health visibility for authorized packs without unsafe default SQL execution.

Dependencies: database Credential Profiles, host dependency monitoring, controlled query monitors where authorized.

---

## 16. Cloud and Kubernetes (business)

Business outcome: cloud accounts and Kubernetes clusters appear as first-class managed estates with vendor IDs normalized into Platform authority (vendor IDs never override tenant/entity authority).

Dependencies: connectors, OTel ingestion where authorized, cardinality/throttle controls, connector health separation from resource health.

---

## 17. Telemetry, log and flow (business)

Business outcome: high-volume telemetry is usable and does not starve critical polling.

Dependencies: FLOW/LOG product roles, local Sensor spool, Kafka (internal), ClickHouse analytics, cardinality and noisy-neighbor controls, pipeline integrity visibility.

---

## 18. NCM (business)

Business outcome: configuration backup, versioning, diff, compliance, controlled push, rollback, approval and audit.

Dependencies: privileged credentials, maker-checker, blast-radius controls, Sensor-near execution, separation of NCM workers from routine polling capacity.

Autonomous closed-loop remediation is **out of initial product scope**; assisted recommendations and operator-controlled workflows come first.

---

## 19. IPAM (business)

Business outcome: IPv4/IPv6 address management with subnet hierarchy, VRF/VLAN context, DNS/DHCP integration where available, utilization, conflicts, history and forecasting—integrated with canonical inventory, not a competing silo.

---

## 20. Topology and dependency (business)

Business outcome: flexible topology and maps (physical, L2, L3, routing, MPLS, VXLAN/EVPN, SD-WAN, traffic, infrastructure, service, causal and geographic) plus customer-defined logical/business-service relationships.

Dependencies: relationship provenance/confidence, temporal graph history for incident windows.

---

## 21. Alert, incident and RCA (business)

Business outcomes:

- Separate events, alerts and incidents; preserve child/symptom evidence under correlation
- Qualify failures before escalating avoidable noise
- Provide RCA with inspectable evidence, contradictions, missing evidence and **INCONCLUSIVE** when warranted
- Measure false-alert and qualification effectiveness continuously

**Dependency statement (systematic):** Reliable alert qualification depends on monitoring freshness, entity identity, topology/dependency context, credential/path health and maintenance/suppression state. RCA quality depends on the same evidence plus change/NCM/path/flow inputs where available. AI/GNN output is hypothesis support, not unquestioned causal truth.

---

## 22. AI-assisted operations (business)

Business outcome: operators receive grounded search, summary, explanation, runbook suggestion and investigation help through an AI Gateway (Gemini initially; future private/local/offline providers).

Constraints:

- AI must not be decorative
- Prompt/model output cannot grant privilege or execute privileged actions
- Core monitoring/RCA must remain operational when hosted AI is unavailable
- Sensitive data masking/boundaries apply before model exposure where configured

---

## 23. Reporting, SLA/OLA and service assurance (business)

Business outcome: Platform shall provide an enterprise Service Assurance capability covering customer SLA, internal-team OLA, provider/underpinning SLA and SLI/SLO objectives from one evidence model. It shall combine monitoring telemetry, qualified state, topology/service dependencies, synthetic/API evidence, incidents and governed external ITSM/CMDB/security/provider inputs without making an external system the monitoring truth.

Platform shall ship editable out-of-box SLA/OLA/report templates for common large-enterprise operating patterns across generic enterprise, banking/financial services, manufacturing, telecom, data-center/colocation, ISP/carrier and cloud/enterprise-IT environments. Templates are starting points, not contractual promises; customer targets, calendars, formulas, exclusions and ownership remain customer policy.

Customers shall be able to create their own SLA/OLA policies from governed available parameters using a safe typed expression/formula engine. Arbitrary Python, JavaScript, SQL or other executable customer code is not part of SLA/report authoring.

Service assurance shall support reusable service-time profiles, IANA timezones, holidays, countries, sites, business hours, shifts, teams, regions, business units, environments, criticality, providers and customer tags. Team/shift attribution must preserve historical handoff evidence; production-authoritative Team scope depends on the governed tenant-scoped Team identity contract.

Contract measurement shall support expected/target and minimum thresholds, measurement windows, per-ticket/per-CI/per-service/per-site/per-provider/population and impacted-unit measurement, versioned inclusion/exclusion rules, maintenance, approved deviations, degraded-but-still-serving treatment, continuous-improvement measures and effective dates. Availability derived from direct monitoring/service evidence must not be replaced by ticket arithmetic where the policy declares monitoring evidence authoritative.

Platform shall preserve raw result, exclusions/adjustments, data-quality/completeness, scope snapshot, policy/formula/calendar version and final result as reproducible evidence. Finalized periods shall not silently change; corrections use an audited restatement workflow. Insufficient monitoring evidence produces explicit provisional/partial/insufficient-evidence state rather than fabricated compliance.

Before activation or material change, authorized users shall be able to simulate/backtest an SLA/OLA policy against retained historical evidence so policy impact is visible before publication.

Reporting shall support scheduled/on-demand obligations, business-day-aware due rules, recipients, approval/attestation, delivery status and drill-down from scorecard to outage/incident/ticket/CI/telemetry evidence. Generated/downloaded/scheduled report artifacts are **CSV and/or PDF only**. APIs remain separate integration/query interfaces and are not report artifact formats.

PDF is the human/governance artifact; CSV is the detailed machine-readable evidence artifact. Report execution must be bounded and lower priority than live telemetry, alerting and incident correctness.

Platform may consume billing, procurement, finance, project or external service-management data for governed formulas/reporting, but does not become an ERP, procurement, billing or ITSM system of record.

Changing a user’s display timezone must not silently alter SLA/OLA measurement semantics.

## 24. Integrations (business)

Business outcome: ITSM and notification integrations (ServiceNow, BMC, SDP, SummitAI, Freshservice, JSM, REST/webhook, email, Teams, Slack, WhatsApp Business, SMS/PagerDuty where authorized) with durable outbound behavior and Integration Test Sandbox.

External ticket success is supporting evidence, not Platform recovery proof.

---

## 25. Administration (business)

Business outcome: day-2 administration is UI-complete for tenants, sites/groups, users/access, authentication, credentials, discovery, monitoring profiles, alert policies, device-down validation, maintenance, notifications, ITSM, Sensors, Agents, retention, HA visibility, AI policy, licensing, security and Platform Operations—without source edits for normal customer behavior.

---

## 26. IAM and security (business)

Platform is intended for banks, defence and global enterprises. Security architecture must be designed from Day 1 for strong enterprise controls, including:

- TLS everywhere appropriate and mTLS between trusted Platform components
- HTTPS-only UI
- OIDC/SAML/LDAP/AD/Entra integration; MFA
- Resource-scoped RBAC and explicit deny semantics
- Secrets encrypted at rest and never sent to browsers unnecessarily
- Certificate/PKI lifecycle management
- Scoped API tokens/service accounts
- Signed software/packages and Device Packs; SBOM and dependency provenance
- Vulnerability and licence scanning
- Tamper-evident audit capabilities (target)
- Break-glass and temporary elevation controls
- NCM/CLI maker-checker and command guardrails
- Sensitive-data masking before storage/AI where configured

Architecture must support alignment with high-assurance expectations (ISO 27001 controls, SOC 2 control families, PCI-oriented controls, RBI/BFSI expectations, defence hardening, FIPS-capable cryptography where required). **No certification may be claimed until formally achieved.** Exact certification roadmap is TBD.

---

## 27. Audit and governance (business)

Business outcome: material actions across auth, RBAC, provisioning, credentials, policies, alerts, NCM/CLI, IPAM, AI, Sensors, HA, licence, retention, backup and upgrade are attributable, queryable and exportable to SIEM where configured.

Target includes durable/tamper-evident audit. Common emitter logging alone is not equivalent to durable audit persistence (implementation gap tracked under production-certification programme #92; Architecture states the target).

---

## 28. Data retention and protection (business)

Customers control normal retention and rollups through UI by data class and scope:

- raw retention; rollup resolutions and retention; archive destination/duration
- emergency purge priority within safe limits
- inheritance/overrides by tenant/region/site/group/device/data type where appropriate
- estimated storage impact before material changes

**Storage Protection Engine (mandatory):**

- 80% utilization is the non-disableable emergency safety watermark for Core and Sensor managed data volumes
- Warn and forecast before reaching the watermark
- At/above watermark, reclaim eligible oldest raw telemetry/log/flow first
- Preserve rollups/aggregates longer than high-resolution raw where possible
- Do not indiscriminately delete protected configuration, identity, current inventory, security and active operational state
- Show exactly what was removed and why; target safe headroom, not 80.1%→79.9% thrash
- Legal/investigation hold can protect incident evidence from normal purge, subject to emergency capacity constraints and explicit warning

Historical telemetry is less important than continuity of live monitoring.

---

## 29. Backup, restore and DR (business)

- HA is distinct from DR
- DR architecture is mandatory in v1.0 business scope
- Exact contractual RPO/RTO and automated-vs-manual DR activation remain **TBD — Decision Required**
- Backup/restore must cover configuration, PostgreSQL state, ClickHouse data/metadata policy, Device Packs, dashboards, reports, integrations, policies, certificates/keys according to secure process and other required state
- Restore testing/integrity verification is required

---

## 30. HA and business continuity (business)

A HA installation shall use exactly two full Core nodes:

- Core-A and Core-B; one Active, one Standby in normal operation
- Both continuously synchronize required persistent state
- Failover target: **less than 60 seconds** for Core UI/control service restoration under certified failure scenarios
- Sensor polling continues independently through Core failover; data is buffered and replayed as capacity permits
- No third customer-visible Core/governance node is part of the required HA topology
- Dual-active/split-brain must be prevented; ambiguous partitions must not create two authoritative active Cores

Sensors/Agents should use the stable **Core HA endpoint** rather than remain permanently bound to a single physical Core address.

**Business note:** exactly-two-Core HA is a fixed product requirement; detailed fencing/failover/rejoin mechanisms for stateful services remain open engineering P0s until certified (Architecture / programme #92). Documentation must not pretend those mechanisms are already certified.

---

## 31. Installability and upgrades (business)

### 31.1 Development isolation (engineering business constraint)

Platform engineering must not interfere with other software on an engineer’s workstation. Routine development, integration environments, stateful dependencies and automated tests run under Docker Compose. Engineers must not be required to install project-specific Java, Go, Python, Node, Kafka, PostgreSQL, ClickHouse or Valkey on the host OS. Local environments must tolerate laptop reboot via named volumes.

### 31.2 Production packaging

Customers must not be forced to install Docker to run production Platform. Certified native packages are required:

- Core: `.deb` and `.rpm`
- Sensor: `.deb` and `.rpm`
- Linux Agent: `.deb` and `.rpm`
- Windows Agent: compiled code-signed Windows Service, MSI, plus BAT wrappers for scripted install/uninstall

Core, Sensor and Agents are separate release artifacts with independent versions, compatibility rules and rollback capability.

### 31.3 Installer experience

Core installer asks Standalone vs HA. For HA it collects HA endpoint/VIP and peer information; first Core enters waiting/ready-for-peer; second completes secure pairing and synchronization.

### 31.4 Reboot and failure continuity

Every mandatory runtime component required by the selected deployment profile must be installed in an enabled and supervised state. Core, Sensor and Linux Agent services, plus any separately managed mandatory Core runtime dependencies (including PostgreSQL, ClickHouse, Kafka and Valkey where they run as separate services), auto-start at OS boot and recover from unexpected process failure through `systemd` or the approved component supervisor. Windows Agent registers as an automatically starting Windows Service with SCM recovery. A machine restart or recoverable process crash must not require routine customer action to restore monitoring/control service. Automatic boot/start and supervised recovery must preserve each component's existing authority, role, fencing, replication, durability and rejoin contracts and must not self-promote Standby, bypass fencing/replication/safe-rejoin rules, create dual-active authority, repeatedly force-start known-unsafe local state, or treat process existence as recovery success. If safe automatic recovery cannot be established, the component remains FAILED, DEGRADED or RECOVERING with actionable evidence and a governed operator/runbook path; standalone/non-HA bounded supervised restart remains mandatory. Exactly-two-Core HA authority/fencing and stateful failover/rejoin remain owned by BUILD-120–124. Optional, scheduled or on-demand workers are not required to remain resident when no enabled workload requires them. Persistent crash loops must remain visible and diagnosable rather than being hidden by endless restart.

### 31.5 Upgrades

Upgrades require preflight checks, drain/rolling behavior where applicable, verification and rollback strategy. Sensors should be upgradable without unnecessarily interrupting monitoring through pool drain/reassignment. Canary/feature-flag controls apply where product requirements authorize.

---

## 32. Supportability (business)

Every component must maintain enough structured operational evidence for support and engineering to determine what failed, when, where, why, what was retried, whether recovery succeeded and what customer action is required.

- Useful but bounded logging; retention/rotation and 80% storage-safety apply
- Secrets and credentials must not be logged
- Support Bundle with automatic secret masking
- Stable customer-facing error codes and correlation IDs
- Customer-visible Platform Health

---

## 33. Operational diagnostics (business)

Authorized NOC/operators must verify and investigate from entity/alert/incident context using Platform-controlled Core/Sensor/Agent paths—not direct browser-to-device networking.

Business expectations include bounded on-demand diagnostics, multi-vantage verification, Verify Now packs, Live Evidence, Investigation Sessions and risk-tiered privileged actions (CLI/NCM) with proportionate RBAC/JIT/MFA/reason/approval/audit.

---

## 34. Licensing business behavior

Platform licensing may support annual, perpetual, device, element and module-based commercial models. Commercial quantities, packaging and pricing remain Founder-reserved.

### 34.1 Continuity behavior (non-grace)

Normative licensing continuity that is **not** in commercial conflict:

- 30-day advance expiry warning
- From 7 days before expiry, every dashboard displays a prominent banner with days remaining
- Historical data remains available and must not be deleted merely because the licence expires
- Licence administration remains available so authorized users can apply an extension/new licence
- Slight over-entitlement should generate clear commercial warnings rather than randomly dropping devices; exact overage policy is TBD

### 34.2 Post-expiry grace — Founder-approved 15 days (2026-09-30)

**Authority:** Founder decision **2026-09-30** — post-expiry licence grace is **15 days** (`EXPIRED_GRACE_15D`).

This decision **supersedes**:
- prior seven-day post-expiry authority;
- normative `FOUNDER-LIC-GRACE UNRESOLVED`;
- any normative **7 vs 15** unresolved conflict.

Canonical continuity state machine (product):

`VALID → EXPIRING_30D → EXPIRING_7D_BANNER → EXPIRED_GRACE_15D → POLLING_SUSPENDED`

Important distinction (must not be collapsed):

- **T-7 pre-expiry warning/banner** = 7 days **before** expiry (unchanged, governed);
- **15-day post-expiry grace** = polling may continue for 15 days **after** expiry.

Normative behavior:

- polling continues during the 15-day post-expiry grace;
- after the 15-day grace ends, polling stops unless the licence is manually extended/renewed;
- historical data remains accessible; no licence-expiry data deletion;
- commercial quantities, packaging, pricing and SKUs remain Founder-reserved.

Historical/superseded provenance only (non-normative): earlier Mother Reference material recorded a seven-day post-expiry grace and temporarily left `FOUNDER-LIC-GRACE` unresolved between 7-day and 15-day historical/source values. Those statements are **superseded** and must not be treated as current commercial truth.

Authorized administrators shall see licensed vs used entities/elements/modules and forecast approaching entitlement thresholds/expiry. Usage foresight does not replace expiry/grace behavior.

---

## 35. UX and accessibility expectations (business)

- Role-aware navigation; readable fonts; light/dark theme; density options
- Accessibility, keyboard navigation, contrast; no color-only status
- Server-side pagination/virtualization for large estates
- Operators may view the same absolute event in their local IANA timezone, tenant default, UTC or another supported zone **without changing operational meaning** of monitoring data
- Changing display timezone must not silently alter alert/SLA/maintenance schedules, polling behavior or stored telemetry
- Host/OS timezone of Core/Sensor/Agent hosts must not redefine product semantics when deployments move between regions
- Shared dashboards/views and scheduled reports that render local wall-clock times must disclose the timezone used

Detailed UI contracts remain in `Platform_09_UI_UX_Master.md`.

---

## 36. Scalability and business sizing targets

### 36.1 Core

For the agreed reference workload, **8 vCPU / 32 GB RAM for up to 2,500 devices** is a mandatory engineering optimization target. It must be validated against the defined Standard Network Workload Profile (SNWP); it is not a claim that every possible 2,500-device workload fits that footprint. SNWP is a TARGET/PROFILE, not benchmark certification by documentation.

### 36.2 Sensor

Current safe planning baseline:

- Network Sensor: 4 vCPU / 16 GB RAM / 150 GB local SSD for approximately 800 normally monitored network devices under agreed reference conditions

Engineering density target to benchmark:

- 8 vCPU / 16 GB Sensor profile targeting approximately 2,000–2,500 normal network devices where workload permits

Device count is never the sole sizing dimension. Monitoring Work Units must account for metrics, interfaces, polling frequency, protocol, RTT, timeouts, Flow, syslog, gNMI, NCM and other work.

Exact certified Core scale beyond 15,000 devices and exact multi-Core trigger remain TBD pending benchmark.

---

## 37. Clean-room and OSS constraints (business)

Platform is built from scratch. Reuse is limited to approved open-source components and newly developed Platform code.

- Permissive OSS should be reused for commodity plumbing wherever practical (protocol parsers, SDKs, UI chart/table libraries, document generation, etc.)
- Platform shall build its own differentiated logic where generic OSS would create conflicting product models—especially canonical entity/relationship model, Device Pack/MIB/API studios, workload-aware Sensor scheduler/placement, policy inheritance, alert qualification, incident correlation, graph/causal RCA, governance/RBAC semantics, unified timeline, dashboard/report UX, storage-protection policy and licensing/entitlements
- Approved OSS must not force disclosure of proprietary Platform source or prevent commercial redistribution under the intended business model
- Top-level permissive licence alone is insufficient; transitive dependencies and bundled assets require review
- Vendor MIB/YANG/SDK/API examples/icons are separate IP from the OSS library used to parse them
- Customers must receive notices/licenses required by shipped dependencies
- Automated license/security watch; freeze/replace if suitability changes
- Do not force customers to deploy a heavyweight nested third-party product when a library can provide the primitive more efficiently

All dependencies must pass the Platform OSS Catalogue process (`Platform_07_OSS_Catalog.md`), SBOM, transitive licence scan, vulnerability scan and third-party-notice generation. Implementation teams may not add a runtime/build dependency merely because it is open source.

---

## 38. Business acceptance gates

The product may not be considered enterprise-ready unless all applicable gates are demonstrated with evidence (not documentation claims alone):

1. UI-only normal provisioning works for individual, bulk and discovery workflows
2. Provisioning errors are actionable and evidence-based
3. Core and Sensor remain within certified resource envelopes for reference workloads
4. HA pair fails over within the certified target without stopping Sensor polling
5. Storage pressure at the 80% watermark does not stop live monitoring and produces an auditable reclamation event
6. Alert qualification demonstrably reduces avoidable device-down false positives
7. RCA can return INCONCLUSIVE and every root-cause conclusion links to inspectable evidence
8. GNN/ML output cannot bypass protocol/evidence guardrails or remediation approval
9. Tenant isolation is enforced at API/query/data layers, not only UI
10. NCM/CLI privileged operations are authenticated, scoped, auditable and safely controlled
11. Upgrade, backup/restore and DR procedures are tested
12. Security, dependency and licence scans are part of release engineering
13. UI is HTTPS-only and common workflows meet certified responsiveness targets
14. Network & Infrastructure Observability Certification Profile (`NIOCP-P1-v1`; future certification profile, NOT YET CERTIFIED until applicable evidence exists) and `OBS-P1` capacity overlay shall demonstrate before applicable capability certification that eBPF socket telemetry and active path tracing operate within resource limits without degrading primary monitoring, transport, or RCA

Additional measurable success metrics Platform should expose or internally track include: provisioning success rate; time to onboard a standard device; Sensor utilization and polling completion; telemetry freshness/completeness; false-alert/qualification metrics; alert compression ratio; MTTD/MTTQ/time-to-root-cause; RCA accepted/partial/rejected/inconclusive rate; blast-radius accuracy where ground truth exists; incident reopen/flapping rate; dashboard/query latency; Core/Sensor footprint; storage compression/growth; ITSM sync success; upgrade/failover success; data-pipeline rejection/duplicate/late-data rates.

---

## 39. Explicit exclusions

Out of initial product scope:

- Full APM (code-level transaction tracing, Java/.NET bytecode manipulation/instrumentation, method-level timing)
- Continuous profiling or flame graphs
- Real User Monitoring (RUM) or session replay
- Full DPI / packet-level Layer-7 observability as a primary product capability, continuous always-on packet capture, TLS decryption, or packet payload reconstruction (bounded triggered packet evidence under OIV rules is explicitly preserved and not demoted)
- Mandatory GPU/NPU requirements
- A mandatory third Core node
- Dedicated external graph databases, dedicated trace databases, or dedicated Elasticsearch/OpenSearch clusters
- A separate eBPF collector backend or pipeline
- Autonomous closed-loop remediation in initial release
- Future Master Core / Regional Core federation as an initial implementation requirement
- Turning Platform into BMS/DCIM, unrestricted SIEM replacement, or uncontrolled automated NCM
- Claiming security/compliance certification before independent achievement

---

## 40. Explicit unresolved Founder / TBD items

| ID | Item | Status |
|---|---|---|
| TBD-01 | Final external product/brand name beyond working name “Platform” | Resolved: **AmrudAi** (commercial customer-facing product name; internal engineering remains "Platform") |
| TBD-02 | Contractual DR RPO/RTO and automated-vs-manual DR failover policy | TBD |
| TBD-03 | Exact compliance certification roadmap/timeline | TBD |
| TBD-04 | Final certified Core scale beyond 15,000 devices and exact multi-Core trigger | Benchmark required |
| TBD-05 | Exact Standard Network Workload Profile values after lab characterization | Engineering benchmark required |
| TBD-06 | Final GNN implementation framework and optional GPU profile | Architecture benchmark/licence review required |
| TBD-07 | Exact licensing overage policy before polling suspension | Commercial / Founder-reserved |
| TBD-08 | Exact default retention durations/rollup periods | Product defaults TBD; UI remains configurable |
| TBD-09 | Exact numeric UI/API SLOs after performance baseline | Benchmark required |
| TBD-10 | Future Master Core/Regional Core release scope | Future architecture only |
| **FOUNDER-LIC-GRACE** | Post-expiry licence grace duration | **RESOLVED 2026-09-30 (Founder):** **15 days** (`EXPIRED_GRACE_15D`). Supersedes prior seven-day post-expiry authority and historical `FOUNDER-LIC-GRACE UNRESOLVED` / 7-vs-15 conflict. T-7 pre-expiry banner remains distinct and governed. |

---

## 41. Cross-document companions

Normative companions (not duplicated here):

- `Platform_07_OSS_Catalog.md` — approved/candidate OSS
- `Platform_08_Requirements_Traceability.md` — global REQ/PRD index
- `Platform_09_UI_UX_Master.md` — UI/shell/dashboard contracts
- `Platform_10_Protocol_Security_Connector_Master.md` — protocol/credential/connectivity
- `Platform_11_Sizing_Capacity_Storage_Master.md` — MWU, sizing, retention constants
- `Platform_12_Decision_Source_Register.md` — decision provenance
- `Platform_19`..`Platform_23` — specialized domain supplements

---

## 42. A2.1 migration note

A2.1 absorbed historical Version 3.0 reconciliation blocks (BRD-OSS, BRD-COMP, BRD-LIC, BRD-STOR), v3.1 development/packaging/continuity sections and v3.2.2 display-timezone outcomes into the numbered normative sections above. Chronological appendices are removed as competing normative truth. Git history retains chronology. Formal identifiers remain in `Platform_08_Requirements_Traceability.md` without net-new or removed IDs under A2.1.

---

## Phase-1 endpoint operational depth — Issues #169/#170

For the first-GA Network & Infrastructure Observability scope, AmrudAi shall provide the following endpoint-operational outcomes for supported Windows/Linux estates:

1. **Installed-software visibility:** maintain a governed current inventory of installed software/packages with version, publisher/vendor, architecture, package/source identity, installation scope and provenance where the operating system exposes them reliably; retain evidence of installation, removal and version change.
2. **Customer-selectable process/service monitoring:** authorized users can choose discovered processes/services to monitor without scripts, YAML or local Agent configuration. Monitoring policy is expressed through the normal Monitoring Profile / Policy architecture.
3. **Business Criticality-driven defaults:** Criticality may suggest or automatically map to an effective Monitoring Profile, but Criticality and Monitoring Profile remain separate concepts and customer override is governed/audited.
4. **Triggered Fast RCA:** when a qualifying monitored condition occurs, AmrudAi rapidly gathers bounded, relevant evidence using the existing Endpoint Diagnostic Snapshot / Diagnostic Job architecture and correlates it with available network, virtualization, storage and hardware/OOB evidence in Core.

The business objective is materially shorter investigation time with low monitored-endpoint overhead and no false certainty.

Explicit exclusions remain: endpoint patch/deployment management, vulnerability scanning, licence metering, EDR, unrestricted shell/PowerShell, endpoint LLM/GNN causal engines, always-on PCAP/full DPI, and unsupported causal guesses.


---

## 43. v3.3.0 business outcomes — out-of-box operations, change assurance and enterprise integration

The Founder-approved v3.3.0 programme strengthens first-use value without widening Platform into a full developer APM or packet/NDR product.

Phase-1 business outcomes are:

1. **Useful out of box:** common enterprise technologies use Official Certified Capability Pack targets so discovery/onboarding can identify technology, preflight access, select a compatible monitoring profile and produce useful inventory, telemetry, topology, dashboards, alerts and diagnostics without weeks of manual monitor construction.
2. **No-context-switch investigation:** the existing Incident/RCA/Unified Operations Timeline experience is productized as an Investigation Workbench combining authorized metrics, alerts/incidents, Flow, logs/events, path/topology, configuration/change, Sensor health, synthetics, ITSM and other available evidence with common time/scope context.
3. **Operational service telemetry, not full APM:** Phase 1 accepts OTLP metrics and structured logs and preserves trace/span correlation context. Bounded sampled spans are Phase 1.5 subject to performance, privacy, cardinality, sampling and retention certification.
4. **Network Change & Configuration Assurance:** NCM must progress beyond backup/push into planned, approved, staged, validated and auditable change with explicit risk/blast-radius evidence, exact approval binding and certified rollback only where supported.
5. **Platform-wide enterprise workflow integration:** ITSM/CMDB integration reduces duplicate operational work across incidents, problems, changes, maintenance, approvals, NCM, remediation, RCA/PIR and inventory/business-service context while preserving Platform monitoring truth.

Phase-1.5 specialist outcomes include endpoint/switch-port intelligence, browser journey synthetics, deeper DB performance intelligence, virtualization optimization, NCM firmware lifecycle and the second-wave ITSM connector certification set. Deep VoIP specialist depth is Phase 2.

Explicit non-goals remain: full traditional developer-centric APM, continuous profiling/flame graphs/source navigation/full RUM/session replay, unlimited trace retention, full always-on DPI/NDR, uncontrolled automated NCM, another graph/CMDB, or customer-specific Core forks.

<!-- PHASE-1-1.5-ENVELOPE-2026-10-09 -->
## Phase 1 / Phase 1.5 business release envelope — Founder decision 2026-10-09

### Phase 1 — Single-Customer Enterprise
The first commercial priority is a production-grade single-customer AmrudAi deployment with one authoritative standalone Core and any certified number of customer-local Sensors/Agents. Phase 1 is a narrower deployment claim, not a lower-quality product tier. Enterprise security, monitoring correctness, durable edge buffering/replay, storage protection, backup/restore, upgrade/recovery, capacity evidence and operational supportability remain mandatory.

The intended Phase-1 outcome is a commercially sellable Network & Infrastructure Observability platform for one enterprise. Core capabilities include certified network/infrastructure collection, discovery/inventory, process/service and installed-software visibility, Flow/Syslog where certified, topology/evidence, semantic alert qualification including No Data != Down, incidents, deterministic RCA, reporting/SLA, certified notification/ITSM paths, IPAM and safe/read-only NCM capabilities. Optional technology modules may ship when independently certified without becoming universal Phase-1 GA blockers.

Phase 1 does not claim two-Core HA, automatic Core failover, shared MSP hosting, cross-customer isolation certification, cross-Core deletion-ACK RPO=0, regional federation or advanced autonomous remediation.

### Phase 1.5 — Enterprise HA + MSP
Phase 1.5 adds exactly-two-Core HA, single-Core-failure deletion-ACK durability, failover/rejoin, shared MSP/multi-customer certification, overlapping-address-space isolation, noisy-neighbour controls and broader privileged automation where independently certified. It reuses Phase-1 contracts and code; there is no separate product fork.

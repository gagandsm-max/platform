# Platform — Product Requirements Document (PRD)

**Document ID:** PLATFORM-PRD-001
**Version:** 3.3.0
**Status:** Authoritative Mother Reference — Complete Product Capability Authority (**A2.1 NORMALIZED_V1**)
**Product name:** Platform (internal engineering/build identifier) / AmrudAi (commercial/customer-facing product name)
**Depends on:** `Platform_01_BRD.md`
**Companions:** `Platform_03_Architecture.md`, `Platform_04_HLD.md`, `Platform_05_LLD.md`, `Platform_06_Dataflow.md`, `Platform_08_Requirements_Traceability.md`, specialized masters `Platform_19`..`Platform_23`
**Normalization state:** NORMALIZED_V1
**Last reconciliation:** 2026-10-11 — Issue #200/#202/#206 Tranche A Sensor RAM ratio correction (≥4 GB/vCPU; retains v3.3.0); prior Phase 1/1.5 certification-envelope reconciliation retained

---

## 1. Purpose and authority
This PRD defines **what Platform must do**. It translates the business requirements into functional, UX, administrative, security, HA/DR, data-management, AI/RCA, integration and non-functional product requirements.

Requirements use normative language:

- **SHALL / MUST** — required.
- **SHOULD** — strongly recommended; deviation requires documented architecture/product approval.
- **MAY** — optional capability.
- **TBD** — unresolved; must not be silently assumed.

Platform is a clean-room product. Industry products such as SolarWinds, ManageEngine and NetAI are sources of architectural/product learning only.

---


**Normative language:** SHALL/MUST, SHOULD, MAY, TBD. TBD must not be silently assumed.

**Maturity:** `SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED`. This PRD specifies product behavior; it does not certify implementation.

**Cross-document contract:** `Platform_07` (OSS), `Platform_08` (global IDs), `Platform_09` (UI detail), `Platform_10` (protocols), `Platform_11` (sizing), `Platform_12` (decisions). Specialized `PRD-OIV-*`/`PRD-ONB-*`/`PRD-EGC-*`/`PRD-MIA-*`/`PRD-EMC-*` remain authoritative supplements in their masters.

## 2. Product scope summary
Platform shall provide a **Unified Network & Infrastructure Observability** experience (commercial product name: **AmrudAi**) covering:

1. Network monitoring, active path observability and traffic analytics.
2. Infrastructure/server/hardware monitoring and bounded Linux process-to-network eBPF evidence.
3. Virtualization and storage.
4. Database monitoring.
5. Cloud and Kubernetes infrastructure and network depth.
6. OpenTelemetry metrics/logs (Phase 1) and future trace correlation reference.
7. Synthetic/service tests and Service Assurance (SLI/SLO/Error Budget).
8. NCM and privileged CLI.
9. IPAM.
10. Discovery, provisioning and inventory.
11. Topology, maps and temporal operational graph relationships.
12. Events, alerts, incidents and evidence qualification.
13. Graph-aware deterministic RCA, anomaly detection and explainable AI assistance.
14. Reporting and SLA.
15. ITSM/notification integrations.
16. Multi-tenancy, RBAC, security and audit.
17. Sensor/Agent administration, HA, DR, backup/restore and upgrades.
18. Data retention, rollups, archive, capacity forecasting and storage protection.
19. Extensibility through MIB Studio, API Studio and Device Pack Studio.

**Hard product boundaries:** Full APM, bytecode instrumentation, method-level timing, continuous profiling, flame graphs, full DPI, always-on bulk PCAP, TLS decryption, and packet payload reconstruction are excluded from Phase 1. (Existing bounded incident-triggered packet evidence under `PRD-OIV-002` remains active and is preserved).

---


## 3. Product information architecture
### 3.1 Primary workspaces

The primary navigation shall be role-aware and organized into these **top-level primary sidebar** workspaces (14-item taxonomy; aligned with `Platform_09` / UI foundation):

- Home
- Dashboards
- Inventory
- Topology & Maps
- Alerts & Incidents
- RCA & Investigations
- Network Configuration
- IPAM
- Reports & SLA
- Integrations
- Studios
- Administration
- Audit & Governance
- Platform Operations

**Provisioning & Discovery is not a top-level primary sidebar item.** It is a major product workspace nested under Inventory:

`Inventory → Provisioning & Discovery`

(with `/provisioning` as a child/compatibility destination under Inventory). Do not introduce a second navigation interpretation that places Provisioning & Discovery as a peer of Inventory in the primary sidebar.

The UI shall not expose 80+ dashboard links as one flat menu. Domain dashboards are grouped; entity-level dashboards open contextually through drill-down.

### 3.2 Global shell

Every authenticated page shall use a consistent shell with, where permitted:

- tenant/customer selector for MSP users;
- global search;
- breadcrumb context;
- time range;
- auto-refresh;
- global quick actions;
- notifications;
- contextual help;
- user/profile menu;
- favorites/recently viewed;
- command palette (`Ctrl/Cmd + K`) for advanced navigation/actions.

### 3.3 HTTPS only

The application UI shall be reachable only over HTTPS/TLS. There shall be no Platform application listener on TCP/80. Default customer access is TCP/443; customer-selected alternate HTTPS ports may be supported.

---


### Additional must-have workspaces (absorbed from PRD-CROSS)
The product shall include the following even where they are implemented as contextual workspaces rather than top-level navigation items:

Asset Reconciliation; Technology Lifecycle Risk; Backup & Restore; DR; Upgrade & Rollback; Desired vs Actual State; Change Impact/Dry Run; Path Analysis; Device/Time Compare; Baseline & Seasonality; Alert Storm/Flapping; Shift Handover; Circuit Inventory; Operational Timeline; Support Bundle; Platform Health; Licensing/Usage; Storage Forecast; AI Boundary/Feedback; Device Pack Certification/Test Lab; Migration Studio; Environment Readiness; Change Freeze/Calendars; Runbook Knowledge Base; Post-Incident Review; Intelligence Quality; Alert Policy Effectiveness; Monitoring Optimization; Sensor Placement Optimization; Certificate/PKI; Proxy; Data Sovereignty; API Governance; Query Protection; Housekeeping; Data Pipeline Integrity.


## 4. Tenant and tenancy requirements

**Feature contract (summary):**
- **Purpose/outcome:** Hard multi-tenant / MSP isolation without separate products
- **Subfeatures:** single-customer mode; MSP mode; overlapping IP handling; noisy-neighbor quotas
- **Primary persona:** MSP Super Admin / Tenant Admin
- **Dependencies:** IAM, RBAC, query layer, persistence tenancy
- **Preconditions:** tenant identity provisioned
- **Primary workflow:** create/select tenant → scoped operations
- **Product behavior:** isolation enforced server-side at API/authz/query/persistence; tenant-aware contracts remain mandatory in Phase 1 single-customer deployments
- **Failure/degraded behavior:** cross-tenant access denied; noisy neighbor throttled
- **Acceptance outcome:** negative cross-tenant tests pass for the declared certification envelope
- **Maturity boundary:** SPECIFIED (target); Phase 1 certifies single-customer enterprise with tenant-aware contracts; MSP/shared-customer adversarial isolation and noisy-neighbor certification are Phase 1.5. Do not infer IMPLEMENTED/CERTIFIED from this PRD alone.

| ID | Requirement |
|---|---|
| PRD-TEN-001 | Platform shall support single-customer and MSP/multi-tenant operating modes. Single-customer is the Phase 1 certification envelope; MSP/shared multi-tenancy certification is Phase 1.5. |
| PRD-TEN-002 | Tenant isolation shall be enforced in API, authorization, query and persistence layers, not only by hiding UI elements. |
| PRD-TEN-003 | MSP users may be scoped to one or multiple customers; customer users shall not see other customers. MSP-mode certification is Phase 1.5. |
| PRD-TEN-004 | Tenants shall have independent policy, dashboards, retention, integrations, credentials, groups and reporting scopes where allowed. |
| PRD-TEN-005 | Overlapping IP address spaces across tenants shall be supported through tenant-aware canonical identity. |
| PRD-TEN-006 | Resource usage/quota controls shall prevent one tenant from exhausting shared query/report/AI/ingestion resources in multi-tenant deployments. |

---


## 5. Core installation, packaging, HA and DR

**Feature contract (summary):**
- **Purpose/outcome:** Installable Core with Standalone (Phase 1 cert envelope) or exactly-two-Core HA (Phase 1.5 cert envelope) and mandatory v1.0 / Phase-1 DR capability
- **Subfeatures:** build separation; installer; HA Active/Standby; DR backup/replication readiness; native packages; auto-start
- **Primary persona:** Platform Super Admin / implementer
- **Dependencies:** PKI, networking, storage, peer Core for HA
- **Preconditions:** OS certification matrix met
- **Primary workflow:** install → configure topology → validate readiness
- **Product behavior:** Phase 1 standalone production-enterprise install; Phase 1.5 HA failover target <60s; Sensors continue buffering; HA ≠ DR
- **Failure/degraded behavior:** fail-safe prefer no dual-active; Sensors buffer; Phase 1 site-loss uses backup/restore/DR rather than Core HA
- **Acceptance outcome:** installer/HA/DR acceptance gates demonstrated for the declared envelope
- **Maturity boundary:** SPECIFIED; HA stateful mechanisms remain open P0 until Phase 1.5 certified (Architecture/BUILD-120–124). Phase 1 certificate must not imply HA.

### 5.1 Build separation

There shall be distinct installable artifacts for:

- Platform Core;
- Platform Sensor;
- Platform Agent (Windows and Linux builds as applicable).

### 5.2 Core installer

The Core installer shall:

1. Verify supported Ubuntu/RHEL version, CPU/RAM/disk, DNS, NTP/time, filesystem and required ports.
2. Ask whether deployment is **Standalone** or **HA**.
3. For standalone, configure the local Core endpoint and initialize the Core stack.
4. For HA, collect HA endpoint/VIP and peer information.
5. Allow the first Core to start in `WAITING_FOR_PEER`/HA-initialization state.
6. Allow the second Core to securely pair with the first using one-time/explicit enrollment information.
7. Establish one Active and one Standby Core and begin state synchronization.
8. Validate UI, internal data services, certificates and synchronization before reporting HA ready.
9. Generate an installation readiness/result report with errors and remediation.

### 5.3 HA behavior

PRD-HA-001..008 define the **Phase 1.5** exactly-two-Core HA certification envelope. They remain architecture/product requirements at maturity SPECIFIED until Phase 1.5 evidence exists. A Phase 1 standalone certificate must not imply these HA guarantees.

| ID | Requirement |
|---|---|
| PRD-HA-001 | Exactly two full Core nodes shall comprise the standard HA application pair. |
| PRD-HA-002 | Exactly one shall be Active and the other Standby during normal operation. |
| PRD-HA-003 | Both shall continuously synchronize required persistent state. |
| PRD-HA-004 | Certified Core failover shall restore UI/control-plane service in less than 60 seconds under supported failure scenarios. |
| PRD-HA-005 | Sensor polling shall continue during Core failover and transient Core unavailability. |
| PRD-HA-006 | Sensors shall buffer/replay unacknowledged telemetry as needed. |
| PRD-HA-007 | The implementation shall prevent dual-active/split-brain. Ambiguous failure may not promote both nodes. |
| PRD-HA-008 | HA status, sync lag, active node, last failover, reason and health shall be visible in Platform Operations. |

### 5.4 DR

DR capability is mandatory in v1.0 and remains mandatory for the **Phase 1** certification envelope. HA is distinct from DR: Phase 1 must certify backup/restore plus site-loss recovery/redeploy/endpoint-redirection behavior for its declared DR profile, without claiming two-Core HA continuity or HA zero-RPO. Product shall include:

- backup and restore workflows;
- DR state export/replication architecture;
- DR readiness validation;
- runbook and audit trail;
- test/verification capability.

Exact contractual/external RPO, RTO and automated/manual DR activation remain TBD and must be parameterized/configurable where architecture permits. Internal technical RPO/RTO engineering targets are Master technical authority.

---


### Native production packages and service continuity
### PRD-DEV-001 — Container-isolated engineering environment

All normal development and automated test stacks shall run under Docker Compose. The repository shall provide a one-command startup path, health-aware startup, deterministic teardown and documented reset/recovery. Project services and databases shall not depend on host-native project runtimes.

### PRD-DEV-002 — Reboot-safe developer state

The development environment shall distinguish durable named volumes from disposable test volumes. Normal laptop shutdown/reboot must not corrupt Platform state. After Docker daemon restart, services configured for development auto-resume must restart according to Compose policy and re-establish health without manual database repair.

### PRD-PKG-001 — Native production packages

Core, Sensor and Linux Agent shall ship as both `.deb` and `.rpm` packages for certified Ubuntu/RHEL versions. Windows Agent shall ship as a code-signed service executable and GA MSI, with install/uninstall BAT wrappers. Docker images are engineering/test artifacts and optional future deployment artifacts; Docker is not a mandatory customer runtime for the initial native installation model.

### PRD-SVC-001 — Auto-start and recovery

- Every mandatory runtime service required by the enabled deployment profile shall be enabled at install/activation time for automatic boot startup; this includes Core, Sensor and Linux Agent services and any separately managed mandatory Core data/runtime services such as PostgreSQL, ClickHouse, Kafka and Valkey.
- Windows Agent shall be installed as an automatically starting Windows Service with configured SCM recovery actions.
- Any unexpected recoverable process exit shall trigger supervised restart using bounded delay/backoff; routine customer intervention shall not be required to restore the affected service.
- Automatic boot/start and supervised recovery shall preserve the component's existing authority, role, fencing, replication, durability and rejoin contracts. Automatic recovery must not self-promote a Standby into an authoritative role; bypass Core Active/Standby fencing; bypass PostgreSQL/Kafka/ClickHouse replication, durability or safe-rejoin rules; create dual-active authority; repeatedly force-start a component whose local state is known unsafe, corrupt or incompatible; or treat PID/process existence as recovery success.
- If safe automatic recovery cannot be established, the component shall remain in an explicit FAILED, DEGRADED or RECOVERING state with actionable health/diagnostic evidence and a governed operator/runbook path rather than forcing unsafe restart. For standalone/non-HA deployments, normal bounded supervised restart remains mandatory.
- Exactly-two-Core HA authority/fencing and stateful service failover/rejoin mechanics remain owned by BUILD-120–124. This continuity requirement does not select Patroni, repmgr, ClickHouse Keeper, Kafka quorum topology, etcd, Consul, Raft, witness nodes, a third Core, or a new supervisor service.
- Dependency-aware startup shall not report a component HEALTHY merely because its process exists; readiness must reflect required dependency health.
- Optional, scheduled or on-demand workers may remain stopped when their capability/workload is not enabled or no work is due; "always on" applies to services required to keep the selected deployment profile operational.
- Services shall expose explicit states: STARTING, HEALTHY, DEGRADED, STOPPING, STOPPED, FAILED, RECOVERING.
- Repeated crash loops shall be surfaced as Platform/system alerts rather than being hidden by infinite restart.

### PRD-LOG-001 — Structured system/application diagnostics

Each component shall emit structured logs containing at minimum timestamp, severity, component, version, node/sensor/agent identity, tenant where relevant, event/error code, correlation/request ID where relevant and safe contextual attributes. Passwords, communities, private keys, tokens and authorization headers shall never be written in clear text.

Required log classes include startup, configuration load, registration/enrollment, connectivity, storage, database/message-bus interaction, polling/collection, provisioning, protocol errors, security events, HA/failover, upgrade/rollback, shutdown, crash/restart and recovery.

### PRD-LOG-002 — Supportability

The Administration and Platform Health workspaces shall expose service health, recent failures, restart count, last successful recovery and links to sanitized diagnostics/support-bundle generation according to RBAC.

---


## 6. Sensor runtime, roles and local continuity

**Feature contract (summary):**
- **Purpose/outcome:** One Universal Sensor runtime with role-based collection near the estate
- **Subfeatures:** NETWORK/INFRASTRUCTURE/FLOW/LOG; conditional SYNTHETIC/CLOUD_API; COMBINED lab; local spool; 80% protection
- **Primary persona:** Platform / Tenant Admin; NOC
- **Dependencies:** Core desired-state, credentials, capacity, mTLS
- **Preconditions:** Sensor enrolled and healthy
- **Primary workflow:** deploy Sensor → assign role → receive work → spool → upload
- **Product behavior:** headless; continues during Core/WAN loss while capacity remains
- **Failure/degraded behavior:** backpressure/reclaim under 80%; role isolation so Flow does not starve SNMP
- **Acceptance outcome:** Sensor continues through Core failover; durable replay idempotent
- **Maturity boundary:** SPECIFIED; production entrypoint may not yet wire full governed scheduler — Architecture states target vs gap


### Canonical Sensor role taxonomy (normative product)

| Role class | Roles | Notes |
|---|---|---|
| Standard production | `NETWORK`, `INFRASTRUCTURE`, `FLOW`, `LOG` | Customer-facing production roles |
| Conditional | `SYNTHETIC`, `CLOUD_API` | Where product requirements justify dedicated Sensors |
| Lab/dev/small | `COMBINED` | Only for explicitly governed lab/dev/small profiles unless separately certified |

**Implementation mismatch:** current runtime also exposes `STREAMING_EVENT` as a general streaming/event role. That implementation role must **not** remain the final product-facing substitute for explicit `FLOW` and `LOG` production roles. Status: **IMPLEMENTATION RECONCILIATION REQUIRED / migration gap**. Do not create multiple Sensor products/codebases. Do not change runtime code in A2.1 documentation work.

### 6.1 Universal runtime and roles

One Universal Sensor Runtime shall support selectable deployment roles. Product-facing production direction:

**Standard production roles**

- `NETWORK`
- `INFRASTRUCTURE`
- `FLOW`
- `LOG`

**Conditional dedicated production roles**

- `SYNTHETIC` — only where scale, security, network locality, API rate isolation or workload characteristics justify a dedicated role;
- `CLOUD_API` — same justification bar for cloud/API-heavy collection.

**Lab / constrained deployments**

- `COMBINED` — lab, development, small deployment or non-high-volume use unless separately certified.

**Runtime reconciliation**

Current runtime also exposes `STREAMING_EVENT` as a general streaming/event role. That implementation role must **not** remain the final product-facing substitute for explicit `FLOW` and `LOG` production roles. Status: **IMPLEMENTATION RECONCILIATION REQUIRED**. Do not create multiple Sensor products/codebases.

### 6.2 Network Sensor responsibilities

Network Sensor shall support, as enabled by policy/device capability:

- ICMP;
- traceroute;
- SNMP polling;
- traps/informs;
- CDP/LLDP/ARP/FDB discovery;
- routing-state collection;
- NETCONF/RESTCONF/gNMI;
- SSH discovery/diagnostics;
- NCM execution;
- IP SLA/SD-WAN and vendor Device Pack monitors;
- lightweight flow/syslog/event ingestion below capacity thresholds.

### 6.3 Infrastructure Sensor responsibilities

Infrastructure Sensor shall support:

- SSH;
- WinRM/WMI;
- Agent ingestion;
- Redfish/IPMI/iDRAC/iLO/CIMC;
- VMware/Hyper-V/Proxmox/Nutanix;
- storage APIs;
- database monitoring;
- Kubernetes/container APIs;
- OpenTelemetry;
- REST/API connectors.

### 6.4 Streaming/Event, Flow and Log Sensor roles

High-volume production Flow ingestion uses the `FLOW` deployment role. High-volume production Log ingestion/forwarding uses the `LOG` deployment role. Lightweight flow/syslog below capacity thresholds may remain on Network/Infrastructure Sensors where policy permits.

Current runtime `STREAMING_EVENT` remains an implementation/general streaming role pending reconciliation to the product-facing `FLOW`/`LOG` roles above.

High-volume NetFlow/IPFIX/sFlow/J-Flow and high-rate log/event ingestion shall move to dedicated `FLOW`/`LOG` (or conditional dedicated) Sensor capacity when the workload would risk starving polling workers. Smaller mixed deployments may use `COMBINED` or lightweight ingestion on Network/Infrastructure Sensors within capacity limits.

### 6.5 Sensor local continuity

Sensors shall:

- be headless for normal administration;
- receive policy/configuration centrally;
- use a durable local spool for Core/WAN disruption;
- expose queue depth, oldest buffered age, disk usage and projected time remaining;
- never stop polling solely because Core is temporarily unreachable while local capacity remains;
- support idempotent replay and sequence tracking;
- apply the 80% local-storage safety policy;
- preserve higher-priority/recent operational data preferentially under extreme local pressure.

**Phase 1 deletion-ACK durability:** a deletion-authorizing Sensor ACK must survive the certified standalone Core failure matrix (application/process/service crash/restart, governed host reboot and other explicitly certified recoverable failures). Catastrophic destruction/loss of the single Core/storage is not Phase-1 zero-RPO HA; that belongs to backup/DR and the declared Phase-1 RPO/RTO profile. Phase 1.5 adds `ACKED-FOR-DELETION TELEMETRY RPO = 0` for the certified permanent one-Core HA failure matrix.

**Phase 1 Sensor authority:** assignment generation/fencing, lease ownership and stale privileged-work rejection remain mandatory with multiple Sensors even when Core is standalone. Cross-Core failover re-anchor and push-protocol HA continuity are Phase 1.5.

---


## 7. Sensor placement, pools and capacity
### 7.1 Placement modes

During provisioning customers shall be able to select:

- **Automatic — Recommended**;
- **Recommend and Require Approval**;
- **Manual Sensor**;
- **Sensor Pool**.

### 7.2 Placement factors

The Sensor Placement Engine shall evaluate:

- actual reachability;
- RTT/network proximity;
- packet loss/timeouts;
- site/region/country/security-zone affinity;
- preferred/allowed/forbidden Sensor pools;
- protocol reachability;
- CPU/RAM/disk;
- polling completion latency/backlog;
- current Monitoring Work Units;
- expected workload of the new entity/profile;
- HA/failover capacity;
- maintenance/drain state;
- tenant and credential policy.

The nearest geographic Sensor must not be selected blindly if it is unhealthy or overloaded.

Platform may recommend Sensor placement improvements from observed locality, capacity and completion evidence (**Sensor placement learning**). Recommendations require operator/admin approval unless an explicit safe auto-rebalance policy is enabled; learning never silently overrides pinned/manual placements.

### 7.3 Sensor pools and redundancy

Sensor Pools shall support:

- no redundancy;
- N+1;
- N+2;
- N+K;
- 1:1 where explicitly required;
- active-active pooled capacity.

Redundancy is evaluated per independently reachable locality/pool. One global spare does not count as redundancy for an isolated site that cannot be reached from it.

### 7.4 Rebalancing

Platform shall monitor workload and may recommend or automatically rebalance workloads when policy permits. Rebalancing shall use hysteresis to avoid device bouncing due to small/transient resource fluctuations.

### 7.5 Capacity representation

UI shall show both:

- approximate entity/device capacity;
- calculated workload percentage based on Monitoring Work Units.

Device count alone shall not be the sole enforcement model.

---


**Maturity note:** logical Region/Site/Sensor Pool product model is required; physical persistence of pools/regions may be incomplete on current main (Architecture/#92). Do not treat UI/logical design as certified physical schema.

## 8. Agent requirements
- Separate Windows and Linux Agent builds.
- Initial Agent enrollment takes a stable Core endpoint.
- Core authenticates/enrolls Agent and shall select an appropriate eligible Sensor during enrollment for a provisional authenticated Agent→Sensor control/transport association used for certificate/identity authority, onboarding/bootstrap validation, bounded health/bootstrap evidence and candidate reconciliation.
- The provisional Sensor association does not itself activate full monitoring, commercial/licence consumption, normal monitoring profiles, customer operational alerts or recurring monitored workload.
- Normal monitoring workload/profile becomes active only after explicit authorized approval or an authorized trusted auto-approval policy and after normal entitlement, capacity and policy checks. Activation retains or re-evaluates the provisional Sensor through the same Sensor Control assignment/versioning authority; no second Sensor authority mechanism is created.
- Core may reassign Agent to another Sensor based on locality/capacity/HA policy.
- Agent shall support secure mTLS identity and certificate rotation.
- Agent shall support host metrics, process/service monitoring, selected logs and local network experience tests.
- Edge network tests shall include ICMP, packet loss, RTT, jitter, traceroute/path change, TCP, DNS, TLS and HTTP/S as policy enables.
- Agent must have local buffering and multiple endpoint/failover behavior.
- Agent upgrade shall support staged/canary deployment and rollback where technically possible.

---


## 9. Provisioning, discovery and identity

**Feature contract (summary):**
- **Purpose/outcome:** UI-driven onboarding without silent licence activation
- **Subfeatures:** individual; bulk; auto-discovery; preflight; firewall matrix; identity/dedup; review queue
- **Primary persona:** Tenant Admin / Network Engineer
- **Dependencies:** credentials, Sensors, packs, capacity
- **Preconditions:** eligible Sensor and Credential Profile
- **Primary workflow:** wizard/import/discovery → preflight → review → activate
- **Product behavior:** actionable failures with evidence; Display Name rules
- **Failure/degraded behavior:** block on capacity/auth/firewall with Fix & Retry
- **Acceptance outcome:** no silent licensed monitoring without policy
- **Maturity boundary:** SPECIFIED; customer Discovery authorization may be fail-closed while IAM integration incomplete (#92)

Navigation: **Inventory → Provisioning & Discovery**.

Tabs:

- Add Entity
- Bulk Import
- Auto Discovery
- Discovery Results
- Provisioning Jobs
- Sensor Assignment
- Unmanaged
- Excluded/Rejected
- Templates

### 9.1 Individual provisioning

Wizard shall include:

1. Identification — address/FQDN, optional Display Name, optional Description, site, criticality, tags.
2. Monitoring type — Auto Detect, SNMP, REST API, gNMI, NETCONF, SSH, ICMP only, Device Pack and domain-specific connector options.
3. Credential Profile.
4. Sensor assignment mode.
5. Preflight tests from the actual candidate Sensor.
6. Device/resource identification and Device Pack/profile selection.
7. Sensor capacity impact preview.
8. Monitoring/alert/topology/NCM capability preview.
9. Provision.
10. Track provisioning job and corrective actions.

### 9.2 Naming

If user enters Display Name, it is preserved.

If Display Name is blank, Platform chooses the best available value in this logical order where available:

1. discovered configured alias/display name;
2. discovered hostname/device name;
3. SNMP `sysName`;
4. API/vendor hostname;
5. DNS FQDN/hostname;
6. IP/address final fallback.

Technical hostname and customer Display Name remain separate fields.

Description is optional. Customer-entered description is preserved. Discovered technical description (`sysDescr`, model/vendor metadata, API descriptions) is stored separately and may be proposed as a default.

### 9.3 Bulk import

Bulk import shall support CSV and XLSX. Customers should use Credential Profile references rather than embedding secrets.

Import shall stage data before provisioning and detect/correct safe issues such as:

- BOM/line-ending normalization;
- blank rows;
- leading/trailing spaces in non-secret fields;
- invalid IP/FQDN;
- invalid port;
- invalid/unknown site/group/tag/profile;
- duplicate row/entity;
- invalid security protocol/version value;
- missing required values.

Sensitive values with leading/trailing whitespace shall generate an explicit warning and require choice to preserve exact value or trim it.

The staging table shall allow inline correction and `Fix & Retry`; users must not be forced into repeated edit-download-upload cycles.

### 9.4 Preflight Diagnostic Engine

The same validation engine shall be used for manual, bulk, discovery-approved and connector provisioning.

It shall distinguish, where evidence permits:

- input invalid;
- DNS failure;
- route unavailable;
- device unreachable;
- port/protocol timeout;
- authentication failed;
- authorization failed;
- protocol disabled/suspected disabled;
- firewall/ACL suspected;
- credential profile invalid;
- TLS/certificate failure;
- duplicate/identity conflict;
- no eligible Sensor;
- Sensor capacity exceeded;
- profile/Device Pack incompatibility;
- API schema/response mismatch;
- MIB dependency required.

Diagnostic results shall state **what failed, evidence, what is uncertain, likely causes and recommended next actions**.

### 9.5 Firewall/connectivity matrix

Provisioning shall generate a required connectivity matrix using actual Sensor source addresses and target scopes/protocols. This may be exported for customer network/security teams.

### 9.6 Auto-discovery

Network discovery shall support:

- CIDR/IP ranges;
- multiple ranges;
- seed devices;
- IPAM subnet scope;
- exclusions;
- ICMP;
- SNMP;
- SSH;
- REST/API;
- NETCONF/gNMI;
- CDP/LLDP/ARP/FDB and routing-neighbor discovery where available.

Discovery shall not automatically consume licences or monitor every discovered entity. Results enter a Review Queue: Provision, Leave Unmanaged, Ignore, Exclude Permanently.

### 9.7 Identity/deduplication

Canonical entity identity shall use tenant-aware combinations of stable signals such as serial number, SNMP engine ID, sysObjectID, MAC, hardware UUID, cloud resource ID, hostname and network namespace rather than IP address alone.

---


### Provisioning completeness (absorbed from PRD-PRV-PLUS)
1. Manual, bulk and discovery provisioning use one common validation pipeline; none may bypass preflight checks.
2. User may enter `Display Name` and `Description` for network, infrastructure, API or any other entity. If Display Name is blank, resolution order is customer alias/device alias → discovered hostname/sysName/API name → DNS/FQDN → IP/address fallback. Customer-entered name is never silently overwritten by later discovery.
3. Sensitive values are never silently trimmed; non-secret textual fields may be normalized with a visible correction record.
4. The diagnostic result differentiates unreachable path, protocol timeout, authentication failure, authorization failure, likely ACL/firewall blockage, protocol-not-enabled suspicion, TLS failure, unsupported profile, duplicate identity, no eligible Sensor and capacity exceeded.
5. Every diagnostic result contains evidence and recommended next action. AI may explain evidence but may not invent certainty.
6. Sensor selection is tested from the Sensor that will actually perform monitoring.
7. Bulk import provides interactive staging and `Fix & Retry`, not repeated spreadsheet rework.
8. Device discovery never immediately consumes licensing/starts polling without the customer's configured auto-approval policy or review queue.
9. Capacity simulation runs before bulk activation and can block unsafe provisioning.


## 10. Credential and security protocol requirements
Credential Profiles shall be centrally managed, encrypted and scoped by tenant/site/group/Sensor policy.

Network/device provisioning must support the following protocol/security categories as certified by device/library compatibility:

| Domain | Required support |
|---|---|
| SNMP | v1, v2c, v3 NoAuthNoPriv, AuthNoPriv, AuthPriv |
| SNMPv3 auth | MD5/SHA-1 only for legacy compatibility where required; SHA-2 variants where supported |
| SNMPv3 privacy | DES only as disabled-by-default legacy option; AES family where supported |
| SSH/NCM | SSHv2 password, public key, keyboard-interactive where required, privileged/enable workflow |
| NETCONF | NETCONF over SSH |
| REST/RESTCONF | HTTPS Basic, Bearer, API key, OAuth2, custom headers, mTLS |
| gNMI/gRPC | TLS/mTLS and supported username/token modes |
| Syslog | UDP/TCP and TLS; secure transport preferred |
| Config transfer | SCP/SFTP; insecure legacy mechanisms disabled by default |
| Windows | WinRM HTTPS; Kerberos/Negotiate/NTLM where required by customer environment |
| Hardware | HTTPS/session/token/basic as applicable to Redfish/vendor APIs |
| Agent/Sensor | certificate-based mTLS |

Legacy/insecure protocols shall be clearly marked **Legacy / Not Recommended**, disabled by default where feasible, explicitly enabled by authorized users and fully audited.

Credential permissions shall distinguish **USE** from **REVEAL**.

---


## 11. Network monitoring requirements
### 11.1 SNMP

Platform shall support:

- v1/v2c/v3 polling;
- GET/GETNEXT/GETBULK/WALK for routine monitoring; SET only through privileged NCM/remediation where explicitly authorized;
- tables/scalars/counters/gauges/enums;
- traps/informs;
- custom MIB upload and dependency resolution;
- customer-defined polling profiles;
- adaptive polling;
- missing-data detection;
- polling statistics and diagnostics.

#### 11.1.1 Enterprise SNMP correctness and usability requirements

| PRD ID | Requirement |
|---|---|
| `PRD-SNMP-001` | SNMP collection shall normalize into typed canonical evidence classes including numeric metrics, exact counters, state metrics, inventory facts, topology facts/events and collection-quality outcomes rather than forcing all varbinds into one numeric shape. |
| `PRD-SNMP-002` | Raw Counter32/Counter64 values shall preserve exact integer semantics. Counter64 shall not be rejected or rounded merely because it exceeds IEEE-754 exact-integer range; derived rates may use floating-point after exact delta calculation. **Implementation truth (current main after PR #84):** the exact raw Counter64 Sensor→Core→Kafka→ClickHouse path is IMPLEMENTED+TESTED using a dedicated exact canonical representation and ClickHouse `UInt64`, with coverage through `2^64-1`; it is not yet benchmarked/certified/deployed. Derived delta/rate/reset/wrap/discontinuity remains SPECIFIED but NOT IMPLEMENTED/CERTIFIED — remaining P0. |
| `PRD-SNMP-003` | Counter processing shall handle wrap, reboot/reset, discontinuity and actual elapsed sample time. `sysUpTime` and `ifCounterDiscontinuityTime` or equivalent evidence shall be used where available; missed polls shall not fabricate nominal-interval rates. |
| `PRD-SNMP-004` | High-capacity interface monitoring shall prefer applicable 64-bit interface counters and shall expose counter discontinuity/quality when rate derivation is invalid or uncertain. |
| `PRD-SNMP-005` | Canonical interface identity shall not depend on `ifIndex` alone. Platform shall reconcile interface identity from stable evidence such as ifName/ifDescr/ifAlias/MAC/physical slot-port/entity data and retain history when ifIndex changes. |
| `PRD-SNMP-006` | Production scheduled SNMP polling shall execute through the governed Sensor runtime/scheduler, assignment fencing, bounded worker pools and durable observation path; protocol-library unit success alone is not production readiness. |
| `PRD-SNMP-007` | Monitoring jobs shall reference governed Credential Profiles. Reusable communities/auth/privacy secrets shall not be exposed to UI, ordinary logs, telemetry, audit payloads or durable generic job history. |
| `PRD-SNMP-008` | SNMPv3 AuthPriv with modern SHA-2/AES combinations is the preferred secure posture where device compatibility permits. v1/v2c, MD5, SHA-1 and DES remain explicit legacy compatibility choices with visible warning/audit. |
| `PRD-SNMP-009` | A capability-aware Poll Planner shall choose GET/GETNEXT/GETBULK/WALK grouping, bulk repetitions, table/scalar strategy, timeout/retry and safe fallback according to Device Pack compatibility, response size, RTT/error evidence and Sensor budget. |
| `PRD-SNMP-010` | Scheduled polling shall use deterministic spreading across the configured interval and bounded per-device/per-worker concurrency so fleet polling, retries and adaptive collection do not create synchronized minute-boundary storms, uncontrolled self-overlap, or starvation of other certified workloads. Restart or Sensor reassignment must not intentionally reshape the entire estate into a burst. |
| `PRD-SNMP-011` | Adaptive SNMP polling shall have explicit minimum interval, duration, cooldown, target-count and Sensor-capacity limits; a broad incident shall not automatically accelerate an unbounded population. |
| `PRD-SNMP-012` | SNMP trap/inform ingestion shall preserve source/security/version/OID/time/provenance, decode through MIB/Capability Pack semantics, apply storm/dedup controls, and feed evidence qualification rather than automatically creating operational alerts. |
| `PRD-SNMP-013` | Trap delivery and Inform acknowledgement semantics shall remain explicit. SNMP Trap is unacknowledged. Successful Inform ACK requires durable local receiver acceptance/accounting after bounded protocol parse, configured security validation and sufficient source/tenant attribution; MIB/Pack enrichment, Core/Kafka/ClickHouse completion and alert/incident creation are not ACK prerequisites. v1/v2c trap source identity is weak evidence; SNMPv3 notification security shall be validated where configured. |
| `PRD-SNMP-014` | MIB Studio shall support upload, dependency/import resolution, validation/compile, OID tree, textual conventions, enums, tables/indexes, notifications, test/walk, provenance/licensing and generation/update of Capability/Device Pack mappings. Numeric OID monitoring shall remain possible without a MIB when an authorized semantic mapping is supplied. |
| `PRD-SNMP-015` | Capability/Device Packs shall declaratively define SNMP identity, poll definitions, table/index semantics, enum/bit mappings, counter/rate semantics, units, inventory/topology mappings, trap rules, diagnostics and compatibility hints. No pack may contain secrets. |
| `PRD-SNMP-016` | Discovery/preflight shall use the selected Sensor and governed Credential Profile to retrieve identity evidence such as sysDescr/sysObjectID/sysName and SNMPv3 engine identity where available; no default community fallback is permitted. |
| `PRD-SNMP-017` | The product shall provide an in-context SNMP Test workflow showing Sensor/vantage, target/port/version/profile identity, connectivity/authentication/authorization result, response latency/retries, identity OIDs, GET/Walk/Bulk-Walk results, raw OID, MIB-resolved name, decoded value, stable error code and remediation without revealing secrets. |
| `PRD-SNMP-018` | Per-device/profile SNMP data quality shall expose poll success, duration, attempted/successful OIDs, retries, timeout/auth/authz/tooBig/noSuch counts, freshness/staleness, expected-vs-received samples and last success. Collection failure shall map to monitoring degradation/unknown semantics rather than device-down by itself. |
| `PRD-SNMP-019` | Routine monitoring shall remain read-only. SNMP SET, if supported, shall execute only through privileged NCM/remediation controls with capability declaration, RBAC, reason/ticket/approval as applicable, pre-read, post-read verification, blast-radius limits and audit. |
| `PRD-SNMP-020` | SNMP storage/API/event contracts shall support exact counters plus typed state/inventory/topology/quality records using the existing PostgreSQL/ClickHouse/Kafka architecture; no new database is selected by this requirement. A logical exact-counter family is required; a dedicated Kafka topic is an implementation option when an equivalently typed contract is proven. |
| `PRD-SNMP-021` | Production certification shall include real or protocol-faithful devices/fixtures covering v1/v2c/v3 security modes, engineID/time-window recovery, malformed/partial responses, TooBig fallback, counter wrap/reset/discontinuity, interface identity changes, trap/inform storms, Inform durable-acceptance/retransmission/duplicate cases and Sensor/Core failure/replay. |
| `PRD-SNMP-022` | SNMP protocol, credential, parser and source failures shall remain distinguishable from target/service operational failure and shall participate in the Alert Evidence/telemetry-quality contracts of `Platform_22`. |
| `PRD-SNMP-023` | The normal scheduled SNMP operational-metric default is **5 minutes**. This default does not require polling every available OID every five minutes; Cap Pack/monitor definitions may schedule static inventory/topology/lifecycle metadata more slowly when freshness requirements remain satisfied. Availability/liveness remains a separate plane (commonly ~60 seconds by policy). Event/streaming telemetry remains event/stream driven. Policy inheritance follows the existing Platform Default → Tenant → Business Unit/Region → Site → Device Group → Device → Monitor/Metric hierarchy; intervals are stored as durations, not a rigid enum. |


### 11.2 Availability and path

- ICMP reachability/RTT/loss.
- Configurable device-down validation.
- Traceroute/path history and changes.
- Active path hop-by-hop latency/loss, path fingerprinting, and divergence detection (`REQ-NET-016`). Flow-stable active-path techniques (such as Paris-traceroute-style techniques evaluated by BUILD-040) may be used to preserve flow identifiers across intermediate hops and avoid ECMP route fluttering where supported. Intermediate unresponsive hops or hidden hops must not assert destination DOWN.
- TCP/DNS/TLS/HTTP validation where relevant.
- Multi-vantage validation from secondary Sensors where policy allows, preserving independent source results and contradictory evidence.

### 11.3 Layer and protocol depth

Device Packs/telemetry shall support deep monitoring for:

- interfaces, errors, discards, utilization and optical metrics;
- VLAN/STP/RSTP/MSTP;
- LACP;
- ARP/MAC/FDB;
- CDP/LLDP;
- BGP;
- OSPF;
- IS-IS;
- BFD;
- HSRP/VRRP;
- MPLS/LSP/VRF;
- VXLAN/EVPN/VNI/VTEP;
- QoS/CBQoS;
- multicast/PIM;
- IP SLA;
- VPN/IPsec;
- SD-WAN;
- WLAN AP/client/radio/controller metrics;
- CUCM/VoIP where Device Pack support exists.

### 11.4 Flow analytics

Platform shall ingest NetFlow v5/v9, IPFIX, sFlow and compatible vendor/J-Flow formats under configured exporter/source/tenant authority. Packet source IP alone is not tenant authority.

Required engineering contracts include: v9/IPFIX template lifecycle/cache/expiry/withdrawal; sampling factor/provenance; sequence/gap/reset evidence; UDP pre-acceptance loss visibility; observation point; ingress/egress/direction evidence; duplicate/exporter-HA semantics; raw evidence preserved before enrichment; temporal entity/IP mapping; DNS/DHCP/IPAM evidence; authenticated user/session evidence where available; ASN/Geo/cloud/SaaS enrichment with confidence/time/provenance.

Directory membership alone does not prove historical IP ownership. Domain/application identity does not equal full URL. Public-IP Geo is approximate. User-attributed Flow requires privacy/RBAC controls. Flow Coverage may compare compatible Flow vs interface evidence. **No Flow ≠ no traffic**. Flow anomaly ≠ device down/root cause.

Enrichment may include entity/device/interface, site/region, IPAM/subnet, VLAN/VRF, ASN/BGP, cloud, service/application metadata when exporter provides it, QoS and tenant. Views shall include top talkers, source/destination, conversations, protocols/ports, site-to-site, interfaces, ASN, matrices, Sankey, anomalies and capacity. Platform shall not claim deep L7 application visibility without exporter-provided metadata or future DPI capability. ClickHouse remains high-volume Flow analytics authority; no new datastore. Production high-volume Flow uses the `FLOW` deployment role.

---


## 12. Infrastructure, hardware, virtualization and storage
### 12.1 Server/OS

Support agentless and optional Agent-based monitoring for:

- CPU/load;
- memory/swap;
- filesystem/disk capacity;
- disk I/O/latency/IOPS;
- network interfaces;
- processes including command-line arguments where permitted;
- services/daemons;
- uptime/reboots;
- selected logs/events.

### 12.2 Hardware/OOB

Redfish/vendor pack support shall cover:

- chassis;
- CPU/DIMM;
- disks/RAID/controllers;
- PSU;
- fans;
- temperature;
- batteries;
- firmware/BIOS;
- NICs;
- lifecycle events.

### 12.3 Virtualization

Support VMware/vCenter/ESXi, Hyper-V, Proxmox and Nutanix with normalized entity relationships such as cluster → host → VM → datastore/network.

### 12.4 Storage/SAN

Support storage systems and SAN fabric monitoring for capacity, IOPS, throughput, latency, pools/volumes/controllers/disks, replication, FC switches/ports/fabrics/zones/WWNs, errors and congestion where vendor interfaces permit.

---


### Windows/Linux infrastructure monitoring modes

Platform shall support both **Agent-based** and **agentless/remote** Windows/Linux infrastructure monitoring as separately certifiable modes.

Customer-visible server monitoring shall include, where the OS/version/privilege supports it:

- CPU utilization and useful scheduler/resource-pressure evidence;
- memory, swap/page/commit and resource-exhaustion evidence;
- filesystem capacity/inodes and block-I/O throughput/IOPS/queue/latency/error evidence;
- network-interface state/throughput/errors/drops and bounded transport-quality counters;
- bounded process resource/state/restart information;
- Linux systemd and Windows service state/restart/failure information;
- OS/boot/uptime/time-quality facts and selected bounded operational/kernel/Event Log evidence;
- applicable hardware/thermal/power evidence through approved native or out-of-band sources.

Linux Agent and Windows Agent are the richer local collection modes. Linux agentless monitoring uses governed SSH query packs; Windows agentless monitoring uses governed WinRM/WMI/CIM query packs. Product/UI/API shall disclose capability differences rather than imply identical raw coverage.

Collection, credential, privilege or path failure is **monitoring-health evidence**. It shall not automatically become host/service DOWN when the monitored state cannot be established.

### 12.5 Linux Agent eBPF process-to-network telemetry (REQ-INF-013)

On supported Linux distributions and kernels under the certified Ubuntu 24.x / RHEL 9.x compatibility matrix (using BTF/CO-RE where the selected implementation requires/supports it), the Linux Agent shall collect bounded process-to-network evidence via eBPF probes:
- Correlate active host processes, systemd services, and container workloads with local/remote socket communication endpoints (local IP/port, remote IP/port, protocol).
- Measure connection performance: active/new/closed sockets, bytes TX/RX, mean TCP RTT, retransmissions, and connection failures.
- Aggregate locally into bounded configurable aggregation intervals with bounded active-key cardinality ceilings and heavy-hitter overflow shedding; do not stream individual socket events over the network.
- Enforce strict data minimization: zero payload capture, zero TLS decryption, zero HTTP/SQL inspection, zero continuous profiling/stack traces, and no environment variable reading.
- Implement resource governance: bounded eBPF memory and resource budgets derived from BUILD-019 / BUILD-130 benchmark certification; transitions to `RESOURCE_GUARDED` capability state with explicit drop/shed/resource-guard counters (`dropped_records_count`, `shed_records_count`) when thresholds are approached.
- Base Agent host metrics collection shall continue unaffected if eBPF is `UNSUPPORTED`, `UNAVAILABLE`, `PRIVILEGE_DENIED`, `KERNEL_INCOMPATIBLE`, or `RESOURCE_GUARDED`.



## 13. Database monitoring requirements
Platform shall provide database packs/connectors for major databases including PostgreSQL, MySQL/MariaDB, Microsoft SQL Server, Oracle, SAP HANA, MongoDB, ClickHouse and Elasticsearch/OpenSearch where interfaces/licensing permit.

Normalized capabilities include:

- availability/connectivity;
- connections/sessions;
- storage;
- locks/waits;
- replication;
- performance/latency;
- query-health indicators where safely obtainable;
- errors;
- host dependency;
- custom SQL query monitoring through a controlled Query Monitor Studio/pack model.

Database credentials shall be scoped and protected. Arbitrary dangerous SQL shall not be enabled by default.

---


## 14. Cloud, Kubernetes and OpenTelemetry
### 14.1 Cloud

Cloud provider roots include AWS, Azure, GCP and OCI. Vendor identifiers never override Platform tenant/entity authority. Required concepts: cloud hierarchy discovery; ephemeral-child lifecycle/reconciliation; provider API quota/rate-budget governance; throttling; freshness/staleness; provider service-health; audit/change evidence; cloud-network evidence; quota/autoscaling evidence; canonical normalized entity/state; connector self-health. New cloud SDK dependencies remain Platform_07 governed and are not auto-approved.

AWS, Azure and GCP integrations shall use supported provider APIs/SDKs and normalize accounts/subscriptions/projects, regions, networks (VPCs/VNets, subnets, route tables, routes, NICs, security policies, NAT gateways, transit/interconnect/private endpoints where applicable, and cloud load balancers), compute workloads, storage, database entities, and Kubernetes relations (`REQ-INF-008`), along with material control-plane change evidence. Cloud/provider flow logs are SUPPORTED WHEN ENABLED/CERTIFIED (subject to provider capability, customer policy, privacy, cost, API/rate limits, cardinality, storage, and certification) and are not a mandatory universal Phase-1 dependency.

### 14.2 Kubernetes

Hierarchy shall include:

Cluster → Node → Namespace → Workload → Pod → Container → Service → Ingress.

Monitor node/pod/container health, CPU/memory, restarts, pending/failed state, events and capacity.

### 14.3 OpenTelemetry

OpenTelemetry integration adheres to a two-phase contract boundary (`REQ-INF-010`):
- **Phase 1:** Ingestion and storage of OpenTelemetry metrics and structured logs into ClickHouse. W3C `trace_id` and `span_id` are extracted as correlation reference fields on logs, events, alerts, and investigations without requiring distributed trace storage.
- **Phase 1.5:** May add bounded customer-provided trace and dependency evidence (such as trace_id, span_id, service identity, dependency edges, latency/error summaries, or selected exemplars) subject to separate implementation and certification; it does not automatically imply a full distributed trace backend, storage of all spans, unlimited trace retention, APM, bytecode agents, or a trace database. Full distributed trace storage/analysis requires future explicit architecture change control.

---


## 15. Synthetics and service assurance
Synthetic Monitor Studio shall support structured, safe tests for:

- ICMP;
- TCP;
- DNS;
- HTTP/HTTPS;
- REST API;
- TLS/certificate checks;
- optional UDP where test semantics are well defined.

HTTP/API tests shall support method, headers, authentication, body, expected status, body/header validation and latency breakdown. Importing a cURL definition may be supported, but Platform shall parse it into a safe structured monitor rather than executing arbitrary shell commands.

---


## 16. Extensibility studios (MIB, API, Device Pack)
MIB Studio remains part of the single Capability Pack extension architecture (not a second pack registry/SDK). Required contracts include import, revision/version, provenance/licensing, dependencies, compile/test, OID/table/notification mapping and firmware/model compatibility.


Required capabilities:

- MIB library;
- upload one/multiple MIBs;
- dependency/import resolution;
- compile/validation;
- OID tree/explorer;
- scalar/table/counter/gauge/enum/trap identification;
- connect/test against sample device;
- walk/test;
- AI-assisted semantic suggestions clearly labeled `AI Suggested`;
- user confirmation;
- normalized metric mapping;
- alert/visualization suggestions;
- trap registration/decoding;
- creation/update of Device Pack;
- versioning/export/import/signing/provenance.

MIB provenance shall classify Platform Supplied, Customer Supplied, Partner Supplied and preserve source/licence/redistribution metadata. Customer-uploaded vendor MIBs shall not automatically be redistributed to other customers.

---


API Studio collection runtime is **monitoring source collection**, distinct from external integration delivery queues. Required contracts include REST/API schema, authentication reference, pagination, extraction/mapping, rate-limit behavior, collection cadence/freshness, schema-drift handling and source provenance. Do not treat an outbound ITSM/webhook connector queue as the API monitoring scheduler.

Webhook contracts distinguish inbound webhook, outbound webhook and callback, and require as applicable: authentication/signature verification, replay protection, tenant/source binding, SSRF/egress restrictions, allowlists, secret handling, schema validation, rate limits, idempotency and auditing. No arbitrary scripts. No second Pack registry/SDK.


Wizard stages:

1. Connection.
2. Authentication.
3. Endpoint/OpenAPI import.
4. Test request.
5. Schema inspection.
6. Entity mapping.
7. Metric mapping.
8. Collections/pagination/chained requests.
9. Alerts/monitor definitions.
10. Dashboard suggestions.
11. Publish/version.

Support:

- OpenAPI/Swagger import as highest automation path;
- GET/POST/PUT/PATCH where safe/required;
- Basic/Bearer/API key/OAuth2/custom header/mTLS;
- JSON and XML;
- JSONPath/JMESPath-like mappings through approved libraries;
- nested arrays/objects;
- pagination/cursors;
- retries/rate limits/timeouts;
- token refresh;
- timestamp/unit/null transformations;
- derived metrics;
- chained endpoints.

AI may suggest schema interpretation but an administrator confirms mappings before production publication.

---


A Device Pack may contain:

- identification rules;
- discovery rules;
- SNMP/MIB/OID/trap mappings;
- REST/API mappings;
- gNMI/YANG paths;
- CLI/parsers;
- NCM commands and modes;
- normalized metrics;
- inventory/entity mappings;
- topology relationships;
- dashboards;
- alert semantics;
- RCA semantics;
- remediation/runbook metadata;
- vendor/model/firmware/API/MIB compatibility;
- provenance/licensing metadata.

Certification levels:

- Official Certified;
- Partner Verified;
- Customer Custom;
- Experimental.

A Device Pack Test Lab shall accept sample SNMP walks, REST responses, CLI output, traps/syslogs and validate extraction, enums, topology, alerts, NCM parsing and dashboards without repeatedly contacting a production device.

---


### OSS implementation basis (product constraint)
Module SHALL use catalogue-approved OSS for commodity primitives and retain Platform-owned product layers (identity, policy, placement, qualification, RCA guardrails, UX). Exact component versions and licence class live in `Platform_07_OSS_Catalog.md`; runtime authority in `Platform_03_Architecture.md`. The historical PRD-OSS tables are absorbed as this constraint — not as a competing catalogue.

## 17. Canonical entity and operational graph
The same physical/logical resource shall not become duplicate entities merely because it is observed through SNMP, flow, syslog, NCM, API or Agent.

Entity families include tenant, region, site, network device, chassis/module/interface, VRF/VLAN, route/peer, server/OS/NIC/disk/process/service, hypervisor/VM, storage, DB, Kubernetes, cloud and business service entities.

Relationship examples:

`CONNECTED_TO`, `DEPENDS_ON`, `ROUTES_VIA`, `MEMBER_OF`, `CONTAINS`, `RUNS_ON`, `HOSTED_ON`, `MANAGED_BY`, `ADVERTISES`, `PEERS_WITH`, `USES`, `SERVES`, `BACKED_BY`, `ASSIGNED_TO`, `IMPACTS`, `CAUSED_BY`.

Every relationship shall record source/provenance, last verified time and confidence/quality where applicable. User-defined logical relationships must be distinguishable from automatically discovered physical/network relationships.

Historical topology shall support viewing the network/service graph as it existed during an incident time window.

---


## 18. Grouping, hierarchy, ownership and circuits
Platform shall support concurrently:

- tags/attributes;
- hierarchical groups;
- dynamic predicate groups;
- business-service groupings.

An entity may belong to multiple dimensions simultaneously.

Groups shall drive dashboards, policies, polling, alerts, reports, RBAC, Sensor assignment, maintenance, SLA, maps, escalation and RCA scope.

Entities may store business owner, technical owner, support team, escalation group, provider/vendor, cost center, criticality and service ownership.

---


Circuit records shall support:

- circuit ID;
- provider;
- bandwidth;
- site;
- primary/backup designation;
- WAN IP/demarcation;
- service type;
- SLA;
- contract/support references;
- mapped monitored interfaces and paths.

This context shall enrich Link, SLA, path and incident workflows.

---


## 19. Monitoring policy, thresholds and adaptive polling
No customer-specific operational behavior should require source-code modification.

Policy inheritance shall support:

Platform Default → Tenant → Business Unit/Region → Site → Device Group → Device → Metric/Data Type.

The UI shall show effective values and the source of inheritance/override.

Policy domains include:

- polling;
- adaptive polling;
- thresholds;
- anomaly sensitivity;
- validation/device-down qualification;
- hysteresis/flapping;
- suppression/maintenance;
- RCA confidence;
- notification/escalation;
- retention/rollup;
- NCM backup/compliance;
- synthetics;
- credentials;
- discovery/topology;
- Sensor placement/HA;
- log/API/MIB mapping;
- dashboards;
- SLA/reporting.

Changes shall be versioned, auditable and rollback-capable at configuration level.

---


Supported conditions shall include:

- static comparison;
- duration;
- consecutive occurrences;
- rate-of-change;
- difference from baseline;
- learned/dynamic baseline;
- missing/no-data;
- composite Boolean expression;
- cross-device/group condition;
- dependency condition;
- business-time/calendar awareness.

Threshold examples in templates are not hard-coded customer rules. Templates are editable starting points.

Learned baselines shall support configurable training window, sensitivity, seasonality and confidence. A learned anomaly does not automatically imply root cause.

---


Adaptive polling may be triggered by:

- metric/threshold;
- trap;
- syslog/event;
- topology change;
- configuration change;
- anomaly;
- incident/RCA;
- manual investigation;
- ITSM/change event.

Policies specify normal frequency, adaptive frequency, duration, cooldown, max duration, priority and return conditions.

Safety limits shall protect Sensor CPU, polling operations/sec, maximum adaptive targets and minimum allowed intervals. Configuration impact preview shall warn/block unsafe policies according to authorization.

---


## 20. Events, alerts, incidents, Alert Qualification Engine, storm and maintenance

**Feature contract (summary):**
- **Purpose/outcome:** Low-noise, evidence-qualified operational alerting
- **Subfeatures:** event/alert/incident model; qualification; storm/dedup/flapping; maintenance/suppression
- **Primary persona:** NOC Operator / Manager
- **Dependencies:** freshness, identity, topology, maintenance calendars, multi-evidence
- **Preconditions:** monitors assigned and telemetry flowing or path-health known
- **Primary workflow:** observe → qualify → correlate → incident → recover
- **Product behavior:** SNMP/path failure alone must not auto-assert DEVICE DOWN when contradictory evidence remains credible
- **Failure/degraded behavior:** UNKNOWN / MONITORING_DEGRADED / path-unavailable semantics preserved
- **Acceptance outcome:** false-alert reduction measurable; severity ≠ confidence
- **Maturity boundary:** SPECIFIED

Platform shall distinguish:

- **Event** — raw observation.
- **Alert** — qualified actionable condition.
- **Incident** — correlated operational problem grouping one or more alerts/events.

Underlying events/child alerts shall remain inspectable even when incident correlation suppresses repetitive notifications.

Alert/incident lifecycle shall support New, Acknowledged, Assigned, Investigating, Monitoring/Recovery, Resolved and Closed states as appropriate.

Ownership, team assignment, notes, escalation, SLA timer and handoff shall be available.

---


A failed SNMP poll shall **not** automatically mean `DEVICE DOWN`.

The state model shall support:

`HEALTHY → PRIMARY_MONITOR_FAILURE → SUSPECT → VALIDATION → QUALIFIED_STATE`.

Possible outcomes include:

- UP;
- DEGRADED;
- UNREACHABLE;
- DEVICE_DOWN;
- PATH_FAILURE;
- CREDENTIAL_FAILURE;
- MONITORING_FAILURE;
- SENSOR_FAILURE;
- UNKNOWN/INCONCLUSIVE.

Validation policies may use:

- SNMP retry;
- ICMP N-of-M attempts;
- TCP management port;
- REST/API secondary check;
- topology/neighbor evidence;
- secondary Sensor;
- traceroute;
- Boolean/consensus rule.

Example: SNMP fails while ICMP/API/traceroute and neighbor evidence remain healthy → `Monitoring Degraded — SNMP unavailable`, not `Device Down`.

Recovery shall also be qualified using configurable stability windows, not one successful poll.

---


Platform shall support:

- event deduplication;
- correlation;
- dependency suppression;
- storm detection;
- notification throttling;
- ITSM ticket throttling;
- flapping detection using customer-defined transition/time rules;
- root-incident grouping while retaining child evidence;
- reopen windows to avoid new-ticket storms.

Product shall expose metrics for raw events vs qualified alerts vs correlated incidents and false/noise reduction.

---


Platform shall distinguish:

- notification mute;
- maintenance suppression;
- dependency suppression.

Maintenance can be manual, scheduled or recurring and scoped to tenant/site/group/device/interface/service/alert/metric as appropriate, with reason, ticket/change reference, approver, auto-expiry and audit.

ITSM change/maintenance windows may synchronize into Platform when configured.

---


## 21. Graph intelligence, GNN and RCA
### 21.1 Architecture principle

Platform RCA is a **hybrid evidence-based graph reasoning system**. It shall not be an LLM-only system and shall not treat GNN output as proof of causality.

### 21.2 Evidence inputs

RCA shall combine, when available:

- temporal event order;
- topology graph;
- protocol relationships;
- telemetry trends/anomalies;
- configuration/NCM changes;
- audit/user changes;
- Sensor/telemetry quality;
- Flow/traffic context;
- path tests;
- service dependencies;
- maintenance/change calendar;
- prior incident patterns;
- learned graph/GNN scores.

### 21.3 GNN learning incorporated from graph-first products

Platform shall maintain a temporal multi-layer operational graph suitable for learned graph features. A GNN/graph-learning component may:

- learn propagation signatures;
- rank likely root entities;
- score correlated edge relevance;
- assist blast-radius prediction;
- identify graph-structural anomalies;
- compare current graph state to learned normal relationships;
- prioritize additional evidence collection.

GNN results must remain subordinate to evidence/protocol constraints. Model output must include version, score/confidence and traceability to the graph snapshot/evidence window used.

### 21.4 RCA output

Every RCA shall expose:

- probable root cause or `INCONCLUSIVE`;
- confidence;
- root entity;
- incident start/evidence time;
- causal chain;
- causal graph;
- affected/blast-radius entities/services;
- supporting evidence;
- contradictory evidence;
- relevant changes;
- missing evidence/telemetry-quality limitations;
- recommended next action/runbook;
- correlated child alerts in chronological order.

### 21.5 Deterministic/learned separation

Protocol-aware rules and canonical relationships provide deterministic constraints. Statistical/ML/GNN components provide learned scoring or anomaly evidence. The system shall never market “GNN = deterministic truth.”

### 21.6 RCA quality benchmark

Engineering/QA shall maintain a labeled incident benchmark including at minimum representative cases such as physical link failure, interface shutdown, BGP/OSPF cascades, MPLS path failure, configuration blackhole, congestion, hardware/power fault, maintenance storm and multi-site dependency failure.

Measure:

- root-cause precision;
- false-root-cause rate;
- no-answer/inconclusive rate;
- time-to-root-cause;
- alert compression;
- explanation completeness;
- blast-radius accuracy;
- remediation recommendation correctness.

No unsupported answer is preferable to a confident wrong answer.

---


### Graph/GNN/AI product constraints (absorbed from PRD-RCA-GNN)
Platform shall incorporate graph-first learning without copying NetAI proprietary implementation:

- topology is a reasoning substrate, not merely a visual map;
- graph is temporal and multi-layer: physical/L2/L3/routing/MPLS/VXLAN/SD-WAN/traffic/infra/service/geo/causal;
- deterministic/protocol evidence and observed configuration change retain explicit provenance;
- ML/GNN can score anomaly, propagation likelihood, likely root candidate and blast radius;
- GNN output is **supporting evidence**, never sufficient by itself to declare causality;
- the RCA result exposes root candidate, confidence, causal chain, supporting evidence, contradictory evidence, affected scope, change context, recommendation and missing evidence;
- `INCONCLUSIVE` is a first-class result;
- raw/child alarms are not destroyed when incident compression occurs;
- training/validation uses labeled incidents and measures root-cause precision, false-root-cause rate, no-answer rate, time-to-root-cause, blast-radius accuracy, alarm-compression ratio and explanation completeness;
- Gemini/LLM operates through AI Gateway for natural-language search, summarization, explanation and recommendation. It does not replace the causal engine and never receives secrets by default.


## 22. AI Gateway and local intelligence placement
Gemini shall be integrated through a provider-independent AI Gateway.

Permitted uses include:

- natural-language operational search;
- incident summary;
- RCA evidence explanation;
- runbook recommendation/summarization;
- provisioning failure explanation;
- MIB/API mapping suggestions;
- investigation assistance;
- report narrative generation where allowed.

AI shall not receive credentials/secrets. Customers configure data boundaries for logs, topology, inventory, configuration snippets and masking.

Every consequential AI interaction shall be auditable with provider/model/version/prompt-tool metadata subject to security policy.

Future local/private/offline models shall plug into the same Gateway contract.

AI feedback shall support Correct / Partially Correct / Incorrect plus optional actual root cause; feedback is retained for evaluation and does not automatically retrain production models without controlled ML governance.

---


### Lightweight/local AI placement

Local intelligence uses three deployment tiers:

1. **Tier 0 — endpoint deterministic intelligence:** capture-eligibility evaluation, hysteresis, bounded rolling/statistical checks and EDS evidence logic. No LLM/GNN/foundation-model runtime is required or permitted on normal monitored Agents/Sensors for EDS.
2. **Tier 1 — Core bounded analytics:** statistical/ML, anomaly, forecast, similarity and optional GNN work executes only within certified cold/bounded resource controls and yields to protected monitoring.
3. **Tier 2 — language/embedding assistance:** LLM/embedding functions run behind AI Gateway either in a certified on-Core profile or through an optional dedicated local/private inference provider.

**If** a dedicated local/private inference provider is supported/enabled, it is optional and must remain stateless with respect to authoritative Platform state. It is not required for Linux/Windows monitoring, EDS, deterministic RCA or base Platform operation; it is not a third Core/HA node; and it may be unavailable without impairing deterministic collection, alerting, incident handling or RCA. Core-resident LLM remains optional. All-AI-disabled remains supported.

Detailed evidence, security, resource and release contracts are authoritative in `Platform_22`, `Platform_23`, `Platform_10`, `Platform_11`, `Platform_14`, `Platform_17` and `Platform_18`.


## 23. Operations timeline, troubleshooting, path analysis and EDS
For device, site, service and incident contexts, Platform shall provide one time-ordered view combining, as available:

- telemetry state changes;
- events/traps/syslog;
- alerts/incidents;
- topology changes;
- routing changes;
- NCM/configuration changes;
- user/audit actions;
- Sensor reassignment/health;
- maintenance;
- ITSM changes;
- RCA conclusions;
- recovery qualification.

Timeline data shall be queryable around incident windows and support historical topology/configuration drill-down.

---


Platform shall implement a **no-context-switch operational troubleshooting experience**. An authorized NOC/operator shall be able to move from observation to verification, diagnosis and evidence capture without leaving the current device/interface/alert/incident context for routine troubleshooting. Flexible composable dashboard interaction patterns may take inspiration from mature dashboard products such as Grafana, but Platform shall not copy Grafana literally; Platform troubleshooting remains domain-aware and action-oriented rather than a generic visualization-only experience.

| ID | Requirement |
|---|---|
| PRD-TRB-001 | Entity/detail, alert and incident workspaces shall expose a persistent permission-aware contextual action bar for applicable operations such as CLI, Ping, Traceroute/Path, SNMP test, Flow, Configuration, Compare, Investigate and Maintenance. |
| PRD-TRB-002 | `Troubleshoot` shall open a persistent drawer/workspace that preserves the originating entity, time range and filters while the operator performs diagnostics; routine tools shall not force navigation to unrelated pages. |
| PRD-TRB-003 | On-demand ICMP shall support bounded packet count/size/timeout, assigned-Sensor execution by default, authorized alternate Sensor/Agent source selection, live loss/RTT/jitter result and evidence attachment. Continuous tests, where allowed, shall be explicitly bounded and cancellable. |
| PRD-TRB-004 | Multi-vantage verification shall compare evidence from eligible Sensors/Agents so Platform can distinguish target failure from path/locality/Sensor failure. Vantage eligibility remains tenant/resource/capacity/policy controlled. |
| PRD-TRB-005 | Alerts/incidents shall support `Verify Now` where applicable. Verification executes a deterministic bounded diagnostic bundle based on entity/technology/alert context and returns individual evidence results; it shall not hide contradictory or failed checks. |
| PRD-TRB-006 | Entity health shall support `Why?` evidence explaining the observations/policies that produce the displayed state, including freshness/quality and contradictory evidence. Missing telemetry shall never be silently translated to device-down. |
| PRD-TRB-007 | `What Changed?` shall correlate relevant Platform/NCM/configuration/topology/routing/maintenance/external change evidence around the selected time/incident where available. Temporal proximity is supporting evidence, not causal proof. |
| PRD-TRB-008 | `Compare` shall support appropriate current-vs-prior-period, baseline, peer entity/interface/site, pre-incident and pre-change comparisons without losing the current context. |
| PRD-TRB-009 | Platform shall distinguish continuously monitored state from fresh operator-triggered verification through a **Live Evidence** presentation containing result time, source/vantage and quality. |
| PRD-TRB-010 | An operator may start an **Investigation Session**. Authorized diagnostic results, CLI sessions/commands, graph/config/flow/log snapshots, notes and related evidence performed in that context are attachable automatically to the investigation and may be linked to an incident. |
| PRD-TRB-011 | Platform shall provide deterministic diagnostic packs for common failure classes (for example device unreachable, WAN degradation, BGP/routing, wireless/client and service reachability) with explicit checks, bounded execution and per-check results. AI may explain/suggest the next permitted test but may not invent results or bypass action authorization. |
| PRD-TRB-012 | Operational friction shall be proportional to risk: safe read-only diagnostics are fast; interactive/privileged CLI and configuration-affecting operations require applicable RBAC/resource scope, JIT/MFA/reason/ticket/approval/command policy and audit. |

For network devices the Troubleshoot surface shall expose, when applicable: Ping; traceroute/path analysis; SNMP test; TCP test; DNS; SSH test; API test; topology; Flow; recent changes; NCM configuration; CLI; RCA; Compare; Verify Now; Live Evidence and Investigation Session.

For servers/infrastructure, equivalent Agent/SSH/WinRM/process/service/log/network tests shall be shown. Databases, APIs, cloud, virtualization and other entity classes shall expose equivalent domain-relevant actions rather than a network-only toolbar.

Temporary **Investigation Mode** may increase polling, run deeper checks and gather additional evidence, then automatically revert according to policy/capacity limits. This is separate from an Investigation Session: Investigation Mode changes bounded collection behavior; an Investigation Session is the operator evidence/context container.

---


User selects a source and destination/vantage. Platform correlates forward/reverse evidence where available, traceroute, latency/loss, interfaces, routing, VRF, SD-WAN/MPLS, Flow, topology and recent changes.

Path view shall identify uncertain/unobserved hops rather than inventing links.

---


### Endpoint Diagnostic Snapshot

Platform shall provide a bounded **Endpoint Diagnostic Snapshot (EDS)** for Windows/Linux infrastructure as a specialized profile of the existing Diagnostic Job / Live Evidence / Investigation architecture (`ENDPOINT_DIAGNOSTIC_SNAPSHOT`). EDS may start automatically after a centrally governed threshold/state condition becomes **capture-eligible**, or from an authorized investigation request. Capture-eligibility is not authoritative host-health state.

EDS shall:

- reuse existing normal observations for capture-eligibility evaluation wherever possible;
- use policy-defined duration, hysteresis, recovery and cooldown rather than one-sample triggering;
- execute only under currently valid Core-issued assignment/policy/control generation (policy ID/version alone is insufficient);
- collect a short-lived typed diagnostic evidence bundle appropriate to the capture-eligible resource class;
- report Diagnostic Job state plus capture completeness `COMPLETE`, `PARTIAL`, `FAILED` or `SUPPRESSED` rather than inventing missing evidence;
- preserve policy/version, authority generation, trigger, entity, source/vantage, time and evidence provenance;
- feed the authoritative Core incident/RCA workflow through the existing Agent/Sensor durability paths (no second EDS transport);
- stop, suppress or degrade with explicit observable reason before creating material additional pressure on an already stressed endpoint or silently destroying routine telemetry durability.

EDS is evidence acquisition, **not endpoint-authoritative RCA** and not a second diagnostics framework. Core remains responsible for cross-domain causal analysis because endpoint-local evidence cannot by itself observe all network, hypervisor, storage, database, cloud, shared-service or change dependencies.

No universal customer CPU/memory/disk threshold is fixed here. Effective values remain governed policy. This section is target architecture/requirements; specified ≠ implemented ≠ tested ≠ certified ≠ deployed.


## 24. NCM and Privileged CLI Session Broker
NCM is Day-1 important. Capabilities below are architecture/product scope; certification follows the Phase 1 / Phase 1.5 split (see also §58.4).

**Phase 1 independently certifiable subset:**
- credentialed SSH/NETCONF/API connections for authorized NCM/CLI paths;
- configuration backup;
- version history;
- diff;
- change detection;
- compliance rules/golden configuration;
- templates (non-executing authoring/storage where certified);
- RBAC;
- reports;
- API;
- browser CLI integration only when credential brokering/RBAC/audit are certified;
- change audit;
- RCA correlation as supporting evidence.

**Phase 1.5 default (unless Founder change control promotes earlier):**
- bulk push;
- scheduling/windows for privileged mass execution;
- maker-checker approval driving automated privileged execution;
- canary/staged automated privileged rollout;
- rollback where adapter/device supports it as part of automated remediation paths.

Semantic diff shall only be claimed where a Device Pack/parser supports it.

NCM worker concurrency/rate limits are independent from routine monitoring workers so NCM jobs cannot starve polling.

Bulk operations shall support dry-run/impact preview and canary/staged execution when the privileged-automation envelope is certified (Phase 1.5 by default).

---


The browser shall never directly receive reusable device credentials.

CLI launch may use an authorized stored Credential Profile, a user-scoped/personal credential where policy permits, or explicitly entered one-time session credentials. One-time credentials must be ephemeral, must not be persisted as reusable secrets unless the user explicitly invokes an authorized credential-management workflow, and must never be logged or exposed to unrelated browser/API clients.

Flow: Browser HTTPS/WebSocket → Core Session Broker → assigned Network Sensor → SSH device.

Controls:

- CLI privilege tiers: no access / read-only / operational / configuration / full privileged, as applicable;
- tenant/resource scope;
- JIT/temporary access;
- reason/ticket requirement;
- MFA/re-auth where policy requires;
- command allow/deny patterns;
- high-risk approval;
- session timeout/idle/concurrency/hours;
- full command audit;
- optional terminal/session playback where configured;
- break-glass.

Telnet is legacy, disabled by default and never automatic fallback.

---


## 25. IPAM requirements
IPAM shall support:

- IPv4 and IPv6 as first-class, including dual-stack estates;
- subnet/supernet hierarchy;
- VLAN/VRF association;
- address state/reservation;
- DNS/DHCP integration;
- leases;
- conflicts/duplicates;
- history;
- discovery/reconciliation;
- utilization;
- exhaustion forecasting;
- APIs;
- audit;
- Microsoft DHCP/DNS and Infoblox integration where APIs are available.

IPAM is integrated into the canonical graph and shall not be a separate competing inventory authority.

---


## 26. Display timezone, scheduling and freshness
| ID | Requirement |
|---|---|
| `PRD-TIME-001` | All authoritative system/event/telemetry timestamps shall remain absolute UTC instants in storage and machine-to-machine contracts (RFC3339/ISO-8601 capable). Rendered wall-clock strings are never source truth. |
| `PRD-TIME-002` | Interactive display shall use named IANA TZDB identifiers. Where no saved preference exists, first use defaults to the browser/device local IANA timezone. Users may select zones such as Asia/Kolkata, Europe/London, America/New_York or UTC; fixed `UTC+X` offsets must not model DST-sensitive regions. |
| `PRD-TIME-003` | Effective display timezone precedence is: explicit dashboard/view/session override → saved user `display_timezone` preference → browser/device local IANA zone → `tenant.timezone` → UTC. Explicit overrides may be preserved in URL/saved views; absence resolves per viewer preference. |
| `PRD-TIME-004` | Changing display timezone shall update labels/calendar grouping only. It must not change telemetry timestamps, alert evaluation, thresholds, incident sequencing, RCA/correlation, polling/retry/freshness, SLA semantics, maintenance-window execution, business calendars, scheduled jobs, report execution schedules or automation schedules. Operational policies requiring wall-clock interpretation carry their own explicit IANA timezone. Host OS timezone is non-authoritative. |
| `PRD-TIME-005` | Reports/exports that render local timestamps must disclose the timezone used. Scheduled reports separate execution/schedule timezone from render/display timezone. Calendar-bucket queries may accept an explicit IANA zone and must return unambiguous absolute bucket boundaries plus timezone metadata. DST and half/quarter-hour correctness are mandatory certification subjects. |


| ID | Requirement |
|---|---|
| `PRD-SCHED-001` | Core owns desired monitoring schedule/policy, placement, recurring assignment and generation/fencing authority. Sensor owns execution of currently authorized work and shall retain durable recurring desired-state sufficient to continue safely during temporary Core outage/failover. No second independent schedule authority and no scheduler microservice. |
| `PRD-SCHED-002` | Monitor Group (semantic/product grouping), Scheduling Class (priority/freshness behavior) and Execution Class (runtime worker/resource pool) shall remain distinct concepts. |
| `PRD-SCHED-003` | Recurring monitoring definitions shall specify both nominal cadence and freshness/maximum staleness. The normal SNMP operational-metric default remains 5 minutes and does not mean all OIDs every five minutes. Availability/liveness remains a separate plane. |
| `PRD-SCHED-004` | Eligible recurring work shall use a versionable deterministic 64-bit phase calculation so fleets do not stampede at clock boundaries. Phase inputs should remain stable across normal restart and preferably across Sensor reassignment; Sensor identity must not dominate where reassignment would reshuffle the estate. |
| `PRD-SCHED-005` | Admission order shall be priority/scheduling class → freshness urgency/deadline → fairness → execution cost/capacity. Opaque ML shall not drive routine collection scheduling. Cost inputs remain bounded/explainable (protocol, operation, cardinality, duration, RTT, timeout/retry, payload size, API quota). |
| `PRD-SCHED-006` | Concurrency shall be bounded by Sensor, execution class, protocol, target, tenant, site, subnet and controller/API as applicable. Identical recurring collections must not stack without bound. Missed cycles coalesce or skip per policy with quality/overrun evidence; never create infinite catch-up queues or fabricate historical samples. |
| `PRD-SCHED-007` | Controller/site/API failure domains shall use bounded retry, dependency-aware backoff, circuit-breaker-like qualification, cheap recovery probes and explicit monitoring-path degradation. `No Data != Device Down` remains invariant. |
| `PRD-SCHED-008` | Scheduler admission shall observe CPU/RAM/spool/queue/latency/error/upload-backlog/freshness-debt pressure. Recovery ramp order is liveness → recovering critical → normal operational → detail → topology/state → inventory/background. Adaptive and event-triggered refresh are capacity/authorization/cooldown/dedup bounded with automatic reversion. |
| `PRD-SCHED-009` | Capability Pack contracts carry applicable cadence/freshness/cost semantics. Where valuable, Agents may sample gauges approximately every 30–60 seconds locally and forward governed lower-rate min/max/average/last/count summaries; raw monotonic counters must not be averaged; tiny-sample P95/P99 must not be represented as statistically meaningful. |


## 27. Reporting, SLA/OLA and Service Assurance

Detailed formal ownership is `Platform_21` `PRD-EGC-003` / `REQ-EGC-003`. The following product behavior is mandatory.

Report Studio shall support:
- governed data-source/metric/entity/incident/ticket/topology selections through authorized catalogues and connectors;
- filters, grouping, calculations/formulas, tables/charts/text/images and branding/layout;
- reusable report templates and industry-oriented out-of-box packs;
- on-demand and scheduled execution, subscriptions/recipients, business-day-aware due rules and execution history/failures;
- report-obligation lifecycle, review/attestation and delivery status;
- **PDF and CSV report artifacts only**; XLSX, JSON and PPTX are not report artifact formats; API responses remain separate integration/query interfaces; generated PDF reports prominently embed the effective commercial product logo (defaulting to the governed AmrudAi logo: matching light/dark background, preserving aspect ratio, or licensed tenant/MSP logo where a governed white-label configuration is active) and effective customer-facing commercial branding; CSV reports contain data only without embedded imagery;
- central branding configuration contract separating internal 'Platform' technical identity from commercial 'AmrudAi' customer-facing name, with logo asset references and white-label/MSP tenant override support (resolving effective commercial product name and logos on customer-facing UI and PDF reports, with data-only CSV);
- RBAC, tenant/resource scope re-evaluation at execution time, query/resource bounds and cancellation;
- drill-down from summarized result to governed evidence.

Service Assurance shall support customer SLA, internal-team OLA, provider/underpinning SLA and SLI/SLO objectives from a common evidence model.

The policy model shall support:
- service/resource scope and measurement basis (service minutes, entity/impacted-unit minutes, transaction/ticket/event counts, weighted score or governed custom formula);
- expected/target and minimum thresholds;
- reusable service-time profiles with IANA timezone, holiday calendar, business hours, country/site offsets, shifts and effective dates;
- region, site, business unit, environment, criticality, provider, service, shift, team and governed customer attributes;
- monthly/quarterly/yearly/rolling/custom measurement windows and per-ticket measurement where applicable;
- versioned inclusion/exclusion rules, approved maintenance, responsibility boundaries and governed country/site/service deviations;
- configurable degraded-condition treatment including informational-only, partial penalty, delayed outage after configured persistence, or immediate outage;
- direct monitoring/synthetic/service evidence as authoritative measurement where policy requires it, with ITSM tickets as supporting evidence rather than a mandatory availability source;
- dependency/service-topology semantics so service availability is not calculated by blindly averaging component uptime;
- safe typed formulas and aggregates without arbitrary SQL/Python/JavaScript/eval;
- continuous-improvement measures such as incident reduction, automation increase, MTTR improvement or compliance improvement;
- OLA dependency chains and provider relationships.

Before activation/change, authorized users shall be able to simulate/backtest the policy against retained historical evidence and compare old/new results without mutating historical truth.

Each measurement period shall preserve immutable/reproducible policy version, formula version, calendar version, scope snapshot, evidence references, raw result, exclusions, adjustments, data-quality/completeness and final result. Period lifecycle shall support `OPEN -> PRELIMINARY -> REVIEW -> FINAL`, with audited `RESTATED` revision for post-final correction rather than silent recalculation.

Service-assurance result quality shall be explicit (`VALID`, `PARTIAL`, `INSUFFICIENT_EVIDENCE`, `DISPUTED` or equivalent). Missing/stale/monitoring-path evidence must not be silently converted into service downtime or perfect compliance.

Multi-vantage policies may use governed quorum/consensus rules. Conflicting evidence is preserved and can yield partial/inconclusive measurement.

Shift/team attribution shall segment incidents/outages across effective ownership intervals and handoffs. Historical ownership must not be rewritten by later team changes. Team-authoritative operation remains gated by the canonical tenant-scoped Team identity/binding contract.

Report obligations shall support a stable report key/reference, owner, frequency, service-time/calendar, due rule, recipients, format (`PDF`, `CSV`), required sections/data, approvers, delivery channel/status and audit.

SLA/OLA reports shall be able to expose target/expected, minimum, actual, trend, remaining allowable downtime/error budget where applicable, breach/at-risk state, qualified downtime, approved exclusions, monitoring completeness, contributing incidents/outages, ownership/provider attribution and policy/evidence versions.

Financial/service-credit formulas may be represented as customer-defined metadata/calculation only when configured; Platform shall not invent contractual credits, pricing, liability or commercial terms.

## 28. ITSM and notification integrations
Supported adapters shall include architecture for:

- ServiceNow;
- BMC Helix/Remedy;
- ServiceDesk Plus;
- SummitAI;
- Freshservice/Freshworks;
- Jira Service Management;
- generic REST/webhook;
- email;
- Microsoft Teams;
- Slack;
- approved WhatsApp Business provider/API;
- SMS/PagerDuty adapters where configured.

ITSM integration shall be bidirectional where the target API permits.

Required connector contracts include: stable Platform incident ↔ external object binding; connector capability/version certification; guided preflight (DNS/TLS/proxy/auth/permissions); type-safe field mapping; required-field/state validation; payload preview before egress; idempotency keys; race-safe bidirectional lifecycle; signed/authenticated inbound callbacks; queue/retry/dead-letter health; pause/drain/replay; safe configuration promotion; credential rebinding; sanitized support evidence. External ticket closure/state/automation success does **not** prove monitored recovery or override Platform health. No arbitrary Python/JavaScript customer mapping runtime. No mandatory new Integration Fabric service.

Ticket creation shall include root cause/confidence, affected scope, correlated alerts, evidence, config changes and recommended actions.

Recovery policy options:

- resolve only in Platform;
- update ITSM but leave ticket open;
- auto-resolve/close selected incident types;
- resolved-monitoring-recovery state.

A durable outbound integration queue, retries, idempotency/correlation IDs and connector health dashboard are required.

An Integration Test Sandbox shall validate authentication, connection, field mapping, create/update/resolve flows before production enablement.

---


## 29. RBAC, audit, API-first and query protection
Authorization model: **Who × Resource × Action × Where × Conditions**.

Dimensions:

- tenant;
- region/site/group/device/service;
- module;
- action;
- condition/time/MFA/ticket/approval.

Explicit deny wins where supported.

Built-in role templates are cloneable/customizable.

Sensitive actions may require MFA, approval, reason or ticket.

Access Simulator shall answer questions such as “Can user X execute configuration command on device Y?” and explain the effective permission path.

All authorization is enforced server-side.

---


Audit shall cover authentication, users/RBAC, provisioning, credentials, monitoring policies, alerts/incidents, maintenance, NCM/CLI, IPAM, dashboards/maps/reports, integrations, AI/remediation, Sensors/HA, licensing, retention, backup/restore and upgrades.

Audit event fields include:

- actor/service/policy engine;
- tenant;
- action;
- object/resource;
- timestamp;
- source UI/API/system;
- source IP/session/request ID;
- before/after;
- result;
- reason/ticket.

Audit storage should support append-only/tamper-evident controls, configurable retention/archival, SIEM export and restricted deletion.

---


All normal customer operations remain UI-first, but the UI shall consume supported control-plane APIs rather than bypassing them with database access.

API Management shall support service accounts, clients/tokens, scopes, rate limits, usage, failures, expiry/rotation and audit.

Outbound event/webhook subscriptions shall include relevant alert, incident, device, NCM, SLA and state-change events.

---


Platform shall protect itself from:

- unbounded dashboard/raw queries;
- high-cardinality custom metrics/labels;
- API connector explosions;
- excessive Flow/log rate;
- excessive report concurrency.

Controls include timeouts, maximum scan/rows, asynchronous large exports, cancellation, per-tenant/user concurrency, cardinality detection and recommendations to drop/normalize unstable labels.

---


**Maturity note:** persistent production IdentityDirectory / PermissionGrantDirectory authority may be absent on current main; Discovery authorization may be intentionally fail-closed until permission integration is complete (#92). Target product requirements remain as specified above.

## 30. Retention, Storage Protection Engine and Legal/investigation hold
Administration → Data & Retention shall allow configuration by data class/scope for:

- raw retention;
- rollup interval/resolution;
- rollup retention;
- archive mode/destination/retention;
- purge priority;
- minimum recent operational window.

Rollups for numeric metrics shall preserve suitable statistics such as min/max/avg/count/first/last and percentile where meaningful. Counters use delta/rate/total semantics. State metrics use availability/duration/transitions rather than numeric averaging.

Historical chart query planning shall automatically select an appropriate resolution for the requested time range and disclose the resolution in UI.

Normal deletion should verify required rollups exist before raw data expires; emergency protection may override normal retention when necessary to protect live monitoring.

---


Mandatory for Core and Sensor managed volumes.

Operational zones:

- <70% normal;
- 70–75% warning/forecast;
- 75–80% aggressive housekeeping/alert;
- ≥80% emergency reclamation.

The 80% emergency watermark is non-disableable.

Reclamation order is policy-controlled within protected-data constraints and normally starts with oldest high-volume raw telemetry/flow/log data, preserving aggregates longer where possible.

Protected configuration, identity, security, current inventory and active incident state are not indiscriminately purged.

UI shall report storage before/after, bytes reclaimed, affected data/time ranges and whether monitoring was interrupted.

If eligible data cannot reclaim sufficient space, Platform raises critical `STORAGE_RECLAMATION_INSUFFICIENT` with required action.

Data-growth forecasting shall estimate time to watermark by volume/data class.

---


Authorized users shall be able to protect incident evidence for a defined scope/time range from normal retention/purge, including selected telemetry, Flow, logs, events, configuration, audit and RCA artifacts. Emergency capacity conflicts shall be surfaced explicitly; Platform must not silently violate protected retention policy.

---


### Retention/protection constraints (absorbed from PRD-STORAGE)
Administrators configure per tenant/data type/scope:

- raw retention;
- each rollup resolution and rollup retention;
- archive action/destination/retention;
- legal-hold behavior;
- emergency purge priority within protected-data constraints.

Rollup semantics must match metric type: gauges preserve min/max/avg/count/percentiles as configured; counters use delta/rate/total as appropriate; states preserve availability, duration, transitions and last state. Historical queries automatically select the appropriate resolution and disclose the resolution used.

Core and Sensors implement non-disableable 80% managed-storage safety protection. Cleanup targets safe headroom rather than merely 79.9%. Purge actions are audited and visible. Monitoring/collection remains the priority.


## 31. Asset lifecycle, technology lifecycle and desired-vs-actual
Lifecycle states shall include Discovered, Pending Approval, Provisioning, Active/Monitored, Monitoring Degraded, Unmanaged, Disabled, Maintenance, Failed Provisioning, Decommission Requested, Decommissioned and Archived as applicable.

Reconciliation detects disappeared devices, identity changes, IP/hostname moves, replacement/serial changes and stale licensed inventory. It shall never silently delete stale devices; they enter a review/decommission workflow.

---


The following capabilities are part of the agreed Platform scope and must not be lost simply because they were identified after the first dashboard catalogue was drafted.

### 31.1 Technology Lifecycle / EOS-EOL visibility

Where authoritative vendor/customer data is legally available, Platform shall track model, firmware/software version, support lifecycle metadata, EOS/EOL dates, support/warranty metadata and lifecycle risk. Lifecycle data must preserve source/provenance and must not fabricate unsupported vendor dates.

The product shall surface:

- unsupported firmware/software;
- hardware approaching EOL/EOS;
- models already out of support;
- inventory affected by lifecycle risk;
- recommended review/upgrade candidates.

This is operational asset intelligence, not a substitute for vulnerability management.

### 31.2 Desired State vs Actual State

For monitoring configuration, Platform shall continuously compare desired policy/assignment state with actual applied Sensor/entity state.

Examples:

- expected Sensor assignment vs actual;
- expected monitoring profile vs actual;
- desired protocol enabled vs observed protocol failure;
- desired credential version vs applied/validated version;
- desired Device Pack version vs actual;
- desired Agent/Sensor version vs actual;
- desired retention/alert policy vs effective configuration.

Drift is visible, explainable and auditable.

### 31.3 Shift Handover

NOC operations shall have a Shift Handover workspace/report covering:

- open critical/major incidents;
- incidents under investigation/recovery;
- unstable/flapping entities;
- active/scheduled maintenance;
- pending NCM/privileged approvals;
- significant configuration changes;
- Sensor/Core/integration degradation;
- ticket synchronization failures;
- capacity/storage/license warnings;
- operator notes and ownership handoff.

### 31.4 Migration Studio

To reduce adoption friction, Platform shall provide a Migration Studio capable of importing and reconciling inventory/configuration data from customer exports/APIs. Initial generic import must support CSV/XLSX/CMDB-style data; vendor-specific migration adapters may be added for SolarWinds, ManageEngine, Zabbix, PRTG, Nagios, LogicMonitor, NetBox and others where legal/API/export mechanisms permit.

Migration outcomes shall classify Imported, Automatically Mapped, Needs Review and Unsupported. Platform must not claim full translation of proprietary policies when semantic equivalence does not exist.

### 31.5 Knowledge Base / Runbook Library

Customers shall maintain approved runbooks, known issues, escalation procedures and troubleshooting guides, scoped by tenant/site/device type/service/incident class.

RCA and AI may recommend or summarize approved runbooks. Customer-approved source content remains authoritative. Runbooks must be versioned/audited and executable steps must not become autonomous actions unless future remediation policy explicitly allows it.

### 31.6 Post-Incident Review / Problem Management

Platform shall generate a draft PIR from incident timeline, RCA, impact, evidence, ITSM ticket, configuration changes, actions and recovery. Human operators add/approve final root cause, corrective action, preventive action and lessons learned. Output can be reported/exported.

### 31.7 Intelligence Quality / RCA Accuracy

Platform shall measure and display, subject to available feedback/ground truth:

- RCA runs;
- confirmed correct/partially correct/incorrect;
- inconclusive rate;
- root-cause precision;
- false-root-cause rate;
- time-to-root-cause;
- explanation completeness;
- blast-radius accuracy;
- recommendation correctness.

Metrics must distinguish actual validated outcomes from unverified confidence scores.

### 31.8 Alert Policy Effectiveness

Platform shall identify noisy/low-value rules using evidence such as alert volume, incident conversion, acknowledgement/closure patterns, repeat rates and suppression. It may recommend review of a policy but does not silently change thresholds.

### 31.9 Monitoring Optimization Recommendations

Platform may recommend reducing unnecessary metric collection/polling when metrics are high-cost but unused by dashboards, alerts, reports or RCA over a configurable analysis period. It may also recommend Sensor redistribution. Recommendations require review/approval unless a safe automatic policy is explicitly enabled.

### 31.10 Change Freeze and Operational Calendars

Platform shall support business calendars, working hours, regional holidays, change freezes and blackout windows. NCM pushes, Platform/Sensor/Agent/Device Pack upgrades and other governed changes can be blocked or require emergency override/approval during freeze windows.

### 31.11 Device / Time Comparison

Operators shall be able to compare device A vs device B, or one entity/time window vs another, aligning normalized metrics, configuration, firmware, monitoring profile, topology, routing, errors and alerts where compatible. Missing/incompatible fields are marked rather than fabricated.

### 31.12 Data Pipeline Integrity

Platform Operations shall expose produced, accepted, consumed, inserted, rejected, duplicate, late, schema-error and dropped telemetry counts. The purpose is to detect silent data loss between Sensor, ingestion, Kafka, processing and ClickHouse.

### 31.13 Scheduled Housekeeping

A Housekeeping workspace shall show status/history/failures for rollups, TTL deletion, emergency purge, database compaction/maintenance, backups, archives, NCM backups, discovery jobs, report jobs and Sensor redistribution. Internal lifecycle work must not be invisible.

### 31.14 Usage and Licensing Forecast

Authorized administrators shall see licensed vs used entities/elements/modules and forecast approaching entitlement thresholds/expiry. This does not replace the agreed expiry/grace behavior.

### 31.15 Contextual Help

Complex settings and diagnostics shall expose `What is this?`, contextual documentation and, where allowed, AI explanation grounded in current Platform documentation/configuration. Help must never modify configuration without a separate explicit action.

---


## 32. Upgrade, canary, rollback and PKI
Platform upgrade workflow shall include:

- preflight;
- backup/restore point;
- compatibility check;
- component/Sensor drain where required;
- install/migrate;
- health verification;
- rollback strategy.

Sensor/Agent/Device Pack changes should support canary/staged rollout (e.g., pilot group → larger group → full deployment) with configurable stop/rollback conditions.

Feature flags may restrict new features to selected tenants/groups during controlled rollout.

---


Administration shall show UI, Sensor, Agent, internal-service and integration certificates, issuer, expiry, days remaining, usage and status.

Support customer CA, CSR/import/renewal, trust stores and certificate rotation. Expiring certificates produce proactive alerts.

---


## 33. Licensing and usage forecast

### Licensing grace — Founder-approved 15-day post-expiry grace

**Authority:** Founder decision **2026-09-30** — post-expiry licence grace is **15 days** (`EXPIRED_GRACE_15D`).

Supersedes prior seven-day post-expiry authority, normative `FOUNDER-LIC-GRACE UNRESOLVED`, and any normative 7-vs-15 unresolved conflict.

Canonical state machine:

`VALID → EXPIRING_30D → EXPIRING_7D_BANNER → EXPIRED_GRACE_15D → POLLING_SUSPENDED`

Distinguish:

- **T-7 pre-expiry banner** (7 days before expiry) — remains governed;
- **15-day post-expiry grace** — polling continues for 15 days after expiry.

Normative behavior:

- polling continues during the 15-day post-expiry grace;
- after the 15-day grace ends, polling stops unless the licence is manually extended/renewed;
- historical data remains accessible; no licence-expiry data deletion;
- engineering must not reintroduce seven-day post-expiry grace as normative truth.

Historical/superseded provenance only: earlier Mother Reference / source material recorded a seven-day post-expiry grace and temporarily treated 7-day vs 15-day as unresolved. That conflict is **closed**.

Normative non-grace continuity (unchanged):

- Warn 30 days before expiry (T-30).
- From 7 days before expiry (T-7), every dashboard shows a top banner: `Licence expires in X days`.
- Licence settings/import remain accessible to authorized admins.
- All licence/entitlement state changes are audited and visible in Usage & Licensing.

### Licence expiry behavior

- T-30 days: warning begins.
- T-7 days: persistent dashboard banner with exact days remaining.
- Post-expiry: **15-day** grace (`EXPIRED_GRACE_15D`); polling continues during grace.
- After the 15-day grace ends: polling stops unless manually extended/renewed; historical data remains available.
- License/entitlement events are audited and visible in Usage & Licensing dashboard.


## 34. UX performance, accessibility and Dashboard Studio
UI shall support:

- light/dark/system theme;
- compact/comfortable/large density/readability modes;
- readable enterprise font sizes;
- keyboard navigation and accessible semantics where practical;
- severity shown with icon/text as well as color;
- server-side pagination;
- virtualized large tables;
- lazy loading;
- query cancellation;
- asynchronous long-running operations with progress;
- WebSocket/SSE updates where beneficial.

UI foundations shall include internationalization readiness (locale-aware formatting where applicable) alongside accessibility requirements above. Exact translation scope remains product-configurable; accessibility and readable contrast remain mandatory.

Exact numeric SLOs are benchmark-defined TBD, but common interactive actions must remain low-latency and regressions shall fail performance certification.

---


Library scopes:

- My Dashboards;
- Shared;
- Team;
- Tenant/Customer;
- System;
- folders/groups.

Actions include Create, Clone, Move, Rename, Share, Import, Export, Delete, Save As, Permissions, Variables, Preview, Undo/Redo.

Panel types include Stat, Gauge, Time Series, Bar, Pie, Table, Heatmap, Histogram, Status Grid, Sankey, Topology, Geo Map, Text, Alert List, Incident List, RCA and domain-specific visualizations.

Panels are draggable, resizable, duplicable and linkable. Users can add/remove/reorder dashboards and group them easily.

---


Where meaningful, dashboards shall support:

- time range and comparison period;
- timezone;
- auto refresh;
- tenant/region/country/state/city/site/business unit/team/device group/dynamic group/device type/vendor/model/technology/criticality/environment/tag/Sensor filters;
- status filters;
- Group By;
- table/chart/map/topology/heatmap/treemap/gauge/Sankey view switching where appropriate;
- Edit, Clone, Save As, Share, Export, Report, Annotation, Alert, Maintenance, Investigate and CLI actions according to RBAC.

---


## 35. Out-of-box dashboard and workspace catalogue
The following catalogue is required out of box. Entity-detail dashboards may be contextual rather than separate left-nav items.

| # | Dashboard / Workspace | Primary users | Out-of-box purpose/panels | Default tabs/views |
|---:|---|---|---|---|
| 1 | **Executive / CIO Overview** | CIO/CTO/management | Overall technology and business-service health, SLA, major incidents, capacity risk, MTTA/MTTR, alert reduction and platform health. | Overview; Business Services; Availability; Incidents; Capacity; SLA; Trends |
| 2 | **Global Operations Overview** | NOC/IT operations | Cross-domain operational health, active incidents/alerts, sites, monitoring degradation, recent changes and ITSM state. | Overview; Network; Infrastructure; Cloud; Services; Incidents; Collectors |
| 3 | **NOC Command Center** | NOC wallboard | Large-screen critical incidents, affected sites/services, WAN/DC health, changes, Sensor health and ticket queue; TV mode. | Wallboard/Full Screen |
| 4 | **Geographic Operations Map** | Global/regional ops | Country/city/site/DC/cloud-region health with drill-down. | Map; Sites; Incidents |
| 5 | **Business Service Health** | Service owners/NOC | Service health, dependencies, incidents, SLA, changes and synthetics. | Health; Dependencies; Topology; Incidents; SLA; Changes |
| 6 | **Site Overview** | Regional/site ops | Network, infrastructure, WAN, wireless, traffic, services, alerts and SLA for one site. | Overview; Network; Infrastructure; WAN; Wireless; Traffic; Services; Alerts; Topology; SLA |
| 7 | **Branch Experience** | Branch/NOC | Branch→DC and DC→branch latency/loss/jitter/path, DNS/HTTP/VPN/SD-WAN and Agent health. | Experience; Paths; Services; Network; Agent; History |
| 8 | **Branch Comparison** | NOC | Fleet heatmap/table comparing branch availability, WAN, Internet, latency/loss/jitter, SD-WAN, Agent, alerts and SLA. | Comparison; Heatmap; SLA |
| 9 | **Network Health Overview** | Network NOC | Network device, interface, link, routing, traffic, availability, error and config-change health. | Overview; Devices; Interfaces; Links; Routing; Traffic; Availability; Errors; Changes |
| 10 | **Network Device Detail** | Network engineer | Universal device page with identity, CPU/memory/uptime/hardware, interfaces, routing, traffic, topology, NCM, events, RCA and audit. | Overview; Interfaces; Routing; Traffic; Hardware; Topology; Configuration; Events; RCA; Monitoring; Audit |
| 11 | **Interface Detail** | Network engineer | Interface state, utilization, errors/discards, traffic, QoS, connectivity, optical/neighbor context and history. | Performance; Errors; Traffic; QoS; Connectivity; Events; History |
| 12 | **Critical Link** | NOC/network | Availability, utilization, latency, jitter, loss, SLA, provider, backup/failover and traffic for critical circuits. | Overview; SLA; Traffic; History |
| 13 | **Link Monitor** | NOC | Mass link table/heatmap with provider, bandwidth, utilization, latency, loss, errors, availability and SLA. | Table; Utilization Heatmap; Latency Heatmap; SLA Breach |
| 14 | **Availability** | NOC/management | Device/link/site/service availability, maintenance-adjusted availability and rollups. | Devices; Links; Sites; Services |
| 15 | **Network Performance** | Network engineer | CPU/memory/interface/latency/loss/errors/drops/queueing/congestion and top offenders. | Overview; Top Offenders; Trends |
| 16 | **Network Capacity** | Capacity/network | Top utilized links, sustained usage, growth, percentiles, forecast and upgrade candidates. | Current; Trends; Forecast; Recommendations |
| 17 | **Network Flow / Traffic Analytics** | Network engineer | Top talkers/conversations/protocols/ports/sites/interfaces/ASN, traffic matrix, Sankey, time series and anomalies. | Overview; Conversations; Sources; Destinations; Protocols; Sites; Interfaces; ASN; Traffic Matrix |
| 18 | **Site-to-Site Traffic** | Network/capacity | Inter-site traffic volumes, protocol mix, trends and capacity context. | Matrix; Trends; Conversations |
| 19 | **Routing Health** | Network engineer | BGP/OSPF/IS-IS/BFD/HA protocol status, flaps and route-change anomalies. | BGP; OSPF; IS-IS; BFD; HA Protocols; Routes; Changes |
| 20 | **BGP** | Network engineer | Peers, state, ASN, prefixes, flaps, uptime, BFD/interface relationships and route changes. | Peers; Prefixes; Flaps; Changes |
| 21 | **MPLS / VRF** | Network engineer | VRF, PE/CE, LSP/path, latency, utilization, route health and site reachability. | VRFs; LSPs; PE-CE; Health; Traffic |
| 22 | **VXLAN / EVPN** | DC network | VTEP, VNI, EVPN peers, underlay/overlay, reachability and MAC/IP relationships. | Overview; VTEPs; VNIs; Peers; Underlay; Overlay |
| 23 | **SD-WAN Overview** | WAN ops | Sites/edges, underlay links, overlay tunnels, SLA classes, BFD, latency/loss/jitter and vendor-normalized KPIs. | Overview; Sites; Links; Tunnels; Underlay; Overlay; SLA; Applications; Incidents |
| 24 | **VPN** | Network/security ops | VPN tunnels, peer, duration, throughput, latency, auth/failure and alert history. | Overview; Tunnels; Peers; History |
| 25 | **WLAN Overview** | Wireless ops | Controllers/APs/clients/radios, channel/interference, RSSI/SNR, auth/association/roaming/retransmits. | Overview; APs; Clients; Radios; Controllers; Sites; Experience |
| 26 | **VoIP / CUCM** | Voice ops | CUCM/gateway/trunk/phone health, registration and call-quality latency/jitter/loss. | Overview; Gateways; Trunks; Phones; Quality |
| 27 | **IP SLA / Network Assurance** | Network ops | RTT/jitter/loss/availability plus DNS/HTTP/UDP/ICMP probes against SLA. | Overview; Tests; SLA; History |
| 28 | **QoS / CBQoS** | Network engineer | Policy/class offered traffic, drops, queues, utilization and congestion history. | Policies; Classes; Drops; Trends |
| 29 | **Infrastructure Health Overview** | Infra NOC | Physical server, OS, virtualization, storage, DB, hardware, cloud and K8s health. | Overview; Servers; Virtualization; Storage; Databases; Cloud; Kubernetes |
| 30 | **Server Fleet** | Infra engineer | Server table with OS/site/CPU/memory/disk/network/process/service/Agent/alerts. | Fleet; Health; Capacity |
| 31 | **Server Detail** | Infra engineer | Host overview, disk, NIC, process, services, hardware, logs, events, RCA and audit. | Overview; Disk; Network; Processes; Services; Hardware; Logs; Events; RCA; Audit |
| 32 | **Process & Service** | Infra engineer | Process name/command line/PID/CPU/memory/restarts/state and Windows/Linux services. | Processes; Services; History |
| 33 | **Hardware / Out-of-Band** | Infra/DC | iDRAC/iLO/CIMC/Redfish chassis, BIOS, CPU/DIMM/RAID/disk/PSU/temp/fans/battery/NIC/lifecycle. | Overview; Components; Sensors; Firmware; Events |
| 34 | **Virtualization Overview** | Infra/virtualization | VMware, Hyper-V, Proxmox, Nutanix clusters/hosts/VMs/datastores, contention and capacity. | Overview; Clusters; Hosts; VMs; Storage; Capacity |
| 35 | **VMware** | VMware admin | vCenter→DC→cluster→ESXi→VM→datastore hierarchy and health. | Overview; Clusters; Hosts; VMs; Datastores; Networks; Capacity; Events |
| 36 | **Nutanix** | Nutanix admin | Cluster/nodes/CVM/VM/storage/alerts/capacity/latency. | Overview; Nodes; CVMs; VMs; Storage; Capacity |
| 37 | **Hyper-V** | Windows virtualization | Host/cluster/VM/storage/network and capacity. | Overview; Hosts; VMs; Storage; Network |
| 38 | **Proxmox** | Proxmox admin | Cluster/node/VM/container/storage/network. | Overview; Nodes; VMs; Containers; Storage; Network |
| 39 | **Storage Overview** | Storage admin | Capacity/utilization/IOPS/throughput/latency/controller/disk/pool/volume/replication/hardware health. | Overview; Pools; Volumes; Hardware; Replication; Capacity |
| 40 | **SAN Fabric** | Storage/network | FC switches/ports/fabrics/zones/WWNs/link/errors/congestion/utilization. | Fabric; Switches; Ports; Zones; Errors |
| 41 | **Data Center Environmental** | DC ops | UPS/PDU/temperature/humidity/power/cooling/rack sensor health. | Overview; Power; Environment; Cooling; Events |
| 42 | **Database Estate Overview** | DBA/NOC | Cross-database availability, sessions, host/storage, replication, locks/errors and health. | Overview; Engines; Health; Capacity; Alerts |
| 43 | **Database Detail** | DBA | Connections, query health, locks/waits, storage, replication, errors, host dependency, events and RCA. | Overview; Connections; Queries; Locks; Storage; Replication; Errors; Host; Events; RCA |
| 44 | **Multi-Cloud Overview** | Cloud ops | AWS/Azure/GCP accounts, regions, compute, storage, DB, network, K8s/LB, health and capacity. | Overview; Accounts; Regions; Compute; Network; Storage; Databases; Kubernetes |
| 45 | **AWS Overview** | AWS ops | EC2/RDS/network/LB/storage/CloudWatch/regions/alerts. | EC2; RDS; Network; Load Balancers; Storage; Regions; Alerts |
| 46 | **Azure Overview** | Azure ops | VMs, Azure Monitor, VNet, SQL, storage, LB and AKS. | VMs; Network; SQL; Storage; Load Balancers; AKS; Alerts |
| 47 | **GCP Overview** | GCP ops | Compute, networking, storage, GKE, DB and monitoring. | Compute; Network; Storage; GKE; Databases; Alerts |
| 48 | **Kubernetes Overview** | Platform/K8s ops | Cluster/node/namespace/workload/pod/container/service/ingress health, events, restarts and capacity. | Overview; Nodes; Namespaces; Workloads; Pods; Containers; Services; Ingress; Events; Capacity |
| 49 | **Container Detail** | K8s/container ops | CPU/memory/network/disk/restarts/state/logs and node/host context. | Overview; Resources; Network; Logs; History |
| 50 | **Synthetic Monitoring Overview** | NOC/service owners | ICMP/TCP/DNS/HTTP/HTTPS/API/TLS tests, availability, latency, failures, locations and SLA. | Overview; Tests; Locations; SLA |
| 51 | **Synthetic Test Detail** | NOC/service owner | Availability/timing/DNS/TCP/TLS/HTTP/location/history with detailed timing. | Overview; Availability; Timing; DNS; TCP; TLS; HTTP; Locations; History |
| 52 | **Certificate** | Security/platform ops | Expiring/invalid certificates, hostname mismatch, chain, weak TLS policy and days remaining. | Overview; Expiring; Invalid; Chain; Policy |
| 53 | **DNS / DHCP / NTP** | Network services | DNS response/latency/record validation; DHCP scope/lease; NTP offset/stratum/sync. | DNS; DHCP; NTP |
| 54 | **AD / Identity Infrastructure** | Identity ops | DCs, replication, LDAP, DNS dependency, Kerberos/auth health and selected RADIUS/TACACS tests. | Overview; Domain Controllers; Replication; Authentication; DNS |
| 55 | **Alert Console** | NOC | Filterable qualified alerts with severity/state/entity/site/duration/confidence/incident/ITSM and bulk operations. | All; Unacknowledged; Assigned; Suppressed; Maintenance |
| 56 | **Incident Console** | NOC | Open/acknowledged/investigating/resolved incidents with root entity, impact, RCA and ticket state. | Open; Acknowledged; Investigating; Resolved; Closed; Suppressed |
| 57 | **Incident Detail** | NOC/engineers | Incident summary, RCA, evidence, correlated alerts, impact, topology, timeline, changes, ITSM, notes and audit. | Summary; Root Cause; Evidence; Correlated Alerts; Impact; Topology; Timeline; Changes; ITSM; Notes; Audit |
| 58 | **RCA / Investigation Workspace** | NOC/engineers | Root cause/confidence, causal graph, evidence, contradictions, missing evidence, metrics, timeline, changes and recommendations. | Conclusion; Evidence; Causal Graph; Timeline; Metrics; Changes; Topology; Recommendations |
| 59 | **False Alert Qualification** | NOC/admin | Primary failures, validation outcomes, monitoring degradation, actual outages and false/noise prevention metrics. | Overview; Validation; Outcomes; Policies |
| 60 | **Maintenance & Suppression** | NOC/admin | Active/scheduled/recurring/expired windows, scope, reason, ticket, creator and approver. | Active; Scheduled; Recurring; History |
| 61 | **Change Timeline** | NOC/RCA | NCM, user, monitoring-policy, maintenance, software, ITSM and device changes over time. | Timeline; Filters; Correlation |
| 62 | **NCM Overview** | Network config team | Backup success/failure, changes, compliance, deviations, approvals and bulk jobs. | Overview; Devices; Backups; Changes; Compliance; Jobs; Templates; Approvals; Reports |
| 63 | **Device Configuration** | Network config | Current config, versions, diff, compliance, history and jobs. | Current Configuration; Versions; Diff; Compliance; History; Jobs |
| 64 | **NCM Compliance** | Network/security | Compliant/noncompliant/unknown by policy/site/vendor with violation detail. | Overview; Policies; Devices; Violations |
| 65 | **Configuration Change** | Network/RCA | Changes correlated with users, CLI sessions, NCM jobs, alerts and incidents. | Timeline; Users; Devices; Incidents |
| 66 | **IPAM Overview** | IPAM admin | IPv4/IPv6 utilization, subnets, free/used/reserved, conflicts, DHCP/DNS and forecast. | Overview; IPv4; IPv6; Subnets; Conflicts; Forecast |
| 67 | **Subnet Detail** | IPAM admin | Addresses, utilization, devices, DHCP, DNS, history and forecast. | Addresses; Utilization; Devices; DHCP; DNS; History; Forecast |
| 68 | **IP Conflict** | IPAM/NOC | Duplicate IP/MAC conflict, first/last seen, site and remediation state. | Open; History; Resolved |
| 69 | **IPAM Capacity Forecast** | IPAM/capacity | Subnet growth and predicted exhaustion date. | Overview; Forecast; Risk |
| 70 | **Capacity Planning Overview** | Capacity/management | Network/server/storage/cloud/DB/site utilization, growth and risk horizon. | Current; Growth; Forecast; Recommendations |
| 71 | **SLA** | Management/NOC | SLA by service/region/provider/site, violations, downtime and maintenance exclusions. | Overview; Services; Sites; Providers; Violations |
| 72 | **ISP / Carrier SLA** | WAN/vendor mgmt | Provider/circuit availability, latency, loss, jitter, target/actual/breach. | Overview; Providers; Circuits; Breaches |
| 73 | **Sensor Estate Overview** | Platform admin | Sensor roles, health, offline/drain/maintenance/standby and capacity. | Overview; Pools; Capacity; Versions |
| 74 | **Sensor Detail** | Platform admin | CPU/RAM/disk/queue/polls/sec/metrics/sec/failures/RTT/buffer/targets/capacity. | Overview; Workload; Targets; Queues; HA; Logs; Updates; Audit |
| 75 | **Sensor Pool / HA** | Platform admin | Active/standby capacity, N+1/N+K/1:1 state, utilization, failover and assignments. | Overview; Assignments; Failover; Capacity |
| 76 | **Telemetry Quality** | Platform/NOC | Freshness/completeness/missing metrics/failed polls/delays/clock/schema errors and data-quality score. | Overview; Freshness; Completeness; Failures; Clock; Schema |
| 77 | **Agent Estate** | Platform/infra | Agent OS/version/online state/update/heartbeat/site/buffer/test profile. | Overview; Versions; Health; Updates; Edge Tests |
| 78 | **Kafka / Data Pipeline Health** | Platform admin | Broker/topic/partition/consumer lag/throughput/schema/ingestion health. | Overview; Topics; Consumers; Errors; Throughput |
| 79 | **Core Database / Storage Health** | Platform admin | PostgreSQL and ClickHouse health, connections/inserts/queries/merges/disk/sync. | PostgreSQL; ClickHouse; Storage; Health |
| 80 | **Integration Health** | Integration admin | ServiceNow/BMC/ServiceDesk/SummitAI/Freshservice/Teams/Slack/email/webhook connection, queue, errors and latency. | Overview; Connectors; Queue; Errors |
| 81 | **ITSM Lifecycle** | NOC/integration | Tickets created/updated/resolved/closed/reopened/failed sync and Platform↔ITSM correlation. | Overview; Created; Updated; Recovery; Failures |
| 82 | **Audit Overview** | Auditor/security | User actions, config changes, login failures, CLI/NCM/RBAC/credential/API actions. | Overview; Privileged; Authentication; Configuration |
| 83 | **Audit Explorer** | Auditor/security | Searchable audit by user/action/resource/module/tenant/site/IP/session/result with before/after. | Search; Detail; Export |
| 84 | **Access Governance** | Security/admin | Users, privileged users, dormant/service accounts, MFA, temporary access and denied attempts. | Overview; Users; Privileged; Temporary Access; Failures |
| 85 | **CLI Session History** | Security/network | User/device/start/end/commands/privilege/result/config-change/ticket and optional playback. | Sessions; Commands; Changes; Playback |

### 35.1 Common dashboard behavior

All applicable dashboards shall inherit the Universal Dashboard Controls defined above. Drill-down must preserve tenant/site/device/time context. A row/entity click should support a quick side panel and full contextual page. Saved views may be Private, Team or Tenant-wide according to authorization.

### 35.2 Role landing pages

Default role mappings:

- Executive → Executive/CIO Overview.
- NOC → NOC Command Center.
- Network Engineer → Network Health.
- Infrastructure Engineer → Infrastructure Health.
- Cloud Engineer → Multi-Cloud Overview.
- Auditor → Audit Explorer.
- MSP Admin → Multi-customer portfolio/global operations view.

Customers may override landing page policy.

---


### Additional OOB workspaces #86–98
These extend the earlier catalogue and are also required out of box:

| # | Dashboard / Workspace | Purpose | Key views/actions |
|---:|---|---|---|
| 86 | **Technology Lifecycle Risk** | EOS/EOL/unsupported firmware/model risk by vendor/site/criticality | Lifecycle risk; Firmware; Models; Sites; Export; Create remediation plan |
| 87 | **Desired vs Actual / Configuration Drift** | Monitoring/assignment/profile/version drift | Drift by Sensor/entity/policy; Explain source; Reconcile; Audit |
| 88 | **Shift Handover** | NOC handover of unresolved operational state | Critical incidents; maintenance; changes; pending approvals; platform issues; notes; handover report |
| 89 | **Migration Studio** | Import/reconcile from existing monitoring/CMDB tools | Sources; Imports; Mapping; Review; Unsupported; Job history |
| 90 | **Intelligence Quality** | RCA/anomaly/qualification quality and feedback metrics | RCA accuracy; Inconclusive; false-root; time-to-RCA; feedback |
| 91 | **Alert Policy Effectiveness** | Identify noisy/ineffective rules | Alert volume; incident conversion; repeats; suppressions; recommendations |
| 92 | **Data Pipeline Integrity** | Prove telemetry continuity across ingestion pipeline | Produced; accepted; Kafka; processed; inserted; rejected; duplicates; late; dropped |
| 93 | **Storage & Retention** | Capacity, growth, retention, rollups, archive and 80% protection | Breakdown by data class/volume; forecast; purge history; policy simulation |
| 94 | **Usage & Licensing** | Entitlement/consumption/expiry forecasting | Licensed vs used; module usage; growth; expiry/grace state |
| 95 | **Housekeeping & Lifecycle Jobs** | Background maintenance/rollup/backup/archive/job status | Running; completed; failed; duration; next run; details |
| 96 | **Post-Incident Review** | Generate/review PIR and problem-management actions | Timeline; root cause; impact; CAPA; lessons; approvals; export |
| 97 | **Runbook Library** | Approved troubleshooting/operations procedures | Search; by incident/device/service; version; owner; recommendation usage |
| 98 | **Device / Time Comparison** | Side-by-side operational comparison | Metrics; config; topology; firmware; alerts; profile; time comparison |

All additional workspaces inherit the same global filters, RBAC, saved views, export and audit principles where applicable.


---

## 36. Topology & Map Studio
Topology navigation shall include Topology Explorer, Physical, L2, L3, Routing, MPLS, VXLAN/EVPN, SD-WAN, Infrastructure, Service Dependency, Traffic, Geo and Custom Maps.

Map Studio shall provide a palette of entities/sites/groups/services/cloud/links/shapes/labels/icons, canvas manipulation, relationship type, health rollup, visibility, layout and style controls.

Automatic layout options may include hierarchical, radial, force, tree, site and geographic. Manual relationships must declare relationship type/source and may not overwrite discovered truth silently.

---


## 37. Administration information architecture
### 37.1 Customers / Tenants

Customer list/create/profile, modules/licensing, deployment, users, Sensors, integrations, usage and audit. Customer users cannot browse other tenants.

### 37.2 Sites, groups and hierarchy

Sites, hierarchical groups, dynamic groups, hierarchies, tags and attributes. Sites may define preferred Network/Infrastructure/Synthetic Sensor pools and fallback pools.

### 37.3 Users & Access

Users, Roles, Groups, Permissions, Service Accounts, API Clients, Temporary Access, Approvals and Sessions.

User detail tabs: Profile, Roles, Scope, Groups, Authentication, Sessions, CLI Access, API Tokens, Audit.

Role Designer shall expose module/action/resource-scope/condition matrix and Access Simulator.

### 37.4 Authentication

Local Authentication, AD, LDAP, SAML, OIDC, Entra ID, MFA, Password Policy and Session Policy. Connections must be testable before enablement.

### 37.5 Credentials & Secrets

SNMP, SSH, WinRM, API, cloud, DB, virtualization, Redfish and generic credential types. Credential metadata includes name/type/scope/allowed Sensors/rotation/last verified/usage/audit.

### 37.6 Discovery

Jobs, ranges/seeds, credentials, schedule, exclusions, results and approval.

### 37.7 Monitoring Profiles

Profiles, Metrics, Polling, Adaptive Polling, Assignments, Inheritance, Thresholds, Validation, Alerts and Topology.

### 37.8 Alert policies

Policies, thresholds, baselines, composite rules, validation, escalation, suppression and dependencies.

### 37.9 Device Down Validation

Dedicated editor for retry counts, ICMP attempts, TCP ports, secondary Sensor, REST check, traceroute and decision expression/consensus.

### 37.10 Maintenance & Suppression

Active, Scheduled, Recurring, History plus create wizard for scope, type, schedule, alert/notification/ITSM effect, reason, ticket and approval.

### 37.11 Notifications

Email, Teams, Slack, approved WhatsApp, webhook, SMS/PagerDuty and custom adapters with routing policies.

### 37.12 ITSM

Connection, Authentication, Field Mapping, Ticket Policy, Recovery, Reopen, Synchronization, Queue, Health and Audit.

### 37.13 Sensors

Sensors, Pools, Assignments, HA, Capacity, Deployment, Versions and Updates.

Sensor Pool settings include role, region/site, capacity, HA mode, affinity and allowed device groups.

### 37.14 Agents

Agents, Policies, Versions, Updates, Edge Tests and Deployment Packages.

### 37.15 Data & Retention

Overview, Retention Policies, Rollups, Archive, Storage Forecast, Emergency Protection, Data Classes and Audit.

### 37.16 HA

Core pair, Sensor pools, state synchronization, failover history and health. Normal customers may have read-only visibility while Platform admins control changes.

### 37.17 AI

Provider, Gemini configuration, Data Policy, Tool Controls, Usage, Audit and Future Offline Provider configuration.

### 37.18 Licensing

Customer, licence type, modules, device/element consumption, expiry, support, HA entitlement and offline licence import/extension.

### 37.19 Security

Security Overview, MFA, Sessions, Certificates, API Tokens, Credential Policies, Encryption, Access Restrictions, Audit Retention, CLI Policies and Break Glass.

### 37.20 System / Platform Operations

Platform, Core Pair, Data Services, Kafka, Logs, Backup, Restore, Upgrade, Support Bundle, Time/NTP, SMTP, Proxy and Certificates.

---


## 38. Onboarding wizard, environment readiness and Migration Studio
A new installation shall not land on an unexplained empty dashboard. Wizard stages:

1. Organization/tenant.
2. Authentication.
3. Sites/regions.
4. Sensors.
5. Credentials.
6. Discovery/provisioning.
7. Monitoring profiles.
8. Alert/validation policies.
9. Notifications/ITSM.
10. Dashboard/landing selection.
11. Deployment Readiness Test.

Show setup completion state and blocking issues.

---


Readiness scanner checks, where applicable:

- Core/Sensor compute and disk;
- DNS/NTP;
- certificates;
- Core/Sensor reachability;
- target/sample reachability;
- required ports/firewall;
- credential test;
- SNMP/SSH/WinRM/API readiness;
- storage growth/headroom;
- licence capacity;
- integrations;
- data-service health.

Generate an Implementation Readiness Report suitable for customer network/security teams.

---


## 39. Backup, restore and supportability
Platform shall expose backup schedules/status/restore points and restore validation.

Support Bundle generation shall collect relevant versions, service health, logs, Sensor/data diagnostics and configuration metadata while automatically masking credentials, tokens, private keys and defined sensitive data.

Every customer-facing technical error should carry a stable error code, customer explanation, technical context, suggested action and correlation/request ID.

**No Silent Failure:** material collection, storage, integration, HA, provisioning, authentication or workflow failures must expose what failed, evidence, impact and remediation guidance rather than failing silently.

---


## 40. Data sovereignty and future regional architecture
Data placement policies shall be architecturally possible so future regional Cores can retain raw telemetry in region and federate summaries upward. This is a future scalability extension and not an initial Master Core implementation requirement.

---


## 41. Non-functional requirements and SNWP
### 41.1 Resource efficiency

- Core target up to 2,500 devices: 8 vCPU / 32 GB under certified Standard Network Workload Profile.
- Sensor current planning baseline: 4 vCPU / 16 GB / 150 GB for ~800 normal network devices.
- Sensor engineering benchmark target (NOT CERTIFIED): 8 vCPU / 32 GB for ~2,000–2,500 normal devices where workload permits.
- Agent footprint target: TBD by OS/workload benchmark; must be explicitly measured and kept low.

### 41.2 UI/API performance

Exact SLOs TBD after baseline benchmark, but release testing must cover dashboard initial load, cached load, search suggestions, table filtering, alert acknowledgement, API latency and large-environment pagination.

### 41.3 Availability

- Non-HA installations have no HA availability claim.
- HA target: 99.95% service design objective subject to deployment assumptions and final SLA definition.
- Phase 1.5 HA profile: Core failover certified target <60s.
- Phase 1.5 HA profile: Sensor monitoring continuity through Core failover.

### 41.4 Scale

Architecture target range is 2,000–200,000 devices globally through horizontal Sensors and future multiple regional Cores. Exact certified scale per release must be benchmarked; architecture target is not a release claim.

### 41.5 Security

TLS 1.2 minimum, TLS 1.3 preferred/supported, TLS 1.0/1.1 disabled, secure cookies/HSTS, least privilege, mTLS internal trust boundaries where specified, secret encryption and audit.

### 41.6 Maintainability

Use modular Core architecture rather than excessive microservices to fit resource targets. All configuration changes versioned/auditable; no normal customer change requires source-code modification.

---


This profile exists to make the 8 vCPU / 32 GB Core target objectively testable. It is a **benchmark profile**, not a restriction on customer configuration.

Engineering shall construct the profile using a representative mixture of enterprise network devices and record the exact composition in the performance test repository. The profile shall include at minimum:

- 2,500 network devices represented by realistic Cisco/HPE/Juniper/firewall/router/switch classes rather than 2,500 identical trivial stubs;
- ICMP availability;
- SNMP v2c/v3 mixture with v3 represented materially;
- interface inventory and status/performance;
- CPU/memory/uptime/environment basics where supported;
- default polling profiles with both five-minute and faster critical-monitor classes represented;
- traps/events;
- topology relationships;
- alert evaluation;
- a representative number of simultaneous dashboard users and background reports;
- representative incident/RCA workload;
- controlled Flow/syslog workload below the threshold that requires a dedicated Streaming Sensor.

**Exact interface count, metrics/device, poll distribution and event rates remain benchmark-derived values and must be committed into the test profile before a capacity claim is certified.** They are not invented in this PRD.

A separate Heavy Network Workload profile shall benchmark high-interface/high-frequency/gNMI/routing/NCM cases.

---


**SNWP status:** SNWP-2500-v1 is a **TARGET/PROFILE** for capacity engineering. Documentation of SNWP does not constitute BENCHMARKED or CERTIFIED capacity claims.

## 42. Product acceptance criteria
Acceptance criteria apply to the selected certification envelope. Before an initial enterprise release for that envelope can be signed off, engineering/QA shall prove at minimum:

- installation/upgrade/rollback and separate Core/Sensor/Agent builds;
- UI-driven provisioning for single, bulk and discovery;
- actionable preflight diagnostics;
- central Sensor/Agent assignment and locality/capacity-aware placement;
- certified SNMP/ICMP/network protocol collection; Flow/Syslog/streaming collection only when those profiles are included and certified;
- infrastructure/cloud/K8s/DB/synthetic coverage committed for release;
- policy inheritance and no hard-coded customer thresholds;
- alert qualification and recovery qualification;
- alert storm/flapping/dependency compression;
- incident lifecycle and ITSM sync;
- temporal operational graph and evidence-preserving RCA;
- GNN/ML integration only behind evidence/quality guardrails;
- `INCONCLUSIVE` RCA behavior;
- NCM/IPAM/CLI governance;
- dashboards/studios/admin/RBAC/audit;
- retention/rollups and 80% storage protection on Core and Sensor;
- Phase 1 standalone: DR architecture/runbook and backup/restore (including site-loss DR) verification;
- HTTPS-only UI;
- SBOM/licence/security scans;
- certified resource/performance tests under SNWP-2500-v1 and OBS-P1 enrichment overlay;
- Unified Network & Infrastructure Observability certification profile (NIOCP-P1-v1 defined in Platform_14 §11; future certification profile, NOT YET CERTIFIED until applicable evidence exists).

Phase 1 standalone certification is not gated by Core HA failover. When the Phase 1.5 HA certification envelope is selected, engineering/QA shall additionally prove:

- Phase 1.5 HA profile: Core HA failover <60s with Sensor continuity.

---


## 43. Explicit PRD TBD register
| ID | Open item |
|---|---|
| PRD-TBD-01 | Exact final brand/product name beyond working name Platform: Resolved — **AmrudAi** (commercial customer-facing product name; internal engineering remains "Platform") |
| PRD-TBD-02 | Exact DR RPO/RTO and failover automation policy |
| PRD-TBD-03 | Final compliance certification schedule |
| PRD-TBD-04 | Exact GNN framework/model architecture/GPU optional profile |
| PRD-TBD-05 | Final SNWP-2500-v1 metric/interface/event composition after lab characterization |
| PRD-TBD-06 | Exact numeric UI/API SLO values after benchmark |
| PRD-TBD-07 | Final default retention/rollup values and minimum emergency-recent window |
| PRD-TBD-08 | Final commercial over-entitlement behavior |
| PRD-TBD-09 | Exact trigger/load threshold for moving flow/syslog to dedicated Streaming Sensor |
| PRD-TBD-10 | Future Master Core/Regional Core release scope |


---


| FOUNDER-LIC-GRACE | Post-expiry licence grace | **RESOLVED 2026-09-30 (Founder):** **15 days** (`EXPIRED_GRACE_15D`). Supersedes prior seven-day post-expiry authority and historical UNRESOLVED 7-vs-15 conflict. T-7 pre-expiry banner remains distinct. |

## 44. A2.1 migration note

A2.1 absorbed Version 3.0 / v3.1 / v3.2.2 and related reconciliation content into the numbered normative sections above. Chronological appendices and residual “must-not-lose” checklists are not current normative truth. Git history preserves chronology. Formal `PRD-*` identifiers remain indexed in `Platform_08_Requirements_Traceability.md` without net-new or removed IDs under A2.1.

---

## Phase-1 endpoint operations requirements — Issues #169/#170

| Requirement ID | Phase-1 product requirement |
|---|---|
| `REQ-INF-014` | Platform shall maintain installed-software inventory for supported Windows/Linux servers, VMs and endpoints. Where reliably available it records native package/product identity, product/package name, optional normalized identity with provenance/confidence, version, vendor/publisher, architecture, package/source type, installation scope, authoritative install/update time when supplied by the OS source, observation time and source provenance. Core owns first-seen/last-seen reconciliation. Periodic authoritative reconciliation defaults to approximately daily with deterministic spreading/jitter; trustworthy OS/package-change evidence may trigger earlier coalesced reconciliation. Normalized changes include `SOFTWARE_INSTALLED`, `SOFTWARE_REMOVED`, and `SOFTWARE_VERSION_CHANGED`. The capability does not imply patch management, deployment, vulnerability management, licence metering, EDR or endpoint management. |
| `REQ-INF-015` | From Server/VM/Endpoint Processes/Services UI, an authorized user shall be able to select a discovered process/service, choose Monitor, configure applicable conditions, save through the normal Monitoring Profile/Policy architecture, inspect effective policy and Stop Monitoring. Durable identity prefers OS service/unit identity, then executable identity/path, then bounded exact process-name fallback; PID is never durable monitor identity. Conditions may include expected absence/stopped state, unexpected termination, restart/restart-rate, expected instance range, sustained CPU/memory and suspected unresponsiveness where supported. RBAC and audit are mandatory; alert policy does not grant application-restart authority. |
| `REQ-INF-016` | Business Criticality and Monitoring Profile shall remain separate. Platform shall support governed automatic mapping (`AUTO_FROM_CRITICALITY`) and explicit profile assignment (`EXPLICIT_PROFILE`). Built-in baseline Monitoring Profile templates are `CRITICAL`, `STANDARD`, and `BASIC`; customers may use governed custom profiles. An explicit profile is not silently replaced when Criticality changes. Automatic changes require preview/capacity impact, versioned desired state, admission, deterministic spreading and reconciliation. Collectors remain policy-agnostic and do not hard-code Criticality branches. |

### Triggered Fast RCA product behavior

The product may use the customer-facing term **Triggered Fast RCA**, but implementation reuses the existing `ENDPOINT_DIAGNOSTIC_SNAPSHOT` Diagnostic Job / Live Evidence / Investigation architecture.

Built-in bounded EDS definition families for Phase 1 are:

- `EDS_CPU_PRESSURE`
- `EDS_MEMORY_PRESSURE`
- `EDS_FILESYSTEM_CAPACITY`
- `EDS_BLOCK_IO_PRESSURE`
- `EDS_PROCESS_FAILURE`
- `EDS_PROCESS_RESTART_LOOP`
- `EDS_PROCESS_UNRESPONSIVE`
- `EDS_NETWORK_DEGRADATION`

These are definitions/profiles under one EDS diagnostic type, not independent RCA engines. Each definition collects only trigger-relevant evidence under hard timeout/item/byte/log-window/resource limits.

For qualifying physical-server or VM incidents, Core may associate additional evidence from certified Hardware/OOB, virtualization, storage and path/network vantages. Guest endpoint evidence shall not claim hypervisor truth; OS evidence shall not claim chassis/RAID/power truth. Missing or contradictory evidence reduces confidence and `INCONCLUSIVE` remains a correct result.

Deep first-GA VM cross-vantage certification is VMware-first through BUILD-033. Other hypervisor families remain monitoring targets but are not silently promoted to equivalent deep Fast-RCA certification without explicit change control and evidence.


---

## 58. v3.3.0 approved product requirements — ACP-001..ACP-018

### 58.1 Official Certified Capability Packs — Phase 1

Platform shall use the existing single Capability Pack architecture. Phase-1 specification targets are: Cisco IOS/IOS-XE, Cisco NX-OS, Juniper Junos, Arista EOS, Fortinet FortiOS, Palo Alto PAN-OS, RHEL/Ubuntu Linux (Agent and SSH dimensions separately), Windows Server (Agent and WinRM/WMI dimensions separately), Windows infrastructure (AD/DC, DNS, DHCP, IIS and certificate health), generic Redfish with explicit vendor/version compatibility, VMware vSphere, Kubernetes, PostgreSQL, Microsoft SQL Server, AWS and Azure.

Each named item is a **certification target**, not a shipped/implemented/certified claim. Applicable certification remains dimensional across Discovery, L1, L2, L3/routing, Topology, Diagnostics, NCM, Security and supported product/version. OOB behavior shall include identification, authentication/preflight, discovery/inventory, canonical entities/relationships, safe default collection/profile, dashboards, baseline alerts/qualification, diagnostics, limitations/provenance and version/upgrade behavior where the pack dimension supports them.

Phase-1.5 roadmap targets include GCP; Aruba/HPE, Dell, Extreme, Check Point and F5; vendor WLAN/SD-WAN; Hyper-V/Nutanix/Proxmox; major storage/SAN families; Oracle/MySQL/MariaDB/SAP HANA/MongoDB/ClickHouse/OpenSearch; Apache/NGINX/Tomcat/JBoss/WebLogic/WebSphere; JVM/.NET/Node.js; Kafka/RabbitMQ/ActiveMQ/Redis; Microsoft 365 and demand-justified backup products. Listing remains non-certification.

### 58.2 Investigation Workbench and Path Intelligence — Phase 1

`REQ-RCA-012` is strengthened so the Unified Operations Timeline is the evidence plane projected as the **Investigation Workbench**. It synchronizes authorized metrics, alerts/incidents, Flow, logs/Syslog/traps/events, topology, active path/routing, configuration/NCM, application/service telemetry, database/cloud changes, deployments, synthetics, Sensor state, Flight Recorder and ITSM activity where available. It does not create a second evidence store, diagnostic scheduler or RCA engine.

`REQ-NET-016` is strengthened as **Path Intelligence**: current/historical/multi-vantage path evidence, path fingerprint/change, hop/destination latency/loss/jitter where reliable, interface/routing/BGP/OSPF/BFD evidence where certified, Flow/configuration/provider/synthetic/service correlation and business-service impact projection through the canonical temporal graph. Hidden/incomplete hops preserve uncertainty and do not assert destination failure.

### 58.3 OpenTelemetry Phase 1 and 1.5

`REQ-INF-010` remains the governing phase boundary. **Phase 1 GA** supports OTLP metrics and structured logs over certified OTLP/gRPC and OTLP/HTTP profiles, resource/service identity reconciliation, trace/span correlation identifiers carried by signals, exemplars where present/useful, authenticated source/tenant binding, cardinality controls, redaction/sensitive-field policy, quality/drop/reject visibility and bounded backpressure. Governed Sensor-local/Streaming-Event Sensor ingress and controlled central/Core ingress are both permitted under the same trust/policy contract.

Phase 1 does **not** require retained distributed spans, trace search/waterfalls, tail-sampling backend, profiler, flame graphs, RUM/session replay, unlimited trace retention or a dedicated trace database.

**Phase 1.5** may add bounded sampled spans, short quota-controlled retention in the existing analytical store, trace search/waterfall, trace-derived service edges, head/tail sampling after benchmark, incident/error/slow-trace prioritization, Incident Flight Recorder pinning and guided Java/.NET/Node.js/Python instrumentation onboarding. Numerical span rate/size/retention/sampling limits remain P0 benchmark outputs.

### 58.4 NCM — Network Change & Configuration Assurance

The full NCM Change Assurance lifecycle is **Plan -> Backup -> Compare -> Preflight -> Assess -> Approve -> Execute -> Validate -> Observe -> Rollback/Escalate -> Audit**. It strengthens existing `REQ-NCM-001..005` with scheduled/on-demand/pre/post backup, protected history, change detection, evidence-based attribution, secret-safe configuration search, raw diff, semantic diff only through certified parsers, desired/golden state with inheritance/exceptions/drift, baseline compliance, versioned templates, deterministic pre-change checks, graph-derived blast-radius evidence, explainable deterministic risk, maker-checker, external ITSM approval binding, canary/fixed staged rollout, explicit stop criteria, post-change validation/stabilization and rollback only through certified adapter/device mechanisms.

**Certification split (Issue #200 / #202):** Phase 1 may independently certify the read-only / low-risk subset (backup/version/diff/compliance/golden-state and controlled manual CLI when RBAC/credential-brokering/audit are certified). Execute/canary/fixed staged rollout, mass push and automated privileged remediation default to the Phase 1.5 envelope unless Founder change control explicitly promotes a capability. Firmware lifecycle remains Phase 1.5.

Phase-1 compliance covers customer/vendor operational baselines and deterministic configuration checks. Advanced CIS/STIG-oriented certified rule content is Phase 1.5/evidence-and-rights dependent and must never be presented as compliance certification merely because a check exists.

The **Change Assurance Record** is an evidence envelope referencing the immutable Change Plan, NCM job, ITSM Change, configuration versions, graph/risk evidence, approvals, incidents and audit. Material target/configuration/window semantics changing after approval make that approval stale. Execution-time deterioration may block an unchanged approved plan through local policy.

NCM firmware lifecycle is Phase 1.5 and must include approved image provenance/repository, checksum/signature where available, entitlement/licensing constraints, compatibility/intermediate-version rules, storage/flash preflight, backup, approval, canary/staged rollout, reboot/health validation, certified rollback where safe, ITSM linkage and audit.

### 58.5 Platform-wide ITSM/CMDB Integration Fabric — Phase 1

Integration Fabric is logical functionality of the existing Integration Service; no mandatory new microservice is created. The common canonical integration model supports Incident, Problem, Change, ChangeTask, Approval, ExternalActivity (PUBLIC_COMMENT or INTERNAL_WORK_NOTE), MaintenanceWindow, AssignmentRef, ExternalPrincipalRef, BusinessServiceRef, CIRef, CIRelationshipRef, AttachmentRef, ExternalBinding and SyncConflict. Major Incident is normally an Incident classification/extension. Service Request may exist in adapter abstraction; service-catalog-initiated governed operations remain Phase 2.

Outbound delivery uses a PostgreSQL transactional outbox to connector workers; inbound delivery uses authenticated webhook/event ingress or bounded incremental polling into a PostgreSQL inbox before governed domain update. Required mechanics include deterministic operation identity, idempotency, connector/version binding, retry classification/backoff+jitter, rate limits, quarantine/DLQ, pause/drain/replay, tenant quotas, lineage, webhook replay protection and multi-signal echo-loop prevention. Kafka may distribute internal normalized events but is not workflow-delivery truth.

Phase-1 connector certification targets are **ServiceNow, Jira Service Management, BMC Helix and ManageEngine ServiceDesk Plus / ServiceDesk Plus MSP**. Phase-1.5 certification-priority targets are **Ivanti Neurons for ITSM, Freshservice/Freshservice MSP, SolarWinds Service Desk, TOPdesk, SysAid and HaloITSM**. This is AmrudAi certification priority, not a claimed global market ranking. Every capability claim is edition/API/version/auth/workflow specific.

### 58.6 CMDB authority and reconciliation

Support Phase-1 **Enrichment** (CMDB -> Platform) and bounded **Publication** (Platform -> CMDB), plus Phase-1 architecture for governed bidirectional reconciliation with deeper relationship certification in Phase 1.5. External CIs bind to canonical Platform entities; no second entity database is created. Last-write-wins and automatic entity deletion from missing external CIs are prohibited.

Observed reachability, telemetry, device configuration, routing, discovered technical topology/runtime relationships and RCA/evidence remain Platform technical truth. ITSM workflow/assignee/CAB state and normal business-owner/support-group/cost-centre/lifecycle metadata remain external authority by default; business criticality, declared service relationships, maintenance policy and selected tags/custom fields may use explicit configurable field/class authority. Conflicts remain visible/auditable.

ServiceNow CI publication/reconciliation uses IRE as the certified default unless a later vendor-specific certification proves a safer supported method. Direct generic CMDB table insertion is not the default architecture.

### 58.7 Integration Mapping Studio and Test Sandbox

Mapping Studio shall be declarative/versioned and support internal field, external object/field, direction, type, transform, enum map, default/required, authority, null behavior, conflict policy and write condition. Mappings are previewable, testable, auditable, promotable and rollback-capable. Arbitrary Python/JavaScript execution is prohibited.

The Integration Test Sandbox returns precise capability state: `SUPPORTED`, `SUPPORTED_WITH_LIMITATION`, `PERMISSION_MISSING`, `API_EDITION_UNAVAILABLE` or `UNSUPPORTED`, and may test authentication, permission, rate limit, CRUD/lifecycle transitions, comments/work notes, change/approval, inbound events, CMDB, relationships, mappings and idempotency.

### 58.8 Controlled specialist phasing

`REQ-NET-017` Endpoint & Switch-Port Intelligence is Phase 1.5 and uses the canonical graph for MAC/IP/user-when-available/AP/switch/port/VLAN attachment history, provenance/reconciliation and rogue/new endpoint evidence; no second endpoint database and no employee-surveillance model.

`PRD-EGC-013` / `REQ-EGC-012` browser journey synthetics are Phase 1.5; Playwright is only a candidate until OSS/runtime/security/capacity admission. `REQ-INF-007` gains Phase-1.5 query fingerprint/wait/blocking/deadlock/regression/plan-change evidence where safely supported, with no autonomous tuning. `REQ-INF-004` gains Phase-1.5 evidence-backed virtualization optimization recommendations with no autonomous VM/resource change. Deep specialist VoIP beyond existing operational monitoring is Phase 2.


### 58.9 Formal requirement ownership and downstream mapping

The following v3.3.0 requirement families are normative extensions of the product requirements above. `Platform_08` remains the single global primary registry; this table prevents the new IDs from becoming index-only/orphan requirements.

| Requirement family | Normative product scope | Required downstream owners |
|---|---|---|
| `REQ-EMC-023..026` | Official Certified Pack baseline, OOB acceptance, dimensional/version certification, second-wave roadmap | Platform_23 primary specialized owner; Platform_03/04 architecture/HLD, Platform_09 UX, Platform_14 quality, Platform_11 capacity where applicable |
| `REQ-RCA-012` extension | Investigation Workbench over Unified Operations Timeline | Platform_03/04/05/06, Platform_09, Platform_17, Platform_19, Platform_22 |
| `REQ-NET-016` extension | Path Intelligence over active-path + canonical temporal graph | Platform_03/04/05/06, Platform_09/10/14/17/22/23 |
| `REQ-OTEL-001..006` | Phase-1 OTLP metrics/logs/context and Phase-1.5 bounded sampled spans/runtime onboarding | Platform_03/04/05/06, Platform_09/10/11/14/16/17/18/19/20/22 |
| `REQ-NCM-006..012` | Change & Configuration Assurance, exact approval binding, canary/staging/postcheck/certified rollback, Phase-1.5 firmware | Platform_03/04/05/06, Platform_09/10/11/14/16/17/18/19 |
| `REQ-INT-006..013` | Platform-wide Integration Fabric, durable inbox/outbox, loop prevention, Mapping Studio/Test Sandbox and connector priorities | Platform_03/04/05/06, Platform_09/10/11/14/16/17/18/20/23 |
| `REQ-CMDB-001..007` | CMDB modes, bindings, authority/conflicts, graph relationships, IRE, lifecycle and scale | Platform_03/04/05/06, Platform_09/10/11/14/16/17/18/20/23 |
| `REQ-NET-017` | Phase-1.5 Endpoint & Switch-Port Intelligence | Platform_03/05/06, Platform_09/14/17/18/20/23 |
| `REQ-EGC-012` phase update | Phase-1.5 browser journey synthetics | Platform_21 primary specialized owner plus Platform_09/11/14/16/18; dependency remains P0-unselected |
| `REQ-INF-007` extension | Phase-1.5 Database Performance Intelligence | Platform_09/11/14/17/21/22/23 |
| `REQ-INF-004` extension | Phase-1.5 Virtualization Optimization | Platform_09/11/14/17/21/23 |

No row above implies implementation, benchmark, certification or deployment.


### 58.10 Formal v3.3 requirement anchors

The following exact IDs are owned by the v3.3 product requirements in §58. These anchors make RTM ownership explicit; they do not create duplicate product semantics.

| Requirement | Normative owner statement |
|---|---|
| `REQ-CMDB-001` | CMDB supports governed Enrichment, bounded Publication, and Phase-1 architecture for Reconciled Bidirectional mode without a second entity/CMDB database. |
| `REQ-INT-006` | The platform-wide canonical external-object model covers Incident, Problem, Change, ChangeTask, Approval, typed ExternalActivity, maintenance, assignment/principal, business service, CI/relationship, attachment, binding and conflict concepts. |
| `REQ-NCM-006` | NCM Change & Configuration Assurance follows Plan -> Backup -> Compare -> Preflight -> Assess -> Approve -> Execute -> Validate -> Observe -> Rollback/Escalate -> Audit. |
| `REQ-OTEL-001` | Phase-1 OTLP certification targets metrics and structured logs with resource identity and trace/span correlation identifiers/exemplars where supplied; retained distributed spans are not a Phase-1 GA requirement. |
| `REQ-CMDB-003` | Field/class authority and explicit conflict handling replace last-write-wins; external CMDB metadata cannot silently overwrite observed Platform technical truth. |
| `REQ-CMDB-005` | Certified ServiceNow CI publication/reconciliation uses IRE by default; generic direct CMDB table writes are not the default integration path. |
| `REQ-CMDB-006` | Missing/deleted external CIs do not automatically delete Platform entities; governed lifecycle/reconciliation and audit apply. |
| `REQ-INT-010` | Mapping Studio is declarative, versioned, previewable, testable, auditable, promotable and rollback-capable; arbitrary customer Python/JavaScript is prohibited. |
| `REQ-INT-011` | Integration Test Sandbox reports SUPPORTED, SUPPORTED_WITH_LIMITATION, PERMISSION_MISSING, API_EDITION_UNAVAILABLE or UNSUPPORTED based on real capability tests. |
| `REQ-INT-012` | Phase-1 connector certification targets are ServiceNow, Jira Service Management, BMC Helix and ManageEngine ServiceDesk Plus/MSP, with exact edition/API/version/auth/workflow certification. |
| `REQ-INT-013` | Phase-1.5 certification-priority targets are Ivanti, Freshservice/MSP, SolarWinds Service Desk, TOPdesk, SysAid and HaloITSM; this is not a global market-share ranking. |
| `REQ-NCM-008` | Pre-change assurance evaluates certified reachability/auth/privilege/current-config/fresh-backup and applicable HA/routing/path/incident/service/freeze/ITSM context. |
| `REQ-NCM-009` | Change risk is deterministic/explainable and graph-derived blast-radius evidence exposes confidence, freshness and uncertainty rather than AI certainty. |
| `REQ-NCM-010` | Canary/fixed staged rollout uses bounded concurrency/failure-domain sequencing and explicit stop conditions; unsafe failure stops future batches. |
| `REQ-NCM-011` | Post-change validation/stabilization compares configuration and operational evidence; rollback uses only certified safe vendor/device mechanisms, otherwise escalation is required. |
| `REQ-NCM-012` | Firmware lifecycle is Phase 1.5 and includes approved provenance/integrity/compatibility/storage/backup/approval/canary/staging/reboot/postcheck/ITSM/audit with certified rollback only. |
| `REQ-OTEL-003` | Phase-1 OTLP enforces request/decompression bounds, cardinality governance, sensitive-field policy, explicit reject/drop accounting and bounded backpressure; no-data is not service failure. |
| `REQ-OTEL-005` | Phase-1.5 may add bounded sampled spans, short quota-controlled retention, trace search/waterfall, trace-derived service edges and sampling after benchmark gates. |
| `REQ-OTEL-006` | Java/.NET/Node.js/Python instrumentation onboarding is a Phase-1.5 certification target; full developer APM/profiling/RUM/session replay remains excluded. |


## Issue #252 — notification, reporting and formula product reconciliation

Governing decision: [Issue #252](https://github.com/gagandsm-max/platform/issues/252), 21 October 2026. This supplement is SPECIFIED only. It preserves version **3.3.0**, the existing formal IDs, the 25-file Mother Reference set and the frozen Issue #200 Phase 1 / Phase 1.5 envelopes. It neither certifies implementation nor removes an existing release obligation.

The existing §27 customer-defined typed formula, Service Assurance and reporting requirements and §28 / §58.5 applicable ITSM targets remain open release gates. Unimplemented functionality is not automatically reassigned to Phase 1.1. Advanced visual formula editing and debounced live-preview UX are additive Phase 1.1 work. Expanded provider adapters, optional distribution targets and advanced scheduling may be Phase 1.1 extensions only where they add to the retained baseline. The 27 October integration checkpoint, 30 October engineering objective and conditional 1 November commercial target remain objectives, not an assured external promise.

Outbound email/integration configuration uses tenant-owned reusable profiles, explicit endpoint/TLS/auth policy and write-only secret references. Activation requires a successful test bound to the exact profile, secret revision, destination and provider policy; material changes invalidate the test. Event delivery and report distribution are typed separate operations over those profiles. Recipients, including external To/Cc/Bcc addresses, are governed by tenant/RBAC/resource/export policy; a role/group label or arbitrary address does not grant access. Ticket lifecycle uses canonical incident-generation bindings and certified mapping/recovery/reopen policy; external ticket state never overrides monitored technical truth.

Reports remain PDF/CSV only. Core-owned schedules use IANA timezone, a durable occurrence identity, version/generation fencing and explicit DST, month-end, missed-run, overlap, pause and restart semantics. Recheck authorization at execution, dispatch and download. Bound encoded message/provider size; large or private artifacts use an authenticated, expiring, revocable route. Automatic ZIP, unrestricted cron, S3/SFTP support and headless-browser deployment are not implied defaults.

The detailed formula/preview contract is owned by Issue #252 and extends the existing governed query/metric catalogue and approved CEL direction: typed checked versioned definitions, no arbitrary executable code/query text, bounded shared preview/production semantics and explicit type/unit/time/quality/cost behavior. No formula runtime is claimed here. Installed/enabled long-lived components and engineering tools must safely recover authorized desired state after shutdown/boot/reboot in every engineering and deployment phase; interactive OAuth and completed one-time operations are not automatically replayed.

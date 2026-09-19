# Platform — Product Requirements Document (PRD)

**Document ID:** PLATFORM-PRD-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference — Engineering Source of Truth
**Depends on:** `Platform_01_BRD.md`  
**Engineering companions:** Architecture, HLD, LLD and Dataflow documents in this set  

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

## 2. Product scope summary

Platform shall provide a unified enterprise operations experience covering:

1. Network monitoring and traffic analytics.
2. Infrastructure/server/hardware monitoring.
3. Virtualization and storage.
4. Database monitoring.
5. Cloud and Kubernetes.
6. OpenTelemetry metrics/logs.
7. Synthetic/service tests.
8. NCM and privileged CLI.
9. IPAM.
10. Discovery, provisioning and inventory.
11. Topology, maps and business-service relationships.
12. Events, alerts, incidents and qualification.
13. Graph-aware RCA, anomaly detection and explainable AI.
14. Reporting and SLA.
15. ITSM/notification integrations.
16. Multi-tenancy, RBAC, security and audit.
17. Sensor/Agent administration, HA, DR, backup/restore and upgrades.
18. Data retention, rollups, archive, capacity forecasting and storage protection.
19. Extensibility through MIB Studio, API Studio and Device Pack Studio.

DPI and full APM are excluded for the initial product scope.

---

## 3. Product information architecture

### 3.1 Primary workspaces

The primary navigation shall be role-aware and organized into these workspaces:

- Home
- Dashboards
- Inventory
- Provisioning & Discovery
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

## 4. Tenant and tenancy requirements

| ID | Requirement |
|---|---|
| PRD-TEN-001 | Platform shall support single-customer and MSP/multi-tenant operating modes. |
| PRD-TEN-002 | Tenant isolation shall be enforced in API, authorization, query and persistence layers, not only by hiding UI elements. |
| PRD-TEN-003 | MSP users may be scoped to one or multiple customers; customer users shall not see other customers. |
| PRD-TEN-004 | Tenants shall have independent policy, dashboards, retention, integrations, credentials, groups and reporting scopes where allowed. |
| PRD-TEN-005 | Overlapping IP address spaces across tenants shall be supported through tenant-aware canonical identity. |
| PRD-TEN-006 | Resource usage/quota controls shall prevent one tenant from exhausting shared query/report/AI/ingestion resources in multi-tenant deployments. |

---

## 5. Core installation and deployment requirements

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

DR capability is mandatory in v1.0. Product shall include:

- backup and restore workflows;
- DR state export/replication architecture;
- DR readiness validation;
- runbook and audit trail;
- test/verification capability.

Exact contractual RPO, RTO and automated/manual DR activation remain TBD and must be parameterized/configurable where architecture permits.

---

## 6. Sensor architecture product requirements

### 6.1 Universal runtime and roles

One common Universal Sensor Runtime/build shall support governed selectable roles/profiles:

- Network Sensor;
- Infrastructure Sensor;
- **Flow Monitoring Sensor**;
- **Log Monitoring Sensor**;
- Synthetic Probe;
- Cloud/API Sensor;
- Edge Sensor Lite deployment profile;
- Combined development/lab profile where explicitly permitted.

Formal production-role requirements:

| PRD ID | Requirement |
|---|---|
| `PRD-SENSOR-001` | Platform shall ship one common Sensor software/runtime and activate only the modules allowed by the selected governed Sensor role; dedicated Flow/Log requirements shall not create separate source trees, installers or independently drifting Sensor products. |
| `PRD-SENSOR-002` | A production `FLOW_SENSOR` shall dedicate its monitoring capacity to NetFlow/IPFIX/sFlow/J-Flow/vendor-compatible Flow ingestion, exporter/template/sampling state, Flow normalization, Flow durability/upload and Flow self-health. It shall not run SNMP polling, discovery, NCM, synthetic monitoring or log collection. |
| `PRD-SENSOR-003` | A production `LOG_SENSOR` shall dedicate its monitoring capacity to governed log/event ingestion such as Syslog/application/network logs and assigned OTLP Log workloads. It shall not run Flow collection, SNMP polling, discovery, NCM or synthetic monitoring. |
| `PRD-SENSOR-004` | Production Sensor role changes shall use a governed drain/reassignment/reconfiguration workflow that stops new work, flushes or accounts for durable spool state, validates no incompatible assignments remain, applies the new role and revalidates health before activation. Combined Flow+Log/high-rate operation is development/lab/test only unless later separately certified by change control. |

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

### 6.4 Dedicated Flow and Log Sensors

Production high-rate Flow and Log collection use distinct dedicated Sensor instances/roles.

**Flow Monitoring Sensor:** NetFlow v5/v9, IPFIX, sFlow and compatible J-Flow/vendor formats; exporter/template/sampling/sequence state; canonical Flow normalization; bounded local Flow spool; Flow upload and self-health.

**Log Monitoring Sensor:** Syslog/application/network logs, assigned log/event receivers and high-rate OTLP Logs according to the governing log/OTel authorities; parser/normalization pipeline; bounded local Log spool; upload/forwarding and self-health.

Network SNMP polling/traps/informs remain owned by the Network Sensor unless a later explicit certified event-role design changes that boundary. Flow and Log Sensors retain common Sensor control identity, mTLS, configuration, health, upgrade and role-specific spool/uploader functionality only.

Production role isolation is mandatory even for small deployments; saving a VM does not justify allowing Flow/log bursts to starve routine monitoring. A combined role may exist only for development/lab/test and is not production-certified.

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

---

## 7. Sensor placement and capacity

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

## 9. Provisioning & Discovery workspace

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
- GET/GETNEXT/GETBULK/WALK/SET where authorized;
- tables/scalars/counters/gauges/enums;
- traps/informs;
- custom MIB upload and dependency resolution;
- customer-defined polling profiles;
- adaptive polling;
- missing-data detection;
- polling statistics and diagnostics.

### 11.2 Availability and path

- ICMP reachability/RTT/loss.
- Configurable device-down validation.
- Traceroute/path history and changes.
- TCP/DNS/TLS/HTTP validation where relevant.
- Multi-vantage validation from secondary Sensors where policy allows.

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

Platform shall ingest NetFlow v5/v9, IPFIX, sFlow and compatible vendor flow formats through dedicated Flow Monitoring Sensors and shall preserve protocol/source semantics before enrichment.

Core product requirements:

| PRD ID | Requirement |
|---|---|
| `PRD-FLOW-001` | Platform shall identify each configured Flow exporter using tenant-scoped canonical exporter identity plus listener/Sensor/source/protocol and protocol-appropriate observation-domain/engine context; UDP source IP alone shall not be treated as durable exporter identity. |
| `PRD-FLOW-002` | Flow source admission shall use configured tenant/listener/exporter mapping, source policy and bounded rate/resource controls. Unknown/unmapped exporters shall be rejected or quarantined according to policy and shall never self-assign tenant authority from packet contents. |
| `PRD-FLOW-003` | NetFlow v9/IPFIX template state shall be exporter/observation-domain scoped and support template arrival/update/expiry/withdrawal/reuse, exporter restart/generation, options templates, enterprise information elements, variable-length fields and explicit missing/invalid-template quality. |
| `PRD-FLOW-004` | Sensor restart may reuse a bounded persisted template cache only when exporter/generation validity is still defensible; stale template reuse after restart/reset shall be invalidated rather than silently decoding records with obsolete semantics. |
| `PRD-FLOW-005` | Sampling metadata shall be first-class. Platform shall distinguish observed from estimated packets/bytes, retain algorithm/rate/source/provenance, represent unknown sampling explicitly and preserve sampling-rate changes over time. Estimated traffic shall never be represented as exact unsampled measurement. |
| `PRD-FLOW-006` | Canonical Flow time shall distinguish flow start/end where protocol evidence exists, exporter/export time, Sensor observed time and Core ingest time with timestamp quality; protocols that do not provide authoritative start/end shall not receive fabricated values. |
| `PRD-FLOW-007` | Protocol-aware sequence/state tracking shall distinguish sequence gaps, duplicates, replay, out-of-order data and exporter reset according to NetFlow/IPFIX/sFlow semantics rather than applying one generic sequence algorithm. |
| `PRD-FLOW-008` | Canonical Flow records shall be IPv4/IPv6 capable and preserve exporter/observation point, five-tuple where available, bytes/packets, sampling, raw ingress/egress interface indexes, canonical interface references, VLAN/VRF, ASN, next-hop/BGP context, QoS/DSCP, NAT fields when exported, application metadata/provenance, template/sequence and quality. Unsupported/missing fields remain null/unknown rather than invented. |
| `PRD-FLOW-009` | Common operational fields shall use strongly typed canonical columns/contracts; vendor/enterprise information elements use bounded controlled extension attributes or Capability Pack mappings rather than unbounded dynamic database columns. |
| `PRD-FLOW-010` | Before Platform durable local acceptance, UDP Flow export is best-effort. After certified local durable acceptance, normalized Flow records participate in the common Sensor replay/idempotency/durability contract with stable Platform ingestion identity and explicit terminal loss/gap accounting. |
| `PRD-FLOW-011` | Flow Data Quality shall expose source/exporter freshness, template status/misses, sequence gaps, sampling certainty, clock/timestamp quality, malformed/unsupported records, socket/queue drops, rate limits, spool/storage pressure and downstream pipeline state. `No Flow Data` shall not imply `No Network Traffic`. |
| `PRD-FLOW-012` | Flow enrichment shall resolve source/destination IPs to canonical entities, endpoint/system name/FQDN/DNS, subnet/site/region, interface/VRF/VLAN, ASN/BGP, cloud/SaaS/application and approved identity context only where evidence exists; every material enrichment shall retain source, effective/observed time, version and confidence/provenance. |
| `PRD-FLOW-013` | User attribution shall be temporal and evidence-based. AD/LDAP/Entra directory data may enrich identities but shall not by itself prove which user controlled an IP at a historical time; attribution may consume Agent login/session, AD/auth events, RADIUS/802.1X, VPN, WLAN/NAC, firewall/proxy identity or equivalent evidence with validity interval, source and confidence. Conflicts remain explicit. |
| `PRD-FLOW-014` | Domain/FQDN/application and URL are distinct concepts. Platform may correlate DNS, exporter application metadata, proxy/SWG/firewall/service logs or other approved evidence to domains/applications; it shall not fabricate a full URL from an IP/domain. Full URL/path/query capture is outside basic Flow semantics and requires an explicit governed source/privacy policy. |
| `PRD-FLOW-015` | Internal geography shall prefer authoritative Platform site/subnet/entity mappings. Public-IP geolocation shall be represented as approximate provider/database evidence with accuracy/confidence/provenance and shall not assert a person's precise physical location. |
| `PRD-FLOW-016` | Historical Flow enrichment shall remain temporally correct when IP ownership, hostname, user session, site, ASN, cloud catalogue, DNS or application mappings change; current enrichment state shall not silently rewrite historical identity. |
| `PRD-FLOW-017` | Raw Flow observations remain directional and observation-point specific. Replayed records from the same source may be deduplicated by stable source/ingestion identity, while the same traffic observed at different exporters/interfaces remains distinct valid evidence and shall not be globally deduplicated by five-tuple alone. |
| `PRD-FLOW-018` | Bidirectional Conversation, site-pair, application and traffic-matrix objects are derived analytical views over raw directional Flow observations and preserve links/provenance back to underlying observation points. |
| `PRD-FLOW-019` | NAT, QoS/DSCP/ECN, TCP flags/ICMP metadata, exporter-supplied application classification and vendor enterprise fields shall retain explicit source/provenance and shall not be synthesized when absent. Application identity shall distinguish exporter-classified, catalogue/pack-mapped, port-inferred and unknown sources. |
| `PRD-FLOW-020` | Platform shall correlate Flow with canonical topology, interface, VRF, routing/BGP, site/circuit and change evidence where available, while preserving that Flow correlation is observation evidence rather than proof of the actual routed path or cause. |
| `PRD-FLOW-021` | Platform shall support a Flow Coverage/consistency signal by comparing normalized Flow-estimated traffic with compatible SNMP/interface counter evidence where semantics/observation point/sampling permit. Coverage states shall expose `GOOD|PARTIAL|LOW|UNKNOWN` or equivalent rather than blindly presenting a misleading exact ratio. |
| `PRD-FLOW-022` | Flow UX shall include a bounded Flow Explorer plus exporter/source health and template diagnostics. Operators shall be able to investigate an interface/link/time range directly into top conversations, sources, destinations, applications, protocols, sites and QoS without losing context. |
| `PRD-FLOW-023` | Flow retention shall distinguish raw normalized Flow, shorter-window detailed aggregates, longer-lived conversation/site/interface/application rollups and incident-protected evidence. Interactive top-N/matrix/Sankey queries shall use bounded ranges/cardinality and suitable rollups rather than unrestricted scans. |
| `PRD-FLOW-024` | Operational Flow anomaly classes may include elephant-flow emergence, top-talker surge, new dominant destination, site-to-site shift, protocol/application-mix shift, fan-in/fan-out change, unexpected interface traffic shift and QoS-class change. Anomaly alone shall not assert device/link failure or causal root cause. |
| `PRD-FLOW-025` | Flow collection shall enforce bounded socket/packet queues, decoder workers, exporter/template/domain cardinality, records/datagram, field/value size, per-exporter fairness/quotas and global Sensor ceilings. One exporter or template/cardinality storm shall not starve other exporters or another Platform Sensor role. |
| `PRD-FLOW-026` | Flow production certification shall measure datagrams/sec, records/sec, bytes/sec, exporter count, template rate/churn, records/datagram, fields/record, sampling mix, IPv4/IPv6, enrichment cost, socket/queue loss, spool growth/replay drain, Core/Kafka/ClickHouse load and bounded analytics/query concurrency. No universal records/sec guarantee is fixed without benchmark evidence. |
| `PRD-FLOW-027` | Flow-source resilience/redundancy shall be certified per exporter protocol/capability (multiple collectors, duplicate export, single collector, VIP/failover or other supported mechanism). Platform shall not assume a generic load balancer is safe for all template/source semantics. |
| `PRD-FLOW-028` | Future cloud-native flow-log inputs such as AWS/Azure/GCP flow logs shall normalize into the same high-level Flow identity/quality/enrichment model where semantics are compatible rather than creating a second NTA product. They are not implied as implemented by this requirement. |
| `PRD-FLOW-029` | Flow-derived user identity and browsing/application context shall be least-data, RBAC/resource scoped and auditable. Platform shall support operational troubleshooting without turning Flow into employee-surveillance or unrestricted browsing-history collection. |
| `PRD-FLOW-030` | Flow absence, exporter/template/Sensor failure, sampling uncertainty or Flow anomaly shall feed telemetry-quality/Alert Evidence semantics and shall not by itself assert `NO_TRAFFIC`, `LINK_DOWN`, `DEVICE_DOWN` or causal incident state. |

Views shall include Overview, Conversations, Sources, Destinations, Applications, Protocols/Ports, Sites, Interfaces, ASN, QoS, Traffic Matrix, Sankey, Exporters, Data Quality, anomalies and capacity. Platform shall not claim deep L7 application or full-URL visibility without explicit exporter/source metadata or a separately governed source; full always-on DPI remains excluded.

---

## 12. Infrastructure, hardware, virtualization and storage requirements

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

## 14. Cloud, Kubernetes and OpenTelemetry requirements

### 14.1 Cloud

AWS, Azure and GCP integrations shall use supported provider APIs/SDKs and normalize accounts/subscriptions/projects, regions, networks, compute, storage, database and load-balancer entities.

### 14.2 Kubernetes

Hierarchy shall include:

Cluster → Node → Namespace → Workload → Pod → Container → Service → Ingress.

Monitor node/pod/container health, CPU/memory, restarts, pending/failed state, events and capacity.

### 14.3 OpenTelemetry

Platform shall accept OTLP metrics and logs initially. Architecture shall be trace-ready, but full APM shall not be marketed until trace storage, analysis and UX are explicitly implemented and accepted.

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

## 16. MIB Studio

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

## 17. API Studio

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

## 18. Device Pack Studio

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

## 19. Canonical entity and graph product requirements

The same physical/logical resource shall not become duplicate entities merely because it is observed through SNMP, flow, syslog, NCM, API or Agent.

Entity families include tenant, region, site, network device, chassis/module/interface, VRF/VLAN, route/peer, server/OS/NIC/disk/process/service, hypervisor/VM, storage, DB, Kubernetes, cloud and business service entities.

Relationship examples:

`CONNECTED_TO`, `DEPENDS_ON`, `ROUTES_VIA`, `MEMBER_OF`, `CONTAINS`, `RUNS_ON`, `HOSTED_ON`, `MANAGED_BY`, `ADVERTISES`, `PEERS_WITH`, `USES`, `SERVES`, `BACKED_BY`, `ASSIGNED_TO`, `IMPACTS`, `CAUSED_BY`.

Every relationship shall record source/provenance, last verified time and confidence/quality where applicable. User-defined logical relationships must be distinguishable from automatically discovered physical/network relationships.

Historical topology shall support viewing the network/service graph as it existed during an incident time window.

---

## 20. Grouping, hierarchy and ownership

Platform shall support concurrently:

- tags/attributes;
- hierarchical groups;
- dynamic predicate groups;
- business-service groupings.

An entity may belong to multiple dimensions simultaneously.

Groups shall drive dashboards, policies, polling, alerts, reports, RBAC, Sensor assignment, maintenance, SLA, maps, escalation and RCA scope.

Entities may store business owner, technical owner, support team, escalation group, provider/vendor, cost center, criticality and service ownership.

---

## 21. Circuit inventory

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

## 22. Monitoring and policy hierarchy

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

## 23. Threshold and baseline engine

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

## 24. Adaptive polling

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

## 25. Event, alert and incident model

Platform shall distinguish:

- **Event** — raw observation.
- **Alert** — qualified actionable condition.
- **Incident** — correlated operational problem grouping one or more alerts/events.

Underlying events/child alerts shall remain inspectable even when incident correlation suppresses repetitive notifications.

Alert/incident lifecycle shall support New, Acknowledged, Assigned, Investigating, Monitoring/Recovery, Resolved and Closed states as appropriate.

Ownership, team assignment, notes, escalation, SLA timer and handoff shall be available.

---

## 26. Alert Qualification Engine

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

## 27. Alert storm, deduplication and flapping protection

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

## 28. Maintenance and suppression

Platform shall distinguish:

- notification mute;
- maintenance suppression;
- dependency suppression.

Maintenance can be manual, scheduled or recurring and scoped to tenant/site/group/device/interface/service/alert/metric as appropriate, with reason, ticket/change reference, approver, auto-expiry and audit.

ITSM change/maintenance windows may synchronize into Platform when configured.

---

## 29. Graph intelligence, GNN and RCA requirements

### 29.1 Architecture principle

Platform RCA is a **hybrid evidence-based graph reasoning system**. It shall not be an LLM-only system and shall not treat GNN output as proof of causality.

### 29.2 Evidence inputs

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

### 29.3 GNN learning incorporated from graph-first products

Platform shall maintain a temporal multi-layer operational graph suitable for learned graph features. A GNN/graph-learning component may:

- learn propagation signatures;
- rank likely root entities;
- score correlated edge relevance;
- assist blast-radius prediction;
- identify graph-structural anomalies;
- compare current graph state to learned normal relationships;
- prioritize additional evidence collection.

GNN results must remain subordinate to evidence/protocol constraints. Model output must include version, score/confidence and traceability to the graph snapshot/evidence window used.

### 29.4 RCA output

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

### 29.5 Deterministic/learned separation

Protocol-aware rules and canonical relationships provide deterministic constraints. Statistical/ML/GNN components provide learned scoring or anomaly evidence. The system shall never market “GNN = deterministic truth.”

### 29.6 RCA quality benchmark

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

## 30. AI Gateway and conversational operations

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

## 31. Unified Operations Timeline

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

## 32. Investigation and troubleshooting workspace

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

## 33. Path Analysis

User selects a source and destination/vantage. Platform correlates forward/reverse evidence where available, traceroute, latency/loss, interfaces, routing, VRF, SD-WAN/MPLS, Flow, topology and recent changes.

Path view shall identify uncertain/unobserved hops rather than inventing links.

---

## 34. NCM requirements

NCM is Day-1 important and shall support:

- credentialed SSH/NETCONF/API connections;
- configuration backup;
- version history;
- diff;
- change detection;
- compliance rules/golden configuration;
- templates;
- bulk push;
- scheduling/windows;
- maker-checker approval;
- RBAC;
- rollback where adapter/device supports it;
- reports;
- API;
- browser CLI integration;
- change audit;
- RCA correlation.

Semantic diff shall only be claimed where a Device Pack/parser supports it.

NCM worker concurrency/rate limits are independent from routine monitoring workers so NCM jobs cannot starve polling.

Bulk operations shall support dry-run/impact preview and canary/staged execution.

---

## 35. Privileged CLI Session Broker

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

## 36. IPAM requirements

IPAM shall support:

- IPv4 and IPv6 as first-class;
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

## 37. Reporting and SLA

Report Studio shall support:

- data source/metric/entity selection;
- filters/grouping;
- calculations/formulas;
- tables/charts/text/images;
- branding/layout;
- schedules/subscriptions;
- PDF/CSV/XLSX/JSON/API outputs;
- execution history/failures;
- RBAC.

SLA engine shall support customer-defined target, measurement window, business hours, maintenance exclusions, grace/qualification, partial/unknown state handling and drill-down to outage contributors.

Long-term SLA/report retention may differ from raw telemetry retention.

---

## 38. ITSM and notification integrations

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

Ticket creation shall include root cause/confidence, affected scope, correlated alerts, evidence, config changes and recommended actions.

Recovery policy options:

- resolve only in Platform;
- update ITSM but leave ticket open;
- auto-resolve/close selected incident types;
- resolved-monitoring-recovery state.

A durable outbound integration queue, retries, idempotency/correlation IDs and connector health dashboard are required.

An Integration Test Sandbox shall validate authentication, connection, field mapping, create/update/resolve flows before production enablement.

---

## 39. RBAC and authorization

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

## 40. Audit and governance

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

## 41. Data retention, rollup and archive

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

## 42. Storage Protection Engine

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

## 43. Legal/investigation hold

Authorized users shall be able to protect incident evidence for a defined scope/time range from normal retention/purge, including selected telemetry, Flow, logs, events, configuration, audit and RCA artifacts. Emergency capacity conflicts shall be surfaced explicitly; Platform must not silently violate protected retention policy.

---

## 44. Asset lifecycle/reconciliation

Lifecycle states shall include Discovered, Pending Approval, Provisioning, Active/Monitored, Monitoring Degraded, Unmanaged, Disabled, Maintenance, Failed Provisioning, Decommission Requested, Decommissioned and Archived as applicable.

Reconciliation detects disappeared devices, identity changes, IP/hostname moves, replacement/serial changes and stale licensed inventory. It shall never silently delete stale devices; they enter a review/decommission workflow.

---

## 45. Upgrade, canary and rollback

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

## 46. Certificate/PKI lifecycle

Administration shall show UI, Sensor, Agent, internal-service and integration certificates, issuer, expiry, days remaining, usage and status.

Support customer CA, CSR/import/renewal, trust stores and certificate rotation. Expiring certificates produce proactive alerts.

---

## 47. API-first requirement

All normal customer operations remain UI-first, but the UI shall consume supported control-plane APIs rather than bypassing them with database access.

API Management shall support service accounts, clients/tokens, scopes, rate limits, usage, failures, expiry/rotation and audit.

Outbound event/webhook subscriptions shall include relevant alert, incident, device, NCM, SLA and state-change events.

---

## 48. Query/cardinality protection

Platform shall protect itself from:

- unbounded dashboard/raw queries;
- high-cardinality custom metrics/labels;
- API connector explosions;
- excessive Flow/log rate;
- excessive report concurrency.

Controls include timeouts, maximum scan/rows, asynchronous large exports, cancellation, per-tenant/user concurrency, cardinality detection and recommendations to drop/normalize unstable labels.

---

## 49. Licensing behavior

- Warn 30 days before expiry.
- From 7 days before expiry, every dashboard shows a top banner: `Licence expires in X days`.
- Default grace: 7 days after expiry; polling continues.
- After grace, polling stops unless manually extended/renewed.
- Historical data remains accessible; no licence-expiry data deletion.
- Licence settings/import remain accessible to authorized admins.
- All state changes audited.

---

## 50. UX performance and accessibility

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

Exact numeric SLOs are benchmark-defined TBD, but common interactive actions must remain low-latency and regressions shall fail performance certification.

---

## 51. Dashboard Studio requirements

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

## 52. Universal dashboard controls

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

## 53. Out-of-box dashboard/workspace catalogue

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

### 53.1 Common dashboard behavior

All applicable dashboards shall inherit the Universal Dashboard Controls defined above. Drill-down must preserve tenant/site/device/time context. A row/entity click should support a quick side panel and full contextual page. Saved views may be Private, Team or Tenant-wide according to authorization.

### 53.2 Role landing pages

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

## 54. Topology & Map Studio

Topology navigation shall include Topology Explorer, Physical, L2, L3, Routing, MPLS, VXLAN/EVPN, SD-WAN, Infrastructure, Service Dependency, Traffic, Geo and Custom Maps.

Map Studio shall provide a palette of entities/sites/groups/services/cloud/links/shapes/labels/icons, canvas manipulation, relationship type, health rollup, visibility, layout and style controls.

Automatic layout options may include hierarchical, radial, force, tree, site and geographic. Manual relationships must declare relationship type/source and may not overwrite discovered truth silently.

---

## 55. Administration information architecture

### 55.1 Customers / Tenants

Customer list/create/profile, modules/licensing, deployment, users, Sensors, integrations, usage and audit. Customer users cannot browse other tenants.

### 55.2 Sites, groups and hierarchy

Sites, hierarchical groups, dynamic groups, hierarchies, tags and attributes. Sites may define preferred Network/Infrastructure/Synthetic Sensor pools and fallback pools.

### 55.3 Users & Access

Users, Roles, Groups, Permissions, Service Accounts, API Clients, Temporary Access, Approvals and Sessions.

User detail tabs: Profile, Roles, Scope, Groups, Authentication, Sessions, CLI Access, API Tokens, Audit.

Role Designer shall expose module/action/resource-scope/condition matrix and Access Simulator.

### 55.4 Authentication

Local Authentication, AD, LDAP, SAML, OIDC, Entra ID, MFA, Password Policy and Session Policy. Connections must be testable before enablement.

### 55.5 Credentials & Secrets

SNMP, SSH, WinRM, API, cloud, DB, virtualization, Redfish and generic credential types. Credential metadata includes name/type/scope/allowed Sensors/rotation/last verified/usage/audit.

### 55.6 Discovery

Jobs, ranges/seeds, credentials, schedule, exclusions, results and approval.

### 55.7 Monitoring Profiles

Profiles, Metrics, Polling, Adaptive Polling, Assignments, Inheritance, Thresholds, Validation, Alerts and Topology.

### 55.8 Alert policies

Policies, thresholds, baselines, composite rules, validation, escalation, suppression and dependencies.

### 55.9 Device Down Validation

Dedicated editor for retry counts, ICMP attempts, TCP ports, secondary Sensor, REST check, traceroute and decision expression/consensus.

### 55.10 Maintenance & Suppression

Active, Scheduled, Recurring, History plus create wizard for scope, type, schedule, alert/notification/ITSM effect, reason, ticket and approval.

### 55.11 Notifications

Email, Teams, Slack, approved WhatsApp, webhook, SMS/PagerDuty and custom adapters with routing policies.

### 55.12 ITSM

Connection, Authentication, Field Mapping, Ticket Policy, Recovery, Reopen, Synchronization, Queue, Health and Audit.

### 55.13 Sensors

Sensors, Pools, Assignments, HA, Capacity, Deployment, Versions and Updates.

Sensor Pool settings include role, region/site, capacity, HA mode, affinity and allowed device groups.

### 55.14 Agents

Agents, Policies, Versions, Updates, Edge Tests and Deployment Packages.

### 55.15 Data & Retention

Overview, Retention Policies, Rollups, Archive, Storage Forecast, Emergency Protection, Data Classes and Audit.

### 55.16 HA

Core pair, Sensor pools, state synchronization, failover history and health. Normal customers may have read-only visibility while Platform admins control changes.

### 55.17 AI

Provider, Gemini configuration, Data Policy, Tool Controls, Usage, Audit and Future Offline Provider configuration.

### 55.18 Licensing

Customer, licence type, modules, device/element consumption, expiry, support, HA entitlement and offline licence import/extension.

### 55.19 Security

Security Overview, MFA, Sessions, Certificates, API Tokens, Credential Policies, Encryption, Access Restrictions, Audit Retention, CLI Policies and Break Glass.

### 55.20 System / Platform Operations

Platform, Core Pair, Data Services, Kafka, Logs, Backup, Restore, Upgrade, Support Bundle, Time/NTP, SMTP, Proxy and Certificates.

---

## 56. Onboarding wizard

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

## 57. Deployment / environment readiness

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

## 58. Backup, restore and supportability

Platform shall expose backup schedules/status/restore points and restore validation.

Support Bundle generation shall collect relevant versions, service health, logs, Sensor/data diagnostics and configuration metadata while automatically masking credentials, tokens, private keys and defined sensitive data.

Every customer-facing technical error should carry a stable error code, customer explanation, technical context, suggested action and correlation/request ID.

---

## 59. Data sovereignty and future regional architecture

Data placement policies shall be architecturally possible so future regional Cores can retain raw telemetry in region and federate summaries upward. This is a future scalability extension and not an initial Master Core implementation requirement.

---

## 60. Non-functional requirements

### 60.1 Resource efficiency

- Core target up to 2,500 devices: 8 vCPU / 16 GB under certified Standard Network Workload Profile.
- Sensor current planning baseline: 4 vCPU / 16 GB / 150 GB for ~800 normal network devices.
- Sensor engineering benchmark target: 8 vCPU / 16 GB for ~2,000–2,500 normal devices where workload permits.
- Agent footprint target: TBD by OS/workload benchmark; must be explicitly measured and kept low.

### 60.2 UI/API performance

Exact SLOs TBD after baseline benchmark, but release testing must cover dashboard initial load, cached load, search suggestions, table filtering, alert acknowledgement, API latency and large-environment pagination.

### 60.3 Availability

- Non-HA installations have no HA availability claim.
- HA target: 99.95% service design objective subject to deployment assumptions and final SLA definition.
- Core failover certified target <60s.
- Sensor monitoring continuity through Core failover.

### 60.4 Scale

Architecture target range is 2,000–200,000 devices globally through horizontal Sensors and future multiple regional Cores. Exact certified scale per release must be benchmarked; architecture target is not a release claim.

### 60.5 Security

TLS 1.2 minimum, TLS 1.3 preferred/supported, TLS 1.0/1.1 disabled, secure cookies/HSTS, least privilege, mTLS internal trust boundaries where specified, secret encryption and audit.

### 60.6 Maintainability

Use modular Core architecture rather than excessive microservices to fit resource targets. All configuration changes versioned/auditable; no normal customer change requires source-code modification.

---

## 61. Reference Standard Network Workload Profile (SNWP-2500-v1)

This profile exists to make the 8 vCPU / 16 GB Core target objectively testable. It is a **benchmark profile**, not a restriction on customer configuration.

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

## 62. Product acceptance criteria summary

Before an initial enterprise release can be signed off, engineering/QA shall prove at minimum:

- installation/upgrade/rollback and separate Core/Sensor/Agent builds;
- UI-driven provisioning for single, bulk and discovery;
- actionable preflight diagnostics;
- central Sensor/Agent assignment and locality/capacity-aware placement;
- SNMP/flow/syslog/network protocol collection;
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
- HA failover <60s with Sensor continuity;
- DR architecture/runbook and backup/restore verification;
- HTTPS-only UI;
- SBOM/licence/security scans;
- certified resource/performance tests.

---

## 63. Explicit PRD TBD register

| ID | Open item |
|---|---|
| PRD-TBD-01 | Exact final brand/product name beyond working name Platform |
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

## 64. Additional mandatory operational capabilities captured from architecture workshops

The following capabilities are part of the agreed Platform scope and must not be lost simply because they were identified after the first dashboard catalogue was drafted.

### 64.1 Technology Lifecycle / EOS-EOL visibility

Where authoritative vendor/customer data is legally available, Platform shall track model, firmware/software version, support lifecycle metadata, EOS/EOL dates, support/warranty metadata and lifecycle risk. Lifecycle data must preserve source/provenance and must not fabricate unsupported vendor dates.

The product shall surface:

- unsupported firmware/software;
- hardware approaching EOL/EOS;
- models already out of support;
- inventory affected by lifecycle risk;
- recommended review/upgrade candidates.

This is operational asset intelligence, not a substitute for vulnerability management.

### 64.2 Desired State vs Actual State

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

### 64.3 Shift Handover

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

### 64.4 Migration Studio

To reduce adoption friction, Platform shall provide a Migration Studio capable of importing and reconciling inventory/configuration data from customer exports/APIs. Initial generic import must support CSV/XLSX/CMDB-style data; vendor-specific migration adapters may be added for SolarWinds, ManageEngine, Zabbix, PRTG, Nagios, LogicMonitor, NetBox and others where legal/API/export mechanisms permit.

Migration outcomes shall classify Imported, Automatically Mapped, Needs Review and Unsupported. Platform must not claim full translation of proprietary policies when semantic equivalence does not exist.

### 64.5 Knowledge Base / Runbook Library

Customers shall maintain approved runbooks, known issues, escalation procedures and troubleshooting guides, scoped by tenant/site/device type/service/incident class.

RCA and AI may recommend or summarize approved runbooks. Customer-approved source content remains authoritative. Runbooks must be versioned/audited and executable steps must not become autonomous actions unless future remediation policy explicitly allows it.

### 64.6 Post-Incident Review / Problem Management

Platform shall generate a draft PIR from incident timeline, RCA, impact, evidence, ITSM ticket, configuration changes, actions and recovery. Human operators add/approve final root cause, corrective action, preventive action and lessons learned. Output can be reported/exported.

### 64.7 Intelligence Quality / RCA Accuracy

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

### 64.8 Alert Policy Effectiveness

Platform shall identify noisy/low-value rules using evidence such as alert volume, incident conversion, acknowledgement/closure patterns, repeat rates and suppression. It may recommend review of a policy but does not silently change thresholds.

### 64.9 Monitoring Optimization Recommendations

Platform may recommend reducing unnecessary metric collection/polling when metrics are high-cost but unused by dashboards, alerts, reports or RCA over a configurable analysis period. It may also recommend Sensor redistribution. Recommendations require review/approval unless a safe automatic policy is explicitly enabled.

### 64.10 Change Freeze and Operational Calendars

Platform shall support business calendars, working hours, regional holidays, change freezes and blackout windows. NCM pushes, Platform/Sensor/Agent/Device Pack upgrades and other governed changes can be blocked or require emergency override/approval during freeze windows.

### 64.11 Device / Time Comparison

Operators shall be able to compare device A vs device B, or one entity/time window vs another, aligning normalized metrics, configuration, firmware, monitoring profile, topology, routing, errors and alerts where compatible. Missing/incompatible fields are marked rather than fabricated.

### 64.12 Data Pipeline Integrity

Platform Operations shall expose produced, accepted, consumed, inserted, rejected, duplicate, late, schema-error and dropped telemetry counts. The purpose is to detect silent data loss between Sensor, ingestion, Kafka, processing and ClickHouse.

### 64.13 Scheduled Housekeeping

A Housekeeping workspace shall show status/history/failures for rollups, TTL deletion, emergency purge, database compaction/maintenance, backups, archives, NCM backups, discovery jobs, report jobs and Sensor redistribution. Internal lifecycle work must not be invisible.

### 64.14 Usage and Licensing Forecast

Authorized administrators shall see licensed vs used entities/elements/modules and forecast approaching entitlement thresholds/expiry. This does not replace the agreed expiry/grace behavior.

### 64.15 Contextual Help

Complex settings and diagnostics shall expose `What is this?`, contextual documentation and, where allowed, AI explanation grounded in current Platform documentation/configuration. Help must never modify configuration without a separate explicit action.

---

## 65. Additional out-of-box dashboards/workspaces

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


## PRD-OSS — Mandatory implementation basis by product module

The following tables are product requirements because they prevent engineering teams from re-inventing commodity protocols or accidentally embedding unsuitable third-party products. Exact versions are controlled by `Platform_07_OSS_Catalog.md`.

### Network and Sensor module implementation basis

| Feature | Required OSS basis | Product requirement / custom layer |
|---|---|---|
| SNMP polling/traps | GoSNMP | customer credential profiles, scheduling, retries, normalization, qualification, MIB/Device Pack semantics |
| MIB parsing | gosmi | MIB Studio, import/dependency validation, OID tree, test-device workflow, provenance, reusable Device Pack generation |
| NetFlow/IPFIX/sFlow | GoFlow2 | enrichment with site/device/interface/IPAM/ASN/VRF/QoS, cardinality controls, ClickHouse analytics and RCA correlation |
| ICMP/traceroute | x/net/icmp + Platform path engine | RTT/loss/jitter, hop/path changes, multi-vantage comparison and Path Analysis |
| DNS | miekg/dns where advanced protocol control required | DNS tests, validation, SLA and failure classification |
| SSH | x/crypto/ssh; scrapligo where suitable | CLI Session Broker, Device Pack prompt/mode behavior, command policy and audit |
| NETCONF | scrapligo/approved NETCONF library | normalized configuration/state collection and Device Pack adapters |
| gNMI | OpenConfig gNMI | subscription profiles, capacity control and normalized metrics |
| YANG | goyang/ygot | model parsing/validation and Device Pack mappings |
| REST/OpenAPI | Go HTTP + kin-openapi | API Studio authentication, endpoint/schema discovery, mappings, chained calls and generated monitors |
| Syslog/log ingestion | Platform receiver + optional Fluent Bit integration | tenant/entity normalization, cardinality, retention and alert/RCA semantics |
| WinRM | masterzen/winrm | Windows agentless monitoring policy and diagnostics |
| Redfish/Swordfish | Gofish | unified hardware lifecycle entities/alerts |
| VMware | govmomi | vCenter/cluster/host/VM/datastore/network graph and monitoring |
| Kubernetes | client-go | cluster/node/namespace/workload/pod/container/service graph |
| OpenTelemetry | OTel Collector/libraries | OTLP metrics/logs ingest; no full APM claim initially |

### NCM implementation basis

| Capability | OSS basis | Product-owned requirement |
|---|---|---|
| Connection/execution | scrapli / scrapligo | tenant/RBAC/credential/Device Pack safety |
| Parallel tasks | Nornir | Platform job orchestration, rate/concurrency/change windows |
| Load/diff/commit | scrapli_cfg | approval, rollback, audit and RCA evidence |
| Vendor abstraction | NAPALM only when approved by transitive license scan | Platform remains independent of NAPALM driver coverage |
| Parsing | NTC Templates/TextFSM + Platform certified parsers | compatibility/provenance and Device Pack certification |
| Templates | Jinja2 | maker-checker, variable validation, secret controls and job history |

### UI implementation basis

| Capability | OSS basis | Product-owned requirement |
|---|---|---|
| Application | React/TypeScript | complete Platform navigation, UX and RBAC behavior |
| Charts | Apache ECharts | native Platform panel library and dashboard schema |
| Tables | TanStack Table + virtualization | server-side query/filter/page semantics at 200K-device scale |
| Dashboard layout | react-grid-layout | folders/groups/permissions/variables/versioning |
| Topology/causal graph | Cytoscape.js | canonical relationship model, provenance/confidence and temporal views |
| Geo maps | MapLibre GL JS | site/region health views; tile/data licensing handled separately |
| CLI terminal | xterm.js | privileged broker, command controls and audit |
| Query state | TanStack Query | client caching only; Core remains authority |

### Core/data/AI implementation basis

| Capability | OSS basis | Product-owned requirement |
|---|---|---|
| Core | Java 21 + Spring Boot | modular product domain/control plane |
| AuthN | Keycloak | resource-scoped authorization and tenant isolation remain Platform-owned |
| Expressions | CEL | safe variables/functions/cost limits, inheritance and simulation |
| Messaging | Kafka + Apicurio + Avro/Protobuf | topic/schema governance, replay/idempotency and processing semantics |
| Control state | PostgreSQL | authoritative schemas and migrations |
| Telemetry | ClickHouse | table design, TTL, rollup, query planner, 80% storage protection |
| Cache | Valkey | only reconstructable/ephemeral state |
| Reports | PDFBox/POI/Commons CSV | Report Studio/SLA semantics |
| Analytics | Polars/NumPy/scikit-learn | baseline/anomaly/forecast workflows |
| GNN | PyTorch + PyG candidate after P0 benchmark | graph feature engineering, labeled incidents, scoring and causal guardrails |

## PRD-PRV-PLUS — Provisioning completeness requirements

1. Manual, bulk and discovery provisioning use one common validation pipeline; none may bypass preflight checks.
2. User may enter `Display Name` and `Description` for network, infrastructure, API or any other entity. If Display Name is blank, resolution order is customer alias/device alias → discovered hostname/sysName/API name → DNS/FQDN → IP/address fallback. Customer-entered name is never silently overwritten by later discovery.
3. Sensitive values are never silently trimmed; non-secret textual fields may be normalized with a visible correction record.
4. The diagnostic result differentiates unreachable path, protocol timeout, authentication failure, authorization failure, likely ACL/firewall blockage, protocol-not-enabled suspicion, TLS failure, unsupported profile, duplicate identity, no eligible Sensor and capacity exceeded.
5. Every diagnostic result contains evidence and recommended next action. AI may explain evidence but may not invent certainty.
6. Sensor selection is tested from the Sensor that will actually perform monitoring.
7. Bulk import provides interactive staging and `Fix & Retry`, not repeated spreadsheet rework.
8. Device discovery never immediately consumes licensing/starts polling without the customer's configured auto-approval policy or review queue.
9. Capacity simulation runs before bulk activation and can block unsafe provisioning.

## PRD-RCA-GNN — Graph/GNN/AI requirements

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

## PRD-STORAGE — Retention, rollup and protection requirements

Administrators configure per tenant/data type/scope:

- raw retention;
- each rollup resolution and rollup retention;
- archive action/destination/retention;
- legal-hold behavior;
- emergency purge priority within protected-data constraints.

Rollup semantics must match metric type: gauges preserve min/max/avg/count/percentiles as configured; counters use delta/rate/total as appropriate; states preserve availability, duration, transitions and last state. Historical queries automatically select the appropriate resolution and disclose the resolution used.

Core and Sensors implement non-disableable 80% managed-storage safety protection. Cleanup targets safe headroom rather than merely 79.9%. Purge actions are audited and visible. Monitoring/collection remains the priority.

## PRD-LICENSE — Expiry behavior

- T-30 days: warning begins.
- T-7 days: persistent dashboard banner with exact days remaining.
- Expiry through +7 days: grace; polling continues.
- End of +7-day grace: polling stops unless manually extended/renewed; historical data remains available.
- License/entitlement events are audited and visible in Usage & Licensing dashboard.

## PRD-CROSS — Additional must-have product workspaces/capabilities

The product shall include the following even where they are implemented as contextual workspaces rather than top-level navigation items:

Asset Reconciliation; Technology Lifecycle Risk; Backup & Restore; DR; Upgrade & Rollback; Desired vs Actual State; Change Impact/Dry Run; Path Analysis; Device/Time Compare; Baseline & Seasonality; Alert Storm/Flapping; Shift Handover; Circuit Inventory; Operational Timeline; Support Bundle; Platform Health; Licensing/Usage; Storage Forecast; AI Boundary/Feedback; Device Pack Certification/Test Lab; Migration Studio; Environment Readiness; Change Freeze/Calendars; Runbook Knowledge Base; Post-Incident Review; Intelligence Quality; Alert Policy Effectiveness; Monitoring Optimization; Sensor Placement Optimization; Certificate/PKI; Proxy; Data Sovereignty; API Governance; Query Protection; Housekeeping; Data Pipeline Integrity.


## Mother-reference cross-document contract (v3.1)

The following supporting mother references are normative and must be read together with this document:

- `Platform_07_OSS_Catalog.md` — approved/candidate OSS by module, license class, usage, rationale and restrictions.
- `Platform_08_Requirements_Traceability.md` — completeness control; a missing mapped requirement is a documentation defect.
- `Platform_09_UI_UX_Master.md` — detailed global shell, Studios, 98 out-of-box dashboard/workspace screen contracts, filters/actions/states/RBAC behavior.
- `Platform_10_Protocol_Security_Connector_Master.md` — protocol/security/authentication/credential/connectivity matrix and provisioning diagnostics.
- `Platform_11_Sizing_Capacity_Storage_Master.md` — Core/Sensor compute targets, MWU, placement, retention/storage and 80% protection.
- `Platform_12_Decision_Source_Register.md` — USER-FIXED vs ARCH-DELEGATED vs REFERENCE-LEARNING vs TBD/P0 decisions.

**OSS rule:** implementation teams may not add a runtime/build dependency merely because it is open source. It must be admitted to the Platform OSS Catalogue by exact version, transitive license/CVE scan and release policy.

## 66. Development/runtime packaging and service-management product requirements — v3.1

### PRD-DEV-001 — Container-isolated engineering environment

All normal development and automated test stacks shall run under Docker Compose. The repository shall provide a one-command startup path, health-aware startup, deterministic teardown and documented reset/recovery. Project services and databases shall not depend on host-native project runtimes.

### PRD-DEV-002 — Reboot-safe developer state

The development environment shall distinguish durable named volumes from disposable test volumes. Normal laptop shutdown/reboot must not corrupt Platform state. After Docker daemon restart, services configured for development auto-resume must restart according to Compose policy and re-establish health without manual database repair.

### PRD-PKG-001 — Native production packages

Core, Sensor and Linux Agent shall ship as both `.deb` and `.rpm` packages for certified Ubuntu/RHEL versions. Windows Agent shall ship as a code-signed service executable and GA MSI, with install/uninstall BAT wrappers. Docker images are engineering/test artifacts and optional future deployment artifacts; Docker is not a mandatory customer runtime for the initial native installation model.

### PRD-SVC-001 — Auto-start and recovery

- Linux Core, Sensor and Agent services shall be enabled at install time for automatic boot startup.
- Windows Agent shall be installed as an automatically starting Windows Service.
- Services shall expose explicit states: STARTING, HEALTHY, DEGRADED, STOPPING, STOPPED, FAILED, RECOVERING.
- Repeated crash loops shall be surfaced as Platform/system alerts rather than being hidden by infinite restart.

### PRD-LOG-001 — Structured system/application diagnostics

Each component shall emit structured logs containing at minimum timestamp, severity, component, version, node/sensor/agent identity, tenant where relevant, event/error code, correlation/request ID where relevant and safe contextual attributes. Passwords, communities, private keys, tokens and authorization headers shall never be written in clear text.

Required log classes include startup, configuration load, registration/enrollment, connectivity, storage, database/message-bus interaction, polling/collection, provisioning, protocol errors, security events, HA/failover, upgrade/rollback, shutdown, crash/restart and recovery.

### PRD-LOG-002 — Supportability

The Administration and Platform Health workspaces shall expose service health, recent failures, restart count, last successful recovery and links to sanitized diagnostics/support-bundle generation according to RBAC.

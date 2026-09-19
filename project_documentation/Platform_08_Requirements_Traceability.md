
# Platform — Requirements Traceability & Completeness Matrix

**Document ID:** PLATFORM-RTM-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Completeness Control**
**Last reconciliation:** 2026-09-19 — v3.2.2 NOC troubleshooting integration-audit closure

## 1. Purpose

This matrix is the formal control against requirements being lost as documentation is split. It consolidates the requirements agreed across the Platform design conversations and maps each requirement family to the mother-reference documents that must implement it.

Every requirement is expected to appear in the BRD/PRD at the appropriate level and, where technical, in Architecture/HLD/LLD/Dataflow. A missing implementation mapping is a documentation defect, not permission to ignore the requirement.

## 2. Requirement catalogue

### Product principles

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-P-001 | Excellent production-grade product quality | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-P-002 | Ease of use; normal customer administration through UI | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-P-003 | Very low false-alert objective with evidence-based qualification | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-P-004 | Meaningful AI; no decorative AI | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-P-005 | Friendly role-aware UI/UX and readable fonts | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-P-006 | Easy implementation/onboarding | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-P-007 | Low CPU/RAM/storage footprint | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-P-008 | Low-latency fast UI/API | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-P-009 | HTTPS/TLS-only UI; no TCP/80 application listener | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-P-010 | No Silent Failure | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-P-011 | Safe by default, flexible by choice | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Core/HA/DR

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-A-001 | Standalone = one Core; HA = exactly two full Core nodes | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-A-002 | One Active Core and one Standby; continuous state synchronization | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-A-003 | Core HA failover under 60 seconds in certified scenario | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-A-004 | Sensors continue polling during Core failover | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-A-005 | Core is brain; Sensors/Agents enroll through Core | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-A-006 | All Core components run on Core nodes for standard deployments | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-A-007 | Sensor-to-Core HTTPS/gRPC mTLS ingestion; Kafka internal | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-A-008 | DR architecture mandatory in v1.0; exact RPO/RTO TBD until business freeze | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-A-009 | Ubuntu and RHEL Core/Sensor; no Windows Core/Sensor initially | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-A-010 | Separate signed Core, Sensor, Windows Agent, Linux Agent builds | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-A-011 | VM/bare metal Linux first; Kubernetes not mandatory | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-A-012 | Future regional/multi-Core + Master Core architecture is future scalability extension only | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Sensor/Agent

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-S-001 | Universal Sensor Runtime with Network, Infrastructure, Combined, Streaming/Event, Synthetic and Cloud/API roles | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-002 | Sensor locality-aware placement close to devices | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-003 | Capacity-aware placement; never overload Sensor for lower RTT | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-004 | Customer modes: Auto, Auto-with-approval, Manual, Sensor Pool | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-005 | Sensor pools support N+1/N+2/N+K/1:1/active-active capacity models | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-006 | Distributed leases/fencing prevent duplicate polling | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-007 | Local durable spool/store-and-forward | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-008 | Sensor 80% disk protection and bounded junk/log cleanup | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-009 | Sensor self-observability: CPU/RAM/disk/queue/poll latency/backlog/failures | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-010 | Initial planning baseline 4 vCPU/16GB/150GB ~800 normal network devices | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-011 | Benchmark 8 vCPU/16GB high-density target ~2,000–2,500 normal network devices | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-012 | Agentless infrastructure target ~800 endpoints per 4C/16G Sensor | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-013 | Agent-push target ~2,200 endpoints per 4C/16G ingestion capacity unit | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-014 | Windows/Linux Agent supports host metrics and edge network tests | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-015 | Branch→DC and DC→branch test support for asymmetric-path evidence | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-S-016 | Agent enrolls with Core and Core assigns correct available Sensor | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Provisioning/Discovery

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-PRV-001 | UI-only customer provisioning: individual, CSV/XLSX bulk and auto discovery | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-002 | Optional user display name and description; auto-derive when blank | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-003 | Do not use IP alone as canonical identity | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-004 | Credential Profiles; avoid secrets in CSV | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-005 | CSV/XLSX staging, normalization, duplicate/error/warning classification | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-006 | Sensitive-field whitespace never silently modified | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-007 | Preflight Diagnostic Engine with actionable failure evidence/remediation | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-008 | Diagnostics execute from actual selected Sensor | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-009 | Required firewall/connectivity matrix export | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-010 | Discovery review queue before licensing/monitoring | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-011 | IP/CIDR/seed/site/IPAM scope + exclusions | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-012 | Provisioning capacity simulation before activation | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-013 | Provisioning jobs support Fix & Retry | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-014 | Environment/Deployment Readiness Test | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-PRV-015 | Asset lifecycle/reconciliation and no automatic deletion | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Network monitoring

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-NET-001 | SNMP v1/v2c/v3 with customer-configurable auth/privacy/security protocols | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-002 | ICMP/traceroute/TCP/DNS/TLS/HTTP probes | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-003 | SNMP traps/informs and syslog | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-004 | NetFlow v5/v9, IPFIX, sFlow, J-Flow/vendor flow | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-005 | NETCONF/RESTCONF/gNMI/gRPC/streaming | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-006 | SSH/CLI; Telnet legacy disabled by default and audited | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-007 | CDP/LLDP/ARP/FDB discovery/topology | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-008 | BGP/OSPF/IS-IS/BFD/MPLS/VXLAN-EVPN/VLAN-STP/VRF/HSRP-VRRP/LACP/QoS-CBQoS/multicast/SD-WAN monitoring | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-009 | WLAN deep client/radio/controller experience | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-010 | VPN/IPsec/IP SLA/CUCM/VoIP operational monitoring | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-011 | DNS/DHCP/NTP operational monitoring | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-012 | TLS/certificate lifecycle monitoring | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-013 | Circuit/ISP inventory and SLA correlation | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-014 | Path Analysis workspace | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NET-015 | IPv4/IPv6/dual-stack first-class support | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Infrastructure/Cloud

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-INF-001 | SSH/WMI/WinRM agentless infrastructure monitoring | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INF-002 | Windows/Linux Agents | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INF-003 | iDRAC/iLO/CIMC/Redfish/IPMI hardware lifecycle | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INF-004 | VMware/Hyper-V/Proxmox/Nutanix | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INF-005 | Storage and SAN/FC monitoring | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INF-006 | UPS/PDU/environment/power/cooling/rack sensors | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INF-007 | Database monitoring packs + generic SQL query monitor | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INF-008 | AWS/Azure/GCP/OCI monitoring; detailed day-one provider-root and Cloud Service Pack requirements are governed by Platform_24 / REQ-CLOBS-* | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INF-009 | Kubernetes/Docker/container monitoring | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INF-010 | OpenTelemetry metrics/logs/traces are P0; proprietary full-bytecode APM agents are not mandatory. Detailed cross-signal/service-graph requirements are governed by Platform_24 / REQ-CLOBS-* | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INF-011 | Identity operational monitoring: AD/LDAP/Kerberos/RADIUS/TACACS+/M365/Exchange | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INF-012 | Synthetic monitoring ICMP/TCP/DNS/HTTP/HTTPS/API/TLS | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Extensibility

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-EXT-001 | MIB Studio upload/import/dependency/OID tree/test/device pack generation | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-EXT-002 | API Studio REST/OpenAPI/Swagger/schema inference/mapping/test | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-EXT-003 | Device Pack canonical package across SNMP/REST/gNMI/CLI/NCM/topology/alerts/RCA/dashboards | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-EXT-004 | Device Packs signed/versioned/exportable/importable/rollback capable | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-EXT-005 | Device Pack certification levels: Official/Partner/Customer/Experimental | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-EXT-006 | Device Pack test simulator using sample SNMP walk/API/CLI/trap/syslog | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-EXT-007 | Vendor MIB/YANG/API/icon/template provenance and redistribution rights tracked | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Policy/Alerts/Incidents

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-ALT-001 | Hierarchical policy inheritance Platform→Tenant→Region/BU→Site→Group→Device→Metric | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-ALT-002 | Static/duration/consecutive/rate/baseline/missing/composite/dependency/business-time thresholds | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-ALT-003 | Adaptive polling with triggers, cooldown and capacity guardrails | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-ALT-004 | Alert qualification state machine; failed SNMP poll != device down | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-ALT-005 | Multi-protocol/multi-vantage validation and confidence | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-ALT-006 | Event vs Alert vs Incident distinction | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-ALT-007 | Alert storm protection/dedup/rate-limit/compression | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-ALT-008 | Flapping detection and reopen windows | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-ALT-009 | Maintenance, notification mute and dependency suppression separated | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-ALT-010 | Recovery qualification before auto-resolution | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-ALT-011 | Alert ownership/assignment/escalation/shift handover | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-ALT-012 | Alert-policy effectiveness analytics | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Graph/RCA/AI

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-RCA-001 | One temporal multi-layer canonical operational graph | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-002 | Physical/L2/L3/routing/MPLS/VXLAN/SDWAN/traffic/infra/service/causal/geo views | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-003 | Relationship provenance/confidence and historical topology | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-004 | Hybrid deterministic+temporal+topology+protocol+change+anomaly RCA | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-005 | Root cause, confidence, causal chain, blast radius, evidence, contradictions, missing evidence, recommendation | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-006 | INCONCLUSIVE is a valid RCA outcome | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-007 | GNN only as hypothesis/propagation/impact scorer, never automatic causal proof | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-008 | Gemini through AI Gateway for NL search/explanation/summaries/recommendations | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-009 | Future private/offline/local LLM provider | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-010 | AI data boundaries, masking and audit | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-011 | RCA correctness feedback and intelligence-quality dashboard | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-012 | Unified Operations Timeline correlating telemetry/events/config/audit/ITSM/RCA | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-RCA-013 | PIR/problem-management generation and customer runbook knowledge base | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### NCM/IPAM/CLI

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-NCM-001 | NCM Day-1: backup/version/diff/change/compliance/golden config/templates/bulk push/approval/rollback/report/API | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NCM-002 | NCM change evidence enters RCA timeline | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NCM-003 | Separate NCM concurrency/rate/window controls | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NCM-004 | Privileged browser CLI Session Broker; browser never gets credentials | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-NCM-005 | CLI RBAC/command policy/JIT/approval/break-glass/session audit | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-IPAM-001 | Native IPAM IPv4/IPv6/subnet/supernet/VLAN/VRF/address/DNS/DHCP/leases/conflicts/history/forecast | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-IPAM-002 | Microsoft DHCP/DNS and Infoblox integration | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-IPAM-003 | IPAM enriches canonical graph and Flow | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### UI/UX/Reporting

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-UI-001 | Native Platform Dashboard Studio with flexible composable dashboards; Grafana may inspire interaction patterns but is not copied/embedded | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-002 | Create/group/folder/clone/import/export/share dashboards | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-003 | Drag/drop/resizable panels and broad chart/table/topology/map/Sankey types | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-004 | Light/dark theme, readable fonts, density options | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-005 | Role-specific landing pages and permission-aware navigation | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-006 | Global search, command palette, breadcrumbs, context side panel, favorites/recent/saved views | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-007 | Inventory with server-side pagination, bulk actions and saved filters | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-008 | Map Studio, Report Studio, MIB Studio, API Studio, Device Pack Studio | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-009 | Report Studio PDF/CSV/XLSX/JSON, formulas, branding, schedules | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-010 | SLA engine with maintenance/business-time rules | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-011 | Accessibility, keyboard navigation, contrast and no color-only status | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-012 | Mobile incident/dashboard experience; desktop for full admin/studios | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-013 | Query protection: timeout/row/scan/concurrency/async export | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-014 | Fast UI with caching/preaggregation/virtualization/lazy loading/query cancellation | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-015 | No-context-switch NOC troubleshooting from entity/alert/incident context | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-016 | Persistent contextual action bar + Troubleshoot drawer/workspace | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-017 | On-demand bounded ICMP/traceroute/SNMP/TCP/DNS/SSH/API tests through eligible Sensor/Agent | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-018 | Multi-vantage verification with contradictory evidence preserved | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-019 | Verify Now deterministic diagnostic bundles/packs | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-020 | Live Evidence distinguishes monitored state from fresh operator-triggered verification | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-021 | Why/What Changed/Compare contextual investigation actions | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-022 | Investigation Session preserves diagnostic/CLI/config/topology/flow evidence references and can link to incident | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-023 | Risk-proportionate action friction; CLI supports stored/personal/one-time credentials without browser exposure of reusable secrets | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-024 | Diagnostic jobs bounded by tenant/scope/target/rate/concurrency/timeout/cancellation and isolated from routine monitoring capacity | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-UI-025 | AI may explain/suggest permitted next tests but cannot fabricate results or bypass authorization/execution controls | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Data lifecycle

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-DATA-001 | Customer-configurable raw retention by data type/scope | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-002 | Customer-configurable rollup resolution and rollup retention | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-003 | Numeric rollups preserve min/max/avg/count/percentiles as appropriate | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-004 | Counter and state metrics use correct rate/delta/availability semantics | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-005 | Automatic query resolution selection based on time range | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-006 | Hot/warm/archive capability and NFS/S3-compatible/customer archive destinations | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-007 | Mandatory Core 80% storage-protection watermark | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-008 | Mandatory Sensor 80% storage-protection watermark | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-009 | Emergency purge oldest eligible raw telemetry/flow/log before protected state | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-010 | Preserve recent operational window within safety limits | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-011 | Storage forecast and retention simulation before policy change | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-012 | Telemetry cardinality/explosion protection | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-013 | Deduplication, replay, late/out-of-order handling | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-014 | Legal hold/preserve incident evidence from purge | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-DATA-015 | Data pipeline integrity counters and no silent data loss | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Integration/Governance/Security

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-INT-001 | ServiceNow/BMC/ServiceDesk Plus/SummitAI/Freshservice/Jira SM/generic REST ITSM | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INT-002 | Email/Teams/Slack/WhatsApp/webhook/SMS/PagerDuty channels | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INT-003 | Bidirectional ITSM field/state/note synchronization where API allows | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INT-004 | Durable outbound integration queue/retry/idempotency/health | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-INT-005 | Integration Test Sandbox before production enablement | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-GOV-001 | Append-only/tamper-evident structured audit, SIEM export and protected retention | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-GOV-002 | RBAC + resource scope + conditions; explicit deny wins | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-GOV-003 | AD/LDAP/SAML/OIDC/Entra identity mapping | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-GOV-004 | Credentials USE vs VIEW separated | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-GOV-005 | MFA/approval/reason/ticket for sensitive actions | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-SEC-001 | Security-by-design for banks/defence/global enterprises | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-SEC-002 | Design for ISO27001/SOC2/PCI/RBI/FIPS/STIG/CIS readiness; no certification claim before achieved | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-SEC-003 | mTLS Sensors/Agents, TLS, encrypted secrets, KMS/HSM support | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-SEC-004 | Signed packages/updates, SBOM, CVE/license/secret scan, secret redaction | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-SEC-005 | PKI/certificate lifecycle management | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-SEC-006 | Proxy/no-proxy support | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-SEC-007 | Data sovereignty/regional placement architecture | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-SEC-008 | Tenant isolation enforced server/API/query layers | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Lifecycle/Commercial

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-LC-001 | Backup/restore with verification | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LC-002 | Safe low-downtime upgrades and rollback | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LC-003 | Canary rollout and feature flags | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LC-004 | Desired-vs-actual state and configuration drift | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LC-005 | Change impact preview and dry-run | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LC-006 | Technology lifecycle/EOL/EOS/firmware risk visibility | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LC-007 | Support bundle with secret masking and error correlation IDs | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LC-008 | Migration Studio from incumbent tools/CMDB/CSV/API | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LC-009 | Template library and onboarding wizard | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LC-010 | Change freeze and business calendars | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LIC-001 | Commercial license supports annual/perpetual/device/element/module models | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LIC-002 | 30-day expiry warning | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LIC-003 | 7-day dashboard banner before expiry | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LIC-004 | 7-day default grace after expiry; then polling stops unless manually extended | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LIC-005 | Historical data remains accessible after polling stops | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-LIC-006 | Usage/licensing dashboard and entitlement-aware UI | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### OSS/IP

| ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| REQ-OSS-001 | Clean-room Platform independent of ThoughtData and NetAI | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-OSS-002 | Only approved OSS catalogue dependencies may ship | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-OSS-003 | Green permissive licenses generally allowed; Yellow review; Red blocked by default | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-OSS-004 | SBOM SPDX + CycloneDX, notices, license texts, provenance/checksums every release | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-OSS-005 | Transitive dependency license/CVE scan mandatory | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-OSS-006 | Vendor MIB/YANG/SDK/API examples/icons/templates require provenance/redistribution rights | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| REQ-OSS-007 | Customer-supplied MIBs may be used for that customer but not automatically redistributed | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| OSS-008 | Air-gap bundles contain all pinned/scanned artifacts; no runtime internet dependency install | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### Engineering governance (v3.2.1)

These are engineering-governance controls, not customer-facing product features.

| ID | Requirement | Index | Decision Register | Eng/Docker | CI/CD | OSS Catalogue | Traceability |
|---|---|:---:|:---:|:---:|:---:|:---:|:---:|
| GOV-REPO-001 | Canonical source repository is `https://github.com/gagandsm-max/platform`; local clones are working copies; authority follows GitHub merge | ✓ | ✓ | ✓ | ✓ | — | ✓ |
| GOV-DOC-001 | Mother documents are READ-MOSTLY; controlled correction is mandatory when errors/contradictions/decision changes/selected dependencies/resolved P0s/omissions/implementation conflicts occur | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| GOV-OSS-001 | Approved dependencies must match `Platform_07_OSS_Catalog.md` and implementation; catalogue omissions require change control before milestone/release | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |


### NOC troubleshooting supporting-master coverage

Requirements `REQ-UI-015` through `REQ-UI-025` are additionally governed by the following specialized/base masters beyond the six logical design columns above:

- `Platform_09_UI_UX_Master.md` — explicit action-bar/drawer/entity/incident/RCA screen behavior.
- `Platform_10_Protocol_Security_Connector_Master.md` — protocol execution, credentials, target safety and diagnostic security contract.
- `Platform_11_Sizing_Capacity_Storage_Master.md` — interactive diagnostic/CLI workload accounting, capacity reserve and no-starvation certification.
- `Platform_14_Test_Performance_and_Release_Quality.md` — positive/negative/E2E/performance release evidence.
- `Platform_17_API_Event_and_Data_Model_Contracts.md` — diagnostic job, Live Evidence, Investigation Session and CLI-session API/event/data contracts.
- `Platform_18_Security_Threat_Model.md` — arbitrary-target, cross-tenant, credential/session and resource-exhaustion threats/mitigations.
- `Platform_21_Enterprise_Gap_Closure_Master.md` — Incident Command Workspace projection when an Investigation Session links to an incident.
- `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` — Active Evidence Acquisition, evidence trust and Suggested Next Test/Copilot constraints.
- `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` — single Capability Pack ownership/certification of technology-specific diagnostics; no second diagnostic-pack framework.

A NOC troubleshooting requirement is not complete merely because the BRD/PRD/HLD/LLD/Dataflow columns are checked; applicable supporting-master contracts above must also be satisfied.

## 3. Completeness gate

A release/major architecture change cannot be declared fully documented until:

1. each new requirement has a stable ID;
2. PRD behavior and acceptance criteria exist;
3. architecture/HLD/LLD ownership exists for technical requirements;
4. at least one normal dataflow and relevant failure dataflow are documented;
5. security/RBAC/audit impact is identified;
6. OSS/dependency impact is recorded in the OSS Catalogue where relevant;
7. capacity/storage/HA/DR impact is evaluated;
8. UI/admin surface is defined for customer-configurable behavior.

Documentation mapping is not implementation evidence. Implementation is not validation. Validation is not certification. Certification is not deployment.

v3.2.2 specialized-master RTM supplements live in their owning masters and are not copied into this base catalogue:

- OIV: `PRD-OIV-*` / `REQ-OIV-*` in `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md`
- ONB: `PRD-ONB-*` / `REQ-ONB-*` in `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md`
- EGC: `PRD-EGC-*` / `REQ-EGC-*` in `Platform_21_Enterprise_Gap_Closure_Master.md`
- MIA: `PRD-MIA-*` / `REQ-MIA-*` in `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md`
- EMC: `PRD-EMC-001` through `PRD-EMC-018` and `REQ-EMC-001` through `REQ-EMC-018` in `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md`

Existing Device Pack RTM rows (`REQ-EXT-*`) remain the base Device Pack completeness control and are supplemented, not replaced, by Capability Pack certification requirements in `Platform_23`.


## 4. Supporting master coverage

- Detailed per-dashboard UI contract: `Platform_09_UI_UX_Master.md`.
- Full protocol/security/credential/connectivity contract: `Platform_10_Protocol_Security_Connector_Master.md`.
- Sizing/MWU/Sensor count/storage lifecycle contract: `Platform_11_Sizing_Capacity_Storage_Master.md`.
- Decision provenance/no-assumption control: `Platform_12_Decision_Source_Register.md`.
- Full OSS license/use/restriction catalogue: `Platform_07_OSS_Catalog.md`.
- Engineering/development/Docker standards: `Platform_13_Engineering_Development_and_Docker_Standards.md`.
- Test/performance/release-quality contract: `Platform_14_Test_Performance_and_Release_Quality.md`.
- CI/CD/packaging/distribution contract: `Platform_15_CI_CD_Packaging_and_Distribution.md`.
- Installation/upgrade/rollback/operations contract: `Platform_16_Installation_Upgrade_Rollback_and_Operations.md`.
- API/event/data-model contract: `Platform_17_API_Event_and_Data_Model_Contracts.md`.
- Security threat model: `Platform_18_Security_Threat_Model.md`.
- OIV specialized-master RTM supplement: `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md`.
- Discovery/onboarding/identity/notification specialized-master RTM supplement: `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md`.
- Enterprise gap-closure specialized-master RTM supplement: `Platform_21_Enterprise_Gap_Closure_Master.md`.
- Monitoring-intelligence/alert-quality/AI-assurance specialized-master RTM supplement: `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md`.
- Enterprise monitoring coverage / Capability Pack certification specialized-master RTM supplement: `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md`.

### Development, packaging, service supervision and diagnostics — v3.1

| Requirement ID | Requirement | BRD | PRD | Architecture | HLD | LLD | Dataflow |
|---|---|---|---|---|---|---|---|
| DEV-DOCKER-001 | All routine local development and automated tests use Docker Compose; no project-specific host runtimes | §21 | PRD-DEV-001 | §34.1 | §35.1 | §71.1 | DF-090 |
| DEV-REBOOT-001 | Local development survives normal laptop reboot via named volumes, restart policy and health-aware recovery | §21.1 | PRD-DEV-002 | §34.2 | §35.1 | §71.2 | DF-090 |
| PKG-LINUX-001 | Core/Sensor/Linux Agent ship as `.deb` and `.rpm` | §21.2 | PRD-PKG-001 | §34.3 | §35.3 | §71.7 | DF-093 |
| PKG-WIN-001 | Windows Agent is compiled signed Windows Service; GA MSI plus BAT wrappers | §21.2 | PRD-PKG-001 | §34.3/34.6 | §35.5 | §71.4/71.7 | DF-092/093 |
| SVC-AUTOBOOT-001 | Core/Sensor/Agents start automatically after host reboot | §21.3 | PRD-SVC-001 | §34.4 | §35.3/35.5 | §71.3/71.4 | DF-091/092 |
| SVC-RECOVERY-001 | Recoverable crashes trigger supervised restart; persistent crash loops remain visible/diagnosable | §21.3 | PRD-SVC-001 | §34.4 | §35.4 | §71.3/71.4 | DF-091/092 |
| LOG-STRUCT-001 | All components maintain structured lifecycle/failure/health logs with correlation IDs and secret redaction | §21.4 | PRD-LOG-001 | §34.5 | §35.4 | §71.5/71.6 | DF-090..093 |
| DEV-WIN-CERT-001 | Windows-specific service/MSI/reboot certification uses isolated Windows runner/VM despite Docker-first development | §21.1/21.2 | PRD-PKG-001 | §34.6 | §35.5 | §71.4 | DF-092/093 |

# Platform — Enterprise Monitoring Coverage, Capability Packs and Real-Customer Certification Master

**Document ID:** PLATFORM-EMC-001
**Version:** 3.3.0
**Status:** **Authoritative Mother Reference — Specialized Master**
**Product:** Platform
**Change class:** Additive v3.3.0 product/engineering requirements; EMC-0 specification only. No runtime implementation, vendor-pack shipment, certification achievement or contractual guarantee is claimed by this document.

---

## 1. Authority, purpose and non-claims

This document is the consolidated specialized master for enterprise monitoring-domain coverage, the Capability Pack product contract, dimensional certification, Customer Acceptance Profiles and Real Customer Requirements normalization.

`Platform_00_Document_Index.md` defines precedence. This master may add behavior where existing files are silent. It must not silently override a higher-precedence explicit requirement. Contradictions require change control before implementation.

This document does **not** select a new OSS dependency, database, Core service, microservice, commercial SKU, price, licence quantity, vendor SDK or runtime loader. Named products below are **certification targets**, not Core hard-coding. Specification is not implementation. Implementation is not validation. Validation is not certification. Certification is not deployment.

### 1.1 Authority boundaries

| Owner | Retains authority for |
|---|---|
| `Platform_10` | Protocol mechanics, connector security, authentication, credential handling, transport/security mechanics |
| `Platform_11` | Sensor/Core sizing, MWU, interactive diagnostic/CLI capacity budgets and no-starvation certification inputs |
| `Platform_14` | Generic testing, performance and release-quality evidence |
| `Platform_17` | Versioned API/event/data-model contracts for diagnostic jobs, Live Evidence, Investigation Sessions and CLI sessions |
| `Platform_19` | SaaS operational intelligence and OIV/remediation/value capabilities |
| `Platform_20` | Governed discovery, onboarding, identity continuity and Notification Center |
| `Platform_21` | Existing enterprise gap-closure requirements (`PRD-EGC-*` / `REQ-EGC-*`) |
| `Platform_22` | Telemetry trust, evidence-qualified alerting, alert quality, RCA/AI assurance |
| `Platform_23` (this master) | Enterprise monitoring-domain catalogue; Capability Pack product contract; L1/L2/L3 maturity; dimensional certification; pack compatibility/version lifecycle; enterprise technology coverage; Customer Acceptance Profiles; Real Customer Requirements normalization; operational-graph vs governed-CMDB semantics; enterprise service-health dependency semantics; coverage measurement; pack certification requirements |

Existing Device Pack contracts remain valid. They are classifications/profiles under the single Capability Pack architecture and must not be broken by this packaging.

---

## 2. Non-negotiable principles

1. **One extensibility architecture: Capability Pack.** Technology Monitoring Pack, Device / Network Pack, SaaS Monitoring Pack, Integration Pack and optional Customer Extension Pack are classifications/profiles of that architecture. An independent Technology Pack framework is prohibited.
2. **No pack gets its own new Core service by default.** Packs load into existing Core/Sensor/Agent/connector surfaces.
3. **No new database** is introduced for pack/certification state.
4. **No arbitrary vendor-specific logic is hard-coded into Platform Core.** Vendor behavior lives in packs/adapters that normalize into Platform-owned contracts.
5. **Certification is dimensional.** Do not collapse Discovery/L1/L2/L3/Topology/NCM/Security into one scalar such as "L2 supported".
6. **Coverage metrics stay distinct.** Architecture Coverage, Specification Coverage, Implementation Coverage, Pack Coverage, Certification Coverage and Integration Coverage must never be reported as a generic "95% supported".
7. **Confidential real-customer data stays out of this set.** Normalize to the public benchmark identity in §4. Do not copy customer names, inventories, locations, quantities, commercial clauses, penalties, unique SLAs, contract language or confidential identifiers.
8. **LLM output is not an authoritative calculator or privileged executor.** Capacity what-if, NCM, ChatOps and remediation remain deterministic, authorized and audited.
9. **Legacy/insecure targets must never weaken Core security defaults.**
10. **Endpoint/EUC monitoring is operational experience, not employee surveillance.**

---

## 3. Formal product requirements

| PRD ID | Requirement |
|---|---|
| `PRD-EMC-001` | Platform shall apply L1/L2/L3 monitoring maturity and dimensional certification per applicable Capability Pack. |
| `PRD-EMC-002` | Platform shall use one reusable Capability Pack architecture supporting Technology Monitoring Pack classifications rather than a parallel framework. |
| `PRD-EMC-003` | Product/version compatibility and certification evidence shall be tested and recorded against the machine-readable compatibility model. |
| `PRD-EMC-004` | Middleware/application-platform monitoring shall be delivered through Capability Packs. |
| `PRD-EMC-005` | Relational and NoSQL/data-platform breadth shall be delivered through certified Capability Packs. |
| `PRD-EMC-006` | Backup/Data Protection is a first-class monitoring family. |
| `PRD-EMC-007` | AWS/Azure/GCP/OCI coverage shall use normalized Platform-owned cloud entity/state contracts. |
| `PRD-EMC-008` | SaaS Monitoring Pack capability is a Capability Pack classification, cross-referenced to `Platform_19`. |
| `PRD-EMC-009` | Platform shall support scheduled/batch job monitoring. |
| `PRD-EMC-010` | Optional EUC/VDI monitoring is a lower-priority pack family with explicit privacy boundaries. |
| `PRD-EMC-011` | Integration Packs are a Capability Pack classification; `Platform_10` retains connector/security mechanics authority. |
| `PRD-EMC-012` | Operational Graph may coexist with governed CMDB state while preserving provenance, confidence and temporality. |
| `PRD-EMC-013` | Service health shall support typed dependencies, redundancy and quorum rather than worst-child aggregation. |
| `PRD-EMC-014` | Named configurable enterprise retention profiles shall be supported; customer contractual retention values are not hard-coded as universal product behavior. |
| `PRD-EMC-015` | Measurable Customer Acceptance Profiles shall exist; exact customer target values remain profile/certification values, not universal product guarantees. |
| `PRD-EMC-016` | Pack lifecycle/compatibility shall be defined across Platform upgrades, including deprecation and EOL. |
| `PRD-EMC-017` | OEM/customer extensions shall use supported pack/SDK mechanisms rather than Core forks. |
| `PRD-EMC-018` | Monitoring-coverage completeness shall be measurable by scope, domain, technology and certification dimension. |
| `PRD-EMC-019` | Windows/Linux infrastructure monitoring shall support and certify **Agent-based** and **agentless/remote** collection as separate capability dimensions while normalizing equivalent host/process/service/evidence semantics where technically available. |
| `PRD-EMC-020` | Certified server monitoring shall cover host CPU/scheduler pressure, memory/swap/paging pressure, filesystem/inode capacity, block I/O, network/NIC quality, OS/uptime, bounded process/service health and applicable hardware/event evidence rather than CPU/memory/disk-capacity gauges alone. |
| `PRD-EMC-021` | Linux Agent, Windows Agent, SSH and WinRM/WMI/CIM monitoring shall expose explicit capability/privilege gaps; unsupported or failed collection shall be represented as missing/degraded monitoring evidence rather than target failure. |
| `PRD-EMC-022` | Infrastructure Capability Packs shall declare continuous-monitoring collectors and bounded EDS diagnostics separately. EDS is Diagnostic Job profile `ENDPOINT_DIAGNOSTIC_SNAPSHOT` within the existing Troubleshoot / Live Evidence architecture (not a second diagnostics framework). Packs shall declare supported OS/version, privilege, evidence fields, resource budget and dimensional certification state. |
| `PRD-EMC-023` | Platform shall define the approved 16-profile Phase-1 Official Certified Capability Pack target baseline without implying implementation or achieved certification. |
| `PRD-EMC-024` | Official Certified Packs shall satisfy the v3.3 OOB acceptance contract spanning identification, compatibility/preflight, discovery/inventory, canonical entities/relationships, safe default monitoring, dashboards, alerts/qualification, diagnostics, limitations/provenance and upgrade behavior where applicable. |
| `PRD-EMC-025` | Capability support claims shall remain dimensional and exact product/version specific and derive from one machine-readable compatibility/certification source of truth. |
| `PRD-EMC-026` | Phase-1.5 second-wave technology coverage is a roadmap/certification target only and requires evidence before support claims. |

---

## 4. Real Customer Requirements normalization

Source benchmark identity for this master:

**Real Customer Enterprise Benchmark — Automotive / ITOM-AIOps**

This is a normalized, non-confidential classification of recurring enterprise ITOM/AIOps needs. It is not a named customer, contract, inventory or SLA.

Each normalized requirement is classified as:

| Class | Meaning |
|---|---|
| **A** | Universal Platform Capability |
| **B** | Technology / Device / Monitoring Pack |
| **C** | Integration Pack |
| **D** | Customer Acceptance / Certification Profile |
| **E** | Customer-specific implementation |
| **F** | Commercial / Contractual |
| **G** | Intentionally excluded / deferred |

Classes E and F must not be copied into Mother References as universal product behavior. Class G remains explicit (see §25).

---

## 5. L1 / L2 / L3 monitoring maturity

| Level | Name | Examples |
|---|---|---|
| **L1** | Availability | reachable; running; interface/path/session up; cluster/HA state; dependency availability |
| **L2** | Performance | latency; utilization; saturation; throughput; errors; queues; contention; resource pressure |
| **L3** | Predictive / Intelligence | anomaly; baseline deviation; forecast; emerging risk; likely failure; service impact; change correlation; recommended action; validated intelligent RCA support |

L3 does not authorize treating GNN/LLM output as causal truth. `Platform_22` remains the alert/RCA/AI-assurance authority. `INCONCLUSIVE` remains valid.

Certification is **dimensional**. Required dimensions at minimum:

- Discovery
- Availability / L1
- Performance / L2
- Intelligence / L3
- Topology
- NCM where relevant
- Diagnostics where relevant
- eBPF process-network evidence where declared (Linux server technology packs)
- Active path observability where declared (network and cloud technology packs)
- Security
- Version compatibility
- Certification profile
- evidence bundle/reference

Allowed dimensional states include: `UNSUPPORTED`, `EXPERIMENTAL`, `CERTIFIED`, `DEPRECATED`, `EOL`.

A technology may therefore be Discovery `CERTIFIED`, L1 `CERTIFIED`, L2 `CERTIFIED`, L3 `EXPERIMENTAL`, Topology `CERTIFIED` and NCM `UNSUPPORTED` at the same time. Do not collapse that into "L2 supported".

---

## 6. Capability Pack contract

This section specifies the product contract only. The runtime loader, SDK and pack registry are **EMC-1** work and are not implemented by this PR.

A Capability Pack, where applicable, declares:

- pack identity
- semantic version
- pack class (`TECHNOLOGY_MONITORING`, `DEVICE_NETWORK`, `SAAS_MONITORING`, `INTEGRATION`, `CUSTOMER_EXTENSION`)
- supported Platform version range
- supported vendor/product/version range
- discovery
- identity normalization
- entities
- relationships
- telemetry mapping
- canonical units
- operational state mapping
- event mapping
- alert/evidence requirements (class contracts remain `Platform_22`)
- topology
- dashboards
- health logic
- diagnostics
- NCM hooks
- remediation hooks
- credentials required
- least-privilege requirements
- API/rate-limit constraints
- resource budget
- upgrade/migration contract
- rollback compatibility
- deprecation/EOL
- certification metadata
- provenance
- signature/integrity requirements
- SNMP poll definitions/scalar-table-index metadata where applicable
- MIB/OID references and unresolved-numeric-OID declarations where applicable
- enum/bit/state mappings where applicable
- exact counter width/rate/reset/discontinuity semantics where applicable
- GETBULK/GETNEXT compatibility and bounded poll-planning hints where applicable
- trap/inform notification mappings, source-trust expectations and storm fingerprint hints where applicable
- SNMP diagnostic declarations and declared privileged SET/NCM actions where applicable

Device / Network Packs continue to satisfy existing Device Pack identification, mapping, provenance and test-lab contracts. They gain the Capability Pack identity/lifecycle/certification envelope without a second schema family.

The UI/product phrase **diagnostic pack** or **diagnostic workflow** does **not** create another package type, SDK, registry or deployment unit. It means a versioned deterministic troubleshooting workflow that references Platform-native checks and, where technology-specific behavior is required, diagnostics declared by the applicable Capability Pack. A Capability Pack diagnostic declaration includes at minimum action identity, supported entity/technology/version scope, read-only/privilege class, protocol/command/API mapping, bounded input schema, expected typed result/error schema, credential/role requirements, safety/rate/resource cost metadata and certification evidence. Arbitrary scripts or LLM-generated commands are not diagnostic declarations.

No pack class may fork Core, add a dedicated database, or bypass RBAC, tenant scope, credential vault, audit or `Platform_10` connector/security mechanics.

---

## 7. Machine-readable compatibility and certification model

One source of truth shall drive future UI/documentation support matrices. Independent marketing tables are prohibited.

Required fields:

- technology family
- vendor
- product
- version range
- discovery state
- L1 state
- L2 state
- L3 state
- topology state
- NCM state
- diagnostics state
- agentless state
- agent state
- integration state
- certification profile
- certification date
- evidence reference
- deprecated status
- EOL status

This model is specified, not implemented. Existence of this section is not a shipped registry.

---

## 8. Monitoring domain catalogue

Families below are coverage targets. Vendor implementations remain packs/certification targets.

### 8.1 Network

Routers; switches; WLAN; firewalls; load balancers / ADC; SD-WAN; DNS; DHCP; PKI/certificate services; flow/NTA; NCM; IPAM; path analysis; topology.

SD-WAN canonical-model mechanics already owned by `Platform_21` are not duplicated here. This family supplies monitoring-coverage and pack-certification classification.

For SNMP-capable network packs, certification is object/semantic aware rather than "SNMP reachable." A pack may declare and separately certify device identity, interface inventory/state, exact high-capacity counters/rates, CPU/memory/environment, routing/neighbor/topology facts, trap/inform semantics, diagnostics and privileged NCM actions. Pack mappings must distinguish raw counter/state/inventory/topology/event evidence and declare firmware/model compatibility where vendor behavior differs. Normal operational-metric cadence defaults to **5 minutes** unless a pack/monitor definition specifies otherwise; packs must not imply that every available OID is polled at that default.

A pack shall not use `ifIndex` as the sole permanent interface identity. Where a vendor provides stronger stable interface/component identifiers, the pack declares them and their reconciliation precedence.

### 8.2 Servers / operating systems

Windows; RHEL; Ubuntu; SLES; Oracle Linux; AIX; other certified Unix/Linux.

Certification **must** separately record:

- agentless/remote monitoring certification;
- Agent-based certification;
- continuous host-metric coverage;
- process/service coverage;
- OS/event/hardware evidence coverage where supported;
- Endpoint Diagnostic Snapshot (EDS) as Diagnostic Job profile `ENDPOINT_DIAGNOSTIC_SNAPSHOT` / investigation coverage (not a second diagnostics framework);
- resource-overhead and least-privilege certification.

Minimum normalized server evidence targets, subject to OS/version/privilege capability, are:

- CPU aggregate and useful scheduler breakdown, load/run-queue or platform equivalent;
- Linux PSI or equivalent resource-pressure evidence where available;
- memory available/used/commit, swap/page activity and OOM/resource-exhaustion evidence;
- filesystem capacity and inode state plus block I/O throughput/IOPS/queue/latency/error evidence;
- NIC state/throughput/errors/drops plus bounded transport-quality counters such as retransmissions where available;
- bounded Top-N process CPU/memory/I/O/state/restart evidence;
- Linux systemd and Windows Service Control Manager service state/restart/failure evidence;
- OS version/boot/uptime/time-quality facts and selected bounded operational/kernel/Event Log evidence;
- hardware/thermal/power evidence only through safe native or approved out-of-band mechanisms.

Agent collection may provide richer/local evidence than remote collection. Product/UI/API shall disclose these differences rather than falsely claim identical raw coverage. Agentless Linux shall use governed SSH query packs; agentless Windows shall use governed WinRM/WMI/CIM query packs. Collection/credential/path failure is monitoring-health evidence and shall not automatically assert host/service failure.

Legacy systems must never weaken Core security defaults.

### 8.3 Virtualization

VMware; Hyper-V; KVM; OpenShift Virtualization.

Canonical entities: host; cluster; VM; datastore; resource pool; virtual switch.

### 8.4 Containers

Kubernetes; EKS; AKS; GKE; OpenShift; Docker; containerd; CRI-O.

Canonical entities: cluster; node; namespace; workload; pod; container; service; ingress; persistent volume.

This does not move Platform Core onto Kubernetes.

### 8.5 Storage / SAN

Canonical entities: array; controller; pool; volume; LUN; disk; share; fabric; port.

### 8.6 Databases

Relational: Oracle; SQL Server; PostgreSQL; MySQL; MariaDB; Db2; SAP HANA.

NoSQL / data platforms: MongoDB; Cassandra; Redis / Valkey; Elasticsearch; OpenSearch; Kafka; ClickHouse; DynamoDB; Cosmos DB.

Canonical entities: instance; cluster; node; database; schema; tablespace; replica; connection pool; transaction log; query workload.

Validated L3 capabilities, where certified, may include: growth risk; replication lag; locks; session pressure; slow/query workload; shard imbalance; GC pressure; capacity risk.

### 8.7 Middleware / application platform

Tomcat; WebSphere; WebLogic; JBoss / WildFly; IIS; .NET; Node.js; SAP NetWeaver; Apache; Nginx; HAProxy; IBM MQ; RabbitMQ; ActiveMQ; TIBCO EMS; MuleSoft; Oracle SOA; SAP PI/PO; Apigee; Kong; AWS API Gateway; Azure API Management.

Metrics/capabilities may include: request rate; errors; latency; thread pools; connection pools; heap; GC; sessions; queues; oldest message; DLQ; consumers; throttle state.

This does **not** reopen full bytecode APM. Integrate with APM products via Integration Packs.

`Platform_21` already requires middleware Packs/connectors. This master owns coverage catalogue, dimensional certification and pack-class contract for that family.

### 8.8 Cloud

AWS; Azure; GCP; OCI using normalized Platform-owned cloud entity/state contracts. Vendor identifiers never override tenant/entity authority.

### 8.9 SaaS / security SaaS

Generic pack contract should support: REST; OAuth; API keys; pagination; JSON extraction; provider health; synthetics; webhooks; canonical state normalization; rate-limit handling; dashboards; alerts.

Example certification **categories** (named products are targets, not Core code): EDR; email security; CSPM/CNAPP; external-risk intelligence; vulnerability management.

SaaS operational-intelligence behavior remains `Platform_19`. This master owns the pack classification and coverage/certification envelope.

### 8.10 Backup / data protection

First-class monitoring family. Platform does not become a backup engine.

Canonical entities: server; agent; proxy; job; policy; repository; replication target; tape/library where applicable.

Canonical states: `SUCCESS`; `PARTIAL`; `FAILED`; `MISSED`; `RUNNING`; `STUCK`.

Capabilities: job duration; throughput; repository capacity; dedupe; RPO status; last successful backup; restore-test evidence; replication health.

`Platform_21` already requires backup product monitoring through Packs/connectors. This master owns family-first-class status, canonical states and certification dimensions.

### 8.11 M365 / collaboration

Microsoft 365; Exchange Online; Teams; SharePoint; OneDrive; on-prem Exchange; SMTP relay.

### 8.12 Batch / scheduled jobs

Sources: cron; Windows Task Scheduler; API-driven jobs; scripts; ETL/Informatica-class systems.

Canonical model: job; schedule; execution; dependency; SLA window.

States: `SUCCESS`; `FAILED`; `MISSED`; `STUCK`; `LATE`; `RUNNING`.

Metrics: duration; failures; dependency status; queue risk; SLA-window risk.

### 8.13 EUC / VDI

Optional/lower-priority pack family. Examples: Citrix; Horizon; Azure Virtual Desktop; printing infrastructure; kiosk/plant terminals; signage.

Privacy boundary: no employee surveillance. No keystroke, content, screenshot, microphone, camera or productivity-scoring collection.

---

## 9. Integration Pack architecture

Integration Pack is a Capability Pack classification. `Platform_10` owns connector/security mechanics.

Categories include: ITSM; SIEM; EDR; Vulnerability Management; CMDB; APM; Collaboration; Security SaaS; Cloud.

Integration requirements include: retry/backoff; idempotency; reconciliation; failure/dead-letter handling; audit; credentials; tenant scope; authorization; API version compatibility.

Teams/ChatOps actions may include: acknowledge; assign; escalate; status; deep link; copilot interaction; approved diagnostics; approved remediation.

Every action must pass through authentication, RBAC, tenant scope, policy/approval and audit. No direct LLM privileged execution.

---

## 10. Operational Graph versus governed CMDB

Two related but distinct uses of the same graph technology. Do **not** create a second graph database merely to model this distinction.

**Operational Graph:** continuously discovered; temporal; observed; provisional relationships allowed; provenance required; confidence required; age/freshness required.

**Governed Inventory / CMDB:** reconciled; normalized; administratively governed; approval/reconciliation state where required.

RCA may use recent provisional Operational Graph relationships before CMDB approval when appropriate. Every relationship used must expose: source; confidence; age; provisional/governed state; supporting evidence.

Discovery/onboarding identity continuity remains `Platform_20`. This section does not weaken governed review before licence/monitoring activation.

---

## 11. Service health

Example hierarchy:

Business Service → Application Service → Components → Middleware / Database → Compute / Container → Network / Storage / Cloud

Dependencies must support: critical; optional; redundant; active/standby; quorum; weighted dependency; SLA path; service criticality.

Service health must **not** simply equal "worst child state". Redundant or optional children, quorum and weighted paths must be evaluable. Semantic states from `Platform_22` (`MONITORING_DEGRADED`, `REDUNDANCY_LOST`, `SLA_DEGRADED`, `EXPECTED_STATE`, `UNKNOWN`, `STALE`) must not be collapsed into generic DOWN.

---

## 12. Event / evidence boundary

Cross-reference `Platform_22`. This master does not duplicate Alert Evidence Contracts, telemetry-quality gates or RCA/AI assurance.

Enterprise flow remains conceptually:

raw evidence → normalize → deduplicate → telemetry-quality evaluation → evidence qualification → qualified alert → topology/temporal correlation → incident/situation → RCA

Canonical operational events should retain: tenant; source; source_type; entity; event_class; state; severity; timestamp; observed_at; dedup/correlation identity; topology/change context; raw reference; schema version.

---

## 13. Log analytics boundary

Platform may provide operational log analytics for: OS; databases; middleware; network; applications; cloud; Agents; integrations.

Support: real-time operational analysis; historical search; incident correlation; retention; RBAC; tenant isolation.

Platform is **not** an unrestricted general SIEM. SIEM products remain Integration Pack targets.

---

## 14. Capacity what-if

Deterministic/statistical scenario analysis such as: traffic +20%; add 100 servers; remove one cluster node; change retention; migrate a site/workload.

Output should include: current utilization; trend; forecast; threshold; confidence; assumptions; remaining headroom.

LLMs must not be the authoritative calculator. Sizing/capacity numbers remain subject to `Platform_11` and certified SNWP evidence; this master does not invent SNWP constants.

---

## 15. NTA / DPI boundary

Native initial capability includes: NetFlow; IPFIX; sFlow; J-Flow/vendor flow; exported application classification; top talkers; conversations; site matrices; QoS analysis; trends; anomalies; capacity analysis.

Bounded Triggered Packet Evidence remains permitted under `Platform_19`. Always-on full DPI remains deferred. This master does not reopen full DPI.

---

## 16. NCM enhancement

Enterprise NCM capability direction: configuration backup; running/startup comparison; golden configuration; history; drift; unauthorized change; compliance; CIS/custom policy; pre-change validation; approval; deployment; post-change validation; rollback; audit; EOL/EOS awareness; recurring drift checks; change-risk scoring; service/topology impact.

No uncontrolled automated configuration deployment. LLM output may not execute privileged NCM actions. Existing Device Pack command-mapping contracts remain consumed, not forked.

---

## 17. IPAM

Specify: IPv4; IPv6; dual-stack; subnet; VLAN; allocation; reservation; conflict; orphan/stale entries; DNS/DHCP reconciliation; capacity/utilization.

Support native functionality and governed third-party integration. IP/MAC remain mutable observations under `Platform_20` identity rules.

---

## 18. Retention profiles

Do not hard-code customer contractual retention values into universal product behavior.

Named configurable profiles such as `Enterprise-180`, `Enterprise-365` and `Regulated-Custom` may be offered. Sizing/capacity validation is required before activation. Exact days remain profile/policy values.

---

## 19. Customer Acceptance Profiles

Measurable dimensions may include: monitoring coverage; discovery completeness; relationship/topology completeness; integration success; alert-noise reduction; false-positive rate; MTTD; MTTA; time-to-root-cause; MTTR; HA; DR; retention; dashboards; training/operational readiness.

Exact customer target values remain profile/certification values. Do not convert contract-specific values into universal product guarantees. Value/MTTR methodology remains `Platform_19`.

---

## 20. Real Customer Requirements registry

Reusable registry fields:

- RCR ID
- segment
- category
- normalized requirement
- requirement classification A–G
- current Platform coverage
- specification status
- implementation status
- pack status
- certification status
- frequency across independent customers
- disposition
- priority
- rationale

Customer identity/source mapping must remain controlled/private and outside this Mother Reference set.

---

## 21. Coverage metrics

Coverage metrics MUST remain distinct:

- Architecture Coverage %
- Specification Coverage %
- Implementation Coverage %
- Pack Coverage %
- Certification Coverage %
- Integration Coverage %

Never report a generic statement such as "95% supported" when only architecture/specification exists.

Specified ≠ implemented. Implemented ≠ tested. Tested ≠ certified. Certified ≠ deployed.

---

## 22. Pack certification harness

Certification evidence requirements (family-specific gates; generic test strategy remains `Platform_14`):

**Discovery:** identity; reconciliation; no duplicates; update/re-discovery behavior.

**L1:** reachability; state; HA; recovery.

**L2:** metrics; units; counters; performance behavior.

**L3:** baseline; anomaly; forecast where supported; service impact; RCA features where applicable, without treating model output as causal authority.

**Security:** minimum privileges; credential handling; secret protection; TLS; tenant isolation.

**Diagnostics:** each declared diagnostic action/workflow proves target scoping, supported-version behavior, safe/read-only versus privileged classification, parameter bounds, normalized result/error mapping, timeout/cancellation, credential handling, audit, resource budget and negative cases. Multi-vantage-capable diagnostics prove independent source provenance and contradiction preservation. A technology is not certified for a diagnostic merely because the generic Platform diagnostic engine exists.

**SNMP where declared:** v1/v2c/v3 security compatibility; selected-Sensor credential/preflight; exact Counter64; Counter32 wrap; reboot/reset/discontinuity; irregular sample interval; scalar/table/index mapping; noSuch/partial-object behavior; GETBULK tooBig/fallback; ifIndex/interface reconciliation; trap/inform source/security/storm behavior; Inform durable local acceptance before ACK; diagnostics; and read-only monitoring versus privileged SET separation.

**eBPF process-network evidence where declared:** socket lifecycle monitoring, local aggregation around `process/service + remote endpoint + protocol/port + time bucket`, connection metrics (active, new, closed, bytes TX/RX, duration, RTT, retransmissions), explicit drop/shed accounting, least-privilege capability execution (`CAP_BPF`/`CAP_PERFMON`), verified BTF/CO-RE compatibility, and explicit capability states (`ACTIVE`, `UNSUPPORTED`, `UNAVAILABLE`, `PRIVILEGE_DENIED`, `KERNEL_INCOMPATIBLE`, `RESOURCE_GUARDED`).

**Active path observability where declared:** flow-stable active path tracing, hop sequence metrics (RTT min/mean/max, packet loss, jitter), path fingerprint stability, divergence detection, middlebox/hidden-hop tolerance, and multi-vantage contradiction preservation.

**Failure:** timeout; malformed response; API rate limit; partial response; stale telemetry; expired/invalid credentials.

**Upgrade:** supported versions; API schema change; migration; rollback/deprecation handling.

**Resource:** CPU; memory; network; API-call budget; telemetry cardinality.

Customer Acceptance Profile results must be evidence-backed. A pack may not be advertised as certified merely because this master lists the technology family.

---

## 23. Formal traceability requirements

Each `REQ-EMC-*` owns the corresponding `PRD-EMC-*`. Implementation status for all rows at this document's creation is **specified / not implemented**. Certification expectation is **dimensional evidence per §5 and §22**; no family is certified by declaration.

| REQ ID | Owning PRD | Affected logical layers | Required evidence |
|---|---|---|---|
| `REQ-EMC-001` | `PRD-EMC-001` | PRD, Architecture, Quality | Dimensional matrix per pack with independent Discovery/L1/L2/L3/Topology/NCM/Diagnostics/Security/version states; no scalar collapse |
| `REQ-EMC-002` | `PRD-EMC-002` | Architecture, HLD, LLD | Single Capability Pack contract; Device Pack remains a classification; no second pack framework or extra Core service/database |
| `REQ-EMC-003` | `PRD-EMC-003` | LLD, Quality, Operations | Machine-readable compatibility records, including diagnostics capability state where declared, plus version-range tests and evidence references |
| `REQ-EMC-004` | `PRD-EMC-004` | PRD, Architecture, Quality | Middleware/application-platform packs with collector-health separation and no bytecode APM implication |
| `REQ-EMC-005` | `PRD-EMC-005` | PRD, Architecture, Quality | Relational and NoSQL/data-platform packs with canonical entities and dimensional certification |
| `REQ-EMC-006` | `PRD-EMC-006` | PRD, Architecture, Quality | Backup/data-protection family with canonical job states and restore-test evidence distinct from job success |
| `REQ-EMC-007` | `PRD-EMC-007` | Architecture, HLD, Quality | AWS/Azure/GCP/OCI normalized contracts; vendor IDs do not override tenant/entity authority |
| `REQ-EMC-008` | `PRD-EMC-008` | PRD, Architecture | SaaS Monitoring Pack class cross-referenced to `Platform_19`; no duplicate OIV mechanics |
| `REQ-EMC-009` | `PRD-EMC-009` | PRD, Architecture, Quality | Scheduled/batch job canonical model, states and SLA-window risk metrics |
| `REQ-EMC-010` | `PRD-EMC-010` | PRD, Security, Quality | Optional EUC/VDI packs with privacy tests proving prohibited surveillance fields are absent |
| `REQ-EMC-011` | `PRD-EMC-011` | Architecture, Security, Quality | Integration Packs using `Platform_10` mechanics; retry/idempotency/audit/RBAC; no LLM privileged execution |
| `REQ-EMC-012` | `PRD-EMC-012` | Architecture, LLD, Quality | Operational Graph vs governed CMDB semantics on one graph technology; provenance/confidence/age/provisional state exposed |
| `REQ-EMC-013` | `PRD-EMC-013` | Architecture, HLD, Quality | Typed/redundant/quorum/weighted service dependencies; health ≠ worst child |
| `REQ-EMC-014` | `PRD-EMC-014` | PRD, Capacity, Quality | Named retention profiles with sizing validation before activation; no hard-coded customer contract days |
| `REQ-EMC-015` | `PRD-EMC-015` | PRD, Quality, Operations | Customer Acceptance Profiles with evidence-backed results; targets are profile values, not universal guarantees |
| `REQ-EMC-016` | `PRD-EMC-016` | Architecture, Quality, Operations | Pack version range, upgrade/migration, rollback, deprecation and EOL evidence |
| `REQ-EMC-017` | `PRD-EMC-017` | Architecture, Security | OEM/customer extensions via pack/SDK only; Core forks prohibited |
| `REQ-EMC-018` | `PRD-EMC-018` | Quality, Operations | Distinct Architecture/Specification/Implementation/Pack/Certification/Integration coverage metrics |
| `REQ-EMC-019` | `PRD-EMC-019` | PRD, Architecture, HLD, Quality | Separate Agent/agentless certification evidence for Linux/Windows with normalized semantic mapping and explicit coverage differences |
| `REQ-EMC-020` | `PRD-EMC-020` | PRD, HLD, LLD, Quality | Server fixtures prove CPU/pressure, memory/swap, filesystem/inodes, block I/O, NIC, process, service, OS and applicable event/hardware coverage |
| `REQ-EMC-021` | `PRD-EMC-021` | Architecture, Security, Quality | Privilege/credential/path/collector failures map to missing/degraded evidence; negative tests prevent false host/service-down assertion |
| `REQ-EMC-022` | `PRD-EMC-022` | Architecture, LLD, Quality, Capacity | Capability Pack manifest/certification distinguishes continuous collectors from EDS Diagnostic Job profile diagnostics and records version/privilege/evidence/resource support; no second diagnostics framework |
| `REQ-EMC-023` | `PRD-EMC-023` | PRD, Capability Pack, Quality | Approved 16-profile Phase-1 target baseline is machine-readable and visibly target/not-certified until dimensional evidence exists |
| `REQ-EMC-024` | `PRD-EMC-024` | PRD, HLD, UI, Quality | OOB acceptance proves identification/preflight/discovery/inventory/entities/relationships/default monitoring/dashboards/alerts/diagnostics/limitations/provenance/upgrade behavior as applicable |
| `REQ-EMC-025` | `PRD-EMC-025` | Architecture, Quality, Operations | Support claims expose independent Discovery/L1/L2/L3/Topology/Diagnostics/NCM/Security/product-version states from one compatibility source |
| `REQ-EMC-026` | `PRD-EMC-026` | PRD, Quality | Second-wave list remains Phase-1.5 roadmap/certification target and cannot be marketed as supported without evidence |

---

## 24. Implementation sequencing (specified, not executed)

This PR performs **EMC-0** only.

| Stage | Scope |
|---|---|
| **EMC-0** — Specification/Product Model | This master; maturity model; pack schema; certification lifecycle; compatibility model; RCR model |
| **EMC-1** — Capability Pack Runtime / SDK | loader/registry; schema validation; versioning; credentials; discovery; telemetry/entity/event mapping; health; test harness |
| **EMC-2** — High-value families | network; server; database; VMware; Kubernetes; middleware; cloud |
| **EMC-3** — Enterprise operational families | backup; M365; scheduled jobs; security SaaS; optional EUC |
| **EMC-4** — Integration certification | ITSM; SIEM; EDR; vulnerability management; APM; CMDB; Teams/collaboration |

---

## 25. Explicit deferrals and exclusions

| Item | Disposition |
|---|---|
| Always-on full DPI | Deferred |
| Unrestricted SIEM replacement | Excluded |
| Full bytecode APM | Excluded; integrate instead |
| Continuous profiling / flame graphs | Excluded |
| Method-level code instrumentation | Excluded |
| Broad BMS/DCIM | Deferred |
| Broad FinOps | Deferred |
| EUC/VDI | Optional / lower priority |
| Vendor-specific pack implementation | Future pack work (EMC-1+) |
| Uncontrolled automated NCM | Prohibited |
| Core forks for customer extensions | Prohibited |
| New Core service or database for packs | Prohibited by default |
| Independent Technology Pack framework | Prohibited |

---

## 26. Founder-reserved decisions

Founder-reserved decisions remain pricing, licensing, commercial SKUs, contractual/customer concessions, contractual SLO/RPO/RTO/retention commitments, legal/regulatory/IP matters, external certification/compliance claims, external customer/partner/investor commitments, and material spending/vendor commitments.

ChatGPT / Platform technical authority governs product architecture, HA topology, databases, services, APIs, security architecture, deployment architecture, technical sequencing, and implementation and release engineering.

The v1 exactly-two-Core Active/Standby topology remains a current fixed Mother Reference decision. It is not casually changed or reopened. Changes to that technical topology are governed by deliberate Mother Reference architecture change control and technical evidence. Founder escalation is required only if a proposed topology change materially changes Founder-reserved business intent, commercial commitments, contractual promises, material spending/vendor commitments, or external commitments.

---

## 27. Implementation-status disclaimer

At creation, this document is EMC-0 requirements/architecture authority only. It does not assert that any `PRD-EMC-*` or `REQ-EMC-*` capability has been implemented, benchmarked, pack-certified, customer-certified or released. Implementation status must be established from exact source, executable tests, CI/runtime evidence, pack certification evidence and release records.

## Final handoff baseline note — 2026-09-21 (Reconciled 2026-09-30)

`Platform_23` remains the highest specialized Mother Reference. The 2026-09-21 architecture review determined **SPECIALIZED MASTER NOT REQUIRED AT THIS BASELINE** for Flow, Logs, Cloud, Extensibility or ITSM; those domains strengthen existing owners. The 2026-09-30 reconciliation (Issue #142) reaffirmed this boundary for Unified Network & Infrastructure Observability Phase 1: eBPF process-network telemetry and active path observability integrate into existing Agent/Sensor/Core and Capability Pack architecture without creating a new specialized master or competing framework. Capability Pack remains the single extension architecture. Dimensional certification and `SPECIFIED != IMPLEMENTED != CERTIFIED` classification remain mandatory.

---

## Phase-1 server inventory and Fast-RCA coverage reconciliation — Issues #169/#170

For supported Windows/Linux server/VM/endpoint certification, coverage now also records:

- installed-software inventory source coverage and authoritative reconciliation support under `REQ-INF-014`;
- customer-selectable process/service monitoring selector coverage under `REQ-INF-015`;
- Criticality/effective Monitoring Profile behavior under `REQ-INF-016`;
- trigger-specific EDS definition support under `PRD-MIA-059`;
- cross-vantage EDS/OOB/virtualization/storage/path evidence support under `PRD-MIA-060`.

Software inventory certification is source-specific. A Linux distro/package manager, Windows source, Agent or agentless mode is not certified merely because another source is supported. Pack/source declarations must state native source, supported versions, required privilege, fields, limits, normalization behavior, expected partial/failure modes and test evidence.

Process/service certification must prove stable selector semantics and PID-reuse safety, not merely enumerate a process list.

EDS certification remains dimensional per definition and OS/version/privilege. Listing `EDS_CPU_PRESSURE` or another definition in a pack does not certify that definition until its evidence/resource/security/negative-path matrix passes.

Physical and virtualization Fast-RCA coverage is multi-vantage: endpoint, OOB, hypervisor, storage and network evidence keep separate provenance. A technology pack may declare the evidence it can provide but does not become causal authority.


---

## 29. v3.3.0 Official Certified Capability Pack baseline and OOB acceptance

### 29.1 Phase-1 certification targets — not current certification claims

The approved first certification wave is:

1. Cisco IOS / IOS-XE
2. Cisco NX-OS
3. Juniper Junos
4. Arista EOS
5. Fortinet FortiOS
6. Palo Alto PAN-OS
7. Linux — RHEL and Ubuntu, with Agent and SSH/agentless dimensions separately certified
8. Windows Server — Agent and WinRM/WMI/CIM dimensions separately certified
9. Windows Infrastructure — AD/Domain Controller, replication, DNS, DHCP, IIS and certificate health where applicable
10. Generic Redfish — with explicit server-vendor/model/version compatibility dimensions
11. VMware vSphere
12. Kubernetes
13. PostgreSQL
14. Microsoft SQL Server
15. AWS
16. Azure

Every item is a **certification target only**. Until the relevant pack/runtime/test evidence exists, status remains specified/not-certified. No vendor name may imply all dimensional capabilities.

### 29.2 Official Certified Pack OOB acceptance contract

Where applicable, an Official Certified Pack/profile must define and certify:

1. identification and stable technology/device identity;
2. exact vendor/product/version compatibility and known limitations;
3. authentication/least-privilege requirements and preflight;
4. discovery and duplicate-safe reconciliation;
5. inventory;
6. canonical metrics/state/events;
7. canonical entities and relationships;
8. health model and monitoring profile;
9. safe default collection/adaptive behavior;
10. out-of-box dashboards/views;
11. baseline alert/qualification rules with evidence semantics;
12. anomaly/baseline inputs where supported;
13. diagnostics and troubleshooting actions;
14. RCA/topology/path/service semantics where supported;
15. MWU/resource/capacity cost metadata;
16. data-quality/freshness/partial/failure expectations;
17. provenance, pack version, Platform compatibility, upgrade/migration/rollback behavior;
18. known-good fixtures and negative/certification tests.

The broader §6 Capability Pack declaration remains authoritative; this list defines customer OOB acceptance, not a second schema or registry.

### 29.3 Dimensional certification

At minimum expose independent states for **Discovery, L1, L2, L3/routing where applicable, Topology, Diagnostics, NCM, Security and supported product/version**. A vendor/family may be certified in some dimensions and unsupported/experimental in others. UI/API/documentation support matrices are generated from the same machine-readable compatibility authority; independent marketing tables are prohibited.

### 29.4 Phase-1.5 second-wave certification roadmap

Evidence-dependent targets include GCP; Aruba/HPE, Dell networking, Extreme, Check Point, F5 and vendor WLAN/SD-WAN; Hyper-V, Nutanix, Proxmox; NetApp, Dell EMC, HPE/Pure Storage and Cisco MDS/FC fabric; Oracle, MySQL/MariaDB, SAP HANA, MongoDB, ClickHouse and OpenSearch; Apache, NGINX, Tomcat, JBoss/WildFly, WebLogic, WebSphere, JVM, .NET and Node.js; Kafka, RabbitMQ, ActiveMQ and Redis; Microsoft 365 and demand-justified backup products. Exact vendor/product/version/dimension scope remains certification work and is not a GA promise merely by appearing here.

### 29.5 Integration Pack certification

ITSM/CMDB adapters remain `INTEGRATION` Capability Packs using Platform_10 mechanics. Connector certification records exact deployment variant/edition/API/auth plus supported Incident/Problem/Change/Approval/activity/reverse-event/CMDB/relationship/business-service capabilities and limitations. Phase-1 connector priorities are ServiceNow, Jira Service Management, BMC Helix and ManageEngine ServiceDesk Plus/MSP; six Phase-1.5 certification-priority connectors are defined by `REQ-INT-013`.

---

## Phase 2 Enterprise RFP Readiness Reconciliation — Issue #269

### Founder-approved phase outcome

This section is the authoritative specialized-master contract for Issue #269.

**Phase 1** remains the frozen Network & Infrastructure Observability core.

**Phase 1.5 — Enterprise Foundation** adds/finishes: exactly-two-Core HA and MSP certification; Integration Fabric and governed CMDB reconciliation; bounded sampled operational OTel spans/service-edge evidence; NCM Change Assurance; first-class serverless; and selected broader Capability/Integration Packs required to unlock enterprise operation.

**Phase 2 — Enterprise ITOM/AIOps RFP Readiness** is a product/certification outcome, not a list of vendor names. By Phase-2 exit, AmrudAi shall be able to satisfy recurring large-enterprise monitoring/ITOM/AIOps requirement classes either **Native — Certified** or **Integrated — Certified**, with full native APM and full native DPI as deliberate exceptions.

**Phase 2.5+** owns full native APM and full native always-on DPI/deep packet analytics.

### Formal Phase-2 Enterprise Customer Requirements

| PRD ID | Phase | Requirement |
|---|---|---|
| `PRD-ECR-001` | P2 | Platform shall achieve Enterprise ITOM/AIOps RFP readiness across heterogeneous large-enterprise estates and pass `ECRP-P2-v1`; specification or vendor listing alone is insufficient. |
| `PRD-ECR-002` | P1.5 | Enterprise Foundation shall include the existing HA/MSP envelope plus Integration Fabric/CMDB reconciliation, bounded operational OTel span/dependency evidence, NCM Change Assurance, serverless and selected enabling packs/integrations. |
| `PRD-ECR-003` | P2 | Platform shall provide certified heterogeneous infrastructure coverage including declared enterprise Linux/Windows/Unix modes, virtualisation/HCI, hardware/OOB, storage/SAN and dependency/topology evidence through Capability Packs. |
| `PRD-ECR-004` | P2 | Platform shall provide enterprise database monitoring depth including Oracle-class estates (including RAC/Exadata where interfaces permit) plus PostgreSQL, SQL Server, MySQL/MariaDB, SAP HANA, MongoDB and other approved data-platform packs, with sessions/waits/locks/deadlocks/replication/storage/performance/change evidence and no autonomous tuning requirement. |
| `PRD-ECR-005` | P2 | Platform shall provide certified middleware/application-platform and batch/scheduler monitoring through Capability Packs for high-demand enterprise technologies using supported APIs/JMX/metrics/logs/synthetics; full bytecode APM is not implied. |
| `PRD-ECR-006` | P2 | Backup/Data Protection shall be a production-certifiable monitoring family covering job outcome, protected-asset coverage, SLA/RPO exposure, repository/media capacity, replication and restore-test/recoverability evidence where exposed. |
| `PRD-ECR-007` | P1.5/P2 | Multi-cloud coverage shall be materially certifiable across AWS, Azure, GCP and OCI; serverless targets include Lambda, Azure Functions, Google Cloud Functions and Cloud Run. Provider/API failure remains monitoring-degraded evidence, not resource DOWN. |
| `PRD-ECR-008` | P2 | Platform shall provide a Regulated Enterprise Deployment Profile for on-prem/private/air-gapped operation, explicit data-egress control, customer PKI/KMS/HSM integration where supported, offline signed updates, auditable privileged support, tamper-evident audit, governed immutable/WORM archive integration where required, legal hold, retention, backup/restore and DR evidence. |
| `PRD-ECR-009` | P2 | Integration Packs shall provide certified coexistence with enterprise monitoring, APM, NPM/DPI, SIEM/SOC, CMDB, ITSM and change/deployment systems using normalized Platform contracts, connector health and provenance. |
| `PRD-ECR-010` | P2 | Enterprise AIOps readiness shall include service mapping, event normalization/correlation, evidence qualification, noise reduction, anomaly/baseline intelligence, deterministic/revisioned RCA, change intelligence, Flight Recorder, Similar Incident Memory, predictive operations only where calibrated, recommendations and controlled remediation with audit/rollback safety. |
| `PRD-ECR-011` | P2 / P2.5+ | Phase 2 shall support OTel metrics/logs, bounded sampled operational spans/service edges, API/synthetics, application/service dependency correlation and external APM integration. Full native APM — bytecode/auto-instrumentation, method/line-level diagnostics, continuous profiling, full RUM/session replay and full APM backend — is Phase 2.5+. |
| `PRD-ECR-012` | P2 / P2.5+ | Phase 2 shall support Flow, active-path intelligence, eBPF process-network evidence, bounded incident-triggered packet evidence and external NPM/DPI integration. Full native always-on DPI — continuous payload capture, TLS decryption, deep L7 reconstruction/packet warehouse/NDR-style DPI — is Phase 2.5+. |
| `PRD-ECR-013` | P2 exit | `ECRP-P2-v1` shall certify representative end-to-end enterprise operation, technology/version coverage, native/integrated evidence, security/tenant isolation, regulated data boundaries, HA/DR, scale/cardinality, retention/archive, failure/recovery, upgrade compatibility and NOC workflows. |

### Phase-2 target families

Minimum target families for certification planning include:
- enterprise Windows/Linux plus demand-backed Unix such as AIX and Solaris;
- VMware, Hyper-V, Nutanix and demand-backed KVM/Proxmox;
- Redfish/iDRAC/iLO/CIMC/IPMI hardware/OOB;
- storage arrays, SAN/FC fabrics, pools/volumes/LUNs/controllers/replication;
- Oracle/RAC/Exadata, PostgreSQL, SQL Server, MySQL/MariaDB, SAP HANA, MongoDB and approved data platforms;
- WebLogic, WebSphere, JBoss/WildFly, Tomcat, IIS, Apache/Nginx, Kafka, IBM MQ, RabbitMQ, API gateways and demand-backed enterprise middleware;
- Veeam, Commvault, NetBackup and demand-backed backup/data-protection products;
- AWS, Azure, GCP and OCI, including the declared serverless targets;
- external monitoring/APM/NPM-DPI/SIEM/CMDB/ITSM/change/deployment integrations.

These are certification targets, not implementation claims. Exact versions and capability dimensions live in the machine-readable compatibility/certification source.

### ECRP-P2-v1

The certification profile must exercise a representative multi-DC/DR/branch/hybrid-cloud estate and prove:
1. discovery/onboarding/identity/CMDB reconciliation;
2. network, host, virtualisation, storage, database, middleware, backup, cloud and declared serverless monitoring;
3. Flow/log/OTel and external observability evidence ingestion;
4. service/dependency topology and service-impact analysis;
5. No Data/connector failure/credential failure/path failure separation from target DOWN;
6. event→alert→incident compression and evidence-qualified states;
7. deterministic/revisioned RCA with contradictions/missing evidence and `INCONCLUSIVE`;
8. ITSM/notification/integration delivery and recovery;
9. regulated data-boundary, retention/archive/legal-hold and support-access controls;
10. HA/DR, replay, upgrade/rollback and connector/pack compatibility;
11. enterprise-scale query/dashboard/report/RCA concurrency and noisy/cardinality cases;
12. end-to-end operator workflows and auditable evidence.

### Formal traceability supplement

| REQ ID | PRD ID | Governing capability |
|---|---|---|
| `REQ-ECR-001` | `PRD-ECR-001` | Enterprise RFP readiness outcome |
| `REQ-ECR-002` | `PRD-ECR-002` | Phase-1.5 Enterprise Foundation |
| `REQ-ECR-003` | `PRD-ECR-003` | Heterogeneous infrastructure/storage depth |
| `REQ-ECR-004` | `PRD-ECR-004` | Database performance monitoring |
| `REQ-ECR-005` | `PRD-ECR-005` | Middleware/batch monitoring |
| `REQ-ECR-006` | `PRD-ECR-006` | Backup/data protection monitoring |
| `REQ-ECR-007` | `PRD-ECR-007` | Multi-cloud/serverless breadth |
| `REQ-ECR-008` | `PRD-ECR-008` | Regulated-enterprise profile |
| `REQ-ECR-009` | `PRD-ECR-009` | Enterprise integration ecosystem |
| `REQ-ECR-010` | `PRD-ECR-010` | Enterprise AIOps maturity |
| `REQ-ECR-011` | `PRD-ECR-011` | Phase-2 APM integration/native boundary |
| `REQ-ECR-012` | `PRD-ECR-012` | Phase-2 DPI integration/native boundary |
| `REQ-ECR-013` | `PRD-ECR-013` | ECRP-P2-v1 certification gate |

### Non-claims

This roadmap is **SPECIFIED** only. It does not claim any listed technology, connector, regulated profile, AIOps feature or ECRP profile is implemented, tested, benchmarked, certified or deployed. Full native APM and full native DPI remain excluded from Phase 2 even if external evidence is integrated.

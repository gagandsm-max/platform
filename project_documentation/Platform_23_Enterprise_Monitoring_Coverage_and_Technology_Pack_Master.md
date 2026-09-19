# Platform — Enterprise Monitoring Coverage, Capability Packs and Real-Customer Certification Master

**Document ID:** PLATFORM-EMC-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Specialized Master**
**Product:** Platform
**Change class:** Additive v3.2.2 product/engineering requirements; EMC-0 specification only. No runtime implementation, vendor-pack shipment, certification achievement or contractual guarantee is claimed by this document.

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
- Flow exporter/protocol/version and enterprise-IE mappings where applicable
- Flow template/sampling/sequence compatibility hints where applicable
- application/QoS/NAT/interface/VRF semantic mappings where applicable
- Flow redundancy/failover test profile where applicable

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

### 8.2 Servers / operating systems

Windows; RHEL; Ubuntu; SLES; Oracle Linux; AIX; other certified Unix/Linux.

Certification **must** separately record:

- agentless/remote monitoring certification
- Agent-based certification

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

Native initial capability includes: NetFlow; IPFIX; sFlow; J-Flow/vendor flow; exporter/template/sampling/data-quality management; endpoint/system/domain/user/site/Geo/ASN/cloud enrichment where evidence exists; exported application classification; top talkers; conversations; site matrices; QoS analysis; trends; operational anomalies; capacity analysis.

Production Flow collection runs on the dedicated FLOW Sensor role defined by the base architecture. Flow Capability/Device Packs may declare vendor-specific exporter identification, information-element mappings, enterprise IEs, application/QoS/NAT semantics, template quirks, sampling semantics and certification fixtures, but they do not create a second Flow engine or bypass source admission, tenant identity, privacy, replay or data-quality contracts.

Certification is dimensional. A vendor/device can be certified for basic Flow export without being certified for application identity, NAT, QoS, user mapping, enterprise IEs, dual-collector failover or every firmware version.

Cloud-native VPC/network Flow logs may later enter through Cloud Capability Packs and normalize into the same canonical Flow model where source semantics permit. They are not a separate NTA product.

Flow endpoint/user/domain/Geo enrichment preserves provenance/confidence. AD/LDAP/Entra directory data alone does not prove a current user/IP session; DNS does not prove full URL; public-IP Geo does not prove a person's precise location.

Bounded Triggered Packet Evidence remains permitted under `Platform_19`. Always-on full DPI remains deferred. This master does not reopen full DPI or employee web-content surveillance.

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

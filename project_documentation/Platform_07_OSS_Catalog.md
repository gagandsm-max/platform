# Platform — Authoritative Open-Source Software Catalogue

**Document ID:** PLATFORM-OSS-001
**Version:** 3.3.0
**Status:** Authoritative Mother Reference — Dependency Allow-List Authority (**A2.3 NORMALIZED_V1**)
**Product name:** Platform (internal engineering/build identifier) / AmrudAi (commercial/customer-facing product name)
**Governing documents:** `Platform_01_BRD.md`, `Platform_02_PRD.md`, `Platform_03_Architecture.md`
**Companions:** `Platform_04_HLD.md`, `Platform_05_LLD.md`, `Platform_08_Requirements_Traceability.md`, `Platform_12_Decision_Source_Register.md`
**Normalization state:** NORMALIZED_V1
**Last reconciliation:** 2026-11-08 — rebaseline verification against canonical main `92f0fb37805c234be2869619abcc01ad26b5f30b` after Issue #302/PR #303; no OSS catalogue change from that Agent custody repair. Prior 2026-11-06 OSS-124 source-implementation truth correction (then main `36e1513f25265d0bcd45bdfe5156ee02fdbf5238`), 2026-10-08 engineering admission (Issue #168 / Master comment `6059162933`) and 2026-09-24 A2.3 normalization / OSS-008→REQ-OSS-008 migration remain unchanged provenance. No new dependency, source pin or release certification.
**Important:** This is an engineering/licensing governance baseline, not legal advice. Exact pinned versions and their complete transitive/bundled licenses must be scanned before release.

**Maturity:** `SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED`. Architectural approval (`Decision`) is not implementation evidence (`Truth state`).

---

## 1. Purpose

Platform is a clean-room proprietary commercial product that deliberately reuses mature permissively licensed protocol engines, SDKs, data infrastructure and UI libraries instead of rebuilding commodity functions. Platform must build its differentiated logic — canonical entity graph, Device Pack model, MIB/API Studios, monitoring scheduler, Sensor placement, policy hierarchy, false-alert qualification, incident correlation, RCA/GNN intelligence, NCM governance, IPAM authority, Dashboard/Report/Map Studios, multi-tenancy, audit semantics and licensing — as Platform-owned code.

The rule is **libraries and infrastructure over nested products**: use mature OSS to avoid reinventing SNMP, Kafka, database engines, SSH, chart rendering, etc.; do not embed another complete NMS/IPAM/dashboard product as Platform's authority.

**Namespace note:** Catalogue IDs use the `OSS-NNN` form (for example catalogue **OSS-008 = ClickHouse**). Formal Mother Reference requirements use `REQ-*` IDs. The former formal air-gap requirement ID `OSS-008` was migrated to **`REQ-OSS-008`** in A2.3 to end the catalogue/requirement namespace collision. Historical prose may mention the old formal ID only when explicitly marked migrated/historical.


### 1.0 Normative requirements — `REQ-OSS` family


This subsection is the **primary normative owner contract** for the `REQ-OSS` family in this document. Each row is an implementation-binding SHALL statement for this owner; it is not an RTM index, ownership footer or summary-only catalogue.

**Shared normative envelope:** every Requirement ID in this subsection inherits the parent section's concrete feature/behavior contract (purpose, persona, dependencies, preconditions, primary workflow, failure/degraded behavior, acceptance outcome, maturity boundary). Implementation writers MUST use this envelope plus the distinctive SHALL below; the RTM one-liner alone is not a sufficient contract.

**Parent-section obligations incorporated by reference:** Parent section obligations for purpose, workflow, failure/degraded behavior, authorization/tenant boundary, acceptance outcome and maturity boundary.

| Requirement ID | Normative SHALL contract | Parent-section envelope | Boundary / non-goal |
|---|---|---|---|
| `REQ-OSS-001` | Platform SHALL enforce: Clean-room Platform independent of ThoughtData and NetAI. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-OSS-002` | Platform SHALL enforce: Only approved OSS catalogue dependencies may ship. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-OSS-003` | Platform SHALL enforce: Green permissive licenses generally allowed; Yellow review; Red blocked by default. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-OSS-004` | Platform SHALL enforce: SBOM SPDX + CycloneDX, notices, license texts, provenance/checksums every release. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-OSS-005` | Platform SHALL enforce: Transitive dependency license/CVE scan mandatory. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-OSS-006` | Platform SHALL enforce: Vendor MIB/YANG/SDK/API examples/icons/templates require provenance/redistribution rights. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-OSS-007` | Platform SHALL enforce: Customer-supplied MIBs may be used for that customer but not automatically redistributed. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |



## 2. Decision labels and truth states

### 2.1 Architectural decision labels (allow-list intent)

- `USE`: approved architectural dependency, subject to pinned-version scan.
- `USE/WRAP`: approved, but must be hidden behind a Platform adapter/interface.
- `CANDIDATE`: technically suitable but not frozen until P0 benchmark/architecture review.
- `YELLOW/CANDIDATE`: top-level license may be acceptable but transitive/driver licensing requires mandatory review.
- `OPTIONAL` / `OPTIONAL-LATER` / `OPTIONAL/DEV`: not part of mandatory current footprint.
- `USE-TOOLING` / `USE-DEV`: engineering/CI/dev tool; not shipped as Platform customer runtime unless separately approved.

### 2.2 Implementation truth states (current evidence)

| Truth state | Meaning |
|---|---|
| `CURRENT_RUNTIME` | Present and used in current runtime implementation (source/manifest/compose evidence). |
| `CURRENT_BUILD_TOOLING` | Used for build/test/development; not required as customer runtime. |
| `APPROVED_NOT_IMPLEMENTED` | Architecturally approved allow-list entry, but not currently wired into the implementation. |
| `CANDIDATE` | Requires benchmark, security, licensing or architectural decision before adoption. |
| `OPTIONAL_FUTURE` | Future/optional capability; not part of the current required footprint. |

`Decision` answers “may Platform use this?”. `Truth state` answers “is it implemented now?”. Do not treat `USE` as proof of shipping.

**Current UI implementation (authoritative):** React 18 + TypeScript + Vite + plain CSS. Tailwind CSS, Radix UI, Redux, Zustand and Next.js are **not** current dependencies.

## 3. Approved/candidate catalogue

| ID | Domain | Capability | OSS / Technology | License | Decision | Truth state | Runtime | How Platform uses it | What remains custom |
|---|---|---|---|---|---|---|---|---|---|
| OSS-001 | Core Framework | Core control plane/API | Spring Boot | Apache-2.0 | USE | CURRENT_RUNTIME | Core | Java 21 modular Core framework, REST/API, security integration, configuration, lifecycle. | Do not build web/application framework plumbing. Platform-specific domain modules, policies, authorization semantics and workflows remain custom. |
| OSS-002 | Streaming | Internal durable event/telemetry backbone | Apache Kafka | Apache-2.0 | USE | CURRENT_RUNTIME | Core | Internal message backbone for normalized telemetry, events, audit, alert/RCA processing and async integration. Kafka is not exposed directly to Sensors. | Kafka is commodity streaming; Platform owns topic contracts, idempotency, routing, retention, replay semantics and two-Core failover integration. |
| OSS-003 | Streaming | Go Kafka client | franz-go | BSD-3-Clause | USE | APPROVED_NOT_IMPLEMENTED | Core/Sensor utilities | Efficient Go Kafka producer/consumer where Go components communicate with internal Kafka. | Do not write Kafka protocol client. Wrap behind Platform producer/consumer interfaces. |
| OSS-004 | Schema Registry | Schema governance | Apicurio Registry | Apache-2.0 | USE | APPROVED_NOT_IMPLEMENTED | Core | Registry for Avro/Protobuf schemas and compatibility rules. | Avoid proprietary schema-registry lock-in; Platform owns subject naming, compatibility policy and release governance. |
| OSS-005 | Schema | Event serialization | Apache Avro | Apache-2.0 | USE | APPROVED_NOT_IMPLEMENTED | Core/Schemas | Candidate/default for selected event families needing schema evolution. | Schema choice is per contract; Platform owns canonical event definitions. |
| OSS-006 | Schema | RPC/event serialization | Protocol Buffers | BSD-3-Clause / bundled notices | USE | APPROVED_NOT_IMPLEMENTED | Core/Sensor/API | Typed RPC/event contracts where protobuf/gRPC is appropriate. | Platform owns canonical schemas and compatibility rules. |
| OSS-007 | Transactional Data | Authoritative control/config database | PostgreSQL | PostgreSQL License | USE | CURRENT_RUNTIME | Core | Tenants, inventory, policies, RBAC metadata, device definitions, current state, IPAM, NCM metadata, licensing, workflow state. | Do not use PostgreSQL as the primary high-volume telemetry store. Platform owns schema and migrations. |
| OSS-008 | Telemetry Data | High-volume analytics/time-series store | ClickHouse | Apache-2.0 | USE | CURRENT_RUNTIME | Core | Metrics, flow, syslog/events, alert history, incident evidence, audit history, SLA/capacity facts and analytics. | Chosen over Elasticsearch as primary store for resource-efficient analytical telemetry. Platform owns schemas, TTLs, rollups and query planner. |
| OSS-009 | Cache | Cache/ephemeral state | Valkey | BSD-3-Clause | USE | CURRENT_RUNTIME | Core | Query cache, ephemeral coordination/state where data loss is acceptable/reconstructable. | Do not store authoritative state only in cache. |
| OSS-010 | Identity | Identity federation | Keycloak | Apache-2.0 top-level; transitive scan required | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Core | OIDC/SAML/LDAP/AD federation, authentication flows and MFA integration. | Keycloak answers identity/authentication; Platform owns tenant/resource authorization. Every distribution must scan Keycloak transitive dependencies. |
| OSS-011 | Authorization | General policy decision engine | Open Policy Agent (OPA) | Apache-2.0 | CANDIDATE | CANDIDATE | Core | Candidate for selected coarse-grained authorization/policy decisions. | Do not make OPA the owner of Platform domain authorization without a benchmark/design decision. Platform RBAC/resource-scope semantics remain custom. |
| OSS-012 | Expressions | Safe expression language | CEL (cel-java / cel-go) | Apache-2.0 | USE | APPROVED_NOT_IMPLEMENTED | Core/Sensor | Threshold formulas, dynamic groups, constrained policy expressions and safe user-defined conditions. | Use non-Turing-complete CEL rather than arbitrary Python/JavaScript in customer policy paths. Platform defines allowed variables/functions/cost limits. |
| OSS-013 | Data Interchange | Columnar in-memory interchange | Apache Arrow | Apache-2.0 with bundled third-party notices | OPTIONAL | OPTIONAL_FUTURE | Core/Analytics | Efficient analytical interchange where it materially reduces copies/serialization. | Only introduce where benchmark justifies it. |
| OSS-014 | Search | Optional dedicated full-text search | OpenSearch | Apache-2.0 | OPTIONAL-LATER | OPTIONAL_FUTURE | Core | Add only if customer log search requirements exceed ClickHouse text-search capability. | Not mandatory in V1 to keep footprint low. Elasticsearch is not the core store. |
| OSS-020 | Network Collection | SNMP v1/v2c/v3 | GoSNMP | BSD-style | USE/WRAP | CURRENT_RUNTIME | Sensor | SNMP Get/GetNext/GetBulk/Walk plus protocol support needed by certified traps/informs and privileged SET actions. | Do not reimplement BER/SNMP. Platform owns scheduling, Credential Profile resolution, Poll Planner, typed normalization/exact counters, validation, retry, trap storm/source trust, qualification and policy. Routine monitoring remains read-only; library SET capability does not authorize monitoring-time SET. |
| OSS-021 | Network Collection | MIB parsing | gosmi | MIT | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Core Studio/Sensor build tooling | Parse SMIv1/v2 MIBs for MIB Studio, OID metadata, textual conventions, enums, tables/indexes and notifications. | Platform owns provenance/licensing, dependency workflow, semantic transforms, sample-device validation, Capability/Device Pack generation and redistribution controls. Approved OSS status does not mean MIB Studio/runtime wiring is already implemented. |
| OSS-022 | Network Collection | NetFlow/IPFIX/sFlow parsing | GoFlow2 | BSD-3-Clause | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Streaming Sensor | Decode NetFlow v5/v9, IPFIX and sFlow. | Platform owns enrichment, canonical flow schema, cardinality controls, Kafka publishing and analytics. |
| OSS-023 | Network Collection | ICMP | golang.org/x/net/icmp | BSD-3-Clause | USE | CURRENT_RUNTIME | Sensor/Agent | ICMP echo and low-level packet operations. | Platform owns scheduling, statistics, validation workflows, jitter/loss aggregation and path correlation. |
| OSS-024 | Network Collection | DNS protocol | miekg/dns | BSD-style | USE | APPROVED_NOT_IMPLEMENTED | Sensor/Agent | Detailed DNS synthetic checks where standard resolver APIs are insufficient. | Platform owns test definitions, validation and SLA semantics. |
| OSS-025 | Network Collection | SSH | golang.org/x/crypto/ssh | BSD-3-Clause | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Sensor | SSH transport for CLI, discovery, diagnostics and NCM. | Platform owns credential brokering, command controls, session audit and vendor behavior. |
| OSS-026 | Network Collection | CLI/NETCONF automation | scrapligo | MIT | CANDIDATE/USE | CANDIDATE | Sensor | Go network CLI/NETCONF automation where it fits Sensor runtime. | Wrap behind Device Pack adapter; avoid lock-in to vendor/platform naming. |
| OSS-027 | Network Collection | gNMI | OpenConfig gNMI | Apache-2.0 | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Sensor | gNMI Get/Set/Subscribe protocol and reference client components. | Platform owns subscription policy, normalization, capacity controls and graph semantics. |
| OSS-028 | Network Collection | YANG parsing | OpenConfig goyang | Apache-2.0 | USE | APPROVED_NOT_IMPLEMENTED | Core Studio/Sensor | YANG model parsing for Device Pack/YANG support. | Platform owns YANG Studio workflow, provenance and mapping. |
| OSS-029 | Network Collection | YANG model utilities | OpenConfig ygot | Apache-2.0 | USE | APPROVED_NOT_IMPLEMENTED | Core Studio/Sensor | YANG-centric validation/marshal/unmarshal utilities. | Platform owns canonical mapping and compatibility policy. |
| OSS-030 | API Connectors | OpenAPI parser/validator | kin-openapi | MIT | USE | APPROVED_NOT_IMPLEMENTED | Core Studio/Sensor | OpenAPI/Swagger import, validation and schema handling for API Studio. | Platform owns endpoint discovery, field/metric/entity inference, chained-call orchestration and Device Pack generation. |
| OSS-031 | Logs/Events | Log/syslog collection helper | Fluent Bit | Apache-2.0 | CANDIDATE/INTEGRATE | CANDIDATE | Streaming Sensor/Agent | Use as an integration/collection component where appropriate for log ingestion and parsing. | Do not make Fluent Bit the Platform event model. Platform owns normalization, cardinality, retention and alert/RCA semantics. |
| OSS-040 | Infrastructure | Windows WinRM | masterzen/winrm | Apache-2.0 | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Infrastructure Sensor | WinRM command/collection plumbing for Windows agentless monitoring. | Platform owns secure policy, metrics, credentials, diagnostics and retries. |
| OSS-041 | Infrastructure | Host metrics helpers | gopsutil | BSD-3-Clause | USE | APPROVED_NOT_IMPLEMENTED | Agent/Sensor | Cross-platform host process/CPU/memory/disk/network primitives where suitable. | Use native OS APIs where required for accuracy; Platform owns schema and collection policy. |
| OSS-042 | Hardware | Redfish/Swordfish | Gofish | BSD-3-Clause | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Infrastructure Sensor | Redfish/Swordfish hardware inventory/health collection. | Platform owns normalized hardware entity model and vendor packs. |
| OSS-043 | Virtualization | VMware vSphere | govmomi | Apache-2.0 | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Infrastructure Sensor | vCenter/ESXi inventory, topology and metrics via vSphere APIs. | Platform owns graph, monitoring policy, dashboards and alerts. |
| OSS-044 | Containers | Kubernetes client | client-go | Apache-2.0 | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Infrastructure/Cloud Sensor | Kubernetes API, inventory, events and workload discovery. | Platform owns canonical K8s graph and correlation. |
| OSS-045 | Observability | OpenTelemetry / OTLP | OpenTelemetry Collector + contrib | Apache-2.0 | USE/INTEGRATE | APPROVED_NOT_IMPLEMENTED | Sensor/Core ingestion | Phase-1 OTLP metrics/logs ingestion and selected certified receivers/processors/exporters where appropriate; Phase-1.5 may add bounded sampled-span handling through the same governed OTel basis after benchmark/security admission. | Platform does not market full developer-centric APM; no dedicated trace database or new proprietary instrumentation protocol is selected. |
| OSS-046 | Containers | Docker/container runtime API | Moby/Docker client libraries | Apache-2.0 | CANDIDATE | CANDIDATE | Infrastructure Sensor | Container runtime inventory/metrics where Docker API is used. | Use only required client APIs and version-pin. |
| OSS-047 | Cloud | AWS API/monitoring | AWS SDK for Go v2 | Apache-2.0 | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Cloud Sensor | AWS inventory/monitoring API access. | Platform owns entity normalization and policy. Version-level dependency scan mandatory. |
| OSS-048 | Cloud | Azure API/monitoring | Azure SDK for Go | MIT / component-specific review | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Cloud Sensor | Azure inventory/monitoring APIs. | Version-level component/license scan required. |
| OSS-049 | Cloud | GCP API/monitoring | Google Cloud Go | Apache-2.0 | USE/WRAP | APPROVED_NOT_IMPLEMENTED | Cloud Sensor | GCP APIs and monitoring integrations. | Platform owns entity normalization and policy. |
| OSS-060 | NCM | Network task orchestration | Nornir | Apache-2.0 | USE/WRAP | APPROVED_NOT_IMPLEMENTED | NCM Worker | Parallel inventory/task orchestration for network automation where Python NCM workers are used. | Platform owns tenancy, approvals, job safety, scheduling and audit. |
| OSS-061 | NCM | Network CLI | scrapli | MIT | USE/WRAP | APPROVED_NOT_IMPLEMENTED | NCM Worker | Fast sync/async network device connections and command execution. | Platform owns device packs, credentials, RBAC and session policy. |
| OSS-062 | NCM | Configuration load/diff/commit | scrapli_cfg | MIT | USE/WRAP | APPROVED_NOT_IMPLEMENTED | NCM Worker | Config prepare/load/diff/commit operations. | Platform owns approval, rollback workflow, compliance and evidence. |
| OSS-063 | NCM | Multi-vendor abstraction | NAPALM | Apache-2.0 top-level; transitive review mandatory | YELLOW/CANDIDATE | CANDIDATE | NCM Worker | Use where driver coverage materially accelerates NCM. | Do not approve blindly: drivers/transitive dependencies can introduce additional license obligations. Prefer scrapli/scrapligo paths where practical. |
| OSS-064 | NCM | CLI parsing templates | NTC Templates / TextFSM | Apache-2.0 | USE/WRAP | APPROVED_NOT_IMPLEMENTED | NCM Worker/Device Packs | Reusable TextFSM parsers/templates for CLI output. | Platform must track template provenance/vendor compatibility and may override with certified Device Pack parsers. |
| OSS-065 | NCM | Configuration templating | Jinja2 | BSD-3-Clause | USE | APPROVED_NOT_IMPLEMENTED | NCM Worker | Safe parameterized configuration templates. | Platform owns template lifecycle, validation, approval and secret redaction. |
| OSS-080 | UI | Web UI framework | React | MIT | USE | CURRENT_RUNTIME | Browser | Current primary UI component framework (`ui/package.json`). | Platform owns product UX, accessibility and role-aware behavior. No embedded Grafana. Current styling is plain CSS (not Tailwind). |
| OSS-081 | UI | Charts | Apache ECharts | Apache-2.0 with bundled BSD notices | USE | APPROVED_NOT_IMPLEMENTED | Browser | Time series, bar, gauge, heatmap, histogram, Sankey and other visualization primitives. **Truth:** approved architectural primitive; not currently wired in `ui/package.json`. | Platform owns panel/query/alert semantics and dashboard builder. |
| OSS-082 | UI | Tables | TanStack Table | MIT | USE | APPROVED_NOT_IMPLEMENTED | Browser | Headless large-table features: sorting/filtering/grouping/columns. **Truth:** approved architectural primitive; not currently wired in `ui/package.json`. | Combine with server-side pagination/virtualization; do not load large estates into browser. |
| OSS-083 | UI | Server state/query caching | TanStack Query | MIT | USE | APPROVED_NOT_IMPLEMENTED | Browser | Client-side server-state caching/retry/invalidation. **Truth:** approved architectural primitive; not currently wired in `ui/package.json`. | Platform API remains source of truth. |
| OSS-084 | UI | Dashboard grid | react-grid-layout | MIT | USE | APPROVED_NOT_IMPLEMENTED | Browser | Drag/drop/resizable dashboard panels. **Truth:** approved architectural primitive; not currently wired in `ui/package.json`. | Platform owns permissions, dashboard schema, persistence, versioning and accessibility. |
| OSS-085 | UI | Topology graph rendering | Cytoscape.js | MIT | USE | APPROVED_NOT_IMPLEMENTED | Browser | Interactive graph/topology/causal graph visualization. **Truth:** approved architectural primitive; not currently wired in `ui/package.json`. | Platform owns operational graph semantics, confidence, historical topology and RCA relationships. |
| OSS-086 | UI | Geospatial maps | MapLibre GL JS | BSD-3-Clause | USE | APPROVED_NOT_IMPLEMENTED | Browser | Geo/site/region map rendering. **Truth:** approved architectural primitive; not currently wired in `ui/package.json`. | Map code license does not license map tiles/data. Tile/data provider rights and attribution are separately managed. |
| OSS-087 | UI | Browser terminal | xterm.js | MIT | USE | APPROVED_NOT_IMPLEMENTED | Browser | Terminal emulator for privileged CLI sessions. **Truth:** approved architectural primitive; not currently wired in `ui/package.json`. | Browser never receives device credentials; Platform Session Broker owns access and audit. |
| OSS-088 | UI | Styling | Tailwind CSS | MIT | OPTIONAL | APPROVED_NOT_IMPLEMENTED | Browser | Historically catalogued utility CSS option. **Not** part of the current UI implementation. | Current UI uses Platform-owned plain CSS. Do not treat Tailwind as required or currently wired. |
| OSS-089 | UI | Accessible primitives | Radix UI | MIT | CANDIDATE | CANDIDATE | Browser | Candidate accessible primitive set only if future UI work selects it after review. | Not current. Platform visual language remains custom; plain CSS is the current styling approach. |
| OSS-100 | Reporting | PDF | Apache PDFBox | Apache-2.0 with external-component notices | USE | APPROVED_NOT_IMPLEMENTED | Core/Report Worker | Generate and manipulate PDF reports. | Platform owns report template/layout/SLA semantics. |
| OSS-101 | Provisioning/Import | XLSX import parsing | Apache POI | Apache-2.0 with subcomponent notices | USE | APPROVED_NOT_IMPLEMENTED | Core/Provisioning | Parse and validate authorized XLSX provisioning/migration import templates where supported. | XLSX is not a Platform report/export artifact format; reports are PDF/CSV only. |
| OSS-102 | Reporting | CSV | Apache Commons CSV | Apache-2.0 | USE | APPROVED_NOT_IMPLEMENTED | Core/Report Worker | CSV generation/parsing. | Commodity function; Platform owns export authorization and limits. |
| OSS-103 | Analytics | DataFrames | Polars | MIT | USE | CURRENT_RUNTIME | Analytics Worker | Efficient batch analytics/transforms when SQL alone is insufficient. | Do not put Polars in hot ingestion path without benchmark. |
| OSS-104 | Analytics | Numerical arrays | NumPy | BSD-3-Clause | USE | CURRENT_RUNTIME | Analytics/AI | Numerical operations. | Version-level scan required. |
| OSS-105 | Analytics | Classical ML | scikit-learn | BSD-3-Clause | USE | APPROVED_NOT_IMPLEMENTED | Analytics/AI | Baselines, anomaly models, classification/regression where appropriate. | Models are evidence inputs, not sole RCA truth. |
| OSS-106 | Analytics | Compatibility dataframes | pandas | BSD-3-Clause | OPTIONAL | OPTIONAL_FUTURE | Analytics/AI | Use only where third-party/model tooling requires pandas compatibility. | Prefer Polars for large batch work to reduce overhead where feasible. |
| OSS-107 | GNN | Deep learning runtime | PyTorch | BSD-3-Clause top-level; installed-package license expression includes bundled components | CANDIDATE/P0 BENCHMARK | CANDIDATE | AI/GNN Worker | Primary candidate for GNN R&D/training/inference because of mature ecosystem. | Not frozen until CPU/GPU footprint, reproducibility, packaging and transitive licenses pass benchmark/review. |
| OSS-108 | GNN | Graph neural networks | PyTorch Geometric | MIT | CANDIDATE/P0 BENCHMARK | CANDIDATE | AI/GNN Worker | Primary candidate for temporal/topology graph experiments and message-passing models. | GNN only ranks hypotheses/impact; deterministic/protocol evidence remains authoritative. |
| OSS-109 | Graph Analytics | Graph algorithms prototype | NetworkX | BSD-3-Clause | OPTIONAL/DEV | OPTIONAL_FUTURE | AI/Engineering | Offline prototyping/evaluation of graph algorithms. | Not suitable for high-rate production graph serving at scale without benchmark. |
| OSS-110 | UI runtime (dev) | HTTPS static UI reverse proxy | NGINX Open Source | BSD-2-Clause (simplified two-clause BSD-like) | USE-DEV / USE-TOOLING | CURRENT_RUNTIME | UI Docker image (`nginx:1.27-alpine`) | Serves built React SPA over HTTPS and reverse-proxies API paths in local/dev Compose. | Do **not** treat NGINX as a mandatory customer production Core dependency solely because Docker development uses it. Production UI serving remains an architecture packaging decision. Platform owns TLS posture, no-port-80 policy, proxy routes and secret handling. Top-level licence insufficient; image OS packages require SBOM/CVE scan. |
| OSS-111 | UI build | JavaScript runtime for UI builds | Node.js | MIT (Node.js core) + bundled third-party licence obligations | USE-TOOLING | CURRENT_BUILD_TOOLING | UI build image (`node:20-alpine`) | Runs npm/Vite/TypeScript builds inside Docker. Baseline: Node.js 20.x (image tag `20-alpine`). | Must not become a required runtime on customer Core merely because React is built with Node. Transitive/bundled notices require SBOM/license scan; arbitrary npm packages are not individually pre-approved here. |
| OSS-112 | UI build | Package manager CLI | npm CLI | Artistic License 2.0 (npm CLI) + dependency-specific licences | YELLOW/USE-TOOLING | CURRENT_BUILD_TOOLING | UI build image / lockfile workflow | Installs and locks UI dependencies from `package.json`/`package-lock.json`. | Unusual top-level licence and transitive review required. Release pipeline derives transitive packages from the lockfile/SBOM and enforces policy; catalogue does not individually approve every npm transitive. |
| OSS-113 | UI build | Bundler / dev server | Vite | MIT | USE-TOOLING | CURRENT_BUILD_TOOLING | UI (`vite@5.4.21` resolved via `package-lock.json`; manifest `^5.4.6`) | React/TypeScript UI production bundling and local Vite tooling inside Docker. | Platform owns SPA architecture, HTTPS delivery and API contracts. Transitive npm packages subject to lockfile/SBOM policy. |
| OSS-114 | UI build | React Vite plugin | @vitejs/plugin-react | MIT | USE-TOOLING | CURRENT_BUILD_TOOLING | UI (`@vitejs/plugin-react@4.7.0` resolved via lockfile; manifest `^4.3.1`) | Enables React Fast Refresh / JSX transform in Vite builds. | Tooling only; not a customer runtime. |
| OSS-115 | UI language | TypeScript toolchain | TypeScript | Apache-2.0 | USE-TOOLING / approved UI toolchain | CURRENT_BUILD_TOOLING | UI (`typescript@5.9.3` resolved via lockfile; manifest `^5.5.4`) | Compile and type-check Platform UI TypeScript sources. | Platform owns UI domain models and UX. |
| OSS-116 | Core logging | Structured JSON log encoder | logstash-logback-encoder | Apache-2.0 principal; MIT-licensed incorporated portions/notices | USE/WRAP | CURRENT_RUNTIME | Core (`7.4` in `core/pom.xml`) | JSON/log encoding for structured Core logs. | Platform owns canonical log fields, secret redaction, event/error taxonomy, correlation IDs, retention policy and audit semantics. Library provides encoding only. Transitive Maven review required. |
| OSS-120 | OSS Governance | OSS policy/compliance | OSS Review Toolkit (ORT) | Apache-2.0 overall | USE-TOOLING | APPROVED_NOT_IMPLEMENTED | CI/CD | Dependency analysis, policy-as-code, attribution, SPDX/CycloneDX SBOM and source archive. | Tooling only; still requires policy/legal review. |
| OSS-121 | OSS Governance | SBOM generation | Syft | Apache-2.0 | USE-TOOLING | APPROVED_NOT_IMPLEMENTED | CI/CD | Generate SPDX/CycloneDX SBOM. | Tooling output must be archived per release. |
| OSS-122 | OSS Governance | Vulnerability/license/secret scanning | Trivy | Apache-2.0 | USE-TOOLING | APPROVED_NOT_IMPLEMENTED | CI/CD | Scan containers/filesystems/SBOMs for vulnerabilities, licenses and secrets. | Scanner classifications do not replace legal policy. |
| OSS-123 | OSS Governance | Deep license/source scan | ScanCode Toolkit | Apache-2.0 overall; reference datasets/third-party have additional licenses | USE-TOOLING | APPROVED_NOT_IMPLEMENTED | CI/CD/Legal | Deep license/copyright/package detection. | Use as tooling, not shipped runtime; comply with its own notices/data licenses. |
| OSS-124 | Linux Agent | Kernel process/network telemetry | github.com/cilium/ebpf | MIT | USE/WRAP | CURRENT_RUNTIME | Linux Agent — optional bounded source implementation | Approved Go library for loading and managing eBPF programs on Linux Agent for bounded process-to-network telemetry. **Engineering-admitted temporary derived source (not a customer release pin):** base `v0.22.0` commit `e55144e17360b60cc4583229c35c2dbf0935b308` + accepted upstream backports only `a204b5faca7fcadd2e5f835f47b02ecc66317aa6` and `4f576b5923e565087a46ed107683196fefb8fb23`; derived archive SHA256 `50abc98cb507b16868419018909fd4da934b2089bfbea4dcd8d0875216d67a4a`. No additional patches without new admission review. Object gates remain fail-closed (`#2103`, `#2083`, `kfunc`/`.ksyms`); Clang-governed object path; exact source/SBOM/provenance retained; migrate/re-evaluate against first acceptable stable upstream release containing equivalent fixes. | Platform owns probe design, local aggregation, resource bounds, loss/shed accounting, durable queueing and canonical entity correlation. **Engineering dependency admission** (satisfied for bounded product implementation; NOT customer support/runtime certification): exact source/version/patch provenance; transitive dependency inventory; licence/notice review; CVE/security review; SPDX/CycloneDX; reproducibility; exact object/loader fail-closed controls; least-privilege model; verifier/map/ring failure semantics; representative live kernel implementation-feasibility proof. **Release/runtime certification remains mandatory** before release/support claims under BUILD-018 / BUILD-019 / BUILD-130 / BUILD-134: certified Ubuntu/RHEL kernel matrix; claimed amd64/arm64 coverage; Secure Boot/lockdown/LSM where claimed; signed package/object provenance; offline/air-gap installation; install/upgrade/rollback; kernel-upgrade behavior; final CPU/RSS/kernel-map/ring/non-interference benchmarks. Engineering admission does not claim customer distribution/legal approval; legal/commercial distribution approval remains separate. Current source evidence: `agent/go.mod` uses the admitted local replacement; `agent/linux/ebpf/` and `agent/linux/cmd/optional_ebpf.go` implement the optional bounded loader/probe path; BUILD-011 whole-BUILD technical acceptance is recorded in the Build Register. This is a source-implementation truth correction, not expansion of the approved source or customer support matrix. `agent/common/collector/` still uses `UnwiredStructuredEvidenceSink` / `TRANSPORT_NOT_WIRED` for physical structured evidence; BUILD-016/062 delivery and downstream persistence remain separate. Do not claim package/kernel-matrix runtime certification, customer release approval or deployment from this row. |


## 4. Evaluated alternatives / explicitly not selected as core dependencies

| Component / approach | Decision and rationale |
|---|---|
| Elasticsearch | Do not use as primary telemetry store. ClickHouse better matches high-volume analytical telemetry and avoids Elastic distribution/license complexity. OpenSearch remains optional later for Lucene-class search. |
| VictoriaMetrics | Not part of the mandatory V1 stack after ClickHouse was selected. Can be benchmarked only if future metrics TCO evidence justifies an additional database. |
| InfluxDB 3 Core | Not primary: OSS Core is not the selected HA/scale model for Platform. |
| QuestDB OSS | Not primary: HA/replication considerations do not fit the current two-Core target as cleanly as the selected architecture. |
| TimescaleDB | Not primary: additional licensing/distribution complexity and duplicate overlap with PostgreSQL+ClickHouse. |
| Redis | Use Valkey for cache/state to keep a clearly permissive BSD-3-Clause baseline. |
| NATS | Not selected because user explicitly chose Apache Kafka. |
| Grafana | Do not embed as customer-facing dashboard product. Build Platform Dashboard Studio on the current React/TypeScript/Vite/plain-CSS UI, with approved chart/table/topology/map/terminal primitives admitted only when implemented. |
| NetBox | Do not embed as Platform IPAM authority. Provide future integration for customers that already use NetBox. |
| rConfig | Do not embed as Platform NCM authority. Reuse lower-level permissive libraries and own NCM governance/workflow. |
| Prometheus as primary TSDB | Not selected; Platform telemetry spans metrics, flow, logs/events and long-term analytics. |
| Prometheus Alertmanager as core alert engine | Not selected; routing is not equivalent to Platform qualification/correlation/RCA. |
| HashiCorp Vault as mandatory bundled service | Not mandatory due footprint/licensing/HA complexity. Platform secret storage must support customer KMS/HSM/enterprise secret integration as appropriate. |
| etcd as mandatory Core quorum | Not mandatory because customer-facing HA is exactly two Core nodes; avoid adding a third quorum node. Sensor job leases/fencing can be implemented using authoritative Core state/DB mechanisms subject to HA spike. |


## 5. Commercial redistribution requirements

1. **Version pinning:** every shipped Maven/NPM/Go/Python/OS package must be pinned or resolved through a reproducible lock/manifest.
2. **Transitive scan:** top-level license is never enough. CI must inspect Maven transitive dependencies, Go modules, Python wheels, NPM packages, native libraries and bundled data.
3. **Attribution:** required copyright and license notices must be preserved in `THIRD_PARTY_NOTICES.txt` and/or the release's license directory.
4. **SBOM:** generate both `SPDX` and `CycloneDX` SBOMs for every release image/package and archive them with the release.
5. **Source/provenance:** keep source provenance, version, hashes and source archive where required/appropriate.
6. **Vulnerability handling:** Critical/High vulnerabilities are release gates subject to documented exception process; all exceptions need owner, rationale, compensating control and expiry.
7. **Relicensing watch:** approved projects remain subject to the scheduled license/security monitoring process; a future license change does not automatically approve the new version.
8. **Customer-supplied content:** uploaded MIBs/YANG/API examples/templates belong to the customer's deployment scope unless redistribution rights are explicitly recorded.
9. **Vendor content:** a publicly downloadable MIB/YANG/SDK is not automatically redistributable. Device Pack metadata must store source, version, copyright, license and redistribution-right status.
10. **Map data:** MapLibre's BSD code license does not license map tiles/geodata. Tile/data provider attribution and redistribution terms are separately governed.
11. **Air-gap:** installers include all required pinned artifacts, SBOMs, notices and checksums. Runtime install/upgrade must not fetch packages from the Internet in air-gapped mode.

## 6. CI compliance pipeline

```text
Dependency proposed
  -> catalogue check
  -> exact version/hash pin
  -> transitive dependency resolution
  -> ORT policy evaluation
  -> Syft SBOM
  -> Trivy vulnerability/license/secret scan
  -> ScanCode deep license/provenance scan where required
  -> GREEN = build allowed
     YELLOW = architecture/legal approval required
     RED = build fails by default
  -> generate notices/license bundle/SBOM/checksums/provenance
  -> archive with release
```

## 7. Module-by-module OSS recommendation summary

| Platform module | Primary OSS foundation | Platform-owned differentiation |
|---|---|---|
| SNMP | GoSNMP + gosmi | polling scheduler, credentials, MIB Studio, normalization, policy, validation |
| NetFlow/IPFIX/sFlow | GoFlow2 | enrichment, canonical flow model, cardinality, analytics, correlation |
| ICMP/traceroute | x/net/icmp + Platform path engine | scheduling, jitter/loss statistics, multi-vantage/path RCA |
| DNS | miekg/dns + Go networking | synthetic profiles, validation, SLA |
| SSH/CLI | x/crypto/ssh / scrapligo | credential broker, command policy, audit, session recording |
| NETCONF/gNMI/YANG | scrapligo/OpenConfig gNMI/goyang/ygot | subscriptions, normalized telemetry, Device Pack/YANG workflow |
| API/REST | Go HTTP + kin-openapi | Connector Studio, schema inference, metric/entity mapping, chained calls |
| Logs/syslog | Fluent Bit integration + Platform receiver | normalization, retention, cardinality, alerts/RCA |
| Windows | masterzen/winrm | secure policies, collection schema, retries/diagnostics |
| Hardware | Gofish | hardware entity model and vendor packs |
| VMware | govmomi | canonical topology/metrics and alerts |
| Kubernetes | client-go + OTel | entity graph, policy and correlation |
| OTel | OpenTelemetry Collector/libraries | tenant mapping, canonical schema and non-APM product UX |
| NCM | Nornir + scrapli + scrapli_cfg + selected NAPALM/NTC/Jinja2 | approval, compliance, golden config, rollback, RCA evidence, audit |
| UI (current implementation) | React 18 + TypeScript + Vite + plain CSS | complete Platform UX shell; Studios/RBAC/query model remain Platform-owned |
| UI (approved future primitives) | ECharts, TanStack Table/Query, react-grid-layout, Cytoscape.js, MapLibre GL JS, xterm.js | Approved allow-list primitives for charts/tables/dashboards/topology/maps/terminal when implemented; not current `ui/package.json` dependencies |
| UI (not current) | Tailwind CSS, Radix UI | Not current dependencies; Tailwind not required; Radix remains candidate only |
| UI build/tooling | Node.js 20 + npm + Vite + TypeScript (+ @vitejs/plugin-react) | Docker-only UI compile/bundle; not customer Core runtime |
| UI HTTPS (dev) | NGINX Open Source | local/dev static+TLS proxy only unless separately selected for production packaging |
| Core logging encode | logstash-logback-encoder | JSON encoding only; Platform owns fields/redaction/taxonomy/correlation/audit |
| Core control | Java 21 + Spring Boot | Platform domain model and workflows |
| Identity | Keycloak | Platform resource authorization/tenant isolation |
| Policy expressions | CEL | Platform policy hierarchy, variables, safety limits and simulation |
| Message bus | Kafka + Apicurio + Avro/Protobuf | canonical event contracts, routing, idempotency/replay |
| Control DB | PostgreSQL | Platform schemas/state ownership |
| Telemetry | ClickHouse | Platform schemas/TTL/rollups/query planner/storage protection |
| Cache | Valkey | cache only; authoritative state remains elsewhere |
| Reports | PDFBox + Commons CSV | Report Studio, SLA/OLA/service-assurance semantics, layouts and scheduling; report artifacts are PDF/CSV only |
| Provisioning import | Apache POI | Authorized XLSX import parsing only; not report generation |
| Analytics | Polars + NumPy + scikit-learn | baselines/anomalies/forecast logic and quality controls |
| GNN | PyTorch + PyTorch Geometric candidate | graph features, labels, training, causal guardrails, evidence fusion |
| OSS governance | ORT + Syft + Trivy + ScanCode | Platform license policy and release approval process |

## 8. Why Platform still builds major modules from scratch

Platform does **not** build SNMP, Kafka, SQL databases, SSH cryptography or chart rendering from scratch because there is no product differentiation in reimplementing mature protocol/infrastructure primitives. Platform **does** build the following because they encode our customer/domain behavior and cannot be delegated to a generic OSS product without fragmenting the experience or data model:

- canonical entity and relationship graph;
- Universal Device Pack format and compatibility/provenance model;
- MIB Studio, API Studio and Device Pack Studio;
- monitoring scheduler, Monitoring Work Units and Sensor Placement Engine;
- policy inheritance/overrides, adaptive polling and impact simulation;
- Alert Qualification Engine and false-alert reduction;
- event/alert/incident correlation and recovery qualification;
- temporal graph, evidence timeline, hybrid RCA and GNN evidence fusion;
- IPAM authority and graph enrichment;
- NCM governance, maker-checker, compliance and rollback orchestration;
- ITSM lifecycle semantics and bidirectional field/state mapping;
- privileged CLI Session Broker and command policy;
- Dashboard/Map/Report Studios and role-aware UI;
- audit semantic model, tenant isolation and Platform authorization;
- data lifecycle/storage protection and licensing/entitlement logic.

## 9. Upstream verification note

Licenses in this document were originally rechecked against official/upstream project pages or repositories during the 2026-09-06 reconciliation and revalidated for A2.3 truth-state classification on 2026-09-24. Because licenses and dependencies can change, the catalogue is an allow-list **by approved version**, not a permanent approval of every future release of the named project.

## 10. Development/test tooling

The following are engineering tools and are **not shipped as mandatory customer runtime dependencies** unless separately approved. Exact versions remain pinned/scanned in CI.

| Capability | Tool / project | License posture | Decision | Shipping boundary |
|---|---|---|---|---|
| Local container runtime basis | Docker Engine / Moby-compatible engine | Moby core is Apache-2.0; developer distribution terms must be respected | USE — DEV/TEST | Developer/CI only |
| Compose orchestration | Docker Compose | Apache-2.0 project | USE — DEV/TEST | Developer/CI only |
| Browser/UI E2E | Playwright | Apache-2.0 | USE — TEST | Test tooling only |
| HTTP/API/load generation | Locust | MIT | CANDIDATE — TEST | Test tooling only |
| Java build | Maven/Gradle as selected per repo | permissive project licenses; exact tool locked | USE — BUILD | Build tooling only |
| Linux package build | Debian/RPM native packaging tools inside build containers | distro tooling licenses vary; build-time only | USE — BUILD | Build tooling only |
| SBOM | Syft | Apache-2.0 | USE — RELEASE | Tooling only |
| Vulnerability/license scan | Trivy | Apache-2.0 | USE — RELEASE | Tooling only |
| OSS governance | OSS Review Toolkit | Apache-2.0 core; exact dependencies scanned | USE — RELEASE | Tooling only |
| Deep license scan | ScanCode Toolkit | tool distribution contains multiple licensed datasets/components | USE — TOOLING WITH REVIEW | Tooling only |

Tooling-only status does not mean licensing can be ignored; it means the tool is not bundled into proprietary customer runtime by default.

### 10.1 Podman decision

Podman compatibility is **not** a v1 engineering requirement. Adding a second local container runtime would increase variance in Compose compatibility, networking, volume behavior, rootless operation and support burden. The project will standardize on Docker Compose until a customer/developer requirement justifies another runtime.

---

## Phase 2 Enterprise RFP Readiness Reconciliation — Issue #269

Issue #269 selects **no new OSS dependency**. Phase-2 breadth shall preferentially use already-approved protocol/SDK foundations and Capability/Integration Pack adapters. Any future database driver, middleware client, backup SDK, OCI SDK, APM/NPM/DPI SDK, browser/APM agent, packet engine or other runtime dependency requires normal exact-version OSS admission before implementation. Full native APM/DPI work is Phase 2.5+ and receives no dependency pre-approval from this roadmap.

---

## Issue #272 OSS decision

Issue #272 selects **no new OSS dependency**.

Future Phase-2.5 implementation may require instrumentation, trace, profiling, packet-capture/decoding or storage components, but each candidate MUST pass the normal OSS admission process for licence/redistribution, security/CVE posture, maintenance health, resource cost, architecture fit, air-gap/offline packaging, tenant/privacy implications, upgrade/rollback and vendor-lock-in/exit risk.

Roadmap inclusion is not dependency approval.

## Current dependency truth reconciliation — 2026-11-06

This correction is bounded to verified source at GitHub main `36e1513f25265d0bcd45bdfe5156ee02fdbf5238`. `CURRENT_RUNTIME` identifies a dependency actually wired in a runtime source path, including an optional disabled-by-default path; it does not certify every deployment profile, activate that path, or grant commercial distribution approval. The source-admission restrictions in OSS-124 remain unchanged.

OSS-004 Apicurio Registry and OSS-005/006 Avro/Protocol Buffers remain approved architecture with `APPROVED_NOT_IMPLEMENTED` current truth. Current JSON transport is not evidence that those dependencies were revoked, and their catalog approval is not evidence that a registry currently runs or that all physical messages use Avro/Protobuf. BUILD-063 and `Platform_17_API_Event_and_Data_Model_Contracts.md` own exact physical serialization, schema compatibility, migration/replay and registry-admission gates. No new runtime service or dependency is introduced by this documentation correction.

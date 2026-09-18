
# Platform — Authoritative Open-Source Software Catalogue

**Document ID:** PLATFORM-OSS-001  
**Version:** 3.2.3
**Status:** **Authoritative Mother Reference — Dependency Allow-List Baseline**  
**Verification date:** 2026-09-06 (v3.2.1 bootstrap UI/build/logging admissions)  
**Important:** This is an engineering/licensing governance baseline, not legal advice. Exact pinned versions and their complete transitive/bundled licenses must be scanned before release.

## 1. Purpose

Platform is a clean-room proprietary commercial product that deliberately reuses mature permissively licensed protocol engines, SDKs, data infrastructure and UI libraries instead of rebuilding commodity functions. Platform must build its differentiated logic — canonical entity graph, Device Pack model, MIB/API Studios, monitoring scheduler, Sensor placement, policy hierarchy, false-alert qualification, incident correlation, RCA/GNN intelligence, NCM governance, IPAM authority, Dashboard/Report/Map Studios, multi-tenancy, audit semantics and licensing — as Platform-owned code.

The rule is **libraries and infrastructure over nested products**: use mature OSS to avoid reinventing SNMP, Kafka, database engines, SSH, chart rendering, etc.; do not embed another complete NMS/IPAM/dashboard product as Platform's authority.

## 2. Decision labels

- `USE`: approved architectural dependency, subject to pinned-version scan.
- `USE/WRAP`: approved, but must be hidden behind a Platform adapter/interface.
- `CANDIDATE`: technically suitable but not frozen until P0 benchmark/architecture review.
- `YELLOW/CANDIDATE`: top-level license may be acceptable but transitive/driver licensing requires mandatory review.
- `OPTIONAL-LATER`: not part of mandatory V1 footprint.
- `USE-TOOLING`: engineering/CI tool; not shipped as Platform runtime unless separately approved.

## 3. Approved/candidate catalogue

| ID | Domain | Capability | OSS / Technology | License | Decision | Runtime | How Platform uses it | What remains custom |
|---|---|---|---|---|---|---|---|---|
| OSS-001 | Core Framework | Core control plane/API | Spring Boot | Apache-2.0 | USE | Core | Java 21 modular Core framework, REST/API, security integration, configuration, lifecycle. | Do not build web/application framework plumbing. Platform-specific domain modules, policies, authorization semantics and workflows remain custom. |
| OSS-002 | Streaming | Internal durable event/telemetry backbone | Apache Kafka | Apache-2.0 | USE | Core | Internal message backbone for normalized telemetry, events, audit, alert/RCA processing and async integration. Kafka is not exposed directly to Sensors. | Kafka is commodity streaming; Platform owns topic contracts, idempotency, routing, retention, replay semantics and two-Core failover integration. |
| OSS-003 | Streaming | Go Kafka client | franz-go | BSD-3-Clause | USE | Core/Sensor utilities | Efficient Go Kafka producer/consumer where Go components communicate with internal Kafka. | Do not write Kafka protocol client. Wrap behind Platform producer/consumer interfaces. |
| OSS-004 | Schema Registry | Schema governance | Apicurio Registry | Apache-2.0 | USE | Core | Registry for Avro/Protobuf schemas and compatibility rules. | Avoid proprietary schema-registry lock-in; Platform owns subject naming, compatibility policy and release governance. |
| OSS-005 | Schema | Event serialization | Apache Avro | Apache-2.0 | USE | Core/Schemas | Candidate/default for selected event families needing schema evolution. | Schema choice is per contract; Platform owns canonical event definitions. |
| OSS-006 | Schema | RPC/event serialization | Protocol Buffers | BSD-3-Clause / bundled notices | USE | Core/Sensor/API | Typed RPC/event contracts where protobuf/gRPC is appropriate. | Platform owns canonical schemas and compatibility rules. |
| OSS-007 | Transactional Data | Authoritative control/config database | PostgreSQL | PostgreSQL License | USE | Core | Tenants, inventory, policies, RBAC metadata, device definitions, current state, IPAM, NCM metadata, licensing, workflow state. | Do not use PostgreSQL as the primary high-volume telemetry store. Platform owns schema and migrations. |
| OSS-008 | Telemetry Data | High-volume analytics/time-series store | ClickHouse | Apache-2.0 | USE | Core | Metrics, flow, syslog/events, alert history, incident evidence, audit history, SLA/capacity facts and analytics. | Chosen over Elasticsearch as primary store for resource-efficient analytical telemetry. Platform owns schemas, TTLs, rollups and query planner. |
| OSS-009 | Cache | Cache/ephemeral state | Valkey | BSD-3-Clause | USE | Core | Query cache, ephemeral coordination/state where data loss is acceptable/reconstructable. | Do not store authoritative state only in cache. |
| OSS-010 | Identity | Identity federation | Keycloak | Apache-2.0 top-level; transitive scan required | USE/WRAP | Core | OIDC/SAML/LDAP/AD federation, authentication flows and MFA integration. | Keycloak answers identity/authentication; Platform owns tenant/resource authorization. Every distribution must scan Keycloak transitive dependencies. |
| OSS-011 | Authorization | General policy decision engine | Open Policy Agent (OPA) | Apache-2.0 | CANDIDATE | Core | Candidate for selected coarse-grained authorization/policy decisions. | Do not make OPA the owner of Platform domain authorization without a benchmark/design decision. Platform RBAC/resource-scope semantics remain custom. |
| OSS-012 | Expressions | Safe expression language | CEL (cel-java / cel-go) | Apache-2.0 | USE | Core/Sensor | Threshold formulas, dynamic groups, constrained policy expressions and safe user-defined conditions. | Use non-Turing-complete CEL rather than arbitrary Python/JavaScript in customer policy paths. Platform defines allowed variables/functions/cost limits. |
| OSS-013 | Data Interchange | Columnar in-memory interchange | Apache Arrow | Apache-2.0 with bundled third-party notices | OPTIONAL | Core/Analytics | Efficient analytical interchange where it materially reduces copies/serialization. | Only introduce where benchmark justifies it. |
| OSS-014 | Search | Optional dedicated full-text search | OpenSearch | Apache-2.0 | OPTIONAL-LATER | Core | Add only if customer log search requirements exceed ClickHouse text-search capability. | Not mandatory in V1 to keep footprint low. Elasticsearch is not the core store. |
| OSS-020 | Network Collection | SNMP v1/v2c/v3 | GoSNMP | BSD-style | USE/WRAP | Sensor | SNMP Get/GetNext/GetBulk/Walk/Set and traps/informs protocol engine. | Do not reimplement BER/SNMP. Platform owns scheduling, credential profiles, normalization, validation, retry and policy. |
| OSS-021 | Network Collection | MIB parsing | gosmi | MIT | USE/WRAP | Core Studio/Sensor build tooling | Parse SMIv1/v2 MIBs for MIB Studio, OID metadata, enums, tables and notifications. | Platform owns MIB provenance, dependency workflow, metric semantics, Device Pack generation and redistribution controls. |
| OSS-022 | Network Collection | NetFlow/IPFIX/sFlow parsing | GoFlow2 | BSD-3-Clause | USE/WRAP | Streaming Sensor | Decode NetFlow v5/v9, IPFIX and sFlow. | Platform owns enrichment, canonical flow schema, cardinality controls, Kafka publishing and analytics. |
| OSS-023 | Network Collection | ICMP | golang.org/x/net/icmp | BSD-3-Clause | USE | Sensor/Agent | ICMP echo and low-level packet operations. | Platform owns scheduling, statistics, validation workflows, jitter/loss aggregation and path correlation. |
| OSS-024 | Network Collection | DNS protocol | miekg/dns | BSD-style | USE | Sensor/Agent | Detailed DNS synthetic checks where standard resolver APIs are insufficient. | Platform owns test definitions, validation and SLA semantics. |
| OSS-025 | Network Collection | SSH | golang.org/x/crypto/ssh | BSD-3-Clause | USE/WRAP | Sensor | SSH transport for CLI, discovery, diagnostics and NCM. | Platform owns credential brokering, command controls, session audit and vendor behavior. |
| OSS-026 | Network Collection | CLI/NETCONF automation | scrapligo | MIT | CANDIDATE/USE | Sensor | Go network CLI/NETCONF automation where it fits Sensor runtime. | Wrap behind Device Pack adapter; avoid lock-in to vendor/platform naming. |
| OSS-027 | Network Collection | gNMI | OpenConfig gNMI | Apache-2.0 | USE/WRAP | Sensor | gNMI Get/Set/Subscribe protocol and reference client components. | Platform owns subscription policy, normalization, capacity controls and graph semantics. |
| OSS-028 | Network Collection | YANG parsing | OpenConfig goyang | Apache-2.0 | USE | Core Studio/Sensor | YANG model parsing for Device Pack/YANG support. | Platform owns YANG Studio workflow, provenance and mapping. |
| OSS-029 | Network Collection | YANG model utilities | OpenConfig ygot | Apache-2.0 | USE | Core Studio/Sensor | YANG-centric validation/marshal/unmarshal utilities. | Platform owns canonical mapping and compatibility policy. |
| OSS-030 | API Connectors | OpenAPI parser/validator | kin-openapi | MIT | USE | Core Studio/Sensor | OpenAPI/Swagger import, validation and schema handling for API Studio. | Platform owns endpoint discovery, field/metric/entity inference, chained-call orchestration and Device Pack generation. |
| OSS-031 | Logs/Events | Log/syslog collection helper | Fluent Bit | Apache-2.0 | CANDIDATE/INTEGRATE | Streaming Sensor/Agent | Use as an integration/collection component where appropriate for log ingestion and parsing. | Do not make Fluent Bit the Platform event model. Platform owns normalization, cardinality, retention and alert/RCA semantics. |
| OSS-040 | Infrastructure | Windows WinRM | masterzen/winrm | Apache-2.0 | USE/WRAP | Infrastructure Sensor | WinRM command/collection plumbing for Windows agentless monitoring. | Platform owns secure policy, metrics, credentials, diagnostics and retries. |
| OSS-041 | Infrastructure | Host metrics helpers | gopsutil | BSD-3-Clause | USE | Agent/Sensor | Cross-platform host process/CPU/memory/disk/network primitives where suitable. | Use native OS APIs where required for accuracy; Platform owns schema and collection policy. |
| OSS-042 | Hardware | Redfish/Swordfish | Gofish | BSD-3-Clause | USE/WRAP | Infrastructure Sensor | Redfish/Swordfish hardware inventory/health collection. | Platform owns normalized hardware entity model and vendor packs. |
| OSS-043 | Virtualization | VMware vSphere | govmomi | Apache-2.0 | USE/WRAP | Infrastructure Sensor | vCenter/ESXi inventory, topology and metrics via vSphere APIs. | Platform owns graph, monitoring policy, dashboards and alerts. |
| OSS-044 | Containers | Kubernetes client | client-go | Apache-2.0 | USE/WRAP | Infrastructure/Cloud Sensor | Kubernetes API, inventory, events and workload discovery. | Platform owns canonical K8s graph and correlation. |
| OSS-045 | Observability | OpenTelemetry / OTLP | OpenTelemetry Collector + contrib | Apache-2.0 | USE/INTEGRATE | Sensor/Core ingestion | OTLP metrics/logs ingestion and selected receivers/exporters where appropriate. | Platform does not market full APM initially. Trace storage/UX is future-ready only. |
| OSS-046 | Containers | Docker/container runtime API | Moby/Docker client libraries | Apache-2.0 | CANDIDATE | Infrastructure Sensor | Container runtime inventory/metrics where Docker API is used. | Use only required client APIs and version-pin. |
| OSS-047 | Cloud | AWS API/monitoring | AWS SDK for Go v2 | Apache-2.0 | USE/WRAP | Cloud Sensor | AWS inventory/monitoring API access. | Platform owns entity normalization and policy. Version-level dependency scan mandatory. |
| OSS-048 | Cloud | Azure API/monitoring | Azure SDK for Go | MIT / component-specific review | USE/WRAP | Cloud Sensor | Azure inventory/monitoring APIs. | Version-level component/license scan required. |
| OSS-049 | Cloud | GCP API/monitoring | Google Cloud Go | Apache-2.0 | USE/WRAP | Cloud Sensor | GCP APIs and monitoring integrations. | Platform owns entity normalization and policy. |
| OSS-060 | NCM | Network task orchestration | Nornir | Apache-2.0 | USE/WRAP | NCM Worker | Parallel inventory/task orchestration for network automation where Python NCM workers are used. | Platform owns tenancy, approvals, job safety, scheduling and audit. |
| OSS-061 | NCM | Network CLI | scrapli | MIT | USE/WRAP | NCM Worker | Fast sync/async network device connections and command execution. | Platform owns device packs, credentials, RBAC and session policy. |
| OSS-062 | NCM | Configuration load/diff/commit | scrapli_cfg | MIT | USE/WRAP | NCM Worker | Config prepare/load/diff/commit operations. | Platform owns approval, rollback workflow, compliance and evidence. |
| OSS-063 | NCM | Multi-vendor abstraction | NAPALM | Apache-2.0 top-level; transitive review mandatory | YELLOW/CANDIDATE | NCM Worker | Use where driver coverage materially accelerates NCM. | Do not approve blindly: drivers/transitive dependencies can introduce additional license obligations. Prefer scrapli/scrapligo paths where practical. |
| OSS-064 | NCM | CLI parsing templates | NTC Templates / TextFSM | Apache-2.0 | USE/WRAP | NCM Worker/Device Packs | Reusable TextFSM parsers/templates for CLI output. | Platform must track template provenance/vendor compatibility and may override with certified Device Pack parsers. |
| OSS-065 | NCM | Configuration templating | Jinja2 | BSD-3-Clause | USE | NCM Worker | Safe parameterized configuration templates. | Platform owns template lifecycle, validation, approval and secret redaction. |
| OSS-080 | UI | Web UI framework | React | MIT | USE | Browser | Primary component framework. | Platform owns product UX, accessibility and role-aware behavior. No embedded Grafana. |
| OSS-081 | UI | Charts | Apache ECharts | Apache-2.0 with bundled BSD notices | USE | Browser | Time series, bar, gauge, heatmap, histogram, Sankey and other visualization primitives. | Platform owns panel/query/alert semantics and dashboard builder. |
| OSS-082 | UI | Tables | TanStack Table | MIT | USE | Browser | Headless large-table features: sorting/filtering/grouping/columns. | Combine with server-side pagination/virtualization; do not load large estates into browser. |
| OSS-083 | UI | Server state/query caching | TanStack Query | MIT | USE | Browser | Client-side server-state caching/retry/invalidation. | Platform API remains source of truth. |
| OSS-084 | UI | Dashboard grid | react-grid-layout | MIT | USE | Browser | Drag/drop/resizable dashboard panels. | Platform owns permissions, dashboard schema, persistence, versioning and accessibility. |
| OSS-085 | UI | Topology graph rendering | Cytoscape.js | MIT | USE | Browser | Interactive graph/topology/causal graph visualization. | Platform owns operational graph semantics, confidence, historical topology and RCA relationships. |
| OSS-086 | UI | Geospatial maps | MapLibre GL JS | BSD-3-Clause | USE | Browser | Geo/site/region map rendering. | Map code license does not license map tiles/data. Tile/data provider rights and attribution are separately managed. |
| OSS-087 | UI | Browser terminal | xterm.js | MIT | USE | Browser | Terminal emulator for privileged CLI sessions. | Browser never receives device credentials; Platform Session Broker owns access and audit. |
| OSS-088 | UI | Styling | Tailwind CSS | MIT | USE | Browser | Utility styling where engineering chooses it. | Design tokens/accessibility and product theming remain Platform-owned. |
| OSS-089 | UI | Accessible primitives | Radix UI | MIT | CANDIDATE/USE | Browser | Accessible low-level UI primitives. | Use only if it meets performance/design needs; Platform visual language remains custom. |
| OSS-100 | Reporting | PDF | Apache PDFBox | Apache-2.0 with external-component notices | USE | Core/Report Worker | Generate and manipulate PDF reports. | Platform owns report template/layout/SLA semantics. |
| OSS-101 | Reporting | XLSX | Apache POI | Apache-2.0 with subcomponent notices | USE | Core/Report Worker | Generate Excel reports/exports. | Platform owns report definitions and data/query safety. |
| OSS-102 | Reporting | CSV | Apache Commons CSV | Apache-2.0 | USE | Core/Report Worker | CSV generation/parsing. | Commodity function; Platform owns export authorization and limits. |
| OSS-103 | Analytics | DataFrames | Polars | MIT | USE | Analytics Worker | Efficient batch analytics/transforms when SQL alone is insufficient. | Do not put Polars in hot ingestion path without benchmark. |
| OSS-104 | Analytics | Numerical arrays | NumPy | BSD-3-Clause | USE | Analytics/AI | Numerical operations. | Version-level scan required. |
| OSS-105 | Analytics | Classical ML | scikit-learn | BSD-3-Clause | USE | Analytics/AI | Baselines, anomaly models, classification/regression where appropriate. | Models are evidence inputs, not sole RCA truth. |
| OSS-106 | Analytics | Compatibility dataframes | pandas | BSD-3-Clause | OPTIONAL | Analytics/AI | Use only where third-party/model tooling requires pandas compatibility. | Prefer Polars for large batch work to reduce overhead where feasible. |
| OSS-107 | GNN | Deep learning runtime | PyTorch | BSD-3-Clause top-level; installed-package license expression includes bundled components | CANDIDATE/P0 BENCHMARK | AI/GNN Worker | Primary candidate for GNN R&D/training/inference because of mature ecosystem. | Not frozen until CPU/GPU footprint, reproducibility, packaging and transitive licenses pass benchmark/review. |
| OSS-108 | GNN | Graph neural networks | PyTorch Geometric | MIT | CANDIDATE/P0 BENCHMARK | AI/GNN Worker | Primary candidate for temporal/topology graph experiments and message-passing models. | GNN only ranks hypotheses/impact; deterministic/protocol evidence remains authoritative. |
| OSS-109 | Graph Analytics | Graph algorithms prototype | NetworkX | BSD-3-Clause | OPTIONAL/DEV | AI/Engineering | Offline prototyping/evaluation of graph algorithms. | Not suitable for high-rate production graph serving at scale without benchmark. |
| OSS-110 | UI runtime (dev) | HTTPS static UI reverse proxy | NGINX Open Source | BSD-2-Clause (simplified two-clause BSD-like) | USE-DEV / USE-TOOLING | UI Docker image (`nginx:1.27-alpine`) | Serves built React SPA over HTTPS and reverse-proxies API paths in local/dev Compose. | Do **not** treat NGINX as a mandatory customer production Core dependency solely because Docker development uses it. Production UI serving remains an architecture packaging decision. Platform owns TLS posture, no-port-80 policy, proxy routes and secret handling. Top-level licence insufficient; image OS packages require SBOM/CVE scan. |
| OSS-111 | UI build | JavaScript runtime for UI builds | Node.js | MIT (Node.js core) + bundled third-party licence obligations | USE-TOOLING | UI build image (`node:20-alpine`) | Runs npm/Vite/TypeScript builds inside Docker. Baseline: Node.js 20.x (image tag `20-alpine`). | Must not become a required runtime on customer Core merely because React is built with Node. Transitive/bundled notices require SBOM/license scan; arbitrary npm packages are not individually pre-approved here. |
| OSS-112 | UI build | Package manager CLI | npm CLI | Artistic License 2.0 (npm CLI) + dependency-specific licences | YELLOW/USE-TOOLING | UI build image / lockfile workflow | Installs and locks UI dependencies from `package.json`/`package-lock.json`. | Unusual top-level licence and transitive review required. Release pipeline derives transitive packages from the lockfile/SBOM and enforces policy; catalogue does not individually approve every npm transitive. |
| OSS-113 | UI build | Bundler / dev server | Vite | MIT | USE-TOOLING | UI (`vite@5.4.21` resolved via `package-lock.json`; manifest `^5.4.6`) | React/TypeScript UI production bundling and local Vite tooling inside Docker. | Platform owns SPA architecture, HTTPS delivery and API contracts. Transitive npm packages subject to lockfile/SBOM policy. |
| OSS-114 | UI build | React Vite plugin | @vitejs/plugin-react | MIT | USE-TOOLING | UI (`@vitejs/plugin-react@4.7.0` resolved via lockfile; manifest `^4.3.1`) | Enables React Fast Refresh / JSX transform in Vite builds. | Tooling only; not a customer runtime. |
| OSS-115 | UI language | TypeScript toolchain | TypeScript | Apache-2.0 | USE-TOOLING / approved UI toolchain | UI (`typescript@5.9.3` resolved via lockfile; manifest `^5.5.4`) | Compile and type-check Platform UI TypeScript sources. | Platform owns UI domain models and UX. |
| OSS-116 | Core logging | Structured JSON log encoder | logstash-logback-encoder | Apache-2.0 principal; MIT-licensed incorporated portions/notices | USE/WRAP | Core (`7.4` in `core/pom.xml`) | JSON/log encoding for structured Core logs. | Platform owns canonical log fields, secret redaction, event/error taxonomy, correlation IDs, retention policy and audit semantics. Library provides encoding only. Transitive Maven review required. |
| OSS-120 | OSS Governance | OSS policy/compliance | OSS Review Toolkit (ORT) | Apache-2.0 overall | USE-TOOLING | CI/CD | Dependency analysis, policy-as-code, attribution, SPDX/CycloneDX SBOM and source archive. | Tooling only; still requires policy/legal review. |
| OSS-121 | OSS Governance | SBOM generation | Syft | Apache-2.0 | USE-TOOLING | CI/CD | Generate SPDX/CycloneDX SBOM. | Tooling output must be archived per release. |
| OSS-122 | OSS Governance | Vulnerability/license/secret scanning | Trivy | Apache-2.0 | USE-TOOLING | CI/CD | Scan containers/filesystems/SBOMs for vulnerabilities, licenses and secrets. | Scanner classifications do not replace legal policy. |
| OSS-123 | OSS Governance | Deep license/source scan | ScanCode Toolkit | Apache-2.0 overall; reference datasets/third-party have additional licenses | USE-TOOLING | CI/CD/Legal | Deep license/copyright/package detection. | Use as tooling, not shipped runtime; comply with its own notices/data licenses. |


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
| Grafana | Do not embed as customer-facing dashboard product. Use React/ECharts/TanStack/Cytoscape/MapLibre and build Platform Dashboard Studio. |
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
| UI | React + ECharts + TanStack + react-grid-layout + Cytoscape + MapLibre + xterm.js | complete Platform UX, Studios, RBAC behavior, query model |
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
| Reports | PDFBox + POI + Commons CSV | Report Studio, SLA semantics, layouts and scheduling |
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

Licenses in this document were rechecked against official/upstream project pages or repositories during the 2026-09-06 reconciliation. Because licenses and dependencies can change, the catalogue is an allow-list **by approved version**, not a permanent approval of every future release of the named project.

## 10. Development/test tooling additions — v3.1

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

---

## v3.2.3 UI/UX dependency-governance clarification

UI/UX design recommendations may name technologies for illustration. The authoritative OSS status is the catalogue above, not recommendation text.

### Already selected / approved architectural basis

The following are already marked `USE` and are the default implementation basis when their capability is implemented:

- React — application framework;
- Apache ECharts — charts and operational visualization primitives;
- TanStack Table — large-table behavior;
- TanStack Query — browser server-state/query caching;
- react-grid-layout — dashboard grid/canvas layout;
- Cytoscape.js — topology/causal graph rendering;
- MapLibre GL JS — geospatial rendering;
- xterm.js — terminal emulator component, subject to the separate privileged Session Broker/NCM security gate;
- Jinja2 — NCM configuration templating only, under NCM validation/approval/secret controls.

`USE` selects the library family; it does not claim the current UI bundle already includes it, that every surrounding feature is production-ready, or that arbitrary versions may be introduced. First implementation still requires exact-version locking, licence/notice verification, CVE/supply-chain scan, SBOM and offline-packaging evidence.

### Not selected merely by recommendation

- Do not add uPlot alongside ECharts without benchmark evidence that ECharts cannot meet a certified requirement.
- Do not add G6/AntV alongside Cytoscape.js without approved architecture change.
- Do not add Mapbox SDK in place of MapLibre without separate licensing/product decision.
- OpenSearch remains `OPTIONAL-LATER`; do not introduce it solely for omnibar search. Elasticsearch is not a core Platform search dependency.
- Puppeteer/Playwright are not production reporting dependencies unless a later rendering requirement passes architecture/OSS/resource/security review.
- Arbitrary customer JavaScript remains prohibited in dashboard/query/transformation paths.

Any future addition still requires bundle/runtime, rendering-density, accessibility, deterministic-test, offline-packaging, maintainership, security, licensing and resource-envelope review.

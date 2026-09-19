# Platform — Extensibility, Capability Pack, MIB, API & Webhook Runtime Master

**Document ID:** PLATFORM-EXTENSIBILITY-001  
**Version:** Candidate 3.2.3 — subject to independent review  
**Status:** **Candidate Specialized Mother Reference — Draft PR for independent challenge/reconciliation**  
**Product:** Platform  
**Change class:** Candidate additive/corrective architecture contract for review against current `main`; no existing requirement is intentionally weakened

---

## 1. Authority and purpose\n\n**Proposed filename:** `Platform_24_Extensibility_CapabilityPack_MIB_API_Webhook_Runtime_Master.md`

This document is a candidate v3.2.3 specialized master for Platform extensibility: new SNMP MIBs, new device/vendor/model/firmware monitoring, REST/OpenAPI connectors, Capability Packs, outbound/inbound/callback webhooks, their persistence, distribution, security, testing, UI and operations.

It follows the consolidated-specialized-master model used by `Platform_19`, `Platform_20`, `Platform_21`, `Platform_22` and `Platform_23`. `Platform_23` remains authoritative for the single Capability Pack architecture and dimensional certification; this candidate adds detailed extensibility authoring/runtime contracts and must be reconciled against current implementation before merge. It adds explicit contracts where the v3.2.2 mother set was directionally correct but insufficiently detailed to prevent implementation agents from inventing incompatible lifecycle, database, API, rollout, webhook or security behavior.

Logical authority:

- §3 = formal product requirements (PRD supplement);
- §4 = prioritized formal engineering/product requirements and acceptance evidence;
- §5 = governing architecture/HLD;
- §6 = LLD/data/storage/state contracts;
- §7 = backend API/event/webhook contracts;
- §8 = end-to-end dataflows;
- §9 = UI/UX contracts;
- §10 = security/threat model additions;
- §11 = capacity/cardinality/performance contracts;
- §12 = quality/certification/release gates;
- §13 = operations/HA/DR/runbooks;
- §14 = formal traceability supplement;
- §15 = Definition of Done and change-control rules.

**Current-main reconciliation note:** this draft was rebased conceptually onto `main` at `df0dd5f19ce502dbc141e8e4b13a84293dc2a7ca`, where Platform 21–23 already exist and Capability Pack is the canonical single extensibility architecture. It must not be merged until the Main Chat independently checks every `REQ-EXTL-*` item for duplication, contradiction, priority and implementation reality.\n\n`Platform_00_Document_Index.md` defines precedence. This master may add detail where existing documents are silent. If a true contradiction exists, it must be reconciled through deliberate Mother Reference change control before implementation.

This document **does not** select a new database, mandatory microservice, arbitrary-code plugin system, Internet marketplace dependency or new OSS component. Existing PostgreSQL/ClickHouse/Kafka/Valkey and Core/Sensor architecture remain governing unless separately changed.

### 1.1 Priority semantics

- **P0** — architecture/security/data-integrity prerequisite. Must be contractually settled and tested before the affected capability is considered implementation-complete or production-safe.
- **P1** — enterprise-production completeness. Builds on P0 and is required before GA of the affected capability. If a P1 capability is intentionally deferred, that affected capability is not represented as GA-complete.
- **P2** — advanced ecosystem/developer-experience/optimization capability. May be sequenced later and must not complicate or weaken the P0/P1 foundation.

---

## 2. Non-negotiable principles

1. **Content before code.** Existing protocol/runtime primitives are extended through governed content, not vendor-specific Core/Sensor forks.
2. **Capability Pack is the canonical device capability package.** MIB/API/YANG/CLI/NCM/topology/alert/RCA/dashboard semantics converge through one versioned Pack model.
3. **MIB upload is not monitor activation.** Only explicit Monitor Definitions create runtime collection.
4. **Published versions are immutable.** Every material change creates a new version with provenance and audit.
5. **Core desired state, Sensor observed state.** Distribution is verified, staged, atomic and recoverable to last-known-good.
6. **PostgreSQL owns control state.** ClickHouse owns high-volume telemetry/history; Kafka remains internal durable event transport; Valkey remains ephemeral/cache. No per-vendor telemetry database design.
7. **No silent semantic change.** Unit/type/entity/mapping changes preserve historical meaning through versioning/migration semantics.
8. **No silent monitoring failure.** Schema drift, Pack application failure, unresolved MIB dependency, webhook backlog and connector degradation are first-class health states.
9. **Untrusted input is hostile.** MIB/OpenAPI/YANG/Pack/fixture parsing and connector destinations are security boundaries.
10. **No arbitrary connector code by default.** Normal mappings use a bounded declarative transformation model.
11. **Webhook delivery is at-least-once.** Idempotency/deduplication is explicit; exactly-once is not claimed.
12. **Private APIs are valid but governed.** SSRF defense uses explicit egress policy, not a simplistic ban on all private addresses.
13. **Certification is evidence, not marketing.** Signature proves integrity/publisher, not certification; family similarity does not imply tested firmware support.
14. **Air-gap remains first-class.** Import/export, certification evidence and runtime operation cannot require Internet access.
15. **UI is the normal control path.** Operators should not need direct DB/YAML edits for supported extensibility lifecycle operations.

---

## 3. Formal product requirements

The following are candidate v3.2.3 PRD supplements pending independent review and reconciliation.

| PRD ID | Requirement |
|---|---|
| `PRD-EXTL-001` | Platform shall make new-device/new-version support content-first through Capability Packs, Monitor Definitions and connector mappings when the required protocol/runtime primitive already exists; code changes require explicit classification and justification. |
| `PRD-EXTL-002` | Platform shall provide governed MIB Studio workflows for import, dependency resolution, compilation, OID/trap semantics, Monitor Definition selection, device/fixture testing, revision impact and versioned publication. |
| `PRD-EXTL-003` | Platform shall provide governed API Studio workflows for secure connection/authentication, OpenAPI/manual operations, schema/entity/metric mapping, bounded pagination/chaining, testing, schema drift, versioned publication and runtime health. |
| `PRD-EXTL-004` | Platform shall use Capability Packs as the canonical signed/versioned package for device identity, compatibility, monitoring, inventory, topology, events, API/gNMI/CLI/NCM mappings, alert/RCA/dashboard/runbook semantics. |
| `PRD-EXTL-005` | Platform shall distribute Capability Packs to Sensors through verified desired/observed state with atomic activation, compatibility validation, last-known-good fallback, canary rollout, rollback/revocation and convergence telemetry. |
| `PRD-EXTL-006` | Platform shall preserve telemetry semantic integrity and control cardinality/resource impact when extensibility content changes. |
| `PRD-EXTL-007` | Platform shall expose outbound, inbound and callback webhook integrations with versioned schemas, strong authentication, replay protection, at-least-once outbound delivery, bounded retry/dead-letter and explicit operational health. |
| `PRD-EXTL-008` | Platform shall enforce tenant isolation, RBAC, separation of duties, artifact signing/provenance, hostile-input parsing controls, egress/SSRF policy, secret references and immutable audit across extensibility workflows. |
| `PRD-EXTL-009` | Platform shall persist authoritative extensibility control state in PostgreSQL and high-volume runtime/history in the existing analytical/event stack without creating vendor-specific telemetry schemas or unnecessary new infrastructure. |
| `PRD-EXTL-010` | Platform shall provide complete MIB Studio, API Studio, Capability Pack Studio and Integration Health user experiences so normal administration, testing, publication, rollout, rollback and diagnosis do not require DB/YAML editing. |
| `PRD-EXTL-011` | Platform shall provide repeatable certification/test evidence for Official/Partner/Customer content appropriate to its certification level and prevent unsupported certification claims. |
| `PRD-EXTL-012` | Platform shall remain air-gapped capable for Capability Pack/MIB/API content import/export and operation; online repositories are optional extensions, not runtime dependencies. |
| `PRD-EXTL-013` | Platform shall monitor the health, lag, resource use and failure state of Capability Pack distribution, SNMP/MIB mappings, API connectors and webhook delivery as part of platform self-monitoring. |
| `PRD-EXTL-014` | Platform shall preserve HA, backup/restore, upgrade and DR correctness for extensibility state/artifacts and detect incompatible content before runtime activation or platform upgrade. |
| `PRD-EXTL-015` | Platform may provide advanced SDK/CLI/CI/GitOps/shadow/AI/repository capabilities only when they preserve the same validation, authorization, audit, compatibility and offline guarantees as the primary product workflow. |

---

## 4. Prioritized formal requirements and acceptance evidence

### 4.1 P0 requirements

#### `REQ-EXTL-P0-001` — Extensibility boundary / content-over-code

**Required behavior**  
Classify every new vendor/model/firmware/MIB/API request as CONTENT_ONLY, PACK_EXTENSION, CONNECTOR_EXTENSION, NEW_PROTOCOL_ADAPTER or CORE_FEATURE. A new device using an already-supported transport/protocol primitive shall not require Core or Sensor source changes when its behavior can be represented by Capability Pack, MIB, API, YANG, parser or Monitor Definition contracts. Source changes are reserved for genuinely new protocol/runtime/security primitives or behavior not safely representable by the extensibility model.

**Minimum acceptance evidence**  
A review can classify a new-device request deterministically; at least one certification fixture proves a new model can be added without Core/Sensor source change; exceptions document why code is unavoidable.

#### `REQ-EXTL-P0-002` — Canonical extensibility domain model

**Required behavior**  
Define first-class tenant-scoped objects for MIB Module/Version/Dependency/OID/Trap, Monitor Definition/Version, API Connector/Version/Operation/Mapping/Checkpoint, Capability Pack/Version/Dependency/Assignment/Rollout/Application State, Webhook Endpoint/Subscription/Delivery/Attempt, Certification Run/Result and Artifact Provenance. Each object has stable ID, version/lifecycle, owner/tenant, timestamps, audit linkage and hashes/signatures where artifact-backed.

**Minimum acceptance evidence**  
PostgreSQL migrations/API schemas represent these objects without competing duplicate lifecycle models; foreign keys/uniqueness/state constraints protect referential integrity.

#### `REQ-EXTL-P0-003` — Formal lifecycle and immutable publication state

**Required behavior**  
Lifecycle state and certification state are separate. Lifecycle follows DRAFT -> VALIDATING -> VALIDATED -> PUBLISHED -> CANARY -> ACTIVE -> DEPRECATED -> RETIRED, with FAILED_VALIDATION, REJECTED, ROLLED_BACK, REVOKED and QUARANTINED exceptional states. Certification state independently records OFFICIAL_CERTIFIED, PARTNER_VERIFIED, CUSTOMER_CUSTOM, EXPERIMENTAL or UNCERTIFIED/NOT_APPLICABLE as policy requires. Publication rules may require a permitted certification state, but certification is not itself a lifecycle step. Published versions are immutable; edits create a new version.

**Minimum acceptance evidence**  
Invalid transitions are rejected server-side; published content cannot be mutated in place; state history is audited; rollback/revocation preserves historical evidence.

#### `REQ-EXTL-P0-004` — PostgreSQL transactional ownership

**Required behavior**  
PostgreSQL is authoritative for extensibility control state: metadata, versions, dependencies, compatibility, assignments, desired/applied state, monitoring definitions, connector/webhook definitions, publication/certification/rollout/rollback/provenance/approval state and transactional checkpoints. Typed relational columns/constraints are used for lifecycle/identity/ownership rather than hiding all authority inside untyped JSON.

**Minimum acceptance evidence**  
Schema ownership is explicit; HA/backup/restore covers the control state; migrations and constraints are tested; no vendor-specific telemetry tables are introduced for each MIB/API.

#### `REQ-EXTL-P0-005` — Artifact storage contract

**Required behavior**  
Raw/compiled artifacts such as MIBs, OpenAPI/YANG documents, compiled Capability Packs, sanitized walks/API/CLI/trap fixtures, manifests, signatures and certification evidence use one Platform-managed artifact abstraction. PostgreSQL stores authoritative metadata and references; artifact bodies use the approved encrypted storage mechanism. Required metadata includes immutable hash, size, media type, tenant, owner, source/provenance, retention class, encryption/signature status and backup class.

**Minimum acceptance evidence**  
Hash mismatch is detected; missing body/reference is surfaced as integrity failure; backup/restore validates metadata-to-artifact referential integrity; no new mandatory storage product is added without change control.

#### `REQ-EXTL-P0-006` — Canonical Monitor Definition schema

**Required behavior**  
A monitored OID or API field becomes telemetry only through a versioned Monitor Definition containing canonical metric key, entity type, source protocol/mapping, value/metric type, unit, collection mode, poll/default policy reference, timeout/retry policy, index/entity mapping, quality/missing-data semantics, counter semantics, cardinality policy, retention/rollup semantics and enabled capability. Metric kinds include GAUGE, COUNTER, STATE, HISTOGRAM and EVENT as applicable.

**Minimum acceptance evidence**  
Every activated Pack metric resolves to a Monitor Definition; unit/type/rollup validation is enforced before publish; mapping changes cannot silently alter historical meaning.

#### `REQ-EXTL-P0-007` — SNMP/MIB semantic contract

**Required behavior**  
MIB handling covers SMIv1/SMIv2, import/dependency graphs, duplicate modules/revisions, OID collisions, textual conventions, scalars, tables, INDEX, AUGMENTS, enums, BITS, Counter32/64, Gauge, TimeTicks, OctetString, address types, sparse tables, inaccessible objects, notifications and varbinds. Unknown/unresolved OIDs remain diagnosable. Uploading a MIB never means polling every OID; useful OIDs are explicitly selected/confirmed into Monitor Definitions and compiled into safe GET/GETNEXT/GETBULK plans.

**Minimum acceptance evidence**  
Compiler/test fixtures cover tables/indexing/counters/traps/dependency failures and collisions; selected monitors, not complete MIB trees, determine runtime polling.

#### `REQ-EXTL-P0-008` — MIB revision semantic diff and impact analysis

**Required behavior**  
A replacement/revision produces semantic diff for added/removed/renamed OIDs, type/enumeration/index/notification/dependency changes and identifies impacted Monitor Definitions, Packs, devices, dashboards and alert semantics. Breaking changes block publication until explicitly resolved.

**Minimum acceptance evidence**  
Publishing a breaking MIB revision without resolution is rejected; UI/API exposes impacted objects and a machine-readable diff; existing active version remains last-known-good.

#### `REQ-EXTL-P0-009` — API Connector bounded runtime contract

**Required behavior**  
Each API operation has bounded connect/TLS/request/job timeouts, maximum response bytes/pages/records/chained calls/retries/redirects, concurrency, requests-per-second and burst. Runtime supports Retry-After/429, safe retry classification, OAuth refresh, cursor/page/token pagination, ETag/Last-Modified where applicable and incremental checkpoints. Pagination loops/unbounded ingestion are detected and terminated.

**Minimum acceptance evidence**  
Negative tests prove page loops, huge responses, rate limits, slow endpoints and token-refresh failures cannot create unbounded CPU/memory/queue usage or silent data corruption.

#### `REQ-EXTL-P0-010` — API schema-drift detection and safe degradation

**Required behavior**  
Store expected/observed schema identity or hash and detect additive/breaking drift. Connector runtime states include HEALTHY, SCHEMA_DRIFT_WARNING, SCHEMA_DRIFT_BREAKING, AUTH_DEGRADED, RATE_LIMITED, PARTIAL and FAILED. Breaking mappings do not emit fabricated zero/default telemetry; affected monitors degrade explicitly and preserve last-success evidence.

**Minimum acceptance evidence**  
A test schema rename/type change produces visible drift, affected-metric inventory and Monitoring Degraded behavior rather than false zero values.

#### `REQ-EXTL-P0-011` — Safe declarative transformation language

**Required behavior**  
Connector transformations use the already-approved CEL foundation (`OSS-012`, cel-java/cel-go as appropriate) for bounded expressions, combined with Platform-owned structured field selectors/extractors for JSON/XML and schema mappings. Supported transformations include extraction, cast, unit/time conversion, null/default handling, enum mapping, conditionals, arithmetic, simple derived metrics and filtering. Arbitrary customer JavaScript/Python/shell execution is forbidden in normal connector mappings. Platform defines the allowed CEL variables/functions and enforces expression cost/time/resource budgets.

**Minimum acceptance evidence**  
Static/runtime validation rejects unsafe/unbounded constructs; mapping CPU/memory limits are tested; no connector definition can execute arbitrary host commands.

#### `REQ-EXTL-P0-012` — Formal Capability Pack manifest schema

**Required behavior**  
Define a versioned machine-validated manifest containing pack ID/version, manifest schema/API level, publisher/certification, creation metadata, minimum/tested Core/Sensor compatibility, dependencies, vendor/model/firmware identity rules, protocols, Monitor Definitions, inventory/topology/trap/syslog/API/gNMI/CLI/NCM/alert/RCA/dashboard/runbook capabilities, provenance, artifact hashes and signature. Secrets are references only and never embedded.

**Minimum acceptance evidence**  
JSON/schema validation is deterministic; unknown incompatible manifest versions are rejected; secrets scanners prove no credential values are packaged.

#### `REQ-EXTL-P0-013` — Deterministic device identification and Pack selection

**Required behavior**  
Pack selection evaluates evidence such as sysObjectID/sysDescr/sysName, vendor/model/serial/firmware, SNMP engine ID, API product identity and gNMI capabilities. Outcomes are EXACT_MATCH, COMPATIBLE_FAMILY_MATCH, GENERIC_FALLBACK, AMBIGUOUS or UNSUPPORTED, with evidence/confidence/compatibility reason. AMBIGUOUS never auto-activates a vendor Pack.

**Minimum acceptance evidence**  
Fixtures with overlapping identity rules produce deterministic ambiguity rather than arbitrary selection; chosen Pack/version and evidence are auditable.

#### `REQ-EXTL-P0-014` — Generic safe monitoring fallback

**Required behavior**  
Where supported safely, unknown devices may use a clearly-labelled limited generic profile (for example ICMP, standard system/interface objects, uptime and generic LLDP) without vendor-specific certification claims. Unsupported capabilities remain explicit.

**Minimum acceptance evidence**  
UI/API marks fallback as limited/generic; vendor-specific monitors/alerts are not silently enabled; moving from generic to certified Pack has an auditable assignment transition.

#### `REQ-EXTL-P0-015` — Capability Pack distribution and convergence protocol

**Required behavior**  
Core desired state drives Pack distribution. Sensor learns desired version, downloads, verifies hash/signature, checks manifest/runtime compatibility, stages, validates locally, activates atomically and reports observed version/hash/status. Partial downloads never activate. Sensor retains last-known-good active version.

**Minimum acceptance evidence**  
Integration tests cover interrupted download, hash/signature failure, incompatible runtime, Core failover and Sensor restart; desired-vs-observed convergence is queryable.

#### `REQ-EXTL-P0-016` — Failed Pack activation isolation

**Required behavior**  
A bad Pack cannot crash Sensor, terminate unrelated pollers, corrupt other Pack state, block heartbeat or overwrite last-known-good. Failure produces PACK_APPLICATION_FAILED with pack/version, Sensor, cause, affected entities, validation evidence, last-known-good version and rollback status.

**Minimum acceptance evidence**  
Fault-injection proves one corrupt Pack leaves unrelated monitoring alive and returns to last-known-good deterministically.

#### `REQ-EXTL-P0-017` — Rollback, revocation and quarantine

**Required behavior**  
Support rollback at entity/group/site/Sensor-pool/tenant/global version scopes, security revocation and emergency quarantine. Revocation prevents new assignment and starts explicit remediation for existing assignments while preserving history. Rollback is idempotent and auditable.

**Minimum acceptance evidence**  
Rollback/revoke tests verify convergence and no orphan desired/applied state; revoked versions cannot be newly activated.

#### `REQ-EXTL-P0-018` — Core/Sensor/Pack compatibility matrix

**Required behavior**  
Compatibility covers Core version, Sensor runtime version, Agent version where relevant, Pack manifest/API level, connector runtime level and event/schema versions. Incompatible Packs are rejected with actionable reason. Upgrade preflight detects Packs/connectors that will become incompatible before Core/Sensor upgrade.

**Minimum acceptance evidence**  
Upgrade certification includes compatibility scan and refusal behavior; no incompatible version is attempted in production runtime.

#### `REQ-EXTL-P0-019` — Webhook direction/type separation

**Required behavior**  
Model outbound Platform-to-external events, inbound external-to-Platform events and connector callbacks as distinct endpoint/subscription behaviors with separate authorization, schemas, validation and failure semantics. A generic webhook label may be UI shorthand but cannot collapse the security/runtime contracts.

**Minimum acceptance evidence**  
API/data model distinguishes OUTBOUND_EVENT, INBOUND_EVENT and CONNECTOR_CALLBACK; permission and signature tests are direction-specific.

#### `REQ-EXTL-P0-020` — Outbound webhook delivery semantics

**Required behavior**  
Outbound delivery is explicitly at-least-once, never falsely exactly-once. Envelope contains stable event_id, delivery_id, tenant_id, event_type, event_schema_version, occurred_at, sent_at, correlation_id, resource references and payload. Delivery states include PENDING, DELIVERING, DELIVERED, RETRY_WAIT, FAILED, DEAD_LETTER and CANCELLED. Retries preserve event identity.

**Minimum acceptance evidence**  
Receiver can deduplicate repeated deliveries; delivery state/history survives restart/failover; duplicate internal execution does not create ambiguous event identity.

#### `REQ-EXTL-P0-021` — Webhook authentication and replay protection

**Required behavior**  
Outbound signing uses per-endpoint HMAC-SHA256 baseline with optional/required mTLS by policy. Inbound validation binds endpoint to tenant and validates signature, timestamp, body hash, allowed skew, nonce/event ID replay state, payload schema and size/rate limits. Secrets are Credential/Secret references and are not readable after creation except per approved secret lifecycle.

**Minimum acceptance evidence**  
Replay, signature-tamper, stale timestamp and cross-tenant endpoint tests fail closed; key rotation is tested without exposing secrets.

#### `REQ-EXTL-P0-022` — Webhook retry, expiry and dead-letter contract

**Required behavior**  
Define retryable/non-retryable status classes, bounded exponential backoff with jitter, maximum attempts, maximum delivery age, dead-letter state, manual replay and audit. Retrying a delivery never creates a logically new business event.

**Minimum acceptance evidence**  
Tests cover 429/5xx/network failure/4xx permanent failure, restart during retry and manual replay; oldest pending/attempt count are observable.

#### `REQ-EXTL-P0-023` — Connector/webhook SSRF and egress controls

**Required behavior**  
API/webhook destinations are governed by explicit tenant/admin egress policy rather than blanket public-only rules. Validate schemes/ports/hostnames, re-resolve/revalidate DNS to mitigate rebinding, revalidate redirects, protect metadata/localhost/internal control-plane targets, respect controlled proxy/no-proxy behavior and audit approved private-network exceptions.

**Minimum acceptance evidence**  
Security tests exercise loopback, link-local/metadata, redirect-to-private, DNS rebinding and approved private API cases; legitimate private monitoring remains possible only under policy.

#### `REQ-EXTL-P0-024` — Parser/upload hostile-input protections

**Required behavior**  
MIB/OpenAPI/YANG/Pack/fixture import is treated as hostile input. Enforce file/archive size, nesting/recursion/dependency depth, decompression ratio, parser CPU/memory/time, path/archive traversal, XML XXE/entity expansion, malformed encoding and dependency-cycle limits. Parsing runs with least privilege and safe temporary-file handling.

**Minimum acceptance evidence**  
Fuzz/negative fixtures demonstrate bounded failure and no file escape, network fetch surprise, XXE, resource exhaustion or service crash.

#### `REQ-EXTL-P0-025` — Signing and publisher trust model

**Required behavior**  
Define manifest/artifact hashing, signing algorithm, Platform/publisher identity, trust roots, key rotation/revocation and verification failure. Cryptographic authenticity/integrity is distinct from certification level: a valid signature does not imply Official Certified.

**Minimum acceptance evidence**  
Verification is mandatory before activation; revoked/untrusted key behavior is deterministic; certification field cannot be inferred solely from signature validity.

#### `REQ-EXTL-P0-026` — Extensibility RBAC namespace and separation of duties

**Required behavior**  
Define fine-grained permissions for MIB view/upload/edit/test, connector view/create/edit/test, Pack view/create/edit/validate/certify/publish/assign/rollback/revoke and webhook view/create/edit/secret-rotate/replay. Edit does not imply publish; use of a credential does not imply reveal.

**Minimum acceptance evidence**  
Authorization tests cover tenant/resource scope and privilege separation; unauthorized publish/revoke/replay is blocked server-side.

#### `REQ-EXTL-P0-027` — Immutable audit contract

**Required behavior**  
Audit upload/import/edit/test/validate/certify/publish/assign/rollout/rollback/deprecate/retire/revoke/quarantine/secret-rotate/webhook-replay actions with actor/service, tenant, object/version, before/after identifiers, reason/ticket where applicable, result, approval, timestamp and request/session/correlation context.

**Minimum acceptance evidence**  
Audit evidence remains queryable after rollback/deletion of drafts; secrets/payload-sensitive data are redacted; mutation tests verify audit creation.

#### `REQ-EXTL-P0-028` — Extensibility self-observability

**Required behavior**  
Expose Pack desired/applied versions and convergence lag/failures/rollbacks; connector success/error/latency/rate-limit/token-refresh/schema-drift; SNMP monitor errors/MIB dependency failures; webhook queue depth/oldest age/retries/dead-letter/last success; signature/certification failures. Metrics/logs/events must support Platform health and support bundles without leaking secrets.

**Minimum acceptance evidence**  
Operational dashboards/health APIs show each critical failure mode; alerts exist for stuck convergence/backlog where configured; no silent failure path remains.

#### `REQ-EXTL-P0-029` — Telemetry semantic integrity across mapping changes

**Required behavior**  
Canonical metric identity keeps stable meaning. Material changes in unit, value type, entity scope or semantics require a new Monitor Definition version/key or explicit migration/compatibility rule; historical telemetry is never silently reinterpreted. Runtime preserves enough source Pack/Monitor Definition version provenance to reconstruct which semantics produced telemetry, either directly on bounded event/observation metadata where justified or through an immutable time-bounded version binding that avoids unnecessary high-cardinality duplication.

**Minimum acceptance evidence**  
Regression test changing bytes/sec to bits/sec or counter to gauge cannot silently reuse incompatible semantics; historical queries remain explainable.

#### `REQ-EXTL-P0-030` — Cardinality, MWU and storage protection

**Required behavior**  
Before activation estimate/validate monitors per entity, SNMP table rows, dynamic entity count, labels/dimensions, API records/requests, metrics/sec, storage/day and Sensor/Core MWU impact. Hard runtime bounds prevent unexpected cardinality explosions and tie into placement/capacity policy.

**Minimum acceptance evidence**  
Pack/connector test reports resource estimate; high-cardinality fixtures are blocked or require explicit policy; runtime breach degrades safely and is observable.

#### `REQ-EXTL-P0-031` — HA consistency and idempotent recovery

**Required behavior**  
Active Core is authoritative for publication/assignment. Core failover preserves published artifacts, desired state and idempotent rollout operations; a Sensor continues its last valid Pack during control-plane loss and resumes convergence after recovery. Publication cannot split-brain into conflicting versions.

**Minimum acceptance evidence**  
Two-Core failover tests cover publish/assign/rollback in-flight and Sensor offline/reconnect without duplicate or contradictory state.

#### `REQ-EXTL-P0-032` — Backup/DR consistency

**Required behavior**  
Backup/restore covers artifact bodies and metadata, Pack/monitor versions, assignments, connectors, webhooks, provenance/certification/signatures/trust metadata and required checkpoints. Restore validates hashes/signatures and references before activation. DR behavior preserves tenant isolation and last-known-good state.

**Minimum acceptance evidence**  
Restore drills verify no orphan metadata/artifacts and no unverified artifact activation; documented RPO/RTO inherits Platform DR decisions.

#### `REQ-EXTL-P0-033` — Required backend resource/API families

**Required behavior**  
Architecture freezes resource ownership for /api/v1/tenants/{tenantId}/mibs, /connectors, /capability-packs, /webhooks and their version/test/validate/publish/assignment/rollout/rollback/health/delivery sub-resources. Exact OpenAPI is source-controlled, but lifecycle/security/idempotency/pagination/concurrency semantics are normative.

**Minimum acceptance evidence**  
OpenAPI/contract tests cover each resource family; no parallel ad-hoc endpoint family bypasses the lifecycle.

#### `REQ-EXTL-P0-034` — Mutation idempotency and optimistic concurrency

**Required behavior**  
Publish, assign, rollout, rollback, revoke, secret rotation and replay operations require idempotency and version/revision preconditions. Concurrent administrators cannot unknowingly overwrite newer drafts/state. Duplicate retries are safe.

**Minimum acceptance evidence**  
Contract tests cover duplicate idempotency keys, stale If-Match/revision and concurrent publish/assign requests.

#### `REQ-EXTL-P0-035` — Certification and mandatory test gate

**Required behavior**  
Official certification requires applicable identification, polling/table/counter/trap/API/pagination/auth/token/gNMI/CLI/topology/inventory/alert/dashboard/NCM/failure/capacity/restart/update/rollback evidence. Customer/Partner content still passes structural, security, compatibility and resource-safety validation even when not Official Certified.

**Minimum acceptance evidence**  
Certification run/result objects store evidence and block certification/publish states when mandatory tests fail.

#### `REQ-EXTL-P0-036` — Requirements traceability truthfulness

**Required behavior**  
Retain legacy REQ-EXT-001..007 as umbrella requirements but add granular REQ-EXTL-* requirements and remove any implication that a single checkmark proves implementation completeness. A requirement is complete only when its governing contract, code, tests, observability, security/tenant isolation and operational evidence exist.

**Minimum acceptance evidence**  
RTM maps every granular requirement to the specialized master and downstream evidence; no broad umbrella row can mask failed granular requirements.

### 4.2 P1 requirements

#### `REQ-EXTL-P1-001` — Capability Pack composition and overlays

**Required behavior**  
Support controlled composition such as vendor-family base plus model/firmware overlays to avoid duplication. Inheritance/resolution is deterministic, bounded and versioned; Sensor receives a resolved immutable manifest rather than dynamically resolving an uncontrolled graph during polling.

**Minimum acceptance evidence**  
Resolver tests prove deterministic results, cycle/conflict detection and identical resolved content hash across nodes.

#### `REQ-EXTL-P1-002` — Pack dependency graph

**Required behavior**  
Represent mandatory/optional dependencies with version constraints and resolved versions. Detect cycles and unresolved mandatory dependencies before publication.

**Minimum acceptance evidence**  
Publish fails on cycles/missing/incompatible mandatory dependency; resolved dependency set is recorded in the published version.

#### `REQ-EXTL-P1-003` — Change-impact preview

**Required behavior**  
Before publish/assignment show affected tenants/entities/Sensors/monitors/dashboards/policies/alerts/topology and estimated MWU, metrics/sec/storage/capability changes.

**Minimum acceptance evidence**  
UI/API provides bounded impact result and stores review evidence for material rollout.

#### `REQ-EXTL-P1-004` — Complete MIB Studio UI

**Required behavior**  
Specify MIB Library, Imports/Dependencies, provenance, compiler errors, OID explorer, selected monitors, table/index semantics, traps, device test/walk, sample values, revision diff/impact, versions, usage, publish and audit surfaces with loading/empty/error/permission states.

**Minimum acceptance evidence**  
UI E2E covers create/import/test/diff/publish failure and RBAC/tenant isolation; raw file editing is not the normal admin path.

#### `REQ-EXTL-P1-005` — Complete API Studio UI

**Required behavior**  
Specify connection/auth, OpenAPI/manual operations, request test console, schema explorer, entity/metric mapping, pagination/chaining, rate/runtime budgets, checkpointing, drift, test results, versions/publish, health and audit surfaces.

**Minimum acceptance evidence**  
UI E2E proves a connector can be authored/tested/published and drift remediated without DB/YAML edits.

#### `REQ-EXTL-P1-006` — Complete Capability Pack Studio UI

**Required behavior**  
Specify manifest/identity/compatibility/protocols/metrics/inventory/topology/events/alerts/dashboards/NCM/RCA/runbooks/dependencies/Test Lab/certification/versions/diff/usage/assignments/rollout/rollback/audit.

**Minimum acceptance evidence**  
UI E2E covers draft->validate->publish->canary->active->rollback and permission failures.

#### `REQ-EXTL-P1-007` — Capability Pack Test Lab fixtures

**Required behavior**  
Formalize repeatable sanitized fixtures for SNMP walk/response/trap, syslog, API JSON/XML, gNMI update, CLI output and topology samples so content can be tested without production devices where feasible.

**Minimum acceptance evidence**  
Fixtures are versioned, bounded and redacted; certification can replay them deterministically.

#### `REQ-EXTL-P1-008` — Certification evidence matrix

**Required behavior**  
Store tested vendor/model/firmware/protocol/Core/Sensor/Pack version, fixture/device evidence, test date/result, known limitations and certifier. Certification applies only to the tested compatibility range.

**Minimum acceptance evidence**  
UI/API never represents untested firmware as certified solely by family match.

#### `REQ-EXTL-P1-009` — Progressive Pack rollout

**Required behavior**  
Support TEST, CANARY, LIMITED and FULL cohorts scoped to selected entity/group/site/Sensor pool/tenant with halt criteria on severe failure. Automatic rollback, if enabled, uses explicit policy and does not trigger on trivial warnings.

**Minimum acceptance evidence**  
Canary tests show halt/continue/rollback decisions are auditable and scoped.

#### `REQ-EXTL-P1-010` — Semantic Pack/version comparison

**Required behavior**  
Provide semantic diff for monitors/OIDs/API paths/types/units/topology/alerts/parsers/NCM/dashboards/compatibility rather than requiring raw JSON diff.

**Minimum acceptance evidence**  
Diff accurately identifies breaking changes and feeds impact preview.

#### `REQ-EXTL-P1-011` — Canonical metric catalogue

**Required behavior**  
Maintain canonical metric catalogue with key/title/description/domain/value type/unit/semantic type/allowed aggregation/default display/directionality/deprecation to avoid vendor-specific semantic duplicates.

**Minimum acceptance evidence**  
Pack validation flags conflicting duplicate metric semantics and suggests/requires canonical mapping.

#### `REQ-EXTL-P1-012` — Durable API checkpointing

**Required behavior**  
Persist safe incremental cursors/watermarks/tokens/last successful sync/page state where connector semantics permit. Partial failure resumes without replaying an unbounded full dataset, while checkpoint commits only after durable acceptance.

**Minimum acceptance evidence**  
Restart/failure tests prove no silent skips and bounded duplication around checkpoints.

#### `REQ-EXTL-P1-013` — Schema-drift remediation workflow

**Required behavior**  
Provide Detected -> Impacted -> Mapping Fix Draft -> Test -> Publish -> Recovered workflow with last-good/new schema comparison and affected mappings.

**Minimum acceptance evidence**  
Operator can remediate drift through normal UI/API; recovery restores collection without editing DB/YAML.

#### `REQ-EXTL-P1-014` — Integration delivery operations

**Required behavior**  
Expose connector/webhook queue, retrying/delivered/failed/dead-letter counts, throughput, latency, last success, oldest pending, rate-limit state, endpoint health and controlled replay/pause/resume actions.

**Minimum acceptance evidence**  
Integration Health UI/API and operational alerts cover stuck queues and failure diagnosis.

#### `REQ-EXTL-P1-015` — External event subscription catalogue

**Required behavior**  
Outbound webhooks subscribe to stable versioned external event types such as alert/incident/device/sensor/integration state events, not arbitrary internal Kafka topic contracts. Internal Kafka schemas remain implementation details.

**Minimum acceptance evidence**  
Event catalogue/version tests protect compatibility and document deprecation.

#### `REQ-EXTL-P1-016` — Integration data minimization

**Required behavior**  
Define payload field allowlists/templates, tenant/resource scope, sensitive classifications and redaction so integrations export only intended data.

**Minimum acceptance evidence**  
Security tests confirm credentials/private evidence cannot leak through default webhook payloads.

#### `REQ-EXTL-P1-017` — Provenance and licensing governance

**Required behavior**  
Track source/owner/publisher/licence/redistribution rights/customer-only flag/received-at/hash/original filename for MIB/YANG/OpenAPI/CLI/template artifacts. Customer-only vendor material is never redistributed to other tenants.

**Minimum acceptance evidence**  
Export/import and support workflows enforce redistribution policy and tenant scope.

#### `REQ-EXTL-P1-018` — Air-gapped import/export

**Required behavior**  
Support signed offline bundles containing manifests, dependencies, compatibility, hashes, provenance/certification and optional sanitized fixtures. Internet/marketplace access is never mandatory.

**Minimum acceptance evidence**  
Offline install/import test verifies complete dependency resolution and signature validation with no external fetch.

#### `REQ-EXTL-P1-019` — Pack lifecycle/deprecation policy

**Required behavior**  
Keep lifecycle, support and security status semantically distinct. Lifecycle remains the P0 state machine; support status distinguishes SUPPORTED, DEPRECATED and END_OF_SUPPORT; REVOKED/QUARANTINED remain security-enforcement states. Deprecation/end-of-support provides a migration path while revocation can block new activation and require remediation.

**Minimum acceptance evidence**  
UI/API and assignment validation distinguish the states; migration/revocation actions are auditable.

#### `REQ-EXTL-P1-020` — Operational runbooks

**Required behavior**  
Provide runbooks for publication/convergence/activation failure, bad Pack rollback, revocation, MIB dependency failure, API drift/rate-limit/OAuth failure, webhook backlog/DLQ and signature failure.

**Minimum acceptance evidence**  
Each runbook has trigger, safety action, evidence, remediation, verification and escalation; support exercises validate key paths.

#### `REQ-EXTL-P1-021` — Support bundle extensions

**Required behavior**  
Support bundles include sanitized current/desired Pack versions, convergence/error state, connector runtime/drift state, MIB dependency errors and webhook queue statistics without secrets.

**Minimum acceptance evidence**  
Automated secret tests scan support bundle; support evidence correlates by IDs/correlation IDs.

#### `REQ-EXTL-P1-022` — Performance certification

**Required behavior**  
Benchmark large OID sets/tables/high-interface devices/SNMPv3/high RTT/API pagination/large responses/rate limits/many connectors/webhook subscribers/trap storms/schema drift and fleet Pack activation.

**Minimum acceptance evidence**  
Release benchmark evidence states limits and regression thresholds for certified workloads.

### 4.3 P2 requirements

#### `REQ-EXTL-P2-001` — Capability Pack SDK

**Required behavior**  
Provide supported schema/fixture/examples/validation/signing/compatibility guidance for internal/partner/customer development without making SDK use mandatory for runtime.

**Minimum acceptance evidence**  
SDK artifacts are versioned and compatible with server validation.

#### `REQ-EXTL-P2-002` — Pack CLI validator

**Required behavior**  
Provide optional CLI operations such as validate/test/diff/inspect/sign while server-side validation remains authoritative.

**Minimum acceptance evidence**  
CLI and server produce compatible validation outcomes for the same manifest.

#### `REQ-EXTL-P2-003` — Capability Pack CI integration

**Required behavior**  
Allow Pack repositories to run schema/dependency/fixture/static-security/compatibility tests in CI for official/partner content.

**Minimum acceptance evidence**  
CI result can attach to certification evidence but cannot bypass server publish authorization.

#### `REQ-EXTL-P2-004` — Shadow Pack evaluation

**Required behavior**  
Allow version N+1 to run on selected entities in non-authoritative shadow mode, comparing extraction, metric count/errors/resources/alerts/topology without changing production telemetry/alerts.

**Minimum acceptance evidence**  
Shadow mode is isolated, bounded and clearly labelled; promotion requires normal approval.

#### `REQ-EXTL-P2-005` — AI-assisted Pack authoring

**Required behavior**  
AI may suggest MIB/API mappings, canonical metric names/units, topology, dashboards and descriptions, always labelled AI Suggested until human validated. AI cannot publish directly.

**Minimum acceptance evidence**  
Authorization tests prove AI path cannot invoke publish; accepted/rejected suggestions are attributable.

#### `REQ-EXTL-P2-006` — Automated compatibility recommendations

**Required behavior**  
Platform may suggest likely compatible Packs for unknown devices using evidence but must not silently activate or mark Official Certified solely on similarity.

**Minimum acceptance evidence**  
Recommendation retains evidence/confidence and requires policy/operator decision when not deterministic.

#### `REQ-EXTL-P2-007` — Promotion environments

**Required behavior**  
Optionally support DEVELOPMENT -> TEST -> STAGING -> PRODUCTION content promotion for enterprises/MSPs while preserving immutable versions and tenant boundaries.

**Minimum acceptance evidence**  
Promotion is explicit and auditable; environments do not create divergent mutable versions.

#### `REQ-EXTL-P2-008` — GitOps-style content management

**Required behavior**  
Permit controlled versioned export/import of Packs/connectors/webhook/monitor definitions while exporting secret references only, never plaintext secret values.

**Minimum acceptance evidence**  
Round-trip preserves IDs/versions where policy allows and rejects unauthorized tenant/environment import.

#### `REQ-EXTL-P2-009` — Repository/offline mirror architecture

**Required behavior**  
Future Pack repository may use signed indexes, hashes, compatibility metadata and customer-approved synchronization with offline mirror support. Runtime cannot depend on Internet availability.

**Minimum acceptance evidence**  
Repository compromise/unavailability cannot invalidate already verified local Pack operation.

#### `REQ-EXTL-P2-010` — External publisher trust chains

**Required behavior**  
Allow approved partner publisher keys under administrator trust policy with rotation/revocation and scoped authority.

**Minimum acceptance evidence**  
Untrusted publisher content cannot activate; partner signature still does not imply Official certification.

#### `REQ-EXTL-P2-011` — Automated API drift suggestions

**Required behavior**  
Platform/AI may generate draft mapping migration proposals from observed schema drift, but proposals require validation and authorized publication.

**Minimum acceptance evidence**  
No production mapping changes automatically solely from inference.

#### `REQ-EXTL-P2-012` — Advanced webhook replay tooling

**Required behavior**  
Provide bounded replay by delivery/time/event type/endpoint/failed-only with deduplication warnings, authorization and audit.

**Minimum acceptance evidence**  
Bulk replay is rate/batch limited and cannot bypass tenant/resource/event-retention policy.

#### `REQ-EXTL-P2-013` — Connector cost/quota analytics

**Required behavior**  
Expose requests/day, bytes, rows, metrics generated, latency, rate-limit/quota events and optional customer-supplied cost/quota context.

**Minimum acceptance evidence**  
Analytics uses measured values and labels estimates/assumptions; it does not fabricate vendor billing.

#### `REQ-EXTL-P2-014` — Reusable Pack components

**Required behavior**  
Allow deterministic reuse of common standards/components such as IF-MIB, ENTITY-MIB, LLDP or common hardware semantics without uncontrolled inheritance.

**Minimum acceptance evidence**  
Resolved published Pack has complete immutable dependency/component hashes and deterministic behavior.

---

## 5. Governing architecture and HLD contract

### 5.1 Logical placement

Extensibility remains inside the existing modular Platform architecture:

```text
Authoring / Control Plane
  MIB Studio -----------  API Studio ------------+--> Extensibility Registry / Artifact Metadata (PostgreSQL)
  Capability Pack Studio ----+--> Artifact Storage (existing approved abstraction)
  Webhook/Connector UI --/
                               |
                         validate / test / certify
                               |
                         sign / publish / assign
                               |
                    desired state + artifact refs
                               |
                 +-------------+-------------+
                 |                           |
              Sensor A                    Sensor B
                 |                           |
        verify / stage / activate    verify / stage / activate
                 |                           |
        SNMP/API/gNMI/CLI/etc.       SNMP/API/gNMI/CLI/etc.
                 +-------------+-------------+
                               |
                    Canonical observations/events
                               |
                             Kafka
                    +----------+----------+
                    |                     |
               ClickHouse         Alert/RCA/current state
                    |                     |
                    +----------+----------+
                               |
                          Query APIs / UI
```

No separate MIB microservice, Capability Pack microservice, API Studio microservice, webhook microservice or new database is created merely because these are separate UI/domain concerns. A later split requires measured scale/failure-isolation evidence and normal architecture change control.

### 5.2 Ownership boundaries

- **Extensibility Registry/Core modules:** drafts, versions, provenance, validation orchestration, certification, publication, assignments, rollout desired state, RBAC/audit.
- **Credential/Secret subsystem:** auth secret material and rotation references. Capability Packs/connectors store secret references only.
- **Sensor Runtime:** device/API execution, Pack verification/staging/activation, local last-known-good, bounded collection, runtime health and observed state.
- **Ingestion:** canonical observation/event acceptance and correlation identifiers; it does not understand vendor-specific MIB semantics beyond normalized contracts.
- **Integration/Notification connector runtime:** outbound/inbound/callback delivery under the same connector security/rate/idempotency controls.
- **Artifact storage abstraction:** immutable bodies/fixtures/manifests/signatures with PostgreSQL authoritative metadata/reference ownership.

### 5.3 Extensibility classification decision

Every requested new integration/device capability must be classified before implementation:

| Class | Meaning | Typical action |
|---|---|---|
| `CONTENT_ONLY` | Existing Pack/connector schemas can represent the change | New version of content only |
| `PACK_EXTENSION` | Existing runtime supports it but Pack schema/content must grow | Capability Pack + Monitor Definitions |
| `CONNECTOR_EXTENSION` | Existing HTTP/API runtime supports it but connector mapping/operation changes | API Connector version |
| `NEW_PROTOCOL_ADAPTER` | Runtime lacks required protocol/transport primitive | Architecture/OSS/security review + adapter implementation |
| `CORE_FEATURE` | Behavior is not safely representable as content/runtime adapter | Formal product/architecture feature |

The classification result and justification are retained with design/issue evidence for material work.

### 5.4 Capability Pack resolution

Identity matching is deterministic. Order is exact identity evidence first, compatible family rules second, generic fallback last. Ambiguity never silently chooses a vendor Pack. Pack composition, when used, is resolved before publication/assignment into an immutable content hash.

### 5.5 Desired/observed convergence

For every assigned Pack, Core stores desired Pack/version/revision and Sensor reports observed Pack/version/hash/application state. Convergence is idempotent, resumable and visible. A control-plane outage does not deactivate a previously valid Pack.

---

## 6. LLD, data, storage and state contracts

### 6.1 PostgreSQL logical schemas

Exact physical schema names may follow repository conventions, but the following authoritative records must exist as typed control-state concepts:

```text
extensibility.mib_module
extensibility.mib_module_version
extensibility.mib_dependency
extensibility.oid_definition
extensibility.trap_definition
extensibility.monitor_definition
extensibility.monitor_definition_version

extensibility.api_connector
extensibility.api_connector_version
extensibility.api_operation
extensibility.api_mapping
extensibility.connector_checkpoint
extensibility.connector_runtime_state

extensibility.capability_pack
extensibility.capability_pack_version
extensibility.capability_pack_dependency
extensibility.capability_pack_assignment
extensibility.capability_pack_rollout
extensibility.capability_pack_application_status

extensibility.certification_run
extensibility.certification_result
extensibility.artifact
extensibility.artifact_provenance

integration.webhook_endpoint
integration.webhook_subscription
integration.webhook_delivery
integration.webhook_delivery_attempt
```

Repository conventions may choose a different approved schema prefix, but must preserve the ownership and relationship model. Tables may be consolidated only when typed constraints and lifecycle semantics remain explicit.

### 6.2 Required common fields

Versioned domain records require as applicable:

```text
id UUID/approved canonical ID
stable logical object ID
version/revision
 tenant_id / scope
state
created_at / created_by
updated_at / updated_by
published_at / published_by
artifact/content hash
schema/API level
provenance reference
compatibility metadata
row revision for optimistic concurrency
audit/correlation reference
```

Published records are immutable. Soft-deletion of drafts must not destroy required audit/certification history.

### 6.3 Monitor Definition contract

Minimum logical contract:

```text
monitor_definition_id
version
canonical_metric_key
entity_type
metric_kind              # GAUGE | COUNTER | STATE | HISTOGRAM | EVENT
source_protocol
source_mapping
value_type
unit
collection_mode
poll/default policy reference
timeout/retry policy
index/entity mapping
missing-data semantics
quality semantics
counter rollover/reset semantics
cardinality policy
retention class
rollup semantics
enabled capability
```

Counter semantics must distinguish reset/wrap where technically inferable. Table/index mappings must produce stable entity identity and avoid high-cardinality labels from arbitrary values.

### 6.4 MIB model

Store source module/version/revision, dependency graph, compiled symbol/OID metadata, textual conventions, table/index relationships, notification definitions and provenance. Dependency resolution is deterministic and detects missing imports, cycles, duplicate/conflicting modules and OID/type conflicts. Normal MIB dependency resolution uses the tenant/Platform library and supplied bundle; it must not silently fetch arbitrary Internet dependencies during compile/test, preserving air-gap and supply-chain controls.

### 6.5 API connector model

A connector version includes endpoint/base URL policy, auth-profile reference, TLS/trust policy, operations, request templates, schema expectations, entity/metric mappings, pagination/chaining, runtime budgets, retry/rate rules, checkpoint semantics, drift policy and test evidence. Credentials/tokens are never stored inline.

### 6.6 Capability Pack manifest minimum schema

```yaml
pack_id: immutable logical ID
pack_version: immutable version
manifest_schema_version: versioned schema
pack_api_level: runtime contract level
publisher: identity/reference
publisher_type: PLATFORM | PARTNER | CUSTOMER
certification_state: OFFICIAL_CERTIFIED | PARTNER_VERIFIED | CUSTOMER_CUSTOM | EXPERIMENTAL
compatibility:
  min_core_version: ...
  min_sensor_version: ...
  tested_sensor_versions: [...]
  vendors: [...]
  models: [...]
  firmware_ranges: [...]
identity_rules: [...]
dependencies: [...]
protocols:
  snmp: ...
  rest: ...
  gnmi: ...
  netconf: ...
  ssh_cli: ...
monitor_definitions: [...]
inventory_mappings: [...]
topology_rules: [...]
trap_rules: [...]
syslog_rules: [...]
api_connectors: [...]
cli_parsers: [...]
ncm_capabilities: [...]
alert_semantics: [...]
rca_semantics: [...]
dashboards: [...]
runbooks: [...]
provenance: [...]
artifact_hashes: [...]
signature: ...
```

### 6.7 Artifact model

Artifact metadata and body are separate. The metadata record binds tenant/scope, content hash, size/media type, storage locator, encryption state, provenance/licensing, retention and signature. Storage locators are never treated as user-authorized direct file paths.

### 6.8 ClickHouse / runtime history

ClickHouse may store high-volume historical health/analytics such as connector request latency/result/rate-limit events, webhook delivery analytics, Pack application/convergence history at fleet scale and schema-drift/runtime events where query volume justifies it. PostgreSQL remains authoritative for current lifecycle state and delivery/assignment identity. Do not move transactional truth to ClickHouse.

### 6.9 Kafka events

Use versioned internal event envelopes for material state changes. Minimum families include Pack published/assigned/application-result/rollback/revocation, connector runtime/drift, certification result and webhook delivery state where asynchronous orchestration requires it. Events include tenant/object/version/correlation/idempotency identity. Kafka topic names remain implementation-owned, but event semantics/schema compatibility are contract-tested.

### 6.10 State-machine enforcement

State transitions are server-enforced, not UI conventions. Invalid transitions, stale revisions and incompatible desired states return stable typed errors. Rollback/revocation transitions are separate from ordinary deprecation.

---

## 7. Backend API, event and webhook contracts

### 7.1 Resource families

At minimum the public/internal management API surface owns:

```text
/api/v1/tenants/{tenantId}/mibs
/api/v1/tenants/{tenantId}/mibs/{id}/versions
/api/v1/tenants/{tenantId}/mibs/{id}/validate
/api/v1/tenants/{tenantId}/mibs/{id}/test
/api/v1/tenants/{tenantId}/mibs/{id}/impact

/api/v1/tenants/{tenantId}/connectors
/api/v1/tenants/{tenantId}/connectors/{id}/versions
/api/v1/tenants/{tenantId}/connectors/{id}/validate
/api/v1/tenants/{tenantId}/connectors/{id}/test
/api/v1/tenants/{tenantId}/connectors/{id}/health
/api/v1/tenants/{tenantId}/connectors/{id}/drift

/api/v1/tenants/{tenantId}/capability-packs
/api/v1/tenants/{tenantId}/capability-packs/{id}/versions
/api/v1/tenants/{tenantId}/capability-packs/{id}/validate
/api/v1/tenants/{tenantId}/capability-packs/{id}/certification-runs
/api/v1/tenants/{tenantId}/capability-packs/{id}/publish
/api/v1/tenants/{tenantId}/capability-packs/{id}/assignments
/api/v1/tenants/{tenantId}/capability-packs/{id}/rollouts
/api/v1/tenants/{tenantId}/capability-packs/{id}/rollback
/api/v1/tenants/{tenantId}/capability-packs/{id}/revoke
/api/v1/tenants/{tenantId}/capability-packs/{id}/usage

/api/v1/tenants/{tenantId}/webhooks
/api/v1/tenants/{tenantId}/webhooks/{id}/test
/api/v1/tenants/{tenantId}/webhooks/{id}/rotate-secret
/api/v1/tenants/{tenantId}/webhooks/{id}/deliveries
/api/v1/tenants/{tenantId}/webhooks/{id}/replay
```

Exact URIs may be normalized to repository API conventions, but no implementation may bypass the resource/lifecycle responsibilities above.

### 7.2 Common mutation semantics

- authenticated/authorized tenant/resource scope;
- stable errors and correlation ID;
- idempotency key for side-effecting retryable operations;
- optimistic concurrency using revision/ETag/If-Match or equivalent;
- bounded pagination/filter/sort;
- immutable published versions;
- audit on security/lifecycle mutations;
- no secret echo in response/logs.

### 7.3 API runtime error taxonomy

At minimum support stable classes for:

`CONNECT_TIMEOUT`, `TLS_FAILURE`, `AUTHENTICATION_FAILED`, `AUTHORIZATION_FAILED`, `RATE_LIMITED`, `RESPONSE_TOO_LARGE`, `PAGINATION_LOOP`, `SCHEMA_DRIFT_WARNING`, `SCHEMA_DRIFT_BREAKING`, `MAPPING_ERROR`, `CHECKPOINT_ERROR`, `DESTINATION_BLOCKED_BY_EGRESS_POLICY`, `UNSUPPORTED_CONTENT_TYPE`, `CONNECTOR_RUNTIME_BUDGET_EXCEEDED`.

### 7.4 External webhook envelope

Outbound external event contract:

```json
{
  "event_id": "stable logical event id",
  "delivery_id": "delivery attempt family id",
  "tenant_id": "tenant scope",
  "event_type": "alert.opened",
  "event_schema_version": 1,
  "occurred_at": "RFC3339",
  "sent_at": "RFC3339",
  "correlation_id": "trace/correlation id",
  "resource": {"type":"...","id":"..."},
  "payload": {}
}
```

Payload schemas are versioned and externally supported. Internal Kafka payloads/topics are not exposed as the external integration contract.

### 7.5 Outbound webhook delivery

Delivery is at-least-once. `event_id` remains stable across retries. `delivery_id` identifies the endpoint delivery object. Attempts record status, HTTP result/error category, bounded response metadata, started/completed times and next retry. Response bodies are size-limited/redacted and never become unbounded logs.

### 7.6 Inbound webhook validation

Inbound endpoint identity resolves tenant and allowed event family before payload processing. Validate method/path, signature, timestamp/skew, body hash, nonce/event ID replay cache, content type/schema, size, rate and authorization. Failed validation does not create a trusted business event.

### 7.7 Egress destination policy

Connector/webhook target validation evaluates scheme, host, resolved IP, port, network zone/policy, redirect target and DNS re-resolution. Explicitly approved private customer APIs are supported; metadata/control-plane/loopback/link-local destinations are denied unless a separate tightly-scoped administrative architecture permits them. Every exception is attributable and auditable.
---

## 8. End-to-end dataflows

### 8.1 New MIB to active monitoring

```text
Upload MIB(s)
 -> size/type/provenance/hash checks
 -> dependency resolution
 -> parse/compile bounded sandbox
 -> OID/table/trap semantic model
 -> select/confirm Monitor Definitions
 -> sample fixture/device test
 -> cardinality/MWU impact
 -> Capability Pack draft/update
 -> validation/security checks
 -> certification level evidence
 -> publish immutable version
 -> canary assignment
 -> Sensor download/hash/signature/compatibility verify
 -> stage/local validate
 -> atomic activate
 -> observed-state ack
 -> SNMP collection plan execution
 -> canonical observations/events
 -> Kafka / ClickHouse / alert/RCA/query/UI
```

### 8.2 MIB revision

```text
new module revision
 -> semantic diff old/new
 -> affected monitors/packs/entities/UI/alerts
 -> breaking-change resolution
 -> new immutable versions
 -> test/certify
 -> canary
 -> promote or rollback
```

### 8.3 New API connector

```text
OpenAPI/manual definition
 -> egress/auth/TLS policy
 -> bounded test request
 -> schema capture/inference
 -> entity/metric mappings
 -> pagination/chaining/checkpoint
 -> runtime budget + cardinality/MWU preview
 -> fixture/live test
 -> publish connector/Pack version
 -> schedule to eligible Sensor/runtime
 -> bounded execution
 -> durable acceptance/checkpoint commit
 -> canonical observations
```

Schema drift enters the explicit drift workflow rather than fabricating data.

### 8.4 Pack distribution/failure

```text
Core desired version
 -> Sensor fetch
 -> verify signature/hash
 -> verify compatibility
 -> stage
 -> local validation
 -> atomic activation
 -> report applied version/hash
```

On failure:

```text
activation rejected
 -> keep last-known-good
 -> PACK_APPLICATION_FAILED
 -> affected entity status/health
 -> retry only when safe/policy allows
 -> operator/automatic scoped rollback where configured
```

### 8.5 Outbound webhook

```text
supported external event generated
 -> subscription/resource authorization
 -> payload minimization/schema version
 -> delivery object persisted
 -> sign request
 -> send under egress/rate policy
 -> delivered OR bounded retry
 -> DEAD_LETTER after policy exhaustion
 -> authorized replay if required
```

### 8.6 Inbound webhook

```text
request received
 -> endpoint/tenant resolution
 -> rate/size gate
 -> signature/timestamp/replay validation
 -> schema validation
 -> authorization/mapping
 -> idempotent domain command/event
 -> audit
 -> bounded response
```

---

## 9. UI/UX contracts

### 9.1 Common Studio behavior

MIB Studio, API Studio and Capability Pack Studio must provide: draft status, immutable versions, dirty/unsaved state, validation results, test evidence, permissions, audit history, semantic diff, usage/impact, publication state, clear failure remediation and tenant scope. Normal operations do not require editing database rows or raw YAML.

### 9.2 MIB Studio

Required surfaces:

- **Library:** module/vendor/revision/source/provenance/licensing/state/usage.
- **Imports & Dependencies:** graph, missing/conflicting import diagnostics.
- **Compiler:** exact source/module error, warning and unsupported semantic.
- **OID Explorer:** hierarchy, syntax/type/access, table/index/AUGMENTS, enum/BITS/textual convention.
- **Monitor Builder:** selected OIDs only, canonical metric, type/unit/entity/index/poll/retention/cardinality/quality semantics.
- **Trap Builder:** notification OID, varbind semantics, normalization/event mapping.
- **Test:** sample-device or fixture walk/test, raw value + normalized preview, timeout/auth/error diagnostics.
- **Revision Diff/Impact:** semantic old/new diff and affected objects.
- **Versions/Publish/Usage/Audit.**

### 9.3 API Studio

Required surfaces:

- Connection/egress destination;
- Authentication/TLS and secret reference;
- OpenAPI import/manual operations;
- Test request console with sanitized request/response;
- Schema explorer;
- entity and metric mappings;
- transformations;
- pagination/chained requests;
- runtime/rate budgets;
- checkpointing;
- cardinality/MWU preview;
- schema drift and remediation;
- versions/test/publish/health/audit.

### 9.4 Capability Pack Studio

Required surfaces:

- Overview/manifest;
- identity matching and ambiguity tests;
- compatibility matrix;
- protocols;
- metrics/Monitor Definitions;
- inventory;
- topology;
- events/traps/syslog;
- API/gNMI/CLI mappings;
- alerts/RCA;
- dashboards;
- NCM/runbooks;
- dependencies;
- Test Lab;
- certification evidence;
- versions/semantic diff;
- usage/impact;
- assignments;
- rollout/convergence;
- rollback/revocation;
- audit.

### 9.5 Integration/Webhook health

Required operational views include endpoint/connector state, queue depth/age, current attempt/retry, throughput, success rate, latency, rate-limit state, last success, dead-letter count, schema drift, test action, pause/enable, replay where authorized, secret-rotation status and audit. Payload/secret visibility follows least privilege and redaction.

### 9.6 UI quality states

Every screen defines loading, empty, partial, permission denied, stale data, validation failure, runtime failure and backend unavailable behavior. Large lists are server-paginated. Long-running validation/certification/rollout is represented by durable job/status state, not a browser-only spinner.

---

## 10. Security and threat-model additions

Threat classes include:

- malicious/corrupt Capability Pack or artifact substitution;
- hostile MIB/OpenAPI/YANG/archive parser input;
- dependency cycles/resource-exhaustion parser bombs;
- SSRF, DNS rebinding and redirect-based egress bypass;
- connector credential theft or accidental secret export;
- arbitrary-code injection through transformations/parsers;
- webhook forgery/replay/cross-tenant endpoint confusion;
- Pack publisher key compromise/revoked signing key;
- unauthorized publish/assign/revoke/replay;
- certification spoofing;
- Pack identity-rule poisoning causing wrong-device selection;
- cardinality/response-size/rate-limit resource exhaustion;
- artifact provenance/licensing leakage across tenants;
- support bundle/log leakage of credentials/payloads;
- compromised connector returning adversarial schema/content;
- rollback downgrade to revoked/insecure version.

Mandatory mitigations are defined by P0 requirements: parser/resource bounds, signing/trust, immutable versions, RBAC/separation of duties, tenant scope, egress policy, constrained transforms, secret references, webhook HMAC/mTLS/replay checks, audit, cardinality/MWU bounds, last-known-good and revocation checks.

No client-side/UI control counts as an authorization or security boundary.

---

## 11. Capacity, cardinality, performance and storage

### 11.1 Pre-activation estimate

MIB/API/Pack tests calculate where possible:

- scalar/table monitors per entity;
- estimated table row/entity count;
- polling requests and response size;
- API calls/pages/objects per run;
- metrics/events per second;
- dynamic labels/dimensions;
- Sensor CPU/memory/network/disk impact;
- Core ingestion impact;
- estimated ClickHouse storage/day before rollup/compression assumptions;
- webhook subscription delivery rate/backlog risk.

### 11.2 Runtime protections

Use bounded queues, rate/concurrency limits, response/body limits, pagination limits, backpressure, cardinality policy and Sensor/Core storage protection. Runtime breaches produce explicit degraded/limited status and do not silently drop critical control state.

### 11.3 Benchmark profiles

Certification benchmarks include large SNMP tables/high interface count, SNMPv3/high RTT, bulk efficiency, trap storms, large/paginated APIs, 429 behavior, many connectors, webhook fanout, Pack fleet activation/convergence, schema drift and restart/failover recovery.

---

## 12. Quality, certification and release gates

### 12.1 Mandatory P0 test classes

- state-machine and database-constraint tests;
- API contract/idempotency/concurrency tests;
- MIB parser/dependency/table/index/counter/trap tests;
- API pagination/rate/timeout/response/drift/checkpoint tests;
- Pack signing/compatibility/distribution/atomic activation/LKG tests;
- Sensor restart/Core failover/canary/rollback/revocation tests;
- tenant/RBAC/separation-of-duties tests;
- SSRF/DNS-rebinding/redirect/egress tests;
- hostile artifact parser/fuzz tests;
- webhook signature/replay/retry/DLQ tests;
- cardinality/MWU/resource-pressure tests;
- backup/restore/integrity tests;
- UI E2E for author/test/publish/rollout/rollback/drift/health workflows.

### 12.2 Certification levels

- **Official Certified:** Platform-controlled evidence meets the mandatory certification matrix for the declared vendor/model/firmware/runtime range.
- **Partner Verified:** approved partner evidence plus mandatory Platform structural/security/runtime-safety validation; UI clearly distinguishes from Official.
- **Customer Custom:** customer-authored/owned; mandatory structural/security/compatibility/resource-safety validation still applies; no vendor certification claim.
- **Experimental:** explicitly limited/non-production state; cannot silently become active production content where policy forbids it.

### 12.3 Release blockers

For affected capabilities, GA is blocked by unresolved P0 defects involving tenant isolation, unauthorized publication, signature/trust bypass, Pack activation corrupting Sensor runtime, silent schema drift/semantic corruption, uncontrolled SSRF, unbounded parser/runtime resource use, broken rollback/LKG, uncontrolled cardinality, webhook replay/auth bypass, or backup/restore inconsistency.

P1 requirements are required before GA for the affected Studio/Capability Pack/API/webhook capability. If a P1 feature is not implemented, that affected capability must remain explicitly non-GA/incomplete rather than being waived by documentation.

---

## 13. Operations, HA, DR and runbooks

Required runbooks:

- Capability Pack publication failure;
- Pack desired/applied convergence failure;
- Pack activation failure and LKG recovery;
- canary halt / bad Pack rollback;
- Pack revocation/quarantine and remediation;
- MIB missing/conflicting dependency;
- MIB revision breaking-change investigation;
- API authentication/OAuth refresh failure;
- API rate-limit exhaustion;
- API schema drift;
- API checkpoint/replay investigation;
- connector egress-policy block;
- webhook queue backlog/oldest-age breach;
- webhook dead-letter accumulation/replay;
- webhook signature/replay-validation failure;
- artifact/signature/integrity validation failure;
- certification failure;
- extensibility backup/restore integrity failure.

Operational evidence uses object/version/Sensor/entity/delivery/correlation IDs and sanitized errors. Secret values, private keys and unrestricted payload bodies are excluded from ordinary logs/support bundles.

HA behavior follows Platform Active/Standby Core authority. Sensor keeps valid runtime state through Core loss. After failover, desired/observed convergence resumes idempotently. DR restore does not activate artifacts until metadata/body hashes and required signatures are verified.

---

## 14. Formal requirements traceability supplement

The legacy base RTM requirements `REQ-EXT-001` through `REQ-EXT-007` remain valid umbrella requirements. This section provides the granular v3.2.3 engineering truth required to prove them complete.

| REQ ID | Priority | Governing PRD | Primary governing sections | Required evidence class |
|---|---|---|---|---|
| `REQ-EXTL-P0-001` | P0 | `PRD-EXTL-001` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-002` | P0 | `PRD-EXTL-009` | §6, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-003` | P0 | `PRD-EXTL-004` | §6, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-004` | P0 | `PRD-EXTL-009` | §6, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-005` | P0 | `PRD-EXTL-009` | §6, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-006` | P0 | `PRD-EXTL-006` | §6, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-007` | P0 | `PRD-EXTL-002` | §8, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-008` | P0 | `PRD-EXTL-002` | §8, §9, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-009` | P0 | `PRD-EXTL-003` | §7, §8, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-010` | P0 | `PRD-EXTL-003` | §13, §7, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-011` | P0 | `PRD-EXTL-003` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-012` | P0 | `PRD-EXTL-004` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-013` | P0 | `PRD-EXTL-004` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-014` | P0 | `PRD-EXTL-004` | §6, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-015` | P0 | `PRD-EXTL-005` | §8, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-016` | P0 | `PRD-EXTL-005` | §8, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-017` | P0 | `PRD-EXTL-005` | §8, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-018` | P0 | `PRD-EXTL-014` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-019` | P0 | `PRD-EXTL-007` | §7, §8, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-020` | P0 | `PRD-EXTL-007` | §7, §8, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-021` | P0 | `PRD-EXTL-007` | §10, §7, §8, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-022` | P0 | `PRD-EXTL-007` | §7, §8, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-023` | P0 | `PRD-EXTL-008` | §10, §7, §8, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-024` | P0 | `PRD-EXTL-008` | §10, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-025` | P0 | `PRD-EXTL-008` | §10, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-026` | P0 | `PRD-EXTL-008` | §10, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-027` | P0 | `PRD-EXTL-008` | §10, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-028` | P0 | `PRD-EXTL-013` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-029` | P0 | `PRD-EXTL-006` | §13, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-030` | P0 | `PRD-EXTL-006` | §11, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-031` | P0 | `PRD-EXTL-014` | §13, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-032` | P0 | `PRD-EXTL-014` | §13, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-033` | P0 | `PRD-EXTL-009` | §7, §9, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-034` | P0 | `PRD-EXTL-009` | §7, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-035` | P0 | `PRD-EXTL-011` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P0-036` | P0 | `PRD-EXTL-011` | §9, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-001` | P1 | `PRD-EXTL-004` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-002` | P1 | `PRD-EXTL-004` | §6, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-003` | P1 | `PRD-EXTL-013` | §13, §9, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-004` | P1 | `PRD-EXTL-002` | §8, §9, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-005` | P1 | `PRD-EXTL-003` | §7, §9, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-006` | P1 | `PRD-EXTL-010` | §9, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-007` | P1 | `PRD-EXTL-011` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-008` | P1 | `PRD-EXTL-011` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-009` | P1 | `PRD-EXTL-004` | §8, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-010` | P1 | `PRD-EXTL-004` | §9, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-011` | P1 | `PRD-EXTL-004` | §6, §9, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-012` | P1 | `PRD-EXTL-003` | §6, §7, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-013` | P1 | `PRD-EXTL-003` | §13, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-014` | P1 | `PRD-EXTL-007` | §7, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-015` | P1 | `PRD-EXTL-007` | §7, §9, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-016` | P1 | `PRD-EXTL-013` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-017` | P1 | `PRD-EXTL-008` | §10, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-018` | P1 | `PRD-EXTL-012` | §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-019` | P1 | `PRD-EXTL-004` | §6, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-020` | P1 | `PRD-EXTL-013` | §13, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-021` | P1 | `PRD-EXTL-013` | §13, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P1-022` | P1 | `PRD-EXTL-011` | §11, §§4,12,14 | contract + automated tests + operational evidence |
| `REQ-EXTL-P2-001` | P2 | `PRD-EXTL-004` | §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-002` | P2 | `PRD-EXTL-004` | §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-003` | P2 | `PRD-EXTL-004` | §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-004` | P2 | `PRD-EXTL-004` | §13, §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-005` | P2 | `PRD-EXTL-004` | §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-006` | P2 | `PRD-EXTL-015` | §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-007` | P2 | `PRD-EXTL-015` | §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-008` | P2 | `PRD-EXTL-015` | §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-009` | P2 | `PRD-EXTL-012` | §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-010` | P2 | `PRD-EXTL-008` | §10, §13, §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-011` | P2 | `PRD-EXTL-003` | §13, §7, §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-012` | P2 | `PRD-EXTL-007` | §10, §7, §8, §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-013` | P2 | `PRD-EXTL-003` | §11, §8, §§4,12,14 | design/implementation evidence when capability is enabled |
| `REQ-EXTL-P2-014` | P2 | `PRD-EXTL-004` | §§4,12,14 | design/implementation evidence when capability is enabled |

### 14.1 Legacy umbrella mapping

| Legacy ID | v3.2.3 granular coverage |
|---|---|
| `REQ-EXT-001` MIB Studio | `REQ-EXTL-P0-006/007/008/024/029/030/035`, `REQ-EXTL-P1-004/007/008/010/011/017/018/022` plus applicable security/operations requirements |
| `REQ-EXT-002` API Studio | `REQ-EXTL-P0-009/010/011/023/024/026/028/030/033/034/035`, `REQ-EXTL-P1-005/012/013/014/016/022` |
| `REQ-EXT-003` Capability Pack canonical package | `REQ-EXTL-P0-001/002/003/006/012/013/014/015/016/018/029`, `REQ-EXTL-P1-001/002/006/010/011` |
| `REQ-EXT-004` signed/versioned/import/export/rollback | `REQ-EXTL-P0-003/005/015/017/018/025/031/032/034`, `REQ-EXTL-P1-018/019` |
| `REQ-EXT-005` certification levels | `REQ-EXTL-P0-025/035`, `REQ-EXTL-P1-007/008/022` |
| `REQ-EXT-006` Test Lab/simulator | `REQ-EXTL-P0-035`, `REQ-EXTL-P1-007`, applicable protocol/runtime negative tests |
| `REQ-EXT-007` provenance/rights | `REQ-EXTL-P0-005/025/027`, `REQ-EXTL-P1-017/018` |

A legacy umbrella row is not considered fully implemented merely because one related screen or parser exists. All applicable granular requirements must have evidence.

---

## 15. Definition of Done and change-control rules

An extensibility feature is not complete when it only parses a MIB, successfully calls an API, or stores a Pack JSON document. Applicable work is complete only when:

1. the formal requirement IDs are mapped to implementation/evidence;
2. PostgreSQL/data/event/API contracts are migrated and backward-compatible where required;
3. tenant/RBAC/secret/security controls pass negative tests;
4. Sensor/Core failure behavior, idempotency and last-known-good behavior are proven;
5. UI provides the supported lifecycle without DB/YAML workarounds;
6. observability and support evidence expose degraded/failure states;
7. HA/backup/restore/upgrade/rollback implications are addressed;
8. performance/cardinality/resource bounds are proven for the supported workload;
9. certification/provenance/compatibility claims match actual evidence;
10. documentation and runbooks are updated and the RTM no longer overstates completion.

### 15.1 Implementation prohibition

Coding agents must not invent alternate Pack lifecycles, duplicate registries, vendor-specific telemetry databases, unbounded transformation scripting, direct secret storage, public-web-only connector assumptions or exactly-once webhook guarantees. If an implementation cannot satisfy this master without a material architecture change, the conflict returns to Mother Reference change control before code diverges.

### 15.2 Advanced P2 sequencing

P2 features are optional until explicitly scheduled. Their absence does not block the safe P0 foundation; their implementation must reuse and preserve P0/P1 lifecycle, validation, security, audit, offline and compatibility contracts rather than bypassing them.
### 15.3 Explicit P0 engineering spikes retained by this master

The requirements above are complete about **what** Platform must guarantee. Two physical implementation choices remain intentionally unresolved rather than guessed:

- `EXTL-SPIKE-001` — **Extensibility artifact-body storage backend and two-Core HA mechanics.** Freeze how immutable MIB/OpenAPI/YANG/Pack/fixture bodies are stored/replicated on Standalone and exactly-two-Core HA deployments, including atomic write, failover availability, backup/restore and corruption recovery, while preserving the architecture rule that no new mandatory storage service is introduced without evidence/change control.
- `EXTL-SPIKE-002` — **Capability Pack/artifact signing canonicalization and key algorithm.** Freeze the canonical serialized representation used for content digests/signatures, the signing algorithm/key format, trust-root storage and key-rotation/revocation mechanics. This must integrate with Platform release/PKI controls and air-gapped operation; implementation must not invent an ad-hoc signature format.

These spikes block production implementation of the affected storage/signing path, not acceptance of the surrounding lifecycle/security requirements. Their resolution must update `Platform_12_Decision_Source_Register.md`, this master and affected API/schema/test contracts before code relies on the result.

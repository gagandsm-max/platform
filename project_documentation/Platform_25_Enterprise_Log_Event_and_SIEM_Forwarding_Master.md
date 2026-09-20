# Platform — Enterprise Log, Event Collection & SIEM Forwarding Master

**Document ID:** PLATFORM-LOG-001  
**Version:** 3.2.2  
**Status:** **Authoritative Mother Reference — Specialized Master**  
**Product:** Platform  
**Change class:** Additive v3.2.2 product/engineering requirements. Specification does not imply implementation, certification, customer deployment, SIEM entitlement or contractual delivery guarantee.

---

## 1. Authority, purpose and scope

This document is the specialized Mother Reference for **enterprise operational log collection, log/event normalization, log search and analytics, log evidence, log routing and SIEM forwarding**.

It defines one coherent product capability spanning Syslog; Linux/application files; journald; Windows Event Log; Windows application files; Kubernetes/container logs; middleware/database logs through Capability Packs; cloud logs through Cloud Capability Packs; OpenTelemetry/OTLP logs; API/webhook/customer custom logs; parsing/multiline; canonical Log Event identity; sensitive-data handling; fingerprinting/pattern analytics; Log Data Quality; retention/archive; Log Explorer; Alert Evidence/RCA/Change Intelligence integration; and external SIEM/log forwarding.

### 1.1 Authority boundaries

| Owner | Retains authority for |
|---|---|
| Platform_10 | protocol mechanics, TLS/mTLS, credentials, connector security, endpoint authentication and firewall/connectivity contracts |
| Platform_11 | Core/Sensor/storage sizing, Streaming/Event Sensor capacity, retention/storage protection and workload certification inputs |
| Platform_14 | generic test/performance/release gates |
| Platform_17 | versioned API/event/data contracts, datastore ownership and schema compatibility |
| Platform_18 | Platform-wide threat model |
| Platform_19 | Incident Flight Recorder, Similar Incident Memory, Change Intelligence, value/MTTR |
| Platform_20 | governed onboarding identity/approval/notifications where reused |
| Platform_22 | telemetry trust, Alert Evidence Contracts, evidence-qualified alerts, RCA/AI assurance |
| Platform_23 | Capability Pack classification/certification; vendor-specific log mappings belong in packs |
| Platform_25 (this master) | unified log source/product model, collection semantics, parser/fingerprint/data-quality contract, Log Explorer, routing/forwarding behavior, SIEM destination lifecycle and class-specific acceptance |

This master may add behavior where older documents are silent. It must not silently override explicit higher-precedence requirements.

---

## 2. Product principles

1. **Collect once, use many times.** A log collected by Platform may feed Platform operations and, subject to policy, one or more external SIEM/log destinations.
2. **One canonical Log Event model.** Syslog, file logs, OTLP logs, Windows events, container logs and API logs do not create independent event models.
3. **Log Event != Alert != Incident.** A log is evidence until qualification policy makes it an alert/incident input.
4. **No-data != target-down.** Collection failure, pipeline failure and source silence are explicit telemetry-quality states.
5. **Secure transport by default.** Syslog/TLS and authenticated API/OTLP transports are preferred; UDP Syslog is best-effort/legacy.
6. **No silent loss.** Forced drop, parser rejection, rate limit, queue overflow, storage shedding and destination rejection are measured and exposed.
7. **No hidden tenant authority.** Payload fields never override server-side tenant/source authorization.
8. **Logs are untrusted content.** Log text cannot grant AI/tool/command authority.
9. **No mandatory new datastore.** PostgreSQL + ClickHouse + Kafka + Sensor spool remain the default architecture unless benchmarks prove a gap.
10. **No SIEM replacement claim.** Platform performs operational log analytics and forwarding; dedicated SIEM/SOC workflows remain integration targets.
11. **High-rate log workloads must not starve monitoring.** Streaming/Event Sensor resource isolation is mandatory when needed.
12. **Air-gap first.** Mandatory collection, parsing, search and forwarding must not require Internet access.
13. **Capability Pack, not Core forks.** Vendor/product parser/mapping behavior belongs in the existing Capability Pack architecture.
14. **Delivery semantics are explicit.** Transport acceptance is not falsely represented as destination indexing/processing acknowledgement.

---

## 3. Formal product requirements

| PRD ID | Requirement |
|---|---|
| PRD-LOG-001 | Platform shall provide one unified log/event collection architecture for Syslog, host/application, Windows, container, OTLP, cloud/API and custom operational logs. |
| PRD-LOG-002 | All supported sources shall normalize into a canonical tenant/entity/service-aware Log Event contract with source provenance and timestamp quality. |
| PRD-LOG-003 | Platform shall support RFC3164 and RFC5424 Syslog over UDP/TCP and secure Syslog/TLS, with correct stream framing for TCP/TLS. |
| PRD-LOG-004 | Applicable edge collection shall use bounded durable buffering/retry/replay and expose certified buffer horizon/loss risk. |
| PRD-LOG-005 | Platform shall provide versioned Log Source Profiles and parser profiles including bounded multiline handling and parser preview. |
| PRD-LOG-006 | Platform shall preserve event time, observed/receive time and ingest time with explicit timestamp-quality semantics. |
| PRD-LOG-007 | Platform shall support source/storage/egress-sensitive data policies including masking, hashing and field/event suppression. |
| PRD-LOG-008 | Platform shall provide deterministic fingerprinting/grouping/storm analytics so repeated logs do not create equivalent alert storms. |
| PRD-LOG-009 | Log events shall feed Telemetry Quality and Alert Evidence Contracts; a log alone shall not automatically assert authoritative target failure. |
| PRD-LOG-010 | Logs shall correlate with metrics, traces, entities, services, incidents and changes where evidence permits. |
| PRD-LOG-011 | Platform shall provide tenant-scoped Log Explorer, context search, pattern views and bounded live-tail capability. |
| PRD-LOG-012 | Platform shall expose per-source Log Data Quality, pipeline health and actionable collection/parser/cardinality findings. |
| PRD-LOG-013 | Platform shall support STORE_ONLY, STORE_AND_FORWARD and governed FORWARD_ONLY source/route modes. |
| PRD-LOG-014 | Platform shall route approved telemetry and Platform intelligence to one or more external SIEM/log destinations through Integration Packs. |
| PRD-LOG-015 | Forwarding shall support raw, normalized and enriched representations while preserving authoritative raw evidence/reference where policy permits. |
| PRD-LOG-016 | Outbound formats shall include RFC5424 Syslog, JSON and, when certified, CEF, LEEF, OTLP Logs and target-native APIs through Integration Packs. |
| PRD-LOG-017 | Each external destination shall have independent bounded queue/retry/backpressure/health state so one failed target cannot block another. |
| PRD-LOG-018 | Platform shall provide destination rate limits, delivery-state visibility, dead-letter handling and authorized bounded replay. |
| PRD-LOG-019 | Routing shall enforce tenant/resource scope, data-residency policy and forwarding-loop prevention. |
| PRD-LOG-020 | Platform shall support high-rate edge forwarding from Streaming/Event Sensors where benchmarked, while Core retains policy/authorization/audit authority. |
| PRD-LOG-021 | Platform shall expose sizing inputs for ingress EPS/bytes, parser cost, retention, search concurrency, route fan-out, queue durability and egress bandwidth. |
| PRD-LOG-022 | Platform shall monitor the full log pipeline itself: receiver, parser, spool, upload, Kafka, ClickHouse, search and SIEM delivery. |
| PRD-LOG-023 | Mandatory log capabilities shall support offline/air-gapped install, operation, upgrade and rollback. |
| PRD-LOG-024 | Technology-specific parsers/mappings shall use Capability Packs and may not bypass canonical identity, RBAC, retention, Alert Evidence or audit. |
| PRD-LOG-025 | Logs supplied to AI/LLM shall be bounded evidence selected by Platform analytics; raw customer log text remains untrusted prompt content. |
| PRD-LOG-026 | Platform shall support customer-visible route transformation/volume preview before activation where historical/current telemetry is available. |
| PRD-LOG-027 | Platform shall provide source/destination lifecycle, credential/certificate health and configuration versioning. |
| PRD-LOG-028 | Platform shall distinguish collection/pipeline/delivery faults from monitored-device/application faults in UI, events, alerts and RCA. |

---

## 4. Unified architecture

~~~text
                        LOG SOURCES
                             |
      +----------------------+----------------------+
      |                      |                      |
   Syslog                 Host/App                OTLP/API
 UDP/TCP/TLS      File/Journald/EventLog       Cloud/Custom
      |                      |                      |
      +----------------------+----------------------+
                             |
                    Streaming/Event Sensor
                             |
                   Validate / Authenticate
                             |
                    Durable Bounded Spool
                             |
                 Parse / Normalize / Redact
                             |
                 Fingerprint / Data Quality
                             |
                    Entity/Service Resolve
                             |
                    Canonical Log Event
                             |
             +---------------+----------------+
             |                                |
             v                                v
       PLATFORM PATH                    ROUTING PATH
       Kafka -> ClickHouse               Policy Router
             |                                |
 Search / Pattern / RCA              Per-destination queues
 Alert Evidence / AI                     |
                                   +------+------+
                                   |      |      |
                                 SIEM   Log DB  OTLP/API
~~~

The logical stages do not require a separate microservice per stage. Existing Sensor/Core/connector deployment units shall be reused unless benchmarked evidence justifies separation.

---

## 5. Log source classes and collection

### 5.1 Syslog

Required profiles: RFC3164, RFC5424 including structured data, UDP, TCP, Syslog over TLS, correct TCP/TLS stream framing including RFC6587-compatible octet counting/non-transparent framing where required by interoperability certification, configurable listeners, source rules, message/field bounds and explicit error/drop counters.

Security posture: TLS 1.2 minimum; TLS 1.3 preferred where compatible; customer CA/certificate lifecycle; optional mTLS; UDP labelled best-effort/legacy. Platform shall not claim acknowledgement semantics UDP cannot provide.

### 5.2 Linux/application files

Configured sources only. Support explicit paths/safely bounded patterns, file identity/inode/generation, durable checkpoint, rotation/rename/truncate/replacement detection, encoding, multiline assembly, rate limits and local buffering. Uncontrolled recursive filesystem harvesting is prohibited.

### 5.3 journald

Preserve useful journal metadata including unit, priority, identifier, PID, boot ID, cursor, timestamp and structured fields. Durable cursor/checkpoint behavior is required.

### 5.4 Windows Event Log and application files

Support authorized System, Application and customer-defined channels. Security channel collection is explicit/least-privilege, not automatic. Preserve provider, event ID, level, channel, computer, record ID, time and structured event data.

### 5.5 Kubernetes/container logs

Preserve cluster, namespace, workload, pod, container, node, restart/generation and service context. Restart/churn must not silently create unrelated duplicate identities.

### 5.6 OpenTelemetry logs

OTLP Logs normalize into the same canonical Log Event model. OTel resource/service identity, trace_id and span_id enrich entity/service/cross-signal correlation. OTLP log storage is not a separate log product.

### 5.7 Cloud/API/custom logs

AWS/Azure/GCP/OCI, SaaS/API/webhook and proprietary customer sources enter through approved Cloud/Capability/Integration Pack interfaces. Customer-specific support shall not require Core forks.

---

## 6. Log Source Profile and lifecycle

A versioned Log Source Profile includes source identity, tenant/site/environment/owner, technology/application, collection method, path/channel/endpoint, transport/security profile, encoding, parser/version, timestamp/timezone rule, severity mapping, multiline policy, entity/service mapping, sensitive-data policy, retention profile, priority, rate/cardinality policy, routing policy and configuration version.

Lifecycle shall align with Platform workflow conventions and include the semantics of DRAFT -> VALIDATING -> READY -> PENDING_APPROVAL where required -> ACTIVE, with explicit DEGRADED, SUSPENDED and DISABLED equivalents. Activation is not silent.

---

## 7. Parser and multiline framework

Versioned Parser Profiles support Syslog, JSON, key/value, justified delimited formats, bounded regular-expression extraction and Grok-like templates only if governed/admitted.

Parser execution enforces input size, field count/size, execution-cost/time and backtracking limits. Multiline policies support Java, .NET, Python, Oracle/database and middleware stack traces without unbounded buffering.

Before activation, UI/API provides sample raw -> parsed -> normalized -> redacted -> entity mapping preview.

---

## 8. Canonical Log Event contract

The versioned canonical contract supports at minimum:

- log_event_id and schema version;
- tenant/site/source identity, source type and transport;
- canonical entity_id and optional service_id;
- event time, observed/receive time and ingest time;
- timestamp quality/timezone evidence;
- original and normalized severity;
- facility where applicable;
- application/component/environment;
- host/cloud/Kubernetes/container identity as controlled attributes;
- event class/code;
- message/body;
- fingerprint/pattern ID;
- trace ID/span ID/correlation ID/request ID where available;
- bounded typed common attributes plus controlled extensible attributes;
- parser ID/version;
- quality/provenance;
- raw evidence/reference and truncation state;
- ingestion/routing correlation ID where required.

Arbitrary parsed customer fields shall not automatically become dedicated database columns/indexes.

---

## 9. Timestamp, severity and Log Data Quality

Preserve event_time, observed_time and ingest_time distinctly. Timestamp quality communicates authoritative, inferred, unavailable, invalid or clock-skewed semantics. Missing event time is never silently represented as authoritative ingest time.

Original source severity is retained alongside normalized Platform severity.

Log Data Quality evaluates parser success/failure, timestamp/clock skew, severity mapping, entity/service mapping, late/out-of-order data, duplicate/replay indicators, cardinality intervention, dropped/rejected/rate-limited events and source/collector health.

---

## 10. Sensitive data, privacy and egress policy

Policies may apply ALLOW, MASK, HASH, DROP_FIELD, DROP_EVENT or equivalent actions by tenant/source/parser/field/classification/pattern.

At minimum protect against accidental exposure of passwords, authorization headers, tokens, cookies, API keys, private keys and configured sensitive identifiers.

Ingress/storage policy and external-egress policy may differ. Platform must not claim perfect automated secret/PII detection. Raw-log access, exports, live tail, support bundles and forwarding are RBAC/resource scoped.

---

## 11. Cardinality, fingerprinting and storm control

Detect unbounded dimensions such as request/session/user IDs, UUIDs, dynamic URLs and uncontrolled parser fields. Policy may warn, normalize, hash, drop an attribute, rate-limit, quarantine or reject according to governed configuration; interventions are visible and counted.

Deterministic fingerprinting normalizes variable tokens while retaining representative evidence. Fingerprints support repeated-event grouping, first/last seen, occurrence rate/count, affected entities/services, new/rare/growing pattern analytics, Similar Incident Memory and alert-noise reduction.

Repeated logs shall not directly create repeated alerts. Platform_22 evidence/qualification contracts remain authoritative.

---

## 12. Data transport, buffering and backpressure

Applicable ingestion uses receive -> validate -> durable bounded edge spool -> upload -> Core acceptance -> Kafka -> downstream consumers/ClickHouse.

Delivery is replay-safe/at-least-once unless a narrower path is explicitly certified otherwise. Exactly-once shall not be claimed without proof.

Sensor/Core outage, WAN loss, Kafka lag and downstream slowdown use bounded queue/backpressure rather than unbounded RAM growth. Fixed 80% Sensor storage protection remains authoritative; unavoidable loss is counted and surfaced with buffer horizon/risk.

---

## 13. Datastore ownership

No new datastore is selected.

**PostgreSQL** owns source/parser profiles, lifecycle, retention/routing policy metadata, destination definitions, credential references, configuration versions, saved-search/dashboard metadata where applicable, dead-letter/replay job metadata and audit references.

**ClickHouse** owns high-volume normalized logs, searchable common fields/controlled attributes, fingerprint/pattern analytics, selected pipeline/delivery analytics and incident-linked log evidence analytics.

**Kafka** carries versioned internal log/event streams and routing triggers and is never exposed directly to browsers.

**Sensor local storage** provides bounded durable spool for applicable edge collection/forwarding.

OpenSearch/Elasticsearch/Loki or another log database is not mandatory. A new datastore requires measured inability of the selected stack to meet certified requirements.

---

## 14. Retention, archive and protected evidence

Retention is class-specific: raw log, normalized log, pattern aggregate, incident-protected evidence and archive. Enterprise 180/365-day profiles may be supported but exact values are not universal contractual guarantees.

Incident/legal-hold rules protect selected evidence. Archive must expose integrity/provenance, authorized retrieval and audit where implemented.

---

## 15. Log Explorer and UI

Provide an Observe -> Logs or equivalent tenant-scoped Log Explorer.

Search/filter dimensions include time, site/environment, entity/service/application, host/device, Kubernetes identities, technology/source/type, severity, parser, event class, fingerprint, trace/span/correlation ID, incident and change.

Support bounded full text, field filters, boolean logic, exists/not-exists, numeric/range and time filters without initially cloning SPL/KQL.

Rows show time, severity, source, entity/service, message and fingerprint/trace/incident indicators. Expanded view may show parsed fields, authorized raw evidence, parser/version, quality/provenance and related trace/incident/change/routing state.

Provide bounded surrounding-context queries, pattern views (Top Errors, New Patterns, Fastest Growing, Rare, Resolved/declining) and permission/rate/time-bounded live tail.

---

## 16. Log onboarding/provisioning UX

Provide Settings -> Data Sources -> Logs -> Add Source or equivalent.

Source types: Syslog, File, journald, Windows Event Log, OpenTelemetry, Kubernetes/Container, Cloud, API/Webhook, Custom.

Wizard covers:

1. identity/scope;
2. collection method;
3. path/channel/endpoint/transport;
4. credential/TLS;
5. parser;
6. timestamp/severity;
7. multiline;
8. entity/service mapping;
9. sensitive-data policy;
10. retention;
11. rate/cardinality;
12. routing;
13. test/validation;
14. parse/transformation preview;
15. capacity/volume estimate;
16. approval/activation.

Preflight distinguishes connectivity, TLS, authentication, authorization, parser and capacity failures.

Source inventory exposes source/type/site/technology, lifecycle state, EPS/bytes, last event, parse quality, mapping/timestamp quality, queue/backlog, drop/reject/rate-limit counts, cardinality state and configuration version.

---

## 17. SIEM/log forwarding

### 17.1 Modes

Support STORE_ONLY, STORE_AND_FORWARD and governed FORWARD_ONLY. FORWARD_ONLY clearly discloses reduced Platform historical search/RCA capability unless separate evidence is retained.

### 17.2 Forwardable classes

Collected telemetry: Syslog, application/OS/container/OTLP/cloud logs and selected events.

Platform intelligence: qualified alerts, incidents, anomalies, bounded RCA summaries/evidence references, topology/change/NCM events and selected Platform/Sensor health events.

### 17.3 Integration Packs and formats

SIEM destinations are Integration Pack classifications under Platform_23. Certification targets may include IBM QRadar, Splunk, Microsoft Sentinel, ArcSight, Elastic, Generic Syslog, Generic HTTPS and Generic OTLP.

Where certified, formats include RFC5424, legacy RFC3164, JSON, CEF, LEEF, OTLP Logs and target-native API payloads.

Routes may emit RAW, NORMALIZED or ENRICHED representations. Enrichment may add approved entity/site/service/criticality/incident/confidence/topology context without destroying authoritative source evidence.

---

## 18. Routing policy, residency and fan-out

Routes may filter by tenant, region/site, source/type, technology, entity/group/service, severity/facility, event class/code, fingerprint, tags/classification and qualified-alert/incident state.

A source/event may fan out to multiple destinations. Each destination has independent queue/health/error state.

Routing supports data-residency constraints such as region/site-specific destinations without product forks.

---

## 19. Forwarding durability and delivery semantics

A failed destination must not block healthy destinations. Logical per-destination queue isolation is mandatory, though physical storage may be shared safely.

Each source Log Event routed to a destination receives a stable tenant-scoped **per-destination delivery identity** derived from or durably linked to the canonical source event identity, route/version and destination. Fan-out to N destinations creates N independently recoverable delivery lifecycles; retry/replay of one destination must not create a new logical source event or silently change another destination's state.

The durable forwarding queue is restart/failover safe. PostgreSQL is authoritative for control-plane delivery lifecycle/attempt/dead-letter/replay state. High-rate payload bodies MAY remain in the canonical ClickHouse/log-evidence store or an approved durable artifact path and be referenced by immutable event/evidence identity rather than duplicated into PostgreSQL, but the referenced source event MUST remain retained/pinned for at least the unresolved delivery horizon. A queued delivery may not outlive the only retrievable copy of the payload it is required to send.

For a Kafka-derived canonical Log Event, the routing consumer MUST idempotently materialize every required per-destination delivery intent, or a durable explicit policy-drop disposition, **before committing the consumed Kafka offset**. A Core crash after materialization but before offset commit may repeat routing work, but stable uniqueness/idempotency must converge on the same delivery identities. A crash before materialization commits must leave the Kafka event replayable. Kafka wakeups are not the sole record of pending external delivery.

Where routing is triggered by a PostgreSQL-authoritative control/business transition, creation of any required outbound delivery intent uses a transactional outbox or equivalent single-PostgreSQL atomic boundary with that transition. A committed transition must not exist without its required durable delivery intent, and an uncommitted transition must not expose a phantom queued delivery.

Delivery states distinguish queued, sending, transport accepted, destination acknowledged where provable, retrying, rate limited, failed, dead letter and explicitly policy-dropped. State/attempt updates are monotonic/versioned or concurrency-guarded so failover/retry workers cannot silently regress a terminal state or double-own the same lease.

A worker marks delivery complete only at the configured destination acceptance boundary. UDP Syslog is best-effort. Successful socket/TLS/HTTP write alone is never represented as "indexed in SIEM" or downstream processing completion unless the destination contract proves that stronger acknowledgement. Ambiguous outcomes enter bounded retry/reconciliation according to destination semantics rather than blind duplicate creation where a target offers an idempotency/correlation mechanism.

Retry/backoff is bounded and destination-specific. Health exposes queue depth/bytes, oldest pending, retry rate, last success and estimated durability horizon.

Repeated transformation/schema/target rejection enters dead-letter state. Authorized operators may inspect, correct mapping, bounded-retry or discard with reason/audit. Dead-letter/discard is a durable terminal disposition; queue/source retention must not delete the only evidence required for audit or an authorized replay before its retention policy permits.

Replay requires destination/time/scope, volume estimate, rate limit, confirmation and audit. Replay preserves the original source event and delivery lineage and creates a bounded new attempt/replay execution rather than an unrelated business event. Unbounded historical replay is prohibited.

Per-destination controls may include EPS, bytes/sec, batch size, concurrency and queue limits. Priority/shedding, if needed, is deterministic, policy-driven, counted and visible. A required delivery may be shed only through an explicit policy disposition that records destination, reason, count/bytes and audit/health evidence; silent queue eviction is prohibited.

---

## 20. Loop and duplicate protection

Prevent Platform -> SIEM -> Platform recursion using trusted internal origin/route/hop metadata or equivalent server-controlled mechanism; customer-editable payload fields alone are insufficient.

Retries/replay preserve stable event/source/route identity where possible. Use destination idempotency mechanisms when supported. Global exactly-once is not claimed.

---

## 21. Edge forwarding

At high volume, forwarding may execute on an eligible Streaming/Event Sensor when benchmarked.

~~~text
                           Core control plane
                         policy/auth/audit/config
                                  |
Log source -> Streaming Sensor ---+----> Platform ingest
                     |
                     +-----------------> SIEM destination
~~~

Core retains route definition, tenant authorization, credentials references, configuration version, audit and lifecycle authority. Sensor may execute high-rate forwarding but cannot invent/alter policy.

---

## 22. Destination/routing UI

Provide Settings -> Data Routing -> Log / SIEM Forwarding or equivalent.

Destination inventory shows destination/type/tenant, protocol/format, health, EPS/bytes, queue depth/bytes, oldest pending, last success, retry/reject/dead-letter rate and credential/certificate expiry where applicable.

Destination wizard covers endpoint/transport, auth/TLS, output format, source scope, filters, raw/normalized/enriched mapping, sensitive-data egress policy, rate/buffer policy, test connection, transformation preview, route-volume estimate and approval/activation.

Route preview estimates matching events/bytes from available telemetry and labels estimates as non-guaranteed.

---

## 23. API, RBAC and audit

Versioned APIs expose Log Sources, Parser Profiles, validation/preview, Log Data Quality, Log Search, Saved Searches where supported, Log Destinations, Routing Policies, destination health/test, Dead-Letter summary/records, Replay Jobs and transformation/volume preview. Exact route strings are frozen in generated OpenAPI before GA.

Permissions distinguish view logs/raw evidence, create/edit source/parser, manage sensitive-data policy, create/edit destination, manage credentials by reference, create/edit/approve route, replay, dead-letter actions, live tail and export.

Audit records source/parser/destination/route changes, credential/certificate rotation references, replay, dead-letter retry/discard and policy changes without writing secrets or unnecessary raw data.

---

## 24. Security threats

Explicit threats include spoofed source, forged tenant/source fields, cross-tenant query/forwarding, malicious parser/regex, oversized/malformed payload, log injection, secret/PII ingestion, high-cardinality/log-flood DoS, queue/disk exhaustion, TLS downgrade/certificate failure, destination credential theft, outbound exfiltration, routing loop, unauthorized replay, dead-letter abuse, prompt injection through log text, compromised collector and collector supply-chain compromise.

Mitigations include server-side tenant authority, TLS/mTLS where applicable, least privilege, bounded parsers, quotas/rate limits, redaction, spool/queue limits, 80% protection, RBAC/audit, loop protection, destination isolation and AI untrusted-content handling.

---

## 25. Alert Evidence, RCA and AI

Logs contribute evidence to Platform_22 and do not create a parallel alert engine.

Correlate log -> metric, log -> trace, log -> change/deployment, log -> topology/entity/service and log -> incident where evidence permits.

Incident Flight Recorder preserves bounded fingerprints/counts/representative records/references for before/during/after windows. Similar Incident Memory may include fingerprints and service/change context.

Never stream unrestricted raw logs wholesale to an LLM. Required path is raw/high-volume logs -> query/fingerprint/statistical analysis -> bounded relevant evidence -> Context Builder -> Incident Copilot.

All log bodies/attributes are untrusted prompt content.

---

## 26. Sizing and self-monitoring

Sizing/certification measures source count, average/peak EPS, bytes/event, transport mix, parser/multiline/redaction/fingerprint cost, cardinality, retention/archive, search/dashboard concurrency, live-tail sessions, destination fan-out, egress EPS/bytes, queue durability, WAN bandwidth and Sensor/Core/Kafka/ClickHouse resource curves.

Platform Health exposes receiver state, accepted/rejected/malformed/oversized events, UDP/socket/queue drops, TCP/TLS failures, parse/redaction/fingerprint errors, source EPS/bytes, spool depth/oldest age, upload retry, Kafka/ClickHouse lag, search latency/errors, routing matches, forwarded/failed/rate-limited events, per-destination queue/age, dead-letter, replay and storage pressure.

No production capacity claim is fixed without benchmark evidence.

---

## 27. HA, DR, installation and operations

Logging fits the fixed Standalone/two-Core Active-Standby architecture and does not add a third Core.

During transient Core/WAN outage, eligible Sensors continue bounded collection/forwarding according to certified spool/route policy.

DR protects source/parser/routing/destination definitions, configuration versions and credential metadata/references under existing backup/restore policy. Historical logs follow configured telemetry/archive policy.

Mandatory capability ships offline. If Fluent Bit or another collector becomes mandatory, it must first be admitted through Platform_07 with exact version, licence/transitive scan, SBOM, CVE/provenance review, offline artifact, hardening and upgrade/rollback plan.

Operational runbooks cover listener/collector failure, parser degradation, spool pressure, Kafka/ClickHouse backlog, cardinality/log storm, destination auth/TLS/outage/rate-limit, outbound queue pressure, dead-letter growth, replay and credential/certificate rotation.

---

## 28. Test and certification matrix

Required classes:

**Collection/protocol:** RFC3164/RFC5424, UDP overload/loss visibility, TCP reconnect/framing/RFC6587, TLS/mTLS/certificate rotation, file rotation/truncate/restart, journald cursor, Windows Event channels, container churn, OTLP logs, malformed/oversized inputs.

**Parser/data quality:** JSON/KV/multiline, bounded regex, parser version upgrade, bad timestamps/clock skew, severity/entity mapping, sensitive masking, cardinality explosion, fingerprint stability.

**Durability/failure:** Sensor restart, Core failover, WAN partition, Kafka backlog/outage, ClickHouse slow/down, disk pressure/80% protection, replay/duplicate delivery, no-silent-loss accounting.

**Search/UI:** tenant/RBAC, Log Explorer, context, patterns, bounded live tail, source onboarding/preview/error states.

**Forwarding:** multiple destinations, one target down while others continue, Syslog TLS, JSON/HTTPS, CEF/LEEF where certified, auth/TLS failure, rate limiting, rejection, dead-letter, replay, preview, fan-out, loop prevention, duplicate/idempotency and cross-tenant denial.

**Security:** forged tenant fields, cross-tenant query/forward, parser abuse, regex/resource abuse, log/prompt injection, sensitive-data leakage, credential exposure and unauthorized replay/route changes.

---

## 29. Formal acceptance catalogue

| REQ ID | Priority | Acceptance |
|---|---|---|
| REQ-LOG-001 | P0 | RFC3164/RFC5424 parser tests cover valid, malformed, partial and bounded-input cases with source/provenance. |
| REQ-LOG-002 | P0 | UDP/TCP/TLS paths are independently tested; TCP/TLS framing interoperates; UDP remains labelled best-effort. |
| REQ-LOG-003 | P0 | Production Sensor wiring accepts configured Syslog and durably spools accepted events where the certified profile requires it. |
| REQ-LOG-004 | P0 | File collection survives rotation/truncate/restart without uncontrolled loss or mass reread in certified cases. |
| REQ-LOG-005 | P0 | journald/Event Log collectors preserve durable cursor/record identity and restart safely where shipped. |
| REQ-LOG-006 | P0 | Parser/multiline limits resist malformed/pathological input and preview matches runtime behavior. |
| REQ-LOG-007 | P0 | Canonical Log Event schema compatibility tests preserve event/observed/ingest time distinction. |
| REQ-LOG-008 | P0 | Entity/service resolution does not create cross-tenant/protocol-only duplicates in representative cases. |
| REQ-LOG-009 | P0 | Sensitive-data policy is enforced before configured storage/egress boundaries; secrets stay out of ordinary logs/audit/support artifacts. |
| REQ-LOG-010 | P0 | Cardinality/log-storm tests remain bounded and every intervention/drop is visible. |
| REQ-LOG-011 | P0 | Log event alone does not assert target DOWN without evidence; collection failure is semantically separate. |
| REQ-LOG-012 | P0 | Sensor/Core/Kafka/ClickHouse interruption tests prove bounded spool/backpressure/replay and exact loss accounting. |
| REQ-LOG-013 | P1 | Log Explorer provides tenant-scoped bounded search, context and pattern navigation without unbounded scans. |
| REQ-LOG-014 | P1 | Live tail enforces tenant/RBAC/source/rate/time limits and exposes no direct Kafka endpoint. |
| REQ-LOG-015 | P0 | Routing policy cannot forward Tenant A data through Tenant B destination/credential. |
| REQ-LOG-016 | P0 | Destination queues are failure-isolated. |
| REQ-LOG-017 | P0 | Delivery UI/API distinguishes queued/sent/transport/destination acknowledgement and never overclaims UDP/indexing state. |
| REQ-LOG-018 | P0 | Rate limit/backoff/dead-letter/replay are bounded, audited and restart-safe in certified cases. |
| REQ-LOG-019 | P0 | Loop tests prevent Platform -> external -> Platform recursion using trusted metadata. |
| REQ-LOG-020 | P0 | Raw/normalized/enriched preview matches emitted payload for certified formats. |
| REQ-LOG-021 | P0 | Sensitive-data egress policy is enforced independently of storage policy where configured. |
| REQ-LOG-022 | P0 | Streaming/Event Sensor log load cannot starve certified polling/Agent/control workloads; safe separation thresholds are benchmarked. |
| REQ-LOG-023 | P0 | Sizing evidence records EPS, bytes, parser cost, retention/search, route fan-out, queue and egress resource curves. |
| REQ-LOG-024 | P0 | Air-gap install/upgrade requires no public runtime download and includes approved collector/parser artifacts. |
| REQ-LOG-025 | P0 | Prompt-injection tests prove log text cannot escalate LLM/tool authorization. |
| REQ-LOG-026 | P1 | Incident/RCA can retrieve bounded related fingerprints/log evidence with provenance. |
| REQ-LOG-027 | P0 | Pipeline self-monitoring exposes ingress/parser/spool/Kafka/ClickHouse/forwarding health and exact drop/reject/dead-letter counts. |
| REQ-LOG-028 | P0 | Upgrade/rollback preserves source/parser/route/destination configuration and supported schema compatibility. |

---

## 30. Implementation sequence

**LOG-0 — contracts/specification:** canonical schema, source/parser/route/destination lifecycle, APIs, sizing and test harness.

**LOG-1 — Syslog production path:** runtime wiring, UDP certification, TCP/RFC6587, TLS, spool -> Core -> Kafka -> ClickHouse and health.

**LOG-2 — canonical parser/log framework:** source profiles, parser/multiline, timestamps/severity, entity resolution, sensitive data, fingerprint/data quality.

**LOG-3 — host/application collection:** Linux file, journald, Windows Event Log/file, Kubernetes/container and checkpoint/rotation.

**LOG-4 — Log Explorer/analytics:** search/context/patterns/live tail/source-quality UI.

**LOG-5 — forwarding foundation:** destination/route model, independent queues, generic Syslog + JSON/HTTPS, retry/dead-letter/replay/loop protection.

**LOG-6 — SIEM Integration Packs:** QRadar/LEEF, Splunk, Sentinel, CEF and other targets according to certification priorities.

**LOG-7 — operational intelligence:** Alert Evidence, Change Intelligence, Flight Recorder, Similar Incident, RCA and bounded AI context.

---

## 31. Non-goals

This master does not make Platform a general SIEM/SOC replacement; require Elasticsearch/OpenSearch/Loki/Splunk as storage; require one microservice per collector/parser/route; allow uncontrolled recursive harvesting; guarantee zero loss beyond finite certified buffering; claim exactly-once delivery; claim destination indexing from transport success; trust payload tenant identity; allow log text to control AI/tools; reopen full APM/full DPI; or approve Fluent Bit/new OSS without Platform_07 admission.

---

## 32. Completion rule

Logging is not enterprise-ready merely because Platform can receive UDP Syslog.

Production readiness requires an evidence-backed end-to-end path:

**collect -> authenticate/identify -> buffer -> parse -> normalize -> redact -> resolve -> store/search -> qualify/correlate -> route -> retry/acknowledge -> monitor -> upgrade/recover**

with security, tenant isolation, sizing, air-gap, HA/failure behavior, UI and certification evidence.

# Platform — Flow Enrichment & Dedicated Sensor Role Reconciliation Handoff

**Status:** NON-AUTHORITATIVE REVIEW / SAFETY-NET ARTIFACT — DO NOT TREAT AS MOTHER REFERENCE  
**Created from canonical main:** `df0dd5f19ce502dbc141e8e4b13a84293dc2a7ca`  
**Purpose:** Preserve accepted Flow/NTA enrichment and production Sensor-role requirements until the active Flow documentation lanes are reconciled.  
**Merge rule:** This file MUST NOT survive as a parallel authority. Before this PR can merge, the accepted content must be reconciled into the existing Mother Reference set, this handoff file must be deleted, and exact-head validation/review must pass.

---

## 1. Why this handoff exists

Canonical main already defines:

- NetFlow/IPFIX/sFlow/J-Flow/vendor Flow as native NTA capability;
- canonical `network_flow` data;
- canonical entity identity independent of transient IP/hostname;
- IPAM/subnet/site/VRF/VLAN/ASN/BGP/cloud/application enrichment foundations;
- temporal identity/topology concepts;
- Flow high-rate safeguards;
- one common Sensor software architecture.

However, the stronger Flow enrichment and production resource-isolation requirements below are not yet fully canonical on `main`.

Active draft Flow PRs (#70 and #74) overlap this area. This handoff exists so those decisions are not lost while the control plane determines the surviving Flow reconciliation lane.

No new specialized Mother Reference is created by this handoff.

---

## 2. Flow identity and enrichment requirements to preserve

### 2.1 Canonical identity

An IP address is an observation, not endpoint identity.

Flow enrichment must resolve, where evidence exists:

```text
Flow IP + observation timestamp
  -> temporal address ownership
  -> canonical tenant-scoped entity
  -> hostname/system name/FQDN and other current/historical metadata
```

Historical Flow must use time-correct identity. Reuse of an IP by another endpoint must not rewrite historical ownership.

Expected evidence may include:

- canonical entity identifiers;
- Agent identity;
- IPAM;
- DHCP lease/history;
- DNS;
- network attachment history;
- VPN/WLAN/NAC evidence;
- other governed identity sources.

Every enrichment must preserve source/provenance and observation/effective time.

### 2.2 Hostname and FQDN

Where evidence exists, Flow may expose:

- system/host name;
- FQDN;
- canonical entity;
- entity type;
- site/subnet/VRF/VLAN.

Hostname/FQDN are human-readable evidence and must not become immutable entity authority.

### 2.3 User attribution requires temporal session evidence

Directory existence/membership does not prove a user used an IP at a Flow timestamp.

User attribution may use time-bounded evidence such as:

- Windows/Linux Agent login/session evidence;
- AD/domain authentication events;
- Microsoft Entra identity events;
- VPN sessions;
- RADIUS / 802.1X;
- WLAN controller user/client mappings;
- NAC;
- firewall user mappings;
- proxy/SWG identity;
- VDI/session mappings.

A normalized attribution should retain, as applicable:

- user identity;
- display name;
- directory/domain;
- identity source;
- session start/end;
- observed time;
- confidence/quality;
- conflict state.

Conflicting evidence must remain visible. Platform must not silently choose one user.

### 2.4 Domain, application and URL are distinct

Ordinary NetFlow/IPFIX/sFlow must not be represented as providing a full URL when it does not.

Flow enrichment must distinguish:

- DNS/FQDN/domain evidence;
- exporter/vendor application classification;
- service/SaaS/cloud classification;
- URL evidence.

A domain is not a URL.

Full URL path/query must not be inferred from DNS/IP correlation.

Where another governed telemetry source supplies URL data, host/path handling must follow explicit privacy/security policy. Query strings, credentials, tokens and personal form content are not default Flow/NTA collection.

### 2.5 Internal geography versus public-IP geolocation

Internal geography should prefer governed Platform inventory/topology:

```text
entity -> subnet/site -> city/region/country
```

Public-IP geolocation is approximate network-location evidence, not proof of a person's physical location.

Public-IP Geo evidence should retain:

- source/database/catalogue;
- version;
- observed/effective time;
- granularity/accuracy class;
- confidence.

### 2.6 ASN, organization, cloud and SaaS context

For external destinations/sources, Flow enrichment should support, where reliable evidence exists:

- ASN;
- BGP/operator organization;
- cloud provider;
- provider region/service where available;
- SaaS/application classification;
- catalogue/prefix source;
- catalogue version/time.

Cloud/SaaS classification must remain provenance-bearing and must not be inferred beyond the evidence.

### 2.7 Recommended enrichment sequence

```text
raw Flow
  -> tenant/exporter/observation-point authority
  -> protocol/template/sequence/sampling normalization
  -> interface/VRF/VLAN
  -> temporal canonical entity resolution
  -> IPAM/DHCP
  -> DNS
  -> Agent/endpoint identity
  -> authenticated session identity sources
  -> site/internal geography
  -> ASN/BGP
  -> public Geo evidence
  -> cloud/SaaS catalogues
  -> exporter application metadata
  -> Flow analytics
```

Every non-raw enrichment must retain source/provenance, time/version and confidence/quality.

### 2.8 Raw evidence preservation

Do not rewrite source Flow merely because enrichment changes later.

Raw accepted Flow/source identity and enrichment-derived facts must remain distinguishable.

Historical queries must either:

- use time-aware enrichment/history; or
- preserve the enrichment version/provenance used for that observation.

---

## 3. Flow data semantics to preserve

A Flow observation should be able to expose, where evidence exists:

### Source

- IP/port;
- MAC where actually known;
- canonical endpoint/device;
- hostname/FQDN;
- attributed user;
- directory/domain;
- site/region/country;
- subnet/VLAN/VRF;
- ASN/organization.

### Destination

- IP/port;
- canonical endpoint/service;
- hostname/FQDN/domain;
- application/service;
- cloud/SaaS provider;
- site/region/country;
- ASN/organization.

### Traffic

- protocol;
- bytes/packets;
- observed versus sampling-estimated values;
- sampling metadata;
- DSCP/QoS;
- NAT;
- TCP flags where available;
- exporter-supplied/application classification provenance.

### Observation quality/provenance

- exporter;
- observation domain/engine;
- ingress/egress interface;
- Flow protocol/version;
- Sensor;
- template/options generation;
- sequence/gap/reset state;
- event/observation time;
- quality;
- enrichment sources/versions.

---

## 4. Privacy and RBAC requirements

User-attributed Flow is operational evidence, not employee-surveillance authority.

The final Mother Reference reconciliation must define separate authorization/audit boundaries for sensitive Flow identity access. Candidate permission granularity to reconcile with the canonical RBAC namespace includes:

- Flow viewing;
- identity-enriched Flow viewing;
- user-attribution viewing;
- Flow export.

Do not create these exact permission names without checking current RBAC conventions.

Access to user-attributed Flow and sensitive exports must be auditable.

Default Flow/NTA behavior must not become unrestricted browsing-history/session-content collection.

---

## 5. Production Sensor-role architecture to preserve

### 5.1 One Sensor software product

Do NOT create three independent Sensor products, source trees or installers.

Use the common Platform Sensor software/runtime with an explicit role selected/provisioned through governed configuration.

### 5.2 Production roles

The production architecture must support dedicated roles equivalent to:

- Network Monitoring Sensor;
- Flow Monitoring Sensor;
- Log Monitoring Sensor.

Final canonical enum names must be reconciled against existing Sensor role/state contracts.

#### Network role

Runs network polling/control workloads such as:

- ICMP/traceroute;
- SNMP;
- gNMI;
- REST monitoring;
- SSH/NETCONF/CLI;
- discovery/topology;
- NCM;
- authorized diagnostics.

#### Flow role

Runs Flow-specific workloads only, plus mandatory common Sensor infrastructure:

- NetFlow/IPFIX/sFlow/J-Flow/vendor Flow listeners;
- exporter/template/observation-domain state;
- sequence/sampling/time-quality handling;
- decode/normalize/enrichment;
- Flow durable spool/upload;
- Flow self-health.

It must not concurrently run normal SNMP polling, discovery, NCM, synthetics or log ingestion in production.

#### Log role

Runs log/event-specific workloads only, plus common Sensor infrastructure:

- Syslog;
- application/network log receiving;
- assigned OTLP Logs where architecture permits;
- parser/normalization;
- log durable spool/upload/forwarding;
- log self-health.

It must not concurrently run Flow or normal Network Sensor polling/discovery/NCM/synthetics in production.

### 5.3 Common mandatory Sensor machinery

Every production role still requires the shared minimum machinery needed to function safely:

- Sensor identity/mTLS;
- Core control/desired state;
- configuration;
- health/readiness;
- upgrade lifecycle;
- local durable spool machinery as applicable;
- audit/telemetry required for the role.

Role isolation does not create independent Sensor products.

### 5.4 Production exclusivity

High-rate Flow and Log roles should be production-exclusive so exporter/log storms cannot starve normal polling/NCM/availability workloads.

Combined high-rate roles may exist only as explicitly non-production development/test/demo/lab profiles unless later certification proves a broader production profile safe.

Production exclusivity is a resource/failure-isolation rule, not a requirement to run hardware at maximum utilization.

### 5.5 Headroom

No universal CPU/RAM/disk utilization target is asserted here.

Certification must derive safe operating envelopes from benchmark evidence and retain headroom for:

- burst load;
- spool growth;
- replay;
- template/parser churn;
- runtime/GC;
- upgrade;
- health;
- failover.

---

## 6. Role-specific sizing and storage

Flow Sensor sizing should use Flow workload dimensions, including:

- exporters;
- datagrams/sec;
- records/sec;
- bytes/sec;
- templates/options/sec;
- template churn;
- sampling;
- fields/record;
- IPv4/IPv6 mix;
- enrichment rate;
- spool rate;
- Core upload rate;
- replay/backlog.

Log Sensor sizing should use:

- sources;
- EPS;
- bytes/sec;
- TCP/TLS connections;
- parser complexity;
- multiline;
- redaction/fingerprinting;
- spool;
- downstream forwarding where supported.

These feed the existing Platform sizing/MWU authority rather than creating a separate capacity model.

Flow and Log roles require independent spool/resource protection so a high-rate workload cannot consume the continuity resources of Network polling.

Existing storage-watermark and no-silent-loss/gap-accounting authority remains governing.

---

## 7. Redundancy requirements

### 7.1 Flow

A production Flow role requires a certified redundancy model.

Exporter behavior differs:

- multiple simultaneous collectors;
- duplicated exports;
- single collector only;
- VIP/listener takeover;
- device-side failover/reconfiguration.

Do not assume a generic load balancer is correct for all Flow protocols/exporters.

Certified profiles must define:

- exporter fan-out/failover;
- template/generation state recovery;
- source identity;
- duplicate/replay handling;
- outage-gap visibility;
- capacity reserve.

### 7.2 Logs

Log redundancy must be protocol/source aware for:

- UDP Syslog;
- TCP Syslog;
- TLS Syslog;
- OTLP/log senders where applicable.

Do not assume identical failover/session behavior for all sources.

---

## 8. Sensor role lifecycle

Role selection must be visible during installation/enrollment/provisioning.

Role conversion must be governed and must not strand assigned work.

Expected lifecycle:

```text
ACTIVE
 -> DRAIN
 -> stop new assignments/sources
 -> flush/preserve spool according to durability contract
 -> verify no incompatible assigned sources/jobs
 -> change role
 -> restart/reconfigure as required
 -> health/readiness validation
 -> ACTIVE
```

Exact state names must be reconciled with the canonical Sensor lifecycle.

No live role conversion may silently leave incompatible workloads assigned.

---

## 9. Failure-domain invariant

The reason for dedicated production roles is monitoring correctness and predictable failure containment.

Examples:

- Flow exporter storm may degrade Flow, but must not starve SNMP/NCM/availability.
- Log storm may degrade logs, but must not starve Flow or Network polling.
- Role-specific backlog/spool exhaustion must surface explicit quality/gap/degradation state rather than masquerading as healthy monitoring.

---

## 10. Explicit non-goals

This reconciliation must NOT:

- create a new specialized Flow Mother Reference;
- create multiple Sensor codebases/installers;
- assume NetFlow/IPFIX contains username/full URL;
- infer precise human physical location from public-IP Geo;
- treat AD membership as session proof;
- enable unrestricted employee browsing surveillance;
- invent fixed CPU/RAM utilization thresholds without benchmarks;
- bypass existing Sensor reliability/durability authority;
- create a new Flow database/service merely for isolation;
- silently merge PR #70 and PR #74 without source review.

---

## 11. Reconciliation requirements for active Flow lanes

Before this PR can ever become merge-ready:

1. Re-fetch then-current canonical `main`.
2. Determine the surviving canonical Flow/NTA lane between PR #70 and PR #74 or their reconciled successor.
3. Compare this handoff requirement-by-requirement with that surviving lane.
4. Preserve every still-valid item above.
5. Remove duplicates already correctly present in the surviving Flow lane.
6. Reconcile with the then-current Agent/Sensor reliability authority.
7. Apply only load-bearing changes to existing Mother Reference files.
8. Do not create another specialized Flow master.
9. Delete this non-authoritative handoff file.
10. Run exact-head static/document checks, CI and independent review before merge.

---

## 12. Current status classification

Everything in this file is a **PROVISIONAL CTO DECISION / REQUIREMENT SAFETY NET**, approved for reconciliation but not canonical until merged into the Mother Reference through normal change control.

Terms:

- **CANONICAL** — supported by merged current `main`.
- **PROVISIONAL CTO DECISION** — approved direction pending Mother Reference reconciliation.
- **IMPLEMENTED** — source/runtime exists.
- **VERIFIED** — executable/static evidence supports the implementation.
- **CERTIFIED** — defined certification gates/evidence have passed.

This file intentionally does not convert provisional Flow decisions into canonical authority by its mere existence.


---

## 13. Additional accepted refinements from the second Flow review

The following refinements are also preserved for the eventual canonical Flow reconciliation. They supplement, and where more specific supersede, the earlier safety-net wording.

### 13.1 Three-layer logical Flow model

The logical Flow model should be treated as three separable layers:

1. **Source Observation** — what the exporter/source actually emitted, including exporter/observation-domain/template/sequence/sampling and source timestamps.
2. **Normalized Flow** — protocol-neutral Flow semantics suitable for ingestion, storage and analysis.
3. **Enrichment Context** — temporal endpoint/user/site/name/domain/application/Geo/ASN/cloud/topology context derived from other evidence.

Enrichment must never overwrite or redefine the source observation.

Physical storage is implementation-delegated. The logical separation does **not** require every enrichment field to be materialized into every historical ClickHouse row. Dictionary IDs, materialized projections, time-aware joins or other designs may be used if they preserve correctness, provenance, tenant isolation, replay behavior and query performance.

### 13.2 Identity-session model

User attribution should consume a canonical, tenant-scoped temporal session concept. A logical session may include:

- stable session/association identity;
- tenant;
- user identity;
- endpoint entity identity;
- IP/address binding where observed;
- session start/end/effective interval;
- source;
- confidence/quality;
- provenance.

This is a logical contract, not an instruction to create a separate Flow identity database or one new table per field.

The canonical entity/identity graph should be reused so the same temporal identity evidence can support Flow, logs, alerts, RCA, topology and endpoint monitoring.

### 13.3 Shared endpoints and attribution states

Shared or multiplexed systems such as:

- Citrix/terminal servers;
- VDI hosts;
- jump hosts;
- kiosks;
- NAT/proxy gateways;
- shared application servers;

may have a known endpoint but ambiguous user attribution.

Final canonical semantics should represent attribution states equivalent to:

- UNKNOWN;
- AMBIGUOUS;
- INFERRED;
- AUTHORITATIVE;

or the existing Platform quality vocabulary if one already covers the same meaning.

Do not fabricate a single user merely to populate a field.

### 13.4 Destination/name evidence levels

The canonical model must keep distinct evidence classes for:

- destination IP;
- reverse DNS;
- DNS-associated FQDN/domain;
- TLS SNI when actually observed by a source;
- HTTP Host when actually observed by a source;
- full URL when actually supplied by an authoritative telemetry source;
- exporter/vendor application classification;
- service/SaaS classification;
- organization/provider;
- ASN.

**Full URL must not be inferred from DNS, SNI, HTTP Host, destination IP or application classification.**

Temporal DNS correlation may support domain/FQDN association when bounded by endpoint/source context, TTL/effective time and other quality rules.

CDN/shared-hosting addresses must not become permanent domain identity.

### 13.5 Full-URL privacy boundary

Full URL/path/query collection is not mandatory Flow functionality.

Where another governed source provides it, the final policy must cover:

- query-string handling;
- token/credential stripping;
- masking/redaction;
- path normalization;
- cardinality bounds;
- RBAC;
- retention;
- export/egress controls.

Arbitrary query strings, credentials, reset tokens and personal content must not be blindly persisted.

### 13.6 Offline GeoIP / ASN datasets

Air-gapped operation requires GeoIP/ASN enrichment to work without a runtime Internet lookup.

Any offline dataset/package admitted for this purpose must have governed:

- source;
- licence;
- version;
- integrity hash;
- update/import workflow;
- provenance;
- compatibility/rollback behavior.

No public Geo API call is allowed per Flow record.

### 13.7 Enrichment execution placement is tiered

The Flow Sensor's first duty is correct, loss-aware reception/decoding and durable forwarding. Expensive enrichment must not make the receive path fragile.

Preferred logical placement:

```text
FLOW SENSOR — HOT PATH
  receive
  exporter/observation-domain authority
  template/options state
  sequence/sampling/time quality
  decode
  protocol normalization
  bounded low-cost local mappings/cache where justified
  durable spool / upload

CORE / INTERNAL STREAM PROCESSING
  canonical entity resolution
  historical IPAM/DHCP/site mapping
  identity-session/user association
  DNS/domain correlation
  AD/Entra/NAC/VPN context
  ASN/Geo/cloud/SaaS classification
  NAT/topology/change context
  versioned enrichment
  analytics/storage/query
```

Kafka remains internal to Core. Sensors continue to communicate through the Platform ingestion boundary rather than writing directly to Kafka.

Some enrichment may be cached at the Sensor only when benchmarking proves it reduces cost without compromising ingestion correctness.

There must be no:

- LDAP/AD query per Flow record;
- live DNS lookup per destination Flow;
- external GeoIP/ASN API lookup per Flow.

### 13.8 Existing Infrastructure Sensor role must not be lost

Canonical main already models an **Infrastructure Sensor Pool** separately from the Network Sensor Pool.

The eventual dedicated-role reconciliation must therefore preserve at least these product role families:

- Network Monitoring Sensor;
- Infrastructure Monitoring Sensor;
- Flow Monitoring Sensor;
- Log Monitoring Sensor;
- other specialist roles only when explicitly governed.

The new exclusivity decision applies specifically to high-rate Flow and Log roles.

It does **not** authorize collapsing Infrastructure monitoring into the Network role without a separate architecture decision.

Final enum names must match the then-current canonical Sensor role model.

### 13.9 Hard placement compatibility

Core placement/assignment must treat Sensor role as a hard eligibility constraint.

Examples:

- SNMP/network polling -> eligible Network role;
- infrastructure/agentless workload -> eligible Infrastructure role as defined by canonical contracts;
- Flow exporter/listener -> Flow role;
- Syslog/log source -> Log role.

Incompatible assignment must fail closed with a stable machine-readable reason. A candidate reason such as `SENSOR_ROLE_INCOMPATIBLE` is acceptable for design discussion, but the final code must follow the canonical error-code namespace.

Capacity pressure must never cause automatic cross-role fallback.

### 13.10 Role assignment and conversion semantics

A Sensor role is effectively immutable while active customer workloads are assigned, but it is not necessarily immutable for the lifetime of the installed software.

Installation/enrollment should expose governed role selection and clearly show exclusive-role behavior.

Role conversion requires drain/reassignment validation as already described in §8.

### 13.11 Flow Sensor onboarding and exporter preflight

The final UI/workflow should support a dedicated Flow Sensor onboarding path and separate Flow exporter/source onboarding.

A Flow Sensor preflight should be able to prove, where applicable:

- packets/datagrams received;
- detected Flow protocol/version;
- exporter identity;
- observation domain/engine;
- template/options receipt and validity;
- data-record decode success;
- sampling state;
- sequence state/gaps;
- interface mapping availability;
- source time quality;
- estimated records/sec and ingress bandwidth;
- Sensor capacity/storage readiness.

This is a preflight/activation workflow, not permission to bypass normal tenant/RBAC/audit controls.

### 13.12 Flow-specific HA and duplicate semantics

Flow HA differs from polling HA because exporters send to configured collector addresses.

Certified deployment profiles must cover source-specific mechanisms such as:

- multiple collector destinations;
- duplicate export;
- active/passive listener/VIP;
- device-specific failover/reconfiguration.

If identical or overlapping export streams can arrive at multiple Flow Sensors, duplicate/replay handling must use exporter/observation-domain/template/sequence/record identity and source semantics.

A 5-tuple alone is insufficient for deduplication.

### 13.13 Flow Sensor self-observability

A production Flow Sensor must expose more than process-up health.

At minimum, certification should consider:

- packets/datagrams/sec;
- records/sec;
- bytes/sec;
- exporters active;
- observation domains;
- template/options count;
- template misses;
- template churn;
- sampling state;
- sequence gaps/resets;
- kernel/socket drops;
- receiver queue drops;
- decode failures;
- decode/normalization latency;
- enrichment latency where local enrichment exists;
- durable spool size/age;
- upload rate;
- replay backlog;
- CPU/RAM/disk/network;
- per-exporter rate;
- per-exporter drops/quality.

Exact metric names and thresholds are implementation/certification concerns.

### 13.14 Log Sensor self-observability and isolation

The same isolation principle applies to Log Sensors.

A dedicated Log role may own, where canonical logging authority permits:

- Syslog UDP/TCP/TLS;
- remote/application/network log receiving;
- OTLP Logs where assigned;
- parsing;
- multiline;
- redaction;
- fingerprinting/deduplication;
- durable spool;
- downstream routing/forwarding when architecture assigns it there.

It must not accept Flow or ordinary polling workloads in production.

### 13.15 Role-specific capacity models

Flow and Log capacity must not be represented primarily by monitored-device count.

Flow workload inputs should include:

- exporters;
- datagrams/sec;
- records/sec;
- bytes/sec;
- templates/options/sec and churn;
- fields/record;
- sampling;
- IPv4/IPv6 mix;
- enrichment cost;
- spool horizon/pressure;
- Core upload/replay rate.

Log workload inputs should include:

- sources;
- EPS;
- bytes/event;
- TCP/TLS connection count;
- parser complexity;
- multiline;
- redaction/fingerprinting;
- routing destinations;
- spool horizon.

These are inputs to the existing Platform sizing/capacity authority rather than a new capacity subsystem.

### 13.16 Flow completeness/coverage quality

Flow completeness should be measured against configured/expected exporter and observation coverage, not by assuming that SNMP proves individual Flow records.

Useful quality dimensions may include:

- expected exporters versus active exporters;
- expected observation domains/interfaces versus observed;
- template readiness;
- sequence gaps;
- sampling knowledge;
- last accepted Flow;
- source freshness;
- source/exporter configuration drift where observable;
- known blind spots.

SNMP/device inventory may provide expected interface/exporter context, but it is supporting evidence rather than authoritative proof that every Flow record was received.

### 13.17 Privacy, RBAC and retention

Identity/domain/full-URL enrichment is more sensitive than ordinary interface telemetry.

The final canonical reconciliation must:

- reuse or extend the canonical RBAC namespace rather than invent permission names in isolation;
- permit separation between ordinary Flow access, identity-attributed Flow, domain/full-URL access and export where appropriate;
- audit sensitive access/export;
- permit signal/association-specific retention classes;
- avoid hard-coding example retention durations as defaults;
- preserve incident/legal-evidence pinning through existing retention authority.

### 13.18 Explainability

Every derived identity/domain/application/Geo association shown to an operator must be explainable.

The UI/API should be able to expose, subject to RBAC:

- value;
- evidence source(s);
- effective/observed time;
- source/dataset version;
- confidence/quality;
- conflicting evidence where applicable.

Prefer per-association/per-field quality rather than one global Flow confidence score.

### 13.19 Storage/query design remains implementation-delegated

The richer enrichment model does not require all fields to be denormalized into every Flow row.

The implementation must benchmark and choose among:

- dictionary IDs;
- materialized enrichment;
- time-aware joins;
- projections/materialized views;
- bounded query-time enrichment;

while preserving tenant isolation, historical correctness, replay/idempotency, retention behavior and acceptable query cost.

No new Flow database or identity database is authorized by these requirements.

### 13.20 Provisional role/enrichment architecture

The current reconciliation target is:

```text
                         PLATFORM CORE
                              |
                  policy / identity / graph
                              |
       +----------------------+----------------------+
       |                      |                      |
 NETWORK SENSOR          FLOW SENSOR            LOG SENSOR
 network role            exclusive Flow         exclusive Log
       |                      |                      |
 polling/discovery       NetFlow/IPFIX          Syslog/logs
 topology/NCM            sFlow/J-Flow           OTLP Logs where assigned
       |                      |                      |
       |                 normalized Flow              |
       +----------------------+----------------------+
                              |
                    identity/context graph
                              |
              DNS/IPAM/Agent/NAC/VPN/AD/Entra
                              |
                     versioned enrichment
                              |
                     internal Core pipeline
                              |
                         ClickHouse/UI/RCA
```

Infrastructure Sensor remains a separate canonical role family and must be included in the final role model even though omitted from this compact diagram.

---

## 14. Challenge outcomes recorded

The following parts of the second review were deliberately **qualified rather than copied literally**:

1. **Full URL:** may be stored only when actually observed/provided by an authoritative source. DNS/SNI/Host correlation can establish domain/host evidence, not a fabricated full URL.
2. **Enrichment-before-Kafka diagram:** not adopted literally. Kafka is internal to Core; expensive enrichment should usually occur after the Sensor has safely decoded/normalized/spooled/uploaded source Flow.
3. **Infrastructure Sensor:** retained as an existing canonical role family; not silently folded into Network Sensor.
4. **Permission names:** candidate names are illustrative only until reconciled with the canonical RBAC namespace.
5. **Retention durations:** examples are not defaults; retention remains policy-driven.
6. **Flow completeness:** SNMP/inventory may provide expected coverage context but does not prove per-record Flow completeness.

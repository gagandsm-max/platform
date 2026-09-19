# Platform — Agent Reliability, Failover, Durability & Resource Assurance Master

**Document ID:** PLATFORM-ARF-001
**Version:** 3.2.2
**Status:** **Authoritative Specialized Mother Reference — Agent Reliability / Failover / Durability / Resource Assurance**
**Product:** Platform
**Effective date:** 2026-09-19
**Scope:** Linux Agent + Windows Agent + Agent↔Sensor↔Core reliability contract
**Change classification:** Additive reliability hardening; no product-version increment; no Agent rewrite authorization

---

## 1. Authority and purpose

This document is the authoritative specialized Mother Reference for Linux/Windows Agent reliability, failover, durability, replay/idempotency, Agent operational state, configuration/upgrade/certificate transactions, Agent-started telemetry-integrity reconciliation, collector isolation, disk/resource protection and production failure certification.

It supplements, and does not silently override, the higher-precedence Platform Mother Reference set, especially:

- `Platform_01_BRD.md`
- `Platform_02_PRD.md`
- `Platform_03_Architecture.md`
- `Platform_04_HLD.md`
- `Platform_05_LLD.md`
- `Platform_06_Dataflow.md`
- `Platform_10_Protocol_Security_Connector_Master.md`
- `Platform_11_Sizing_Capacity_Storage_Master.md`
- `Platform_14_Test_Performance_and_Release_Quality.md`
- `Platform_15_CI_CD_Packaging_and_Distribution.md`
- `Platform_16_Installation_Upgrade_Rollback_and_Operations.md`
- `Platform_17_API_Event_and_Data_Model_Contracts.md`
- `Platform_18_Security_Threat_Model.md`
- `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md`
- `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md`

The engineering objective is:

> Failures may occur anywhere, but no single certified failure may silently destroy unexplained telemetry, create conflicting authority, corrupt Agent identity, or convert a monitoring-path failure into a false host-down assertion. Collection and recovery shall continue wherever safe, telemetry shall remain recoverable within the certified buffering horizon, duplicates shall be handled idempotently, and unavoidable loss shall be explicitly measured, surfaced and audited.

Absolute zero-loss is **not** a product claim.

---

## 2. Existing architecture is preserved

This document does **not** authorize replacement of the existing Agent/Sensor architecture.

The following remain architectural foundations:

- Go-based lightweight Linux/Windows Agent runtime;
- shared `agent/common` reliability/domain logic where OS-neutral;
- Core-authoritative Sensor placement and Agent assignment;
- mTLS identity;
- durable local Agent queue/spool;
- durable Sensor spool;
- asynchronous Agent→Sensor→Core transport;
- replay-safe/idempotent processing;
- versioned desired state;
- centralized policy;
- signed native packaging and updates;
- Linux `.deb` / `.rpm`;
- Windows compiled signed service + MSI;
- no customer-WAN Kafka;
- no new Agent database server.

Implementation shall enhance existing mechanisms before introducing new ones.

### 2.1 Explicit non-goals

Do **not**:

- replace the existing Agent durable queue merely to rename it a WAL;
- introduce PostgreSQL, RocksDB, Elasticsearch, an embedded server database, JVM or Python runtime into the normal Agent path without benchmark-backed architecture approval;
- create a second Agent control plane;
- create a second Sensor-placement authority;
- create a second Agent identity system;
- create parallel Linux and Windows reliability engines where shared implementation is practical;
- stream normal telemetry simultaneously to two Sensors solely for redundancy;
- allow an Agent to discover or select arbitrary Sensors outside Core authorization;
- synchronously couple normal Agent collection to Core, Kafka or ClickHouse availability;
- claim certified CPU/RAM/disk/network limits without measured evidence.

---

## 3. Formal additive product requirements

### PRD-ARF-001 — Durable Agent local protection

Linux and Windows Agents shall durably preserve unacknowledged telemetry across Agent process failure, service restart and host reboot within the certified storage horizon. The implementation shall remain lightweight and bounded.

### PRD-ARF-002 — Explicit durability/ACK contract

Agent, Sensor and Core shall expose explicit durability stages sufficient to prove where telemetry is recoverable. A transport-level success response alone shall never authorize deletion of the Agent's only recoverable copy.

### PRD-ARF-003 — Stable observation identity and replay safety

Agent-produced observations/batches shall retain stable source identity, sequence lineage and correlation data through Sensor, Core, Kafka and persistence so retry/failover can be processed idempotently.

### PRD-ARF-004 — Core-authorized primary and fallback Sensors

Production Agent assignment shall support one primary Sensor plus zero or more policy-authorized fallback Sensors where HA policy/topology requires them. Core Sensor Control remains authoritative.

### PRD-ARF-005 — Single active telemetry destination by default

Normal steady-state Agent telemetry shall use one authoritative active Sensor. Fallback Sensors may be pre-authorized/prevalidated but shall not receive a second full telemetry stream unless an explicitly approved criticality policy requires dual delivery.

### PRD-ARF-006 — Assignment fencing and split-brain prevention

Stale or conflicting Agent→Sensor assignments shall not become authoritative. Assignment authority shall use monotonic fencing/version semantics and fail closed on ambiguity.

### PRD-ARF-007 — Explicit Agent operational state machine

Agent health shall expose meaningful operational states and transition reasons rather than only CONNECTED/DISCONNECTED.

### PRD-ARF-008 — Authorized failover without immediate Core dependency

An already enrolled Agent may fail over to a previously Core-authorized fallback Sensor during Core/WAN disruption without requiring a fresh synchronous Core decision, provided identity, assignment validity and fencing checks succeed.

### PRD-ARF-009 — Capacity-safe failover

HA placement shall reserve sufficient Sensor capacity for failure absorption, replay backlog and operating headroom. N+1 refers to usable capacity, not merely server count.

### PRD-ARF-010 — Atomic Agent configuration

Agent desired configuration/policy application shall be validated, staged and atomically activated. A failed new configuration shall leave the previous known-good configuration active.

### PRD-ARF-011 — Safe certificate rotation

Agent/Sensor certificate rotation shall use overlap/staged validation so routine credential renewal does not unnecessarily interrupt healthy monitoring.

### PRD-ARF-012 — Transactional Agent upgrade and rollback

Agent upgrade success requires restored monitoring continuity, not merely process start. Failed upgrade validation shall automatically return to a known-good signed version where technically supported.

### PRD-ARF-013 — Agent-started pipeline integrity

Platform shall reconcile telemetry from Agent production through Sensor durability, Core acceptance, streaming and final persistence/deduplication. Unexplained deltas are integrity faults.

### PRD-ARF-014 — Explicit backpressure

Agent/Sensor transport shall use bounded retry, exponential backoff, jitter and replay-rate control. Congestion shall not cause unbounded reconnect/upload storms.

### PRD-ARF-015 — Collector failure isolation

Failure, timeout or overload of one Agent collector shall degrade that collector rather than silently terminate unrelated collectors, transport, local durability or the Agent core where technically feasible.

### PRD-ARF-016 — Monitoring-path failure is not host failure

`AGENT_OFFLINE`, `SENSOR_UNREACHABLE`, `TELEMETRY_STALE`, `COLLECTOR_DEGRADED`, `AUTHENTICATION_FAILURE` and `PIPELINE_BACKLOG` are not equivalent to `HOST_DOWN`. Host/service failure assertions require the applicable evidence contract.

### PRD-ARF-017 — Bounded Agent resource/storage behavior

Agent CPU, RAM, disk, network, queues, threads/goroutines and logs shall remain bounded under certified workloads and failure/replay scenarios. Storage protection shall prioritize the monitored host over telemetry retention.

### PRD-ARF-018 — Failure-injection and resource certification

Agent reliability is not production-ready until required failure modes, recovery behavior, replay/idempotency, security/tenant boundaries and resource envelopes are demonstrated with repeatable evidence on Linux and Windows as applicable.

---

## 4. Agent durable queue / WAL contract

The current durable Agent queue is the baseline implementation and shall be evolved in place unless an approved architecture review proves replacement necessary.

### 4.1 Required invariants

The Agent queue shall provide:

- append-before-send durability;
- stable monotonic sequence lineage;
- payload integrity checksum;
- bounded record size;
- bounded queue record count;
- bounded queue bytes;
- crash/reboot recovery;
- atomic publication of durable records/state;
- fail-closed handling where durability cannot be confirmed;
- replay in original source order where ordering is required;
- explicit corruption/quarantine behavior;
- exact accounting for pending/accepted/quarantined/dropped records;
- no deletion from an untrusted or ambiguous ACK.

A segmented append-only implementation is acceptable; one universal physical file layout is not mandated.

### 4.2 Logical identity

Every Agent record/batch shall carry, directly or derivably:

- tenant identity;
- `agent_id`;
- canonical entity reference where available;
- stable record/observation ID;
- Agent source sequence;
- batch/range identity;
- event time;
- ingest/creation time;
- schema version;
- correlation ID where applicable;
- payload checksum;
- assignment generation/context required for authority validation.

A `boot_epoch` may be added if needed by the final lineage design, but must not replace durable monotonic Agent sequence identity.

---

## 5. Durability stages and ACK semantics

### 5.1 Logical stages

The platform shall distinguish at least:

```text
AGENT_DURABLE
SENSOR_DURABLE
CORE_INGRESS_ACCEPTED
PIPELINE_PERSISTED
```

Additional internal stages may be exposed, such as Kafka-produced/consumed and ClickHouse-inserted.

### 5.2 Current Agent deletion authority

Until the open Core two-node Kafka/ClickHouse/fencing P0 items are experimentally resolved, the Agent shall **not** hard-code `CORE_DURABLE` as its deletion boundary.

The current safe contract is:

> Agent deletion may occur only after a valid, Agent-bound, batch/range-bound application ACK that proves the receiving authorized Sensor has durably persisted the accepted range and can recover/replay it after Sensor process restart.

The Sensor MUST NOT emit that ACK before its durability boundary has succeeded.

The Agent MUST retain the records if:

- ACK is absent;
- ACK identity does not match the Agent;
- ACK batch/range does not match submitted data;
- ACK sequence regresses;
- ACK is outside the submitted range;
- receiving Sensor is unauthorized/stale;
- Agent cannot durably commit its own accepted watermark/cleanup metadata.

### 5.3 Downstream durability evolution

Platform shall preserve enough end-to-end sequence/identity state to permit future tightening of the Agent deletion boundary through a versioned contract if engineering evidence shows that a downstream Core-durable ACK is required and operationally viable.

No documentation may claim a final `CORE_DURABLE` semantics before the relevant P0 design work is resolved.

---

## 6. End-to-end observation identity and idempotency

Retry and failover are expected to create duplicate transmission. Correctness shall therefore rely on deterministic identity and idempotent processing, not on assuming duplicate transport can always be prevented.

The identity chain shall remain traceable across:

```text
Agent
→ Sensor
→ Core ingestion
→ Kafka
→ normalization/consumers
→ ClickHouse/persistent destination
→ Alert/RCA/Incident evidence where applicable
```

Downstream retry shall not create duplicate:

- metrics/state facts where deduplication applies;
- operational alerts;
- incidents;
- SLA samples;
- RCA evidence records.

The exact physical deduplication layer may vary by event class, but the original source identity must remain available for reconciliation.

---

## 7. Primary/fallback Sensor assignment

### 7.1 Assignment model

The Agent reliability contract shall support one authoritative assignment generation containing:

- assignment ID;
- monotonic assignment generation/version;
- tenant ID;
- Agent ID;
- primary Sensor ID/endpoint;
- zero or more authorized fallback Sensor IDs/endpoints;
- issue time;
- validity/expiry where used;
- policy/config hash where needed;
- signature/authenticity evidence where the offline failover contract requires it.

The exact serialization is an LLD/API decision. Multiple independent assignment authorities are prohibited.

### 7.2 Placement inputs

Core Sensor Control shall consider at least:

- tenant/security boundary;
- site/zone/locality;
- reachability;
- RTT/loss;
- Sensor role/capability;
- Sensor health;
- CPU/RAM;
- disk;
- queue/backlog;
- Monitoring Work Units/current workload;
- maintenance/drain state;
- HA reserve/failover capacity.

### 7.3 Active destination rule

At any time the Agent shall have one active authoritative telemetry destination unless an explicit approved criticality profile authorizes dual delivery.

Fallback readiness may include:

- cached verified endpoint;
- cached Core-issued assignment;
- trust material;
- lightweight health/authentication prevalidation.

It shall not double normal telemetry load by default.

---

## 8. Assignment fencing and failover

### 8.1 Fencing

The existing monotonic assignment `generation` is the primary fencing primitive and shall be extended rather than replaced unless evidence requires a distinct lease/fencing token.

A receiving Sensor shall reject delivery when:

- tenant/Agent identity does not match assignment;
- the Sensor is not authorized by the current assignment/fallback set;
- assignment generation is stale;
- assignment is expired/revoked where expiry/revocation applies;
- conflicting authority cannot be resolved safely.

### 8.2 Offline-authorized failover

Target sequence:

```text
Primary Sensor fails
  ↓
Agent continues local collection
  ↓
failure threshold / circuit state reached
  ↓
Agent selects previously Core-authorized fallback
  ↓
mTLS
  ↓
fallback validates identity + assignment + generation/fence
  ↓
Agent resumes telemetry
  ↓
Core later reconciles observed placement
```

Core remains the authority that created the fallback authorization. Core is not required to be synchronously reachable for every failover attempt.

### 8.3 Failback

Failback to a preferred primary must avoid oscillation. The connection manager shall use hysteresis/cooldown and current assignment authority rather than immediately bouncing between Sensors.

---

## 9. Agent operational state machine

The implementation shall expose a bounded state model including, at minimum where applicable:

```text
STARTING
ENROLLING
PENDING_APPROVAL
CONNECTING
ACTIVE_PRIMARY
ACTIVE_FALLBACK
BUFFERING
REPLAYING
REASSIGNING
CONFIG_APPLYING
CERT_ROTATING
UPGRADING
DEGRADED
SECURITY_HOLD
NO_AUTHORIZED_SENSOR
OFFLINE
```

Equivalent names may be used if semantics remain explicit.

Every material transition shall expose:

- previous state;
- new state;
- timestamp;
- stable reason/error code;
- correlation ID where applicable;
- active endpoint/Sensor;
- assignment generation;
- retry attempt/circuit state;
- certificate state;
- queue depth/bytes;
- oldest unacknowledged age;
- last successful durable delivery;
- relevant collector health.

The platform shall make these states available to support/NOC operations rather than reducing all failures to `Agent Offline`.

---

## 10. Configuration transaction

Desired Agent policy/config shall follow:

```text
receive version/hash
→ authenticate/verify source
→ schema validate
→ semantic validate
→ collector/runtime compatibility validate
→ resource/safety validate
→ stage
→ atomic activate
→ self-test
→ report APPLIED(version, hash)
```

On failure:

```text
REJECT(new version, stable reason)
KEEP(previous known-good version)
```

Partial mixed-version configuration is prohibited.

Desired state and observed/applied state shall remain separately visible.

---

## 11. Certificate rotation

Routine certificate rotation shall use staged trust overlap:

```text
issue replacement
→ distribute/validate trust
→ validate new identity path
→ switch active credential
→ verify authenticated Sensor communication
→ retain old valid path for bounded overlap
→ revoke/remove old credential
```

If replacement validation fails, the Agent should remain on the currently valid trusted path until policy/security limits require hold or intervention.

Expired, revoked or conflicting identities shall fail closed.

---

## 12. Agent upgrade and rollback

Upgrade sequence shall include:

```text
preflight
→ capacity/free-space check
→ package signature/hash verification
→ compatibility check
→ preserve known-good executable/config/state needed for rollback
→ stage
→ activate
→ service/process health
→ identity recovery
→ Sensor handshake
→ durable telemetry acceptance test
→ success
```

If post-upgrade monitoring validation fails, automatically roll back where technically supported.

An Agent upgrade is **not** successful merely because the new process starts.

Release evidence shall include package/reboot/upgrade/rollback results for:

- certified Linux distributions;
- Windows Service/MSI certification environment.

---

## 13. Backpressure, replay and retry

Agent transport shall implement bounded:

- connection timeout;
- delivery timeout;
- retry count/state;
- exponential backoff;
- jitter;
- circuit-breaker behavior where useful;
- batch records/bytes;
- replay rate;
- concurrent delivery workers.

When Sensor capacity pressure or transient unavailability occurs:

```text
continue safe local collection
→ durable queue grows
→ bounded retry/backoff
→ reduce replay/transmit rate
→ recover without reconnect storm
```

Replay after outage must coexist with current collection without starving the monitored host or overwhelming Sensor/Core.

---

## 14. Collector isolation

Each collector shall have:

- bounded execution time;
- cancellation;
- bounded work/result queue;
- independent health;
- stable failure reason;
- resource accounting where relevant.

A CPU/process/log/network collector failure shall not automatically terminate unrelated collection, transport or local durability.

The implementation may use goroutine-level isolation for normal collectors and stronger process isolation only where measured risk justifies the complexity.

Collector state shall support at least:

```text
HEALTHY
DEGRADED
TIMED_OUT
BACKPRESSURED
DISABLED
FAILED
```

Equivalent semantics are acceptable.

---

## 15. Monitoring-state semantics

The Agent reliability domain shall integrate with Platform alert-evidence rules.

These conditions are distinct:

- `HOST_DOWN`
- `AGENT_OFFLINE`
- `SENSOR_UNREACHABLE`
- `TELEMETRY_STALE`
- `COLLECTOR_DEGRADED`
- `AUTHENTICATION_FAILURE`
- `PIPELINE_BACKLOG`
- `NO_AUTHORIZED_SENSOR`
- `BUFFERING`
- `SECURITY_HOLD`

An unreachable Agent is evidence about the monitoring path. It is not sufficient by itself to assert the host is down when credible contradictory evidence exists.

---

## 16. Pipeline integrity and reconciliation

### 16.1 Required stages

Platform operations shall be able to reconcile, at appropriate aggregation granularity:

```text
Agent Produced
Agent Durably Spooled
Sensor Durably Accepted
Sensor Forwarded
Core Accepted
Kafka Produced
Kafka Consumed
Persistent Store Inserted
Deduplicated
Rejected
Explicitly Shed/Lost
```

### 16.2 Accounting invariant

For a closed reconciliation window, all produced telemetry shall be explainable as persisted, pending, deduplicated, explicitly rejected, or explicitly lost/shed.

Conceptually:

```text
Produced
=
Persisted
+ Pending
+ Deduplicated
+ Explicitly Rejected
+ Explicitly Lost/Shed
```

An unexplained delta shall raise an internal telemetry-integrity condition with tenant/source/time/range context sufficient for investigation.

Healthy processes do not, by themselves, prove healthy telemetry.

---

## 17. Agent disk-pressure and loss policy

Agent storage protects the monitored host first.

Agent disk policy shall be separately certified from the fixed Core/Sensor 80% managed-storage rule unless later architecture explicitly adopts a common threshold.

Controls shall include:

- queue byte limit;
- record-count limit;
- minimum filesystem free-space floor;
- warning/critical state;
- oldest queued age/protection horizon;
- bounded logs;
- no unbounded preallocation;
- segmented/grow-on-demand storage where appropriate.

Approximate priority when emergency shedding becomes unavoidable:

```text
security / critical-state evidence
state transitions / integrity facts
important operational metrics
normal metrics
verbose logs
```

The exact class policy is configuration/architecture-controlled.

If telemetry is discarded because outage/storage limits exceed the certified protection horizon, Platform shall record and later report:

- exact or best-known source sequence/range;
- count/bytes;
- class/priority;
- time window;
- reason code;
- disk state;
- Agent/Sensor context;
- audit/integrity event.

Silent discard is prohibited.

---

## 18. WAL sizing model

Do not use one universal fixed Agent spool size.

Sizing is workload/profile dependent:

```text
required WAL
=
P95 compressed telemetry generation rate
× required outage-protection horizon
× metadata/replay overhead
× burst/safety factor
```

A starting engineering safety factor of approximately `1.30` may be tested, but it is not a universal certified constant.

Admin/customer policy may select the protection horizon within safe host-resource limits.

---

## 19. Capacity-safe Sensor HA

Failover capacity shall account for:

```text
normal assigned workload
+ failed-Sensor takeover workload
+ replay backlog
+ operational headroom
```

Placement/admission shall fail or downgrade HA assurance when the pool cannot absorb the required failure case.

A pool with multiple Sensors but insufficient spare capacity shall not be represented as certified N+1.

---

## 20. Common vs OS-specific implementation

### 20.1 Shared `agent/common` domain

Where technically practical, Linux and Windows shall share:

- telemetry envelope;
- observation/source identity;
- sequence lineage;
- durable queue contract;
- ACK validation;
- replay/idempotency;
- connection/failover manager;
- Sensor assignment/fencing;
- retry/backoff;
- operational state model;
- desired/applied configuration transaction;
- certificate state;
- upgrade state machine;
- integrity telemetry;
- health/error schema.

### 20.2 Linux-specific

Linux owns:

- systemd integration;
- `.deb`/`.rpm`;
- Linux filesystem/permissions/capabilities;
- journald integration;
- Linux collectors.

The Linux service shall remain least-privilege and hardened subject to collector compatibility.

### 20.3 Windows-specific

Windows owns:

- Windows Service/SCM;
- MSI + supported scripted wrappers;
- NTFS/ACL;
- Windows Event Log;
- Windows collectors;
- Windows-specific service recovery and installer semantics.

Reliability behavior shall not be duplicated independently unless OS/runtime evidence makes separation necessary.

---

## 21. Resource certification targets

The following are **engineering certification targets pending benchmark evidence**, not current product claims:

### 21.1 Standard Agent target

- steady-state CPU target: `≤1% of one logical CPU`;
- short controlled burst objective: `≤5%` during collection/replay/update where workload permits;
- steady-state RSS target: `≤100 MB`;
- initial normal certification ceiling objective: `128 MB`;
- base installed footprint target: `≤200 MB` excluding dynamic WAL and rollback assets;
- no embedded DB server;
- no JVM;
- no Python runtime required for normal Agent operation;
- no unbounded queue;
- no unbounded goroutines/threads;
- no unbounded logs.

Heavy profiles shall be benchmarked separately.

These values may be revised only from measured evidence and controlled Mother Reference change.

### 21.2 Required measurements

Certification records at least:

- P50/P95/P99 CPU;
- steady and peak RSS;
- collection latency;
- records/bytes generated;
- bytes transmitted;
- queue write rate and fsync cost;
- WAL growth rate;
- replay throughput;
- replay + live-collection contention;
- network overhead;
- disk occupancy/protection horizon;
- startup/recovery time.

---

## 22. Mandatory failure-injection matrix

Automated or repeatable certified testing shall cover, as applicable:

### 22.1 Agent-local
- kill Agent process;
- host reboot;
- crash loop;
- WAL full;
- WAL corruption;
- filesystem read-only;
- disk I/O failure;
- partial durability/fsync failure.

### 22.2 Agent↔Sensor
- primary Sensor failure;
- fallback Sensor failure;
- primary + fallback unavailable;
- Sensor restart;
- Sensor CPU/disk/queue pressure;
- DNS failure;
- route failure;
- TCP timeout/reset;
- packet loss/high latency/network flap;
- ACK loss;
- duplicate ACK;
- duplicate telemetry;
- out-of-order telemetry;
- replayed telemetry.

### 22.3 Identity/authority
- mTLS failure;
- expired Agent certificate;
- expired/revoked Sensor certificate;
- Agent certificate revocation;
- certificate rotation failure;
- clock skew;
- stale assignment;
- conflicting assignment;
- attempted dual ownership;
- reassignment;
- unauthorized fallback;
- tenant mismatch.

### 22.4 Core/pipeline
- Core unavailable;
- Core HA failover;
- Kafka unavailable;
- ClickHouse unavailable;
- long Core outage;
- large replay backlog;
- replay while current collection continues.

### 22.5 Desired state/lifecycle
- bad policy;
- bad config version;
- partial config attempt;
- Agent upgrade failure;
- rollback;
- package corruption;
- signature failure.

### 22.6 Network identity changes
- DHCP/IP change;
- VPN transition;
- interface change.

Every test shall answer:

- Was safe collection preserved?
- Was telemetry durably retained within the certified horizon?
- Was data lost?
- If lost, was the exact/best-known loss measured and surfaced?
- Was recovery automatic where safe?
- Was duplicate operational processing prevented?
- Was the correct Agent state shown?
- Was the failure reason observable?
- Was an audit/integrity event emitted where required?
- Did CPU/RAM/disk/network remain bounded?
- Were tenant/security boundaries preserved?

---

## 23. Release-blocking acceptance criteria

Agent reliability shall not be called production-ready until evidence proves, for the certified profile:

1. Primary Sensor failure does not stop safe local collection.
2. Agent can use an already-authorized fallback without manual reinstall.
3. Stale/conflicting Sensor assignment cannot become authoritative.
4. Replayed Agent telemetry does not create duplicate operational outcomes.
5. Reboot preserves identity, configuration and unacknowledged telemetry.
6. Bad configuration cannot partially activate.
7. Failed upgrade returns to a known-good version where technically supported.
8. Certificate rotation avoids unnecessary monitoring outage.
9. Core/Sensor failure is visible as degraded/buffering/failover state rather than silent disappearance.
10. Pipeline reconciliation explains produced telemetry through final disposition.
11. CPU/RAM/disk/network are bounded under certified load and failure/replay tests.
12. Linux Agent remains lightweight under its certified profile.
13. Windows Agent meets equivalent reliability semantics under Windows certification.
14. Agent-offline conditions do not automatically become false `HOST_DOWN`.
15. Tenant/identity/security boundaries remain intact.
16. Any unavoidable data loss is explicitly quantified and surfaced.

---

## 24. Current P0 dependencies and conservative rule

This specialized master does **not** resolve by declaration:

- exact two-Core Kafka failover;
- exact two-Core ClickHouse synchronization/failover;
- exact Core HA fencing implementation;
- final Sensor spool sizing/certification;
- any future end-to-end `CORE_DURABLE` ACK propagation design.

Until those are experimentally resolved, engineering shall prefer conservative retention and explicit state over premature deletion or unsupported durability claims.

The current certified Agent deletion contract may rely on `SENSOR_DURABLE` only when Sensor durability-before-ACK, restart recovery and replay semantics are proven by tests.

---

## 25. Implementation discipline

Before changing Agent/Sensor/Core code for this domain, engineering shall classify each relevant requirement as:

```text
IMPLEMENTED ALREADY
PARTIALLY IMPLEMENTED
MISSING
CONFLICTING
DOCUMENTATION ONLY
TEST ONLY
```

Do not infer implementation from this document.

Use the existing repository authority/routing model:

- source-truth rebaseline first;
- smallest defensible writer set;
- Agent/API/Sensor/SRE/security/performance specialists only where relevant;
- writer may not self-certify;
- exact-head/current-base review;
- runtime failure injection for durability/failover changes;
- benchmark evidence for resource claims;
- evidence bundle before readiness claim.

Do not rewrite the entire Agent.

---

## 26. Required engineering workstream decomposition

Recommended bounded implementation tranches:

```text
ARF1  Current-state requirement/code/test reconciliation
ARF2  Durability-stage + ACK contract formalization
ARF3  End-to-end identity + reconciliation counters
ARF4  Primary/fallback Sensor assignment contract
ARF5  Fencing + offline-authorized failover
ARF6  Agent operational FSM + diagnostics
ARF7  Atomic configuration transaction
ARF8  Certificate rotation transaction
ARF9  Upgrade/rollback transaction
ARF10 Collector isolation
ARF11 Backpressure/replay controls
ARF12 WAL/disk-pressure policy
ARF13 Shared Linux/Windows reliability consolidation where needed
ARF14 Failure-injection certification
ARF15 CPU/RAM/disk/network benchmark certification
```

Implementation ordering may change based on dependency evidence, but P0 durability/authority correctness precedes cosmetic status/UI work.

---

## 27. Formal requirements traceability supplement

| REQ ID | Requirement | Governing PRD | Primary logical sections | Mandatory evidence |
|---|---|---|---|---|
| `REQ-ARF-001` | Durable bounded Agent local queue across restart/reboot | `PRD-ARF-001` | §§4,17,18 | crash/reboot/disk tests |
| `REQ-ARF-002` | Explicit durability stages and safe ACK deletion rule | `PRD-ARF-002` | §5 | ACK/fsync/restart failure injection |
| `REQ-ARF-003` | Stable observation identity and replay idempotency | `PRD-ARF-003` | §§4,6,16 | duplicate/replay reconciliation tests |
| `REQ-ARF-004` | Core-authorized primary + fallback Sensors | `PRD-ARF-004` | §7 | placement/failover tests |
| `REQ-ARF-005` | Single active telemetry destination by default | `PRD-ARF-005` | §7 | load/transport verification |
| `REQ-ARF-006` | Assignment fencing / split-brain prevention | `PRD-ARF-006` | §8 | stale/conflicting authority tests |
| `REQ-ARF-007` | Explicit Agent operational FSM | `PRD-ARF-007` | §9 | transition/reason observability tests |
| `REQ-ARF-008` | Failover without immediate Core dependency | `PRD-ARF-008` | §8 | combined Core+primary outage test |
| `REQ-ARF-009` | Capacity-safe failover reserve | `PRD-ARF-009` | §19 | placement/admission/capacity evidence |
| `REQ-ARF-010` | Atomic configuration transaction | `PRD-ARF-010` | §10 | invalid/partial config tests |
| `REQ-ARF-011` | Safe certificate rotation overlap | `PRD-ARF-011` | §11 | rotation/failure tests |
| `REQ-ARF-012` | Upgrade health verification and rollback | `PRD-ARF-012` | §12 | package/reboot/rollback evidence |
| `REQ-ARF-013` | Agent-started end-to-end pipeline reconciliation | `PRD-ARF-013` | §16 | produced→persisted accounting |
| `REQ-ARF-014` | Bounded backpressure/replay | `PRD-ARF-014` | §13 | overload/recovery tests |
| `REQ-ARF-015` | Collector failure isolation | `PRD-ARF-015` | §14 | collector timeout/crash tests |
| `REQ-ARF-016` | Monitoring-path state distinct from HOST_DOWN | `PRD-ARF-016` | §15 | alert-evidence negative tests |
| `REQ-ARF-017` | Bounded Agent resource/storage behavior | `PRD-ARF-017` | §§17–21 | benchmark/resource evidence |
| `REQ-ARF-018` | Failure-injection and platform certification gate | `PRD-ARF-018` | §§22–23 | complete certification evidence |

---

## 28. Definition of done

A change governed by this master is not complete until applicable:

- code;
- unit/contract/integration tests;
- failure-injection evidence;
- observability and stable error/reason codes;
- tenant/security negative tests;
- install/reboot/upgrade/rollback behavior;
- desired/applied-state evidence;
- replay/idempotency evidence;
- pipeline-integrity evidence;
- CPU/RAM/disk/network benchmark evidence where resource claims are affected;
- Mother Reference/decision/traceability updates where a contract changes;
- exact-head independent review;

are complete.

Specification is not implementation. Unit tests are not production certification. Green process health is not telemetry-integrity proof.

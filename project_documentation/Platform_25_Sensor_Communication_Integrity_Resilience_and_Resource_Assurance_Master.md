# Platform — Sensor Communication Integrity, Resilience & Resource Assurance Master

**Document ID:** PLATFORM-SCR-001  
**Version:** 3.2.2  
**Status:** **Authoritative Specialized Mother Reference — Sensor Communication Integrity / Resilience / Resource Assurance**  
**Product:** Platform  
**Effective date:** 2026-09-19  
**Scope:** Network / Infrastructure / Streaming-Event / Combined Sensors, Sensor-to-Device/source and Sensor-to-Core communication  
**Change classification:** Additive reliability hardening; no product-version increment; no Sensor rewrite authorization

---

## 1. Authority and purpose

This document is the authoritative specialized Mother Reference for communication correctness and resilience across the path:

~~~text
Device / source
<-> Sensor collection/receiver
-> Sensor durable spool
-> Core ingestion
-> Kafka/consumers
-> persistent telemetry
-> qualification / alert / RCA evidence
~~~

It supplements, and does not silently replace, existing Platform authority including REQ-S-001..016, REQ-NET-001..015, REQ-ALT-003..010, REQ-DATA-008, REQ-DATA-013, REQ-DATA-015, the Sensor contracts in Platforms 03–18, and monitoring-evidence/semantic-state requirements in Platform_22.

The governing objective is:

> Platform shall remain truthful when communication fails. A certified communication failure must not silently destroy telemetry inside Platform durability boundaries, create duplicate authoritative ownership, trigger uncontrolled retry amplification, starve current monitoring behind replay, or convert a monitoring-path failure into an unsupported device/service-down conclusion.

Platform does not promise physical zero packet loss for source protocols/networks it does not control.

The stronger contract is:

> Zero unexplained telemetry loss within certified Platform durability boundaries; explicit visibility of unavoidable source/network/platform loss; deterministic replay/idempotency; and no incorrect resource-state conclusion solely because monitoring communication failed.

---

## 2. Preserve the existing Sensor architecture

The following architecture remains authoritative unless a benchmarked engineering review proves otherwise:

- Go Universal Sensor Runtime;
- headless/lightweight Sensor;
- bounded execution-class worker pools;
- no full historical database on Sensor;
- Core-authoritative policy/assignment;
- mTLS Sensor-to-Core;
- durable local Sensor spool;
- sequence/replay/idempotency model;
- MWU-based capacity and Sensor Pools;
- locality/reachability/capacity/security-aware placement;
- adaptive polling;
- multi-protocol/multi-vantage validation;
- explicit UNKNOWN / INCONCLUSIVE / monitoring-degraded semantics;
- failed primary poll is not automatically device-down;
- mandatory 80% Sensor storage protection;
- Pipeline Integrity;
- Platform self-monitoring;
- horizontal Sensor scale;
- dedicated Streaming/Event Sensor only when measured workload requires role separation.

### 2.1 Explicit non-goals

Do not add merely for communication reliability:

- Sensor PostgreSQL;
- Sensor ClickHouse;
- Sensor Kafka;
- Sensor Valkey;
- heavyweight embedded database;
- JVM Sensor process;
- Python runtime dependency for normal Sensor collection;
- separate queue/reliability microservice;
- always-on full duplicate polling;
- always-on duplicate telemetry upload;
- always-on packet capture;
- mandatory dedicated Streaming Sensor for small workloads;
- mandatory N+1 Sensor deployment for customers/policies that do not require Sensor HA.

Prefer compact in-process state, bounded worker pools, append-only durable storage, counters/watermarks, smarter scheduling and failure-aware policy.

---

## 3. Existing requirements are preserved, not duplicated

The following were already authoritative before this supplement:

| Existing capability | Existing authority | This master |
|---|---|---|
| Durable Sensor spool / store-and-forward | REQ-S-007, HLD/LLD | Preserve; strengthen evidence only |
| Bounded worker pools | Sensor Architecture/HLD | Preserve |
| Retry/backoff/jitter | Sensor LLD/runtime contracts | Preserve |
| Sequence/replay/idempotency | REQ-DATA-013, LLD §7 | Preserve |
| Pipeline Integrity | REQ-DATA-015, HLD §34.7/HLD-X1, LLD §69, DF-070 | Preserve; certification strengthened |
| 80% Sensor protection | REQ-S-008, REQ-DATA-008 | Preserve |
| MWU/capacity placement | REQ-S-003, Platform_11 | Preserve |
| Sensor pools / HA capacity models | REQ-S-005 | Preserve |
| Alert qualification / failed SNMP is not DOWN | REQ-ALT-004/005 | Preserve |
| Telemetry quality / semantic degraded states | Platform_05 / Platform_22 | Preserve |
| Core durable ACK boundary | LLD §7 + existing P0 | Remains P0; do not guess |

This master adds only the under-specified or genuinely missing communication-reliability contracts below.

---

## 4. Additive product requirements

### PRD-SCR-001 — Partition-safe Sensor ownership

Sensor ownership/leases/fencing shall explicitly balance temporary disconnected monitoring continuity against stale duplicate ownership. A network partition shall not silently permit indefinite conflicting polling.

### PRD-SCR-002 — Push-protocol HA profiles

Where a monitored source sends telemetry to a collector destination, Platform shall expose supported redundancy patterns appropriate to source capability and customer criticality. Sensor reassignment alone shall not be represented as push-telemetry HA.

### PRD-SCR-003 — Target-specific fallback readiness evidence

For HA-critical pull monitoring, failover eligibility shall consider target-specific path/protocol/credential/security compatibility, not only backup Sensor process/resource health.

### PRD-SCR-004 — Poll de-synchronization

Periodic collection shall distribute work deterministically across the monitoring interval, with bounded jitter where appropriate, so common schedule boundaries do not create avoidable Sensor/device/Core bursts.

### PRD-SCR-005 — Correlated failure-domain retry control

During strong evidence of a shared failure domain, Platform shall bound retry amplification while preserving enough validation to detect recovery. Correlation shall not suppress independent failures without sufficient evidence.

### PRD-SCR-006 — Control/current/replay service protection

Control-plane communication and current operational telemetry shall remain responsive during large replay backlogs. Replay shall progress fairly but may not monopolize Sensor-to-Core transport/resource budgets.

### PRD-SCR-007 — Bounded benchmark-driven uploader control

Sensor uploading shall remain bounded by record count and encoded bytes and shall additionally account for latency/freshness through a maximum batch-age/fairness contract. Any adaptive sizing/compression behavior shall be benchmark-driven rather than arbitrary.

### PRD-SCR-008 — Protocol-specific continuity and loss evidence

Platform shall expose protocol-specific transport/session/authentication/gap/drop evidence sufficient to distinguish monitoring-path degradation from monitored-resource failure.

### PRD-SCR-009 — Buffer horizon as an operational capacity signal

Sensor storage health shall expose estimated remaining buffering/outage survivability from recent workload and usable spool capacity, with sustained/forecast evidence rather than transient-burst-only admission decisions.

### PRD-SCR-010 — Sensor communication resource certification

Every certified Sensor release/profile shall quantify communication reliability overhead, retry amplification, backlog recovery behavior and resource efficiency under healthy and failure scenarios. Reliability features shall not silently raise the minimum Sensor hardware profile.

---

## 5. Existing P0 — durable Core ACK boundary remains unresolved

The existing LLD contract remains authoritative:

~~~text
Sensor durable spool
-> Core validates
-> Core durable ingestion path
-> accepted_through_sequence
-> Sensor durable ACK watermark
-> Sensor may remove acknowledged records
~~~

The final meaning of ACK_DURABLE is still blocked on the certified two-Core Kafka/ClickHouse/PostgreSQL/fencing design.

Engineering must answer experimentally:

> If Core-A returns an ACK for Sensor sequence N and Core-A fails immediately afterward, under what proven state is sequence N recoverable after supported Core failover?

Until that P0 is resolved:

- do not declare Kafka alone the durability boundary;
- do not silently introduce a third full Core;
- do not weaken Sensor replay retention;
- do not claim zero-loss HA;
- do not make a durability claim stronger than tested evidence.

This supplement creates no second ACK mechanism.

---

## 6. Partition-safe Sensor ownership, leases and fencing

The existing monotonic assignment/fencing version remains the primary ownership fence.

The final design shall define:

- assignment identity/version;
- Sensor identity;
- target/entity identity;
- issue/renewal timing;
- disconnected grace behavior where allowed;
- stale-owner rejection/revocation behavior;
- reassignment hold-off;
- failover start condition;
- reconnect reconciliation;
- failback/hysteresis.

Additional lease IDs, epochs or signed fencing tokens are introduced only if required by the proven failure model.

A short hard lease that stops all polling immediately on Core loss is not acceptable if it defeats the existing requirement that Sensors continue monitoring during temporary Core/WAN outages. Unlimited disconnected autonomy is also not acceptable where Core may legitimately reassign the target.

Therefore the certified design shall establish a bounded, policy-aware disconnected ownership model. Exact durations are benchmark/operational-policy decisions, not guessed constants in this document.

Minimum partition test:

~~~text
Sensor-A owns Target-X
A loses Core connectivity
A still reaches X
Core can reach Sensor-B
~~~

Evidence must establish whether/for how long A continues, when Core may reassign, when B may start, whether A is fenced from newer work, what happens when A reconnects, and whether duplicate polling occurred and how it was bounded/explained.

---

## 7. Push-protocol HA

Push sources include, as applicable:

- SNMP traps/informs;
- Syslog;
- NetFlow/IPFIX/sFlow/J-Flow;
- vendor event streams/webhooks;
- other source-initiated streams.

The critical distinction is:

> Reassigning pull work to Sensor-B does not redirect a source that is still configured to send to Sensor-A.

Platform may support one or more deployment-specific patterns:

- stable collector VIP/FQDN implemented by a supported HA mechanism;
- source/vendor-supported multiple destinations;
- dual collectors where protocol/source semantics permit;
- another certified receiver redundancy mechanism.

Anycast is not selected by declaration.

Deployment policy:

- Standalone/small deployment may use one collector with documented standalone availability.
- Enterprise/critical deployments may enable redundant receiver paths.
- Stronger push HA is required only where customer policy/criticality and source capability justify it.
- If the source technology cannot guarantee delivery, Platform exposes the limitation rather than claiming completeness.

Where redundant push delivery can duplicate the same source event, canonical identity/deduplication shall prevent duplicate alerts/incidents and incorrect counting where protocol semantics permit reliable duplicate identification.

---

## 8. Fallback Sensor readiness

For a specific target, HA readiness is stronger than “backup Sensor is healthy.”

For critical pull monitoring, readiness evidence may include:

~~~text
capacity
+ network/path reachability
+ protocol endpoint reachability
+ credential usability
+ security-zone eligibility
+ Sensor role/capability/version compatibility
+ assignment/fencing eligibility
~~~

Backup readiness testing shall be policy/criticality driven and may use route/path evidence, ICMP where appropriate, TCP connect, TLS handshake, bounded SNMP/API authentication probe, and cached recent validation.

Do not run a duplicate full monitoring profile merely to prove standby readiness.

Validation shall be staggered, rate/resource bounded, cached for a defined evidence age, and invalidated/revalidated on relevant configuration/network/credential changes. The added resource cost must be benchmarked.

---

## 9. Poll de-synchronization

Periodic jobs shall not naturally converge on wall-clock boundaries such as every device firing at :00.

The scheduler shall support deterministic spreading within the requested collection window. The spread should be stable enough to avoid random churn while bounded jitter may be applied to retry/recovery paths.

Invariant:

> A five-minute policy remains approximately a five-minute policy; de-synchronization changes execution phase, not the requested monitoring frequency.

Certification measures peak vs average poll operations/sec, Sensor CPU/network peaks, device-management-plane request burst, spool/upload burst, and effect on freshness/SLA semantics.

---

## 10. Correlated failure-domain retry control

Per-job bounded retries remain required. In addition, the scheduler/qualification layer shall support shared-failure protection where strong evidence identifies a common domain, for example:

- Sensor/network interface;
- gateway/path;
- site/WAN/circuit;
- subnet/VLAN where authoritative evidence exists;
- credential profile;
- protocol/provider endpoint.

Candidate behavior:

~~~text
isolated failure
-> normal bounded retry

large correlated same-domain failure
-> path/dependency qualification
-> reduce/suppress redundant retries
-> retain bounded recovery probes
-> restore normal cadence on recovery
~~~

Topology proximity or simultaneous timeout alone is not sufficient to suppress unrelated monitoring.

This mechanism shall reduce retry amplification during incidents and must itself be resource bounded.

---

## 11. Control/current/replay priority and fairness

The Sensor shall protect the following logical service order:

~~~text
P0 identity / PKI / fencing
P1 heartbeat / assignment / lease / control
P2 critical current events/state
P3 current telemetry
P4 recent buffered telemetry
P5 old replay backlog
~~~

The exact physical implementation may use separate control path, priority classes, weighted queue scheduling, uploader budget allocation, or bounded replay concurrency/rate. It does not require separate services.

Fairness rules:

- Control/heartbeat/fencing must not wait behind bulk replay.
- Current telemetry must have bounded latency while old backlog exists.
- Old replay must eventually progress; it cannot starve indefinitely.
- Ordering constraints that are semantically required must be preserved.
- Priority must not invalidate sequence/deduplication correctness.

---

## 12. Bounded uploader and batching contract

Existing record-count, payload-byte and request-byte limits remain.

The uploader shall additionally expose/consider:

- maximum time current data waits for a batch;
- Core response latency/errors/backpressure;
- queue depth and oldest age;
- incoming rate;
- Sensor CPU and bandwidth where measurable;
- replay share/budget.

Adaptive behavior is optional until benchmarks prove it improves the certified envelope.

Avoid both one observation per request and unbounded giant batches. Compression and dynamic batch sizing remain benchmark-driven.

---

## 13. Buffer horizon and storage forecasting

Sensor health shall expose, where recent rate evidence is sufficient:

- spool bytes/records;
- usable bytes before protection action;
- observed spool growth rate;
- oldest unacknowledged age;
- projected warning/protection time;
- estimated remaining buffer duration;
- replay drain rate and estimated completion time when recovering;
- confidence/data sufficiency for the estimate;
- current loss-risk/protection state.

Conceptually:

~~~text
buffer_horizon
=
usable_remaining_spool_capacity
/
sustained_recent_spool_growth_rate
~~~

Short bursts shall not by themselves cause permanent placement rejection. Placement/admission may use sustained buffer-horizon risk as one input alongside MWU/CPU/RAM/disk/HA reserve.

---

## 14. Protocol-specific continuity evidence

A generic PROTOCOL_HEALTHY flag is insufficient for diagnosis and alert qualification.

### 14.1 SNMP

Distinguish where determinable: transport/network timeout, authentication, privacy/security negotiation, authorization/access, protocol error, tooBig, partial/malformed response, OID/object availability, and bulk compatibility/resource limit.

### 14.2 gNMI/streaming

Track subscription/session state, subscription/session epoch or equivalent continuity identity, last event time, last receive time, reconnect count/reason, source sequence/gap evidence where available, and backpressure/drop evidence.

A reconnect with unprovable continuity shall surface a telemetry-gap/quality condition rather than silently imply complete coverage.

### 14.3 Flow

Track where available: exporter sequence/gap, template state/age, template misses, receiver/kernel drops, decode drops/errors, and queue/rate-limit drops.

### 14.4 Syslog

Track TCP/TLS connection/session state for connection-oriented transports, UDP application queue drops, kernel/socket drops where the OS/runtime can provide reliable evidence, parser/malformed drops, oversized/rate-limit drops, and durable-sink failures.

### 14.5 REST/API

Distinguish transport/DNS/TLS, authentication/authorization, token refresh, 429/rate limit, server failure, timeout, schema/mapping change, and partial/semantic response failure.

### 14.6 SSH/NETCONF

Distinguish transport, host-key change/validation, authentication, privilege/authorization, session limit, capability change, and command/parser result failure.

These are monitoring evidence states; they are not automatically monitored-resource-down states.

---

## 15. UDP/source-loss visibility

Platform cannot guarantee delivery of unacknowledged source datagrams before receipt.

For UDP Syslog, ordinary traps, many Flow exporters and similar sources:

- do not claim end-to-end guaranteed source delivery;
- expose Platform-observable drops/gaps;
- preserve application-level drop counters;
- add kernel/socket drop evidence where technically reliable;
- use exporter/source sequence gaps where available;
- distinguish template/decoder/rate-limit/queue/storage drops.

Deployment guidance should prefer, where supported and justified, Syslog TCP/TLS over UDP, SNMP INFORM over unacknowledged trap, and multiple push destinations/redundant collectors for critical sources.

Compatibility with UDP remains mandatory where required.

---

## 16. Alert correctness remains non-negotiable

Existing REQ-ALT-004/005 remains authoritative.

Conceptually:

~~~text
HEALTHY
-> PRIMARY_MONITOR_FAILURE
-> SUSPECT
-> VALIDATION
-> QUALIFIED_STATE
~~~

Valid outcomes remain distinguishable, including UP, DEGRADED, UNREACHABLE, DEVICE_DOWN, PATH_FAILURE, CREDENTIAL_FAILURE, MONITORING_FAILURE, SENSOR_FAILURE and UNKNOWN / INCONCLUSIVE.

A communication failure is evidence about the monitoring path. It is not sufficient alone to assert DEVICE_DOWN.

---

## 17. Pipeline Integrity is not duplicated

Existing REQ-DATA-015, HLD §34.7/HLD-X1, LLD §69 and DF-070 remain authoritative.

At suitable stream granularity Platform reconciles compact watermarks/counters such as SENSOR_PRODUCED, SENSOR_ACKED, CORE_ACCEPTED, KAFKA_PRODUCED, KAFKA_CONSUMED, STORAGE_PERSISTED, DUPLICATE, REJECTED and DROPPED_WITH_REASON.

Do not persist redundant copies of every observation solely for reconciliation when compact counters/watermarks provide equivalent evidence.

Any unexplained delta is a Platform internal integrity condition.

---

## 18. Sensor resource-efficiency certification

Do not raise existing Sensor planning targets merely to implement this master without measured evidence.

Certification shall record, by workload profile as applicable:

- CPU per device/MWU;
- RAM per device/MWU;
- poll operations/sec/core;
- metrics/sec/core;
- Flow records/sec/core;
- Syslog events/sec/core;
- Agent ingestion/sec/core;
- network bytes/observation;
- disk bytes written/observation;
- spool write amplification;
- spool compression ratio where used;
- upload/replay throughput;
- retry amplification;
- goroutine/thread count;
- open file descriptors;
- socket count;
- Go GC overhead;
- queue depth/saturation;
- current-telemetry latency during replay.

Test states include healthy baseline, WAN/Core loss, site/shared-path failure, credential failure, Core recovery/replay, Sensor overload, Sensor failover/reassignment, mass target timeout, Flow/Syslog burst, and storage pressure.

Reliability controls fail certification if they materially degrade the monitored environment or silently require a larger Sensor tier without controlled change.

---

## 19. Cost-aware deployment profiles

### Standalone / small

~~~text
one Sensor
no Sensor-HA guarantee
durable spool
normal monitoring
documented standalone receiver availability
~~~

### Enterprise HA

~~~text
Sensor Pool
N+1 / N+K / 1:1 as policy/capacity requires
capacity reserve
bounded failover/replay
~~~

### Critical site / critical push telemetry

~~~text
verified fallback pull path
receiver redundancy where source permits
independent failure domains where justified
multi-vantage validation according to policy
~~~

Maximum redundancy is not mandatory for every customer.

---

## 20. Mandatory communication failure certification

The base Platform_14 fault-injection requirements remain. Sensor communication certification additionally covers, as applicable:

Device/source path:
- single/burst packet loss;
- 5/10/20% loss;
- latency/jitter;
- duplicate/out-of-order responses;
- DNS/route/ACL/port/MTU failure;
- SNMP timeout/auth/privacy/protocol/tooBig/partial/malformed;
- REST 401/403/429/5xx/timeout/token/schema/partial;
- SSH timeout/host-key/auth/session-limit;
- gNMI disconnect/reconnect/reboot/gap;
- trap/Syslog/Flow bursts;
- template miss;
- UDP receiver saturation.

Sensor:
- process restart/reboot;
- CPU/RAM/FD/socket pressure;
- disk pressure/read-only filesystem;
- spool corruption;
- worker-pool saturation;
- timeout/retry storm;
- primary Sensor loss;
- fallback unavailable/unreachable/firewall blocked;
- stale ownership;
- reassignment race;
- large replay backlog.

Core path:
- Core unreachable/stable endpoint failure;
- TLS/certificate failure/rotation;
- Kafka unavailable/lag;
- ClickHouse unavailable;
- Core failover;
- ACK loss;
- duplicate/out-of-order replay;
- backlog recovery.

Every case records whether safe polling/receiving continued, current-monitoring latency, telemetry loss and exact/best-known explanation, deduplication correctness, retry amplification, Sensor CPU/RAM/disk/network, whether an incorrect DOWN state occurred, actual failure-domain evidence, automatic recovery, and audit/operational visibility.

---

## 21. Release-blocking communication acceptance

A Sensor communication profile is not production-certified until applicable evidence proves:

1. Single primary poll failure does not directly create false device-down.
2. Shared failures do not create uncontrolled retry storms.
3. Sensor overload does not silently discard Platform-accepted telemetry.
4. Core outage does not stop safe collection while certified buffering capacity remains.
5. Replay does not starve control or current telemetry.
6. Sensor spool deletion obeys the certified durable ACK boundary.
7. Duplicate replay does not duplicate alerts/incidents or distort replay-sensitive metrics.
8. HA pull failover is fenced and deterministic for the certified partition scenarios.
9. Critical fallback eligibility includes actual target-specific readiness evidence according to policy.
10. Push-protocol HA/limitations are documented and tested for the supported deployment pattern.
11. Streaming reconnect/gap uncertainty is explicit.
12. UDP/source loss is not represented as guaranteed completeness.
13. Storage pressure/forced loss is measured, audited and surfaced.
14. Resource use remains within the certified Sensor profile.
15. No heavyweight Sensor dependency was introduced merely to solve coordination.
16. Poll distribution and correlated retry behavior remain bounded at scale.

---

## 22. Implementation-status reconciliation requirement

Before implementation, each item shall be classified against exact current source as:

~~~text
ALREADY REQUIRED AND IMPLEMENTED
ALREADY REQUIRED BUT NOT IMPLEMENTED
IMPLEMENTED BUT UNDER-SPECIFIED
DOCUMENTED ONLY
PARTIALLY IMPLEMENTED
ACTUAL ARCHITECTURE GAP
TEST/CERTIFICATION GAP ONLY
CONFLICTING
UNNECESSARY
REQUIRES P0 SPIKE
~~~

Specification is not implementation.

Known high-level implementation priorities at adoption are:

- preserve and extend current durable spool/uploader/scheduler rather than replace them;
- resolve the existing durable Core ACK P0 through the two-Core HA spike;
- complete authoritative Sensor target assignment/partition behavior before claiming HA ownership certification;
- implement push HA only through supported deployment/source patterns;
- add failure/resource tests before raising readiness.

Current-source observations are evidence for planning, not permanent normative statements.

---

## 23. Minimal implementation workstreams

Recommended bounded tranches:

~~~text
SCR1  Current Sensor/Core communication reconciliation
SCR2  Core durable ACK P0 spike/certification
SCR3  Sensor ownership partition/fencing contract
SCR4  Push-protocol HA deployment contracts
SCR5  Fallback readiness evidence
SCR6  Poll de-synchronization
SCR7  Correlated retry amplification control
SCR8  Current-vs-replay fairness / uploader batch-age control
SCR9  Buffer-horizon forecasting
SCR10 Protocol continuity / UDP loss telemetry
SCR11 Communication failure-injection suite
SCR12 Sensor resource-efficiency certification
~~~

The sequence may change by dependency evidence. Do not rewrite the Sensor.

---

## 24. Formal traceability supplement

| REQ ID | Requirement | Governing PRD | Primary sections | Mandatory evidence |
|---|---|---|---|---|
| REQ-SCR-001 | Partition-safe ownership balances continuity and stale-owner fencing | PRD-SCR-001 | §6 | partition/reassignment/failback tests |
| REQ-SCR-002 | Push-protocol HA profiles and explicit limitations | PRD-SCR-002 | §7 | receiver failover/source-pattern tests |
| REQ-SCR-003 | Target-specific fallback readiness evidence | PRD-SCR-003 | §8 | target reachability/credential/capacity tests |
| REQ-SCR-004 | Deterministic poll spreading / bounded jitter | PRD-SCR-004 | §9 | peak-vs-average poll/resource benchmark |
| REQ-SCR-005 | Strong-evidence correlated retry suppression with recovery probes | PRD-SCR-005 | §10 | shared-failure amplification tests |
| REQ-SCR-006 | Control/current telemetry protected from replay starvation | PRD-SCR-006 | §11 | large-backlog fairness tests |
| REQ-SCR-007 | Bounded batch age and benchmark-driven uploader adaptation | PRD-SCR-007 | §12 | latency/throughput/resource benchmark |
| REQ-SCR-008 | Protocol-specific continuity/gap/drop evidence | PRD-SCR-008 | §§14–15 | protocol negative/burst/drop tests |
| REQ-SCR-009 | Buffer-horizon forecast and capacity visibility | PRD-SCR-009 | §13 | outage/growth/forecast accuracy tests |
| REQ-SCR-010 | Sensor communication resource certification | PRD-SCR-010 | §§18–21 | certified resource/failure evidence |

Existing requirements remain governing for spool durability, Pipeline Integrity, data replay/idempotency, 80% storage protection, capacity placement and alert correctness.

---

## 25. Definition of done

A change governed by this master is not complete until applicable evidence covers:

- source-truth reconciliation;
- deterministic unit/component/contract tests;
- real protocol/integration tests where appropriate;
- partition/fencing tests;
- replay/idempotency;
- push-receiver behavior where applicable;
- protocol-specific error/drop evidence;
- false-alert negative tests;
- storage pressure/loss accounting;
- failure injection;
- CPU/RAM/disk/network/FD/socket/goroutine measurements;
- retry amplification and replay fairness;
- exact-head independent review;
- updated decision/traceability authority if the contract changes.

Specified is not implemented. Implemented is not tested. Tested is not certified. Healthy processes do not prove telemetry integrity.

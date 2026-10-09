# Platform — Sizing, Capacity, Sensor Distribution & Storage Master

**Document ID:** PLATFORM-SIZING-001  
**Version:** 3.3.0
**Status:** **Authoritative Mother Reference — Engineering Sizing Model**  
**Product:** Platform

## 1. Governing rule

Device count is a commercial/planning input, not the only engineering load measure. Platform must size and operate using measured workload: poll operations/sec, metrics/sec, interfaces/entities, polling interval, protocol cost, RTT/timeouts, Flow/log rates, streaming subscriptions, NCM jobs, synthetic tests, **interactive/on-demand diagnostics and CLI sessions**, concurrent users, query/report/RCA demand and retention.

All numbers below are **engineering planning targets until benchmark-certified**. They are not contractual scale claims merely because incumbent products publish similar densities.

## 2. Core compute targets

### 2.1 Non-HA

| Monitored network devices | Core count | Engineering target per Core |
|---:|---:|---|
| 1,000 | 1 | **8 vCPU / 32 GB RAM** |
| 2,000 | 1 | **8 vCPU / 32 GB RAM** |
| up to 2,500 | 1 | **8 vCPU / 32 GB RAM mandatory optimization target under SNWP-2500-v1** |
| 5,000 | 1 | **12 vCPU / 48 GB RAM planning target** |
| 10,000 | 1 | **16 vCPU / 64 GB RAM planning target** |
| 15,000 | 1 | **24 vCPU / 96 GB RAM planning target** |

### 2.2 HA

HA uses **exactly two full Core nodes**, one Active and one Standby. Each full Core is sized to carry the certified workload after failover. There is no third customer-visible Core/governance node.

| Devices | Full Core nodes | Per-Core target |
|---:|---:|---|
| 1,000 | 2 | 8 vCPU / 32 GB |
| 2,000 | 2 | 8 vCPU / 32 GB |
| up to 2,500 | 2 | 8 vCPU / 32 GB |
| 5,000 | 2 | 12 vCPU / 48 GB |
| 10,000 | 2 | 16 vCPU / 64 GB |
| 15,000 | 2 | 24 vCPU / 96 GB |

The exact two-Core synchronization/fencing mechanisms for Kafka/ClickHouse/PostgreSQL are P0 engineering spikes. Technology must adapt to the fixed product requirement; no third full Core may be introduced silently.

### 2.3 Local AI / ML / LLM compute envelope — v3.2.2

Core-resident local intelligence is an **optional certified execution profile** within the planned Core footprint and shall **not increase the minimum certified Core sizing tiers**. Standard Platform operation is CPU-first and shall not require a GPU. AI/ML/LLM workloads are subordinate to collection, ingestion, storage durability, alert qualification, incident processing and deterministic RCA. A deployment that requires stronger compute isolation may use an optional dedicated local/private inference provider through AI Gateway; that provider is sized/certified separately and is not a Core/HA node.

Planning envelopes below are hard design targets until benchmark certification:

| Per-Core target | Core-resident local-intelligence peak budget (if enabled) | Core-resident LLM class (if enabled) | Concurrent local LLM requests | Notes |
|---|---:|---|---:|---|
| **8 vCPU / 32 GB** | **≤2 vCPU burst / ≤3 GB RAM peak** | **0.5B–1.5B parameter, 4-bit quantized** | **1** | Standard smallest Core. A model above 1.5B is not assumed; up to ~3B may be admitted only if SNWP certification proves no monitoring regression. |
| **12 vCPU / 48 GB** | **≤3 vCPU burst / ≤4 GB RAM peak** | **1B–3B, 4-bit quantized** | **1–2** | Extra capacity primarily buys monitoring/user concurrency, not permission for an unnecessarily large model. |
| **16 vCPU / 64 GB** | **≤3 vCPU burst / ≤5 GB RAM peak** | **1B–3B, 4-bit quantized** | **2** | AI remains lower priority than live monitoring and storage. |
| **24 vCPU / 96 GB** | **≤4 vCPU burst / ≤6 GB RAM peak** | **1B–3B, 4-bit quantized** | **2–4** | Higher concurrency is preferred over increasing model size. |

When Core-resident local intelligence is enabled on the 8 vCPU / 32 GB tier, the normal non-interactive target is **<1 vCPU average and approximately 1–1.5 GB RAM**, with the larger peak envelope used only during bounded inference/analytics activity. The base product does not require Core-resident LLM inference.

Mandatory resource rules:

- **No mandatory GPU/NPU.** Optional accelerator support may be certified later, but no GPU may become a prerequisite for standard Core monitoring, alerting or deterministic RCA.
- **No foundation-model training/fine-tuning on customer production Core.** Platform ships/loads approved pre-trained quantized local models. Customer-specific intelligence comes from evidence retrieval, tools, topology, Similar Incident Memory and policy, not foundation-model fine-tuning.
- Lightweight statistical/ML baseline recalibration is permitted, but it is scheduled, bounded, interruptible and lower priority than live monitoring.
- Local LLM inference is **on demand**. On the 8/32 tier the model should be unloaded or memory-mapped with minimal resident state when idle and released after a bounded idle period unless benchmarked headroom permits keeping it warm.
- Small-Core local LLM operating context should normally remain **2K–4K tokens**; Platform retrieval/context builders reduce large telemetry sets to bounded structured evidence before inference. Raw high-volume telemetry is not streamed wholesale into an LLM.
- GNN inference operates on bounded incident-relevant temporal subgraphs, not the full enterprise graph for every observation.
- AI resource limits are enforced by a Resource Governor/circuit breaker. Recommended degradation order under resource pressure is: postpone model recalibration/training → reduce/queue LLM concurrency → defer optional GNN/embedding work → unload/suspend local LLM. Deterministic monitoring, alert qualification and RCA continue.
- Resource protection inputs include Core CPU/memory/swap pressure, Kafka lag, ClickHouse/ingestion backlog, query latency, incident storms and storage pressure.
- Service/process controls shall enforce CPU quota, memory limit, bounded queue/concurrency, inference timeout and lower CPU/I/O priority for AI/analytics workers relative to protected Core functions.
- Local model/runtime artifacts are offline-packaged, signed/checksummed, versioned and governed by the Platform model registry. Planning allowance for the standard local model/ML/GNN artifacts and bounded runtime cache is approximately **3–6 GB disk**; exact packaged size is release-measured.

Resource priority is:

```text
Priority 1  collection / ingestion / PostgreSQL / ClickHouse / Kafka / Sensor communication
Priority 2  alert qualification / incident correlation / deterministic RCA
Priority 3  lightweight ML and bounded GNN inference
Priority 4  local LLM interactive assistance
Priority 5  model recalibration / training / challenger evaluation
```

A Core release fails resource certification if enabling the supported local-intelligence profile causes the certified monitoring workload to miss ingestion, alerting, query, durability, failover or storage-protection objectives.

## 3. Network Sensor planning

Current safe planning baseline retained from prior deployments:

- **4 vCPU / 16 GB RAM / 150 GB SSD ≈ 800 normal network devices** under representative SNMP/ICMP workload and reasonable RTT.
- **8 vCPU / 16 GB RAM ≈ 2,000–2,500 normal network devices** is an engineering benchmark objective, not yet a product guarantee.

### 3.1 Planning count at current 800-device baseline

| Network devices | Active Sensors | With one N+1 Sensor Pool |
|---:|---:|---:|
| 1,000 | 2 | 3 |
| 2,000 | 3 | 4 |
| 5,000 | 7 | 8 |
| 10,000 | 13 | 14 |
| 15,000 | 19 | 20 |

If devices are divided into multiple network-isolated/locality pools, redundancy is applied per pool. Therefore final Sensor count is distribution-dependent rather than one global `active + 1`.

## 4. Infrastructure Sensor planning

- Agentless SSH/WMI/WinRM planning target: approximately **800 endpoints per 4 vCPU / 16 GB Sensor**.
- Agent-push ingestion planning target: approximately **2,200 endpoints per 4 vCPU / 16 GB ingestion capacity unit**.
- These are separate workload classes and are not added to network-device counts as if they had identical cost.

## 5. Streaming/Event Sensor

High-volume NetFlow/IPFIX/sFlow/J-Flow, Syslog, traps and high-rate OTel are separated from polling when volume risks starving normal collection. A small deployment may combine roles, but Platform's Capacity Engine must recommend/require role separation once safe limits are approached. Exact records/sec and EPS certification values are benchmark-defined.

## 6. Monitoring Work Units (MWU)

Platform must compute a workload score using at least:

- protocol type and operation cost;
- metrics/OIDs/entities/interface count;
- poll/subscription interval;
- SNMP GET/BULK plan and timeout/retry behavior;
- SNMP scalar/table mix, average/max OIDs per poll, GETBULK repetitions/response bytes and fallback frequency;
- SNMPv3 security CPU cost and session/engine-discovery behavior;
- Counter32/Counter64 raw + derived-rate processing cost;
- trap/inform EPS, notification size, decode/MIB cost and storm-control rate;
- adaptive-poll target count, accelerated interval, duration/cooldown and worst-case burst;
- MIB/Capability Pack mapping cost and interface/entity cardinality;
- RTT, packet loss and timeout rate;
- gNMI stream rate;
- REST calls and response size;
- Flow records/sec and template rate;
- Syslog/events/sec;
- agent/agentless collection cost;
- NCM/CLI scheduled/concurrent jobs;
- interactive/on-demand diagnostic jobs, multi-vantage fan-out, bounded continuous tests and concurrent CLI sessions;
- discovery/adaptive-polling burst;
- local spool/backlog and CPU/RAM/disk headroom.

UI shows both simple device count and calculated workload/capacity. Placement never assigns a device to the lowest-RTT Sensor if doing so would overload it.

### 6.1 Interactive diagnostics and CLI capacity reserve

Interactive troubleshooting is an explicit workload class, not free capacity. Routine scheduled monitoring, ingestion durability and recovery work retain priority over ad-hoc operator diagnostics.

- Sensors/Agents maintain bounded concurrency/worker budgets for on-demand Ping/traceroute/SNMP/TCP/DNS/SSH/API/domain tests and browser CLI.
- Multi-vantage execution consumes capacity independently at every selected vantage; fan-out is limited by tenant policy, action policy and current headroom.
- Bounded continuous tests consume an ongoing interactive budget and must have maximum duration plus cancellation.
- When interactive capacity is exhausted, Platform queues, limits or rejects diagnostics with an explicit reason rather than starving routine monitoring.
- Sensor/vantage eligibility for an on-demand action considers current MWU, interactive-job load, CPU/RAM, queue/backlog, role, locality, security boundary and maintenance/drain state.
- UI capacity/health surfaces expose active/queued diagnostic jobs, interactive utilization/rejections and whether a requested vantage is unavailable because of capacity or policy.
- No fixed reserve percentage is invented here; release certification establishes safe concurrency for each certified Sensor profile.

## 7. Sensor Placement Engine

Eligibility considers: tenant/security boundary, protocol reachability, preferred/allowed/forbidden pools, site/region locality, measured RTT/loss, Sensor role, CPU/RAM, current MWU, queue/backlog, local disk, HA reserve and maintenance/drain state.

Customer modes: **Automatic (recommended)**, **Automatic with Approval**, **Manual**, **Pool Based**. Rebalancing uses hysteresis to avoid thrashing.

## 8. Standard Network Workload Profile SNWP-2500-v1

The 8 vCPU/32 GB Core target is certified only against a versioned workload profile containing 2,500 representative enterprise network devices; ICMP; meaningful SNMPv2c/v3 mix; interface inventory/performance; CPU/memory/uptime/environment; traps; topology; alert evaluation; representative dashboards/reports/incidents/RCA; and Flow/syslog below dedicated-streaming threshold. Exact interface/metric/event distributions are committed in the performance repository before certification, not invented in product documentation.

**CORE_RESOURCE_FEASIBILITY_GATE_1** (governance checkpoint after BUILD-060–065 are runnable enough for representative aggregate measurements; before BUILD-066): FEASIBILITY CHECKPOINT, **not** capacity certification. Measures Core JVM CPU/RAM, heap, PostgreSQL, Kafka, ClickHouse, Valkey, OS/filesystem/cache reserve, disk/IO, ingestion lag/backlog, protected workload latency, recovery reserve and dominant bottleneck, with baseline AI/local inference DISABLED/DEFAULT-OFF. Result: `PASS` / `CONDITIONAL_PASS` / `FAIL`. The 8-vCPU / 32-GB Core profile remains **TARGET / NOT BENCHMARK-CERTIFIED**. Do not silently increase hardware minimums on FAIL. Full capacity/scale certification remains BUILD-130.

A separate Heavy Network Workload profile covers high-interface, aggressive polling, heavy routing, gNMI, API/NCM and high-rate stream cases.

SNMP benchmark evidence shall additionally publish poll operations/sec, OIDs/sec, successful/timeout/auth-failure mix, GETBULK response-size distribution, average/peak poll duration, retry rate, scheduler queue age, exact-counter/rate cost, trap/inform EPS and CPU/RAM/network usage. Fleet tests use deterministic poll spreading across the configured interval (normal operational default **5 minutes** unless overridden); a synchronized all-device minute-boundary is not an acceptable benchmark profile unless explicitly testing storm resilience.

Adaptive polling is capacity-accounted separately. Its configured maximum targets/interval/duration must preserve normal polling freshness and HA reserve. High-rate trap/event load may require Streaming/Event Sensor separation rather than consuming Network Poll workers.

### 8.1 Observability Phase-1 Enrichment Overlay (OBS-P1)

Do **not** create a duplicate `NOWP-2500` base workload profile. Base capacity planning continues to reuse `SNWP-2500-v1` as the baseline. Observability enrichment is measured and certified through a versioned **OBS-P1 enrichment overlay** evaluated incrementally across:

1. **BASE:** standard `SNWP-2500-v1` reference workload.
2. **BASE + temporal topology:** temporal operational graph relationship snapshots and incident-window reconstruction.
3. **BASE + active path:** bounded multi-vantage active path probes (ICMP/TCP/UDP), path fingerprinting, and change tracking.
4. **BASE + representative eBPF subset:** Linux Agent eBPF process-to-network telemetry across representative Linux workloads with bounded local aggregation.
5. **BASE + investigation/RCA:** concurrent Investigation Sessions, Live Evidence streaming, and deterministic causal RCA evaluations.
6. **Representative combined Phase-1 observability profile:** all above capabilities active concurrently under representative enterprise operational mix.

For each overlay profile, benchmarks must measure incremental:
- Core CPU and RAM utilization;
- Sensor CPU and RAM utilization;
- Agent CPU and RAM utilization;
- Linux kernel-map memory on monitored hosts;
- local and centralized disk I/O and storage growth;
- network bandwidth consumption between Agent↔Sensor and Sensor↔Core;
- ingestion gateway, Kafka throughput/backlog, and ClickHouse write/merge impact;
- large-estate query and dashboard latency;
- base monitoring polling freshness and completion percentage;
- scheduler queue age and admission delays;
- alert qualification and incident creation latency;
- investigation and deterministic RCA response latency.

**Inviolable non-interference gate:** Certification fails if optional observability enrichment causes base monitoring objectives (scheduled polling, durability, liveness, alert qualification, deterministic RCA, or HA failover targets) to be violated or degraded. Engineering is strictly prohibited from "fixing" an overlay benchmark failure by silently raising minimum Core, Sensor, or Agent hardware tiers. If resource budgets are exceeded, eBPF aggregation, active path probing cadences, and analytical enrichment must load-shed, sample, or suspend to protect primary monitoring correctness.


## 9. Core storage sizing model

A fixed “TB per device” number is prohibited because storage depends on data volume and customer retention. The sizing calculator computes:

```text
raw metric bytes/day
+ state/event bytes/day
+ Flow bytes/day
+ syslog/log bytes/day
+ audit/incident/NCM/diagnostic-investigation evidence bytes/day
+ Kafka operational retention
+ PostgreSQL control growth
+ ClickHouse merge/working headroom
+ archive/backup headroom
+ filesystem reserve
```

Inputs include sample/record rate, average encoded row size after benchmarked compression, raw retention, each rollup resolution/retention, archive policy and legal holds. The UI displays current/proposed storage, growth/day, projected 80% date and recommended capacity.

## 10. Retention and rollups

Customer admins configure by data type/scope:

- raw retention;
- rollup resolution(s);
- rollup retention(s);
- archive destination/retention;
- emergency purge priority within safe data classes.

Numeric gauges preserve min/max/avg/count and percentiles where meaningful. Counters preserve rate/delta/total semantics; states preserve availability/down duration/transitions/last state. Historical queries automatically choose an appropriate resolution and disclose the resolution used.

## 11. 80% Storage Protection Engine

### 11.1 Core

The 80% safety watermark is non-disableable. Pre-watermark zones provide warning/forecast/normal TTL/compaction. At or above 80%, emergency reclamation starts and targets safe headroom rather than stopping at 79.9%. Oldest eligible raw telemetry, Flow and logs are reclaimed before protected platform state. Configuration, identity/RBAC, current inventory, licensing, current incidents and protected security/governance data are not indiscriminately purged. Audit/compliance data follows protected retention/archival rules.

### 11.2 Sensors

The same 80% principle applies to Sensor disks. Sensor cleanup prioritizes junk/temp/debug logs and already-delivered/expired spool data; if necessary during prolonged outage, oldest eligible lower-priority buffered raw data may be shed according to policy while polling continues. Sensor must expose remaining buffer horizon and loss risk.

### 11.3 Legal hold

Incident/investigation legal holds protect selected time windows/entities/evidence from normal purge and make the Storage Engine reclaim other eligible data first.

## 12. Performance/resource certification

Every release benchmarks Core CPU/RAM, Sensor CPU/RAM/disk, metrics/sec, polls/sec, Flow records/sec, syslog EPS, ingestion lag, query latency, dashboard load latency, report/RCA concurrency, **interactive diagnostic/CLI concurrency, multi-vantage fan-out, bounded continuous-test load and diagnostic latency under simultaneous certified monitoring load**, compression, retention cleanup and failure recovery. Certification must prove that interactive troubleshooting cannot starve scheduled monitoring/ingestion. Regressions beyond approved tolerance fail certification or require documented exception.

For releases enabling Local AI/ML/LLM, certification additionally records per-module normal/peak CPU and RAM, inference latency, local LLM concurrency/context, model load/unload behavior, queue/backpressure, GNN/embedding workload, AI artifact disk footprint and the effect of AI load on ingestion, alert qualification, query latency and failover. The supported local-intelligence profile must pass inside the same certified Core tier; it does not create a hidden higher minimum hardware requirement.

## 13. Development workstation resource behavior — v3.1

The Docker development environment must be profile-driven so that an engineer is not forced to run the entire production-equivalent topology on a laptop. `dev` starts only the services needed for the current task; `scale`, `ha`, `security` and `dr` are explicit opt-in profiles.

Stateful service containers must have bounded development defaults and named volumes. Docker restart after laptop reboot must not create uncontrolled CPU/RAM spikes; startup sequencing and health checks shall stagger expensive data services where necessary.

## 14. Local log/disk protection

Docker volumes and container logs are included in workstation disk monitoring guidance. Development instructions shall include commands to identify large images/volumes, safely prune disposable build artifacts and retain named volumes that contain intentional development state. Automated scripts must never indiscriminately prune all Docker data on a shared development laptop.

---

## 15. Endpoint Diagnostic Snapshot and isolated local-inference resource model

### 15.1 Endpoint EDS resource contract

EDS is event-driven Diagnostic Job profile evidence acquisition on the existing Agent → Sensor → Core durability path, not continuous duplicate monitoring and not a second transport/queue product.

Resource rules:

- capture-eligibility evaluation reuses normal Agent/Sensor observations wherever possible;
- no separate always-on RCA interpreter/process is required on the endpoint;
- one or another small bounded number of concurrent captures is permitted only according to certified endpoint profile;
- capture duration, evidence bytes, Top-N cardinality, log/Event Log lookback and retry frequency have hard policy/implementation ceilings;
- repeated equivalent capture-eligible conditions are coalesced and cooldown-governed;
- site/tenant/estate storm budgets prevent simultaneous threshold events from producing an evidence-upload storm;
- endpoint pressure checks can shorten/suppress capture; a `PARTIAL`/`SUPPRESSED` result with explicit reason is preferable to worsening the incident;
- logical priority/fairness protects, in order: authority/control/assignment → current monitoring correctness → normal telemetry durability → critical event evidence → diagnostic evidence → old/bulk replay per existing replay policy;
- EDS queue/upload remains lower priority than protected endpoint workload and lower priority on Sensor/Core than routine collection/ingestion durability;
- suppressed/shortened/dropped/expired/shed diagnostic evidence must be explicit, observable and auditable; diagnostic protection must never silently destroy routine telemetry durability.

A literal zero-CPU promise is prohibited. Certification instead measures **incremental idle overhead** and **bounded transient capture overhead** for representative Linux/Windows endpoint classes.

### 15.2 Infrastructure Sensor capacity accounting

Infrastructure Sensor MWU accounting shall include:

- SSH/WinRM recurring collection cost;
- EDS diagnostic query cost;
- target RTT/timeouts/authentication cost;
- output/parser cost;
- concurrent Agent EDS receive/forward workload;
- agentless EDS fan-out/storm load.

When the Infrastructure Sensor lacks headroom, diagnostics queue/defer/reject explicitly before scheduled monitoring is starved.

### 15.3 Optional dedicated local/private inference provider

A dedicated inference provider is a separately sized **conditional** optional component for customers that do not want LLM/embedding bursts on Core. **If** that mode is supported/enabled, the isolation/security/non-authority requirements below are mandatory. It is **not required** for Linux/Windows monitoring, agentless SSH/WinRM/WMI/CIM, EDS, deterministic Core RCA or base Platform operation. Core-resident LLM remains optional. All-AI-disabled remains supported.

It:

- is not counted as a third Core;
- stores no authoritative control/telemetry database;
- is not part of Core HA quorum/fencing;
- may be CPU-only or use an optional accelerator according to its own certified profile;
- receives bounded context from AI Gateway rather than raw unrestricted telemetry;
- can be stopped/unavailable without affecting deterministic monitoring/RCA;
- is not an EDS release prerequisite.

Exact CPU/RAM/GPU/model combinations are release-certified profiles, not universal minimums. No dedicated inference hardware is required for Platform base operation.

## Scheduler / freshness capacity notes — v3.2.2 handoff

Admission and MWU accounting must observe CPU, RAM, spool/disk horizon, queue depth, execution latency, timeout/error rate, upload backlog and freshness debt. Low-priority work may defer only inside governed freshness limits. Protect liveness, recovery, critical operational monitoring and authority/control work. Adaptive/event storms must not accelerate unbounded populations. No new monitoring datastore is invented for self-observability.

---

## Phase-1 endpoint inventory and Triggered Fast RCA capacity addendum — Issues #169/#170

### Installed-software inventory

- Default authoritative reconciliation is approximately once per 24 hours, but schedules are deterministically spread/jittered; fleet scans must not align at midnight or restart time.
- Trustworthy OS/package change signals may request earlier reconciliation, subject to coalescing/cooldown and resource admission.
- Enumeration item count, payload bytes, duration and Agent/Sensor concurrency are bounded and benchmarked per supported profile.
- Differential/change evidence may reduce transport/storage cost, but periodic full authoritative reconciliation remains required to repair missed local events.
- Inventory work is lower priority than authority/control, routine monitoring and durable telemetry.
- Software/package names/versions are inventory dimensions, not unbounded metric-label cardinality.
- Exact per-endpoint inventory limits are release-certified measurements, not universal documentation promises.

### Process/service monitoring and Criticality-driven profiles

- Monitoring arbitrary transient process populations is prohibited. Stable selectors and bounded monitored-definition counts are capacity inputs.
- A bulk Criticality/profile change uses capacity preview/admission and deterministic rollout; it must not synchronously accelerate an estate.
- CRITICAL does not mean maximum frequency/all metrics; all profiles remain constrained by certified Agent/Sensor/Core/cardinality budgets.

### Triggered Fast RCA

Existing EDS budgets remain authoritative. Cross-vantage enrichment adds explicit fan-out budgets per incident/site/tenant and does not automatically query every OOB/virtualization/storage/path source. Evidence already present in the time window is reused before issuing another active diagnostic where sufficient.

Under pressure, protected ordering remains:
`authority/control → routine collection → telemetry durability → critical events → diagnostic evidence → bulk/old replay`.

Fast RCA certification measures endpoint capture overhead, Agent/Sensor upload, Core evidence fan-out, incident/RCA latency and effect on protected monitoring. No fixed universal CPU/RAM/latency promise is created without BUILD-019/130/134 evidence.


---

## 16. v3.3.0 new workload dimensions and benchmark gates

No universal capacity number is introduced by ACP-001..ACP-018. New certification must measure the following independent dimensions and prove protected workloads retain headroom.

### 16.1 OpenTelemetry

Measure metrics points/s, structured log records/s, encoded/uncompressed bytes/record, resource/service count, attributes/record, active cardinality, compression ratio, accepted/rejected/dropped rates, Sensor/Core queue depth and oldest age, replay/recovery rate, ClickHouse write/query amplification and storage/retention. Phase-1.5 trace certification additionally measures spans/s, bytes/span, active/in-flight traces, trace duration distribution, sampling/tail-sampling memory/state, trace query latency and incident-retention overhead. Exact rates/retention remain P0 benchmark outputs.

### 16.2 ITSM/Integration Service

Measure outbound/inbound operations/s, webhook burst/storm handling, per-tenant queue isolation, external outage backlog, retry recovery without vendor thundering herd, 429/rate-limit recovery, DLQ/quarantine growth and connector-health latency. Retry traffic must not starve monitoring ingestion/control.

### 16.3 CMDB reconciliation

Certification scenarios include representative **100K, 500K and 1M+ external bindings/CIs** and multi-million relationships where applicable, with incremental reconciliation windows, match/conflict processing, PostgreSQL/graph/query cost, API-rate limits and restart/checkpoint recovery. These are benchmark scales, not contractual limits or guaranteed capacities.

### 16.4 NCM and firmware

NCM concurrency budgets are independent of routine polling and bounded by tenant, site, vendor/device family, HA/failure domain and device/vendor session/API limits. Measure backup/config size, diff/search workload, concurrent pre/post checks, batch/canary timing, rollback traffic and Integration Service dependency. Phase-1.5 firmware adds repository/storage/bandwidth, staged image transfer, reboot concurrency and rollback-image reserve.

### 16.5 Browser journey synthetics — Phase 1.5

Before selecting/shipping a browser runtime, benchmark memory/browser, CPU/browser, concurrent journeys/Sensor, page/network size, screenshot/artifact volume, timeout/cancellation, browser crash cleanup, secret isolation and effect on protected Sensor workloads.


### v3.3 formal CMDB scale requirement anchor

- `REQ-CMDB-007` — CMDB reconciliation is incremental, idempotent, rate-bounded and audited; certification benchmarks representative 100K, 500K and 1M+ bindings/CIs plus multi-million relationships where applicable without converting those test points into universal capacity claims.

<!-- PHASE-1-1.5-ENVELOPE-2026-10-09 -->
## Phase 1 / Phase 1.5 capacity certification overlay — 2026-10-09

### Phase 1
- the 8 vCPU / 32 GB Core value remains an engineering target, not an achieved capacity claim, for the standalone SNWP-2500-v1 profile;
- one Core is measured with all mandatory Phase-1 services and the exact selected module set;
- capacity evidence separates sustained, burst and backlog-recovery capacity;
- Sensor capacity is certified by role/workload (NETWORK, INFRASTRUCTURE, FLOW, LOG, Agent ingress, small COMBINED where explicitly supported), not by device count alone;
- spool sizing uses measured retained bytes/sec × required outage duration × write/compression/indexing amplification × safety/recovery headroom;
- swap is a resilience safety valve, never counted as equivalent physical RAM or normal performance headroom.

### Phase 1.5
Adds per-Core failover reserve, live workload + backlog replay after failover, shared-MSP/noisy-neighbour quotas and the resource cost of two-Core durability/rejoin. HA per-Core targets do not block Phase-1 standalone certification.

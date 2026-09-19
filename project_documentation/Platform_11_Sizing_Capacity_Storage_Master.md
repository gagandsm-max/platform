# Platform — Sizing, Capacity, Sensor Distribution & Storage Master

**Document ID:** PLATFORM-SIZING-001  
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Engineering Sizing Model**  
**Product:** Platform

## 1. Governing rule

Device count is a commercial/planning input, not the only engineering load measure. Platform must size and operate using measured workload: poll operations/sec, metrics/sec, interfaces/entities, polling interval, protocol cost, RTT/timeouts, Flow/log rates, streaming subscriptions, NCM jobs, synthetic tests, **interactive/on-demand diagnostics and CLI sessions**, concurrent users, query/report/RCA demand and retention.

All numbers below are **engineering planning targets until benchmark-certified**. They are not contractual scale claims merely because incumbent products publish similar densities.

## 2. Core compute targets

### 2.1 Non-HA

| Monitored network devices | Core count | Engineering target per Core |
|---:|---:|---|
| 1,000 | 1 | **8 vCPU / 16 GB RAM** |
| 2,000 | 1 | **8 vCPU / 16 GB RAM** |
| up to 2,500 | 1 | **8 vCPU / 16 GB RAM mandatory optimization target under SNWP-2500-v1** |
| 5,000 | 1 | **12 vCPU / 24 GB RAM planning target** |
| 10,000 | 1 | **16 vCPU / 32 GB RAM planning target** |
| 15,000 | 1 | **24 vCPU / 48 GB RAM planning target** |

### 2.2 HA

HA uses **exactly two full Core nodes**, one Active and one Standby. Each full Core is sized to carry the certified workload after failover. There is no third customer-visible Core/governance node.

| Devices | Full Core nodes | Per-Core target |
|---:|---:|---|
| 1,000 | 2 | 8 vCPU / 16 GB |
| 2,000 | 2 | 8 vCPU / 16 GB |
| up to 2,500 | 2 | 8 vCPU / 16 GB |
| 5,000 | 2 | 12 vCPU / 24 GB |
| 10,000 | 2 | 16 vCPU / 32 GB |
| 15,000 | 2 | 24 vCPU / 48 GB |

The exact two-Core synchronization/fencing mechanisms for Kafka/ClickHouse/PostgreSQL are P0 engineering spikes. Technology must adapt to the fixed product requirement; no third full Core may be introduced silently.

### 2.3 Local AI / ML / LLM compute envelope — v3.2.2

Local intelligence is part of the planned Core footprint; it shall **not increase the minimum certified Core sizing tiers**. Standard Platform operation is CPU-first and shall not require a GPU. AI/ML/LLM workloads are subordinate to collection, ingestion, storage durability, alert qualification, incident processing and deterministic RCA.

Planning envelopes below are hard design targets until benchmark certification:

| Per-Core target | Local-intelligence peak budget | Default local LLM class | Concurrent local LLM requests | Notes |
|---|---:|---|---:|---|
| **8 vCPU / 16 GB** | **≤2 vCPU burst / ≤3 GB RAM peak** | **0.5B–1.5B parameter, 4-bit quantized** | **1** | Standard smallest Core. A model above 1.5B is not assumed; up to ~3B may be admitted only if SNWP certification proves no monitoring regression. |
| **12 vCPU / 24 GB** | **≤3 vCPU burst / ≤4 GB RAM peak** | **1B–3B, 4-bit quantized** | **1–2** | Extra capacity primarily buys monitoring/user concurrency, not permission for an unnecessarily large model. |
| **16 vCPU / 32 GB** | **≤3 vCPU burst / ≤5 GB RAM peak** | **1B–3B, 4-bit quantized** | **2** | AI remains lower priority than live monitoring and storage. |
| **24 vCPU / 48 GB** | **≤4 vCPU burst / ≤6 GB RAM peak** | **1B–3B, 4-bit quantized** | **2–4** | Higher concurrency is preferred over increasing model size. |

For the 8 vCPU / 16 GB tier the normal non-interactive local-intelligence target is **<1 vCPU average and approximately 1–1.5 GB RAM**, with the larger peak envelope used only during bounded inference/analytics activity.

Mandatory resource rules:

- **No mandatory GPU/NPU.** Optional accelerator support may be certified later, but no GPU may become a prerequisite for standard Core monitoring, alerting or deterministic RCA.
- **No foundation-model training/fine-tuning on customer production Core.** Platform ships/loads approved pre-trained quantized local models. Customer-specific intelligence comes from evidence retrieval, tools, topology, Similar Incident Memory and policy, not foundation-model fine-tuning.
- Lightweight statistical/ML baseline recalibration is permitted, but it is scheduled, bounded, interruptible and lower priority than live monitoring.
- Local LLM inference is **on demand**. On the 8/16 tier the model should be unloaded or memory-mapped with minimal resident state when idle and released after a bounded idle period unless benchmarked headroom permits keeping it warm.
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

## 5. Dedicated Flow and Log Sensor sizing

Production Flow and Log collection are **role-exclusive**, not threshold-triggered mixed workloads.

- `FLOW_SENSOR`: NetFlow/IPFIX/sFlow/J-Flow/vendor Flow only, plus mandatory common Sensor services.
- `LOG_SENSOR`: Syslog/log/event/assigned OTLP-log workloads only, plus mandatory common Sensor services.
- Neither role shares a production instance with ordinary polling/discovery/NCM/synthetics or with the other high-rate role.
- Combined high-rate roles exist only for development/CI/demo/explicit non-production lab use.

Exact Flow records/sec and Log EPS values remain benchmark-defined. Dedicated roles still require safety headroom for bursts, spool growth, replay, runtime/GC, upgrades, health and failover; 'use the whole Sensor' means exclusive workload ownership, not 100% resource saturation.

### 5.1 Flow Sensor workload model

Flow sizing/certification includes at minimum:

- configured/active exporters and listeners;
- datagrams/sec, records/sec and bytes/sec;
- NetFlow v5/v9, IPFIX, sFlow/J-Flow mix;
- average/max records/datagram and fields/record;
- observation domains/exporter;
- template count, templates/sec and template churn;
- enterprise/variable-length information-element cost;
- sampling mix and normalization cost;
- IPv4/IPv6/NAT/QoS/application metadata mix;
- identity/DNS/IPAM/user/ASN/cloud/GeoIP enrichment hit rate and lookup/cache cost;
- kernel/socket drop behavior and receive-buffer pressure;
- decode/normalize/spool queue depth/age;
- local durable-spool write rate/horizon and 80% protection;
- Core upload bandwidth, replay drain time and downstream ingestion lag;
- ClickHouse insert/rollup cost;
- Top-N/matrix/Sankey/query concurrency.

Per-exporter fairness and global ceilings are included in capacity tests so a single source cannot consume the certified role envelope.

Flow Sensor Pool redundancy is sized per exporter/failover profile. Capacity planning includes spare receiver/decode/spool/upload headroom for a failed peer or moved exporter assignment and accounts for duplicate-export overhead where exporters send to more than one collector. A generic `active + 1` count is not assumed across all Flow deployments until exporter behavior and locality/security boundaries are known.

### 5.2 Log Sensor workload model

Log Sensor sizing is independent of Flow sizing and uses source count, EPS, bytes/sec, transport/TLS connections, parser/multiline/redaction/fingerprint cost, spool/forwarding queues and egress. Flow and Log capacity are never added into one production Sensor budget.

## 6. Monitoring Work Units (MWU)

Platform must compute a workload score using at least:

- protocol type and operation cost;
- metrics/OIDs/entities/interface count;
- poll/subscription interval;
- SNMP GET/BULK plan and timeout/retry behavior;
- RTT, packet loss and timeout rate;
- gNMI stream rate;
- REST calls and response size;
- Flow records/sec, datagrams/sec, bytes/sec, exporter/domain count, template rate/churn, sampling/enrichment and spool cost (Flow Sensor only);
- Syslog/events/sec, bytes/sec, parser/multiline/redaction/forwarding cost (Log Sensor only);
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

The 8 vCPU/16 GB Core target is certified only against a versioned workload profile containing 2,500 representative enterprise network devices; ICMP; meaningful SNMPv2c/v3 mix; interface inventory/performance; CPU/memory/uptime/environment; traps; topology; alert evaluation; representative dashboards/reports/incidents/RCA; with Flow and high-rate log workloads excluded from the Network Sensor profile because production Flow/Log run on dedicated roles. Exact interface/metric/event distributions are committed in the performance repository before certification, not invented in product documentation.

A separate Heavy Network Workload profile covers high-interface, aggressive polling, heavy routing, gNMI and API/NCM cases. Flow and high-rate log streams use their own dedicated-role benchmark profiles rather than a mixed Network Sensor profile.

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
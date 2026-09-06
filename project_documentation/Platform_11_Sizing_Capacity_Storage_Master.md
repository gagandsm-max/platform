# Platform — Sizing, Capacity, Sensor Distribution & Storage Master

**Document ID:** PLATFORM-SIZING-001  
**Version:** 3.2.1  
**Status:** **Authoritative Mother Reference — Engineering Sizing Model**  
**Product:** Platform

## 1. Governing rule

Device count is a commercial/planning input, not the only engineering load measure. Platform must size and operate using measured workload: poll operations/sec, metrics/sec, interfaces/entities, polling interval, protocol cost, RTT/timeouts, Flow/log rates, streaming subscriptions, NCM jobs, synthetic tests, concurrent users, query/report/RCA demand and retention.

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
- RTT, packet loss and timeout rate;
- gNMI stream rate;
- REST calls and response size;
- Flow records/sec and template rate;
- Syslog/events/sec;
- agent/agentless collection cost;
- NCM/CLI scheduled/concurrent jobs;
- discovery/adaptive-polling burst;
- local spool/backlog and CPU/RAM/disk headroom.

UI shows both simple device count and calculated workload/capacity. Placement never assigns a device to the lowest-RTT Sensor if doing so would overload it.

## 7. Sensor Placement Engine

Eligibility considers: tenant/security boundary, protocol reachability, preferred/allowed/forbidden pools, site/region locality, measured RTT/loss, Sensor role, CPU/RAM, current MWU, queue/backlog, local disk, HA reserve and maintenance/drain state.

Customer modes: **Automatic (recommended)**, **Automatic with Approval**, **Manual**, **Pool Based**. Rebalancing uses hysteresis to avoid thrashing.

## 8. Standard Network Workload Profile SNWP-2500-v1

The 8 vCPU/16 GB Core target is certified only against a versioned workload profile containing 2,500 representative enterprise network devices; ICMP; meaningful SNMPv2c/v3 mix; interface inventory/performance; CPU/memory/uptime/environment; traps; topology; alert evaluation; representative dashboards/reports/incidents/RCA; and Flow/syslog below dedicated-streaming threshold. Exact interface/metric/event distributions are committed in the performance repository before certification, not invented in product documentation.

A separate Heavy Network Workload profile covers high-interface, aggressive polling, heavy routing, gNMI, API/NCM and high-rate stream cases.

## 9. Core storage sizing model

A fixed “TB per device” number is prohibited because storage depends on data volume and customer retention. The sizing calculator computes:

```text
raw metric bytes/day
+ state/event bytes/day
+ Flow bytes/day
+ syslog/log bytes/day
+ audit/incident/NCM bytes/day
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

Every release benchmarks Core CPU/RAM, Sensor CPU/RAM/disk, metrics/sec, polls/sec, Flow records/sec, syslog EPS, ingestion lag, query latency, dashboard load latency, report/RCA concurrency, compression, retention cleanup and failure recovery. Regressions beyond approved tolerance fail certification or require documented exception.

## 13. Development workstation resource behavior — v3.1

The Docker development environment must be profile-driven so that an engineer is not forced to run the entire production-equivalent topology on a laptop. `dev` starts only the services needed for the current task; `scale`, `ha`, `security` and `dr` are explicit opt-in profiles.

Stateful service containers must have bounded development defaults and named volumes. Docker restart after laptop reboot must not create uncontrolled CPU/RAM spikes; startup sequencing and health checks shall stagger expensive data services where necessary.

## 14. Local log/disk protection

Docker volumes and container logs are included in workstation disk monitoring guidance. Development instructions shall include commands to identify large images/volumes, safely prune disposable build artifacts and retain named volumes that contain intentional development state. Automated scripts must never indiscriminately prune all Docker data on a shared development laptop.

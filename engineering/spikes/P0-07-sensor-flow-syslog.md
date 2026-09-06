# Engineering Spike Record: P0-07 — Streaming Sensor Flow Records/sec & Syslog EPS

**Spike ID:** SPIKE-P0-07  
**Status:** **OPEN**  
**Owner:** Principal Collector / Ingestion Engineer  
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md` §5.2, `Platform_11_Sizing_Capacity_Storage_Master.md` §4  
**Target Closure Day:** Day 16 (Sprint 2)  

---

## 1. Exact Unresolved Question
What are the certified maximum sustained NetFlow/sFlow/IPFIX records per second and Syslog Events Per Second (EPS) that a single Dedicated Streaming Sensor can ingest, normalize, and forward to Core without buffer drops?

## 2. Why It Matters
When high-volume event rates threaten polling operations, Platform deploys a Dedicated Streaming/Event Sensor. Its maximum throughput must be certified to advise customer sizing and avoid silent UDP socket drops.

## 3. GA Blocking Impact
**HIGH / SIZING ACCURACY.** Required for customer capacity sizing guidelines and hardware specifications in high-traffic datacenter environments.

## 4. Evaluated Alternatives (Open for Spike Testing)
* **Design A (Go Pure-Userspace UDP Loop):** Standard Go UDP listeners with ring-buffer channels.
* **Design B (Linux eBPF / XDP Ingestion Acceleration):** Optional low-level packet filter for ultra-high packet rates (>100k EPS).
* **Design C (Multi-threaded C/Go hybrid or Zero-Copy IO):** Memory mapped ring buffers.

## 5. Evidence Required
1. Benchmarked loss-curves: Packet generation at 10k, 25k, 50k, 100k records/sec.
2. UDP socket buffer drop counts (`netstat -su`).
3. Core ingestion latency and ClickHouse batch insert throughput.

## 6. Test & Benchmark Methodology
Use synthetic Flow generator (`flowgen` / `tcpreplay`) and Syslog generator (`loggen`); flood Streaming Sensor container; measure CPU utilization, spool buffer write rate, and end-to-end ingestion delay.

## 7. Acceptance Criteria
* Sustained 50,000 flow records/sec with <0.01% drop rate on 8C/16G profile.
* Sustained 25,000 Syslog EPS with structured normalization.
* Spool buffer circuit breaker engages cleanly if Core becomes unreachable.

## 8. Affected Components & Mother Documents
* **Components:** `sensor` (streaming engine), `core` ingestion.
* **Mother Documents:** `Platform_03_Architecture.md`, `Platform_10_Protocol_Security_Connector_Master.md`, `Platform_11_Sizing_Capacity_Storage_Master.md`.

## 9. Dependencies
* Requires Flow and Syslog generator tools (Phase 8 test infrastructure).

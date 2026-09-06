# Engineering Spike Record: P0-01 — Two-Core Kafka Implementation & Failover

**Spike ID:** SPIKE-P0-01  
**Status:** **OPEN**  
**Owner:** Staff Infrastructure / Streaming Architect  
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md` §4.3, §7.2  
**Target Closure Day:** Day 5 (Sprint 1)  

---

## 1. Exact Unresolved Question
How can Apache Kafka KRaft mode be deployed across exactly two Core nodes (Active/Standby) to provide transparent streaming failover without violating the non-negotiable architectural constraint prohibiting a third full Core node or external quorum node?

## 2. Why It Matters
Kafka is the internal event and streaming backbone for all Sensor telemetry ingestion, incident qualification, and control signals. During a Core failover (<60s target), Sensors must not permanently drop events or block ingestion buffers.

## 3. GA Blocking Impact
**CRITICAL / GA BLOCKER.** Without an agreed two-node Kafka HA topology, dual-Core Active/Standby failover cannot be tested, certified, or deployed commercially.

## 4. Evaluated Alternatives (Open for Spike Testing)
* **Alternative A (Active KRaft Broker + Mirrored Standby):** Run an active Kafka broker on Core-1 and an active broker on Core-2 with replication factor 2 and min.insync.replicas=1, with an automated controller tie-breaker.
* **Alternative B (Active Broker + Passive Standby Cold/Warm Standby):** Run Kafka strictly on Active Core, with persistent volume block-level replication (e.g. DRBD or shared SAN) to Standby.
* **Alternative C (Valkey/Raft-Assisted Quorum Fencing):** Use lightweight internal KRaft quorum with dynamic re-assignment on supervisor failover.

## 5. Evidence Required
1. Chaos test results: Network partition between Core-1 and Core-2; kill Active Core process.
2. Measurements of ingestion delay, message loss count, and recovery time.
3. Verification that zero Sensor spools are corrupted during failover.

## 6. Test & Benchmark Methodology
Simulate 10,000 events/sec streaming from 2 Sensors into Core-1; execute `docker pause` / SIGKILL on Core-1; measure Standby promotion time, broker failover latency, and verify consumer group offset continuity.

## 7. Acceptance Criteria
* Standby Core Kafka broker promotes and accepts traffic in <30 seconds.
* Zero uncommitted message duplicates that violate idempotency.
* No third full Core node required.

## 8. Affected Components & Mother Documents
* **Components:** `core`, `kafka`, `sensor` (coreclient).
* **Mother Documents:** `Platform_03_Architecture.md`, `Platform_04_HLD.md`, `Platform_05_LLD.md`, `Platform_10_Protocol_Security_Connector_Master.md`.

## 9. Dependencies
* Spike P0-04 (Core fencing and VIP mechanism).

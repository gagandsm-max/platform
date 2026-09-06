# Engineering Spike Record: P0-02 — Two-Core ClickHouse Replication, Promotion & Fencing

**Spike ID:** SPIKE-P0-02  
**Status:** **OPEN**  
**Owner:** Principal Database / Telemetry Engineer  
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md` §4.2, §7.3  
**Target Closure Day:** Day 6 (Sprint 1)  

---

## 1. Exact Unresolved Question
What is the exact ClickHouse replication and promotion design across two Core nodes that ensures historical telemetry, flow, and log continuity without split-brain mutation under a 2-node constraint?

## 2. Why It Matters
ClickHouse stores all time-series metrics, flow records, syslogs, and raw events. If Standby takes over without clean replication and fencing, telemetry data could diverge or suffer table corruption.

## 3. GA Blocking Impact
**CRITICAL / GA BLOCKER.** Commercial enterprise customers will not accept data divergence or query failure after high-availability failover.

## 4. Evaluated Alternatives (Open for Spike Testing)
* **Alternative A (ClickHouse Keeper 2-node / Learner topology):** ClickHouse Keeper running on both Core nodes with Raft learner or witness role.
* **Alternative B (ReplicatedMergeTree with Active-Standby Replication):** Active writes to both nodes (`ReplicatedMergeTree`), with Standby in read-only mode until promoted.
* **Alternative C (Storage-Level Asynchronous Mirroring):** Active Core writes to local ClickHouse; continuous block/backup synchronization to Standby.

## 5. Evidence Required
1. Benchmark under 50,000 metric rows/sec write load during node failure.
2. Verification of table partition integrity and `SELECT count(*)` consistency post-promotion.
3. Proof that the demoted old Active node cannot accept split-brain writes.

## 6. Test & Benchmark Methodology
Generate continuous time-series and flow insertion; sever network connectivity between nodes; promote Standby; query data continuity; restore old Active and verify rejoin/catch-up behavior.

## 7. Acceptance Criteria
* Telemetry write failover completed in <45s.
* Zero data loss for acknowledged ClickHouse blocks.
* Clean rejoin of resurrected node without manual table repair.

## 8. Affected Components & Mother Documents
* **Components:** `clickhouse`, `core` ingestion pipeline.
* **Mother Documents:** `Platform_03_Architecture.md`, `Platform_05_LLD.md`, `Platform_11_Sizing_Capacity_Storage_Master.md`, `Platform_17_API_Event_and_Data_Model_Contracts.md`.

## 9. Dependencies
* Spike P0-01 (Kafka event bus), Spike P0-04 (Fencing).

# Engineering Spike Record: P0-02 — Two-Core ClickHouse Replication / Promotion / Fencing

**Spike ID:** SPIKE-P0-02
**Status:** **OPEN**
**Owner:** Staff Data Platform Architect
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md`, `Platform_11_Sizing_Capacity_Storage_Master.md`
**Target Closure Day:** Day 6 (Sprint 1)

---

## Classification legend

| Label | Meaning |
|---|---|
| **AUTHORITATIVE FIXED REQUIREMENT** | Mother-reference fixed |
| **PROPOSED TEST POINT — NON-AUTHORITATIVE** | Benchmark load/timing only |
| **TBD — DECISION REQUIRED** | Unresolved mechanism |

---

## 1. Exact unresolved question

**TBD — DECISION REQUIRED:** Exact two-Core ClickHouse replication/promotion/fencing design for Active/Standby without a third full Core, including behaviour under failed replication and rejoin/resync.

---

## 2. Authoritative fixed requirements

* ClickHouse is the primary high-volume telemetry/analytics store (**AUTHORITATIVE FIXED REQUIREMENT**).
* Exactly two full Cores (**AUTHORITATIVE FIXED REQUIREMENT**).
* Overall certified Core service restoration **&lt;60s** (**AUTHORITATIVE FIXED REQUIREMENT**).
* Absolute zero telemetry loss is **not** a Platform guarantee; 80% storage protection and policy shedding apply at edges/stores as documented (**AUTHORITATIVE FIXED REQUIREMENT**).
* Durable-ACK behaviour for Sensor ingest must stay consistent with the final ACK contract from P0-01 (**AUTHORITATIVE FIXED REQUIREMENT** once that contract freezes).

---

## 3. Required outcomes to prove empirically

* No split-brain writers.
* Replay/duplicate handling is deterministic and visible.
* Failed replication is observable (health/alerts), not silent.
* Standby is recoverable; rejoin/resync is supported.
* Acknowledged-data behaviour matches the durable-ACK contract.

---

## 4. Candidate mechanisms (spike only)

Mechanisms (dual-write/backfill, replica promotion patterns, storage-assisted warm standby, etc.) remain **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION** where not already USE in Platform_07. Do not invent a frozen CH Keeper 3-node quorum that implies a third Core.

---

## 5. Evidence required

1. Failover/promotion drill with writer fencing.
2. Duplicate/replay verification after Standby promotion.
3. Replication-lag / failure visibility evidence.
4. Rejoin/resync of former Active.

---

## 6. Test methodology (non-authoritative)

Example write-load stages: 5k / 10k / 25k / 50k rows/sec — **PROPOSED TEST POINT — NON-AUTHORITATIVE** (not acceptance).
Example timing observations during promotion — **PROPOSED TEST POINT — NON-AUTHORITATIVE**. Do **not** treat “&lt;45s” or “50,000 rows/sec” as frozen acceptance.

---

## 7. Closure criteria

Architecture approval of the two-Core CH design + measured evidence recorded; mother docs updated if thresholds become contractual. Overall Core restoration remains **&lt;60s**.

---

## 8. Affected components & mother documents

* **Components:** ClickHouse, `core` ingest/analytics paths.
* **Mother Documents:** `Platform_03`, `Platform_05`, `Platform_06`, `Platform_11`, `Platform_12`.

## 9. Dependencies

* P0-01 durable ACK; P0-04 fencing.

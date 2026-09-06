# Engineering Spike Record: P0-01 — Two-Core Kafka Implementation & Failover

**Spike ID:** SPIKE-P0-01
**Status:** **OPEN**
**Owner:** Staff Infrastructure / Streaming Architect
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md`, `Platform_05_LLD.md`
**Target Closure Day:** Day 5 (Sprint 1)

---

## Classification legend

| Label | Meaning |
|---|---|
| **AUTHORITATIVE FIXED REQUIREMENT** | Mother-reference fixed; must not be weakened by this spike |
| **PROPOSED TEST POINT — NON-AUTHORITATIVE** | Exploratory load/timing point only; not acceptance until mother docs are updated |
| **TBD — DECISION REQUIRED** | Unresolved; inventing a frozen answer here is non-conformant |
| **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION** | Technology may be experimented with; not catalogue-selected |

---

## 1. Exact unresolved question

**TBD — DECISION REQUIRED:** What is the exact two-Core Kafka implementation/failover design (and durable ACK boundary) while honouring **exactly two full Core nodes** and **no third full Core / mandatory external quorum node**?

Do **not** assume the production answer is “two-node KRaft quorum.” Kafka as product technology is fixed; the **two-Core mechanism** is not.

---

## 2. Authoritative fixed requirements

* Kafka is the **internal** Core streaming backbone (**AUTHORITATIVE FIXED REQUIREMENT**).
* Sensors **never** connect directly to Kafka; Sensor → Core is HTTPS/gRPC over mTLS (**AUTHORITATIVE FIXED REQUIREMENT**).
* Sensor local spool is part of edge replay protection during Core/WAN loss (**AUTHORITATIVE FIXED REQUIREMENT**).
* Exactly two full Cores, Active/Standby (**AUTHORITATIVE FIXED REQUIREMENT**).
* Overall certified Core **service restoration** target remains **&lt;60 seconds** (**AUTHORITATIVE FIXED REQUIREMENT**). Any internal Kafka promotion budget is **benchmark-derived**, not invented here.
* Do **not** claim that two Kafka/KRaft voters are inherently fault-tolerant or that a two-voter quorum is safe (**AUTHORITATIVE FIXED REQUIREMENT** — no false quorum safety claim).

---

## 3. Why it matters

Without an evidence-backed two-Core Kafka design and durable ACK contract, HA failover cannot be certified. Sensors continue polling/buffering while capacity permits; absolute zero-loss is **not** promised.

---

## 4. Candidate mechanisms (spike only)

* **Alternative A — Active broker + standby mirror/replication path:** **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION** for any non-catalogue companion tooling.
* **Alternative B — Active-only Kafka with storage-level standby warm/cold restart:** **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION** (e.g. block replication tools are not Platform_07 selections).
* **Alternative C — Other controller/fencing-assisted designs:** **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION**.

KRaft may appear in Docker bootstrap; that does **not** freeze the two-Core production failover design.

---

## 5. Evidence required

1. Failure matrix: Active kill, network partition, Standby promotion, former-Active rejoin.
2. Measured ingestion delay, duplicate/idempotent handling, and recovery time (**PROPOSED TEST POINT — NON-AUTHORITATIVE** until frozen).
3. Proof Sensors do not require direct Kafka access; spool replay remains coherent.
4. Explicit durable-ACK boundary definition (when Core may ACK Sensor batches).

---

## 6. Test methodology (non-authoritative load points)

* Example load steps: 1k / 5k / 10k events/sec — **PROPOSED TEST POINT — NON-AUTHORITATIVE**.
* Chaos: pause/SIGKILL Active; measure promotion and consumer continuity — timings are **PROPOSED TEST POINT — NON-AUTHORITATIVE**.

---

## 7. Closure criteria (not invented SLAs)

Spike closes only when architecture/product formally approve:

* the two-Core Kafka mechanism;
* durable ACK semantics;
* measured promotion/recovery budgets that roll into mother docs / certification evidence;

while preserving overall Core restoration **&lt;60s** and no third full Core.

---

## 8. Affected components & mother documents

* **Components:** `core`, internal Kafka, `sensor` (spool / coreclient).
* **Mother Documents:** `Platform_03`, `Platform_04`, `Platform_05`, `Platform_06`, `Platform_12`.

## 9. Dependencies

* Spike P0-04 (Core fencing / VIP failure matrix).

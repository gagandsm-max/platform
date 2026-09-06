# Engineering Spike Record: P0-03 — PostgreSQL Two-Core Failover / Fencing / Rewind

**Spike ID:** SPIKE-P0-03
**Status:** **OPEN**
**Owner:** Staff Data / Control-Plane Architect
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md`, `Platform_17_API_Event_and_Data_Model_Contracts.md`
**Target Closure Day:** Day 6 (Sprint 1)

---

## Classification legend

| Label | Meaning |
|---|---|
| **AUTHORITATIVE FIXED REQUIREMENT** | Mother-reference fixed |
| **PROPOSED TEST POINT — NON-AUTHORITATIVE** | Exploratory only |
| **TBD — DECISION REQUIRED** | Unresolved |
| **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION** | Not selected |

---

## 1. Exact unresolved question

**TBD — DECISION REQUIRED:** Exact PostgreSQL two-Core failover, fencing, and rejoin/rewind mechanics — including failover controller choice, synchronous vs asynchronous commit behaviour, and resulting availability/data-loss window.

---

## 2. Authoritative fixed requirements

* PostgreSQL is authoritative transactional/control state (**AUTHORITATIVE FIXED REQUIREMENT**).
* Architecture currently describes **primary + standby streaming replication** as the direction (**AUTHORITATIVE FIXED REQUIREMENT** for store role; mechanism details remain spike scope).
* Exactly two full Cores; overall Core service restoration **&lt;60s** (**AUTHORITATIVE FIXED REQUIREMENT**).
* Schema migrations must be **versioned, automated, upgrade-tested** (**AUTHORITATIVE FIXED REQUIREMENT**).
* **Liquibase and Flyway are not selected** as frozen architecture choices (**AUTHORITATIVE FIXED REQUIREMENT** — do not treat either as mandatory).

---

## 3. Explicitly not frozen by this spike

Do **not** freeze as mandatory:

* `pg_rewind`;
* Patroni;
* repmgr;
* Liquibase/Flyway;
* “&lt;30 second PostgreSQL promotion”;
* “zero lost committed transactions” regardless of commit policy.

These may appear only as **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION** or as **PROPOSED TEST POINT — NON-AUTHORITATIVE** observations.

---

## 4. Evidence required

1. Promotion and fencing under Active failure / partition.
2. Rejoin of former primary after safe resync (mechanism **TBD**).
3. Measured behaviour under sync vs async commit candidates — **data-loss window is itself evidence**, not assumed zero.
4. Migration/upgrade behaviour across failover for the **bootstrap schema_migration / chosen future migrator** (not Liquibase by default).

---

## 5. Test methodology (non-authoritative)

Promotion timing measurements are **PROPOSED TEST POINT — NON-AUTHORITATIVE**. Overall Core restoration budget remains **&lt;60s**.

---

## 6. Closure criteria

Formal approval of: failover controller, fencing, rejoin/rewind, commit policy, and documented data-loss/availability window; mother docs updated accordingly.

---

## 7. Affected components & mother documents

* **Components:** PostgreSQL, `core` JDBC/state.
* **Mother Documents:** `Platform_03`, `Platform_05`, `Platform_12`, `Platform_17`.

## 8. Dependencies

* P0-04 fencing/VIP matrix.

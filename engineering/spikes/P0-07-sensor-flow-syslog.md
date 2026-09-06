# Engineering Spike Record: P0-07 — Streaming Sensor Flow / Syslog Certified Rates

**Spike ID:** SPIKE-P0-07
**Status:** **OPEN**
**Owner:** Staff Streaming Sensor Architect
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md`, `Platform_11_Sizing_Capacity_Storage_Master.md`
**Target Closure Day:** Day 10 (Sprint 2)

---

## Classification legend

| Label | Meaning |
|---|---|
| **AUTHORITATIVE FIXED REQUIREMENT** | Mother-reference fixed |
| **PROPOSED TEST POINT — NON-AUTHORITATIVE** | Load-generator stage only |
| **TBD — DECISION REQUIRED** | Certified rates unresolved |

---

## 1. Exact unresolved question

**TBD — DECISION REQUIRED:** Certified Streaming Sensor Flow records/sec and Syslog EPS (and associated drop/backpressure behaviour under defined profiles).

---

## 2. Authoritative fixed requirements

* High-rate events may use a Dedicated Streaming/Event Sensor when volume threatens polling (**AUTHORITATIVE FIXED REQUIREMENT**).
* Absolute “&lt;0.01% drops” or fixed 50k Flow / 25k Syslog guarantees are **not** mother-fixed (**AUTHORITATIVE FIXED REQUIREMENT** — do not invent them here).
* 80% Sensor storage protection and data-loss-risk exposure remain mandatory (**AUTHORITATIVE FIXED REQUIREMENT**).

---

## 3. Explicitly not frozen acceptance

Remove as frozen acceptance (may keep only as labelled stages):

* 50,000 Flow records/sec;
* 25,000 Syslog EPS;
* &lt;0.01% drops.

---

## 4. Proposed load-generator stages (non-authoritative)

Example stages: **10k / 25k / 50k / 100k** Flow r/s or Syslog EPS — **PROPOSED TEST POINT — NON-AUTHORITATIVE**. Not targets or guarantees.

---

## 5. Evidence curves to produce

Certified result must define curves vs:

* CPU;
* RAM;
* socket drops;
* spool growth;
* normalize throughput;
* upload throughput;
* Core ingest latency;
* downstream Kafka/ClickHouse pressure.

---

## 6. Closure criteria

Mother/sizing update with certified Flow r/s and Syslog EPS for defined hardware profiles after evidence review.

---

## 7. Affected components & mother documents

* **Components:** Streaming Sensor, Core ingest, Kafka/ClickHouse path.
* **Mother Documents:** `Platform_03`, `Platform_11`, `Platform_12`, `Platform_14`.

## 8. Dependencies

* P0-01 / P0-02 pressure behaviour under high rate.

# Engineering Spike Record: P0-06 — SNWP-2500-v1 Workload Constants & Certified Results

**Spike ID:** SPIKE-P0-06
**Status:** **OPEN**
**Owner:** Staff Performance Architect
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_11_Sizing_Capacity_Storage_Master.md`, `Platform_03_Architecture.md`
**Target Closure Day:** Day 18 (Sprint 3)

---

## Classification legend

| Label | Meaning |
|---|---|
| **AUTHORITATIVE FIXED REQUIREMENT** | Mother-reference fixed |
| **PROPOSED TEST POINT — NON-AUTHORITATIVE** | Candidate workload scenario only |
| **TBD — DECISION REQUIRED** | Final SNWP constants unresolved |

---

## 1. Exact unresolved question

**TBD — DECISION REQUIRED:** Final SNWP-2500-v1 metric/interface/event mix and certified performance results.

---

## 2. Authoritative fixed requirements

* Core ≤2,500 reference-device engineering target on **8 vCPU / 16 GB** under the eventual versioned SNWP-2500-v1 profile (**AUTHORITATIVE FIXED REQUIREMENT** for the compute envelope; workload constants themselves are TBD).
* Final SNWP constants are **not** frozen in mother docs and **must be discovered by benchmark** (**AUTHORITATIVE FIXED REQUIREMENT**).

---

## 3. Explicitly not authoritative

Do **not** treat any of the following as acceptance criteria:

* 20 interfaces/device;
* 96 interfaces/device;
* polling every 30 seconds;
* polling every 60 seconds;
* “&lt;50 second cycle”;
* “&lt;12 GiB RAM”.

Those may appear only as **PROPOSED TEST POINT — NON-AUTHORITATIVE** candidates if used at all.

---

## 4. What this spike must discover

* device mix;
* interfaces/device;
* metrics/device;
* poll intervals;
* SNMP versions;
* RTT distributions;
* timeout/retry distribution;
* event rates;
* concurrent UI/query/report/RCA load where applicable;
* acceptable CPU/RAM/disk/query/poll behaviour under the 8C/16G envelope.

---

## 5. Proposed scenarios (non-authoritative)

Any campus/DC profiles with sample interface counts or intervals are **PROPOSED TEST POINT — NON-AUTHORITATIVE** until evidence + mother-doc update.

---

## 6. Closure criteria

Versioned SNWP-2500-v1 constants + certified results recorded in mother/sizing docs after review. Until then, values remain **TBD — DECISION REQUIRED**.

---

## 7. Affected components & mother documents

* **Components:** Sensor, Core, benchmark harness.
* **Mother Documents:** `Platform_11`, `Platform_12`, `Platform_14`.

## 8. Dependencies

* Stable collection paths; HA spikes for failover-under-load if claimed.

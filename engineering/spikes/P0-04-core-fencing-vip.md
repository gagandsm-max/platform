# Engineering Spike Record: P0-04 — Core Service Fencing / VIP Failure Matrix

**Spike ID:** SPIKE-P0-04
**Status:** **OPEN**
**Owner:** Staff SRE / HA Architect
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md`, `Platform_04_HLD.md`
**Target Closure Day:** Day 7 (Sprint 1)

---

## Classification legend

| Label | Meaning |
|---|---|
| **AUTHORITATIVE FIXED REQUIREMENT** | Mother-reference fixed |
| **PROPOSED TEST POINT — NON-AUTHORITATIVE** | Candidate mechanism or timing observation |
| **TBD — DECISION REQUIRED** | Exact implementation unresolved |
| **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION** | Not selected |

---

## 1. Exact unresolved question

**TBD — DECISION REQUIRED:** Exact Core service fencing / VIP (or equivalent endpoint ownership) implementation and the certified failure matrix for Active/Standby transitions.

---

## 2. Authoritative fixed requirements

* Exactly one authoritative Active at a time (**AUTHORITATIVE FIXED REQUIREMENT**).
* Ambiguous partition must not produce dual-active (**AUTHORITATIVE FIXED REQUIREMENT**).
* Automatic promotion only with safe fencing / positive failure determination (**AUTHORITATIVE FIXED REQUIREMENT**).
* Certified overall Core service restoration **&lt;60 seconds** (**AUTHORITATIVE FIXED REQUIREMENT**).
* Returning former Active rejoins only after safe resynchronization (**AUTHORITATIVE FIXED REQUIREMENT**).
* Sensors continue polling/buffering during Core failover while capacity permits (**AUTHORITATIVE FIXED REQUIREMENT**).

---

## 3. Explicitly not frozen

Do **not** freeze Keepalived, VRRP, BGP, a specific STONITH implementation, “&lt;45s VIP transition,” or unconditional self-fencing semantics. These may be listed only as **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION** or as **PROPOSED TEST POINT — NON-AUTHORITATIVE** experiments.

---

## 4. Failure matrix to build (outcomes TBD until evidenced)

| Scenario | Required property (fixed) | Implementation result |
|---|---|---|
| Active process crash | Single Active; restore &lt;60s overall | **TBD — DECISION REQUIRED** |
| Network partition | No dual-active | **TBD — DECISION REQUIRED** |
| Standby unreachable | Active remains Active | **TBD — DECISION REQUIRED** |
| Former Active returns | Rejoin only after safe resync | **TBD — DECISION REQUIRED** |
| Split-brain attempt | Fenced / refused | **TBD — DECISION REQUIRED** |

---

## 5. Evidence required

Chaos drills for each matrix row; observed VIP/endpoint ownership; fencing evidence; Sensor reconnect behaviour.

---

## 6. Closure criteria

Approved fencing/VIP (or equivalent) design + filled failure matrix with measured evidence; no invented SLA below the overall **&lt;60s** Core restoration requirement.

---

## 7. Affected components & mother documents

* **Components:** Core HA supervisor, installers, Sensor/Agent endpoint config.
* **Mother Documents:** `Platform_03`, `Platform_04`, `Platform_12`, `Platform_16`.

## 8. Dependencies

* P0-01 / P0-02 / P0-03 data-plane behaviours.

# Engineering Spike Record: P0-10 — Product SKU & Licensing Quantities

**Spike ID:** SPIKE-P0-10
**Status:** **OPEN**
**Owner:** Staff Product Manager & Technical Licensing Lead
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_01_BRD.md`, `Platform_02_PRD.md`
**Target Closure Day:** Day 20 (Sprint 3)

---

## Classification legend

| Label | Meaning |
|---|---|
| **AUTHORITATIVE FIXED REQUIREMENT** | Already fixed licensing behaviour |
| **TBD — DECISION REQUIRED** | Exact SKU quantities/prices and detailed technical metering/implementation evidence |
| **PROPOSED TEST POINT — NON-AUTHORITATIVE** | Exploratory entitlement scenarios |

---

## 1. Exact unresolved question

**TBD — DECISION REQUIRED:** Exact SKU quantities, prices, maintenance fees and contracts remain Founder-reserved. F-001–F-010/F-002A fix commercial policy; technical counting, measurement units/window, rounding, volume applicability, durable allowance/admission, maintenance separation, signed deployment identity/transfer/offline activation and certification remain OPEN.

---

## 2. Authoritative fixed licensing behaviour

Preserve these rules (**AUTHORITATIVE FIXED REQUIREMENT**):

* **30-day** advance expiry warning;
* from **7 days** before expiry, prominent dashboard banner;
* **15-day post-expiry grace** for finite software usage rights (`EXPIRED_GRACE_15D`); perpetual maintenance expiry does not suspend entitled monitoring;
* **polling continues during grace**;
* **after grace, polling stops** unless licence is renewed/extended;
* **historical data remains accessible**;
* licence administration remains available.

F-002/F-002A approved: up to 10% temporary overage for 30 calendar days, immediate warning, preserve existing monitoring, block additional activation above cap/after expiry until regularized; authorized commercial extension/audit only. Separate from usage-right expiry grace and security/capacity. Founder-approved Option A: Flow/Syslog daily unique accepted bytes UTC, warn/regularize and restrict additional activation above cap/after expiry, continue existing valid ingestion, no commercial drops/throttles. Physical metering, replay and acceptance evidence remain open.

---

## 3. Explicitly incorrect statements to avoid

* Treating approved 10%/30-day overage as implemented/certified or inventing SKU quantities/volume-meter semantics.
* Confusing commercial overage or perpetual maintenance expiry with expiring usage rights; preserve independent security/capacity and finite usage-right expiry grace.
* Freezing SKU quantities in this spike without commercial approval.

---

## 4. Evidence required

1. Licence key format/verification tests (mechanism **TBD** / candidate).
2. Expiry timeline tests proving: warning → banner → grace (polling on) → post-grace (polling off) → historical data + admin still available.
3. Boundary/rounding/concurrency, warning, expiry and restart/replay tests for approved 10%/30-day overage; preserve existing monitoring, authorized extension and block new activation when required.
4. Annual/perpetual maintenance separation, approved measurement/packaging, secure deployment-bound transfer/DR/offline activation, no HA double count and compatibility/migration/security evidence.

---

## 5. Closure criteria

Approved exact SKU/quantity matrix where required; approved policy recorded in existing Mother References; independently verified implementation, migration, security, failure/recovery and release evidence. **OPEN** until evidence, not closed by this policy amendment.

---

## 6. Affected components & mother documents

* **Components:** `core` licensing, `ui` licence admin.
* **Mother Documents:** `Platform_01`, `Platform_02`, `Platform_12`, `Platform_17`.

## 7. Dependencies

* Core API contracts; audit events for licence state changes.

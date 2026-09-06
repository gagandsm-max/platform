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
| **TBD — DECISION REQUIRED** | SKU quantities / overage policy details |
| **PROPOSED TEST POINT — NON-AUTHORITATIVE** | Exploratory entitlement scenarios |

---

## 1. Exact unresolved question

**TBD — DECISION REQUIRED:** Exact product SKUs/licensing quantities and commercial tiers beyond already-fixed behavioural rules; exact over-entitlement policy beyond the fixed “slight overage should warn rather than randomly drop devices” guidance.

---

## 2. Authoritative fixed licensing behaviour

Preserve these rules (**AUTHORITATIVE FIXED REQUIREMENT**):

* **30-day** advance expiry warning;
* from **7 days** before expiry, prominent dashboard banner;
* default **7-day grace** after expiry;
* **polling continues during grace**;
* **after grace, polling stops** unless licence is renewed/extended;
* **historical data remains accessible**;
* licence administration remains available.

Over-entitlement behaviour is **separate** and remains **TBD — DECISION REQUIRED** beyond: slight overage should generate clear commercial warnings rather than randomly dropping devices.

---

## 3. Explicitly incorrect statements to avoid

* Invented **110%** threshold as acceptance.
* Claims that over-capacity can **never** stop monitoring (expiry-after-grace **does** stop polling; overage policy is TBD and must not contradict expiry rules).
* Freezing SKU quantities in this spike without commercial approval.

---

## 4. Evidence required

1. Licence key format/verification tests (mechanism **TBD** / candidate).
2. Expiry timeline tests proving: warning → banner → grace (polling on) → post-grace (polling off) → historical data + admin still available.
3. Slight-overage warning behaviour (**PROPOSED TEST POINT — NON-AUTHORITATIVE** numeric thresholds until policy freezes).

---

## 5. Closure criteria

Approved SKU/quantity matrix + overage policy recorded in mother docs; implementation matches fixed expiry/grace rules above.

---

## 6. Affected components & mother documents

* **Components:** `core` licensing, `ui` licence admin.
* **Mother Documents:** `Platform_01`, `Platform_02`, `Platform_12`, `Platform_17`.

## 7. Dependencies

* Core API contracts; audit events for licence state changes.

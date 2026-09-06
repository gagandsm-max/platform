# Engineering Spike Record: P0-05 — Contractual DR RPO/RTO & Data-Loss Objectives

**Spike ID:** SPIKE-P0-05
**Status:** **OPEN**
**Owner:** Staff Product Architect & SRE Lead
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_01_BRD.md`, `Platform_03_Architecture.md`
**Target Closure Day:** Day 12 (Sprint 2)

---

## Classification legend

| Label | Meaning |
|---|---|
| **AUTHORITATIVE FIXED REQUIREMENT** | Mother-reference fixed |
| **PROPOSED COMMERCIAL/ENGINEERING OPTIONS — NOT APPROVED** | Discussion tiers only |
| **TBD — DECISION REQUIRED** | Contractual values unresolved |
| **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION** | Implementation not selected |

---

## 1. Exact unresolved question

**TBD — DECISION REQUIRED:** Final contractual DR RPO/RTO and data-loss objectives.

---

## 2. Authoritative fixed requirements

* DR capability is **mandatory in v1.0** (**AUTHORITATIVE FIXED REQUIREMENT**).
* Exact RPO/RTO remain **TBD — DECISION REQUIRED** until product/architecture freeze them (**AUTHORITATIVE FIXED REQUIREMENT** — do not invent contracts here).

---

## 3. Proposed options (not approved)

The following are **PROPOSED COMMERCIAL/ENGINEERING OPTIONS — NOT APPROVED**:

* Aggressive tier examples (e.g. ~15 min RPO / ~1 h RTO).
* Standard enterprise examples (e.g. ~1 h RPO / ~4 h RTO).
* Batch asynchronous examples (e.g. ~4 h RPO / ~8 h RTO).

Mention of S3, NFS, rsync, WAL shipping, or similar is **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION** — **not** selected by this document.

---

## 4. Evidence required

Site-loss drill measurements; inventory consistency; quantified unreplicated telemetry/config; runbook timings. Acceptance is to **measure candidate designs and present evidence** so product/architecture can freeze contractual RPO/RTO — not to assert them here.

---

## 5. Closure criteria

Mother-document update recording approved RPO/RTO/data-loss objectives after evidence review.

---

## 6. Affected components & mother documents

* **Components:** backup/restore, replication hooks, DR runbooks.
* **Mother Documents:** `Platform_01`, `Platform_03`, `Platform_12`, `Platform_16`.

## 7. Dependencies

* P0-02, P0-03.

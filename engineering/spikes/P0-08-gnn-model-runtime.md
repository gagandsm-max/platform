# Engineering Spike Record: P0-08 — GNN Model Architecture, Training & Runtime Compute

**Spike ID:** SPIKE-P0-08
**Status:** **OPEN**
**Owner:** Staff AI / Machine Learning Architect
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md`, `Platform_07_OSS_Catalog.md` (OSS-107, OSS-108 candidates)
**Target Closure Day:** Day 14 (Sprint 2)

---

## Classification legend

| Label | Meaning |
|---|---|
| **AUTHORITATIVE FIXED REQUIREMENT** | Mother-reference fixed |
| **PROPOSED TEST POINT — NON-AUTHORITATIVE** | Benchmark point only |
| **TBD — DECISION REQUIRED** | Production enablement unresolved |
| **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION** | Runtime not selected |

---

## 1. Exact unresolved question

**TBD — DECISION REQUIRED:** Exact GNN model architecture, training labels, inference latency, CPU/GPU footprint, and production enablement threshold.

---

## 2. Authoritative fixed requirements

* Deterministic / protocol / topology evidence remains authoritative (**AUTHORITATIVE FIXED REQUIREMENT**).
* GNN augments scoring / blast-radius; **not** causal proof (**AUTHORITATIVE FIXED REQUIREMENT**).
* **INCONCLUSIVE** is mandatory (**AUTHORITATIVE FIXED REQUIREMENT**).
* PyTorch / PyTorch Geometric are **CANDIDATE/P0 BENCHMARK** catalogue entries — not frozen production runtime (**AUTHORITATIVE FIXED REQUIREMENT**).
* Do **not** silently defer required v1 GNN production capability to v1.1 (**AUTHORITATIVE FIXED REQUIREMENT** — capability remains in agreed v1/30-day scope; exact runtime remains P0).

---

## 3. Explicitly not selected yet

Do **not** select ONNX, Triton, a specific PyTorch topology, or a GPU requirement as GA truth. Any such option is **CANDIDATE FOR SPIKE ONLY — OSS/ARCHITECTURE APPROVAL REQUIRED BEFORE SELECTION**.

---

## 4. Proposed benchmark points (non-authoritative)

Examples such as 10,000-node graphs, &lt;1.5s latency, or &lt;1.5 GiB RAM are **PROPOSED TEST POINT — NON-AUTHORITATIVE** only — not acceptance criteria.

---

## 5. Evidence required

1. Model/label definition and reproducibility notes.
2. CPU/GPU footprint and latency distributions across graph sizes.
3. Proof engine can emit **INCONCLUSIVE**.
4. Production enablement threshold proposal backed by evidence (still **TBD** until approved).

---

## 6. Closure criteria

Approved production GNN runtime/model/compute/enablement threshold recorded in mother docs / OSS catalogue as appropriate.

---

## 7. Affected components & mother documents

* **Components:** `analytics` / AI worker, Core RCA fusion.
* **Mother Documents:** `Platform_03`, `Platform_07`, `Platform_12`.

## 8. Dependencies

* Topology graph availability; OSS licence/packaging review for chosen runtime.

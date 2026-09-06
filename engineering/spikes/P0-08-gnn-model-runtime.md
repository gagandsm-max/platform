# Engineering Spike Record: P0-08 — GNN Model Architecture, Training & Runtime Compute

**Spike ID:** SPIKE-P0-08  
**Status:** **OPEN**  
**Owner:** Staff AI / Machine Learning Architect  
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md` §8, `Platform_07_OSS_Catalog.md` (OSS-107, OSS-108)  
**Target Closure Day:** Day 14 (Sprint 2)  

---

## 1. Exact Unresolved Question
What is the exact Graph Neural Network (GNN) model architecture, training data requirements, runtime inference latency, and compute footprint (CPU vs GPU) required to score root-cause hypotheses, and when is GNN enabled in production?

## 2. Why It Matters
Platform uses deterministic protocol and topology correlation as authoritative evidence. GNN is strictly a hypothesis and blast-radius scorer. It must not consume excessive CPU/RAM or introduce opaque non-deterministic behavior.

## 3. GA Blocking Impact
**HIGH / ARCHITECTURAL BOUNDARY.** Resolves whether PyTorch/PyG can be shipped in the standard container image or if an embedded ONNX Runtime with pre-trained weights is the GA deployment model.

## 4. Evaluated Alternatives (Open for Spike Testing)
* **Alternative A (Embedded ONNX Runtime in Java Core):** Pre-train model offline; run inference in-process via ONNX Runtime in Core Java (lightweight, zero Python runtime on Core).
* **Alternative B (Python Analytics Container Sidecar with PyTorch/PyG):** Microservice architecture using gRPC to call a dedicated Python AI worker.
* **Alternative C (Deterministic Graph Algorithms Only for v1.0):** NetworkX / Dijkstra / betweenness centrality for v1.0, with GNN deferred to v1.1.

## 5. Evidence Required
1. Sizing benchmark: Memory and CPU consumption of inference engine on 10,000-node graph.
2. Latency measurement: Root-cause scoring completed in <2.0 seconds.
3. Proof that the engine supports and emits `INCONCLUSIVE` when graph entropy is high.

## 6. Test & Benchmark Methodology
Construct complex topology graphs with multi-hop dependencies and cascaded synthetic alert storms; benchmark inference speed and accuracy; verify failure modes.

## 7. Acceptance Criteria
* Inference completed in <1.5s for 10,000-node operational graph.
* Container RAM footprint <1.5 GiB in CPU-only mode.
* Unconditional support for `INCONCLUSIVE` classification.

## 8. Affected Components & Mother Documents
* **Components:** `core`, `analytics`.
* **Mother Documents:** `Platform_03_Architecture.md`, `Platform_07_OSS_Catalog.md`, `Platform_11_Sizing_Capacity_Storage_Master.md`.

## 9. Dependencies
* Requires topology graph generation benchmarks.

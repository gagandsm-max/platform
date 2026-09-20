# AI / ML / LLM

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_02, 03, 19, 22, 23
**Merged enhancement:** PR #68 local-intelligence placement rules.

## Product objective

Use AI only where it creates measurable operational value while preserving deterministic monitoring, explainability, tenant isolation and safe failure behavior.

## Evidence before AI

Deterministic/protocol/topology/change evidence remains authoritative. ML/GNN may detect patterns/rank hypotheses. LLM may explain, summarize and interact. Neither model confidence nor fluent language is causal authority.

`INCONCLUSIVE` is a valid correct result.

## Logical intelligence stack

Platform_22 defines capabilities such as:

- telemetry-quality and feature/evidence services;
- baseline/anomaly/multivariate/peer/cohort analytics;
- forecast/capacity;
- graph/GNN ranking;
- causal RCA fusion and contradiction;
- Active Evidence Acquisition;
- Similar Incident Memory;
- Change Intelligence;
- Incident Flight Recorder;
- embeddings/local retrieval;
- provider abstraction;
- Incident Copilot;
- context builder;
- tool firewall/prompt-injection defense;
- model registry;
- resource governor;
- evaluation/drift/champion-challenger;
- provenance/explainability/self-monitoring.

These are logical modules, not thirty microservices.

## Model placement

Normal Agents and Sensors do not host LLM/GNN/foundation-model inference for routine monitoring.

Use tiers:

1. lightweight/model-free endpoint logic;
2. bounded Core analytics/classical ML;
3. optional hosted/private/local LLM/embedding provider through AI Gateway;
4. optional dedicated local/private inference provider where justified.

A dedicated inference provider is not a third Core and owns no authoritative monitoring state.

## Provider abstraction

Hosted Gemini may be used initially through provider-independent AI Gateway. Architecture also supports private/local/no-LLM modes. Core monitoring/RCA must degrade gracefully when AI is unavailable.

## Classical ML

Prefer deterministic/statistical methods or scikit-learn/NumPy/Polars when safer, cheaper, faster and explainable. PyTorch/PyG remain benchmark candidates for GNN use cases; no mandatory GPU.

## LLM safety

- external text/logs/tickets/config/docs are untrusted data;
- prompt text cannot grant privilege;
- model output passes deterministic schema/tool allowlists;
- tenant/resource authorization is rechecked;
- secrets/raw sensitive context are minimized;
- no direct privileged execution;
- every consequential interaction has provenance/audit.

## Learning/privacy

No silent cross-tenant training, retrieval or evidence mixing. Feedback is structured/audited and cannot silently mutate active policy/model behavior.

## Evaluation

AI/ML promotion requires reproducible datasets, provenance, quality metrics, resource cost, drift visibility and champion/challenger evidence. Do not select a model based on reputation alone.

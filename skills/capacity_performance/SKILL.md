<!-- generated from project_agents/skills/capacity_performance/SKILL.md sha256:ede1584fda29862715edf198446c1e9d1616a0c39e81cdae68b05c26f770de03 -->

---
name: capacity_performance
description: Hot-path capacity, latency, lag, cardinality and resource-budget evidence for Core, Sensor, UI and storage protection.
---

# capacity_performance

## Purpose
Require measured capacity evidence instead of anecdote.

## Use when
Polling, ingestion, query, UI, HA replay or sizing changes.

## Required procedure
1. Identify the hot path and budget.
2. Measure before/after with bounded load.
3. Include cardinality and 80% storage effects.
4. Do not invent universal SLO percentages.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `capacity_performance` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

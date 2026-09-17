---
name: distributed_systems
description: Design and change Core/Sensor distributed behavior: queues, backpressure, timeouts, consistency, failover and split-brain avoidance without reopening two-Core P0 designs.
---

<!-- generated from project_agents/skills/distributed_systems/SKILL.md sha256:6806eb734954ce413d2503e47e70cbaa525df5e8c1a4e57ab1575d80c8de0607 -->

# distributed_systems

## Purpose
Keep Platform distributed behavior bounded, fail-closed and evidence-backed.

## Use when
Changing coordination, queues, leadership, replay, or cross-node timing.

## Required procedure
1. Identify the consistency/timeout/ownership invariant.
2. Trace failure and retry classification.
3. Prove backpressure and poison handling.
4. Do not invent unresolved two-Core fencing.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `distributed_systems` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

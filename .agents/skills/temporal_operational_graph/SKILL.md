---
name: temporal_operational_graph
description: Temporal operational graph, topology provenance/confidence and relationship history.
---

<!-- generated from project_agents/skills/temporal_operational_graph/SKILL.md sha256:f4584cd4941f188d4e156e9abb25880cf2f519149da976549f6f5df23f502770 -->

# temporal_operational_graph

## Purpose
Change graph edges with provenance, not guessed topology.

## Use when
Entity relationships, topology history, or RCA subgraphs.

## Required procedure
1. Every edge needs provenance/confidence.
2. Do not select a new graph database by default.
3. History is not silently current root cause.
4. Conflicting identifiers require reconciliation.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `temporal_operational_graph` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

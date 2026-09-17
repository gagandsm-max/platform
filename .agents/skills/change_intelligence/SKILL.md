---
name: change_intelligence
description: Change Intelligence: external/provider/change evidence as supporting context with explicit non-causality.
---

<!-- generated from project_agents/skills/change_intelligence/SKILL.md sha256:c50b9bbe76a89f219c3f336411a56322177e4008b1131388ff06260c387a8e6f -->

# change_intelligence

## Purpose
Attach change evidence without temporal-proximity causation.

## Use when
Change feeds, deploy markers, or provider incidents.

## Required procedure
1. Record source, time, and scope.
2. Provider text is not proof.
3. Contradictions remain visible.
4. Do not auto-close incidents from change text.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `change_intelligence` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

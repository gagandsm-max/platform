<!-- generated from project_agents/skills/monitoring_work_units/SKILL.md sha256:03cb3b465024141c8fb0b2824fb1190c1c3a250d3d1cba1a18f71c40f3c11ddc -->

---
name: monitoring_work_units
description: Monitoring work-unit placement, Sensor assignment, capacity impact and locality constraints.
---

# monitoring_work_units

## Purpose
Place work on Sensors without silent overload.

## Use when
Placement policy, work-unit schema, or Sensor assignment.

## Required procedure
1. Honor locality/capacity and single-winner placement.
2. Preview licence/Sensor impact before activation.
3. Newly found entities are not silently licensed.
4. Record placement evidence.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `monitoring_work_units` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

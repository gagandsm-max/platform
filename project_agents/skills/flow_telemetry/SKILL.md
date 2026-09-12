---
name: flow_telemetry
description: Flow export collection, template/replay/drop counters, cardinality control and Sensor isolation from flow floods.
---

# flow_telemetry

## Purpose
Ingest flow without starving polling/NCM or exploding cardinality.

## Use when
NetFlow/IPFIX/sFlow or flow worker changes.

## Required procedure
1. Bound decode/queue CPU.
2. Preserve template/replay/drop counters.
3. Protect 80% disk and tenant isolation.
4. Normalize without using IP as sole identity.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `flow_telemetry` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

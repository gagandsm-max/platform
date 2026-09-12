---
name: sensor_edge
description: Sensor process/runtime mechanics: scheduling, worker pools, leases/fencing, durable spool, uploader/replay, resource lifecycle and Sensor self-observability.
---

# sensor_edge

## Purpose
Change Sensor edge mechanics without starving collection or violating 80% storage protection.

## Use when
Sensor binary, workers, spool, uploader, leases, or Sensor health.

## Required procedure
1. Separate protocol semantics from runtime mechanics.
2. Bound pools, queues, disk and CPU.
3. Prove spool/replay/ack fail-closed behavior.
4. Measure impact on polling and event paths.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `sensor_edge` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

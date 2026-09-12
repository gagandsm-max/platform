<!-- generated from project_agents/skills/go_runtime/SKILL.md sha256:13791a924ff0434503598b69a1d570109369663c8b689816249dc95f7115e402 -->

---
name: go_runtime
description: Go Sensor/Agent implementation: context/cancellation, bounded concurrency, durable files, and fail-closed ack/offset behavior.
---

# go_runtime

## Purpose
Change Go code with Platform concurrency and durability invariants.

## Use when
Editing `sensor/` or `agent/` Go code.

## Required procedure
1. Preserve context timeouts and bounded goroutines.
2. Do not ack before durable success.
3. Add focused Go tests for the invariant.
4. Keep OSS within the catalogue.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `go_runtime` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

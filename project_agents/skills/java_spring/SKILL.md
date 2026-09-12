---
name: java_spring
description: Java 21 / Spring Core implementation: transactions, validation, actuator boundaries, and fail-closed consumers.
---

# java_spring

## Purpose
Change Core Java with tenant, transaction and consumer safety.

## Use when
Editing `core/` Java/Spring.

## Required procedure
1. Keep tenant scope in every query/command.
2. Validate at the API/event boundary.
3. Consumers must not advance offsets on poison without parking.
4. Use Java 21 only; no new framework.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `java_spring` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

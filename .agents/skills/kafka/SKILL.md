---
name: kafka
description: Internal Kafka transport, consumer fail-closed ACK/offset, poison parking and lag evidence.
---

<!-- generated from project_agents/skills/kafka/SKILL.md sha256:43bc23de4acf109d0e813a2757e6d9cf0f74e198fcae8f1e2f42f964cb13ca6b -->

# kafka

## Purpose
Change Kafka as internal transport only.

## Use when
Topics, producers, consumers, or poison handling.

## Required procedure
1. Kafka stays internal; no external Kafka API.
2. Fail-closed offsets on poison/retry exhaustion.
3. Do not invent two-Core Kafka fencing.
4. Bound payload logging and redact secrets.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `kafka` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

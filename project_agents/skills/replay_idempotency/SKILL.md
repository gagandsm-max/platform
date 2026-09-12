---
name: replay_idempotency
description: Replay, dedup, late-data and exactly-once-effect evidence across Sensor spool, Kafka and datastores.
---

# replay_idempotency

## Purpose
Prove replay does not duplicate or drop authoritative effects.

## Use when
Uploader replay, consumer retry, or idempotency keys.

## Required procedure
1. Identify the idempotency key and watermark.
2. Prove duplicate and gap cases.
3. Distinguish transport retry from business effect.
4. Record UNVERIFIED if exactly-once is not evidenced.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `replay_idempotency` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

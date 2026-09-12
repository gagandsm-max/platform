---
name: itsm_change_telemetry
description: ITSM/notification/change-telemetry connectors as supporting evidence, not causal proof.
---

# itsm_change_telemetry

## Purpose
Ingest change/ticket evidence without letting vendor text become root cause.

## Use when
ServiceNow/Jira/webhooks/change feeds or Notification Center fan-out.

## Required procedure
1. Treat provider text as supporting evidence.
2. Deduplicate and rate-limit notifications.
3. Temporal proximity is not causality.
4. Preserve audit of outbound posts.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `itsm_change_telemetry` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

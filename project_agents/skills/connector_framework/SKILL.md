---
name: connector_framework
description: Outbound connector framework: auth, rate limit, retry, idempotency, durable outbound and certification.
---

# connector_framework

## Purpose
Add or change connectors without vendor IDs overriding Platform authority.

## Use when
SaaS/ITSM/webhook/carrier connectors.

## Required procedure
1. Normalize into Platform-owned contracts.
2. Least-privilege credentials; redact logs.
3. Durable outbound with bounded retry.
4. Certification/provenance required before enablement claims.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `connector_framework` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

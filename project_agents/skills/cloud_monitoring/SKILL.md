---
name: cloud_monitoring
description: Cloud-account/target monitoring, OpenTelemetry import boundaries and vendor-ID normalization.
---

# cloud_monitoring

## Purpose
Ingest cloud/OTel evidence without vendor identifiers overriding tenant/entity authority.

## Use when
AWS/Azure/GCP target monitoring or OTel receivers.

## Required procedure
1. Normalize into Platform contracts.
2. Credentials least-privilege and redacted.
3. Collector health ≠ target failure.
4. New SDKs need OSS/security review.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `cloud_monitoring` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

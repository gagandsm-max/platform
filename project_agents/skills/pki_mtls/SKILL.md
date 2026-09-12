---
name: pki_mtls
description: PKI, mTLS enrollment, certificate identity and untrusted-cert rejection.
---

# pki_mtls

## Purpose
Keep Sensor/Agent/Core identity certificate-based.

## Use when
Certs, enrollment, mTLS, or trust bundles.

## Required procedure
1. Reject missing/untrusted client certs.
2. Never disable TLS validation to get green tests.
3. Rotation/expiry is observable.
4. Do not commit generated keys.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `pki_mtls` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

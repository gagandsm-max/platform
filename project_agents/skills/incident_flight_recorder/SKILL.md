---
name: incident_flight_recorder
description: Incident Flight Recorder and Similar Incident Memory: bounded, tenant-scoped, advisory historical evidence.
---

# incident_flight_recorder

## Purpose
Capture incident evidence without always-on DPI or silent cause reuse.

## Use when
Flight Recorder orchestration, similarity search, or packet-evidence bounds.

## Required procedure
1. Bound duration/bytes/packets/rate.
2. Tenant-scoped, encrypted, audited.
3. Similarity is advisory, not current RCA.
4. Capture must not starve polling/NCM.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `incident_flight_recorder` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

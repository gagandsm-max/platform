---
name: secrets_handling
description: Credential Profiles, connector secrets, CI credentials, redaction and support-bundle safety.
---

# secrets_handling

## Purpose
Stop secrets from entering git, logs, or evidence bundles.

## Use when
Any secret, token, key, or support bundle path.

## Required procedure
1. Use Credential Profiles/secret service.
2. Redact nested causes and structured logs.
3. Evidence is pointer-based and bounded.
4. Scan diffs for accidental secret material.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `secrets_handling` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

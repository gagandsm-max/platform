<!-- generated from project_agents/skills/oauth_oidc_clients/SKILL.md sha256:b8ee3f270e40254a500da8ae60cbde66793884e299d7aa8b51eb8cb818bbf4b9 -->

---
name: oauth_oidc_clients
description: OAuth/OIDC client integrations, token handling, redirect safety and secret storage.
---

# oauth_oidc_clients

## Purpose
Integrate identity protocols without storing tokens insecurely.

## Use when
OAuth/OIDC client credentials, callbacks, or token refresh.

## Required procedure
1. Store tokens in the secret service, never logs.
2. Validate state/nonce/redirect.
3. Least privilege scopes.
4. Negative tests for token leakage.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `oauth_oidc_clients` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

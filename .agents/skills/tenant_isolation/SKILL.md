<!-- generated from project_agents/skills/tenant_isolation/SKILL.md sha256:abd3e839c945ba20042fe9513949d8de06bc36d9e3738dfb27c882a763f56e7e -->

---
name: tenant_isolation
description: Server-side tenant isolation, RBAC/resource scope and cross-tenant negative cases.
---

# tenant_isolation

## Purpose
Keep every query/command/tenant-scoped.

## Use when
Any API, query, export, search, or AI context that touches tenant data.

## Required procedure
1. Enforce tenant in server authorization, not UI hiding.
2. Test unauthorized and cross-tenant access.
3. No silent cross-tenant learning.
4. Resource scope must be server-checked.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `tenant_isolation` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

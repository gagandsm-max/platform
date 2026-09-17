---
name: postgresql
description: PostgreSQL control-state schema, transactions, tenant isolation and backup/restore of authoritative data.
---

<!-- generated from project_agents/skills/postgresql/SKILL.md sha256:997b1b9e2041c785a328e86437408c3f86f77bd4a436e3a42ecb599391035c3f -->

# postgresql

## Purpose
Change PostgreSQL as the transactional authority.

## Use when
Flyway/schema, control APIs, or PG backup/restore.

## Required procedure
1. Keep tenant predicates on every query.
2. Assess retry/partial migration; do not assume down migrations.
3. Do not invent two-Core PG fencing.
4. Never log secrets or raw customer payloads.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `postgresql` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

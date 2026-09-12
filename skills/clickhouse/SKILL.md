<!-- generated from project_agents/skills/clickhouse/SKILL.md sha256:167df7f30dd6451e2e09495c8a308b689755e6a9317976f1b37965830c10a228 -->

---
name: clickhouse
description: ClickHouse historical analytics, insert idempotency, retention and lag without treating CH as control state.
---

# clickhouse

## Purpose
Change ClickHouse as high-volume history, not authority.

## Use when
CH tables, consumers, retention, or analytics queries.

## Required procedure
1. CH is not transactional authority.
2. Inserts must be retry-safe/idempotent where required.
3. Offset must not advance on insert failure.
4. Honor 80% storage protection.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `clickhouse` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

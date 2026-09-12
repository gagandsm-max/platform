<!-- generated from project_agents/skills/customer_value_mttr/SKILL.md sha256:db2c1a84c1e2068e16830203bee5429b0f482882b28f0191362d1feb1cc60860 -->

---
name: customer_value_mttr
description: MTTR/value analytics with cohort methodology, data quality and no fabricated savings percentages.
---

# customer_value_mttr

## Purpose
Publish value metrics that expose assumptions.

## Use when
MTTR dashboards, value reports, or cohort definitions.

## Required procedure
1. Expose sample, exclusions, data quality.
2. Do not fabricate generic customer savings.
3. Platform_19 governs OIV metrics.
4. Keep tenant isolation on analytics queries.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `customer_value_mttr` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

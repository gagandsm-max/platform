<!-- generated from project_agents/skills/syslog_traps/SKILL.md sha256:0a510b254136c67e5c2cb7e80e026531393bb85c0be4662704322e0e999d6394 -->

---
name: syslog_traps
description: Syslog and SNMP trap ingestion, source attribution, storm control and evidence-quality parsing.
---

# syslog_traps

## Purpose
Ingest asynchronous events without notification floods or false DOWN.

## Use when
Syslog/trap listeners, parsers, or storm control.

## Required procedure
1. Rate-limit and bound parse cost.
2. Preserve source/time/severity.
3. Deduplicate notification generation.
4. Do not treat missing syslog as device-down by itself.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `syslog_traps` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

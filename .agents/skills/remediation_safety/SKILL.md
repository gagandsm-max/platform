---
name: remediation_safety
description: Controlled remediation: deterministic actions, maker-checker, blast-radius, rollback and no LLM privileged execution.
---

<!-- generated from project_agents/skills/remediation_safety/SKILL.md sha256:c22487a7956626b943a21ad9aa585fd3a54862464d24803a99a6aafa40196a4f -->

# remediation_safety

## Purpose
Change config/remediation without unsafe automation.

## Use when
NCM jobs, privileged commands, or auto-remediation proposals.

## Required procedure
1. Versioned deterministic actions only.
2. Preconditions, approval, audit, post-validation.
3. LLM output cannot execute privileged actions.
4. Rollback/fail-safe must be documented.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `remediation_safety` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

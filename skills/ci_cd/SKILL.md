<!-- generated from project_agents/skills/ci_cd/SKILL.md sha256:297e93916ad4d60b693170657ede95eaae98ed13116a2e4ee6e06639f026398a -->

---
name: ci_cd
description: GitHub Actions/CI wiring, exact-head evidence, and no test weakening to obtain green.
---

# ci_cd

## Purpose
Change CI without making GitHub Actions an architecture freeze.

## Use when
Workflows, required checks, or CI scripts.

## Required procedure
1. Exact-head CI is the merge evidence.
2. Do not skip isolation checks except documented PLATFORM_CI.
3. Agent-pack filters must include generated surfaces.
4. Selecting GitHub does not freeze CI architecture.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `ci_cd` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

---
name: threat_modelling
description: Threat-model trust boundaries, privileged actions, Device Packs, AI tools and update channels before implementation.
---

<!-- generated from project_agents/skills/threat_modelling/SKILL.md sha256:9d44ffeb4203b20d194c76dc7408cc55b38f5a79a146d148071f14736de47cd8 -->

# threat_modelling

## Purpose
Identify abuse cases before writing code.

## Use when
New trust boundary, protocol, privileged action, or AI tool.

## Required procedure
1. Name assets, actors, trust boundaries.
2. Include nested exception/log leakage.
3. Require negative tests for new boundaries.
4. Stop on unresolved P0 security TBD.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `threat_modelling` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

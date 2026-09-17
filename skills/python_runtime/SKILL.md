---
name: python_runtime
description: Approved Python use for AI/analytics/NCM workers: isolation, no silent host installs, and catalogue-constrained dependencies.
---

<!-- generated from project_agents/skills/python_runtime/SKILL.md sha256:b33d6793df25f143386ee1ea305781b1c8c6163a30b4b25b3eea137af849064b -->

# python_runtime

## Purpose
Change Python only where Platform already approved it.

## Use when
NCM workers, AI gateway helpers, or approved analytics.

## Required procedure
1. Confirm Python is allowed for this path.
2. Do not install project runtimes on the developer host.
3. Keep secrets out of process environment logs.
4. Test fail-closed job behavior.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `python_runtime` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

---
name: bounded_implementation
description: Apply the smallest defensible change inside the approved scope with explicit non-goals and no unrelated refactor.
---

<!-- generated from project_agents/skills/bounded_implementation/SKILL.md sha256:bed49fba60918fc6b22d456297af997131635100259d51bb881433bc9e9cfc71 -->

# bounded_implementation

## Purpose
Implement the smallest defensible change after the requirement/root cause is known.

## Required procedure
1. State exact files/functions and approved behavior change.
2. State explicit non-goals before editing.
3. Preserve existing contracts unless the governing requirement explicitly changes them.
4. Add or update only tests required to prove the changed invariant.
5. Inspect the final diff for unrelated refactor, dependency, configuration and formatting churn.

## Guardrails
- No direct writes to `main`.
- No test/auth/tenant/durability/security weakening.
- No new dependency, database, service, model or vendor commitment without approved review.
- Writer cannot self-certify completion.
- Once IMPLEMENTATION has frozen `scope[]`, do not silently write outside it. If a genuinely required file is discovered outside frozen scope, return through DIAGNOSIS/ROUTING, record the scope amendment, then resume.

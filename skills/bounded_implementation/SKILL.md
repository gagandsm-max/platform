---
name: bounded_implementation
description: Apply the smallest defensible change inside the approved scope with explicit non-goals and no unrelated refactor.
---

<!-- generated from project_agents/skills/bounded_implementation/SKILL.md sha256:17e53fbc23a6cf316be8afd8a433095f74f262869ae01fe49a5c1651d7ccb4ef -->

# bounded_implementation

## Purpose

Implement the smallest defensible change after the exact root cause or governing requirement is known—without unrelated refactor, dependency sprawl or environment workarounds.

## Use when

Any IMPLEMENTATION-phase product or control-plane write after DIAGNOSIS has frozen (or is freezing) scope. Required for `platform-minimal-change-engineer` and every Cursor CLI write task that mutates repository files under a task manifest.

## Authoritative inputs

Always read:

1. Exact root-cause statement or requirement IDs from the task (not chat paraphrase alone)
2. Task manifest `scope[]`, `nonGoals[]`, `stopConditions[]`, `writerTool`, `verifierTool`, `gatekeeper`
3. Governing Mother Reference sections discovered from `Platform_00` for the change class
4. Existing contracts/tests owning the invariant being changed
5. `project_agents/tool_authority.json` when tool/writer posture is in play

If root cause/requirement is still ambiguous, stop and return to `root_cause_first` / DIAGNOSIS—do not “explore by rewriting”.

## Invariants

- Exact files/functions in scope; no silent expansion after IMPLEMENTATION freezes `scope[]`.
- Explicit non-goals recorded before the first edit.
- No unrelated refactor, formatting churn, renames or “while here” cleanup.
- No new OSS dependency, database, service, model or vendor without approved catalogue/security review.
- No host-environment workaround (laptop installs of Java/Go/Node/DB, disabling TLS, skipping Docker Compose policy).
- Minimum coherent patch that preserves rollback/compatibility posture of the touched contracts.
- Writer cannot self-certify `REALITY_GATE` / `MERGE_READY` / `POST_MERGE_VERIFY` / `COMPLETE`.

## Required procedure

1. Restate the exact root cause or requirement in one paragraph with evidence pointers.
2. List exact files/functions in declared scope and the approved behavior delta.
3. State non-goals (adjacent refactors, drive-by lint, dependency bumps, doc rewrites outside reservation).
4. Implement the minimum coherent patch—prefer extending an existing seam over new abstractions.
5. Add or update only the negative/positive tests required to prove the changed invariant.
6. Assess rollback/compatibility: callers, schemas, feature flags, mixed-version Sensor/Core/Agent, migration reservation conflicts.
7. Diff-review the final patch for scope creep, secret leakage, test weakening and unrelated churn.
8. Package evidence before claiming LOCAL_VALIDATION readiness; hand to independent `verifierTool`.

## Negative / failure cases

- Patching a symptom while the evidenced failure mechanism remains open.
- Editing files outside frozen `scope[]` without DIAGNOSIS/ROUTING amendment.
- “Fixing” tests by weakening assertions, skipping auth/tenant checks or marking failures xfail without authority.
- Introducing a dependency to avoid understanding the existing code path.
- Local host installs or Compose shortcuts that will not reproduce in CI/Docker.
- Architecture contradiction with Mother Reference “resolved” by inventing a convenient interpretation.

## Security / tenant implications

Do not bypass or dilute tenant isolation, authn/authz, mTLS validation, secret handling or durability/ack fail-closed behavior to make the patch smaller. Security-negative coverage for the touched boundary is part of the minimum coherent change when the invariant is security-relevant.

## Required evidence

- Exact SHA/branch/worktree and scope list.
- Diffstat limited to declared files (or recorded scope amendment).
- Commands/test identifiers with pass/fail counts for the changed invariant.
- Rollback/compatibility notes.
- Explicit record of tests not run.
- Claim labelling via `evidence_bundle` types; no summary-only completion claim.

## Stop conditions

Stop when:

- Root cause/requirement is not exact.
- Required file lies outside frozen scope and no amendment is authorized.
- Mother documents conflict and precedence is unresolved.
- Change needs new OSS/vendor/runtime not yet approved.
- Only path forward is an environment workaround or test weakening.
- Writer is being asked to self-certify merge readiness.

## Anti-patterns

- Large opportunistic refactors under a bug-fix ticket.
- Copying proprietary ThoughtData/Enterprise360/NetAI code.
- Drive-by markdown/docs unrelated to the invariant.
- Expanding P2 Skill/doc depth while assigned a runtime bug (or the reverse) without a new task.
- Parallel writers on the same checkout.

## Handoff / output

Return: root-cause pointer; scope/non-goals; files changed; tests run/not run; rollback notes; residual risks; next owner = independent verifier. If scope must grow, return through DIAGNOSIS/ROUTING with an amendment—do not silently continue.

## Non-ownership

Does not own product architecture decisions, Mother Reference edits without change control, certification gates, or multi-checkout writes. Does not replace `migration_safety`, `security_negative_testing` or domain Skills when those risks are present—it constrains how they are applied.

---
name: remediation_safety
description: Privileged remediation with authorization, blast-radius, dry-run, rollback and audit—never direct LLM execution.
---

<!-- generated from project_agents/skills/remediation_safety/SKILL.md sha256:6af772c9daa9852259923908872f3255529a7abbb12fb2091d652fb45ecc9ae1 -->

# remediation_safety

## Purpose

Run controlled remediation as versioned deterministic actions with authorization, preconditions, blast-radius caps, audit and post-action verification—never as free-form LLM device control.

## Use when

Remediation playbooks, NCM apply, automated or semi-automated fix actions, dry-run/rollback paths, or any agent proposing privileged changes. Required for `platform-ncm-remediation-engineer` and OIV controlled remediation (`Platform_19`).

## Authoritative inputs

Always read:

1. `Platform_19` controlled-remediation requirements
2. `Platform_18` threat model for privileged actions
3. Existing remediation/NCM action catalogue on the exact head
4. RBAC/JIT/approval workflows in product docs
5. `device_control_protocols` Skill for protocol transport safety

If dry-run, automated rollback or maker-checker is not implemented for an action, mark `UNVERIFIED` and refuse to claim it.

## Invariants

- Authorization + tenant/resource scope before every action; LLM text cannot grant privilege.
- Approval where Mother requires maker-checker / dual control.
- Dry-run when the action type supports it; skip only with explicit recorded waiver.
- Blast radius: target count, concurrency, geographic/site scope capped.
- Idempotency: safe retry or explicit non-idempotent warnings.
- Preconditions checked (state, maintenance, health)—fail closed if unmet.
- Rollback plan required for mutating actions; if automation missing, human escalation path is mandatory.
- Rate limits prevent remediation storms.
- Failure detection during/after action; abort rules documented.
- Audit: actor, approval, inputs, outputs, evidence IDs.
- Post-action verification (polls/diagnostics) before success claim.
- Human escalation on uncertainty—not silent continue.

## Required procedure

1. Inventory the action against the allowlisted catalogue; reject ad-hoc commands.
2. Verify authz, approvals and reason codes for this tenant/resource.
3. Run dry-run if available; capture predicted blast radius.
4. Check preconditions; abort if UNKNOWN collector health would make verification impossible.
5. Execute with rate limits; stream audit/evidence.
6. Detect mid-flight failure; trigger rollback or escalate per runbook—do not invent rollback.
7. Post-verify with independent evidence; distinguish “command accepted” from “intent achieved”.
8. Record outcome; never mark MERGE_READY from remediation success alone.

## Negative / failure cases

- LLM directly emitting SSH commands as remediation.
- Applying to entire site when approval was for one device.
- Success on ACK without post-verify.
- Retrying non-idempotent reboot in a loop.
- Remediating during collector outage masking results.

## Security / tenant implications

Privileged. Cross-tenant remediation prohibited. Secrets in templates redacted. Break-glass still audited. Export of remediation transcripts RBAC-gated.

## Required evidence

- Allowlist/action version
- Authz/approval records
- Dry-run/blast-radius artefacts
- Pre/post verification results
- Rollback or escalation record
- Exact SHA; `UNVERIFIED` automation gaps

## Stop conditions

Stop when:

- Action not allowlisted
- Required approval missing
- Blast radius unbounded
- Rollback required but `UNVERIFIED` and no human owner
- Post-verification impossible yet success is demanded

## Anti-patterns

- “Autopilot fix all similar alerts”
- Disabling dry-run permanently
- Writer self-certifying remediation safety
- Treating Similar Incident Memory as auto-remediation authority

## Handoff / output

Return: action evidence pack; verification; residual risk; nextOwner (NCM/AppSec verifier). Not MERGE_READY alone.

## Non-ownership

Does not own alert policy design, IFR capture, or invent autonomous healing. Owns remediation execution safety discipline.

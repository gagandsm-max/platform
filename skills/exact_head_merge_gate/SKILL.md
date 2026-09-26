---
name: exact_head_merge_gate
description: Gate review and merge claims on the exact current head, complete required CI/evidence and independent verification; drift invalidates stale evidence.
---

<!-- generated from project_agents/skills/exact_head_merge_gate/SKILL.md sha256:56c3fb32e2e3dc0b6a5cfcadc3e23e0e0062025262d99cad1076f651b182558f -->

# exact_head_merge_gate

## Purpose

Prevent stale, writer-certified or green-CI-only evidence from authorizing review, merge or completion. Gates bind to an **immutable audited SHA** and must be rebuilt after every push.

## Use when

Any claim of SOURCE_REVIEW pass, CI_VALIDATION, REALITY_GATE, MERGE_READY, POST_MERGE_VERIFY or COMPLETE; any PR merge authorization; any reuse of prior audit bundles. Required for `platform-reality-checker`, gatekeepers and verifiers.

## Authoritative inputs

Always read:

1. Exact GitHub PR/base/head SHAs (`gh`, git fetch)—not chat memory
2. Task manifest `writerTool`, `verifierTool`, `gatekeeper`, `exactHead`, status
3. Required CI checks and evidence pointers for this task class (migration, security, runtime)
4. `project_agents/tool_authority.json` (writer cannot self-certify)
5. Historical failed audits retained for the PR/lane (do not delete inconvenient failures)

## Invariants

- Evidence is valid only for the **exact current head**. Every push invalidates prior gate evidence.
- Writer cannot self-certify REALITY_GATE / MERGE_READY / POST_MERGE_VERIFY / COMPLETE.
- Independent audit/verifier is required; GitHub Copilot is never sole certification evidence.
- Zero unresolved P0/P1 applicable to the change class before MERGE_READY.
- `mergeable=true` / green required checks alone are **not** certification.
- Migration and security gates applicable to the diff must be current on the same SHA.
- Guarded merge only under gatekeeper authority; post-merge must verify exact `main` SHA.
- Historical failed audits are retained; success does not erase prior failure records.

## Required procedure

1. Fetch authoritative `main`, PR base and exact head; record full 40-character SHAs.
2. Confirm the reviewed diff equals the complete exact-head diff (no hidden commits).
3. Confirm all required CI/evidence URLs/runs belong to that exact head—not an ancestor.
4. Confirm `writerTool ≠ verifierTool` and `writerTool ≠ gatekeeper`; reject writer self-certification.
5. Re-check migration_safety / security_negative_testing / runtime / product gates per risk; require zero unresolved P0/P1.
6. If head/base/main moved since evidence collection, mark stale, return to rebaseline and rebuild the bundle.
7. Only gatekeeper-authorized merge; after merge, verify the landed `main` SHA and run POST_MERGE_VERIFY evidence.
8. Preserve failed audit artefacts alongside the passing bundle for the lane.

## Negative / failure cases

- Reusing CI from SHA N-1 after a “typo fix” push.
- Writer marking MERGE_READY on their own implementation lane.
- Treating Copilot review summary as certification.
- Ignoring open P0/P1 because “CI is green”.
- Merging on `mergeable=true` without independent exact-head audit.
- Skipping post-merge exact-main verification.
- Deleting or rewriting historical failed audit notes.

## Security / tenant implications

Gate evidence must not embed secrets. Cross-tenant or credentialed runtime proof follows `secrets_handling`. Unauthorized merge or force-push to protected branches is a security incident, not a process shortcut.

## Required evidence

- Immutable audited SHA (exactHead) and authoritative main at decision time
- CI run IDs bound to that SHA
- Independent verifier/gatekeeper identities
- Migration/security/runtime gate results or explicit N/A with reason
- Unresolved P0/P1 count = 0 (or BLOCKED)
- Post-merge main SHA verification when COMPLETE claimed
- Stale-invalidations log if pushes occurred mid-review
- Claim types via `evidence_bundle`—no summary-only certification

## Stop conditions

Stop merge/certification when:

- Head drifted mid-gate
- Writer equals verifier or gatekeeper for certifying states
- Required CI missing on exact head
- Applicable P0/P1 remains open
- Migration/security evidence is UNVERIFIED for an in-scope risk

## Anti-patterns

- Green-CI-only certification
- “LGTM” without SHA pins
- Amending/force-pushing shared history to hide failed audits
- Certifying from chat summaries
- Parallel writers collapsing verifier independence

## Handoff / output

Return: SHA pins; CI binding; independence proof; residual risks; gate decision (PASS/BLOCKED/REJECTED); nextOwner. On drift, hand back to REBASELINE with stale markers.

## Non-ownership

Does not implement product fixes, weaken required checks, grant Copilot certification authority, or replace domain Skills. Owns the gate discipline only.

---
name: evidence_bundle
description: Package typed SOURCE_FACT, EXECUTION_EVIDENCE, CI_EVIDENCE, INFERENCE and UNVERIFIED claims with reproducible references.
---

# evidence_bundle

## Purpose

Produce auditable, reproducible evidence for Platform engineering claims without overstating what was proved. Summary prose alone never certifies a gate.

## Use when

Closing LOCAL_VALIDATION, SOURCE_REVIEW, CI_VALIDATION, REALITY_GATE, MERGE_READY, POST_MERGE_VERIFY or any handoff that asserts tests, runtime, security, migration or merge readiness. Required for `platform-evidence-collector`, `platform-reality-checker` and writer→verifier handoffs.

## Authoritative inputs

Always capture:

1. Exact repository (`gagandsm-max/platform` or declared fork), branch, worktree path and full 40-character commit SHA(s) for base and head
2. Task manifest identity: `taskId`, `status`, `selectedAgents`, `selectedSkills`, `writerTool`, `verifierTool`, `gatekeeper`
3. Governing requirement IDs and Mother Reference sections cited
4. Exact commands, container/Compose project names, and test identifiers executed
5. CI run URLs/IDs tied to the same SHA when CI_EVIDENCE is claimed

Prior chat summaries are not repository truth. Re-fetch before reusing older bundles (`source_truth_rebaseline` / `exact_head_merge_gate`).

## Invariants

- Claim types are disjoint and honest:
  - `SOURCE_FACT` — directly observed repository/source/config fact; requires `evidencePointer`
  - `EXECUTION_EVIDENCE` — result of an executed command/mutation locally or in a controlled runtime; requires `evidencePointer`
  - `CI_EVIDENCE` — CI result tied to an exact SHA; requires `evidencePointer`
  - `INFERENCE` — reasoned conclusion from cited facts/evidence; not a substitute for required gates
  - `UNVERIFIED` — required claim not yet proved; must never satisfy a required evidence gate
- Exact commands and test identifiers beat paraphrases.
- Pass/fail counts and omitted/unexecuted tests are recorded explicitly.
- Stale head/base invalidates the bundle until recomputed.
- No fabricated logs, green screenshots without commands, or silent promotion of `UNVERIFIED` → fact.

## Required procedure

1. Record repository, ref, authoritative main SHA, task head SHA, and observation time.
2. List selected agents/skills and the gate being advanced.
3. For each material claim, assign exactly one claim type and a pointer (path, test id, CI run, log snippet reference)—prefer pointers over embedded raw artifacts.
4. Execute or cite commands with working directory and exit status; capture pass/fail/skip counts.
5. Attach runtime logs, migration outputs and security/failure evidence when those risks are in scope—or mark `NOT_RUN`/`UNVERIFIED` with reason.
6. Enumerate tests not executed that a reader might assume ran.
7. Re-check head/base drift; if drifted, mark the bundle stale and stop certification claims.
8. Hand the bundle to an independent verifier/gatekeeper; writer does not close REALITY_GATE/MERGE_READY/COMPLETE.

## Negative / failure cases

- Summary-only “all tests passed” without command/test ids or SHA.
- Relabelling source inspection as runtime evidence.
- Reusing CI from a different SHA or branch.
- Embedding secrets, tokens, private keys, unredacted support bundles or customer payloads.
- Hiding failed tests by deleting them from the narrative.
- Claiming absence of a class of bugs because one path did not fail.
- Using `INFERENCE` to satisfy a required `EXECUTION_EVIDENCE`/`CI_EVIDENCE` gate.

## Security / tenant boundary

Redact secrets and tenant-sensitive payloads before capture. Evidence remains bounded and tenant-scoped where applicable. Do not exfiltrate production data into the bundle to “prove” a fix. Support-bundle references must follow secrets_handling redaction rules.

## Required evidence

Minimum bundle fields:

- repository / branch / worktree / exactHead / authoritativeMain
- commands and test identifiers with pass/fail/skip counts
- runtime/migration/security evidence or explicit omissions
- claims[] with claimType + statement + evidencePointer
- blockers, unresolved risks, nextOwner/nextState
- drift check result

## Stop conditions

Stop certification progression when:

- Head or main has drifted since the evidence was gathered
- Required claim remains `UNVERIFIED`
- Pointers do not resolve
- Bundle would require embedding secrets to be believable
- Writer and verifier/gatekeeper are not independent

## Anti-patterns

- Screenshot-only greenery
- “LGTM” without pointers
- Fabricated or reconstructed copyrighted/proprietary logs
- Quietly editing the bundle after a failed verifier pass without a new SHA
- Treating coverage percentage as proof of an invariant

## Handoff / output

Emit a structured handoff (`handoff.schema.json` fields or equivalent) with claims, testResults, runtimeEvidence, securityEvidence, knownFailures, unresolvedRisks, nextState and nextOwner. Flag stale-head explicitly.

## Non-ownership

Does not implement product fixes, weaken tests to obtain green evidence, grant merge authority, or replace domain Skills (`migration_safety`, `security_negative_testing`, `alert_quality_evidence`). Does not certify beyond what pointers prove.

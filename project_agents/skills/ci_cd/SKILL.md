---
name: ci_cd
description: GitHub Actions/CI wiring, exact-head evidence, and no test weakening to obtain green.
---

# ci_cd

## Purpose

Keep CI/CD evidence exact-SHA, complete for the change class, and honest—never green by skipping, hiding or weakening failing tests.

## Use when

Editing workflows, required checks, runners, caches, artifacts, deployment promotion, or interpreting CI for gates. Required for `platform-devops-automator` and any MERGE_READY evidence that cites CI.

## Authoritative inputs

Always read:

1. `.github/workflows/*` at the exact head under test
2. Branch protection / required checks expectations for the target branch
3. Task requiredTests / requiredEvidence lists
4. `project_agents/tool_authority.json` (CI is not merge authority by itself)
5. Runner/label documentation and self-hosted runner inventory when used
6. Artifact/provenance expectations for releases/packages when in scope

## Invariants

- Workflows and evidence bind to an **exact SHA**; reruns must be attributed.
- Required gates for the change class must run (static/unit/integration/DB/UI/security as applicable).
- Distinguish fail-fast jobs from diagnostic/informational jobs; diagnostics do not waive required failures.
- Migration testing is mandatory when schema/migration paths change.
- Artifact provenance and dependency/cache integrity matter for release claims.
- Secrets in CI use platform secret mechanisms—never plaintext in logs/yaml.
- Self-hosted runners stay isolated; do not collide with unrelated persistent developer Docker containers/volumes.
- Cancellation/timeouts are fail-closed for required gates unless explicitly diagnostic.
- Rerun evidence must state why and on which SHA.
- Branch protection expectations are not optional for protected branches.
- No hiding/skipping/xfailing failing product tests to obtain green.
- Deployment promotion requires explicit evidence separate from unit green.

## Required procedure

1. Identify workflows applicable to the diff and the exact SHA.
2. Map required vs diagnostic jobs; ensure required set covers static contracts, unit, integration, DB/postgres tags, UI and security smoke as dictated by the change.
3. Confirm migration tests run for migration diffs; do not rely on unit-only green.
4. Inspect secret usage, cache keys and artifact upload paths for leakage/poisoning risk.
5. On self-hosted runners: verify labels, isolation and that jobs will not `docker prune` unrelated persistent containers.
6. Network/Docker collisions: use project-scoped compose names; document ports.
7. For failures: capture job logs bound to SHA; fix product/tests—do not skip.
8. For promotion/deploy jobs: require separate approval/evidence trail; CI green ≠ production authority.
9. Record CI run IDs, SHA, required-check summary for `exact_head_merge_gate`.

## Negative / failure cases

- Required check missing from branch protection after workflow rename.
- Cache restoring poisoned dependencies without checksum verification.
- Printing secrets in `set -x` logs.
- Global docker prune on a shared runner.
- Cancelling a required job and treating absence as pass.
- Rerunning only the green jobs after a red required job.
- Deploying from a workflow_dispatch without SHA pin.

## Security / tenant implications

CI secrets are credentials—least privilege, rotated, never in forks unchecked. Tenant fixtures in integration tests must not be real customer data. Supply-chain: pin actions by SHA where policy requires; review new actions via OSS catalogue process when shipping product dependencies from CI.

## Required evidence

- Workflow file paths + SHA
- CI run URLs/IDs bound to that SHA
- Required-check matrix result
- Migration/security job results or explicit N/A
- Secret-handling confirmation (no leakage)
- Runner isolation notes when self-hosted
- Promotion evidence when deploy claimed

## Stop conditions

Stop merge/promotion when:

- Required checks failed or missing on exact head
- Tests were skipped/weakened to green
- Runner contamination cannot be excluded
- Secrets leaked or were committed
- Promotion lacks authorization evidence

## Anti-patterns

- `continue-on-error: true` on required product gates
- Deleting assertions to pass CI
- “It passed locally” as CI replacement
- Using latest mutable action tags against policy
- Writer self-certifying deploy

## Handoff / output

Return: SHA-bound CI matrix; gaps; secret/runner risks; promotion status; nextOwner (verifier/gatekeeper). CI green alone is not MERGE_READY.

## Non-ownership

Does not replace product domain Skills, grant merge authority, or authorize production change windows. Owns CI honesty and exact-SHA wiring.

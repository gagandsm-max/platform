---
name: post_merge_rebaseline
description: After an authorized merge, re-fetch main and dependent heads, invalidate stale stack assumptions and reconcile downstream work before continuing.
---

<!-- generated from project_agents/skills/post_merge_rebaseline/SKILL.md sha256:ef066be274de94009f4ea6b03efdabb7541721778ded220d911e9bd7292aeb2b -->

# post_merge_rebaseline

## Purpose

After an authorized merge, re-establish truth on the new `main` SHA, invalidate pre-merge evidence, and reconcile dependent branches/worktrees before any further certification or stacked work.

## Use when

Immediately after gatekeeper-authorized merge to `main`; before COMPLETE claims; before starting the next BUILD/lane that depends on merged work; when stacked PRs must rebase; when Build Register / Issue closure is asserted.

## Authoritative inputs

Always fetch/read:

1. Exact merge commit SHA on GitHub `main` (not local memory)
2. PR number, merge timestamp, and proof the PR actually merged (`gh pr view --json state,mergeCommit`)
3. Post-merge CI runs bound to that exact `main` SHA
4. Build Register / Issue status fields if the lane uses them (read-only unless authorized)
5. Dependent branch/worktree list for the same repository
6. Projection/pack sync requirements when `project_agents/` merged

## Invariants

- Verify the **exact main merge SHA** before claiming post-merge success.
- Verify the PR actually merged; open/closed-unmerged is not enough.
- Post-merge CI must be tied to the exact new `main` SHA—not the pre-merge PR head alone.
- Refresh all local/worktree bases that track this lane; stale branches are declared.
- Pre-merge certification evidence is stale for COMPLETE; rebuild on `main` as required.
- Build Register transitions follow control-plane rules—writers do not self-advance certification state casually.
- Issue/PR closure requires evidence (merged SHA + applicable gates), not emoji reactions.
- Generated projections (agents/skills) must be synchronized when pack sources merged.
- Inventory new dependencies/migrations introduced by the merge.
- No certification based on pre-merge SHA alone.

## Required procedure

1. Fetch `origin/main` and record the merge commit SHA; confirm it matches the PR mergeCommit.
2. Confirm PR state is MERGED; capture PR URL and merge time.
3. Identify required post-merge CI; verify runs executed on the merge SHA (or explicit follow-up SHA policy).
4. Update every relevant local worktree/branch: merge/rebase onto new main per lane rules; reject silent divergence.
5. Mark pre-merge audit bundles stale for COMPLETE; list what must be re-proven on main.
6. If agent/skill pack files merged, run pack sync/validate and confirm projection counts.
7. Inventory migrations/dependencies/config landed; flag follow-up BUILD/register actions.
8. Only then recommend Issue/PR closure or COMPLETE—with evidence pointers.
9. Hand stacked work a new base SHA; do not continue stacked implementation on pre-merge assumptions.

## Negative / failure cases

- Declaring COMPLETE from the last PR-head CI while `main` moved.
- Closing the Issue because the PR is “green” but not merged.
- Leaving dependent worktrees on the old base.
- Skipping pack sync after governance merges.
- Advancing Build Register without post-merge evidence.
- Treating squash/rebase rewrite confusion as optional.

## Security / tenant implications

Post-merge must not weaken protected-branch rules or bypass required reviews. Secrets introduced by the merge remain governed by `secrets_handling`. Do not publish support bundles containing post-merge credentials. Tenant-affecting schema landings need migration_safety follow-through on main.

## Required evidence

- Exact `main` merge SHA + PR merge proof
- Post-merge CI IDs bound to that SHA (or documented waiver with gatekeeper)
- Worktree/branch refresh list
- Stale-evidence invalidation note
- Pack sync/validate output when applicable
- Migration/dependency inventory delta
- Closure recommendation with pointers—not vibes

## Stop conditions

Stop COMPLETE/closure when:

- PR not actually merged
- Merge SHA cannot be fetched
- Required post-merge CI missing/failed on main
- Dependent lanes would silently build on stale bases
- Pack projections drift after a pack-affecting merge

## Anti-patterns

- “Merge made CI unnecessary”
- Certifying from the PR head after squash-merge produced a new SHA
- Force-updating shared branches without coordination
- Writer self-closing REALITY_GATE/COMPLETE
- Ignoring migration landings on main

## Handoff / output

Return: main SHA; PR merge proof; CI bindings; refreshed bases; stale markers; inventory deltas; nextOwner (verifier/gatekeeper/next BUILD writer). Writer cannot self-certify COMPLETE.

## Non-ownership

Does not merge PRs, rewrite history, invent Build Register policy, or replace `exact_head_merge_gate`. Owns post-merge truth restoration only.

---
name: source_truth_rebaseline
description: Re-fetch authoritative main, exact task head, PR base/head and governing repository truth before relying on prior conclusions.
---

# source_truth_rebaseline

## Purpose

Establish repository truth before diagnosis, implementation, verification or merge-gate claims. Every later claim is invalid if this step used a stale SHA, dirty workspace or chat summary as source.

## Use when

Task intake, diagnosis, routing, implementation start, any evidence reuse, CI interpretation, PR review, REALITY_GATE / MERGE_READY / POST_MERGE_VERIFY work, or any disagreement between local checkout and GitHub. Required before `mother_reference_reconciliation` and `exact_head_merge_gate`.

## Authoritative inputs

Always read / fetch:

1. Canonical GitHub repository `https://github.com/gagandsm-max/platform` — `main` exact 40-char SHA after `git fetch`
2. Task branch/PR number, base SHA, head SHA from GitHub (`gh pr view`, `git ls-remote`), not chat memory
3. Root `AGENTS.md` and `project_agents/tool_authority.json` for tool roles
4. Exact-head `project_documentation/Platform_00_Document_Index.md` to discover the current Mother Reference set
5. Local worktree status: branch, detached HEAD, dirty/untracked files, upstream tracking

Do not treat Cursor/Codex/Antigravity summaries, stale agent transcripts or exported zip trees as authoritative GitHub source without SHA proof.

## Invariants

- **Canonical source of truth is GitHub `main` exact SHA**, not the local index alone.
- Branch/PR/head must be verified against GitHub; local `HEAD` may be stale, detached or on the wrong remote.
- **SOURCE_FACT** requires fetch/evidence; local workspace state is observation until reconciled.
- Fetch before any claim that names a SHA, PR status or Mother Reference version.
- Stale-head detection is mandatory: if evidence SHA ≠ current exact head, mark blocked.
- Dirty/untracked local changes must be declared; they are not repository truth and must not be silently committed or ignored when claiming cleanliness.
- Detached HEAD / external exports cannot certify merge without reattaching to a named branch/PR SHA.
- Mother Reference vs code conflicts are classified, not papered over.
- CI evidence must be associated with the exact SHA under discussion.
- No assumption from tool summaries; re-query GitHub/git when uncertain.
- Output always includes authoritative SHA pins + evidence pointers.

## Required procedure

1. `git fetch origin` (or equivalent) and record authoritative `main` SHA (full 40 chars).
2. Resolve task branch/PR: base SHA, head SHA, mergeability flags from GitHub.
3. Compare local `HEAD` / worktree to GitHub head; if divergent, stop reuse of prior conclusions.
4. Inspect dirty/untracked state; list paths; classify as task work vs contamination.
5. Read `Platform_00` from the exact task head; discover Mother Reference set dynamically (no frozen file-count ceiling).
6. Bind any CI run IDs to the exact SHA they executed; reject ancestor-SHA greens.
7. Emit a rebaseline record: authoritative main SHA, PR/base/head SHAs, dirty-state summary, Mother Reference index SHA/path, and claim types (`SOURCE_FACT` / `UNVERIFIED`).
8. Hand off only after SHA pins are written; do not proceed on “looks current”.

## Negative / failure cases

- Citing chat “we’re on X” without `git rev-parse` / `gh` evidence.
- Using a local commit that was never pushed as if it were the PR head.
- Reusing CI from SHA N-1 after a corrective push.
- Ignoring dirty files while claiming a clean rebaseline.
- Treating a detached export or Cloud snapshot as GitHub `main`.
- Assuming Mother Reference count from memory instead of `Platform_00` at exact head.
- Collapsing “local builds” into `SOURCE_FACT` without command/SHA evidence.

## Security / tenant implications

Do not embed secrets from `.env`, certs or credential files into the rebaseline record. Cross-tenant or customer data in local logs stays redacted. Write authority remains per `tool_authority.json`—rebaseline itself is read-first and must not become an unauthorized write lane.

## Required evidence

- Authoritative `main` SHA (40-char)
- PR/base/head SHAs and PR number when applicable
- Fetch command/time or `gh` output pointers
- Dirty/untracked inventory or explicit clean statement
- CI↔SHA binding when CI is cited
- Mother Reference discovery pointer (`Platform_00` at exact head)
- Claim labels via `evidence_bundle` discipline

## Stop conditions

Stop downstream work when:

- GitHub cannot be fetched and SHA cannot be proven
- Local head ≠ claimed PR head without deliberate sync plan
- Dirty state includes secrets or unrelated product changes outside scope
- Mother Reference index at exact head is missing/unreadable
- Task premise depends on a SHA that no longer exists on the remote

## Anti-patterns

- “Trust the previous agent summary”
- Force-pushing to manufacture a convenient head mid-review
- Certifying from `origin/main` remembered yesterday
- Editing Mother docs during a read-only rebaseline
- Skipping fetch because “Docker already has the code”

## Handoff / output

Return: authoritative main SHA; PR/base/head SHAs; dirty-state; Mother Reference discovery note; CI bindings; stale markers; nextOwner (`mother_reference_reconciliation` / diagnosis / writer). Writer cannot self-certify merge readiness from this Skill alone.

## Non-ownership

Does not implement product fixes, rewrite Mother References, certify MERGE_READY/COMPLETE, or grant write elevation. Owns source-truth establishment only.

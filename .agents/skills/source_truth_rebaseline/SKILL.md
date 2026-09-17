---
name: source_truth_rebaseline
description: Re-fetch authoritative main, exact task head, PR base/head and governing repository truth before relying on prior conclusions.
---

<!-- generated from project_agents/skills/source_truth_rebaseline/SKILL.md sha256:3de34d43dc4f3b7ae69c8f16a9dd9fac4f968fb23c9233d048bc16cb37e5abf3 -->

# source_truth_rebaseline

## Purpose
Establish repository truth before diagnosis, implementation, verification, or merge-gate claims.

## Required procedure
1. Read root `AGENTS.md` and the repository routing/governance files applicable to the task.
2. Fetch authoritative `main`, task PR/base/head and exact SHAs from GitHub.
3. Read `project_documentation/Platform_00_Document_Index.md` from the exact task head and use it to discover the current Mother Reference set dynamically; never assume a numeric document ceiling.
4. Detect main/base/head drift since the evidence being reused. If drift exists, mark stale review/evidence blocked until reconciled.
5. Record claims using `SOURCE_FACT`, `EXECUTION_EVIDENCE`, `CI_EVIDENCE`, `INFERENCE`, or `UNVERIFIED`.

## Guardrails
- Prior chat summaries are not repository truth.
- Never silently substitute a different base/head.
- Do not write while performing a read-only rebaseline.

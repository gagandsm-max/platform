# post_merge_rebaseline

## Purpose
Reconcile downstream work after an authorized upstream merge changes repository truth.

## Required procedure
1. Fetch new authoritative `main` SHA and merged PR result.
2. Re-fetch every dependent open PR base/head and mergeability.
3. Identify evidence and reviews invalidated by changed base/main.
4. Reconcile stacked branches without force-push unless separately authorized.
5. Re-run required exact-head validation after reconciliation.
6. Update handoff state and evidence bundle before downstream work resumes.

## Guardrails
- Never assume a stack remains coherent after upstream merge.
- Never silently retarget or rewrite history.
- Do not treat an old CI run as evidence for a reconciled head.

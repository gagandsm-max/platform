<!-- generated from project_agents/skills/exact_head_merge_gate/SKILL.md sha256:2995cf42fd6a5fd9f72b8824ebdc4cb2210a5fe9417a485e631dfdeff214be64 -->

---
name: exact_head_merge_gate
description: Gate review and merge claims on the exact current head, complete required CI/evidence and independent verification; drift invalidates stale evidence.
---

# exact_head_merge_gate

## Purpose
Prevent stale evidence from authorizing review or merge.

## Required procedure
1. Fetch current authoritative main, PR base and exact head.
2. Confirm reviewed diff is the complete exact-head diff.
3. Confirm all required CI/evidence belongs to that exact head.
4. Confirm `writerTool` and independent `verifierTool` are distinct where verification is required. The writer cannot self-certify `MERGE_READY` or `COMPLETE`.
5. Re-check security, migration, runtime and product-specific gates according to task risk.
6. If any relevant head/base/main moved, invalidate stale gate evidence and return to rebaseline.

## Guardrails
- `mergeable=true` is not equivalent to verified/merge-ready.
- A prior green run on another SHA is not exact-head CI evidence.
- Never merge unless separately authorized.

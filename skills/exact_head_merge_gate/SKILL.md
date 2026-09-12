# exact_head_merge_gate

## Purpose
Prevent stale evidence from authorizing review or merge.

## Required procedure
1. Fetch current authoritative main, PR base and exact head.
2. Confirm reviewed diff is the complete exact-head diff.
3. Confirm all required CI/evidence belongs to that exact head.
4. Confirm writer and independent verifier are distinct where verification is required.
5. Re-check security, migration, runtime and product-specific gates according to task risk.
6. If any relevant head/base/main moved, invalidate stale gate evidence and return to rebaseline.

## Guardrails
- `mergeable=true` is not equivalent to verified/merge-ready.
- A prior green run on another SHA is not exact-head CI evidence.
- Never merge unless separately authorized.

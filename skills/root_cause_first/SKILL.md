# root_cause_first

## Purpose
Prevent speculative or symptom-only implementation.

## Required procedure
1. Reproduce or trace the reported failure from authoritative source/tests/evidence.
2. Identify the exact mechanism, boundary and invariant being violated.
3. Separate verified root cause from contributing factors and hypotheses.
4. Define the smallest test that fails for the root cause before selecting a repair.
5. Route implementation only after the mechanism is evidenced.

## Guardrails
- Do not broaden into architecture work merely because nearby code is imperfect.
- Do not weaken tests to fit an implementation.
- If the root cause cannot be proved, report `UNVERIFIED` and remain in diagnosis.

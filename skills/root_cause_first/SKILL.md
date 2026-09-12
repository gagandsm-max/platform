<!-- generated from project_agents/skills/root_cause_first/SKILL.md sha256:71ee0225446fc71e018e50e980b45e047e3430362d71abf03095338f039da1a5 -->

# root_cause_first

## Purpose
Prevent speculative or symptom-only implementation.

## Required procedure
1. Reproduce or trace the reported failure from authoritative source/tests/evidence.
2. Identify the exact mechanism, boundary and invariant being violated.
3. Separate verified root cause from contributing factors and hypotheses.
4. Capture verified failure evidence appropriate to the defect before selecting a repair. That evidence may be a reproducing test, a traced runtime/log failure, a contract mismatch, or another bounded observation of the failing mechanism.
5. Add or update a regression test where that is technically meaningful for the defect class. Do not universally require creation of a new failing test when the failure is already proved by other verified evidence and a regression test cannot honestly exercise the mechanism.
6. Route implementation only after the mechanism is evidenced.

## Guardrails
- Do not broaden into architecture work merely because nearby code is imperfect.
- Do not weaken tests to fit an implementation.
- Do not treat an absent unit-test file as proof that diagnosis is incomplete when equivalent verified failure evidence already exists.
- If the root cause cannot be proved, report `UNVERIFIED` and remain in diagnosis.

---
name: root_cause_first
description: Establish the evidenced failure mechanism before implementation; reject symptom-only patches and unsupported assumptions.
---

<!-- generated from project_agents/skills/root_cause_first/SKILL.md sha256:b00bf25dca066d06dca3020b70b648711b0565741f85691a2522b165b83aba46 -->

# root_cause_first

## Purpose

Establish the evidenced failure mechanism before coding. Symptom-only patches, speculative refactors and environment workarounds that hide product defects are out of scope.

## Use when

Any bug, flaky test, CI failure, production incident, BUILD corrective, or “fix” task before `bounded_implementation`. Required when multiple hypotheses exist or when prior patches failed.

## Authoritative inputs

Always obtain:

1. Exact failing command/test/CI job and exact SHA (`source_truth_rebaseline`)
2. Reproducible failure transcript (logs, stack, assertions)—not paraphrases
3. Governing requirement / expected behaviour from Mother docs when behaviour is contested
4. Recent related diffs and prior corrective commits on the lane
5. Runtime/environment facts labelled separately from application defects

## Invariants

- Reproduce the **exact** failure on the claimed SHA before proposing a fix.
- Classify cause: application/code, config, environment/lab, dependency/OSS, or test harness—do not collapse classes.
- Inspect source, logs and runtime evidence; do not invent a story that fits one log line.
- Hypotheses must be falsifiable and tested; discard survivors that fail disproof.
- Do not modify the environment to bypass a product defect (weaken assertions, skip gates, disable TLS, raise timeouts to hide races) unless the task is explicitly lab-harness repair with declared scope.
- No speculative refactor while the mechanism is unknown.
- Prefer the smallest coherent intervention that removes the root mechanism.
- Verify **root-cause disappearance**, not only symptom disappearance.
- Require regression evidence that the failure cannot recur under the same trigger.
- Stop when evidence contradicts the task premise (wrong SHA, wrong component, cannot reproduce).

## Required procedure

1. Pin SHA and reproduce the failure with the same command/flags as the report; capture raw output.
2. Classify observed layer (app/config/env/dependency/harness) with evidence.
3. Form 1–3 falsifiable hypotheses; design the cheapest disproof for each.
4. Inspect responsible source paths and related tests; record what was and was not read.
5. Reject environment bypasses that would leave the product defect intact.
6. Select the smallest coherent fix addressing the surviving mechanism; list non-goals.
7. Implement via `bounded_implementation`; add/adjust the narrowest regression test.
8. Re-run the exact failing case plus adjacent regressions; confirm the mechanism is gone.
9. If unreproducible or premise false, return BLOCKED/INCONCLUSIVE with evidence—do not “fix anyway”.

## Negative / failure cases

- Patching a flaky sleep instead of the race.
- Skipping the failing test to green CI.
- Rewriting large modules because “it might be cleaner”.
- Changing Docker/host state to avoid a deterministic product bug.
- Declaring fixed because a different test passed.
- Continuing after proving the bug is on another component/SHA.

## Security / tenant implications

Do not disable auth, tenant predicates, TLS verification or secret redaction to “make it pass”. Security-sensitive root causes require `security_negative_testing` / `secrets_handling` follow-through. Cross-tenant contamination in repro data must be redacted in evidence.

## Required evidence

- Exact SHA + repro command + failing output
- Classification and hypothesis log (tested/failed/survived)
- Source paths inspected
- Diff rationale tied to mechanism
- Regression test ID/command and PASS output
- Explicit statement that symptom and mechanism were both checked
- `UNVERIFIED` if runtime proof could not be obtained

## Stop conditions

Stop coding when:

- Failure cannot be reproduced on the claimed SHA
- Evidence shows the premise/component is wrong
- Only bypasses (skip/weaken) would green the gate
- Fix requires unresolved architecture/Decision Register work outside task scope
- Multiple writers would be needed on the same checkout

## Anti-patterns

- Shotgun logging without a hypothesis
- “Works on my machine” without SHA/command parity
- Drive-by refactors in the same commit as the root-cause fix
- Writer self-certifying that the RCA is complete
- Treating LLM confidence as causal proof

## Handoff / output

Return: mechanism statement; classification; evidence pointers; proposed smallest fix or BLOCKED; regression plan; nextOwner (`bounded_implementation` / verifier). Do not mark MERGE_READY from RCA alone.

## Non-ownership

Does not own final merge certification, Mother rewrites, or broad refactors. Owns mechanism establishment and rejection of symptom-only work.

---
name: rca_hypothesis_fusion
description: Evidence-grounded RCA hypothesis fusion with contradictions, confidence and INCONCLUSIVE as valid outcomes.
---

<!-- generated from project_agents/skills/rca_hypothesis_fusion/SKILL.md sha256:8548a1244e312cd53b22e6820a3f0d7508e16c3396fb02e30f0f5caa17bb6b6d -->

# rca_hypothesis_fusion

## Purpose

Fuse deterministic evidence into ranked RCA hypotheses with explicit confidence, alternatives and contradictions—never forcing a root cause or letting an LLM assert unsupported causality.

## Use when

Incident RCA panels, hypothesis engines, GNN/LLM-assisted explanations, or alert→incident causal narratives. Required for `platform-ai-engineer` / RCA paths under `Platform_22` when present.

## Authoritative inputs

Always read:

1. `Platform_22` / Architecture rules: AI is evidence support; `INCONCLUSIVE` is allowed
2. Alert Evidence Contracts and collector-health separation
3. Topology / change / telemetry evidence APIs on the exact head
4. `ai_evidence_safety` and `change_intelligence` Skills
5. Existing RCA output schema (observed facts, likely cause, confidence, contradictions, next validation)

If GNN/LLM fusion is not implemented, document deterministic-only behaviour and mark model fusion `UNVERIFIED`.

## Invariants

- Deterministic hypotheses and checks run first; models may rank/explain—not override facts.
- Evidence types are labelled (telemetry, topology, change, collector health, operator).
- Topology and temporal evidence carry timestamps and provenance.
- Change evidence is correlative until corroborated.
- Collector health is evaluated before resource failure assertions.
- Contradictions are retained and shown—not deleted for neatness.
- Confidence is separate from alert severity.
- Alternative hypotheses remain visible when plausible.
- **No forced root cause**; `UNKNOWN` / `INCONCLUSIVE` are valid correct outcomes.
- Causal vs correlated evidence is distinguished in wording.
- Operator feedback is structured/audited—not silent model mutation.
- Provenance of each supporting fact is required.
- **No LLM unsupported causal assertion**; prompts cannot mint privilege or “certainty.”

## Required procedure

1. Gather evidence classes in scope; verify each source exists on this SHA.
2. Run deterministic consistency checks (collector health, maintenance, corroboration).
3. Build hypothesis set with supporting and contradicting facts.
4. Score confidence conservatively; allow `INCONCLUSIVE`.
5. If LLM/GNN used: constrain to allowlisted tools/schemas; ground in cited evidence IDs.
6. Reject outputs that assert cause without cited facts.
7. Expose alternatives + next validation steps to operators.
8. Record feedback separately from active policy/model weights unless governed evaluation says otherwise.

## Negative / failure cases

- Single SNMP timeout ⇒ “root cause: device hardware failure.”
- LLM narrative without evidence IDs.
- Hiding contradictory ping success.
- Auto-remediating from RCA text alone.
- Treating Similar Incident Memory similarity as current cause.

## Security / tenant implications

RCA packages are tenant-scoped. No cross-tenant evidence mixing for “better models.” Prompts/tool calls cannot escalate privilege. Redact secrets in evidence snippets.

## Required evidence

- Hypothesis set with citations
- Contradiction list
- Confidence rationale
- Model/tool versions or `UNVERIFIED` if unused
- Exact SHA

## Stop conditions

Stop when:

- Forced single cause demanded against contradictions
- Collector health ignored
- LLM output would execute privileged actions
- Cross-tenant evidence requested for training/fusion

## Anti-patterns

- Confidence theatre (0.99 with one weak signal)
- Deleting alternatives to “look decisive”
- Writer self-certifying RCA accuracy

## Handoff / output

Return: hypothesis pack; INCONCLUSIVE if warranted; next validations; nextOwner (model-qa / reality checker). Not MERGE_READY alone.

## Non-ownership

Does not own alert policy authoring, remediation execution, or invent causal certainty. Owns RCA hypothesis fusion discipline.

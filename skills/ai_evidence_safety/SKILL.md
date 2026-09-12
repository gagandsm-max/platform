<!-- generated from project_agents/skills/ai_evidence_safety/SKILL.md sha256:9c2b7514b2f869089269e961d1ecbd6e09f96e13bf6c28948e61b785326c09db -->

# ai_evidence_safety

## Purpose
Keep AI/ML assistance evidence-bound, tenant-safe and non-authoritative where deterministic controls are safer.

## Required checks
- Claims cite authoritative evidence and expose contradictions/missing evidence.
- GNN/LLM output is never treated as causal authority.
- No LLM directly performs privileged action; tool use passes deterministic authorization/policy controls.
- Prompt/tool input is treated as untrusted and prompt-injection paths are tested.
- Learning is tenant-scoped; no silent cross-tenant training.
- Provider/data-egress policy is enforced for hosted/private/local/no-LLM modes.
- Core monitoring degrades safely without internet, LLM or GNN.
- Model/data drift, evaluation and provenance are observable.

## Guardrails
- Do not select a model/vendor/runtime dependency without required OSS, security and performance review.
- `INCONCLUSIVE` is preferable to unsupported certainty.

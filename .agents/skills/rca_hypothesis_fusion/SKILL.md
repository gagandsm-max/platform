---
name: rca_hypothesis_fusion
description: RCA hypothesis fusion: observed facts, likely cause, confidence, contradictions, missing evidence and INCONCLUSIVE.
---

<!-- generated from project_agents/skills/rca_hypothesis_fusion/SKILL.md sha256:f816b8768681c9f9d13776c7ab4066211c068aaae22706c94166ed10b3f6633e -->

# rca_hypothesis_fusion

## Purpose
Fuse evidence into hypotheses without causal overclaim.

## Use when
RCA engines, Copilot, or incident cause ranking.

## Required procedure
1. Separate facts, hypotheses, confidence.
2. GNN/LLM are not causal authority.
3. INCONCLUSIVE is valid.
4. Similar-incident memory is advisory only.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `rca_hypothesis_fusion` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

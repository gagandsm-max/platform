<!-- generated from project_agents/skills/alert_quality_evidence/SKILL.md sha256:26a259c3ce7bce2b75d5838b9672735f8c81f95aae118463a53a2f88eb89137f -->

# alert_quality_evidence

## Purpose
Evaluate monitoring intelligence against Platform_22 evidence and alert-quality contracts.

## Required checks
- Class-specific Alert Evidence Contract is identified.
- Telemetry quality/trust is evaluated before asserting target failure.
- Severity and confidence remain independent.
- Semantic states such as `MONITORING_DEGRADED`, `EXPECTED_STATE`, `UNKNOWN` and `STALE` are not collapsed into failure.
- Policy promotion follows DRAFT → BACKTEST → SHADOW → ACTIVE with measurable false-alert gates.
- `INCONCLUSIVE` remains a valid outcome when evidence is insufficient.

## Guardrails
- Never invent universal thresholds.
- Operator feedback must not silently mutate active policy.
- Evidence coverage and missing evidence must remain visible.

<!-- generated from project_agents/skills/alert_quality_evidence/SKILL.md sha256:f5331db52b8fa46a987fc25758c39599701d6eb2546212f33f5b508b59d7aaf8 -->

---
name: alert_quality_evidence
description: Evaluate alert evidence quality, telemetry trust, false-alert controls, severity/confidence separation and INCONCLUSIVE behavior.
---

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

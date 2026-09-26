---
name: alert_quality_evidence
description: Evaluate alert evidence quality, telemetry trust, false-alert controls, severity/confidence separation and INCONCLUSIVE behavior.
---

# alert_quality_evidence

## Purpose

Evaluate monitoring intelligence against Platform_22 Alert Evidence Contracts so operational assertions stay evidence-bound, telemetry-trust-aware and free of false DOWN semantics.

## Use when

Changing alert classes/policies, evidence contracts, collector-health gates, semantic monitoring states, false-positive/false-negative measurement, BACKTEST/SHADOW promotion, Incident Copilot alert claims, or any path that asserts target failure from telemetry. Required for monitoring-intelligence, alert-quality and AI-assurance work under Platform_22.

## Authoritative inputs

Always read:

1. `project_documentation/Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` (Alert Evidence Contracts, lifecycle, trust labels)
2. `project_documentation/Platform_02_PRD.md` alert/severity semantics
3. `project_documentation/Platform_08_Requirements_Traceability.md` for REQ-MIA identifiers in scope
4. Class-specific contract fixtures and labelled incident sets referenced by the task
5. Topology/dependency evidence sources when corroboration is required by the contract

Do not invent universal quality thresholds. Governing requirements and contract versions are authoritative.

## Invariants

- Collector/source health, freshness, timestamp quality and evidence gaps are evaluated **before** classifying the monitored target as failed.
- No Data ≠ Down. Missing/stale/absent telemetry is not authoritative failure when the contract requires corroboration or when collector health is degraded.
- Severity (operational impact) and confidence/trust (evidentiary support) are independent dimensions.
- Semantic states `UNKNOWN`, `STALE`, `MONITORING_DEGRADED`, `REDUNDANCY_LOST`, `SLA_DEGRADED`, `EXPECTED_STATE` must not be collapsed into generic DOWN.
- Contradictory credible evidence blocks authoritative failure; prefer `INCONCLUSIVE` or degraded semantics over unsupported certainty.
- Policy promotion follows `DRAFT → BACKTEST → SHADOW → ACTIVE` with measurable false-alert evidence; bypass promotion is forbidden.
- Operator feedback is structured input to evaluation—it must not silently mutate ACTIVE policy.

## Required procedure

1. Identify the alert class and the versioned Alert Evidence Contract: primary evidence, corroboration, contradictions, telemetry-quality prerequisites, maintenance/expected-state, severity logic, confidence logic, suppression/storm, recovery evidence.
2. Assess collector/sensor/agent/source health and freshness first. If monitoring path is unhealthy, emit `MONITORING_DEGRADED` / `STALE` / `UNKNOWN` per contract—do not assert target DOWN from blind No Data.
3. Apply class-specific evidence checks. Require topology/dependency corroboration when the contract names it (for example reachability vs SNMP-only, shared DNS/DHCP/NTP/AAA dependencies).
4. Separate severity assignment from confidence/trust labels (`CONFIRMED`/`HIGH`/`MEDIUM`/`LOW`/`INCONCLUSIVE`). Downgrade trust on contradiction or missing evidence without inventing severity.
5. For policy changes: run BACKTEST against deterministic historical/synthetic fixtures; then SHADOW against live evidence without authoritative customer alerting; compare false positive/false negative and operator outcomes before ACTIVE.
6. Document safe activation gate: approval, version, rollback path, measured FP/FN evidence, and blockers (grounding, tenant, resource regressions).
7. Preserve recovery evidence and suppression/dedup behavior; storms must not create notification floods or self-induced outages from unbounded active validation.

## Negative / failure cases

- SNMP or single-source failure declared DOWN while ICMP/path/topology still shows reachability (or the reverse without contract authority).
- Treating empty time series as zero-healthy or as DOWN without collector-health evaluation.
- Promoting DRAFT → ACTIVE without BACKTEST/SHADOW evidence.
- Collapsing `STALE`/`UNKNOWN`/`MONITORING_DEGRADED` into DOWN in UI, API or Kafka events.
- Ignoring contradictory evidence because severity “feels” high.
- Active evidence acquisition that is unbounded, cross-tenant, or capable of inducing outage.
- Silent ACTIVE policy mutation from operator chat/feedback.

## Security / tenant implications

Alert evaluation, fixtures, shadow comparisons and Copilot citations must remain tenant-scoped and RBAC/resource-scoped. Do not leak cross-tenant incident labels into evaluation sets. Active validation requires authorization, rate/resource limits and audit. Secrets used for probes stay in Credential Profiles—never in alert payloads or logs.

## Required evidence

- Contract id/version and REQ-MIA identifiers exercised.
- Collector-health-before-target-health fixtures (No Data, stale, degraded source).
- Contradiction and corroboration cases (including topology/dependency).
- Severity vs confidence separation tests.
- BACKTEST and SHADOW results with FP/FN counts; omitted stages explicitly `NOT_RUN`.
- Safe activation checklist (approval, rollback, blockers).
- Exact SHA/commands/test ids; claim types not overstated.

## Stop conditions

Stop when:

- No class-specific contract exists for a production-critical assertion.
- Required corroborating signals are unavailable and the contract forbids single-source authority.
- Lifecycle evidence for promotion is missing or fabricated.
- Mother Reference / Platform_22 conflicts with the proposed semantics.
- Activation would weaken tenant isolation or create unbounded probe load.

## Anti-patterns

- Universal magic thresholds copied across unrelated alert classes.
- “Green because CI unit tests passed” without contract fixtures.
- UI-only hiding of degraded states.
- Using AI confidence as severity.
- Declaring device-down solely because one primary monitor failed.

## Handoff / output

Return: contract coverage matrix; collector-vs-target results; contradiction handling; lifecycle stage evidence; FP/FN summary; remaining `UNVERIFIED`; next verifier (`platform-model-qa` / `platform-code-reviewer` / ChatGPT gate). Do not self-certify ACTIVE rollout.

## Non-ownership

Does not invent contractual SLAs, authorize Founder-reserved commitments, replace topology authority, implement unrelated UI chrome, or close `MERGE_READY`/`COMPLETE`. Does not override Mother Reference alert semantics.

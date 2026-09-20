# Alerts

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_02, 09, 17, 19, 22

## Product objective

Turn qualified operational evidence into actionable alerts without confusing raw telemetry, events or collection failures with confirmed monitored-resource failure.

## Alert pipeline

```text
Telemetry / event
 -> candidate condition
 -> telemetry-quality gate
 -> Alert Evidence Contract
 -> corroboration / contradiction
 -> qualification
 -> Alert
 -> correlation / incident
 -> notification / ITSM
 -> qualified recovery
```

An Event is not an Alert. An Alert is not automatically an Incident.

## Alert Evidence Contracts

Material alert classes define:

- required evidence;
- supporting evidence;
- contradictory evidence;
- missing/unknown evidence;
- collection-quality requirements;
- severity;
- confidence;
- recovery evidence.

A single source failure must not become authoritative when credible contradictory evidence remains healthy.

## Semantic states

Platform supports states including `HEALTHY`, `DEGRADED`, `UNREACHABLE`, `DEVICE_DOWN`, `PATH_FAILURE`, `CREDENTIAL_FAILURE`, `MONITORING_FAILURE`, `MONITORING_DEGRADED`, `REDUNDANCY_LOST`, `SLA_DEGRADED`, `EXPECTED_STATE`, `STALE` and `UNKNOWN` as applicable.

Severity and evidence confidence are independent.

## Policy lifecycle

New/changed policy follows governed lifecycle such as:

`DRAFT -> BACKTEST -> SHADOW -> ACTIVE`

where applicable. Thresholds, suppression, hysteresis, debounce and recovery windows are evidence/policy driven rather than universal hard-coded values.

## Noise control

Use:

- deduplication;
- temporal grouping;
- maintenance/suppression;
- dependency/topology context;
- storm handling;
- state transitions;
- evidence contradictions;
- source-health gates.

Suppression must preserve underlying evidence for audit/RCA.

## Recovery

Recovery is qualified, not simply "the next sample is green." Recovery policy may require stability window, corroboration and source freshness.

## UX

Alert detail should expose:

- asserted state and severity;
- confidence;
- why the alert exists;
- evidence used;
- contradictions/missing evidence;
- topology/service impact;
- related changes;
- related incidents;
- verification actions;
- acknowledgement/assignment;
- suppression/maintenance context;
- ITSM binding.

## Security

Alert actions are tenant/resource scoped. Notification/ITSM delivery does not change monitored state. AI explanations cannot create or resolve alerts without deterministic policy.

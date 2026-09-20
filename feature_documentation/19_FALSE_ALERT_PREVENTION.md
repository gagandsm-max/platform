# False Alert Prevention

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_02, 09, 19, 22

## Product objective

Minimize false operational assertions by validating telemetry quality, requiring class-appropriate evidence, preserving contradictions and distinguishing monitoring failure from resource failure.

## Core principle

**No Data != Device Down.**

Likewise:

- SNMP failure != device failure;
- missing Flow != no traffic;
- log silence != service healthy/down;
- cloud API failure != cloud resource failure;
- one failed synthetic != service outage.

## Alert Evidence Contract

Each material alert class defines required/supporting/contradictory/missing evidence and recovery criteria.

Before asserting operational failure evaluate:

- collector/Sensor/Agent health;
- freshness/staleness;
- clock/timestamp quality;
- credential/protocol errors;
- source gaps;
- contradictory protocols/vantages;
- maintenance/suppression;
- dependency/path state.

## Semantic states

Use explicit intermediate states rather than generic DOWN:

- `MONITORING_DEGRADED`;
- `UNKNOWN`;
- `STALE`;
- `PATH_FAILURE`;
- `CREDENTIAL_FAILURE`;
- `REDUNDANCY_LOST`;
- `SLA_DEGRADED`;
- `EXPECTED_STATE`.

## Noise reduction

- debounce/hysteresis;
- qualified consecutive evidence where appropriate;
- dedup/grouping;
- dependency suppression with child evidence retained;
- maintenance;
- storm aggregation;
- recovery stability;
- topology/change context.

## Policy lifecycle

Use DRAFT -> BACKTEST -> SHADOW -> ACTIVE where applicable. Backtest/shadow metrics include false positives, missed detections, alert volume, time-to-detect and evidence coverage.

## Multi-vantage validation

For path/reachability conditions, independent Sensor/Agent vantage evidence can distinguish local path failure from target failure.

## Operator feedback

Allow structured classifications such as useful/false/duplicate/wrong cause/missing evidence. Feedback feeds evaluation but does not silently alter active policy.

## UX

Alert/Incident detail shows "Why?", evidence coverage, source health, contradictions and missing evidence so the user can understand why Platform asserted or withheld a failure.

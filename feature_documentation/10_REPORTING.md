# Reporting

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 07, 09, 11, 17, 19

## Product objective

Provide tenant-scoped operational, management, SLA, capacity and evidence reports in readable and exportable formats without bypassing data-access controls.

## Report families

Examples include:

- availability/SLA;
- alert/incident trends;
- MTTR/value;
- capacity/utilization/forecast;
- inventory/coverage;
- network/interface/Flow;
- server/cloud/database;
- configuration/NCM compliance;
- telemetry/data quality;
- Sensor/Agent health;
- executive summaries;
- audit/operational evidence where authorized.

## Formats

Approved tooling supports PDF, XLSX and CSV generation. Browser views can also be exported according to RBAC and row/scan limits.

## Scheduling

Reports may be on-demand or scheduled. Scheduling stores the report definition and scope, not an unrestricted query. Delivery respects tenant/resource scope and approved recipients/channels.

## Time and data semantics

Reports must show:

- timezone;
- evaluation period;
- data freshness/quality;
- rollup resolution;
- exclusions/maintenance;
- sampling or estimated-data caveats where relevant;
- assumptions for SLA/value/forecast results.

Historical reports should use time-correct identity/enrichment when required.

## Performance

Large reports execute asynchronously with progress/cancellation and bounded query/resource budgets. They use appropriate ClickHouse rollups rather than scanning unrestricted raw telemetry.

## Security

Exports are authorized server-side. Sensitive raw logs, credentials, user identity or audit material require explicit permissions. Generated files receive bounded retention and audit.

## Evidence-backed claims

Reports must distinguish measured facts, derived metrics, estimates and AI-generated explanations. AI cannot fabricate unavailable measurements.

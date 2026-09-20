# Incidents

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_02, 09, 17, 19, 21, 22

## Product objective

Represent an operational problem spanning one or more qualified alerts/evidence sources as a governed incident with ownership, impact, timeline, RCA, actions and external-ticket linkage.

## Incident formation

Incidents may be created from correlated qualified alerts/events according to deterministic policy. Correlation considers time, topology/dependency, entity/service identity, change context and known causal relationships.

Alert grouping alone does not prove common root cause.

## Incident object

An incident should retain:

- incident ID/tenant;
- lifecycle state;
- severity/priority;
- confidence;
- affected entities/services;
- correlated alerts;
- timeline;
- topology/dependency impact;
- change/config context;
- evidence references;
- RCA hypotheses/conclusion;
- owner/assignment;
- notes/actions;
- ITSM bindings;
- remediation attempts/results;
- recovery evidence;
- audit.

## Incident Command Workspace

The workspace is a projection over authoritative incident/alert/change/topology/evidence objects. It provides timeline, ownership, current hypothesis/evidence, actions/approvals and integration links; it does not become an independent chat authority.

## Flight Recorder

Capture bounded before/during/after evidence such as:

- metric summaries;
- flow top consumers/shifts;
- log fingerprints/representative records;
- topology/path changes;
- configuration changes;
- diagnostics;
- collector/source quality.

Do not attach millions of raw rows to the incident object.

## Investigation

Authorized users can start an Investigation Session. Diagnostic results, CLI actions, flow/log snapshots, notes and evidence can be linked automatically to the incident while preserving source/vantage/time/provenance.

## Lifecycle

Typical states include open/investigating/identified/mitigating/monitoring/resolved/closed or the current governed equivalent. Remote ITSM state does not silently override Platform health or incident truth.

## Recovery and closure

Resolution requires appropriate recovery evidence and policy. Customer ticket closure alone is not monitored recovery.

## Metrics

MTTD/MTTA/MTTR and value metrics expose data quality, cohort/sample methodology and assumptions. Do not fabricate savings or causal improvement percentages.

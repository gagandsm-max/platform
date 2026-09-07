# MTTR / Operational Intelligence Workstream

**Status:** NON-MOTHER engineering sequencing control.  
**Source requirement:** `project_documentation/Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md`.  
**Mother Reference:** Platform v3.2.1 remains authoritative.

## Objective

Integrate the new MTTR/value capabilities into the existing 30-day programme without turning them into a separate monolithic AI subsystem.

## Workstream sequence

1. **Connector/change foundation** — normalize external deployment/change facts and extend connector contracts for SaaS/carrier integrations.
2. **Incident Flight Recorder** — evidence-window contracts, evidence references, graph/timeline replay, quality/completeness telemetry.
3. **Similar Incident Memory** — resolved-incident index, similarity features, accepted-outcome feedback and false-match tests.
4. **Cloud-native graph enrichment** — runtime Kubernetes/OTel/service-mesh dependency edges with provenance/confidence/history.
5. **Platform Value Engine** — incident milestone timestamps, comparable baseline cohorts, attribution facts and customer value dashboard.
6. **Controlled remediation** — bounded reversible action classes first; authorization/approval/blast-radius/preflight/validation/rollback mandatory.
7. **Triggered packet evidence** — only after security, privacy, disk/resource and privilege gates pass.
8. **Certified SaaS/carrier packs** — expand based on customer demand and provider API/commercial viability.

## Cross-cutting acceptance

Every work item must include:

- tenant isolation;
- RBAC/resource scope;
- audit/correlation IDs;
- bounded queues/queries/resource use;
- failure/partial-data behavior;
- replay/idempotency where relevant;
- self-observability;
- migration/upgrade behavior;
- security/abuse tests;
- performance impact;
- UI error/partial/stale states;
- documentation/traceability.

## Required value methodology

MTTR/value features may not publish a claimed improvement unless the UI can show the baseline period, sample size, matching/cohort dimensions and data limitations. Financial benefit requires customer-approved cost assumptions.

## Release posture

The workstream is not complete when the feature code exists. Release requires the applicable Mother Reference consolidation, tests, operational runbooks, failure evidence and GA quality gates.

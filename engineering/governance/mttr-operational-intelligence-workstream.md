# MTTR / Operational Intelligence Workstream

**Status:** NON-MOTHER engineering sequencing control  
**Authoritative source:** `project_documentation/Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md`  
**Mother Reference:** Platform v3.2.2 after merge to canonical `main`

## Objective

Implement the approved `PRD-OIV-*` / `REQ-OIV-*` capabilities as coherent vertical slices without creating a separate monolithic AI subsystem or weakening existing GA gates.

## Workstream sequence

1. **Connector/change foundation** — normalize external deployment/change facts and extend connector contracts for SaaS/carrier integrations.
2. **Incident Flight Recorder** — evidence-window contracts, evidence references, graph/timeline replay, quality/completeness telemetry.
3. **Platform Value Engine foundation** — incident milestone timestamps, comparable baseline cohorts, contribution facts and value dashboard.
4. **Similar Incident Memory** — resolved-incident index, similarity features, accepted-outcome feedback and false-match tests.
5. **Cloud-native graph enrichment** — runtime Kubernetes/OTel/service-mesh dependency edges with provenance/confidence/history.
6. **Controlled remediation** — bounded reversible action classes first; authorization/approval/blast-radius/preflight/validation/rollback mandatory.
7. **Triggered packet evidence** — only after security, privacy, disk/resource and privilege gates pass.
8. **Certified SaaS/carrier packs** — expand based on customer demand and provider API/commercial viability.

## Cross-cutting acceptance

Every work item must include tenant isolation, RBAC/resource scope, audit/correlation IDs, bounded resource use, failure/partial-data behavior, replay/idempotency where relevant, self-observability, migration/upgrade behavior, security/abuse tests, performance impact, UI partial/stale/insufficient-evidence states and documentation/traceability.

## Required value methodology

MTTR/value features may not publish a claimed improvement unless the UI can show the baseline period, sample size, matching/cohort dimensions and data limitations. Financial benefit requires customer-approved cost assumptions.

## Release posture

Before the documentation PR is merged, this workstream is planning only. After merge, implementation may proceed against the formal `PRD-OIV-*` / `REQ-OIV-*` requirements.

Release requires applicable `Platform_19` §12 gates plus existing Mother Reference security, HA, upgrade, packaging, performance and GA evidence requirements.

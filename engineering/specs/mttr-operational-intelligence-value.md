# Platform Operational Intelligence, MTTR Reduction & Value — Implementation Navigation

**Status:** NON-MOTHER implementation navigation  
**Authoritative source:** `project_documentation/Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md`  
**Mother Reference:** Platform v3.2.2 after merge to canonical `main`

This file does not redefine product behavior. Engineering shall implement the formal `PRD-OIV-*` / `REQ-OIV-*` requirements in `Platform_19` and the existing Mother Reference contracts they supplement.

## Scope navigation

| Capability | Formal requirement |
|---|---|
| First-class SaaS monitoring | `PRD-OIV-001` / `REQ-OIV-001` |
| Triggered packet evidence | `PRD-OIV-002` / `REQ-OIV-002` |
| Cloud-native dependency discovery | `PRD-OIV-003` / `REQ-OIV-003` |
| External deployment/change telemetry | `PRD-OIV-004` / `REQ-OIV-004` |
| Carrier/ISP operational integration | `PRD-OIV-005` / `REQ-OIV-005` |
| Incident Flight Recorder | `PRD-OIV-006` / `REQ-OIV-006` |
| Similar Incident Memory | `PRD-OIV-007` / `REQ-OIV-007` |
| Cross-domain Change Intelligence | `PRD-OIV-008` / `REQ-OIV-008` |
| Controlled remediation | `PRD-OIV-009` / `REQ-OIV-009` |
| Platform Value / MTTR Engine | `PRD-OIV-010` / `REQ-OIV-010` |
| Per-incident Platform contribution | `PRD-OIV-011` / `REQ-OIV-011` |
| Four-layer customer-value dashboard | `PRD-OIV-012` / `REQ-OIV-012` |
| OIV self-observability | `PRD-OIV-013` / `REQ-OIV-013` |
| OIV role-aware APIs/UI/audit | `PRD-OIV-014` / `REQ-OIV-014` |

## Implementation order

1. Connector/change normalization for SaaS/carrier/external changes.
2. Flight Recorder evidence contracts/orchestration and incident UI.
3. Platform Value timestamp/attribution model and dashboard foundation.
4. Similar Incident Memory.
5. Cloud-native runtime graph enrichment.
6. Controlled remediation for bounded action classes.
7. Triggered packet evidence after security/resource controls pass.
8. Expand certified SaaS/carrier packs based on demand/API viability.

## Mandatory architecture boundaries

- Reuse the existing modular Core/Sensor architecture; do not build one monolithic AI service.
- Reuse PostgreSQL/ClickHouse/Kafka/Valkey before proposing another mandatory database.
- No dedicated vector database without benchmark + architecture + OSS approval.
- Triggered packet evidence is bounded diagnostics, not full always-on DPI.
- No LLM-direct privileged remediation.
- Similarity/provider/change evidence is advisory/supporting evidence, not causal proof.
- Value calculations must expose cohort/sample methodology and must not fabricate customer or financial savings.

## Merge gate

Before the documentation PR is merged, this file is non-authoritative planning. After merge, implementation may proceed against `Platform_19` as part of Mother Reference v3.2.2.

## Definition of done

Follow `Platform_19` §12 and existing Mother quality gates. Code existence is not completion; implementation, tenant isolation, security, failure behavior, observability, migration/upgrade impact, performance/resource evidence, UI behavior and operational runbooks are required.
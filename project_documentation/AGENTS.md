# Platform Repository Engineering Instructions

This file should be available at the repository root of a working clone (commonly `~/nms/AGENTS.md`). For ChatGPT Project/source management it may also be stored in `project_documentation`; the preferred repository setup is a root copy or symlink to this authoritative file. It is a navigation and enforcement file, not a replacement for the mother-reference documents.

**Mother-reference version:** 3.2.2  
**Canonical repository (USER-FIXED):** https://github.com/gagandsm-max/platform  

Local clones (including `~/nms`) are working copies. Changes become authoritative only after push and merge into the canonical GitHub repository.

## Authority chain

Before changing code or build/deployment assets, read in this order:

1. `project_documentation/Platform_00_Document_Index.md`
2. Relevant BRD/PRD requirement(s)
3. `Platform_03_Architecture.md`
4. Relevant HLD/LLD/Dataflow sections
5. `Platform_07_OSS_Catalog.md`
6. Relevant specialized master (`09` through `19`)
7. **Mandatory when the task touches operational intelligence/value:** `Platform_19_Operational_Intelligence_MTTR_and_Value.md`
8. `Platform_12_Decision_Source_Register.md` when a decision/source is unclear

If documents conflict, stop and resolve through the precedence defined in `Platform_00_Document_Index.md`. Do not invent a convenient interpretation.

## Canonical GitHub governance (USER-FIXED)

- GitHub repository `https://github.com/gagandsm-max/platform` is the system of record.
- Use short-lived branches for normal development; material changes go through pull requests.
- Do not push normal development changes directly to the protected/default branch.
- Do not force-push shared/default branches.
- Commits must be traceable to the change being made; preserve decision provenance in history.
- Selecting GitHub does **not** freeze a CI provider. GitHub Actions is not a fixed architecture decision unless separately approved.

## Mother-document change control (READ-MOSTLY)

Mother documents under `project_documentation/` are **READ-MOSTLY** and authoritative.

Do **not casually** modify them while implementing features.

A mother document **MUST** be updated when:

1. a factual/documentation error is identified;
2. two mother documents contradict one another;
3. an approved product decision changes;
4. an approved architecture decision changes;
5. an implementation dependency becomes formally selected;
6. a P0/TBD item is resolved;
7. a requirement was accidentally omitted;
8. an implemented behavior would otherwise conflict with documented truth.

Such updates require deliberate change control, cross-document reconciliation, Decision Register update where a decision changes, and GitHub review/merge. No implementation may permanently diverge from mother-reference truth. If implementation discovers a mother-document error, correct the mother documents rather than knowingly preserve a false specification.

## Non-negotiable engineering rules

- Product name is **Platform**.
- Clean-room implementation. Do not copy ThoughtData, Enterprise360 or NetAI proprietary code/assets.
- All routine local development and automated testing run in Docker Compose. Do not install project-specific Java/Go/Python/Node/Kafka/PostgreSQL/ClickHouse/Valkey dependencies directly on the developer laptop.
- Docker development must be reboot-safe through named volumes, health checks, deterministic startup and documented reset/recovery.
- Production Core/Sensor/Linux Agent are native `.deb` and `.rpm` packages. Windows Agent is a compiled code-signed Windows Service packaged as MSI, with BAT wrappers for scripted install/uninstall.
- Core, Sensor and Agents auto-start after host reboot and implement supervised recovery with visible crash-loop diagnostics.
- Structured logs, stable error codes, correlation IDs, health state and support diagnostics are mandatory. Never log secrets.
- Only dependencies approved in `Platform_07_OSS_Catalog.md` may ship. New OSS requires catalogue + transitive-license/CVE/SBOM review before merge.
- Two full Core nodes only for HA: Active/Standby, <60s failover target. Sensors continue collecting/buffering during Core failover while capacity permits (80% storage protection; absolute zero-loss is not promised).
- Sensor/Agent external transport is HTTPS/gRPC over mTLS; Kafka is internal.
- Core and Sensor 80% storage protection is mandatory.
- UI is HTTPS-only; no application listener on port 80.
- Do not declare device-down solely because SNMP or another primary monitor fails when corroborating evidence shows the device remains reachable.
- AI/GNN output is evidence/hypothesis support, not unquestioned causal truth. RCA must support INCONCLUSIVE.
- First-class SaaS monitoring, carrier/ISP enrichment, external change/deployment telemetry, Incident Flight Recorder, Similar Incident Memory, cloud-native dependency discovery, cross-domain Change Intelligence, controlled remediation and Platform Value/MTTR evidence are governed by `Platform_19_Operational_Intelligence_MTTR_and_Value.md`.
- Triggered packet evidence is bounded incident diagnostics, not always-on DPI. Privilege, capture scope, duration/volume, retention, tenant isolation and resource limits are mandatory.
- Controlled remediation never means direct LLM privileged execution. Use the approved diagnose → precondition/blast-radius → approval/policy → execute → validate → rollback/escalate workflow.
- Never claim customer MTTR or financial benefit without the evidence/methodology required by Platform_19.

## Definition of done

A task is not complete until code, tests, observability, error behavior, upgrade/migration impact, OSS/security implications and relevant documentation/traceability are complete.

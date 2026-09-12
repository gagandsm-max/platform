# Platform Repository Engineering Instructions

This file should be available at the repository root of a working clone (commonly `~/nms/AGENTS.md`). For ChatGPT Project/source management it may also be stored in `project_documentation`; the preferred repository setup is a root copy or symlink to this authoritative file. It is a navigation and enforcement file, not a replacement for the mother-reference documents.

**Mother-reference version:** 3.2.2  
**Canonical repository (USER-FIXED):** https://github.com/gagandsm-max/platform  

Local clones (including `~/nms`) are working copies. Changes become authoritative only after push and merge into the canonical GitHub repository.

## Authority chain

Before changing code or build/deployment assets, read in this order:

1. `project_documentation/Platform_00_Document_Index.md`
2. Relevant BRD/PRD requirement(s), including formal specialized-master supplements where applicable
3. `Platform_03_Architecture.md`
4. Relevant HLD/LLD/Dataflow sections
5. `Platform_07_OSS_Catalog.md`
6. Relevant specialized masters discovered from `Platform_00_Document_Index.md`
7. `Platform_12_Decision_Source_Register.md` when a decision/source is unclear

For work involving SaaS monitoring, carrier/ISP integration, external changes/deployments, cloud-native dependencies, Incident Flight Recorder, Similar Incident Memory, triggered packet evidence, controlled remediation, MTTR/value measurement or customer-value dashboards, **`Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` is mandatory preflight reading**.

For work involving network discovery review, Agent onboarding/enrollment identity, dynamic endpoint identity/reconciliation, approval/exclusion workflow, or the in-product Notification Center, **`Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md` is mandatory preflight reading**.

For work involving normalized SD-WAN, SCIM lifecycle, SLI/SLO/error budgets, audit tamper evidence, middleware/backup/SASE connectors, tenant offboarding/data portability, licensing safety, declarative configuration/policy-as-code, Incident Command Workspace, privacy-bounded endpoint DEX, browser synthetics, customer-operated outside-in probes or future regional federation, **`Platform_21_Enterprise_Gap_Closure_Master.md` is mandatory preflight reading** when that specialized master is present in the governing branch/index.

For work involving alert qualification/evidence contracts, telemetry-quality trust, semantic monitoring states, alert-policy lifecycle/backtesting/shadowing, false-alert measurement, active evidence acquisition, evidence coverage, RCA confidence/contradictions, AI/ML evaluation, Incident Copilot, AI provider/egress controls, tenant-scoped learning, prompt-injection/tool-use controls, shared-service dependency monitoring, PKI assurance, predictive operations or AI observability, **`Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` is mandatory preflight reading** when that specialized master is present in the governing branch/index.

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

Such updates require deliberate change control, cross-document reconciliation, Decision Register update where a decision changes, and GitHub review/merge.
No implementation may permanently diverge from mother-reference truth. If implementation discovers a mother-document error, correct the mother documents rather than knowingly preserve a false specification.

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
- Alert policy must use class-appropriate evidence contracts; telemetry/collector health is evaluated before operational failure is asserted.
- Alert severity and evidentiary confidence are separate dimensions. `MONITORING_DEGRADED`, `REDUNDANCY_LOST`, `SLA_DEGRADED`, `EXPECTED_STATE`, `UNKNOWN` and `STALE` must not be collapsed into generic DOWN semantics.
- AI/GNN output is evidence/hypothesis support, not unquestioned causal truth. RCA must support `INCONCLUSIVE`.
- GNN/LLM output must never be treated as causal authority merely because the model score or wording is confident.
- LLM/AI tool use is mediated through deterministic allowlisted actions, tenant/resource authorization, schema validation and policy enforcement; prompt content cannot grant privilege.
- Learning/evaluation data is tenant-scoped unless explicitly authorized otherwise; no silent cross-tenant training or evidence mixing.
- Core monitoring/RCA must remain operational when hosted AI is unavailable; graceful AI degradation cannot disable deterministic monitoring.
- Triggered packet evidence must remain bounded, privileged, tenant-scoped, auditable and resource-limited; it does not authorize full always-on DPI.
- Similar Incident Memory is advisory; historical similarity must not silently become current root cause.
- External provider/change evidence is supporting evidence; provider text or temporal proximity alone is not causal proof.
- Controlled remediation requires versioned deterministic actions, tenant/resource authorization, preconditions, blast-radius controls, approval/policy, audit, post-action validation and rollback/fail-safe behavior. LLM output may not directly execute privileged actions.
- Platform Value/MTTR metrics must expose data quality, cohort/sample methodology and assumptions. Do not fabricate generic customer savings or causal percentages.

## Definition of done

A task is not complete until code, tests, observability, error behavior, upgrade/migration impact, OSS/security implications and relevant documentation/traceability are complete.

For OIV capabilities, Definition of Done also includes applicable tenant isolation, evidence quality, packet-artifact protection, similarity false-positive, remediation deny/rollback, value-attribution and resource-regression tests defined in `Platform_19`.

For enterprise-gap capabilities governed by `Platform_21`, Definition of Done also includes applicable canonical-model/connector contract tests, tenant isolation, least-privilege credential handling, collector-health separation, failure-mode behavior, upgrade/backward compatibility, audit evidence and the formal `REQ-EGC-*` acceptance evidence.

For monitoring-intelligence/AI-assurance capabilities governed by `Platform_22`, Definition of Done also includes the applicable Alert Evidence Contract, telemetry-quality negative cases, semantic-state behavior, false-alert/backtest/shadow evidence, confidence/evidence coverage, contradiction handling, tenant-isolation and data-egress tests, prompt-injection/tool-firewall tests, graceful AI degradation, model/data-drift observability and the formal `REQ-MIA-*` acceptance evidence.

## v3.2.2 discovery, onboarding, identity and notification safeguards

- Network auto-discovery and Agent enrollment feed a governed onboarding/review lifecycle. Newly found/enrolled entities do not silently become licensed/actively monitored unless an explicit authorized trusted auto-approval policy applies.
- Approve, reject, unmanaged and exclude decisions are tenant-scoped, authorized and audited with actor, timestamp and operator comment/reason; bulk decisions preserve per-item result/evidence.
- Before activation, authorized users can review/enrich Display Name, Alias, Description, Site, Group/Sub-group, Tags, Criticality, monitoring profile, alert/validation policy inheritance and applicable Sensor/licence/capacity impact.
- Agent-backed endpoint identity is anchored to canonical `entity_id` plus unique `agent_id`/mTLS identity. System/host name is the preferred human-readable technical name; IP/MAC are mutable observations and must not create a new endpoint solely because DHCP/VPN/Wi-Fi changed them.
- Reinstall, reimage, cloned Agent identity, hostname collision and conflicting stable identifiers require explicit reconciliation; never silently merge or duplicate inventory.
- The in-product Notification Center is one role-aware global surface. Recipient access and deep links are enforced server-side by tenant, RBAC, resource scope and subscription; operational Alerts/Incidents remain distinct from platform/workflow notifications.
- Discovery/onboarding notification generation is deduplicated, bounded and rate-limited so scans, rediscovery or Agent reconnect storms cannot create notification floods.

## v3.2.2 enterprise gap-closure safeguards

- Vendor SD-WAN/SASE/backup/middleware integrations normalize into Platform-owned contracts; vendor identifiers never override tenant/entity authority.
- SCIM v2 is a compatibility target until independently validated; do not claim certification/compliance by specification alone.
- SLI/SLO/error-budget calculations expose scope, windows, exclusions and evidence; contractual commitments remain Founder-reserved unless formally approved.
- Audit integrity must provide independently verifiable tamper evidence without assuming a cryptographic construction before architecture/security review.
- Tenant offboarding distinguishes active-data deletion, backup/archive retention or expiry, legal hold and final destruction evidence; never promise immediate physical erasure from immutable backups.
- Existing seven-day licence grace remains governed behavior; do not invent prices, quantities or SKUs.
- Policy-as-code/Git inputs are declarations only and cannot bypass Platform RBAC, validation, approval, audit or runtime safety.
- Endpoint DEX is operational-experience monitoring, not employee surveillance; prohibited content/surveillance collection remains out of scope.
- Browser synthetics, outside-in probes and regional federation do not select a runtime/vendor/dependency or reopen v1 exactly-two-Core Active/Standby by implication.

## v3.2.2 monitoring-intelligence, alert-quality and AI-assurance safeguards

- Every material operational assertion uses a class-specific Alert Evidence Contract; single-source failure does not become authoritative failure when contradictory evidence remains credible.
- Evaluate collector reachability, freshness, timestamp quality, source health and evidence gaps before classifying the monitored resource as failed.
- Alert severity expresses operational impact; confidence expresses evidentiary support. Never substitute one for the other.
- New/changed alert policies progress through governed `DRAFT -> BACKTEST -> SHADOW -> ACTIVE` lifecycle where applicable, with reproducible evidence and no invented universal quality threshold.
- Active evidence acquisition is bounded, authorized, tenant-scoped, rate/resource limited and must not create a self-induced outage or bypass protocol/security policy.
- RCA and Incident Copilot outputs expose observed facts, likely cause, confidence, supporting evidence, contradictions, missing evidence and next validation; `INCONCLUSIVE` is an acceptable correct result.
- Operator feedback is structured/audited input to evaluation and policy/model improvement; it must not silently mutate active alert policy or model behavior.
- Prompt/model output cannot directly invoke privileged actions. Deterministic tool-use policy, schema validation, authorization and audit remain authoritative.
- Hosted/private/local/no-LLM provider modes must preserve data-egress controls and deterministic fallback; core monitoring cannot require Internet or an LLM/GNN to function.
- AI/ML releases require reproducible evaluation, provenance, model/data-drift visibility and resource-budget evidence. Champion/challenger decisions require measured evidence, not vendor/model reputation.
- Shared services such as DNS/DHCP/NTP/PTP/RADIUS/TACACS+/LDAP/AD and PKI/certificate state are dependency evidence for incident reasoning; facilities/power evidence may explain infrastructure incidents but does not turn Platform into BMS/DCIM.
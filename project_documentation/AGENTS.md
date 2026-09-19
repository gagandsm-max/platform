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

For work involving enterprise monitoring-domain catalogue, Capability Pack contract/lifecycle, dimensional L1/L2/L3 certification, Customer Acceptance Profiles, Real Customer Requirements normalization, operational-graph versus governed-CMDB semantics, or coverage measurement, **`Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` is mandatory preflight reading** when that specialized master is present in the governing branch/index.

For work involving NOC no-context-switch troubleshooting, contextual action bars/drawers, embedded CLI, on-demand or multi-vantage diagnostics, `Verify Now`, Live Evidence, Investigation Sessions, or diagnostic workflow definitions, mandatory preflight reading includes **`Platform_09_UI_UX_Master.md`, `Platform_10_Protocol_Security_Connector_Master.md`, `Platform_11_Sizing_Capacity_Storage_Master.md`, `Platform_17_API_Event_and_Data_Model_Contracts.md`, `Platform_18_Security_Threat_Model.md` and `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md`**. Read `Platform_23` as well when technology/Capability Pack diagnostics are involved.

If documents conflict, stop and resolve through the precedence defined in `Platform_00_Document_Index.md`. Do not invent a convenient interpretation.

## Canonical GitHub governance (USER-FIXED)

- GitHub repository `https://github.com/gagandsm-max/platform` is the system of record.
- Use short-lived branches for normal development; material changes go through pull requests.
- Do not push normal development changes directly to the protected/default branch.
- Do not force-push shared/default branches.
- Commits must be traceable to the change being made; preserve decision provenance in history.
- Selecting GitHub does **not** freeze a CI provider. GitHub Actions is not a fixed architecture decision unless separately approved.

## Tool authority model

- **ChatGPT + GitHub is the Platform control plane.** It owns source-truth rebaselining, architecture/technical decisions, task sequencing, PR integration, merge authority, progress accounting and final technical acceptance.
- **Cursor CLI / Cursor Cloud are the primary implementation writers.** They operate only on explicitly assigned branches/worktrees and bounded scope. A Cursor writer never self-certifies merge readiness.
- **Antigravity CLI is verifier-first and read-only/adversarial by default.** Write authority is exceptional, explicit and isolated to a dedicated branch/worktree; Antigravity must not become a second writer on an active Cursor branch.
- **Codex CLI / GitHub Codex is an independent exact-SHA adversarial auditor and certification gate when explicitly invoked.** It is not a routine developer/fixer and must not write production changes unless ChatGPT explicitly assigns an isolated implementation lane.
- **One writer per branch/worktree.** Parallelism is for independent review/evidence by default; parallel writes require isolated branches/worktrees and later controlled integration.
- Every tool must re-fetch authoritative `main` and exact task/PR head before acting. Main/head drift invalidates stale review.
- No writer may close `REALITY_GATE`, `MERGE_READY`, `POST_MERGE_VERIFY` or `COMPLETE` for its own work.

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
- NOC troubleshooting is a first-class UI workflow: preserve device/interface/alert/incident context while authorized users launch bounded on-demand diagnostics, multi-vantage verification, browser CLI and investigation evidence capture. Low-risk diagnostics must be fast; privileged/configuration actions require proportionate RBAC/JIT/MFA/reason/approval controls.
- On-demand diagnostics must run through Platform-controlled Core/Sensor/Agent paths, not direct browser-to-device networking; arbitrary cross-tenant targets, unbounded continuous tests and browser exposure of reusable credentials are prohibited.

## Definition of done

A task is not complete until code, tests, observability, error behavior, upgrade/migration impact, OSS/security implications and relevant documentation/traceability are complete.

For OIV capabilities, Definition of Done also includes applicable tenant isolation, evidence quality, packet-artifact protection, similarity false-positive, remediation deny/rollback, value-attribution and resource-regression tests defined in `Platform_19`.

For enterprise-gap capabilities governed by `Platform_21`, Definition of Done also includes applicable canonical-model/connector contract tests, tenant isolation, least-privilege credential handling, collector-health separation, failure-mode behavior, upgrade/backward compatibility, audit evidence and the formal `REQ-EGC-*` acceptance evidence.

For monitoring-intelligence/AI-assurance capabilities governed by `Platform_22`, Definition of Done also includes the applicable Alert Evidence Contract, telemetry-quality negative cases, semantic-state behavior, false-alert/backtest/shadow evidence, confidence/evidence coverage, contradiction handling, tenant-isolation and data-egress tests, prompt-injection/tool-firewall tests, graceful AI degradation, model/data-drift observability and the formal `REQ-MIA-*` acceptance evidence.

For enterprise-monitoring-coverage / Capability Pack capabilities governed by `Platform_23`, Definition of Done also includes dimensional certification evidence, pack compatibility/version evidence, tenant isolation, least-privilege credential handling, collector-health separation, failure-mode behavior and the formal `REQ-EMC-*` acceptance evidence. Specification of a technology family is not certification.

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

## v3.2.2 enterprise monitoring coverage and Capability Pack safeguards

- Capability Pack is the only extensibility architecture. Technology Monitoring, Device/Network, SaaS Monitoring, Integration and optional Customer Extension packs are classifications of that architecture, not competing frameworks.
- Certification is dimensional. Do not collapse Discovery/L1/L2/L3/Topology/NCM/Security into one scalar support claim.
- Named products in `Platform_23` are certification targets, not Core hard-coding, shipped packs or achieved certification.
- Coverage metrics remain distinct (architecture/specification/implementation/pack/certification/integration). Specified is not implemented; tested is not certified; certified is not deployed.
- OEM/customer extensions use supported pack/SDK mechanisms. Core forks are prohibited.
- Always-on full DPI, unrestricted SIEM replacement, full bytecode APM, broad BMS/DCIM and uncontrolled automated NCM remain excluded or deferred as recorded in `Platform_23`.

## Cursor Cloud specific instructions

Hosted Cloud Agents use `.cursor/environment.json` and `.cursor/Dockerfile`. The GitHub repository is `https://github.com/gagandsm-max/platform`.

Cloud VMs follow the same contract as a developer laptop:

- Do not install project-specific Java, Go, Python, Node, Kafka, PostgreSQL, ClickHouse, or Valkey on the Cloud VM host.
- All routine development and automated testing run through Docker Compose (`make doctor`, `make certs`, `make dev-up`, `make test-unit`).
- Copy `.env.example` to `.env` when `.env` is missing. Never commit `.env`, generated certs, or keys.
- The environment `start` command starts Docker. Then run `make certs` if `deploy/docker/certs/platform-dev.crt` is missing.
- Do not start the full Compose stack unless the assigned task needs running services. The stack is memory-heavy.
- Default loopback ports: UI `23000`, Core `28443`.
- A personal My Machines worker named `platform-dev` can execute Cloud Agent tool calls against this local checkout.

## Engineering capability pack (routing)

Do not paste the agent or Skill catalogue here. Resolve it from manifests.

- Canonical agents: `project_agents/canonical/` via `project_agents/manifest.json`
- Canonical Skills: `project_agents/skills/` via `project_agents/skills/manifest.json`
- Generated Cursor subagents: `.cursor/agents/`
- Generated Antigravity agents: `.agents/agents/`
- Generated shared Skills (Cursor + Antigravity): `.agents/skills/`
- Legacy Skill mirror: `skills/`
- Codex compatibility projection: `.codex/agents/` (not a v1.2 certification gate)
- Routing: `project_agents/AGENT_MATRIX.md`, `project_agents/CHATGPT_AGENT_ROUTER.md`, `project_agents/skills/AGENT_SKILL_MATRIX.md`, `project_agents/skills/SKILL_ROUTER.md`
- Tool contracts: `project_agents/TOOL_COMPATIBILITY.md`
- Writer isolation: one writer per checkout; parallel writes use separate branches/worktrees
- Evidence: typed claims, exact-head current-base CI, no writer self-certification

Capability Pack v1.2 currently resolves 12 P0 + 22 P1 agents and a bounded Skill library from those manifests. Those counts are snapshots, not a frozen product requirement. Platform product version remains v3.2.2. Current Mother Reference file count and set are discovered from `Platform_00_Document_Index.md` §3; do not freeze a historical file-count ceiling.

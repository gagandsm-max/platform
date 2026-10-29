# Platform Repository Engineering Instructions

This file should be available at the repository root of a working clone (commonly `~/nms/AGENTS.md`). For ChatGPT Project/source management it may also be stored in `project_documentation`; the preferred repository setup is a root copy or symlink to this authoritative file. It is a navigation and enforcement file, not a replacement for the mother-reference documents.

**Mother-reference version:** 3.3.0
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

For work involving normalized SD-WAN, SCIM lifecycle, SLA/OLA/Provider-SLA service assurance, SLI/SLO/error budgets, audit tamper evidence, middleware/backup/SASE connectors, tenant offboarding/data portability, licensing safety, declarative configuration/policy-as-code, Incident Command Workspace, privacy-bounded endpoint DEX, browser synthetics, customer-operated outside-in probes or future regional federation, **`Platform_21_Enterprise_Gap_Closure_Master.md` is mandatory preflight reading** when that specialized master is present in the governing branch/index.

For work involving alert qualification/evidence contracts, telemetry-quality trust, semantic monitoring states, alert-policy lifecycle/backtesting/shadowing, false-alert measurement, active evidence acquisition, evidence coverage, RCA confidence/contradictions, AI/ML evaluation, Incident Copilot, AI provider/egress controls, tenant-scoped learning, prompt-injection/tool-use controls, shared-service dependency monitoring, PKI assurance, predictive operations or AI observability, **`Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` is mandatory preflight reading** when that specialized master is present in the governing branch/index.

For work involving enterprise monitoring-domain catalogue, Capability Pack contract/lifecycle, dimensional L1/L2/L3 certification, Customer Acceptance Profiles, Real Customer Requirements normalization, operational-graph versus governed-CMDB semantics, or coverage measurement, **`Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` is mandatory preflight reading** when that specialized master is present in the governing branch/index.

For work involving NOC no-context-switch troubleshooting, contextual action bars/drawers, embedded CLI, on-demand or multi-vantage diagnostics, `Verify Now`, Live Evidence, Investigation Sessions, or diagnostic workflow definitions, mandatory preflight reading includes **`Platform_09_UI_UX_Master.md`, `Platform_10_Protocol_Security_Connector_Master.md`, `Platform_11_Sizing_Capacity_Storage_Master.md`, `Platform_17_API_Event_and_Data_Model_Contracts.md`, `Platform_18_Security_Threat_Model.md` and `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md`**. Read `Platform_23` as well when technology/Capability Pack diagnostics are involved.

For work involving Unified Network & Infrastructure Observability Phase 1, active path observability (flow-stable / multi-vantage path tracing), bounded Linux Agent eBPF socket telemetry, cloud network depth, or OTLP trace context boundaries, mandatory reading includes **`Platform_01_BRD.md`, `Platform_02_PRD.md`, `Platform_03_Architecture.md`, `Platform_05_LLD.md`, `Platform_10_Protocol_Security_Connector_Master.md`, `Platform_11_Sizing_Capacity_Storage_Master.md`, `Platform_14_Test_Performance_and_Release_Quality.md`, `Platform_17_API_Event_and_Data_Model_Contracts.md`, `Platform_18_Security_Threat_Model.md`, `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` and `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md`**.

If documents conflict, stop and resolve through the precedence defined in `Platform_00_Document_Index.md`. Do not invent a convenient interpretation.

## Canonical GitHub governance (USER-FIXED)

- GitHub repository `https://github.com/gagandsm-max/platform` is the system of record.
- Use short-lived branches for normal development; material changes go through pull requests.
- Do not push normal development changes directly to the protected/default branch.
- Do not force-push shared/default branches.
- Commits must be traceable to the change being made; preserve decision provenance in history.
- Selecting GitHub does **not** freeze a CI provider. GitHub Actions is not a fixed architecture decision unless separately approved.

## Tool authority model

- **ChatGPT + GitHub is the Platform control plane.** It owns source-truth rebaselining, architecture/technical decisions, task sequencing, PR integration, merge authority, progress accounting and final technical acceptance/certification. It does not write product code.
- **Cursor CLI is the default bounded implementation writer.** It operates only on explicitly assigned branches/worktrees and bounded scope. A Cursor writer never self-certifies merge readiness and requires no `writerElevation`.
- **Codex is an additional authorized bounded implementation, test and debugging writer** (Founder decision 2026-10-07). Codex CLI and ChatGPT Desktop App in Codex mode are approved client interfaces to the same Codex authority, not separate writers or certification authorities. Codex cannot independently verify, act as gatekeeper, merge or self-certify. Plain ChatGPT Desktop conversation is not assumed to have repository filesystem/execution access unless Codex mode or explicitly connected local-computer access is active.
- **Cursor Cloud is a never-writer under current policy.** Elevation cannot authorize it; any future change requires an explicit Founder decision recorded in `project_agents/tool_authority.json`.
- **Antigravity CLI is read-only/adversarial.** It is the independent exact-SHA auditor, negative/failure/security challenger and certification-gate evidence provider. It is not an implementation writer under the active commercial-readiness workflow, cannot validate its own work, cannot self-certify, and is not final certification authority — ChatGPT + GitHub performs final technical certification.
- **GitHub Copilot is a never-writer** and limited auxiliary PR/code-review only. Elevation cannot authorize it as writer, verifier, gatekeeper, merge authority, or sole certification evidence.
- **One writer per checkout/worktree.** Cursor CLI and Codex may run concurrently only on isolated branches/worktrees with non-overlapping file ownership and later controlled integration.
- Every tool must re-fetch authoritative `main` and exact task/PR head before acting. Main/head drift invalidates stale review.
- No writer may close `REALITY_GATE`, `MERGE_READY`, `POST_MERGE_VERIFY` or `COMPLETE` for its own work.
- Authoritative machine-readable source: `project_agents/tool_authority.json`.

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

- Product name is **Platform** (internal engineering/build identifier); commercial/customer-facing product name is **AmrudAi**.
- Customer-facing UI displays effective commercial branding (defaulting to AmrudAi, or licensed tenant/MSP branding where white-label is active). Generated, downloaded and scheduled report artifacts are **PDF and/or CSV only**. All generated PDF reports must carry the effective commercial logo (defaulting to the governed AmrudAi logo, or licensed tenant/MSP logo where white-label is active); CSV reports contain tabular data only.
- Clean-room implementation. Do not copy ThoughtData, Enterprise360 or NetAI proprietary code/assets.
- All routine local development and automated testing run in Docker Compose. Do not install project-specific Java/Go/Python/Node/Kafka/PostgreSQL/ClickHouse/Valkey dependencies directly on the developer laptop.
- Docker development must be reboot-safe through named volumes, health checks, deterministic startup and documented reset/recovery.
- Production Core/Sensor/Linux Agent are native `.deb` and `.rpm` packages. Windows Agent is a compiled code-signed Windows Service packaged as MSI, with BAT wrappers for scripted install/uninstall.
- Every mandatory runtime service required by the enabled deployment profile auto-starts after host reboot and implements bounded supervised recovery with dependency-aware readiness and visible crash-loop diagnostics. This includes Core/Sensor/Agents and separately managed mandatory Core data/runtime services such as PostgreSQL, ClickHouse, Kafka and Valkey where applicable. Optional/on-demand workers run only when required; customers must not need routine manual process starts after reboot or a recoverable crash. Automatic boot/start and supervised recovery must preserve existing authority, role, fencing, replication, durability and rejoin contracts; they must not self-promote Standby, bypass Core Active/Standby fencing, bypass PostgreSQL/Kafka/ClickHouse replication/durability/safe-rejoin rules, create dual-active authority, repeatedly force-start known-unsafe local state, or treat process existence as recovery success. If safe automatic recovery cannot be established, the component remains FAILED/DEGRADED/RECOVERING with actionable evidence and a governed operator/runbook path. Standalone/non-HA bounded supervised restart remains mandatory. BUILD-120–124 remain the owners of exactly-two-Core HA authority/fencing and stateful failover/rejoin; do not invent Patroni/repmgr/Keeper/etcd/Consul/Raft/witness/third-Core or a new supervisor service here.
- Structured logs, stable error codes, correlation IDs, health state and support diagnostics are mandatory. Never log secrets.
- Only dependencies approved in `Platform_07_OSS_Catalog.md` may ship. New OSS requires catalogue + transitive-license/CVE/SBOM review before merge.
- Phase 1 certification envelope uses one authoritative standalone Core (no Core-HA availability claim). Exactly-two-Core Active/Standby HA (<60s failover target) remains fixed architecture and is a Phase 1.5 certification envelope (BUILD-120–124). Sensors continue collecting/buffering during Core unavailability/failover while capacity permits (80% storage protection; absolute zero-loss is not promised). HA is distinct from DR.
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
- Product positioning for Phase 1 is **Unified Network & Infrastructure Observability** (target capability set subject to applicable implementation, benchmark and certification gates); it does not authorize full APM (code-level transaction tracing, Java/.NET bytecode manipulation/instrumentation, method-level timing), continuous profiling, flame graphs, RUM, full DPI, always-on PCAP, TLS decryption, packet reconstruction, mandatory GPU/NPU, third Core, dedicated trace DB, dedicated graph DB, or separate eBPF backend. Triggered packet evidence remains bounded, privileged, tenant-scoped, and preserved under `Platform_19`.
- Bounded Linux Agent eBPF process-network evidence (`REQ-INF-013`) executes in-kernel socket telemetry with strict local aggregation (`process/service + canonical target + port + bounded configurable bucket`), using existing Agent→Sensor→Core durable paths and zero separate eBPF services or datastores. Protected workloads (authority, polling, transport, ingestion, alert qualification, deterministic RCA) strictly take precedence over eBPF enrichment; eBPF must drop/shed/suspend before primary monitoring is harmed.
- Active path observability (`REQ-NET-016`) runs bounded multi-vantage active path tracing preserving flow identity where supported to eliminate false multipath anomalies. Incomplete path evidence must never be collapsed into generic device down.
- OpenTelemetry ingestion accepts metric and log telemetry and preserves trace context references (`REQ-INF-010`); Phase 1.5 may add bounded customer-supplied trace and dependency evidence without committing to a full distributed trace backend.

## Phase 1 / Phase 1.5 certification envelopes (Issue #200 / #202)

Frozen certification envelopes for Mother Reference reconciliation. Architecture target ≠ implementation ≠ tested ≠ benchmarked ≠ certified ≠ deployed. Tranche A retains Mother Reference set version **3.3.0**; set-wide advance to v3.4.0 is reserved for gated Tranche D.

**Phase 1 — single-customer enterprise (production, not lab MVP):**
- one authoritative standalone Core; no Core-HA service-continuity claim and no MSP/shared-customer isolation certification claim;
- multiple sites/regions, Sensors/Sensor Pools and Agents permitted;
- tenant-aware identity/API/event/security/RBAC/audit contracts remain mandatory even when only one customer is deployed;
- Sensor assignment generation/fencing, lease ownership and stale privileged-work rejection remain mandatory with a standalone Core;
- deletion-authorizing Sensor ACK must survive the certified standalone failure matrix (process/service crash/restart, governed host reboot and other explicitly certified recoverable failures); catastrophic single-Core/storage destruction is not Phase-1 zero-RPO HA;
- backup/restore and site-loss DR (redeploy/endpoint redirection for the declared Phase-1 DR profile) are mandatory; HA ≠ DR;
- deterministic monitoring/alerting/RCA must operate with LLM/GNN unavailable; `REQ-MIA-033` CPU-only applicability is preserved for certified local-AI profiles that declare Local AI/ML/LLM mandatory—it does not make LLM a Phase-1 unconditional requirement;
- NCM Phase-1 independently certifiable subset: config backup/version/diff/compliance/golden-state; controlled manual CLI only when RBAC/credential brokering/audit are certified. Mass push/canary/automated privileged remediation defaults to Phase 1.5 unless Founder change control promotes it.

**Phase 1.5 — HA / MSP / scale expansion (same architecture/codebase):**
- exactly two full Core nodes Active/Standby with positive fencing and split-brain prevention;
- PostgreSQL/Kafka/ClickHouse failover/rejoin, stable endpoint, Sensor reconnect/re-anchor and push-protocol HA continuity;
- `ACKED-FOR-DELETION TELEMETRY RPO = 0` for the certified permanent single-Core HA failure matrix (distinct from site/DR RPO);
- MSP/shared multi-tenancy adversarial isolation and noisy-neighbor certification;
- privileged automation (maker-checker, canary/bounded rollout, blast-radius, rollback where eligible) under existing NCM/remediation safety contracts.

BUILD-011 technical post-merge evidence at main `179a72837d050f4c757858452d3719c2e8978b95` does not imply Phase-1 GA, capacity, Core HA, or eBPF physical-transport certification. See `Platform_00` A.15, `Platform_03` and `Platform_12` Issue #200/#202 decision entries.

## Definition of done

A task is not complete until code, tests, observability, error behavior, upgrade/migration impact, OSS/security implications and relevant documentation/traceability are complete.

For OIV capabilities, Definition of Done also includes applicable tenant isolation, evidence quality, packet-artifact protection, similarity false-positive, remediation deny/rollback, value-attribution and resource-regression tests defined in `Platform_19`.

For enterprise-gap capabilities governed by `Platform_21`, Definition of Done also includes applicable canonical-model/connector contract tests, tenant isolation, least-privilege credential handling, collector-health separation, failure-mode behavior, upgrade/backward compatibility, audit evidence and the formal `REQ-EGC-*` acceptance evidence.

For monitoring-intelligence/AI-assurance capabilities governed by `Platform_22`, Definition of Done also includes the applicable Alert Evidence Contract, telemetry-quality negative cases, semantic-state behavior, false-alert/backtest/shadow evidence, confidence/evidence coverage, contradiction handling, tenant-isolation and data-egress tests, prompt-injection/tool-firewall tests, graceful AI degradation, model/data-drift observability and the formal `REQ-MIA-*` acceptance evidence.

For enterprise-monitoring-coverage / Capability Pack capabilities governed by `Platform_23`, Definition of Done also includes dimensional certification evidence, pack compatibility/version evidence, tenant isolation, least-privilege credential handling, collector-health separation, failure-mode behavior and the formal `REQ-EMC-*` acceptance evidence. Specification of a technology family is not certification.

For Unified Network & Infrastructure Observability capabilities governed by Phase 1 closure, Definition of Done also includes the 16 certification scenario gates defined in `Platform_14` Section 11 (`NIOCP-P1-v1`; future certification profile, NOT YET CERTIFIED until applicable evidence exists) and adherence to the `OBS-P1` capacity overlay defined in `Platform_11` Section 8.1.

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
- Founder-approved post-expiry licence grace is **15 days** (`EXPIRED_GRACE_15D`); the T-7 pre-expiry dashboard banner remains governed and is distinct from post-expiry grace. Do not invent prices, quantities or SKUs.
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

## v3.2.2 network and infrastructure observability safeguards

- In-kernel eBPF instrumentation operates under least-privilege (`CAP_BPF`/`CAP_PERFMON` or target-kernel equivalent under certified compatibility matrix) with BTF/CO-RE compatibility where supported, kernel map entry limits, ring-buffer drop accounting, and strict data minimization (zero payload, TLS, HTTP body, SQL text, env vars).
- Monitored Linux endpoints report explicit capability states (`UNSUPPORTED`, `UNAVAILABLE`, `PRIVILEGE_DENIED`, `KERNEL_INCOMPATIBLE`, `RESOURCE_GUARDED`) without crashing or stopping host metrics collection.
- Protected workloads (authority, polling, transport, ingestion, alert qualification, deterministic RCA) always take precedence over eBPF socket enrichment; eBPF rate/volume is bounded and must shed/sample before primary monitoring is harmed.
- Active path diagnostics enforce bounded probe rate limits and target count caps with non-O(N²) fan-out controls. Intermediate hop packet drops or multipath changes must not override positive health corroborated by ICMP or SNMP.
- Cloud network monitoring normalizes VPC/VNet topology, gateway metrics, and cloud/provider flow logs (SUPPORTED WHEN ENABLED/CERTIFIED, subject to provider capability, policy, cost, API rate limits, cardinality, storage, and certification; not a mandatory universal dependency) into canonical entity models; connector health is separated from cloud target operational status.

## Cursor Cloud policy (never-writer)

Cursor Cloud is a **never-writer** under current `project_agents/tool_authority.json`. Task-manifest `writerElevation` cannot authorize it. Do not route product implementation to Cursor Cloud unless the Founder explicitly changes that policy file.

If a Founder-authorized Cloud environment is later approved, it must still obey the laptop contract (Docker Compose only; no host-installed project runtimes; no secrets in git; UI `23000` / Core `28443`). Until that decision exists, use Cursor CLI on an assigned local/worktree checkout.

## Engineering capability pack (routing)

Do not paste the agent or Skill catalogue here. Resolve it from manifests.

- Canonical agents: `project_agents/canonical/` via `project_agents/manifest.json`
- Canonical Skills: `project_agents/skills/` via `project_agents/skills/manifest.json`
- Generated Cursor subagents: `.cursor/agents/`
- Generated Antigravity agents: `.agents/agents/`
- Generated shared Skills (Cursor + Antigravity): `.agents/skills/`
- Legacy Skill mirror: `skills/`
- Generated Codex agents: `.codex/agents/` (canonical read agents remain read-only; write agents use workspace-write; never an independent certification lane)
- Routing: `project_agents/AGENT_MATRIX.md`, `project_agents/CHATGPT_AGENT_ROUTER.md`, `project_agents/skills/AGENT_SKILL_MATRIX.md`, `project_agents/skills/SKILL_ROUTER.md`
- Tool contracts: `project_agents/TOOL_COMPATIBILITY.md`
- Writer isolation: one writer per checkout; parallel writes use separate branches/worktrees
- Evidence: typed claims, exact-head current-base CI, no writer self-certification

Capability Pack v1.3 currently resolves 12 P0 + 22 P1 agents and a bounded Skill library (63 Skills) from those manifests. Those counts are snapshots, not a frozen product requirement. Platform product version remains v3.3.0. Current Mother Reference file count and set are discovered from `Platform_00_Document_Index.md` §3; do not freeze a historical file-count ceiling.

---

## v3.2.2 Phase-1 endpoint operations and Triggered Fast RCA reconciliation

For work involving **installed-software inventory**, **customer-selectable process/service monitoring**, **Criticality-to-Monitoring-Profile mapping**, or **Triggered Fast RCA / Endpoint Diagnostic Snapshot (EDS)**, mandatory preflight reading includes `Platform_02`, `Platform_03`, `Platform_05`, `Platform_06`, `Platform_08`, `Platform_09`, `Platform_11`, `Platform_12`, `Platform_14`, `Platform_17`, `Platform_18`, `Platform_19`, `Platform_20`, `Platform_22`, `Platform_23` and the Master Build Sequence Register.

Non-negotiable boundaries:

- Installed-software inventory is a **Phase-1 / first-GA requirement** for supported Windows/Linux endpoints by Master architecture decision (`PHASE_1_ACCEPTED_BY_ARCHITECTURE_AUTHORITY`) after Founder objective + Phase-allocation delegation. It is inventory/change evidence, not patch management, software deployment, vulnerability management, licence metering, EDR or endpoint management.
- Linux BUILD-011 and Windows BUILD-014 own bounded OS-native source collection/normalization and typed `software_inventory_snapshot` evidence boundaries; BUILD-016/062/063/064 own structured transport/ingestion/storage foundations; BUILD-098 owns end-to-end reconciliation/current-state/change evidence/query. BUILD-030/031 agentless producers are optional when certified. Current scalar Agent CollectionBatch path must not fake structured inventory.
- Structured software inventory must not be forced into high-cardinality scalar metric labels merely to reuse the metric envelope. Software-change evidence is corroborating only and must not hard-block deterministic RCA.
- Process/service monitoring uses stable service/unit/executable selectors; PID is runtime evidence only and never durable monitor identity.
- Criticality describes business importance. Monitoring Profile describes monitoring behavior. `AUTO_FROM_CRITICALITY` and `EXPLICIT_PROFILE` are distinct assignment modes; explicit profile assignment is never silently replaced by a later Criticality change.
- Triggered Fast RCA is a **product experience over the existing Diagnostic Job / EDS / Live Evidence / Investigation / Incident Evidence Window / deterministic Core RCA architecture**. It is not a second endpoint RCA engine, diagnostic scheduler, evidence store, transport or causal authority.
- Built-in trigger-specific EDS definitions remain bounded and evidence-oriented; endpoint indicators are factual evidence qualifications, not unsupported causal conclusions.
- Guest Agents do not infer hypervisor cause. Physical/OOB, virtualization, storage and path evidence comes from the certified owning vantage and is correlated by Core with independent provenance.
- Routine monitoring, authority/control and telemetry durability always take priority over software inventory refreshes and diagnostic enrichment under resource pressure.


## v3.3.0 ACP-001..ACP-018 safeguards

For work involving Official Certified Capability Packs, Investigation Workbench, Path Intelligence, OpenTelemetry/OTLP, NCM Change Assurance, ITSM/CMDB synchronization, Mapping Studio, connector certification, endpoint/switch-port intelligence, browser journeys, database performance intelligence or virtualization optimization, preflight the exact current Mother References and requirement owners before implementation.

Non-negotiable boundaries:

- Capability Pack remains the only extensibility/pack architecture. Do not introduce a parallel Technology Pack registry, SDK or Core fork.
- Named Phase-1 and Phase-1.5 technologies/connectors are certification **targets** until exact product/version/dimension evidence passes; specification is not certification.
- Investigation Workbench reuses Investigation Session, Unified Operations Timeline, Flight Recorder, canonical graph and deterministic RCA. Path Intelligence reuses active-path + temporal graph. Do not create competing evidence/topology/RCA authorities.
- Phase-1 OTLP is metrics + structured logs + resource identity + trace/span correlation identifiers/exemplars where supplied. Retained sampled spans and trace waterfall/search are Phase 1.5 after explicit trace economics/security/sampling certification. Do not silently promote traces into the Phase-1 GA gate.
- OTLP tenant/source authority is server-derived from authenticated ingress. OpenTelemetry resource attributes never select tenant authority. No mandatory trace database is authorized.
- NCM execution requires exact Change Plan/version/target/configuration/window approval binding plus local Platform RBAC/policy/preconditions. ITSM approval alone never authorizes unsafe execution. Semantic diff and rollback are capability-certified, never universal. Firmware is Phase 1.5.
- Integration Fabric is logical functionality of the existing Integration Service unless later evidence justifies a process boundary. Outbound uses transactional outbox; inbound uses authenticated inbox/event processing or bounded incremental polling. Free-text external content cannot trigger privileged commands.
- ITSM workflow state never overrides monitored technical truth. External CIs/relationships bind/enrich the canonical entity/graph under explicit authority/conflict rules; no second CMDB/entity database and no last-write-wins.
- ServiceNow CI publication/reconciliation uses certified IRE behavior by default. Exact vendor edition/API/auth/capability remains a certification gate.
- Browser journeys are Phase 1.5 and do not preselect Playwright or another runtime until OSS/security/capacity admission. Full RUM/session replay remain excluded.
- Full developer-centric APM and full always-on DPI/NDR remain excluded. Existing bounded incident-triggered packet evidence remains governed under Platform_19.

The v3.3.0 requirements are architecture/product authority only. Runtime implementation, benchmark, certification and deployment remain owned by their future BUILDs and evidence gates.

---

## Phase 2 Enterprise RFP Readiness Reconciliation — Issue #269

**Founder-approved product-roadmap authority (2026-10-29).** Before implementing or reviewing any Phase 1.5, Phase 2, enterprise technology-coverage, regulated-enterprise, external-monitoring-tool integration, APM-adjacent or packet-intelligence work, engineers/agents MUST read Issue #269 and the governing Phase contract in `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md`.

Non-negotiable execution rules:
- Phase 1 remains the frozen production Network & Infrastructure Observability core; do not pull Phase-2 breadth into Phase 1 merely because a customer RFP asks for it.
- Phase 1.5 is the enterprise foundation: exactly-two-Core HA/MSP certification, Integration Fabric/CMDB reconciliation, bounded operational OTel span/dependency evidence, NCM Change Assurance, selected broader packs and serverless.
- Phase 2 is the Enterprise ITOM/AIOps RFP-readiness envelope. Its exit gate is `ECRP-P2-v1`, not feature presence.
- Full native APM (bytecode/auto-instrumentation, method/line-level tracing, continuous profiling, full RUM/session replay) and full native always-on DPI/deep packet analytics are prohibited from Phase 2 and require Phase 2.5+ change control.
- Phase 2 MAY integrate with external APM/NPM/DPI/SIEM/CMDB/ITSM tools and ingest their evidence through governed Integration Packs; external-tool evidence never bypasses tenant identity, provenance, confidence, RBAC, audit, rate/cardinality controls or Platform RCA safety.
- Capability/vendor/product/version claims remain dimensional certification claims. A named technology in a Mother Reference is never proof of implementation/certification.
- Do not create customer-specific Core forks. Use Capability Packs, Integration Packs, policies and canonical contracts.

---

## Issue #272 — enterprise partial-closure and Phase 2.5 Deep Observability governance

Issue #272 is the follow-up authority to Issue #269. It does not change Phase-1 first-GA scope and does not create implementation/certification claims.

Engineering agents MUST apply these rules:
1. No accepted enterprise requirement may remain an unowned generic "later/partial/TBD" at roadmap level. It must identify phase, native/integrated/customer-profile route, owning capability/pack, certification method and explicit non-goal/fallback.
2. Phase 2 remains Enterprise ITOM/AIOps RFP Readiness. Full native APM/RUM/profiling and full native DPI/continuous packet analytics remain outside Phase 2.
3. Phase 2.5 is **Native Deep Observability** and owns full native APM plus full native DPI/Network Performance Intelligence.
4. Phase 2.5 planning MUST NOT select new OSS/dependencies, create a new mandatory service/database, repurpose existing BUILD IDs, create customer-specific Core forks, or weaken security/privacy boundaries without separate change control.
5. Phase-2 exit requires every accepted enterprise requirement class to resolve to one of: **Native — Certified**, **Integrated — Certified**, **Customer Profile — Certifiable**, or **Explicit Phase-2.5 deferment**.
6. SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED remains mandatory.


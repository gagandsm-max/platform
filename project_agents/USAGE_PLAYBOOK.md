# Platform Agent Routing Playbook

## Decision

Do **not** run all agents. Resolve the authoritative inventory from `project_agents/manifest.json`, resolve the current Mother Reference set dynamically from `project_documentation/Platform_00_Document_Index.md`, then select the smallest agent/skill set that materially improves the task.

The normal governed pattern is:

`INTAKE → REBASELINE → REQUIREMENTS → DIAGNOSIS → ROUTING → IMPLEMENTATION → LOCAL_VALIDATION → SOURCE_REVIEW → ADVERSARIAL_RUNTIME → SECURITY_DOMAIN_REVIEW → CI_VALIDATION → REALITY_GATE → MERGE_READY → POST_MERGE_VERIFY → COMPLETE`

`REJECTED` is a terminal exception. `BLOCKED` is a resumable exception, not a success terminal. `MERGE_READY` is not terminal: `COMPLETE` requires `POST_MERGE_VERIFY`. Risk gates may be `REQUIRED` or `NOT_APPLICABLE_WITH_EVIDENCE`; they are never silently skipped. Writer self-certification is prohibited, and authoritative-main/exact-head drift fails closed.

Use 2–5 agents for most tasks. Add specialists and canonical skills only when the subsystem/risk requires them. From ROUTING onward, `selectedSkills` must be a non-empty set of canonical Skill names.

## Default pipelines

Implementation-stage rows include explicit `selectedAgents` and `selectedSkills`. Risk-based routing still applies: omit specialists that the task does not need.

| Task | selectedAgents | selectedSkills |
|---|---|---|
| Small bug | `platform-codebase-onboarding-engineer`, `platform-minimal-change-engineer`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `root_cause_first`, `bounded_implementation`, `evidence_bundle` |
| Cross-cutting/unclear bug | `platform-codebase-onboarding-engineer`, `platform-codebase-archaeologist`, `platform-minimal-change-engineer`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `root_cause_first`, `bounded_implementation`, `post_merge_rebaseline`, `evidence_bundle` |
| Core API/event | `platform-codebase-onboarding-engineer`, `platform-api-platform-engineer`, `platform-minimal-change-engineer`, `platform-test-automation-engineer`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `bounded_implementation`, `api_event_contracts`, `evidence_bundle` |
| Sensor/protocol | `platform-codebase-onboarding-engineer`, `platform-network-engineer`, `platform-sensor-runtime-engineer`, `platform-minimal-change-engineer`, `platform-test-automation-engineer`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `bounded_implementation`, `snmp_mib`, `sensor_edge`, `runtime_failure_injection`, `evidence_bundle` |
| Agent reliability / failover / durability | `platform-codebase-onboarding-engineer`, `platform-api-platform-engineer`, `platform-sensor-runtime-engineer`, `platform-sre`, `platform-test-automation-engineer`, `platform-performance-benchmarker`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `mother_reference_reconciliation`, `bounded_implementation`, `sensor_edge`, `replay_idempotency`, `runtime_failure_injection`, `install_upgrade`, `evidence_bundle` |
| Sensor communication integrity | `platform-codebase-onboarding-engineer`, `platform-sensor-runtime-engineer`, `platform-network-engineer`, `platform-api-platform-engineer`, `platform-telemetry-pipeline-engineer`, `platform-database-reliability-engineer`, `platform-test-automation-engineer`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `mother_reference_reconciliation`, `sensor_edge`, `replay_idempotency`, `distributed_systems`, `runtime_failure_injection`, `evidence_bundle` |
| Device Pack | `platform-codebase-onboarding-engineer`, `platform-device-pack-engineer`, `platform-test-automation-engineer`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `bounded_implementation`, `device_pack_authoring`, `evidence_bundle` |
| Discovery / onboarding / Notification Center | `platform-identity-access-engineer`, `platform-workflow-architect`, `platform-network-engineer`, `platform-ux-architect`, `platform-product-ui-designer`, `platform-frontend-developer`, `platform-test-automation-engineer` | `source_truth_rebaseline`, `mother_reference_reconciliation`, `tenant_isolation`, `ux_operational_design`, `evidence_bundle` |
| Capability Pack / enterprise monitoring coverage | `platform-device-pack-engineer`, `platform-infrastructure-cloud-engineer`, `platform-telemetry-pipeline-engineer`, `platform-integration-connector-engineer`, `platform-ncm-remediation-engineer`, `platform-network-engineer`, `platform-test-automation-engineer` | `source_truth_rebaseline`, `mother_reference_reconciliation`, `device_pack_authoring`, `connector_framework`, `evidence_bundle` |
| Telemetry pipeline | `platform-codebase-onboarding-engineer`, `platform-telemetry-pipeline-engineer`, `platform-database-reliability-engineer`, `platform-test-automation-engineer`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `bounded_implementation`, `kafka`, `replay_idempotency`, `evidence_bundle` |
| NCM / remediation | `platform-codebase-onboarding-engineer`, `platform-workflow-architect`, `platform-ncm-remediation-engineer`, `platform-security-architect`, `platform-test-automation-engineer`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `bounded_implementation`, `remediation_safety`, `evidence_bundle` |
| Connector / ITSM / change feed | `platform-codebase-onboarding-engineer`, `platform-integration-connector-engineer`, `platform-api-platform-engineer`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `bounded_implementation`, `connector_framework`, `itsm_change_telemetry`, `evidence_bundle` |
| Infrastructure / cloud target | `platform-codebase-onboarding-engineer`, `platform-infrastructure-cloud-engineer`, `platform-test-automation-engineer`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `bounded_implementation`, `kubernetes_monitoring`, `cloud_monitoring`, `evidence_bundle` |
| Database/migration | `platform-codebase-onboarding-engineer`, `platform-database-reliability-engineer`, `platform-test-automation-engineer`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `bounded_implementation`, `migration_safety`, `evidence_bundle` |
| IAM/security | `platform-codebase-onboarding-engineer`, `platform-identity-access-engineer`, `platform-security-architect`, `platform-appsec-engineer`, `platform-test-automation-engineer`, `platform-evidence-collector` | `source_truth_rebaseline`, `bounded_implementation`, `tenant_isolation`, `security_negative_testing`, `evidence_bundle` |
| Material UI feature | `platform-ux-architect`, `platform-product-ui-designer`, `platform-frontend-developer`, `platform-test-automation-engineer`, `platform-accessibility-auditor`, `platform-ui-finish-gate-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `mother_reference_reconciliation`, `bounded_implementation`, `ux_operational_design`, `web_interface_quality`, `accessibility_inclusive_design`, `typescript_react`, `evidence_bundle` |
| Tiny UI copy/spacing fix | `platform-frontend-developer`, `platform-code-reviewer` | `bounded_implementation`, `typescript_react`, `web_interface_quality`, `evidence_bundle` |
| RCA/graph/AI | `platform-workflow-architect`, `platform-knowledge-graph-engineer`, `platform-ai-engineer`, `platform-model-qa`, `platform-security-architect`, `platform-evidence-collector` | `source_truth_rebaseline`, `ai_evidence_safety`, `alert_quality_evidence`, `evidence_bundle` |
| CI/package/upgrade | `platform-devops-automator`, `platform-sre`, `platform-test-automation-engineer`, `platform-evidence-collector`, `platform-reality-checker` | `source_truth_rebaseline`, `ci_cd`, `install_upgrade`, `sbom_provenance`, `evidence_bundle` |
| Parallel PR integration | `platform-git-workflow-master`, `platform-codebase-archaeologist`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `post_merge_rebaseline`, `evidence_bundle` |
| Release/merge gate | `platform-reality-checker`, `platform-code-reviewer`, `platform-evidence-collector` | `source_truth_rebaseline`, `exact_head_merge_gate`, `evidence_bundle` |

Material UI pipeline: requirements/source review → `platform-ux-architect` → `platform-product-ui-designer` → implementation-ready UI specification → `platform-frontend-developer` → runtime/UI tests → `platform-accessibility-auditor` → large-estate/performance review where applicable → `platform-ui-finish-gate-reviewer` → `platform-evidence-collector`. Add `platform-performance-benchmarker` when large-estate/query cost is in play. Neither designer nor frontend may self-certify accessibility or final finish.

## Skill authority

Canonical skills live in `project_agents/skills/`. `.agents/skills/` is the shared Cursor/Antigravity generated projection. Root `skills/` remains the legacy generated mirror in this tranche. Both generated trees must match canonical content exactly. Resolve the current skill inventory from `project_agents/skills/manifest.json`; do not freeze a skill count in policy. Per-agent required/conditional Skills: `project_agents/skills/AGENT_SKILL_MATRIX.md`.

Skills complement agents; they do not create new authority or allow a writer to certify its own output.

## Rules for parallelism

Parallelize **read-heavy independent verification**, not multiple agents writing the same checkout. Good parallel work: code review + security review + test-gap analysis. Risky parallel work: two agents editing the same files or schema.

For parallel write work, use separate Git branches/worktrees and let `platform-git-workflow-master` coordinate integration.

## Evidence and security rules

- Claims must be typed and backed by the appropriate source, execution, CI, runtime or security evidence.
- Deep leakage review inspects direct logs, structured attributes, exception text, nested causes, framework handlers, retry/recovery wrappers, support bundles and traces/telemetry—not only obvious logger calls.
- Security-sensitive acknowledgement/offset behavior must be tested fail-closed where applicable.
- Do not weaken tests, auth, tenant isolation, durability or branch/security controls to obtain green CI.
- Do not use destructive generic Docker prune or `down -v` operations as a routine cleanup or validation shortcut.

Material product/capability changes (new monitoring family, pack class, integration coverage, or certification claim) must answer:

- **A.** Does the implementation comply with the current Mother Reference set discovered from `Platform_00_Document_Index.md` §3, including task-relevant specialized masters?
- **B.** Does it measurably improve Real Customer Enterprise Coverage as normalized in `Platform_23` (not a lab-only slice presented as coverage)?
- **C.** Which Capability Pack / Integration coverage state changed (architecture, specification, implementation, pack, certification, or integration)? Specified is not implemented.
- **D.** What certification evidence exists for the claimed dimensions? Do not treat listing a technology family as certification.

Trivial edits, comments, and isolated bug fixes do not require this checklist.

## Routing chain

`task intent → owning agent (AGENT_MATRIX) → required Skills (AGENT_SKILL_MATRIX) → risk-triggered reviewers → writerTool → verifierTool → evidence gate`

ChatGPT+GitHub should route from these documents without loading every Skill body. Cursor `--agent` is unsupported. Antigravity default posture is read-only/adversarial; write requires explicit authority. See `TOOL_COMPATIBILITY.md`.

## Do not use an agent when

- the main agent can answer a small, local question in one pass;
- the agent adds no independent evidence or specialist knowledge;
- invoking it would duplicate another agent's role;
- the task is a Founder-reserved business/legal/commercial decision.

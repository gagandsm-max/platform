# Start Here — Using Platform Agents

You do not need to memorize the current agent inventory or invoke agents manually every time. The authoritative inventory is `project_agents/manifest.json`; routing should remain manifest-derived so future additions or removals do not require policy rewrites.

## The operating model

1. **ChatGPT remains CTO/orchestrator.** Use it for architecture, task decomposition, PR review and deciding which specialist is needed.
2. **Root `AGENTS.md` remains law.** Agent files never override the Platform Mother Reference.
3. **Resolve Mother Reference dynamically.** Parse `project_documentation/Platform_00_Document_Index.md` for the current authoritative set instead of relying on a packaged document ceiling or remembered file count.
4. **Use canonical skills.** Capability/risk procedures live under `project_agents/skills/`. Generated `.agents/skills/` is the shared Cursor/Antigravity projection. Root `skills/` remains a legacy generated mirror in this tranche. Neither generated tree is a second authority.
5. **Tool roles are fixed by governance.** Cursor CLI is the default bounded writer using `.cursor/agents/` and `.agents/skills/`. Cursor Cloud and GitHub Copilot are never-writers under current policy. Antigravity uses `.agents/agents/` and `.agents/skills/` and is read-only/adversarial by default; Codex uses `.codex/agents/` as a read-only exact-SHA auditor/certification projection by default. ChatGPT/Founder may assign Antigravity or Codex an exceptional isolated write via task-manifest `writerElevation` without changing generated adapter defaults. See `TOOL_COMPATIBILITY.md` and `project_agents/tool_authority.json`. Cursor `--agent` is unsupported.
6. **Use the smallest team.** Most tasks need one explorer, one implementation/domain agent, and one independent verifier.
7. **Separate writing from verification.** The agent that writes the patch must not be the only agent that declares it correct.
8. **Rebaseline before action.** Record authoritative main, exact input head, dependencies and required evidence before implementation or review; main/head drift blocks stale certification.

## The one prompt to remember

Use this in ChatGPT, Cursor, Codex or Antigravity:

```text
Route this task using the Platform agent pack. Read root AGENTS.md, parse project_documentation/Platform_00_Document_Index.md for the current Mother Reference set, and select applicable canonical skills from project_agents/skills/. Use the smallest useful set of agents. Separate diagnosis, implementation and independent verification. Do not allow parallel write agents to edit the same checkout.

Task: <what I want>
Constraints: <branch/PR/scope constraints, if any>
```

## Common examples

Implementation-stage examples record explicit `selectedAgents` and `selectedSkills`. From ROUTING onward an active engineering task cannot proceed with an empty `selectedSkills` array. A governance-only task still selects canonical governance Skills such as `source_truth_rebaseline` / `bounded_implementation`.

### Fix a bug

```text
Use platform-codebase-onboarding-engineer to verify the root cause, platform-minimal-change-engineer to make the smallest fix, then platform-code-reviewer and platform-evidence-collector to verify it independently.
selectedAgents: [platform-codebase-onboarding-engineer, platform-minimal-change-engineer, platform-code-reviewer, platform-evidence-collector]
selectedSkills: [source_truth_rebaseline, root_cause_first, bounded_implementation, evidence_bundle]
```

### Build a network/Sensor feature

```text
Use platform-codebase-onboarding-engineer. Route protocol/domain work to platform-network-engineer, Sensor runtime (pools/spool/uploader/leases) to platform-sensor-runtime-engineer, and Device Pack schema to platform-device-pack-engineer. Use platform-minimal-change-engineer for the patch and platform-test-automation-engineer for tests. Finish with code-reviewer and evidence-collector.
selectedAgents: [platform-codebase-onboarding-engineer, platform-network-engineer, platform-sensor-runtime-engineer, platform-device-pack-engineer, platform-minimal-change-engineer, platform-test-automation-engineer, platform-code-reviewer, platform-evidence-collector]
selectedSkills: [source_truth_rebaseline, mother_reference_reconciliation, bounded_implementation, snmp_mib, sensor_edge, device_pack_authoring, evidence_bundle]
```

### Privileged NCM / connector / telemetry pipeline

```text
NCM/remediation: platform-ncm-remediation-engineer with remediation_safety (no LLM privileged execution). Connectors: platform-integration-connector-engineer. Post-accept streaming: platform-telemetry-pipeline-engineer. Infrastructure targets: platform-infrastructure-cloud-engineer. Always separate writerTool from verifierTool and close with evidence_bundle / exact_head_merge_gate.
selectedAgents: [platform-ncm-remediation-engineer, platform-integration-connector-engineer, platform-telemetry-pipeline-engineer, platform-infrastructure-cloud-engineer, platform-code-reviewer, platform-evidence-collector]
selectedSkills: [source_truth_rebaseline, bounded_implementation, remediation_safety, connector_framework, kafka, kubernetes_monitoring, evidence_bundle]
```

### Build a UI feature

Material UI uses the ownership chain. Tiny copy/spacing fixes need not invoke the entire chain.

```text
Material operational screen:
selectedAgents: [platform-ux-architect, platform-product-ui-designer, platform-frontend-developer, platform-test-automation-engineer, platform-accessibility-auditor, platform-ui-finish-gate-reviewer, platform-evidence-collector]
selectedSkills: [source_truth_rebaseline, mother_reference_reconciliation, bounded_implementation, ux_operational_design, web_interface_quality, accessibility_inclusive_design, typescript_react, evidence_bundle]

Tiny UI copy/spacing fix:
selectedAgents: [platform-frontend-developer, platform-code-reviewer]
selectedSkills: [bounded_implementation, typescript_react, web_interface_quality, evidence_bundle]
```

### Build AI/RCA capability

```text
Use platform-knowledge-graph-engineer and platform-ai-engineer, then platform-model-qa. Add platform-security-architect/appsec for tool access, tenant data, prompt/context or automated-action risk. Reality-check before enablement claims.
selectedAgents: [platform-knowledge-graph-engineer, platform-ai-engineer, platform-model-qa, platform-security-architect, platform-reality-checker]
selectedSkills: [source_truth_rebaseline, ai_evidence_safety, alert_quality_evidence, rca_hypothesis_fusion, evidence_bundle]
```

### Governed discovery / onboarding / Notification Center

```text
Use existing specialists. Do not create a new onboarding-identity agent.
selectedAgents: [platform-identity-access-engineer, platform-workflow-architect, platform-network-engineer, platform-product-ui-designer, platform-ux-architect, platform-frontend-developer, platform-security-architect, platform-test-automation-engineer]
selectedSkills: [source_truth_rebaseline, mother_reference_reconciliation, tenant_isolation, ux_operational_design, evidence_bundle]
```

### Enterprise monitoring coverage / Capability Pack certification (specification or later pack work)

```text
Capability Pack is the single extensibility architecture. Device Pack remains a classification.
selectedAgents: [platform-device-pack-engineer, platform-infrastructure-cloud-engineer, platform-telemetry-pipeline-engineer, platform-integration-connector-engineer, platform-ncm-remediation-engineer, platform-network-engineer, platform-test-automation-engineer]
selectedSkills: [source_truth_rebaseline, mother_reference_reconciliation, device_pack_authoring, connector_framework, evidence_bundle]
```

### Review a PR

```text
Use platform-code-reviewer plus the domain specialist. Add platform-appsec-engineer if auth/tenant/secrets/untrusted input is touched. Run platform-evidence-collector. Apply exact_head_merge_gate before any merge-ready claim. Use platform-reality-checker only for a completion/release-readiness decision.
selectedAgents: [platform-code-reviewer, platform-evidence-collector, platform-reality-checker]
selectedSkills: [source_truth_rebaseline, exact_head_merge_gate, evidence_bundle]
```

## What P0 and P1 mean

- **P0 agents** protect engineering truth: scope, code understanding, review, security, testing, evidence and performance. They are not all invoked on every task.
- **P1 agents** are domain specialists for API, network, Sensor runtime, Device Packs, telemetry pipelines, NCM/remediation, connectors, infrastructure targets, database, SRE, IAM, graph/AI, DevOps/Git/workflows, UX architecture, product UI specification and UI implementation.

If you are unsure which agent to use, do not guess. Route from `project_agents/manifest.json`, `project_agents/AGENT_MATRIX.md`, `project_agents/USAGE_PLAYBOOK.md`, `project_agents/skills/AGENT_SKILL_MATRIX.md`, and the applicable canonical skills.

The Platform product version remains v3.2.2 unless the Founder explicitly changes it. The current Mother Reference file set and precedence are always discovered from `project_documentation/Platform_00_Document_Index.md`; no agent or guide may freeze a historical document count as permanent authority.

# Start Here — Using Platform Agents

You do not need to memorize the current agent inventory or invoke agents manually every time. The authoritative inventory is `project_agents/manifest.json`; routing should remain manifest-derived so future additions or removals do not require policy rewrites.

## The operating model

1. **ChatGPT remains CTO/orchestrator.** Use it for architecture, task decomposition, PR review and deciding which specialist is needed.
2. **Root `AGENTS.md` remains law.** Agent files never override the Platform Mother Reference.
3. **Resolve Mother Reference dynamically.** Parse `project_documentation/Platform_00_Document_Index.md` for the current authoritative set instead of relying on a packaged document ceiling or remembered file count.
4. **Use canonical skills.** Capability/risk procedures live under `project_agents/skills/`. Generated `.agents/skills/` is the shared Cursor/Antigravity projection. Root `skills/` remains a legacy generated mirror in this tranche. Neither generated tree is a second authority.
5. **Coding tools execute.** Cursor Cloud/CLI use `.cursor/agents/` and `.agents/skills/`. Antigravity uses `.agents/agents/` and `.agents/skills/`. Codex compatibility stays in `.codex/agents/` and is not a v1.2 certification gate. Cursor `--agent` is unsupported. See `TOOL_COMPATIBILITY.md`.
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

## Five common examples

### Fix a bug

```text
Use platform-codebase-onboarding-engineer to verify the root cause, platform-minimal-change-engineer to make the smallest fix, then platform-code-reviewer and platform-evidence-collector to verify it independently. Apply root_cause_first, bounded_implementation and evidence_bundle as applicable.
```

### Build a network/Sensor feature

```text
Use platform-codebase-onboarding-engineer. Route protocol/domain work to platform-network-engineer, Sensor runtime (pools/spool/uploader/leases) to platform-sensor-runtime-engineer, and Device Pack schema to platform-device-pack-engineer. Use platform-minimal-change-engineer for the patch and platform-test-automation-engineer for tests. Finish with code-reviewer and evidence-collector.
```

### Privileged NCM / connector / telemetry pipeline

```text
NCM/remediation: platform-ncm-remediation-engineer with remediation_safety (no LLM privileged execution). Connectors: platform-integration-connector-engineer. Post-accept streaming: platform-telemetry-pipeline-engineer. Infrastructure targets: platform-infrastructure-cloud-engineer. Always separate writerTool from verifierTool and close with evidence_bundle / exact_head_merge_gate.
```

### Build a UI feature

```text
Use platform-ux-architect to define the screen contract, platform-frontend-developer to implement it, then platform-accessibility-auditor and platform-ui-finish-gate-reviewer. Collect browser/test evidence before calling it complete.
```

### Build AI/RCA capability

```text
Use platform-knowledge-graph-engineer and platform-ai-engineer, then platform-model-qa. Add platform-security-architect/appsec for tool access, tenant data, prompt/context or automated-action risk. Apply ai_evidence_safety and alert_quality_evidence where relevant. Reality-check before enablement claims.
```

### Review a PR

```text
Use platform-code-reviewer plus the domain specialist. Add platform-appsec-engineer if auth/tenant/secrets/untrusted input is touched. Run platform-evidence-collector. Apply exact_head_merge_gate before any merge-ready claim. Use platform-reality-checker only for a completion/release-readiness decision.
```

## What P0 and P1 mean

- **P0 agents** protect engineering truth: scope, code understanding, review, security, testing, evidence and performance. They are not all invoked on every task.
- **P1 agents** are domain specialists for API, network, Sensor runtime, Device Packs, telemetry pipelines, NCM/remediation, connectors, infrastructure targets, database, SRE, IAM, graph/AI, DevOps/Git/workflows and UI.

If you are unsure which agent to use, do not guess. Route from `project_agents/manifest.json`, `project_agents/AGENT_MATRIX.md`, `project_agents/USAGE_PLAYBOOK.md`, `project_agents/skills/AGENT_SKILL_MATRIX.md`, and the applicable canonical skills.

The Platform product version remains v3.2.2 unless the Founder explicitly changes it. The current Mother Reference file set and precedence are always discovered from `project_documentation/Platform_00_Document_Index.md`; no agent or guide may freeze a historical document count as permanent authority.

# ChatGPT + GitHub Platform Agent Router

## Operating model

ChatGPT remains the principal Platform architecture/technical authority and repository control plane. Specialist agents and canonical skills are bounded execution/verification lenses, not competing authorities.

Before routing any non-trivial task:

1. Re-fetch authoritative `main` and the exact input head/PR state.
2. Read root `AGENTS.md`.
3. Parse `project_documentation/Platform_00_Document_Index.md` to resolve the current Mother Reference set and precedence dynamically; do not rely on a remembered file count or historical ceiling.
4. Read `project_agents/manifest.json`, then choose only exact canonical agent names from `project_agents/AGENT_MATRIX.md`.
5. Select applicable capability/risk procedures from canonical `project_agents/skills/`; root `skills/` are generated discovery mirrors, not a second source of truth.
6. Record security boundaries, migrations, dependencies, required tests/evidence, `writerTool`, `verifierTool` and gatekeeper in the task manifest when the task warrants a governed workflow. Do not use `writer`/`verifier` aliases.
7. Writer self-certification is prohibited; main/head drift invalidates stale review.

### Generic invocation

```text
Using the connected Platform repository, first rebaseline authoritative main and exact input head, read root AGENTS.md, parse project_documentation/Platform_00_Document_Index.md for the current Mother Reference set, then read project_agents/canonical/<tier>/<agent>.md and the applicable project_agents/skills/<skill>/SKILL.md files. Operate under that bounded specialist role for this task. Do not broaden scope or override Platform decisions.

Task: <task>
Branch/PR constraints: <constraints>
Required evidence: <evidence>
```

### Ask ChatGPT to route for you

```text
Route this Platform engineering task using the smallest useful set of exact canonical agents from project_agents/manifest.json, AGENT_MATRIX.md and USAGE_PLAYBOOK.md, plus applicable canonical skills. Keep yourself as CTO/orchestrator. Rebaseline source truth first. Separate read-only diagnosis, bounded implementation and independent verification. Do not let parallel write agents edit the same checkout.

Task: <task>
```

### PR review

```text
Use platform-codebase-onboarding-engineer to map the affected path, platform-code-reviewer to review correctness, platform-appsec-engineer if security/tenant/auth is touched, and platform-evidence-collector to verify tests. Apply exact_head_merge_gate; add security_negative_testing or runtime_failure_injection when the risk requires them. Summarize only material blockers and evidence.
```

### Feature implementation

```text
First map the actual implementation with platform-codebase-onboarding-engineer. Select the one domain specialist appropriate to the subsystem. Use platform-minimal-change-engineer for the patch. Apply bounded_implementation and migration_safety where relevant. Then run platform-code-reviewer and platform-evidence-collector independently. Use platform-reality-checker only when claiming the feature is complete/release-ready.
```

### Tool/runtime boundaries

- ChatGPT+GitHub coordinates repository truth, GitHub writes, PR state and CI evidence; it does not claim local Cursor/Antigravity execution and has no native filesystem subagent loader.
- Cursor Cloud uses `.cursor/agents/` and shared `.agents/skills/`. Cursor CLI uses the same paths plus the proven flags in `TOOL_COMPATIBILITY.md`. Do not invent `--agent`.
- Antigravity uses `.agents/agents/` and `.agents/skills/`. `--agent` is supported. Default Platform posture is read-only/adversarial; write mode requires explicit authority.
- Codex compatibility projection remains `.codex/agents/` and is not a v1.2 certification gate. Do not invent a Codex skill path.
- Never use destructive generic Docker cleanup such as broad prune or `down -v` as an orchestration shortcut.

## Routing chain

`task intent → owning agent → required Skills → risk-triggered reviewers → writerTool → verifierTool → evidence gate`

Do not load every Skill body. Use this table, `AGENT_MATRIX.md` and `skills/AGENT_SKILL_MATRIX.md`.

| Scenario | Owning agent | Required Skills (minimum) | Risk-triggered reviewers | writerTool | verifierTool | Evidence gate |
|---|---|---|---|---|---|---|
| Surgical bug fix after RCA | `platform-minimal-change-engineer` | `root_cause_first`, `bounded_implementation`, `evidence_bundle` | `platform-code-reviewer` | `cursor-cloud` | `chatgpt-github` | `exact_head_merge_gate` |
| Sensor spool/uploader/leases | `platform-sensor-runtime-engineer` | `sensor_edge`, `go_runtime`, `replay_idempotency` | `platform-code-reviewer`, `platform-sre` | `cursor-cloud` | `chatgpt-github` | `evidence_bundle` |
| SNMP/ICMP/flow/syslog semantics | `platform-network-engineer` | `snmp_mib`, `icmp_path`, `flow_telemetry` | `platform-code-reviewer` | `cursor-cloud` | `chatgpt-github` | `alert_quality_evidence` |
| Device Pack schema | `platform-device-pack-engineer` | `device_pack_authoring` | `platform-network-engineer` (protocol mapping), `platform-code-reviewer` | `cursor-cloud` | `chatgpt-github` | `evidence_bundle` |
| Kafka lag after accept | `platform-telemetry-pipeline-engineer` | `kafka`, `replay_idempotency`, `distributed_systems` | `platform-database-reliability-engineer`, `platform-code-reviewer` | `cursor-cloud` | `chatgpt-github` | `runtime_failure_injection` |
| Privileged NCM change | `platform-ncm-remediation-engineer` | `remediation_safety`, `device_control_protocols` | `platform-security-architect`, `platform-appsec-engineer` | `cursor-cloud` | `chatgpt-github` | `security_negative_testing` |
| ITSM/connector | `platform-integration-connector-engineer` | `connector_framework`, `itsm_change_telemetry`, `oauth_oidc_clients` | `platform-api-platform-engineer`, `platform-appsec-engineer` | `cursor-cloud` | `chatgpt-github` | `evidence_bundle` |
| Linux/K8s/cloud **target** monitoring | `platform-infrastructure-cloud-engineer` | `linux_systems`, `kubernetes_monitoring`, `cloud_monitoring` | `platform-security-architect` | `cursor-cloud` | `chatgpt-github` | `evidence_bundle` |
| Merge-ready claim | `platform-reality-checker` | `exact_head_merge_gate`, `evidence_bundle` | independent verifier only; writer forbidden | n/a (read) | `chatgpt-github` | exact-head current-base CI |

Writer self-certification is prohibited. Antigravity verifier-first means default read-only/adversarial review before any write grant.

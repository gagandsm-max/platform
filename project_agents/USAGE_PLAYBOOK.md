# Platform Agent Routing Playbook

## Decision

Do **not** run all agents. The normal pattern is:

`Understand → Implement → Verify`

Use 2–5 agents for most tasks. Add specialists only when the subsystem/risk requires them.

## Default pipelines

| Task | Recommended agent sequence |
|---|---|
| Small bug | `platform-codebase-onboarding-engineer` → `platform-minimal-change-engineer` → `platform-code-reviewer` → `platform-evidence-collector` |
| Cross-cutting/unclear bug | `platform-codebase-onboarding-engineer` + `platform-codebase-archaeologist` → domain specialist → `platform-minimal-change-engineer` → reviewer/evidence |
| Core API/event | onboarding → `platform-api-platform-engineer` + minimal-change → test-automation → code-reviewer → evidence → reality-checker when completion is claimed |
| Sensor/protocol | onboarding → `platform-network-engineer` + minimal-change → test-automation → performance-benchmarker for hot path → reviewer/evidence |
| Database/migration | onboarding → `platform-database-reliability-engineer` → appsec if tenant/security-sensitive → test-automation → reviewer/evidence/reality-checker |
| IAM/security | onboarding → `platform-identity-access-engineer` + `platform-security-architect` → appsec/secrets → test-automation → AI-code-security-auditor if AI-generated → evidence/reality-checker |
| UI feature | `platform-ux-architect` → frontend-developer → test-automation → accessibility-auditor → ui-finish-gate-reviewer → evidence |
| RCA/graph | workflow-architect if flow unclear → knowledge-graph-engineer → AI-engineer if learned/LLM capability → model-qa → security-architect/appsec → evidence/reality-checker |
| CI/package/upgrade | devops-automator + SRE → security/secrets as needed → test-automation → evidence → reality-checker |
| Parallel PR integration | git-workflow-master → codebase-archaeologist → code-reviewer → targeted tests/evidence |
| Release gate | reality-checker coordinates evidence from code-reviewer, appsec, secrets, performance, accessibility/UI, DB/migration and model-QA only where applicable. |

## Evidence routing

Apply the quality gates that the changed behavior can actually affect; do not run every gate mechanically and do not omit an applicable gate merely because a specialist is absent.

- **Unit/static** — changed deterministic logic, parsers, validation, generation, configuration or contracts.
- **Integration/API/contract** — service boundaries, APIs/events, protocol/control contracts, persistence interactions or generated interface compatibility.
- **Security/tenant isolation** — authentication, authorization, secrets, untrusted input, tenant/resource scope, privileged actions or trust-boundary changes.
- **Migration/failure/recovery** — schemas, durable state, upgrade paths, retries, queues, leases/fencing, HA/DR or operational recovery behavior.
- **Performance** — hot paths, polling/ingestion, storage/query cardinality, high-rate streams, dashboard/query scale or resource-sensitive changes.
- **UI/browser/accessibility** — user-visible workflows, browser state, role-aware UX, keyboard/focus/contrast/responsiveness or large-table behavior.
- **Packaging/release** — installers, images, CI/CD, signing/SBOM, package layout, upgrade/rollback or release-promotion changes.

The evidence collector records what actually ran. The reality checker may reject a completion/production/GA claim when an applicable gate is missing, even if narrower tests are green.

## Tool runtime boundaries

- **Cursor:** native `@agent` picker discovers `.cursor/agents/` and enforces `readonly: true` frontmatter.
- **Antigravity:** native `/agents` workspace discovery uses `.agents/agents/` and enforces `commandExecutionPolicy: off` (read) / `sandbox` (write).
- **Codex:** native named-role discovery from `.codex/agents/` applies role identity and developer instructions, but child role TOML files do not enforce sandbox boundaries (subagents inherit parent session permissions in Codex 0.153.4). Governed Platform Codex execution must use the session-mode launcher:
  ```bash
  python3 project_agents/tools/run_codex_agent.py <agent-name> [prompt]
  ```

## Rules for parallelism

Parallelize **read-heavy independent verification**, not multiple agents writing the same checkout. Good parallel work: code review + security review + test-gap analysis. Risky parallel work: two agents editing the same files or schema.

For parallel write work, use separate Git branches/worktrees and let `platform-git-workflow-master` coordinate integration.

## Do not use an agent when

- the main agent can answer a small, local question in one pass;
- the agent adds no independent evidence or specialist knowledge;
- invoking it would duplicate another agent's role;
- the task is a Founder-reserved business/legal/commercial decision.

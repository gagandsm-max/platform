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

## Rules for parallelism

Parallelize **read-heavy independent verification**, not multiple agents writing the same checkout. Good parallel work: code review + security review + test-gap analysis. Risky parallel work: two agents editing the same files or schema.

For parallel write work, use separate Git branches/worktrees and let `platform-git-workflow-master` coordinate integration.

## Do not use an agent when

- the main agent can answer a small, local question in one pass;
- the agent adds no independent evidence or specialist knowledge;
- invoking it would duplicate another agent's role;
- the task is a Founder-reserved business/legal/commercial decision.

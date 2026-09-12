# Platform Agent Routing Playbook

## Decision

Do **not** run all agents. Resolve the authoritative inventory from `project_agents/manifest.json`, resolve the current Mother Reference set dynamically from `project_documentation/Platform_00_Document_Index.md`, then select the smallest agent/skill set that materially improves the task.

The normal governed pattern is:

`INTAKE → REBASELINE → REQUIREMENTS → DIAGNOSIS → ROUTING → IMPLEMENTATION → LOCAL_VALIDATION → SOURCE_REVIEW → ADVERSARIAL_RUNTIME → SECURITY_DOMAIN_REVIEW → CI_VALIDATION → REALITY_GATE → MERGE_READY → POST_MERGE_VERIFY → COMPLETE`

`REJECTED` is a terminal exception. `BLOCKED` is a resumable exception, not a success terminal. `MERGE_READY` is not terminal: `COMPLETE` requires `POST_MERGE_VERIFY`. Risk gates may be `REQUIRED` or `NOT_APPLICABLE_WITH_EVIDENCE`; they are never silently skipped. Writer self-certification is prohibited, and authoritative-main/exact-head drift fails closed.

Use 2–5 agents for most tasks. Add specialists and canonical skills only when the subsystem/risk requires them.

## Default pipelines

| Task | Recommended agent sequence / skills |
|---|---|
| Small bug | `platform-codebase-onboarding-engineer` → `platform-minimal-change-engineer` → `platform-code-reviewer` → `platform-evidence-collector`; `source_truth_rebaseline`, `root_cause_first`, `bounded_implementation`, `evidence_bundle` |
| Cross-cutting/unclear bug | `platform-codebase-onboarding-engineer` + `platform-codebase-archaeologist` → domain specialist → `platform-minimal-change-engineer` → reviewer/evidence; add `root_cause_first` and `post_merge_rebaseline` |
| Core API/event | onboarding → `platform-api-platform-engineer` + minimal-change → test-automation → code-reviewer → evidence → reality-checker when completion is claimed; add security/failure skills as risk dictates |
| Sensor/protocol | onboarding → `platform-network-engineer` + minimal-change → test-automation → performance-benchmarker for hot path → reviewer/evidence; use `runtime_failure_injection` for failover/queue/durability semantics |
| Database/migration | onboarding → `platform-database-reliability-engineer` → appsec if tenant/security-sensitive → test-automation → reviewer/evidence/reality-checker; `migration_safety` is required for schema/data migrations |
| IAM/security | onboarding → `platform-identity-access-engineer` + `platform-security-architect` → appsec/secrets → test-automation → AI-code-security-auditor if AI-generated → evidence/reality-checker; add `security_negative_testing` |
| UI feature | `platform-ux-architect` → frontend-developer → test-automation → accessibility-auditor → ui-finish-gate-reviewer → evidence |
| RCA/graph/AI | workflow-architect if flow unclear → knowledge-graph-engineer → AI-engineer if learned/LLM capability → model-qa → security-architect/appsec → evidence/reality-checker; add `alert_quality_evidence` and `ai_evidence_safety` as applicable |
| CI/package/upgrade | devops-automator + SRE → security/secrets as needed → test-automation → evidence → reality-checker |
| Parallel PR integration | git-workflow-master → codebase-archaeologist → code-reviewer → targeted tests/evidence; use `source_truth_rebaseline` and `post_merge_rebaseline` |
| Release/merge gate | reality-checker coordinates evidence from code-reviewer, appsec, secrets, performance, accessibility/UI, DB/migration and model-QA only where applicable; `exact_head_merge_gate` is mandatory before a merge-ready claim |

## Skill authority

Canonical skills live in `project_agents/skills/`. Root `skills/` are deterministic generated mirrors for discovery and must match canonical content exactly. Resolve the current skill inventory from `project_agents/skills/manifest.json`; do not freeze a skill count in policy.

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

## Do not use an agent when

- the main agent can answer a small, local question in one pass;
- the agent adds no independent evidence or specialist knowledge;
- invoking it would duplicate another agent's role;
- the task is a Founder-reserved business/legal/commercial decision.

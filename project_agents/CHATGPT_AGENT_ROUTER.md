# ChatGPT + GitHub Platform Agent Router

## Operating model

ChatGPT remains the principal Platform architecture/technical authority. Specialist agents are temporary repository-defined lenses, not persistent installed ChatGPT agents and not competing authorities.

Always route with the smallest useful set. Separate read-only diagnosis, bounded implementation and independent verification. A writer cannot be its own sole verifier, and parallel write specialists must not share the same checkout.

### Generic invocation

```text
Using the connected Platform repository, first read root AGENTS.md and the relevant mother-reference files. Then read project_agents/AGENT_MATRIX.md and project_agents/USAGE_PLAYBOOK.md. Read the selected project_agents/canonical/<tier>/<agent>.md definition before adopting that specialist role. Operate under that role only for this task. Do not broaden scope, override Platform decisions, or claim production/GA readiness without the applicable evidence/reality gate.

Task: <task>
Branch/PR constraints: <constraints>
Required evidence: <evidence>
```

### Ask ChatGPT to route for you

```text
Route this Platform engineering task using the smallest useful set of agents from project_agents/AGENT_MATRIX.md and project_agents/USAGE_PLAYBOOK.md. Keep yourself as CTO/orchestrator. Read each selected canonical agent definition before using that role. Separate read-only diagnosis, implementation, and independent verification. Do not let parallel write agents edit the same checkout.

Task: <task>
```

### GitHub-only specialist task

```text
Using connected GitHub only, read root AGENTS.md, project_agents/AGENT_MATRIX.md, project_agents/USAGE_PLAYBOOK.md and the requested canonical specialist definition before acting. Treat the specialist as subordinate to root AGENTS.md and the Mother Reference. Do not imply that 27 persistent agents are installed in ChatGPT.

Specialist: <agent name>
Task: <task>
```

### PR review

```text
Use platform-codebase-onboarding-engineer to map the affected path, platform-code-reviewer to review correctness, platform-appsec-engineer if security/tenant/auth is touched, and platform-evidence-collector to verify tests. Summarize only material blockers and evidence.
```

### Feature implementation

```text
First map the actual implementation with platform-codebase-onboarding-engineer. Select the one domain specialist appropriate to the subsystem. Use platform-minimal-change-engineer for the patch. Then run platform-code-reviewer and platform-evidence-collector independently. Use platform-reality-checker only when claiming the feature is complete/release-ready.
```

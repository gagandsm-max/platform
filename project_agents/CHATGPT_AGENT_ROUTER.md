# ChatGPT + GitHub Platform Agent Router

## Operating model

ChatGPT remains the principal Platform architecture/technical authority. Specialist agents are temporary lenses, not competing authorities.

### Generic invocation

```text
Using the connected Platform repository, first read root AGENTS.md and the relevant mother-reference files. Then read project_agents/canonical/<tier>/<agent>.md and operate under that specialist role for this task. Do not broaden scope or override Platform decisions.

Task: <task>
Branch/PR constraints: <constraints>
Required evidence: <evidence>
```

### Ask ChatGPT to route for you

```text
Route this Platform engineering task using the smallest useful set of agents from project_agents/AGENT_MATRIX.md and USAGE_PLAYBOOK.md. Keep yourself as CTO/orchestrator. Separate read-only diagnosis, implementation, and independent verification. Do not let parallel write agents edit the same checkout.

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

# Start Here — Using Platform Agents

You do not need to learn 27 agents or invoke them manually every time.

## The operating model

1. **ChatGPT remains CTO/orchestrator.** Use it for architecture, task decomposition, PR review and deciding which specialist is needed.
2. **Root `AGENTS.md` remains law.** Agent files never override the Platform mother reference.
3. **Coding tools execute.** Cursor, Codex and Antigravity discover their native agent adapters automatically after the files are in the repository.
4. **Use the smallest team.** Most tasks need one explorer, one implementation/domain agent, and one independent verifier.
5. **Separate writing from verification.** The agent that writes the patch should not be the only agent that declares it correct.

## Canonical repository wins over offline snapshots

The current GitHub repository `project_agents/` deployment is authoritative for specialist-agent content. Offline Platform Agent Pack ZIPs are distribution snapshots only; they are never higher authority than the current canonical GitHub deployment.

**Do not reinstall an older offline bundle over a newer repository deployment.** Older installers can copy canonical definitions, manifests and native adapters into an existing checkout and therefore can downgrade or overwrite newer governed content. Compare the repository version/hashes first and refresh an offline bundle separately when needed.

## The one prompt to remember

Use this in ChatGPT, Cursor, Codex or Antigravity:

```text
Route this task using the Platform agent pack. Read root AGENTS.md and the relevant mother-reference files first. Use the smallest useful set of agents. Separate diagnosis, implementation and independent verification. Do not allow parallel write agents to edit the same checkout.

Task: <what I want>
Constraints: <branch/PR/scope constraints, if any>
```

## Five common examples

### Fix a bug

```text
Use platform-codebase-onboarding-engineer to verify the root cause, platform-minimal-change-engineer to make the smallest fix, then platform-code-reviewer and platform-evidence-collector to verify it independently.
```

### Build a network/Sensor feature

```text
Use platform-codebase-onboarding-engineer, then platform-network-engineer plus platform-minimal-change-engineer. Add platform-test-automation-engineer. Use platform-performance-benchmarker only if the change affects polling/ingestion/hot paths. Finish with code-reviewer and evidence-collector.
```

### Build a UI feature

```text
Use platform-ux-architect to define the screen contract, platform-frontend-developer to implement it, then platform-accessibility-auditor and platform-ui-finish-gate-reviewer. Collect browser/test evidence before calling it complete.
```

### Build AI/RCA capability

```text
Use platform-knowledge-graph-engineer and platform-ai-engineer, then platform-model-qa. Add platform-security-architect/appsec for tool access, tenant data, prompt/context or automated-action risk. Reality-check before enablement claims.
```

### Review a PR

```text
Use platform-code-reviewer plus the domain specialist. Add platform-appsec-engineer if auth/tenant/secrets/untrusted input is touched. Run platform-evidence-collector. Use platform-reality-checker only for a completion/release-readiness decision.
```

## What P0 and P1 mean

- **P0 agents** protect engineering truth: scope, code understanding, review, security, testing, evidence and performance. They are not all invoked on every task.
- **P1 agents** are domain specialists for API, network, database, SRE, IAM, graph/AI, DevOps/Git/workflows and UI.

If you are unsure which agent to use, do not guess. Ask the parent agent to route using `project_agents/USAGE_PLAYBOOK.md`.

> Current-project note: the canonical repository is Mother Reference 3.2.2 and includes `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md`; current root `AGENTS.md` always wins over this packaged snapshot.

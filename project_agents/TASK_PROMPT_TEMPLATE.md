# Platform Engineering Task Template

Copy this into the parent agent in ChatGPT/Cursor/Codex/Antigravity.

```text
You are the parent Platform engineering agent.

Repository: gagandsm-max/platform
Authority: root AGENTS.md + Platform mother-reference set.

TASK
<exact objective>

SCOPE
<subsystem/files/branch/PR constraints if known>

NON-GOALS
<what must not be changed>

ACCEPTANCE
<observable behavior/tests/evidence required>

AGENT ROUTING
Read project_agents/AGENT_MATRIX.md and project_agents/USAGE_PLAYBOOK.md. Select the smallest useful set of agents. Use read-only diagnosis first when the implementation/root cause is unclear. Use platform-minimal-change-engineer for bounded implementation discipline. Use independent review/evidence after writes. Do not run parallel write agents in the same checkout.

COMPLETION
Return changed files, tests/evidence, risks/limitations and unresolved items. Do not claim production/GA readiness unless platform-reality-checker verifies the applicable quality gates.
```

# Platform Agent Pack — Operational Smoke Test

This document separates **static deployment validation** from **tool runtime validation**. Static repository checks prove deterministic deployment assets. Runtime checks prove that Cursor, Antigravity, ChatGPT+GitHub and GitHub CI actually enforce/discover those assets. Do not claim one from evidence of the other.

## 1. Static deployment validation

Run from the repository root without modifying production files:

```bash
python3 project_agents/tools/sync_agents.py
python3 project_agents/tools/validate_agents.py
python3 project_agents/tools/sync_agents.py
git diff --exit-code
git diff --check
```

Expected evidence:

- manifest and canonical SHA integrity pass;
- exactly 27 canonical agents and exactly 27 adapters for Cursor, Codex and Antigravity;
- Cursor frontmatter parses and enforces manifest-driven `readonly` plus `model: inherit`;
- Antigravity frontmatter contains only the governed tool names, correct command policy, `mainAgent: false`, `subagent: true`, `model: inherit` and canonical SHA;
- Codex TOML parses with Python stdlib `tomllib`, contains no deceptive role-level sandbox claims, and preserves canonical SHA marker and developer instructions;
- controlled Codex session launcher (`project_agents/tools/run_codex_agent.py`) projects manifest mode to native session sandbox;
- root authority/routing assets exist;
- second synchronization produces no diff.

Static validation does **not** prove that a vendor UI/CLI discovered or enforced a custom agent at runtime.

## 2. Cursor runtime validation

1. Open the project at the repository root.
2. Confirm project custom subagents are discovered from `.cursor/agents/`.
3. Confirm representative agents are visible:
   - `platform-codebase-onboarding-engineer`
   - `platform-minimal-change-engineer`
   - `platform-code-reviewer`
   - `platform-evidence-collector`
4. Invoke a READ agent against a disposable/non-production task. Confirm Cursor reports/enforces read-only capability, cannot edit files and cannot execute state-changing shell commands.
5. Confirm the READ agent can still inspect repository files and return evidence.
6. Invoke a WRITE agent only in an isolated disposable branch/worktree. Explicitly request one bounded change to a disposable smoke-test file, verify the write is possible, then discard the worktree/branch.
7. Confirm root `AGENTS.md` remains the higher authority.

Do not use Core, Sensor, Agent runtime, UI, analytics, schemas, device packs or Mother Reference files as the write smoke target.

## 3. Antigravity runtime validation

1. Open the repository as the active workspace and use `/agents`.
2. Confirm the expected 27 workspace agents are visible from `.agents/agents/`.
3. Invoke a representative READ agent.
4. Confirm it reads root `AGENTS.md`/project authority before material work.
5. Confirm `replace_file_content` is absent from the READ agent tool set.
6. Confirm terminal commands are not autonomously executed under `commandExecutionPolicy: off`; any state-capable command requires explicit approval.
7. Confirm an approved inspection/test command can be used when the parent/user explicitly authorizes it.
8. Invoke a WRITE agent only in a disposable worktree and verify a deliberately bounded disposable-file write when explicitly instructed.
9. Confirm `.agents/rules/platform-authority.md` is applied and no specialist overrides root authority.

Terminal sandboxing alone is not accepted as repository read-only evidence because sandboxed commands may still write the mounted workspace.

## 4. Codex runtime validation

Codex 0.153.4:
- discovers `.codex/agents/*.toml` standalone role files;
- applies role `developer_instructions` faithfully to the agent session;
- does **not** apply role-level sandbox permissions (subagents inherit parent/session permissions);
- cannot provide security enforcement through role TOML fields alone.

Therefore governed Platform Codex execution must use the controlled session launcher:

```bash
python3 project_agents/tools/run_codex_agent.py <agent-name> [prompt]
```

Runtime certification requirements:

1. **READ role (`platform-code-reviewer`):**
   - Launcher selects native `read-only` session (`-s read-only`).
   - Repository reading succeeds (e.g. `head -n 5 AGENTS.md`).
   - File write attempts (`apply_patch`) are blocked by the native session sandbox.
   - Shell write redirection (e.g. `echo PROBE > ...`) is blocked by the native sandbox.
2. **WRITE role (`platform-minimal-change-engineer`):**
   - Launcher selects native `workspace-write` session (`-s workspace-write`).
   - Bounded disposable workspace write succeeds when host sandbox allows.
   - Delete probe file immediately; zero commits.
3. **Safety rules:**
   - Never certify via `danger-full-access`.
   - Never claim a role TOML itself provides permission enforcement.
   - If native `workspace-write` fails because of Codex/host sandbox startup (e.g. unprivileged user namespace or bubblewrap loopback `RTM_NEWADDR` restrictions), report **`CODEX WRITE RUNTIME BLOCKED`**, not Agent Pack PASS.

## 5. ChatGPT + GitHub runtime validation

Using connected GitHub only:

1. Read root `AGENTS.md`.
2. Read `project_agents/AGENT_MATRIX.md` and `project_agents/USAGE_PLAYBOOK.md`.
3. Read the requested `project_agents/canonical/<tier>/<agent>.md` definition before adopting that specialist lens.
4. Perform a read-only repository task under the selected role.
5. Confirm ChatGPT remains CTO/orchestrator and the specialist remains subordinate to root authority/Mother Reference.
6. Confirm the response does not claim that 27 persistent specialist agents are installed in ChatGPT; these are repository-defined roles/adapters.
7. For completion/release claims, confirm independent evidence/reality-gate requirements are applied.

## 6. GitHub / CI validation

Run locally where the repository-supported environment exists:

```bash
python3 project_agents/tools/validate_agents.py
make test-static-contracts
```

Then push the branch and inspect **current-head** Platform CI. Static Contracts must execute `python3 project_agents/tools/validate_agents.py` unconditionally and fail if the validator is missing or invalid. Record the workflow run ID, exact head SHA and every job conclusion.

Do not automate Cursor, Antigravity or Codex installation/discovery through GitHub Actions. Vendor runtime smoke remains an explicit tool-runtime check outside CI.

## 7. Pass criteria

**STATIC DEPLOYMENT VALIDATION PASS** requires deterministic synchronization, validator PASS, zero second-sync diff, clean `git diff --check`, expected adapter counts and applicable static CI evidence.

**TOOL RUNTIME VALIDATION PASS** requires the relevant vendor/runtime checks above. Until Cursor, Antigravity and Codex discovery and permission behavior are observed in their actual runtimes, report them as **NOT VERIFIED**, not inferred from static files.

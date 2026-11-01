# Issue #285 Phase A: native v2 task preflight

This is a deterministic **inspection contract**, not an authorization or merge
service. It extends `control_plane.py` through `inspect_task_preflight` and uses
the existing v2 task schema, state machine, role rules and scope validator.
Existing task/handoff schemas and transition behavior are unchanged.

Authority: Issue #285 Phase A; Mother Reference v3.3.0 discovered through
`Platform_00` sections 5–6; `Platform_12` section 18.1 (writer independence);
`Platform_13` engineering preflight and Docker standards; `PRD-DEV-001`.
No new product requirement, library dependency, runtime service or certification
claim. Private-source access uses the operator's already installed GitHub CLI.

## Invocation and trust boundary

Run a trusted, independently deployed control-plane installation:

```sh
python3 -B /trusted/platform/project_agents/tools/task_preflight.py \
  --worktree /absolute/assigned/worktree --manifest /assigned/task.json
```

The inspected repository is data. The tool never imports Python from that
repository, executes its hooks, fetches/pushes branches, writes task state,
modifies its index, or merges. The native module and its policy data come from
the running installation; its native inputs must match the pinned current main.
Python installation/import paths must themselves be trusted. Do not execute this
PR's code with privileged CI credentials, and do not mount credentials or a
Docker socket into the test container. Deploying the trusted CI runner belongs
to Phase B; this PR does not change any workflow or required-check policy.

The CLI accepts no actor, token, CI result, override, policy path or bypass flag.
Legal tool-role combinations are **validated declarations**, not authenticated
caller identities. Exit 0 means structural inspection passed, with all of:

```json
{"result":"INSPECTION_ONLY","structuralStatus":"PASS","authorized":false,
 "authorization":"UNVERIFIED","independentReview":"UNVERIFIED",
 "ciEvidence":"UNVERIFIED","mergeReady":false}
```

No consumer may interpret this as permission to write, independent acceptance,
CI_VALIDATION, REALITY_GATE or MERGE_READY. A self-supplied task can claim scope
and roles; inspection cannot establish their approval. Trusted task publication,
credential binding and lane leases remain required in Phases B/C. There is no
authorization-granting API in Phase A, even for a caller claiming gatekeeper.
All rejection paths return exit 2 and a stable code without raw Git stderr.

### Private source adapter (P1-A correction)

The source reader invokes the trusted installed `/usr/bin/gh` with a fixed
read-only GraphQL query for canonical repository main/task refs, an explicit
`github.com` host and `cwd=/`. It does not invoke Git credential helpers or
execute inspected-repository configuration. No executable/query/host override
is accepted from task data or the CLI. The task branch is a validated query
variable; the repository identity and returned commit/ref types are checked.

Only the trusted launcher's `HOME`, `XDG_CONFIG_HOME`, `GH_CONFIG_DIR`, `GH_TOKEN`
and `GITHUB_TOKEN` authentication settings are inherited by this adapter.
These settings and the installed binary must be outside the untrusted task's
control. Debug, proxy/TLS/host overrides and inherited Git settings are excluded.
Credentials are never put in subprocess arguments or results; stderr is discarded.
Errors/timeouts do not retain credential-bearing exception contexts or causes.
Local Git inspection does not receive GH/GITHUB credential environment values.

The adapter has a 20-second subprocess deadline, refuses partial GraphQL errors,
and has no cached/public/anonymous fallback. Missing CLI, unavailable keyring,
denied authentication and timeouts block inspection. An absent branch is accepted
only as an explicit null ref in an otherwise successful canonical repository
read. Source-access authentication does not identify the task actor, approve
scope or certify independent review. Read-only operations do not establish that
the operator's credential itself has least-privilege grants; that audit remains
Phase C. This adapter is not a privileged CI policy deployment.

## State and source semantics

| Native status | Structural behavior |
| --- | --- |
| IMPLEMENTATION, LOCAL_VALIDATION | Named non-main assigned writer branch. Local HEAD must equal manifest exactHead. New unpublished branches and local commits ahead of the remote branch are allowed. Remote divergence/ahead/unknown ancestry blocks. Result lane is WRITER_IN_PROGRESS. |
| SOURCE_REVIEW, ADVERSARIAL_RUNTIME, SECURITY_DOMAIN_REVIEW, CI_VALIDATION | Remote branch must exist and equal exactHead. Worktree/index must be clean, including ignored/untracked files. Detached review checkout is allowed. Result is only PUBLISHED_REVIEW_CANDIDATE; publication is not independent review or CI proof. |
| REALITY_GATE, MERGE_READY, POST_MERGE_VERIFY, COMPLETE | Always blocked: TRUSTED_GATE_EVIDENCE_REQUIRED. There is no Phase A path accepting supplied CI/actor claims. |
| Other states | Blocked as unsupported by this physical preflight. Native v2 lifecycle is unchanged. |

The authoritative main pin is also the inspection diff base; it must be the
current remote main and an ancestor of local HEAD. This deliberately rejects
an out-of-date task base instead of silently choosing a merge base. Live main
and branch refs are read twice through the authenticated fixed GitHub API query
outside the target repository, without target URL rewrites or Git configuration.
Network failures never fall back to cached `origin/main`. Origin fetch and push
URLs, physical worktree path and manifest repository must match. Local commits
need an updated exactHead in their task input but need not yet be pushed.

Actual PR identity/base, synthetic merge parents, authenticated CI checkout/run
provenance, required check applicability, skipped/missing checks and independent
review identity remain Phase B work. In the A-green/B-head case no merge
acceptance is possible here; ancestry is insufficient for published evidence.

## Filesystem and policy checks

Changes are the union of main→HEAD, HEAD→index, index→worktree, untracked and
ignored-untracked paths. NUL-delimited Git output prevents quoted-path parsing
errors. Separate index/worktree diffs catch cancellation. Rename detection is
disabled so both source and destination must satisfy the allowed scope.
Scope is a literal relative file/directory prefix, never a glob. Traversal,
absolute paths, `.git`, control characters, non-UTF-8 and ambiguous paths block.

Changed symlinks and symlink parents, tree/index symlink modes, submodule
gitlinks, special files and unmerged indexes block. Canonical unchanged root
`AGENTS.md` may remain its existing symlink. Policy bytes/modes are compared
directly with pinned main, including changes hidden by index flags. Any remaining
assume-unchanged/skip-worktree flag blocks ordinary source inspection as well.

Protected surfaces include Mother References, **all `project_agents/**`**, generated
projections, GitHub workflows and any nested `AGENTS.md`. This includes current
and future router/governance documents, such as `CHATGPT_AGENT_ROUTER.md`,
`AGENT_MATRIX.md`, `START_HERE.md`, `USAGE_PLAYBOOK.md`, `TOOL_COMPATIBILITY.md`
and `TASK_PROMPT_TEMPLATE.md`. Widening task scope cannot exempt
those surfaces. This guard-development PR changes protected code: it needs the
separate authorized governance review lane and cannot use its own ordinary-task
preflight to approve itself. There is intentionally no policy-change bypass.

This is a point-in-time check, not an atomic filesystem snapshot, OS sandbox,
credential boundary, or lease. It rechecks observable head/ref/path inventory
drift but cannot exclude concurrent byte edits or same-user compromise. These
limitations must not be converted into enforcement claims.

## POC disposition

The local POC was examined as untrusted reference material, not executed or
copied as implementation authority. Its 22-test claim is not reused as evidence.
Findings: remote equality incorrectly rejected unpushed writer commits; dynamic
target policy import introduced an execution/trust boundary; actor strings did
not authenticate; HEAD→worktree alone could miss staged/unstaged cancellation;
index-only symlinks and hidden policy edits needed physical negative coverage.
Phase A uses native v2 contracts and new real-Git fixtures for these boundaries.

## Validation and rollback

Run from the repository root, using a dedicated Compose project:

```sh
docker compose -p issue285-preflight-a -f tests/engineering_orchestrator/preflight.compose.yaml build preflight-tests
docker compose -p issue285-preflight-a -f tests/engineering_orchestrator/preflight.compose.yaml run --rm --no-deps preflight-tests python3 -B -m unittest tests.engineering_orchestrator.test_task_preflight -v
docker compose -p issue285-preflight-a -f tests/engineering_orchestrator/preflight.compose.yaml run --rm --no-deps preflight-tests python3 -B project_agents/tools/validate_agents.py
docker compose -p issue285-preflight-a -f tests/engineering_orchestrator/preflight.compose.yaml run --rm --no-deps preflight-tests
```

The one-shot test fixture uses existing Python/Git tooling, a read-only source
mount, non-root user, no network, dropped capabilities and bounded tmpfs. It has
no product services, persistent volumes or host-runtime installs. Git fixtures
create/commit/push only within disposable `/tmp` repositories; remote observations
are read from their local bare remotes. No real GitHub approval/authentication is
mocked into success. Tests cover positive, negative, adversarial and regression
paths. Timeout/outage/drift injection is at the Git observation boundary; this
is not process-crash, HA, Kafka or product durability evidence.

Authentication subprocess fixtures use synthetic credential canaries and a real
process timeout; they are distinct from actual private-source evidence. `/tmp`
remains non-executable: fixture scripts run with the trusted Python interpreter.
The container init reaps fixture descendants; the PID budget stays at 128.

The separate opt-in `live-source` Compose profile verifies actual private access
and rejected authentication through the production adapter. It uses bridge
networking, the installed `/usr/bin/gh` mounted read-only, and the same non-root,
read-only, capability-dropped container restrictions. It is never selected by
ordinary offline discovery or CI. Do not mount a desktop keyring/DBus socket,
credential directory, Docker socket or writable repository. From a trusted
launcher outside the inspected checkout, pipe the existing CLI's credential
directly to stdin; token bytes are not printed, stored or placed in arguments:

```sh
set -o pipefail
/usr/bin/gh auth token --hostname github.com 2>/dev/null | \
  docker compose -p issue285-preflight-a \
    -f /home/gagan/nms-acp-v3/tests/engineering_orchestrator/preflight.compose.yaml \
    --profile live-source run --build --rm --no-deps -T preflight-live-source
```

The trusted launcher must authorize this local probe and its code; do not run
arbitrary PR policy with privileged CI credentials. The probe sets the credential
only for its process lifetime, then tests an intentionally invalid synthetic
token as the negative case. Both live tests must pass; no network failure or
missing credential is treated as SKIP/PASS. The output contains only test
outcomes and repository SHAs, not credential values or caller role claims.

Rollback removes the additive inspection entry point/module/tests/document.
No schema migration, product behavior or existing state-machine transition
changes. Required Platform CI and independent Antigravity exact-pushed-SHA
review remain separate evidence. Final gatekeeper is chatgpt-github.

## Frozen implementation scope

- `project_agents/tools/control_plane.py`
- `project_agents/tools/task_preflight.py`
- `project_agents/TASK_PREFLIGHT.md`
- `tests/engineering_orchestrator/test_task_preflight.py`
- `tests/engineering_orchestrator/preflight.compose.yaml`
- `tests/engineering_orchestrator/preflight.Dockerfile`

Mother References, tool-authority rules, canonical skill manifests and generated
projections are excluded. No automatic merge, direct main push, new dependency
or local privilege expansion is included. Remote branch-protection enforcement,
trusted task-capsule scope, overlapping lanes and credential-role enforcement
remain P0 residual limitations owned by Phases B/C, not resolved by local hooks.

## Writer validation history (retained)

Baseline: `b6b4581207126d59872b97999da8a99907d857fe`, assigned branch
`feat/issue285-agent-control-v3`, worktree `/home/gagan/nms-acp-v3`.
Writer: codex; independent verifier: antigravity in a separate read-only
checkout; final gatekeeper: chatgpt-github. Task: Issue #285 Phase A.

- First focused run: 53 executed, 52 PASS, 1 FAIL, 0 skipped. The unmerged-index
  case blocked but returned `INVALID_GIT_INDEX` instead of `UNMERGED_INDEX`.
  A broad ValueError catch was corrected to preserve PreflightError codes.
- Corrected focused run: 53 PASS, 0 FAIL, 0 skipped.
- Agent-pack validator: PASS, 34 agents / 63 skills synchronized.
- Initial full orchestrator run: 302 executed, 301 PASS, 0 FAIL, 1 skipped.
  `test_effective_diff_excludes_product_runtime` had no resolvable base because
  the initial isolated container did not mount Git metadata. This skip is not
  a pass; exact-commit validation must supply read-only Git metadata and BASE_SHA.
- Expanded focused run: 55 PASS, 0 FAIL, 0 skipped, adding an actual failed Git
  remote read and proof that target external-diff/fsmonitor hooks never execute.

These are local EXECUTION_EVIDENCE on the writer's pre-commit tree, not CI or
independent review. The draft PR records the final immutable source SHA,
exact-commit results and any subsequent failures without replacing this history.
Product runtime/HA/durability tests are NOT_RUN (no product runtime change).
Authenticated CI/role-spoofing enforcement and independent review are UNVERIFIED
and require the named downstream owners. No MERGE_READY or COMPLETE claim.

### P1 correction history (2026-11-01)

Review: PR #286 review `5727049673`, head
`2e41a7c9255fafc1b254d55eccf9d33e160a1a53`; P1-A private GitHub access and
P1-B incomplete governance protection. Both findings remain in the review history.

- First corrective focused run: 69 executed; 63 PASS, 3 FAIL, 3 ERROR. Synthetic
  adapter scripts could not execute from noexec tmpfs. Two Git fixture setup
  failures were initially reported only as nonzero subprocess exits.
- Diagnostic focused run after invoking fixtures via the trusted interpreter:
  69 executed; 67 PASS, 2 FAIL. Fixture stderr identified Git thread/fork
  exhaustion (`Resource temporarily unavailable`). Added container init for
  descendant reaping without increasing limits; subsequent focused run: 69 PASS,
  0 FAIL, 0 ERROR, 0 SKIP.
- Initial live probe using only a mounted hosts configuration: 2 executed;
  1 PASS (denied), 1 ERROR (authorized access unavailable). The existing login
  uses the desktop keyring. No desktop bus/keyring was mounted and the failure
  was not reclassified as a pass.
- Corrected live probe using trusted host CLI → stdin: 2 PASS, 0 FAIL/ERROR/SKIP.
  Actual private repository main was `b6b4581207126d59872b97999da8a99907d857fe`;
  published branch was still `2e41a7c9255fafc1b254d55eccf9d33e160a1a53` before
  the corrective push. This is live source-access evidence, not CI/actor authority.

Toolchain evidence for these runs: installed GitHub CLI 2.45.0, executable SHA256
`2fd925d68889746976958342fb749bf102bc7dc8bcba3abfa533a80ad7791673`;
offline image `sha256:aae176ae3520fbaff24955dfcaf803c87a527603e27ae0ed2025b081913b1253`;
live image `sha256:f706908ae66e4c3c8c17c9246374894423ed9be6c7700cbc8fbcff50cbee7193`.
The cached Python basis was
`python:3.12-slim@sha256:a6e34c598f2467ed0e9a8d349809fcd8b5c603269512df273a0bb1784edc11b1`.
P2 rebuild reproducibility remains limited by the mutable Dockerfile tag/apt
inputs; these recorded image identities do not claim reproducible future builds.

The corrective PR update records final exact-commit full-suite/validator results,
post-push live ref evidence and CI run identity separately. Prior tests are not
carried forward as evidence for a new SHA. Independent Antigravity review and
ChatGPT Web final disposition remain pending.

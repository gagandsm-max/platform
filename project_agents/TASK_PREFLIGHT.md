# Issue #285 Phase A: native v2 task preflight

This is a deterministic **inspection contract**, not an authorization or merge
service. It extends `control_plane.py` through `inspect_task_preflight` and uses
the existing v2 task schema, state machine, role rules and scope validator.
Existing task/handoff schemas and transition behavior are unchanged.

Authority: Issue #285 Phase A; Mother Reference v3.3.0 discovered through
`Platform_00` sections 5–6; `Platform_12` section 18.1 (writer independence);
`Platform_13` engineering preflight and Docker standards; `PRD-DEV-001`.
No new product requirement, dependency, runtime service or certification claim.

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
and branch refs are read twice through a fixed canonical HTTPS URL outside the
target repository, without target URL rewrites or inherited Git configuration.
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

Protected surfaces include Mother References, tool authority, native schemas,
state machine, validator code, manifests, canonical agents/skills, projections,
GitHub workflows and any nested `AGENTS.md`. Widening task scope cannot exempt
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

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

Sibling modules resolve by their installed `__file__` directory, with one module
and exception identity shared by direct-script, legacy bare and package imports.
There is no CWD/sys.path fallback for loading the other module. The documented
CLI and `python -P` package imports preserve specific PreflightError codes.
Conflicting preloaded module identities are rejected. Use one independently
trusted installation per interpreter; interpreter startup, standard library and
initial package resolution must also be trusted (prefer `-B -P`). This is not a
sandbox for an already compromised Python process.

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

Changes are the union of main→HEAD, HEAD→index, raw index→filesystem comparison,
untracked and ignored-untracked paths. NUL-delimited tree/index metadata prevents
quoted-path parsing errors. Object/mode comparisons preserve both staged and
unstaged cancellation, and both rename endpoints must satisfy allowed scope.
No worktree `git diff`, index refresh or conversion-enabled content command is
used. Git clean/process filters, attributes and local configuration cannot supply
comparison bytes or executable commands. Local Git runs through `/usr/bin/git`
with a minimal environment, hooks/fsmonitor disabled, remote protocols denied
and lazy fetching disabled. Ignore rules only label files already found by raw
traversal; they never exempt paths from inspection.
Scope is a literal relative file/directory prefix, never a glob. Traversal,
absolute paths, `.git`, control characters, non-UTF-8 and ambiguous paths block.

Raw traversal uses descriptor-relative `scandir`, no-follow stat/open and directory
descriptors, including ignored/untracked files and empty directories that Git
omits. Nested `.git` files, directories and symlinks always block. Only a verified
root metadata directory or linked-worktree gitfile with matching administrative
directory/common directory/backlink is excluded. Root metadata symlinks and
arbitrary gitfile redirects are rejected. No symlink targets are traversed.
Symlink text is read descriptor-relative without following its destination and
hashed as a raw Git blob with mode `120000`. `_physical_snapshot` only observes;
`_inventory` accepts a link only if the same path, mode and blob identity exist
in all four views: pinned authoritative baseline, HEAD, index and physical scan.
This provenance rule has no filename whitelist. It permits the canonical vendor
links and root `AGENTS.md`, including unchanged dangling links, without granting
permission to access their targets. Link text counts toward the scan byte budget.
Added/untracked/ignored links, changed targets, deletions, replacements and
staged/committed changes block, including changes hidden by restored disk bytes.
Changed-path parent symlink checks remain mandatory; no linked directory is
traversed. Submodule gitlinks, special files (including FIFO/socket), inaccessible
entries and unmerged indexes block. Root `AGENTS.md` retains the additional
governance-policy comparison against pinned main. Policy bytes/modes are checked even if hidden by index flags; any
remaining assume-unchanged/skip-worktree flag also blocks ordinary inspection.

Each regular file is hashed as the raw Git blob header plus streamed bytes using
the repository's SHA-1 or SHA-256 object format. Reads are at most 64 KiB, capped
at 64 MiB per file and 256 MiB total per physical scan; traversal caps are 100,000
entries, 64 directory levels, 4,096 UTF-8 path bytes and 30 seconds per scan.
Limit exhaustion fails closed as INVENTORY_LIMIT. V2 task/remote SHA contracts
remain 40-hex SHA-1; raw SHA-256 inventory is tested, but full SHA-256 task support
requires separate v2 schema change control and is not claimed here.

Raw bytes deliberately do not apply CRLF/EOL, encoding, clean/smudge or LFS
normalization. A CRLF working file differing from its LF index blob, or an LFS
materialized payload differing from its index pointer, is dirty. It blocks review,
and ordinary scope/governance rules still apply during implementation. Oversized
LFS payloads fail the byte cap. No normalization/ignore/bypass option is provided.
Use byte-identical, bounded review fixtures; generated `.env`/bytecode remain
subject to the same checks. This can intentionally reject otherwise Git-clean
development checkouts.

Protected surfaces include Mother References, **all `project_agents/**`**, generated
projections, GitHub workflows and any nested `AGENTS.md`. This includes current
and future router/governance documents, such as `CHATGPT_AGENT_ROUTER.md`,
`AGENT_MATRIX.md`, `START_HERE.md`, `USAGE_PLAYBOOK.md`, `TOOL_COMPATIBILITY.md`
and `TASK_PROMPT_TEMPLATE.md`. Widening task scope cannot exempt
those surfaces. This guard-development PR changes protected code: it needs the
separate authorized governance review lane and cannot use its own ordinary-task
preflight to approve itself. There is intentionally no policy-change bypass.

This is a point-in-time check, not an atomic filesystem snapshot, OS sandbox,
credential boundary, or lease. It compares raw digests, file/directory identity,
size, mode, mtime and ctime before/after streaming and across two inventories,
plus index/object/ref observations. Detected changes fail closed, including changes
retaining the same path inventory. It cannot exclude all concurrent edits or
same-user compromise. These
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

Run from the assigned repository root, using a dedicated Compose project. The
real-tree smoke requires readable Git metadata/object storage; linked worktrees
need the common Git directory mounted at its actual path. Match the non-root
container UID/GID to the checkout owner instead of changing object permissions.
For this assigned checkout (rebaseline the recorded BASE_SHA before reuse):

```sh
docker compose -p issue285-preflight-a -f tests/engineering_orchestrator/preflight.compose.yaml build preflight-tests
fixture_runner=(docker compose -p issue285-preflight-a -f tests/engineering_orchestrator/preflight.compose.yaml run --rm --no-deps -T
  --user "$(id -u):$(id -g)"
  -v /home/gagan/nms/.git:/home/gagan/nms/.git:ro
  -v "$PWD:$PWD:ro" -w "$PWD"
  -e BASE_SHA=b6b4581207126d59872b97999da8a99907d857fe
  -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e "GIT_CONFIG_VALUE_0=$PWD"
  preflight-tests)
"${fixture_runner[@]}" python3 -B -m unittest tests.engineering_orchestrator.test_task_preflight -v
"${fixture_runner[@]}" python3 -B project_agents/tools/validate_agents.py
"${fixture_runner[@]}" python3 -B project_documentation/tools/validate_mother_references.py
"${fixture_runner[@]}" python3 -B tests/engineering_orchestrator/run_tests.py
```

The one-shot test fixture uses existing Python/Git tooling, a read-only source
mount, non-root user, no network, dropped capabilities and bounded tmpfs. It has
no product services, persistent volumes or host-runtime installs. Git fixtures
create/commit/push only within disposable `/tmp` repositories; remote observations
are read from their local bare remotes. No real GitHub approval/authentication is
mocked into success. Tests cover positive, negative, adversarial and regression
paths. Timeout/outage/drift injection is at the Git observation boundary; this
is not process-crash, HA, Kafka or product durability evidence.

The current real-tree smoke pins `HEAD^{tree}` once, enumerates its tracked paths,
blob IDs, sizes and modes, and reads only raw Git objects. It never scans or opens
source worktree files. Each blob is length/hash verified; a new private fixture
receives raw file bytes or symlink text and exact executable modes. Raw
`hash-object --no-filters` and explicit index records avoid checkout/add/archive
conversion. The resulting index/commit tree must equal the pinned source tree.
Source attributes/configuration cannot supply executable filter/hook commands.
Git runs with a minimal credential-free environment, fixed executable, disabled
hooks/fsmonitor/lazy fetching and denied protocols. No source config is copied.

The harness rejects unsafe/duplicate paths, symlink-parent conflicts, gitlinks,
non-blob entries, pre-existing/aliased destinations, object mismatch and missing
metadata; it has no workspace-copy fallback. Manifest output is capped at 16 MiB,
files at 64 MiB, total blob bytes at 256 MiB, entries at 100,000, paths at 4,096
bytes/64 components, commands at 30 seconds and the export loop at 120 seconds
between operations. These are fixture bounds, not production authorization.
Linux inotify regressions (with a positive read control) monitor ignored secrets,
key-named files, untracked content and a sparse oversized build artifact across
Python and Git child processes. They also check absence from the destination tree
and object database. Symlink targets are never read, including a tracked link to
an ignored synthetic `.env`. Synthetic credentials only; no real secrets needed.

Authentication subprocess fixtures use synthetic credential canaries and a real
process timeout; they are distinct from actual private-source evidence. `/tmp`
remains non-executable: fixture scripts run with the trusted Python interpreter.
The container init reaps fixture descendants; the PID budget stays at 128.

### Credential-free PR tests (P1-C correction)

PR-controlled tests accept synthetic fixtures only. The real-token stdin reader,
live-auth test mode and network-enabled Compose service have been removed.
The sole test service has no network or credential/CLI mounts and closes stdin
to `/dev/null` before Python starts, even when a caller attaches a pipe. The old
live-mode argument is rejected without reading stdin. Negative regressions cover
these boundaries and the absence of an executable credential-pipe recipe.
Synthetic source success, denial, timeout, malformed/partial responses and
exception/redaction cases remain active.

Never supply real credentials through stdin, environment, files, sockets or
command arguments to mutable PR tests. Read-only mounts, non-root execution,
redacted logs and dropped capabilities do not make such code a safe credential
recipient. Compose configuration and its tests are themselves PR-controlled;
their passing assertions are not an independent sandbox or protection against
an operator overriding mounts, entrypoint, environment or network settings.

### Separately trusted external verification

An independently reviewed harness installed outside the checkout may use an
installed trusted `gh` client to read the fixed canonical repository refs. Its
code, configuration, dependencies, query, executable paths and installation
digest must be independently approved and pinned; it must not import, execute,
source or build code/configuration from the PR while holding credentials. Use
a separately provisioned least-privilege read-only principal, bounded timeout,
fixed GitHub host/repository and redacted failure handling. Credential material
stays entirely in that external trust domain and never reaches the PR process.

Record only sanitized repository identity, queried refs, exact SHAs, observation
time, harness/CLI identity and success/denial status as external evidence. Treat
that output as data, not executable instructions or actor/merge authorization.
An unavailable or denied read remains a failure, never a cached success. PR tests
can exercise synthetic responses offline; reading actual refs with an external
client proves source access only, not execution of this proposed adapter.

To test the proposed adapter itself with real credentials, an independent owner
must first vet and pin its exact code and dependencies, then deploy them into a
separate trusted installation that cannot load mutable PR code. That deployment,
credential review and integration execution are NOT_RUN in this correction and
are not authorized by a writer's green tests. No external harness is introduced
within this six-file scope; independent Antigravity review and ChatGPT Web own
the subsequent evidence disposition.

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

**Historical source-access evidence only, insufficient for security certification.**
The live-probe results below predate P1-C review `5727140670`. The real credential
was delivered to PR-controlled Python with network access. No theft is established,
but passing tests and absence of logged secrets cannot exclude exfiltration.
The credential owner should inspect prior grants and consider rotation, especially
for write/admin privileges; that assessment/rotation is not performed by this PR.

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

### P1-C correction history (2026-11-01)

Reviewed input head: `4269ba3d09df4baaf8cf700488ed9f7a3e50cf56`.
Review: https://github.com/gagandsm-max/platform/pull/286#pullrequestreview-5727140670.
At that historical head the complete orchestrator suite passed 318/318 with no
failures/errors/skips and both repository validators passed. The post-push live
probe passed 2/2 and read branch `4269ba3d09df4baaf8cf700488ed9f7a3e50cf56` and
main `b6b4581207126d59872b97999da8a99907d857fe`. This remains historical access
evidence, insufficient for security certification and not repeated here.

P1-C removes the credential-bearing test path and retains synthetic regressions.
The source adapter, whole `project_agents/**` policy, native v2 contracts and
INSPECTION_ONLY behavior are unchanged. The draft PR records new exact-commit
tests, negative container probes, validators and CI separately. Prior failed runs
above remain part of the evidence history. Independent acceptance remains pending.

- First P1-C focused run: 73 executed, 72 PASS / 1 FAIL, 0 errors/skips.
  The new removed-mode regression used direct script execution, which failed
  package import (`ModuleNotFoundError: tests`, exit 1) before argument parsing.
  Diagnostic reproduction confirmed this. Corrected the regression to use the
  former module invocation with an open, empty stdin pipe; it must reject with
  exit 2 without waiting for credential input. No production behavior was changed.
- Synthetic container boundary probe: PASS. Attached stdin was empty in Python;
  host GH_TOKEN/GITHUB_TOKEN canaries were not inherited; no `/usr/bin/gh` mount;
  non-root execution and loopback-only interfaces. No real credential was used.

### Independently adjudicated audit corrections (2026-11-01)

Rejected input: `b5425241dae3b75b4584173437eb4bdb680004ff`. Lead adjudication:
https://github.com/gagandsm-max/platform/pull/286#pullrequestreview-5727320526.
The complete Antigravity report was read from its retained transcript, step 135:
`/home/gagan/.gemini/antigravity-cli/brain/90e1c717-dc80-4a97-9450-dd8b01c2d736/.system_generated/logs/transcript_full.jsonl`.
The report and original Git objects are preserved; this correction does not amend
their findings or convert the old 322-test/green-CI evidence into security acceptance.

| Original adverse evidence | Rejected-head result | Correction / new regression |
| --- | --- | --- |
| P0-01 clean/process conversion commands | Target clean command executed; transformed output concealed out-of-scope bytes in writer/review lanes | Raw streamed index-blob comparison; both driver types tested with marker commands, scope concealment and both lanes |
| P1-01 circular bare imports | Direct CLI lost WRONG_REPOSITORY; safe-path package import broke; hostile CWD module executed | File-anchored single module identity; real CLI/package subprocesses with/without -P and hostile sibling modules |
| P1-02 Git-omitted physical paths | Nested governance/outside .git trees and symlinks passed clean review | Bounded descriptor traversal; nested metadata files/directories/symlinks, ignored/unlisted paths, special files, unreadable paths, linked worktrees |

First corrective focused run: 94 executed, 93 PASS / 1 FAIL, zero errors/skips.
The entry-count resource cap rejected, but emitted INVALID_GIT_INDEX instead of
INVENTORY_LIMIT. Corrected the error classification without weakening the cap.
Second focused run: 94 PASS, zero failures/errors/skips. Final exact-commit full
suite/validator results and CI identity are recorded separately in the draft PR.

Remaining limitations: this implementation requires Linux/POSIX no-follow and
directory-descriptor filesystem operations. Blocked/remote filesystems can stall
an individual OS read; the scan deadline is checked between bounded reads, not a
kernel I/O cancellation guarantee. No atomic snapshot/lease or same-user process
isolation is claimed. Git/source subprocess output buffering remains the audited
P2 hardening item; raw file hashing is streamed and capped. Desktop keyring
availability, ignored build artifacts, image reproducibility and privileged
self-hosted CI isolation remain separate P2/Phase B/C work; no real credentials
are passed into PR tests to work around them. Next independent Antigravity audit
must use an isolated detached exact-SHA checkout, never the writer worktree.

### P1-03 tracked-symlink operability correction (2026-11-01)

Input: `be68199d22d0b0a8338ee1fbf2c743616afebd3d`; authoritative main:
`b6b4581207126d59872b97999da8a99907d857fe`. Lead review:
https://github.com/gagandsm-max/platform/pull/286#pullrequestreview-5727599101.
Antigravity reproduced SYMLINK_PATH on the two unchanged vendored loader links.
The prior 344/344 suite, 95/95 preflight cases and successful CI run 41292504105
remain historical evidence and did not detect this real-checkout blocker.

The correction uses four-view provenance for every symlink, including AGENTS.md.
Only task_preflight.py, test_task_preflight.py and this document are changed.
There is no ordinary-policy exemption for this governance-code PR itself.

First focused correction run: **109 PASS**, 0 failures/errors/skips, 28.498s.
The historical real-tree smoke copied the actual AmrudAi source (preserving symlink
text and modes, excluding root Git metadata) into an offline disposable clean
Git checkout. It checks the three baseline link blob IDs from the lead review,
full raw inventory, governance integrity, writer/review INSPECTION_ONLY responses,
and OUTSIDE_SCOPE for a subsequent unauthorized file. Its remote observations are
synthetic: this is operability evidence, not GitHub authentication or authority.
Other regressions cover dangling baseline links, unknown filenames, changed
targets, staged/committed cancellation, deletions/mode replacement, ignored and
out-of-scope links, external targets, parent traversal, scan budgets and read-time
mutation. SHA-256 raw-link hashing is also exercised by the object-format test.
Exact-commit suite, validators, assigned-checkout smoke and new CI evidence are
recorded separately in PR #286. All earlier failures and insufficient live-probe
evidence above remain retained. New Antigravity audit and ChatGPT Web disposition
remain required; passing writer tests do not establish independent certification.

### P1-04 tracked-tree fixture correction (2026-11-01)

Input: `6c237172c87e7315646cade14d40636357eabeaf`; main remains
`b6b4581207126d59872b97999da8a99907d857fe`. Lead reviews:
https://github.com/gagandsm-max/platform/pull/286#pullrequestreview-5727638463 and
https://github.com/gagandsm-max/platform/pull/286#pullrequestreview-5727920992.
Exact-head independent audit: retained transcript step 194 at
`/home/gagan/.gemini/antigravity-cli/brain/e6e645a8-01f3-4b64-8e11-2a3772e17a4f/.system_generated/logs/transcript_full.jsonl`.

Antigravity confirmed the former whole-workspace copy and force-add incorporated
synthetic ignored `.env`, `secrets/id_rsa.key` and `build/output.bin` into temporary
Git history. The prior smoke/358-test passing evidence is insufficient to prove
safe fixture data handling. No real-credential exposure or exfiltration is
established. Earlier P0/P1 findings and historical live-probe evidence remain
retained above. Only the test file and this document change for P1-04; production
preflight, workflows, runner cleanup and PostgreSQL implementation are unchanged.

First targeted run: **7 executed, 6 PASS / 1 ERROR**, zero failures/skips. The
exporter failed closed as FIXTURE_GIT_FAILED: Git could not read tree object
9671a48306aafca3a526773ff9496cb8d81e566b from an owner-only pack under container
UID 65534. Repeating with checkout-owner non-root UID/GID 1000:1000 and the same
read-only mounts passed **7/7 in 21.235s**. No object permissions, checks or
production code were changed. Subsequent SHA-256 and timeout regressions and
the pre-commit full suite passed **366/366 in 47.246s**, with zero failures,
errors or skips. Both canonical validators passed. Exact-commit results are
reported separately in PR #286.

Prior CI run 41298313964 remains failed/incomplete evidence. Review 5727920992
classifies the reported integration job interruption as runner communication
failure; PostgreSQL assertion failure and infrastructure root cause remain
UNVERIFIED. Runner diagnosis/repair is a separate assignment. This correction
does not inspect runner host logs or alter CI services/containers/volumes.
New exact-head CI, independent Antigravity audit and ChatGPT Web disposition
remain required. No merge-readiness or independent certification is asserted.

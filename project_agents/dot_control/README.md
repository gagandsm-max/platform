# AmrudAi Dot Host-Control Bridge v1

Repository-owned, host-visible, user-space control bridge that runs as Linux
user `gagan` and executes **only** strictly validated AmrudAi task manifests.

Governing issue: `#185` — Dot Master Takeover v2
Architecture / Master hold: Issue `#185` comment `6036944290`

Disposition target after implementation:

`DOT_HOST_CONTROL_BRIDGE_V1_IMPLEMENTED_AWAITING_MASTER_REVIEW`

## Architecture

```
Dot (managed sandbox)
  └─ writes prompt + manifest.tmp under /home/gagan/nms-dot-control/tasks/<taskId>/
  └─ atomic rename → inbox/<taskId>.json

Host bridge (user gagan, no sudo)
  inbox/ → validate → processing/ → exclusive worktree lock
       → allowlisted action → logs/ + results/<taskId>.result.json
       → state/ → release lock

Dot reads results/<taskId>.result.json
```

The bridge never accepts a free-form command string. Every `argv` is constructed
internally from allowlisted action types and validated fields.

## Brand / schema identity

- Internal schema `$id`: `urn:amrudai:schema:dot-control:task-manifest:v1`
- JSON Schema meta: `https://json-schema.org/draft/2020-12/schema` (unchanged)
- Public website (when a resource is actually publicly hosted): `https://amrudai.com`
- Forbidden: the legacy mistyped brand domain and unserved SaaS-style public schema URLs

This schema is **not** a hosted SaaS endpoint.

## Trust boundaries

| Zone | Authority |
|------|-----------|
| Dot managed Work/Codex sandbox | May write prompt + manifest under `tasks/<taskId>/` and inbox rename only |
| Host bridge | May run allowlisted git/test actions inside approved worktrees |
| Cursor CLI / Codex | Sole writers for an isolated worktree (never Dot) — **live launch UNSUPPORTED in v1** |
| Antigravity | Read-only audit on a **separate detached** worktree — **live launch UNSUPPORTED in v1** until proven |
| Master + GitHub | Merge and certification gates (bridge never merges or certifies) |

## Supported actions (v1)

| Action | Purpose | Live execution |
|--------|---------|----------------|
| `REPO_PROBE` | Fixed read-only git evidence (no fetch) | Yes — honest SUCCESS/PARTIAL/FAILED |
| `GIT_COMMIT_PROOF` | Gate 3 proof: one-file local commit on non-main branch | Yes — isolated Git env; hooks/signing/filters disabled or refused |
| `TEST_PROFILE` | Hard-coded profile IDs (planning argv only) | **UNSUPPORTED** — no child process; no false suite evidence |
| `CURSOR_TASK` | Argv construction / dry-run only | **UNSUPPORTED** until credential isolation proven |
| `CODEX_TASK` | Argv construction / dry-run only | **UNSUPPORTED** until credential isolation proven |
| `ANTIGRAVITY_AUDIT` | Argv construction / dry-run only | **UNSUPPORTED** until read-only boundary proven |

## Unsupported actions (v1)

- Live Cursor/Codex/Antigravity child launch (false push/PR flags are not enforcement)
- Live `TEST_PROFILE` child launch (repository-controlled Python/Make is not a sandbox)
- Arbitrary shell / `sh -c` / `bash -c` / `eval` / `shell=True`
- Free-form `command` / `argv` / `sudo` fields in manifests
- Unknown manifest fields (runtime matches `additionalProperties: false`)
- Push, pull-request creation, merge, certification
- Direct writes to `main` / `master` / `origin/main`
- `danger-full-access` Codex sandbox
- Cursor `--force` / `--yolo`
- Antigravity `--dangerously-skip-permissions`
- Docker socket, systemd privileged operations, GitHub Actions host namespace control
- Product runtime mutation via this bridge
- Dynamic Python imports from task data

## v1.1 credential-isolation requirements (not implemented here)

Before live CURSOR_TASK/CODEX_TASK may be enabled:

- isolated HOME / tool config
- no GitHub/Git credential helper
- no SSH agent (`SSH_AUTH_SOCK` absent)
- `GIT_TERMINAL_PROMPT=0`
- `GIT_ASKPASS` disabled
- proven failed push/PR attempts under isolation
- required tool authentication without granting repository credentials

## Path security

Approved repository/worktree roots:

- `/home/gagan/nms`
- `/home/gagan/nms-*`

Approved control root:

- `/home/gagan/nms-dot-control`

Bound repository identity: `gagandsm-max/platform` (manifest + origin remote).

Rejected: `.ssh`, `.gnupg`, browser profiles, password stores, personal documents,
unrelated repos, relative traversal, symlink worktree/control roots/subdirs,
paths outside approved roots (`Path.resolve()` containment).

Control root/subdirs use mode `0700`; manifests/results/state/log/lock files use
`0600`. Owner UID must match the bridge user. Symlink `logs`/`results`/`state`/
`locks` and result/lock symlinks are rejected. Durable opens use `O_NOFOLLOW`
where practical. **Explicit `init` may create/repair modes; normal oneshot/daemon
operation rejects unsafe modes without repairing.**

## Branch / SHA / base security

Before every action the bridge verifies:

- git repository state
- branch or detached HEAD rules
- exact `expectedHead`
- `authoritativeBase` exists (`git cat-file -e <base>^{commit}`)
- `authoritativeBase` is an ancestor of `expectedHead`
- origin remote normalizes to `gagandsm-max/platform` (credentials sanitized, never logged)
- clean/dirty requirements

Governed historical bases are allowed; `authoritativeBase` need not equal
`origin/main`, but missing `origin/main` makes `REPO_PROBE` non-SUCCESS.

## Change scope (CURSOR_TASK / CODEX_TASK)

After a writer action the bridge unions (NUL-safe Git output):

1. committed paths in `startingHead..endingHead`
2. staged
3. unstaged
4. untracked

Every path is validated as a safe repository-relative path. Applied against
`allowedFiles` / `forbiddenFiles` with explicit `allowCommit` semantics.

## Locking

One exclusive `fcntl.flock` lock per canonical worktree. Lock records include
`taskId`, `action`, `writerTool`, `worktree`, `branch`, `expectedHead`,
`bridgePid`, `childPid`, `createdAt`, `heartbeatAt`.

- Cursor and Codex cannot own the same worktree concurrently.
- Live locks are never auto-deleted.
- Stale locks require PID + worktree-state inspection before reclaim.
- Lock release occurs only after durable provisional result/state exist.
- Result is then updated with the final released-lock lifecycle.
- Evidence-write failure keeps non-success forensic state; lock is not silently
  released with missing evidence.
- Inbox/processing filenames must be exactly `<taskId>.json`.
- A process-wide task-id flock prevents concurrent duplicate admission.

## Process / timeout / logging

`subprocess` with argv arrays and `shell=False`, new process group, bounded
timeout. On timeout: `SIGTERM` process group → grace → `SIGKILL`.

Durable stdout/stderr/bridge logs:

- hard byte bounds (`MAX_STDOUT_LOG_BYTES`, `MAX_STDERR_LOG_BYTES`, `MAX_BRIDGE_LOG_BYTES`)
- raw capture retained at most bound + small redaction overlap, then redacted
- credential redaction across chunk boundaries before durable storage
- final durable file size (including truncation marker) never exceeds the bound
- truncation marked in `TaskResult`
- captured byte counts recorded
- file mode `0600`
- no environment dumps
- `safe_read_text` uses a bounded `os.read`, never `read_bytes()[:n]` of a huge file

Child env omits `HOME` / `SSH_AUTH_SOCK` and forces `GIT_TERMINAL_PROMPT=0`.
`GIT_COMMIT_PROOF` uses an isolated Git config (no system/global config, empty
`core.hooksPath`, signing disabled, no credential helper) and refuses
clean/smudge/filter attributes.

## Evidence model

Every `results/<taskId>.result.json` includes task identity, tools, worktree,
branch, starting/ending HEAD, authoritative base, redacted argv, timestamps,
exit code, timeout state, log paths, changed/committed files, commit SHA,
push/PR status (`NOT_ATTEMPTED` in v1), lock lifecycle, truncation flags, and
disposition.

Never logged: tokens, private keys, credential-bearing URLs, environment dumps.

## Dot submission workflow

1. Dot creates a local Codex task scoped only to
   `/home/gagan/nms-dot-control/tasks/<taskId>/`
2. That task writes the prompt file and `manifest.tmp`
3. After local validation it renames to
   `/home/gagan/nms-dot-control/inbox/<taskId>.json`
   (write `.tmp` → fsync/close → rename to `.json`)
4. Host bridge processes outside the managed Dot sandbox
5. Dot reads `/home/gagan/nms-dot-control/results/<taskId>.result.json`

The managed Dot task never needs Git-metadata, Docker-socket, or systemd write access.

## Setup / init

Runtime state lives **outside** the repository:

```bash
PYTHONPATH=project_agents python3 -m dot_control \
  --control-root /home/gagan/nms-dot-control init
```

Creates: `inbox/`, `processing/`, `results/`, `logs/`, `locks/`, `state/`,
`quarantine/`, `tasks/` at mode `0700`.

Tests must not create the production control root; they use temporary roots.

## CLI modes

```bash
# Validate / dry-run (no execution)
PYTHONPATH=project_agents python3 -m dot_control validate /path/to/manifest.json

# Process exactly one inbox manifest
PYTHONPATH=project_agents python3 -m dot_control oneshot

# Daemon poll
PYTHONPATH=project_agents python3 -m dot_control daemon --poll 2

# Inspect lock
PYTHONPATH=project_agents python3 -m dot_control inspect-lock /home/gagan/nms-foo
```

Also: `python3 project_agents/dot_control/bridge.py <subcommand>`.

## systemd user service

Template only: `systemd/amrudai-dot-control.service`
Do **not** install or enable during implementation. User-space only (no sudo).

## Recovery

- Invalid manifests → `quarantine/` with reason sidecar
- Crash mid-task → non-success disposition when recovered; lock file retains forensics
- Stale lock → `inspect-lock`, confirm PIDs dead, then retry (reclaim on acquire)
- Live lock → do not delete; wait or investigate holder

## Security boundaries / revocation

- No root service, no sudo, no unrestricted host access
- No credentials in manifests or logs
- Disable user unit / stop daemon to revoke bridge execution
- Remove or chmod control-root inbox to stop new admissions

## Why unrestricted sudo and arbitrary shell are prohibited

Dot’s managed sandboxes cannot write Git metadata reliably. The temptation is to
grant the agent host shell/sudo. That collapses the security model: a prompt or
manifest field becomes remote code execution on the founder laptop. The bridge
exists so Dot can *request* allowlisted operations while the host constructs
every argv from validated structured fields under path, branch, lock, and
evidence constraints.

## Known limitations

- Live Cursor/Codex/Antigravity/TEST_PROFILE are UNSUPPORTED in v1 (argv dry-run only).
- v1 does not perform push/PR/merge/certification.
- Manifest `allowPush`/`allowPullRequest` must be false, but that alone is not
  technical push prevention — credential isolation is deferred to v1.1.
- Test profile names are truthful suite IDs only
  (`capability-pack`, `capability-pack-unittest`, `static-contracts`) — never
  implying Agent Go/build gates — and live execution remains disabled.
- Bridge does not supervise Docker or GitHub Actions runners.
- Production control-root init is operator-driven, not automatic from tests.

## Manifest schema

See `task.schema.json`. Required fields include `schemaVersion`, `taskId`,
`governingIssue`, `action`, `writerTool`, `verifierTool`, `repository`,
`worktree`, `branch`, `authoritativeBase`, `expectedHead`, `allowedFiles`,
`forbiddenFiles`, `promptFile`, `timeoutSeconds`, `allowCommit`, `allowPush`,
`allowPullRequest`, `createdAt`, `requestedBy`.

Action-specific: `commitMessage` (`GIT_COMMIT_PROOF`), `testProfile` (`TEST_PROFILE`).

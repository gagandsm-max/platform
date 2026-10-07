# AmrudAi Dot Host-Control Bridge v1

Repository-owned, host-visible, user-space control bridge that runs as Linux
user `gagan` and executes **only** strictly validated AmrudAi task manifests.

Governing issue: `#185` — Dot Master Takeover v2  
Architecture comment: Issue `#185` comment `6036350515`

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

## Trust boundaries

| Zone | Authority |
|------|-----------|
| Dot managed Work/Codex sandbox | May write prompt + manifest under `tasks/<taskId>/` and inbox rename only |
| Host bridge | May run allowlisted git/CLI/test actions inside approved worktrees |
| Cursor CLI / Codex | Sole writers for an isolated worktree (never Dot) |
| Antigravity | Read-only audit on a **separate detached** worktree |
| Master + GitHub | Merge and certification gates (bridge never merges or certifies) |

## Supported actions (v1)

| Action | Purpose |
|--------|---------|
| `REPO_PROBE` | Fixed read-only git evidence (no fetch) |
| `GIT_COMMIT_PROOF` | Gate 3 proof: one-file local commit on non-main branch |
| `CURSOR_TASK` | Launch Cursor CLI (`agent --print --trust --workspace …`) |
| `CODEX_TASK` | Launch Codex (`codex exec --sandbox workspace-write --approve-for-me`) |
| `ANTIGRAVITY_AUDIT` | Launch `agy --print --sandbox --mode plan` on detached clean HEAD |
| `TEST_PROFILE` | Run hard-coded profile IDs only |

## Unsupported actions (v1)

- Arbitrary shell / `sh -c` / `bash -c` / `eval` / `shell=True`
- Free-form `command` / `argv` / `sudo` fields in manifests
- Push, pull-request creation, merge, certification
- Direct writes to `main` / `master` / `origin/main`
- `danger-full-access` Codex sandbox
- Cursor `--force` / `--yolo`
- Antigravity `--dangerously-skip-permissions`
- Docker socket, systemd privileged operations, GitHub Actions host namespace control
- Product runtime mutation via this bridge
- Dynamic Python imports from task data

## Path security

Approved repository/worktree roots:

- `/home/gagan/nms`
- `/home/gagan/nms-*`

Approved control root:

- `/home/gagan/nms-dot-control`

Rejected: `.ssh`, `.gnupg`, browser profiles, password stores, personal documents,
unrelated repos, relative traversal, symlink worktree/control roots, paths outside
approved roots (`Path.resolve()` containment).

## Branch / SHA security

Before every action the bridge verifies git repository state, branch or detached
HEAD rules, exact `expectedHead`, `authoritativeBase` (40-char hex), and
clean/dirty requirements. Drift stops execution.

- Writers must not use detached HEAD.
- Antigravity must use detached HEAD on a verifier worktree (never the writer tree).

## Locking

One exclusive `fcntl.flock` lock per canonical worktree. Lock records include
`taskId`, `action`, `writerTool`, `worktree`, `branch`, `expectedHead`,
`bridgePid`, `childPid`, `createdAt`, `heartbeatAt`.

- Cursor and Codex cannot own the same worktree concurrently.
- Live locks are never auto-deleted.
- Stale locks require PID + worktree-state inspection before reclaim.
- Lock release occurs only after child termination and evidence finalization.

## Process / timeout handling

`subprocess` with argv arrays and `shell=False`, new process group, stdout/stderr
to files, bounded timeout. On timeout: `SIGTERM` process group → grace →
`SIGKILL`. Child PID recorded. Minimal sanitized environment (no env dumps).

## Evidence model

Every `results/<taskId>.result.json` includes task identity, tools, worktree,
branch, starting/ending HEAD, authoritative base, redacted argv, timestamps,
exit code, timeout state, log paths, changed/committed files, commit SHA,
push/PR status (`NOT_ATTEMPTED` in v1), lock lifecycle, and disposition.

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
`quarantine/`, `tasks/`.

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

- Live Cursor/Codex/Antigravity runs require local auth and may still hit tool
  approval UX; bridge omits `--force`/`--yolo`/`danger-full-access` by design.
- v1 does not perform push/PR/merge/certification.
- v1 test profiles are narrow (`capability-pack`, `agent`, `static-contracts`).
- Bridge does not supervise Docker or GitHub Actions runners.
- Production control-root init is operator-driven, not automatic from tests.

## Manifest schema

See `task.schema.json`. Required fields include `schemaVersion`, `taskId`,
`governingIssue`, `action`, `writerTool`, `verifierTool`, `repository`,
`worktree`, `branch`, `authoritativeBase`, `expectedHead`, `allowedFiles`,
`forbiddenFiles`, `promptFile`, `timeoutSeconds`, `allowCommit`, `allowPush`,
`allowPullRequest`, `createdAt`, `requestedBy`.

Action-specific: `commitMessage` (`GIT_COMMIT_PROOF`), `testProfile` (`TEST_PROFILE`).

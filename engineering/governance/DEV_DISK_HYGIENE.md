# Development Disk Hygiene and Resource Limits

Governing requirements: `Platform_13_Engineering_Development_and_Docker_Standards.md`,
`Platform_11_Sizing_Capacity_Storage_Master.md` (§14 local log/disk protection).

## Model

| State | Storage | Lifecycle |
|-------|---------|-----------|
| Normal `dev` services | Named Platform volumes (`platform_pg_data`, Kafka/CH/Valkey/spools) | **Persistent** across `dev-down` / reboot |
| Day-02 / integration Postgres helpers | **tmpfs** at `/var/lib/postgresql/data` | **Ephemeral**; no anonymous Docker volumes |
| Build artifacts / BuildKit cache | Docker builder cache | Reclaimable via `make dev-hygiene` |
| Container stdout/stderr | `json-file` `max-size=10m` `max-file=3` | Bounded per service |

## Why generic volume prune is forbidden

`docker volume prune` and `docker system prune --volumes` delete **unreferenced** volumes,
including intentional Platform named volumes after a temporary `compose down`, HA spike
data, and any labeled test volume still under investigation. Automated scripts must never
indiscriminately prune Docker data on a shared laptop.

## Commands

### `make dev-disk-check` (read-only)

Reports:

- root filesystem use %
- `docker system df` / builder cache footprint
- image/container/volume counts
- presence of named Platform volumes

Thresholds:

| Use % | Level | Action |
|------:|-------|--------|
| &lt;70 | NORMAL | Continue |
| ≥70 | WARNING | Investigate; prefer hygiene before large builds |
| ≥80 | CLEANUP REQUIRED | Run `make dev-hygiene`, then re-check |
| ≥90 | CRITICAL | Do **not** start scale/HA workloads until space is reclaimed |

Exit codes: `0` NORMAL/WARNING, `1` CLEANUP REQUIRED, `2` CRITICAL.

### `make dev-hygiene` (safe reclaim only)

Allowed:

- BuildKit cache older than **72h**
- BuildKit cache capped to approximately **10GB** max-used (`--reserved-space 2GB`)
- **Dangling** images (`docker image prune -f` only)
- Unreferenced volumes that carry **both** labels:
  - `com.platform.lifecycle=ephemeral`
  - `com.platform.owner=test`

Never:

- `docker volume prune` / `docker system prune --volumes`
- `docker image prune -a`
- Named Platform persistent volumes
- Active containers / source / worktrees / credentials

### `make dev-reset CONFIRM=yes` (destructive)

Explicit wipe of Compose named volumes for a clean-slate **dev** environment. Not part of
daily hygiene.

## Compose logging

`compose.yaml` defines a shared `x-logging` anchor (`*platform-logging`) applied to all
long-running services. Repository Compose limits remain required even if Docker daemon
defaults are configured later.

## Host recommendations (manual approval only)

Do **not** apply automatically from the repository.

### journald (preventive; retain current host bound)

This host already uses a tight journal ceiling. Do **not** raise it.

Recommended retain/align (manual only; do not auto-edit host):

```
# /etc/systemd/journald.conf
[Journal]
SystemMaxUse=100M
RuntimeMaxUse=200M
```

If RuntimeMaxUse is unset today, 200M is an optional runtime bound; keeping
SystemMaxUse=100M is mandatory for this laptop. Then:
`sudo systemctl restart systemd-journald` only after explicit approval.

### Docker daemon log defaults (new containers)

```json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}
```

Daemon defaults generally apply to **newly created** containers; existing services may need
recreation during an approved maintenance window. Keep Compose logging regardless.

## Historical anonymous volume pool (~41GB)

See `engineering/governance/ANON_PG_VOLUME_POOL_DISPOSITION.md`. Deletion requires
independent review — not performed by `dev-hygiene`.

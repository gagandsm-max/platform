# Packaging, reboot, upgrade, rollback

## Artifacts
- deb, rpm, MSI; systemd units; Windows Service
- Reboot auto-start and dependency readiness
- Upgrade/rollback; spool preservation; identity preservation; configuration migration
- Crash-loop evidence must be visible and bounded

## Invariants
- Auto-start must not self-promote HA authority or bypass fencing.
- Spool/identity must survive upgrade when durability is claimed.

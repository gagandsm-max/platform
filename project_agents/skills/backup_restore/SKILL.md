---
name: backup_restore
description: Backup/restore, retention, cardinality and 80% storage-protection behavior including legal-hold limits.
---

# backup_restore

## Purpose

Protect Platform recovery paths for PostgreSQL control state, ClickHouse analytics, configuration/secret **references**, and understand Sensor spool recovery boundaries—without inventing unsupported backup technology or promising impossible erasure.

## Use when

Backup jobs, PITR, restore drills, retention/cardinality changes, legal-hold behaviour, disk 80% protection interactions with recovery, upgrade/migration recovery planning, or any claim of RPO/RTO from backups. Required for `platform-database-reliability-engineer`, SRE and storage-protection work.

## Authoritative inputs

Always read:

1. `project_documentation/Platform_03_Architecture.md` — PostgreSQL = control state; ClickHouse = analytics; Kafka internal
2. `project_documentation/Platform_11_Sizing_Capacity_Storage_Master.md` — retention, 80% protection, cardinality
3. Existing approved backup/PITR mechanisms in-repo/runbooks (do not invent vendors)
4. `secrets_handling` for secret-material vs reference rules during restore
5. Task migration reservations when restore intersects schema versions

Standby Core is **not** backup. HA (`ha_dr`) is not a substitute for restore drills.

## Invariants

- PostgreSQL backup/PITR is the authority for control-state recovery when claimed; exercise the **approved** toolpath only.
- ClickHouse recovery follows its approved path (e.g. rebuild/replay/restore per architecture)—do not pretend PG dumps recover CH.
- Configuration restore uses secret **references**, not committed secret material.
- Sensor local spool recovery is bounded and device-local; it is not a cluster backup of Core.
- Backups/restores honour encryption and tenant separation where the product stores tenant data.
- Restore integrity and application consistency must be verified (not only file copy success).
- Failed/partial restore has an explicit abort/retry/roll-forward decision.
- Never promise immediate physical erasure from immutable backups/legal hold.
- Do not invent unsupported backup technology or cloud vendor features not in OSS catalogue / Decision Register.

## Required procedure

1. Classify data class: PostgreSQL control state, ClickHouse analytics, config/secret references, Sensor spool, Kafka (usually not “backed up” as customer backup—state elsewhere).
2. Confirm the approved backup mechanism for each class in scope; cite runbook/module paths.
3. Define encryption at rest/in transit for backup artefacts and access control (tenant/admin RBAC).
4. Plan restore drill: target environment (lab), point-in-time if PITR, schema/app version compatibility.
5. Execute drill; verify integrity (checksums/row counts/invariant queries) and application consistency (login, tenant isolation sample, critical control reads).
6. Measure RPO/RTO from the drill clocks; record method.
7. Inject or simulate failed/partial restore; document operator decision tree.
8. For upgrade/migration recovery: prove backup taken pre-migration and restore-to-pre-migration works or is explicitly out of scope with owner.
9. Cross-check 80% storage protection: backups must not be the excuse to disable protection.

## Negative / failure cases

- Calling Active/Standby HA a backup strategy.
- Restoring PG and assuming ClickHouse/Sensor spool are consistent without checks.
- Embedding raw secrets into backup tarballs checked into git or evidence bundles.
- Successful file restore with broken tenant isolation or failed app boot.
- Partial restore left online as healthy.
- Promising customers immediate purge from immutable backup media.
- Inventing a new backup product mid-task without catalogue/Decision Register.

## Security / tenant implications

Backup artefacts are sensitive: encrypt, access-control, retain tenant separation, redact when sampling for evidence. Restore drills use lab credentials. Support bundles from restore failures follow `secrets_handling`. Cross-tenant restore into shared lab DBs is forbidden without isolation.

## Required evidence

- Per-class backup mechanism identifiers (no invented tech)
- Drill commands, timestamps, RPO/RTO measurements
- Integrity/consistency verification queries
- Encryption/access-control notes
- Failed/partial restore outcome
- Sensor spool boundary statement when Agents/Sensors in scope
- Explicit `NOT_RUN` for classes not drilled
- Exact SHA; claim types via `evidence_bundle`

## Stop conditions

Stop when:

- Required backup technology is unspecified or would need a new unapproved dependency
- Restore claim is requested without a drill
- HA is being substituted for backup evidence
- Legal-hold/immutable retention contradicts a requested hard-delete promise

## Anti-patterns

- “We have standby, we’re fine”
- Untested cron jobs counted as restore capability
- Copying production backups onto laptops
- Disabling retention to shrink backup size without governance
- Writer self-certifying DR readiness

## Handoff / output

Return: class matrix; drill results; RPO/RTO; residual NOT_RUN; next SRE/DB verifier. Link `ha_dr` if failover was confused with restore.

## Non-ownership

Does not select new backup vendors, invent PITR mechanisms, authorize production restores, or certify MERGE_READY alone. Does not replace `migration_safety` or `ha_dr`.

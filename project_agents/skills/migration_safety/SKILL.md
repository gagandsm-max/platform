# migration_safety

## Purpose
Protect data and compatibility when a task touches schema, storage format, retention or migration behavior.

## Required evidence
- Forward and rollback/compatibility behavior. Rollback means assessed compatibility, expansion/contraction safety, or an explicit documented recovery path. Do not require automatic down migrations.
- Assessed retry and partial-application behavior. Document what happens if the migration is retried, interrupted, or applied more than once. This does not imply that every numbered Flyway migration must be directly rerunnable.
- Tenant-isolation and authorization impact.
- Existing-data/backfill handling.
- Failure behavior for partial application and restart.
- Migration reservation/ordering conflicts with parallel work.

## Guardrails
- Never reuse an occupied migration identifier without proof it is safe.
- Never make destructive data behavior the implicit rollback plan.
- Schema success alone is insufficient; verify data invariants and operational recovery.
- Do not invent a down-migration requirement that the governing schema process does not support.

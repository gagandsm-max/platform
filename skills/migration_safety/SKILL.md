# migration_safety

## Purpose
Protect data and compatibility when a task touches schema, storage format, retention or migration behavior.

## Required evidence
- Forward and rollback/compatibility behavior.
- Idempotency/retry behavior.
- Tenant-isolation and authorization impact.
- Existing-data/backfill handling.
- Failure behavior for partial application and restart.
- Migration reservation/ordering conflicts with parallel work.

## Guardrails
- Never reuse an occupied migration identifier without proof it is safe.
- Never make destructive data behavior the implicit rollback plan.
- Schema success alone is insufficient; verify data invariants and operational recovery.

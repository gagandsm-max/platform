# Change Plan approval binding

Exact binding required before mutating NCM execution:

1. Change Plan id + immutable plan version
2. Target set (entity ids) identical to approved set
3. Configuration payload / template version hash
4. Maintenance window id / bounds
5. Approval record (actor, timestamp, decision) under Platform RBAC—not ITSM text alone

## Critical invariant
AI-generated configuration or CLI does not constitute authorization. ITSM approval alone never authorizes unsafe execution when local Platform RBAC/policy/preconditions deny.

## Failure
If any binding element drifts (target added, payload changed, window expired), fail closed and require re-approval.

<!-- generated from project_agents/skills/backup_restore/SKILL.md sha256:a935a877ddb4a5b7f512ab51892c7c8510acf56d7fa20c1ea34bfa9f2859aa01 -->

---
name: backup_restore
description: Backup/restore, retention, cardinality and 80% storage-protection behavior including legal-hold limits.
---

# backup_restore

## Purpose
Protect recovery paths without promising impossible erasure.

## Use when
Backup jobs, restore drills, retention, or disk-protection.

## Required procedure
1. Distinguish active delete vs backup/legal hold.
2. Never promise immediate physical erasure from immutable backups.
3. Cardinality/retention must be class-specific.
4. Restore tests are required for restore claims.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `backup_restore` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.

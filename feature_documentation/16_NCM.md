# Network Configuration Management (NCM)

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 05, 09, 10, 18, 19, 23

## Product objective

Provide governed network configuration backup, comparison, compliance, change execution and rollback evidence without turning monitoring workers into unrestricted configuration engines.

## Core capabilities

- scheduled/on-demand configuration backup;
- version history and diff;
- compliance/policy evaluation;
- startup/running comparison where supported;
- search/export under RBAC;
- configuration change detection;
- approved command/action execution;
- rollback/fail-safe where the target supports it;
- incident/change/RCA evidence;
- ITSM linkage.

## Device/Capability Pack role

Capability Packs declare vendor/model/version-specific:

- transport/protocol;
- prompts/modes;
- backup commands/APIs;
- parser/normalization;
- config sections;
- compliance facts;
- safe diagnostic actions;
- privileged change actions;
- rollback/verification behavior.

No pack bypasses Credential Profiles, RBAC, tenant scope, audit or change policy.

## Change workflow

```text
Request / policy
 -> target + capability validation
 -> current config / precondition evidence
 -> diff / impact preview
 -> RBAC + approval/maker-checker where required
 -> execute bounded action
 -> collect result
 -> post-change verification
 -> health validation
 -> rollback/escalate if required
 -> audit / ITSM / incident timeline
```

## Security

Privileged operations require least-privilege credentials, resource scope, command/action allowlists, reason/ticket/approval according to policy, bounded timeouts and full audit. LLM output cannot directly execute commands.

## SNMP SET

Routine SNMP polling remains read-only. Any supported SNMP SET belongs here or in controlled remediation, with declared Pack action schema, authorization, pre-read and post-read verification.

## Automation

Automated remediation is allowed only for versioned deterministic actions with preconditions, blast-radius limits, authorization, audit, post-validation and rollback/fail-safe behavior.

## Monitoring relationship

NCM failure does not imply device failure. Configuration change is evidence for correlation/RCA; temporal proximity alone does not prove causality.

## UX

Configuration workspace should expose backups, versions, diff, compliance, pending/approved changes, execution status, rollback, related alerts/incidents and audit.

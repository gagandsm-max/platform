---
name: ncm_configuration_assurance
description: NCM configuration acquisition, history, diff, drift, compliance, Change Plan binding and pre/post/rollback evidence—without treating AI text as authorization.
---

<!-- generated from project_agents/skills/ncm_configuration_assurance/SKILL.md sha256:19dc0ddfd2455034a3a75fb0dbe8c9d0f3a0a0f65c14e7ccd6cf5e796b5a7552 -->

# ncm_configuration_assurance

## Purpose

Own configuration assurance evidence for network configuration management: acquisition, normalization, history/version evidence, running vs startup comparison, diff, golden config, drift, compliance, Change Plan binding, pre/post checks, rollback evidence and change/RCA correlation—while AI-generated configuration or CLI never constitutes authorization.

## Use when

NCM backup/diff/compliance, golden-config drift, Change Plan execution evidence, pre-change/post-change validation, rollback evidence packaging, or correlating configuration change with incident/RCA. Required for `platform-ncm-remediation-engineer`.

## Authoritative inputs

Always read:

1. Exact Change Plan / version / target / window / approval binding on the governing head
2. `references/config-acquisition-normalization.md`, `references/change-plan-approval-binding.md`, `references/pre-post-validation-rollback.md`
3. `remediation_safety` for privileged-action authorization, maker-checker, blast-radius and rollback authorization
4. `device_control_protocols` for protocol/transport safety only
5. Device Pack NCM capability claims (listed ≠ certified)

If semantic diff, automated rollback or compliance pack is not certified for the vendor/family/version, mark `UNVERIFIED`.

## Invariants

- Configuration acquisition is tenant-scoped, credential-profile bound and auditable.
- Normalization preserves vendor provenance; vendor IDs never override tenant/entity authority.
- History/version evidence is append-only for operator-visible lineage; deletes require governed retention rules.
- Running vs startup configuration are distinct evidence classes; do not collapse them.
- Diff/golden/drift/compliance claims require reproducible baselines and scope.
- Change Plan binding is exact: plan version + targets + configuration + window + approval.
- Pre-change and post-change checks are first-class evidence, not optional narrative.
- Rollback evidence is distinct from rollback authorization (`remediation_safety`).
- Change/RCA correlation is supporting evidence; temporal proximity is not causal proof.
- **AI-generated configuration or CLI does not constitute authorization.**
- Collector/transport failure ≠ unauthorized config change and ≠ device DOWN.

## Required procedure

1. Identify targets, credential profile and NCM capability certification state.
2. Acquire configuration through Platform-controlled paths; record source, timestamp and identity.
3. Normalize into Platform-owned configuration evidence without inventing semantic meaning.
4. Compare running vs startup / golden / prior version with explicit scope.
5. Bind any mutating work to an exact Change Plan + approval evidence.
6. Execute pre-change checks; fail closed when preconditions or evidence are insufficient.
7. After change, capture post-change configuration and validation evidence.
8. Package rollback evidence; never treat LLM text as approval to execute rollback.
9. Correlate change evidence into RCA/investigation as supporting context only.

## Negative / failure cases

- Applying CLI because an LLM drafted it.
- Claiming compliance when baseline/version is missing.
- Treating ITSM ticket text alone as Change Plan approval binding.
- Declaring drift without a golden/prior baseline.
- Collapsing pre-check failure into silent continue.
- Using device_control_protocols Skill as a substitute for Change Plan authorization.

## Security / tenant implications

Configuration content may contain secrets—redact before logs/support bundles. Tenant/RBAC/resource scope gates acquisition and mutation. Privileged apply remains under remediation_safety; this Skill does not grant privilege.

## Required evidence

- Acquisition/normalization provenance
- Diff/drift/compliance scope and baselines
- Change Plan/version/target/window/approval binding IDs
- Pre/post validation and rollback evidence references
- Exact SHA

## Stop conditions

Stop when:

- AI/CLI text is being used as authorization
- Change Plan binding is incomplete
- Required pre/post checks cannot run and mutation is still requested
- Vendor semantic-diff capability is uncertified but claimed universal
- Scope expands into protocol transport redesign

## Anti-patterns

- Free-form SSH apply outside Change Plan
- Golden config as marketing checkbox without reproducible baseline
- Writer self-certifying NCM safety

## Handoff / output

Return: acquisition/diff/compliance evidence; Change Plan binding; pre/post/rollback package; residual risks; nextOwner (`remediation_safety` / security / device-pack as applicable). Not MERGE_READY alone.

## Non-ownership

Does not own protocol/transport safety (`device_control_protocols`) or privileged-action authorization/maker-checker/blast-radius (`remediation_safety`). Does not invent firmware upgrade as Phase-1 universal capability.

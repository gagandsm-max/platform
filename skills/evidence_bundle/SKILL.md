<!-- generated from project_agents/skills/evidence_bundle/SKILL.md sha256:6db5ef8670c0c99c80b655ac3db989c8a362fb783a2ed78361428ff16879b602 -->

# evidence_bundle

## Purpose
Produce auditable evidence without overstating what was proved.

## Claim types
- `SOURCE_FACT`: directly observed repository/source/config fact. Requires `evidencePointer`.
- `EXECUTION_EVIDENCE`: result of an executed mutation or command. Requires `evidencePointer`.
- `CI_EVIDENCE`: result from CI tied to an exact SHA. Requires `evidencePointer`.
- `INFERENCE`: reasoned conclusion from facts/evidence. Cite supporting evidence where applicable.
- `UNVERIFIED`: required claim not yet proved. `UNVERIFIED` must never satisfy a required evidence gate and must not be silently promoted.

## Required bundle fields
Record task/head/base identity, selected agents/skills, commands or GitHub actions used, relevant tests, runtime/security evidence, blockers, and next owner/state. Keep raw identifiers/SHA/run references sufficient for reproduction. Prefer pointer-based references to bounded artifacts over embedding raw material.

## Prohibited bundle contents
Do not embed secrets, credentials, tokens, private keys, raw arbitrary logs, customer payloads, unredacted support bundles, or sensitive telemetry. Redact before capture. Evidence must remain bounded, tenant-scoped where applicable, and pointer-based when the raw artifact cannot be included safely.

## Guardrails
- Do not relabel source inspection as runtime evidence.
- Absence of a failure in one path does not prove global absence.
- Explicitly preserve `UNVERIFIED` items rather than filling gaps with assumptions.

# evidence_bundle

## Purpose
Produce auditable evidence without overstating what was proved.

## Claim types
- `SOURCE_FACT`: directly observed repository/source/config fact.
- `EXECUTION_EVIDENCE`: result of an executed mutation or command.
- `CI_EVIDENCE`: result from CI tied to an exact SHA.
- `INFERENCE`: reasoned conclusion from facts/evidence.
- `UNVERIFIED`: required claim not yet proved.

## Required bundle fields
Record task/head/base identity, selected agents/skills, commands or GitHub actions used, relevant tests, runtime/security evidence, blockers, and next owner/state. Keep raw identifiers/SHA/run references sufficient for reproduction.

## Guardrails
- Do not relabel source inspection as runtime evidence.
- Absence of a failure in one path does not prove global absence.
- Explicitly preserve `UNVERIFIED` items rather than filling gaps with assumptions.

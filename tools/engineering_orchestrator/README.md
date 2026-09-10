# Engineering Orchestrator — Phase 1

Deterministic, network-free Python foundation for Platform engineering task
state, evidence, authority, and an append-only SHA-256 journal.

**Hash-chain journaling provides tamper detection only.** It is **not** a
signature scheme and does **not** provide non-repudiation.

**Actors are LOGICAL / UNATTESTED in Phase 1.** `INDEPENDENT_REVIEWER` and other
actor labels are workflow roles only — not authenticated worker identities.
Cryptographic worker identity and trusted evidence ingestion are Phase 2.

**Hash/state integrity ≠ evidence attestation.** Phase 1 evidence is always
`UNATTESTED`. There is no `TRUSTED` attestation value in Phase 1.

## Scope (Phase 1)

Included:

- Explicit FSM with authority-separated transitions + expected-state CAS
- Dedicated `reconcile_base()` for `FULL_VALIDATION` → `BASE_RECONCILIATION`
- Failure classification + bounded transient retry (`MAX=2`)
- Evidence model with provenance, RequiredGates, path/dependency invalidation
- Path/lane ownership checks
- Migration **reservation model only** (permission-gated; no migration files)
- Append-only journal with replay/validate (reconcile-only HEAD/base changes)
- Single atomic `bundle.json` store with generation CAS (one writer process)
- Structured manual handoffs (`codex_enabled` default **false**)
- Founder status projection, readiness vs 100%-pass
- Network-free CLI (`argparse`)
- JSON schemas under `schemas/` + stdlib `validation.py`
- Standalone deterministic unittest suite (wired into Platform CI Unit job)

## Not included (Phase 2+)

- Authenticated actor identities / cryptographic evidence signing
- GitHub API writes / remote locks / tags / autonomous merge
- AI tool invocation / Codex worker execution
- Distributed locking / consensus
- Language-level dependency graphs
- Trusted CI attestation

## Authority (critical)

| Transition | Allowed actor |
|---|---|
| Pre-review through `FULL_VALIDATION` | `WRITER` / `SYSTEM` |
| `FULL_VALIDATION` → `BASE_RECONCILIATION` | `SYSTEM` via `reconcile_base` only |
| `BASE_RECONCILIATION` → `INDEPENDENT_REVIEW` | `SYSTEM` |
| `INDEPENDENT_REVIEW` → `EXACT_HEAD_CI` | `INDEPENDENT_REVIEWER` only |
| `EXACT_HEAD_CI` → `REALITY_GATE` | `SYSTEM` only |
| `REALITY_GATE` → `READY_FOR_MERGE_GATE` | `REALITY_CHECKER` only |
| `READY_FOR_MERGE_GATE` → `MERGED` | `MERGE_AUTHORITY` only |
| `BUSINESS_DECISION_REQUIRED` resume | `FOUNDER_CTO` only |

Writer cannot self-certify readiness. CI alone never promotes readiness.
100%-pass ≠ readiness. Empty applicable RequiredGate set ≠ 100% PASS.

## Store

Single `bundle.json` (`schema_version`, `generation`, `task`, `journal`,
`evidence`, `lanes`, `migrations`, `handoffs`). Mutations use temp + `os.replace`
with `expected_generation` CAS. **Phase 1 assumes one writer process.**

## CLI

```bash
python3 -m tools.engineering_orchestrator --help

python3 -m tools.engineering_orchestrator init-task \
  --store /tmp/eo-store --task-id T1 --title "demo" --head-sha abc --base-sha base

python3 -m tools.engineering_orchestrator transition \
  --store /tmp/eo-store --to-state SOURCE_INSPECTION \
  --expected-state TASK_ACCEPTED --actor WRITER

python3 -m tools.engineering_orchestrator reconcile-base \
  --store /tmp/eo-store --expected-state FULL_VALIDATION \
  --expected-head-sha abc --expected-base-sha base \
  --new-head-sha def --new-base-sha base2 \
  --changed-path tools/engineering_orchestrator/engine.py \
  --verification-evidence-id src1 --impact-known

python3 -m tools.engineering_orchestrator record-evidence \
  --store /tmp/eo-store --evidence-id e1 --type LOCAL_TEST \
  --head-sha abc --claim-scope default --result PASS \
  --collected-at 2026-01-01T00:00:00Z --command-identity "unittest"
```

Subcommands: `init-task`, `transition`, `reconcile-base`, `record-evidence`,
`status`, `replay`, `validate-journal`, `handoff-create`.

## Tests

```bash
python3 tests/engineering_orchestrator/run_tests.py
```

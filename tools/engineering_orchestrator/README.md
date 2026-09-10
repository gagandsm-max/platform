# Engineering Orchestrator — Phase 1

Deterministic, network-free Python foundation for Platform engineering task
state, evidence, authority, and an append-only SHA-256 journal.

**Hash-chain journaling provides tamper detection only.** It is **not** a
signature scheme and does **not** provide non-repudiation.

## Scope (Phase 1)

Included:

- Explicit FSM with authority-separated transitions
- Failure classification + bounded transient retry (`MAX=2`)
- Evidence model with forbidden promotions and HEAD invalidation
- Path/lane ownership checks
- Migration **reservation model only** (JSON/in-memory; no migration files)
- Append-only journal with replay/validate
- Structured manual handoffs (`codex_enabled` default **false**)
- Founder status projection, readiness vs 100%-pass
- Network-free CLI (`argparse`)
- JSON schemas under `schemas/`
- Standalone deterministic unittest suite

## Not included (Phase 2+)

- GitHub API writes / remote locks / tags
- AI tool invocation / Codex worker execution
- Subprocess worker dispatch
- Autonomous source writes or autonomous merge
- External services
- Wiring these tests into Platform CI / Makefile shared runners

## Authority (critical)

| Transition | Allowed actor |
|---|---|
| `REALITY_GATE` → `READY_FOR_MERGE_GATE` | `REALITY_CHECKER` only |
| `READY_FOR_MERGE_GATE` → `MERGED` | `MERGE_AUTHORITY` only |
| Writer → `READY_FOR_MERGE_GATE` / `MERGED` | **rejected** |

Writer cannot self-certify readiness. CI alone never promotes readiness.
100%-pass ≠ readiness.

## CLI

From the repository root (worktree):

```bash
python3 -m tools.engineering_orchestrator --help

python3 -m tools.engineering_orchestrator init-task \
  --store /tmp/eo-store --task-id T1 --title "demo" --head-sha abc

python3 -m tools.engineering_orchestrator transition \
  --store /tmp/eo-store --to-state SOURCE_INSPECTION --actor WRITER

python3 -m tools.engineering_orchestrator record-evidence \
  --store /tmp/eo-store --evidence-id e1 --type LOCAL_TEST \
  --head-sha abc --claim-scope default --result PASS --collected-at 2026-01-01T00:00:00Z

python3 -m tools.engineering_orchestrator status --store /tmp/eo-store
python3 -m tools.engineering_orchestrator validate-journal --store /tmp/eo-store
python3 -m tools.engineering_orchestrator replay --store /tmp/eo-store
python3 -m tools.engineering_orchestrator handoff-create \
  --store /tmp/eo-store --handoff-id h1 --actor-role writer \
  --expected-return-state IMPLEMENTATION
```

Subcommands: `init-task`, `transition`, `record-evidence`, `status`, `replay`,
`validate-journal`, `handoff-create`.

## Tests

Standalone runner (no pytest required):

```bash
python3 tests/engineering_orchestrator/run_tests.py
```

Or unittest discovery from repo root:

```bash
python3 -m unittest discover -s tests/engineering_orchestrator -v
```

## CI coverage gap (Phase 2 follow-up)

These tests are **not** wired into Platform CI (`.github/workflows/**`) or the
root `Makefile` / shared `tests/run-*.sh` runners in Phase 1. Integrating them
would require shared-runner changes that are explicitly out of scope.

Until Phase 2 wires discovery, treat local `run_tests.py` as the authoritative
Phase-1 validation path for this package.

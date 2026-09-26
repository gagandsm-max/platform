---
name: python_runtime
description: Deterministic Python for Platform governance/tooling—safe I/O, exit codes, no hidden network or product runtime dependency.
---

# python_runtime

## Purpose

Write deterministic Python for Platform governance, validators and tooling: safe filesystem/subprocess use, stable exit codes and reproducible output—without introducing Python as a product runtime dependency unless architecture explicitly requires it.

## Use when

Changing `project_agents/tools`, validators, report generators, orchestrator helpers, or other approved Python utilities. Not for inventing Core business logic in Python.

## Authoritative inputs

Always read:

1. Existing tool entrypoints and test runners on the exact head
2. Mother rule: routine product runtimes are Go/Java/TS unless approved
3. `sbom_provenance` before adding Python packages
4. Task path scope—do not mutate outside assignment
5. Encoding/locale expectations for deterministic reports

## Invariants

- Governance/tooling use by default; no silent product-runtime Python service.
- Scripts are deterministic given inputs; no hidden network dependency.
- Subprocess calls use argument lists (no shell=True) unless justified and audited.
- Filesystem/path safety: no traversal outside allowed roots.
- JSON/YAML parsing fail-closed on corrupt input.
- Process exit codes distinguish OK vs validation failure vs internal error.
- Output reproducible for `--check` gates (stable ordering).
- Dependency avoidance; new deps need catalogue review.
- Test isolation; temp files cleaned; explicit encodings (UTF-8).
- Secrets never logged; no mutation outside task scope.
- No Python product dependency unless explicitly architectural and approved.

## Required procedure

1. Confirm the change is tooling/governance (or approved exception).
2. Avoid network calls; if required for a tool, document and gate offline tests.
3. Use pathlib-safe joins; validate user-supplied paths.
4. Parse JSON/YAML with clear errors; stable serialization for reports.
5. Return non-zero on validation failure; print actionable stderr.
6. Add unit tests with temp dirs; no reliance on developer laptop packages beyond stdlib/approved.
7. Do not add Flask/Django app servers “for convenience.”

## Negative / failure cases

- `curl | python` install patterns in docs for Platform tools.
- Writing outside the repo/worktree silently.
- Non-deterministic dict iteration breaking `--check`.
- Embedding tokens in exception strings.
- Shipping a Python sidecar as Core without architecture approval.

## Security / tenant implications

Tools may read manifests—treat as trusted repo content only. Don’t execute skill body as code. Subprocess to docker/git must not pass unsanitized input to shell.

## Required evidence

- Test results for the tool
- Determinism/`--check` proof when applicable
- Dependency justification or “stdlib only”
- Exact SHA

## Stop conditions

Stop when:

- Hidden network required for unit validation
- Path traversal possible
- Product Python runtime introduced without authority
- Non-deterministic gate output

## Anti-patterns

- `os.system`
- Mutable global caches across tests without reset
- Writer self-certifying tooling correctness without tests

## Handoff / output

Return: tool behaviour; tests; risks; nextOwner (governance verifier). Not MERGE_READY alone.

## Non-ownership

Does not own Go/Java product runtimes or invent AI Python services without catalogue/architecture. Owns Python tooling discipline.

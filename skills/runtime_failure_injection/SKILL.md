# runtime_failure_injection

## Purpose
Turn source-level failure assumptions into runtime evidence.

## Required procedure
1. Identify the durability/retry/timeout/crash invariant.
2. Inject the narrowest failure at the boundary that can violate it.
3. Verify externally observable state after failure and restart/retry.
4. Prove acknowledgements/deletions/offsets occur only after the required durable boundary.
5. Record environment and commands so evidence is reproducible.

## Guardrails
- Source inspection is `SOURCE_FACT`, not runtime evidence.
- Do not use destructive generic `docker prune` or `docker compose down -v` as a test shortcut.
- Do not claim local runtime execution from ChatGPT+GitHub when it was not executed locally.

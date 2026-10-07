---
name: go_runtime
description: Production Go service conventions for Sensor/Agent—context, goroutines, durability, TLS clients and race-safe tests.
---

<!-- generated from project_agents/skills/go_runtime/SKILL.md sha256:7ca1aad40654b474acb9df5324d8eab50d269d96b4347e493b72021f19591a02 -->

# go_runtime

## Purpose

Write and review production Go for Sensor/Agent (and other approved Go services) with context cancellation, goroutine ownership, bounded concurrency, durable I/O where required and race-detector-clean tests.

## Use when

Go service changes, worker pools, spool/uploader, HTTP/gRPC clients, JSON codecs, or flaky race/timeout defects. Required for Sensor/Agent Go paths.

## Authoritative inputs

Always read:

1. Existing Go module layouts and `go test`/`-race` conventions on the exact head
2. Mother packaging notes for Sensor/Agent
3. `sensor_edge`, `replay_idempotency`, `pki_mtls` when transport/spool touched
4. Error-code / structured logging conventions in-tree
5. Task stopConditions for unsupported OS APIs
6. `references/context-concurrency-race.md` and `evals/cases.json` for commercial-depth BUILD/TEST/DEBUG checks
7. Remember: microbenchmark evidence is not production capacity certification (`capacity_performance`)

## Invariants

- `context.Context` propagates cancel/deadline; no orphan work after cancel.
- Goroutine ownership is clear; every goroutine has shutdown path.
- Channel lifecycle: senders/receivers documented; no send-on-closed surprises.
- Bounded concurrency (semaphores/worker pools)—no unbounded fan-out.
- Timeouts on I/O and RPCs; retry loops bounded and jittered.
- Panic containment at process boundaries; unexpected panics become supervised failure, not silent continue.
- Resource cleanup via `defer`/Close; file durability/`fsync` where durability is claimed.
- Strict JSON (unknown fields policy documented); HTTP/TLS clients verify certificates.
- Error wrapping/classification stable for operators; no secret material in errors/logs.
- Race testing (`go test -race`) on concurrency-sensitive packages.
- Deterministic tests; no leaked goroutines in tests.
- No global mutable state without justification and synchronization.

## Required procedure

1. Identify concurrency and I/O surfaces changed.
2. Thread context through calls; add timeouts.
3. Bound worker pools; document channel ownership.
4. Ensure Close/fsync paths for durable spool claims.
5. Classify errors; redact secrets.
6. Run unit tests with `-race` where applicable; fix leaks.
7. Avoid init-time global mutation; prefer explicit constructors.

## Negative / failure cases

- Goroutine leak on request cancel.
- Retry loop without context or max attempts.
- `json.Unmarshal` into maps losing type safety on contracts.
- Logging bearer tokens / key PEMs.
- Data race on shared map without mutex.

## Security / tenant implications

TLS verification stays on. mTLS material never logged. Tenant IDs in requests validated server-side even for Go workers. Path traversal checks on filesystem spool roots.

## Required evidence

- `go test` / `-race` results for touched packages
- Timeout/concurrency notes
- Durability/`fsync` evidence when claimed
- Exact SHA

## Stop conditions

Stop when:

- Unbounded concurrency introduced
- TLS verify disabled
- Panic swallowed without supervised recovery path
- Race detector failures ignored

## Anti-patterns

- `time.Sleep` as synchronization
- Fire-and-forget goroutines
- Writer self-certifying Go concurrency safety

## Handoff / output

Return: concurrency model; test evidence; residual risks; nextOwner (Sensor/Agent verifier). Not MERGE_READY alone.

## Non-ownership

Does not own Java Core, invent HA topology, or replace protocol Skills. Owns Go runtime discipline.

---
name: capacity_performance
description: Hot-path capacity, latency, lag, cardinality and resource-budget evidence for Core, Sensor, UI and storage protection.
---

# capacity_performance

## Purpose

Produce honest capacity and performance evidence for Platform hot paths—telemetry rate, cardinality, retention, queues and storage protection—without extrapolating from trivial lab runs without stated assumptions.

## Use when

Sizing claims, performance regressions, Kafka/DB/CH bottlenecks, dashboard concurrency, polling interval changes, retention growth, 80% Sensor storage protection behaviour, or release headroom statements. Required for `platform-performance-benchmarker` and capacity-affecting BUILD work.

## Authoritative inputs

Always read:

1. `Platform_11_Sizing_Capacity_Storage_Master.md` and Architecture capacity notes
2. 80% Sensor/Core storage-protection rules in `AGENTS.md`
3. Current polling intervals, device/sensor/customer counts in the scenario
4. Kafka/PG/CH resource configs for the environment under test
5. Benchmark harness definitions and exact SHA
6. Prior measurement baselines (if any) with their assumptions
7. `references/hot-path-budgets.md` and fixtures/evals for polling/ingestion/cardinality/lag and engineering-limit≠SKU

## Invariants

- Cover telemetry rate, device/sensor/customer counts, cardinality, retention and polling intervals as first-class inputs.
- Kafka throughput, DB/ClickHouse IOPS/storage, and dashboard concurrency are measured or explicitly `UNVERIFIED`.
- Queue/backlog and retry storms are part of saturation analysis.
- **80% Sensor storage protection** behaviour must be evidenced when spool/capacity is in scope.
- Identify saturation points and remaining headroom; do not claim infinite scale.
- Benchmark environment is named (hardware class, container limits, noisy neighbours).
- Report **p50/p95/p99** (or explicit alternative) where latency matters.
- Bottleneck identification names the limiting resource (CPU, disk, lock, network, cardinality).
- No extrapolation from trivial lab runs without explicit scaling assumptions and uncertainty.
- Capacity claims are not product marketing savings percentages (`customer_value_mttr` governs value math).

## Required procedure

1. Define the workload model: entities, sensors, poll intervals, telemetry rate, cardinality dimensions, retention.
2. State environment: CPU/RAM/disk/network, compose vs bare, contending processes.
3. Choose measurements: throughput, lag, latency percentiles, error rate, spool fill, PG/CH/Kafka metrics.
4. Run baseline vs candidate on the exact SHA; avoid mixing configs mid-run.
5. Induce or observe backlog/retry storm behaviour; record protection triggers (80%).
6. Locate bottleneck with evidence (profiles, wait events, consumer lag charts).
7. Compute headroom vs target load; if extrapolating, write assumptions and confidence bounds.
8. Reject claims that ignore retention storage growth or dashboard concurrency.
9. Package results with raw command outputs and SHA; mark gaps `UNVERIFIED`/`NOT_RUN`.

## Negative / failure cases

- “100 devices worked → 100k devices fine”.
- Ignoring ClickHouse cardinality blow-ups.
- Declaring success while Sensor spool is in protection/degraded.
- Publishing average latency only when p99 is terrible.
- Load tests against a shared noisy host without disclosure.
- Treating Kafka consumer catch-up as free.

## Security / tenant implications

Load generators must use synthetic tenants—not production data. Do not disable auth to “go faster” in benchmarks that claim production fidelity. Multi-tenant noisy-neighbour effects should be called out when relevant. Evidence bundles redact secrets.

## Required evidence

- Workload model + assumptions
- Environment description
- Exact SHA + commands
- Throughput/lag/latency percentiles
- Storage protection observations when applicable
- Bottleneck statement
- Headroom / risk conclusion
- Explicit extrapolation limits

## Stop conditions

Stop certification of capacity claims when:

- Environment is undefined
- Only trivial N was tested but large-N is claimed without assumptions
- Protection/backpressure paths were not observed though load should trigger them
- Bottleneck is unknown yet “ready for GA scale” is requested

## Anti-patterns

- Microbench as system proof
- Turning off 80% protection for prettier charts
- Single-run anecdotes
- Writer self-certifying performance readiness
- Confusing UI snappiness on empty DB with production concurrency

## Handoff / output

Return: model; measurements; bottleneck; headroom; assumptions; nextOwner (SRE/verifier). Capacity PASS is not MERGE_READY alone.

## Non-ownership

Does not set contractual SLAs, invent HA topology, or replace domain correctness Skills. Owns measurement honesty for capacity/performance claims.

# Platform P0 Engineering Spike: Two-Core Stateful HA Lab

This directory contains the reproducible Docker-based experimental lab and evidence for evaluating Platform's **EXACTLY-TWO-CORE** Stateful HA architecture.

## Contents

- `compose.ha-spike.yaml`: Isolated Docker Compose stack (Kafka A/B, ClickHouse A/B, PostgreSQL A/B, HA Harness).
- `harness/`:
  - `docker_client.py`: Python Docker API client over `/var/run/docker.sock`.
  - `ha_state_machine.py`: Finite state machine enforcing transitions and no-split-brain invariant.
  - `kafka_harness.py`: Kafka dual-write, failure drills, and failover timing runner.
  - `clickhouse_harness.py`: ClickHouse dual-insert, deduplication, DDL consistency, and failover timing runner.
  - `run_spike_tests.py`: Master test runner executing all 12 failure scenarios and timing benchmarks.
- `evidence/`:
  - `raw_metrics.json`: Complete serialized metrics from all test runs.
  - `kafka_timings.csv`: Timing breakdown across 10 iterations of Kafka failover.
  - `clickhouse_timings.csv`: Timing breakdown across 10 iterations of ClickHouse failover.
- `results.md`: Detailed test results and analysis.
- `failure-matrix.md`: 12-scenario failure matrix.
- `architecture-candidate.md`: Concrete Active/Standby candidate specification.

## Execution

The entire spike runs inside isolated Docker containers:

```bash
docker compose -p platform-ha-spike -f engineering/spikes/two-core-ha/compose.ha-spike.yaml up -d
docker build -t platform-ha-harness engineering/spikes/two-core-ha/harness
docker run --rm --network ha_spike_net -v /var/run/docker.sock:/var/run/docker.sock -v $(pwd):/workspace platform-ha-harness
docker compose -p platform-ha-spike -f engineering/spikes/two-core-ha/compose.ha-spike.yaml down -v
```

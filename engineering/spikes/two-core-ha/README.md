# Platform P0 Engineering Spike: Two-Core Stateful HA Lab

Corrected evidence spike. Distinguishes **MEASURED / EXECUTED / MODELLED / INFERRED / PENDING PRODUCTION VALIDATION**.

Evidence SHA for last documented run: `96f93345a3a21701f140e889dbd9655e0184b5b8`

## Contents

- `compose.ha-spike.yaml` — isolated lab (Kafka A/B, ClickHouse A/B, PostgreSQL A/B present but PG HA **NOT TESTED**)
- `harness/` — FSM model, Kafka/ClickHouse harnesses, runner
- `evidence/` — `raw_metrics.json`, timing CSVs
- `results.md`, `failure-matrix.md`, `architecture-candidate.md`

## Execution (Docker only)

```bash
cd /path/to/repo
docker compose -p platform-ha-spike -f engineering/spikes/two-core-ha/compose.ha-spike.yaml up -d --wait || \
  docker compose -p platform-ha-spike -f engineering/spikes/two-core-ha/compose.ha-spike.yaml up -d
# wait for healthy if --wait unsupported
docker build -t platform-ha-harness engineering/spikes/two-core-ha/harness
docker run --rm --network ha_spike_net \
  -e SPIKE_GIT_COMMIT=$(git rev-parse HEAD) \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$(pwd)":/workspace \
  platform-ha-harness
docker compose -p platform-ha-spike -f engineering/spikes/two-core-ha/compose.ha-spike.yaml down -v
```

## What this proves / does not prove

See `results.md` and `failure-matrix.md`. Do not treat lab timings as certified Core HA &lt;60s.

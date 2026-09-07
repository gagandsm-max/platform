# Platform P0 Spike: Two-Core Stateful HA Experimental Results (Corrected)

**Document ID:** SPIKE-P0-EVIDENCE-001  
**Evidence SHA (this run):** `96f93345a3a21701f140e889dbd9655e0184b5b8`  
**Mother Reference Version:** `v3.2.1`  

Legend: **MEASURED** | **EXECUTED** | **MODELLED** | **INFERRED** | **PENDING PRODUCTION VALIDATION**

---

## 1. Executive Summary

This Docker lab spike evaluates candidate patterns for EXACTLY-TWO-CORE HA. It does **not** certify production Core HA &lt;60s.

### Corrected findings

| Topic | Classification | Finding |
|---|---|---|
| Two-voter KRaft/Keeper | ANALYTICALLY REJECTED | Lab uses two independent 1-voter clusters; shared 2-voter quorum was **not** instantiated |
| Kafka dual-write ACK | EXECUTED | STRICT_DUAL only; degraded sequence advance **REJECTED** |
| Kafka consumer dedup | NOT PROVEN | Broker stores duplicates on replay |
| Kafka failover timing | MEASURED (lab lower bound) | Median **0.4779s** with consume-verified T4; **not** certified Core HA |
| ClickHouse FINAL idempotence | EXECUTED | Query-time idempotent under FINAL for identical tested ORDER BY key; not complete physical dedup |
| ClickHouse event identity | EXECUTED | Different `batch_id` ⇒ duplicate FINAL rows; stable event identity **REQUIRED** |
| ClickHouse divergence/resync | EXECUTED (spike helper) | Lab helper converged; production resync still **PENDING** |
| ClickHouse DDL partial fail | EXECUTED | `declared_ready=false` while B down; restore + reconcile → schemas match |
| FSM / epoch | MODEL-ONLY / EXECUTED model | Exact epoch match; future epochs rejected; not production proof |
| PostgreSQL P0-03 | NOT TESTED | Unresolved |
| Resources | MEASURED (incomplete) | INCOMPLETE — idle/lab container samples only; do NOT claim comfortably within 8 vCPU/16GB Core budget from this evidence alone |
| Ops complexity (install/DR/support) | INFERRED | Engineering estimates only — **not proven** by this spike |

---

## 2. Kafka Failover Timings (Part 5) — LAB LOWER BOUND

Scope: `LAB_SERVICE_SWITCH_LOWER_BOUND` / fencing `LAB_FENCING_ANALOGUE`  
`not_certified_core_ha_60s=True`  
T4 requires exact `batch_id` consume: `True`  
All batches consumed: `True`

| Iteration | Lab fence (s) | Promote model (s) | Ingest (s) | Consume verify (s) | Total (s) |
|---|---|---|---|---|---|
| 1 | 0.2117 | 0.0000 | 0.0099 | 0.1696 | 0.3912 |
| 2 | 0.2492 | 0.0000 | 0.0084 | 0.2072 | 0.4648 |
| 3 | 0.2842 | 0.0000 | 0.0069 | 0.1730 | 0.4641 |
| 4 | 0.3064 | 0.0000 | 0.0166 | 0.2377 | 0.5608 |
| 5 | 0.3024 | 0.0000 | 0.0110 | 0.1927 | 0.5061 |
| 6 | 0.2291 | 0.0000 | 0.0069 | 0.1752 | 0.4111 |
| 7 | 0.2929 | 0.0000 | 0.0137 | 0.1844 | 0.4910 |
| 8 | 0.2487 | 0.0000 | 0.0065 | 0.1639 | 0.4192 |
| 9 | 0.3340 | 0.0000 | 0.0104 | 0.1940 | 0.5384 |
| 10 | 0.3751 | 0.0000 | 0.0166 | 0.2419 | 0.6337 |

- Min/Median/P95/Max total (s): 0.3912 / 0.4779 / 0.6337 / 0.6337
- Dual-write latency (MEASURED): mean 8.64 ms, p95 11.32 ms

Excluded from timing: heartbeat, production fencing API, power confirmation, VIP/DNS, Core promotion, TLS readiness, Sensor reconnect/ACK, consumer checkpoint, customer latency.

---

## 3. ClickHouse Failover Timings (Part 10) — LAB LOWER BOUND

| Iteration | Lab fence (s) | Promote model (s) | Ingest (s) | Total (s) |
|---|---|---|---|---|
| 1 | 0.3069 | 0.0000 | 0.0089 | 0.3325 |
| 2 | 0.2110 | 0.0000 | 0.0078 | 0.2294 |
| 3 | 0.2377 | 0.0001 | 0.0076 | 0.2540 |
| 4 | 0.2189 | 0.0000 | 0.0081 | 0.2873 |
| 5 | 0.2148 | 0.0000 | 0.0090 | 0.2338 |
| 6 | 0.3057 | 0.0000 | 0.0135 | 0.3416 |
| 7 | 0.1972 | 0.0000 | 0.0067 | 0.2129 |
| 8 | 0.3114 | 0.0000 | 0.0093 | 0.3338 |
| 9 | 0.2086 | 0.0000 | 0.0064 | 0.2215 |
| 10 | 0.2153 | 0.0000 | 0.0074 | 0.2313 |

- Min/Median/P95/Max total (s): 0.2129 / 0.2439 / 0.3416 / 0.3416

Dedup semantics: query-time idempotent result for identical tested ORDER BY key under FINAL; NOT complete physical duplicate elimination; production FINAL affordability UNPROVEN

DDL: healthy ready=True; partial recovery success=True

---

## 4. FSM / Epoch

- Classification: `MODEL-ONLY` / claim_level=`model_invariant_test`
- `not_mathematical_proof=True`
- Epoch exact-match: `{'current_accepted': True, 'stale_rejected': True, 'future_rejected': True, 'evidence': 'EXECUTED'}`

---

## 5. Resources

{
  "evidence": "MEASURED",
  "containers": {
    "ha-kafka-a": {
      "memory_usage_mb": 329.65,
      "memory_limit_mb": 31861.98,
      "cpu_percent_sample": 1.6,
      "network": {
        "rx_bytes": 9451,
        "tx_bytes": 3351
      },
      "blkio": {
        "read_bytes": 13926400,
        "write_bytes": 3727360
      }
    },
    "ha-kafka-b": {
      "memory_usage_mb": 321.81,
      "memory_limit_mb": 31861.98,
      "cpu_percent_sample": 1.31,
      "network": {
        "rx_bytes": 123071,
        "tx_bytes": 261155
      },
      "blkio": {
        "read_bytes": 86016,
        "write_bytes": 8237056
      }
    },
    "ha-clickhouse-a": {
      "memory_usage_mb": 246.38,
      "memory_limit_mb": 31861.98,
      "cpu_percent_sample": 4.73,
      "network": {
        "rx_bytes": 5333,
        "tx_bytes": 3034
      },
      "blkio": {
        "read_bytes": 0,
        "write_bytes": 0
      }
    },
    "ha-clickhouse-b": {
      "memory_usage_mb": 296.58,
      "memory_limit_mb": 31861.98,
      "cpu_percent_sample": 4.31,
      "network": {
        "rx_bytes": 35166,
        "tx_bytes": 29526
      },
      "blkio": {
        "read_bytes": 7847936,
        "write_bytes": 2801664
      }
    },
    "ha-postgres-a": {
      "memory_usage_mb": 65.59,
      "memory_limit_mb": 31861.98,
      "cpu_percent_sample": 0.0,
      "network": {
        "rx_bytes": 17823,
        "tx_bytes": 126
      },
      "blkio": {
        "read_bytes": 3407872,
        "write_bytes": 53489664
      }
    },
    "ha-postgres-b": {
      "memory_usage_mb": 63.86,
      "memory_limit_mb": 31861.98,
      "cpu_percent_sample": 0.0,
      "network": {
        "rx_bytes": 17378,
        "tx_bytes": 126
      },
      "blkio": {
        "read_bytes": 36864,
        "write_bytes": 53489664
      }
    }
  },
  "core_a_total_mem_mb": 641.62,
  "core_b_total_mem_mb": 682.25,
  "dimensions": {
    "cpu": "single Docker stats sample (not load-tested)",
    "memory": "container RSS-ish usage from Docker stats",
    "disk": "blkio counters best-effort; may be zero/incomplete under cgroup v2",
    "network": "cumulative rx/tx bytes counters (not failover overhead delta)"
  },
  "profile_evaluation": "INCOMPLETE \u2014 idle/lab container samples only; do NOT claim comfortably within 8 vCPU/16GB Core budget from this evidence alone",
  "missing_or_weak": []
}

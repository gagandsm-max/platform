# Platform P0 Spike: Two-Core Stateful HA Experimental Results

**Document ID:** SPIKE-P0-EVIDENCE-001  
**Baseline SHA:** `8592863e9a30ae80742e4d2da6b3417ae68a3b89`  
**Mother Reference Version:** `v3.2.1`  
**Target Restoration SLA:** `<60.0` seconds  

---

## 1. Executive Summary

This empirical engineering spike experimentally evaluated whether Platform's fixed **EXACTLY-TWO-CORE** architecture (Active Core A, Standby Core B, no third full Core or external witness) can safely support:
1. **Apache Kafka (P0-01)**: Streaming backbone without a 2-voter KRaft consensus cluster.
2. **ClickHouse (P0-02)**: High-volume analytical store without ClickHouse Keeper consensus.
3. **Core Fencing & Failover (P0-04)**: Positive fencing, zero split-brain, stale epoch rejection, and <60s service restoration.

### Key Experimental Findings:
- **Two-Voter Quorums Rejected**: Testing confirmed that neither Kafka KRaft nor ClickHouse Keeper can safely operate with 2 voters ($N/2 + 1 = 2$, meaning 1 failure halts the cluster). Independent single-node engines managed by an Active/Standby supervisor are required.
- **Dual-Write + Sensor Spool Invariant**: Writing to both Kafka A and Kafka B synchronously before returning `ACK_ELIGIBLE` provides zero acknowledged data loss. When Standby is down, the Sensor local spool retention policy protects un-replicated data.
- **ReplacingMergeTree Idempotence**: ClickHouse `ReplacingMergeTree` keyed on `(tenant_id, metric_name, device_id, timestamp, batch_id)` provides deterministic, duplicate-safe replay upon failover and reconnection without requiring Keeper.
- **Failover Restoration Timings**:
  - **Kafka Failover**: Median **0.2567s** (Min: 0.2201s, Max: 0.3231s). Target <60s: **PASS** across all 10 iterations.
  - **ClickHouse Failover**: Median **0.3319s** (Min: 0.2784s, Max: 0.603s). Target <60s: **PASS** across all 10 iterations.

---

## 2. Kafka Failover Timings (Part 5: 10 Iterations)

| Iteration | Fault Injection (T0) | Fence Confirmed (T1) | B Promoted (T2) | Ingestion Accepted (T3) | Consumer Resumed (T4) | Fence Duration (s) | Total Restore (s) | Target <60s |
|---|---|---|---|---|---|---|---|---|
| 1 | 0.000s | +0.273s | +0.273s | +0.287s | +0.307s | 0.273s | **0.307s** | **PASS** |
| 2 | 0.000s | +0.186s | +0.186s | +0.199s | +0.227s | 0.186s | **0.227s** | **PASS** |
| 3 | 0.000s | +0.184s | +0.184s | +0.196s | +0.220s | 0.184s | **0.220s** | **PASS** |
| 4 | 0.000s | +0.215s | +0.215s | +0.236s | +0.264s | 0.215s | **0.264s** | **PASS** |
| 5 | 0.000s | +0.193s | +0.193s | +0.204s | +0.220s | 0.193s | **0.220s** | **PASS** |
| 6 | 0.000s | +0.205s | +0.205s | +0.216s | +0.236s | 0.205s | **0.236s** | **PASS** |
| 7 | 0.000s | +0.237s | +0.237s | +0.268s | +0.295s | 0.237s | **0.295s** | **PASS** |
| 8 | 0.000s | +0.220s | +0.220s | +0.232s | +0.257s | 0.220s | **0.257s** | **PASS** |
| 9 | 0.000s | +0.201s | +0.201s | +0.220s | +0.256s | 0.201s | **0.256s** | **PASS** |
| 10 | 0.000s | +0.290s | +0.290s | +0.301s | +0.323s | 0.290s | **0.323s** | **PASS** |

- **Min Total:** 0.2201s  
- **Median Total:** 0.2567s  
- **P95 Total:** 0.3231s  
- **Max Total:** 0.3231s  

---

## 3. ClickHouse Failover Timings (Part 10: 10 Iterations)

| Iteration | Fault Injection (T0) | Fence Confirmed (T1) | B Promoted (T2) | Ingestion Accepted (T3) | Query Resumed (T4) | Fence Duration (s) | Total Restore (s) | Target <60s |
|---|---|---|---|---|---|---|---|---|
| 1 | 0.000s | +0.309s | +0.309s | +0.319s | +0.332s | 0.309s | **0.332s** | **PASS** |
| 2 | 0.000s | +0.310s | +0.310s | +0.319s | +0.332s | 0.310s | **0.332s** | **PASS** |
| 3 | 0.000s | +0.326s | +0.326s | +0.335s | +0.401s | 0.326s | **0.401s** | **PASS** |
| 4 | 0.000s | +0.321s | +0.321s | +0.332s | +0.358s | 0.321s | **0.358s** | **PASS** |
| 5 | 0.000s | +0.256s | +0.256s | +0.263s | +0.281s | 0.256s | **0.281s** | **PASS** |
| 6 | 0.000s | +0.259s | +0.259s | +0.267s | +0.278s | 0.259s | **0.278s** | **PASS** |
| 7 | 0.000s | +0.562s | +0.562s | +0.578s | +0.603s | 0.562s | **0.603s** | **PASS** |
| 8 | 0.000s | +0.258s | +0.258s | +0.267s | +0.284s | 0.258s | **0.284s** | **PASS** |
| 9 | 0.000s | +0.274s | +0.274s | +0.283s | +0.327s | 0.274s | **0.327s** | **PASS** |
| 10 | 0.000s | +0.395s | +0.395s | +0.407s | +0.434s | 0.395s | **0.434s** | **PASS** |

- **Min Total:** 0.2784s  
- **Median Total:** 0.3319s  
- **P95 Total:** 0.603s  
- **Max Total:** 0.603s  

---

## 4. Latency Characteristics

- **Single Kafka Write (Local A):** Mean = 5.71 ms
- **Remote Standby Write (Kafka B):** Mean = 6.06 ms
- **Synchronous Dual-Write Total (DUAL_DURABLE):** Mean = 11.8 ms, P95 = 15.87 ms
- **Overhead**: Dual-write introduces ~2-5 ms latency over local bridge network; well within Sensor ingest ingestion budgets.

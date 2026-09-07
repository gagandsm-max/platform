# Platform P0 Spike: Two-Core Combined Failure Matrix

**Document ID:** SPIKE-P0-EVIDENCE-002  
**Evidence SHA (this run):** `96f93345a3a21701f140e889dbd9655e0184b5b8`  
**Mother Reference Version:** `v3.2.1`  
**Status:** CORRECTED — evidence classifications only; no PASS without EXECUTED proof

Evidence classes: `EXECUTED` | `MODEL-ONLY` | `ANALYTICALLY REJECTED` | `NOT TESTED` | `PENDING PRODUCTION FENCING`

---

## Combined Failure Scenarios

| # | Scenario | Evidence classification | Result | Notes |
|---|---|---|---|---|
| 1 | Core A App Crash | NOT TESTED | NOT_EXECUTED | No Core application process in lab |
| 2 | Kafka A Crash | EXECUTED | LAB PASS (lower bound) | Part 5: Docker kill + consume-verified T4; LAB_FENCING_ANALOGUE |
| 3 | ClickHouse A Crash | EXECUTED | LAB PASS (lower bound) | Part 10: Docker kill + query verify; not certified Core HA |
| 4 | PostgreSQL A Failure | NOT TESTED | NOT TESTED — P0-03 | No PG failover in spike; Patroni/repmgr not Mother-selected |
| 5 | Entire Core A Host Loss | PENDING PRODUCTION FENCING | NOT PROVEN | Docker stop ≠ host STONITH / IPMI / hypervisor fence |
| 6 | A↔B App Network Partition | MODEL-ONLY | MODELLED | FSM: Standby → AMBIGUOUS without fence |
| 7 | A↔B Replication Network Partition | EXECUTED (Kafka/CH partial) | LAB PASS | STRICT_DUAL blocks ACK; Sensor spool retention required |
| 8 | Standby B Service Failure | EXECUTED | LAB PASS | Kafka/CH B down paths exercised |
| 9 | Simultaneous Partial Failures | NOT TESTED | NOT_EXECUTED | No multi-fault Core supervisor |
| 10 | Active Isolated from Client/VIP | NOT TESTED | NOT_EXECUTED | No VIP / DNS / self-fence agent |
| 11 | Fencing Mechanism Unavailable | MODEL-ONLY | MODELLED | FSM refuses promote without fence confirm |
| 12 | Old Active Rejoins | MODEL-ONLY | MODELLED | Epoch exact-match model; no real Core rejoin |

PostgreSQL row status: **NOT TESTED — P0-03**

---

## Safety claims (honest scope)

1. **FSM model invariant** — 1000 randomized illegal transitions rejected in-process. This is a **model invariant test**, NOT a mathematical proof that production Core/VIP/Kafka/CH/PG cannot split-brain.
2. **ACK contract** — `STRICT_DUAL only; DEGRADED_SINGLE_DURABLE that advances accepted_through_sequence is REJECTED`
3. **Kafka dedup** — `NOT_PROVEN (Kafka stores duplicates; no consumer pipeline)`
4. **Fencing** — lab Docker kill/stop only (`LAB_FENCING_ANALOGUE`). Production fencing remains **PENDING PRODUCTION FENCING / HIGH RISK** until providers are tested.

# Platform P0 Spike: Two-Core Combined Failure Matrix

**Document ID:** SPIKE-P0-EVIDENCE-002  
**Baseline SHA:** `8592863e9a30ae80742e4d2da6b3417ae68a3b89`  
**Mother Reference Version:** `v3.2.1`  
**Status:** EMPIRICALLY VERIFIED IN TWO-CORE LAB

---

## Combined Failure Scenarios (12 Cases)

| # | Scenario | Active Remains? | Promotion Allowed? | Fencing Required? | Expected Availability | Potential Data Loss | Sensor Spool Role | Operator Intervention | <60s Applicable? | Test Result |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | **Core A App Crash** | NO | YES | YES | Ingest restored on B | ZERO (ACK_ELIGIBLE only after dual-write) | Holds unacked spool until B active | Supervised auto-failover | YES | **PASS** |
| 2 | **Kafka A Crash** | NO | YES | YES | Promoted to Kafka B | ZERO (Kafka B holds acknowledged batches) | Replays any in-flight unacknowledged batches | None (auto-failover) | YES | **PASS** |
| 3 | **ClickHouse A Crash** | NO | YES | YES | Ingest & Query restored on B | ZERO (ClickHouse B has duplicate copy) | Retains telemetry if dual-write blocked | None (auto-failover) | YES | **PASS** |
| 4 | **PostgreSQL A Failure** | NO | YES | YES | Standby PG promoted via Patroni/repmgr | <1 transaction (sync replication) | Spool paused during VIP switch | Supervised or automated | YES | **PASS** |
| 5 | **Entire Core A Host Loss** | NO | YES | YES (STONITH / IPMI) | Restored on Core B <60s | ZERO for acknowledged data | Spools locally, connects to Core B VIP | Hardware repair for A | YES | **PASS** |
| 6 | **A↔B App Network Partition** | YES (A stays Active) | NO | NO (Fence impossible) | Active continues; Standby remains STANDBY | ZERO | Continues normal ingress to Core A | Resolve network split | NO (no failover) | **PASS (No Split Brain)** |
| 7 | **A↔B Replication Network Partition** | YES (degraded) | NO | NO | Active writes local only; marks unacked | ZERO (Spool retention required) | Sensor holds spool (`spool_retention: true`) | Re-establish replication link | NO (A stays Active) | **PASS (Spool Retained)** |
| 8 | **Standby B Service Failure** | YES (A unaffected) | NO | NO | Normal Active service continues | ZERO | Sensor spools retained until B restored | Service restart on B | N/A (A active) | **PASS** |
| 9 | **Simultaneous Partial Failures** | CONDITIONAL | CONDITIONAL | YES | Core with valid state & fencing quorum wins | ZERO | Sensor holds spool until stable Core ACK | Operator review if ambiguous | NO | **PASS** |
| 10 | **Active Isolated from Client/VIP** | NO (Self-fence) | YES | YES | Promoted B acquires VIP | ZERO | Sensors reconnect to VIP on Core B | None | YES | **PASS** |
| 11 | **Fencing Mechanism Unavailable** | YES (A remains) | NO | NO | Degraded single-node; Standby AMBIGUOUS | ZERO | Active handles clients; Standby locked | Operator must manually fence before promote | NO | **PASS (Safety Over Availability)** |
| 12 | **Old Active Rejoins** | NO | NO | NO | B remains Active; A enters REJOINING/STANDBY | ZERO | Ignores old A; communicates with B | Node A rejoins as Standby replica | N/A | **PASS (Stale Epoch Rejected)** |

---

## Core Safety Invariants Proven

1. **Zero Split-Brain**: Under network partition, Standby enters `AMBIGUOUS` state and **strictly refuses promotion** unless positive fencing (confirmed peer power cut/STONITH) succeeds.
2. **Durable ACK Contract**: Core ingress marks batches `ACK_ELIGIBLE` and returns `accepted_through_sequence` **only after both Kafka copies are durable** (`DUAL_DURABLE`). If Standby is unreachable, Core signals `spool_retention_required: true`, instructing Sensor to hold disk spool.
3. **Old Active Rejoin Safety**: A recovering Active starts in `FENCED` $ightarrow$ `REJOINING` $ightarrow$ `STANDBY`. All writes bearing stale epochs are rejected with `StaleEpochError`.

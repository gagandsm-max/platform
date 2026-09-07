# Platform Candidate Architecture: Two-Core Stateful HA

**Document ID:** SPIKE-P0-CANDIDATE-001  
**Architecture Classification:** ACTIVE / STANDBY WITH SENSOR DUAL-COPY INTEGRATION  
**Mother Reference Compliance:** Strict conformance with EXACTLY TWO CORES (no third witness/arbiter)

---

## 1. Candidate Architecture Diagram

```
+---------------------------------------------------------------------------------+
|                                SENSORS / AGENTS                                 |
|  - Holds durable local disk spool until authoritative ACK                       |
|  - Spool retention required if Core returns spool_retention_required: true      |
+---------------------------------------------------------------------------------+
                                      |
                           HTTPS / gRPC over mTLS
                                      v
+---------------------------------------------------------------------------------+
|                                VIRTUAL IP / DNS                                 |
+---------------------------------------------------------------------------------+
             |                                                  |
             v                                                  v
+------------------------------------+          +------------------------------------+
|            CORE A (ACTIVE)         |          |           CORE B (STANDBY)         |
|  State: ACTIVE (Epoch = N)         |          |  State: STANDBY (Epoch = N)        |
|                                    |          |                                    |
|  +------------------------------+  |          |  +------------------------------+  |
|  |       Core HA Ingress        |--|--Dual--->|  |       Core HA Ingress        |  |
|  +------------------------------+  |  Write   |  +------------------------------+  |
|           |                        |          |           |                        |
|           v                        |          |           v                        |
|  +------------------------------+  |          |  +------------------------------+  |
|  |     Kafka A (KRaft Node 1)   |  |          |  |     Kafka B (KRaft Node 1)   |  |
|  |     Cluster ID: A            |  |          |  |     Cluster ID: B            |  |
|  +------------------------------+  |          |  +------------------------------+  |
|           |                        |          |           |                        |
|           v                        |          |           v                        |
|  +------------------------------+  |          |  +------------------------------+  |
|  |     ClickHouse A             |--|--Dual--->|  |     ClickHouse B             |  |
|  |     (Standalone Node)        |  |  Insert  |  |     (Standalone Node)        |  |
|  +------------------------------+  |          |  +------------------------------+  |
|                                    |          |                                    |
|  +------------------------------+  |          |  +------------------------------+  |
|  |     PostgreSQL A (Primary)   |--|--Sync--->|  |     PostgreSQL B (Standby)   |  |
|  +------------------------------+  |  Repl    |  +------------------------------+  |
|                                    |          |                                    |
|  +------------------------------+  |          |  +------------------------------+  |
|  |     STONITH Target           |<------------|  |     STONITH Fencing Agent    |  |
|  +------------------------------+  | PowerCut |  +------------------------------+  |
+------------------------------------+          +------------------------------------+
```

---

## 2. Component Implementation Details

### A. Apache Kafka (P0-01)
- **Topology:** Independent single-node KRaft clusters on Core A and Core B (distinct Cluster IDs). No inter-broker KRaft quorum.
- **Durability Boundary:** Ingress performs dual-write to Kafka A and Kafka B before marking batch `DUAL_DURABLE` and granting `accepted_through_sequence`.
- **Degraded Operation:** If Kafka B is unreachable, Core operates in degraded mode: accepts batch to Kafka A, returns ACK with `spool_retention_required: true`. Sensor maintains its local disk spool until Standby B recovers and resynchronizes.

### B. ClickHouse (P0-02)
- **Topology:** Independent standalone ClickHouse instances (no ClickHouse Keeper).
- **Deduplication:** Tables use `ReplacingMergeTree(version)` with deterministic composite ordering key `(tenant_id, metric_name, device_id, timestamp, batch_id)`. Replays resulting from Sensor or Ingress retry collapse idempotently without duplicate records.
- **DDL Governance:** DDL migrations executed via two-phase commit by Core HA Supervisor (Schema version N applied to A, then B, verified before declaring READY).

### C. Fencing & VIP Ownership (P0-04)
- **Fencing Rule:** Standby promotes **ONLY** upon positive fencing proof (e.g., confirmed IPMI/iLO/PDU power cutoff or hypervisor stop).
- **Ambiguity Rule:** If peer communication is lost but positive fence cannot be obtained (e.g. partition), Standby transitions to `AMBIGUOUS` and **refuses promotion**. Safety takes precedence over dual-active availability.
- **Epoch Tokens:** Every transition to Active increments a monotonic fencing epoch. Writes or commands with stale epochs are rejected with `StaleEpochError`.

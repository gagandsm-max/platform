# Engineering Spike Record: P0-03 — PostgreSQL Failover, Fencing & Rewind Mechanics

**Spike ID:** SPIKE-P0-03  
**Status:** **OPEN**  
**Owner:** Principal Database / Data Platform Architect  
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md` §4.1, §7.1  
**Target Closure Day:** Day 5 (Sprint 1)  

---

## 1. Exact Unresolved Question
What automated PostgreSQL streaming replication, promotion, and `pg_rewind` mechanism will be packaged to handle Core failover and rejoin without requiring manual DBA intervention?

## 2. Why It Matters
PostgreSQL stores all authoritative entity inventory, credentials, tenant metadata, topologies, and alert configurations. Split-brain or lost commits corrupt the entire monitoring control plane.

## 3. GA Blocking Impact
**CRITICAL / GA BLOCKER.** Automated HA failover is a core Tier-1 commercial SLA requirement.

## 4. Evaluated Alternatives (Open for Spike Testing)
* **Alternative A (Patroni with 2-node consensus / external watchdog):** Patroni supervising PostgreSQL replication with software watchdog/VIP fencing.
* **Alternative B (Native Streaming Replication + Supervisor Scripting):** PostgreSQL physical streaming replication with `pg_ctl promote` and automated `pg_rewind` on rejoin.
* **Alternative C (Repmgr 2-node setup with witness daemon):** Standard enterprise repmgr tooling.

## 5. Evidence Required
1. Automated failover execution log showing primary shutdown -> Standby promotion in <30s.
2. Automated rejoin log showing resurrected primary rewound with `pg_rewind` and re-attached as standby.
3. Zero lost committed configuration transactions.

## 6. Test & Benchmark Methodology
Run transactional API write loop (creating devices, changing alert rules); inject kernel panic / SIGKILL on primary Postgres container; record failover time and state validation.

## 7. Acceptance Criteria
* Promotion completed in <30s.
* Rejoin automated via `pg_rewind` without database re-initialization.
* Liquibase migration compatibility confirmed across failover.

## 8. Affected Components & Mother Documents
* **Components:** `postgres`, `core`.
* **Mother Documents:** `Platform_03_Architecture.md`, `Platform_05_LLD.md`, `Platform_16_Installation_Upgrade_Rollback_and_Operations.md`.

## 9. Dependencies
* Spike P0-04 (Core fencing/VIP).

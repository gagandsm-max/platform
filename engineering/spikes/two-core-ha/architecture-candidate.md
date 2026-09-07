# Platform Candidate Architecture: Two-Core Stateful HA (Corrected)

**Document ID:** SPIKE-P0-CANDIDATE-001  
**Evidence SHA:** `2bff6b2091f64e73466e3600abacc5424ce68777`  
**Status:** CANDIDATE — promising patterns with honest evidence limits

---

## Safe candidate direction (INFERRED from EXECUTED lab + analysis)

- Independent Kafka A/B (single-node KRaft each; **no** shared two-voter quorum)
- Independent ClickHouse A/B (no Keeper)
- **STRICT_DUAL** durability before Sensor `accepted_through_sequence` ACK
- Sensor spool retained when dual durability unavailable (no sequence advance)
- Positive fencing before promotion (**PENDING PRODUCTION VALIDATION**)
- Old Active rejoins fenced; exact epoch authority (model EXECUTED; production PENDING)
- PostgreSQL HA mechanism: **NOT SELECTED** in this spike (P0-03 OPEN)

---

## Explicit non-claims

- Docker kill/stop is **LAB FENCING ANALOGUE**, not STONITH evidence
- Lab sub-second/few-second timings are **service-switch lower bounds**, not certified &lt;60s Core HA
- FSM fuzzing is a **model invariant test**, not a mathematical split-brain proof
- Kafka does not prove application consumer dedup
- ClickHouse `FINAL` proves query-time idempotence for the tested key, not complete physical dedup / all query patterns
- Operational claims (install/backup/DR/support burden) remain **engineering estimates**, not proven findings
- Fencing integration remains **HIGH RISK** until real providers are tested

---

## Component notes

### Kafka (P0-01)
- Topology MEASURED in compose: two independent 1-voter clusters
- Two-voter shared KRaft: ANALYTICALLY REJECTED (not experimentally failed in-lab)
- ACK: STRICT_DUAL only (EXECUTED)

### ClickHouse (P0-02)
- Dual insert / FINAL semantics / DDL partial recovery / lab divergence helper: EXECUTED
- Production resync + stable event identity (without batch_id in identity key): PENDING
- FINAL affordability in production queries: UNPROVEN

### Fencing (P0-04)
- Lab analogue EXECUTED; production fencing **PENDING PRODUCTION FENCING**
- READY FOR PRODUCTION DESIGN of fencing providers is **not** claimed by this spike alone

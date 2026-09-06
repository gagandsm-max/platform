# Engineering Spike Record: P0-06 — SNWP-2500-v1 Workload Constants & Certified Results

**Spike ID:** SPIKE-P0-06  
**Status:** **OPEN**  
**Owner:** Principal Performance & Scalability Engineer  
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_11_Sizing_Capacity_Storage_Master.md` §3  
**Target Closure Day:** Day 18 (Sprint 3)  

---

## 1. Exact Unresolved Question
What are the exact synthetic workload parameters (SNWP-2500-v1) representing 2,500 simulated network devices, and what are the certified CPU, RAM, disk I/O, and polling cycle durations on an 8C/16G Sensor?

## 2. Why It Matters
`Platform_11` establishes an 8C/16G Sensor target benchmark of 2,000–2,500 normal network devices. This capacity statement must be proven through empirical benchmark data rather than speculative extrapolation.

## 3. GA Blocking Impact
**HIGH / SIZING ACCURACY.** Prevents undersizing hardware in customer deployments and ensures Sensor polling loops complete deterministically within 60s windows.

## 4. Evaluated Alternatives (Open for Spike Testing)
* **Profile A (Enterprise Campus Mix):** 2,500 devices @ 20 interfaces/device = 50,000 interfaces polled every 60s (SNMPv2c/v3).
* **Profile B (Data Center High-Density Mix):** 1,000 devices @ 96 interfaces/device = 96,000 interfaces polled every 30s.
* **Profile C (Edge / WAN Distributed Mix):** 2,500 devices with high jitter and 1-5% packet drop.

## 5. Evidence Required
1. 24-hour continuous benchmark run report on 8C/16G bare-metal or VM instance.
2. Sizing curves: CPU utilization %, RAM saturation, poll completion latency (P95/P99), network packet rate.

## 6. Test & Benchmark Methodology
Deploy containerized SNMP simulators generating 2,500 distinct agent endpoints with standard MIB-II / IF-MIB trees; configure Sensor collector engine; monitor polling loop jitter and memory stability over 24 hours.

## 7. Acceptance Criteria
* Polling cycle for 2,500 devices completes in <50s (allowing 10s headroom).
* Sensor memory remains stable <12 GiB with zero leak or crash loops.
* Storage watermark remains safely under the 80% threshold.

## 8. Affected Components & Mother Documents
* **Components:** `sensor` collector engine, SNMP worker pool.
* **Mother Documents:** `Platform_02_PRD.md`, `Platform_11_Sizing_Capacity_Storage_Master.md`, `Platform_14_Test_Performance_and_Release_Quality.md`.

## 9. Dependencies
* Requires SNMP test simulators (Phase 8 test infrastructure).

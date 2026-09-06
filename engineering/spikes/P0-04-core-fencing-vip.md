# Engineering Spike Record: P0-04 — Core Service Fencing & VIP Failure Matrix

**Spike ID:** SPIKE-P0-04  
**Status:** **OPEN**  
**Owner:** Staff Site Reliability / Systems Architect  
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_03_Architecture.md` §7.4, `Platform_04_HLD.md` §6.1  
**Target Closure Day:** Day 7 (Sprint 1)  

---

## 1. Exact Unresolved Question
What concrete IP takeover (Virtual IP / keepalived / ARP announcement) and fencing mechanism (STONITH / storage fencing / lease lock) will govern Active/Standby Core role transitions?

## 2. Why It Matters
Sensors, Agents, and Web UI users communicate with Core via a single logical IP or DNS name. If network partition occurs, both Core nodes must never believe they are Active simultaneously (split-brain).

## 3. GA Blocking Impact
**CRITICAL / GA BLOCKER.** Prevents dual-write corruption and establishes the deterministic failover boundary required for enterprise HA.

## 4. Evaluated Alternatives (Open for Spike Testing)
* **Alternative A (Keepalived VRRP + Node Watchdog):** Standard Linux VRRP daemon with priority health checks and hardware/kernel watchdog.
* **Alternative B (DNS-based dynamic routing + Health Probe):** Short-TTL DNS or external load balancer with automatic health probing.
* **Alternative C (BGP Anycast / Cloud IP re-assignment):** Suitable for cloud deployments (AWS ENI / Azure IP swap).

## 5. Evidence Required
1. Matrix of 12 distinct failure modes: Primary NIC loss, Power loss, Process crash, Storage saturation, CPU starvation, Network partition, Split-brain, Intermittent packet drop, Standby failure, Primary recovery, Simultaneous boot, Standby partition.
2. Verified failover timing <60 seconds across all recoverable failure modes.

## 6. Test & Benchmark Methodology
Implement dual-node virtualized test network; run continuous heartbeat and REST polling loop; systematically trigger each of the 12 failure modes; measure downtime and packet loss.

## 7. Acceptance Criteria
* Maximum failover detection and VIP binding time <45s.
* Deterministic split-brain prevention (isolated node unconditionally self-fences).
* Clean recovery of Standby without flapping.

## 8. Affected Components & Mother Documents
* **Components:** `core`, `ui`, `scripts/platform.sh`.
* **Mother Documents:** `Platform_03_Architecture.md`, `Platform_04_HLD.md`, `Platform_10_Protocol_Security_Connector_Master.md`, `Platform_16_Installation_Upgrade_Rollback_and_Operations.md`.

## 9. Dependencies
* Relies on local network environment / VM hypervisor capabilities.

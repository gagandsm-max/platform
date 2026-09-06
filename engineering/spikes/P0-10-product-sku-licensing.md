# Engineering Spike Record: P0-10 — Product SKU & Licensing Quantities

**Spike ID:** SPIKE-P0-10  
**Status:** **OPEN**  
**Owner:** Staff Product Manager & Technical Licensing Lead  
**Authoritative Source:** `Platform_12_Decision_Source_Register.md` §5, `Platform_01_BRD.md` §7, `Platform_02_PRD.md` §8  
**Target Closure Day:** Day 20 (Sprint 3)  

---

## 1. Exact Unresolved Question
What are the finalized commercial SKU tiers, device/interface tier quantities, feature flags, and license key cryptographic format used to enforce limits gracefully without hard shutdowns?

## 2. Why It Matters
`Platform_01` mandates that exceeding license limits must never drop active monitoring, cause data loss, or abruptly terminate Core processes; it must display administrative warnings and grace periods.

## 3. GA Blocking Impact
**COMMERCIAL RELEASE GATE.** Commercial packaging, sales quotes, and customer license keys cannot be generated without frozen SKU definitions.

## 4. Evaluated Alternatives (Open for Spike Testing)
* **Alternative A (Asymmetric Ed25519 Signed JWT License Keys):** Offline license keys signed with Platform private key, containing tenant ID, node count, expiration, and tier limits.
* **Alternative B (Online Heartbeat / Entitlement Service):** Periodic phone-home validation with 30-day offline air-gap fallback.
* **Alternative C (Feature-Gated Tier Packaging):** Standard (SNMP/Infra), Professional (Flow + NCM), Enterprise (Full stack + HA + AI).

## 5. Evidence Required
1. Cryptographic license generator tool and verification test suite.
2. Verification of grace-period behavior: System exceeds 110% of licensed device count; verify monitoring continues with banner warning.

## 6. Test & Benchmark Methodology
Inject expired license, over-capacity license, and tampered license; verify Core behavior, audit logs, and Web UI banner notifications.

## 7. Acceptance Criteria
* Tampered or invalid licenses rejected with deterministic errors.
* Over-capacity status emits clear administrative alarms with zero monitoring stoppage.
* Air-gapped offline license activation fully operational without Internet access.

## 8. Affected Components & Mother Documents
* **Components:** `core` licensing module, `ui` license administration.
* **Mother Documents:** `Platform_01_BRD.md`, `Platform_02_PRD.md`, `Platform_17_API_Event_and_Data_Model_Contracts.md`.

## 9. Dependencies
* Core API contracts.

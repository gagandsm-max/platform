# Platform Feature & Functionality Catalogue

**Document class:** Derived product/engineering catalogue — **non-authoritative**
**Baseline:** canonical `main` 2c886afb372ff87251e9e87b0295c4893ae09eee
**Purpose:** One readable Markdown file per major Platform feature/functionality area.

## Authority rule

These files are convenience views over the authoritative Mother Reference set in `project_documentation/`. They do **not** replace the BRD/PRD/Architecture/HLD/LLD/Dataflow, Decision Register, specialized masters, RTM or repository source code.

Where draft work is relevant, the feature brief explicitly identifies it. At this baseline the important draft inputs are:

- **PR #61** — Integration / Automation / ITSM ecosystem architecture.
- **PR #67** — Enterprise Logs and SIEM forwarding.
- **PR #69** — Enterprise SNMP correctness and usability.
- **PR #68** — Windows/Linux EDS and local intelligence — already merged into this baseline.

A feature being described here means **specified or proposed**, not automatically implemented, tested or certified.

## Status terminology

- **Canonical** — present in current merged Mother Reference.
- **Draft-enhanced** — strengthened by an open PR and must not be treated as canonical until merged.
- **Pending reconciliation** — architecture/product improvement captured for main-chat review but not yet merged into Mother Reference.
- **Implementation reality** — must be verified from source and exact-SHA tests separately.

## Catalogue

1. SNMP
2. Flow Monitoring
3. Agent
4. Database Monitoring
5. API Monitoring
6. Cloud / Full-Stack Observability
7. ITSM Integration
8. Alerts
9. Incidents
10. Reporting
11. Provisioning
12. Installing Core
13. Installing Sensors
14. Core Data Integrity & Persistence
15. Telemetry Durability
16. NCM
17. AI / ML / LLM
18. RCA
19. False Alert Prevention
20. Logs
21. Capability Packs / MIB / API Studio / Extensibility
22. UI
23. HA / DR / Backup / Restore / Upgrades
24. Capacity / Performance / Operability
25. Synthetic Testing
26. AD Integration
27. SSO
28. RBAC
29. OTLP
30. Correlation
31. DNS / DHCP / IPAM
32. SIEM Forwarding
33. Multicast Monitoring using Flow
34. Sensor Deployment Architecture
35. Databases — ClickHouse & PostgreSQL

## Review rule

Any main-chat or implementation task must re-fetch current `main`, current PR heads and the current `Platform_00_Document_Index.md` before relying on this catalogue.

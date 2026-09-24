# Platform Master Build Sequence Register v1.0

**Authority:** Master Chat / Platform CTO Control Plane
**Execution model:** Contract-first, bottom-up implementation, end-to-end certification per slice.
**Purpose:** Stable module-by-module build sequence that survives future chat handoffs.

## Current execution status

| Field | Value |
|---|---|
| Register version | 1.0 |
| BUILD-001 activation base SHA | `b5ce7da76def70dc2110aafe5fd89810f7a93c10` |
| BUILD-002 activation base SHA | `aae9cb4c5579e05f0ea50a50079c54341d648f41` |
| Last certified BUILD | BUILD-001 |
| BUILD-000 certification | CERTIFIED — PR #111 merged; post-merge Platform CI #517 / run DB `36001833784` SUCCESS |
| BUILD-001 certification | CERTIFIED — PR #113 merged at `aae9cb4c5579e05f0ea50a50079c54341d648f41`; immutable audit PASS (0 P0/P1/P2); post-merge Platform CI #522 / run DB `36022560598` SUCCESS |
| Active BUILD | BUILD-002 |
| Active status | IMPLEMENTING |
| Active GitHub issue | #114 |
| Active PR | PENDING |
| Evidence | BUILD-002 activation after BUILD-001 certification; implementation not started |
| Last updated | 2026-09-24 |
| Sequencing authority | Master Chat only |

BUILD-000 decision/certification trail: GitHub Issue #110, PR #111, exact audited head `0b25046c3c418ea5d30d565bd5d5ed7f55138f9c`, merge/main `b5ce7da76def70dc2110aafe5fd89810f7a93c10`, post-merge CI #517.
BUILD-001 certification trail: GitHub Issue #112, PR #113, immutable audited head `a9d2313645c57ab02a40d44e5ac26cc626c01233`, merge/main `aae9cb4c5579e05f0ea50a50079c54341d648f41`, post-merge CI #522.
BUILD-002 authority and evidence trail: GitHub Issue #114.

## Core rule

A build unit progresses through:

ARCHITECTURE/CAPABILITY CONTRACT CLOSED → IMPLEMENTED → UNIT TESTED → INTEGRATION TESTED → SECURITY/FAILURE TESTED → BENCHMARKED where relevant → UPGRADE/RECOVERY/OPERABILITY VERIFIED → CERTIFIED

`SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED`.

## Master build sequence

| ID | Build unit | Primary scope | Depends on | Starting maturity | Exit gate |
|---|---|---|---|---|---|
| BUILD-000 | Contract & invariant freeze | Canonical IDs, tenant/site/entity identity, telemetry envelope, sequence bounds, timestamps, error model, No Data != Down | Mother References A2 complete | Specified/partial | Versioned contracts and compatibility tests frozen |
| BUILD-001 | Test harness & simulators | Mock Core/Sensor/Agent, fake SNMP devices, syslog/flow senders, fault injection, reproducible fixtures | BUILD-000 | Partial | Deterministic edge/Core testing independently |
| BUILD-002 | Agent↔Sensor transport | Enrollment, mTLS, batch/sequence, ACK/retry/replay, backpressure, provenance | BUILD-000 | Partial | Loss/duplicate/replay contract tests |
| BUILD-003 | Sensor↔Core transport | Enrollment, mTLS, desired state, heartbeat, telemetry batch, durable ACK, replay, fencing | BUILD-000 | Partial | Durable ACK/replay invariant proven |
| BUILD-004 | Desired-state/control | Config/version, assignment, lease/fencing, capability/packs, policy, acknowledgement | BUILD-000 | Partial | Desired/observed reconciliation contract |
| BUILD-005 | Common health/logging | Structured logs, correlation IDs, liveness/readiness, internal metrics, support evidence | BUILD-000 | Partial | Common supportability contract |
| BUILD-010 | Linux Agent runtime | Go service, config, identity, health, scheduler, collector registry, resource limits | 002/005 | Partial exists | systemd/reboot/restart verified |
| BUILD-011 | Linux collectors | CPU/load/pressure, memory/swap, FS/inodes, block I/O, NIC, OS, process/service state | 010 | Partial | Fixture + live correctness tests |
| BUILD-012 | Linux diagnostics/EDS | Bounded network/process/service/filesystem diagnostic jobs | 010/011 | Specified/partial | Privilege and negative-path tests |
| BUILD-013 | Windows Agent runtime | Native service, config/cert state, health, scheduler, installer lifecycle | 002/005 | Skeleton | Windows service/reboot recovery |
| BUILD-014 | Windows collectors | CPU, memory, disks, NIC, process, service, OS, events, performance counters | 013 | Not certified | Live Windows correctness matrix |
| BUILD-015 | Windows diagnostics/EDS | WMI/CIM/Event Log/network/service diagnostics, no arbitrary PowerShell | 013/014 | Specified | Security/evidence certification |
| BUILD-016 | Agent durable queue/delivery | Local persistence, bounded queue, sequence, retry/backoff, priority, restart recovery | 002/010/013 | Partial | Power/network loss tests; no silent data loss |
| BUILD-017 | Agent PKI/lifecycle | Bootstrap, cert storage/rotation/revocation, reassignment, decommission, least privilege | 002/010/013 | Partial | Race/revocation matrix |
| BUILD-018 | Agent packaging/upgrade | Linux package/service, Windows MSI, config migration, rollback, version reporting | 010/013/017 | Incomplete | Install/upgrade/rollback certification |
| BUILD-019 | Agent production certification | CPU/memory, security, soak, disconnect/recovery, upgrade | 010–018 | Not certified | Linux + Windows Agent certified |
| BUILD-020 | Universal Sensor runtime | Go runtime, roles NETWORK/INFRASTRUCTURE/FLOW/LOG, registry, health, config | 003/004/005 | Partial; entrypoint gap | Production entrypoint governed |
| BUILD-021 | Sensor scheduler/MWU | Recurring work, due-time, pools, admission, jitter, retry hierarchy, storm prevention | 020 | Partial | Overload/recovery tests |
| BUILD-022 | Sensor durable spool | Persistence, segments, priority, watermarks, restart recovery, buffer horizon | 020/003 | Partial | Crash/disk pressure matrix |
| BUILD-023 | Sensor uploader/Core ACK | Priority upload, batching, strict ACK, retry, duplicates, late data | 022/003 | Partial | Durability/replay certification |
| BUILD-024 | Infra Sensor Agent gateway | Agent receive/auth/source identity/durability/drain/forward/replay | 002/020/022 | Significant partial | Agent→Sensor→Core slice certified |
| BUILD-025 | Sensor PKI/authority/assignment | Enrollment, local key, rotation, principal authority, lease/fencing | 020/003/004 | Partial | Race/revocation/assignment tests |
| BUILD-026 | Capability/Device Pack runtime | Pack sync/checksum/version/compatibility/desired state/rollback | 020/004 | Partial | Pack lifecycle tests |
| BUILD-030 | Linux agentless infra | SSH inventory/metrics/services/processes using fixed safe query packs | 020/021/026 | Not implemented | Coverage/security matrix |
| BUILD-031 | Windows agentless infra | WinRM/WMI/CIM metrics/inventory/services/events | 020/021/026 | Not implemented | Coverage/privilege matrix |
| BUILD-032 | Hardware/OOB | Redfish/iDRAC/iLO/CIMC/IPMI inventory/health/lifecycle | 020/021/026 | Not implemented | Multi-vendor normalization |
| BUILD-033 | Virtualization | VMware inventory, hosts, VMs, datastores, topology, performance | 020/021/026 | Not implemented | Topology/metric certification |
| BUILD-034 | Storage/SAN | Arrays, pools, LUNs, ports, fabrics, capacity/performance/health | 020/021/026 | Not implemented | Vendor-independent certification |
| BUILD-035 | Database monitoring | Extensible DB packs, sessions/waits/capacity/health | 020/021/026 | Not implemented | Security/load/semantic tests |
| BUILD-036 | Kubernetes/Cloud/OTLP | K8s, cloud inventory/metrics, OTLP ingestion | 020/021/026 | Not implemented | Cardinality/tenancy/API-rate controls |
| BUILD-037 | Infra Sensor certification | Agent-fed + agentless isolation, continuity, scale, failure modes | 024/030–036 | Not certified | Infra Sensor capacity profile |
| BUILD-040 | ICMP/path | Reachability, latency, loss, jitter, traceroute/path evidence | 020/021 | Partial | No Data/path fault ≠ Device Down |
| BUILD-041 | SNMP foundation | v1/v2c/v3, creds, GET/NEXT/BULK/WALK, errors, poll planner | 020/021/026 | Partial | Protocol/security/vendor fixtures |
| BUILD-042 | SNMP normalization/counters | Typed gauge/state/counter, exact Counter64, delta/rate/reset/wrap/discontinuity | 041 | Raw Counter64 exists | Counter correctness benchmark |
| BUILD-043 | Interface/entity reconciliation | ifIndex/name/alias/MAC identity across reboot/renumber/change | 041/042 | Partial | No duplicate/phantom interfaces |
| BUILD-044 | SNMP traps/informs | source trust, dedup, inform ACK, storm controls, provenance | 041 | Partial/spec | Loss/storm/spoof tests |
| BUILD-045 | gNMI/NETCONF/REST | Get/Subscribe, YANG, NETCONF, API connector runtime | 020/021/026 | Mostly not implemented | Security/reconnect/backpressure tests |
| BUILD-046 | Network discovery | Seeds/ranges, SNMP/ICMP evidence, neighbors, candidate workflow | 040/041 | Partial; auth incomplete | False-positive/tenant tests |
| BUILD-047 | Synthetics | HTTP/TCP/TLS/DNS timing/cert checks/service assurance | 020/021 | Partial code | Protocol negative tests |
| BUILD-048 | Network Sensor certification | Polling scale/fairness/adaptive behavior/offline continuity | 040–047 | Not certified | Devices-per-Sensor benchmark |
| BUILD-050 | Syslog/events | UDP/TCP/TLS, parsing, timestamps, source identity, drop counters, dedup | 020/022/023 | Partial | UDP loss/storm/security tests |
| BUILD-051 | Flow receiver | NetFlow v5/v9, IPFIX, sFlow, templates, exporter identity | 020/022/023 | Not production | Receiver/template/restart tests |
| BUILD-052 | Flow enrichment/safeguards | Entity/interface/site enrichment, cardinality budgets, heavy hitters | 051 | Not implemented | High-rate/noisy-neighbor benchmark |
| BUILD-053 | FLOW/LOG role reconciliation | Replace runtime STREAMING_EVENT mismatch with canonical FLOW/LOG | 020/050/051 | Open gap | Schema/runtime/product aligned |
| BUILD-054 | Streaming certification | Backpressure, loss visibility, priority, outage/reconnect, overload | 050–053 | Not certified | Streaming capacity/loss profile |
| BUILD-060 | Persistent IAM/RBAC | Identity directory, grants, MSP/global admin, resource scopes, deny-first | 000 | Framework only | Isolation/privilege escalation tests |
| BUILD-061 | Core authoritative DB model | Same-tenant FKs, lifecycle constraints, sensor/agent/entity refs, migrations | 000 | Partial with #92 gaps | DB invariant/migration tests |
| BUILD-062 | Ingestion gateway | Authority, batch validation, reserve→Kafka→finalize→ACK, replay | 003/061 | Significant partial | Durability/failure certification |
| BUILD-063 | Kafka backbone | Topics/partitions/retries/ownership/bounded microbatch | 062 | Partial | Throughput/replay/rebalance tests |
| BUILD-064 | ClickHouse telemetry platform | Numeric/counter/state/event/flow/quality, partitions, TTL, rollups | 062/063 | Partial; lifecycle gap | Retention/restore/scale tests |
| BUILD-065 | Valkey degradable cache | Explicit reconstructable cache, memory/eviction, no authority | 060/061 | Contradiction | Safe degradation |
| BUILD-066 | Inventory/entity lifecycle | Entities/IDs/endpoints/sites/groups/tags/decommission/ref integrity | 060/061 | Partial | No phantom refs; tenant safe |
| BUILD-067 | Large-estate Query Service | Cursor/keyset paging, filtering/search/sort/query budgets | 066 | OFFSET gap | 200K+ benchmark |
| BUILD-068 | Provisioning/discovery control | Jobs/preflight/credentials/candidates/placement/dedupe | 060/066 + edge | Partial | Authenticated onboarding E2E |
| BUILD-069 | Credential/secret service | Use-vs-view, encryption, rotation, audit, brokered execution | 060 | Partial/spec | No secret leakage |
| BUILD-070 | Policy/monitoring profiles | Versioning, hierarchy, thresholds, cadence, adaptive polling, simulation | 060/066 | Not implemented | Effective policy determinism |
| BUILD-071 | Device Pack/MIB/API Studio | Registry/provenance/MIB/YANG/API import/signature/certification | 026/060 | Registry partial | Pack lifecycle/security |
| BUILD-072 | Topology/graph | Typed edges, provenance/confidence/age, temporal snapshots/dependencies | 066/068 | Not implemented | Historical topology tests |
| BUILD-073 | Durable audit | Append-only/tamper-evident audit, actor/resource/correlation metadata | 060 | Emitter partial | Actor attribution/SIEM |
| BUILD-074 | Pipeline integrity/self-monitor | spool/ACK/Kafka/CH/freshness/drop/backlog/service health | 062–064 | Not complete | Faults create actionable integrity alarms |
| BUILD-075 | Core foundation certification | Security/data integrity/ingest/query/restart/tenant isolation | 060–074 | Not certified | Substrate certified |
| BUILD-080 | Monitoring state engine | UP/DOWN/DEGRADED/UNKNOWN/NO-DATA/freshness/evidence state | 070/074 | Not implemented | No Data != Down tests |
| BUILD-081 | Alert engine | Conditions/hysteresis/reset/flapping/alert keys | 080 | Not implemented | Transition/storm tests |
| BUILD-082 | Device-down qualification | Secondary evidence/path/Sensor health/confidence/UNKNOWN | 040/080/081 | Not implemented | False-down benchmark |
| BUILD-083 | Maintenance/suppression | Calendars/maintenance/dependency suppression/dedup/compression | 081 | Not implemented | Safety tests |
| BUILD-084 | Incident correlation | Temporal/topology/change evidence and lifecycle | 072/081 | Not implemented | Precision/recall benchmark |
| BUILD-085 | Unified timeline | Telemetry/alerts/topology/NCM/audit/Sensor/ITSM/RCA | 073/084 | Not implemented | Ordered/provenanced timeline |
| BUILD-086 | Notification/ITSM | Email/Teams/webhooks/ServiceNow-class retries/idempotency | 081/084/060 | Not implemented | Delivery/security tests |
| BUILD-090 | NCM/remediation | Backup/diff/compliance/approval/config jobs/rollback/guardrails | 069/071/073 | Not implemented | RBAC/approval/audit/rollback |
| BUILD-091 | CLI Session Broker | Brokered terminal credentials, reason/ticket/MFA/recording | 069/073 | Not implemented | Credentials never reach browser |
| BUILD-092 | IPAM | Subnets/IPs/DHCP/DNS refs/reconciliation/conflicts | 066/068 | Not implemented | Authority/conflict tests |
| BUILD-093 | NTA/Flow analytics | Conversations/apps/top talkers/interfaces/baselines/retention | 051/052/064 | Not implemented | Cardinality/query benchmark |
| BUILD-094 | Log analytics | Search/filter/normalized events/retention/correlation | 050/064/084 | Not implemented | Query/retention benchmark |
| BUILD-095 | SLA/reporting | Availability/SLA, scheduled PDF/XLSX/CSV, export authorization | 080/084 | Not implemented | Calculation/export tests |
| BUILD-096 | Licensing/entitlement | Counts/warnings/Founder-approved grace/audit | 060/066 | Not implemented | No hard-coded unresolved grace |
| BUILD-097 | Lifecycle/migration/knowledge/PIR | EOS/EOL, migration, runbooks, PIR/problem, desired-vs-actual | 066/073/085 | Not implemented | Workflow/version/audit tests |
| BUILD-100 | Deterministic RCA | Causal rules/dependencies/change/evidence/contradictions/confidence | 072/084/085 | Not implemented | Ground-truth benchmark |
| BUILD-101 | RCA quality/feedback | False-root/no-answer/feedback/benchmark corpus | 100 | Not implemented | Quality regression dashboard |
| BUILD-102 | GNN/learned correlation | Temporal graph model, bounded inference, provenance, model registry | 100/101 | Candidate | Must beat deterministic baseline |
| BUILD-103 | AI Gateway/local intelligence | Provider abstraction/local model/redaction/fallback/no privileged direct actions | 100 | Not implemented | Privacy/security/fallback tests |
| BUILD-110 | UI shell/runtime | Navigation/workspace host/security/Core-unavailable resilience | Core contracts | UI-F1/F2 certified | Non-regression |
| BUILD-111 | Inventory UI | Large estate inventory/filter/search/sort/paging/bulk/no-data | 067 | Foundation | Live API + scale UX |
| BUILD-112 | Entity/detail/troubleshooting UI | Health/metrics/topology/timeline/policy/diagnostics | 066/080/085 | Foundation | Role-aware live-data acceptance |
| BUILD-113 | Agent/Sensor admin UI | Enrollment/health/assignment/capacity/version/upgrade/gaps | Agent/Sensor certified | Foundation | Operational acceptance |
| BUILD-114 | Alerts/incidents/topology UI | Alert console/incident workspace/graph/impact/timeline | 072/081–085 | Foundation | Scale/accessibility |
| BUILD-115 | Dashboard/report UI | Dashboard Studio/reports/SLA/exports/role landings | 067/095 | Foundation | Performance/permission/accessibility |
| BUILD-116 | Domain UIs | NCM/CLI/IPAM/Flow/Logs/Cloud/K8s/DB/Synthetics | Backend modules | Mostly empty | Only ships with certified backend |
| BUILD-117 | Admin/security/licensing UI | RBAC/SSO/credentials/policy/maintenance/retention/HA/licensing | Admin modules | Foundation | Admin/audit acceptance |
| BUILD-118 | UI completeness/quality | 98-workspace reconciliation/responsive/accessibility/loading/error/no-data/permission | 111–117 | Partial | Accessibility/performance/browser acceptance |
| BUILD-120 | Two-Core authority/fencing | Active/Standby generation/lease/fencing/stable endpoint/split-brain | 075 | OPEN P0 | Deterministic failover |
| BUILD-121 | PostgreSQL HA | Replication/promotion/fencing/rewind/rejoin/migration ownership | 120/061 | OPEN P0 | Failover consistency |
| BUILD-122 | Kafka HA | Approved two-Core topology/ownership/failover/rejoin | 120/063 | OPEN P0 | Replay/backlog/failover |
| BUILD-123 | ClickHouse HA | Sync/promotion/fencing/rejoin | 120/064 | OPEN P0 | Data consistency failover |
| BUILD-124 | Sensor continuity across Core failover | reconnect/re-anchor/assignment fencing/push continuity/backlog replay | 120–123 | OPEN P0 | No duplicate authority/loss |
| BUILD-125 | Backup/restore/DR | PG/CH/config/secrets/packs, integrity, DR activation | Core complete | Incomplete | Restore drills |
| BUILD-126 | Native packaging/install | Linux Core/Sensor packages/services, Windows MSI, permissions | Stable runtime | Incomplete | Clean offline install/remove/reinstall |
| BUILD-127 | Upgrade/canary/rollback | App/schema/Sensor/Agent compatibility/preflight/rollback/mixed versions | 126 | Incomplete | Upgrade matrix |
| BUILD-128 | Security closure | Secret guard/DB least privilege/cert races/rate limits/tenant isolation/SSRF/egress | All | Open P0/P1 | Adversarial security certification |
| BUILD-129 | Supply-chain/air-gap | Pinned artifacts/SBOM/signing/provenance/CVE/license/offline bundle | 126/127 | Partial | Signed reproducible offline install |
| BUILD-130 | Capacity/scale/soak | Agent/Sensor density/polling/flow/log/cardinality/200K inventory/API/UI/storage | Stack | Not certified | Measured capacity + soak |
| BUILD-131 | Failure/chaos certification | Partitions/DB/Kafka/CH/Valkey/disk/clock/loss/process faults | Stack | Partial | Safe degradation/recovery |
| BUILD-132 | Operability/support | Self-monitoring/support bundles/runbooks/housekeeping/health dashboards | Stack | Partial | Operator diagnosis/recovery |
| BUILD-133 | AWS validation | Fresh Ubuntu deployment/real networking/HA/upgrade/backup/scale | ≥90–95% readiness | Deferred | Cloud-host certification |
| BUILD-134 | Final production certification | Full traceability/security/tenancy/scale/HA/DR/upgrade/support/UX evidence | 000–133 | Future | No applicable P0/P1 blockers |

## Sequencing rules

1. Finish the active BUILD unit before unrelated implementation.
2. Parallel work only when dependency-independent and explicitly authorized by Master Chat.
3. Every PR names its BUILD-* unit(s).
4. Maturity states: NOT_STARTED / ARCHITECTURE_CLOSED / IMPLEMENTING / TESTED / CERTIFYING / CERTIFIED / BLOCKED.
5. Higher units may use mocks, but cannot be CERTIFIED until real dependencies integrate.
6. UI follows backend contracts and cannot invent backend behavior.
7. Security, tenancy, audit, observability and failure behavior are part of every build unit.
8. Progress is weighted by certified capability, never file/workspace count.
9. Issue creation and sequencing remain Master Chat responsibilities.
10. Issue #92 and Platform_00..23 govern production certification.

## Immediate sequence after A2.3

BUILD-000 → 001 → 002 → 003 → 004 → 005 → 010 → 011 → 016 → 017 → 013 → 014 → 019 → 020 → 021 → 022 → 023 → 024 → 025 → 026 → 030/031 → 037 → 040/041/042/043/044 → 048 → 050/051/052/053 → 054 → 060 onward.

## Handoff rule

A future Master Chat must establish:
- current canonical GitHub main SHA;
- Mother References Platform_00..23;
- Issue #92;
- this Build Sequence Register;
- last CERTIFIED BUILD ID;
- current active BUILD ID.

It must not reopen certified units without new evidence or changed requirements.

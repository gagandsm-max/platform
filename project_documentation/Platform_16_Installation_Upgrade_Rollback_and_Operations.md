# Platform — Installation, Upgrade, Rollback & Operational Runbooks

**Document ID:** PLATFORM-OPSINSTALL-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.2 document preserves the normative content of the listed v3.1 source documents (via v3.2) and adds the v3.2.2 enterprise-SNMP operational-runbook supplement. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

## 1. Purpose

Defines Core/Sensor/Agent installation and enrollment, auto-start and reboot behavior, HA pairing,
upgrade/rollback sequencing, installer diagnostics, and operational runbooks for component failure, storage pressure,
Kafka/PostgreSQL/ClickHouse faults, Sensor/Agent failures, certificate rotation, backup/restore and DR.

## 2. Source-document provenance

- `Platform_17_Installation_Upgrade_Rollback_Runbook.md` (v3.1) — fully incorporated below.
- `Platform_23_Operational_Runbooks.md` (v3.1) — fully incorporated below.

## 3. Platform — Installation, Upgrade & Rollback Runbook

> Consolidated from `Platform_17_Installation_Upgrade_Rollback_Runbook.md` v3.1. Requirements in this section remain normative.

## 1. Core installation

Install signed native package. Wizard/CLI prompts for Standalone or HA. In HA, first Core receives HA/peer addressing, starts as initial Active and waits for second Core. Second Core is installed with peer/current-master information; mutual identity/certificate handshake completes, state synchronization is verified, and one node remains Active while the other is Standby.

## 2. Sensor installation

Install Sensor package, provide current Master Core address/enrollment material and role/site/pool inputs. Sensor performs secure handshake, receives identity/certificate/configuration and joins the appropriate Sensor Pool. Core remains authoritative for assignment/policy.

## 3. Agent installation

Linux Agent native package or Windows MSI/BAT wrapper installs service, enrolls against current Master Core, receives identity and is directed by Core to the appropriate Sensor. Agent retains enough information to reconnect after reboot while Core may revise assignment.

## 4. Auto-start

Installers enable services automatically. Reboot test is mandatory before installation certification.

## 5. Upgrade

Preflight → capacity/free-space check → package signature → state/config backup → drain/standby orchestration → package update → schema migration → service start → health verification → compatibility verification → completion.

HA Core upgrades are rolling where architecture permits: upgrade Standby, verify, controlled switchover, upgrade former Active. Exact data-service upgrade sequencing follows component version compatibility.

## 6. Rollback

Rollback restores compatible package/config/schema state where supported. One-way data migrations require explicit release note and pre-upgrade backup/restore plan.

## 7. Failure diagnostics

Every installer returns stable error code, writes sanitized install log and points to remediation. Failed install must not leave ambiguous partially registered identities without cleanup/retry guidance.

## 4. Platform — Operational Runbooks Master

> Consolidated from `Platform_23_Operational_Runbooks.md` v3.1. Requirements in this section remain normative.

## 1. Required runbooks

Core process failure; repeated crash loop; HA failover; Sensor failure; Agent offline; Kafka unavailable/lag; PostgreSQL fault; ClickHouse fault; Valkey fault; ingestion backlog; Core storage ≥80%; Sensor disk ≥80%; certificate expiry/rotation; ITSM connector failure; **SNMP poll backlog/degradation; SNMPv3 authentication/engine/time-window failure; SNMP credential rotation; GETBULK/tooBig incompatibility; counter reset/discontinuity anomaly; trap/inform storm or receiver failure; MIB/Capability Pack mapping failure; interface-identity reconciliation conflict**; package upgrade failure; backup failure; restore; DR activation; corrupted/invalid Device Pack; licensing expiry; telemetry-quality degradation; support-bundle generation.

## 2. Runbook structure

Every runbook states trigger, impact, immediate safety action, diagnostic evidence, commands/UI path, remediation, verification, escalation and audit/support evidence. It must distinguish automated self-heal from operator-required intervention.

## 3. Logging

Support teams should first use Platform Health/Audit/Support Bundle rather than ad-hoc shell access. Shell commands are documented only where necessary and must not expose secrets.


## 4. SNMP operational runbook supplement — v3.2.2

SNMP operational diagnostics use Platform Health, device/profile Monitoring Quality and the in-product SNMP Test before ad-hoc shell tools where possible.

Runbooks shall distinguish at minimum:

- target/path unreachable versus SNMP protocol unavailable;
- timeout versus wrong community/v3 username/auth/privacy/context versus authorization/no-access;
- `unknownEngineID` / `notInTimeWindow` / reboot-time resynchronization;
- `tooBig` / bulk-planning incompatibility versus general timeout;
- unsupported optional OID versus mandatory Pack mapping failure;
- raw counter wrap/reset/discontinuity versus genuine traffic-rate change;
- interface ifIndex renumbering versus genuine interface replacement;
- routine trap burst versus sustained trap storm/receiver overload;
- trap/Inform receiver loss versus monitored-device operational failure;
- Inform durable-acceptance failure versus enrichment/downstream pipeline lag;
- Credential Profile rotation/application drift;
- MIB compile/dependency/provenance failure;
- Sensor worker/backlog/capacity pressure.

Remediation must not insert hard-coded communities or downgrade SNMP security silently. Any temporary legacy mode change is authorized/audited and verified. Routine runbooks do not use SNMP SET; configuration-changing SET belongs to approved NCM/remediation procedure.

Support bundles include sanitized SNMP component health, poll-plan/profile/version identifiers, stable error counts, queue/backlog, last success/failure, trap receiver/storm counters, counter-quality statistics and MIB/Pack version/provenance references, but never community/auth/privacy secret values.


## Timezone / scheduler / HA operations notes — v3.2.2 handoff

- IANA TZDB must be maintained as part of supported platform/runtime updates.
- Host timezone does not define product semantics for telemetry, alerts, SLAs or schedules.
- Schedule and timezone preference changes are auditable.
- Scheduler recovery does not mass-catch-up missed history or fabricate samples.
- Two-Core HA remains uncertified until Platform_14 gates pass; operators must not treat documentation as production HA certification.
- No new infrastructure service is introduced by this baseline.

## 5. Linux Agent eBPF and active-path operational runbook supplement — v3.2.2

> **Reconciliation reference:** Issue #142 (Master Chat comments `5909076167` and `5909787672`). Target Phase-1 capabilities are subject to applicable implementation, benchmark, and certification gates.

### 5.1 Linux Agent eBPF operational states and runbooks

The Linux Agent exposes discrete operational capability states for eBPF:

- `UNSUPPORTED`: Host kernel version or configuration falls outside the certified Ubuntu 24.x/RHEL 9.x compatibility matrix or lacks required eBPF/BTF features (e.g. `CONFIG_DEBUG_INFO_BTF` disabled). The Agent continues normal metric/inventory collection; eBPF is disabled without operator intervention.
- `UNAVAILABLE`: Kernel supports eBPF but runtime probe attachment or ELF loading failed. Runbook: inspect `/var/log/platform/agent/agent.log` for verifier errors, check kernel lock-down / security modules (SELinux/AppArmor), and verify agent package integrity.
- `PRIVILEGE_DENIED`: Agent service lacks required Linux capabilities (`CAP_BPF`, `CAP_PERFMON`, or `CAP_SYS_ADMIN`). Runbook: verify `systemd` unit capability settings (`AmbientCapabilities=CAP_BPF CAP_PERFMON CAP_NET_RAW`); do not grant full root privileges when capabilities suffice.
- `KERNEL_INCOMPATIBLE`: Kernel BTF layout differs from packaged CO-RE expectations or verifier rejects program on host kernel patchlevel. Runbook: update agent package or submit kernel profile; base monitoring continues unaffected.
- `RESOURCE_GUARDED`: Agent runtime detects host memory pressure, CPU threshold breach, or kernel map saturation. eBPF collection drops or throttles aggressively; `shed_count` and `resource_guarded` counters increment. Base telemetry and transport are protected. Runbook: inspect high-cardinality workloads, short-lived socket storms, or tune aggregation window settings.

Support bundles collect eBPF operational status, map usage, dropped/shed counters, and kernel version metadata without capturing packet payloads or environment secrets.

### 5.2 Active path degradation runbook

- **Incomplete traceroutes:** Intermediate hops returning `*` (ICMP time-exceeded suppressed) must not be treated as path failure if destination is reachable. Operators should verify destination probe results and edge status.
- **Hidden hops and ECMP variance:** Alternating path fingerprints across probes indicate multi-path routing (ECMP/flow-hashing), not path flapping. The UI and alerts group ECMP path variants under the canonical path identifier.
- **Multi-vantage contradiction:** Different Sensors reporting conflicting reachability to the same target indicates localized network partition, WAN path failure, or upstream routing issues. The incident console correlates multi-vantage evidence to identify the fault domain.

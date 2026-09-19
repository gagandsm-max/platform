# Platform — Installation, Upgrade, Rollback & Operational Runbooks

**Document ID:** PLATFORM-OPSINSTALL-001
**Version:** 3.2.1
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.1 document preserves the normative content of the listed v3.1 source documents (via v3.2). Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

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

Install Sensor package, provide current Master Core address/enrollment material and role/site/pool inputs. Sensor performs secure handshake, receives identity/certificate/configuration and joins the appropriate Sensor Pool. Core remains authoritative for assignment/policy. Production role selection explicitly distinguishes `NETWORK_SENSOR`, `INFRASTRUCTURE_SENSOR`, `FLOW_SENSOR`, `LOG_SENSOR` and other certified profiles. Flow/Log roles are exclusive; installer/readiness checks verify that incompatible data-plane modules are disabled and that required listener ports, disk/spool and network capacity are available.

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

Core process failure; repeated crash loop; HA failover; Sensor failure; Agent offline; Kafka unavailable/lag; PostgreSQL fault; ClickHouse fault; Valkey fault; ingestion backlog; Core storage ≥80%; Sensor disk ≥80%; certificate expiry/rotation; ITSM connector failure; **Flow Sensor receiver/socket drop; unknown/quarantined exporter; template missing/churn; sequence gaps/exporter reset; sampling unknown/change; Flow spool/backlog/replay; Flow enrichment identity conflict; Flow-vs-SNMP coverage degradation; Flow role misconfiguration; Log Sensor role misconfiguration**; package upgrade failure; backup failure; restore; DR activation; corrupted/invalid Device Pack; licensing expiry; telemetry-quality degradation; support-bundle generation.

## 2. Runbook structure

Every runbook states trigger, impact, immediate safety action, diagnostic evidence, commands/UI path, remediation, verification, escalation and audit/support evidence. It must distinguish automated self-heal from operator-required intervention.

## 3. Logging

Support teams should first use Platform Health/Audit/Support Bundle rather than ad-hoc shell access. Shell commands are documented only where necessary and must not expose secrets.

## 5. Dedicated Sensor role conversion and Flow operations supplement

Production `FLOW_SENSOR`/`LOG_SENSOR` role conversion is a governed operation:

1. place Sensor in drain/maintenance;
2. stop new source/job assignment;
3. verify and drain or explicitly disposition durable spool/backlog;
4. remove incompatible source assignments/listeners;
5. apply signed/effective role configuration;
6. restart/reconfigure required modules;
7. run role-specific listener/storage/network/capacity readiness checks;
8. activate compatible assignments only after health passes;
9. audit operator, reason, old/new role and queue disposition.

Flow runbooks distinguish exporter silence from no traffic, packet/socket loss from decoder rejection, missing templates from exporter failure, sequence reset from packet loss, sampling change from genuine traffic change, stale template from current generation, identity-enrichment conflict from raw source truth, and downstream storage failure from source collection failure.

Support bundles include sanitized role, listener/exporter IDs, protocol/version, observation domains, template hashes/state, sampling state, sequence/drop counters, queue/spool/backlog, last record, replay/drain and downstream lag. They do not expose sensitive user/session attributes beyond authorized diagnostic need.
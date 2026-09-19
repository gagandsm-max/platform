# Platform — Installation, Upgrade, Rollback & Operational Runbooks

**Document ID:** PLATFORM-OPSINSTALL-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.2 document preserves the normative content of the listed v3.1 source documents (via v3.2) and adds the v3.2.2 dedicated Flow/Log Sensor installation and Flow operational-runbook supplement. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

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

For production high-rate roles the installer/enrollment workflow presents explicit `Flow Monitoring Sensor` and `Log Monitoring Sensor` choices using the same Sensor package. Selected role is recorded in authoritative Sensor Control state and only allowed modules/listeners/workers are activated.

A production Flow Sensor does not activate Log/SNMP/discovery/NCM/synthetic workers. A production Log Sensor does not activate Flow/SNMP/discovery/NCM/synthetic workers. Combined high-rate Flow+Log mode, if retained for engineering convenience, is labelled development/lab/test and is not accepted as production configuration.

## 3. Agent installation

Linux Agent native package or Windows MSI/BAT wrapper installs service, enrolls against current Master Core, receives identity and is directed by Core to the appropriate Sensor. Agent retains enough information to reconnect after reboot while Core may revise assignment.

## 4. Auto-start

Installers enable services automatically. Reboot test is mandatory before installation certification.

## 5. Upgrade

Preflight → capacity/free-space check → package signature → state/config backup → drain/standby orchestration → package update → schema migration → service start → health verification → compatibility verification → completion.

Sensor role conversion uses the same controlled lifecycle but is not a simple live toggle: stop new assignments/source activation → drain/reassign sources where supported → flush or durably account for local spool → verify no incompatible assignments remain → apply role/configuration → restart/reinitialize role modules as required → validate source/listener and Core ingestion health.

HA Core upgrades are rolling where architecture permits: upgrade Standby, verify, controlled switchover, upgrade former Active. Exact data-service upgrade sequencing follows component version compatibility.

## 6. Rollback

Rollback restores compatible package/config/schema state where supported. One-way data migrations require explicit release note and pre-upgrade backup/restore plan.

## 7. Failure diagnostics

Every installer returns stable error code, writes sanitized install log and points to remediation. Failed install must not leave ambiguous partially registered identities without cleanup/retry guidance.

## 4. Platform — Operational Runbooks Master

> Consolidated from `Platform_23_Operational_Runbooks.md` v3.1. Requirements in this section remain normative.

## 1. Required runbooks

Core process failure; repeated crash loop; HA failover; Sensor failure; Agent offline; Kafka unavailable/lag; PostgreSQL fault; ClickHouse fault; Valkey fault; ingestion backlog; Core storage ≥80%; Sensor disk ≥80%; certificate expiry/rotation; ITSM connector failure; **Flow Sensor listener/receiver failure; unknown/quarantined exporter; template missing/churn/invalid state; sampling unknown/change; sequence gaps/exporter reset; UDP/socket/queue loss; exporter flood/fairness/rate-limit; Flow spool pressure/replay backlog; Flow enrichment/DNS/session/Geo/ASN catalogue degradation; Flow-vs-SNMP coverage mismatch; Flow role conversion; Flow source redundancy/failover**; **Log Sensor role isolation/spool pressure**; package upgrade failure; backup failure; restore; DR activation; corrupted/invalid Device Pack; licensing expiry; telemetry-quality degradation; support-bundle generation.

## 2. Runbook structure

Every runbook states trigger, impact, immediate safety action, diagnostic evidence, commands/UI path, remediation, verification, escalation and audit/support evidence. It must distinguish automated self-heal from operator-required intervention.

## 3. Logging

Support teams should first use Platform Health/Audit/Support Bundle rather than ad-hoc shell access. Shell commands are documented only where necessary and must not expose secrets.


## 5. Flow Sensor operational supplement — v3.2.2

Operational diagnosis shall distinguish exporter/source failure, template/sampling/sequence problems, Sensor receiver loss, spool/backpressure and downstream pipeline failure from real traffic changes.

Support evidence includes sanitized Flow Sensor role/config revision, listener/exporter IDs, protocol/version, observation-domain counts, template state/churn/misses, sampling state/history, datagrams/records/bytes rate, socket/queue drops, malformed/rejected/quarantined/rate-limited counters, sequence-gap/reset state, spool depth/oldest age, Core upload/replay state, ClickHouse/Kafka lag, enrichment-source health and Flow Coverage evidence. Customer traffic payloads and user-attribution data are minimized according to RBAC/privacy policy.

Runbooks shall never interpret missing Flow as proof of zero traffic without corroboration. Temporary source acceptance or rate-limit changes require authorization/audit. Public-IP Geo and user attribution are treated as evidence with provenance/confidence, not absolute truth.

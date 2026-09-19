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

Install Sensor package, provide current Master Core address/enrollment material and role/site/pool inputs. Sensor performs secure handshake, receives identity/certificate/configuration and joins the appropriate Sensor Pool. Core remains authoritative for assignment/policy.

Production installer/enrollment exposes the Sensor role explicitly. `FLOW` and `LOG` are exclusive production roles in the common Sensor package. The effective configuration starts only modules permitted by the selected role and rejects incompatible assignments/listeners. A `COMBINED_DEV` or equivalent mixed role, if shipped, is clearly marked not production certified.

For FLOW Sensor activation, readiness validates listening interfaces/ports, exporter/source policy, expected rate/capacity inputs, socket/kernel limits, role-local spool/disk and Core ingestion path. For LOG Sensor activation, readiness validates configured log transports/sources, connection/parser capacity, role-local spool and applicable forwarding path.

## 3. Agent installation

Linux Agent native package or Windows MSI/BAT wrapper installs service, enrolls against current Master Core, receives identity and is directed by Core to the appropriate Sensor. Agent retains enough information to reconnect after reboot while Core may revise assignment.

## 4. Auto-start

Installers enable services automatically. Reboot test is mandatory before installation certification.

## 5. Upgrade

Preflight → capacity/free-space check → package signature → state/config backup → drain/standby orchestration → package update → schema migration → service start → health verification → compatibility verification → completion.

A production Sensor role change uses a separate governed workflow:

```text
request role change
→ reject new incompatible assignments/sources
→ drain/reassign active sources/work
→ flush or explicitly account for role-local durable backlog
→ verify no incompatible active assignment/listener
→ apply role/configuration
→ restart/reconcile where required
→ role-specific readiness/capacity validation
→ activate
```

Role change is not used as a shortcut around capacity planning or HA/redundancy. Exporters/log sources are reconfigured/reassigned according to their protocol-specific failover behavior before the old role is removed.

HA Core upgrades are rolling where architecture permits: upgrade Standby, verify, controlled switchover, upgrade former Active. Exact data-service upgrade sequencing follows component version compatibility.

## 6. Rollback

Rollback restores compatible package/config/schema state where supported. One-way data migrations require explicit release note and pre-upgrade backup/restore plan.

## 7. Failure diagnostics

Every installer returns stable error code, writes sanitized install log and points to remediation. Failed install must not leave ambiguous partially registered identities without cleanup/retry guidance.

## 4. Platform — Operational Runbooks Master

> Consolidated from `Platform_23_Operational_Runbooks.md` v3.1. Requirements in this section remain normative.

## 1. Required runbooks

Core process failure; repeated crash loop; HA failover; Sensor failure; Agent offline; Kafka unavailable/lag; PostgreSQL fault; ClickHouse fault; Valkey fault; ingestion backlog; Core storage ≥80%; Sensor disk ≥80%; certificate expiry/rotation; ITSM connector failure; **Flow exporter no-data/unknown-source; Flow template missing/churn; sampling unknown/change; sequence gaps/exporter reset; Flow receiver/socket/queue drops; Flow Sensor overload/spool pressure; Flow identity/enrichment conflict; Flow redundancy/failover; Log Sensor overload/spool pressure; production Sensor role violation/failed role change**; package upgrade failure; backup failure; restore; DR activation; corrupted/invalid Device Pack; licensing expiry; telemetry-quality degradation; support-bundle generation.

## 2. Runbook structure

Every runbook states trigger, impact, immediate safety action, diagnostic evidence, commands/UI path, remediation, verification, escalation and audit/support evidence. It must distinguish automated self-heal from operator-required intervention.

## 3. Logging

Support teams should first use Platform Health/Audit/Support Bundle rather than ad-hoc shell access. Shell commands are documented only where necessary and must not expose secrets.


## 5. Dedicated Flow/Log Sensor operational supplement — v3.2.2

Flow runbooks distinguish at least:

- exporter/source unreachable or no recent data;
- unknown/quarantined exporter versus expected configured source;
- packets received but template missing/invalid;
- template churn/ID reuse/exporter restart;
- sequence gaps versus Platform receiver drops;
- sampling unknown/rate change versus genuine traffic change;
- socket/kernel receive-buffer exhaustion;
- decoder/normalizer queue backlog;
- per-exporter throttling/fairness intervention;
- role-local spool pressure and remaining certified buffer horizon;
- Core/Kafka/ClickHouse downstream degradation;
- temporal identity/user/DNS/Geo enrichment unknown/conflict;
- Flow Coverage discrepancy versus interface counters;
- duplicate-export/multi-collector failover behavior.

Log runbooks remain separate and a Log storm shall not be remediated by moving Log work onto a Flow or Network Sensor.

Support bundles for a FLOW Sensor include sanitized role/config version, listener/source/exporter identifiers, protocol/version, observation-domain/template counts and ages, sampling state, sequence/drop counters, socket/queue health, decoder lag, records/bytes rate, spool/backlog, upload/downstream health and recent stable error codes. User identity evidence is minimized; credentials and unnecessary raw Flow payloads are excluded.

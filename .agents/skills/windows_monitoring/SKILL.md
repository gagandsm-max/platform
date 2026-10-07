---
name: windows_monitoring
description: Windows Agent service, MSI/BAT install, event/perf evidence and privacy-bounded endpoint collection.
---

<!-- generated from project_agents/skills/windows_monitoring/SKILL.md sha256:c5fd536e1a3991a0c44e5167185e410be28547baf39d3bd37d192014f028e578 -->

# windows_monitoring

## Purpose

Monitor Windows hosts via Agent/remote methods with stable identity, least-privilege collection and version-honest semantics—without equating collector failure to host DOWN or widening into surveillance.

## Use when

Windows Agent service/MSI work, Event Log/PerfMon/WMI/CIM collection, Windows service monitoring, or Windows host inventory/identity. Required for Windows Agent paths.

## Authoritative inputs

Always read:

1. Existing Windows Agent/collector code and MSI/BAT packaging on the exact head
2. `agent_architecture` for identity/spool/mTLS
3. Privacy-bounded collection rules (`Platform_20` / Agent privacy)
4. Credential Profile guidance for remote Windows methods if used
5. Tested Windows version matrix (do not assume all SKUs)
6. `references/agent-vs-winrm-wmi.md` and fixtures/evals for WinRM/WMI auth, firewall and SCM recovery

If WinRM remote-only, WMI, or specific Event Log channels are unimplemented, mark `UNVERIFIED`.

## Invariants

- Stable Windows host identity via Agent/`entity_id`—not DHCP IP.
- Agent vs remote methods declared; MSI/BAT install/uninstall and service auto-start are packaging concerns with evidence.
- Windows services, Event Log, PerfMon/WMI/CIM (where applicable), process and disk/volume metrics follow implemented sources only.
- Reboot and time sync are operational signals.
- Domain/workgroup context recorded as observation—not sole identity.
- Credentials least privilege; UAC/WinRM/firewall constraints respected.
- Service-account rotation without embedding secrets in scripts committed to git.
- Privacy/data minimization: operational telemetry only.
- **Collector failure ≠ host DOWN** without authorized evidence contract.
- Windows version differences explicit in support claims.

## Required procedure

1. Confirm Agent/remote capabilities on this SHA; list `UNVERIFIED` APIs/channels.
2. Verify identity continuity across IP change and reinstall reconciliation (`agent_architecture`).
3. Validate service install, auto-start after reboot, and supervised recovery diagnostics where packaging claims it.
4. Implement/verify Event Log/Perf counters with size/rate bounds; redact sensitive payloads.
5. For remote methods: document WinRM/firewall/UAC requirements; fail closed on auth errors.
6. Separate Agent stop/auth failure from host DOWN in alert paths.
7. Test on claimed Windows versions or mark gaps.
8. Ensure upgrade preserves unacked spool and identity keys.

## Negative / failure cases

- Host DOWN only because Agent service stopped.
- Collecting full process command lines with passwords.
- Hard-coding domain admin for collectors.
- Claiming Server 2012 support without tests.
- Deleting local spool on intermittent connectivity.

## Security / tenant implications

Windows Agents handle keys—protect ACLs. Event logs may contain secrets/PII—minimize and redact. WinRM exposure is a threat-model topic. Tenant isolation on enrollment. Code signing for MSI where required by release process.

## Required evidence

- Method/version matrix + `UNVERIFIED`
- Identity/spool/ACK tests
- Service recovery / reboot notes
- DOWN-separation statement
- Exact SHA

## Stop conditions

Stop when:

- IP-only identity
- Pre-ACK spool deletion
- Over-privileged default accounts required
- Version support over-claimed
- Surveillance-class collection requested

## Anti-patterns

- Disabling code signing “for speed”
- Using Domain Admin as the Agent account
- Writer self-certifying Windows estate readiness

## Handoff / output

Return: collection model; packaging/identity evidence; gaps; nextOwner (Windows Agent verifier / AppSec). Not MERGE_READY alone.

## Non-ownership

Does not own Linux monitoring, invent full EDR/SIEM, or bypass `agent_architecture`. Owns Windows host monitoring discipline.

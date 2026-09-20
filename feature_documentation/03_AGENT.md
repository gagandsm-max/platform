# Agent

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 03, 04, 05, 06, 09, 10, 11, 14, 17, 18, 20, 22, 23  
**Canonical enhancement:** PR #68 merged into the baseline.

## Product objective

Provide a secure Windows/Linux endpoint and infrastructure collection runtime for telemetry that is best obtained locally, while keeping monitoring lightweight, explainable and resilient.

## Identity and enrollment

Agent identity is anchored to canonical `entity_id` plus unique `agent_id` and mTLS identity. Hostname is a human-readable technical identity; IP/MAC are mutable observations.

Enrollment uses Core authority, establishes authenticated transport and enters governed onboarding/review. Reinstall, reimage, clone and stable-identifier conflicts require reconciliation; they must not silently merge or duplicate active endpoints.

## Supported operating model

- Windows Agent: compiled code-signed Windows Service, MSI package plus BAT wrappers for scripted deployment.
- Linux Agent: native service packaged for supported Debian/Ubuntu and RHEL-family systems.
- Auto-start after reboot, supervised recovery and visible crash-loop diagnostics.
- Outbound secure control/telemetry path; no browser-to-agent direct control.

## Monitoring scope

Agent telemetry can include host CPU/scheduler/pressure, memory/swap/paging, disk/filesystem/block I/O, NIC/network quality, processes, services, selected OS/event/application logs, local reachability and policy-authorized diagnostics.

Agent and agentless monitoring are distinct certification dimensions but normalize to the same canonical host/process/service evidence model.

## Endpoint Diagnostic Snapshot (EDS)

EDS is bounded evidence acquisition triggered by qualified policy breach or authorized investigation. It is not endpoint-authoritative RCA.

Requirements include:

- reuse normal telemetry for trigger decisions;
- hysteresis/recovery/cooldown;
- bounded duration, payload and CPU/memory cost;
- typed evidence rather than arbitrary shell dumping;
- no always-running LLM/GNN/foundation model on normal Agents;
- near-zero incremental idle overhead target plus benchmarked transient cost.

## Reliability

Agent keeps bounded local buffering, reconnect/replay and multiple endpoint/failover behavior as defined by the common durability authority. Collection degradation is distinct from endpoint-down.

## Security

Least privilege, mTLS identity, signed packages, secret redaction, bounded diagnostics, tenant/resource authorization and auditable actions are mandatory. Agent must not become an unrestricted remote shell or surveillance tool.

## Privacy

Endpoint DEX and identity/session telemetry are operational signals only. Keystrokes, content, screenshots, microphone/camera capture and employee productivity scoring are outside approved scope.

## Flow/user enrichment role

Where policy permits, Agent login/session telemetry can provide time-bounded endpoint-to-user evidence for Flow enrichment. This association must carry source/time/confidence and may not be inferred from directory existence alone.

## Implementation note

This brief reflects merged PR #68 architecture. Exact implemented collectors, Windows/Linux parity, packaging and EDS runtime state must still be verified from source and certification evidence.

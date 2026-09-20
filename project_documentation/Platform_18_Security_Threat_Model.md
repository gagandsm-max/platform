# Platform — Security Threat Model

**Document ID:** PLATFORM-THREAT-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference

## 1. Assets

Credentials, certificates/private keys, tenant data, telemetry, **SNMP communities/v3 auth/privacy secrets, MIB/Capability Pack mappings, trap source/security context**, network configurations, privileged CLI/NCM access, audit records, licensing, AI context and software supply chain.

## 2. Trust boundaries

Browser↔Core; Sensor/Agent↔Core ingestion/control; **Network/Streaming Sensor↔SNMP-managed device/notification source**; Core↔data services; Core↔ITSM/cloud/AI; NCM worker↔managed device; package/update channel; tenant boundary; HA peer boundary.

## 3. Threat classes

Spoofed Sensor/Agent enrollment, credential theft, tenant breakout, command injection, malicious Device Pack/connector, supply-chain dependency compromise, Kafka/event spoofing, replay, insecure legacy protocol misuse, **SNMP community/auth/privacy secret leakage, SNMPv3 downgrade/misconfiguration, engineID/time-window abuse or confusion, forged v1/v2c traps, trap/inform floods, malicious/oversized varbind or MIB content, OID/poll amplification, unbounded GETBULK/WALK, credential cross-tenant reuse, arbitrary SNMP SET/configuration mutation, counter/identity poisoning**, log secret leakage, storage exhaustion, privilege escalation, destructive NCM action, poisoned AI context, GNN/AI over-trust, malicious upgrade, arbitrary-target diagnostic abuse/SSRF-like pivoting, cross-tenant diagnostic execution, diagnostic amplification/resource exhaustion, CLI session hijack, one-time credential capture/persistence, command-policy bypass and investigation-evidence tampering.

## 4. Key mitigations

mTLS identity, single-use enrollment tokens, encrypted secret storage, server-side RBAC/resource scope, maker-checker, command policy, signed packages, SBOM/provenance, audit, bounded inputs/queries, schema validation, content redaction, data-quality/confidence handling, **governed Credential Profile references, secret-redacted SNMP jobs, preferred v3 authPriv SHA-2/AES policy, explicit legacy-mode warning/audit, bounded OIDs/results/GETBULK/WALK/retries, per-device worker limits, trap source/security classification, trap storm/rate controls, Inform durable local acceptance before ACK, MIB provenance/dependency validation, typed exception/counter quality, interface-identity reconciliation and separation of SNMP SET into privileged NCM/remediation**, AI tool guardrails, INCONCLUSIVE RCA state, canonical-target authorization, Sensor/Agent-mediated diagnostics rather than browser-direct target sockets, hard diagnostic timeout/rate/concurrency/fan-out limits, cancellation, source/vantage provenance, ephemeral one-time CLI credentials, WebSocket/session binding and expiry, step-up controls for privileged commands, tamper-evident investigation/audit references and separation of monitored versus on-demand evidence.

## 5. Development threat controls

Docker development networks are not trusted production boundaries. Test secrets only. No production credentials in local images/volumes. Build/signing credentials stay outside normal development containers.

---

## 6. Endpoint diagnostics and local-inference threat extension

Additional threats and mandatory controls:

| Threat | Required control |
|---|---|
| Malicious/modified diagnostic definition becomes remote-code execution | Signed/versioned Platform-controlled definitions, allowlisted fixed operations + structured parameters, schema/parameter validation, exact compatibility checks and audit; no `sh -c`/`bash -c`/arbitrary PowerShell facility for routine monitoring/EDS |
| Agent EDS used for local privilege escalation | Least-privilege Agent identity, no arbitrary shell/script upload, explicit privilege requirements, fail/partial rather than silent elevation |
| WinRM/SSH diagnostics become generic administrative backdoor | Security-profile scoped credentials, read-only query packs, host/certificate validation, operation allowlists, timeout/output bounds and no browser exposure of reusable credentials |
| Stale/revoked/expired/replayed diagnostic authority executes | Core-issued assignment/policy/control generation fencing; fail closed on obsolete authority; provenance retains authorizing generation |
| Diagnostic storm worsens an outage or starves routine telemetry | Per-endpoint/site/tenant concurrency/rate budgets, cooldown/coalescing, resource guard, cancellation, explicit incomplete/suppressed evidence and lower scheduling priority than routine monitoring/telemetry durability |
| Sensitive endpoint data leaks through evidence | Data minimization, default exclusion of environment/user content/arbitrary files/packet payloads/unrestricted command lines, bounded logs and redaction |
| Fabricated/replayed EDS evidence poisons RCA | mTLS identity, assignment authority, idempotency/replay protection, schema validation, source/vantage provenance, immutable/auditable evidence references |
| Endpoint capture-eligibility/indicator is promoted to false causal truth | Capture-eligibility is not host-health authority; typed factual evidence only; Core contradiction/causal evaluation; `INCONCLUSIVE` support; no unsupported free-form cause/fix emitted as fact |
| Local/private LLM reaches device credentials or control plane | AI Gateway mediation, bounded authorized context, no credential forwarding, deterministic tool firewall, no direct Agent/Sensor/device network authority |
| Optional inference node becomes hidden third Core/state authority | No authoritative database/quorum/fencing role, stateless inference boundary, explicit failure isolation and authentication only through approved AI Gateway contract; mode is conditional and not required for EDS/base monitoring |
| Model/runtime compromise affects monitoring | Model/runtime supply-chain validation, sandbox/resource limits, optional-service isolation, no mandatory dependency for deterministic monitoring/RCA |

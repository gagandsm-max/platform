# Platform — Security Threat Model

**Document ID:** PLATFORM-THREAT-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference

## 1. Assets

Credentials, certificates/private keys, tenant data, telemetry, network configurations, privileged CLI/NCM access, audit records, licensing, AI context and software supply chain.

## 2. Trust boundaries

Browser↔Core; Sensor/Agent↔Core ingestion/control; Core↔data services; Core↔ITSM/cloud/AI; NCM worker↔managed device; package/update channel; tenant boundary; HA peer boundary.

## 3. Threat classes

Spoofed Sensor/Agent enrollment, credential theft, tenant breakout, command injection, malicious Device Pack/connector, supply-chain dependency compromise, Kafka/event spoofing, replay, insecure legacy protocol misuse, log secret leakage, storage exhaustion, privilege escalation, destructive NCM action, poisoned AI context, GNN/AI over-trust, malicious upgrade, arbitrary-target diagnostic abuse/SSRF-like pivoting, cross-tenant diagnostic execution, diagnostic amplification/resource exhaustion, CLI session hijack, one-time credential capture/persistence, command-policy bypass and investigation-evidence tampering.

## 4. Key mitigations

mTLS identity, single-use enrollment tokens, encrypted secret storage, server-side RBAC/resource scope, maker-checker, command policy, signed packages, SBOM/provenance, audit, bounded inputs/queries, schema validation, content redaction, data-quality/confidence handling, AI tool guardrails, INCONCLUSIVE RCA state, canonical-target authorization, Sensor/Agent-mediated diagnostics rather than browser-direct target sockets, hard diagnostic timeout/rate/concurrency/fan-out limits, cancellation, source/vantage provenance, ephemeral one-time CLI credentials, WebSocket/session binding and expiry, step-up controls for privileged commands, tamper-evident investigation/audit references and separation of monitored versus on-demand evidence.

## 5. Development threat controls

Docker development networks are not trusted production boundaries. Test secrets only. No production credentials in local images/volumes. Build/signing credentials stay outside normal development containers.

---

## 6. Endpoint diagnostics and local-inference threat extension

Additional threats and mandatory controls:

| Threat | Required control |
|---|---|
| Malicious/modified diagnostic definition becomes remote-code execution | Signed/versioned Platform-controlled definitions, allowlisted operations, schema/parameter validation, exact compatibility checks and audit |
| Agent EDS used for local privilege escalation | Least-privilege Agent identity, no arbitrary shell/script upload, explicit privilege requirements, fail/partial rather than silent elevation |
| WinRM/SSH diagnostics become generic administrative backdoor | Security-profile scoped credentials, read-only query packs, host/certificate validation, operation allowlists, timeout/output bounds and no browser exposure of reusable credentials |
| Diagnostic storm worsens an outage | Per-endpoint/site/tenant concurrency/rate budgets, cooldown/coalescing, resource guard, cancellation and lower scheduling priority than routine monitoring |
| Sensitive endpoint data leaks through evidence | Data minimization, default exclusion of environment/user content/arbitrary files/packet payloads/unrestricted command lines, bounded logs and redaction |
| Fabricated/replayed EDS evidence poisons RCA | mTLS identity, assignment authority, idempotency/replay protection, schema validation, source/vantage provenance, immutable/auditable evidence references |
| Endpoint indicator is promoted to false causal truth | Typed factual evidence only; Core contradiction/causal evaluation; `INCONCLUSIVE` support; no unsupported free-form cause/fix emitted as fact |
| Local/private LLM reaches device credentials or control plane | AI Gateway mediation, bounded authorized context, no credential forwarding, deterministic tool firewall, no direct Agent/Sensor/device network authority |
| Optional inference node becomes hidden third Core/state authority | No authoritative database/quorum/fencing role, stateless inference boundary, explicit failure isolation and authentication only through approved AI Gateway contract |
| Model/runtime compromise affects monitoring | Model/runtime supply-chain validation, sandbox/resource limits, optional-service isolation, no mandatory dependency for deterministic monitoring/RCA |


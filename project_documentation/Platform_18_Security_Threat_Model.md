# Platform — Security Threat Model

**Document ID:** PLATFORM-THREAT-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference

## 1. Assets

Credentials, certificates/private keys, tenant data, telemetry, **SNMP communities/v3 auth/privacy secrets, MIB/Capability Pack mappings, trap source/security context**, network configurations, privileged CLI/NCM access, audit records, licensing, AI context, software supply chain, **host kernel integrity, eBPF maps/programs, and active path topologies**.

## 2. Trust boundaries

Browser↔Core; Sensor/Agent↔Core ingestion/control; **Network/Streaming Sensor↔SNMP-managed device/notification source**; Core↔data services; Core↔ITSM/cloud/AI; NCM worker↔managed device; package/update channel; tenant boundary; HA peer boundary; **Linux Agent↔host kernel/eBPF; Sensor active path probe↔transit network paths**.

## 3. Threat classes

Spoofed Sensor/Agent enrollment, credential theft, tenant breakout, command injection, malicious Device Pack/connector, supply-chain dependency compromise, Kafka/event spoofing, replay, insecure legacy protocol misuse, **SNMP community/auth/privacy secret leakage, SNMPv3 downgrade/misconfiguration, engineID/time-window abuse or confusion, forged v1/v2c traps, trap/inform floods, malicious/oversized varbind or MIB content, OID/poll amplification, unbounded GETBULK/WALK, credential cross-tenant reuse, arbitrary SNMP SET/configuration mutation, counter/identity poisoning**, log secret leakage, storage exhaustion, privilege escalation, destructive NCM action, poisoned AI context, GNN/AI over-trust, malicious upgrade, arbitrary-target diagnostic abuse/SSRF-like pivoting, cross-tenant diagnostic execution, diagnostic amplification/resource exhaustion, CLI session hijack, one-time credential capture/persistence, command-policy bypass, investigation-evidence tampering, **eBPF kernel privilege escalation, bytecode/package tampering, eBPF map/kernel memory exhaustion, ring-buffer event loss, process metadata/payload privacy leakage, rapid short-lived connection cardinality attacks, and active path probe amplification**.

## 4. Key mitigations

mTLS identity, single-use enrollment tokens, encrypted secret storage, server-side RBAC/resource scope, maker-checker, command policy, signed packages, SBOM/provenance, audit, bounded inputs/queries, schema validation, content redaction, data-quality/confidence handling, **governed Credential Profile references, secret-redacted SNMP jobs, preferred v3 authPriv SHA-2/AES policy, explicit legacy-mode warning/audit, bounded OIDs/results/GETBULK/WALK/retries, per-device worker limits, trap source/security classification, trap storm/rate controls, Inform durable local acceptance before ACK, MIB provenance/dependency validation, typed exception/counter quality, interface-identity reconciliation and separation of SNMP SET into privileged NCM/remediation**, AI tool guardrails, INCONCLUSIVE RCA state, canonical-target authorization, Sensor/Agent-mediated diagnostics rather than browser-direct target sockets, hard diagnostic timeout/rate/concurrency/fan-out limits, cancellation, source/vantage provenance, ephemeral one-time CLI credentials, WebSocket/session binding and expiry, step-up controls for privileged commands, tamper-evident investigation/audit references, separation of monitored versus on-demand evidence, **least-privilege capability boundaries (CAP_BPF/CAP_PERFMON where available), kernel verifier validation, BTF/CO-RE signed bytecode where supported, bounded map bounds with eviction, strict data minimization (no payload/TLS/HTTP/SQL), safe executable/service identity correlation (excluding process command lines), cardinality budgets with Top-N overflow shedding, and rate-limited active path probes with flow-stable multipath awareness**.

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


## Display timezone / scheduler / extensibility / ITSM threat notes — v3.2.2 handoff

- Timezone preferences are tenant/user scoped; invalid IANA zones are rejected; display preference cannot modify operational policy without separate authorization.
- Scheduler assignments remain fenced/versioned and tenant-scoped; adaptive/event-triggered refresh is bounded and authorized.
- Webhooks/API Studio require signature/auth, replay protection, SSRF/egress controls, secret handling, tenant binding and rate limits.
- ITSM inbound callbacks are signed/authenticated; remote ticket state cannot override Platform health authority.

## 8. Regional Sensor / enrollment / transport threat extension

Additional threats:

- stolen/replayed Sensor or Agent enrollment token;
- enrollment authority used for the wrong tenant/Region/Site/Pool/role;
- Core receiving or persisting endpoint private-key material;
- certificate/SAN verification disabled for IP/VIP bootstrap convenience;
- stale Sensor-pool membership or Agent assignment regaining authority;
- rogue Sensor claiming another tenant/site/pool;
- plaintext Platform-owned telemetry/control interception, tampering or credential exposure;
- downgrade from mTLS/encrypted Platform transport to insecure mode;
- capacity-placement manipulation that intentionally overloads one Sensor/failure domain.

Mandatory controls:

- short-lived single-use enrollment authority with tenant/deployment and optional Region/Site/Pool/role scope;
- endpoint-local private-key generation and proof-of-possession; Core stores only public certificate/credential metadata required for trust;
- CA/trust and DNS/IP SAN verification remain enabled; bootstrap fails closed on mismatch;
- unique Sensor/Agent service identity, revocation/rotation and auditable lifecycle;
- monotonically advancing assignment/membership generations with stale/replayed authority rejection;
- server-side tenant/site/pool/capability authorization before assignment or telemetry acceptance;
- encrypted authenticated Platform-owned control/telemetry channels; mTLS remains required where specified;
- **no production plaintext Sensor↔Core or Agent↔Sensor mode is approved by this reconciliation**;
- insecure monitored-device protocols are explicit legacy compatibility modes and cannot downgrade Platform-owned trust boundaries;
- placement/capacity inputs are bounded, authoritative and auditable.

## 9. RCA evidence-poisoning / causal-integrity extension

Threats include forged chronology, stale topology, malicious or compromised Capability Pack causal rules, poisoned Similar Incident history, fabricated recovery evidence, confidence manipulation and cross-tenant hypothesis/evidence references.

Controls require immutable evidence references, tenant/RBAC authorization on every evidence lookup, event-time + ingest-time provenance, rule/model/version provenance, explicit contradictory/missing evidence, immutable RCA revisions, bounded counterfactual reasoning, shared-risk provenance/confidence and preservation of `INCONCLUSIVE`. LLM/GNN output remains non-authoritative causal assistance and cannot rewrite source evidence or execute remediation directly.

## 10. Linux Agent eBPF kernel instrumentation & active path threat extension — v3.2.2

> **Reconciliation reference:** Issue #142 (Master Chat comments `5909076167` and `5909787672`). Governed by `REQ-INF-013` (bounded Linux Agent eBPF process-to-network telemetry), `REQ-NET-016` (active path hop/loss/latency/fingerprint observability), and decisions `DEC-ARCH-036` through `DEC-ARCH-042`. Target Phase-1 capabilities are subject to applicable implementation, benchmark, and certification gates.

Additional threats and mandatory controls for Linux kernel instrumentation and active path probing:

| Threat | Risk / Impact | Mandatory control |
|---|---|---|
| **Kernel privilege escalation via eBPF program loader** | Attacker leverages eBPF helper calls or verifier bugs to execute arbitrary kernel code or escape container/cgroup boundaries. | Least privilege enforcement: Linux Agent drops all unneeded capabilities; runs with bounded `CAP_BPF` + `CAP_PERFMON` (or minimum kernel capability equivalents across certified Ubuntu 24.x/RHEL 9.x kernels; or guarded root on older kernels under strict audit). Kernel verifier is never bypassed. eBPF programs use read-only maps where possible. Arbitrary user-supplied BPF bytecode injection is prohibited. |
| **Bytecode / package tampering** | Attacker replaces compiled eBPF `.o` objects on disk with malicious probes to hijack network socket tracing or steal credentials. | Platform-compiled eBPF objects are cryptographically signed during official build; sha256 checksums verified before kernel load. Bytecode is installed to root-owned read-only directories (`/usr/lib/platform/ebpf/`). Unsigned, unverified, or modified objects fail closed with status `UNAVAILABLE` or `PRIVILEGE_DENIED`. |
| **eBPF map / kernel memory exhaustion (Host DoS)** | Rapid socket churn (e.g. port scan or DDoS) allocates excessive entries in kernel BPF hash maps, exhausting kernel slab memory and crashing host OS. | Strict bounded sizing: all BPF maps specify explicit bounded `max_entries` ceilings. Map allocation uses LRU eviction (`BPF_MAP_TYPE_LRU_HASH`) for socket tracking. If kernel memory pressure or map allocation failures occur, Agent enters `RESOURCE_GUARDED` capability state, shedding socket events and emitting explicit shed counters. |
| **Ring-buffer event loss & telemetry poisoning** | High connection event rate overwhelms the BPF perf/ring buffer, leading to silent packet/connection omissions and corrupted network graphs. | Non-blocking ring buffer consumption with explicit ring-buffer drop accounting. Overflows increment `dropped_records_count`. When drops exceed threshold, telemetry quality transitions from `VALID` to `PARTIAL` or `RESOURCE_GUARDED`. Silent dropping is prohibited. |
| **Process metadata privacy leakage & PII capture** | eBPF probes inspect socket buffers, exfiltrating customer PII, HTTP headers, TLS session keys, database SQL statements, or token secrets in environment variables. | **Strict Architectural Data Minimization by Design:** Probe attach points are strictly bounded to socket state lifecycle and socket connection performance metrics (`tcp_info`). Payload memory inspection, packet capture, TLS decryption, HTTP header/body reading, SQL text inspection, and process environment variable scanning are **STRICTLY PROHIBITED**. eBPF evidence excludes command-line text and correlates strictly using safe identity attributes (process name, executable identity, service identity, container/cgroup, network namespace, and ephemeral PID). If another governed process-inventory subsystem separately observes bounded command metadata under explicit policy, eBPF evidence may reference or correlate to that process entity without duplicating command-line text into process-network records. |
| **Rapid connection cardinality attacks** | Ephemeral port churn or randomized target IP floods explode cardinality in Agent local aggregation tables, exhausting Agent RAM. | Local aggregation table cardinality budget: bounded active keys per configurable aggregation interval. Top-N heavy-hitter sorting with explicit overflow bucket (`other_endpoints`). Memory and CPU resource budgets derived from BUILD-019 / BUILD-130 benchmark certification. If memory exceeds limit, aggregation switches to coarse sampling with explicit `SAMPLED`, `PARTIAL`, and `shed_records_count` indicators. |
| **Active path probe amplification & SSRF abuse** | Operator or compromised API schedules high-rate ICMP/TCP/UDP traceroutes toward external targets, causing Platform to participate in DDoS or network scanning. | Tenant/entity authorization on all probe targets. Probes strictly bounded to verified monitored infrastructure entities, cloud endpoints, or licensed outside-in targets. Bounded probe rate limits and scheduling (flow-stable techniques evaluated in BUILD-040). Hard concurrency limits on Sensors. Arbitrary CIDR range sweeping is rejected. |
| **Unsupported kernel crash / panic** | Loading modern eBPF bytecode on non-BTF or outdated kernel causes kernel oops or system panic. | Comprehensive pre-flight compatibility check: probe for BTF (`/sys/kernel/btf/vmlinux`), certified kernel compatibility matrix (Ubuntu 24.x / RHEL 9.x), and required eBPF helper functions. If pre-flight check fails, Agent transitions gracefully to `KERNEL_INCOMPATIBLE` or `UNSUPPORTED` without loading probes. System panics, kernel crashes, or boot loops are strictly prevented. |

---

## Phase-1 endpoint inventory and Triggered Fast RCA threat extension — Issues #169/#170

| Threat | Mandatory control |
|---|---|
| Malicious package metadata causes parser/normalization injection or cardinality exhaustion | Typed bounded parsers; preserve native identity; length/item limits; normalization version/provenance; reject malformed data; no package strings promoted to commands or metric-label explosions |
| Package-manager/registry source unavailable or incomplete snapshot is misread as mass uninstall | Versioned `software_inventory_snapshot` completeness (`COMPLETE|PARTIAL|FAILED`, truncation, `FULL|TARGETED`); only COMPLETE non-truncated authoritative FULL same-source/scope snapshots may infer general absence/removal; failure/permission denial and PARTIAL/FAILED/truncated/TARGETED captures cannot generate mass or unrelated removals |
| Agentless software inventory becomes generic shell/PowerShell | Platform-owned signed/versioned allowlisted query packs, fixed operations + typed parameters, timeout/output bounds, read-only privilege and audit; no user/model supplied command strings |
| Process monitor selector is redirected by PID reuse or ambiguous pattern | PID prohibited as durable identity; stable service/executable selectors; bounded exact-name fallback; ambiguity/reconciliation state rather than silent retargeting |
| Customer selector becomes command/regex injection | No shell expression contract; bounded typed selector schema; canonical escaping/validation; target/resource authorization |
| Criticality mass edit triggers monitoring storm | capacity/MWU preview, admission, versioned desired state, deterministic spreading and rollback; collectors do not self-accelerate from raw Criticality |
| Triggered Fast RCA fans out across estate and worsens outage | endpoint/site/tenant/estate budgets; relationship/capability-aware fan-out; reuse existing evidence; cancellation/deadlines; routine monitoring/durability priority |
| Guest evidence is promoted to hypervisor/hardware truth | source/vantage provenance and type separation; guest cannot author hypervisor/OOB facts; Core contradiction/correlation only |
| Diagnostic evidence leaks software/process/customer data | minimize to approved metadata; no arbitrary files/env/secrets/payloads/unrestricted command lines/logs; RBAC/tenant scope/retention/redaction |

No new privilege, endpoint Python runtime, unrestricted shell/PowerShell facility or endpoint causal AI is authorized by these enhancements.

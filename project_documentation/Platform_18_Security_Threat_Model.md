# Platform — Security Threat Model

**Document ID:** PLATFORM-THREAT-001  
**Version:** 3.3.0
**Status:** Authoritative Mother Reference
**Last reconciliation:** 2026-10-12 — Issue #200/#203 Tranche B Phase 1/1.5 certification-envelope reconciliation (trust boundaries/tenant/NCM/HA peer)

## 1. Assets

Credentials, certificates/private keys, tenant data, telemetry, **SNMP communities/v3 auth/privacy secrets, MIB/Capability Pack mappings, trap source/security context**, network configurations, privileged CLI/NCM access, audit records, licensing, AI context, software supply chain, **host kernel integrity, eBPF maps/programs, and active path topologies**.

## 2. Trust boundaries

Browser↔Core; Sensor/Agent↔Core ingestion/control; **Network/Streaming Sensor↔SNMP-managed device/notification source**; Core↔data services; Core↔ITSM/cloud/AI; NCM worker↔managed device; package/update channel; tenant boundary; HA peer boundary; **Linux Agent↔host kernel/eBPF; Sensor active path probe↔transit network paths**.

**Certification envelope / Issue #200:** Phase 1 certifies one authoritative standalone Core with tenant-aware identity, mTLS Sensor/Agent enrollment, server-side RBAC/resource scope and audit—even for single-customer enterprise—without claiming MSP/shared-customer isolation certification. The HA peer boundary (exactly-two-Core Active/Standby fencing, ambiguous-partition fail-closed, former-Active rejoin protection) is the **Phase 1.5** envelope. HA ≠ DR. No third Core, witness, replicated ingress journal or speculative coordination service is introduced by this threat model.

## 3. Threat classes

Spoofed Sensor/Agent enrollment, credential theft, tenant breakout, command injection, malicious Device Pack/connector, supply-chain dependency compromise, Kafka/event spoofing, replay, insecure legacy protocol misuse, **SNMP community/auth/privacy secret leakage, SNMPv3 downgrade/misconfiguration, engineID/time-window abuse or confusion, forged v1/v2c traps, trap/inform floods, malicious/oversized varbind or MIB content, OID/poll amplification, unbounded GETBULK/WALK, credential cross-tenant reuse, arbitrary SNMP SET/configuration mutation, counter/identity poisoning**, log secret leakage, storage exhaustion, privilege escalation, destructive NCM action, poisoned AI context, GNN/AI over-trust, malicious upgrade, arbitrary-target diagnostic abuse/SSRF-like pivoting, cross-tenant diagnostic execution, diagnostic amplification/resource exhaustion, CLI session hijack, one-time credential capture/persistence, command-policy bypass, investigation-evidence tampering, **eBPF kernel privilege escalation, bytecode/package tampering, eBPF map/kernel memory exhaustion, ring-buffer event loss, process metadata/payload privacy leakage, rapid short-lived connection cardinality attacks, and active path probe amplification**.

## 4. Key mitigations

mTLS identity, single-use enrollment tokens, encrypted secret storage, server-side RBAC/resource scope, maker-checker, command policy, signed packages, SBOM/provenance, audit, bounded inputs/queries, schema validation, content redaction, data-quality/confidence handling, **governed Credential Profile references, secret-redacted SNMP jobs, preferred v3 authPriv SHA-2/AES policy, explicit legacy-mode warning/audit, bounded OIDs/results/GETBULK/WALK/retries, per-device worker limits, trap source/security classification, trap storm/rate controls, Inform durable local acceptance before ACK, MIB provenance/dependency validation, typed exception/counter quality, interface-identity reconciliation and separation of SNMP SET into privileged NCM/remediation**, AI tool guardrails, INCONCLUSIVE RCA state, canonical-target authorization, Sensor/Agent-mediated diagnostics rather than browser-direct target sockets, hard diagnostic timeout/rate/concurrency/fan-out limits, cancellation, source/vantage provenance, ephemeral one-time CLI credentials, WebSocket/session binding and expiry, step-up controls for privileged commands, tamper-evident investigation/audit references, separation of monitored versus on-demand evidence, **least-privilege capability boundaries (CAP_BPF/CAP_PERFMON where available), kernel verifier validation, BTF/CO-RE signed bytecode where supported, bounded map bounds with eviction, strict data minimization (no payload/TLS/HTTP/SQL), safe executable/service identity correlation (excluding process command lines), cardinality budgets with Top-N overflow shedding, and rate-limited active path probes with flow-stable multipath awareness**.

### 4.1 Network Sensor protocol-edge threat clarification — 2026-10-30

- **Overlapping or spoofed Flow exporters:** a bare UDP port/shared source IP is not sufficient tenant or exporter identity. Bind authoritative exporter scope to independently verified ingress topology/routing domain/authorization and authenticated Sensor identity; segregate routing/listeners/VRFs where necessary and fail closed on ambiguous or untrusted sources. Quarantined/unmapped traffic must not enter canonical Flow analytics. Phase 1 covers intra-tenant overlapping RFC1918/VRF spaces; MSP/shared-tenancy isolation certification remains Phase 1.5.
- **Flow template poisoning and false utilization:** stale/reused template IDs, exporter reset, spoofed template packets, conflicting Observation Domains, shared Data/Options Template ID collisions, UDP Template Withdrawal misuse and double-applied/unknown sampling factors can corrupt traffic analytics or leak across tenant scope. Bound parser resources; require live validation after restart; ignore UDP Template Withdrawal; validate session/context/lifetime; keep sFlow sampled-frame evidence distinct from sFlow interface/device Counter samples and from IPFIX/NetFlow counters; do not treat Counter samples as ordinary per-flow facts or scale cumulative interface counters by packet sample rate; capture length is not automatic original frame length; keep immutable raw versus separately identified estimates; track template misses, invalidation, sampling-quality and untrusted-source evidence.
- **Inform ACK/data-loss ambiguity:** unflushed memory buffers or unpersisted drop counters cannot authorize success ACK. Persist unique authoritative receiver `snmpEngineID`; increment/persist boots and reset/re-sync `snmpEngineTime` on reboot (RFC 3414); fail closed/re-initialize if nonvolatile engine state is lost or invalid; separate USM discovery from Inform ACK; SNMPv3 durable idempotency uses ScopedPDU `request-id` + authenticated security/context + payload evidence (not outer `msgID`); SNMPv2c source/community remains weak/non-cryptographic; never silently discard durably ACKed Informs. Bound storm admission, durable aggregation, retransmission, disk I/O and ACK outstanding state; protect critical telemetry/liveness from notification/flow overload with class-aware shedding and respect Sensor 80% governed protection. No hardcoded EPS/IOPS/timeout quota claims.
- **SNMP execution secrets and crash diagnostics:** Go garbage-collected strings cannot be promised to be reliably zeroized. Minimize plaintext lifetime; prevent secrets from reaching logs, support bundles or durable scheduler desired-state files; validate OS-specific dumpability controls rather than forcing untested global directives; enforce file permissions, redaction and independent negative leakage tests. Legacy v1/v2c notification identity stays lower-trust than authenticated SNMPv3 and must not be called cryptographically authenticated.
- **Active-path false divergence:** uncertain multipath / unpinned observations (ECMP, CoPP, MPLS, IPv6 Flow Label, return-path asymmetry) must not be weaponized into false PATH_DIVERGENCE/RTT_SHIFT/LOSS_ANOMALY/DEVICE_DOWN. Fingerprint-only change emission without dual baseline/current comparison eligibility is a false-event risk; absent optional quality metadata must be treated as not comparison-eligible.

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
| **eBPF map / kernel memory exhaustion (Host DoS)** | Rapid socket churn (e.g. port scan or DDoS) allocates excessive entries in kernel BPF hash maps, exhausting kernel slab memory and crashing host OS. | Strict bounded sizing: all BPF maps specify explicit bounded `max_entries` ceilings. Initial BUILD-011 socket-state tracking uses `BPF_MAP_TYPE_HASH` (not silent LRU eviction) with explicit update-failure / drop accounting when insertion fails. If kernel memory pressure or map allocation/update failures occur, Agent enters `RESOURCE_GUARDED` capability state, shedding socket events and emitting explicit shed/loss counters. Silent eviction is prohibited. `BPF_MAP_TYPE_LRU_HASH` may be reconsidered later only if exact or conservatively provable eviction/loss accounting is designed, tested and independently certified. |
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
| Package-manager/registry source unavailable or incomplete snapshot is misread as mass uninstall | Versioned `software_inventory_snapshot` completeness (`COMPLETE|PARTIAL|FAILED`, truncation, `FULL|TARGETED`); only COMPLETE non-truncated authoritative FULL same endpoint+native-source+`inventory_scope` snapshots may infer general absence/removal; failure/permission denial and PARTIAL/FAILED/truncated/TARGETED captures cannot generate mass or unrelated removals |
| Replay/late/out-of-order inventory snapshot rolls current state backwards or fabricates change/lifecycle history | Stable `snapshot_id` across same-capture retry/replay; Core idempotent on snapshot identity; duplicate replay cannot emit duplicate `SOFTWARE_*` or mutate first/last-seen; older FULL after newer FULL may be historical evidence only and cannot replace current authority or reverse events; prefer Agent/Sensor/Diagnostic Job transport ordering over wall-clock-only authority; source/`inventory_scope` definition changes cannot masquerade as mass uninstall/install |
| Agentless software inventory becomes generic shell/PowerShell | Platform-owned signed/versioned allowlisted query packs, fixed operations + typed parameters, timeout/output bounds, read-only privilege and audit; no user/model supplied command strings |
| Process monitor selector is redirected by PID reuse or ambiguous pattern | PID prohibited as durable identity; stable service/executable selectors; bounded exact-name fallback; ambiguity/reconciliation state rather than silent retargeting |
| Customer selector becomes command/regex injection | No shell expression contract; bounded typed selector schema; canonical escaping/validation; target/resource authorization |
| Criticality mass edit triggers monitoring storm | capacity/MWU preview, admission, versioned desired state, deterministic spreading and rollback; collectors do not self-accelerate from raw Criticality |
| Triggered Fast RCA fans out across estate and worsens outage | endpoint/site/tenant/estate budgets; relationship/capability-aware fan-out; reuse existing evidence; cancellation/deadlines; routine monitoring/durability priority |
| Guest evidence is promoted to hypervisor/hardware truth | source/vantage provenance and type separation; guest cannot author hypervisor/OOB facts; Core contradiction/correlation only |
| Diagnostic evidence leaks software/process/customer data | minimize to approved metadata; no arbitrary files/env/secrets/payloads/unrestricted command lines/logs; RBAC/tenant scope/retention/redaction |

No new privilege, endpoint Python runtime, unrestricted shell/PowerShell facility or endpoint causal AI is authorized by these enhancements.

**Windows Agent additional threat/negative-test contract (BUILD-014/015/016/018/019).** A virtual service account may present a domain computer identity to remote resources; local vs. remote identity and egress rights require explicit threat review. Unconditional `LocalSystem`/`SeDebugPrivilege`, broad SCM SDDL ACEs and local performance/event-reader group membership are not safe substitutes for a least-privilege Windows read capability matrix. Per-source denied access must be observable without a broadening privilege fallback. Protect Agent enrollment keys and the NTFS spool with service-scoped ACLs, integrity checks and security-product interference tests; do not disable Defender/EDR or add unapproved global exclusions to pass benchmarks. Volume/provider hangs, reboot/upgrade races, PID recycling, out-of-order inventory and oversized socket tables must not starve SCM control, leak private process/network data, corrupt replay identity, or authorize deleting telemetry before a valid durable ACK. Windows event evidence must retain provider/channel and access/freshness provenance to resist mistaken or attacker-injected causal attribution.



---

## 12. v3.3.0 threat extension — OTLP, integrations, CMDB, NCM assurance and browser evidence

| Threat | Mandatory control outcome |
|---|---|
| Forged external approval | Authenticated connector + expected workflow/approver + exact immutable Change Plan binding + independent local authorization/policy/preconditions. |
| Stale approval replay | External event/version/replay checks plus plan version/target/config/window binding; stale approval rejected. |
| Cross-tenant webhook/event | Tenant derived from authenticated connector/endpoint binding, never arbitrary payload tenant; server authorization before domain update. |
| Sync-loop DoS | Multi-factor event/object/version/correlation/operation/hash/origin/lineage loop prevention plus rate limits. |
| Malicious work note/comment | Treat all external text/HTML as untrusted; sanitize/escape; never parse free text as command, template, target, authorization or executable mapping. |
| Malicious attachment | Explicit feature enablement, type/size limits, safe storage, malware scanning where supported/required, access/TTL/audit; never auto-execute. |
| SSRF/egress abuse | Connector destination allowlists, URL/DNS/IP/redirect validation, proxy policy, private-address controls where applicable and bounded timeouts. |
| CMDB poisoning / wrong CI match | Explicit authority profile, match evidence/confidence, conflict queue, tenant scope, no last-write-wins and no automatic destructive reconciliation. |
| NCM command/template injection | Typed/validated templates and certified Device Pack actions; secret-safe rendering; no ITSM text -> CLI; RBAC/approval/audit. Phase 1 independently certifiable surface is backup/version/diff/compliance/golden-state + controlled manual CLI when credential brokering/RBAC/audit are certified; mass push/canary/automated privileged remediation defaults to Phase 1.5. |
| Firmware substitution/supply-chain | Approved repository/provenance, digest/signature when available, entitlement/compatibility validation, controlled access and audit. |
| Unsafe rollback | Rollback only via certified vendor/device mechanism with preconditions; otherwise stop/escalate. |
| OTLP tenant spoofing | Authenticated server-side source identity; OTel attributes never authorize tenant. |
| OTel PII/secrets | Attribute/body classification, drop/mask/hash policy, bounded retention and AI data boundary before persistence/use where configured. |
| OTel cardinality/volume attack | Per-source/tenant quotas, attribute/key/value/count limits, rate limits, cardinality intervention with visible accounting, bounded queues/backpressure. |
| Compression/payload bomb/parser abuse | Pre/post-decompression limits, parse time/memory bounds, schema validation, malformed input rejection and rate controls. |
| Browser credential leakage | Vault references/ephemeral secrets, worker isolation, no plaintext in logs/screenshots/support bundles, scoped access and rotation. |
| Sensitive screenshots/artifacts | Explicit policy, encryption/access controls, redaction where supported, short TTL/legal-hold handling and audit. |
| Malicious Capability Pack | Signature/integrity/provenance, schema validation, declared privileges/resource budget, compatibility/certification and no secret embedding. |
| Compromised vendor API/account | Least-privilege service account, secret rotation/revocation, rate/circuit controls, audit/conflict visibility; external compromise cannot bypass local privileged policy. |
| Vendor rate-limit exhaustion | Bounded queues, token/rate budgets, Retry-After aware scheduling where available, exponential backoff+jitter and backlog recovery protection. |

Full developer-centric APM, full always-on DPI/NDR and ticket/comment-driven arbitrary execution remain outside the approved trust boundary.

---

## 13. Phase 1 / Phase 1.5 certification envelopes — Issue #200 / #203

Architecture target ≠ implementation ≠ tested ≠ benchmarked ≠ certified ≠ deployed. Mother Reference set version remains **3.3.0** through Tranche B; v3.4.0 is reserved for gated Tranche D.

| Envelope | Security certification emphasis | Explicit nonclaims |
|---|---|---|
| **Phase 1** | Standalone Core; mTLS Sensor/Agent identity; tenant-aware RBAC/resource scope/audit; Credential Profile separation; assignment/fencing stale-work rejection; Inform local ACK != Core durable ACK; `No Data != Device Down` semantic integrity; backup/restore + site-loss DR security hooks | No Core-HA availability claim; no MSP/shared-tenant isolation certification claim; Issue #218 Core ICMP typed-acceptance gap is an integrity/availability risk until closed—not an authorization to invent false DOWN; NCM automation not Phase 1 default |
| **Phase 1.5** | Exactly-two-Core HA peer fencing/split-brain prevention; PG/Kafka/ClickHouse continuity trust; HA deletion-ACK RPO=0 matrix; MSP/shared-tenant adversarial isolation across identity/API/RBAC/credentials/NCM/AI/audit; privileged automation guardrails | Physical fencing/replication mechanisms remain unselected; no third Core/witness/journal preselected; HA does not replace DR |

## Phase 1.5 Serverless / FaaS threat extension — Issue #268

Governing requirement: `REQ-SLS-006`. Status: SPECIFIED / NOT IMPLEMENTED / NOT CERTIFIED.

| Threat | Abuse | Mandatory control |
|---|---|---|
| Long-lived over-privileged cloud keys | Stolen root/access keys enable estate-wide read or invoke | Prefer STS/role, managed identity, workload identity; least privilege; rotation; audit |
| Credential exfiltration to browser | UI/API returns provider secrets | Server-side credential brokering only; never return secrets to browser |
| Secret/env capture | Connector logs function env secrets | Redaction; baseline excludes env secret values |
| Payload/source abuse | Capture request/response bodies or pull source as “monitoring” | Baseline monitoring forbids payload capture and does not require source retrieval |
| Arbitrary invoke | Health checks invoke customer functions | Prohibit arbitrary invocation for health testing |
| SSRF / tenant escape via connector endpoints | Malicious endpoint/identity pivots | Allowlists, DNS/IP/redirect validation, timeouts, tenant binding |
| Provider ID authority override | Same ARN/name across tenants merges inventory | Platform tenant-scoped `entity_id` authority; provider IDs attributes only |
| Cardinality/DoS | High-cardinality tags/log floods exhaust Platform | Budgets, shedding, pagination (`REQ-SLS-007`) |
| False Function DOWN | Connector outage stamped as outage | Monitoring/connector quality ≠ Function DOWN (`REQ-SLS-005`) |

OCI Functions remain out of Phase 1.5 required scope unless later promoted.

---

## Phase 2 Enterprise RFP Readiness Reconciliation — Issue #269

Additional Phase-2 threat classes: malicious/compromised enterprise connector; forged external APM/NPM/DPI/CMDB/SIEM evidence; cross-tenant external ID collision; connector credential theft; API quota exhaustion; high-cardinality/vendor-payload exhaustion; regulated data egress; support-session exfiltration; mutable archive presented as immutable; unsupported pack/version silently collecting wrong semantics.

Mitigations: source authentication, tenant-bound external bindings, schema/rate/cardinality bounds, replay/idempotency, Credential Profiles, least privilege, data-egress policy, audit, evidence provenance/confidence, pack/version fail-closed compatibility, archive checksum/tamper evidence/WORM integration where required, and independent ECRP-P2-v1 security negatives.

Native full APM/DPI threat models are deferred with those capabilities to Phase 2.5+.

---

## Issue #272 deep-observability threat extension

Phase 2.5 introduces high-sensitivity threats that SHALL be explicitly modeled before implementation:
- secrets/PII captured in traces, RUM, session replay or packet payloads;
- TLS-key compromise or over-broad decryption authority;
- cross-tenant trace/profile/packet leakage;
- packet/session replay misuse;
- instrumentation supply-chain compromise;
- decoder/parser memory-safety and malformed-input attacks;
- cardinality/packet-rate storage exhaustion;
- unauthorized raw-PCAP export;
- excessive privileged capture;
- covert data egress through external analysis providers.

Required controls include least privilege, explicit enablement, audit, encryption, redaction/masking, tenant isolation, bounded retention, export controls, key custody, rate/cardinality limits and safe parser isolation.

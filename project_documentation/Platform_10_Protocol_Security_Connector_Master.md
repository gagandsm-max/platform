# Platform — Protocol, Security, Credential & Connector Master

**Document ID:** PLATFORM-PROTOCOL-001  
**Version:** 3.3.0
**Status:** **Authoritative Mother Reference — Protocol/Security/Connector Contract**  
**Product:** Platform

## 1. Governing principles

1. All customer-facing protocol/security configuration is exposed through Platform UI and API; no routine code/YAML/database editing.
2. Credential Profiles are reusable, encrypted and separately authorized for `USE`, `EDIT` and `REVEAL`.
3. Secure mechanisms are default; legacy/insecure mechanisms are disabled by default, explicitly labelled and audited.
4. Ports shown below are protocol defaults/examples, not hard-coded customer constraints. Device Packs and administrator policy may set vendor/customer-specific ports.
5. Provisioning preflight runs from the actual selected Sensor and distinguishes reachability, port/path, protocol, authentication and authorization failures.
6. The product never claims “firewall blocked” unless evidence proves it; otherwise it reports `ACL_FIREWALL_SUSPECTED` with supporting evidence and tests.

## 2. Network/security protocol matrix

| Capability | Protocol/security modes | Typical/default endpoint | UI configuration | OSS foundation | Required Platform behavior |
|---|---|---|---|---|---|
| Availability | ICMPv4/ICMPv6 | No TCP/UDP port | timeout, attempts, interval, packet size, source/Sensor | `golang.org/x/net/icmp` | loss/RTT/jitter, multi-vantage evidence, qualification input |
| SNMP polling | SNMP v1, v2c, v3 | UDP/161 default; configurable | version, community or v3 user/security level/auth/privacy/context, timeout/retry, bulk size | GoSNMP | GET/GETNEXT/GETBULK/WALK for routine monitoring; actionable typed errors; SET only through privileged NCM/remediation |
| SNMPv3 auth | noAuthNoPriv, authNoPriv, authPriv; MD5/SHA-1 legacy where required; supported SHA-2 variants | SNMP endpoint | profile-level algorithm selection | GoSNMP + Platform wrapper | secure defaults, compatibility validation, no secret exposure |
| SNMPv3 privacy | DES legacy opt-in; AES variants where library/device supports | SNMP endpoint | privacy algorithm/key and security level | GoSNMP + Platform wrapper | compatibility test before provisioning |
| Traps/Informs | SNMP v1/v2c/v3 | UDP/162 default; configurable | listener, source allowlist, v3 security | GoSNMP receiver/Platform event service | decode via MIB, normalize, correlate, rate protect; Inform ACK only after durable local acceptance/accounting |
| Syslog | UDP/TCP Syslog, Syslog over TLS | 514 / 6514 defaults | transport, TLS, source rules, parser/profile | Platform receiver; Fluent Bit integration candidate | normalization, cardinality/rate limits, retention, correlation |
| SSH/CLI | SSHv2 password/public-key/keyboard-interactive; privilege/enable patterns by Device Pack | TCP/22 default | credential, key, host-key policy, privilege policy, command controls | `x/crypto/ssh`, scrapligo/scrapli for selected roles | CLI broker, NCM, audit, no browser credentials |
| Telnet | Legacy only | TCP/23 default | explicit enable at tenant/device policy | small Platform adapter | disabled by default; never automatic fallback; warning/audit |
| NETCONF | NETCONF over SSH | TCP/830 default | credential, host key/TLS as vendor supports, capabilities | scrapligo / selected NETCONF library | capability discovery, normalized config/state |
| REST/RESTCONF | HTTPS; Basic/Bearer/API key/OAuth2/custom header/mTLS | TCP/443 default | base URL, port, auth, TLS/CA/client cert, headers, timeout/retry | Go HTTP, kin-openapi | API Studio mapping, paging/chained calls, schema/versioning |
| gNMI/gRPC | TLS/mTLS + username/token where vendor supports | vendor-defined; often 9339 but never assumed | endpoint, TLS trust/client cert, auth, subscription mode/path | OpenConfig gNMI, goyang/ygot | ONCE/POLL/STREAM as supported, backpressure, normalized telemetry |
| DNS | UDP/TCP DNS; DoT/DoH may be added via service packs where required | 53; secure modes configurable | server, record type, expected answer, timeout | miekg/dns / Go HTTP/TLS | synthetic validation, latency, record correctness |
| HTTP/HTTPS synthetic | HTTP/1.1, HTTP/2 where client supports; TLS | 80/443 target-specific | method, URL, headers, body, validations, TLS checks | Go HTTP/TLS | safe structured monitor; no arbitrary shell execution |
| TLS/certificate | TLS 1.2/1.3 checks; certificate/chain/hostname/expiry | target-specific | CA/trust, hostname, expected policy | Go `crypto/tls`, `x509` | expiry/chain/hostname/version/handshake monitoring |
| SCP/SFTP | SSH secure transfer | TCP/22 default | credential/key and destination | SSH libraries | NCM/config transfer where needed |
| TFTP/FTP | Legacy only if explicitly required | protocol default | explicit policy | Platform/OS adapter subject to review | disabled by default; warning/audit; no secret fallback |
| Flow | NetFlow v5/v9, IPFIX, sFlow; J-Flow/vendor variants through compatible formats/packs | exporter/collector configurable | receiver port, template handling, source mapping | GoFlow2 | enrichment, de-dup/template cache, high-rate safeguards |
| CDP/LLDP/ARP/FDB | SNMP/CLI/API/gNMI derived | underlying protocol endpoints | discovery profile | GoSNMP/CLI/gNMI | topology provenance/confidence/history |
| Routing/state | BGP/OSPF/IS-IS/BFD/MPLS/VXLAN-EVPN/VRF/STP/HSRP/VRRP/LACP/QoS/multicast/SD-WAN via SNMP/gNMI/NETCONF/REST/CLI | vendor-specific | Device Pack/Monitoring Profile | protocol foundations above | canonical graph and RCA semantics, not a routing daemon |

## 2.1 SNMP security, credential and execution supplement

- **Preferred posture:** SNMPv3 `authPriv` using supported SHA-2 authentication and AES privacy where the managed device supports it. v1/v2c, MD5, SHA-1 and DES are legacy compatibility modes, explicitly selected, labelled and audited; they are not automatic downgrade/fallback paths.
- **Credential Profiles:** normal jobs carry a profile reference/authorization context. `USE` does not imply `REVEAL`. Reusable community/auth/privacy material must not be returned to ordinary browsers, written to telemetry/event/audit records or retained in generic job history. Sensors receive/materialize only the minimum execution secret through the governed secret-delivery/resolution mechanism.
- **No default community:** Platform shall never probe production targets with an implicit/default `public` community.
- **SNMPv3 state:** engine ID discovery, engine boots/time synchronization, context, `unknownEngineID` and `notInTimeWindow` are explicit protocol outcomes with bounded recovery; authentication/security failures are not treated as generic timeouts.
- **Polling is read-only:** monitoring/discovery/on-demand test supports read operations only. SET is routed through privileged NCM/remediation policy and cannot be enabled merely by selecting an SNMP monitoring profile.
- **Cadence:** normal scheduled SNMP operational metrics default to **5 minutes** under the existing policy hierarchy; Cap Pack/monitor definitions select objects and may schedule static inventory/topology/lifecycle metadata more slowly. Availability/liveness remains a separate plane.
- **Target authorization:** scheduled/on-demand SNMP targets derive from tenant-scoped canonical entities/authorized diagnostic scope. Arbitrary target testing follows the same anti-pivot/SSRF-like restrictions as other Sensor diagnostics.
- **Trap trust:** v1/v2c UDP trap source/community is weak identity evidence. v3 notifications use configured USM/security validation. Source allowlist/policy, bounds and rate protection apply before event qualification.
- **Inform ACK:** SNMP Trap is unacknowledged. Successful Inform ACK requires durable local receiver acceptance/accounting after bounded parse, configured security validation and sufficient source/tenant attribution while preserving the raw typed notification. MIB/Pack enrichment, Core/Kafka/ClickHouse completion and alert/incident creation are not ACK prerequisites. ACK means local accepted/accounted only.

## 3. Infrastructure/cloud credential matrix

| Domain | Authentication/security | OSS/SDK | Platform-owned layer |
|---|---|---|---|
| Windows agentless | WinRM HTTPS; Kerberos/Negotiate/NTLM as required by customer environment | masterzen/winrm | credentials, policies, PowerShell/collection packs, diagnostics |
| Linux/Unix agentless | SSH keys/password; sudo/privilege escalation policy | x/crypto/ssh | safe command packs, metric normalization, audit |
| Windows/Linux Agent | per-Agent identity certificate and mTLS, certificate rotation | Go TLS + Platform PKI | enrollment, config signing, failover endpoints, local spool |
| Redfish/iDRAC/iLO/CIMC | HTTPS session/basic/token; mTLS where vendor supports | Gofish | hardware canonical model, pack compatibility |
| VMware | HTTPS/API credentials/token mechanisms | govmomi | inventory/topology/metric normalization |
| Hyper-V | WinRM/WMI | masterzen/winrm/native APIs | entity graph/metrics |
| Proxmox/Nutanix/storage | HTTPS REST/API/SNMP/SSH per pack | Go HTTP/GoSNMP | API Studio/Device Pack normalization |
| Databases | engine-specific username/password, TLS, certificate, integrated auth where driver supports | approved driver per engine | query packs, secret management, normalization; driver license review |
| AWS | IAM roles/keys/session credentials as deployment permits | AWS SDK for Go v2 | account/region/resource normalization |
| Azure | service principal/managed identity/OAuth as deployment permits | Azure SDK for Go | subscription/resource normalization |
| GCP | service account/workload identity/OAuth as deployment permits | Google Cloud Go | project/resource normalization |
| Kubernetes | kubeconfig/service-account/client cert/bearer token under least privilege | client-go | discovery/entity graph/RBAC safe use |
| OpenTelemetry | OTLP gRPC/HTTP secured with TLS/mTLS/token as configured | OTel Collector/libraries | tenant/entity mapping, quotas, schema |

## 4. Platform identity and UI security

- UI is HTTPS only. Default public UI endpoint is TCP/443 or customer-selected HTTPS port. Platform must not expose an application listener on TCP/80.
- TLS 1.2 minimum; TLS 1.3 supported/preferred; TLS 1.0/1.1 disabled.
- HSTS, Secure/HttpOnly cookies and suitable SameSite policy.
- Authentication options: Local where permitted, AD/LDAP, SAML, OIDC, Microsoft Entra ID, MFA.
- Keycloak is the selected identity-federation foundation; Platform resource authorization remains server-side and Platform-owned.
- API clients use scoped OAuth2/OIDC/API credentials; mTLS may be required for selected trusted integrations.
- Sensors and Agents use enrollment followed by unique mTLS identity; enrollment tokens are single-use/short-lived and audited.

## 5. Provisioning diagnostic contract

Every device/resource provisioning workflow executes layered tests:

```text
input normalization
 -> DNS/address validation
 -> Sensor eligibility/capacity
 -> route/reachability
 -> transport/port or protocol response
 -> TLS/host-key verification where relevant
 -> authentication
 -> authorization/capability
 -> identity/device-pack match
 -> monitoring-profile dry run
 -> capacity impact
 -> provision
```

Canonical errors include `INPUT_INVALID`, `DEVICE_UNREACHABLE`, `DNS_FAILURE`, `ROUTE_UNAVAILABLE`, `PROTOCOL_TIMEOUT`, `PROTOCOL_DISABLED_SUSPECTED`, `AUTHENTICATION_FAILED`, `AUTHORIZATION_FAILED`, `ACL_FIREWALL_SUSPECTED`, `PORT_UNREACHABLE`, `CREDENTIAL_PROFILE_INVALID`, `DEVICE_ALREADY_EXISTS`, `MIB_REQUIRED`, `API_SCHEMA_INVALID`, `TLS_CERTIFICATE_FAILURE`, `SENSOR_CAPACITY_EXCEEDED`, `NO_ELIGIBLE_SENSOR`, `MONITORING_PROFILE_INCOMPATIBLE`, `DEVICE_IDENTITY_CONFLICT`.

## 5A. On-demand operational diagnostic and CLI contract

Provisioning diagnostic primitives are reusable for authorized operational troubleshooting, but operational execution has an explicit job/audit context and may not bypass normal security or monitoring boundaries.

1. On-demand ICMP/traceroute/SNMP/TCP/DNS/SSH/API/domain tests execute from an eligible Platform Sensor/Agent selected by Core; the browser does not obtain raw network reachability to managed targets.
2. Default target is a tenant-scoped canonical entity/endpoint. Any ad-hoc target capability must be separately authorized and restricted against cross-tenant, link-local/metadata/control-plane and arbitrary-network abuse.
3. Packet count/size, duration, timeout, rate, concurrent jobs, multi-vantage fan-out and continuous-test duration are bounded and cancellable.
4. Multi-vantage tests preserve source identity and independent results. Failed remote vantage plus successful local vantage must not be flattened into `device down`.
5. Results expose stable codes, evidence, uncertainty/quality and remediation where known; raw vendor/library errors are sanitized for customer display.
6. CLI may use an authorized Credential Profile, policy-permitted personal credential or one-time session credential. Reusable secret material is never returned to the normal browser; one-time credentials are ephemeral/redacted and not promoted to reusable storage implicitly.
7. CLI privilege is policy-tiered. Configuration/destructive command classes can require JIT, MFA/re-auth, reason/ticket, maker-checker/approval and command deny/allow policy.
8. Every diagnostic/CLI job records actor, tenant, origin, canonical target, source/vantage, action, start/end/result, correlation ID and investigation/incident reference where applicable without logging secret values.
9. Interactive diagnostics use bounded worker/concurrency budgets so they cannot starve scheduled polling, ingestion or NCM safety work.

## 6. Firewall/connectivity matrix generation

Provisioning and Readiness Test must generate a customer-exportable matrix containing source Sensor/Core/Agent, destination/scope, protocol, port, purpose, TLS requirement and validation status. The matrix is generated from effective monitoring profiles and Device Packs, not from a static port list.

## 7. Connector framework

The Connector framework supports reusable, versioned connectors with: authentication profile, endpoint, field mapping, retries/timeouts/backoff, idempotency/correlation ID, rate limit, durable outbound queue, health, test sandbox and audit. ITSM targets include ServiceNow, BMC Helix/Remedy, ServiceDesk Plus, SummitAI, Freshservice/Freshworks, Jira Service Management and Generic REST. Notification targets include email, Teams, Slack, WhatsApp Business/provider API, webhook, SMS and PagerDuty where customer supplies the approved service/account.

## 8. Security acceptance gates

- Secrets are never stored in CSV by default and never displayed in ordinary logs/support bundles.
- `USE` permission never implies `REVEAL` permission.
- Legacy protocols are disabled by default and visibly marked.
- All protocol failures preserve raw technical reason internally while presenting safe actionable customer text.
- Certificates, keys and trust stores have lifecycle/expiry/rotation UI.
- Every security-sensitive action is audited with actor, tenant, object, result, reason/ticket and request/session context.

---

## 9. Endpoint and infrastructure diagnostic execution contract

### 9.1 Agent-based diagnostics

Linux/Windows Agent EDS executes as Diagnostic Job profile `ENDPOINT_DIAGNOSTIC_SNAPSHOT` through the existing authenticated Agent runtime and identity. It uses native read-only evidence paths under least privilege. EDS does not authorize an arbitrary local-shell, PowerShell, script-upload or customer-code execution interface.

A diagnostic request from Core/Sensor must be authenticated, tenant/entity scoped, versioned, generation/authority fenced, time bounded and replay/idempotency protected. Agent applies only a supported diagnostic definition under currently valid Core-issued authority and reports unsupported/denied/expired/stale requests explicitly.

### 9.2 Linux SSH agentless diagnostics

Infrastructure Sensor SSH diagnostics require:

- SSHv2;
- host-key validation;
- security-profile/credential binding;
- public-key authentication preferred where applicable;
- least-privilege account;
- controlled sudo only for explicitly certified read-only operations (never generic privilege escalation);
- allowlisted/versioned query pack as **fixed approved executable/operation + structured argv + typed constrained parameters**;
- parameter schema validation with no unchecked interpolation;
- prohibition of routine `sh -c` / `bash -c` / arbitrary command strings from users, API clients or model output;
- command timeout, output-byte limit and concurrency budget;
- secret/redaction controls;
- audit of query definition/version, target, actor/`diagnostic_job_id` and result code.

Generic unrestricted shell execution is not part of routine monitoring/EDS.

### 9.3 Windows agentless diagnostics

WinRM/WMI/CIM diagnostics require HTTPS/TLS where supported by the selected transport policy, credential/profile scoping, certificate validation and bounded versioned query definitions. Recurring monitoring/EDS shall prefer structured CIM/WMI/performance/service/Event Log/WinRM operations over PowerShell.

Where a certified PowerShell fallback is unavoidable, the exact script/command template is Platform-owned, fixed, versioned, read-only by declared policy, exact-operation scoped, typed-parameter constrained, output/timeout bounded, audited and separately security-certified. No generic PowerShell facility and no model/user-supplied arbitrary PowerShell are permitted for routine monitoring/EDS.

### 9.4 Evidence privacy

Default EDS excludes environment variables, arbitrary file content, packet payload, interactive user content and unrestricted process command lines. Privacy-sensitive process arguments/log fields require explicit policy, minimization/redaction and tenant authorization.

### 9.5 AI boundary

Agent/Sensor credentials, reusable secrets and unrestricted device-control capability are never provided to a local/private LLM provider. AI Gateway receives only bounded authorized evidence context and all tool/action authority remains deterministic Platform policy. Dedicated local/private inference, when supported, is conditional and not required for EDS or base monitoring.

## Flow / Log / Webhook / ITSM connector security notes — v3.2.2 handoff

- Flow: configured exporter/source/tenant authority; packet source IP alone is insufficient; sampling/template/sequence evidence retained; user-attributed Flow is privacy/RBAC sensitive.
- Log forwarding: secret/PII minimization/redaction; per-destination queue isolation; SIEM outage must not block base monitoring.
- Webhooks: signature/auth, replay protection, tenant/source binding, SSRF/egress allowlists, secrets, schema validation, rate limits, idempotency, audit; no arbitrary scripts.
- ITSM callbacks: signed/authenticated inbound; remote ticket state cannot override Platform health authority.

---

## 10. Linux Agent eBPF kernel instrumentation and active path probe mechanics — v3.2.2

> **Reconciliation reference:** Issue #142 (Master Chat comments `5909076167` and `5909787672`). Governed by `REQ-INF-013` (bounded Linux Agent eBPF process-to-network telemetry), `REQ-NET-016` (active path hop/loss/latency/fingerprint observability), and decisions `DEC-ARCH-036`, `DEC-ARCH-037`, `DEC-ARCH-038`. Target Phase-1 capabilities are subject to applicable implementation, benchmark, and certification gates.

### 10.1 Linux Agent eBPF kernel instrumentation mechanics

1. **Least-Privilege Execution:** The Linux Agent eBPF collector operates under least privilege across certified Ubuntu 24.x and RHEL 9.x kernels. The Agent requests granular Linux capabilities (such as `CAP_BPF` and `CAP_PERFMON` where available) rather than blanket `CAP_SYS_ADMIN`. On legacy enterprise kernels, the Agent uses bounded privilege separation with strict audit logging.
2. **CO-RE (Compile Once – Run Everywhere) and BTF:** eBPF probe objects leverage BPF Type Format (BTF) and CO-RE where the selected implementation requires/supports it. Pre-flight checks verify BTF availability. If BTF is missing, outdated, or incompatible with the compiled object, the Agent safely aborts eBPF initialization and sets capability state to `KERNEL_INCOMPATIBLE`.
3. **Deterministic Capability State Machine:**
   - `ACTIVE`: eBPF bytecode verified and attached; BPF ring-buffer healthy; local aggregation active.
   - `UNSUPPORTED`: Host architecture or OS kernel is non-Linux or below minimum supported eBPF baseline.
   - `UNAVAILABLE`: Host eBPF subsystem disabled by sysctl or security policy (e.g. `kernel.unprivileged_bpf_disabled=2`).
   - `PRIVILEGE_DENIED`: Agent process lacks required Linux capabilities (`CAP_BPF`, `CAP_PERFMON`).
   - `KERNEL_INCOMPATIBLE`: Kernel verifier rejected bytecode, or BTF definitions do not match running kernel.
   - `RESOURCE_GUARDED`: Local Agent memory or kernel map capacity exceeded; shedding/sampling enabled to protect host stability.
4. **Data Minimization by Architecture:** Attach points are strictly limited to socket state lifecycle and connection performance metrics. The eBPF collector is architecturally barred from:
   - Reading socket payload buffers (no application data capture);
   - Reading TLS/SSL memory or plaintext streams (no TLS decryption);
   - Reading HTTP request/response bodies or headers;
   - Reading database SQL statements or query parameters;
   - Inspecting environment variables or process memory;
   - Tracing arbitrary system calls or user-space functions.
5. **Cardinality and Resource Protection:** Kernel maps are bounded with eviction protection. Local aggregation in the Agent process enforces bounded active-key/cardinality ceilings per configurable aggregation interval. Memory and CPU budgets are derived from BUILD-019 / BUILD-130 benchmark certification. If limits are approached, the Agent sheds records to an aggregate overflow bucket and reports `shed_records_count` and `dropped_records_count`.

### 10.2 Active path probing mechanics

1. **Flow-Stable Probing:** Active path probing implemented by Network Sensors uses flow-stable, multipath/ECMP-aware probing techniques (evaluating flow-stable techniques such as Paris-traceroute-style methods in BUILD-040). Probes maintain flow stability across varying TTLs so that transit Equal-Cost Multi-Path (ECMP) load balancers route probes along consistent physical paths, preventing artificial hop flapping and route illusion.
2. **Probe Rate and Concurrency Limits:** Probing operations enforce bounded rate limits, bounded intervals, bounded TTL ceilings, and per-hop timeouts to prevent network flooding and avoid self-induced congestion. Exact defaults and tunable ranges are determined and certified in BUILD-040.
3. **Non-O(N²) Target Bounds:** Probing is restricted to explicitly configured infrastructure paths, inter-site links, cloud interconnects, and licensed outside-in probe targets. Automated all-to-all (full mesh) probing across arbitrary endpoints is strictly prohibited to prevent combinatorial probe explosions.
4. **Middlebox and Hidden-Hop Handling:** Intermediate routers or firewalls that rate-limit or silently drop ICMP Time Exceeded packets produce unresponsive hops. Such hops are explicitly recorded with `hop_state = UNRESPONSIVE` or `HIDDEN`, preserving the hop index in the ordered path record without asserting target unreachable or path failure.

---

## Phase-1 endpoint inventory/process/Fast-RCA connector reconciliation — Issues #169/#170

### Agentless installed-software inventory

Where Linux SSH or Windows WinRM/WMI/CIM agentless software inventory is certified, it uses the same governed query-pack model as diagnostics:

- Platform-owned fixed/versioned operation definitions;
- typed constrained parameters;
- read-only least privilege;
- bounded timeout/output/item count;
- normalized typed result schema;
- no user/model supplied arbitrary shell or PowerShell;
- no raw package-manager command output as the durable/public contract;
- source/provenance/partial-failure state retained.

A query/source failure is inventory-monitoring degradation and is not evidence that all software was uninstalled.

### Process/service monitor safety

Customer Monitor/Stop Monitoring policy never turns a displayed PID, process command line or user-provided expression into an executable command. Durable selectors are typed service/unit/executable identities with bounded exact-name fallback; PID reuse cannot silently retarget a monitor.

### Triggered Fast RCA connector behavior

Cross-vantage enrichment reuses certified owning connectors. Core may request or associate bounded OOB/virtualization/storage/path evidence only for authorized canonical related entities. Browser, endpoint Agent and local AI do not obtain arbitrary connector credentials or unrestricted target reachability.

The existing Diagnostic Job and query-pack controls apply unchanged: tenant/entity authority, fencing/generation, deadline, output bounds, rate/storm controls, audit and secret minimization.

### Windows installed-software inventory side-effect prohibition

Windows installed-software inventory is observational. Certified Agent/WinRM/WMI/CIM implementations must not use an enumeration mechanism known to trigger installer consistency checks, repair/reconfiguration, installation or other target mutation as a side effect merely to discover software. In particular, `Win32_Product`-style MSI inventory is not an approved default discovery mechanism. Prefer read-only supported registry/package/MSIX/AppX/OS inventory sources appropriate to the certified Windows version, with explicit provenance and partial-source behavior.


---

## 11. v3.3.0 OTLP, ITSM/CMDB and privileged-change connector contract

### 11.1 OTLP ingress

Certified OTLP/gRPC and OTLP/HTTP listeners are configurable and default to TLS-protected operation. mTLS is preferred where deployment permits; scoped token/OAuth-style credentials may be certified for compatible deployment profiles. Tenant/site/source authority is derived from the authenticated Platform ingress/connector binding, never from caller-supplied resource attributes or headers not bound by server configuration. Enforce compressed and post-decompression request bounds, record/attribute length/count limits, cardinality quotas, schema/protobuf/JSON validation where supported, rate limits, redaction/data-minimization policy and explicit accepted/rejected/dropped accounting.

### 11.2 Platform-wide ITSM adapter contract

Vendor adapters implement capability-advertised mappings over the canonical external-object model; unsupported objects/transitions must fail explicitly. Outbound connector egress uses destination allowlists/SSRF protections, TLS verification, proxy policy, least-privilege credentials, rate-budget awareness and deterministic operation/idempotency metadata. Inbound webhook/event processing requires connector authentication/signature where supported, replay protection, source binding, payload/schema/size bounds, rate limiting and server-side tenant derivation. Free-text comments/work notes are untrusted content and never directly become commands, templates, targets or authorization.

Where webhooks/events are insufficient, bounded incremental polling must use provider-supported updated-since/cursor/checkpoint/pagination semantics when available; repeated full-instance scanning is prohibited where an incremental API exists.

### 11.3 Connector certification priorities

Phase 1 certification targets: ServiceNow, Jira Service Management, BMC Helix, ManageEngine ServiceDesk Plus / MSP. Phase 1.5 certification-priority targets: Ivanti Neurons for ITSM, Freshservice/Freshservice MSP, SolarWinds Service Desk, TOPdesk, SysAid and HaloITSM. Certification is exact cloud/on-prem variant, edition, API version, auth mode, object capability, approval/transition semantics, inbound-event/polling behavior, rate limits, custom fields and CMDB/assets/relationship behavior. A listed vendor is not a certified connector until evidence passes.

### 11.4 ServiceNow CMDB

Certified CI publication/reconciliation uses the Identification and Reconciliation Engine (IRE) path by default. Generic direct table insertion is not the standard publication mechanism. Exact IRE endpoint/behavior remains part of ServiceNow connector version/edition certification.

### 11.5 NCM/firmware privileged boundary

External ITSM approval is workflow evidence, not privileged execution authority. NCM execution additionally requires valid connector identity, expected external workflow state/approver, non-stale exact Change Plan/hash/targets/window binding, local RBAC/resource scope/policy/preconditions and audit. Stale/replayed approval is rejected. Firmware transfer/execution in Phase 1.5 requires approved repository/provenance/integrity and vendor-specific capability certification; no universal rollback claim.

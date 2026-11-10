# Platform — Protocol, Security, Credential & Connector Master

**Document ID:** PLATFORM-PROTOCOL-001  
**Version:** 3.3.0
**Status:** **Authoritative Mother Reference — Protocol/Security/Connector Contract**  
**Product:** Platform
**Last reconciliation:** 2026-10-12 — Issue #200/#203 Tranche B Phase 1/1.5 certification-envelope reconciliation (protocol/ICMP/NCM/mTLS/tenant boundaries)

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
| Availability | ICMPv4/ICMPv6 | No TCP/UDP port | timeout, attempts, interval, packet size, source/Sensor | `golang.org/x/net/icmp` | Typed observation: loss/RTT/jitter, multi-vantage evidence, `GOOD`/`PARTIAL`/`UNKNOWN` quality; never sole DEVICE_DOWN authority when corroboration exists (`No Data != Device Down`) |
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

### LLDP/CDP neighbor discovery evidence profile — REQ-NET-007

- **Collection:** Existing Network Sensor uses read-only SNMPv3 authPriv where supported to retrieve LLDP-MIB local/remote chassis/port identifiers, timemark/age, capabilities and management-address facts, or CISCO-CDP-MIB cdpCacheTable with cdpCacheIfIndex, Device-ID, remote Port-ID and optional address/platform/capability. Vendor-approved CLI/API/gNMI is a bounded alternative; no default raw L2 packet capture, implicit community, credential downgrade or SNMP SET.
- **Identity:** SNMP authenticates the reporting device, not every advertised remote neighbor. Reconcile LLDP local ports and CDP ifIndex against IF-MIB/bridge-port/ifName/ifAlias/physical-port identity; ifIndex may change after reboot. Advertised remote names/IP/MAC/device IDs are untrusted until tenant-scoped corroboration and cannot create privileged endpoints or cross-tenant links.
- **Semantics:** These protocols reveal L2 adjacency evidence, not directed failure parentage. Preserve source, freshness/TTL, one-sided/contradictory rows, evidence quality and as-of history. Disabled/unsupported protocol, missing neighbor, truncated walk or expired row means UNKNOWN/stale evidence rather than confirmed outage.
- **Safety/capacity:** Govern target eligibility, row/cardinality/byte/time/retry budgets and cleanup; block source spoofing and unauthorized discovery. Require multi-vendor, malformed row, port renumber, LAG/STP/ECMP, one-sided, stale, cross-tenant and storm/regression cases. Status: SPECIFIED, implementation/certification not inferred.

## 2.1 SNMP security, credential and execution supplement


- **Preferred posture:** SNMPv3 `authPriv` using supported SHA-2 authentication and AES privacy where the managed device supports it. v1/v2c, MD5, SHA-1 and DES are legacy compatibility modes, explicitly selected, labelled and audited; they are not automatic downgrade/fallback paths.
- **Credential Profiles:** normal jobs carry a profile reference/authorization context. `USE` does not imply `REVEAL`. Reusable community/auth/privacy material must not be returned to ordinary browsers, written to telemetry/event/audit records or retained in generic job history. Sensors receive/materialize only the minimum execution secret through the governed secret-delivery/resolution mechanism.
- **No default community:** Platform shall never probe production targets with an implicit/default `public` community.
- **SNMPv3 state:** engine ID discovery, engine boots/time synchronization, context, `unknownEngineID` and `notInTimeWindow` are explicit protocol outcomes with bounded recovery; authentication/security failures are not treated as generic timeouts.
- **Polling is read-only:** monitoring/discovery/on-demand test supports read operations only. SET is routed through privileged NCM/remediation policy and cannot be enabled merely by selecting an SNMP monitoring profile.
- **Cadence:** normal scheduled SNMP operational metrics default to **5 minutes** under the existing policy hierarchy; Cap Pack/monitor definitions select objects and may schedule static inventory/topology/lifecycle metadata more slowly. Availability/liveness remains a separate plane.
- **Target authorization:** scheduled/on-demand SNMP targets derive from tenant-scoped canonical entities/authorized diagnostic scope. Arbitrary target testing follows the same anti-pivot/SSRF-like restrictions as other Sensor diagnostics.
- **Trap trust:** v1/v2c UDP trap source/community is weak identity evidence. v3 notifications use configured USM/security validation. Source allowlist/policy, bounds and rate protection apply before event qualification.
- **Inform ACK:** SNMP Trap is unacknowledged. Successful Inform ACK requires durable local receiver acceptance/accounting after bounded parse, configured security validation and sufficient source/tenant attribution while preserving the raw typed notification. The receiver maintains a persistent unique authoritative local `snmpEngineID` (listener identity only—not a unique sender/notifier). Per RFC 3414 §2.2.2 and authoritative `Platform_05` §12.8: normal restart with a known valid persisted `snmpEngineBoots` monotonically advances and persists boots and reinitializes `snmpEngineTime`; if the latest boots value cannot be determined (loss/corruption/rollback), set `snmpEngineBoots` to **2147483647** (latched fail-closed: authenticated messages fail `notInTimeWindow`; no successful authenticated Inform ACKs); recover only by controlled manual reconfiguration with a new unique `snmpEngineID` or new USM auth/privacy secrets—never by guessing/inventing a boot count. USM discovery/time-sync is distinct from Inform ACK. Durable SNMPv3 retry idempotency uses ScopedPDU `request-id` plus authenticated security/context identity and canonical notification payload evidence as the business-notification fingerprint—not outer `msgID`, and not USM discovery—but that fingerprint alone does not uniquely identify a notifier when multiple devices share the same USM security principal. Durable deduplication MUST be scoped to an independently qualified tenant/notifier identity from governed authentication, admission and source binding; a shared USM `securityName` or unverified UDP source MUST NOT be treated as cryptographically unique device identity. Separately admitted notifiers with identical request-id/security context/payload MUST retain independent durable event evidence. Genuine same-notifier retransmission MAY deduplicate under the bounded durable policy with occurrence accounting; separately occurring notifications after request-id reuse must preserve durable occurrence accounting, source evidence, ambiguity and distinct critical-transition evidence—and must not promise impossible cryptographic differentiation of wire-identical notifications that lack unique occurrence identity. Ambiguous notifier identity is fail-closed: never collapse distinct sources into one event and never ACK a silently discarded notification. SNMPv2c Inform source/community attribution is weak and not cryptographically authenticated and remains subject to the same fail-closed ambiguous-identity rule. Distinct critical events remain distinguishable before semantic enrichment; durably ACKed Informs must not be silently discarded. Negative acceptance evidence is required for same-notifier retransmission, same-notifier request-id reuse / distinct-occurrence ambiguity, wire-identical undecidable duplicate, and two-notifier collision under existing `Platform_14` SNMP Inform ownership (SPECIFIED; no new Platform_14 section or certification-status claim). MIB/Pack enrichment, Core/Kafka/ClickHouse completion and alert/incident creation are not ACK prerequisites. ACK means local accepted/accounted only.
- **Protocol-edge acceptance ownership (SPECIFIED only):** Adversarial acceptance for adaptive GETBULK validity versus malformed responses, Inform engine durability/idempotency, Flow ingress/template/sampling integrity, qualified path-identity comparison and class-aware self-induced measurement protection is owned by the existing Network Sensor contracts in this master together with `Platform_05` / `Platform_17` and the existing `Platform_14` SNMP/Flow/path certification supplements. Do not renumber `Platform_14` to carry a separate Network Sensor matrix. The Flow receiver and Inform production runtime remain `NOT IMPLEMENTED` / `NOT CERTIFIED` unless exact-head source and physical acceptance evidence separately prove readiness.

### 2.2 ICMP typed-observation and Core acceptance boundary

Phase 1 architecture requires ICMP availability evidence as a protocol-aware typed Sensor observation with sent/received, RTT, loss, IPv4/IPv6, vantage/provenance and quality states. Unreachable versus packet-loss and incomplete path hops must remain distinguishable from authoritative DEVICE_DOWN. Core must accept ICMP evidence through a governed Sensor→Core durable path without silently dropping mixed batches or ACKing evidence it did not persist.

**Issue #218 implementation gap (SOURCE_FACT / NOT CLOSED):** Current Core numeric-only ingestion rejects Sensor `icmp_observation_v1` envelopes that lack `protocolPayload.observations` or `metric_id`+numeric `value`. That Core gap is a P1 telemetry-integrity blocker for end-to-end ICMP enablement; it does not authorize rewriting Mother Reference architecture to numeric-only ICMP or collapsing missing ICMP into DEVICE_DOWN. Closure requires Master-selected Core contract work and evidence—not documentation fiction.

### 2.3 Syslog ingress profiles and tenant attribution — 2026-10-30 audit reconciliation

A `LogIngressProfile` is a **logical, server-administered configuration contract**, not a new service or new wire protocol. It binds `profile_id`, assigned Sensor/site/tenant authority, transport/listener/interface or trusted route, configured source identities/CIDRs, TLS trust/client identity where supported, parser/redaction policy, quotas and reject/quarantine handling. The precise API/schema name and physical persistence remain governed by BUILD-062/063/064 and the existing config authority (Platform_17); there is no assumption that a single tenant needs its own VIP or port.

- **Phase 1:** Single-customer Syslog may use a shared listener only where source-to-entity and server-side tenant assignment are deterministic; payload hostname, untrusted headers and source IP alone are never tenant authority.
- **Phase 1.5 MSP/shared tenancy:** Before enabling any listener handling multiple tenants, prove **unique ingress binding** across overlapping RFC 1918, NAT, forwarding relays, interfaces and sites. A distinct listener/IP, trusted site/Sensor boundary or authenticated TLS client certificate mapped through a server-side Credential/Ingress Profile are possible mechanisms; no mechanism is mandated without deployment proof. TLS cert DN/SAN text alone does not create tenant authority. CIDR allowlisting is a filter, not a proof of identity when sources overlap.
- If more than one tenant/source authority matches or none can be proved, **fail closed**: no default-tenant assignment, no cross-tenant entity lookup, and no raw event stored in a tenant's spool. Drop with reason accounting or use a bounded access-restricted **metadata-only** unresolved diagnostic record; never a shared unredacted dead-letter stream.
- Protocol boundaries remain transport-specific: UDP message/datagram and loss semantics, TCP Syslog framing (RFC 6587 where supported), and certificate-validated TLS Syslog (RFC 5425) are not interchangeable. Source configuration and framing compatibility are part of acceptance.

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
| AWS | IAM roles/STS-style temporary credentials preferred over long-lived root keys where supported; least-privilege scoped for inventory/monitoring/logs/events/change as permitted | AWS SDK for Go v2 | account/region/resource normalization; Phase 1.5 serverless (Lambda) under `REQ-SLS-006` |
| Azure | managed identity/service principal/OAuth as appropriate; least privilege | Azure SDK for Go | subscription/resource normalization; Phase 1.5 serverless (Functions) under `REQ-SLS-006` |
| GCP | workload/service identity/OAuth as appropriate; least privilege | Google Cloud Go | project/resource normalization; Phase 1.5 serverless (Cloud Functions/Cloud Run) under `REQ-SLS-006` |
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

### 9.3.1 Agentless transport/session and target-protection invariants

SSH and WinRM/WMI/CIM query packs use **bounded per-target and per-credential concurrency**, Sensor-wide worker/MWU admission, deadline/timeout/cancellation where supported, output/item limits, retry+jitter budgets, and leak-free session cleanup. Capacity is based on target and Sensor measurements, not fixed ceilings such as `100 SSH sessions` or `two WinRM shells per host`. Session/transport reuse is **optional and verified per operation**: long-lived WinRS/PowerShell shells are not an unconditional pool and must not outlive credential revocation or assignment/fencing authority. Each connection validates SSH host keys or WinRM HTTPS server identity; rotate/revoke credentials without reusing stale authenticated contexts.

WinRM shell/process quotas and GPOs vary by target; collector packs must distinguish shell-backed execution from direct CIM/WMI/WS-Man operations and certify the chosen transport/OS/version. The existing fixed, read-only, versioned PowerShell fallback remains permitted when required, and `Win32_Product`-style side-effecting inventory remains prohibited. Linux sudo/TTY/permission denial degrades only affected probes to `PARTIAL` with a missing-evidence/privilege reason; it cannot fabricate host DOWN or turn a failed privileged check into a zero value.

### 9.4 Evidence privacy

Default EDS excludes environment variables, arbitrary file content, packet payload, interactive user content and unrestricted process command lines. Privacy-sensitive process arguments/log fields require explicit policy, minimization/redaction and tenant authorization.

### 9.5 AI boundary

Agent/Sensor credentials, reusable secrets and unrestricted device-control capability are never provided to a local/private LLM provider. AI Gateway receives only bounded authorized evidence context and all tool/action authority remains deterministic Platform policy. Dedicated local/private inference, when supported, is conditional and not required for EDS or base monitoring.

## Flow / Log / Webhook / ITSM connector security notes — v3.2.2 handoff

- Flow: configured exporter/source/tenant authority; packet source IP alone is insufficient; sampling/template/sequence evidence retained; user-attributed Flow is privacy/RBAC sensitive. Tenant identity MUST NEVER be taken from a Flow packet field.
- Flow first Phase-1 UDP profile: BUILD-051 freezes executable ordering for `FLOW_UDP_P1_PROFILE_V1` (`SPECIFIED_NOT_IMPLEMENTED` until code/evidence; BUILD-051 remains `NOT_IMPLEMENTED`). Scope = UDP NetFlow v5/v9, IPFIX v10 over UDP, sFlow v5. J-Flow is not a proprietary parser here — only when the exporter's wire format is certified as one of those NetFlow/IPFIX formats. Target package `sensor/internal/protocol/flow/` (no new microservice). GoFlow2 (`OSS-022`) import requires `FLOW_PARSER_DEPENDENCY_ADMISSION_GATE` (exact version/commit, licence, SBOM/CVE, API surface, checksum, fixture compatibility, rollback) before BUILD-051 source dispatch that imports it. Protocol-detail ownership: `Platform_17` §13.9 + this section; Build Register owns slices/gates.
- Flow ingress isolation: a bare UDP port is not a trusted isolation boundary. Bind exporter identity to independently verified ingress context, routing domain and authorization (listener/interface/VRF/site/Sensor). Ambiguous mappings fail closed; spoofable UDP origin remains low-trust unless independently protected/validated. Quarantined/unmapped traffic must not produce canonical Flow analytics. Phase 1 covers intra-tenant overlapping RFC1918/VRF spaces on verified ingress contexts; Phase 1.5 MSP certification tests cross-tenant overlapping addresses and source/tenant spoofing.
- Flow stream identity (do not collapse solely by source IP): NetFlow v5 binds authoritative exporter binding + source IP/port + engine type/ID; NetFlow v9 binds exporter binding + source IP/port + Source ID/Observation Domain; IPFIX UDP binds exporter binding + source IP/UDP port + collector local IP/destination port + Observation Domain ID; sFlow v5 binds exporter binding + agent address + sub-agent ID.
- Flow template and sampling safety: NetFlow v9/IPFIX template keys and lifetimes include exporter transport/Observation Domain or Source ID context and the shared Data/Options Template ID namespace. After collector restart, template authority starts EMPTY/volatile — do not decode UDP Data Sets from unvalidated persisted templates; require live template refresh. Activated profile MUST supply `template_refresh_timeout_seconds > 0`. Enforce UDP refresh/redefinition semantics; valid redefinition replaces atomically; malformed redefinition must not corrupt prior valid state; **ignore UDP Template Withdrawal as unreliable withdrawal authority**. IPFIX V1 uses `NO_BUFFER_UNTEMPLATED_DATA`. Sequence/loss uses protocol-native semantics (v5 flow-sequence; v9 cumulative Export Packet sequence; IPFIX record-based — Template/Options Template records do not increment data-record sequence); gaps are quality/loss evidence, never Device Down. Distinguish sFlow sampled-frame evidence, sFlow interface/device Counter samples (typed metric paths—not ordinary per-flow `network_flow` facts), and IPFIX/NetFlow counters. sFlow sampled header capture length is not automatically original frame length; preserve original frame length only where provided and qualified. Never treat cumulative sFlow interface counters as per-flow bytes/packets or scale them by packet sample rate. BUILD-051 decoder preserves sampling evidence only and does not manufacture estimated traffic totals; `SAMPLED_UNKNOWN` is never `UNSAMPLED`. Missing templates and uncertain sampling are quality/coverage signals. Expose immutable raw source values and separately identified estimates; exporter sampling configuration is not a blanket numeric multiplier. No new local KV store, mandatory physical schema columns or breaking schema migration is selected.
- Flow first-profile failure vocabulary (logical classes; exact constants follow repository naming): `MALFORMED_PACKET`, `UNSUPPORTED_PROTOCOL_VERSION`, `EXPORTER_UNBOUND`, `EXPORTER_AMBIGUOUS`, `TEMPLATE_MISSING`, `TEMPLATE_EXPIRED`, `TEMPLATE_REDEFINITION_INVALID`, `SEQUENCE_GAP`, `EXPORTER_RESTART_OR_DISCONTINUITY`, `UNSUPPORTED_RECORD`, `QUALITY_PARTIAL`. Parser errors MUST NOT become target DOWN.
- Log forwarding: secret/PII minimization/redaction; per-destination queue isolation; SIEM outage must not block base monitoring.
- Webhooks: signature/auth, replay protection, tenant/source binding, SSRF/egress allowlists, secrets, schema validation, rate limits, idempotency, audit; no arbitrary scripts.
- ITSM callbacks: signed/authenticated inbound; remote ticket state cannot override Platform health authority.

### Log/SIEM forwarding isolation

Where external SIEM forwarding is enabled, use per-destination/tenant bounded **asynchronous** workers, retry/backoff, authorization/egress and independent queue/memory/disk quotas; a separate disk partition or `10 GB` allocation is not mandatory. Primary monitoring, authority/control and durable Sensor→Core replay retain priority. Forwarding backlog/oldest age/failed delivery/drops are visible. When a finite forwarding quota is exhausted, apply the configured, legally reviewed forwarding retention/loss policy and expose the gap; do not promise absolute non-interference, lossless delivery, or an unlimited compliance quarantine. Distinguish a transport send from destination application acknowledgement where an actual sink ACK exists.

---

## 10. Linux Agent eBPF kernel instrumentation and active path probe mechanics — v3.2.2

> **Reconciliation reference:** Issue #142 (Master Chat comments `5909076167` and `5909787672`). Governed by `REQ-INF-013` (bounded Linux Agent eBPF process-to-network telemetry), `REQ-NET-016` (active path hop/loss/latency/fingerprint observability), and decisions `DEC-ARCH-036`, `DEC-ARCH-037`, `DEC-ARCH-038`. Target Phase-1 capabilities are subject to applicable implementation, benchmark, and certification gates.

### 10.1 Linux Agent eBPF kernel instrumentation mechanics

1. **Least-Privilege Execution:** The Linux Agent eBPF collector operates under least privilege across certified Ubuntu 24.x and RHEL 9.x kernels. The Agent requests granular Linux capabilities (such as `CAP_BPF` and `CAP_PERFMON` where available) rather than blanket `CAP_SYS_ADMIN`. On legacy enterprise kernels, the Agent uses bounded privilege separation with strict audit logging.
2. **CO-RE (Compile Once – Run Everywhere) and BTF:** eBPF probe objects leverage BPF Type Format (BTF) and CO-RE where the selected implementation requires/supports it. Pre-flight checks verify BTF availability. If BTF is missing, outdated, or incompatible with the compiled object, the Agent safely aborts eBPF initialization and sets capability state to `KERNEL_INCOMPATIBLE`.
3. **Deterministic Capability State Machine:**
   - `ACTIVE`: eBPF bytecode verified and attached; BPF ring-buffer healthy; local aggregation active.
   - `UNSUPPORTED`: Host architecture or OS kernel is non-Linux or below minimum supported eBPF baseline.
   - `UNAVAILABLE`: Required eBPF functionality is unavailable under verified host/kernel/security policy. A single sysctl observation is not sufficient to classify the collector; bounded preflight and actual permitted load/attach capability determine the outcome, with a non-secret reason code.
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

1. **Flow-Stable Probing:** Network Sensor active path probing shall evaluate flow-stable, multipath/ECMP-aware techniques such as Paris-traceroute-style methods in BUILD-040. Preserve protocol-relevant flow identifiers across TTL/hop probes where feasible and validate checksum/identifier handling for selected UDP/ICMP/TCP modes. Flow-stable probes do **not** guarantee identical physical paths. Do not assert that an invariant UDP/ICMP checksum, IPv4 header checksum, IPv6 Flow Label or five-tuple guarantees one physical path under per-packet balancing, NAT, MPLS, middleboxes, ICMP CoPP or return-path asymmetry. BUILD-040 must certify measured false-divergence/hidden-hop behaviour and emit machine-readable `path_identity_quality` / `comparison_eligible` (`Platform_17` §11.3). A path fingerprint may describe a candidate observation but MUST NOT assert topology or latency/loss change when ambiguous, unpinned, insufficient or unqualified; `PathChangeEvidence` requires both baseline and current observations to be qualified and comparable. Absent optional quality metadata is not comparison-eligible; fingerprint-only change emitters are not safe. Uncertain multipath must not cause false `PATH_DIVERGENCE`, `RTT_SHIFT`, `LOSS_ANOMALY` or `DEVICE_DOWN`. Existing `schema_version` and BUILD-040 ownership are preserved.
2. **Probe Rate and Concurrency Limits:** Probing operations enforce bounded rate limits, bounded intervals, bounded TTL ceilings, and per-hop timeouts to prevent network flooding and avoid self-induced congestion or self-induced path-RTT measurement degradation. Exact defaults and tunable ranges are determined and certified in BUILD-040; Mother References do not hardcode EPS/IOPS/timeout quotas.
3. **Non-O(N²) Target Bounds:** Probing is restricted to explicitly configured infrastructure paths, inter-site links, cloud interconnects, and licensed outside-in probe targets. Automated all-to-all (full mesh) probing across arbitrary endpoints is strictly prohibited to prevent combinatorial probe explosions.
4. **Middlebox and Hidden-Hop Handling:** Intermediate routers or firewalls that rate-limit or silently drop ICMP Time Exceeded packets (including CoPP) produce unresponsive hops. Such hops are explicitly recorded with `hop_state = UNRESPONSIVE` or `HIDDEN`, preserving the hop index in the ordered path record without asserting target unreachable or path failure. MPLS label visibility and return-path asymmetry remain quality/uncertainty evidence.

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

### 11.1.1 OTLP cardinality and sensitive-attribute admission

Enforce per-tenant, Resource/service, metric identity and data-type budgets **before local durable acceptance**: request/decoded-byte/attribute limits, bounded in-flight stream and series state, rate/cardinality and fairness budgets, PII/secret minimization, and explicit accepted/rejected accounting. When a metric would exceed a budget, reject the affected points/series with correct OTLP partial-success semantics (where supported), without silently rewriting series identity, merging counter streams, changing cumulative Sum/histogram temporality to Gauge, or storing stripped sensitive values in an `overflow_attributes` JSON column. Any governed aggregation must have a separately certified, semantics-preserving transformation and provenance. HLL/sketches are optional **estimation** mechanisms, not a required dependency nor exact membership authority.

### 11.2 Platform-wide ITSM adapter contract

Vendor adapters implement capability-advertised mappings over the canonical external-object model; unsupported objects/transitions must fail explicitly. Outbound connector egress uses destination allowlists/SSRF protections, TLS verification, proxy policy, least-privilege credentials, rate-budget awareness and deterministic operation/idempotency metadata. Inbound webhook/event processing requires connector authentication/signature where supported, replay protection, source binding, payload/schema/size bounds, rate limiting and server-side tenant derivation. Free-text comments/work notes are untrusted content and never directly become commands, templates, targets or authorization.

Where webhooks/events are insufficient, bounded incremental polling must use provider-supported updated-since/cursor/checkpoint/pagination semantics when available; repeated full-instance scanning is prohibited where an incremental API exists.

### 11.3 Connector certification priorities

Phase 1 certification targets: ServiceNow, Jira Service Management, BMC Helix, ManageEngine ServiceDesk Plus / MSP. Phase 1.5 certification-priority targets: Ivanti Neurons for ITSM, Freshservice/Freshservice MSP, SolarWinds Service Desk, TOPdesk, SysAid and HaloITSM. Certification is exact cloud/on-prem variant, edition, API version, auth mode, object capability, approval/transition semantics, inbound-event/polling behavior, rate limits, custom fields and CMDB/assets/relationship behavior. A listed vendor is not a certified connector until evidence passes.

### 11.4 ServiceNow CMDB

Certified CI publication/reconciliation uses the Identification and Reconciliation Engine (IRE) path by default. Generic direct table insertion is not the standard publication mechanism. Exact IRE endpoint/behavior remains part of ServiceNow connector version/edition certification.

### 11.5 NCM/firmware privileged boundary

External ITSM approval is workflow evidence, not privileged execution authority. NCM execution additionally requires valid connector identity, expected external workflow state/approver, non-stale exact Change Plan/hash/targets/window binding, local RBAC/resource scope/policy/preconditions and audit. Stale/replayed approval is rejected. Firmware transfer/execution in Phase 1.5 requires approved repository/provenance/integrity and vendor-specific capability certification; no universal rollback claim.

**Phase 1 / Phase 1.5 privileged separation (Issue #200):** Phase 1 independently certifiable NCM is backup/version/diff/compliance/golden-state plus controlled manual CLI only when RBAC/credential brokering/audit are certified. Mass push, canary/staged automated privileged remediation and maker-checker automation defaults belong to **Phase 1.5** unless Founder change control promotes them. Protocol connectors must not bypass Sensor assignment/fencing or local Platform authorization.

---

### 11.6 Hypervisor, cloud, database and storage API safety

- VMware inventory/topology connectors should use version-compatible `PropertyCollector` initial bounded retrieval and incremental update/version handling where supported; performance counters use `PerformanceManager`, not an assumed PropertyCollector metric stream. Restart, invalid update token, expired session and partial update require bounded resync/checkpointing with explicit evidence quality; benchmark benefits instead of claiming `95%` traffic reduction.
- Cloud requests use provider/account/subscription/project, region, **service and operation** rate budgets, SDK-supported retry/backoff/jitter, pagination/point-count limits and fairness. Do not hard-code an AWS CloudWatch `GetMetricData` `50 TPS` universal ceiling; quota values are provider-published, operation/region/account-specific and changeable. Prefer SDK v2 credential caching/refresh; refresh from actual STS expiry with a bounded safety margin, not an assumed compulsory one-hour lifetime or fixed ten-minute prefetch.
- Database query packs enforce least privilege, read-only and allowlisted SQL, bounded per-target connections, server-side statement/execution deadlines where supported, cancellation and connection cleanup; no global `MaxOpenConns=2` or `5s` promise. Storage/SMI-S/REST targets get operation-specific concurrency/call deadlines and a separate expensive-work budget so a slow controller does not starve base polling. API/permission/transport failure is connector or monitoring `PARTIAL/UNKNOWN`, not asserted guest/database/volume DOWN without corroboration.

## 12. Phase 1 / Phase 1.5 certification envelopes — Issue #200 / #203

Architecture target ≠ implementation ≠ tested ≠ benchmarked ≠ certified ≠ deployed. Mother Reference set version remains **3.3.0** through Tranche B; v3.4.0 is reserved for gated Tranche D.

| Envelope | Protocol / identity emphasis | Tenancy / MSP | Explicit nonclaims |
|---|---|---|---|
| **Phase 1** | Standalone Core; Sensor/Agent mTLS enrollment; SNMP typed read monitoring; ICMP typed availability evidence; Inform local ACK boundary; multi-Sensor assignment fencing | Tenant-aware Credential Profiles, RBAC and audit mandatory for single-customer enterprise; no MSP/shared-customer isolation cert claim | Issue #218 Core ICMP acceptance gap open; NCM automation not Phase 1 default; no Core-HA claim |
| **Phase 1.5** | Same protocol contracts under exactly-two-Core HA continuity; push-protocol HA reconnect/re-anchor | MSP/shared multi-tenancy adversarial isolation certification across connectors/credentials/NCM | Physical HA mechanisms unselected; firmware remains Phase 1.5; listed ITSM vendors are targets until evidence passes |

## Phase 1.5 Serverless connector security — Issue #268

Governing requirements: `REQ-SLS-006`, `PRD-SLS-006`. Status: SPECIFIED / NOT IMPLEMENTED / NOT CERTIFIED.

Mandatory controls for AWS Lambda, Azure Functions, Google Cloud Functions and Google Cloud Run connectors:

- least-privilege cloud identity; prefer temporary STS/role, managed identity or workload identity patterns where supported;
- tenant/resource scope on all reads and actions; server-side authorization;
- provider credentials never returned to browser;
- secrets and environment-variable values must not be logged, audited in cleartext, or captured into telemetry/support bundles;
- function request/response payload capture is NOT part of baseline serverless monitoring;
- no source-code retrieval required for baseline monitoring;
- no arbitrary function invocation for health testing;
- bounded APIs, dimensions and tags;
- connector endpoints/identities MUST NOT create SSRF or tenant escape (allowlists, DNS/IP validation, redirect controls, private-address policy where applicable, timeouts);
- audit onboarding/configuration/credential actions;
- provider IDs cannot override Platform tenant/entity authority.

OCI Functions remain future unless separately promoted; no new OSS SDK is admitted by this documentation task (reuse Platform_07 OSS-047/048/049).

---

## Phase 2 Enterprise RFP Readiness Reconciliation — Issue #269

Phase-2 Integration Packs shall support governed adapters for monitoring tools, APM, NPM/DPI, SIEM/SOC, CMDB, ITSM, deployment/change systems and enterprise technology APIs. Every adapter inherits Credential Profile, TLS/mTLS/OAuth/API-key policy, bounded pagination/rate budgets, replay/idempotency, source validation, connector health, test sandbox and audit.

OCI is added as a Phase-2 cloud-provider target through the Cloud/API + Capability Pack architecture; exact SDK/dependency admission remains Platform_07 authority.

No Phase-2 connector may bypass server-side tenant/resource authorization or import vendor identity as canonical Platform authority.

---

## Issue #272 protocol/security planning

Phase 2 closure contracts SHALL require exact product/version/API/protocol/privilege matrices for gNMI, SD-WAN, legacy OS, DB, middleware, M365, EUC/VDI, security SaaS and buyer-specific integrations.

Phase 2.5 planning adds governed protocol families for approved APM instrumentation/OTel-compatible trace interoperability, browser/mobile telemetry, profiling transport, SPAN/TAP/vTAP/packet-broker capture, packet protocol decoding and optional controlled TLS decryption.

No TLS decryption or payload capture may be enabled without explicit authorization, key-custody controls, audit, privacy policy and bounded retention.

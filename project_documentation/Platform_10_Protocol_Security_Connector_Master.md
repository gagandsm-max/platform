# Platform — Protocol, Security, Credential & Connector Master

**Document ID:** PLATFORM-PROTOCOL-001  
**Version:** 3.2.2  
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
| SNMP polling | SNMP v1, v2c, v3 | UDP/161 default; configurable | version, community or v3 user/security level/auth/privacy/context, timeout/retry, bulk size | GoSNMP | GET/GETNEXT/GETBULK/WALK/SET where permitted; actionable errors |
| SNMPv3 auth | noAuthNoPriv, authNoPriv, authPriv; MD5/SHA-1 legacy where required; supported SHA-2 variants | SNMP endpoint | profile-level algorithm selection | GoSNMP + Platform wrapper | secure defaults, compatibility validation, no secret exposure |
| SNMPv3 privacy | DES legacy opt-in; AES variants where library/device supports | SNMP endpoint | privacy algorithm/key and security level | GoSNMP + Platform wrapper | compatibility test before provisioning |
| Traps/Informs | SNMP v1/v2c/v3 | UDP/162 default; configurable | listener, source allowlist, v3 security | GoSNMP receiver/Platform event service | decode via MIB, normalize, correlate, rate protect |
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

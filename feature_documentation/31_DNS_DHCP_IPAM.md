# DNS / DHCP / IPAM

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 09, 10, 20, 22, 23

## Product objective

Provide authoritative operational context for addresses, names, leases and network ownership while using DNS/DHCP as both monitored shared services and identity/enrichment evidence.

## IPAM

Platform IPAM should manage/observe:

- IPv4 and IPv6;
- subnets/prefixes;
- addresses;
- VLAN/site/VRF context;
- reservation/allocation state;
- conflicts/duplicates;
- orphan/stale addresses;
- utilization/capacity;
- entity associations;
- DNS/DHCP reconciliation.

IP/MAC remain mutable observations under the canonical identity model.

## DHCP integration

Integrations may include Microsoft DHCP/DNS, Infoblox and other certified providers/APIs.

Useful evidence includes:

- lease address;
- MAC/client ID;
- hostname/FQDN;
- lease start/end;
- scope/site;
- server/source;
- reservation state.

Historical lease timing is important for Flow/user/entity enrichment.

## DNS integration

Monitor DNS service availability, latency and response correctness. Preserve name/address mapping provenance and observed validity window where used for enrichment.

DNS evidence can enrich Flow destinations with domain/FQDN, but a DNS name is not a full URL and shared/CDN IPs can map to many domains.

## Shared-service monitoring

DNS/DHCP are dependency evidence for incidents. A DNS outage may explain application/synthetic failures without meaning the application process itself failed.

## Reconciliation

Avoid silent identity rewrite:

- DHCP IP movement updates address ownership over time;
- stale leases/records are visible;
- conflicting DNS/CMDB/Agent/SNMP evidence enters reconciliation;
- historical telemetry retains time-correct mapping.

## Security

Tenant/site scope, least-privilege API credentials, secure transport and audit apply. DNS responses and DHCP metadata do not override authenticated tenant authority.

## UX

Provide subnet/address inventory, lease/reservation context, DNS record relationships, conflicts, utilization, related entities/services, monitoring health and Flow/topology drill-down.

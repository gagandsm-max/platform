# Flow Monitoring

**Class:** Derived feature brief — non-authoritative
**Canonical sources:** Platform_02, 03, 04, 05, 06, 07, 09, 10, 11, 14, 17, 18, 22, 23
**Enhancement state:** significant items below are **pending Mother Reference reconciliation**.

## Product objective

Provide trustworthy network traffic analytics from NetFlow v5/v9, IPFIX, sFlow and compatible vendor/J-Flow formats while preserving exporter, observation-point, sampling, template, timing and data-quality provenance.

Flow explains **who/what is using the network**. It must not be confused with packet/DPI truth or with SNMP interface utilization truth.

## Production Sensor role

Production Flow collection is assigned to a dedicated `sensor_role = FLOW`. A FLOW Sensor runs Flow receiver/decoder/template/sampling/normalization/spool/upload/health functions only, plus common Sensor control/mTLS/configuration/upgrade services. It does not run SNMP polling, discovery, NCM, synthetics or Log collection.

The same Sensor software package is reused; role exclusivity avoids software fragmentation while protecting performance.

## Ingestion architecture

```text
Exporter
 -> FLOW Sensor UDP receiver
 -> configured exporter/source attribution
 -> NetFlow/IPFIX/sFlow decoder
 -> template + sampling + sequence state
 -> canonical Flow observation
 -> bounded durable spool
 -> Core ingestion
 -> Kafka network.flow
 -> ClickHouse
 -> rollups/enrichment/analytics
```

GoFlow2 remains the selected parsing foundation; Platform owns source trust, schemas, templates, sampling semantics, enrichment, durability and analytics.

## Exporter identity and source trust

UDP source IP alone is not durable identity. Exporter state should bind tenant, listener/Sensor, configured exporter, canonical device, transport source, protocol, observation domain and exporter generation/epoch.

Unknown exporters are rejected or quarantined according to policy. Flow payload fields never establish tenant authority.

## Templates

NetFlow v9/IPFIX require bounded template lifecycle handling:

- template arrival/update/withdrawal/expiry;
- template ID reuse;
- observation-domain scoping;
- exporter restart/generation;
- options templates;
- enterprise IEs;
- variable-length fields;
- records arriving before their template.

Missing templates become visible `TEMPLATE_MISSING` quality evidence, never zero traffic. A bounded persistent template cache may survive Sensor restart only with generation/expiry safeguards.

## Sampling

Observed and estimated values are separate. Preserve sampling algorithm/rate/source/known state, observed bytes/packets, estimated bytes/packets and estimation quality. Sampling-rate changes are temporal provenance; they must not create fake volume shifts.

sFlow packet/counter samples must not be flattened into fabricated NetFlow semantics. Record kind remains visible.

## Canonical Flow data

Common typed fields should include exporter/Sensor/protocol/version/observation domain, export/observed/ingest/start/end times with quality, IPv4/IPv6, ports/protocol, bytes/packets, sampling, interfaces, VRF/VLAN, ASN, next hop, DSCP/ECN/QoS, app metadata/provenance, NAT fields when exported, template/version, source sequence and quality.

Do not make every IPFIX enterprise IE a ClickHouse column. Use strong common columns plus bounded extension attributes.

## Sequence, loss and durability

Protocol-specific sequence tracking should expose duplicate, replay, out-of-order, exporter reset and sequence gaps. Do not use one sequence algorithm for every protocol.

Quality accounting distinguishes kernel/socket drops, queue drops, malformed records, unknown exporter, missing template, unsupported IE, oversize, rate-limit, spool/storage pressure, Core/Kafka/ClickHouse failures.

Before local durable acceptance, UDP is best-effort. After certified durable acceptance, generic Sensor replay/idempotency rules apply.

## Enrichment: IP, endpoint, user, DNS, AD and geo

A Flow IP should be temporally enriched to the canonical entity when evidence exists. Enrichment may include hostname/FQDN, endpoint type, DHCP/IPAM ownership, site/subnet, user/session identity, AD/Entra attributes, VPN/RADIUS/NAC/WLAN identity, ASN, public-IP geo, cloud/SaaS provider and exporter application metadata.

Rules:

- IP is an observation, not permanent endpoint identity.
- Historical Flow uses time-correct identity/enrichment; today's DHCP/AD mapping must not rewrite yesterday's Flow.
- AD proves directory identity, not necessarily who used an IP at a specific time. User attribution needs temporal session evidence and provenance/confidence.
- Public IP geolocation is approximate; internal site mapping is preferred for internal addresses.
- Domain/FQDN and full URL are distinct. Basic Flow generally does not reveal full URLs.
- Full URL path/query storage is not a default NMS capability and is privacy-sensitive.
- Application classification records provenance: exporter-classified, DNS-correlated, port/service inferred, Pack-mapped, etc.

## Analytics and UX

Primary `Observe -> Flow / Traffic` views should include Overview, Conversations, Top Talkers, Sources, Destinations, Applications, Protocols/Ports, Sites, Interfaces, ASN, QoS, Traffic Matrix, Sankey, Exporters and Data Quality.

Conversation drill-down retains raw observation direction/provenance while presenting bidirectional conversation analytics. NAT, sampling, observation point and application-classification provenance are visible.

Key workflow: **Interface -> Investigate Traffic** opens Flow scoped to interface/time and shows top consumers, apps, sites and QoS.

## Cross-signal value

Correlate Flow with:

- SNMP interface octets/utilization and queue/discard evidence;
- routing/BGP/VRF/topology/path;
- DNS/DHCP/IPAM/entity identity;
- configuration/change events;
- incidents and Flight Recorder.

A first-class Flow Coverage quality signal can compare properly normalized Flow volume against SNMP interface traffic while accounting for sampling, direction and observation point. It is quality evidence, not a simplistic universal percentage.

## Security and fairness

Bound exporters/domains/templates/fields/record size/template churn/rates. Use per-exporter fairness so one source cannot starve others. Test same source IP across tenants, NAT/relay, forged domains, template collision and cross-tenant enrichment.

## Storage and retention

ClickHouse remains the Flow analytics store. Raw normalized Flow gets shorter configurable retention; conversation/site/interface/application aggregates can live longer; incident-protected evidence follows incident retention. Top-N and Sankey queries are bounded and use appropriate rollups.

## Implementation reality

Current canonical documents specify Flow well at product level, but production receiver/runtime, GoFlow2 dependency wiring, template/sampling state, ClickHouse Flow schema, enrichment and Flow UI must be verified from source; they must not be assumed implemented from this brief.

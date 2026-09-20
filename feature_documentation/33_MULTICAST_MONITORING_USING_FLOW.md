# Multicast Monitoring using Flow

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Flow/NTA requirements in Platform_02/23 plus network/topology/RCA architecture  
**Enhancement state:** pending Mother Reference reconciliation.

## Product objective

Use Flow telemetry to provide traffic-volume and source/group visibility for multicast while correlating it with network-control evidence from SNMP/API/CLI/gNMI. Flow alone is **not** sufficient to certify multicast routing health.

## What Flow can provide

Where the exporter/protocol exposes multicast records, Platform can identify:

- multicast destination/group address;
- source address;
- `(S,G)` traffic pattern;
- protocol/ports;
- bytes/packets or sampled estimates;
- ingress/egress interfaces;
- exporter/observation point;
- VRF/VLAN/site;
- QoS/DSCP where exported;
- temporal traffic start/stop/shift.

This supports multicast top sources/groups, site/interface distribution, volume trends and anomaly detection.

## What Flow cannot prove by itself

Flow generally does not fully prove:

- IGMP/MLD receiver membership;
- PIM neighbor health;
- RP/BSR state;
- RPF correctness;
- multicast routing table correctness;
- receiver reachability;
- packet loss at every hop.

Those require correlation with SNMP, CLI/API/gNMI, routing tables and receiver/network evidence.

## Canonical multicast analytics

Useful derived views:

- Top Multicast Groups;
- Top Multicast Sources;
- Source -> Group matrix;
- Group -> Site/interface distribution;
- unexpected/new groups;
- traffic disappearance;
- sudden fan-out or volume surge;
- per-group QoS;
- duplicate/multi-observation path evidence.

## Identity

Preserve IPv4 multicast ranges and IPv6 multicast addresses as actual group identifiers. Do not treat multicast destination IP as an endpoint entity like a unicast host.

Create logical multicast group/source-group objects only where useful, tenant/VRF scoped and provenance-aware.

## Sampling

Sampled Flow remains estimated. A sampled multicast record must not be presented as exact packet delivery volume.

## Cross-signal health

Example qualification:

```text
Multicast Flow disappears
+ SNMP interface traffic remains
+ PIM/RP/IGMP state degraded
 -> strong multicast-path hypothesis

Multicast Flow disappears
+ exporter/template/sequence health degraded
 -> Flow monitoring degradation, not multicast outage
```

## UI

`Observe -> Flow -> Multicast` may provide:

- source/group filters;
- group traffic timeline;
- sites/interfaces;
- exporters;
- receivers where independently learned;
- PIM/IGMP/MLD context;
- related topology/path;
- Data Quality;
- related alerts/incidents.

## Alert semantics

A Flow-only traffic disappearance should not automatically generate a "multicast down" alert. Use Alert Evidence Contracts combining Flow quality, exporter health and control-plane/receiver evidence.

## Certification

Test IPv4/IPv6 multicast, multiple VRFs, sampled/unsampled export, multiple observation points, exporter loss/template gaps and correlation with PIM/IGMP/MLD evidence.

---
name: flow_telemetry
description: Flow export collection, template/replay/drop counters, cardinality control and Sensor isolation from flow floods.
---

# flow_telemetry

## Purpose

Ingest NetFlow/sFlow/IPFIX with correct exporter identity, sampling and loss visibility—without letting flow floods or weak flow signals drive false reachability/DOWN conclusions.

## Use when

Flow listener/collector changes, template handling, sampling math, cardinality controls, Sensor flow worker isolation, or analytics derived from flow. Required for `platform-network-engineer` / Sensor flow paths.

## Authoritative inputs

Always read:

1. Architecture notes on flow as telemetry (not sole reachability proof)
2. Existing Sensor flow listener/worker code and drop/sequence counters on the exact head
3. `Platform_11` cardinality/retention guidance
4. Device Pack / exporter inventory identity rules
5. Storage-protection / backpressure behaviour for Sensor spool

If IPFIX mediation features or specific vendor templates are not implemented, mark `UNVERIFIED`.

## Invariants

- NetFlow/sFlow/IPFIX semantics differ—do not conflate sampling and counter fields across protocols.
- Exporter identity is anchored to Platform entity/sensor observation—not raw IP alone when DHCP/NAT churn exists.
- Sampling rates must be recorded; unsampled assumptions are forbidden when sample_rate ≠ 1.
- Sequence loss / UDP loss visibility is mandatory for evidence quality (counters, gaps).
- Timestamp correctness (flow start/end vs receive time) is documented per path.
- High-cardinality control (prefixes, ports, labels) prevents CH/Sensor explosion.
- Dedupe/replay expectations are explicit under at-least-once upstream.
- Backpressure: Sensor isolates flow floods from other workers; 80% storage protection still applies.
- **No false reachability conclusions:** missing flows ≠ interface/device DOWN; flow is corroboration/analytics.

## Required procedure

1. State protocol (NetFlow v5/v9, sFlow, IPFIX) and template/version assumptions present in code.
2. Verify exporter→entity mapping and tenant/site scope.
3. Confirm sampling field handling and document uncertainty when absent.
4. Expose or verify sequence/drop/UDP loss counters; add tests for gap detection where code exists.
5. Bound cardinality (aggregation, allowlists, bloom/limits as implemented); refuse unbounded raw explode.
6. Validate timestamp fields; reject silent TZ/epoch mistakes in parsers under change.
7. Prove backpressure/flood isolation or mark `UNVERIFIED` with risk.
8. Ensure alert/RCA paths do not declare DOWN from flow absence alone.

## Negative / failure cases

- Declaring WAN DOWN because flow PPS dropped (could be sampling/exporter mute).
- Ignoring sequence gaps while claiming complete forensic flow.
- Storing per-flow labels that explode cardinality overnight.
- Collapsing sFlow counter samples into byte-accurate billing claims without caveats.

## Security / tenant implications

Flow payloads can reveal internal addressing—tenant isolation on store/query is mandatory. Raw PCAP-like exports are out of scope unless separately authorized; do not widen into always-on DPI. Redact secrets if flow options templates carry them.

## Required evidence

- Protocol/template coverage statement
- Loss/sequence counter evidence or `UNVERIFIED`
- Cardinality bounds
- Backpressure/flood test or risk note
- DOWN-separation statement
- Exact SHA

## Stop conditions

Stop when:

- Design uses flow as sole reachability authority
- Cardinality unbounded with no control plan
- Exporter identity cannot be tenant-scoped
- Flood isolation absent yet production enablement is claimed

## Anti-patterns

- “Flows look quiet so the link is down”
- Disabling drop counters to hide loss
- Writer self-certifying flow correctness from a lab generator alone without assumptions

## Handoff / output

Return: protocol notes; loss/cardinality evidence; alert-safety statement; nextOwner (Sensor/network verifier). Not MERGE_READY alone.

## Non-ownership

Does not own SNMP polling, packet capture DPI productization, or CH schema alone (`clickhouse`). Owns flow-ingest evidence discipline.

# Correlation

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 03, 06, 19, 22

## Product objective

Reduce operational noise and improve incident understanding by correlating independent evidence across entities, protocols, topology, time, changes and service dependencies without turning correlation into unsupported causation.

## Correlation domains

Platform correlation may combine:

- alerts and events;
- SNMP/gNMI/API/Agent metrics;
- Flow;
- Logs/Syslog;
- OTLP;
- cloud/Kubernetes;
- synthetics;
- topology/dependencies;
- routing/path evidence;
- NCM/configuration changes;
- DNS/DHCP/IPAM/AD/PKI/shared-service evidence;
- external provider/ITSM/change evidence.

## Canonical identity first

Correlation uses tenant-scoped canonical entity/service identities rather than joining arbitrary protocol keys. IP, hostname, ifIndex, pod name and vendor IDs are observations/provenance, not universal identity by themselves.

## Correlation dimensions

- temporal proximity;
- same entity/service;
- parent/child or dependency relationship;
- same path/circuit/site;
- same change window;
- same protocol/session/exporter/source;
- shared failure domain;
- repeated fingerprint/pattern;
- multi-vantage agreement/disagreement.

## Deduplication vs correlation

Deduplication collapses repeated representations of the same logical evidence while retaining counts/provenance.

Correlation links distinct evidence that may be related.

Do not globally deduplicate Flow records observed at different network points merely because their 5-tuples match; those are valid multi-observation evidence.

## Causation boundary

Correlation may say:

- "these events are related";
- "this change preceded the symptom";
- "these entities share a dependency".

It may not automatically say:

- "this change caused the outage";
- "this top talker caused congestion".

Causal RCA requires mechanism and supporting/contradictory evidence.

## Topology-aware suppression

When a parent/dependency failure explains multiple child alerts, Platform may group/suppress workflow noise while preserving child evidence and allowing recovery verification.

## Cross-signal examples

- interface utilization + queue drops + latency + Flow top consumers;
- DNS failure + application synthetic failure + service dependency;
- BGP change + path shift + site-to-site Flow shift;
- database replication lag + storage pressure + application latency;
- log error-pattern surge + deployment change + metric anomaly.

## Quality

Correlation confidence must account for telemetry freshness, clock quality, source health and missing evidence. Bad clocks or stale topology reduce confidence.

## UX

Incident/RCA views show why items were correlated, which relationship/time rule matched, contradictions, source quality and alternative explanations.

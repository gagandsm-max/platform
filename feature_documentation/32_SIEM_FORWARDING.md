# SIEM Forwarding

**Class:** Derived feature brief — non-authoritative  
**Canonical sources:** Platform_04, 10, 17, 18, 23  
**Draft enhancement:** PR #67  
**Pending correction:** central connector-runtime forwarding is the default; direct Sensor->SIEM is optional/evidence-gated.

## Product objective

Allow Platform-collected operational logs/events and selected Platform intelligence to be securely routed to customer SIEM/log platforms without turning Platform into a full SIEM.

## Operating modes

Per governed source/route:

- `STORE_ONLY`;
- `STORE_AND_FORWARD`;
- `FORWARD_ONLY`.

`FORWARD_ONLY` must clearly disclose reduced Platform historical search/RCA capability unless separate evidence is retained.

## Forwardable classes

Collected telemetry:

- Syslog;
- application/OS/container logs;
- OTLP Logs;
- cloud/custom log events;
- selected operational events.

Platform intelligence:

- qualified alerts;
- incidents;
- anomalies;
- bounded RCA summaries/references;
- topology/change/NCM events;
- selected Platform/Sensor health events.

## Destination model

SIEM/log destinations use the common connector runtime and Integration/Capability Packs. Potential certification targets include Generic Syslog, Generic HTTPS, QRadar, Splunk, Microsoft Sentinel, ArcSight, Elastic and OTLP where appropriate.

Named products are targets, not hard-coded Core dependencies.

## Formats

Where certified:

- RFC5424;
- legacy RFC3164;
- JSON;
- CEF;
- LEEF;
- OTLP Logs;
- target-native APIs.

Routes may send RAW, NORMALIZED or ENRICHED representations while preserving source evidence/provenance.

## Default forwarding placement

**Default:** central Platform connector-runtime forwarding.

Direct LOG/FLOW Sensor -> external SIEM is not the initial required path. It is optional only when benchmark/residency/WAN evidence proves the central path insufficient and the additional credential/state/failover complexity is justified.

## Reliability

Each destination has independent bounded logical queue/backpressure/health. One failed target must not block another.

Expose states such as:

- queued;
- sending;
- transport accepted;
- destination acknowledged where provable;
- retrying;
- rate limited;
- failed;
- dead letter;
- policy dropped.

Successful UDP/socket send must not be labeled "indexed in SIEM".

## Replay / dead letter

Authorized bounded replay includes destination, time/scope, event/byte estimate, rate, confirmation and audit. Repeated mapping/schema rejection becomes visible dead-letter state.

## Security

Requirements include:

- tenant-safe destination/credential binding;
- endpoint/scheme validation;
- controlled DNS resolution;
- SSRF/DNS-rebinding protection;
- egress policy/allowlist;
- TLS/mTLS/token/OAuth as applicable;
- separate storage vs egress sensitive-data policy;
- loop prevention using trusted origin/route/hop metadata;
- audit.

## Implementation note

PR #67 contains detailed draft architecture but is stale/conflicted with newer main and must be reconciled before any canonical/implementation claim.

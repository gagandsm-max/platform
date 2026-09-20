# SNMP

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_02, 03, 04, 05, 06, 07, 09, 10, 11, 14, 17, 18, 22, 23
**Draft enhancement:** PR #69

## Product objective

Provide enterprise-grade SNMP monitoring that preserves protocol truth, scales safely, separates monitoring-path failure from device failure, and gives operators direct troubleshooting evidence.

## Core capability

Platform supports SNMP v1/v2c/v3, GET, GETNEXT, GETBULK, WALK/BULKWALK, retry/backoff, bounded execution, typed errors and governed Credential Profiles. Routine monitoring is read-only. SNMP SET belongs only to privileged NCM/remediation workflows.

SNMP data feeds the canonical entity graph and can produce:

- numeric measurements;
- exact Counter32/Counter64 observations;
- state/enumeration observations;
- inventory facts;
- topology/relationship facts;
- trap/inform events;
- collection-quality/exception evidence.

## Correctness requirements

Counter64 must remain exact before rate derivation. A valid value above the IEEE-754 exact integer range must not be rejected or silently rounded. Counter rate processing uses actual elapsed time and handles wrap, device reboot/reset and interface discontinuity using evidence such as `sysUpTime` and `ifCounterDiscontinuityTime` where available.

`NoSuchObject`, `NoSuchInstance` and `EndOfMibView` are explicit quality outcomes; an optional unsupported OID must not automatically invalidate unrelated valid observations.

`ifIndex` is not permanent interface identity. Interface reconciliation should use ifName/ifDescr/ifAlias, MAC, physical slot/port, entPhysical/vendor identifiers and temporal evidence, with ambiguity surfaced rather than silently merged.

## Poll execution

Target production path:

```text
Core policy / assignment / Credential Profile
  -> Sensor scheduler + fencing
  -> Capability Pack poll definitions
  -> bounded Poll Planner
  -> GoSNMP
  -> typed normalization
  -> durable Sensor spool
  -> Core ingestion
  -> Kafka / ClickHouse / graph / evidence consumers
```

The Poll Planner is logical functionality inside the Sensor runtime, not a separate service. It governs GET versus GETBULK, scalar/table grouping, max repetitions, timeout/retry, device quirks, `tooBig` fallback, deterministic fleet spreading and per-device concurrency.

## Security

Preferred posture is SNMPv3 `authPriv` with supported SHA-2/AES where device compatibility permits. v1/v2c, MD5, SHA-1 and DES are legacy compatibility modes with explicit selection/warning/audit; no silent downgrade.

No implicit `public` community. Credential Profile `USE` does not mean `REVEAL`; reusable communities/auth/privacy secrets must stay out of telemetry, audit, Kafka, ClickHouse, support bundles and ordinary job history.

## Traps and informs

Trap/inform input must be bounded, source-attributed, decoded through MIB/Capability Pack semantics, storm-controlled and fed into Alert Evidence rather than directly becoming an alert.

- Trap: unacknowledged protocol behavior.
- Inform: ACK only after the certified receiver acceptance/accounting boundary; ACK does not mean Core persistence, ClickHouse persistence, alert creation or incident creation.
- v1/v2c trap source identity is weak evidence; v3 security is stronger where configured.

## MIB Studio / troubleshooting

MIB Studio should support upload, dependency/import validation, OID tree, textual conventions, enums, tables/indexes, notifications, provenance/licensing and bounded sample-device GET/WALK/BULKWALK. Numeric OID monitoring remains possible without a MIB if an explicit semantic mapping exists.

SNMP Test should show Sensor/vantage, target, version, Credential Profile name, connectivity/auth/authz, identity OIDs, latency/retries, raw OID, resolved name, typed value, decoded value, stable error and remediation — never secrets.

## Monitoring quality

Expose poll success, duration, attempted/successful OIDs, timeout/auth/authz/tooBig/noSuch counts, retries, freshness/staleness, expected-vs-received samples and last success.

Valid state example: **DEVICE HEALTHY + SNMP MONITORING DEGRADED**.

## Implementation note

PR #69 strengthens these contracts but is still draft. Existing protocol code is materially ahead of the complete production SNMP control path; production scheduler wiring, exact Core Counter64 handling, typed non-numeric Core ingestion, Pack execution, traps/informs, MIB Studio and quality UI require exact-source verification before any readiness claim.

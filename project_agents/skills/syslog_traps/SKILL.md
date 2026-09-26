---
name: syslog_traps
description: Syslog and SNMP trap ingestion, source attribution, storm control and evidence-quality parsing.
---

# syslog_traps

## Purpose

Ingest syslog and SNMP traps with trustworthy source identity, loss visibility and storm control—without letting malformed or bursty events corrupt tenant routing or invent severity.

## Use when

Syslog/trap listeners, parsers, severity maps, dedupe windows, TLS syslog, or Sensor event ingress changes. Required for network-event paths and alert evidence that cites traps/syslog.

## Authoritative inputs

Always read:

1. Existing syslog/trap listener code and transport options on the exact head
2. Architecture notes on event evidence vs authoritative state
3. Tenant/site routing and Sensor assignment rules
4. Retention/raw-event storage boundaries (`Platform_11`, privacy constraints)
5. Device Pack / parser catalogues when present

If RELP, mutual-TLS syslog, or advanced trap correlation is not implemented, mark `UNVERIFIED`.

## Invariants

- Transport may be UDP/TCP/TLS as implemented; UDP loss is expected—visibility required.
- Source identity: prefer authenticated/TLS identity or managed Sensor vantage; raw UDP spoofing risk must be stated.
- Loss visibility (drops, queue overflows) is part of evidence quality.
- Parsing is bounded; malformed events are counted and parked—not able to crash the worker.
- Timestamps: device vs receive time distinguished; TZ assumptions documented.
- Trap/syslog dedupe windows prevent storms from creating alert floods.
- Severity normalization maps vendor severities into Platform classes without collapsing distinct states into DOWN.
- Tenant/site routing is server-side from Sensor/source binding—not attacker-controlled PRI fields alone.
- Raw-event retention is bounded and redacted per policy; not unlimited forensic tape by default.

## Required procedure

1. State transports in scope and their loss/auth properties.
2. Verify source→tenant/site binding; document spoofing residual risk for UDP.
3. Implement/verify parsers with malformed-event fixtures; never eval user content.
4. Define dedupe/burst keys and rate limits; test burst behaviour.
5. Normalize severity/facility with explicit maps; refuse silent “all critical”.
6. Record drop counters and queue backpressure; tie to Sensor isolation.
7. Confirm retention/redaction for raw payloads; secrets stripped.
8. Ensure alert paths treat syslog/trap as evidence—not sole DOWN without policy/corroboration.

## Negative / failure cases

- Trusting UDP syslog hostname field as tenant authority.
- Parser panic on malformed CEF/LEE F.
- Deduping away distinct incidents incorrectly (over-agg).
- Storing full payloads with passwords from config-change syslogs.
- Trap flood taking down SNMP pollers on the same Sensor.

## Security / tenant implications

Cross-tenant event injection is a critical failure class. TLS client auth when available. PII/secret redaction on export. Support bundles sample, not dump unbounded raw UDP.

## Required evidence

- Transport/auth matrix
- Malformed/burst test results
- Dedupe/rate-limit settings
- Tenant routing proof
- Retention/redaction notes
- Exact SHA; `UNVERIFIED` transports

## Stop conditions

Stop when:

- Tenant routing can be spoofed without mitigation disclosure
- Worker crashes on malformed input
- Unlimited raw retention requested against policy
- Trap/syslog alone would force DOWN semantics improperly

## Anti-patterns

- “UDP is fine, we never lose messages”
- Regex-only parsing of untrusted blobs without size limits
- Writer self-certifying parser completeness for all vendors

## Handoff / output

Return: ingest safety notes; storm controls; evidence-quality caveats; nextOwner (Sensor/telemetry verifier). Not MERGE_READY alone.

## Non-ownership

Does not own SNMP polling (`snmp_mib`), full SIEM replacement, or always-on DPI. Owns syslog/trap ingest discipline.

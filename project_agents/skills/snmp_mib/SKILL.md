---
name: snmp_mib
description: SNMP/MIB collection, SNMPv3 security, counter semantics and Device Pack mapping without declaring device-down from SNMP alone.
---

# snmp_mib

## Purpose

Collect SNMP evidence with protocol-correct security and counter semantics, Device Pack mapping, and strict separation of SNMP failure from device DOWN.

## Use when

SNMP poller/worker changes, MIB/OID mapping, SNMPv3 credential handling, counter/interface metrics, bulk-walk tuning, or Device Pack SNMP collection. Required for Sensor SNMP paths and `platform-network-engineer`.

## Authoritative inputs

Always read:

1. Device Pack schema/identification for the target family
2. Credential Profile rules for SNMP communities/USM
3. Existing SNMP worker timeout/retry/bulk code on the exact head
4. Mother rule: do not declare device-down solely from SNMP failure when corroboration exists
5. `Platform_10` / MIB guidance when present

If a MIB compiler, auto-fallback pack, or specific vendor MIB is not in-tree, mark `UNVERIFIED`.

## Invariants

- Support SNMP **v1/v2c/v3** only as implemented; v3 auth/priv parameters are mandatory when v3 is selected.
- Credentials never logged; engineID/time-window issues are diagnosed as collector auth problems—not inventing new secrets in tickets.
- **Counter wrap vs Counter64:** 32-bit counters need wrap-aware deltas; prefer Counter64 where available; rate math must state assumptions.
- Index/table walking respects lexicographic order and sparse tables; no naive +1 index assumptions across vendors.
- Timeout/retry budgets are bounded; poll-storm control prevents synchronized stampedes.
- MIB fallback (standard → vendor) is pack-defined—not ad-hoc OID guessing in Core.
- Bulk limits (max-repetitions/getbulk size) protect agent CPU and Sensor threads.
- Unsupported OIDs: classified as capability gaps/`UNSUPPORTED`, not device failure.
- **SNMP poll failure ≠ device DOWN** when ICMP/other corroboration remains credible.

## Required procedure

1. Identify pack, SNMP version and credential profile for the target.
2. Validate timeout/retry/bulk settings against device class; avoid storming large tables.
3. Map OIDs via Device Pack; record unsupported OIDs explicitly.
4. For counters: choose Counter64 when present; implement wrap-safe deltas for 32-bit; document poll interval vs wrap risk.
5. Handle SNMPv3 engineID/time sync failures as monitoring-degraded/auth errors with remediation steps that do not weaken crypto.
6. Separate collector errors (timeout, auth, tooBig) from operational interface/device states.
7. Add tests for wrap, sparse tables and auth failure where code changes.
8. Confirm alert paths require corroboration before DOWN.

## Negative / failure cases

- Marking device DOWN on SNMP timeout alone while ping succeeds.
- Using Counter32 rates without wrap handling on busy links.
- Walking entire trees every poll without bulk limits.
- Pasting v3 authKey into logs for “debugging”.
- Hard-coding vendor OIDs outside packs.

## Security / tenant implications

SNMPv3 priv is required where policy says so; v1/v2c communities are secrets. Tenant isolation on metric storage/query. No cross-tenant credential reuse.

## Required evidence

- Pack/OID mapping references
- Version/credential class (not secret values)
- Timeout/retry/bulk settings
- Counter semantics notes
- Corroboration/DOWN-separation proof
- Exact SHA; `UNVERIFIED` MIB gaps

## Stop conditions

Stop when:

- DOWN would be asserted from SNMP alone
- v3 parameters would be weakened to “make it work”
- Pack mapping absent and ad-hoc OIDs are proposed as permanent
- Poll storm risk unmitigated for large estates

## Anti-patterns

- Community string `public` in fixtures committed as production default
- Ignoring `tooBig`/resource errors
- Writer self-certifying SNMP pack certification without lab evidence

## Handoff / output

Return: collection semantics; failure classification; tests; nextOwner (Device Pack / Sensor verifier). Not MERGE_READY alone.

## Non-ownership

Does not own trap ingestion (`syslog_traps`), flow, or invent MIB certification. Owns SNMP poll evidence discipline.

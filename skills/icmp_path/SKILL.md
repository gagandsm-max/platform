---
name: icmp_path
description: ICMP/path reachability, RTT/loss/timeout evidence and path diagnostics as corroboration, not sole identity.
---

<!-- generated from project_agents/skills/icmp_path/SKILL.md sha256:30d0159d6bc56a27499538d9f6bc68e2adc4027f3385b886720b276a98c1eb6e -->

# icmp_path

## Purpose

Produce ICMP/path reachability evidence (echo, loss, RTT, traceroute) as corroboration—with honest UNKNOWN semantics—and avoid ICMP-only device DOWN unless policy explicitly authorizes a corroborated conclusion.

## Use when

ICMP polling, path/traceroute diagnostics, reachability corroboration for alert evidence, or Sensor/Agent ping workers. Required for network reachability paths.

## Authoritative inputs

Always read:

1. Mother rule: do not declare device-down solely from one weak monitor when corroboration exists
2. Existing ICMP/path worker code on the exact head
3. Alert Evidence Contract / `alert_quality_evidence` Skill
4. Rate-limit and vantage (Sensor locality) constraints
5. IPv4/IPv6 support matrix actually implemented

If TCP ping, ECMP-aware path or continuous path recording is absent, mark `UNVERIFIED`.

## Invariants

- ICMP echo success/fail is evidence—not identity.
- Packet loss and RTT are measured with stated probe counts/intervals.
- Traceroute/path hops are best-effort; ACL/firewall filtering and asymmetric routing can hide hops.
- Rate limiting prevents probe storms.
- Unreachable vs monitoring-path unavailable are distinct (Sensor cannot reach target ≠ target down globally).
- IPv4/IPv6 handled per implementation; do not assume v6 parity.
- DNS independence: prefer numeric targets for the probe itself when name resolution is separate evidence.
- Retries/timeouts bounded; classify timeout as collector/path uncertainty when appropriate.
- **UNKNOWN** when vantage/path health is degraded.
- **No ICMP-only device DOWN** unless an approved policy explicitly authorizes that evidence contract with required corroboration rules met.

## Required procedure

1. Confirm ICMP/path capabilities on this SHA; list `UNVERIFIED` methods.
2. Define probe parameters (count, interval, timeout, payload) and rate limits.
3. Separate Sensor vantage failure from target failure in results/alerts.
4. Record loss/RTT distributions; avoid single-probe flapping.
5. For traceroute: document ACL/asymmetry caveats; do not invent missing hops.
6. Align alert policies with evidence contracts; refuse ICMP-only DOWN unless explicitly authorized and tested.
7. Add tests for timeout, filtered ICMP, and IPv6 if claimed.
8. Ensure identity continues to use `entity_id`—not last-responsive IP alone.

## Negative / failure cases

- Marking device DOWN on one missed ping while SNMP/Agent healthy.
- Flooding ICMP and triggering network ACL blocks (self-inflicted).
- Treating traceroute gap as proof of link cut.
- Using DNS failure as ICMP failure without separation.

## Security / tenant implications

Probes stay within tenant-authorized targets. No arbitrary Internet scanning. Path output may reveal internal topology—RBAC on diagnostics. Rate limits are a safety control.

## Required evidence

- Probe parameter docs
- Vantage vs target failure classification
- Alert-policy interaction notes
- Test results; `UNVERIFIED` methods
- Exact SHA

## Stop conditions

Stop when:

- ICMP-only DOWN would ship without authorized evidence contract
- Rate limits absent for estate-scale enablement
- Vantage failure collapsed into target DOWN
- IPv6 claimed without implementation

## Anti-patterns

- “Ping is truth”
- Disabling jitter so all Sensors ping on the second
- Writer self-certifying path analytics completeness

## Handoff / output

Return: probe semantics; corroboration rules; risks; nextOwner (network/alert verifier). Not MERGE_READY alone.

## Non-ownership

Does not own SNMP/flow, invent continuous path recorder, or set contractual SLAs. Owns ICMP/path evidence discipline.

---
name: topology_routing
description: Routing/topology discovery, network-service dependencies (DNS/DHCP/NTP/AAA) and provenance/confidence on graph edges.
---

<!-- generated from project_agents/skills/topology_routing/SKILL.md sha256:38b00477b9adc633c1f47f049c75316ed868a8fa0f67c51867da7f862270bf12 -->

# topology_routing

## Purpose

Derive topology and routing relationships with explicit evidence provenance and confidence—never letting a single weak signal drive destructive service-impact conclusions.

## Use when

LLDP/CDP/routing-table ingestion, graph edge creation, dependency mapping (DNS/DHCP/NTP/AAA), topology UI/API, or RCA subgraph inputs. Required for `platform-knowledge-graph-engineer` / network topology work.

## Authoritative inputs

Always read:

1. Temporal operational graph / topology Mother sections when present (`Platform_03`, graph skills)
2. Existing discovery collectors and confidence fields on the exact head
3. Tenant isolation rules for graph queries
4. Alert/RCA rules that consume topology (contradiction handling)
5. Decision Register entries for unresolved graph-database choices—do not invent a new graph DB

If L3 correlation, traffic-engineered overlays or automated edge deletion policies are not built, mark `UNVERIFIED`.

## Invariants

- Evidence sources (LLDP, CDP, routing tables, configs, agent, operator) are recorded per edge.
- Link confidence is multi-signal; weak single sources stay low-confidence.
- Stale edges age out or are marked STALE—do not present as current without timestamps.
- Contradictions (A→B via LLDP vs routing saying otherwise) are retained as contradictions, not silently overwritten.
- Dependency graph effects on RCA are advisory unless corroborated.
- Tenant isolation on graph read/write is mandatory.
- Topology churn must not flood notifications/alerts (`Platform_20` awareness).
- **No single weak signal** causes destructive service-impact or auto-remediation conclusions.

## Required procedure

1. List evidence sources used for each edge type in scope; verify code paths exist.
2. Define confidence/provenance fields; refuse edges without source + time.
3. Handle stale detection (last-seen, TTL); test churn.
4. On contradiction, record both claims; do not delete the weaker without policy.
5. Separate shared-service dependencies (DNS/DHCP/NTP/AAA) as dependency evidence—not facilities/BMS scope.
6. Enforce tenant predicates on graph APIs.
7. Ensure RCA/alert consumers expose confidence and contradictions (`INCONCLUSIVE` allowed).
8. Mark unimplemented discovery methods `UNVERIFIED`.

## Negative / failure cases

- One LLDP frame ⇒ “cut this link” remediation.
- Deleting edges on transient discovery miss.
- Cross-tenant graph traversal via shared CE device IDs.
- Treating CDP from untrusted port as authoritative without caveats.
- Notification floods on scan churn.

## Security / tenant implications

Topology reveals attack surface—export is RBAC-scoped. Do not expose management IPs broadly. Operator edits are audited. No silent merge of conflicting identities (`Platform_20`).

## Required evidence

- Source/confidence model
- Stale/contradiction test results
- Tenant isolation proof
- RCA consumption notes
- Exact SHA; `UNVERIFIED` discovery methods

## Stop conditions

Stop when:

- Design asserts certainty from a single weak signal
- Tenant isolation missing on graph APIs
- Auto-remediation would fire from topology alone
- New graph database selection is implied without Decision Register

## Anti-patterns

- Overwriting provenance to “clean the UI”
- Equating drawing tools with discovery truth
- Writer self-certifying topology completeness estate-wide

## Handoff / output

Return: edge model; confidence rules; contradiction handling; nextOwner (graph/network verifier). Not MERGE_READY alone.

## Non-ownership

Does not own GNN training, invent CMDB replacement semantics, or select graph DB tech. Owns topology evidence discipline.

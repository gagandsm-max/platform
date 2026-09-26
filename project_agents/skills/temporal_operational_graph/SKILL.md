---
name: temporal_operational_graph
description: Temporal operational graph, topology provenance/confidence and relationship history.
---

# temporal_operational_graph

## Purpose

Maintain a tenant-scoped temporal operational graph of typed nodes/edges with provenance, valid-time semantics and contradiction handling—without unsupported causal inference.

## Use when

Graph schema changes, topology history, dependency edges, entity merge/split, graph query APIs, or RCA subgraphs. Required for `platform-knowledge-graph-engineer` work.

## Authoritative inputs

Always read:

1. Architecture / Mother sections on operational graph (not CMDB replacement semantics)
2. Existing graph schema and storage on the exact head—verify what is implemented
3. `topology_routing` and `Platform_20` identity/reconciliation rules
4. Tenant isolation requirements for graph APIs
5. Decision Register before inventing a new graph database

If bi-temporal store or GNN features are absent, mark `UNVERIFIED`.

## Invariants

- Nodes/edges are typed; unknown types fail closed or map explicitly.
- Tenant boundary on every read/write; no cross-tenant traversal.
- Provenance (source, method, actor) on edges.
- `first_seen` / `last_seen` (or equivalents) maintained.
- Confidence is stored and queryable.
- Valid-time vs ingest-time distinguished when both exist; otherwise document the single clock.
- Stale edges are labelled or expired—not presented as current without timestamps.
- Contradictory evidence co-exists under policy—not silent overwrite.
- Topology snapshots / dependency history support point-in-time questions as implemented.
- Entity merge/split follows reconciliation rules—never silent duplicate collapse.
- Late telemetry updates history honestly; does not fabricate earlier certainty.
- Graph churn must not flood notifications.
- Query safety: bounds, authz, no unbounded recursive walks without limits.
- **No unsupported causal inference** from edge presence alone.

## Required procedure

1. Confirm graph features on this SHA; list `UNVERIFIED` capabilities.
2. Define/extend types with provenance + confidence fields.
3. Implement valid-time/ingest-time handling as designed; test late updates.
4. Enforce tenant predicates; negative-test cross-tenant queries.
5. Handle stale and contradictory edges in API responses.
6. For merge/split: require explicit reconciliation evidence (`Platform_20`).
7. Bound queries (depth/limit); add abuse tests if API exposed.
8. Ensure RCA consumers treat graph edges as evidence with confidence—not proof.

## Negative / failure cases

- Overwriting LLDP edge with CDP without retaining contradiction.
- Cross-tenant edge via shared PE device ID mistake.
- Interpreting “depends_on” as proven root cause.
- Unbounded graph export to browser.
- Silent merge of two entities after hostname collision.

## Security / tenant implications

Graph reveals attack paths—RBAC on queries/exports. Mutations audited. No prompt/LLM privilege to rewrite edges. PII in node properties minimized.

## Required evidence

- Schema/type notes
- Tenant negative tests
- Stale/contradiction/late-data tests
- Query bounds
- Exact SHA; `UNVERIFIED` list

## Stop conditions

Stop when:

- Tenant isolation broken
- New graph DB implied without Decision Register
- Causal claims forced from weak edges
- Merge/split would silent-destroy provenance

## Anti-patterns

- Treating drawing-tool graphs as temporal truth
- Deleting history to “save storage” without policy
- Writer self-certifying graph completeness

## Handoff / output

Return: schema/temporal rules; test evidence; gaps; nextOwner (graph verifier). Not MERGE_READY alone.

## Non-ownership

Does not own GNN training authority, invent CMDB, or select graph DB tech casually. Owns temporal operational graph discipline.

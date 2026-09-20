# Root Cause Analysis (RCA)

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 03, 19, 22

## Product objective

Produce evidence-backed root-cause hypotheses by combining deterministic state, topology/dependencies, temporal order, changes, telemetry quality and bounded AI assistance.

## RCA contract

RCA output should expose:

- observed facts;
- affected entities/services;
- likely cause/hypotheses;
- confidence;
- supporting evidence;
- contradictory evidence;
- missing evidence;
- change/topology context;
- alternative hypotheses;
- next validation action;
- `INCONCLUSIVE` when evidence is insufficient.

## Evidence sources

Potential evidence includes:

- SNMP/gNMI/API/Agent metrics and states;
- Flow;
- logs/events;
- OTLP;
- topology/dependency graph;
- routing/path evidence;
- DNS/DHCP/AD/PKI/shared services;
- configuration/NCM changes;
- cloud/provider events;
- synthetics;
- diagnostics;
- collector/source quality.

## Deterministic first

Protocol and topology facts can establish some causal relationships directly. Statistical anomaly and GNN rank hypotheses. LLM explains evidence and suggests authorized validation; it does not become causal authority.

## Contradictions

RCA must preserve contradictions rather than hide them. Example: SNMP timeout with ICMP/API/neighbor evidence healthy should prefer monitoring-path degradation over device-down.

## Active Evidence Acquisition

When uncertainty is material, Platform may collect bounded authorized diagnostics such as ping/path, SNMP test, API test, log query or configuration evidence. Acquisition obeys rate/resource/security limits.

## Change Intelligence

Correlate configuration, deployment, routing, cloud and external-provider changes to incident onset. Correlation is not automatic causation; hypotheses require mechanism/evidence.

## Similar Incident Memory

Historical similarity is advisory. Similar incidents can provide candidate causes/actions but must not silently set current root cause.

## Incident integration

RCA is versioned evidence linked to Incident timeline and may be recomputed as new data arrives. Human/operator feedback is auditable input.

## Automation boundary

RCA can recommend remediation. Execution still passes deterministic NCM/remediation authorization, preconditions, approval, blast-radius, validation and rollback controls.

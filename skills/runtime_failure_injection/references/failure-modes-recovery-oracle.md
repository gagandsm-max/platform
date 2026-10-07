# Failure modes and recovery oracle

## Inject
- Failure before vs after durable acceptance
- Lost ACK
- Crash
- Dependency outage: Kafka, ClickHouse, DB
- Queue saturation

## Oracle
- Recovery oracle must distinguish: accepted durable work vs never-accepted.
- Source inference ≠ runtime evidence.

## Forbidden shortcuts
- Never use destructive Docker prune / volume deletion shortcuts to “fix” state.

# OpenTelemetry / OTLP

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_01, 02, 03, 04, 07, 10, 11, 19, 22  
**Current scope:** metrics and logs initially; trace-ready, not full native APM claim.

## Product objective

Accept OpenTelemetry signals through standard OTLP interfaces, normalize them into Platform identity/evidence contracts and correlate them with infrastructure/network/service data.

## Initial signals

- OTLP Metrics;
- OTLP Logs.

Architecture remains trace-ready, but full traces/APM are not marketed until storage, dependency extraction, retention, analysis, sampling and UX are implemented and accepted.

## Collection topology

Supported deployments may include customer-owned OTel SDKs/Collectors and optional governed Platform Collector profiles.

```text
Apps / OTel SDKs / Collectors
 -> OTLP gRPC/HTTP
 -> authenticated Platform ingestion / eligible gateway
 -> tenant + resource identity normalization
 -> bounded durability
 -> Kafka / ClickHouse / log/metric consumers
 -> Alert/RCA/graph
```

Do not fork OpenTelemetry Collector merely to own standard protocol behavior.

## Identity and tenancy

Authenticated connection/configuration establishes tenant authority. Telemetry attributes cannot override tenant identity.

Use OTel resource/service attributes to help resolve canonical entity/service identity, with ambiguity surfaced.

## Semantic conventions

Preserve standard resource/signal fields and schema/version provenance. Platform-specific normalization should not destroy source attributes required for debugging.

## Cardinality

OTel can create very high label/resource cardinality. Apply visible cardinality guardrails, quotas and data-quality feedback; no silent uncontrolled expansion.

## Security

TLS/mTLS/token/auth profile as configured, tenant-scoped quotas, payload bounds, schema validation and egress/privacy policy. Logs and span/event text are untrusted AI input.

## Reliability

High-rate OTLP must not starve certified monitoring. Capacity Engine may require dedicated ingestion resources/roles based on benchmarks. Durable acceptance/replay follows the generic telemetry-durability contract.

## Cross-signal correlation

Where trace/span IDs exist in logs and future trace support, retain them for correlation. Metrics/logs/traces remain separate source signals feeding one entity/service/dependency model.

## Implementation boundary

"OTLP supported" does not mean "full APM." Any dedicated OTLP specialized authority should only be created if cross-document review proves it is needed and does not duplicate existing protocol/log/AI masters.

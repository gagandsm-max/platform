---
name: java_spring
description: Java 21 / Spring Core service conventions—transactions, validation, Kafka boundaries, security and test slices.
---

<!-- generated from project_agents/skills/java_spring/SKILL.md sha256:6825d7b58c7a1e232b2ee6ad54bd41c3e24440bb8645a56007cd33333f465c7d -->

# java_spring

## Purpose

Implement Platform Core in Java 21 / Spring with clear transactional ownership, server-side authorization, validated requests and honest ApplicationContext/failure behaviour—no hidden environment workarounds.

## Use when

Core service/API changes, JPA/JDBC transactions, Spring Kafka listeners, security annotations, or test-slice failures. Required for Core Java paths.

## Authoritative inputs

Always read:

1. Existing Core module structure and Spring Boot version on the exact head
2. Flyway/migration and PG transaction patterns (`postgresql`, `migration_safety`)
3. Kafka consumer ACK vs business transaction patterns (`kafka`, `replay_idempotency`)
4. Security/tenant filter conventions (`tenant_isolation`)
5. Observability/error-code standards in-tree

## Invariants

- Java 21 language level; avoid obsolete APIs without justification.
- Spring lifecycle beans are explicit; `@PostConstruct` side effects stay bounded.
- Transactional boundaries declared (`@Transactional` or template)—DB writes not half-committed.
- Dependency injection over manual singleton registries.
- Request validation at API boundary; reject bad input fail-closed.
- Exception/error mapping to stable error classes; no stack traces to anonymous clients.
- DB transaction ownership clear vs Kafka poll thread; do not hold DB TX across network calls.
- Kafka transaction boundaries only where actually used and proven.
- Connection/resource pools sized and closed; no connection leaks.
- Concurrency on shared state justified and tested.
- Immutable DTO/contracts preferred at API edges.
- Test slices (`@WebMvcTest`, `@DataJpaTest`, etc.) match the layer under test.
- ApplicationContext failures fail startup visibly—not silent degraded half-boot for auth.
- No hidden env workaround to bypass product defects.
- Structured observability (correlation IDs) without secrets.
- Security annotations / filters enforce server-side authorization.

## Required procedure

1. State the Spring layer touched (web, service, persistence, messaging).
2. Define transaction and Kafka ACK boundaries; draw the crash window.
3. Validate inputs; map errors to stable codes.
4. Enforce tenant/authz before mutation.
5. Add/adjust tests (slice or integration) for success and failure paths.
6. Verify context fails closed on misconfig (e.g., missing DataSource) in tests where relevant.
7. Run Core unit/integration targets available in Docker/Make.

## Negative / failure cases

- `@Transactional` spanning outbound HTTP.
- Catching Exception and returning 200.
- Disabling security filter “for local only” in shared profiles.
- Mutating shared static maps across requests.
- Using Kafka offset commit as DB commit substitute.

## Security / tenant implications

Method security / filters are mandatory for tenant APIs. Secrets from env/keystore only. Actuator exposures least-privilege. Logging MDC includes correlation ID, not tokens.

## Required evidence

- Test names/results for changed slices
- Transaction/Kafka boundary notes
- Authz coverage for new endpoints
- Exact SHA

## Stop conditions

Stop when:

- Authz missing on mutating API
- TX boundary undefined on dual-write paths
- Context boots without required security beans
- Hidden workaround bypasses failing tests

## Anti-patterns

- Field injection everywhere without reason
- `System.out` debugging in production paths
- Writer self-certifying Core correctness

## Handoff / output

Return: TX/messaging design; test evidence; risks; nextOwner (Core/API verifier). Not MERGE_READY alone.

## Non-ownership

Does not own Go Sensor, invent BUILD-120 HA, or replace `postgresql`/`kafka` Skills. Owns Java/Spring discipline.

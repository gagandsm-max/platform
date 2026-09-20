# API Monitoring

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_02, 09, 10, 14, 17, 23

## Product objective

Provide deterministic synthetic monitoring for HTTP/HTTPS APIs with structured requests, assertions, timing and evidence, distinct from generic integration connectors.

## Monitor definition

An API monitor may define:

- URL/endpoint and method;
- headers and content type;
- authentication using governed Credential Profiles;
- request body/template;
- timeout/retry;
- expected status;
- header/body assertions;
- JSON/XML/schema/path assertions where supported;
- latency thresholds and breakdown;
- chained steps when explicitly configured;
- execution location/vantage and schedule.

Import from cURL/OpenAPI may assist setup, but Platform parses inputs into a safe structured monitor; it must not execute arbitrary shell commands.

## Timing

Expose DNS, connect, TLS, time-to-first-byte and total duration when the execution stack can measure them reliably. Failed stages remain distinct.

## Authentication

Supported patterns may include Basic, Bearer, API key, OAuth2, custom headers and mTLS. Secret values never appear in normal UI, logs, audit or synthetic result payloads.

## Assertions and evidence

Store bounded request metadata and response evidence required to explain success/failure. Sensitive headers/body fields are redacted. Large bodies are not retained by default.

## OpenAPI / API Studio

API Studio can import endpoint definitions, help map parameters/auth/assertions, preview a monitor and publish approved definitions. AI may suggest schema interpretation, but an administrator confirms production mappings.

## Alert semantics

One failed synthetic is evidence, not automatically a service outage. Qualification policy considers retries, multiple vantages, dependency/network/DNS/TLS evidence, maintenance and telemetry quality.

## UX

Provide API monitor inventory, execution history, latency/error charts, assertion failures, certificate/DNS/TLS context, per-step evidence, Run Now and multi-vantage comparison where configured.

## Limits

API Monitoring does not imply full distributed tracing or bytecode APM. It tests externally visible API behavior.

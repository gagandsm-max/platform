# Synthetic Testing — URL, API, Web Application & Service Checks

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 09, 10, 14, 21

## Product objective

Actively test externally visible service behavior from controlled vantages, including network reachability, DNS, TLS, TCP, HTTP/S, APIs and future browser journeys.

## Initial synthetic types

- ICMP reachability/loss/RTT;
- TCP connect;
- DNS query/validation;
- TLS handshake/certificate validation;
- HTTP/HTTPS URL availability and latency;
- API request/assertion monitoring;
- multi-step API chains where governed;
- basic service-port tests;
- scheduled and on-demand execution.

## HTTP/URL monitor

Configuration may include:

- URL;
- method;
- headers/auth;
- timeout;
- redirect policy;
- expected status;
- body/header assertions;
- certificate requirements;
- response-size/evidence limits;
- schedule and vantages.

Measure DNS/connect/TLS/TTFB/total where supported.

## API synthetic

See `05_API_MONITORING.md`. API checks are structured; cURL imports are parsed, not executed as arbitrary shell.

## Web application / browser synthetic

Browser journey testing is a future governed capability under Platform_21. It may include scripted navigation, assertions, timing and bounded evidence capture.

No browser runtime/vendor dependency is selected merely by the requirement. Browser synthetics must not silently expand into full session recording or employee surveillance.

## Multi-vantage testing

Run the same test from multiple authorized Sensors/Agents/outside-in probes to distinguish local path failures from service failures.

## Alert qualification

One synthetic failure does not automatically mean the application is down. Consider:

- retry;
- independent vantages;
- DNS/TLS/network evidence;
- maintenance;
- dependency health;
- test-runner health.

## Evidence

Store bounded results, timing, assertion failure, vantage and correlation ID. Sensitive headers/bodies/tokens are redacted.

## Outside-in probes

Future customer-operated probes require explicit identity, tenant scope, replay resistance, bounded buffering, time quality and operating model before production.

## UX

Provide inventory, history, SLA/availability charts, latency breakdown, failed assertion, Run Now, Compare Vantage and related Alert/Incident/RCA links.

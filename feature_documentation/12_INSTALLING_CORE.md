# Installing Core

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_02, 03, 15, 16

## Product objective

Install a production Core predictably in Standalone or exactly-two-node HA mode, including preflight, certificates, data services, health verification, offline support, upgrades and rollback.

## Supported model

Production Core targets supported Ubuntu/RHEL environments using native packages. Development remains Docker Compose and must not be confused with customer packaging.

Installer mode:

- **Standalone** — one Core.
- **HA** — exactly two full Core nodes, Active/Standby. No third customer-visible Core is introduced.

## Preflight

Validate:

- supported OS/version;
- CPU/RAM/disk;
- DNS and NTP/time;
- filesystem/mounts;
- required ports;
- host identity;
- certificates/PKI inputs;
- standalone or HA endpoint/VIP details;
- peer reachability for HA;
- required offline artifacts and checksums.

Failures produce explicit remediation.

## Installation

The installer establishes Platform services, PostgreSQL, ClickHouse, Kafka, Valkey and Core application components according to the approved deployment topology; creates/loads certificates; configures service auto-start; applies schema migrations; and starts health checks.

## HA setup

For HA, pair exactly two Cores using the supplied peer/HA addressing and stable customer endpoint/VIP. Validate synchronization, fencing/failover prerequisites and service health before reporting HA ready.

## Security

No default production secrets. Sensitive installer input is protected and not written to normal logs. UI/API is HTTPS only.

## Air gap

Offline installation includes required pinned artifacts, SBOMs, notices and checksums. Runtime install/upgrade must not fetch from the Internet in air-gapped mode.

## Verification

Installation is complete only after:

- API/UI health;
- internal service health;
- certificate validation;
- database connectivity/migrations;
- storage thresholds;
- backup readiness;
- HA state where applicable;
- support diagnostic evidence.

Generate an installation readiness/result report with failures and remediation.

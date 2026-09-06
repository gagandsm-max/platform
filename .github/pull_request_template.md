## 1. Scope & Description
<!-- Provide a clear summary of the changes introduced and their purpose in the 30-day programme. -->

## 2. Mother-Reference Requirements Affected
<!-- Cite specific requirements from Platform_01 (BRD), Platform_02 (PRD), or specialized masters. -->

## 3. Architecture Impact
<!-- Describe any impact on Core, Sensor, Agent, UI, or datastore subsystems. -->

## 4. Mother-Document Changes
<!-- Did this PR modify any file in project_documentation/? If yes, list files and state rationale per AGENTS.md change control. -->
- [ ] No mother-reference documents modified.
- [ ] Mother-reference documents updated (details below):

## 5. OSS & Dependency Changes
<!-- List any direct/build/runtime dependencies added or updated, confirming presence in Platform_07_OSS_Catalog.md. -->
- [ ] No new dependencies introduced.
- [ ] Dependencies added/modified (catalog compliance verified):

## 6. Schema & Data Contracts
<!-- Note any changes to PostgreSQL schema, ClickHouse tables, Kafka event topics, or Valkey keys. -->

## 7. Security & Trust Boundaries
<!-- Confirm mTLS, HTTPS-only, non-root user, secret handling, and threat model compliance. -->

## 8. Test Execution & Evidence
<!-- List automated tests executed and attach evidence. -->
- [ ] `make doctor`
- [ ] `make test-unit`
- [ ] `make test-integration`
- [ ] `make test-contract`
- [ ] `make test-security-smoke`
- [ ] `make post-reboot-verify`

## 9. Failure & Negative Behavior Verification
<!-- Detail the fault injection, timeout, or negative access tests executed (e.g. invalid certs, component outage). -->

## 10. Resource & Performance Footprint
<!-- Report CPU, memory, and container footprint impact. -->

## 11. UI / Visual Evidence
<!-- Embed screenshots or visual verification if changes touch the Platform Web UI. -->

## 12. Known Limitations & Follow-ups
<!-- Document any temporary trade-offs or technical debt requiring scheduled sprint work. -->

## 13. P0 / TBD Implications
<!-- Does this PR resolve or impact any of the 10 authoritative P0/TBD Engineering Spikes in Platform_12? -->

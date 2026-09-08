# Platform Provisioning Readiness Contract Foundation

**Scope:** static provisioning request, readiness/diagnostic evidence, Sensor-eligibility evidence, and connectivity-matrix contracts only.
**Mother Reference:** Platform v3.2.2.
**Runtime connectivity/provisioning:** not implemented or certified by this checkpoint.

## 1. Purpose

This directory defines the Platform-owned contract boundary that future Core, Sensor and UI implementations can share for provisioning readiness without embedding live probes, placement algorithms, credential storage or vendor behavior into the schema.

The foundation covers:

- versioned provisioning requests using safe declarative references;
- the authoritative ordered readiness/preflight stages;
- evidence-first diagnostic outcomes;
- the canonical provisioning failure taxonomy;
- Sensor eligibility evidence without scoring logic;
- generated connectivity/firewall requirements;
- bounded static validation and fictional fixtures.

A diagnostic classification is **not proof of the physical root cause unless the recorded evidence establishes it**.

`ACL_FIREWALL_SUSPECTED` != proven firewall block.
`PROTOCOL_TIMEOUT` != device down.
`PORT_UNREACHABLE` != firewall block.

## 2. Authority boundaries

The Mother Reference assigns provisioning orchestration to Core, actual preflight execution to the selected Sensor where applicable, placement decisions to Sensor Control/placement policy, credential values to Credential Profiles/secret services, and monitoring behavior to Monitoring Profiles/policies.

This contract therefore does **not**:

- authorize a tenant, user, Sensor or resource;
- reveal or store credential secret values;
- select a Sensor;
- define placement scores or weights;
- define polling intervals, thresholds, retries or timeouts;
- define vendor commands, OIDs, REST paths or gNMI paths;
- execute DNS, ICMP, sockets, TLS, SNMP, SSH, NETCONF, REST or gNMI;
- create a device/entity;
- implement discovery or live provisioning;
- depend on draft Device Pack PR #16.

Device Pack references are opaque identifiers only. Cross-contract Device Pack validation belongs to a later integration checkpoint after the contracts are independently validated and merged.

## 3. Directory layout

```text
schemas/provisioning/
  README.md
  provisioning-request.schema.json
  readiness-result.schema.json
  connectivity-matrix.schema.json
  tools/
    validate.mjs
  examples/
    provisioning-request.json
    readiness-success.json
    connectivity-matrix.json
  fixtures/
    cases.json
```

The JSON Schema files document the structural contract. `tools/validate.mjs` performs separate Platform semantic/security validation. The tool is **not** presented as a generic JSON Schema implementation.

## 4. Contract versioning

Each document contains:

- `documentType` — one of the three document contracts in this directory;
- `schemaVersion` — `1.0.0` for this foundation.

Breaking contract changes require a major schema-version change and a compatibility/migration plan under the Platform API/event contract rules.

The `1.0.0` contract number is a Platform-owned schema version, not a product release, operational threshold or runtime compatibility claim.

## 5. Identifier and defensive bounds

Current Core already defines correlation IDs as maximum 128 characters using `[A-Za-z0-9._:-]+`; this contract reuses that implemented grammar for `correlationId` and bounded opaque identifiers where a more specific authoritative format does not exist.

Current Core also defines defensive text bounds such as 255-character names, 2,000-character descriptions, 128-character tag keys and 512-character tag values. The validator reuses those where the concepts align.

Additional collection/document limits in the validator are **defensive parser/input-safety bounds only**. They are not customer capacity, latency, polling, timeout, threshold, licence or scale defaults.

## 6. Provisioning request contract

`provisioning-request.schema.json` contains only declarative request data:

- contract/document version;
- request and correlation identifiers;
- tenant context identifier;
- target type and address/name input;
- optional Display Name and Description;
- optional site and Sensor Pool references;
- optional Monitoring Profile reference;
- optional opaque Device Pack reference/version hint;
- optional Credential Profile reference;
- requested protocol declarations;
- caller-provided tags;
- optional idempotency reference.

A `CredentialProfileRef` identifies stored Platform state only. It never carries password, SNMP community, key, token or other secret material and does not itself prove authorization to use the profile.

Protocol declarations never supply static default ports. A `port`, when supplied, is a configuration-derived value. ICMP and traceroute declarations do not accept a port.

## 7. Authoritative readiness pipeline

The readiness result preserves the Mother-Reference order exactly:

1. `INPUT_NORMALIZATION`
2. `DNS_ADDRESS_VALIDATION`
3. `SENSOR_ELIGIBILITY_CAPACITY`
4. `ROUTE_REACHABILITY`
5. `TRANSPORT_PROTOCOL_RESPONSE`
6. `TLS_HOST_KEY_VERIFICATION`
7. `AUTHENTICATION`
8. `AUTHORIZATION_CAPABILITY`
9. `IDENTITY_DEVICE_PACK_MATCH`
10. `MONITORING_PROFILE_DRY_RUN`
11. `CAPACITY_IMPACT`
12. `PROVISION`

Every readiness result carries each stage exactly once and in this order. That makes omissions/reordering visible and prevents consumers from inferring a different workflow.

### 7.1 Stage status semantics

- `NOT_RUN` — no evidence-producing execution has occurred.
- `RUNNING` — reserved for a future asynchronous implementation currently executing the stage.
- `PASS` — this stage has explicit supporting evidence and no canonical failure.
- `FAIL` — this stage has explicit evidence, a canonical provisioning failure code and safe explanation.
- `INCONCLUSIVE` — evidence exists but is insufficient or contradictory for a definitive result.
- `UNKNOWN` — required evidence is unavailable/not observed; this is never converted to `PASS`.
- `SKIPPED_NOT_APPLICABLE` — the stage is not applicable to the configured path, for example TLS verification on a non-TLS protocol.

A stage after the first unresolved/blocking stage cannot report evaluated success/failure as if earlier prerequisites had succeeded. Later stages remain `NOT_RUN` or, where intrinsically not applicable, `SKIPPED_NOT_APPLICABLE`.

The validator permits at most one `RUNNING` stage.

### 7.2 PASS/FAIL semantics

A `PASS` must contain evidence and must not carry a failure code.

A `FAIL` must contain:

- a Mother-Reference canonical failure code;
- bounded evidence;
- a safe customer-facing explanation;
- an evidence assessment of `OBSERVED` or `SUSPECTED`.

`INCONCLUSIVE` and `UNKNOWN` deliberately carry no invented `UNCLASSIFIED` failure code. They use evidence/uncertainty fields until the condition can be correctly classified or the canonical taxonomy is extended through change control.

## 8. Canonical provisioning failure codes

This foundation preserves the Mother Reference names exactly:

- `INPUT_INVALID`
- `DEVICE_UNREACHABLE`
- `DNS_FAILURE`
- `ROUTE_UNAVAILABLE`
- `PROTOCOL_TIMEOUT`
- `PROTOCOL_DISABLED_SUSPECTED`
- `AUTHENTICATION_FAILED`
- `AUTHORIZATION_FAILED`
- `ACL_FIREWALL_SUSPECTED`
- `PORT_UNREACHABLE`
- `CREDENTIAL_PROFILE_INVALID`
- `DEVICE_ALREADY_EXISTS`
- `MIB_REQUIRED`
- `API_SCHEMA_INVALID`
- `TLS_CERTIFICATE_FAILURE`
- `SENSOR_CAPACITY_EXCEEDED`
- `NO_ELIGIBLE_SENSOR`
- `MONITORING_PROFILE_INCOMPATIBLE`
- `DEVICE_IDENTITY_CONFLICT`

Unknown conditions must not be forced into a semantically incorrect code merely to avoid `UNKNOWN`/`INCONCLUSIVE`.

`ACL_FIREWALL_SUSPECTED` and `PROTOCOL_DISABLED_SUSPECTED` require a `SUSPECTED` failure assessment. The validator rejects invented certainty such as `FIREWALL_BLOCKED` because it is not a canonical provisioning code.

## 9. Evidence model

Evidence is structured and bounded. An evidence entry can preserve:

- stable evidence ID;
- `OBSERVED`, `DERIVED`, `SUSPECTED` or `UNKNOWN` evidence kind;
- governed source class;
- safe summary;
- optional source/endpoint reference;
- optional protocol/transport context;
- optional source-local technical code;
- optional observation timestamp.

Protocol-local Sensor errors such as SNMP or synthetic error codes can be retained as evidence `technicalCode` values. They do not replace the canonical provisioning failure taxonomy.

No raw device response, unbounded log body, secret, command output or arbitrary nested payload exists in this foundation.

Authentication and authorization remain separate stages and separate canonical failures. A successful protocol response followed by authentication failure must not be collapsed into transport failure; authenticated-but-not-authorized must not be reported as authentication failure.

No-response evidence must not be silently reinterpreted as device-down evidence.

## 10. Retryability

This contract does **not** standardize a cross-protocol retryability field. Current protocol implementations may carry local transient/retry classifications, but Mother Reference does not define one canonical provisioning retryability semantic. Freezing one here would be an assumption.

Future integration may add a governed retry classification after Core/Sensor semantics are reconciled.

## 11. Sensor eligibility evidence

A readiness result can carry candidate-Sensor evidence without defining the placement algorithm.

Each candidate records:

- Sensor reference;
- optional site/pool context;
- selected/not-selected flag and bounded reason;
- independent eligibility checks.

Check categories are:

- `SITE_POOL_COMPATIBILITY`
- `CAPABILITY_PROTOCOL_SUPPORT`
- `REACHABILITY`
- `LATENCY_LOSS`
- `CAPACITY`
- `SECURITY_POLICY`
- `MAINTENANCE_DRAIN`
- `HEALTH_READINESS`

There are no weights, scores, RTT thresholds, CPU percentages or invented capacity limits in this contract. Evidence may carry observations supplied by a future implementation, but the contract does not decide eligibility from them.

At most one candidate may be marked selected. A selected Sensor cannot simultaneously contain a failed eligibility check.

## 12. Connectivity / firewall matrix

`connectivity-matrix.schema.json` defines the customer-exportable requirement/evidence matrix. Rows can contain:

- source role (`SENSOR`, `CORE`, `AGENT`) and opaque identity/context references;
- destination and scope;
- protocol and transport;
- configuration-derived port where applicable;
- direction;
- purpose;
- TLS requirement and optional security-mode label;
- governed source of the requirement;
- validation status;
- bounded evidence;
- optional observation timestamp.

Requirement sources are:

- `MONITORING_PROFILE`
- `DEVICE_PACK`
- `PLATFORM_SERVICE`
- `CONNECTOR`
- `OTHER_GOVERNED_SOURCE`

The matrix is designed to be **generated from effective configuration later**. It is not a static universal firewall list and this repository contains no customer firewall assumptions.

A matrix validation status is evidence state only; it is not firewall policy authorization and is not proof that a firewall caused a failure.

## 13. Relationship to Device Packs

Device Packs can later contribute compatibility/protocol requirements and identity mappings. This contract stores only opaque Device Pack references and governed requirement-source metadata.

It does not import, load or validate PR #16 content. A later integration checkpoint can add cross-contract validation only after both independent contracts are merged.

## 14. Relationship to Monitoring Profiles

Monitoring Profiles/policies remain authoritative for metric selection, polling, adaptive polling, inheritance/assignment, thresholds, validation and alerts. This contract carries an opaque Monitoring Profile reference and can record that a connectivity row came from an effective profile.

No polling interval, threshold, retry or timeout is embedded here.

## 15. Relationship to Credential Profiles

Credential Profiles own encrypted secrets and scope/authorization. The provisioning request can reference an existing Credential Profile, but:

- no secret value is allowed;
- `USE` does not imply `REVEAL`;
- possessing a reference does not grant authorization;
- validation does not retrieve credentials.

## 16. Security and redaction boundary

Static validation fails closed for:

- password/password-hash fields;
- SNMP community fields;
- private keys;
- bearer/API/access/refresh tokens;
- API keys/client secrets;
- cookies/session secrets;
- arbitrary secret-bearing fields;
- URL userinfo credentials and secret-bearing query parameters;
- executable/script/shell/command/code fields;
- path traversal/encoded traversal and absolute filesystem paths;
- unsafe HTML/script/event-handler content;
- control characters;
- unknown schema fields;
- oversized strings/arrays/documents.

The validator makes zero network calls and never evaluates executable content.

Ordinary diagnostic text is intentionally bounded and redaction-safe. Deeper technical data belongs behind governed diagnostic/evidence references rather than being echoed into customer-facing fields.

## 17. Provisioning workflow-state boundary

The Mother Reference LLD already defines the broader device-provisioning state machine (`STAGED`, `NORMALIZED`, `INPUT_FAILED`, `IDENTITY_CHECK`, `DUPLICATE_REVIEW`, `SENSOR_SELECT`, `NO_SENSOR`, `PREFLIGHT`, `PREFLIGHT_FAILED`, `READY`, `PROVISIONING`, `VERIFYING`, `ACTIVE`, `FAILED_PROVISIONING`).

This checkpoint does not create another workflow engine or competing lifecycle vocabulary. Its contracts describe the request, readiness evidence and connectivity matrix that a future implementation can attach to that authoritative workflow.

Cancellation semantics and any additional public workflow-state API remain outside this checkpoint rather than being guessed.

## 18. Static validator

Default fixture/example validation:

```bash
node schemas/provisioning/tools/validate.mjs
```

Validate one or more contract documents/directories:

```bash
node schemas/provisioning/tools/validate.mjs schemas/provisioning/examples
```

The tool uses Node.js standard-library modules only. Node.js 20 is already approved repository UI/build tooling; this lane adds no npm dependency and no lockfile.

## 19. Fixtures

Fixtures are fictional and non-production. They use reserved TEST-NET addresses or `.test` names and contain no real customer/domain/credential data.

Coverage includes successful and failed readiness chains, skipped non-applicable TLS, Sensor eligibility failures, ACL/firewall suspicion, connectivity rows, malformed/contradictory states, secret leakage attempts, traversal/executable content, strict unknown fields and defensive size bounds.

Fixture success means only that static contract rules accept/reject the document as expected. It is not evidence that a DNS lookup, connection, Sensor, firewall or device was contacted.

## 20. Future integration checkpoints

Later work must separately implement:

- Core request persistence/orchestration;
- credential authorization/resolution;
- Sensor selection and placement scoring;
- live preflight execution;
- discovery and provisioning jobs;
- Device Pack/runtime integration;
- Monitoring Profile effective-policy resolution;
- UI workflow and remediation actions;
- connectivity-matrix generation from actual effective configuration;
- protocol/device integration and certification.

None of those capabilities is claimed by this contract foundation.

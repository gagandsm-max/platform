# Configuration acquisition and normalization

## Acquisition
- Prefer Platform-controlled Core/Sensor paths; never browser-direct device sessions for NCM evidence.
- Record: tenant, entity_id, target identity, credential profile id (not secret), protocol, timestamp, collector health.
- Capture running and startup configuration as separate artifacts when the device exposes both.
- Bound size/time; oversized config must fail closed with actionable evidence—not silent truncate presented as complete.

## Normalization
- Preserve raw artifact hash/provenance alongside normalized text/structure.
- Normalize line endings and trivial noise only when documented; never invent semantic parses for uncertified families.
- Vendor/OEM extensions remain annotated; they do not become Platform authority identifiers.
- Secret-looking tokens (passwords, keys, community strings) must be redacted in operator-exported views.

## History / version evidence
- Each successful acquisition creates immutable version evidence with actor/system provenance.
- Operators compare versions by explicit version ids—not by “latest” alone.

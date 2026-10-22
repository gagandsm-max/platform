/**
 * Test-only fixtures for PR246 entity metric observation UI contract.
 * NON_PRODUCTION — never use as live telemetry or demo estate data.
 * F_RENDER_BINDING_CORRECTION_20261022 continues to use these fixtures
 * for render-time ownership regression cases (execution NOT_RUN in that dispatch).
 */

export const FIXTURE_PROVENANCE = Object.freeze({
  kind: 'NON_PRODUCTION_TEST_FIXTURE',
  fabricatesLiveTelemetry: false,
  path: 'ui/tests/fixtures/metric-observations.fixture.mjs',
  backendContractPin: '006dfc53244b5e27f64db88d275fede518964d86',
});

export const FIXTURE_TENANT_A = '11111111-1111-1111-1111-111111111111';
export const FIXTURE_TENANT_B = '22222222-2222-2222-2222-222222222222';
export const FIXTURE_ENTITY = '33333333-3333-3333-3333-333333333333';

/** Exact Counter64 decimal string above Number.MAX_SAFE_INTEGER (2^53-1). */
export const COUNTER64_ABOVE_SAFE_INTEGER = '18446744073709551615';

export const FIXTURE_WINDOW = Object.freeze({
  from: '2026-10-21T10:00:00.000Z',
  to: '2026-10-21T11:00:00.000Z',
});

export function counter64SuccessBody(overrides = {}) {
  return {
    tenantId: FIXTURE_TENANT_A,
    entityId: FIXTURE_ENTITY,
    metricId: '.1.3.6.1.2.1.31.1.1.1.6.1',
    valueKind: 'COUNTER64',
    from: FIXTURE_WINDOW.from,
    to: FIXTURE_WINDOW.to,
    limit: 100,
    returned: 1,
    truncated: false,
    observationState: 'OK',
    freshness: 'HISTORICAL',
    qualityNote: 'Historical observation window returned; freshness is HISTORICAL, not current-liveness FRESH.',
    provenance: {
      sourceView: 'platform.metric_counter',
      ordering: 'event_time ASC, ingest_seq ASC then present event_time DESC, ingest_seq DESC',
      provenanceClass: 'clickhouse-canonical-final-view',
    },
    observations: [
      {
        timestamp: '2026-10-21T10:30:00.000Z',
        value: null,
        counterValue: COUNTER64_ABOVE_SAFE_INTEGER,
        quality: 1,
        flags: 0,
        sensorId: 'sensor-fixture-1',
        sequenceId: 'seq-1',
        sourceProtocol: 'SNMP',
      },
    ],
    ...overrides,
  };
}

export function numericSuccessBody(overrides = {}) {
  return {
    tenantId: FIXTURE_TENANT_A,
    entityId: FIXTURE_ENTITY,
    metricId: 'system.cpu.utilization',
    valueKind: 'NUMERIC',
    from: FIXTURE_WINDOW.from,
    to: FIXTURE_WINDOW.to,
    limit: 100,
    returned: 1,
    truncated: true,
    observationState: 'OK',
    freshness: 'HISTORICAL',
    qualityNote: 'Historical observation window returned. Result truncated to latest 100 observations.',
    provenance: {
      sourceView: 'platform.metric_numeric',
      ordering: 'event_time ASC, ingest_seq ASC then present event_time DESC, ingest_seq DESC',
      provenanceClass: 'clickhouse-canonical-final-view',
    },
    observations: [
      {
        timestamp: '2026-10-21T10:15:00.000Z',
        value: 42.5,
        counterValue: null,
        quality: 1,
        flags: 0,
        sensorId: 'sensor-fixture-1',
        sequenceId: 'seq-n1',
        sourceProtocol: 'AGENT',
      },
    ],
    ...overrides,
  };
}

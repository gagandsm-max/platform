/**
 * NON-PRODUCTION TEST FIXTURE ONLY.
 *
 * Provenance: ui/tests/fixtures/health-overview.fixture.mjs
 * Purpose: Exercise typed health-overview view-model presentation states.
 * Must never be imported by production UI modules under ui/src/.
 */

export const FIXTURE_PROVENANCE = {
  kind: 'NON_PRODUCTION_TEST_FIXTURE',
  path: 'ui/tests/fixtures/health-overview.fixture.mjs',
  fabricatesLiveTelemetry: false,
};

/** Structural row placeholders — evidence statuses only, no UP/DOWN fabrication. */
export const NETWORK_PARTIAL_ROWS = Object.freeze([
  Object.freeze({
    id: 'fixture-net-1',
    primaryLabel: 'fixture-edge-router',
    secondaryLabel: 'TEST FIXTURE — not a customer device',
    siteLabel: 'fixture-site-a',
    status: 'PARTIAL',
    statusExplanation: 'Fixture partial SNMP evidence; not live telemetry.',
    freshness: 'stale',
    collector: 'degraded',
    lastUpdateLabel: 'fixture-stale',
    cells: Object.freeze({
      identifier: 'fixture-only',
      vendorModel: 'fixture / not-certified',
      availability: 'UNKNOWN',
      primaryKpi: '—',
      sensor: 'fixture-sensor',
      alerts: '—',
    }),
  }),
]);

export const INFRA_UNKNOWN_ROWS = Object.freeze([
  Object.freeze({
    id: 'fixture-host-1',
    primaryLabel: 'fixture-linux-host',
    secondaryLabel: 'TEST FIXTURE — not a customer host',
    siteLabel: 'fixture-site-b',
    status: 'UNKNOWN',
    statusExplanation: 'Fixture unknown Agent evidence; not live telemetry.',
    freshness: 'missing',
    collector: 'unavailable',
    lastUpdateLabel: 'fixture-missing',
    cells: Object.freeze({
      os: 'Linux (fixture)',
      cpu: '—',
      memory: '—',
      disk: '—',
      network: '—',
      method: 'Agent (fixture)',
      alerts: '—',
    }),
  }),
]);

export const ALL_PRESENTATION_STATES = Object.freeze([
  'loading',
  'empty',
  'partial',
  'stale',
  'permission-restricted',
  'not-licensed',
  'query-limited',
  'error',
  'monitoring-degraded',
  'normal',
  'source-unavailable',
  'unknown',
]);

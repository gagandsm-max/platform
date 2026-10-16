import test from 'node:test';
import assert from 'node:assert/strict';
import {
  ALL_PRESENTATION_STATES,
  FIXTURE_PROVENANCE,
  INFRA_UNKNOWN_ROWS,
  NETWORK_PARTIAL_ROWS,
} from './fixtures/health-overview.fixture.mjs';
import {
  HEALTH_OVERVIEW_PRESENTATION_STATES,
  HEALTH_OVERVIEW_STATE_LABELS,
  INFRASTRUCTURE_HEALTH_COLUMNS,
  NETWORK_HEALTH_COLUMNS,
  buildHealthOverviewStateCopy,
  buildInfrastructureHealthOverviewVm,
  buildNetworkHealthOverviewVm,
  isDeviceDownFabrication,
  resolveHealthOverviewHostDataState,
  resolveHealthOverviewProviderBinding,
  resolveUnboundHealthPresentation,
  screenDataStateToHealthPresentation,
} from '../.test-dist/platform/health-overview/index.js';
import { findWorkspaceByPath } from '../.test-dist/platform/navigation.js';
import { isAssertivePageState } from '../.test-dist/platform/screen-state.js';
import { foundationDataStateForCapability } from '../.test-dist/platform/workspace-host-state.js';

test('health-overview fixture is explicitly non-production', () => {
  assert.equal(FIXTURE_PROVENANCE.kind, 'NON_PRODUCTION_TEST_FIXTURE');
  assert.equal(FIXTURE_PROVENANCE.fabricatesLiveTelemetry, false);
  assert.match(FIXTURE_PROVENANCE.path, /tests\/fixtures\//);
});

test('UI-009 and UI-029 registry routes remain PRESENTATION PENDING_BACKEND', () => {
  const network = findWorkspaceByPath('/network-health-overview');
  const infra = findWorkspaceByPath('/infrastructure-health-overview');
  assert.ok(network);
  assert.ok(infra);
  assert.equal(network.id, 'ui-009');
  assert.equal(network.uiCode, 'UI-009');
  assert.equal(network.backendCapability, 'PENDING_BACKEND');
  assert.equal(infra.id, 'ui-029');
  assert.equal(infra.uiCode, 'UI-029');
  assert.equal(infra.backendCapability, 'PENDING_BACKEND');
});

test('unbound network health overview defaults to source-unavailable without fabricated rows', () => {
  const model = buildNetworkHealthOverviewVm();
  assert.equal(model.uiCode, 'UI-009');
  assert.equal(model.workspaceId, 'ui-009');
  assert.equal(model.presentationState, 'source-unavailable');
  assert.equal(model.dataBound, false);
  assert.equal(model.rows.length, 0);
  assert.equal(model.panels.length, 4);
  assert.ok(model.panels.every((panel) => panel.valueLabel !== '0'));
  assert.ok(
    model.panels.every((panel) =>
      ['Not connected', 'Unknown', 'Source unavailable'].includes(panel.valueLabel)),
  );
  assert.deepEqual(
    model.columns.map((column) => column.id),
    NETWORK_HEALTH_COLUMNS.map((column) => column.id),
  );
  assert.ok(
    model.safeguardingNotes.some((note) => note.includes('DEVICE_DOWN')),
  );
});

test('unbound infrastructure health overview defaults to source-unavailable without fabricated rows', () => {
  const model = buildInfrastructureHealthOverviewVm();
  assert.equal(model.uiCode, 'UI-029');
  assert.equal(model.workspaceId, 'ui-029');
  assert.equal(model.presentationState, 'source-unavailable');
  assert.equal(model.dataBound, false);
  assert.equal(model.rows.length, 0);
  assert.deepEqual(
    model.columns.map((column) => column.id),
    INFRASTRUCTURE_HEALTH_COLUMNS.map((column) => column.id),
  );
  assert.ok(
    model.safeguardingNotes.some((note) =>
      note.includes('HOST_DOWN') || note.includes('DEVICE_DOWN')),
  );
});

test('PENDING_BACKEND and INTEGRATED unbound paths never imply Normal health', () => {
  assert.equal(resolveUnboundHealthPresentation('PENDING_BACKEND'), 'source-unavailable');
  assert.equal(resolveUnboundHealthPresentation('INTEGRATED'), 'source-unavailable');
  assert.notEqual(buildNetworkHealthOverviewVm({ capabilityStatus: 'INTEGRATED' }).presentationState, 'normal');
  assert.notEqual(
    buildInfrastructureHealthOverviewVm({ capabilityStatus: 'INTEGRATED' }).presentationState,
    'normal',
  );
});

test('every health-overview presentation state has operator copy and labels', () => {
  assert.deepEqual([...HEALTH_OVERVIEW_PRESENTATION_STATES], [...ALL_PRESENTATION_STATES]);
  for (const kind of HEALTH_OVERVIEW_PRESENTATION_STATES) {
    assert.ok(HEALTH_OVERVIEW_STATE_LABELS[kind]);
    const networkCopy = buildHealthOverviewStateCopy('Network', kind, {
      correlationId: 'corr-fixture',
      errorId: 'err-fixture',
      affectedSource: 'fixture-sensor',
    });
    assert.equal(networkCopy.kind, kind);
    assert.ok(networkCopy.title.length > 0);
    assert.ok(networkCopy.explanation.length > 0);
    assert.ok(networkCopy.nextAction.length > 0);
    // Explicit: copy must not assert device/host down from the state alone.
    assert.equal(/asserts?\s+(device|host)\s+down/i.test(networkCopy.explanation), false);
    assert.equal(/device down|host down|DEVICE_DOWN|HOST_DOWN/i.test(networkCopy.title), false);
    assert.ok(networkCopy.evidence.some((item) => /must not be presented as device or host down/i.test(item)));
  }
});

test('test fixtures may supply structural rows without UP/DOWN fabrication', () => {
  const network = buildNetworkHealthOverviewVm({
    dataBound: true,
    presentationState: 'partial',
    affectedSource: 'fixture-sensor',
    rows: NETWORK_PARTIAL_ROWS,
  });
  assert.equal(network.presentationState, 'partial');
  assert.equal(network.rows.length, 1);
  assert.equal(network.rows[0].status, 'PARTIAL');
  assert.equal(isDeviceDownFabrication(network.rows[0].status), false);
  assert.equal(isDeviceDownFabrication(network.rows[0].cells.availability), false);

  const infra = buildInfrastructureHealthOverviewVm({
    dataBound: true,
    presentationState: 'unknown',
    rows: INFRA_UNKNOWN_ROWS,
  });
  assert.equal(infra.presentationState, 'unknown');
  assert.equal(infra.rows[0].status, 'UNKNOWN');
  assert.equal(isDeviceDownFabrication('DEVICE_DOWN'), true);
  assert.equal(isDeviceDownFabrication('DOWN'), true);
  assert.equal(isDeviceDownFabrication('UNKNOWN'), false);
});

test('screen data states map into health presentation without inventing Normal as healthy', () => {
  assert.equal(screenDataStateToHealthPresentation('loading'), 'loading');
  assert.equal(screenDataStateToHealthPresentation('empty'), 'empty');
  assert.equal(screenDataStateToHealthPresentation('partial'), 'partial');
  assert.equal(screenDataStateToHealthPresentation('stale'), 'stale');
  assert.equal(screenDataStateToHealthPresentation('permission-restricted'), 'permission-restricted');
  assert.equal(screenDataStateToHealthPresentation('not-licensed'), 'not-licensed');
  assert.equal(screenDataStateToHealthPresentation('query-limited'), 'query-limited');
  assert.equal(screenDataStateToHealthPresentation('error'), 'error');
  assert.equal(screenDataStateToHealthPresentation('monitoring-degraded'), 'monitoring-degraded');
  // Bound provider may represent query-normal / data-available; never DEVICE_UP / HOST_UP.
  assert.equal(screenDataStateToHealthPresentation('normal'), 'normal');
});

test('WorkspaceHost-supplied dataState without dataBound is preserved (provider binding)', () => {
  // Regression: wrappers defaulted dataBound=false and ignored dataState via (dataBound && dataState).
  for (const state of [
    'not-licensed',
    'permission-restricted',
    'loading',
    'error',
    'stale',
    'monitoring-degraded',
  ]) {
    const binding = resolveHealthOverviewProviderBinding({ dataState: state });
    assert.equal(binding.dataBound, true);
    assert.equal(binding.presentationState, state);
    const network = buildNetworkHealthOverviewVm({
      dataBound: binding.dataBound,
      presentationState: binding.presentationState,
    });
    const infra = buildInfrastructureHealthOverviewVm({
      dataBound: binding.dataBound,
      presentationState: binding.presentationState,
    });
    assert.equal(network.presentationState, state);
    assert.equal(infra.presentationState, state);
    assert.equal(network.dataBound, true);
    assert.equal(infra.dataBound, true);
  }
});

test('bound normal is data-available / query-normal, not target UP or healthy metrics', () => {
  const binding = resolveHealthOverviewProviderBinding({ dataState: 'normal' });
  assert.equal(binding.dataBound, true);
  assert.equal(binding.presentationState, 'normal');

  const network = buildNetworkHealthOverviewVm({
    dataBound: binding.dataBound,
    presentationState: binding.presentationState,
  });
  assert.equal(network.presentationState, 'normal');
  assert.equal(HEALTH_OVERVIEW_STATE_LABELS.normal, 'Data available');
  assert.match(network.stateCopy.title, /data available|query normal/i);
  assert.match(network.stateCopy.explanation, /query normal|data available/i);
  assert.match(network.stateCopy.explanation, /not a DEVICE_UP, HOST_UP/i);
  assert.match(network.stateCopy.explanation, /healthy-metrics claim/i);
  assert.equal(/\bis (DEVICE_UP|HOST_UP)\b/i.test(network.stateCopy.explanation), false);
  assert.equal(/\bdevices?\s+are\s+healthy\b/i.test(network.stateCopy.explanation), false);
  assert.equal(network.rows.length, 0);
  assert.ok(
    network.panels.every((panel) =>
      ['Not connected', 'Unknown', 'Source unavailable'].includes(panel.valueLabel)),
  );
});

test('Not Licensed remains distinct from Permission Restricted', () => {
  const licensed = buildHealthOverviewStateCopy('Network', 'not-licensed');
  const restricted = buildHealthOverviewStateCopy('Network', 'permission-restricted');
  assert.equal(licensed.kind, 'not-licensed');
  assert.equal(restricted.kind, 'permission-restricted');
  assert.equal(licensed.title, 'Not licensed');
  assert.equal(restricted.title, 'Permission restricted');
  assert.notEqual(licensed.title, restricted.title);
  assert.equal(/request workspace permission/i.test(licensed.nextAction), false);
  assert.match(restricted.nextAction, /workspace permission/i);
});

test('unbound path without provider retains Source Unavailable', () => {
  const unbound = resolveHealthOverviewProviderBinding({});
  assert.equal(unbound.dataBound, false);
  assert.equal(unbound.presentationState, undefined);
  assert.equal(buildNetworkHealthOverviewVm().presentationState, 'source-unavailable');
  assert.equal(buildInfrastructureHealthOverviewVm().presentationState, 'source-unavailable');
});

test('UI-009/UI-029 host chrome resolves unbound to source-unavailable, not foundation empty', () => {
  assert.equal(foundationDataStateForCapability('PENDING_BACKEND'), 'empty');
  assert.equal(resolveHealthOverviewHostDataState(undefined), 'source-unavailable');
  assert.equal(resolveHealthOverviewHostDataState('not-licensed'), 'not-licensed');
  assert.equal(resolveHealthOverviewHostDataState('normal'), 'normal');
  assert.notEqual(
    resolveHealthOverviewHostDataState(undefined),
    foundationDataStateForCapability('PENDING_BACKEND'),
  );
});

test('source-unavailable is polite status, not assertive alert', () => {
  assert.equal(isAssertivePageState('source-unavailable'), false);
  assert.equal(isAssertivePageState('unknown'), false);
  assert.equal(isAssertivePageState('error'), true);
  assert.equal(isAssertivePageState('monitoring-degraded'), true);
});

test('bound provider evidence must not claim the API is unbound', () => {
  const bound = buildHealthOverviewStateCopy('Network', 'normal', { dataBound: true });
  assert.equal(bound.evidence.some((item) => /until a tenant-scoped Core read API is bound/i.test(item)), false);
  assert.ok(bound.evidence.some((item) => /bound provider data-state/i.test(item)));
  assert.ok(bound.evidence.some((item) => /not live customer telemetry/i.test(item)));

  const unbound = buildHealthOverviewStateCopy('Network', 'source-unavailable', { dataBound: false });
  assert.ok(unbound.evidence.some((item) => /until a tenant-scoped Core read API is bound/i.test(item)));
});

test('recovery actions stay non-interactive without inventing handlers', () => {
  for (const kind of ['error', 'partial', 'stale', 'monitoring-degraded', 'normal', 'empty']) {
    const copy = buildHealthOverviewStateCopy('Network', kind, { dataBound: true });
    assert.equal(copy.retryAvailable, false);
    assert.equal(copy.diagnosticAvailable, false);
    assert.match(copy.nextAction, /non-interactive until Core recovery handlers are bound/i);
    assert.equal(/\bRetry the query\b/i.test(copy.nextAction), false);
  }
});

test('dataBound without explicit state remains unknown, never Normal', () => {
  const network = buildNetworkHealthOverviewVm({ dataBound: true });
  const infra = buildInfrastructureHealthOverviewVm({ dataBound: true });
  assert.equal(network.presentationState, 'unknown');
  assert.equal(infra.presentationState, 'unknown');
  assert.notEqual(network.presentationState, 'normal');
  assert.notEqual(infra.presentationState, 'normal');
});

test('operator copy discloses FOUNDATION_ACCESS is not authorization', () => {
  const copy = buildHealthOverviewStateCopy('Network', 'source-unavailable');
  assert.ok(copy.evidence.some((item) => /FOUNDATION_ACCESS is not authorization/i.test(item)));
  assert.equal(isDeviceDownFabrication('HOST_DOWN'), true);
});

test('active tab metadata flows into the view-model', () => {
  const model = buildNetworkHealthOverviewVm({ activeTab: 'Interfaces' });
  assert.equal(model.activeTab, 'Interfaces');
  assert.equal(model.capabilityStatus, 'PENDING_BACKEND');
  const infra = buildInfrastructureHealthOverviewVm({ activeTab: 'Servers' });
  assert.equal(infra.activeTab, 'Servers');
  assert.ok(
    infra.panels.every((panel) =>
      ['Not connected', 'Unknown', 'Source unavailable'].includes(panel.valueLabel)),
  );
});

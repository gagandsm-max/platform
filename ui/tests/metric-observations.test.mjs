/**
 * SOURCE_ONLY / NOT_RUN under F_C16_METRIC_OBSERVATION_UI_20261021.
 * Model/unit contract tests — not browser or component coverage.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  COUNTER64_ABOVE_SAFE_INTEGER,
  FIXTURE_ENTITY,
  FIXTURE_PROVENANCE,
  FIXTURE_TENANT_A,
  FIXTURE_TENANT_B,
  FIXTURE_WINDOW,
  counter64SuccessBody,
  numericSuccessBody,
} from './fixtures/metric-observations.fixture.mjs';
import {
  METRIC_OBSERVATION_DEFAULT_LIMIT,
  METRIC_OBSERVATION_MAX_LIMIT,
  METRIC_OBSERVATION_MAX_WINDOW_MS,
  assertResponseMatchesRequest,
  bindingToQuery,
  buildMetricObservationRequestPath,
  escapeForText,
  isAllowlistedMetricSelector,
  isCompleteMetricObservationBinding,
  isExactCounter64Decimal,
  observationStateToPageKind,
  parseExactFlags,
  parseMetricObservationResponse,
  parseObservationItem,
} from '../.test-dist/platform/metric-observations.js';
import { fetchMetricObservations } from '../.test-dist/platform/metric-observation-client.js';
import {
  MetricObservationController,
  shouldAcceptGeneration,
} from '../.test-dist/platform/metric-observation-state.js';

const BASE_BINDING = {
  tenantId: FIXTURE_TENANT_A,
  entityId: FIXTURE_ENTITY,
  metricId: '.1.3.6.1.2.1.31.1.1.1.6.1',
  valueKind: 'COUNTER64',
  from: FIXTURE_WINDOW.from,
  to: FIXTURE_WINDOW.to,
  limit: 100,
};

test('fixture provenance is explicitly non-production', () => {
  assert.equal(FIXTURE_PROVENANCE.kind, 'NON_PRODUCTION_TEST_FIXTURE');
  assert.equal(FIXTURE_PROVENANCE.fabricatesLiveTelemetry, false);
  assert.equal(FIXTURE_PROVENANCE.backendContractPin, '006dfc53244b5e27f64db88d275fede518964d86');
});

test('documents PR246 route, query params and server bounds', () => {
  assert.equal(METRIC_OBSERVATION_DEFAULT_LIMIT, 100);
  assert.equal(METRIC_OBSERVATION_MAX_LIMIT, 500);
  assert.equal(METRIC_OBSERVATION_MAX_WINDOW_MS, 24 * 60 * 60 * 1000);
  const query = bindingToQuery({ ...BASE_BINDING, transport: async () => new Response('{}') });
  const path = buildMetricObservationRequestPath(query);
  assert.match(
    path,
    /^\/api\/v1\/tenants\/11111111-1111-1111-1111-111111111111\/entities\/33333333-3333-3333-3333-333333333333\/metric-observations\?/,
  );
  assert.match(path, /metricId=%2E1%2E3%2E6%2E1%2E2%2E1%2E31%2E1%2E1%2E1%2E6%2E1/);
  assert.match(path, /valueKind=COUNTER64/);
  assert.match(path, /from=2026-10-21T10%3A00%3A00.000Z/);
  assert.match(path, /to=2026-10-21T11%3A00%3A00.000Z/);
  assert.match(path, /limit=100/);
  assert.equal(isAllowlistedMetricSelector('system.cpu.utilization', 'NUMERIC'), true);
  assert.equal(isAllowlistedMetricSelector('.1.3.6.1.2.1.31.1.1.1.6.1', 'COUNTER64'), true);
  assert.equal(isAllowlistedMetricSelector('if.hc_in_octets', 'COUNTER64'), false);
});

test('Counter64 decimal text above 2^53 is preserved without Number conversion', () => {
  assert.equal(isExactCounter64Decimal(COUNTER64_ABOVE_SAFE_INTEGER), true);
  assert.ok(Number(COUNTER64_ABOVE_SAFE_INTEGER) > Number.MAX_SAFE_INTEGER);
  const item = parseObservationItem(
    {
      timestamp: '2026-10-21T10:30:00.000Z',
      value: null,
      counterValue: COUNTER64_ABOVE_SAFE_INTEGER,
      quality: 1,
      flags: 0,
      sensorId: 's1',
      sequenceId: '1',
      sourceProtocol: 'SNMP',
    },
    'COUNTER64',
  );
  assert.equal(item.counterValue, COUNTER64_ABOVE_SAFE_INTEGER);
  assert.equal(item.value, null);
  // Prove we did not round via IEEE float: string identity remains exact uint64 max.
  assert.notEqual(String(Number(item.counterValue)), item.counterValue);
});

test('unsafe JSON flags are rejected rather than silently rounded', () => {
  assert.equal(parseExactFlags(7), 7);
  assert.throws(
    () => parseExactFlags(Number.MAX_SAFE_INTEGER + 10),
    /cannot be represented exactly/,
  );
});

test('response/context mismatch is rejected', () => {
  const body = counter64SuccessBody({ tenantId: FIXTURE_TENANT_B });
  const request = bindingToQuery({ ...BASE_BINDING, transport: async () => new Response('{}') });
  assert.throws(
    () => parseMetricObservationResponse(body, request),
    /does not match the initiating request context/,
  );
  assert.throws(
    () => assertResponseMatchesRequest(
      {
        tenantId: FIXTURE_TENANT_B,
        entityId: FIXTURE_ENTITY,
        metricId: BASE_BINDING.metricId,
        valueKind: 'COUNTER64',
        from: FIXTURE_WINDOW.from,
        to: FIXTURE_WINDOW.to,
        limit: 100,
      },
      request,
    ),
    /does not match/,
  );
});

test('incomplete binding is not complete and controller performs zero requests', () => {
  assert.equal(isCompleteMetricObservationBinding(null), false);
  assert.equal(isCompleteMetricObservationBinding({
    ...BASE_BINDING,
    transport: undefined,
  }), false);
  const controller = new MetricObservationController();
  const views = [];
  controller.subscribe((v) => views.push(v));
  controller.setBinding(null);
  assert.equal(controller.getTransportRequestCount(), 0);
  assert.equal(controller.getView().pageKind, 'source-unavailable');
  assert.equal(controller.getView().observationState, 'SOURCE_UNAVAILABLE');
  assert.equal(controller.getView().observations.length, 0);
  controller.dispose();
});

test('fetchMetricObservations forwards AbortSignal and builds same-origin path', async () => {
  let seenPath = '';
  let seenSignal = null;
  const body = counter64SuccessBody();
  const binding = {
    ...BASE_BINDING,
    transport: async (path, init) => {
      seenPath = path;
      seenSignal = init.signal;
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    },
  };
  const ac = new AbortController();
  const result = await fetchMetricObservations(binding, ac.signal);
  assert.equal(seenSignal, ac.signal);
  assert.match(seenPath, /^\/api\/v1\/tenants\//);
  assert.equal(result.response.observations[0].counterValue, COUNTER64_ABOVE_SAFE_INTEGER);
});

test('cancellation and out-of-order generations are rejected', async () => {
  assert.equal(shouldAcceptGeneration(3, 3), true);
  assert.equal(shouldAcceptGeneration(3, 2), false);

  let release;
  const gate = new Promise((resolve) => {
    release = resolve;
  });
  const controller = new MetricObservationController();
  const views = [];
  controller.subscribe((v) => views.push({ ...v, observations: [...v.observations] }));

  const slowBinding = {
    ...BASE_BINDING,
    transport: async (_path, init) => {
      await gate;
      if (init.signal?.aborted) {
        const err = new DOMException('Aborted', 'AbortError');
        throw err;
      }
      return new Response(JSON.stringify(counter64SuccessBody()), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    },
  };

  controller.setBinding(slowBinding);
  const genAfterFirst = controller.getView().generation;
  // Switch tenant/context before slow response — clears rows and bumps generation.
  controller.setBinding({
    ...BASE_BINDING,
    tenantId: FIXTURE_TENANT_B,
    transport: async () => new Response(JSON.stringify(counter64SuccessBody({
      tenantId: FIXTURE_TENANT_B,
    })), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  });
  assert.ok(controller.getView().generation > genAfterFirst);
  // Immediate clear: no prior-tenant rows retained after context change.
  assert.equal(controller.getView().observations.length, 0);
  release();
  await new Promise((r) => setTimeout(r, 20));
  // First response must not overwrite the newer generation.
  assert.notEqual(controller.getView().request?.tenantId, FIXTURE_TENANT_A);
  controller.dispose();
});

test('tenant switch clears observations before the next ready state', async () => {
  const controller = new MetricObservationController();
  const snapshots = [];
  controller.subscribe((v) => {
    snapshots.push({
      generation: v.generation,
      phase: v.phase,
      tenant: v.request?.tenantId ?? null,
      rows: v.observations.length,
    });
  });

  controller.setBinding({
    ...BASE_BINDING,
    transport: async () => new Response(JSON.stringify(counter64SuccessBody()), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  });
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(controller.getView().phase, 'ready');
  assert.equal(controller.getView().observations.length, 1);

  controller.setBinding({
    ...BASE_BINDING,
    tenantId: FIXTURE_TENANT_B,
    transport: async () => {
      await new Promise((r) => setTimeout(r, 30));
      return new Response(JSON.stringify(counter64SuccessBody({ tenantId: FIXTURE_TENANT_B })), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    },
  });
  // Immediate post-switch snapshot must show cleared rows.
  const afterSwitch = snapshots[snapshots.length - 1];
  assert.equal(afterSwitch.rows, 0);
  assert.equal(afterSwitch.phase === 'loading' || afterSwitch.phase === 'unbound', true);
  await new Promise((r) => setTimeout(r, 50));
  assert.equal(controller.getView().request?.tenantId, FIXTURE_TENANT_B);
  controller.dispose();
});

test('semantic states, freshness and truncation remain distinct', () => {
  assert.equal(observationStateToPageKind('OK'), 'normal');
  assert.equal(observationStateToPageKind('UNKNOWN'), 'unknown');
  assert.equal(observationStateToPageKind('STALE'), 'stale');
  assert.equal(observationStateToPageKind('MONITORING_DEGRADED'), 'monitoring-degraded');
  assert.equal(observationStateToPageKind('NO_DATA'), 'empty');
  assert.equal(observationStateToPageKind('SOURCE_UNAVAILABLE'), 'source-unavailable');
  const request = bindingToQuery({ ...BASE_BINDING, metricId: 'system.cpu.utilization', valueKind: 'NUMERIC', transport: async () => new Response('{}') });
  const parsed = parseMetricObservationResponse(numericSuccessBody(), {
    ...request,
    metricId: 'system.cpu.utilization',
    valueKind: 'NUMERIC',
  });
  assert.equal(parsed.truncated, true);
  assert.equal(parsed.freshness, 'HISTORICAL');
  assert.equal(parsed.observationState, 'OK');
  assert.match(parsed.qualityNote, /truncated/i);
});

test('permission and server errors do not become empty successful data', async () => {
  for (const [status, code] of [
    [401, 'UNAUTHORIZED'],
    [403, 'FORBIDDEN'],
    [404, 'NOT_FOUND'],
    [500, 'SERVER_ERROR'],
  ]) {
    const controller = new MetricObservationController();
    controller.setBinding({
      ...BASE_BINDING,
      transport: async () => new Response('denied', { status }),
    });
    await new Promise((r) => setTimeout(r, 20));
    assert.equal(controller.getView().phase, 'error');
    assert.equal(controller.getView().errorCode, code);
    assert.equal(controller.getView().observations.length, 0);
    assert.notEqual(controller.getView().pageKind, 'normal');
    assert.notEqual(controller.getView().pageKind, 'empty');
    controller.dispose();
  }
});

test('escapeForText supports XSS-safe text rendering in the harness', () => {
  assert.equal(
    escapeForText(`<script>alert("x")</script>&'`),
    '&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt;&amp;&#39;',
  );
});

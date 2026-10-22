/**
 * SOURCE_ONLY / NOT_RUN under F_C16_METRIC_OBSERVATION_UI_20261021,
 * F_RENDER_BINDING_CORRECTION_20261022 and F_RENDER_GUARD_EDGE_CASES_20261022.
 * Model/unit + controller contract tests — not browser or React component coverage.
 * Render-binding / invalid-limit cases exercise production
 * selectRenderableMetricObservationView and MetricObservationController.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  COUNTER64_ABOVE_SAFE_INTEGER,
  FIXTURE_ENTITY,
  FIXTURE_ENTITY_ALT,
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
  isUuid,
  isValidMetricObservationLimit,
  metricObservationInvalidLimitMessage,
  observationStateToPageKind,
  ownershipFromBinding,
  ownershipMatchesBinding,
  parseExactFlags,
  parseMetricObservationResponse,
  parseObservationItem,
  resolveMetricObservationLimit,
  tryEffectiveMetricObservationLimit,
} from '../.test-dist/platform/metric-observations.js';
import { fetchMetricObservations } from '../.test-dist/platform/metric-observation-client.js';
import {
  MetricObservationController,
  createInvalidLimitView,
  createPendingContextView,
  createUnboundMetricObservationView,
  selectRenderableMetricObservationView,
  shouldAcceptGeneration,
} from '../.test-dist/platform/metric-observation-state.js';

/** Assert a selected view exposes none of the stale observation-derived surfaces. */
function assertNoStaleObservationSurface(selected) {
  assert.equal(selected.request, null);
  assert.equal(selected.observations.length, 0);
  assert.equal(selected.provenance, null);
  assert.equal(selected.qualityNote, null);
  assert.equal(selected.correlationId, null);
  assert.equal(selected.retryAvailable, false);
  assert.notEqual(selected.phase, 'ready');
}

function assertQueryLimitSurface(selected) {
  assertNoStaleObservationSurface(selected);
  assert.equal(selected.phase, 'error');
  assert.equal(selected.pageKind, 'query-limited');
  assert.equal(selected.errorCode, 'QUERY_LIMIT');
  assert.equal(selected.message, metricObservationInvalidLimitMessage());
  assert.equal(selected.ownership, null);
}

function createDeferred() {
  let resolve;
  let reject;
  const promise = new Promise((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

async function waitUntil(predicate, label = 'condition', timeoutMs = 2000) {
  const start = Date.now();
  while (!predicate()) {
    if (Date.now() - start > timeoutMs) {
      throw new Error(`Timed out waiting for ${label}`);
    }
    await new Promise((r) => setImmediate(r));
  }
}

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

const BASE_BINDING = {
  tenantId: FIXTURE_TENANT_A,
  entityId: FIXTURE_ENTITY,
  metricId: '.1.3.6.1.2.1.31.1.1.1.6.1',
  valueKind: 'COUNTER64',
  from: FIXTURE_WINDOW.from,
  to: FIXTURE_WINDOW.to,
  limit: 100,
};

function completeBinding(overrides = {}) {
  return {
    ...BASE_BINDING,
    transport: async () => jsonResponse({}),
    ...overrides,
  };
}

test('fixture provenance is explicitly non-production', () => {
  assert.equal(FIXTURE_PROVENANCE.kind, 'NON_PRODUCTION_TEST_FIXTURE');
  assert.equal(FIXTURE_PROVENANCE.fabricatesLiveTelemetry, false);
  assert.equal(FIXTURE_PROVENANCE.backendContractPin, '006dfc53244b5e27f64db88d275fede518964d86');
});

test('fixture tenant/entity IDs satisfy production isUuid', () => {
  assert.equal(isUuid(FIXTURE_TENANT_A), true);
  assert.equal(isUuid(FIXTURE_TENANT_B), true);
  assert.equal(isUuid(FIXTURE_ENTITY), true);
  assert.equal(isUuid(FIXTURE_ENTITY_ALT), true);
});

test('documents PR246 route, query params and server bounds', () => {
  assert.equal(METRIC_OBSERVATION_DEFAULT_LIMIT, 100);
  assert.equal(METRIC_OBSERVATION_MAX_LIMIT, 500);
  assert.equal(METRIC_OBSERVATION_MAX_WINDOW_MS, 24 * 60 * 60 * 1000);
  const binding = completeBinding();
  assert.equal(isCompleteMetricObservationBinding(binding), true);
  const query = bindingToQuery(binding);
  const path = buildMetricObservationRequestPath(query);
  const url = new URL(path, 'https://platform.local');
  assert.equal(
    url.pathname,
    `/api/v1/tenants/${FIXTURE_TENANT_A}/entities/${FIXTURE_ENTITY}/metric-observations`,
  );
  // URLSearchParams may leave "." unencoded — assert decoded metricId, not %2E.
  assert.equal(url.searchParams.get('metricId'), BASE_BINDING.metricId);
  assert.equal(url.searchParams.get('valueKind'), 'COUNTER64');
  assert.equal(url.searchParams.get('from'), FIXTURE_WINDOW.from);
  assert.equal(url.searchParams.get('to'), FIXTURE_WINDOW.to);
  assert.equal(url.searchParams.get('limit'), '100');
  assert.match(path, /from=2026-10-21T10%3A00%3A00\.000Z/);
  assert.match(path, /to=2026-10-21T11%3A00%3A00\.000Z/);
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
  const binding = completeBinding();
  assert.equal(isCompleteMetricObservationBinding(binding), true);
  const request = bindingToQuery(binding);
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
  controller.subscribe(() => {});
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
  const binding = completeBinding({
    transport: async (path, init) => {
      seenPath = path;
      seenSignal = init.signal;
      return jsonResponse(body);
    },
  });
  assert.equal(isCompleteMetricObservationBinding(binding), true);
  const ac = new AbortController();
  const result = await fetchMetricObservations(binding, ac.signal);
  assert.equal(seenSignal, ac.signal);
  assert.match(seenPath, /^\/api\/v1\/tenants\//);
  assert.equal(result.response.observations[0].counterValue, COUNTER64_ABOVE_SAFE_INTEGER);
});

test('cancellation and out-of-order generations are rejected', async () => {
  assert.equal(shouldAcceptGeneration(3, 3), true);
  assert.equal(shouldAcceptGeneration(3, 2), false);

  const gate = createDeferred();
  const controller = new MetricObservationController();
  controller.subscribe(() => {});

  const slowBinding = completeBinding({
    transport: async (_path, init) => {
      await gate.promise;
      if (init.signal?.aborted) {
        throw new DOMException('Aborted', 'AbortError');
      }
      return jsonResponse(counter64SuccessBody());
    },
  });
  assert.equal(isCompleteMetricObservationBinding(slowBinding), true);

  controller.setBinding(slowBinding);
  const genAfterFirst = controller.getView().generation;
  const nextBinding = completeBinding({
    tenantId: FIXTURE_TENANT_B,
    transport: async () => jsonResponse(counter64SuccessBody({ tenantId: FIXTURE_TENANT_B })),
  });
  assert.equal(isCompleteMetricObservationBinding(nextBinding), true);
  controller.setBinding(nextBinding);
  assert.ok(controller.getView().generation > genAfterFirst);
  assert.equal(controller.getView().observations.length, 0);
  gate.resolve();
  await waitUntil(
    () => controller.getView().phase === 'ready'
      && controller.getView().request?.tenantId === FIXTURE_TENANT_B,
    'tenant-B ready after cancel',
  );
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

  const first = completeBinding({
    transport: async () => jsonResponse(counter64SuccessBody()),
  });
  assert.equal(isCompleteMetricObservationBinding(first), true);
  controller.setBinding(first);
  await waitUntil(() => controller.getView().phase === 'ready', 'first ready');
  assert.equal(controller.getView().observations.length, 1);

  const secondGate = createDeferred();
  const second = completeBinding({
    tenantId: FIXTURE_TENANT_B,
    transport: async () => {
      await secondGate.promise;
      return jsonResponse(counter64SuccessBody({ tenantId: FIXTURE_TENANT_B }));
    },
  });
  assert.equal(isCompleteMetricObservationBinding(second), true);
  controller.setBinding(second);
  const afterSwitch = snapshots[snapshots.length - 1];
  assert.equal(afterSwitch.rows, 0);
  assert.equal(afterSwitch.phase === 'loading' || afterSwitch.phase === 'unbound', true);
  secondGate.resolve();
  await waitUntil(
    () => controller.getView().request?.tenantId === FIXTURE_TENANT_B
      && controller.getView().phase === 'ready',
    'tenant-B ready',
  );
  controller.dispose();
});

test('semantic states, freshness and truncation remain distinct', () => {
  assert.equal(observationStateToPageKind('OK'), 'normal');
  assert.equal(observationStateToPageKind('UNKNOWN'), 'unknown');
  assert.equal(observationStateToPageKind('STALE'), 'stale');
  assert.equal(observationStateToPageKind('MONITORING_DEGRADED'), 'monitoring-degraded');
  assert.equal(observationStateToPageKind('NO_DATA'), 'empty');
  assert.equal(observationStateToPageKind('SOURCE_UNAVAILABLE'), 'source-unavailable');
  const binding = completeBinding({
    metricId: 'system.cpu.utilization',
    valueKind: 'NUMERIC',
  });
  assert.equal(isCompleteMetricObservationBinding(binding), true);
  const request = bindingToQuery(binding);
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
    const binding = completeBinding({
      transport: async () => new Response('denied', { status }),
    });
    assert.equal(isCompleteMetricObservationBinding(binding), true);
    controller.setBinding(binding);
    await waitUntil(() => controller.getView().phase === 'error', `error ${code}`);
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

// --- Render-time ownership + invalid-limit edge cases (pure/controller; not React) ---

function makeTransport(label) {
  const fn = async () => jsonResponse({});
  fn.__label = label;
  return fn;
}

function readyOwnedView(binding, overrides = {}) {
  assert.equal(isCompleteMetricObservationBinding(binding), true);
  const ownership = ownershipFromBinding(binding);
  assert.ok(ownership, 'readyOwnedView requires a valid ownership (valid limit)');
  const request = bindingToQuery(binding);
  return {
    phase: 'ready',
    pageKind: 'normal',
    title: 'Entity metric observations · OK',
    message: 'ok',
    observationState: 'OK',
    freshness: 'HISTORICAL',
    truncated: false,
    qualityNote: 'Historical observation window returned',
    provenance: {
      sourceView: 'platform.metric_counter',
      ordering: 'event_time ASC',
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
    request,
    errorCode: null,
    correlationId: null,
    retryAvailable: false,
    retryCount: 0,
    generation: 1,
    ownership,
    ...overrides,
  };
}

test('render gate: different tenant with SAME transport rejects prior surfaces', () => {
  const shared = makeTransport('shared');
  const bindingA = completeBinding({ transport: shared });
  const bindingB = completeBinding({ tenantId: FIXTURE_TENANT_B, transport: shared });
  assert.equal(isCompleteMetricObservationBinding(bindingA), true);
  assert.equal(isCompleteMetricObservationBinding(bindingB), true);
  const storedA = readyOwnedView(bindingA);
  assert.equal(ownershipMatchesBinding(storedA.ownership, bindingB), false);
  const selected = selectRenderableMetricObservationView(storedA, bindingB);
  assertNoStaleObservationSurface(selected);
  assert.equal(selected.phase, 'loading');
  assert.notEqual(selected.request?.tenantId, FIXTURE_TENANT_A);
});

test('render gate: ready view + binding removed/incomplete → SOURCE_UNAVAILABLE', () => {
  const binding = completeBinding({ transport: makeTransport('T') });
  const stale = {
    ...readyOwnedView(binding),
    retryAvailable: true,
    correlationId: 'corr-stale',
  };
  for (const incomplete of [null, undefined, { ...binding, transport: undefined }, { ...binding, tenantId: '' }]) {
    const selected = selectRenderableMetricObservationView(stale, incomplete);
    assert.equal(selected.phase, 'unbound');
    assert.equal(selected.pageKind, 'source-unavailable');
    assert.equal(selected.observationState, 'SOURCE_UNAVAILABLE');
    assert.equal(selected.errorCode, 'SOURCE_UNAVAILABLE');
    assertNoStaleObservationSurface(selected);
    assert.equal(isCompleteMetricObservationBinding(incomplete), false);
  }
  const controller = new MetricObservationController();
  const before = controller.getTransportRequestCount();
  controller.setBinding(null);
  assert.equal(controller.getTransportRequestCount(), before);
  controller.dispose();
});

test('render gate: entity/metric/kind/window/effective-limit changes reject old view', () => {
  const transport = makeTransport('same');
  const base = completeBinding({ transport });
  const stored = readyOwnedView(base);
  const variants = [
    completeBinding({ transport, entityId: FIXTURE_ENTITY_ALT }),
    completeBinding({ transport, metricId: '.1.3.6.1.2.1.31.1.1.1.10.1' }),
    completeBinding({ transport, metricId: 'system.cpu.utilization', valueKind: 'NUMERIC' }),
    completeBinding({ transport, from: '2026-10-21T09:00:00.000Z' }),
    completeBinding({ transport, to: '2026-10-21T12:00:00.000Z' }),
    completeBinding({ transport, limit: 50 }),
  ];
  for (const next of variants) {
    assert.equal(isCompleteMetricObservationBinding(next), true);
    assert.equal(ownershipMatchesBinding(stored.ownership, next), false);
    const selected = selectRenderableMetricObservationView(stored, next);
    assertNoStaleObservationSurface(selected);
    assert.equal(selected.phase, 'loading');
  }
});

test('render gate: same query but DIFFERENT transport rejects earlier result/action', () => {
  const transportOld = makeTransport('old');
  const transportNew = makeTransport('new');
  const bindingOld = completeBinding({ transport: transportOld });
  const bindingNew = completeBinding({ transport: transportNew });
  assert.equal(isCompleteMetricObservationBinding(bindingOld), true);
  assert.equal(isCompleteMetricObservationBinding(bindingNew), true);
  const stored = readyOwnedView(bindingOld, {
    retryAvailable: true,
    phase: 'error',
    pageKind: 'error',
    errorCode: 'NETWORK',
    correlationId: 'corr-old',
    observations: [],
    request: null,
    provenance: null,
    qualityNote: null,
  });
  assert.equal(ownershipMatchesBinding(stored.ownership, bindingNew), false);
  const selected = selectRenderableMetricObservationView(stored, bindingNew);
  assertNoStaleObservationSurface(selected);
  assert.equal(selected.retryAvailable, false);
  assert.equal(selected.errorCode, null);
  assert.equal(selected.correlationId, null);
  assert.notEqual(transportOld, transportNew);
});

test('render gate: matching binding preserves exact Counter64', () => {
  const binding = completeBinding({ transport: makeTransport('match') });
  assert.equal(isCompleteMetricObservationBinding(binding), true);
  const stored = readyOwnedView(binding);
  assert.equal(ownershipMatchesBinding(stored.ownership, binding), true);
  const selected = selectRenderableMetricObservationView(stored, binding);
  assert.equal(selected, stored);
  assert.equal(selected.phase, 'ready');
  assert.equal(selected.request.tenantId, FIXTURE_TENANT_A);
  assert.equal(selected.observations[0].counterValue, COUNTER64_ABOVE_SAFE_INTEGER);
  assert.notEqual(String(Number(selected.observations[0].counterValue)), selected.observations[0].counterValue);
});

test('render gate: previous errors/correlation/retry do not leak into a new context', () => {
  const transportA = makeTransport('err-a');
  const transportB = makeTransport('err-b');
  const bindingA = completeBinding({ transport: transportA });
  const bindingB = completeBinding({ tenantId: FIXTURE_TENANT_B, transport: transportB });
  const storedError = readyOwnedView(bindingA, {
    phase: 'error',
    pageKind: 'error',
    errorCode: 'NETWORK',
    correlationId: 'corr-leak',
    retryAvailable: true,
    observations: [],
    request: null,
    provenance: null,
    qualityNote: 'stale-note',
    message: 'prior failure',
  });
  const selected = selectRenderableMetricObservationView(storedError, bindingB);
  assertNoStaleObservationSurface(selected);
  assert.equal(selected.errorCode, null);
  assert.notEqual(selected.message, 'prior failure');
});

test('late response that ignores abort cannot overwrite or become renderable', async () => {
  const oldGate = createDeferred();
  let oldTransportCalls = 0;
  let newTransportCalls = 0;
  const transportOld = async () => {
    oldTransportCalls += 1;
    await oldGate.promise;
    // Intentionally ignore abort — old request "succeeds" after the new context.
    return jsonResponse(counter64SuccessBody({
      tenantId: FIXTURE_TENANT_A,
      qualityNote: 'STALE_OLD_TRANSPORT_RESULT',
    }));
  };
  const transportNew = async () => {
    newTransportCalls += 1;
    return jsonResponse(counter64SuccessBody({ tenantId: FIXTURE_TENANT_B }));
  };
  const bindingOld = completeBinding({ transport: transportOld });
  const bindingNew = completeBinding({ tenantId: FIXTURE_TENANT_B, transport: transportNew });
  assert.equal(isCompleteMetricObservationBinding(bindingOld), true);
  assert.equal(isCompleteMetricObservationBinding(bindingNew), true);

  const controller = new MetricObservationController();
  controller.subscribe(() => {});
  controller.setBinding(bindingOld);
  controller.setBinding(bindingNew);
  await waitUntil(
    () => controller.getView().phase === 'ready'
      && controller.getView().request?.tenantId === FIXTURE_TENANT_B,
    'new binding ready',
  );
  assert.equal(newTransportCalls, 1);

  oldGate.resolve();
  // Flush the ignored-abort resolution; generation/ownership must keep B.
  await waitUntil(() => oldTransportCalls === 1, 'old transport resumed');
  await new Promise((r) => setImmediate(r));
  await new Promise((r) => setImmediate(r));

  assert.equal(controller.getView().request?.tenantId, FIXTURE_TENANT_B);
  assert.notEqual(controller.getView().qualityNote, 'STALE_OLD_TRANSPORT_RESULT');
  const forgedOld = readyOwnedView(bindingOld);
  assertNoStaleObservationSurface(selectRenderableMetricObservationView(forgedOld, bindingNew));
  assert.equal(
    selectRenderableMetricObservationView(controller.getView(), bindingNew).request?.tenantId,
    FIXTURE_TENANT_B,
  );
  controller.dispose();
});

// --- F1 invalid-limit / QUERY_LIMIT boundary ---

test('invalid limits are rejected without throw from ownership/selection helpers', () => {
  for (const bad of [0, -1, 501, 1.5, Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY]) {
    assert.equal(isValidMetricObservationLimit(bad), false);
    assert.equal(tryEffectiveMetricObservationLimit(bad), null);
    assert.throws(() => resolveMetricObservationLimit(bad), /limit must be between/);
    const binding = completeBinding({ limit: bad, transport: makeTransport('bad') });
    assert.equal(isCompleteMetricObservationBinding(binding), true);
    assert.equal(ownershipFromBinding(binding), null);
    assert.equal(ownershipMatchesBinding({
      tenantId: FIXTURE_TENANT_A,
      entityId: FIXTURE_ENTITY,
      metricId: BASE_BINDING.metricId,
      valueKind: 'COUNTER64',
      from: FIXTURE_WINDOW.from,
      to: FIXTURE_WINDOW.to,
      limit: 100,
      transport: binding.transport,
    }, binding), false);
    assert.doesNotThrow(() => selectRenderableMetricObservationView(
      createUnboundMetricObservationView(),
      binding,
    ));
    assertQueryLimitSurface(selectRenderableMetricObservationView(
      createUnboundMetricObservationView(),
      binding,
    ));
  }
});

test('omitted limit equals default; boundaries 1 and 500 are valid', () => {
  assert.equal(tryEffectiveMetricObservationLimit(undefined), METRIC_OBSERVATION_DEFAULT_LIMIT);
  assert.equal(tryEffectiveMetricObservationLimit(null), METRIC_OBSERVATION_DEFAULT_LIMIT);
  assert.equal(resolveMetricObservationLimit(undefined), METRIC_OBSERVATION_DEFAULT_LIMIT);
  assert.equal(isValidMetricObservationLimit(1), true);
  assert.equal(isValidMetricObservationLimit(500), true);
  assert.equal(tryEffectiveMetricObservationLimit(1), 1);
  assert.equal(tryEffectiveMetricObservationLimit(500), 500);
  const omitted = completeBinding({ limit: undefined, transport: makeTransport('omit') });
  delete omitted.limit;
  assert.equal(isCompleteMetricObservationBinding(omitted), true);
  const ownership = ownershipFromBinding(omitted);
  assert.equal(ownership.limit, METRIC_OBSERVATION_DEFAULT_LIMIT);
  assert.equal(ownershipMatchesBinding(ownership, omitted), true);
});

test('controller: invalid initial binding → QUERY_LIMIT, zero transport, no unhandled rejection', async () => {
  const controller = new MetricObservationController();
  let unhandled = null;
  const onReject = (reason) => {
    unhandled = reason;
  };
  process.once('unhandledRejection', onReject);
  try {
    const binding = completeBinding({
      limit: 0,
      transport: async () => {
        throw new Error('transport must not run');
      },
    });
    assert.equal(isCompleteMetricObservationBinding(binding), true);
    controller.setBinding(binding);
    await waitUntil(
      () => controller.getView().errorCode === 'QUERY_LIMIT',
      'invalid-limit QUERY_LIMIT',
    );
    assert.equal(controller.getTransportRequestCount(), 0);
    assertQueryLimitSurface(selectRenderableMetricObservationView(controller.getView(), binding));
    assert.equal(controller.getView().pageKind, 'query-limited');
    assert.equal(controller.getView().retryAvailable, false);
    await new Promise((r) => setImmediate(r));
    assert.equal(unhandled, null);
  } finally {
    process.off('unhandledRejection', onReject);
    controller.dispose();
  }
});

test('controller: valid-ready then invalid-limit clears stale content and blocks transport', async () => {
  const controller = new MetricObservationController();
  let transportCalls = 0;
  const valid = completeBinding({
    transport: async () => {
      transportCalls += 1;
      return jsonResponse(counter64SuccessBody());
    },
  });
  assert.equal(isCompleteMetricObservationBinding(valid), true);
  controller.setBinding(valid);
  await waitUntil(() => controller.getView().phase === 'ready', 'valid ready');
  assert.equal(transportCalls, 1);
  const readyStored = { ...controller.getView() };

  const invalid = completeBinding({
    limit: 501,
    transport: async () => {
      transportCalls += 1;
      throw new Error('invalid limit must not transport');
    },
  });
  assert.equal(isCompleteMetricObservationBinding(invalid), true);
  // Render gate before effects: ready view + invalid binding → QUERY_LIMIT, no stale rows.
  assertQueryLimitSurface(selectRenderableMetricObservationView(readyStored, invalid));

  controller.setBinding(invalid);
  await waitUntil(
    () => controller.getView().errorCode === 'QUERY_LIMIT',
    'post-ready invalid QUERY_LIMIT',
  );
  assert.equal(transportCalls, 1);
  assertQueryLimitSurface(selectRenderableMetricObservationView(controller.getView(), invalid));
  assert.equal(controller.getView().observations.length, 0);
  assert.equal(controller.getView().request, null);
  assert.equal(controller.getView().retryAvailable, false);
  controller.dispose();
});

test('controller: valid binding recovers after invalid limit input', async () => {
  const controller = new MetricObservationController();
  let transportCalls = 0;
  const invalid = completeBinding({
    limit: -1,
    transport: async () => {
      transportCalls += 1;
      throw new Error('no transport on invalid');
    },
  });
  controller.setBinding(invalid);
  await waitUntil(() => controller.getView().errorCode === 'QUERY_LIMIT', 'invalid first');
  assert.equal(transportCalls, 0);

  const valid = completeBinding({
    transport: async () => {
      transportCalls += 1;
      return jsonResponse(counter64SuccessBody());
    },
  });
  assert.equal(isCompleteMetricObservationBinding(valid), true);
  controller.setBinding(valid);
  await waitUntil(() => controller.getView().phase === 'ready', 'recover ready');
  assert.equal(transportCalls, 1);
  assert.equal(controller.getView().observations[0].counterValue, COUNTER64_ABOVE_SAFE_INTEGER);
  assert.equal(
    selectRenderableMetricObservationView(controller.getView(), valid).phase,
    'ready',
  );
  controller.dispose();
});

test('createInvalidLimitView matches selector QUERY_LIMIT contract', () => {
  const view = createInvalidLimitView(7);
  assertQueryLimitSurface(view);
  assert.equal(view.generation, 7);
  assert.equal(createPendingContextView(0).phase, 'loading');
});

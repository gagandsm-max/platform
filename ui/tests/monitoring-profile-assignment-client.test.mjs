import test from 'node:test';
import assert from 'node:assert/strict';
import {
  buildMonitoringProfileAssignmentPath,
  getMonitoringProfileAssignment,
  readSafeAssignmentEtag,
} from '../.test-dist/platform/monitoring/assignment-client.js';

const TENANT = '018f0e0a-0000-7000-8000-0000000000a1';
const ENTITY = '018f0e0a-0000-7000-8000-0000000000c3';
const PROFILE = '018f0e0a-0000-7000-8000-0000000000e5';
const PATH = `/api/v1/tenants/${TENANT}/entities/${ENTITY}/monitoring-profile-assignment`;

const VALID_BODY = {
  entityId: ENTITY,
  profileId: PROFILE,
  profileVersionNo: 7,
  assignmentMode: 'EXPLICIT_PROFILE',
  rowVersion: 15,
  effectiveFrom: '2026-11-18T00:00:00Z',
};

const headerMap = (entries = {}) => ({
  get(name) {
    const key = String(name).toLowerCase();
    for (const [k, v] of Object.entries(entries)) {
      if (String(k).toLowerCase() === key) {
        return v;
      }
    }
    return null;
  },
});

const fakeTransport = (handler) => ({
  async request(init) {
    return handler(init);
  },
});

const jsonResponse = (status, body, headers = {}) => ({
  status,
  headers: headerMap({
    'Content-Type': 'application/json',
    ...headers,
  }),
  async text() {
    return typeof body === 'string' ? body : JSON.stringify(body);
  },
});

test('buildMonitoringProfileAssignmentPath matches Core GET contract exactly', () => {
  assert.equal(buildMonitoringProfileAssignmentPath(TENANT, ENTITY), PATH);
  assert.equal(
    buildMonitoringProfileAssignmentPath(TENANT.toUpperCase(), ENTITY.toUpperCase()),
    PATH,
  );
});

test('path builder rejects UUID path and header injection attempts without calling transport', async () => {
  const calls = [];
  const transport = fakeTransport(async (init) => {
    calls.push(init);
    return jsonResponse(200, VALID_BODY);
  });

  const injected = [
    `${TENANT}/../${ENTITY}`,
    `${TENANT}%2f..%2f`,
    `${TENANT}\r\nX-Injected: 1`,
    `${TENANT}/extra`,
    'not-a-uuid',
    '',
  ];
  for (const bad of injected) {
    const tenantResult = await getMonitoringProfileAssignment(transport, {
      tenantId: bad,
      entityId: ENTITY,
    });
    assert.equal(tenantResult.kind, 'invalid_request');
    assert.equal(tenantResult.reason, 'invalid_tenant_id');

    const entityResult = await getMonitoringProfileAssignment(transport, {
      tenantId: TENANT,
      entityId: bad,
    });
    assert.equal(entityResult.kind, 'invalid_request');
    assert.equal(entityResult.reason, 'invalid_entity_id');
  }
  assert.equal(calls.length, 0);
  assert.equal(buildMonitoringProfileAssignmentPath(`${TENANT}/../x`, ENTITY), null);
});

test('200 authorized projection returns typed assignment and opaque ETag when safe', async () => {
  const transport = fakeTransport(async (init) => {
    assert.equal(init.method, 'GET');
    assert.equal(init.path, PATH);
    return jsonResponse(200, VALID_BODY, {
      ETag: '"15"',
      'X-Correlation-Id': 'corr-fixture',
    });
  });

  const result = await getMonitoringProfileAssignment(transport, {
    tenantId: TENANT,
    entityId: ENTITY,
  });
  assert.equal(result.kind, 'authorized');
  assert.deepEqual(result.assignment, VALID_BODY);
  assert.equal(result.etag, '"15"');
  assert.equal(result.correlationId, 'corr-fixture');
  assert.equal(Object.hasOwn(result, 'ifMatch'), false);
});

test('204 is no_assignment and is distinct from masked 404 denial', async () => {
  const transport204 = fakeTransport(async () => ({
    status: 204,
    headers: headerMap({ 'X-Correlation-Id': 'corr-204' }),
    async text() {
      return '';
    },
  }));
  const empty = await getMonitoringProfileAssignment(transport204, {
    tenantId: TENANT,
    entityId: ENTITY,
  });
  assert.equal(empty.kind, 'no_assignment');
  assert.equal(empty.correlationId, 'corr-204');

  const transport404 = fakeTransport(async () => jsonResponse(404, {
    code: 'RESOURCE_NOT_FOUND',
    message: 'Resource not found',
  }, { 'X-Correlation-Id': 'corr-404' }));
  const denied = await getMonitoringProfileAssignment(transport404, {
    tenantId: TENANT,
    entityId: ENTITY,
  });
  assert.equal(denied.kind, 'masked_denial');
  assert.notEqual(denied.kind, 'no_assignment');
  assert.equal(denied.correlationId, 'corr-404');
});

test('401 and 403 map to denied; 429 maps to throttled', async () => {
  for (const status of [401, 403]) {
    const result = await getMonitoringProfileAssignment(
      fakeTransport(async () => jsonResponse(status, { code: 'DENIED' })),
      { tenantId: TENANT, entityId: ENTITY },
    );
    assert.equal(result.kind, 'denied');
    assert.equal(result.status, status);
  }

  const throttled = await getMonitoringProfileAssignment(
    fakeTransport(async () => jsonResponse(429, { code: 'RATE_LIMITED' })),
    { tenantId: TENANT, entityId: ENTITY },
  );
  assert.equal(throttled.kind, 'throttled');
});

test('502/503/504 map to unavailable; other statuses are unexpected_content', async () => {
  for (const status of [502, 503, 504]) {
    const result = await getMonitoringProfileAssignment(
      fakeTransport(async () => jsonResponse(status, { code: 'DOWN' })),
      { tenantId: TENANT, entityId: ENTITY },
    );
    assert.equal(result.kind, 'unavailable');
    assert.equal(result.status, status);
  }

  const unexpected = await getMonitoringProfileAssignment(
    fakeTransport(async () => jsonResponse(418, { code: 'TEAPOT' })),
    { tenantId: TENANT, entityId: ENTITY },
  );
  assert.equal(unexpected.kind, 'unexpected_content');
  assert.equal(unexpected.status, 418);
});

test('network, abort and generic transport failures are typed without payload logging', async () => {
  const logs = [];
  const originalLog = console.log;
  const originalError = console.error;
  console.log = (...args) => {
    logs.push(['log', ...args]);
  };
  console.error = (...args) => {
    logs.push(['error', ...args]);
  };
  try {
    const network = await getMonitoringProfileAssignment(
      fakeTransport(async () => {
        throw Object.assign(new TypeError('fetch failed'), { name: 'TypeError' });
      }),
      { tenantId: TENANT, entityId: ENTITY },
    );
    assert.equal(network.kind, 'network_error');

    const aborted = await getMonitoringProfileAssignment(
      fakeTransport(async () => {
        const err = new Error('aborted');
        err.name = 'AbortError';
        throw err;
      }),
      { tenantId: TENANT, entityId: ENTITY },
    );
    assert.equal(aborted.kind, 'aborted');

    const generic = await getMonitoringProfileAssignment(
      fakeTransport(async () => {
        throw new Error('boom');
      }),
      { tenantId: TENANT, entityId: ENTITY },
    );
    assert.equal(generic.kind, 'error');
  } finally {
    console.log = originalLog;
    console.error = originalError;
  }
  assert.equal(logs.length, 0);
});

test('unexpected content-type and malformed/secret-bearing incomplete payloads are rejected', async () => {
  const html = await getMonitoringProfileAssignment(
    fakeTransport(async () => ({
      status: 200,
      headers: headerMap({ 'Content-Type': 'text/html' }),
      async text() {
        return '<html>secret-token=abc</html>';
      },
    })),
    { tenantId: TENANT, entityId: ENTITY },
  );
  assert.equal(html.kind, 'unexpected_content');

  const incomplete = await getMonitoringProfileAssignment(
    fakeTransport(async () => jsonResponse(200, {
      entityId: ENTITY,
      definition: { secret: 'nope' },
      assignedBy: 'actor',
      capacityEstimate: 99,
    })),
    { tenantId: TENANT, entityId: ENTITY },
  );
  assert.equal(incomplete.kind, 'unexpected_payload');

  const mismatchedEntity = await getMonitoringProfileAssignment(
    fakeTransport(async () => jsonResponse(200, {
      ...VALID_BODY,
      entityId: '018f0e0a-0000-7000-8000-0000000000d4',
    })),
    { tenantId: TENANT, entityId: ENTITY },
  );
  assert.equal(mismatchedEntity.kind, 'unexpected_payload');
});

test('unsafe numeric rowVersion is rejected; ETag is never invented as a precise Number/If-Match', async () => {
  const unsafeNumber = Number.MAX_SAFE_INTEGER + 2;
  assert.equal(Number.isSafeInteger(unsafeNumber), false);

  const rejected = await getMonitoringProfileAssignment(
    fakeTransport(async () => jsonResponse(200, {
      ...VALID_BODY,
      rowVersion: unsafeNumber,
    }, { ETag: '"9223372036854775807"' })),
    { tenantId: TENANT, entityId: ENTITY },
  );
  assert.equal(rejected.kind, 'unexpected_payload');

  // Parsed Long.MAX_VALUE JSON number loses precision in JS Number.
  const maxBody = `{"entityId":"${ENTITY}","profileId":"${PROFILE}","profileVersionNo":7,"assignmentMode":"EXPLICIT_PROFILE","rowVersion":9223372036854775807,"effectiveFrom":"2026-11-18T00:00:00Z"}`;
  const maxRejected = await getMonitoringProfileAssignment(
    fakeTransport(async () => ({
      status: 200,
      headers: headerMap({
        'Content-Type': 'application/json',
        ETag: '"9223372036854775807"',
      }),
      async text() {
        return maxBody;
      },
    })),
    { tenantId: TENANT, entityId: ENTITY },
  );
  assert.equal(maxRejected.kind, 'unexpected_payload');

  assert.equal(readSafeAssignmentEtag(headerMap({ ETag: '"9223372036854775807"' })), '"9223372036854775807"');
  assert.equal(readSafeAssignmentEtag(headerMap({ ETag: 'W/"15"' })), undefined);
  assert.equal(readSafeAssignmentEtag(headerMap({ ETag: '"15", "16"' })), undefined);
  assert.equal(readSafeAssignmentEtag(headerMap({ ETag: '""' })), undefined);

  const authorized = await getMonitoringProfileAssignment(
    fakeTransport(async () => jsonResponse(200, VALID_BODY, { ETag: '"15"' })),
    { tenantId: TENANT, entityId: ENTITY },
  );
  assert.equal(authorized.kind, 'authorized');
  assert.equal(authorized.etag, '"15"');
  assert.equal(typeof authorized.assignment.rowVersion, 'number');
  // Adapter must not treat ETag / rowVersion as a write precondition token.
  assert.equal(authorized.ifMatch, undefined);
  assert.equal(Object.prototype.hasOwnProperty.call(authorized, 'ifMatch'), false);
});

test('invalid ETag is dropped while safe projection remains authorized', async () => {
  const result = await getMonitoringProfileAssignment(
    fakeTransport(async () => jsonResponse(200, VALID_BODY, { ETag: 'W/"15"' })),
    { tenantId: TENANT, entityId: ENTITY },
  );
  assert.equal(result.kind, 'authorized');
  assert.equal(result.etag, undefined);
  assert.equal(result.assignment.rowVersion, 15);
});

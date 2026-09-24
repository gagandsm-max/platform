#!/usr/bin/env node
/**
 * Platform edge transport contract validator (BUILD-000).
 * Dependency-free Node stdlib only. Not a generic JSON Schema engine.
 * Enforces Number.isSafeInteger for JSON-carried sequences/control counters.
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, '..');
const MAX_SAFE_SEQUENCE = 9007199254740991;
const MAX_DOCUMENT_BYTES = 256 * 1024;

const UUID_RE = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const CORR_RE = /^[A-Za-z0-9._:-]+$/;
const CHECKSUM_RE = /^[a-f0-9]{64}$/;
const STREAM_RE = /^stream-[0-9a-f]{32}$/;
const TS_Z_RE = /^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?Z$/;

const SECRET_KEYS = new Set([
  'password', 'passwd', 'token', 'bearer', 'apikey', 'privatekey', 'clientsecret',
  'cookie', 'secret', 'accesstoken', 'refreshtoken', 'community',
]);

const SCHEMAS = {
  'agent-telemetry-batch': validateAgentBatch,
  'agent-telemetry-ack': validateAgentAck,
  'sensor-telemetry-batch': validateSensorBatch,
  'sensor-telemetry-ack': validateSensorAck,
  'sensor-agent-ingress-batch': validateIngressBatch,
  'sensor-agent-ingress-ack': validateIngressAck,
  'sensor-heartbeat-request': validateHeartbeatRequest,
  'sensor-heartbeat-response': validateHeartbeatResponse,
  'standard-error': validateStandardError,
};

function fail(codes, code, message) {
  codes.push(code);
  return message;
}

function isPlainObject(v) {
  return v !== null && typeof v === 'object' && !Array.isArray(v);
}

function assertNoSecrets(obj, codes, pathPrefix = '') {
  if (Array.isArray(obj)) {
    obj.forEach((v, i) => assertNoSecrets(v, codes, `${pathPrefix}[${i}]`));
    return;
  }
  if (!isPlainObject(obj)) return;
  for (const [k, v] of Object.entries(obj)) {
    const key = k.toLowerCase().replace(/[^a-z0-9]/g, '');
    if (SECRET_KEYS.has(key)) {
      fail(codes, 'FORBIDDEN_SECRET_FIELD', `${pathPrefix}.${k}`);
    }
    assertNoSecrets(v, codes, `${pathPrefix}.${k}`);
  }
}

function assertExactKeys(obj, allowed, required, codes) {
  if (!isPlainObject(obj)) {
    fail(codes, 'INVALID_CONTRACT', 'document must be an object');
    return;
  }
  for (const k of Object.keys(obj)) {
    if (!allowed.has(k)) fail(codes, 'UNKNOWN_FIELD', `unknown field: ${k}`);
  }
  for (const k of required) {
    if (!(k in obj)) fail(codes, 'MISSING_REQUIRED_FIELD', `missing: ${k}`);
  }
}

function assertSafeSequence(value, field, codes) {
  if (typeof value === 'string') {
    fail(codes, 'SEQUENCE_TYPE_INVALID', `${field} must be JSON number, not string`);
    return;
  }
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    fail(codes, 'SEQUENCE_TYPE_INVALID', `${field} must be a finite number`);
    return;
  }
  if (!Number.isInteger(value)) {
    fail(codes, 'SEQUENCE_FRACTIONAL', `${field} must be an integer`);
    return;
  }
  if (!Number.isSafeInteger(value)) {
    fail(codes, 'SEQUENCE_NOT_SAFE_INTEGER', `${field}=${value} exceeds Number.isSafeInteger`);
    return;
  }
  if (value < 1) {
    fail(codes, 'SEQUENCE_OUT_OF_BOUNDS', `${field}=${value} must be >= 1`);
    return;
  }
  if (value > MAX_SAFE_SEQUENCE) {
    fail(codes, 'SEQUENCE_OUT_OF_BOUNDS', `${field}=${value} must be <= ${MAX_SAFE_SEQUENCE}`);
  }
}

function assertSafeControl(value, field, codes, { min = 0 } = {}) {
  if (value === undefined) return;
  if (typeof value !== 'number' || !Number.isSafeInteger(value)) {
    fail(codes, 'CONTROL_COUNTER_INVALID', `${field} must be Number.isSafeInteger`);
    return;
  }
  if (value < min || value > MAX_SAFE_SEQUENCE) {
    fail(codes, 'CONTROL_COUNTER_OUT_OF_BOUNDS', `${field}=${value}`);
  }
}

function assertUuid(value, field, codes) {
  if (typeof value !== 'string' || !UUID_RE.test(value)) {
    fail(codes, 'INVALID_UUID', `${field} must be UUID text`);
  }
}

function assertAgentId(value, field, codes) {
  if (typeof value !== 'string' || value.length < 1 || value.length > 128) {
    fail(codes, 'INVALID_AGENT_ID', field);
  }
}

function assertSensorId(value, field, codes) {
  if (typeof value !== 'string' || value.length < 1 || value.length > 255) {
    fail(codes, 'INVALID_SENSOR_ID', field);
  }
}

function assertBatchId(value, field, codes) {
  if (typeof value !== 'string' || value.length < 1 || value.length > 255) {
    fail(codes, 'INVALID_BATCH_ID', field);
  }
}

function assertCorr(value, field, codes) {
  if (typeof value !== 'string' || value.length < 1 || value.length > 128 || !CORR_RE.test(value)) {
    fail(codes, 'INVALID_CORRELATION_ID', field);
  }
}

function assertChecksum(value, field, codes) {
  if (typeof value !== 'string' || !CHECKSUM_RE.test(value)) {
    fail(codes, 'INVALID_CHECKSUM', field);
  }
}

function assertStream(value, field, codes) {
  if (typeof value !== 'string' || !STREAM_RE.test(value)) {
    fail(codes, 'INVALID_SENSOR_STREAM_ID', field);
  }
}

function assertUtcTs(value, field, codes) {
  if (typeof value !== 'string' || !TS_Z_RE.test(value)) {
    fail(codes, 'INVALID_UTC_TIMESTAMP', field);
  }
}

function validateAgentBatch(doc, codes) {
  const allowed = new Set([
    'schemaVersion', 'agentId', 'tenantId', 'siteId', 'correlationId', 'batchId',
    'firstSequence', 'lastSequence', 'recordCount', 'records',
  ]);
  assertExactKeys(doc, allowed, [
    'schemaVersion', 'agentId', 'tenantId', 'correlationId', 'batchId',
    'firstSequence', 'lastSequence', 'recordCount', 'records',
  ], codes);
  if (codes.length) return;
  if (doc.schemaVersion !== 1) fail(codes, 'UNSUPPORTED_SCHEMA_VERSION', 'schemaVersion must be 1');
  assertAgentId(doc.agentId, 'agentId', codes);
  assertUuid(doc.tenantId, 'tenantId', codes);
  if ('siteId' in doc) assertUuid(doc.siteId, 'siteId', codes);
  assertCorr(doc.correlationId, 'correlationId', codes);
  assertBatchId(doc.batchId, 'batchId', codes);
  assertSafeSequence(doc.firstSequence, 'firstSequence', codes);
  assertSafeSequence(doc.lastSequence, 'lastSequence', codes);
  if (typeof doc.recordCount !== 'number' || !Number.isSafeInteger(doc.recordCount) || doc.recordCount < 1) {
    fail(codes, 'INVALID_RECORD_COUNT', 'recordCount');
  }
  if (!Array.isArray(doc.records)) {
    fail(codes, 'INVALID_CONTRACT', 'records must be array');
    return;
  }
  if (doc.recordCount !== doc.records.length) fail(codes, 'RECORD_COUNT_MISMATCH', 'recordCount != records.length');
  if (doc.firstSequence > doc.lastSequence) fail(codes, 'SEQUENCE_BOUNDS_INVALID', 'firstSequence > lastSequence');
  const seen = new Set();
  for (const [i, rec] of doc.records.entries()) {
    const rAllowed = new Set(['sequence', 'id', 'createdAt', 'checksum', 'payload']);
    assertExactKeys(rec, rAllowed, ['sequence', 'id', 'createdAt', 'checksum', 'payload'], codes);
    assertSafeSequence(rec.sequence, `records[${i}].sequence`, codes);
    if (seen.has(rec.sequence)) fail(codes, 'DUPLICATE_SEQUENCE', `records[${i}].sequence`);
    seen.add(rec.sequence);
    if (typeof rec.id !== 'string' || !CORR_RE.test(rec.id)) fail(codes, 'INVALID_RECORD_ID', `records[${i}].id`);
    assertUtcTs(rec.createdAt, `records[${i}].createdAt`, codes);
    assertChecksum(rec.checksum, `records[${i}].checksum`, codes);
    if (!isPlainObject(rec.payload)) fail(codes, 'INVALID_CONTRACT', `records[${i}].payload`);
    if ('entityId' in rec) fail(codes, 'FORBIDDEN_ENTITY_ID', 'Agent v1 must not carry authoritative entityId');
  }
  if (doc.records.length) {
    const first = doc.records[0].sequence;
    const last = doc.records[doc.records.length - 1].sequence;
    if (first !== doc.firstSequence || last !== doc.lastSequence) {
      fail(codes, 'SEQUENCE_BOUNDS_INVALID', 'envelope bounds must match first/last record sequences');
    }
  }
  assertNoSecrets(doc, codes);
}

function validateAgentAck(doc, codes, ctx = {}) {
  const allowed = new Set(['status', 'accepted_through_sequence', 'agent_id', 'batch_id', 'correlation_id']);
  assertExactKeys(doc, allowed, ['status', 'accepted_through_sequence', 'agent_id', 'batch_id', 'correlation_id'], codes);
  // Wrong casing probes
  for (const bad of ['acceptedThroughSequence', 'agentId', 'batchId', 'correlationId']) {
    if (bad in (ctx.raw || {})) fail(codes, 'WRONG_ACK_CASING', bad);
  }
  if (codes.length && !doc) return;
  if (doc.status !== 'ACCEPTED') fail(codes, 'INVALID_ACK_STATUS', 'status');
  assertSafeSequence(doc.accepted_through_sequence, 'accepted_through_sequence', codes);
  assertAgentId(doc.agent_id, 'agent_id', codes);
  assertBatchId(doc.batch_id, 'batch_id', codes);
  assertCorr(doc.correlation_id, 'correlation_id', codes);
  if (ctx.submitted) {
    const first = ctx.submitted.firstSequence;
    const last = ctx.submitted.lastSequence;
    const ack = doc.accepted_through_sequence;
    if (typeof first === 'number' && typeof ack === 'number' && Number.isSafeInteger(ack)) {
      if (ack < first) fail(codes, 'ACK_BELOW_FIRST', 'accepted_through_sequence < firstSequence');
      if (ack > last) fail(codes, 'ACK_ABOVE_LAST', 'accepted_through_sequence > lastSequence');
    }
  }
}

function validateSensorBatch(doc, codes) {
  const allowed = new Set([
    'schemaVersion', 'sensorId', 'bootEpoch', 'batchId', 'firstSequence', 'lastSequence', 'checksum', 'records',
  ]);
  assertExactKeys(doc, allowed, [
    'schemaVersion', 'sensorId', 'bootEpoch', 'batchId', 'firstSequence', 'lastSequence', 'checksum', 'records',
  ], codes);
  if (codes.length) return;
  if (doc.schemaVersion !== 1) fail(codes, 'UNSUPPORTED_SCHEMA_VERSION', 'schemaVersion must be 1');
  assertSensorId(doc.sensorId, 'sensorId', codes);
  assertStream(doc.bootEpoch, 'bootEpoch', codes);
  assertBatchId(doc.batchId, 'batchId', codes);
  assertSafeSequence(doc.firstSequence, 'firstSequence', codes);
  assertSafeSequence(doc.lastSequence, 'lastSequence', codes);
  assertChecksum(doc.checksum, 'checksum', codes);
  if (!Array.isArray(doc.records) || doc.records.length < 1) fail(codes, 'INVALID_CONTRACT', 'records');
  if (doc.firstSequence > doc.lastSequence) fail(codes, 'SEQUENCE_BOUNDS_INVALID', 'first > last');
  assertNoSecrets(doc, codes);
}

function validateSensorAck(doc, codes, ctx = {}) {
  const allowed = new Set(['status', 'accepted_through_sequence', 'sensor_identity']);
  assertExactKeys(doc, allowed, ['status', 'accepted_through_sequence', 'sensor_identity'], codes);
  if (doc && typeof doc.status === 'string' && doc.status.length < 1) fail(codes, 'INVALID_ACK_STATUS', 'status');
  if (doc) {
    assertSafeSequence(doc.accepted_through_sequence, 'accepted_through_sequence', codes);
    assertSensorId(doc.sensor_identity, 'sensor_identity', codes);
  }
  if (ctx.submitted) {
    const first = ctx.submitted.firstSequence;
    const last = ctx.submitted.lastSequence;
    const ack = doc?.accepted_through_sequence;
    if (typeof first === 'number' && typeof ack === 'number' && Number.isSafeInteger(ack)) {
      if (ack < first) fail(codes, 'ACK_BELOW_FIRST', 'accepted_through_sequence < firstSequence');
      if (ack > last) fail(codes, 'ACK_ABOVE_LAST', 'accepted_through_sequence > lastSequence');
    }
  }
}

function validateIngressBatch(doc, codes) {
  const allowed = new Set([
    'schemaVersion', 'sensorId', 'batchId', 'firstIngressSequence', 'lastIngressSequence',
    'recordCount', 'checksum', 'correlationId', 'records',
  ]);
  assertExactKeys(doc, allowed, [
    'schemaVersion', 'sensorId', 'batchId', 'firstIngressSequence', 'lastIngressSequence',
    'recordCount', 'checksum', 'correlationId', 'records',
  ], codes);
  if (codes.length) return;
  if (doc.schemaVersion !== 1) fail(codes, 'UNSUPPORTED_SCHEMA_VERSION', 'schemaVersion');
  assertSensorId(doc.sensorId, 'sensorId', codes);
  assertBatchId(doc.batchId, 'batchId', codes);
  assertSafeSequence(doc.firstIngressSequence, 'firstIngressSequence', codes);
  assertSafeSequence(doc.lastIngressSequence, 'lastIngressSequence', codes);
  if (doc.firstIngressSequence > doc.lastIngressSequence) fail(codes, 'SEQUENCE_BOUNDS_INVALID', 'first > last');
  if (typeof doc.recordCount !== 'number' || !Number.isSafeInteger(doc.recordCount) || doc.recordCount < 1) {
    fail(codes, 'INVALID_RECORD_COUNT', 'recordCount');
  }
  assertChecksum(doc.checksum, 'checksum', codes);
  assertCorr(doc.correlationId, 'correlationId', codes);
  if (!Array.isArray(doc.records)) {
    fail(codes, 'INVALID_CONTRACT', 'records');
    return;
  }
  if (doc.recordCount !== doc.records.length) fail(codes, 'RECORD_COUNT_MISMATCH', 'recordCount');
  const seen = new Set();
  for (const [i, rec] of doc.records.entries()) {
    const rAllowed = new Set([
      'sensorIngressSequence', 'sensorIngressRecordId', 'tenantId', 'agentId', 'acceptingSensorId',
      'assignmentGeneration', 'agentFirstSequence', 'agentLastSequence', 'agentBatchId',
      'correlationId', 'acceptedAt', 'records',
    ]);
    assertExactKeys(rec, rAllowed, [...rAllowed], codes);
    assertSafeSequence(rec.sensorIngressSequence, `records[${i}].sensorIngressSequence`, codes);
    if (seen.has(rec.sensorIngressSequence)) fail(codes, 'DUPLICATE_SEQUENCE', `records[${i}]`);
    seen.add(rec.sensorIngressSequence);
    assertUuid(rec.tenantId, `records[${i}].tenantId`, codes);
    assertAgentId(rec.agentId, `records[${i}].agentId`, codes);
    assertSensorId(rec.acceptingSensorId, `records[${i}].acceptingSensorId`, codes);
    assertSafeControl(rec.assignmentGeneration, `records[${i}].assignmentGeneration`, codes, { min: 1 });
    assertSafeSequence(rec.agentFirstSequence, `records[${i}].agentFirstSequence`, codes);
    assertSafeSequence(rec.agentLastSequence, `records[${i}].agentLastSequence`, codes);
    assertBatchId(rec.agentBatchId, `records[${i}].agentBatchId`, codes);
    assertCorr(rec.correlationId, `records[${i}].correlationId`, codes);
    assertUtcTs(rec.acceptedAt, `records[${i}].acceptedAt`, codes);
    if (!Array.isArray(rec.records)) fail(codes, 'INVALID_CONTRACT', `records[${i}].records`);
  }
  assertNoSecrets(doc, codes);
}

function validateIngressAck(doc, codes, ctx = {}) {
  const allowed = new Set(['status', 'sensorId', 'batchId', 'acceptedThroughIngressSequence', 'correlationId']);
  assertExactKeys(doc, allowed, ['status', 'sensorId', 'batchId', 'acceptedThroughIngressSequence', 'correlationId'], codes);
  for (const bad of ['accepted_through_sequence', 'sensor_id', 'batch_id', 'correlation_id']) {
    if (ctx.raw && bad in ctx.raw) fail(codes, 'WRONG_ACK_CASING', bad);
  }
  if (!doc) return;
  if (typeof doc.status !== 'string' || !doc.status) fail(codes, 'INVALID_ACK_STATUS', 'status');
  assertSensorId(doc.sensorId, 'sensorId', codes);
  assertBatchId(doc.batchId, 'batchId', codes);
  assertSafeSequence(doc.acceptedThroughIngressSequence, 'acceptedThroughIngressSequence', codes);
  assertCorr(doc.correlationId, 'correlationId', codes);
  if (ctx.submitted) {
    const first = ctx.submitted.firstIngressSequence;
    const last = ctx.submitted.lastIngressSequence;
    const ack = doc.acceptedThroughIngressSequence;
    if (typeof first === 'number' && typeof ack === 'number' && Number.isSafeInteger(ack)) {
      if (ack < first) fail(codes, 'ACK_BELOW_FIRST', 'acceptedThroughIngressSequence < firstIngressSequence');
      if (ack > last) fail(codes, 'ACK_ABOVE_LAST', 'acceptedThroughIngressSequence > lastIngressSequence');
    }
  }
}

function validateHeartbeatRequest(doc, codes) {
  const allowed = new Set([
    'sensorId', 'version', 'role', 'state', 'capabilities', 'queueBytes',
    'oldestQueuedAgeSec', 'assignmentVersion', 'policyVersionWatermark',
  ]);
  assertExactKeys(doc, allowed, ['sensorId', 'version', 'role', 'state'], codes);
  if (codes.length) return;
  assertSensorId(doc.sensorId, 'sensorId', codes);
  if (typeof doc.version !== 'string' || !doc.version) fail(codes, 'INVALID_CONTRACT', 'version');
  if (typeof doc.role !== 'string' || !doc.role) fail(codes, 'INVALID_CONTRACT', 'role');
  if (typeof doc.state !== 'string' || !doc.state) fail(codes, 'INVALID_CONTRACT', 'state');
  assertSafeControl(doc.assignmentVersion, 'assignmentVersion', codes, { min: 0 });
  assertSafeControl(doc.policyVersionWatermark, 'policyVersionWatermark', codes, { min: 0 });
  assertSafeControl(doc.queueBytes, 'queueBytes', codes, { min: 0 });
  assertSafeControl(doc.oldestQueuedAgeSec, 'oldestQueuedAgeSec', codes, { min: 0 });
  assertNoSecrets(doc, codes);
}

function validateHeartbeatResponse(doc, codes) {
  const allowed = new Set(['status', 'authenticated', 'sensor_identity', 'tenant_id', 'received_at']);
  assertExactKeys(doc, allowed, ['status', 'authenticated', 'sensor_identity', 'tenant_id', 'received_at'], codes);
  // Desired-state authority fields are forbidden on v1 response.
  const forbiddenAuthority = [
    'desiredConfigVersion', 'assignmentVersion', 'policyVersionWatermark', 'drain',
    'desired_config_version', 'assignment_version',
  ];
  for (const k of forbiddenAuthority) {
    // Detected via UNKNOWN_FIELD if present because additionalProperties false.
  }
  if (!doc) return;
  if (typeof doc.status !== 'string' || !doc.status) fail(codes, 'INVALID_CONTRACT', 'status');
  if (typeof doc.authenticated !== 'boolean') fail(codes, 'INVALID_CONTRACT', 'authenticated');
  assertSensorId(doc.sensor_identity, 'sensor_identity', codes);
  assertUuid(doc.tenant_id, 'tenant_id', codes);
  assertUtcTs(doc.received_at, 'received_at', codes);
}

function validateStandardError(doc, codes) {
  const allowed = new Set(['status', 'code', 'message']);
  assertExactKeys(doc, allowed, ['status', 'code', 'message'], codes);
  if (!doc) return;
  if (typeof doc.status !== 'string' || !doc.status) fail(codes, 'INVALID_CONTRACT', 'status');
  if (typeof doc.code !== 'string' || !doc.code || !CORR_RE.test(doc.code)) {
    fail(codes, 'MISSING_STABLE_CODE', 'code');
  }
  if (typeof doc.message !== 'string' || !doc.message) fail(codes, 'INVALID_CONTRACT', 'message');
}

function deepClone(v) {
  return JSON.parse(JSON.stringify(v));
}

function getPath(obj, parts) {
  let cur = obj;
  for (const p of parts) {
    if (cur == null) return undefined;
    cur = cur[p];
  }
  return cur;
}

function setPath(obj, parts, value) {
  let cur = obj;
  for (let i = 0; i < parts.length - 1; i++) {
    const p = parts[i];
    if (cur[p] == null || typeof cur[p] !== 'object') cur[p] = typeof parts[i + 1] === 'number' ? [] : {};
    cur = cur[p];
  }
  cur[parts[parts.length - 1]] = value;
}

function deletePath(obj, parts) {
  let cur = obj;
  for (let i = 0; i < parts.length - 1; i++) cur = cur?.[parts[i]];
  if (cur && typeof cur === 'object') delete cur[parts[parts.length - 1]];
}

function applyMutation(doc, mutation) {
  const op = mutation.op;
  const p = mutation.path || [];
  if (op === 'set') setPath(doc, p, mutation.value);
  else if (op === 'delete') deletePath(doc, p);
  else if (op === 'replaceRaw') Object.assign(doc, mutation.value);
  else throw new Error(`unknown mutation op: ${op}`);
}

function loadJson(rel) {
  const full = path.join(ROOT, rel);
  const buf = fs.readFileSync(full);
  if (buf.length > MAX_DOCUMENT_BYTES) throw new Error(`document too large: ${rel}`);
  return JSON.parse(buf.toString('utf8'));
}

function validateDocument(schemaName, doc, ctx = {}) {
  const codes = [];
  const fn = SCHEMAS[schemaName];
  if (!fn) {
    codes.push('UNKNOWN_SCHEMA');
    return codes;
  }
  fn(doc, codes, ctx);
  return [...new Set(codes)];
}

function main() {
  const fixtures = loadJson('fixtures/cases.json');
  const examples = {};
  for (const [key, rel] of Object.entries(fixtures.examples)) {
    examples[key] = loadJson(rel);
  }

  let passed = 0;
  let failed = 0;
  const failures = [];

  for (const testCase of fixtures.cases) {
    const schema = testCase.schema;
    let doc;
    if (testCase.raw) {
      doc = deepClone(testCase.raw);
    } else {
      const baseKey = testCase.base;
      if (!examples[baseKey]) throw new Error(`missing example base: ${baseKey}`);
      doc = deepClone(examples[baseKey]);
      for (const m of testCase.mutations || []) applyMutation(doc, m);
    }
    const ctx = {
      raw: doc,
      submitted: testCase.submitted
        ? (typeof testCase.submitted === 'string' ? examples[testCase.submitted] : testCase.submitted)
        : undefined,
    };
    // For wrong-casing ACK tests, mutations may introduce camelCase fields while deleting snake_case.
    const codes = validateDocument(schema, doc, ctx);
    const valid = codes.length === 0;
    const expectValid = testCase.valid === true;
    let ok = valid === expectValid;
    if (!expectValid && testCase.expectedCodes?.length) {
      ok = testCase.expectedCodes.every((c) => codes.includes(c));
    }
    if (ok) {
      passed += 1;
    } else {
      failed += 1;
      failures.push({
        name: testCase.name,
        expectValid,
        codes,
        expectedCodes: testCase.expectedCodes || [],
      });
    }
  }

  // Prove schema JSON parse + MAX_SAFE_SEQUENCE constant present in common defs.
  const common = loadJson('common-definitions.schema.json');
  if (common.$defs.MAX_SAFE_SEQUENCE.const !== MAX_SAFE_SEQUENCE) {
    failed += 1;
    failures.push({ name: 'common-max-safe-sequence-const', codes: ['MAX_SAFE_SEQUENCE_MISMATCH'] });
  } else {
    passed += 1;
  }

  console.log(`EDGE CONTRACT VALIDATION: ${passed} passed, ${failed} failed`);
  console.log(`MAX_SAFE_SEQUENCE=${MAX_SAFE_SEQUENCE}`);
  if (failures.length) {
    for (const f of failures) {
      console.error(`  FAIL ${f.name}: codes=${JSON.stringify(f.codes)} expected=${JSON.stringify(f.expectedCodes || [])}`);
    }
    process.exit(1);
  }
  console.log('EDGE CONTRACT VALIDATION PASSED');
}

main();

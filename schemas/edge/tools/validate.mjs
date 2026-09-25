#!/usr/bin/env node
/**
 * Platform edge transport contract validator (BUILD-000).
 * Dependency-free Node stdlib only. Not a generic JSON Schema engine.
 * Enforces Number.isSafeInteger for JSON-carried sequences/control counters.
 * Includes bounded schema-set self-check to prevent schema/validator drift.
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, '..');

/** FROZEN_BUILD_000_INVARIANT — JSON-safe ceiling (2^53-1). Runtime hop enforcement: BUILD-002/003. */
const MAX_SAFE_SEQUENCE = 9007199254740991;
/** CURRENT_IMPLEMENTED_V1 Agent→Sensor receiver DefaultMaxRecords. */
const AGENT_MAX_RECORDS = 100;
/** CURRENT_IMPLEMENTED_V1 Core IngestionValidator.MAX_RECORDS. */
const SENSOR_TELEMETRY_MAX_RECORDS = 1000;
/** CURRENT_IMPLEMENTED_V1 Core AgentIngestionValidator.MAX_RECORDS. */
const AGENT_INGRESS_MAX_RECORDS = 1000;
/** CURRENT_IMPLEMENTED_V1 Agent→Sensor DeliveryEnvelope.batchId (validateIdentifier max 256). */
const AGENT_DELIVERY_BATCH_ID_MAX = 256;
/** CURRENT_IMPLEMENTED_V1 Core-bound batchId (VARCHAR(255)). */
const CORE_BATCH_ID_MAX = 255;
/**
 * Local fixture/input safety bound for this test tool only.
 * NOT the Agent/Sensor/Core wire batch-size contract.
 */
const MAX_FIXTURE_DOCUMENT_BYTES = 256 * 1024;

const UUID_RE = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const CORR_RE = /^[A-Za-z0-9._:-]+$/;
/** Accepted wire: A-F or a-f. Canonical producer emission: lowercase recommended. */
const CHECKSUM_RE = /^[A-Fa-f0-9]{64}$/;
const STREAM_RE = /^stream-[0-9a-f]{32}$/;
const TS_Z_RE = /^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?Z$/;
const DRAFT_2020_12 = 'https://json-schema.org/draft/2020-12/schema';

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

function assertAgentDeliveryBatchId(value, field, codes) {
  if (typeof value !== 'string' || value.length < 1 || value.length > AGENT_DELIVERY_BATCH_ID_MAX) {
    fail(codes, 'INVALID_BATCH_ID', field);
  }
}

function assertCoreBatchId(value, field, codes) {
  if (typeof value !== 'string' || value.length < 1 || value.length > CORE_BATCH_ID_MAX) {
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

function assertAgentStream(value, field, codes) {
  if (typeof value !== 'string' || !STREAM_RE.test(value)) {
    fail(codes, 'INVALID_AGENT_STREAM_ID', field);
  }
}

function assertUtcTs(value, field, codes) {
  if (typeof value !== 'string' || !TS_Z_RE.test(value)) {
    fail(codes, 'INVALID_UTC_TIMESTAMP', field);
  }
}

/**
 * Core IngestionValidator / AgentIngestionValidator semantics:
 * complete unordered set of sequences in [first, last], no duplicates,
 * count == last-first+1. NOT used for Agent→Sensor delivery (stricter).
 */
function assertCompleteSequenceSet(records, first, last, codes, seqField = 'sequence') {
  if (!Array.isArray(records)) {
    fail(codes, 'INVALID_CONTRACT', 'records must be array');
    return;
  }
  const expectedCount = last - first + 1;
  if (!Number.isSafeInteger(expectedCount) || expectedCount < 1) {
    fail(codes, 'SEQUENCE_BOUNDS_INVALID', 'invalid first/last span');
    return;
  }
  if (records.length !== expectedCount) {
    fail(codes, 'RECORD_COUNT_MISMATCH', `records.length ${records.length} != last-first+1 (${expectedCount})`);
  }
  const seen = new Set();
  for (let i = 0; i < records.length; i++) {
    const rec = records[i];
    if (!isPlainObject(rec)) {
      fail(codes, 'INVALID_CONTRACT', `records[${i}] must be object`);
      continue;
    }
    if (!(seqField in rec)) {
      fail(codes, 'MISSING_REQUIRED_FIELD', `records[${i}].${seqField}`);
      continue;
    }
    assertSafeSequence(rec[seqField], `records[${i}].${seqField}`, codes);
    const seq = rec[seqField];
    if (typeof seq !== 'number' || !Number.isSafeInteger(seq)) continue;
    if (seq < first) fail(codes, 'SEQUENCE_BELOW_RANGE', `records[${i}].${seqField}=${seq}`);
    if (seq > last) fail(codes, 'SEQUENCE_ABOVE_RANGE', `records[${i}].${seqField}=${seq}`);
    if (seen.has(seq)) fail(codes, 'DUPLICATE_SEQUENCE', `records[${i}].${seqField}=${seq}`);
    seen.add(seq);
  }
  if (Number.isSafeInteger(first) && Number.isSafeInteger(last) && first <= last) {
    for (let s = first; s <= last; s++) {
      if (!seen.has(s)) {
        fail(codes, 'SEQUENCE_GAP', `missing sequence ${s}`);
        break;
      }
    }
  }
}

/**
 * Sensor ValidateEnvelope semantics: strictly ascending contiguous sequences.
 * records[i].sequence == records[i-1].sequence + 1; endpoints match first/last;
 * length == last-first+1. Do NOT use unordered-set semantics here.
 */
function assertStrictContiguousAgentSequences(records, first, last, codes, pathPrefix = 'records') {
  if (!Array.isArray(records)) {
    fail(codes, 'INVALID_CONTRACT', `${pathPrefix} must be array`);
    return;
  }
  const expectedCount = last - first + 1;
  if (!Number.isSafeInteger(expectedCount) || expectedCount < 1) {
    fail(codes, 'SEQUENCE_BOUNDS_INVALID', `${pathPrefix} invalid first/last span`);
    return;
  }
  if (records.length !== expectedCount) {
    fail(codes, 'RECORD_COUNT_MISMATCH', `${pathPrefix}.length ${records.length} != last-first+1 (${expectedCount})`);
  }
  if (!records.length) return;
  if (records[0]?.sequence !== first) {
    fail(codes, 'SEQUENCE_BOUNDS_INVALID', `${pathPrefix}[0].sequence must equal firstSequence ${first}`);
  }
  if (records[records.length - 1]?.sequence !== last) {
    fail(codes, 'SEQUENCE_BOUNDS_INVALID', `${pathPrefix}[last].sequence must equal lastSequence ${last}`);
  }
  for (let i = 0; i < records.length; i++) {
    const seq = records[i]?.sequence;
    if (typeof seq !== 'number' || !Number.isSafeInteger(seq)) continue;
    if (i > 0) {
      const prev = records[i - 1]?.sequence;
      if (typeof prev === 'number' && Number.isSafeInteger(prev) && seq !== prev + 1) {
        fail(codes, 'SEQUENCE_NOT_CONTIGUOUS', `${pathPrefix}[${i}].sequence expected ${prev + 1}, got ${seq}`);
      }
    }
  }
}

function validateAgentDeliveryRecord(rec, pathPrefix, codes) {
  const rAllowed = new Set(['sequence', 'id', 'createdAt', 'checksum', 'payload']);
  assertExactKeys(rec, rAllowed, ['sequence', 'id', 'createdAt', 'checksum', 'payload'], codes);
  assertSafeSequence(rec.sequence, `${pathPrefix}.sequence`, codes);
  if (typeof rec.id !== 'string' || !CORR_RE.test(rec.id)) fail(codes, 'INVALID_RECORD_ID', `${pathPrefix}.id`);
  assertUtcTs(rec.createdAt, `${pathPrefix}.createdAt`, codes);
  assertChecksum(rec.checksum, `${pathPrefix}.checksum`, codes);
  // Receiver-compatible: any non-empty JSON (json.RawMessage). Producer-current: object.
  if (!('payload' in rec) || rec.payload === undefined) {
    fail(codes, 'MISSING_REQUIRED_FIELD', `${pathPrefix}.payload`);
  } else {
    try {
      const encoded = JSON.stringify(rec.payload);
      if (encoded === undefined || encoded.length < 1) {
        fail(codes, 'INVALID_CONTRACT', `${pathPrefix}.payload must be non-empty JSON`);
      }
    } catch {
      fail(codes, 'INVALID_CONTRACT', `${pathPrefix}.payload must be JSON-serializable`);
    }
  }
  if (isPlainObject(rec) && 'entityId' in rec) {
    fail(codes, 'FORBIDDEN_ENTITY_ID', 'Agent v1 must not carry authoritative entityId');
  }
}

function validateAgentBatch(doc, codes) {
  const allowed = new Set([
    'schemaVersion', 'agentId', 'tenantId', 'siteId', 'correlationId', 'batchId',
    'agentStreamId', 'firstSequence', 'lastSequence', 'recordCount', 'records',
  ]);
  assertExactKeys(doc, allowed, [
    'schemaVersion', 'agentId', 'tenantId', 'correlationId', 'batchId',
    'firstSequence', 'lastSequence', 'recordCount', 'records',
  ], codes);
  if (codes.length) return;
  if (doc.schemaVersion !== 1 && doc.schemaVersion !== 2) {
    fail(codes, 'UNSUPPORTED_SCHEMA_VERSION', 'schemaVersion must be 1 or 2');
  }
  if (doc.schemaVersion === 2) {
    if (!('agentStreamId' in doc)) {
      fail(codes, 'AGENT_STREAM_ID_REQUIRED', 'agentStreamId is required for schemaVersion 2');
      fail(codes, 'MISSING_REQUIRED_FIELD', 'agentStreamId');
    } else {
      assertAgentStream(doc.agentStreamId, 'agentStreamId', codes);
    }
  } else if ('agentStreamId' in doc) {
    fail(codes, 'AGENT_STREAM_ID_FORBIDDEN', 'agentStreamId must be absent for schemaVersion 1');
  }
  assertAgentId(doc.agentId, 'agentId', codes);
  assertUuid(doc.tenantId, 'tenantId', codes);
  if ('siteId' in doc) assertUuid(doc.siteId, 'siteId', codes);
  assertCorr(doc.correlationId, 'correlationId', codes);
  assertAgentDeliveryBatchId(doc.batchId, 'batchId', codes);
  assertSafeSequence(doc.firstSequence, 'firstSequence', codes);
  assertSafeSequence(doc.lastSequence, 'lastSequence', codes);
  if (typeof doc.recordCount !== 'number' || !Number.isSafeInteger(doc.recordCount) || doc.recordCount < 1) {
    fail(codes, 'INVALID_RECORD_COUNT', 'recordCount');
  } else if (doc.recordCount > AGENT_MAX_RECORDS) {
    fail(codes, 'RECORD_COUNT_EXCEEDED', `recordCount ${doc.recordCount} > ${AGENT_MAX_RECORDS}`);
  }
  if (!Array.isArray(doc.records)) {
    fail(codes, 'INVALID_CONTRACT', 'records must be array');
    return;
  }
  if (doc.records.length > AGENT_MAX_RECORDS) {
    fail(codes, 'RECORD_COUNT_EXCEEDED', `records.length ${doc.records.length} > ${AGENT_MAX_RECORDS}`);
  }
  if (doc.recordCount !== doc.records.length) fail(codes, 'RECORD_COUNT_MISMATCH', 'recordCount != records.length');
  if (doc.firstSequence > doc.lastSequence) fail(codes, 'SEQUENCE_BOUNDS_INVALID', 'firstSequence > lastSequence');
  const expectedSpan = doc.lastSequence - doc.firstSequence + 1;
  if (Number.isSafeInteger(expectedSpan) && doc.recordCount !== expectedSpan) {
    fail(codes, 'RECORD_COUNT_MISMATCH', 'recordCount != lastSequence-firstSequence+1');
  }
  for (const [i, rec] of doc.records.entries()) {
    validateAgentDeliveryRecord(rec, `records[${i}]`, codes);
  }
  assertStrictContiguousAgentSequences(doc.records, doc.firstSequence, doc.lastSequence, codes, 'records');
  assertNoSecrets(doc, codes);
}

function validateAgentAck(doc, codes, ctx = {}) {
  const allowed = new Set(['status', 'accepted_through_sequence', 'agent_id', 'batch_id', 'correlation_id']);
  assertExactKeys(doc, allowed, ['status', 'accepted_through_sequence', 'agent_id', 'batch_id', 'correlation_id'], codes);
  for (const bad of ['acceptedThroughSequence', 'agentId', 'batchId', 'correlationId']) {
    if (bad in (ctx.raw || {})) fail(codes, 'WRONG_ACK_CASING', bad);
  }
  if (codes.length && !doc) return;
  if (doc.status !== 'ACCEPTED') fail(codes, 'INVALID_ACK_STATUS', 'status');
  assertSafeSequence(doc.accepted_through_sequence, 'accepted_through_sequence', codes);
  assertAgentId(doc.agent_id, 'agent_id', codes);
  assertAgentDeliveryBatchId(doc.batch_id, 'batch_id', codes);
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
  assertCoreBatchId(doc.batchId, 'batchId', codes);
  assertSafeSequence(doc.firstSequence, 'firstSequence', codes);
  assertSafeSequence(doc.lastSequence, 'lastSequence', codes);
  assertChecksum(doc.checksum, 'checksum', codes);
  if (!Array.isArray(doc.records)) {
    fail(codes, 'INVALID_CONTRACT', 'records must be array');
    return;
  }
  if (doc.records.length < 1) fail(codes, 'INVALID_CONTRACT', 'records empty');
  if (doc.records.length > SENSOR_TELEMETRY_MAX_RECORDS) {
    fail(codes, 'RECORD_COUNT_EXCEEDED', `records.length ${doc.records.length} > ${SENSOR_TELEMETRY_MAX_RECORDS}`);
  }
  if (doc.firstSequence > doc.lastSequence) fail(codes, 'SEQUENCE_BOUNDS_INVALID', 'first > last');
  else assertCompleteSequenceSet(doc.records, doc.firstSequence, doc.lastSequence, codes, 'sequence');
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
    'schemaVersion', 'sensorId', 'sensorAgentSpoolStreamId', 'batchId', 'firstIngressSequence', 'lastIngressSequence',
    'recordCount', 'checksum', 'correlationId', 'records',
  ]);
  const required = [
    'schemaVersion', 'sensorId', 'batchId', 'firstIngressSequence', 'lastIngressSequence',
    'recordCount', 'checksum', 'correlationId', 'records',
  ];
  assertExactKeys(doc, allowed, required, codes);
  if (codes.length) return;
  if (doc.schemaVersion !== 1 && doc.schemaVersion !== 2) {
    fail(codes, 'UNSUPPORTED_SCHEMA_VERSION', 'schemaVersion must be 1 or 2');
  }
  if (doc.schemaVersion === 2) {
    if (!Object.prototype.hasOwnProperty.call(doc, 'sensorAgentSpoolStreamId')) {
      fail(codes, 'SENSOR_AGENT_SPOOL_STREAM_ID_REQUIRED', 'sensorAgentSpoolStreamId is required for schemaVersion 2');
    } else {
      assertStream(doc.sensorAgentSpoolStreamId, 'sensorAgentSpoolStreamId', codes);
    }
  } else if (Object.prototype.hasOwnProperty.call(doc, 'sensorAgentSpoolStreamId')) {
    fail(codes, 'SENSOR_AGENT_SPOOL_STREAM_ID_FORBIDDEN', 'sensorAgentSpoolStreamId must be absent for schemaVersion 1');
  }
  assertSensorId(doc.sensorId, 'sensorId', codes);
  assertCoreBatchId(doc.batchId, 'batchId', codes);
  assertSafeSequence(doc.firstIngressSequence, 'firstIngressSequence', codes);
  assertSafeSequence(doc.lastIngressSequence, 'lastIngressSequence', codes);
  if (doc.firstIngressSequence > doc.lastIngressSequence) fail(codes, 'SEQUENCE_BOUNDS_INVALID', 'first > last');
  if (typeof doc.recordCount !== 'number' || !Number.isSafeInteger(doc.recordCount) || doc.recordCount < 1) {
    fail(codes, 'INVALID_RECORD_COUNT', 'recordCount');
  } else if (doc.recordCount > AGENT_INGRESS_MAX_RECORDS) {
    fail(codes, 'RECORD_COUNT_EXCEEDED', `recordCount ${doc.recordCount} > ${AGENT_INGRESS_MAX_RECORDS}`);
  }
  assertChecksum(doc.checksum, 'checksum', codes);
  assertCorr(doc.correlationId, 'correlationId', codes);
  if (!Array.isArray(doc.records)) {
    fail(codes, 'INVALID_CONTRACT', 'records');
    return;
  }
  if (doc.records.length > AGENT_INGRESS_MAX_RECORDS) {
    fail(codes, 'RECORD_COUNT_EXCEEDED', `records.length ${doc.records.length} > ${AGENT_INGRESS_MAX_RECORDS}`);
  }
  if (doc.recordCount !== doc.records.length) fail(codes, 'RECORD_COUNT_MISMATCH', 'recordCount');
  // Outer ingress: Core AgentIngestionValidator complete unordered set.
  if (!(doc.firstIngressSequence > doc.lastIngressSequence)) {
    assertCompleteSequenceSet(
      doc.records,
      doc.firstIngressSequence,
      doc.lastIngressSequence,
      codes,
      'sensorIngressSequence',
    );
  }
  for (const [i, rec] of doc.records.entries()) {
    const rAllowed = new Set([
      'sensorIngressSequence', 'sensorIngressRecordId', 'tenantId', 'agentId', 'acceptingSensorId',
      'assignmentGeneration', 'agentFirstSequence', 'agentLastSequence', 'agentBatchId',
      'correlationId', 'acceptedAt', 'records',
    ]);
    assertExactKeys(rec, rAllowed, [...rAllowed], codes);
    assertUuid(rec.tenantId, `records[${i}].tenantId`, codes);
    assertAgentId(rec.agentId, `records[${i}].agentId`, codes);
    assertSensorId(rec.acceptingSensorId, `records[${i}].acceptingSensorId`, codes);
    assertSafeControl(rec.assignmentGeneration, `records[${i}].assignmentGeneration`, codes, { min: 1 });
    assertSafeSequence(rec.agentFirstSequence, `records[${i}].agentFirstSequence`, codes);
    assertSafeSequence(rec.agentLastSequence, `records[${i}].agentLastSequence`, codes);
    assertAgentDeliveryBatchId(rec.agentBatchId, `records[${i}].agentBatchId`, codes);
    assertCorr(rec.correlationId, `records[${i}].correlationId`, codes);
    assertUtcTs(rec.acceptedAt, `records[${i}].acceptedAt`, codes);
    if (!Array.isArray(rec.records)) {
      fail(codes, 'INVALID_CONTRACT', `records[${i}].records`);
      continue;
    }
    if (rec.records.length < 1) {
      fail(codes, 'INVALID_CONTRACT', `records[${i}].records must be non-empty`);
    }
    if (rec.records.length > AGENT_MAX_RECORDS) {
      fail(codes, 'RECORD_COUNT_EXCEEDED', `records[${i}].records.length ${rec.records.length} > ${AGENT_MAX_RECORDS}`);
    }
    for (const [j, delivery] of rec.records.entries()) {
      validateAgentDeliveryRecord(delivery, `records[${i}].records[${j}]`, codes);
    }
    if (
      typeof rec.agentFirstSequence === 'number'
      && typeof rec.agentLastSequence === 'number'
      && Number.isSafeInteger(rec.agentFirstSequence)
      && Number.isSafeInteger(rec.agentLastSequence)
      && rec.agentFirstSequence <= rec.agentLastSequence
    ) {
      assertStrictContiguousAgentSequences(
        rec.records,
        rec.agentFirstSequence,
        rec.agentLastSequence,
        codes,
        `records[${i}].records`,
      );
    } else if (rec.agentFirstSequence > rec.agentLastSequence) {
      fail(codes, 'SEQUENCE_BOUNDS_INVALID', `records[${i}] agentFirst > agentLast`);
    }
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
  assertCoreBatchId(doc.batchId, 'batchId', codes);
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
  if (buf.length > MAX_FIXTURE_DOCUMENT_BYTES) throw new Error(`fixture document too large: ${rel}`);
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

function listSchemaFiles() {
  return fs.readdirSync(ROOT)
    .filter((f) => f.endsWith('.schema.json'))
    .sort();
}

function collectRefs(node, out = []) {
  if (Array.isArray(node)) {
    node.forEach((v) => collectRefs(v, out));
    return out;
  }
  if (!isPlainObject(node)) return out;
  for (const [k, v] of Object.entries(node)) {
    if (k === '$ref' && typeof v === 'string') out.push(v);
    else collectRefs(v, out);
  }
  return out;
}

function schemaMaxItems(schema, propPath) {
  let cur = schema;
  for (const p of propPath) {
    cur = cur?.[p];
  }
  return cur;
}

/**
 * Bounded schema-set self-check (not a generic JSON Schema engine).
 */
function runSchemaSetSelfCheck(loadedSchemas) {
  const codes = [];
  const ids = new Map();

  for (const [file, schema] of Object.entries(loadedSchemas)) {
    if (schema.$schema !== DRAFT_2020_12) {
      codes.push(`SCHEMA_DRAFT_INVALID:${file}`);
    }
    if (typeof schema.$id !== 'string' || !schema.$id.trim()) {
      codes.push(`SCHEMA_ID_EMPTY:${file}`);
    } else if (ids.has(schema.$id)) {
      codes.push(`SCHEMA_ID_DUPLICATE:${file}`);
    } else {
      ids.set(schema.$id, file);
    }

    for (const ref of collectRefs(schema)) {
      if (ref.startsWith('http://') || ref.startsWith('https://')) {
        if (!ref.startsWith('https://platform.invalid/schemas/edge/')) {
          // $schema draft URI is not a $ref; only flag unexpected remote $refs.
          codes.push(`UNEXPECTED_REMOTE_REF:${file}:${ref}`);
        }
        continue;
      }
      const [filePart, frag] = ref.split('#');
      const targetFile = filePart || file;
      if (!loadedSchemas[targetFile]) {
        codes.push(`BROKEN_LOCAL_REF:${file}:${ref}`);
        continue;
      }
      if (frag) {
        const parts = frag.replace(/^\//, '').split('/');
        let cur = loadedSchemas[targetFile];
        for (const p of parts) {
          cur = cur?.[p];
        }
        if (cur === undefined) codes.push(`BROKEN_LOCAL_REF_DEF:${file}:${ref}`);
      }
    }
  }

  const common = loadedSchemas['common-definitions.schema.json'];
  if (!common) {
    codes.push('MISSING_COMMON_DEFINITIONS');
    return codes;
  }
  if (common.$defs?.MAX_SAFE_SEQUENCE?.const !== MAX_SAFE_SEQUENCE) {
    codes.push('MAX_SAFE_SEQUENCE_MISMATCH');
  }
  if (common.$defs?.sequenceInteger?.maximum !== MAX_SAFE_SEQUENCE) {
    codes.push('SEQUENCE_INTEGER_MAX_MISMATCH');
  }
  if (common.$defs?.agentDeliveryBatchId?.maxLength !== AGENT_DELIVERY_BATCH_ID_MAX) {
    codes.push('AGENT_DELIVERY_BATCH_ID_MAX_MISMATCH');
  }
  if (common.$defs?.coreBatchId?.maxLength !== CORE_BATCH_ID_MAX) {
    codes.push('CORE_BATCH_ID_MAX_MISMATCH');
  }
  if (common.$defs?.checksumSha256Hex?.pattern !== CHECKSUM_RE.source) {
    codes.push('CHECKSUM_PATTERN_MISMATCH');
  }
  if (common.$defs?.sensorStreamId?.pattern !== STREAM_RE.source) {
    codes.push('STREAM_PATTERN_MISMATCH');
  }
  if (common.$defs?.agentStreamId?.pattern !== STREAM_RE.source) {
    codes.push('AGENT_STREAM_PATTERN_MISMATCH');
  }
  if (common.$defs?.schemaVersionV2?.const !== 2) {
    codes.push('SCHEMA_VERSION_V2_MISMATCH');
  }
  if (common.$defs?.AGENT_MAX_RECORDS?.const !== AGENT_MAX_RECORDS) {
    codes.push('AGENT_MAX_RECORDS_DEF_MISMATCH');
  }
  if (common.$defs?.SENSOR_TELEMETRY_MAX_RECORDS?.const !== SENSOR_TELEMETRY_MAX_RECORDS) {
    codes.push('SENSOR_TELEMETRY_MAX_RECORDS_DEF_MISMATCH');
  }
  if (common.$defs?.AGENT_INGRESS_MAX_RECORDS?.const !== AGENT_INGRESS_MAX_RECORDS) {
    codes.push('AGENT_INGRESS_MAX_RECORDS_DEF_MISMATCH');
  }

  const agentBatch = loadedSchemas['agent-telemetry-batch.schema.json'];
  if (schemaMaxItems(agentBatch, ['properties', 'records', 'maxItems']) !== AGENT_MAX_RECORDS) {
    codes.push('AGENT_BATCH_MAX_ITEMS_MISMATCH');
  }
  if (schemaMaxItems(agentBatch, ['properties', 'recordCount', 'maximum']) !== AGENT_MAX_RECORDS) {
    codes.push('AGENT_BATCH_RECORD_COUNT_MAX_MISMATCH');
  }
  const sensorBatch = loadedSchemas['sensor-telemetry-batch.schema.json'];
  if (schemaMaxItems(sensorBatch, ['properties', 'records', 'maxItems']) !== SENSOR_TELEMETRY_MAX_RECORDS) {
    codes.push('SENSOR_TELEMETRY_MAX_ITEMS_MISMATCH');
  }
  const ingressBatch = loadedSchemas['sensor-agent-ingress-batch.schema.json'];
  if (schemaMaxItems(ingressBatch, ['properties', 'records', 'maxItems']) !== AGENT_INGRESS_MAX_RECORDS) {
    codes.push('AGENT_INGRESS_MAX_ITEMS_MISMATCH');
  }
  if (schemaMaxItems(ingressBatch, ['properties', 'recordCount', 'maximum']) !== AGENT_INGRESS_MAX_RECORDS) {
    codes.push('AGENT_INGRESS_RECORD_COUNT_MAX_MISMATCH');
  }
  const nestedMax = schemaMaxItems(
    ingressBatch,
    ['properties', 'records', 'items', 'properties', 'records', 'maxItems'],
  );
  if (nestedMax !== AGENT_MAX_RECORDS) {
    codes.push('NESTED_AGENT_RECORDS_MAX_ITEMS_MISMATCH');
  }
  const nestedMin = schemaMaxItems(
    ingressBatch,
    ['properties', 'records', 'items', 'properties', 'records', 'minItems'],
  );
  if (nestedMin !== 1) {
    codes.push('NESTED_AGENT_RECORDS_MIN_ITEMS_MISMATCH');
  }
  if (!common.$defs?.agentDeliveryRecord) {
    codes.push('MISSING_AGENT_DELIVERY_RECORD_DEF');
  }
  const agentItemsRef = schemaMaxItems(agentBatch, ['properties', 'records', 'items', '$ref']);
  const nestedItemsRef = schemaMaxItems(
    ingressBatch,
    ['properties', 'records', 'items', 'properties', 'records', 'items', '$ref'],
  );
  const expectedDeliveryRef = 'common-definitions.schema.json#/$defs/agentDeliveryRecord';
  if (agentItemsRef !== expectedDeliveryRef) {
    codes.push('AGENT_BATCH_DELIVERY_RECORD_REF_MISMATCH');
  }
  if (nestedItemsRef !== expectedDeliveryRef) {
    codes.push('INGRESS_NESTED_DELIVERY_RECORD_REF_MISMATCH');
  }

  return codes;
}

function hex64(ch = 'a') {
  return ch.repeat(64);
}

function makeAgentRecords(n, startSeq = 1) {
  const records = [];
  for (let i = 0; i < n; i++) {
    const seq = startSeq + i;
    records.push({
      sequence: seq,
      id: `rec-${seq}`,
      createdAt: '2026-09-24T10:00:00.000Z',
      checksum: hex64('a'),
      payload: {},
    });
  }
  return records;
}

function makeSensorRecords(n, startSeq = 1) {
  const records = [];
  for (let i = 0; i < n; i++) {
    records.push({ sequence: startSeq + i, observationType: 'metric' });
  }
  return records;
}

function makeIngressRecord(seq, nestedCount = 1, nestedStart = 1) {
  const nested = makeAgentRecords(nestedCount, nestedStart);
  return {
    sensorIngressSequence: seq,
    sensorIngressRecordId: `ing-${seq}`,
    tenantId: '11111111-1111-4111-8111-111111111111',
    agentId: 'agent-1',
    acceptingSensorId: 'sensor-1',
    assignmentGeneration: 1,
    agentFirstSequence: nestedStart,
    agentLastSequence: nestedStart + nestedCount - 1,
    agentBatchId: 'agent-batch-1',
    correlationId: 'corr-1',
    acceptedAt: '2026-09-24T10:00:00.000Z',
    records: nested,
  };
}

function runGeneratedBoundaryTests(examples) {
  const results = [];
  const agentBase = deepClone(examples.agentBatch);
  const sensorBase = deepClone(examples.sensorBatch);
  const ingressBase = deepClone(examples.ingressBatch);

  // Agent 100 PASS
  {
    const doc = deepClone(agentBase);
    doc.records = makeAgentRecords(AGENT_MAX_RECORDS);
    doc.firstSequence = 1;
    doc.lastSequence = AGENT_MAX_RECORDS;
    doc.recordCount = AGENT_MAX_RECORDS;
    const codes = validateDocument('agent-telemetry-batch', doc);
    results.push({ name: 'boundary-agent-100-pass', ok: codes.length === 0, codes });
  }
  // Agent 101 FAIL
  {
    const doc = deepClone(agentBase);
    doc.records = makeAgentRecords(AGENT_MAX_RECORDS + 1);
    doc.firstSequence = 1;
    doc.lastSequence = AGENT_MAX_RECORDS + 1;
    doc.recordCount = AGENT_MAX_RECORDS + 1;
    const codes = validateDocument('agent-telemetry-batch', doc);
    results.push({
      name: 'boundary-agent-101-fail',
      ok: codes.includes('RECORD_COUNT_EXCEEDED'),
      codes,
    });
  }
  // Sensor 1000 PASS
  {
    const doc = deepClone(sensorBase);
    doc.records = makeSensorRecords(SENSOR_TELEMETRY_MAX_RECORDS);
    doc.firstSequence = 1;
    doc.lastSequence = SENSOR_TELEMETRY_MAX_RECORDS;
    const codes = validateDocument('sensor-telemetry-batch', doc);
    results.push({ name: 'boundary-sensor-1000-pass', ok: codes.length === 0, codes });
  }
  // Sensor 1001 FAIL
  {
    const doc = deepClone(sensorBase);
    doc.records = makeSensorRecords(SENSOR_TELEMETRY_MAX_RECORDS + 1);
    doc.firstSequence = 1;
    doc.lastSequence = SENSOR_TELEMETRY_MAX_RECORDS + 1;
    const codes = validateDocument('sensor-telemetry-batch', doc);
    results.push({
      name: 'boundary-sensor-1001-fail',
      ok: codes.includes('RECORD_COUNT_EXCEEDED'),
      codes,
    });
  }
  // Ingress 1000 PASS
  {
    const doc = deepClone(ingressBase);
    doc.records = Array.from({ length: AGENT_INGRESS_MAX_RECORDS }, (_, i) => makeIngressRecord(i + 1));
    doc.firstIngressSequence = 1;
    doc.lastIngressSequence = AGENT_INGRESS_MAX_RECORDS;
    doc.recordCount = AGENT_INGRESS_MAX_RECORDS;
    const codes = validateDocument('sensor-agent-ingress-batch', doc);
    results.push({ name: 'boundary-ingress-1000-pass', ok: codes.length === 0, codes });
  }
  // Ingress 1001 FAIL
  {
    const doc = deepClone(ingressBase);
    doc.records = Array.from({ length: AGENT_INGRESS_MAX_RECORDS + 1 }, (_, i) => makeIngressRecord(i + 1));
    doc.firstIngressSequence = 1;
    doc.lastIngressSequence = AGENT_INGRESS_MAX_RECORDS + 1;
    doc.recordCount = AGENT_INGRESS_MAX_RECORDS + 1;
    const codes = validateDocument('sensor-agent-ingress-batch', doc);
    results.push({
      name: 'boundary-ingress-1001-fail',
      ok: codes.includes('RECORD_COUNT_EXCEEDED'),
      codes,
    });
  }
  // Agent batchId 256 PASS / 257 FAIL
  {
    const doc = deepClone(agentBase);
    doc.batchId = 'a'.repeat(AGENT_DELIVERY_BATCH_ID_MAX);
    const codes = validateDocument('agent-telemetry-batch', doc);
    results.push({ name: 'boundary-agent-batchid-256-pass', ok: codes.length === 0, codes });
  }
  {
    const doc = deepClone(agentBase);
    doc.batchId = 'a'.repeat(AGENT_DELIVERY_BATCH_ID_MAX + 1);
    const codes = validateDocument('agent-telemetry-batch', doc);
    results.push({
      name: 'boundary-agent-batchid-257-fail',
      ok: codes.includes('INVALID_BATCH_ID'),
      codes,
    });
  }
  // Core batchId 255 PASS / 256 FAIL
  {
    const doc = deepClone(sensorBase);
    doc.batchId = 'c'.repeat(CORE_BATCH_ID_MAX);
    const codes = validateDocument('sensor-telemetry-batch', doc);
    results.push({ name: 'boundary-core-batchid-255-pass', ok: codes.length === 0, codes });
  }
  {
    const doc = deepClone(sensorBase);
    doc.batchId = 'c'.repeat(CORE_BATCH_ID_MAX + 1);
    const codes = validateDocument('sensor-telemetry-batch', doc);
    results.push({
      name: 'boundary-core-batchid-256-fail',
      ok: codes.includes('INVALID_BATCH_ID'),
      codes,
    });
  }
  // Nested Agent delivery: 100 contiguous PASS / 101 FAIL / empty FAIL
  {
    const doc = deepClone(ingressBase);
    doc.records = [makeIngressRecord(1, AGENT_MAX_RECORDS, 1)];
    doc.firstIngressSequence = 1;
    doc.lastIngressSequence = 1;
    doc.recordCount = 1;
    const codes = validateDocument('sensor-agent-ingress-batch', doc);
    results.push({ name: 'boundary-nested-agent-100-pass', ok: codes.length === 0, codes });
  }
  {
    const doc = deepClone(ingressBase);
    doc.records = [makeIngressRecord(1, AGENT_MAX_RECORDS + 1, 1)];
    doc.firstIngressSequence = 1;
    doc.lastIngressSequence = 1;
    doc.recordCount = 1;
    const codes = validateDocument('sensor-agent-ingress-batch', doc);
    results.push({
      name: 'boundary-nested-agent-101-fail',
      ok: codes.includes('RECORD_COUNT_EXCEEDED'),
      codes,
    });
  }
  {
    const doc = deepClone(ingressBase);
    doc.records = [makeIngressRecord(1, 1, 1)];
    doc.records[0].records = [];
    doc.records[0].agentFirstSequence = 1;
    doc.records[0].agentLastSequence = 1;
    const codes = validateDocument('sensor-agent-ingress-batch', doc);
    results.push({
      name: 'boundary-nested-agent-empty-fail',
      ok: codes.includes('INVALID_CONTRACT') || codes.includes('RECORD_COUNT_MISMATCH'),
      codes,
    });
  }
  return results;
}

/**
 * In-memory negative probes for schema-set self-check (tests 17–19).
 * Does not modify on-disk schemas.
 */
function runSchemaDriftNegativeTests(loadedSchemas) {
  const results = [];

  // 17: broken local $ref
  {
    const clone = deepClone(loadedSchemas);
    clone['agent-telemetry-batch.schema.json'].properties.agentId.$ref =
      'missing-file.schema.json#/$defs/agentId';
    const codes = runSchemaSetSelfCheck(clone);
    results.push({
      name: 'schema-selfcheck-broken-ref-fail',
      ok: codes.some((c) => c.startsWith('BROKEN_LOCAL_REF:')),
      codes,
    });
  }
  // 18: duplicate $id
  {
    const clone = deepClone(loadedSchemas);
    clone['agent-telemetry-ack.schema.json'].$id = clone['agent-telemetry-batch.schema.json'].$id;
    const codes = runSchemaSetSelfCheck(clone);
    results.push({
      name: 'schema-selfcheck-duplicate-id-fail',
      ok: codes.some((c) => c.startsWith('SCHEMA_ID_DUPLICATE:')),
      codes,
    });
  }
  // 19: critical limit drift without validator parity
  {
    const clone = deepClone(loadedSchemas);
    clone['agent-telemetry-batch.schema.json'].properties.records.maxItems = 9999;
    const codes = runSchemaSetSelfCheck(clone);
    results.push({
      name: 'schema-selfcheck-limit-drift-fail',
      ok: codes.includes('AGENT_BATCH_MAX_ITEMS_MISMATCH'),
      codes,
    });
  }
  return results;
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

  function record(name, ok, codes = [], expectedCodes = []) {
    if (ok) passed += 1;
    else {
      failed += 1;
      failures.push({ name, codes, expectedCodes });
    }
  }

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
    const codes = validateDocument(schema, doc, ctx);
    const valid = codes.length === 0;
    const expectValid = testCase.valid === true;
    let ok = valid === expectValid;
    if (!expectValid && testCase.expectedCodes?.length) {
      ok = testCase.expectedCodes.every((c) => codes.includes(c));
    }
    record(testCase.name, ok, codes, testCase.expectedCodes || []);
  }

  // Load and parse every *.schema.json
  const loadedSchemas = {};
  for (const file of listSchemaFiles()) {
    try {
      loadedSchemas[file] = loadJson(file);
    } catch (e) {
      record(`schema-parse-${file}`, false, [`PARSE_ERROR:${e.message}`]);
    }
  }

  const selfCheckCodes = runSchemaSetSelfCheck(loadedSchemas);
  record('schema-set-self-check', selfCheckCodes.length === 0, selfCheckCodes);

  for (const r of runGeneratedBoundaryTests(examples)) {
    record(r.name, r.ok, r.codes);
  }
  for (const r of runSchemaDriftNegativeTests(loadedSchemas)) {
    record(r.name, r.ok, r.codes);
  }

  console.log(`EDGE CONTRACT VALIDATION: ${passed} passed, ${failed} failed`);
  console.log(`MAX_SAFE_SEQUENCE=${MAX_SAFE_SEQUENCE}`);
  console.log(`AGENT_MAX_RECORDS=${AGENT_MAX_RECORDS} SENSOR_TELEMETRY_MAX=${SENSOR_TELEMETRY_MAX_RECORDS} INGRESS_MAX=${AGENT_INGRESS_MAX_RECORDS}`);
  console.log(`AGENT_BATCH_ID_MAX=${AGENT_DELIVERY_BATCH_ID_MAX} CORE_BATCH_ID_MAX=${CORE_BATCH_ID_MAX}`);
  console.log(`MAX_FIXTURE_DOCUMENT_BYTES=${MAX_FIXTURE_DOCUMENT_BYTES} (fixture tool bound; not wire contract)`);
  if (failures.length) {
    for (const f of failures) {
      console.error(`  FAIL ${f.name}: codes=${JSON.stringify(f.codes)} expected=${JSON.stringify(f.expectedCodes || [])}`);
    }
    process.exit(1);
  }
  console.log('EDGE CONTRACT VALIDATION PASSED');
}

main();

#!/usr/bin/env node
/**
 * Platform supportability contract validator (BUILD-005 Slice 1).
 * Dependency-free Node stdlib only. Not a generic JSON Schema engine.
 * Enforces canonical structured-log/health/support-evidence/route-role invariants.
 * Runtime conformance is deferred to later BUILD-005 slices.
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, '..');

const MAX_DOCUMENT_BYTES = 256 * 1024;
const MAX_INPUT_FILE_BYTES = 1024 * 1024;
const MAX_DISCOVERED_DOCUMENTS = 256;
const DRAFT_2020_12 = 'https://json-schema.org/draft/2020-12/schema';
const ID_PREFIX = 'https://platform.invalid/schemas/supportability/';

const TS_Z_RE = /^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?Z$/;
const CORR_RE = /^[A-Za-z0-9._:-]+$/;
const UUID_RE = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const COMPONENT_RE = /^[A-Za-z0-9._:-]+$/;
const EVENT_RE = /^[A-Za-z0-9._:-]+$/;
const SAFE_KEY_RE = /^[A-Za-z][A-Za-z0-9_]*$/;
const DEP_KEY_RE = /^[A-Za-z][A-Za-z0-9._:-]*$/;
const PATH_RE = /^\/[A-Za-z0-9._/-]+$/;

const LOG_LEVELS = new Set(['TRACE', 'DEBUG', 'INFO', 'WARN', 'ERROR', 'FATAL']);
const LIVENESS_STATUS = new Set(['UP', 'SHUTTING_DOWN']);
const READINESS_STATUS = new Set(['UP', 'DEGRADED', 'NOT_READY']);
const COMPONENT_STATES = new Set(['UP', 'DEGRADED', 'NOT_READY', 'SHUTTING_DOWN', 'UNKNOWN']);
const SCHEDULER_STATUS = new Set(['RUNNING', 'IDLE', 'DEGRADED', 'STOPPED', 'UNKNOWN']);
const CONTROL_STATUS = new Set(['IN_SYNC', 'SYNC_PENDING', 'DEGRADED', 'REJECTED', 'UNKNOWN']);
const DELIVERY_STATUS = new Set(['OK', 'BACKLOG', 'BLOCKED', 'FAILED', 'UNKNOWN']);
const DEP_STATUS = new Set(['UP', 'DEGRADED', 'DOWN', 'UNKNOWN']);
const RUNTIMES = new Set(['core', 'sensor', 'agent', 'analytics']);
const CONFORMANCE = new Set(['FOUNDATIONAL', 'BASELINE_LATER', 'PROSPECTIVE']);
const ROUTE_ROLES = new Set(['LIVENESS', 'READINESS', 'DETAILED', 'PUBLIC_OVERALL']);

const LOG_REQUIRED = ['ts', 'level', 'component', 'version', 'service_instance_id', 'message'];
const LOG_ALLOWED = new Set([
  ...LOG_REQUIRED,
  'node_id', 'sensor_id', 'agent_id', 'tenant_id', 'event_code', 'correlation_id',
  'request_id', 'job_id', 'entity_id', 'protocol', 'result', 'retry_count', 'safe_context',
]);
const NON_CANONICAL_LOG_KEYS = new Set(['time', 'timestamp', 'msg', 'service', 'error_code']);

const LIVENESS_ALLOWED = new Set(['status', 'component', 'ts', 'service_instance_id']);
const LIVENESS_FORBIDDEN = new Set([
  'database', 'database_connectivity', 'db', 'core_reachability', 'core', 'storage',
  'storage_pressure', 'storage_pressure_percent', 'queue_depth', 'queue', 'scheduler',
  'scheduler_status', 'capability_pack', 'pack_trust', 'delivery', 'delivery_status',
  'monitored_device', 'monitored_server', 'dependency_status', 'spool_pressure_percent',
]);

const READINESS_ALLOWED = new Set([
  'status', 'ready', 'http_semantic', 'component', 'ts', 'service_instance_id', 'reason_code',
]);

const SUPPORT_ALLOWED = new Set([
  'document_type', 'component', 'version', 'service_instance_id', 'ts', 'uptime_seconds',
  'component_state', 'storage_pressure_percent', 'spool_pressure_percent', 'queue_depth',
  'queue_capacity', 'scheduler_status', 'control_reconciliation_status', 'delivery_status',
  'dependency_status', 'counters', 'gauges', 'reason_codes', 'state_timestamps',
]);
const SUPPORT_PAYLOAD_KEYS = new Set([
  'payload', 'body', 'records', 'telemetry', 'packet', 'packets', 'batch', 'batches',
  'customer_payload', 'raw',
]);

const SECRET_KEYS = new Set([
  'password', 'passwd', 'community', 'snmpcommunity', 'token', 'apitoken', 'apikey',
  'bearertoken', 'bearer', 'privatekey', 'privatekeypem', 'clientsecret', 'authorization',
  'authorizationheader', 'cookie', 'cookies', 'sessioncookie', 'secret', 'secretvalue',
  'accesstoken', 'refreshtoken', 'credential', 'credentials', 'credentialvalue',
]);

const INFRA_REQUIRE_RE = /\b(require|requires|must\s+use|introduce|introduces)\b[\s\S]{0,60}\b(prometheus|opentelemetry|otel\s*collector|metrics?\s*database|time[-\s]?series\s*database|sidecar|external\s+monitoring)\b/i;
const INFRA_ADD_REQUIRE_RE = /\b(require|requires|must)\b[\s\S]{0,40}\badd\b[\s\S]{0,40}\b(prometheus|opentelemetry|otel\s*collector|metrics?\s*database|sidecar)\b/i;

function claimsInfrastructureRequirement(text) {
  if (typeof text !== 'string') return false;
  if (/\b(do not|must not|never|without|no new|not add|not require|not force)\b/i.test(text)) return false;
  return INFRA_REQUIRE_RE.test(text) || INFRA_ADD_REQUIRE_RE.test(text);
}

const SCHEMA_FILES = [
  'common-definitions.schema.json',
  'structured-log-entry.schema.json',
  'liveness.schema.json',
  'readiness.schema.json',
  'support-evidence.schema.json',
  'runtime-probe-routes.schema.json',
];

function isObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function add(issues, code, source, jsonPath, message) {
  issues.push({ code, source, path: jsonPath, message });
}

function token(key) {
  return String(key).toLowerCase().replace(/[^a-z0-9]/g, '');
}

function clone(value) {
  return JSON.parse(JSON.stringify(value));
}

function assertKeys(obj, allowed, issues, source, p) {
  for (const key of Object.keys(obj)) {
    if (!allowed.has(key)) add(issues, 'UNKNOWN_FIELD', source, `${p}.${key}`, `Unknown field: ${key}`);
  }
}

function requireKeys(obj, required, issues, source, p) {
  for (const key of required) {
    if (!(key in obj)) add(issues, 'MISSING_REQUIRED_FIELD', source, `${p}.${key}`, `Missing required field: ${key}`);
  }
}

function assertUtcTs(value, issues, source, p) {
  if (typeof value !== 'string' || value.length < 20 || value.length > 40 || !TS_Z_RE.test(value)) {
    add(issues, 'INVALID_UTC_TIMESTAMP', source, p, 'ts must be UTC RFC3339 with Z');
  }
}

function assertComponent(value, issues, source, p) {
  if (typeof value !== 'string' || value.length < 1 || value.length > 64 || !COMPONENT_RE.test(value)) {
    add(issues, 'INVALID_COMPONENT', source, p, 'component must be bounded opaque identifier');
  }
}

function assertServiceInstanceId(value, issues, source, p) {
  if (typeof value !== 'string' || value.length < 1 || value.length > 128 || !CORR_RE.test(value)) {
    add(issues, 'INVALID_SERVICE_INSTANCE_ID', source, p, 'service_instance_id must be bounded non-secret instance identity');
  }
}

function assertCorrelationId(value, issues, source, p) {
  if (typeof value !== 'string' || value.length < 1 || value.length > 128 || !CORR_RE.test(value)) {
    add(issues, 'INVALID_CORRELATION_ID', source, p, 'correlation_id must match ^[A-Za-z0-9._:-]{1,128}$');
  }
}

function assertEventCode(value, issues, source, p) {
  if (typeof value !== 'string' || value.length < 1 || value.length > 64 || !EVENT_RE.test(value)) {
    add(issues, 'INVALID_EVENT_CODE', source, p, 'event_code must be bounded opaque code <= 64');
  }
}

function assertBoundedId(value, issues, source, p, max = 128) {
  if (typeof value !== 'string' || value.length < 1 || value.length > max || !CORR_RE.test(value)) {
    add(issues, 'INVALID_BOUNDED_ID', source, p, `bounded id invalid (max ${max})`);
  }
}

function assertNoSecrets(obj, issues, source, pathPrefix = '$') {
  if (Array.isArray(obj)) {
    obj.forEach((v, i) => assertNoSecrets(v, issues, source, `${pathPrefix}[${i}]`));
    return;
  }
  if (!isObject(obj)) {
    if (typeof obj === 'string') {
      if (/^\s*-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----/m.test(obj)) {
        add(issues, 'PRIVATE_KEY_MATERIAL', source, pathPrefix, 'PEM private-key material is prohibited');
      }
      if (/^\s*Bearer\s+\S+/i.test(obj) || /^\s*Basic\s+\S+/i.test(obj)) {
        add(issues, 'AUTHORIZATION_MATERIAL', source, pathPrefix, 'Authorization credential material is prohibited');
      }
    }
    return;
  }
  for (const [k, v] of Object.entries(obj)) {
    const normalized = token(k);
    const child = `${pathPrefix}.${k}`;
    if (SECRET_KEYS.has(normalized)) {
      add(issues, 'FORBIDDEN_SECRET_FIELD', source, child, `Secret-bearing key prohibited: ${k}`);
    }
    if (normalized === 'authorization' || normalized === 'authorizationheader') {
      add(issues, 'AUTHORIZATION_MATERIAL', source, child, 'Authorization header/material is prohibited');
    }
    if (normalized.includes('privatekey')) {
      add(issues, 'PRIVATE_KEY_MATERIAL', source, child, 'Private-key field is prohibited');
    }
    assertNoSecrets(v, issues, source, child);
  }
}

function assertSafeContext(value, issues, source, p) {
  if (!isObject(value)) {
    add(issues, 'INVALID_TYPE', source, p, 'safe_context must be an object');
    return;
  }
  const keys = Object.keys(value);
  if (keys.length > 16) add(issues, 'OBJECT_BOUNDS', source, p, 'safe_context exceeds maxProperties=16');
  for (const [k, v] of Object.entries(value)) {
    if (typeof k !== 'string' || k.length < 1 || k.length > 64 || !SAFE_KEY_RE.test(k)) {
      add(issues, 'INVALID_SAFE_CONTEXT_KEY', source, `${p}.${k}`, 'safe_context key bounds violated');
    }
    if (isObject(v) || Array.isArray(v)) {
      add(issues, 'NESTING_PROHIBITED', source, `${p}.${k}`, 'safe_context values must be primitives only');
      continue;
    }
    if (typeof v === 'string' && v.length > 256) {
      add(issues, 'STRING_BOUNDS', source, `${p}.${k}`, 'safe_context string exceeds 256');
    }
    if (!(v === null || typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean')) {
      add(issues, 'INVALID_TYPE', source, `${p}.${k}`, 'safe_context value type unsupported');
    }
  }
  assertNoSecrets(value, issues, source, p);
}

function validateStructuredLog(doc, issues, source) {
  if (!isObject(doc)) {
    add(issues, 'INVALID_TYPE', source, '$', 'structured log must be object');
    return;
  }
  for (const key of Object.keys(doc)) {
    if (NON_CANONICAL_LOG_KEYS.has(key)) {
      add(issues, 'NON_CANONICAL_FIELD', source, `$.${key}`, `Non-canonical log field: ${key}`);
    }
  }
  assertKeys(doc, LOG_ALLOWED, issues, source, '$');
  requireKeys(doc, LOG_REQUIRED, issues, source, '$');

  if ('ts' in doc) assertUtcTs(doc.ts, issues, source, '$.ts');
  if ('level' in doc && (typeof doc.level !== 'string' || !LOG_LEVELS.has(doc.level))) {
    add(issues, 'INVALID_LOG_LEVEL', source, '$.level', 'level must be TRACE|DEBUG|INFO|WARN|ERROR|FATAL');
  }
  if ('component' in doc) assertComponent(doc.component, issues, source, '$.component');
  if ('version' in doc && (typeof doc.version !== 'string' || doc.version.length < 1 || doc.version.length > 64)) {
    add(issues, 'STRING_BOUNDS', source, '$.version', 'version bounds violated');
  }
  if ('service_instance_id' in doc) assertServiceInstanceId(doc.service_instance_id, issues, source, '$.service_instance_id');
  if ('message' in doc) {
    if (typeof doc.message !== 'string' || doc.message.length < 1 || doc.message.length > 1024) {
      add(issues, 'STRING_BOUNDS', source, '$.message', 'message must be 1..1024 characters');
    }
  }
  if ('node_id' in doc) assertBoundedId(doc.node_id, issues, source, '$.node_id');
  if ('sensor_id' in doc) {
    if (typeof doc.sensor_id !== 'string' || doc.sensor_id.length < 1 || doc.sensor_id.length > 255) {
      add(issues, 'INVALID_BOUNDED_ID', source, '$.sensor_id', 'sensor_id bounds violated');
    }
  }
  if ('agent_id' in doc) {
    if (typeof doc.agent_id !== 'string' || doc.agent_id.length < 1 || doc.agent_id.length > 128) {
      add(issues, 'INVALID_BOUNDED_ID', source, '$.agent_id', 'agent_id bounds violated');
    }
  }
  if ('tenant_id' in doc) {
    if (typeof doc.tenant_id !== 'string' || !UUID_RE.test(doc.tenant_id)) {
      add(issues, 'INVALID_TENANT_ID', source, '$.tenant_id', 'tenant_id must be UUID when present');
    }
  }
  if ('event_code' in doc) assertEventCode(doc.event_code, issues, source, '$.event_code');
  if ('correlation_id' in doc) assertCorrelationId(doc.correlation_id, issues, source, '$.correlation_id');
  if ('request_id' in doc) assertBoundedId(doc.request_id, issues, source, '$.request_id');
  if ('job_id' in doc) assertBoundedId(doc.job_id, issues, source, '$.job_id');
  if ('entity_id' in doc) assertBoundedId(doc.entity_id, issues, source, '$.entity_id');
  if ('protocol' in doc) {
    if (typeof doc.protocol !== 'string' || doc.protocol.length < 1 || doc.protocol.length > 32 || !COMPONENT_RE.test(doc.protocol)) {
      add(issues, 'STRING_BOUNDS', source, '$.protocol', 'protocol bounds violated');
    }
  }
  if ('result' in doc) {
    if (typeof doc.result !== 'string' || doc.result.length < 1 || doc.result.length > 32 || !COMPONENT_RE.test(doc.result)) {
      add(issues, 'STRING_BOUNDS', source, '$.result', 'result bounds violated');
    }
  }
  if ('retry_count' in doc) {
    if (typeof doc.retry_count !== 'number' || !Number.isSafeInteger(doc.retry_count) || doc.retry_count < 0) {
      add(issues, 'INVALID_RETRY_COUNT', source, '$.retry_count', 'retry_count must be integer >= 0');
    }
  }
  if ('safe_context' in doc) assertSafeContext(doc.safe_context, issues, source, '$.safe_context');
  assertNoSecrets(doc, issues, source, '$');
}

function validateLiveness(doc, issues, source) {
  if (!isObject(doc)) {
    add(issues, 'INVALID_TYPE', source, '$', 'liveness must be object');
    return;
  }
  for (const key of Object.keys(doc)) {
    if (LIVENESS_FORBIDDEN.has(key) || LIVENESS_FORBIDDEN.has(token(key))) {
      add(issues, 'LIVENESS_DEPENDENCY_PROHIBITED', source, `$.${key}`, 'Liveness must not carry dependency/detail diagnostics');
    }
  }
  assertKeys(doc, LIVENESS_ALLOWED, issues, source, '$');
  requireKeys(doc, ['status', 'component', 'ts'], issues, source, '$');
  if ('status' in doc && (typeof doc.status !== 'string' || !LIVENESS_STATUS.has(doc.status))) {
    add(issues, 'INVALID_LIVENESS_STATUS', source, '$.status', 'status must be UP|SHUTTING_DOWN');
  }
  if ('component' in doc) assertComponent(doc.component, issues, source, '$.component');
  if ('ts' in doc) assertUtcTs(doc.ts, issues, source, '$.ts');
  if ('service_instance_id' in doc) assertServiceInstanceId(doc.service_instance_id, issues, source, '$.service_instance_id');
  assertNoSecrets(doc, issues, source, '$');
}

function validateReadiness(doc, issues, source) {
  if (!isObject(doc)) {
    add(issues, 'INVALID_TYPE', source, '$', 'readiness must be object');
    return;
  }
  assertKeys(doc, READINESS_ALLOWED, issues, source, '$');
  requireKeys(doc, ['status', 'ready', 'http_semantic', 'component', 'ts'], issues, source, '$');
  if ('status' in doc && (typeof doc.status !== 'string' || !READINESS_STATUS.has(doc.status))) {
    add(issues, 'INVALID_READINESS_STATUS', source, '$.status', 'status must be UP|DEGRADED|NOT_READY');
  }
  if ('ready' in doc && typeof doc.ready !== 'boolean') {
    add(issues, 'INVALID_TYPE', source, '$.ready', 'ready must be boolean');
  }
  if ('http_semantic' in doc && doc.http_semantic !== 200 && doc.http_semantic !== 503) {
    add(issues, 'INVALID_HTTP_SEMANTIC', source, '$.http_semantic', 'http_semantic must be 200 or 503');
  }
  if (typeof doc.ready === 'boolean' && (doc.http_semantic === 200 || doc.http_semantic === 503)) {
    if (doc.ready === true && doc.http_semantic !== 200) {
      add(issues, 'READINESS_HTTP_SEMANTIC', source, '$.http_semantic', 'ready=true requires http_semantic=200');
    }
    if (doc.ready === false && doc.http_semantic !== 503) {
      add(issues, 'READINESS_HTTP_SEMANTIC', source, '$.http_semantic', 'ready=false requires http_semantic=503');
    }
  }
  // Explicitly permit DEGRADED + ready=true + 200 (no error).
  if (doc.status === 'DEGRADED' && doc.ready === true && doc.http_semantic === 200) {
    // valid by design — DEGRADED != NOT_READY
  }
  if ('component' in doc) assertComponent(doc.component, issues, source, '$.component');
  if ('ts' in doc) assertUtcTs(doc.ts, issues, source, '$.ts');
  if ('service_instance_id' in doc) assertServiceInstanceId(doc.service_instance_id, issues, source, '$.service_instance_id');
  if ('reason_code' in doc) assertEventCode(doc.reason_code, issues, source, '$.reason_code');
  assertNoSecrets(doc, issues, source, '$');
}

function validateSupportEvidence(doc, issues, source) {
  if (!isObject(doc)) {
    add(issues, 'INVALID_TYPE', source, '$', 'support evidence must be object');
    return;
  }
  for (const key of Object.keys(doc)) {
    if (SUPPORT_PAYLOAD_KEYS.has(key) || SUPPORT_PAYLOAD_KEYS.has(token(key))) {
      add(issues, 'SUPPORT_PAYLOAD_PROHIBITED', source, `$.${key}`, 'Arbitrary payload/body/telemetry data prohibited in support evidence');
    }
  }
  assertKeys(doc, SUPPORT_ALLOWED, issues, source, '$');
  requireKeys(doc, ['document_type', 'component', 'version', 'service_instance_id', 'ts'], issues, source, '$');
  if (doc.document_type !== 'SUPPORT_EVIDENCE') {
    add(issues, 'INVALID_DOCUMENT_TYPE', source, '$.document_type', 'document_type must be SUPPORT_EVIDENCE');
  }
  if ('component' in doc) assertComponent(doc.component, issues, source, '$.component');
  if ('version' in doc && (typeof doc.version !== 'string' || doc.version.length < 1 || doc.version.length > 64)) {
    add(issues, 'STRING_BOUNDS', source, '$.version', 'version bounds violated');
  }
  if ('service_instance_id' in doc) assertServiceInstanceId(doc.service_instance_id, issues, source, '$.service_instance_id');
  if ('ts' in doc) assertUtcTs(doc.ts, issues, source, '$.ts');
  for (const field of ['uptime_seconds', 'queue_depth', 'queue_capacity']) {
    if (field in doc) {
      const v = doc[field];
      if (typeof v !== 'number' || !Number.isSafeInteger(v) || v < 0) {
        add(issues, 'INVALID_COUNTER', source, `$.${field}`, `${field} must be integer >= 0`);
      }
    }
  }
  for (const field of ['storage_pressure_percent', 'spool_pressure_percent']) {
    if (field in doc) {
      const v = doc[field];
      if (typeof v !== 'number' || !Number.isSafeInteger(v) || v < 0 || v > 100) {
        add(issues, 'INVALID_PERCENT', source, `$.${field}`, `${field} must be 0..100`);
      }
    }
  }
  if ('component_state' in doc && !COMPONENT_STATES.has(doc.component_state)) {
    add(issues, 'INVALID_COMPONENT_STATE', source, '$.component_state', 'invalid component_state');
  }
  if ('scheduler_status' in doc && !SCHEDULER_STATUS.has(doc.scheduler_status)) {
    add(issues, 'INVALID_SCHEDULER_STATUS', source, '$.scheduler_status', 'invalid scheduler_status');
  }
  if ('control_reconciliation_status' in doc && !CONTROL_STATUS.has(doc.control_reconciliation_status)) {
    add(issues, 'INVALID_CONTROL_STATUS', source, '$.control_reconciliation_status', 'invalid control_reconciliation_status');
  }
  if ('delivery_status' in doc && !DELIVERY_STATUS.has(doc.delivery_status)) {
    add(issues, 'INVALID_DELIVERY_STATUS', source, '$.delivery_status', 'invalid delivery_status');
  }
  if ('dependency_status' in doc) {
    if (!isObject(doc.dependency_status)) {
      add(issues, 'INVALID_TYPE', source, '$.dependency_status', 'dependency_status must be object');
    } else {
      const keys = Object.keys(doc.dependency_status);
      if (keys.length > 16) add(issues, 'OBJECT_BOUNDS', source, '$.dependency_status', 'dependency_status maxProperties=16');
      for (const [k, v] of Object.entries(doc.dependency_status)) {
        if (!DEP_KEY_RE.test(k) || k.length > 64) add(issues, 'INVALID_SAFE_CONTEXT_KEY', source, `$.dependency_status.${k}`, 'dependency key bounds');
        if (!DEP_STATUS.has(v)) add(issues, 'INVALID_DEPENDENCY_STATUS', source, `$.dependency_status.${k}`, 'invalid dependency status');
        if (isObject(v) || Array.isArray(v)) add(issues, 'NESTING_PROHIBITED', source, `$.dependency_status.${k}`, 'no nested dependency objects');
      }
    }
  }
  if ('counters' in doc) {
    if (!isObject(doc.counters)) add(issues, 'INVALID_TYPE', source, '$.counters', 'counters must be object');
    else {
      if (Object.keys(doc.counters).length > 32) add(issues, 'OBJECT_BOUNDS', source, '$.counters', 'counters maxProperties=32');
      for (const [k, v] of Object.entries(doc.counters)) {
        if (!SAFE_KEY_RE.test(k) || k.length > 64) add(issues, 'INVALID_SAFE_CONTEXT_KEY', source, `$.counters.${k}`, 'counter key bounds');
        if (typeof v !== 'number' || !Number.isSafeInteger(v) || v < 0) {
          add(issues, 'INVALID_COUNTER', source, `$.counters.${k}`, 'counter must be integer >= 0');
        }
      }
    }
  }
  if ('gauges' in doc) {
    if (!isObject(doc.gauges)) add(issues, 'INVALID_TYPE', source, '$.gauges', 'gauges must be object');
    else {
      if (Object.keys(doc.gauges).length > 32) add(issues, 'OBJECT_BOUNDS', source, '$.gauges', 'gauges maxProperties=32');
      for (const [k, v] of Object.entries(doc.gauges)) {
        if (!SAFE_KEY_RE.test(k) || k.length > 64) add(issues, 'INVALID_SAFE_CONTEXT_KEY', source, `$.gauges.${k}`, 'gauge key bounds');
        if (typeof v !== 'number' || !Number.isFinite(v)) add(issues, 'INVALID_GAUGE', source, `$.gauges.${k}`, 'gauge must be finite number');
      }
    }
  }
  if ('reason_codes' in doc) {
    if (!Array.isArray(doc.reason_codes) || doc.reason_codes.length > 16) {
      add(issues, 'ARRAY_BOUNDS', source, '$.reason_codes', 'reason_codes maxItems=16');
    } else {
      doc.reason_codes.forEach((code, i) => assertEventCode(code, issues, source, `$.reason_codes[${i}]`));
    }
  }
  if ('state_timestamps' in doc) {
    if (!isObject(doc.state_timestamps)) add(issues, 'INVALID_TYPE', source, '$.state_timestamps', 'state_timestamps must be object');
    else {
      if (Object.keys(doc.state_timestamps).length > 16) add(issues, 'OBJECT_BOUNDS', source, '$.state_timestamps', 'state_timestamps maxProperties=16');
      for (const [k, v] of Object.entries(doc.state_timestamps)) {
        if (!SAFE_KEY_RE.test(k)) add(issues, 'INVALID_SAFE_CONTEXT_KEY', source, `$.state_timestamps.${k}`, 'timestamp key bounds');
        assertUtcTs(v, issues, source, `$.state_timestamps.${k}`);
      }
    }
  }
  assertNoSecrets(doc, issues, source, '$');
}

function validateProbeRoutes(doc, issues, source) {
  if (!isObject(doc)) {
    add(issues, 'INVALID_TYPE', source, '$', 'runtime-probe-routes must be object');
    return;
  }
  const allowed = new Set(['document_type', 'schema_version', 'runtimes', 'non_goals']);
  assertKeys(doc, allowed, issues, source, '$');
  requireKeys(doc, ['document_type', 'schema_version', 'runtimes'], issues, source, '$');
  if (doc.document_type !== 'RUNTIME_PROBE_ROUTES') {
    add(issues, 'INVALID_DOCUMENT_TYPE', source, '$.document_type', 'document_type must be RUNTIME_PROBE_ROUTES');
  }
  if (doc.schema_version !== '1.0.0') {
    add(issues, 'INVALID_SCHEMA_VERSION', source, '$.schema_version', 'schema_version must be 1.0.0');
  }
  if (!Array.isArray(doc.runtimes) || doc.runtimes.length < 1 || doc.runtimes.length > 8) {
    add(issues, 'ARRAY_BOUNDS', source, '$.runtimes', 'runtimes must be 1..8');
    return;
  }

  const byRuntime = new Map();
  doc.runtimes.forEach((rt, ri) => {
    const rp = `$.runtimes[${ri}]`;
    if (!isObject(rt)) {
      add(issues, 'INVALID_TYPE', source, rp, 'runtime entry must be object');
      return;
    }
    assertKeys(rt, new Set(['runtime', 'conformance', 'routes', 'notes']), issues, source, rp);
    requireKeys(rt, ['runtime', 'conformance', 'routes'], issues, source, rp);
    if (!RUNTIMES.has(rt.runtime)) add(issues, 'UNKNOWN_RUNTIME', source, `${rp}.runtime`, 'unknown runtime');
    if (!CONFORMANCE.has(rt.conformance)) add(issues, 'INVALID_CONFORMANCE', source, `${rp}.conformance`, 'invalid conformance');
    if (typeof rt.notes === 'string' && rt.notes.length > 512) add(issues, 'STRING_BOUNDS', source, `${rp}.notes`, 'notes too long');
    if (!Array.isArray(rt.routes) || rt.routes.length < 1 || rt.routes.length > 16) {
      add(issues, 'ARRAY_BOUNDS', source, `${rp}.routes`, 'routes must be 1..16');
      return;
    }
    byRuntime.set(rt.runtime, rt);
    rt.routes.forEach((route, ji) => {
      const jp = `${rp}.routes[${ji}]`;
      if (!isObject(route)) {
        add(issues, 'INVALID_TYPE', source, jp, 'route must be object');
        return;
      }
      assertKeys(route, new Set(['path', 'role', 'compatibility', 'notes']), issues, source, jp);
      requireKeys(route, ['path', 'role', 'compatibility'], issues, source, jp);
      if (typeof route.path !== 'string' || !PATH_RE.test(route.path) || route.path.length > 128) {
        add(issues, 'INVALID_ROUTE_PATH', source, `${jp}.path`, 'invalid route path');
      }
      if (!ROUTE_ROLES.has(route.role)) add(issues, 'INVALID_ROUTE_ROLE', source, `${jp}.role`, 'invalid role');
      if (typeof route.compatibility !== 'boolean') add(issues, 'INVALID_TYPE', source, `${jp}.compatibility`, 'compatibility must be boolean');
      if (typeof route.notes === 'string' && route.notes.length > 256) add(issues, 'STRING_BOUNDS', source, `${jp}.notes`, 'notes too long');
      if (route.path === '/api/v1/health/detailed' || (rt.runtime === 'core' && route.role === 'DETAILED')) {
        add(issues, 'CORE_DETAILED_API_PROHIBITED', source, jp, 'Core detailed health API is out of BUILD-005 scope');
      }
    });
  });

  const sensor = byRuntime.get('sensor');
  if (sensor) {
    const healthz = sensor.routes.find((r) => r.path === '/healthz');
    if (!healthz || healthz.role !== 'DETAILED' || healthz.compatibility !== true) {
      add(issues, 'ROUTE_ROLE_INVARIANT', source, '$.runtimes[sensor]/healthz', 'Sensor /healthz must remain compatibility DETAILED, not readiness');
    }
    const livez = sensor.routes.find((r) => r.path === '/livez');
    const readyz = sensor.routes.find((r) => r.path === '/readyz');
    if (!livez || livez.role !== 'LIVENESS') add(issues, 'ROUTE_ROLE_INVARIANT', source, '$.runtimes[sensor]/livez', 'Sensor /livez must be LIVENESS');
    if (!readyz || readyz.role !== 'READINESS') add(issues, 'ROUTE_ROLE_INVARIANT', source, '$.runtimes[sensor]/readyz', 'Sensor /readyz must be READINESS');
  }

  const agent = byRuntime.get('agent');
  if (agent) {
    const healthz = agent.routes.find((r) => r.path === '/healthz');
    if (!healthz || healthz.role !== 'READINESS' || healthz.compatibility !== true) {
      add(issues, 'ROUTE_ROLE_INVARIANT', source, '$.runtimes[agent]/healthz', 'Agent /healthz must remain compatibility READINESS, not detailed');
    }
    const live = agent.routes.find((r) => r.path === '/live');
    const ready = agent.routes.find((r) => r.path === '/ready');
    if (!live || live.role !== 'LIVENESS') add(issues, 'ROUTE_ROLE_INVARIANT', source, '$.runtimes[agent]/live', 'Agent /live must be LIVENESS');
    if (!ready || ready.role !== 'READINESS') add(issues, 'ROUTE_ROLE_INVARIANT', source, '$.runtimes[agent]/ready', 'Agent /ready must be READINESS');
  }

  const core = byRuntime.get('core');
  if (core) {
    const paths = new Set(core.routes.map((r) => r.path));
    for (const required of ['/actuator/health/liveness', '/actuator/health/readiness', '/actuator/health']) {
      if (!paths.has(required)) add(issues, 'ROUTE_ROLE_INVARIANT', source, '$.runtimes[core]', `Core missing required probe path ${required}`);
    }
  }

  // Prove Sensor and Agent /healthz semantics differ when both present.
  if (sensor && agent) {
    const s = sensor.routes.find((r) => r.path === '/healthz');
    const a = agent.routes.find((r) => r.path === '/healthz');
    if (s && a && s.role === a.role) {
      add(issues, 'ROUTE_ROLE_INVARIANT', source, '$.runtimes', 'Sensor and Agent /healthz must retain different compatibility semantics');
    }
  }

  if ('non_goals' in doc) {
    if (!Array.isArray(doc.non_goals) || doc.non_goals.length > 16) {
      add(issues, 'ARRAY_BOUNDS', source, '$.non_goals', 'non_goals maxItems=16');
    } else {
      doc.non_goals.forEach((item, i) => {
        if (typeof item !== 'string' || item.length > 256) {
          add(issues, 'STRING_BOUNDS', source, `$.non_goals[${i}]`, 'non_goal string bounds');
          return;
        }
        if (claimsInfrastructureRequirement(item)) {
          add(issues, 'INFRASTRUCTURE_NON_GOAL', source, `$.non_goals[${i}]`, 'Must not introduce Prometheus/OTel/metrics-DB/sidecar infrastructure');
        }
      });
    }
  }

  const serialized = JSON.stringify(doc);
  void serialized;
  for (const rt of doc.runtimes) {
    if (typeof rt.notes === 'string' && claimsInfrastructureRequirement(rt.notes)) {
      add(issues, 'INFRASTRUCTURE_NON_GOAL', source, '$.runtimes', 'Runtime notes must not require external metrics infrastructure');
    }
  }

  assertNoSecrets(doc, issues, source, '$');
}

function validateDocument(kind, document, source = '<memory>') {
  const issues = [];
  switch (kind) {
    case 'structured-log':
      validateStructuredLog(document, issues, source);
      break;
    case 'liveness':
      validateLiveness(document, issues, source);
      break;
    case 'readiness':
      validateReadiness(document, issues, source);
      break;
    case 'support-evidence':
      validateSupportEvidence(document, issues, source);
      break;
    case 'runtime-probe-routes':
      validateProbeRoutes(document, issues, source);
      break;
    default:
      add(issues, 'UNKNOWN_KIND', source, '$', `Unknown document kind: ${kind}`);
  }
  return issues;
}

function parseJsonFile(filePath) {
  const stat = fs.statSync(filePath);
  if (stat.size > MAX_INPUT_FILE_BYTES) throw new Error(`Input exceeds ${MAX_INPUT_FILE_BYTES} bytes: ${filePath}`);
  const text = fs.readFileSync(filePath, 'utf8');
  if (Buffer.byteLength(text, 'utf8') > MAX_DOCUMENT_BYTES && !filePath.endsWith('cases.json')) {
    throw new Error(`Document exceeds ${MAX_DOCUMENT_BYTES} bytes: ${filePath}`);
  }
  return JSON.parse(text);
}

function collectLocalRefs(node, refs, file) {
  if (Array.isArray(node)) {
    node.forEach((v) => collectLocalRefs(v, refs, file));
    return;
  }
  if (!isObject(node)) return;
  for (const [k, v] of Object.entries(node)) {
    if (k === '$ref' && typeof v === 'string') refs.push({ file, ref: v });
    else collectLocalRefs(v, refs, file);
  }
}

function resolveRefTarget(schemaRoot, ref) {
  if (!ref.startsWith('#/')) return null;
  const parts = ref.slice(2).split('/');
  let cur = schemaRoot;
  for (const part of parts) {
    if (!isObject(cur) || !(part in cur)) return undefined;
    cur = cur[part];
  }
  return cur;
}

function selfCheckSchemas() {
  const issues = [];
  const ids = new Set();
  const schemas = new Map();

  for (const name of SCHEMA_FILES) {
    const full = path.join(ROOT, name);
    if (!fs.existsSync(full)) {
      add(issues, 'MISSING_SCHEMA', name, '$', `Missing schema file: ${name}`);
      continue;
    }
    let doc;
    try {
      doc = parseJsonFile(full);
    } catch (error) {
      add(issues, 'JSON_PARSE_ERROR', name, '$', error.message);
      continue;
    }
    if (doc.$schema !== DRAFT_2020_12) {
      add(issues, 'INVALID_SCHEMA_DRAFT', name, '$.$schema', 'Schemas must use JSON Schema Draft 2020-12');
    }
    if (typeof doc.$id !== 'string' || !doc.$id.startsWith(ID_PREFIX)) {
      add(issues, 'INVALID_SCHEMA_ID', name, '$.$id', `/$id must start with ${ID_PREFIX}`);
    } else if (ids.has(doc.$id)) {
      add(issues, 'DUPLICATE_SCHEMA_ID', name, '$.$id', `Duplicate $id: ${doc.$id}`);
    } else {
      ids.add(doc.$id);
    }
    schemas.set(name, doc);
  }

  for (const [name, doc] of schemas.entries()) {
    const refs = [];
    collectLocalRefs(doc, refs, name);
    for (const { ref } of refs) {
      if (ref.startsWith('#')) {
        if (resolveRefTarget(doc, ref) === undefined) {
          add(issues, 'BROKEN_LOCAL_REF', name, '$ref', `Broken local $ref: ${ref}`);
        }
      } else if (ref.includes('#')) {
        const [filePart, frag] = ref.split('#');
        const targetFile = path.basename(filePart);
        if (!schemas.has(targetFile)) {
          add(issues, 'BROKEN_LOCAL_REF', name, '$ref', `Unknown local schema ref file: ${ref}`);
        } else if (resolveRefTarget(schemas.get(targetFile), `#${frag}`) === undefined) {
          add(issues, 'BROKEN_LOCAL_REF', name, '$ref', `Broken cross-file local $ref: ${ref}`);
        }
      } else if (!ref.startsWith('https://json-schema.org/')) {
        add(issues, 'NON_LOCAL_REF', name, '$ref', `Only local $ref allowed except standard $schema: ${ref}`);
      }
    }
  }

  return issues;
}

function printIssues(issues) {
  const sorted = [...issues].sort((a, b) => `${a.source}|${a.path}|${a.code}`.localeCompare(`${b.source}|${b.path}|${b.code}`));
  for (const issue of sorted) console.error(`${issue.code}\t${issue.source}\t${issue.path}\t${issue.message}`);
}

function getAt(root, segments) {
  let current = root;
  for (const segment of segments) current = current[segment];
  return current;
}

function parentAt(root, segments) {
  if (!Array.isArray(segments) || segments.length === 0) throw new Error('Mutation path must be non-empty.');
  let current = root;
  for (let i = 0; i < segments.length - 1; i += 1) current = current[segments[i]];
  return { parent: current, key: segments[segments.length - 1] };
}

function applyMutation(document, mutation) {
  if (!isObject(mutation) || !Array.isArray(mutation.path)) throw new Error('Fixture mutation requires path[].');
  if (mutation.op === 'set') {
    const { parent, key } = parentAt(document, mutation.path);
    parent[key] = clone(mutation.value);
  } else if (mutation.op === 'delete') {
    const { parent, key } = parentAt(document, mutation.path);
    delete parent[key];
  } else if (mutation.op === 'repeat') {
    const { parent, key } = parentAt(document, mutation.path);
    if (typeof mutation.value !== 'string' || !Number.isSafeInteger(mutation.count) || mutation.count < 0 || mutation.count > 100000) {
      throw new Error('Invalid repeat mutation.');
    }
    parent[key] = mutation.value.repeat(mutation.count);
  } else if (mutation.op === 'append') {
    const target = getAt(document, mutation.path);
    if (!Array.isArray(target)) throw new Error('append mutation requires array path');
    target.push(clone(mutation.value));
  } else {
    throw new Error(`Unknown fixture mutation op: ${String(mutation.op)}`);
  }
}

function loadFixtureBases(suite) {
  if (!isObject(suite.examples)) throw new Error('fixtures/cases.json requires examples object.');
  const bases = new Map();
  for (const [name, relative] of Object.entries(suite.examples)) {
    if (typeof relative !== 'string') throw new Error(`Fixture example path for ${name} must be a string.`);
    bases.set(name, parseJsonFile(path.join(ROOT, relative)));
  }
  return bases;
}

function runFixtureSuite() {
  const schemaIssues = selfCheckSchemas();
  if (schemaIssues.length > 0) {
    console.error('SCHEMA SELF-CHECK FAILURE');
    printIssues(schemaIssues);
    return 1;
  }

  const suite = parseJsonFile(path.join(ROOT, 'fixtures', 'cases.json'));
  const bases = loadFixtureBases(suite);
  let failures = 0;

  // Validate example bases with inferred kinds from case definitions / filename heuristics.
  const exampleKind = {
    'log-core': 'structured-log',
    'log-sensor': 'structured-log',
    'log-agent': 'structured-log',
    'log-analytics': 'structured-log',
    'liveness-up': 'liveness',
    'liveness-shutting-down': 'liveness',
    'readiness-up': 'readiness',
    'readiness-degraded-ready': 'readiness',
    'readiness-degraded-not-ready': 'readiness',
    'support-evidence': 'support-evidence',
    'runtime-probe-routes': 'runtime-probe-routes',
  };

  for (const [name, document] of bases.entries()) {
    const kind = exampleKind[name];
    if (!kind) {
      console.error(`EXAMPLE VALIDATION FAILURE: unknown example kind for ${name}`);
      failures += 1;
      continue;
    }
    const issues = validateDocument(kind, document, `example:${name}`);
    if (issues.length > 0) {
      console.error(`EXAMPLE VALIDATION FAILURE: ${name}`);
      printIssues(issues);
      failures += 1;
    }
  }

  if (!Array.isArray(suite.cases)) throw new Error('fixtures/cases.json requires cases[].');
  for (const fixture of suite.cases) {
    if (!isObject(fixture) || typeof fixture.name !== 'string' || typeof fixture.base !== 'string' || typeof fixture.kind !== 'string') {
      throw new Error('Each fixture requires name, base, and kind.');
    }
    if (!bases.has(fixture.base)) throw new Error(`Unknown fixture base: ${fixture.base}`);
    const document = clone(bases.get(fixture.base));
    if (Array.isArray(fixture.mutations)) fixture.mutations.forEach((mutation) => applyMutation(document, mutation));

    const issues = validateDocument(fixture.kind, document, `fixture:${fixture.name}`);
    const actualValid = issues.length === 0;
    const expectedValid = fixture.valid === true;
    const codes = new Set(issues.map((issue) => issue.code));
    const expectedCodes = Array.isArray(fixture.expectedCodes) ? fixture.expectedCodes : [];
    const missingCodes = expectedCodes.filter((code) => !codes.has(code));

    if (actualValid !== expectedValid || missingCodes.length > 0) {
      console.error(`FIXTURE FAILURE: ${fixture.name}`);
      console.error(`  expected valid=${expectedValid}, actual valid=${actualValid}`);
      if (missingCodes.length > 0) console.error(`  missing expected codes: ${missingCodes.join(', ')}`);
      printIssues(issues);
      failures += 1;
    }
  }

  if (failures > 0) {
    console.error(`FAILED: ${failures} fixture/example validation group(s).`);
    return 1;
  }
  console.log(`SUPPORTABILITY CONTRACT OK: ${SCHEMA_FILES.length} schema(s), ${suite.cases.length} fixture case(s), ${bases.size} example(s).`);
  console.log('Invariants: DEGRADED!=NOT_READY; Sensor/Agent /healthz semantics differ; no Core detailed API; no Prometheus/OTel infrastructure.');
  return 0;
}

function main() {
  try {
    process.exitCode = runFixtureSuite();
  } catch (error) {
    console.error(`VALIDATOR_ERROR\t${error.message}`);
    process.exitCode = 2;
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) main();

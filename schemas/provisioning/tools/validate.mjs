#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, '..');
const CONTRACT_VERSION = '1.0.0';

// Defensive parser/input-safety bounds only. They are not operational/customer defaults.
const MAX_DOCUMENT_BYTES = 256 * 1024;
const MAX_INPUT_FILE_BYTES = 1024 * 1024;
const MAX_DISCOVERED_DOCUMENTS = 256;
const MAX_EVIDENCE = 32;
const MAX_NEXT_ACTIONS = 16;
const MAX_SENSOR_CANDIDATES = 64;
const MAX_MATRIX_ROWS = 512;

const REF_RE = /^[A-Za-z0-9._:-]+$/;
const SAFE_TYPE_RE = /^[A-Za-z][A-Za-z0-9._:-]*$/;

const DOCUMENT_TYPES = new Set(['PROVISIONING_REQUEST', 'READINESS_RESULT', 'CONNECTIVITY_MATRIX']);
const PROTOCOLS = new Set([
  'ICMP', 'TRACEROUTE', 'TCP', 'SNMP', 'SNMP_TRAP', 'SYSLOG', 'SSH_CLI', 'TELNET',
  'NETCONF', 'REST', 'RESTCONF', 'GNMI', 'GRPC', 'FLOW', 'DNS', 'HTTP', 'HTTPS', 'TLS',
  'SCP', 'SFTP', 'TFTP', 'FTP', 'WINRM', 'REDFISH', 'CLOUD_API', 'OTLP',
]);
const TRANSPORTS = new Set(['IP', 'TCP', 'UDP', 'TLS', 'HTTPS', 'SSH', 'GRPC', 'MTLS']);
const NO_PORT_PROTOCOLS = new Set(['ICMP', 'TRACEROUTE']);

const STAGES = [
  'INPUT_NORMALIZATION',
  'DNS_ADDRESS_VALIDATION',
  'SENSOR_ELIGIBILITY_CAPACITY',
  'ROUTE_REACHABILITY',
  'TRANSPORT_PROTOCOL_RESPONSE',
  'TLS_HOST_KEY_VERIFICATION',
  'AUTHENTICATION',
  'AUTHORIZATION_CAPABILITY',
  'IDENTITY_DEVICE_PACK_MATCH',
  'MONITORING_PROFILE_DRY_RUN',
  'CAPACITY_IMPACT',
  'PROVISION',
];
const STAGE_SET = new Set(STAGES);
const STATUSES = new Set(['NOT_RUN', 'RUNNING', 'PASS', 'FAIL', 'INCONCLUSIVE', 'UNKNOWN', 'SKIPPED_NOT_APPLICABLE']);

const FAILURE_CODES = new Set([
  'INPUT_INVALID', 'DEVICE_UNREACHABLE', 'DNS_FAILURE', 'ROUTE_UNAVAILABLE', 'PROTOCOL_TIMEOUT',
  'PROTOCOL_DISABLED_SUSPECTED', 'AUTHENTICATION_FAILED', 'AUTHORIZATION_FAILED',
  'ACL_FIREWALL_SUSPECTED', 'PORT_UNREACHABLE', 'CREDENTIAL_PROFILE_INVALID',
  'DEVICE_ALREADY_EXISTS', 'MIB_REQUIRED', 'API_SCHEMA_INVALID', 'TLS_CERTIFICATE_FAILURE',
  'SENSOR_CAPACITY_EXCEEDED', 'NO_ELIGIBLE_SENSOR', 'MONITORING_PROFILE_INCOMPATIBLE',
  'DEVICE_IDENTITY_CONFLICT',
]);

// Conservative stage association. DEVICE_UNREACHABLE is deliberately restricted to route/reachability;
// a transport/protocol timeout alone must not silently become a device-down/unreachable conclusion.
const FAILURE_STAGE_MAP = new Map([
  ['INPUT_INVALID', new Set(['INPUT_NORMALIZATION', 'DNS_ADDRESS_VALIDATION'])],
  ['DEVICE_UNREACHABLE', new Set(['ROUTE_REACHABILITY'])],
  ['DNS_FAILURE', new Set(['DNS_ADDRESS_VALIDATION'])],
  ['ROUTE_UNAVAILABLE', new Set(['ROUTE_REACHABILITY'])],
  ['PROTOCOL_TIMEOUT', new Set(['TRANSPORT_PROTOCOL_RESPONSE'])],
  ['PROTOCOL_DISABLED_SUSPECTED', new Set(['TRANSPORT_PROTOCOL_RESPONSE'])],
  ['AUTHENTICATION_FAILED', new Set(['AUTHENTICATION'])],
  ['AUTHORIZATION_FAILED', new Set(['AUTHORIZATION_CAPABILITY'])],
  ['ACL_FIREWALL_SUSPECTED', new Set(['ROUTE_REACHABILITY', 'TRANSPORT_PROTOCOL_RESPONSE'])],
  ['PORT_UNREACHABLE', new Set(['TRANSPORT_PROTOCOL_RESPONSE'])],
  ['CREDENTIAL_PROFILE_INVALID', new Set(['INPUT_NORMALIZATION', 'AUTHENTICATION'])],
  ['DEVICE_ALREADY_EXISTS', new Set(['IDENTITY_DEVICE_PACK_MATCH', 'PROVISION'])],
  ['MIB_REQUIRED', new Set(['IDENTITY_DEVICE_PACK_MATCH', 'MONITORING_PROFILE_DRY_RUN'])],
  ['API_SCHEMA_INVALID', new Set(['IDENTITY_DEVICE_PACK_MATCH', 'MONITORING_PROFILE_DRY_RUN'])],
  ['TLS_CERTIFICATE_FAILURE', new Set(['TLS_HOST_KEY_VERIFICATION'])],
  ['SENSOR_CAPACITY_EXCEEDED', new Set(['SENSOR_ELIGIBILITY_CAPACITY', 'CAPACITY_IMPACT'])],
  ['NO_ELIGIBLE_SENSOR', new Set(['SENSOR_ELIGIBILITY_CAPACITY'])],
  ['MONITORING_PROFILE_INCOMPATIBLE', new Set(['MONITORING_PROFILE_DRY_RUN'])],
  ['DEVICE_IDENTITY_CONFLICT', new Set(['IDENTITY_DEVICE_PACK_MATCH', 'PROVISION'])],
]);

const EVIDENCE_KINDS = new Set(['OBSERVED', 'DERIVED', 'SUSPECTED', 'UNKNOWN']);
const EVIDENCE_SOURCES = new Set([
  'REQUEST', 'CORE', 'SENSOR', 'PLATFORM_POLICY', 'MONITORING_PROFILE', 'DEVICE_PACK',
  'CONNECTOR', 'OTHER_GOVERNED_SOURCE',
]);
const SENSOR_CHECK_CATEGORIES = new Set([
  'SITE_POOL_COMPATIBILITY', 'CAPABILITY_PROTOCOL_SUPPORT', 'REACHABILITY', 'LATENCY_LOSS',
  'CAPACITY', 'SECURITY_POLICY', 'MAINTENANCE_DRAIN', 'HEALTH_READINESS',
]);
const MATRIX_SOURCE_ROLES = new Set(['SENSOR', 'CORE', 'AGENT']);
const DIRECTIONS = new Set(['OUTBOUND', 'INBOUND', 'BIDIRECTIONAL']);
const REQUIREMENT_SOURCE_TYPES = new Set([
  'MONITORING_PROFILE', 'DEVICE_PACK', 'PLATFORM_SERVICE', 'CONNECTOR', 'OTHER_GOVERNED_SOURCE',
]);

const FORBIDDEN_SECRET_KEYS = new Set([
  'password', 'passwordhash', 'passwd', 'community', 'privatekey', 'token', 'bearer', 'apikey',
  'clientsecret', 'cookie', 'cookies', 'sessioncookie', 'secret', 'secretvalue', 'credentialvalue',
  'accesstoken', 'refreshtoken',
]);
const FORBIDDEN_EXECUTABLE_KEYS = new Set([
  'script', 'scripts', 'shell', 'command', 'commands', 'exec', 'executable', 'javascript',
  'python', 'code', 'inlinecode',
]);

const REQUEST_KEYS = new Set([
  'documentType', 'schemaVersion', 'requestId', 'correlationId', 'tenantId', 'target', 'siteId',
  'sensorPoolId', 'monitoringProfileRef', 'devicePackRef', 'devicePackVersion', 'credentialProfileRef',
  'idempotencyKey', 'requestedProtocols', 'tags',
]);
const TARGET_KEYS = new Set(['targetType', 'address', 'displayName', 'description']);
const REQUEST_PROTOCOL_KEYS = new Set(['family', 'transport', 'port', 'tlsRequired', 'securityMode']);
const TAG_KEYS = new Set(['key', 'value']);

const READINESS_KEYS = new Set([
  'documentType', 'schemaVersion', 'requestId', 'correlationId', 'tenantId', 'target',
  'sensorEligibility', 'stages',
]);
const READINESS_TARGET_KEYS = new Set(['targetType', 'address']);
const SENSOR_ELIGIBILITY_KEYS = new Set(['candidates']);
const SENSOR_CANDIDATE_KEYS = new Set(['sensorId', 'siteId', 'poolId', 'selected', 'reason', 'checks']);
const SENSOR_CHECK_KEYS = new Set(['category', 'status', 'evidence']);
const STAGE_KEYS = new Set([
  'stageId', 'status', 'startedAt', 'completedAt', 'durationMs', 'sensorId', 'endpoint', 'protocol',
  'transport', 'evidence', 'customerExplanation', 'failureCode', 'failureAssessment', 'uncertainty',
  'diagnosticsRef', 'nextActions',
]);
const ENDPOINT_KEYS = new Set(['destination', 'port']);
const EVIDENCE_KEYS = new Set([
  'evidenceId', 'kind', 'source', 'summary', 'sourceRef', 'endpointRef', 'technicalCode',
  'protocol', 'transport', 'observedAt',
]);
const EXECUTION_CONTEXT_KEYS = ['startedAt', 'completedAt', 'durationMs', 'sensorId', 'endpoint'];

const MATRIX_KEYS = new Set(['documentType', 'schemaVersion', 'requestId', 'correlationId', 'tenantId', 'rows']);
const MATRIX_ROW_KEYS = new Set([
  'rowId', 'source', 'destination', 'protocol', 'transport', 'port', 'direction', 'purpose',
  'tlsRequired', 'securityMode', 'requirementSource', 'validationStatus', 'evidence', 'observedAt',
]);
const MATRIX_SOURCE_KEYS = new Set(['role', 'identityRef', 'siteId', 'poolId', 'scope']);
const MATRIX_DESTINATION_KEYS = new Set(['address', 'scope']);
const MATRIX_REQUIREMENT_SOURCE_KEYS = new Set(['type', 'reference']);

function isObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function add(issues, code, source, jsonPath, message) {
  issues.push({ code, source, path: jsonPath, message });
}

function stableIssues(issues) {
  return [...issues].sort((a, b) => `${a.source}|${a.path}|${a.code}`.localeCompare(`${b.source}|${b.path}|${b.code}`));
}

function keyToken(key) {
  return key.toLowerCase().replace(/[_\-.]/g, '');
}

function assertKeys(object, allowed, issues, source, jsonPath) {
  if (!isObject(object)) return;
  for (const key of Object.keys(object)) {
    if (!allowed.has(key)) add(issues, 'UNKNOWN_FIELD', source, `${jsonPath}.${key}`, 'Field is not defined by this provisioning contract.');
  }
}

function requireKeys(object, required, issues, source, jsonPath) {
  if (!isObject(object)) return;
  for (const key of required) {
    if (!(key in object)) add(issues, 'MISSING_REQUIRED_FIELD', source, `${jsonPath}.${key}`, 'Required field is missing.');
  }
}

function safeString(value, min, max, issues, source, jsonPath) {
  if (typeof value !== 'string') {
    add(issues, 'INVALID_TYPE', source, jsonPath, 'Expected string.');
    return false;
  }
  if (value.length < min || value.length > max) {
    add(issues, 'STRING_BOUNDS', source, jsonPath, `String length must be ${min}-${max}.`);
    return false;
  }
  if (/\p{Cc}/u.test(value)) add(issues, 'CONTROL_CHARACTER', source, jsonPath, 'Control characters are not allowed.');
  return true;
}

function ref128(value, issues, source, jsonPath) {
  if (!safeString(value, 1, 128, issues, source, jsonPath)) return false;
  if (!REF_RE.test(value)) {
    add(issues, 'INVALID_REFERENCE', source, jsonPath, 'Reference must use the current bounded opaque identifier grammar.');
    return false;
  }
  return true;
}

function arrayValue(value, min, max, issues, source, jsonPath) {
  if (!Array.isArray(value)) {
    add(issues, 'INVALID_TYPE', source, jsonPath, 'Expected array.');
    return false;
  }
  if (value.length < min || value.length > max) add(issues, 'ARRAY_BOUNDS', source, jsonPath, `Array length must be ${min}-${max}.`);
  return true;
}

function enumValue(value, allowed, code, issues, source, jsonPath) {
  if (!allowed.has(value)) {
    add(issues, code, source, jsonPath, `Unsupported value: ${String(value)}.`);
    return false;
  }
  return true;
}

function safeInteger(value, min, max, code, issues, source, jsonPath) {
  if (!Number.isSafeInteger(value) || value < min || (max !== null && value > max)) {
    add(issues, code, source, jsonPath, max === null ? `Expected safe integer >= ${min}.` : `Expected safe integer ${min}-${max}.`);
    return false;
  }
  return true;
}

function isoTimestamp(value, issues, source, jsonPath) {
  if (!safeString(value, 1, 64, issues, source, jsonPath)) return false;
  if (Number.isNaN(Date.parse(value))) {
    add(issues, 'INVALID_TIMESTAMP', source, jsonPath, 'Timestamp must be parseable ISO-8601/date-time text.');
    return false;
  }
  return true;
}

function scanForbidden(value, issues, source, jsonPath = '$') {
  if (Array.isArray(value)) {
    value.forEach((item, index) => scanForbidden(item, issues, source, `${jsonPath}[${index}]`));
    return;
  }
  if (!isObject(value)) return;

  for (const [key, child] of Object.entries(value)) {
    const token = keyToken(key);
    const childPath = `${jsonPath}.${key}`;
    if (FORBIDDEN_SECRET_KEYS.has(token)) add(issues, 'FORBIDDEN_SECRET_FIELD', source, childPath, 'Secret-bearing fields are prohibited; use Credential Profile references only.');
    if (FORBIDDEN_EXECUTABLE_KEYS.has(token)) add(issues, 'FORBIDDEN_EXECUTABLE_FIELD', source, childPath, 'Executable/script/command content is prohibited.');

    if (typeof child === 'string') {
      if (/-----BEGIN [A-Z ]*PRIVATE KEY-----/.test(child)) add(issues, 'FORBIDDEN_SECRET_VALUE', source, childPath, 'Private-key material is prohibited.');
      if (/:\/\/[^/\s:@]+:[^/\s@]+@/.test(child)) add(issues, 'EMBEDDED_URL_CREDENTIAL', source, childPath, 'Credentials embedded in URLs are prohibited.');
      if (/^Bearer\s+\S+/i.test(child) || /[?&](?:token|api[_-]?key|password|secret|access[_-]?token|bearer)=/i.test(child)) add(issues, 'FORBIDDEN_SECRET_VALUE', source, childPath, 'Token/secret material is prohibited.');
      if (/(?:^|[\\/])\.\.(?:[\\/]|$)|%2e%2e|%2f\.\.|%5c\.\./i.test(child)) add(issues, 'PATH_TRAVERSAL', source, childPath, 'Path traversal is prohibited.');
      if (/^(?:\/|[A-Za-z]:[\\/]|\\\\)/.test(child) && !/^[A-Za-z][A-Za-z0-9+.-]*:\/\//.test(child)) add(issues, 'ABSOLUTE_FILESYSTEM_PATH', source, childPath, 'Absolute filesystem references are prohibited.');
      if (/<\s*(?:script|iframe|object|embed|svg)\b|\bon[a-z]+\s*=|javascript\s*:/i.test(child)) add(issues, 'UNSAFE_HTML', source, childPath, 'Executable/active HTML content is prohibited in diagnostic text.');
      if (/\b(?:firewall|acl)\b.{0,24}\bblocked\b|\bblocked\b.{0,24}\b(?:firewall|acl)\b/i.test(child)) add(issues, 'UNSAFE_FIREWALL_CERTAINTY', source, childPath, 'Do not state firewall/ACL blockage as physical cause without a separately proven evidence contract.');
    }
    scanForbidden(child, issues, source, childPath);
  }
}

function validateHeader(document, expectedType, issues, source) {
  if ('documentType' in document && document.documentType !== expectedType) add(issues, 'DOCUMENT_TYPE_MISMATCH', source, '$.documentType', `Expected ${expectedType}.`);
  if ('schemaVersion' in document && document.schemaVersion !== CONTRACT_VERSION) add(issues, 'UNSUPPORTED_SCHEMA_VERSION', source, '$.schemaVersion', `Expected ${CONTRACT_VERSION}.`);
  if ('requestId' in document) ref128(document.requestId, issues, source, '$.requestId');
  if ('correlationId' in document) ref128(document.correlationId, issues, source, '$.correlationId');
  if ('tenantId' in document) ref128(document.tenantId, issues, source, '$.tenantId');
}

function validateProtocol(family, issues, source, jsonPath) {
  return enumValue(family, PROTOCOLS, 'UNKNOWN_PROTOCOL', issues, source, jsonPath);
}

function validateTransport(transport, issues, source, jsonPath) {
  return enumValue(transport, TRANSPORTS, 'UNKNOWN_TRANSPORT', issues, source, jsonPath);
}

function validateRequest(document, issues, source) {
  assertKeys(document, REQUEST_KEYS, issues, source, '$');
  requireKeys(document, ['documentType', 'schemaVersion', 'requestId', 'correlationId', 'tenantId', 'target', 'requestedProtocols'], issues, source, '$');
  validateHeader(document, 'PROVISIONING_REQUEST', issues, source);

  if ('target' in document) {
    const target = document.target;
    if (!isObject(target)) add(issues, 'INVALID_TYPE', source, '$.target', 'Expected object.');
    else {
      assertKeys(target, TARGET_KEYS, issues, source, '$.target');
      requireKeys(target, ['targetType', 'address'], issues, source, '$.target');
      if ('targetType' in target && safeString(target.targetType, 1, 64, issues, source, '$.target.targetType') && !SAFE_TYPE_RE.test(target.targetType)) add(issues, 'INVALID_TARGET_TYPE', source, '$.target.targetType', 'Target type must be a bounded symbolic identifier.');
      if ('address' in target) safeString(target.address, 1, 512, issues, source, '$.target.address');
      if ('displayName' in target) safeString(target.displayName, 1, 255, issues, source, '$.target.displayName');
      if ('description' in target) safeString(target.description, 1, 2000, issues, source, '$.target.description');
    }
  }

  for (const key of ['siteId', 'sensorPoolId', 'monitoringProfileRef', 'devicePackRef', 'credentialProfileRef', 'idempotencyKey']) if (key in document) ref128(document[key], issues, source, `$.${key}`);
  if ('devicePackVersion' in document) safeString(document.devicePackVersion, 1, 64, issues, source, '$.devicePackVersion');

  if ('requestedProtocols' in document && arrayValue(document.requestedProtocols, 1, 32, issues, source, '$.requestedProtocols')) {
    const declarations = new Set();
    document.requestedProtocols.forEach((declaration, index) => {
      validateRequestProtocol(declaration, issues, source, `$.requestedProtocols[${index}]`);
      if (isObject(declaration)) {
        const key = `${declaration.family ?? ''}|${declaration.transport ?? ''}|${declaration.port ?? ''}|${declaration.tlsRequired ?? ''}|${declaration.securityMode ?? ''}`;
        if (declarations.has(key)) add(issues, 'DUPLICATE_PROTOCOL_DECLARATION', source, `$.requestedProtocols[${index}]`, 'Equivalent requested protocol declaration is duplicated.');
        declarations.add(key);
      }
    });
  }

  if ('tags' in document && arrayValue(document.tags, 0, 64, issues, source, '$.tags')) {
    const keys = new Set();
    document.tags.forEach((tag, index) => {
      const p = `$.tags[${index}]`;
      if (!isObject(tag)) return add(issues, 'INVALID_TYPE', source, p, 'Expected object.');
      assertKeys(tag, TAG_KEYS, issues, source, p);
      requireKeys(tag, ['key', 'value'], issues, source, p);
      if ('key' in tag && safeString(tag.key, 1, 128, issues, source, `${p}.key`)) {
        if (keys.has(tag.key)) add(issues, 'DUPLICATE_TAG_KEY', source, `${p}.key`, `Duplicate tag key: ${tag.key}.`);
        keys.add(tag.key);
      }
      if ('value' in tag) safeString(tag.value, 1, 512, issues, source, `${p}.value`);
    });
  }
}

function validateRequestProtocol(declaration, issues, source, p) {
  if (!isObject(declaration)) return add(issues, 'INVALID_TYPE', source, p, 'Expected object.');
  assertKeys(declaration, REQUEST_PROTOCOL_KEYS, issues, source, p);
  requireKeys(declaration, ['family'], issues, source, p);
  if ('family' in declaration) validateProtocol(declaration.family, issues, source, `${p}.family`);
  if ('transport' in declaration) validateTransport(declaration.transport, issues, source, `${p}.transport`);
  if ('port' in declaration) {
    safeInteger(declaration.port, 1, 65535, 'INVALID_PORT', issues, source, `${p}.port`);
    if (NO_PORT_PROTOCOLS.has(declaration.family)) add(issues, 'PORT_NOT_APPLICABLE', source, `${p}.port`, `${declaration.family} does not use this contract's TCP/UDP port field.`);
  }
  if ('tlsRequired' in declaration && typeof declaration.tlsRequired !== 'boolean') add(issues, 'INVALID_TYPE', source, `${p}.tlsRequired`, 'Expected boolean.');
  if ('securityMode' in declaration) safeString(declaration.securityMode, 1, 64, issues, source, `${p}.securityMode`);
}

function validateTarget(target, issues, source, p) {
  if (!isObject(target)) return add(issues, 'INVALID_TYPE', source, p, 'Expected object.');
  assertKeys(target, READINESS_TARGET_KEYS, issues, source, p);
  requireKeys(target, ['targetType', 'address'], issues, source, p);
  if ('targetType' in target && safeString(target.targetType, 1, 64, issues, source, `${p}.targetType`) && !SAFE_TYPE_RE.test(target.targetType)) add(issues, 'INVALID_TARGET_TYPE', source, `${p}.targetType`, 'Target type must be a bounded symbolic identifier.');
  if ('address' in target) safeString(target.address, 1, 512, issues, source, `${p}.address`);
}

function validateEvidenceList(list, max, issues, source, p, evidenceIds) {
  if (!arrayValue(list, 0, max, issues, source, p)) return;
  list.forEach((evidence, index) => {
    const ep = `${p}[${index}]`;
    if (!isObject(evidence)) return add(issues, 'INVALID_TYPE', source, ep, 'Expected object.');
    assertKeys(evidence, EVIDENCE_KEYS, issues, source, ep);
    requireKeys(evidence, ['evidenceId', 'kind', 'source', 'summary'], issues, source, ep);
    if ('evidenceId' in evidence && ref128(evidence.evidenceId, issues, source, `${ep}.evidenceId`)) {
      if (evidenceIds.has(evidence.evidenceId)) add(issues, 'DUPLICATE_EVIDENCE_ID', source, `${ep}.evidenceId`, `Duplicate evidence ID: ${evidence.evidenceId}.`);
      evidenceIds.add(evidence.evidenceId);
    }
    if ('kind' in evidence) enumValue(evidence.kind, EVIDENCE_KINDS, 'UNKNOWN_EVIDENCE_KIND', issues, source, `${ep}.kind`);
    if ('source' in evidence) enumValue(evidence.source, EVIDENCE_SOURCES, 'UNKNOWN_EVIDENCE_SOURCE', issues, source, `${ep}.source`);
    if ('summary' in evidence) safeString(evidence.summary, 1, 512, issues, source, `${ep}.summary`);
    for (const key of ['sourceRef', 'endpointRef']) if (key in evidence) safeString(evidence[key], 1, 512, issues, source, `${ep}.${key}`);
    if ('technicalCode' in evidence) ref128(evidence.technicalCode, issues, source, `${ep}.technicalCode`);
    if ('protocol' in evidence) validateProtocol(evidence.protocol, issues, source, `${ep}.protocol`);
    if ('transport' in evidence) validateTransport(evidence.transport, issues, source, `${ep}.transport`);
    if ('observedAt' in evidence) isoTimestamp(evidence.observedAt, issues, source, `${ep}.observedAt`);
  });
}

function validateReadiness(document, issues, source) {
  assertKeys(document, READINESS_KEYS, issues, source, '$');
  requireKeys(document, ['documentType', 'schemaVersion', 'requestId', 'correlationId', 'tenantId', 'target', 'stages'], issues, source, '$');
  validateHeader(document, 'READINESS_RESULT', issues, source);
  if ('target' in document) validateTarget(document.target, issues, source, '$.target');

  const evidenceIds = new Set();
  let sensorContext = { ids: new Set(), selectedId: null, selectedCount: 0 };
  if ('sensorEligibility' in document) sensorContext = validateSensorEligibility(document.sensorEligibility, issues, source, evidenceIds);

  if (!('stages' in document) || !arrayValue(document.stages, STAGES.length, STAGES.length, issues, source, '$.stages')) return;

  const seenStages = new Set();
  let runningCount = 0;
  document.stages.forEach((stage, index) => {
    const p = `$.stages[${index}]`;
    if (!isObject(stage)) return add(issues, 'INVALID_TYPE', source, p, 'Expected object.');
    assertKeys(stage, STAGE_KEYS, issues, source, p);
    requireKeys(stage, ['stageId', 'status', 'evidence'], issues, source, p);

    if ('stageId' in stage) {
      if (!STAGE_SET.has(stage.stageId)) add(issues, 'UNKNOWN_STAGE', source, `${p}.stageId`, `Unknown readiness stage: ${String(stage.stageId)}.`);
      if (seenStages.has(stage.stageId)) add(issues, 'DUPLICATE_STAGE', source, `${p}.stageId`, `Duplicate readiness stage: ${String(stage.stageId)}.`);
      seenStages.add(stage.stageId);
      if (stage.stageId !== STAGES[index]) add(issues, 'STAGE_ORDER_VIOLATION', source, `${p}.stageId`, `Expected ${STAGES[index]} at index ${index}.`);
    }
    if ('status' in stage) {
      enumValue(stage.status, STATUSES, 'UNKNOWN_STATUS', issues, source, `${p}.status`);
      if (stage.status === 'RUNNING') runningCount += 1;
    }
    if ('startedAt' in stage) isoTimestamp(stage.startedAt, issues, source, `${p}.startedAt`);
    if ('completedAt' in stage) isoTimestamp(stage.completedAt, issues, source, `${p}.completedAt`);
    if ('durationMs' in stage) safeInteger(stage.durationMs, 0, null, 'INVALID_DURATION', issues, source, `${p}.durationMs`);
    if ('startedAt' in stage && 'completedAt' in stage && !Number.isNaN(Date.parse(stage.startedAt)) && !Number.isNaN(Date.parse(stage.completedAt)) && Date.parse(stage.completedAt) < Date.parse(stage.startedAt)) add(issues, 'STAGE_TIME_ORDER', source, `${p}.completedAt`, 'completedAt cannot precede startedAt.');

    if ('sensorId' in stage) {
      ref128(stage.sensorId, issues, source, `${p}.sensorId`);
      if (sensorContext.ids.size > 0 && !sensorContext.ids.has(stage.sensorId)) add(issues, 'SENSOR_REFERENCE_NOT_FOUND', source, `${p}.sensorId`, `Sensor ${stage.sensorId} is not present in sensorEligibility.candidates.`);
    }
    if ('endpoint' in stage) validateEndpoint(stage.endpoint, issues, source, `${p}.endpoint`, stage.protocol);
    if ('protocol' in stage) validateProtocol(stage.protocol, issues, source, `${p}.protocol`);
    if ('transport' in stage) validateTransport(stage.transport, issues, source, `${p}.transport`);
    validateEvidenceList(stage.evidence, MAX_EVIDENCE, issues, source, `${p}.evidence`, evidenceIds);
    if ('customerExplanation' in stage) safeString(stage.customerExplanation, 1, 2000, issues, source, `${p}.customerExplanation`);
    if ('uncertainty' in stage) safeString(stage.uncertainty, 1, 2000, issues, source, `${p}.uncertainty`);
    if ('diagnosticsRef' in stage) {
      if (!safeString(stage.diagnosticsRef, 1, 512, issues, source, `${p}.diagnosticsRef`) || !REF_RE.test(stage.diagnosticsRef)) add(issues, 'INVALID_DIAGNOSTICS_REFERENCE', source, `${p}.diagnosticsRef`, 'Diagnostics reference must be opaque, bounded and non-filesystem.');
    }
    if ('nextActions' in stage && arrayValue(stage.nextActions, 0, MAX_NEXT_ACTIONS, issues, source, `${p}.nextActions`)) stage.nextActions.forEach((action, actionIndex) => safeString(action, 1, 512, issues, source, `${p}.nextActions[${actionIndex}]`));

    validateStageStatus(stage, issues, source, p);
  });

  if (runningCount > 1) add(issues, 'MULTIPLE_RUNNING_STAGES', source, '$.stages', 'At most one readiness stage may be RUNNING.');
  validateStageProgression(document.stages, issues, source);
  validateSensorStageConsistency(document, sensorContext, issues, source);
}

function validateEndpoint(endpoint, issues, source, p, protocol) {
  if (!isObject(endpoint)) return add(issues, 'INVALID_TYPE', source, p, 'Expected object.');
  assertKeys(endpoint, ENDPOINT_KEYS, issues, source, p);
  requireKeys(endpoint, ['destination'], issues, source, p);
  if ('destination' in endpoint) safeString(endpoint.destination, 1, 512, issues, source, `${p}.destination`);
  if ('port' in endpoint) {
    safeInteger(endpoint.port, 1, 65535, 'INVALID_PORT', issues, source, `${p}.port`);
    if (NO_PORT_PROTOCOLS.has(protocol)) add(issues, 'PORT_NOT_APPLICABLE', source, `${p}.port`, `${protocol} does not use this contract's TCP/UDP port field.`);
  }
}

function validateStageStatus(stage, issues, source, p) {
  const status = stage.status;
  const evidenceCount = Array.isArray(stage.evidence) ? stage.evidence.length : 0;
  const hasFailureCode = 'failureCode' in stage;
  const hasAssessment = 'failureAssessment' in stage;

  if (hasFailureCode && !FAILURE_CODES.has(stage.failureCode)) add(issues, 'UNKNOWN_FAILURE_CODE', source, `${p}.failureCode`, `Non-canonical provisioning failure code: ${String(stage.failureCode)}.`);
  if (hasAssessment && !new Set(['OBSERVED', 'SUSPECTED']).has(stage.failureAssessment)) add(issues, 'UNKNOWN_FAILURE_ASSESSMENT', source, `${p}.failureAssessment`, 'Expected OBSERVED or SUSPECTED.');

  if (status === 'PASS') {
    if (evidenceCount === 0) add(issues, 'PASS_EVIDENCE_REQUIRED', source, `${p}.evidence`, 'PASS requires explicit supporting evidence.');
    if (hasFailureCode || hasAssessment) add(issues, 'PASS_WITH_FAILURE', source, p, 'PASS cannot carry failureCode/failureAssessment.');
    if ('uncertainty' in stage) add(issues, 'PASS_WITH_UNCERTAINTY', source, `${p}.uncertainty`, 'Material uncertainty must use INCONCLUSIVE or UNKNOWN rather than PASS.');
  } else if (status === 'FAIL') {
    if (evidenceCount === 0) add(issues, 'FAIL_EVIDENCE_REQUIRED', source, `${p}.evidence`, 'FAIL requires evidence.');
    if (!hasFailureCode) add(issues, 'FAIL_CODE_REQUIRED', source, `${p}.failureCode`, 'FAIL requires a canonical provisioning failure code.');
    if (!hasAssessment) add(issues, 'FAIL_ASSESSMENT_REQUIRED', source, `${p}.failureAssessment`, 'FAIL requires OBSERVED or SUSPECTED assessment.');
    if (typeof stage.customerExplanation !== 'string' || stage.customerExplanation.length === 0) add(issues, 'FAIL_EXPLANATION_REQUIRED', source, `${p}.customerExplanation`, 'FAIL requires a safe customer-facing explanation.');
    if (FAILURE_CODES.has(stage.failureCode)) {
      const allowedStages = FAILURE_STAGE_MAP.get(stage.failureCode);
      if (allowedStages && !allowedStages.has(stage.stageId)) add(issues, 'FAILURE_CODE_STAGE_MISMATCH', source, `${p}.failureCode`, `${stage.failureCode} is not valid for ${stage.stageId}.`);
    }
    if (stage.failureCode === 'ACL_FIREWALL_SUSPECTED' || stage.failureCode === 'PROTOCOL_DISABLED_SUSPECTED') {
      if (stage.failureAssessment !== 'SUSPECTED') add(issues, 'SUSPECTED_CODE_REQUIRES_SUSPECTED_ASSESSMENT', source, `${p}.failureAssessment`, `${stage.failureCode} requires SUSPECTED assessment.`);
      if (!Array.isArray(stage.evidence) || !stage.evidence.some((item) => item?.kind === 'SUSPECTED')) add(issues, 'SUSPECTED_CODE_REQUIRES_SUSPECTED_EVIDENCE', source, `${p}.evidence`, `${stage.failureCode} requires explicitly suspected evidence.`);
    }
  } else if (status === 'INCONCLUSIVE') {
    if (evidenceCount === 0) add(issues, 'INCONCLUSIVE_EVIDENCE_REQUIRED', source, `${p}.evidence`, 'INCONCLUSIVE requires evidence showing why the result cannot be resolved.');
    if (typeof stage.uncertainty !== 'string' || stage.uncertainty.length === 0) add(issues, 'UNCERTAINTY_REQUIRED', source, `${p}.uncertainty`, 'INCONCLUSIVE requires bounded uncertainty text.');
    if (hasFailureCode || hasAssessment) add(issues, 'STATUS_FAILURE_CONFLICT', source, p, 'INCONCLUSIVE must not invent a canonical failure code.');
  } else if (status === 'UNKNOWN') {
    if (typeof stage.uncertainty !== 'string' || stage.uncertainty.length === 0) add(issues, 'UNCERTAINTY_REQUIRED', source, `${p}.uncertainty`, 'UNKNOWN requires bounded uncertainty text.');
    if (hasFailureCode || hasAssessment) add(issues, 'STATUS_FAILURE_CONFLICT', source, p, 'UNKNOWN must not invent a canonical failure code.');
  } else if (status === 'NOT_RUN' || status === 'SKIPPED_NOT_APPLICABLE') {
    if (evidenceCount > 0) add(issues, 'NONEXECUTED_STAGE_WITH_EVIDENCE', source, `${p}.evidence`, `${status} must not contain execution evidence.`);
    if (hasFailureCode || hasAssessment) add(issues, 'STATUS_FAILURE_CONFLICT', source, p, `${status} cannot carry failure metadata.`);
    for (const key of EXECUTION_CONTEXT_KEYS) if (key in stage) add(issues, 'NONEXECUTED_STAGE_WITH_CONTEXT', source, `${p}.${key}`, `${status} must not imply execution context.`);
  } else if (status === 'RUNNING') {
    if (hasFailureCode || hasAssessment) add(issues, 'STATUS_FAILURE_CONFLICT', source, p, 'RUNNING cannot carry terminal failure metadata.');
    if ('completedAt' in stage) add(issues, 'RUNNING_STAGE_COMPLETED', source, `${p}.completedAt`, 'RUNNING cannot carry completedAt.');
  }
}

function validateStageProgression(stages, issues, source) {
  let blocked = false;
  for (let index = 0; index < stages.length; index += 1) {
    const stage = stages[index];
    if (!isObject(stage) || !STATUSES.has(stage.status)) continue;
    if (blocked && stage.status !== 'NOT_RUN' && stage.status !== 'SKIPPED_NOT_APPLICABLE') add(issues, 'STAGE_PREREQUISITE_VIOLATION', source, `$.stages[${index}].status`, 'A later stage cannot imply execution after an earlier blocking/unresolved stage.');
    if (stage.status !== 'PASS' && stage.status !== 'SKIPPED_NOT_APPLICABLE') blocked = true;
  }
}

function validateSensorEligibility(sensorEligibility, issues, source, evidenceIds) {
  const result = { ids: new Set(), selectedId: null, selectedCount: 0 };
  const p = '$.sensorEligibility';
  if (!isObject(sensorEligibility)) {
    add(issues, 'INVALID_TYPE', source, p, 'Expected object.');
    return result;
  }
  assertKeys(sensorEligibility, SENSOR_ELIGIBILITY_KEYS, issues, source, p);
  requireKeys(sensorEligibility, ['candidates'], issues, source, p);
  if (!arrayValue(sensorEligibility.candidates, 0, MAX_SENSOR_CANDIDATES, issues, source, `${p}.candidates`)) return result;

  sensorEligibility.candidates.forEach((candidate, index) => {
    const cp = `${p}.candidates[${index}]`;
    if (!isObject(candidate)) return add(issues, 'INVALID_TYPE', source, cp, 'Expected object.');
    assertKeys(candidate, SENSOR_CANDIDATE_KEYS, issues, source, cp);
    requireKeys(candidate, ['sensorId', 'selected', 'reason', 'checks'], issues, source, cp);
    if ('sensorId' in candidate && ref128(candidate.sensorId, issues, source, `${cp}.sensorId`)) {
      if (result.ids.has(candidate.sensorId)) add(issues, 'DUPLICATE_SENSOR_CANDIDATE', source, `${cp}.sensorId`, `Duplicate Sensor candidate: ${candidate.sensorId}.`);
      result.ids.add(candidate.sensorId);
    }
    for (const key of ['siteId', 'poolId']) if (key in candidate) ref128(candidate[key], issues, source, `${cp}.${key}`);
    if ('selected' in candidate && typeof candidate.selected !== 'boolean') add(issues, 'INVALID_TYPE', source, `${cp}.selected`, 'Expected boolean.');
    if ('reason' in candidate) safeString(candidate.reason, 1, 512, issues, source, `${cp}.reason`);
    if (candidate.selected === true) {
      result.selectedCount += 1;
      result.selectedId = candidate.sensorId;
    }

    const categories = new Set();
    if ('checks' in candidate && arrayValue(candidate.checks, 1, SENSOR_CHECK_CATEGORIES.size, issues, source, `${cp}.checks`)) {
      candidate.checks.forEach((check, checkIndex) => {
        const kp = `${cp}.checks[${checkIndex}]`;
        if (!isObject(check)) return add(issues, 'INVALID_TYPE', source, kp, 'Expected object.');
        assertKeys(check, SENSOR_CHECK_KEYS, issues, source, kp);
        requireKeys(check, ['category', 'status', 'evidence'], issues, source, kp);
        if ('category' in check) {
          enumValue(check.category, SENSOR_CHECK_CATEGORIES, 'UNKNOWN_SENSOR_CHECK_CATEGORY', issues, source, `${kp}.category`);
          if (categories.has(check.category)) add(issues, 'DUPLICATE_SENSOR_CHECK', source, `${kp}.category`, `Duplicate Sensor eligibility check: ${check.category}.`);
          categories.add(check.category);
        }
        if ('status' in check) enumValue(check.status, STATUSES, 'UNKNOWN_STATUS', issues, source, `${kp}.status`);
        validateEvidenceList(check.evidence, 16, issues, source, `${kp}.evidence`, evidenceIds);
        const count = Array.isArray(check.evidence) ? check.evidence.length : 0;
        if (['PASS', 'FAIL', 'INCONCLUSIVE'].includes(check.status) && count === 0) add(issues, 'SENSOR_CHECK_EVIDENCE_REQUIRED', source, `${kp}.evidence`, `${check.status} Sensor eligibility check requires evidence.`);
        if (['NOT_RUN', 'SKIPPED_NOT_APPLICABLE'].includes(check.status) && count > 0) add(issues, 'NONEXECUTED_STAGE_WITH_EVIDENCE', source, `${kp}.evidence`, `${check.status} Sensor eligibility check must not contain execution evidence.`);
        if (candidate.selected === true && !['PASS', 'SKIPPED_NOT_APPLICABLE'].includes(check.status)) add(issues, 'SELECTED_SENSOR_INELIGIBLE', source, `${kp}.status`, 'Selected Sensor may not contain unresolved or failed eligibility checks.');
      });
    }
  });

  if (result.selectedCount > 1) add(issues, 'MULTIPLE_SELECTED_SENSORS', source, `${p}.candidates`, 'At most one Sensor candidate may be selected.');
  return result;
}

function validateSensorStageConsistency(document, sensorContext, issues, source) {
  if (!Array.isArray(document.stages) || document.stages.length < 3) return;
  const stage = document.stages[2];
  if (!isObject(stage) || stage.stageId !== 'SENSOR_ELIGIBILITY_CAPACITY') return;

  if (stage.status !== 'NOT_RUN' && stage.status !== 'SKIPPED_NOT_APPLICABLE' && !isObject(document.sensorEligibility)) {
    add(issues, 'SENSOR_ELIGIBILITY_EVIDENCE_REQUIRED', source, '$.sensorEligibility', 'Executed Sensor eligibility stage requires candidate evidence.');
    return;
  }
  if (stage.status === 'PASS') {
    if (sensorContext.selectedCount !== 1) add(issues, 'SELECTED_SENSOR_REQUIRED', source, '$.sensorEligibility.candidates', 'PASS requires exactly one selected Sensor candidate.');
    if (typeof stage.sensorId !== 'string') add(issues, 'SELECTED_SENSOR_REFERENCE_REQUIRED', source, '$.stages[2].sensorId', 'PASS requires the selected Sensor reference.');
    else if (sensorContext.selectedId && stage.sensorId !== sensorContext.selectedId) add(issues, 'SELECTED_SENSOR_REFERENCE_MISMATCH', source, '$.stages[2].sensorId', 'Stage Sensor reference does not match selected candidate.');
  }
  if (stage.status === 'FAIL' && (stage.failureCode === 'NO_ELIGIBLE_SENSOR' || stage.failureCode === 'SENSOR_CAPACITY_EXCEEDED') && sensorContext.selectedCount !== 0) add(issues, 'FAILED_SENSOR_STAGE_HAS_SELECTION', source, '$.sensorEligibility.candidates', `${stage.failureCode} must not mark a candidate selected.`);
}

function validateMatrix(document, issues, source) {
  assertKeys(document, MATRIX_KEYS, issues, source, '$');
  requireKeys(document, ['documentType', 'schemaVersion', 'requestId', 'correlationId', 'tenantId', 'rows'], issues, source, '$');
  validateHeader(document, 'CONNECTIVITY_MATRIX', issues, source);
  if (!('rows' in document) || !arrayValue(document.rows, 0, MAX_MATRIX_ROWS, issues, source, '$.rows')) return;

  const rowIds = new Set();
  const semanticRows = new Set();
  const evidenceIds = new Set();
  document.rows.forEach((row, index) => {
    const p = `$.rows[${index}]`;
    if (!isObject(row)) return add(issues, 'INVALID_TYPE', source, p, 'Expected object.');
    assertKeys(row, MATRIX_ROW_KEYS, issues, source, p);
    requireKeys(row, ['rowId', 'source', 'destination', 'protocol', 'direction', 'purpose', 'tlsRequired', 'requirementSource', 'validationStatus', 'evidence'], issues, source, p);

    if ('rowId' in row && ref128(row.rowId, issues, source, `${p}.rowId`)) {
      if (rowIds.has(row.rowId)) add(issues, 'DUPLICATE_MATRIX_ROW_ID', source, `${p}.rowId`, `Duplicate connectivity row ID: ${row.rowId}.`);
      rowIds.add(row.rowId);
    }
    validateMatrixSource(row.source, issues, source, `${p}.source`);
    validateMatrixDestination(row.destination, issues, source, `${p}.destination`);
    if ('protocol' in row) validateProtocol(row.protocol, issues, source, `${p}.protocol`);
    if ('transport' in row) validateTransport(row.transport, issues, source, `${p}.transport`);
    if ('port' in row) {
      safeInteger(row.port, 1, 65535, 'INVALID_PORT', issues, source, `${p}.port`);
      if (NO_PORT_PROTOCOLS.has(row.protocol)) add(issues, 'PORT_NOT_APPLICABLE', source, `${p}.port`, `${row.protocol} does not use this contract's TCP/UDP port field.`);
    }
    if ('direction' in row) enumValue(row.direction, DIRECTIONS, 'UNKNOWN_DIRECTION', issues, source, `${p}.direction`);
    if ('purpose' in row) safeString(row.purpose, 1, 512, issues, source, `${p}.purpose`);
    if ('tlsRequired' in row && typeof row.tlsRequired !== 'boolean') add(issues, 'INVALID_TYPE', source, `${p}.tlsRequired`, 'Expected boolean.');
    if ('securityMode' in row) safeString(row.securityMode, 1, 64, issues, source, `${p}.securityMode`);
    validateRequirementSource(row.requirementSource, issues, source, `${p}.requirementSource`);
    if ('validationStatus' in row) enumValue(row.validationStatus, STATUSES, 'UNKNOWN_STATUS', issues, source, `${p}.validationStatus`);
    validateEvidenceList(row.evidence, MAX_EVIDENCE, issues, source, `${p}.evidence`, evidenceIds);
    if ('observedAt' in row) isoTimestamp(row.observedAt, issues, source, `${p}.observedAt`);
    validateMatrixEvidenceState(row, issues, source, p);

    const semanticKey = matrixSemanticKey(row);
    if (semanticRows.has(semanticKey)) add(issues, 'DUPLICATE_MATRIX_REQUIREMENT', source, p, 'Equivalent connectivity requirement row is duplicated.');
    semanticRows.add(semanticKey);
  });
}

function validateMatrixSource(value, issues, source, p) {
  if (!isObject(value)) return add(issues, 'INVALID_TYPE', source, p, 'Expected object.');
  assertKeys(value, MATRIX_SOURCE_KEYS, issues, source, p);
  requireKeys(value, ['role'], issues, source, p);
  if ('role' in value) enumValue(value.role, MATRIX_SOURCE_ROLES, 'UNKNOWN_SOURCE_ROLE', issues, source, `${p}.role`);
  for (const key of ['identityRef', 'siteId', 'poolId']) if (key in value) ref128(value[key], issues, source, `${p}.${key}`);
  if ('scope' in value) safeString(value.scope, 1, 512, issues, source, `${p}.scope`);
}

function validateMatrixDestination(value, issues, source, p) {
  if (!isObject(value)) return add(issues, 'INVALID_TYPE', source, p, 'Expected object.');
  assertKeys(value, MATRIX_DESTINATION_KEYS, issues, source, p);
  requireKeys(value, ['address'], issues, source, p);
  if ('address' in value) safeString(value.address, 1, 512, issues, source, `${p}.address`);
  if ('scope' in value) safeString(value.scope, 1, 512, issues, source, `${p}.scope`);
}

function validateRequirementSource(value, issues, source, p) {
  if (!isObject(value)) return add(issues, 'INVALID_TYPE', source, p, 'Expected object.');
  assertKeys(value, MATRIX_REQUIREMENT_SOURCE_KEYS, issues, source, p);
  requireKeys(value, ['type', 'reference'], issues, source, p);
  if ('type' in value) enumValue(value.type, REQUIREMENT_SOURCE_TYPES, 'UNKNOWN_REQUIREMENT_SOURCE', issues, source, `${p}.type`);
  if ('reference' in value) ref128(value.reference, issues, source, `${p}.reference`);
}

function validateMatrixEvidenceState(row, issues, source, p) {
  const count = Array.isArray(row.evidence) ? row.evidence.length : 0;
  if (['PASS', 'FAIL', 'INCONCLUSIVE'].includes(row.validationStatus) && count === 0) add(issues, 'MATRIX_EVIDENCE_REQUIRED', source, `${p}.evidence`, `${row.validationStatus} connectivity validation requires evidence.`);
  if (['NOT_RUN', 'SKIPPED_NOT_APPLICABLE'].includes(row.validationStatus) && count > 0) add(issues, 'NONEXECUTED_STAGE_WITH_EVIDENCE', source, `${p}.evidence`, `${row.validationStatus} connectivity validation must not contain execution evidence.`);
}

function matrixSemanticKey(row) {
  return [
    row?.source?.role ?? '', row?.source?.identityRef ?? '', row?.source?.siteId ?? '', row?.source?.poolId ?? '', row?.source?.scope ?? '',
    row?.destination?.address ?? '', row?.destination?.scope ?? '', row?.protocol ?? '', row?.transport ?? '', row?.port ?? '',
    row?.direction ?? '', row?.purpose ?? '', row?.tlsRequired ?? '', row?.securityMode ?? '',
    row?.requirementSource?.type ?? '', row?.requirementSource?.reference ?? '',
  ].join('|');
}

export function validateDocument(document, source = '<memory>') {
  const issues = [];
  if (!isObject(document)) {
    add(issues, 'INVALID_DOCUMENT', source, '$', 'Provisioning contract document must be a JSON object.');
    return stableIssues(issues);
  }

  let serialized = '';
  try { serialized = JSON.stringify(document); } catch { serialized = ''; }
  if (Buffer.byteLength(serialized, 'utf8') > MAX_DOCUMENT_BYTES) add(issues, 'DOCUMENT_TOO_LARGE', source, '$', `Document exceeds defensive ${MAX_DOCUMENT_BYTES}-byte bound.`);

  scanForbidden(document, issues, source);
  if (!DOCUMENT_TYPES.has(document.documentType)) {
    add(issues, 'UNKNOWN_DOCUMENT_TYPE', source, '$.documentType', `Unknown provisioning documentType: ${String(document.documentType)}.`);
    return stableIssues(issues);
  }

  if (document.documentType === 'PROVISIONING_REQUEST') validateRequest(document, issues, source);
  else if (document.documentType === 'READINESS_RESULT') validateReadiness(document, issues, source);
  else validateMatrix(document, issues, source);
  return stableIssues(issues);
}

export function validateDocumentSet(entries) {
  const issues = [];
  const docs = entries.map((entry) => ({ document: entry.document, source: entry.source ?? '<memory>' }));
  docs.forEach(({ document, source }) => issues.push(...validateDocument(document, source)));

  const byRequest = new Map();
  for (const entry of docs) {
    const doc = entry.document;
    if (!isObject(doc) || typeof doc.requestId !== 'string') continue;
    if (!byRequest.has(doc.requestId)) byRequest.set(doc.requestId, []);
    byRequest.get(doc.requestId).push(entry);
  }

  for (const [requestId, group] of byRequest) {
    const correlationIds = new Set(group.map((entry) => entry.document.correlationId).filter((v) => typeof v === 'string'));
    const tenantIds = new Set(group.map((entry) => entry.document.tenantId).filter((v) => typeof v === 'string'));
    if (correlationIds.size > 1) add(issues, 'CORRELATION_REFERENCE_MISMATCH', group[0].source, '$.correlationId', `Documents for ${requestId} disagree on correlationId.`);
    if (tenantIds.size > 1) add(issues, 'TENANT_REFERENCE_MISMATCH', group[0].source, '$.tenantId', `Documents for ${requestId} disagree on tenantId.`);

    const request = group.find((entry) => entry.document.documentType === 'PROVISIONING_REQUEST')?.document;
    if (request) {
      for (const entry of group) {
        if (entry.document.documentType !== 'READINESS_RESULT') continue;
        if (request.target?.address !== entry.document.target?.address || request.target?.targetType !== entry.document.target?.targetType) add(issues, 'TARGET_REFERENCE_MISMATCH', entry.source, '$.target', `Readiness target does not match provisioning request ${requestId}.`);
      }
    }
  }
  return stableIssues(issues);
}

function parseJsonFile(file) {
  const stat = fs.statSync(file);
  if (stat.size > MAX_INPUT_FILE_BYTES) throw new Error(`Input exceeds defensive ${MAX_INPUT_FILE_BYTES}-byte file bound: ${file}`);
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function discoverJsonFiles(inputPath) {
  const resolved = path.resolve(inputPath);
  const stat = fs.statSync(resolved);
  if (stat.isFile()) return [resolved];
  if (!stat.isDirectory()) throw new Error(`Unsupported input: ${inputPath}`);

  const files = [];
  const walk = (dir) => {
    const entries = fs.readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name));
    for (const entry of entries) {
      const full = path.join(dir, entry.name);
      if (entry.isSymbolicLink()) continue;
      if (entry.isDirectory()) walk(full);
      else if (entry.isFile() && entry.name.endsWith('.json') && !entry.name.endsWith('.schema.json') && entry.name !== 'cases.json') files.push(full);
      if (files.length > MAX_DISCOVERED_DOCUMENTS) throw new Error(`More than ${MAX_DISCOVERED_DOCUMENTS} JSON documents discovered; validation is bounded.`);
    }
  };
  walk(resolved);
  return files;
}

function printIssues(issues) {
  for (const issue of stableIssues(issues)) console.error(`${issue.code}\t${issue.source}\t${issue.path}\t${issue.message}`);
}

function validateFiles(inputs) {
  const files = inputs.flatMap(discoverJsonFiles);
  if (files.length === 0) throw new Error('No provisioning contract JSON documents found.');
  const entries = [];
  const issues = [];
  for (const file of files) {
    const source = path.relative(process.cwd(), file);
    try { entries.push({ document: parseJsonFile(file), source }); }
    catch (error) { add(issues, 'JSON_PARSE_ERROR', source, '$', error.message); }
  }
  issues.push(...validateDocumentSet(entries));
  if (issues.length > 0) {
    printIssues(issues);
    return 1;
  }
  console.log(`VALID: ${entries.length} provisioning contract document(s).`);
  return 0;
}

function clone(value) {
  return JSON.parse(JSON.stringify(value));
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
    if (typeof mutation.value !== 'string' || !Number.isSafeInteger(mutation.count) || mutation.count < 0 || mutation.count > 100000) throw new Error('Invalid repeat mutation.');
    parent[key] = mutation.value.repeat(mutation.count);
  } else if (mutation.op === 'swap') {
    if (!Array.isArray(mutation.otherPath)) throw new Error('Swap mutation requires otherPath[].');
    const a = clone(getAt(document, mutation.path));
    const b = clone(getAt(document, mutation.otherPath));
    const pa = parentAt(document, mutation.path);
    const pb = parentAt(document, mutation.otherPath);
    pa.parent[pa.key] = b;
    pb.parent[pb.key] = a;
  } else throw new Error(`Unknown fixture mutation op: ${String(mutation.op)}`);
}

function clearTerminalFields(stage) {
  delete stage.failureCode;
  delete stage.failureAssessment;
  delete stage.customerExplanation;
  delete stage.uncertainty;
  delete stage.diagnosticsRef;
  delete stage.nextActions;
}

function clearExecutionContext(stage) {
  delete stage.startedAt;
  delete stage.completedAt;
  delete stage.durationMs;
  delete stage.sensorId;
  delete stage.endpoint;
}

function materializeFailure(document, fixture) {
  if (document.documentType !== 'READINESS_RESULT') throw new Error('Failure fixture mode requires readiness base.');
  const index = document.stages.findIndex((stage) => stage.stageId === fixture.stageId);
  if (index < 0) throw new Error(`Fixture stage not found: ${fixture.stageId}`);
  const stage = document.stages[index];
  clearTerminalFields(stage);
  stage.status = 'FAIL';
  stage.failureCode = fixture.failureCode;
  stage.failureAssessment = fixture.failureAssessment;
  stage.customerExplanation = `Fictional fixture classification for ${fixture.failureCode}; no live connectivity or provisioning was executed.`;
  stage.evidence = [{
    evidenceId: `fixture.${fixture.name}.evidence`,
    kind: fixture.failureAssessment === 'SUSPECTED' ? 'SUSPECTED' : 'OBSERVED',
    source: ['SENSOR_ELIGIBILITY_CAPACITY', 'IDENTITY_DEVICE_PACK_MATCH', 'MONITORING_PROFILE_DRY_RUN', 'CAPACITY_IMPACT', 'PROVISION'].includes(fixture.stageId) ? 'CORE' : 'SENSOR',
    summary: `Fictional bounded evidence for ${fixture.failureCode}.`,
  }];

  if (fixture.failureCode === 'DNS_FAILURE') document.target.address = 'device.example.test';

  for (let i = index + 1; i < document.stages.length; i += 1) {
    const later = document.stages[i];
    clearTerminalFields(later);
    clearExecutionContext(later);
    later.status = 'NOT_RUN';
    later.evidence = [];
  }

  if (index < 2) delete document.sensorEligibility;
  if (index === 2 && isObject(document.sensorEligibility) && Array.isArray(document.sensorEligibility.candidates)) {
    for (const candidate of document.sensorEligibility.candidates) {
      candidate.selected = false;
      candidate.reason = `Fictional fixture rejection for ${fixture.failureCode}; no placement algorithm was executed.`;
      const category = fixture.failureCode === 'SENSOR_CAPACITY_EXCEEDED' ? 'CAPACITY' : 'HEALTH_READINESS';
      const check = candidate.checks?.find((item) => item.category === category);
      if (check) {
        check.status = 'FAIL';
        check.evidence = [{
          evidenceId: `fixture.${fixture.name}.sensor-check`,
          kind: 'OBSERVED',
          source: 'CORE',
          summary: `Fictional Sensor eligibility evidence for ${fixture.failureCode}.`,
        }];
      }
    }
    delete stage.sensorId;
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
  const suite = parseJsonFile(path.join(ROOT, 'fixtures', 'cases.json'));
  const bases = loadFixtureBases(suite);
  let failures = 0;

  const exampleEntries = [...bases.entries()].map(([name, document]) => ({ document, source: `example:${name}` }));
  const exampleIssues = validateDocumentSet(exampleEntries);
  if (exampleIssues.length > 0) {
    console.error('EXAMPLE VALIDATION FAILURE');
    printIssues(exampleIssues);
    failures += 1;
  }

  if (!Array.isArray(suite.cases)) throw new Error('fixtures/cases.json requires cases[].');
  for (const fixture of suite.cases) {
    if (!isObject(fixture) || typeof fixture.name !== 'string' || typeof fixture.base !== 'string') throw new Error('Each fixture requires name and base.');
    if (!bases.has(fixture.base)) throw new Error(`Unknown fixture base: ${fixture.base}`);
    const document = clone(bases.get(fixture.base));
    if (fixture.mode === 'failure') materializeFailure(document, fixture);
    if (Array.isArray(fixture.mutations)) fixture.mutations.forEach((mutation) => applyMutation(document, mutation));

    const issues = validateDocument(document, `fixture:${fixture.name}`);
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
  console.log(`VALIDATOR FIXTURE SUITE OK: ${suite.cases.length} case(s) plus ${bases.size} example document(s).`);
  return 0;
}

function main() {
  try {
    const args = process.argv.slice(2);
    process.exitCode = args.length === 0 ? runFixtureSuite() : validateFiles(args);
  } catch (error) {
    console.error(`VALIDATOR_ERROR\t${error.message}`);
    process.exitCode = 2;
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) main();

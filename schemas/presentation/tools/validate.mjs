#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, '..');
const CONTRACT_VERSION = '1.0.0';

// Defensive parser/input-safety bounds only. They are not operational defaults,
// query row limits, refresh intervals, SLA targets, retention values or licence limits.
const MAX_DOCUMENT_BYTES = 512 * 1024;
const MAX_INPUT_FILE_BYTES = 1024 * 1024;
const MAX_DISCOVERED_DOCUMENTS = 256;
const MAX_STRING = 4000;
const MAX_LAYOUT_COORDINATE = 100000;
const MAX_LAYOUT_DIMENSION = 100000;

const REF_RE = /^[A-Za-z0-9._:-]+$/;
const TIMEZONE_RE = /^[A-Za-z0-9._+\-/]+$/;

const DOCUMENT_TYPES = new Set(['DASHBOARD_DEFINITION', 'REPORT_DEFINITION', 'SAVED_VIEW']);
const ORIGINS = new Set(['SYSTEM', 'CUSTOMER_CREATED', 'CLONE']);
const SCOPES = new Set(['MY', 'TEAM', 'SHARED', 'TENANT_CUSTOMER', 'SYSTEM']);
const PANEL_TYPES = new Set([
  'STAT', 'GAUGE', 'TIME_SERIES', 'BAR', 'PIE', 'TABLE', 'HEATMAP', 'HISTOGRAM',
  'STATUS_GRID', 'SANKEY', 'TOPOLOGY', 'GEO', 'TEXT', 'ALERT_LIST', 'INCIDENT_LIST',
  'RCA', 'DOMAIN_SPECIFIC',
]);
const CONTEXT_KEYS = new Set([
  'TIME_RANGE', 'TIMEZONE', 'COMPARISON_PERIOD', 'AUTO_REFRESH', 'TENANT_CUSTOMER',
  'REGION', 'SITE', 'GROUP_SERVICE', 'ENTITY', 'TAGS', 'TECHNOLOGY_DOMAIN',
  'BUSINESS_SERVICE', 'INCIDENT', 'SLA',
]);
const DRILL_DESTINATIONS = new Set([
  'ENTITY_DETAIL', 'DASHBOARD', 'TOPOLOGY', 'TIMELINE', 'RCA', 'INCIDENT', 'ALERT',
  'NCM_CLI', 'IPAM', 'FLOW', 'ITSM', 'AUDIT', 'REPORT',
]);
const REQUIRED_STATES = [
  'LOADING', 'EMPTY', 'PARTIAL_DATA', 'STALE_DATA', 'PERMISSION_RESTRICTED',
  'NOT_LICENSED', 'QUERY_LIMITED', 'ERROR', 'MONITORING_DEGRADED', 'NORMAL',
];
const REQUIRED_STATE_SET = new Set(REQUIRED_STATES);
const REPORT_SECTION_TYPES = new Set(['TEXT', 'TABLE', 'VISUALIZATION', 'IMAGE_REFERENCE']);
const EXPORT_FORMATS = new Set(['PDF', 'CSV', 'XLSX', 'JSON', 'API']);
const DELIVERY_CLASSES = new Set(['DOWNLOAD', 'EMAIL', 'INTEGRATION']);
const PARAMETER_SOURCE_KINDS = new Set(['VARIABLE', 'FILTER', 'CONTEXT']);

const SECRET_KEY_TOKENS = new Set([
  'password', 'passwordhash', 'passwd', 'token', 'bearer', 'bearertoken', 'apikey',
  'privatekey', 'privatekeypem', 'clientsecret', 'cookie', 'cookies', 'sessioncookie',
  'databasecredential', 'databasecredentials', 'databasecredentialref', 'dbpassword',
  'datasourcesecret', 'credentialvalue', 'secret', 'secretvalue', 'accesstoken', 'refreshtoken',
]);
const EXECUTABLE_KEY_TOKENS = new Set([
  'sql', 'rawsql', 'querytext', 'querystring', 'javascript', 'python', 'shell', 'command',
  'commands', 'script', 'scripts', 'exec', 'executable', 'expression', 'formula', 'inlinecode',
  'code', 'croncommand', 'hook', 'scripturl',
]);
const URL_KEY_TOKENS = new Set(['url', 'href', 'uri', 'externalurl', 'linkurl']);
const AUTHORIZATION_DECISION_KEYS = new Set([
  'authorized', 'authorizationdecision', 'accessgranted', 'entitled', 'effectivepermission',
  'grant', 'grants', 'deny', 'denied', 'allow', 'allowed',
]);

const DASHBOARD_KEYS = new Set([
  'documentType', 'schemaVersion', 'definitionId', 'definitionVersion', 'displayName',
  'description', 'origin', 'scope', 'systemDefinition', 'recoverableSystemDefinition',
  'derivedFrom', 'folderRef', 'category', 'tags', 'visibility', 'contextPolicy',
  'queryBindings', 'variables', 'filters', 'views', 'defaultViewRef', 'panels',
  'actionRefs', 'drillDowns', 'stateHandling', 'performance', 'provenance',
  'valuePresentationPolicy',
]);
const REPORT_KEYS = new Set([
  'documentType', 'schemaVersion', 'definitionId', 'definitionVersion', 'displayName',
  'description', 'origin', 'scope', 'systemDefinition', 'recoverableSystemDefinition',
  'derivedFrom', 'category', 'folderRef', 'tags', 'visibility', 'contextPolicy',
  'queryBindings', 'variables', 'filters', 'sections', 'allowedFormats', 'slaPolicyRef',
  'schedule', 'deliveryTargets', 'stateHandling', 'provenance', 'valuePresentationPolicy',
]);
const SAVED_VIEW_KEYS = new Set([
  'documentType', 'schemaVersion', 'savedViewId', 'definitionVersion', 'displayName',
  'description', 'ownerRef', 'scopeRef', 'targetType', 'targetRef', 'filterState',
  'timeContextPolicyRef', 'sortRef', 'groupRef', 'viewModeRef', 'provenance',
  'serverAuthorizationRequired',
]);
const DERIVED_KEYS = new Set(['definitionId', 'definitionVersion']);
const VISIBILITY_KEYS = new Set([
  'roleRefs', 'resourceScopeRef', 'permissionRefs', 'entitlementRefs', 'serverAuthorizationRequired',
]);
const CONTEXT_POLICY_KEYS = new Set(['runtimeContextKeys', 'defaultTimePolicyRef', 'timePolicyRef']);
const QUERY_KEYS = new Set(['queryId', 'queryRef', 'dataSourceClassRef', 'parameterRefs']);
const VARIABLE_KEYS = new Set([
  'variableId', 'label', 'sourceRef', 'defaultSelectionRefs', 'multiSelect', 'required',
  'persistInUrl', 'dependsOnVariableRef',
]);
const FILTER_KEYS = new Set([
  'filterId', 'label', 'sourceRef', 'operatorRef', 'variableRef', 'runtimeContextKey', 'reusableFilterRef',
]);
const VIEW_KEYS = new Set(['viewId', 'label', 'panelRefs']);
const PANEL_KEYS = new Set([
  'panelId', 'title', 'description', 'type', 'layout', 'queryUses', 'transformationRefs',
  'visualizationSettingsRef', 'thresholdPolicyRef', 'fieldOverrideRefs', 'filterRefs',
  'variableRefs', 'actionRefs', 'drillDowns', 'statePolicyRef', 'visibility',
]);
const LAYOUT_KEYS = new Set(['x', 'y', 'width', 'height']);
const QUERY_USE_KEYS = new Set(['queryId', 'bindings']);
const PARAMETER_BINDING_KEYS = new Set(['parameterRef', 'sourceKind', 'sourceRef']);
const DRILLDOWN_KEYS = new Set(['drillDownId', 'destinationType', 'targetRef', 'preserveContext']);
const PRESERVE_CONTEXT_KEYS = new Set(['tenant', 'scope', 'time', 'filters']);
const STATE_HANDLING_KEYS = new Set([
  'states', 'showFreshnessWhenSupplied', 'showProvenanceWhenSupplied',
  'showAffectedSourceWhenSupplied', 'showCorrelationIdWhenSupplied', 'causalInferenceFromMissingData',
]);
const STATE_ENTRY_KEYS = new Set(['state', 'presentationRef']);
const PERFORMANCE_KEYS = new Set([
  'serverSidePagination', 'serverSideFiltering', 'serverSideSorting', 'virtualizationHint', 'queryPolicyRef',
]);
const PROVENANCE_KEYS = new Set(['sourceRef', 'sourceVersion', 'notes']);
const VALUE_POLICY_KEYS = new Set([
  'methodologyRef', 'customerAssumptionRef', 'requireSampleSizeDisclosure',
  'requireAssumptionDisclosure', 'unsupportedCausalClaimsProhibited',
  'financialEstimatesRequireCustomerApprovedAssumptions',
]);
const SECTION_KEYS = new Set([
  'sectionId', 'title', 'description', 'type', 'queryUses', 'text', 'tableDefinitionRef',
  'visualizationDefinitionRef', 'imageRef', 'statePolicyRef',
]);
const SCHEDULE_KEYS = new Set(['scheduleRef', 'timezone', 'enabled']);
const DELIVERY_KEYS = new Set(['deliveryClass', 'targetRef']);
const SAVED_FILTER_KEYS = new Set(['filterRef', 'selectionRefs']);

function isObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function keyToken(key) {
  return String(key).toLowerCase().replace(/[_\-.]/g, '');
}

function add(issues, code, source, jsonPath, message) {
  issues.push({ code, source, path: jsonPath, message });
}

function sortedIssues(issues) {
  return [...issues].sort((a, b) => `${a.source}|${a.path}|${a.code}`.localeCompare(`${b.source}|${b.path}|${b.code}`));
}

function assertObject(value, issues, source, jsonPath) {
  if (!isObject(value)) {
    add(issues, 'INVALID_TYPE', source, jsonPath, 'Expected object.');
    return false;
  }
  return true;
}

function assertKeys(object, allowed, issues, source, jsonPath) {
  if (!isObject(object)) return;
  for (const key of Object.keys(object)) {
    if (!allowed.has(key)) add(issues, 'UNKNOWN_FIELD', source, `${jsonPath}.${key}`, 'Field is not defined by this presentation contract.');
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
  if (value.length < min || value.length > max) add(issues, 'STRING_BOUNDS', source, jsonPath, `String length must be ${min}-${max}.`);
  if (/\p{Cc}/u.test(value)) add(issues, 'CONTROL_CHARACTER', source, jsonPath, 'Control characters are not allowed.');
  return value.length >= min && value.length <= max;
}

function ref128(value, issues, source, jsonPath) {
  if (!safeString(value, 1, 128, issues, source, jsonPath)) return false;
  if (!REF_RE.test(value)) {
    add(issues, 'INVALID_REFERENCE', source, jsonPath, 'Expected bounded opaque identifier grammar [A-Za-z0-9._:-]+.');
    return false;
  }
  return true;
}

function version64(value, issues, source, jsonPath) {
  if (!safeString(value, 1, 64, issues, source, jsonPath)) return false;
  if (!REF_RE.test(value)) {
    add(issues, 'INVALID_VERSION', source, jsonPath, 'Definition version must be a bounded opaque identifier.');
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

function boolValue(value, issues, source, jsonPath) {
  if (typeof value !== 'boolean') add(issues, 'INVALID_TYPE', source, jsonPath, 'Expected boolean.');
}

function enumValue(value, allowed, code, issues, source, jsonPath) {
  if (!allowed.has(value)) {
    add(issues, code, source, jsonPath, `Unsupported value: ${String(value)}.`);
    return false;
  }
  return true;
}

function uniquePrimitiveArray(value, issues, source, jsonPath) {
  if (!Array.isArray(value)) return;
  const seen = new Set();
  value.forEach((item, index) => {
    if (seen.has(item)) add(issues, 'DUPLICATE_VALUE', source, `${jsonPath}[${index}]`, `Duplicate value: ${String(item)}.`);
    seen.add(item);
  });
}

function scanForbidden(value, issues, source, jsonPath = '$') {
  if (Array.isArray(value)) {
    value.forEach((item, index) => scanForbidden(item, issues, source, `${jsonPath}[${index}]`));
    return;
  }
  if (!isObject(value)) return;

  for (const [key, child] of Object.entries(value)) {
    const token = keyToken(key);
    const p = `${jsonPath}.${key}`;
    if (SECRET_KEY_TOKENS.has(token)) add(issues, 'FORBIDDEN_SECRET_FIELD', source, p, 'Secret-bearing fields are prohibited in presentation definitions.');
    if (EXECUTABLE_KEY_TOKENS.has(token)) add(issues, 'FORBIDDEN_EXECUTABLE_FIELD', source, p, 'Executable/query/expression fields are prohibited in this contract.');
    if (URL_KEY_TOKENS.has(token)) add(issues, 'FORBIDDEN_URL_FIELD', source, p, 'Raw URL fields are prohibited; use governed opaque references.');
    if (AUTHORIZATION_DECISION_KEYS.has(token)) add(issues, 'AUTHORIZATION_SEMANTICS_PROHIBITED', source, p, 'Presentation metadata may reference authorization inputs but may not embed authorization/entitlement decisions.');

    if (typeof child === 'string') {
      if (child.length > MAX_STRING) add(issues, 'UNBOUNDED_TEXT', source, p, `String exceeds defensive ${MAX_STRING}-character bound.`);
      if (/-----BEGIN [A-Z ]*PRIVATE KEY-----/.test(child)) add(issues, 'PRIVATE_KEY_MATERIAL', source, p, 'Private-key material is prohibited.');
      if (/:\/\/[^/\s:@]+:[^/\s@]+@/.test(child)) add(issues, 'EMBEDDED_URL_CREDENTIAL', source, p, 'Credentials embedded in URLs are prohibited.');
      if (/^(?:javascript|data|file|vbscript):/i.test(child.trim())) add(issues, 'DANGEROUS_URL_SCHEME', source, p, 'Dangerous executable URL scheme is prohibited.');
      if (/<\s*(?:script|iframe|object|embed|svg)\b|\bon[a-z]+\s*=|javascript\s*:/i.test(child)) add(issues, 'UNSAFE_HTML', source, p, 'Active/raw HTML or script content is prohibited.');
      if (/(?:^|[\\/])\.\.(?:[\\/]|$)|%2e%2e|%2f\.\.|%5c\.\./i.test(child)) add(issues, 'PATH_TRAVERSAL', source, p, 'Path traversal is prohibited.');
      if (/^(?:\/|[A-Za-z]:[\\/]|\\\\)/.test(child) && !/^[A-Za-z][A-Za-z0-9+.-]*:\/\//.test(child)) add(issues, 'ABSOLUTE_FILESYSTEM_PATH', source, p, 'Absolute filesystem paths are prohibited.');
      if (/\bSELECT\b[\s\S]{0,512}\bFROM\b|\bINSERT\s+INTO\b|\bDELETE\s+FROM\b|\bUPDATE\b[\s\S]{0,256}\bSET\b|\bDROP\s+(?:TABLE|DATABASE)\b|\bUNION\s+SELECT\b/i.test(child)) add(issues, 'RAW_SQL_PROHIBITED', source, p, 'Raw SQL is prohibited; definitions must use governed query references.');
      if (/\b(?:python|javascript|node|bash|sh)\s*[-:]|\bfunction\s*\(|=>|\$\(|`[^`]+`/i.test(child)) add(issues, 'EXECUTABLE_CONTENT', source, p, 'Executable/script-like content is prohibited.');
      if (/\b(?:mttr|savings|reduction|benefit)\b[^\n]{0,64}\b\d+(?:\.\d+)?\s*%/i.test(child) || /\b\d+(?:\.\d+)?\s*%[^\n]{0,64}\b(?:mttr|savings|reduction|benefit)\b/i.test(child)) add(issues, 'UNSUPPORTED_VALUE_CLAIM', source, p, 'Static definitions may not embed unsupported MTTR/savings/benefit percentages.');
    }
    scanForbidden(child, issues, source, p);
  }
}

function validateHeader(document, expectedType, issues, source) {
  if (document.documentType !== expectedType) add(issues, 'DOCUMENT_TYPE_MISMATCH', source, '$.documentType', `Expected ${expectedType}.`);
  if (document.schemaVersion !== CONTRACT_VERSION) add(issues, 'UNSUPPORTED_SCHEMA_VERSION', source, '$.schemaVersion', `Expected schemaVersion ${CONTRACT_VERSION}.`);
}

function validateOrigin(document, issues, source) {
  enumValue(document.origin, ORIGINS, 'INVALID_ORIGIN', issues, source, '$.origin');
  enumValue(document.scope, SCOPES, 'INVALID_SCOPE', issues, source, '$.scope');
  boolValue(document.systemDefinition, issues, source, '$.systemDefinition');
  boolValue(document.recoverableSystemDefinition, issues, source, '$.recoverableSystemDefinition');

  if (document.origin === 'SYSTEM') {
    if (document.scope !== 'SYSTEM') add(issues, 'SYSTEM_SCOPE_REQUIRED', source, '$.scope', 'SYSTEM definitions must use SYSTEM scope.');
    if (document.systemDefinition !== true) add(issues, 'SYSTEM_IDENTITY_REQUIRED', source, '$.systemDefinition', 'SYSTEM origin requires systemDefinition=true.');
    if (document.recoverableSystemDefinition !== true) add(issues, 'RECOVERABLE_SYSTEM_REQUIRED', source, '$.recoverableSystemDefinition', 'SYSTEM origin requires recoverableSystemDefinition=true.');
    if ('derivedFrom' in document) add(issues, 'SYSTEM_DERIVATION_PROHIBITED', source, '$.derivedFrom', 'Authoritative SYSTEM definitions may not masquerade as clones.');
  }
  if (document.origin === 'CUSTOMER_CREATED') {
    if (document.scope === 'SYSTEM') add(issues, 'CUSTOMER_SYSTEM_SCOPE_PROHIBITED', source, '$.scope', 'Customer-created definitions cannot use SYSTEM scope.');
    if (document.systemDefinition === true || document.recoverableSystemDefinition === true) add(issues, 'CUSTOMER_SYSTEM_MASQUERADE', source, '$', 'Customer-created definitions cannot mark themselves as authoritative/recoverable SYSTEM definitions.');
    if ('derivedFrom' in document) add(issues, 'CUSTOMER_DERIVATION_MISMATCH', source, '$.derivedFrom', 'Derived customer copies must use CLONE origin.');
  }
  if (document.origin === 'CLONE') {
    if (document.scope === 'SYSTEM') add(issues, 'CLONE_SYSTEM_SCOPE_PROHIBITED', source, '$.scope', 'Clones cannot use SYSTEM scope.');
    if (document.systemDefinition === true || document.recoverableSystemDefinition === true) add(issues, 'CLONE_SYSTEM_MASQUERADE', source, '$', 'A clone cannot masquerade as an authoritative/recoverable SYSTEM definition.');
    if (!isObject(document.derivedFrom)) add(issues, 'CLONE_PROVENANCE_REQUIRED', source, '$.derivedFrom', 'CLONE origin requires explicit derivation provenance.');
  }

  if ('derivedFrom' in document) {
    if (assertObject(document.derivedFrom, issues, source, '$.derivedFrom')) {
      assertKeys(document.derivedFrom, DERIVED_KEYS, issues, source, '$.derivedFrom');
      requireKeys(document.derivedFrom, ['definitionId', 'definitionVersion'], issues, source, '$.derivedFrom');
      if ('definitionId' in document.derivedFrom) {
        ref128(document.derivedFrom.definitionId, issues, source, '$.derivedFrom.definitionId');
        if (document.derivedFrom.definitionId === document.definitionId) add(issues, 'CLONE_IDENTITY_COLLISION', source, '$.derivedFrom.definitionId', 'Clone definitionId must differ from its source definitionId.');
      }
      if ('definitionVersion' in document.derivedFrom) version64(document.derivedFrom.definitionVersion, issues, source, '$.derivedFrom.definitionVersion');
    }
  }
}

function validateVisibility(value, issues, source, p) {
  if (!assertObject(value, issues, source, p)) return;
  assertKeys(value, VISIBILITY_KEYS, issues, source, p);
  requireKeys(value, ['serverAuthorizationRequired'], issues, source, p);
  if (value.serverAuthorizationRequired !== true) add(issues, 'SERVER_AUTHORIZATION_REQUIRED', source, `${p}.serverAuthorizationRequired`, 'Server-side authorization must remain authoritative.');
  for (const key of ['roleRefs', 'permissionRefs', 'entitlementRefs']) {
    if (key in value && arrayValue(value[key], 0, 32, issues, source, `${p}.${key}`)) {
      uniquePrimitiveArray(value[key], issues, source, `${p}.${key}`);
      value[key].forEach((item, index) => ref128(item, issues, source, `${p}.${key}[${index}]`));
    }
  }
  if ('resourceScopeRef' in value) ref128(value.resourceScopeRef, issues, source, `${p}.resourceScopeRef`);
}

function validateContextPolicy(value, issues, source, p) {
  const contexts = new Set();
  if (!assertObject(value, issues, source, p)) return contexts;
  assertKeys(value, CONTEXT_POLICY_KEYS, issues, source, p);
  requireKeys(value, ['runtimeContextKeys'], issues, source, p);
  if (arrayValue(value.runtimeContextKeys, 0, CONTEXT_KEYS.size, issues, source, `${p}.runtimeContextKeys`)) {
    value.runtimeContextKeys.forEach((item, index) => {
      enumValue(item, CONTEXT_KEYS, 'UNKNOWN_CONTEXT_KEY', issues, source, `${p}.runtimeContextKeys[${index}]`);
      if (contexts.has(item)) add(issues, 'DUPLICATE_CONTEXT_KEY', source, `${p}.runtimeContextKeys[${index}]`, `Duplicate context key: ${String(item)}.`);
      contexts.add(item);
    });
  }
  for (const key of ['defaultTimePolicyRef', 'timePolicyRef']) if (key in value) ref128(value[key], issues, source, `${p}.${key}`);
  return contexts;
}

function collectById(items, idKey, max, allowedKeys, requiredKeys, issues, source, p) {
  const map = new Map();
  if (!arrayValue(items, 0, max, issues, source, p)) return map;
  items.forEach((item, index) => {
    const ip = `${p}[${index}]`;
    if (!assertObject(item, issues, source, ip)) return;
    assertKeys(item, allowedKeys, issues, source, ip);
    requireKeys(item, requiredKeys, issues, source, ip);
    if (idKey in item && ref128(item[idKey], issues, source, `${ip}.${idKey}`)) {
      if (map.has(item[idKey])) add(issues, `DUPLICATE_${idKey.replace(/([A-Z])/g, '_$1').toUpperCase()}`, source, `${ip}.${idKey}`, `Duplicate ${idKey}: ${item[idKey]}.`);
      else map.set(item[idKey], item);
    }
  });
  return map;
}

function validateQueryBindings(items, issues, source, p = '$.queryBindings') {
  const queries = collectById(items, 'queryId', 128, QUERY_KEYS, ['queryId', 'queryRef', 'dataSourceClassRef', 'parameterRefs'], issues, source, p);
  for (const [id, query] of queries) {
    ref128(query.queryRef, issues, source, `${p}[queryId=${id}].queryRef`);
    ref128(query.dataSourceClassRef, issues, source, `${p}[queryId=${id}].dataSourceClassRef`);
    if (arrayValue(query.parameterRefs, 0, 32, issues, source, `${p}[queryId=${id}].parameterRefs`)) {
      uniquePrimitiveArray(query.parameterRefs, issues, source, `${p}[queryId=${id}].parameterRefs`);
      query.parameterRefs.forEach((item, index) => ref128(item, issues, source, `${p}[queryId=${id}].parameterRefs[${index}]`));
    }
  }
  return queries;
}

function validateVariables(items, issues, source, p = '$.variables') {
  const variables = collectById(items, 'variableId', 64, VARIABLE_KEYS, ['variableId', 'label', 'sourceRef', 'multiSelect', 'required', 'persistInUrl'], issues, source, p);
  for (const [id, variable] of variables) {
    safeString(variable.label, 1, 128, issues, source, `${p}[variableId=${id}].label`);
    ref128(variable.sourceRef, issues, source, `${p}[variableId=${id}].sourceRef`);
    boolValue(variable.multiSelect, issues, source, `${p}[variableId=${id}].multiSelect`);
    boolValue(variable.required, issues, source, `${p}[variableId=${id}].required`);
    boolValue(variable.persistInUrl, issues, source, `${p}[variableId=${id}].persistInUrl`);
    if ('defaultSelectionRefs' in variable && arrayValue(variable.defaultSelectionRefs, 0, 32, issues, source, `${p}[variableId=${id}].defaultSelectionRefs`)) {
      uniquePrimitiveArray(variable.defaultSelectionRefs, issues, source, `${p}[variableId=${id}].defaultSelectionRefs`);
      variable.defaultSelectionRefs.forEach((item, index) => ref128(item, issues, source, `${p}[variableId=${id}].defaultSelectionRefs[${index}]`));
    }
    if ('dependsOnVariableRef' in variable) ref128(variable.dependsOnVariableRef, issues, source, `${p}[variableId=${id}].dependsOnVariableRef`);
  }

  for (const [id, variable] of variables) {
    if ('dependsOnVariableRef' in variable && !variables.has(variable.dependsOnVariableRef)) add(issues, 'UNDEFINED_VARIABLE_DEPENDENCY', source, `${p}[variableId=${id}].dependsOnVariableRef`, `Undefined variable dependency: ${variable.dependsOnVariableRef}.`);
  }

  const visiting = new Set();
  const visited = new Set();
  const visit = (id, trail) => {
    if (visited.has(id)) return;
    if (visiting.has(id)) {
      add(issues, 'VARIABLE_DEPENDENCY_CYCLE', source, p, `Variable dependency cycle: ${[...trail, id].join(' -> ')}.`);
      return;
    }
    visiting.add(id);
    const dep = variables.get(id)?.dependsOnVariableRef;
    if (dep && variables.has(dep)) visit(dep, [...trail, id]);
    visiting.delete(id);
    visited.add(id);
  };
  for (const id of variables.keys()) visit(id, []);
  return variables;
}

function validateFilters(items, variables, contexts, issues, source, p = '$.filters') {
  const filters = collectById(items, 'filterId', 64, FILTER_KEYS, ['filterId', 'label', 'sourceRef', 'operatorRef'], issues, source, p);
  for (const [id, filter] of filters) {
    safeString(filter.label, 1, 128, issues, source, `${p}[filterId=${id}].label`);
    ref128(filter.sourceRef, issues, source, `${p}[filterId=${id}].sourceRef`);
    ref128(filter.operatorRef, issues, source, `${p}[filterId=${id}].operatorRef`);
    if ('reusableFilterRef' in filter) ref128(filter.reusableFilterRef, issues, source, `${p}[filterId=${id}].reusableFilterRef`);
    if ('variableRef' in filter) {
      ref128(filter.variableRef, issues, source, `${p}[filterId=${id}].variableRef`);
      if (!variables.has(filter.variableRef)) add(issues, 'UNDEFINED_VARIABLE_REFERENCE', source, `${p}[filterId=${id}].variableRef`, `Undefined variable reference: ${filter.variableRef}.`);
    }
    if ('runtimeContextKey' in filter) {
      enumValue(filter.runtimeContextKey, CONTEXT_KEYS, 'UNKNOWN_CONTEXT_KEY', issues, source, `${p}[filterId=${id}].runtimeContextKey`);
      if (!contexts.has(filter.runtimeContextKey)) add(issues, 'UNDECLARED_CONTEXT_REFERENCE', source, `${p}[filterId=${id}].runtimeContextKey`, `Context ${filter.runtimeContextKey} is not declared by contextPolicy.`);
    }
    if ('variableRef' in filter && 'runtimeContextKey' in filter) add(issues, 'FILTER_SOURCE_AMBIGUOUS', source, `${p}[filterId=${id}]`, 'A filter cannot bind both variableRef and runtimeContextKey.');
  }
  return filters;
}

function validateQueryUse(use, queries, variables, filters, contexts, issues, source, p) {
  if (!assertObject(use, issues, source, p)) return;
  assertKeys(use, QUERY_USE_KEYS, issues, source, p);
  requireKeys(use, ['queryId', 'bindings'], issues, source, p);
  if (!ref128(use.queryId, issues, source, `${p}.queryId`)) return;
  const query = queries.get(use.queryId);
  if (!query) add(issues, 'UNDEFINED_QUERY_REFERENCE', source, `${p}.queryId`, `Undefined local queryId: ${use.queryId}.`);
  const bound = new Set();
  if (arrayValue(use.bindings, 0, 32, issues, source, `${p}.bindings`)) {
    use.bindings.forEach((binding, index) => {
      const bp = `${p}.bindings[${index}]`;
      if (!assertObject(binding, issues, source, bp)) return;
      assertKeys(binding, PARAMETER_BINDING_KEYS, issues, source, bp);
      requireKeys(binding, ['parameterRef', 'sourceKind', 'sourceRef'], issues, source, bp);
      if ('parameterRef' in binding && ref128(binding.parameterRef, issues, source, `${bp}.parameterRef`)) {
        if (bound.has(binding.parameterRef)) add(issues, 'DUPLICATE_PARAMETER_BINDING', source, `${bp}.parameterRef`, `Duplicate parameter binding: ${binding.parameterRef}.`);
        bound.add(binding.parameterRef);
        if (query && !query.parameterRefs.includes(binding.parameterRef)) add(issues, 'UNKNOWN_QUERY_PARAMETER', source, `${bp}.parameterRef`, `Parameter ${binding.parameterRef} is not declared by query ${use.queryId}.`);
      }
      enumValue(binding.sourceKind, PARAMETER_SOURCE_KINDS, 'UNKNOWN_PARAMETER_SOURCE', issues, source, `${bp}.sourceKind`);
      if ('sourceRef' in binding) ref128(binding.sourceRef, issues, source, `${bp}.sourceRef`);
      if (binding.sourceKind === 'VARIABLE' && !variables.has(binding.sourceRef)) add(issues, 'UNDEFINED_VARIABLE_REFERENCE', source, `${bp}.sourceRef`, `Undefined variable reference: ${binding.sourceRef}.`);
      if (binding.sourceKind === 'FILTER' && !filters.has(binding.sourceRef)) add(issues, 'UNDEFINED_FILTER_REFERENCE', source, `${bp}.sourceRef`, `Undefined filter reference: ${binding.sourceRef}.`);
      if (binding.sourceKind === 'CONTEXT') {
        if (!CONTEXT_KEYS.has(binding.sourceRef)) add(issues, 'UNKNOWN_CONTEXT_KEY', source, `${bp}.sourceRef`, `Unknown context key: ${binding.sourceRef}.`);
        else if (!contexts.has(binding.sourceRef)) add(issues, 'UNDECLARED_CONTEXT_REFERENCE', source, `${bp}.sourceRef`, `Context ${binding.sourceRef} is not declared by contextPolicy.`);
      }
    });
  }
  if (query) {
    for (const parameterRef of query.parameterRefs) if (!bound.has(parameterRef)) add(issues, 'MISSING_QUERY_PARAMETER_BINDING', source, p, `Query ${use.queryId} parameter ${parameterRef} is not bound.`);
  }
}

function validateDrillDown(value, issues, source, p) {
  if (!assertObject(value, issues, source, p)) return;
  assertKeys(value, DRILLDOWN_KEYS, issues, source, p);
  requireKeys(value, ['drillDownId', 'destinationType', 'targetRef', 'preserveContext'], issues, source, p);
  ref128(value.drillDownId, issues, source, `${p}.drillDownId`);
  enumValue(value.destinationType, DRILL_DESTINATIONS, 'INVALID_DRILLDOWN_DESTINATION', issues, source, `${p}.destinationType`);
  ref128(value.targetRef, issues, source, `${p}.targetRef`);
  if (assertObject(value.preserveContext, issues, source, `${p}.preserveContext`)) {
    assertKeys(value.preserveContext, PRESERVE_CONTEXT_KEYS, issues, source, `${p}.preserveContext`);
    requireKeys(value.preserveContext, ['tenant', 'scope', 'time', 'filters'], issues, source, `${p}.preserveContext`);
    for (const key of PRESERVE_CONTEXT_KEYS) if (key in value.preserveContext) boolValue(value.preserveContext[key], issues, source, `${p}.preserveContext.${key}`);
  }
}

function validateStateHandling(value, issues, source, p, dashboard = false) {
  if (!assertObject(value, issues, source, p)) return;
  assertKeys(value, STATE_HANDLING_KEYS, issues, source, p);
  requireKeys(value, ['states', 'causalInferenceFromMissingData'], issues, source, p);
  const seen = new Set();
  if (arrayValue(value.states, REQUIRED_STATES.length, REQUIRED_STATES.length, issues, source, `${p}.states`)) {
    value.states.forEach((entry, index) => {
      const ep = `${p}.states[${index}]`;
      if (!assertObject(entry, issues, source, ep)) return;
      assertKeys(entry, STATE_ENTRY_KEYS, issues, source, ep);
      requireKeys(entry, ['state', 'presentationRef'], issues, source, ep);
      if (!REQUIRED_STATE_SET.has(entry.state)) add(issues, 'UNKNOWN_REQUIRED_STATE', source, `${ep}.state`, `Unknown required state: ${String(entry.state)}.`);
      else if (seen.has(entry.state)) add(issues, 'DUPLICATE_REQUIRED_STATE', source, `${ep}.state`, `Duplicate required state: ${entry.state}.`);
      else seen.add(entry.state);
      if ('presentationRef' in entry) ref128(entry.presentationRef, issues, source, `${ep}.presentationRef`);
    });
  }
  for (const state of REQUIRED_STATES) if (!seen.has(state)) add(issues, 'MISSING_REQUIRED_STATE', source, `${p}.states`, `Required state ${state} is missing.`);
  if (value.causalInferenceFromMissingData !== 'PROHIBITED') add(issues, 'MISSING_DATA_CAUSAL_INFERENCE_PROHIBITED', source, `${p}.causalInferenceFromMissingData`, 'Definitions must not infer operational cause from missing telemetry.');
  if (dashboard) {
    for (const key of ['showFreshnessWhenSupplied', 'showProvenanceWhenSupplied', 'showAffectedSourceWhenSupplied', 'showCorrelationIdWhenSupplied']) {
      if (!(key in value)) add(issues, 'MISSING_REQUIRED_FIELD', source, `${p}.${key}`, 'Dashboard state handling must declare this runtime-display behavior.');
      else boolValue(value[key], issues, source, `${p}.${key}`);
    }
  }
}

function validateProvenance(value, issues, source, p) {
  if (!assertObject(value, issues, source, p)) return;
  assertKeys(value, PROVENANCE_KEYS, issues, source, p);
  if ('sourceRef' in value) ref128(value.sourceRef, issues, source, `${p}.sourceRef`);
  if ('sourceVersion' in value) version64(value.sourceVersion, issues, source, `${p}.sourceVersion`);
  if ('notes' in value) safeString(value.notes, 1, 512, issues, source, `${p}.notes`);
}

function validateValuePolicy(value, issues, source, p) {
  if (!assertObject(value, issues, source, p)) return;
  assertKeys(value, VALUE_POLICY_KEYS, issues, source, p);
  requireKeys(value, ['methodologyRef', 'requireSampleSizeDisclosure', 'requireAssumptionDisclosure', 'unsupportedCausalClaimsProhibited', 'financialEstimatesRequireCustomerApprovedAssumptions'], issues, source, p);
  if ('methodologyRef' in value) ref128(value.methodologyRef, issues, source, `${p}.methodologyRef`);
  if ('customerAssumptionRef' in value) ref128(value.customerAssumptionRef, issues, source, `${p}.customerAssumptionRef`);
  for (const key of ['requireSampleSizeDisclosure', 'requireAssumptionDisclosure', 'unsupportedCausalClaimsProhibited', 'financialEstimatesRequireCustomerApprovedAssumptions']) {
    if (value[key] !== true) add(issues, 'VALUE_PRESENTATION_GUARDRAIL_REQUIRED', source, `${p}.${key}`, `${key} must be true when valuePresentationPolicy is present.`);
  }
}

function rectanglesOverlap(a, b) {
  return a.x < b.x + b.width && a.x + a.width > b.x && a.y < b.y + b.height && a.y + a.height > b.y;
}

function validateLayout(layout, issues, source, p) {
  if (!assertObject(layout, issues, source, p)) return false;
  assertKeys(layout, LAYOUT_KEYS, issues, source, p);
  requireKeys(layout, ['x', 'y', 'width', 'height'], issues, source, p);
  for (const key of ['x', 'y', 'width', 'height']) {
    const value = layout[key];
    const min = key === 'x' || key === 'y' ? 0 : 1;
    const max = key === 'x' || key === 'y' ? MAX_LAYOUT_COORDINATE : MAX_LAYOUT_DIMENSION;
    if (!Number.isSafeInteger(value) || value < min || value > max) add(issues, 'INVALID_LAYOUT', source, `${p}.${key}`, `Expected safe integer ${min}-${max}.`);
  }
  return ['x', 'y', 'width', 'height'].every((key) => Number.isSafeInteger(layout[key]));
}

function validateDashboard(document, issues, source) {
  assertKeys(document, DASHBOARD_KEYS, issues, source, '$');
  requireKeys(document, ['documentType', 'schemaVersion', 'definitionId', 'definitionVersion', 'displayName', 'origin', 'scope', 'systemDefinition', 'recoverableSystemDefinition', 'visibility', 'contextPolicy', 'queryBindings', 'variables', 'filters', 'views', 'panels', 'stateHandling', 'performance'], issues, source, '$');
  validateHeader(document, 'DASHBOARD_DEFINITION', issues, source);
  ref128(document.definitionId, issues, source, '$.definitionId');
  version64(document.definitionVersion, issues, source, '$.definitionVersion');
  safeString(document.displayName, 1, 255, issues, source, '$.displayName');
  if ('description' in document) safeString(document.description, 1, 2000, issues, source, '$.description');
  validateOrigin(document, issues, source);
  validateVisibility(document.visibility, issues, source, '$.visibility');
  const contexts = validateContextPolicy(document.contextPolicy, issues, source, '$.contextPolicy');
  const queries = validateQueryBindings(document.queryBindings, issues, source);
  const variables = validateVariables(document.variables, issues, source);
  const filters = validateFilters(document.filters, variables, contexts, issues, source);

  for (const key of ['folderRef']) if (key in document) ref128(document[key], issues, source, `$.${key}`);
  if ('category' in document) safeString(document.category, 1, 128, issues, source, '$.category');
  if ('tags' in document && arrayValue(document.tags, 0, 64, issues, source, '$.tags')) {
    uniquePrimitiveArray(document.tags, issues, source, '$.tags');
    document.tags.forEach((tag, index) => safeString(tag, 1, 128, issues, source, `$.tags[${index}]`));
  }

  const panels = collectById(document.panels, 'panelId', 256, PANEL_KEYS, ['panelId', 'title', 'type', 'layout', 'queryUses'], issues, source, '$.panels');
  const panelLayouts = new Map();
  for (const [panelId, panel] of panels) {
    const p = `$.panels[panelId=${panelId}]`;
    safeString(panel.title, 1, 255, issues, source, `${p}.title`);
    if ('description' in panel) safeString(panel.description, 1, 2000, issues, source, `${p}.description`);
    enumValue(panel.type, PANEL_TYPES, 'INVALID_PANEL_TYPE', issues, source, `${p}.type`);
    if (validateLayout(panel.layout, issues, source, `${p}.layout`)) panelLayouts.set(panelId, panel.layout);
    if (arrayValue(panel.queryUses, 0, 16, issues, source, `${p}.queryUses`)) panel.queryUses.forEach((use, index) => validateQueryUse(use, queries, variables, filters, contexts, issues, source, `${p}.queryUses[${index}]`));
    for (const key of ['transformationRefs', 'fieldOverrideRefs', 'filterRefs', 'variableRefs', 'actionRefs']) {
      if (key in panel && arrayValue(panel[key], 0, key === 'fieldOverrideRefs' || key === 'filterRefs' || key === 'variableRefs' ? 32 : 16, issues, source, `${p}.${key}`)) {
        uniquePrimitiveArray(panel[key], issues, source, `${p}.${key}`);
        panel[key].forEach((ref, index) => ref128(ref, issues, source, `${p}.${key}[${index}]`));
      }
    }
    for (const ref of panel.filterRefs ?? []) if (!filters.has(ref)) add(issues, 'UNDEFINED_FILTER_REFERENCE', source, `${p}.filterRefs`, `Undefined filter reference: ${ref}.`);
    for (const ref of panel.variableRefs ?? []) if (!variables.has(ref)) add(issues, 'UNDEFINED_VARIABLE_REFERENCE', source, `${p}.variableRefs`, `Undefined variable reference: ${ref}.`);
    for (const key of ['visualizationSettingsRef', 'thresholdPolicyRef', 'statePolicyRef']) if (key in panel) ref128(panel[key], issues, source, `${p}.${key}`);
    if ('drillDowns' in panel && arrayValue(panel.drillDowns, 0, 16, issues, source, `${p}.drillDowns`)) {
      const drillIds = new Set();
      panel.drillDowns.forEach((drill, index) => {
        validateDrillDown(drill, issues, source, `${p}.drillDowns[${index}]`);
        if (isObject(drill) && typeof drill.drillDownId === 'string') {
          if (drillIds.has(drill.drillDownId)) add(issues, 'DUPLICATE_DRILLDOWN_ID', source, `${p}.drillDowns[${index}].drillDownId`, `Duplicate drillDownId: ${drill.drillDownId}.`);
          drillIds.add(drill.drillDownId);
        }
      });
    }
    if ('visibility' in panel) validateVisibility(panel.visibility, issues, source, `${p}.visibility`);
  }

  const views = collectById(document.views, 'viewId', 32, VIEW_KEYS, ['viewId', 'label', 'panelRefs'], issues, source, '$.views');
  const panelMembership = new Map([...panels.keys()].map((id) => [id, new Set()]));
  for (const [viewId, view] of views) {
    safeString(view.label, 1, 128, issues, source, `$.views[viewId=${viewId}].label`);
    if (arrayValue(view.panelRefs, 0, 256, issues, source, `$.views[viewId=${viewId}].panelRefs`)) {
      uniquePrimitiveArray(view.panelRefs, issues, source, `$.views[viewId=${viewId}].panelRefs`);
      view.panelRefs.forEach((ref, index) => {
        ref128(ref, issues, source, `$.views[viewId=${viewId}].panelRefs[${index}]`);
        if (!panels.has(ref)) add(issues, 'UNDEFINED_PANEL_REFERENCE', source, `$.views[viewId=${viewId}].panelRefs[${index}]`, `Undefined panel reference: ${ref}.`);
        else panelMembership.get(ref).add(viewId);
      });
      for (let i = 0; i < view.panelRefs.length; i += 1) {
        for (let j = i + 1; j < view.panelRefs.length; j += 1) {
          const a = view.panelRefs[i];
          const b = view.panelRefs[j];
          if (panelLayouts.has(a) && panelLayouts.has(b) && rectanglesOverlap(panelLayouts.get(a), panelLayouts.get(b))) add(issues, 'LAYOUT_OVERLAP', source, `$.views[viewId=${viewId}].panelRefs`, `Panels ${a} and ${b} overlap in view ${viewId}.`);
        }
      }
    }
  }
  for (const [panelId, memberships] of panelMembership) if (memberships.size === 0) add(issues, 'ORPHAN_PANEL', source, '$.panels', `Panel ${panelId} is not referenced by any view.`);
  if ('defaultViewRef' in document) {
    ref128(document.defaultViewRef, issues, source, '$.defaultViewRef');
    if (!views.has(document.defaultViewRef)) add(issues, 'UNDEFINED_VIEW_REFERENCE', source, '$.defaultViewRef', `Undefined default view: ${document.defaultViewRef}.`);
  }

  if ('actionRefs' in document && arrayValue(document.actionRefs, 0, 32, issues, source, '$.actionRefs')) {
    uniquePrimitiveArray(document.actionRefs, issues, source, '$.actionRefs');
    document.actionRefs.forEach((ref, index) => ref128(ref, issues, source, `$.actionRefs[${index}]`));
  }
  if ('drillDowns' in document && arrayValue(document.drillDowns, 0, 32, issues, source, '$.drillDowns')) {
    const ids = new Set();
    document.drillDowns.forEach((drill, index) => {
      validateDrillDown(drill, issues, source, `$.drillDowns[${index}]`);
      if (isObject(drill) && typeof drill.drillDownId === 'string') {
        if (ids.has(drill.drillDownId)) add(issues, 'DUPLICATE_DRILLDOWN_ID', source, `$.drillDowns[${index}].drillDownId`, `Duplicate drillDownId: ${drill.drillDownId}.`);
        ids.add(drill.drillDownId);
      }
    });
  }

  validateStateHandling(document.stateHandling, issues, source, '$.stateHandling', true);
  if (assertObject(document.performance, issues, source, '$.performance')) {
    assertKeys(document.performance, PERFORMANCE_KEYS, issues, source, '$.performance');
    requireKeys(document.performance, ['serverSidePagination', 'serverSideFiltering', 'serverSideSorting', 'virtualizationHint'], issues, source, '$.performance');
    for (const key of ['serverSidePagination', 'serverSideFiltering', 'serverSideSorting', 'virtualizationHint']) if (key in document.performance) boolValue(document.performance[key], issues, source, `$.performance.${key}`);
    if ('queryPolicyRef' in document.performance) ref128(document.performance.queryPolicyRef, issues, source, '$.performance.queryPolicyRef');
  }
  if ('provenance' in document) validateProvenance(document.provenance, issues, source, '$.provenance');
  if ('valuePresentationPolicy' in document) validateValuePolicy(document.valuePresentationPolicy, issues, source, '$.valuePresentationPolicy');
}

function validateReport(document, issues, source) {
  assertKeys(document, REPORT_KEYS, issues, source, '$');
  requireKeys(document, ['documentType', 'schemaVersion', 'definitionId', 'definitionVersion', 'displayName', 'origin', 'scope', 'systemDefinition', 'recoverableSystemDefinition', 'visibility', 'contextPolicy', 'queryBindings', 'variables', 'filters', 'sections', 'allowedFormats'], issues, source, '$');
  validateHeader(document, 'REPORT_DEFINITION', issues, source);
  ref128(document.definitionId, issues, source, '$.definitionId');
  version64(document.definitionVersion, issues, source, '$.definitionVersion');
  safeString(document.displayName, 1, 255, issues, source, '$.displayName');
  if ('description' in document) safeString(document.description, 1, 2000, issues, source, '$.description');
  validateOrigin(document, issues, source);
  validateVisibility(document.visibility, issues, source, '$.visibility');
  const contexts = validateContextPolicy(document.contextPolicy, issues, source, '$.contextPolicy');
  const queries = validateQueryBindings(document.queryBindings, issues, source);
  const variables = validateVariables(document.variables, issues, source);
  const filters = validateFilters(document.filters, variables, contexts, issues, source);

  const sections = collectById(document.sections, 'sectionId', 128, SECTION_KEYS, ['sectionId', 'title', 'type', 'queryUses'], issues, source, '$.sections');
  for (const [sectionId, section] of sections) {
    const p = `$.sections[sectionId=${sectionId}]`;
    safeString(section.title, 1, 255, issues, source, `${p}.title`);
    if ('description' in section) safeString(section.description, 1, 2000, issues, source, `${p}.description`);
    enumValue(section.type, REPORT_SECTION_TYPES, 'INVALID_REPORT_SECTION_TYPE', issues, source, `${p}.type`);
    if (arrayValue(section.queryUses, 0, 16, issues, source, `${p}.queryUses`)) section.queryUses.forEach((use, index) => validateQueryUse(use, queries, variables, filters, contexts, issues, source, `${p}.queryUses[${index}]`));
    for (const key of ['tableDefinitionRef', 'visualizationDefinitionRef', 'imageRef', 'statePolicyRef']) if (key in section) ref128(section[key], issues, source, `${p}.${key}`);
    if ('text' in section) safeString(section.text, 1, 4000, issues, source, `${p}.text`);

    if (section.type === 'TEXT') {
      if (typeof section.text !== 'string') add(issues, 'REPORT_SECTION_REFERENCE_REQUIRED', source, `${p}.text`, 'TEXT section requires bounded plain text/Markdown metadata.');
      for (const key of ['tableDefinitionRef', 'visualizationDefinitionRef', 'imageRef']) if (key in section) add(issues, 'REPORT_SECTION_REFERENCE_CONFLICT', source, `${p}.${key}`, `TEXT section cannot carry ${key}.`);
    } else if (section.type === 'TABLE') {
      if (typeof section.tableDefinitionRef !== 'string') add(issues, 'REPORT_SECTION_REFERENCE_REQUIRED', source, `${p}.tableDefinitionRef`, 'TABLE section requires tableDefinitionRef.');
      if (!Array.isArray(section.queryUses) || section.queryUses.length === 0) add(issues, 'REPORT_SECTION_QUERY_REQUIRED', source, `${p}.queryUses`, 'TABLE section requires at least one governed query use.');
    } else if (section.type === 'VISUALIZATION') {
      if (typeof section.visualizationDefinitionRef !== 'string') add(issues, 'REPORT_SECTION_REFERENCE_REQUIRED', source, `${p}.visualizationDefinitionRef`, 'VISUALIZATION section requires visualizationDefinitionRef.');
      if (!Array.isArray(section.queryUses) || section.queryUses.length === 0) add(issues, 'REPORT_SECTION_QUERY_REQUIRED', source, `${p}.queryUses`, 'VISUALIZATION section requires at least one governed query use.');
    } else if (section.type === 'IMAGE_REFERENCE') {
      if (typeof section.imageRef !== 'string') add(issues, 'REPORT_SECTION_REFERENCE_REQUIRED', source, `${p}.imageRef`, 'IMAGE_REFERENCE section requires governed imageRef.');
      if (Array.isArray(section.queryUses) && section.queryUses.length > 0) add(issues, 'REPORT_SECTION_QUERY_CONFLICT', source, `${p}.queryUses`, 'IMAGE_REFERENCE section does not execute a query in this contract.');
    }
  }

  if (arrayValue(document.allowedFormats, 1, EXPORT_FORMATS.size, issues, source, '$.allowedFormats')) {
    const seen = new Set();
    document.allowedFormats.forEach((format, index) => {
      enumValue(format, EXPORT_FORMATS, 'INVALID_EXPORT_FORMAT', issues, source, `$.allowedFormats[${index}]`);
      if (seen.has(format)) add(issues, 'DUPLICATE_EXPORT_FORMAT', source, `$.allowedFormats[${index}]`, `Duplicate export format: ${format}.`);
      seen.add(format);
    });
  }
  if ('slaPolicyRef' in document) ref128(document.slaPolicyRef, issues, source, '$.slaPolicyRef');
  if ('schedule' in document) validateSchedule(document.schedule, issues, source, '$.schedule');
  if ('deliveryTargets' in document && arrayValue(document.deliveryTargets, 0, 32, issues, source, '$.deliveryTargets')) {
    const seen = new Set();
    document.deliveryTargets.forEach((target, index) => {
      const p = `$.deliveryTargets[${index}]`;
      if (!assertObject(target, issues, source, p)) return;
      assertKeys(target, DELIVERY_KEYS, issues, source, p);
      requireKeys(target, ['deliveryClass'], issues, source, p);
      enumValue(target.deliveryClass, DELIVERY_CLASSES, 'INVALID_DELIVERY_CLASS', issues, source, `${p}.deliveryClass`);
      if ('targetRef' in target) ref128(target.targetRef, issues, source, `${p}.targetRef`);
      if (['EMAIL', 'INTEGRATION'].includes(target.deliveryClass) && typeof target.targetRef !== 'string') add(issues, 'DELIVERY_TARGET_REFERENCE_REQUIRED', source, `${p}.targetRef`, `${target.deliveryClass} delivery requires a governed targetRef.`);
      if (target.deliveryClass === 'DOWNLOAD' && 'targetRef' in target) add(issues, 'DOWNLOAD_TARGET_REFERENCE_PROHIBITED', source, `${p}.targetRef`, 'DOWNLOAD delivery does not carry a recipient/integration target.');
      const key = `${target.deliveryClass}|${target.targetRef ?? ''}`;
      if (seen.has(key)) add(issues, 'DUPLICATE_DELIVERY_TARGET', source, p, 'Duplicate report delivery target.');
      seen.add(key);
    });
  }
  if ('stateHandling' in document) validateStateHandling(document.stateHandling, issues, source, '$.stateHandling', false);
  if ('provenance' in document) validateProvenance(document.provenance, issues, source, '$.provenance');
  if ('valuePresentationPolicy' in document) validateValuePolicy(document.valuePresentationPolicy, issues, source, '$.valuePresentationPolicy');
}

function validateSchedule(schedule, issues, source, p) {
  if (!assertObject(schedule, issues, source, p)) return;
  assertKeys(schedule, SCHEDULE_KEYS, issues, source, p);
  requireKeys(schedule, ['scheduleRef', 'timezone', 'enabled'], issues, source, p);
  if ('scheduleRef' in schedule) ref128(schedule.scheduleRef, issues, source, `${p}.scheduleRef`);
  if ('timezone' in schedule) {
    if (safeString(schedule.timezone, 1, 64, issues, source, `${p}.timezone`) && !TIMEZONE_RE.test(schedule.timezone)) add(issues, 'INVALID_SCHEDULE_METADATA', source, `${p}.timezone`, 'Timezone must be a bounded non-executable timezone identifier; runtime performs authoritative ZoneId validation.');
  }
  if ('enabled' in schedule) boolValue(schedule.enabled, issues, source, `${p}.enabled`);
}

function validateSavedView(document, issues, source) {
  assertKeys(document, SAVED_VIEW_KEYS, issues, source, '$');
  requireKeys(document, ['documentType', 'schemaVersion', 'savedViewId', 'definitionVersion', 'displayName', 'ownerRef', 'scopeRef', 'targetType', 'targetRef', 'filterState', 'timeContextPolicyRef', 'serverAuthorizationRequired'], issues, source, '$');
  validateHeader(document, 'SAVED_VIEW', issues, source);
  ref128(document.savedViewId, issues, source, '$.savedViewId');
  version64(document.definitionVersion, issues, source, '$.definitionVersion');
  safeString(document.displayName, 1, 255, issues, source, '$.displayName');
  if ('description' in document) safeString(document.description, 1, 2000, issues, source, '$.description');
  for (const key of ['ownerRef', 'scopeRef', 'targetRef', 'timeContextPolicyRef', 'sortRef', 'groupRef', 'viewModeRef']) if (key in document) ref128(document[key], issues, source, `$.${key}`);
  if (!new Set(['DASHBOARD', 'WORKSPACE']).has(document.targetType)) add(issues, 'INVALID_SAVED_VIEW_TARGET', source, '$.targetType', `Unsupported saved view target: ${String(document.targetType)}.`);
  if (document.serverAuthorizationRequired !== true) add(issues, 'SERVER_AUTHORIZATION_REQUIRED', source, '$.serverAuthorizationRequired', 'Saved views never grant data access; server authorization must be re-evaluated.');
  const seenFilters = new Set();
  if (arrayValue(document.filterState, 0, 64, issues, source, '$.filterState')) {
    document.filterState.forEach((state, index) => {
      const p = `$.filterState[${index}]`;
      if (!assertObject(state, issues, source, p)) return;
      assertKeys(state, SAVED_FILTER_KEYS, issues, source, p);
      requireKeys(state, ['filterRef', 'selectionRefs'], issues, source, p);
      if ('filterRef' in state && ref128(state.filterRef, issues, source, `${p}.filterRef`)) {
        if (seenFilters.has(state.filterRef)) add(issues, 'DUPLICATE_SAVED_FILTER', source, `${p}.filterRef`, `Duplicate saved filter: ${state.filterRef}.`);
        seenFilters.add(state.filterRef);
      }
      if (arrayValue(state.selectionRefs, 0, 64, issues, source, `${p}.selectionRefs`)) {
        uniquePrimitiveArray(state.selectionRefs, issues, source, `${p}.selectionRefs`);
        state.selectionRefs.forEach((ref, refIndex) => ref128(ref, issues, source, `${p}.selectionRefs[${refIndex}]`));
      }
    });
  }
  if ('provenance' in document) {
    if (assertObject(document.provenance, issues, source, '$.provenance')) {
      assertKeys(document.provenance, new Set(['sourceRef', 'sourceVersion']), issues, source, '$.provenance');
      if ('sourceRef' in document.provenance) ref128(document.provenance.sourceRef, issues, source, '$.provenance.sourceRef');
      if ('sourceVersion' in document.provenance) version64(document.provenance.sourceVersion, issues, source, '$.provenance.sourceVersion');
    }
  }
}

export function validateDocument(document, source = '<memory>') {
  const issues = [];
  if (!isObject(document)) {
    add(issues, 'INVALID_DOCUMENT', source, '$', 'Presentation contract document must be a JSON object.');
    return sortedIssues(issues);
  }
  let serialized = '';
  try { serialized = JSON.stringify(document); } catch { serialized = ''; }
  if (Buffer.byteLength(serialized, 'utf8') > MAX_DOCUMENT_BYTES) add(issues, 'DOCUMENT_TOO_LARGE', source, '$', `Document exceeds defensive ${MAX_DOCUMENT_BYTES}-byte bound.`);
  scanForbidden(document, issues, source);

  if (!DOCUMENT_TYPES.has(document.documentType)) {
    add(issues, 'UNKNOWN_DOCUMENT_TYPE', source, '$.documentType', `Unknown presentation documentType: ${String(document.documentType)}.`);
    return sortedIssues(issues);
  }
  if (document.documentType === 'DASHBOARD_DEFINITION') validateDashboard(document, issues, source);
  else if (document.documentType === 'REPORT_DEFINITION') validateReport(document, issues, source);
  else validateSavedView(document, issues, source);
  return sortedIssues(issues);
}

export function validateDocumentSet(entries) {
  const issues = [];
  const docs = entries.map((entry) => ({ document: entry.document, source: entry.source ?? '<memory>' }));
  docs.forEach(({ document, source }) => issues.push(...validateDocument(document, source)));

  const definitionIds = new Map();
  const dashboards = new Set();
  for (const entry of docs) {
    const doc = entry.document;
    if (!isObject(doc)) continue;
    const id = doc.documentType === 'SAVED_VIEW' ? doc.savedViewId : doc.definitionId;
    if (typeof id === 'string') {
      const key = `${doc.documentType}|${id}`;
      if (definitionIds.has(key)) add(issues, 'DUPLICATE_DEFINITION_ID', entry.source, doc.documentType === 'SAVED_VIEW' ? '$.savedViewId' : '$.definitionId', `Duplicate ${doc.documentType} identifier ${id}; first seen in ${definitionIds.get(key)}.`);
      else definitionIds.set(key, entry.source);
    }
    if (doc.documentType === 'DASHBOARD_DEFINITION' && typeof doc.definitionId === 'string') dashboards.add(doc.definitionId);
  }

  if (dashboards.size > 0) {
    for (const entry of docs) {
      const doc = entry.document;
      if (isObject(doc) && doc.documentType === 'SAVED_VIEW' && doc.targetType === 'DASHBOARD' && typeof doc.targetRef === 'string' && !dashboards.has(doc.targetRef)) add(issues, 'SAVED_VIEW_TARGET_NOT_FOUND', entry.source, '$.targetRef', `Saved view target dashboard ${doc.targetRef} is not present in this validated document set.`);
    }
  }

  return sortedIssues(issues);
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
      else if (entry.isFile() && entry.name.endsWith('.json') && !entry.name.endsWith('.schema.json') && full !== path.join(ROOT, 'fixtures', 'cases.json')) files.push(full);
      if (files.length > MAX_DISCOVERED_DOCUMENTS) throw new Error(`More than ${MAX_DISCOVERED_DOCUMENTS} presentation documents discovered; validation is bounded.`);
    }
  };
  walk(resolved);
  return files;
}

function printIssues(issues) {
  for (const issue of sortedIssues(issues)) console.error(`${issue.code}\t${issue.source}\t${issue.path}\t${issue.message}`);
}

function validateFiles(inputs) {
  const files = inputs.flatMap(discoverJsonFiles);
  if (files.length === 0) throw new Error('No presentation definition JSON documents found.');
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
  console.log(`VALID: ${entries.length} presentation contract document(s).`);
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
  } else if (mutation.op === 'appendCopy') {
    if (!Array.isArray(mutation.sourcePath)) throw new Error('appendCopy requires sourcePath[].');
    const target = getAt(document, mutation.path);
    if (!Array.isArray(target)) throw new Error('appendCopy target must be an array.');
    target.push(clone(getAt(document, mutation.sourcePath)));
  } else if (mutation.op === 'append') {
    const target = getAt(document, mutation.path);
    if (!Array.isArray(target)) throw new Error('append target must be an array.');
    target.push(clone(mutation.value));
  } else if (mutation.op === 'swap') {
    if (!Array.isArray(mutation.otherPath)) throw new Error('swap requires otherPath[].');
    const a = clone(getAt(document, mutation.path));
    const b = clone(getAt(document, mutation.otherPath));
    const pa = parentAt(document, mutation.path);
    const pb = parentAt(document, mutation.otherPath);
    pa.parent[pa.key] = b;
    pb.parent[pb.key] = a;
  } else {
    throw new Error(`Unknown fixture mutation op: ${String(mutation.op)}.`);
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
    if (!isObject(fixture) || typeof fixture.name !== 'string') throw new Error('Each fixture requires name.');
    const baseNames = Array.isArray(fixture.bases) ? fixture.bases : [fixture.base];
    if (baseNames.some((name) => typeof name !== 'string' || !bases.has(name))) throw new Error(`Fixture ${fixture.name} references unknown base.`);
    const documents = baseNames.map((name) => clone(bases.get(name)));
    for (const mutation of fixture.mutations ?? []) {
      const index = mutation.documentIndex ?? 0;
      if (!Number.isInteger(index) || index < 0 || index >= documents.length) throw new Error(`Fixture ${fixture.name} mutation documentIndex out of range.`);
      applyMutation(documents[index], mutation);
    }
    const entries = documents.map((document, index) => ({ document, source: `fixture:${fixture.name}:${index}` }));
    const issues = validateDocumentSet(entries);
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

#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, '..');
const CONTRACT_VERSION = '1.0.0';
const MAX_MANIFEST_BYTES = 256 * 1024;
const MAX_DISCOVERED_MANIFESTS = 256;

const PROTOCOLS = new Set([
  'ICMP', 'SNMP', 'SNMP_TRAP', 'SYSLOG', 'SSH_CLI', 'TELNET', 'NETCONF', 'REST',
  'RESTCONF', 'GNMI', 'GRPC', 'FLOW', 'DNS', 'HTTP', 'HTTPS', 'TLS', 'REDFISH',
  'WINRM', 'CLOUD_API', 'OTLP', 'SCP', 'SFTP', 'TFTP', 'FTP',
]);
const LEGACY_PROTOCOLS = new Set(['TELNET', 'TFTP', 'FTP']);
const TRANSPORT_MODES = new Set(['IP', 'UDP', 'TCP', 'TLS', 'HTTPS', 'SSH', 'GRPC', 'MTLS']);
const SECURITY_MODES = new Set([
  'TLS', 'MTLS', 'SSHV2', 'SNMPV3', 'KERBEROS', 'NEGOTIATE', 'NTLM', 'BASIC',
  'BEARER', 'API_KEY', 'OAUTH2', 'PUBLIC_KEY', 'PASSWORD', 'KEYBOARD_INTERACTIVE',
]);
const CREDENTIAL_TYPES = new Set(['SNMP', 'SSH', 'WINRM', 'REST', 'GNMI', 'DB', 'CLOUD', 'REDFISH']);
const CERTIFICATIONS = new Set(['OFFICIAL_CERTIFIED', 'PARTNER_VERIFIED', 'CUSTOMER_CUSTOM', 'EXPERIMENTAL']);
const PUBLISHER_TYPES = new Set(['PLATFORM', 'PARTNER', 'CUSTOMER']);
const SOURCE_TYPES = new Set(['PLATFORM_SUPPLIED', 'PARTNER_SUPPLIED', 'CUSTOMER_SUPPLIED']);
const REDISTRIBUTION = new Set(['ALLOWED', 'RESTRICTED', 'UNKNOWN']);
const MATCH_FIELDS = new Set(['vendor', 'family', 'product', 'model', 'platform', 'software', 'firmware', 'api', 'mib']);
const MATCH_OPERATORS = new Set(['EXACT', 'GLOB']);
const REFERENCE_KINDS = new Set(['MIB', 'YANG', 'OPENAPI', 'MODEL', 'OTHER']);
const VALUE_TYPES = new Set(['BOOLEAN', 'INTEGER', 'NUMBER', 'STRING', 'STATE']);
const TRANSFORM_TYPES = new Set(['IDENTITY', 'SCALE', 'OFFSET']);

const TOP_KEYS = new Set([
  'schemaVersion', 'packId', 'packVersion', 'name', 'publisher', 'certification', 'metadata',
  'matchRules', 'capabilities', 'protocols', 'references', 'normalization', 'dependencies', 'provenance',
]);
const FORBIDDEN_SECRET_KEYS = new Set([
  'password', 'passwd', 'community', 'secret', 'apitoken', 'accesstoken', 'refreshtoken',
  'bearertoken', 'privatekey', 'clientsecret', 'cookie', 'cookies', 'credentialvalue',
  'credentialprofileid', 'authorization',
]);
const FORBIDDEN_EXECUTABLE_KEYS = new Set([
  'script', 'scripts', 'shell', 'command', 'commands', 'exec', 'executable', 'javascript',
  'python', 'code', 'inlinecode',
]);

const PACK_ID_RE = /^[a-z0-9]+(?:[.-][a-z0-9]+)*$/;
const SHORT_ID_RE = /^[a-z][a-z0-9-]*$/;
const SEMANTIC_KEY_RE = /^[a-z][a-z0-9]*(?:\.[a-z0-9][a-z0-9_-]*)+$/;
const ENTITY_TYPE_RE = /^[a-z][a-z0-9_.-]*$/;
const SEMVER_RE = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);
const token = (k) => k.toLowerCase().replace(/[_\-.]/g, '');
const issue = (issues, code, source, jsonPath, message) => issues.push({ code, source, path: jsonPath, message });

function keys(object, allowed, required, issues, source, jsonPath) {
  if (!isObject(object)) {
    issue(issues, 'INVALID_TYPE', source, jsonPath, 'Expected object.');
    return false;
  }
  for (const key of Object.keys(object)) {
    if (!allowed.has(key)) issue(issues, 'UNKNOWN_FIELD', source, `${jsonPath}.${key}`, 'Field is not defined by the Device Pack contract.');
  }
  for (const key of required) {
    if (!(key in object)) issue(issues, 'MISSING_REQUIRED_FIELD', source, `${jsonPath}.${key}`, 'Required field is missing.');
  }
  return true;
}

function stringValue(value, min, max, issues, source, jsonPath) {
  if (typeof value !== 'string') {
    issue(issues, 'INVALID_TYPE', source, jsonPath, 'Expected string.');
    return false;
  }
  if (value.length < min || value.length > max) {
    issue(issues, 'STRING_BOUNDS', source, jsonPath, `String length must be ${min}-${max}.`);
    return false;
  }
  if (/\p{Cc}/u.test(value)) issue(issues, 'CONTROL_CHARACTER', source, jsonPath, 'Control characters are prohibited.');
  return true;
}

function arrayValue(value, min, max, issues, source, jsonPath) {
  if (!Array.isArray(value)) {
    issue(issues, 'INVALID_TYPE', source, jsonPath, 'Expected array.');
    return false;
  }
  if (value.length < min || value.length > max) issue(issues, 'ARRAY_BOUNDS', source, jsonPath, `Array length must be ${min}-${max}.`);
  return true;
}

function enumValue(value, allowed, issues, source, jsonPath, code = 'UNKNOWN_ENUM') {
  if (!allowed.has(value)) {
    issue(issues, code, source, jsonPath, `Unsupported value: ${String(value)}.`);
    return false;
  }
  return true;
}

const validPackId = (v) => typeof v === 'string' && v.length >= 3 && v.length <= 128 && PACK_ID_RE.test(v);
const validShortId = (v) => typeof v === 'string' && v.length >= 1 && v.length <= 64 && SHORT_ID_RE.test(v);
const validSemver = (v) => typeof v === 'string' && v.length >= 5 && v.length <= 64 && SEMVER_RE.test(v);
const validSemanticKey = (v) => typeof v === 'string' && v.length >= 3 && v.length <= 128 && SEMANTIC_KEY_RE.test(v);

function safePackagePath(value) {
  if (typeof value !== 'string' || value.length < 1 || value.length > 240) return false;
  const lowered = value.toLowerCase();
  if (path.posix.isAbsolute(value) || path.win32.isAbsolute(value) || value.includes('\\')) return false;
  if (lowered.includes('%2e') || lowered.includes('%2f') || lowered.includes('%5c')) return false;
  const parts = value.split('/');
  return !parts.some((part) => part === '' || part === '.' || part === '..');
}

function absoluteFilesystemReference(value) {
  return typeof value === 'string' && (
    path.posix.isAbsolute(value) || path.win32.isAbsolute(value) || /^file:/i.test(value)
  );
}

function scanForbidden(value, issues, source, jsonPath = '$') {
  if (Array.isArray(value)) {
    value.forEach((child, index) => scanForbidden(child, issues, source, `${jsonPath}[${index}]`));
    return;
  }
  if (!isObject(value)) return;

  for (const [key, child] of Object.entries(value)) {
    const t = token(key);
    const childPath = `${jsonPath}.${key}`;
    if (FORBIDDEN_SECRET_KEYS.has(t)) issue(issues, 'FORBIDDEN_SECRET_FIELD', source, childPath, 'Credential secret values and Credential Profile IDs are prohibited.');
    if (FORBIDDEN_EXECUTABLE_KEYS.has(t)) issue(issues, 'FORBIDDEN_EXECUTABLE_FIELD', source, childPath, 'Executable/script/command content is prohibited.');
    if (t === 'packagepath' && !safePackagePath(child)) issue(issues, 'PATH_TRAVERSAL', source, childPath, 'Package path must be a safe relative POSIX path.');
    if (t === 'sourcereference' && absoluteFilesystemReference(child)) issue(issues, 'ABSOLUTE_FILESYSTEM_REFERENCE', source, childPath, 'Absolute filesystem references are prohibited.');

    if (typeof child === 'string') {
      if (/-----BEGIN [A-Z ]*PRIVATE KEY-----/.test(child)) issue(issues, 'FORBIDDEN_SECRET_VALUE', source, childPath, 'Private-key material is prohibited.');
      if (/:\/\/[^/\s:@]+:[^/\s@]+@/.test(child)) issue(issues, 'EMBEDDED_URL_CREDENTIAL', source, childPath, 'Credentials embedded in URLs are prohibited.');
      if (/^Bearer\s+\S+/i.test(child) || /[?&](?:token|api[_-]?key|password|secret)=/i.test(child)) issue(issues, 'FORBIDDEN_SECRET_VALUE', source, childPath, 'Token/secret material is prohibited.');
    }
    scanForbidden(child, issues, source, childPath);
  }
}

function unique(items, selector, code, issues, source, jsonPath) {
  const seen = new Set();
  items.forEach((item, index) => {
    const value = selector(item);
    if (typeof value !== 'string' && typeof value !== 'number') return;
    if (seen.has(value)) issue(issues, code, source, `${jsonPath}[${index}]`, `Duplicate value: ${String(value)}.`);
    else seen.add(value);
  });
}

function ruleSignature(rule) {
  if (!isObject(rule) || !Array.isArray(rule.conditions)) return 'invalid';
  const parts = rule.conditions.map((c) => `${c?.field}|${c?.operator}|${c?.caseSensitive}|${c?.value}`).sort();
  return `${rule.priority}|${parts.join('::')}`;
}

function validatePublisher(value, issues, source) {
  const p = '$.publisher';
  if (!keys(value, new Set(['type', 'name']), ['type', 'name'], issues, source, p)) return;
  if ('type' in value) enumValue(value.type, PUBLISHER_TYPES, issues, source, `${p}.type`);
  if ('name' in value) stringValue(value.name, 1, 160, issues, source, `${p}.name`);
}

function validateMetadata(value, issues, source) {
  if (value === undefined) return;
  const p = '$.metadata';
  if (!keys(value, new Set(['vendor', 'family', 'product', 'description']), [], issues, source, p)) return;
  for (const key of ['vendor', 'family', 'product']) if (key in value) stringValue(value[key], 1, 160, issues, source, `${p}.${key}`);
  if ('description' in value) stringValue(value.description, 1, 512, issues, source, `${p}.description`);
}

function validateMatchRules(value, issues, source) {
  const p = '$.matchRules';
  if (!arrayValue(value, 1, 64, issues, source, p)) return;
  unique(value, (r) => r?.ruleId, 'DUPLICATE_MATCH_RULE_ID', issues, source, p);
  unique(value, (r) => r?.priority, 'AMBIGUOUS_MATCH_PRIORITY', issues, source, p);
  const signatures = new Set();

  value.forEach((rule, i) => {
    const rp = `${p}[${i}]`;
    if (!keys(rule, new Set(['ruleId', 'priority', 'conditions']), ['ruleId', 'priority', 'conditions'], issues, source, rp)) return;
    if ('ruleId' in rule && !validShortId(rule.ruleId)) issue(issues, 'INVALID_MATCH_RULE_ID', source, `${rp}.ruleId`, 'Invalid match rule ID.');
    if ('priority' in rule && (!Number.isInteger(rule.priority) || rule.priority < 0 || rule.priority > 1000)) issue(issues, 'INVALID_MATCH_PRIORITY', source, `${rp}.priority`, 'Priority must be an integer from 0 to 1000.');
    if (!arrayValue(rule.conditions, 1, 16, issues, source, `${rp}.conditions`)) return;

    const seenFields = new Set();
    rule.conditions.forEach((condition, j) => {
      const cp = `${rp}.conditions[${j}]`;
      if (!keys(condition, new Set(['field', 'operator', 'value', 'caseSensitive']), ['field', 'operator', 'value', 'caseSensitive'], issues, source, cp)) return;
      if ('field' in condition) {
        enumValue(condition.field, MATCH_FIELDS, issues, source, `${cp}.field`, 'UNKNOWN_MATCH_FIELD');
        if (seenFields.has(condition.field)) issue(issues, 'DUPLICATE_MATCH_FIELD', source, `${cp}.field`, 'A match rule may declare a selector field only once.');
        seenFields.add(condition.field);
      }
      if ('operator' in condition) enumValue(condition.operator, MATCH_OPERATORS, issues, source, `${cp}.operator`, 'UNKNOWN_MATCH_OPERATOR');
      if ('value' in condition && stringValue(condition.value, 1, 160, issues, source, `${cp}.value`)) {
        if (condition.operator === 'EXACT' && /[*?]/.test(condition.value)) issue(issues, 'EXACT_MATCH_HAS_WILDCARD', source, `${cp}.value`, 'EXACT values must be literal.');
        if (condition.operator === 'GLOB' && (/[\[\]{}()|^$\\]/.test(condition.value) || condition.value.includes('**'))) issue(issues, 'UNSAFE_GLOB', source, `${cp}.value`, 'GLOB supports only bounded * and ? wildcards; regex syntax and ** are rejected.');
      }
      if ('caseSensitive' in condition && typeof condition.caseSensitive !== 'boolean') issue(issues, 'INVALID_TYPE', source, `${cp}.caseSensitive`, 'Expected boolean.');
    });

    const signature = ruleSignature(rule);
    if (signatures.has(signature)) issue(issues, 'DUPLICATE_MATCH_RULE', source, rp, 'Equivalent match rule already exists in this pack.');
    signatures.add(signature);
  });
}

function validateCapabilities(value, issues, source) {
  const p = '$.capabilities';
  if (!arrayValue(value, 0, 128, issues, source, p)) return;
  unique(value, (c) => c?.id, 'DUPLICATE_CAPABILITY', issues, source, p);
  value.forEach((capability, i) => {
    const cp = `${p}[${i}]`;
    if (!keys(capability, new Set(['id', 'description', 'requiresProtocols']), ['id'], issues, source, cp)) return;
    if ('id' in capability && !validSemanticKey(capability.id)) issue(issues, 'INVALID_CAPABILITY_ID', source, `${cp}.id`, 'Capability ID must use dotted lower-case form.');
    if ('description' in capability) stringValue(capability.description, 1, 512, issues, source, `${cp}.description`);
    if ('requiresProtocols' in capability && arrayValue(capability.requiresProtocols, 0, 16, issues, source, `${cp}.requiresProtocols`)) {
      unique(capability.requiresProtocols, (v) => v, 'DUPLICATE_PROTOCOL_REFERENCE', issues, source, `${cp}.requiresProtocols`);
      capability.requiresProtocols.forEach((family, j) => enumValue(family, PROTOCOLS, issues, source, `${cp}.requiresProtocols[${j}]`, 'UNKNOWN_PROTOCOL'));
    }
  });
}

function validateProtocols(value, issues, source) {
  const p = '$.protocols';
  if (!arrayValue(value, 0, 32, issues, source, p)) return;
  unique(value, (v) => v?.family, 'DUPLICATE_PROTOCOL', issues, source, p);

  value.forEach((protocol, i) => {
    const pp = `${p}[${i}]`;
    const allowed = new Set(['family', 'requirement', 'legacy', 'suggestedPort', 'transportModes', 'securityModes', 'credentialProfileType', 'prerequisiteCapabilities']);
    if (!keys(protocol, allowed, ['family', 'requirement', 'legacy'], issues, source, pp)) return;
    if ('family' in protocol) enumValue(protocol.family, PROTOCOLS, issues, source, `${pp}.family`, 'UNKNOWN_PROTOCOL');
    if ('requirement' in protocol && !new Set(['REQUIRED', 'OPTIONAL']).has(protocol.requirement)) issue(issues, 'UNKNOWN_REQUIREMENT', source, `${pp}.requirement`, 'Protocol requirement must be REQUIRED or OPTIONAL.');
    if ('legacy' in protocol && typeof protocol.legacy !== 'boolean') issue(issues, 'INVALID_TYPE', source, `${pp}.legacy`, 'Expected boolean.');
    if (LEGACY_PROTOCOLS.has(protocol.family)) {
      if (protocol.legacy !== true) issue(issues, 'LEGACY_PROTOCOL_NOT_FLAGGED', source, `${pp}.legacy`, `${protocol.family} must be explicitly marked legacy.`);
      if (protocol.requirement === 'REQUIRED') issue(issues, 'LEGACY_PROTOCOL_CANNOT_BE_REQUIRED', source, `${pp}.requirement`, `${protocol.family} cannot be required by a Device Pack.`);
    }
    if ('suggestedPort' in protocol && (!Number.isInteger(protocol.suggestedPort) || protocol.suggestedPort < 1 || protocol.suggestedPort > 65535)) issue(issues, 'INVALID_PORT', source, `${pp}.suggestedPort`, 'Suggested port must be 1-65535.');
    if ('transportModes' in protocol && arrayValue(protocol.transportModes, 0, 8, issues, source, `${pp}.transportModes`)) {
      unique(protocol.transportModes, (v) => v, 'DUPLICATE_TRANSPORT_MODE', issues, source, `${pp}.transportModes`);
      protocol.transportModes.forEach((v, j) => enumValue(v, TRANSPORT_MODES, issues, source, `${pp}.transportModes[${j}]`, 'UNKNOWN_TRANSPORT_MODE'));
    }
    if ('securityModes' in protocol && arrayValue(protocol.securityModes, 0, 12, issues, source, `${pp}.securityModes`)) {
      unique(protocol.securityModes, (v) => v, 'DUPLICATE_SECURITY_MODE', issues, source, `${pp}.securityModes`);
      protocol.securityModes.forEach((v, j) => enumValue(v, SECURITY_MODES, issues, source, `${pp}.securityModes[${j}]`, 'UNKNOWN_SECURITY_MODE'));
    }
    if ('credentialProfileType' in protocol) enumValue(protocol.credentialProfileType, CREDENTIAL_TYPES, issues, source, `${pp}.credentialProfileType`, 'UNKNOWN_CREDENTIAL_PROFILE_TYPE');
    if ('prerequisiteCapabilities' in protocol && arrayValue(protocol.prerequisiteCapabilities, 0, 16, issues, source, `${pp}.prerequisiteCapabilities`)) {
      unique(protocol.prerequisiteCapabilities, (v) => v, 'DUPLICATE_CAPABILITY_REFERENCE', issues, source, `${pp}.prerequisiteCapabilities`);
      protocol.prerequisiteCapabilities.forEach((id, j) => {
        if (!validSemanticKey(id)) issue(issues, 'INVALID_CAPABILITY_ID', source, `${pp}.prerequisiteCapabilities[${j}]`, 'Invalid capability reference.');
      });
    }
  });
}

function validateReferences(value, issues, source) {
  const p = '$.references';
  if (!arrayValue(value, 0, 256, issues, source, p)) return;
  unique(value, (r) => r?.id, 'DUPLICATE_REFERENCE', issues, source, p);
  value.forEach((reference, i) => {
    const rp = `${p}[${i}]`;
    const allowed = new Set(['id', 'kind', 'version', 'packagePath', 'sourceReference', 'provenanceId']);
    if (!keys(reference, allowed, ['id', 'kind', 'provenanceId'], issues, source, rp)) return;
    if ('id' in reference && !validShortId(reference.id)) issue(issues, 'INVALID_REFERENCE_ID', source, `${rp}.id`, 'Invalid reference ID.');
    if ('kind' in reference) enumValue(reference.kind, REFERENCE_KINDS, issues, source, `${rp}.kind`, 'UNKNOWN_REFERENCE_KIND');
    if ('version' in reference) stringValue(reference.version, 1, 80, issues, source, `${rp}.version`);
    if ('packagePath' in reference && !safePackagePath(reference.packagePath)) issue(issues, 'PATH_TRAVERSAL', source, `${rp}.packagePath`, 'Package path must be safe and relative.');
    if ('sourceReference' in reference) {
      stringValue(reference.sourceReference, 1, 512, issues, source, `${rp}.sourceReference`);
      if (absoluteFilesystemReference(reference.sourceReference)) issue(issues, 'ABSOLUTE_FILESYSTEM_REFERENCE', source, `${rp}.sourceReference`, 'Absolute filesystem references are prohibited.');
    }
    if ('provenanceId' in reference && !validShortId(reference.provenanceId)) issue(issues, 'INVALID_PROVENANCE_ID', source, `${rp}.provenanceId`, 'Invalid provenance reference.');
    if (!('packagePath' in reference) && !('sourceReference' in reference)) issue(issues, 'REFERENCE_LOCATION_REQUIRED', source, rp, 'Reference requires packagePath or sourceReference metadata.');
  });
}

function validateTransform(value, issues, source, p) {
  if (!keys(value, new Set(['type', 'factor', 'offset']), ['type'], issues, source, p)) return;
  if (!('type' in value) || !enumValue(value.type, TRANSFORM_TYPES, issues, source, `${p}.type`, 'UNKNOWN_TRANSFORM')) return;
  if (value.type === 'IDENTITY' && ('factor' in value || 'offset' in value)) issue(issues, 'INVALID_TRANSFORM_PARAMETERS', source, p, 'IDENTITY accepts no parameters.');
  if (value.type === 'SCALE') {
    if (typeof value.factor !== 'number' || !Number.isFinite(value.factor)) issue(issues, 'INVALID_TRANSFORM_PARAMETERS', source, `${p}.factor`, 'SCALE requires a finite factor.');
    if ('offset' in value) issue(issues, 'INVALID_TRANSFORM_PARAMETERS', source, `${p}.offset`, 'SCALE does not accept offset.');
  }
  if (value.type === 'OFFSET') {
    if (typeof value.offset !== 'number' || !Number.isFinite(value.offset)) issue(issues, 'INVALID_TRANSFORM_PARAMETERS', source, `${p}.offset`, 'OFFSET requires a finite offset.');
    if ('factor' in value) issue(issues, 'INVALID_TRANSFORM_PARAMETERS', source, `${p}.factor`, 'OFFSET does not accept factor.');
  }
}

function validateNormalization(value, issues, source) {
  const p = '$.normalization';
  if (!arrayValue(value, 0, 512, issues, source, p)) return;
  unique(value, (n) => n?.key, 'NORMALIZATION_COLLISION', issues, source, p);
  value.forEach((mapping, i) => {
    const mp = `${p}[${i}]`;
    const allowed = new Set(['key', 'source', 'valueType', 'unit', 'target', 'qualityFallback', 'provenanceId', 'transform']);
    if (!keys(mapping, allowed, ['key', 'source', 'valueType', 'target', 'qualityFallback', 'provenanceId'], issues, source, mp)) return;
    if ('key' in mapping && !validSemanticKey(mapping.key)) issue(issues, 'INVALID_NORMALIZATION_KEY', source, `${mp}.key`, 'Normalization key must use dotted lower-case form.');
    if ('valueType' in mapping) enumValue(mapping.valueType, VALUE_TYPES, issues, source, `${mp}.valueType`, 'UNKNOWN_VALUE_TYPE');
    if ('unit' in mapping) stringValue(mapping.unit, 1, 32, issues, source, `${mp}.unit`);
    if ('qualityFallback' in mapping && mapping.qualityFallback !== 'UNKNOWN') issue(issues, 'UNSAFE_QUALITY_FALLBACK', source, `${mp}.qualityFallback`, 'Fallback quality may only be UNKNOWN; packs must not fabricate healthy data.');
    if ('provenanceId' in mapping && !validShortId(mapping.provenanceId)) issue(issues, 'INVALID_PROVENANCE_ID', source, `${mp}.provenanceId`, 'Invalid provenance reference.');

    if ('source' in mapping) {
      const sp = `${mp}.source`;
      if (keys(mapping.source, new Set(['protocol', 'field', 'referenceId']), ['protocol', 'field'], issues, source, sp)) {
        if ('protocol' in mapping.source) enumValue(mapping.source.protocol, PROTOCOLS, issues, source, `${sp}.protocol`, 'UNKNOWN_PROTOCOL');
        if ('field' in mapping.source) stringValue(mapping.source.field, 1, 256, issues, source, `${sp}.field`);
        if ('referenceId' in mapping.source && !validShortId(mapping.source.referenceId)) issue(issues, 'INVALID_REFERENCE_ID', source, `${sp}.referenceId`, 'Invalid reference ID.');
      }
    }

    if ('target' in mapping) {
      const tp = `${mp}.target`;
      if (keys(mapping.target, new Set(['entityType', 'component']), ['entityType'], issues, source, tp)) {
        if ('entityType' in mapping.target) {
          const okay = stringValue(mapping.target.entityType, 1, 80, issues, source, `${tp}.entityType`);
          if (okay && !ENTITY_TYPE_RE.test(mapping.target.entityType)) issue(issues, 'INVALID_ENTITY_TYPE', source, `${tp}.entityType`, 'Invalid entity type identifier.');
        }
        if ('component' in mapping.target) stringValue(mapping.target.component, 1, 160, issues, source, `${tp}.component`);
      }
    }
    if ('transform' in mapping) validateTransform(mapping.transform, issues, source, `${mp}.transform`);
  });
}

function validateDependencies(value, issues, source) {
  const p = '$.dependencies';
  if (!arrayValue(value, 0, 64, issues, source, p)) return;
  const seen = new Set();
  value.forEach((dependency, i) => {
    const dp = `${p}[${i}]`;
    if (!keys(dependency, new Set(['packId', 'packVersion']), ['packId', 'packVersion'], issues, source, dp)) return;
    if ('packId' in dependency && !validPackId(dependency.packId)) issue(issues, 'INVALID_PACK_ID', source, `${dp}.packId`, 'Invalid dependency packId.');
    if ('packVersion' in dependency && !validSemver(dependency.packVersion)) issue(issues, 'MALFORMED_VERSION', source, `${dp}.packVersion`, 'Dependency version must use SemVer shape.');
    const identity = `${dependency.packId}@${dependency.packVersion}`;
    if (seen.has(identity)) issue(issues, 'DUPLICATE_DEPENDENCY', source, dp, `Duplicate dependency ${identity}.`);
    seen.add(identity);
  });
}

function validateProvenance(value, issues, source) {
  const p = '$.provenance';
  if (!arrayValue(value, 1, 128, issues, source, p)) return;
  unique(value, (v) => v?.id, 'DUPLICATE_PROVENANCE', issues, source, p);
  value.forEach((entry, i) => {
    const ep = `${p}[${i}]`;
    const allowed = new Set(['id', 'sourceType', 'sourceReference', 'license', 'redistribution']);
    if (!keys(entry, allowed, ['id', 'sourceType', 'sourceReference', 'redistribution'], issues, source, ep)) return;
    if ('id' in entry && !validShortId(entry.id)) issue(issues, 'INVALID_PROVENANCE_ID', source, `${ep}.id`, 'Invalid provenance ID.');
    if ('sourceType' in entry) enumValue(entry.sourceType, SOURCE_TYPES, issues, source, `${ep}.sourceType`, 'UNKNOWN_SOURCE_TYPE');
    if ('sourceReference' in entry) {
      stringValue(entry.sourceReference, 1, 512, issues, source, `${ep}.sourceReference`);
      if (absoluteFilesystemReference(entry.sourceReference)) issue(issues, 'ABSOLUTE_FILESYSTEM_REFERENCE', source, `${ep}.sourceReference`, 'Absolute filesystem references are prohibited.');
    }
    if ('license' in entry) stringValue(entry.license, 1, 160, issues, source, `${ep}.license`);
    if ('redistribution' in entry) enumValue(entry.redistribution, REDISTRIBUTION, issues, source, `${ep}.redistribution`, 'UNKNOWN_REDISTRIBUTION');
  });
}

function validateInternalReferences(manifest, issues, source) {
  const protocols = new Set(Array.isArray(manifest.protocols) ? manifest.protocols.map((v) => v?.family).filter((v) => PROTOCOLS.has(v)) : []);
  const capabilities = new Set(Array.isArray(manifest.capabilities) ? manifest.capabilities.map((v) => v?.id).filter((v) => validSemanticKey(v)) : []);
  const references = new Set(Array.isArray(manifest.references) ? manifest.references.map((v) => v?.id).filter((v) => validShortId(v)) : []);
  const provenance = new Set(Array.isArray(manifest.provenance) ? manifest.provenance.map((v) => v?.id).filter((v) => validShortId(v)) : []);

  if (Array.isArray(manifest.capabilities)) manifest.capabilities.forEach((capability, i) => {
    if (!Array.isArray(capability?.requiresProtocols)) return;
    capability.requiresProtocols.forEach((family, j) => {
      if (PROTOCOLS.has(family) && !protocols.has(family)) issue(issues, 'CAPABILITY_PROTOCOL_NOT_DECLARED', source, `$.capabilities[${i}].requiresProtocols[${j}]`, `Protocol ${family} is not declared.`);
    });
  });

  if (Array.isArray(manifest.protocols)) manifest.protocols.forEach((protocol, i) => {
    if (!Array.isArray(protocol?.prerequisiteCapabilities)) return;
    protocol.prerequisiteCapabilities.forEach((id, j) => {
      if (validSemanticKey(id) && !capabilities.has(id)) issue(issues, 'CAPABILITY_REFERENCE_NOT_FOUND', source, `$.protocols[${i}].prerequisiteCapabilities[${j}]`, `Capability ${id} is not declared.`);
    });
  });

  if (Array.isArray(manifest.references)) manifest.references.forEach((reference, i) => {
    if (validShortId(reference?.provenanceId) && !provenance.has(reference.provenanceId)) issue(issues, 'PROVENANCE_REFERENCE_NOT_FOUND', source, `$.references[${i}].provenanceId`, `Provenance ${reference.provenanceId} is not declared.`);
  });

  if (Array.isArray(manifest.normalization)) manifest.normalization.forEach((mapping, i) => {
    if (PROTOCOLS.has(mapping?.source?.protocol) && !protocols.has(mapping.source.protocol)) issue(issues, 'NORMALIZATION_PROTOCOL_NOT_DECLARED', source, `$.normalization[${i}].source.protocol`, `Protocol ${mapping.source.protocol} is not declared.`);
    if (validShortId(mapping?.source?.referenceId) && !references.has(mapping.source.referenceId)) issue(issues, 'REFERENCE_NOT_FOUND', source, `$.normalization[${i}].source.referenceId`, `Reference ${mapping.source.referenceId} is not declared.`);
    if (validShortId(mapping?.provenanceId) && !provenance.has(mapping.provenanceId)) issue(issues, 'PROVENANCE_REFERENCE_NOT_FOUND', source, `$.normalization[${i}].provenanceId`, `Provenance ${mapping.provenanceId} is not declared.`);
  });
}

export function validateManifest(manifest, source = '<memory>') {
  const issues = [];
  if (!isObject(manifest)) {
    issue(issues, 'INVALID_MANIFEST', source, '$', 'Manifest must be a JSON object.');
    return issues;
  }

  let serialized = '';
  try { serialized = JSON.stringify(manifest); } catch { serialized = ''; }
  if (Buffer.byteLength(serialized, 'utf8') > MAX_MANIFEST_BYTES) issue(issues, 'MANIFEST_TOO_LARGE', source, '$', `Manifest exceeds ${MAX_MANIFEST_BYTES} bytes.`);

  scanForbidden(manifest, issues, source);
  if (!keys(manifest, TOP_KEYS, ['schemaVersion', 'packId', 'packVersion', 'name', 'publisher', 'certification', 'matchRules', 'capabilities', 'protocols', 'references', 'normalization', 'dependencies', 'provenance'], issues, source, '$')) return issues;

  if ('schemaVersion' in manifest && manifest.schemaVersion !== CONTRACT_VERSION) issue(issues, 'UNSUPPORTED_SCHEMA_VERSION', source, '$.schemaVersion', `Expected ${CONTRACT_VERSION}.`);
  if ('packId' in manifest && !validPackId(manifest.packId)) issue(issues, 'INVALID_PACK_ID', source, '$.packId', 'packId must be stable lower-case dotted/hyphenated identifier.');
  if ('packVersion' in manifest && !validSemver(manifest.packVersion)) issue(issues, 'MALFORMED_VERSION', source, '$.packVersion', 'packVersion must use SemVer 2.0 shape.');
  if ('name' in manifest) stringValue(manifest.name, 1, 160, issues, source, '$.name');
  if ('publisher' in manifest) validatePublisher(manifest.publisher, issues, source);
  if ('certification' in manifest) enumValue(manifest.certification, CERTIFICATIONS, issues, source, '$.certification', 'UNKNOWN_CERTIFICATION');
  validateMetadata(manifest.metadata, issues, source);
  if ('matchRules' in manifest) validateMatchRules(manifest.matchRules, issues, source);
  if ('capabilities' in manifest) validateCapabilities(manifest.capabilities, issues, source);
  if ('protocols' in manifest) validateProtocols(manifest.protocols, issues, source);
  if ('references' in manifest) validateReferences(manifest.references, issues, source);
  if ('normalization' in manifest) validateNormalization(manifest.normalization, issues, source);
  if ('dependencies' in manifest) validateDependencies(manifest.dependencies, issues, source);
  if ('provenance' in manifest) validateProvenance(manifest.provenance, issues, source);
  validateInternalReferences(manifest, issues, source);
  return issues;
}

function stableIssues(issues) {
  return [...issues].sort((a, b) => `${a.source}|${a.path}|${a.code}`.localeCompare(`${b.source}|${b.path}|${b.code}`));
}

function detectDependencyCycles(byId) {
  const issues = [];
  const state = new Map();
  const stack = [];

  function visit(packId) {
    const current = state.get(packId) ?? 0;
    if (current === 2) return;
    if (current === 1) {
      const start = stack.indexOf(packId);
      const cycle = [...stack.slice(start), packId];
      const entry = byId.get(packId);
      issue(issues, 'DEPENDENCY_CYCLE', entry?.source ?? '<set>', '$.dependencies', `Dependency cycle: ${cycle.join(' -> ')}.`);
      return;
    }
    state.set(packId, 1);
    stack.push(packId);
    const entry = byId.get(packId);
    const dependencies = Array.isArray(entry?.manifest?.dependencies) ? entry.manifest.dependencies : [];
    for (const dep of dependencies) if (isObject(dep) && byId.has(dep.packId)) visit(dep.packId);
    stack.pop();
    state.set(packId, 2);
  }

  for (const packId of byId.keys()) visit(packId);
  return issues;
}

export function validateManifestSet(entries) {
  const issues = [];
  const manifests = entries.map((entry) => ({ manifest: entry.manifest, source: entry.source ?? '<memory>' }));
  manifests.forEach(({ manifest, source }) => issues.push(...validateManifest(manifest, source)));

  const byId = new Map();
  for (const entry of manifests) {
    const { manifest, source } = entry;
    if (!isObject(manifest) || !validPackId(manifest.packId) || !validSemver(manifest.packVersion)) continue;
    if (byId.has(manifest.packId)) issue(issues, 'DUPLICATE_PACK_ID', source, '$.packId', `packId ${manifest.packId} is already present in this validation set.`);
    else byId.set(manifest.packId, entry);
  }

  const definitions = new Map();
  for (const { manifest, source } of manifests) {
    if (!isObject(manifest) || !Array.isArray(manifest.matchRules)) continue;
    for (const rule of manifest.matchRules) {
      if (!isObject(rule) || !Number.isInteger(rule.priority) || !Array.isArray(rule.conditions)) continue;
      const exactCount = rule.conditions.filter((c) => c?.operator === 'EXACT').length;
      const key = `${rule.priority}|${exactCount}|${rule.conditions.length}|${ruleSignature(rule)}`;
      const prior = definitions.get(key);
      if (prior && prior.packId !== manifest.packId) issue(issues, 'AMBIGUOUS_MATCH_DEFINITION', source, '$.matchRules', `Equivalent equal-strength match is also declared by ${prior.packId}.`);
      else definitions.set(key, { packId: manifest.packId });
    }
  }

  for (const { manifest, source } of manifests) {
    if (!isObject(manifest) || !Array.isArray(manifest.dependencies) || !validPackId(manifest.packId)) continue;
    for (let i = 0; i < manifest.dependencies.length; i += 1) {
      const dependency = manifest.dependencies[i];
      if (!isObject(dependency) || !validPackId(dependency.packId) || !validSemver(dependency.packVersion)) continue;
      if (dependency.packId === manifest.packId) {
        issue(issues, 'SELF_DEPENDENCY', source, `$.dependencies[${i}]`, 'A pack cannot depend on itself.');
        continue;
      }
      const target = byId.get(dependency.packId);
      if (!target) issue(issues, 'DEPENDENCY_NOT_FOUND', source, `$.dependencies[${i}]`, `Dependency ${dependency.packId}@${dependency.packVersion} is not present.`);
      else if (target.manifest.packVersion !== dependency.packVersion) issue(issues, 'DEPENDENCY_VERSION_MISMATCH', source, `$.dependencies[${i}]`, `Dependency requires ${dependency.packVersion}, validation set contains ${target.manifest.packVersion}.`);
    }
  }

  issues.push(...detectDependencyCycles(byId));
  return stableIssues(issues);
}

function parseJsonFile(file) {
  const raw = fs.readFileSync(file, 'utf8');
  if (Buffer.byteLength(raw, 'utf8') > MAX_MANIFEST_BYTES * 4) throw new Error(`Input file is unreasonably large: ${file}`);
  return JSON.parse(raw);
}

function discoverManifestFiles(inputPath) {
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
      else if (entry.isFile() && entry.name === 'manifest.json') files.push(full);
      if (files.length > MAX_DISCOVERED_MANIFESTS) throw new Error(`More than ${MAX_DISCOVERED_MANIFESTS} manifests discovered; validation is bounded.`);
    }
  };
  walk(resolved);
  return files;
}

function printIssues(issues) {
  for (const item of stableIssues(issues)) console.error(`${item.code}\t${item.source}\t${item.path}\t${item.message}`);
}

function validateFiles(inputs) {
  const files = inputs.flatMap(discoverManifestFiles);
  if (files.length === 0) throw new Error('No manifest.json files found.');
  const entries = [];
  const parseIssues = [];
  for (const file of files) {
    try { entries.push({ manifest: parseJsonFile(file), source: path.relative(process.cwd(), file) }); }
    catch (error) { issue(parseIssues, 'JSON_PARSE_ERROR', path.relative(process.cwd(), file), '$', error.message); }
  }
  const issues = [...parseIssues, ...validateManifestSet(entries)];
  if (issues.length > 0) {
    printIssues(issues);
    return 1;
  }
  console.log(`VALID: ${entries.length} Device Pack manifest(s).`);
  return 0;
}

function runFixtureSuite() {
  const suite = parseJsonFile(path.join(ROOT, 'fixtures', 'cases.json'));
  let failures = 0;

  if (Array.isArray(suite.examples)) {
    const exampleEntries = [];
    for (const relative of suite.examples) {
      const file = path.join(ROOT, relative);
      try { exampleEntries.push({ manifest: parseJsonFile(file), source: `example:${relative}` }); }
      catch (error) {
        console.error(`EXAMPLE PARSE FAILURE ${relative}: ${error.message}`);
        failures += 1;
      }
    }
    const issues = validateManifestSet(exampleEntries);
    if (issues.length > 0) {
      console.error('EXAMPLE VALIDATION FAILURE');
      printIssues(issues);
      failures += 1;
    }
  }

  if (!Array.isArray(suite.cases)) throw new Error('fixtures/cases.json must contain cases[].');
  for (const fixture of suite.cases) {
    const entries = Array.isArray(fixture.manifests)
      ? fixture.manifests.map((manifest, index) => ({ manifest, source: `fixture:${fixture.name}:${index}` }))
      : [];
    const issues = validateManifestSet(entries);
    const actualValid = issues.length === 0;
    const expectedValid = fixture.valid === true;
    const actualCodes = new Set(issues.map((item) => item.code));
    const expectedCodes = Array.isArray(fixture.expectedCodes) ? fixture.expectedCodes : [];
    const missing = expectedCodes.filter((code) => !actualCodes.has(code));
    if (actualValid !== expectedValid || missing.length > 0) {
      console.error(`FIXTURE FAILURE: ${fixture.name}`);
      console.error(`  expected valid=${expectedValid}, actual valid=${actualValid}`);
      if (missing.length > 0) console.error(`  missing expected codes: ${missing.join(', ')}`);
      printIssues(issues);
      failures += 1;
    }
  }

  if (failures > 0) {
    console.error(`FAILED: ${failures} fixture/example validation group(s).`);
    return 1;
  }
  console.log(`VALIDATOR FIXTURE SUITE OK: ${suite.cases.length} case(s) plus ${suite.examples?.length ?? 0} example(s).`);
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

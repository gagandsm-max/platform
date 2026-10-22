/**
 * PR246 Entity Metric Observation contract (exact SHA 006dfc53).
 * Types and pure helpers only — no transport, session or authorization authority.
 */

export const METRIC_OBSERVATION_DEFAULT_LIMIT = 100;
export const METRIC_OBSERVATION_MAX_LIMIT = 500;
/** Matches Core {@code MetricObservationQuery.MAX_WINDOW} = 24 hours (ms form; display/compat). */
export const METRIC_OBSERVATION_MAX_WINDOW_MS = 24 * 60 * 60 * 1000;
/**
 * Exact 24h as nanoseconds — authoritative window bound for
 * {@link assertMetricObservationWindow} (safe integer: 8.64e13 is below 2^53-1).
 * Aligns with Core {@code Instant}/{@code Duration} nanosecond precision.
 */
export const METRIC_OBSERVATION_MAX_WINDOW_NS = 24 * 60 * 60 * 1_000_000_000;

export const METRIC_VALUE_KINDS = ['NUMERIC', 'COUNTER64'] as const;
export type MetricValueKind = (typeof METRIC_VALUE_KINDS)[number];

export const OBSERVATION_READ_STATES = [
  'OK',
  'UNKNOWN',
  'STALE',
  'MONITORING_DEGRADED',
  'NO_DATA',
  'SOURCE_UNAVAILABLE',
] as const;
export type ObservationReadState = (typeof OBSERVATION_READ_STATES)[number];

export const FRESHNESS_CLASSES = [
  'FRESH',
  'STALE',
  'HISTORICAL',
  'MISSING',
  'UNKNOWN',
] as const;
export type FreshnessClass = (typeof FRESHNESS_CLASSES)[number];

/** Exact Phase-1 agent selectors from AllowedMetricSelector. */
export const AGENT_METRIC_SELECTORS = {
  CPU_UTILIZATION: { metricId: 'system.cpu.utilization', valueKind: 'NUMERIC' as const },
  MEMORY_UTILIZATION: { metricId: 'system.memory.utilization', valueKind: 'NUMERIC' as const },
} as const;

export const SNMP_COUNTER64_OBJECT_ARCS = [
  '.1.3.6.1.2.1.31.1.1.1.6', // ifHCInOctets
  '.1.3.6.1.2.1.31.1.1.1.10', // ifHCOutOctets
] as const;

export const SNMP_NUMERIC_OBJECT_ARCS = [
  '.1.3.6.1.2.1.2.2.1.8', // ifOperStatus
] as const;

export interface MetricObservationQuery {
  tenantId: string;
  entityId: string;
  metricId: string;
  valueKind: MetricValueKind;
  /** Inclusive lower bound as ISO-8601 instant. */
  from: string;
  /** Exclusive upper bound as ISO-8601 instant. */
  to: string;
  limit: number;
}

export interface MetricObservationProvenance {
  sourceView: string;
  ordering: string;
  provenanceClass: string;
}

/**
 * Observation row. Counter64 uses exact decimal text in {@code counterValue};
 * NUMERIC uses IEEE float {@code value}. The two must never be collapsed.
 */
export interface MetricObservationItem {
  timestamp: string;
  value: number | null;
  counterValue: string | null;
  quality: number;
  /**
   * Exact flags when JSON number is a safe integer; otherwise the original
   * decimal digit string. Never silently rounded from an unsafe Number.
   */
  flags: number | string;
  sensorId: string | null;
  sequenceId: string | null;
  sourceProtocol: string | null;
}

export interface MetricObservationResponse {
  tenantId: string;
  entityId: string;
  metricId: string;
  valueKind: MetricValueKind;
  from: string;
  to: string;
  limit: number;
  returned: number;
  truncated: boolean;
  observationState: ObservationReadState;
  freshness: FreshnessClass;
  qualityNote: string;
  provenance: MetricObservationProvenance;
  observations: MetricObservationItem[];
}

export type MetricObservationClientErrorCode =
  | 'SOURCE_UNAVAILABLE'
  | 'UNAUTHORIZED'
  | 'FORBIDDEN'
  | 'NOT_FOUND'
  | 'QUERY_LIMIT'
  | 'VALIDATION'
  | 'TIMEOUT'
  | 'ABORTED'
  | 'MALFORMED_RESPONSE'
  | 'CONTEXT_MISMATCH'
  | 'SERVER_ERROR'
  | 'NETWORK';

export class MetricObservationClientError extends Error {
  readonly code: MetricObservationClientErrorCode;
  readonly httpStatus: number | null;
  readonly correlationId: string | null;

  constructor(
    code: MetricObservationClientErrorCode,
    message: string,
    options?: { httpStatus?: number | null; correlationId?: string | null; cause?: unknown },
  ) {
    // ES2020 lib typings accept Error(message) only; attach cause without ErrorOptions.
    super(message);
    this.name = 'MetricObservationClientError';
    this.code = code;
    this.httpStatus = options?.httpStatus ?? null;
    this.correlationId = options?.correlationId ?? null;
    if (options?.cause !== undefined) {
      Object.defineProperty(this, 'cause', {
        value: options.cause,
        enumerable: false,
        configurable: true,
        writable: true,
      });
    }
  }
}

/**
 * Canonical textual UUID shape matching Core `UUID.fromString` / path binding:
 * exact hexadecimal 8-4-4-4-12. No UI-only version 1–5 (or invented variant)
 * whitelist — Integrated Core uses v7 IDs (e.g. 018f0e0a-0000-7000-8000-…).
 * Tenant/entity path values remain selectors, never proof of authorization.
 */
const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const COUNTER64_DECIMAL_RE = /^[0-9]+$/;
const DECIMAL_INSTANCE_RE = /^[1-9][0-9]*$/;

export function isUuid(value: string): boolean {
  return UUID_RE.test(value);
}

/**
 * Canonical UUID identity equality after shape validation.
 * Core serializes lowercase; UI shape validation accepts mixed case.
 * Tenant/entity only — never apply to arbitrary metricId strings.
 */
export function uuidIdentityEquals(a: string, b: string): boolean {
  return isUuid(a) && isUuid(b) && a.toLowerCase() === b.toLowerCase();
}

/**
 * ISO-8601 instant with optional fractional seconds (1–9 digits) and Z / numeric
 * offset. Fractional digits are preserved as nanoseconds — never truncated to ms
 * before comparison (Date.parse alone would collapse sub-ms differences).
 */
const INSTANT_RE =
  /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2}|[+-]\d{4})$/;

export interface MetricObservationInstantParts {
  /** Whole UTC seconds since Unix epoch (floor). */
  epochSec: number;
  /** Nanoseconds within the second [0, 999_999_999]. */
  nano: number;
}

/**
 * Parse a supported ISO-8601 instant, preserving nanosecond fraction.
 * Returns null for malformed or unsupported forms (including >9 fractional digits).
 */
export function parseMetricObservationInstant(
  iso: string,
): MetricObservationInstantParts | null {
  if (typeof iso !== 'string' || iso.length < 20 || iso.length > 40) {
    return null;
  }
  const match = INSTANT_RE.exec(iso);
  if (!match) {
    return null;
  }
  const base = match[1];
  const frac = match[2] ?? '';
  let offset = match[3];
  if (/^[+-]\d{4}$/.test(offset)) {
    offset = `${offset.slice(0, 3)}:${offset.slice(3)}`;
  }
  // Parse whole-second wall time only — do not feed fractional digits to Date.
  const ms = Date.parse(`${base}${offset}`);
  if (!Number.isFinite(ms)) {
    return null;
  }
  const nano = Number.parseInt(`${frac}${'0'.repeat(9 - frac.length)}`, 10);
  if (!Number.isInteger(nano) || nano < 0 || nano > 999_999_999) {
    return null;
  }
  return { epochSec: Math.floor(ms / 1000), nano };
}

/**
 * Semantic instant equality: `.000Z` ≡ `Z`, equal offset forms of the same
 * instant match, distinct nanoseconds do not. Malformed inputs are not equal.
 */
export function metricObservationInstantsEqual(a: string, b: string): boolean {
  const left = parseMetricObservationInstant(a);
  const right = parseMetricObservationInstant(b);
  if (!left || !right) {
    return false;
  }
  return left.epochSec === right.epochSec && left.nano === right.nano;
}

/** Compare parsed instants: -1 if a before b, 0 if equal, 1 if a after b. */
export function compareMetricObservationInstants(
  a: MetricObservationInstantParts,
  b: MetricObservationInstantParts,
): number {
  if (a.epochSec !== b.epochSec) {
    return a.epochSec < b.epochSec ? -1 : 1;
  }
  if (a.nano !== b.nano) {
    return a.nano < b.nano ? -1 : 1;
  }
  return 0;
}

/**
 * Duration in nanoseconds from {@code from} to {@code to}.
 * Caller must ensure {@code to} is strictly after {@code from}.
 */
export function metricObservationWindowDurationNs(
  from: MetricObservationInstantParts,
  to: MetricObservationInstantParts,
): number {
  return (to.epochSec - from.epochSec) * 1_000_000_000 + (to.nano - from.nano);
}

export function isExactCounter64Decimal(value: string): boolean {
  return COUNTER64_DECIMAL_RE.test(value);
}

/**
 * Injected authenticated transport. Caller supplies bearer/session authority;
 * this module never fabricates credentials, cookies or tenant identity.
 */
export type AuthenticatedMetricTransport = (
  input: string,
  init: {
    method: 'GET';
    signal?: AbortSignal;
    headers?: Record<string, string>;
  },
) => Promise<Response>;

export interface MetricObservationBinding {
  tenantId: string;
  entityId: string;
  metricId: string;
  valueKind: MetricValueKind;
  from: string;
  to: string;
  limit?: number;
  transport: AuthenticatedMetricTransport;
}

/**
 * Explicit ownership of a panel view / controller result. Includes the injected
 * transport function identity so a replacement transport cannot reuse prior data.
 */
export interface MetricObservationOwnership {
  tenantId: string;
  entityId: string;
  metricId: string;
  valueKind: MetricValueKind;
  from: string;
  to: string;
  limit: number;
  transport: AuthenticatedMetricTransport;
}

export function isCompleteMetricObservationBinding(
  binding: MetricObservationBinding | null | undefined,
): binding is MetricObservationBinding {
  if (!binding) {
    return false;
  }
  if (typeof binding.transport !== 'function') {
    return false;
  }
  if (!isUuid(binding.tenantId) || !isUuid(binding.entityId)) {
    return false;
  }
  if (!binding.metricId || !binding.from || !binding.to) {
    return false;
  }
  if (!METRIC_VALUE_KINDS.includes(binding.valueKind)) {
    return false;
  }
  return true;
}

/**
 * Non-throwing effective limit. Omitted/null → default. Explicitly invalid
 * values return null (never clamped or substituted).
 */
export function tryEffectiveMetricObservationLimit(
  limit: number | undefined | null,
): number | null {
  if (limit === undefined || limit === null) {
    return METRIC_OBSERVATION_DEFAULT_LIMIT;
  }
  if (typeof limit !== 'number' || !Number.isInteger(limit) || limit < 1 || limit > METRIC_OBSERVATION_MAX_LIMIT) {
    return null;
  }
  return limit;
}

/** True when limit is omitted (default) or an integer in [1, MAX]. */
export function isValidMetricObservationLimit(
  limit: number | undefined | null,
): boolean {
  return tryEffectiveMetricObservationLimit(limit) !== null;
}

/**
 * Build ownership from a complete binding with a valid effective limit.
 * Returns null when the limit is explicitly invalid — never throws.
 */
export function ownershipFromBinding(
  binding: MetricObservationBinding,
): MetricObservationOwnership | null {
  const limit = tryEffectiveMetricObservationLimit(binding.limit);
  if (limit === null) {
    return null;
  }
  return {
    tenantId: binding.tenantId,
    entityId: binding.entityId,
    metricId: binding.metricId,
    valueKind: binding.valueKind,
    from: binding.from,
    to: binding.to,
    limit,
    transport: binding.transport,
  };
}

/**
 * True when stored ownership belongs to the CURRENT complete binding, including
 * transport identity. Used at render time before showing observation-derived UI.
 * Never throws for invalid limits — returns false instead.
 */
export function ownershipMatchesBinding(
  ownership: MetricObservationOwnership | null | undefined,
  binding: MetricObservationBinding | null | undefined,
): boolean {
  if (!ownership || !isCompleteMetricObservationBinding(binding)) {
    return false;
  }
  const expected = ownershipFromBinding(binding);
  if (!expected) {
    return false;
  }
  return (
    ownership.tenantId === expected.tenantId
    && ownership.entityId === expected.entityId
    && ownership.metricId === expected.metricId
    && ownership.valueKind === expected.valueKind
    && ownership.from === expected.from
    && ownership.to === expected.to
    && ownership.limit === expected.limit
    && ownership.transport === expected.transport
  );
}

export function resolveMetricObservationLimit(limit: number | undefined): number {
  const effective = tryEffectiveMetricObservationLimit(limit);
  if (effective === null) {
    throw new MetricObservationClientError(
      'QUERY_LIMIT',
      `limit must be between 1 and ${METRIC_OBSERVATION_MAX_LIMIT}`,
    );
  }
  return effective;
}

/** Stable message for local QUERY_LIMIT (invalid explicit limit). */
export function metricObservationInvalidLimitMessage(): string {
  return `limit must be between 1 and ${METRIC_OBSERVATION_MAX_LIMIT}`;
}

/**
 * Validate observation window using nanosecond Instant pairs (not Date.parse ms).
 * Core MetricObservationQuery compares java.time.Instant / Duration at nano precision.
 */
export function assertMetricObservationWindow(fromIso: string, toIso: string): void {
  const from = parseMetricObservationInstant(fromIso);
  const to = parseMetricObservationInstant(toIso);
  if (!from || !to) {
    throw new MetricObservationClientError(
      'VALIDATION',
      'from and to must be ISO-8601 instants',
    );
  }
  if (compareMetricObservationInstants(to, from) <= 0) {
    throw new MetricObservationClientError(
      'VALIDATION',
      'to must be strictly after from',
    );
  }
  if (metricObservationWindowDurationNs(from, to) > METRIC_OBSERVATION_MAX_WINDOW_NS) {
    throw new MetricObservationClientError(
      'QUERY_LIMIT',
      'time window must be at most 24 hours',
    );
  }
}

/** Same-origin relative route with encoded path/query parameters (PR246 controller). */
export function buildMetricObservationRequestPath(query: MetricObservationQuery): string {
  const path =
    `/api/v1/tenants/${encodeURIComponent(query.tenantId)}` +
    `/entities/${encodeURIComponent(query.entityId)}/metric-observations`;
  const params = new URLSearchParams();
  params.set('metricId', query.metricId);
  params.set('valueKind', query.valueKind);
  params.set('from', query.from);
  params.set('to', query.to);
  params.set('limit', String(query.limit));
  return `${path}?${params.toString()}`;
}

export function bindingToQuery(binding: MetricObservationBinding): MetricObservationQuery {
  assertMetricObservationWindow(binding.from, binding.to);
  return {
    tenantId: binding.tenantId,
    entityId: binding.entityId,
    metricId: binding.metricId,
    valueKind: binding.valueKind,
    from: binding.from,
    to: binding.to,
    limit: resolveMetricObservationLimit(binding.limit),
  };
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function requireString(obj: Record<string, unknown>, key: string): string {
  const value = obj[key];
  if (typeof value !== 'string') {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      `Field '${key}' must be a string`,
    );
  }
  return value;
}

function requireBoolean(obj: Record<string, unknown>, key: string): boolean {
  const value = obj[key];
  if (typeof value !== 'boolean') {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      `Field '${key}' must be a boolean`,
    );
  }
  return value;
}

function requireInt(obj: Record<string, unknown>, key: string): number {
  const value = obj[key];
  if (typeof value !== 'number' || !Number.isInteger(value)) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      `Field '${key}' must be an integer`,
    );
  }
  return value;
}

/**
 * Preserve exact flags. Unsafe JSON numbers (outside Number.MAX_SAFE_INTEGER)
 * are rejected — never silently rounded.
 */
export function parseExactFlags(raw: unknown): number | string {
  if (typeof raw === 'number') {
    if (!Number.isInteger(raw) || !Number.isSafeInteger(raw)) {
      throw new MetricObservationClientError(
        'MALFORMED_RESPONSE',
        'Observation flags cannot be represented exactly as a JSON number',
      );
    }
    return raw;
  }
  if (typeof raw === 'string' && /^-?[0-9]+$/.test(raw)) {
    return raw;
  }
  throw new MetricObservationClientError(
    'MALFORMED_RESPONSE',
    'Observation flags must be an exact integer',
  );
}

export function parseObservationItem(
  raw: unknown,
  valueKind: MetricValueKind,
): MetricObservationItem {
  if (!isObject(raw)) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'Observation item must be an object',
    );
  }
  const timestamp = requireString(raw, 'timestamp');
  const quality = requireInt(raw, 'quality');
  if (quality !== 0 && quality !== 1 && quality !== 2) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'Observation quality must be 0, 1 or 2',
    );
  }
  const flags = parseExactFlags(raw.flags);
  const sensorId = raw.sensorId == null ? null : requireString(raw as Record<string, unknown>, 'sensorId');
  const sequenceId = raw.sequenceId == null ? null : String(raw.sequenceId);
  const sourceProtocol = raw.sourceProtocol == null ? null : String(raw.sourceProtocol);

  if (valueKind === 'COUNTER64') {
    if (raw.value != null) {
      throw new MetricObservationClientError(
        'MALFORMED_RESPONSE',
        'COUNTER64 observations must not carry a numeric value field',
      );
    }
    const counterValue = requireString(raw, 'counterValue');
    if (!isExactCounter64Decimal(counterValue)) {
      throw new MetricObservationClientError(
        'MALFORMED_RESPONSE',
        'COUNTER64 counterValue must be an exact unsigned decimal string',
      );
    }
    // Never Number()/parseFloat() Counter64 — preserve decimal text above 2^53.
    return {
      timestamp,
      value: null,
      counterValue,
      quality,
      flags,
      sensorId,
      sequenceId,
      sourceProtocol,
    };
  }

  if (raw.counterValue != null) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'NUMERIC observations must not carry counterValue',
    );
  }
  if (typeof raw.value !== 'number' || !Number.isFinite(raw.value)) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'NUMERIC observation value must be a finite number',
    );
  }
  return {
    timestamp,
    value: raw.value,
    counterValue: null,
    quality,
    flags,
    sensorId,
    sequenceId,
    sourceProtocol,
  };
}

export function parseMetricObservationResponse(
  raw: unknown,
  request: MetricObservationQuery,
): MetricObservationResponse {
  if (!isObject(raw)) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'Metric observation response must be a JSON object',
    );
  }
  const tenantId = requireString(raw, 'tenantId');
  const entityId = requireString(raw, 'entityId');
  const metricId = requireString(raw, 'metricId');
  const valueKindRaw = requireString(raw, 'valueKind');
  if (!METRIC_VALUE_KINDS.includes(valueKindRaw as MetricValueKind)) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'valueKind must be NUMERIC or COUNTER64',
    );
  }
  const valueKind = valueKindRaw as MetricValueKind;
  const from = requireString(raw, 'from');
  const to = requireString(raw, 'to');
  const limit = requireInt(raw, 'limit');
  const returned = requireInt(raw, 'returned');
  const truncated = requireBoolean(raw, 'truncated');
  const observationState = requireString(raw, 'observationState');
  if (!OBSERVATION_READ_STATES.includes(observationState as ObservationReadState)) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      `Unsupported observationState '${observationState}'`,
    );
  }
  const freshness = requireString(raw, 'freshness');
  if (!FRESHNESS_CLASSES.includes(freshness as FreshnessClass)) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      `Unsupported freshness '${freshness}'`,
    );
  }
  const qualityNote = requireString(raw, 'qualityNote');
  if (!isObject(raw.provenance)) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'provenance object is required',
    );
  }
  const provenance: MetricObservationProvenance = {
    sourceView: requireString(raw.provenance, 'sourceView'),
    ordering: requireString(raw.provenance, 'ordering'),
    provenanceClass: requireString(raw.provenance, 'provenanceClass'),
  };
  if (!Array.isArray(raw.observations)) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'observations must be an array',
    );
  }
  // Bound/consistency checks BEFORE mapping rows — never silently truncate.
  if (returned < 0) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'returned count must be non-negative',
    );
  }
  if (
    returned > request.limit
    || raw.observations.length > request.limit
    || returned > METRIC_OBSERVATION_MAX_LIMIT
    || raw.observations.length > METRIC_OBSERVATION_MAX_LIMIT
  ) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'observation page exceeds request or server maximum limit',
    );
  }
  if (returned !== raw.observations.length) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'returned count does not match observations length',
    );
  }
  if (!parseMetricObservationInstant(from) || !parseMetricObservationInstant(to)) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'from and to must be ISO-8601 instants',
    );
  }
  const observations = raw.observations.map((item) => parseObservationItem(item, valueKind));

  assertResponseMatchesRequest(
    {
      tenantId,
      entityId,
      metricId,
      valueKind,
      from,
      to,
      limit,
    },
    request,
  );

  return {
    tenantId,
    entityId,
    metricId,
    valueKind,
    from,
    to,
    limit,
    returned,
    truncated,
    observationState: observationState as ObservationReadState,
    freshness: freshness as FreshnessClass,
    qualityNote,
    provenance,
    observations,
  };
}

export function assertResponseMatchesRequest(
  responseIdentity: Pick<
    MetricObservationResponse,
    'tenantId' | 'entityId' | 'metricId' | 'valueKind' | 'from' | 'to' | 'limit'
  >,
  request: MetricObservationQuery,
): void {
  // Tenant/entity: shape-valid then case-insensitive (Core lowercase canonical).
  // metricId remains exact / case-sensitive. Instants compare semantically.
  if (
    !uuidIdentityEquals(responseIdentity.tenantId, request.tenantId)
    || !uuidIdentityEquals(responseIdentity.entityId, request.entityId)
    || responseIdentity.metricId !== request.metricId
    || responseIdentity.valueKind !== request.valueKind
    || !metricObservationInstantsEqual(responseIdentity.from, request.from)
    || !metricObservationInstantsEqual(responseIdentity.to, request.to)
    || responseIdentity.limit !== request.limit
  ) {
    throw new MetricObservationClientError(
      'CONTEXT_MISMATCH',
      'Metric observation response does not match the initiating request context',
    );
  }
}

export function isAllowlistedMetricSelector(metricId: string, valueKind: MetricValueKind): boolean {
  const trimmed = metricId.trim();
  if (
    (trimmed === AGENT_METRIC_SELECTORS.CPU_UTILIZATION.metricId
      && valueKind === 'NUMERIC')
    || (trimmed === AGENT_METRIC_SELECTORS.MEMORY_UTILIZATION.metricId
      && valueKind === 'NUMERIC')
  ) {
    return true;
  }
  const families =
    valueKind === 'COUNTER64' ? SNMP_COUNTER64_OBJECT_ARCS : SNMP_NUMERIC_OBJECT_ARCS;
  for (const arc of families) {
    const prefix = `${arc}.`;
    if (!trimmed.startsWith(prefix)) {
      continue;
    }
    const instance = trimmed.slice(prefix.length);
    if (!DECIMAL_INSTANCE_RE.test(instance) || instance.length > 10) {
      continue;
    }
    const n = Number(instance);
    if (Number.isInteger(n) && n >= 1 && n <= 2_147_483_647) {
      return true;
    }
  }
  return false;
}

/** Map server observation states onto PageStateKind without collapsing to device-down. */
export function observationStateToPageKind(
  state: ObservationReadState,
):
  | 'normal'
  | 'unknown'
  | 'stale'
  | 'monitoring-degraded'
  | 'empty'
  | 'source-unavailable' {
  switch (state) {
    case 'OK':
      return 'normal';
    case 'UNKNOWN':
      return 'unknown';
    case 'STALE':
      return 'stale';
    case 'MONITORING_DEGRADED':
      return 'monitoring-degraded';
    case 'NO_DATA':
      return 'empty';
    case 'SOURCE_UNAVAILABLE':
      return 'source-unavailable';
    default: {
      const _exhaustive: never = state;
      return _exhaustive;
    }
  }
}

export function escapeForText(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

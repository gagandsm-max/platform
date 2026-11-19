/**
 * BUILD-070 read-only monitoring-profile assignment HTTP adapter.
 *
 * Mirrors Core `MonitoringProfileAssignmentController`:
 * GET /api/v1/tenants/{tenantId}/entities/{entityId}/monitoring-profile-assignment
 * 200 AssignmentResponse projection; 204 no assignment; masked 404 denial.
 *
 * Accepts an injected authenticated transport only — no login, token storage,
 * App-global fetch, UI mount, or mutation / If-Match write controls.
 */

export type MonitoringProfileAssignmentMode =
  | 'AUTO_FROM_CRITICALITY'
  | 'EXPLICIT_PROFILE';

/** Bounded Core AssignmentResponse fields after contract verification. */
export interface MonitoringProfileAssignmentProjection {
  readonly entityId: string;
  readonly profileId: string;
  readonly profileVersionNo: number;
  readonly assignmentMode: MonitoringProfileAssignmentMode;
  /** Present only when the JSON number is a safe positive integer. */
  readonly rowVersion: number;
  readonly effectiveFrom: string;
}

/**
 * Minimal authenticated transport seam. Callers supply auth; this adapter
 * never attaches Authorization or invents credentials.
 */
export interface MonitoringProfileAssignmentTransportRequest {
  readonly method: 'GET';
  readonly path: string;
  readonly signal?: AbortSignal;
}

export interface MonitoringProfileAssignmentTransportHeaders {
  get(name: string): string | null;
}

export interface MonitoringProfileAssignmentTransportResponse {
  readonly status: number;
  readonly headers: MonitoringProfileAssignmentTransportHeaders;
  text(): Promise<string>;
}

export interface MonitoringProfileAssignmentTransport {
  request(
    init: MonitoringProfileAssignmentTransportRequest,
  ): Promise<MonitoringProfileAssignmentTransportResponse>;
}

export interface GetMonitoringProfileAssignmentInput {
  readonly tenantId: string;
  readonly entityId: string;
  readonly signal?: AbortSignal;
}

export type MonitoringProfileAssignmentClientResult =
  | {
      readonly kind: 'authorized';
      readonly assignment: MonitoringProfileAssignmentProjection;
      /** Opaque strong ETag from response headers when safe/valid — never a write token. */
      readonly etag?: string;
      readonly correlationId?: string;
    }
  | {
      readonly kind: 'no_assignment';
      readonly correlationId?: string;
    }
  | {
      readonly kind: 'masked_denial';
      readonly correlationId?: string;
    }
  | {
      readonly kind: 'denied';
      readonly status: 401 | 403;
      readonly correlationId?: string;
    }
  | {
      readonly kind: 'throttled';
      readonly correlationId?: string;
    }
  | {
      readonly kind: 'unavailable';
      readonly status: number;
      readonly correlationId?: string;
    }
  | {
      readonly kind: 'unexpected_content';
      readonly status: number;
      readonly correlationId?: string;
    }
  | {
      readonly kind: 'unexpected_payload';
      readonly correlationId?: string;
    }
  | {
      readonly kind: 'network_error';
    }
  | {
      readonly kind: 'aborted';
    }
  | {
      readonly kind: 'error';
    }
  | {
      readonly kind: 'invalid_request';
      readonly reason: 'invalid_tenant_id' | 'invalid_entity_id';
    };

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Strong quoted ETag whose opaque tag is a positive decimal (rowVersion wire form). */
const SAFE_STRONG_ETAG_RE = /^"[1-9][0-9]{0,18}"$/;

const CORRELATION_ID_RE = /^[A-Za-z0-9._:-]{1,128}$/;

const ASSIGNMENT_PATH_SUFFIX = '/monitoring-profile-assignment';

const isAssignmentMode = (value: unknown): value is MonitoringProfileAssignmentMode =>
  value === 'AUTO_FROM_CRITICALITY' || value === 'EXPLICIT_PROFILE';

const normalizeUuid = (value: unknown): string | null => {
  if (typeof value !== 'string') {
    return null;
  }
  const trimmed = value.trim();
  if (!UUID_RE.test(trimmed)) {
    return null;
  }
  // Reject embedded path/header injection even if a UUID substring matches.
  if (trimmed !== value || /[/\r\n\0\\?#]/.test(value)) {
    return null;
  }
  return trimmed.toLowerCase();
};

export const buildMonitoringProfileAssignmentPath = (
  tenantId: string,
  entityId: string,
): string | null => {
  const tenant = normalizeUuid(tenantId);
  const entity = normalizeUuid(entityId);
  if (tenant === null || entity === null) {
    return null;
  }
  return `/api/v1/tenants/${tenant}/entities/${entity}${ASSIGNMENT_PATH_SUFFIX}`;
};

const readCorrelationId = (
  headers: MonitoringProfileAssignmentTransportHeaders,
): string | undefined => {
  const raw = headers.get('X-Correlation-Id') ?? headers.get('x-correlation-id');
  if (raw == null) {
    return undefined;
  }
  const trimmed = raw.trim();
  if (!CORRELATION_ID_RE.test(trimmed)) {
    return undefined;
  }
  return trimmed;
};

/**
 * Preserve a standards-compliant strong quoted ETag only when the opaque tag
 * is a safe positive decimal string. Never derive If-Match / write tokens and
 * never invent a precise Number from a large 64-bit tag.
 */
export const readSafeAssignmentEtag = (
  headers: MonitoringProfileAssignmentTransportHeaders,
): string | undefined => {
  const raw = headers.get('ETag') ?? headers.get('etag');
  if (raw == null) {
    return undefined;
  }
  const trimmed = raw.trim();
  if (!SAFE_STRONG_ETAG_RE.test(trimmed)) {
    return undefined;
  }
  // Bound to signed 64-bit positive long decimal without Number coercion.
  const digits = trimmed.slice(1, -1);
  if (digits.length > 19) {
    return undefined;
  }
  if (digits.length === 19 && digits > '9223372036854775807') {
    return undefined;
  }
  return trimmed;
};

const isSafePositiveInteger = (value: unknown): value is number =>
  typeof value === 'number'
  && Number.isInteger(value)
  && value > 0
  && Number.isSafeInteger(value);

const isSafeProfileVersion = (value: unknown): value is number =>
  typeof value === 'number'
  && Number.isInteger(value)
  && value >= 1
  && Number.isSafeInteger(value);

const parseAuthorizedProjection = (
  bodyText: string,
  expectedEntityId: string,
): MonitoringProfileAssignmentProjection | null => {
  let parsed: unknown;
  try {
    parsed = JSON.parse(bodyText);
  } catch {
    return null;
  }
  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return null;
  }

  const record = parsed as Record<string, unknown>;

  const entityId = normalizeUuid(record.entityId);
  const profileId = normalizeUuid(record.profileId);
  if (entityId === null || profileId === null) {
    return null;
  }
  if (entityId !== expectedEntityId) {
    return null;
  }
  if (!isAssignmentMode(record.assignmentMode)) {
    return null;
  }
  if (!isSafeProfileVersion(record.profileVersionNo)) {
    return null;
  }

  // Reject unsafe / lossy 64-bit JSON numbers — do not invent precision.
  if (typeof record.rowVersion === 'number' && !isSafePositiveInteger(record.rowVersion)) {
    return null;
  }
  if (!isSafePositiveInteger(record.rowVersion)) {
    return null;
  }

  if (typeof record.effectiveFrom !== 'string') {
    return null;
  }
  const effectiveFrom = record.effectiveFrom.trim();
  if (effectiveFrom.length < 1 || Number.isNaN(Date.parse(effectiveFrom))) {
    return null;
  }

  return {
    entityId,
    profileId,
    profileVersionNo: record.profileVersionNo,
    assignmentMode: record.assignmentMode,
    rowVersion: record.rowVersion,
    effectiveFrom,
  };
};

const isAbortError = (error: unknown): boolean => {
  if (error === null || typeof error !== 'object') {
    return false;
  }
  const name = (error as { name?: unknown }).name;
  return name === 'AbortError';
};

const isNetworkError = (error: unknown): boolean => {
  if (error === null || typeof error !== 'object') {
    return false;
  }
  const name = (error as { name?: unknown }).name;
  const message = (error as { message?: unknown }).message;
  if (name === 'TypeError') {
    return true;
  }
  if (typeof message === 'string' && /network|fetch failed|econnrefused|enotfound/i.test(message)) {
    return true;
  }
  return false;
};

/**
 * Read the tenant/entity monitoring-profile assignment via an injected transport.
 * Does not log response bodies or surface secrets (definition / assignedBy / credentials).
 */
export const getMonitoringProfileAssignment = async (
  transport: MonitoringProfileAssignmentTransport,
  input: GetMonitoringProfileAssignmentInput,
): Promise<MonitoringProfileAssignmentClientResult> => {
  const tenantId = normalizeUuid(input.tenantId);
  if (tenantId === null) {
    return { kind: 'invalid_request', reason: 'invalid_tenant_id' };
  }
  const entityId = normalizeUuid(input.entityId);
  if (entityId === null) {
    return { kind: 'invalid_request', reason: 'invalid_entity_id' };
  }

  const path = buildMonitoringProfileAssignmentPath(tenantId, entityId);
  if (path === null) {
    return { kind: 'invalid_request', reason: 'invalid_entity_id' };
  }

  let response: MonitoringProfileAssignmentTransportResponse;
  try {
    response = await transport.request({
      method: 'GET',
      path,
      signal: input.signal,
    });
  } catch (error) {
    if (isAbortError(error)) {
      return { kind: 'aborted' };
    }
    if (isNetworkError(error)) {
      return { kind: 'network_error' };
    }
    return { kind: 'error' };
  }

  const correlationId = readCorrelationId(response.headers);

  if (response.status === 204) {
    return correlationId === undefined
      ? { kind: 'no_assignment' }
      : { kind: 'no_assignment', correlationId };
  }

  if (response.status === 404) {
    // Masked denial — never collapse into authorized empty / no_assignment.
    return correlationId === undefined
      ? { kind: 'masked_denial' }
      : { kind: 'masked_denial', correlationId };
  }

  if (response.status === 401 || response.status === 403) {
    return {
      kind: 'denied',
      status: response.status,
      ...(correlationId === undefined ? {} : { correlationId }),
    };
  }

  if (response.status === 429) {
    return correlationId === undefined
      ? { kind: 'throttled' }
      : { kind: 'throttled', correlationId };
  }

  if (response.status === 502 || response.status === 503 || response.status === 504) {
    return {
      kind: 'unavailable',
      status: response.status,
      ...(correlationId === undefined ? {} : { correlationId }),
    };
  }

  if (response.status !== 200) {
    return {
      kind: 'unexpected_content',
      status: response.status,
      ...(correlationId === undefined ? {} : { correlationId }),
    };
  }

  let bodyText: string;
  try {
    bodyText = await response.text();
  } catch (error) {
    if (isAbortError(error)) {
      return { kind: 'aborted' };
    }
    if (isNetworkError(error)) {
      return { kind: 'network_error' };
    }
    return { kind: 'error' };
  }

  if (typeof bodyText !== 'string' || bodyText.trim().length < 1) {
    return correlationId === undefined
      ? { kind: 'unexpected_payload' }
      : { kind: 'unexpected_payload', correlationId };
  }

  const contentType = response.headers.get('Content-Type') ?? response.headers.get('content-type');
  if (contentType != null && contentType.trim().length > 0) {
    const media = contentType.split(';', 1)[0]?.trim().toLowerCase() ?? '';
    if (media !== 'application/json' && media !== 'application/problem+json') {
      return {
        kind: 'unexpected_content',
        status: 200,
        ...(correlationId === undefined ? {} : { correlationId }),
      };
    }
  }

  const assignment = parseAuthorizedProjection(bodyText, entityId);
  if (assignment === null) {
    return correlationId === undefined
      ? { kind: 'unexpected_payload' }
      : { kind: 'unexpected_payload', correlationId };
  }

  const etag = readSafeAssignmentEtag(response.headers);
  return {
    kind: 'authorized',
    assignment,
    ...(etag === undefined ? {} : { etag }),
    ...(correlationId === undefined ? {} : { correlationId }),
  };
};

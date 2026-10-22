/**
 * Bounded metric-observation client. Uses an injected authenticated transport;
 * never fabricates tenant, token, cookie, role or demo data.
 */

import {
  AuthenticatedMetricTransport,
  MetricObservationBinding,
  MetricObservationClientError,
  MetricObservationClientErrorCode,
  MetricObservationQuery,
  MetricObservationResponse,
  bindingToQuery,
  buildMetricObservationRequestPath,
  isCompleteMetricObservationBinding,
  parseMetricObservationResponse,
} from './metric-observations.js';

export interface MetricObservationFetchResult {
  request: MetricObservationQuery;
  path: string;
  response: MetricObservationResponse;
}

function correlationFrom(response: Response): string | null {
  return response.headers.get('x-correlation-id')
    ?? response.headers.get('X-Correlation-Id');
}

export function mapHttpStatusToError(
  status: number,
  correlationId: string | null,
): MetricObservationClientError {
  let code: MetricObservationClientErrorCode;
  let message: string;
  switch (status) {
    case 401:
      code = 'UNAUTHORIZED';
      message = 'Authentication required for metric observations';
      break;
    case 403:
      code = 'FORBIDDEN';
      message = 'Not authorized to read metric observations for this entity';
      break;
    case 404:
      // Opaque: must not claim the entity exists from a 404 alone.
      code = 'NOT_FOUND';
      message = 'Metric observations were not available for this request (not found or unauthorized)';
      break;
    case 400:
      code = 'VALIDATION';
      message = 'Metric observation request failed validation';
      break;
    case 408:
    case 504:
      code = 'TIMEOUT';
      message = 'Metric observation request timed out';
      break;
    case 429:
      code = 'QUERY_LIMIT';
      message = 'Metric observation request exceeded rate or query limits';
      break;
    default:
      if (status >= 500) {
        code = 'SERVER_ERROR';
        message = 'Metric observation service error';
      } else {
        code = 'SERVER_ERROR';
        message = `Unexpected metric observation HTTP status ${status}`;
      }
  }
  return new MetricObservationClientError(code, message, { httpStatus: status, correlationId });
}

/**
 * Core ApiExceptionHandler body: { code, message, correlationId, timestamp }.
 * Only the exact known code QUERY_LIMIT_EXCEEDED (HTTP 400) is remapped to
 * QUERY_LIMIT. Arbitrary server messages are never surfaced.
 */
export async function mapHttpErrorResponse(
  response: Response,
  correlationId: string | null,
): Promise<MetricObservationClientError> {
  if (response.status === 400) {
    const knownCode = await readExactErrorCode(response);
    if (knownCode === 'QUERY_LIMIT_EXCEEDED') {
      return new MetricObservationClientError(
        'QUERY_LIMIT',
        'Metric observation request exceeded query limits',
        { httpStatus: 400, correlationId },
      );
    }
  }
  return mapHttpStatusToError(response.status, correlationId);
}

async function readExactErrorCode(response: Response): Promise<string | null> {
  try {
    const raw: unknown = await response.json();
    if (
      typeof raw === 'object'
      && raw !== null
      && !Array.isArray(raw)
      && typeof (raw as { code?: unknown }).code === 'string'
    ) {
      return (raw as { code: string }).code;
    }
  } catch {
    // Non-JSON / empty body → fall through to status mapping.
  }
  return null;
}

/**
 * Perform one bounded GET. Requires a complete binding. AbortSignal is forwarded
 * through the injected transport without inventing credentials.
 */
export async function fetchMetricObservations(
  binding: MetricObservationBinding,
  signal?: AbortSignal,
): Promise<MetricObservationFetchResult> {
  if (!isCompleteMetricObservationBinding(binding)) {
    throw new MetricObservationClientError(
      'SOURCE_UNAVAILABLE',
      'Metric observation binding is incomplete; no request was sent',
    );
  }
  const request = bindingToQuery(binding);
  const path = buildMetricObservationRequestPath(request);
  const transport: AuthenticatedMetricTransport = binding.transport;

  let response: Response;
  try {
    response = await transport(path, {
      method: 'GET',
      signal,
      headers: {
        Accept: 'application/json',
      },
    });
  } catch (cause) {
    const aborted =
      signal?.aborted
      || (cause instanceof Error && cause.name === 'AbortError');
    if (aborted) {
      throw new MetricObservationClientError('ABORTED', 'Metric observation request was cancelled', {
        cause,
      });
    }
    throw new MetricObservationClientError('NETWORK', 'Metric observation transport failed', {
      cause,
    });
  }

  const correlationId = correlationFrom(response);
  if (!response.ok) {
    throw await mapHttpErrorResponse(response, correlationId);
  }

  let raw: unknown;
  try {
    raw = await response.json();
  } catch (cause) {
    throw new MetricObservationClientError(
      'MALFORMED_RESPONSE',
      'Metric observation response is not valid JSON',
      { httpStatus: response.status, correlationId, cause },
    );
  }

  const parsed = parseMetricObservationResponse(raw, request);
  return { request, path, response: parsed };
}

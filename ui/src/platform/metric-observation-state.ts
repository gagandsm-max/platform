/**
 * Metric observation panel state machine.
 * Unbound → SOURCE_UNAVAILABLE with zero requests.
 * Binding/context change cancels in-flight work and clears prior-tenant rows immediately.
 */

import { fetchMetricObservations } from './metric-observation-client.js';
import {
  MetricObservationBinding,
  MetricObservationClientError,
  MetricObservationItem,
  MetricObservationQuery,
  MetricObservationResponse,
  ObservationReadState,
  FreshnessClass,
  isCompleteMetricObservationBinding,
  observationStateToPageKind,
} from './metric-observations.js';
import type { PageStateKind } from './screen-state.js';

export const METRIC_OBSERVATION_MAX_MANUAL_RETRIES = 3;

export type MetricObservationPanelPhase =
  | 'unbound'
  | 'loading'
  | 'ready'
  | 'error';

export interface MetricObservationPanelView {
  phase: MetricObservationPanelPhase;
  pageKind: PageStateKind;
  title: string;
  message: string;
  observationState: ObservationReadState | null;
  freshness: FreshnessClass | null;
  truncated: boolean;
  qualityNote: string | null;
  provenance: MetricObservationResponse['provenance'] | null;
  observations: readonly MetricObservationItem[];
  request: MetricObservationQuery | null;
  errorCode: string | null;
  correlationId: string | null;
  retryAvailable: boolean;
  retryCount: number;
  /** Generation token; responses with a lower generation must be ignored. */
  generation: number;
}

export type MetricObservationListener = (view: MetricObservationPanelView) => void;

const EMPTY_VIEW: MetricObservationPanelView = {
  phase: 'unbound',
  pageKind: 'source-unavailable',
  title: 'Source unavailable',
  message:
    'No authenticated tenant/entity/query binding is available. Metric observations are not requested and no demo data is shown. FOUNDATION_ACCESS is presentation-only and never authorizes this panel.',
  observationState: 'SOURCE_UNAVAILABLE',
  freshness: null,
  truncated: false,
  qualityNote: null,
  provenance: null,
  observations: [],
  request: null,
  errorCode: 'SOURCE_UNAVAILABLE',
  correlationId: null,
  retryAvailable: false,
  retryCount: 0,
  generation: 0,
};

function clearRowsView(
  generation: number,
  retryCount: number,
  title: string,
  message: string,
  pageKind: PageStateKind,
  phase: MetricObservationPanelPhase,
  errorCode: string | null,
): MetricObservationPanelView {
  return {
    ...EMPTY_VIEW,
    phase,
    pageKind,
    title,
    message,
    observations: [],
    generation,
    retryCount,
    errorCode,
    retryAvailable: false,
  };
}

function readyView(
  generation: number,
  retryCount: number,
  response: MetricObservationResponse,
  request: MetricObservationQuery,
): MetricObservationPanelView {
  const pageKind = observationStateToPageKind(response.observationState);
  const historicalNote =
    response.freshness === 'HISTORICAL'
      ? ' Freshness is HISTORICAL — a successful historical window is not current healthy monitoring.'
      : '';
  const truncatedNote = response.truncated
    ? ` Result truncated to the latest ${response.limit} observations.`
    : '';
  return {
    phase: 'ready',
    pageKind,
    title: `Entity metric observations · ${response.observationState}`,
    message: `${response.qualityNote}${historicalNote}${truncatedNote}`,
    observationState: response.observationState,
    freshness: response.freshness,
    truncated: response.truncated,
    qualityNote: response.qualityNote,
    provenance: response.provenance,
    observations: response.observations,
    request,
    errorCode: null,
    correlationId: null,
    retryAvailable: false,
    retryCount,
    generation,
  };
}

function errorView(
  generation: number,
  retryCount: number,
  error: MetricObservationClientError,
): MetricObservationPanelView {
  let pageKind: PageStateKind = 'error';
  switch (error.code) {
    case 'UNAUTHORIZED':
    case 'FORBIDDEN':
      pageKind = 'permission-restricted';
      break;
    case 'NOT_FOUND':
      pageKind = 'not-found';
      break;
    case 'QUERY_LIMIT':
      pageKind = 'query-limited';
      break;
    case 'SOURCE_UNAVAILABLE':
      pageKind = 'source-unavailable';
      break;
    case 'ABORTED':
      pageKind = 'loading';
      break;
    default:
      pageKind = 'error';
  }
  const retryAvailable =
    retryCount < METRIC_OBSERVATION_MAX_MANUAL_RETRIES
    && error.code !== 'UNAUTHORIZED'
    && error.code !== 'FORBIDDEN'
    && error.code !== 'SOURCE_UNAVAILABLE'
    && error.code !== 'ABORTED';
  return {
    phase: 'error',
    pageKind,
    title: 'Metric observation request failed',
    message: error.message,
    observationState: null,
    freshness: null,
    truncated: false,
    qualityNote: null,
    provenance: null,
    observations: [],
    request: null,
    errorCode: error.code,
    correlationId: error.correlationId,
    retryAvailable,
    retryCount,
    generation,
  };
}

/**
 * Returns true when an in-flight response should be applied.
 * Out-of-order / stale generations must be rejected.
 */
export function shouldAcceptGeneration(
  activeGeneration: number,
  responseGeneration: number,
): boolean {
  return responseGeneration === activeGeneration;
}

export class MetricObservationController {
  private binding: MetricObservationBinding | null = null;
  private generation = 0;
  private retryCount = 0;
  private abort: AbortController | null = null;
  private view: MetricObservationPanelView = { ...EMPTY_VIEW };
  private readonly listeners = new Set<MetricObservationListener>();
  private requestCount = 0;

  /** Test/oracle: number of transport invocations since construction. */
  getTransportRequestCount(): number {
    return this.requestCount;
  }

  getView(): MetricObservationPanelView {
    return this.view;
  }

  subscribe(listener: MetricObservationListener): () => void {
    this.listeners.add(listener);
    listener(this.view);
    return () => {
      this.listeners.delete(listener);
    };
  }

  private emit(next: MetricObservationPanelView): void {
    this.view = next;
    for (const listener of this.listeners) {
      listener(next);
    }
  }

  private cancelInFlight(): void {
    if (this.abort) {
      this.abort.abort();
      this.abort = null;
    }
  }

  /**
   * Apply a new binding. Clears previous-tenant rows immediately before any
   * subsequent render/fetch. Incomplete binding → SOURCE_UNAVAILABLE, zero requests.
   */
  setBinding(binding: MetricObservationBinding | null | undefined): void {
    this.cancelInFlight();
    this.generation += 1;
    this.retryCount = 0;
    const generation = this.generation;

    // Clear prior context rows immediately on any context change.
    this.emit(
      clearRowsView(
        generation,
        0,
        'Source unavailable',
        EMPTY_VIEW.message,
        'source-unavailable',
        'unbound',
        'SOURCE_UNAVAILABLE',
      ),
    );

    if (!isCompleteMetricObservationBinding(binding)) {
      this.binding = null;
      return;
    }

    this.binding = binding;
    void this.load(generation, 0);
  }

  /** Bounded manual retry — never an automatic polling storm. */
  retry(): void {
    if (!this.binding || !this.view.retryAvailable) {
      return;
    }
    if (this.retryCount >= METRIC_OBSERVATION_MAX_MANUAL_RETRIES) {
      return;
    }
    this.cancelInFlight();
    this.generation += 1;
    const nextRetry = this.retryCount + 1;
    void this.load(this.generation, nextRetry);
  }

  dispose(): void {
    this.cancelInFlight();
    this.generation += 1;
    this.binding = null;
    this.listeners.clear();
    this.emit({ ...EMPTY_VIEW, generation: this.generation });
  }

  private async load(generation: number, retryCount: number): Promise<void> {
    const binding = this.binding;
    if (!binding) {
      return;
    }
    this.retryCount = retryCount;
    this.emit({
      ...clearRowsView(
        generation,
        retryCount,
        'Loading metric observations',
        'Requesting bounded observations for the selected entity.',
        'loading',
        'loading',
        null,
      ),
      retryAvailable: false,
    });

    const abort = new AbortController();
    this.abort = abort;
    try {
      this.requestCount += 1;
      const result = await fetchMetricObservations(binding, abort.signal);
      if (!shouldAcceptGeneration(this.generation, generation)) {
        return;
      }
      this.emit(readyView(generation, retryCount, result.response, result.request));
    } catch (cause) {
      if (!shouldAcceptGeneration(this.generation, generation)) {
        return;
      }
      const error =
        cause instanceof MetricObservationClientError
          ? cause
          : new MetricObservationClientError(
            'NETWORK',
            'Metric observation request failed',
            { cause },
          );
      if (error.code === 'ABORTED') {
        return;
      }
      this.emit(errorView(generation, retryCount, error));
    } finally {
      if (this.abort === abort) {
        this.abort = null;
      }
    }
  }
}

export function createUnboundMetricObservationView(): MetricObservationPanelView {
  return { ...EMPTY_VIEW };
}

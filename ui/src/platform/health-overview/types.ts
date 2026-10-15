/**
 * Typed presentation contracts for UI-009 Network Health Overview and
 * UI-029 Infrastructure Health Overview.
 *
 * These are UI view-model boundaries only. They do not invent Core APIs,
 * authentication, or live telemetry. Production paths must not fabricate
 * device UP/DOWN, metric values, or alert counters.
 */

import type { ScreenDataState } from '../screen-state.js';

/** Operator-facing presentation states for health overview surfaces. */
export type HealthOverviewPresentationState =
  | 'loading'
  | 'empty'
  | 'partial'
  | 'stale'
  | 'permission-restricted'
  | 'not-licensed'
  | 'query-limited'
  | 'error'
  | 'monitoring-degraded'
  | 'source-unavailable'
  | 'unknown';

/**
 * Semantic entity presentation status.
 * Missing or unavailable monitoring evidence must never collapse to DEVICE_DOWN.
 */
export type EntityEvidenceStatus =
  | 'UNKNOWN'
  | 'STALE'
  | 'MONITORING_DEGRADED'
  | 'PARTIAL'
  | 'SOURCE_UNAVAILABLE'
  | 'NO_MONITORING_DATA'
  | 'EXPECTED_STATE';

export type TelemetryFreshnessPresentation =
  | 'unknown'
  | 'fresh'
  | 'stale'
  | 'missing'
  | 'not-connected';

export type CollectorHealthPresentation =
  | 'unknown'
  | 'healthy'
  | 'degraded'
  | 'unavailable'
  | 'not-connected';

export interface HealthOverviewColumn {
  readonly id: string;
  readonly label: string;
  readonly description: string;
}

export interface HealthOverviewPanelDescriptor {
  readonly id: string;
  readonly title: string;
  readonly purpose: string;
  /** Explicit placeholder — never a fabricated KPI value. */
  readonly valueLabel: string;
  readonly evidenceNote: string;
}

export interface HealthOverviewRowPlaceholder {
  readonly id: string;
  readonly primaryLabel: string;
  readonly secondaryLabel: string;
  readonly siteLabel: string;
  readonly status: EntityEvidenceStatus;
  readonly statusExplanation: string;
  readonly freshness: TelemetryFreshnessPresentation;
  readonly collector: CollectorHealthPresentation;
  readonly lastUpdateLabel: string;
  readonly cells: Readonly<Record<string, string>>;
}

export interface HealthOverviewStateCopy {
  readonly kind: HealthOverviewPresentationState;
  readonly title: string;
  readonly explanation: string;
  readonly nextAction: string;
  readonly evidence: readonly string[];
  readonly retryAvailable: boolean;
  readonly diagnosticAvailable: boolean;
}

export interface HealthOverviewViewModelBase {
  readonly uiCode: 'UI-009' | 'UI-029';
  readonly workspaceId: 'ui-009' | 'ui-029';
  readonly label: string;
  readonly domain: 'Network' | 'Infrastructure';
  readonly activeTab: string;
  readonly presentationState: HealthOverviewPresentationState;
  readonly stateCopy: HealthOverviewStateCopy;
  readonly capabilityStatus: 'PENDING_BACKEND' | 'INTEGRATED';
  readonly dataBound: boolean;
  readonly panels: readonly HealthOverviewPanelDescriptor[];
  readonly columns: readonly HealthOverviewColumn[];
  readonly rows: readonly HealthOverviewRowPlaceholder[];
  readonly safeguardingNotes: readonly string[];
}

export interface NetworkHealthOverviewVm extends HealthOverviewViewModelBase {
  readonly uiCode: 'UI-009';
  readonly workspaceId: 'ui-009';
  readonly domain: 'Network';
}

export interface InfrastructureHealthOverviewVm extends HealthOverviewViewModelBase {
  readonly uiCode: 'UI-029';
  readonly workspaceId: 'ui-029';
  readonly domain: 'Infrastructure';
}

export type HealthOverviewVm = NetworkHealthOverviewVm | InfrastructureHealthOverviewVm;

export interface HealthOverviewBuildInput {
  readonly activeTab?: string;
  readonly presentationState?: HealthOverviewPresentationState;
  readonly capabilityStatus?: 'PENDING_BACKEND' | 'INTEGRATED';
  readonly dataBound?: boolean;
  readonly correlationId?: string;
  readonly errorId?: string;
  readonly affectedSource?: string;
  readonly rows?: readonly HealthOverviewRowPlaceholder[];
}

/** Maps a Platform_09 screen data state into health-overview presentation when bound. */
export const screenDataStateToHealthPresentation = (
  state: ScreenDataState,
): HealthOverviewPresentationState => {
  switch (state) {
    case 'loading':
      return 'loading';
    case 'empty':
      return 'empty';
    case 'partial':
      return 'partial';
    case 'stale':
      return 'stale';
    case 'permission-restricted':
      return 'permission-restricted';
    case 'not-licensed':
      return 'not-licensed';
    case 'query-limited':
      return 'query-limited';
    case 'error':
      return 'error';
    case 'monitoring-degraded':
      return 'monitoring-degraded';
    case 'normal':
      // Normal live health requires a bound provider; unbound paths stay unknown.
      return 'unknown';
    default: {
      const _exhaustive: never = state;
      return _exhaustive;
    }
  }
};

export const ENTITY_EVIDENCE_STATUS_LABELS: Readonly<Record<EntityEvidenceStatus, string>> = {
  UNKNOWN: 'Unknown',
  STALE: 'Stale',
  MONITORING_DEGRADED: 'Monitoring degraded',
  PARTIAL: 'Partial evidence',
  SOURCE_UNAVAILABLE: 'Source unavailable',
  NO_MONITORING_DATA: 'No monitoring data',
  EXPECTED_STATE: 'Expected state',
};

export const isDeviceDownFabrication = (status: string): boolean => {
  const normalized = status.trim().toUpperCase().replace(/[\s-]+/g, '_');
  return normalized === 'DEVICE_DOWN' || normalized === 'DOWN' || normalized === 'HOST_DOWN';
};

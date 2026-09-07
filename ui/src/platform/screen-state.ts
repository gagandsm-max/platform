export type ScreenDataState =
  | 'loading'
  | 'empty'
  | 'partial'
  | 'stale'
  | 'permission-restricted'
  | 'not-licensed'
  | 'query-limited'
  | 'error'
  | 'monitoring-degraded'
  | 'normal';

export type PageStateKind = ScreenDataState | 'not-found';

export const REQUIRED_SCREEN_STATES: readonly ScreenDataState[] = [
  'loading',
  'empty',
  'partial',
  'stale',
  'permission-restricted',
  'not-licensed',
  'query-limited',
  'error',
  'monitoring-degraded',
  'normal',
] as const;

export const SCREEN_STATE_LABELS: Readonly<Record<PageStateKind, string>> = {
  loading: 'Loading',
  empty: 'Empty',
  partial: 'Partial Data',
  stale: 'Stale Data',
  'permission-restricted': 'Permission Restricted',
  'not-licensed': 'Not Licensed',
  'query-limited': 'Query Limited',
  error: 'Error',
  'monitoring-degraded': 'Monitoring Degraded',
  normal: 'Normal',
  'not-found': 'Not Found',
};

export interface ScreenStateModel {
  kind: PageStateKind;
  title: string;
  explanation: string;
  evidence?: readonly string[];
  context?: string;
  correlationId?: string;
  errorId?: string;
  affectedSource?: string;
  retryAvailable: boolean;
  diagnosticAvailable?: boolean;
  nextActionText?: string;
}

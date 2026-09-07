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

export interface ScreenStateModel {
  kind: PageStateKind;
  title: string;
  explanation: string;
  correlationId?: string;
  errorId?: string;
  affectedSource?: string;
  retryAvailable: boolean;
  nextActionText?: string;
}

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

import React from 'react';

export type PageStateKind =
  | 'loading'
  | 'empty'
  | 'partial'
  | 'stale'
  | 'permission'
  | 'not-licensed'
  | 'query-limited'
  | 'error'
  | 'monitoring-degraded';

interface PageStateProps {
  kind: PageStateKind;
  title: string;
  message: string;
  correlationId?: string;
  actionLabel?: string;
  onAction?: () => void;
}

const STATE_LABELS: Record<PageStateKind, string> = {
  loading: 'Loading',
  empty: 'Empty',
  partial: 'Partial Data',
  stale: 'Stale Data',
  permission: 'Permission Restricted',
  'not-licensed': 'Not Licensed',
  'query-limited': 'Query Limited',
  error: 'Error',
  'monitoring-degraded': 'Monitoring Degraded',
};

export const PageState: React.FC<PageStateProps> = ({
  kind,
  title,
  message,
  correlationId,
  actionLabel,
  onAction,
}) => (
  <section className={`page-state page-state--${kind}`} role={kind === 'error' ? 'alert' : 'status'}>
    <div className="page-state__label">{STATE_LABELS[kind]}</div>
    <h2>{title}</h2>
    <p>{message}</p>
    {correlationId ? (
      <p className="page-state__correlation">
        Correlation ID: <code>{correlationId}</code>
      </p>
    ) : null}
    {actionLabel && onAction ? (
      <button type="button" className="button button--primary" onClick={onAction}>
        {actionLabel}
      </button>
    ) : null}
  </section>
);

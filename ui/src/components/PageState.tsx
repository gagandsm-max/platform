import React from 'react';
import { PageStateKind, SCREEN_STATE_LABELS } from '../platform/screen-state';

interface PageStateProps {
  kind: PageStateKind;
  title: string;
  message: string;
  context?: string;
  evidence?: readonly string[];
  correlationId?: string;
  errorId?: string;
  actionLabel?: string;
  onAction?: () => void;
  diagnosticLabel?: string;
  onDiagnostic?: () => void;
}

export const PageState: React.FC<PageStateProps> = ({
  kind,
  title,
  message,
  context,
  evidence,
  correlationId,
  errorId,
  actionLabel,
  onAction,
  diagnosticLabel,
  onDiagnostic,
}) => {
  const urgent =
    kind === 'error' || kind === 'monitoring-degraded' || kind === 'source-unavailable';
  const hasActions = Boolean((actionLabel && onAction) || (diagnosticLabel && onDiagnostic));

  return (
    <section
      className={`page-state page-state--${kind}`}
      role={urgent ? 'alert' : 'status'}
      aria-live={urgent ? 'assertive' : 'polite'}
      aria-busy={kind === 'loading' ? true : undefined}
    >
      <div className="page-state__label">{SCREEN_STATE_LABELS[kind]}</div>
      <h2>{title}</h2>
      <p>{message}</p>

      {context ? (
        <p className="page-state__context">
          <strong>Context:</strong> {context}
        </p>
      ) : null}

      {evidence && evidence.length > 0 ? (
        <div className="page-state__evidence">
          <h3>Available evidence</h3>
          <ul>
            {evidence.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </div>
      ) : null}

      {correlationId || errorId ? (
        <dl className="page-state__ids">
          {correlationId ? (
            <>
              <dt>Correlation ID</dt>
              <dd><code>{correlationId}</code></dd>
            </>
          ) : null}
          {errorId ? (
            <>
              <dt>Error ID</dt>
              <dd><code>{errorId}</code></dd>
            </>
          ) : null}
        </dl>
      ) : null}

      {hasActions ? (
        <div className="page-state__actions">
          {actionLabel && onAction ? (
            <button type="button" className="button button--primary" onClick={onAction}>
              {actionLabel}
            </button>
          ) : null}
          {diagnosticLabel && onDiagnostic ? (
            <button type="button" className="button button--secondary" onClick={onDiagnostic}>
              {diagnosticLabel}
            </button>
          ) : null}
        </div>
      ) : null}
    </section>
  );
};

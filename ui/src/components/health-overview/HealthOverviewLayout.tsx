import React from 'react';
import {
  ENTITY_EVIDENCE_STATUS_LABELS,
  HEALTH_OVERVIEW_STATE_LABELS,
  type HealthOverviewColumn,
  type HealthOverviewRowPlaceholder,
  type HealthOverviewVm,
} from '../../platform/health-overview';
import { PageState } from '../PageState';
import type { PageStateKind } from '../../platform/screen-state';

interface HealthOverviewLayoutProps {
  readonly model: HealthOverviewVm;
  readonly onRetry?: () => void;
  readonly onDiagnostic?: () => void;
}

const toPageStateKind = (kind: HealthOverviewVm['presentationState']): PageStateKind => kind;

const cellValue = (
  column: HealthOverviewColumn,
  row: HealthOverviewRowPlaceholder,
): React.ReactNode => {
  if (column.id === 'entity' || column.id === 'host') {
    return (
      <>
        <span className="health-overview__primary">{row.primaryLabel}</span>
        <span className="health-overview__secondary">{row.secondaryLabel}</span>
      </>
    );
  }
  if (column.id === 'status') {
    return (
      <span className="health-overview__status">
        <span className="health-overview__status-text">
          {ENTITY_EVIDENCE_STATUS_LABELS[row.status]}
        </span>
        <span className="sr-only">. {row.statusExplanation}</span>
      </span>
    );
  }
  if (column.id === 'site') {
    return row.siteLabel;
  }
  if (column.id === 'lastUpdate') {
    return row.lastUpdateLabel;
  }
  return row.cells[column.id] ?? '—';
};

export const HealthOverviewLayout: React.FC<HealthOverviewLayoutProps> = ({
  model,
  onRetry,
  onDiagnostic,
}) => {
  const stateLabel = HEALTH_OVERVIEW_STATE_LABELS[model.presentationState];
  const showTable = model.rows.length > 0;
  const pageKind = toPageStateKind(model.presentationState);

  return (
    <div
      className="health-overview"
      data-ui-code={model.uiCode}
      data-workspace-id={model.workspaceId}
      data-presentation-state={model.presentationState}
      data-data-bound={model.dataBound ? 'true' : 'false'}
    >
      <section
        className="health-overview__state-banner"
        aria-label={`${model.label} presentation state`}
        data-presentation-state={model.presentationState}
      >
        <span className="health-overview__state-label">{stateLabel}</span>
        <p className="health-overview__state-summary">
          Active view: <strong>{model.activeTab}</strong>
          <span aria-hidden="true"> · </span>
          Capability: {model.capabilityStatus === 'PENDING_BACKEND' ? 'pending' : 'integrated'}
          <span aria-hidden="true"> · </span>
          Live data binding: {model.dataBound ? 'bound' : 'not bound'}
        </p>
      </section>

      <PageState
        kind={pageKind}
        title={model.stateCopy.title}
        message={model.stateCopy.explanation}
        context={model.stateCopy.nextAction}
        evidence={model.stateCopy.evidence}
        actionLabel={model.stateCopy.retryAvailable ? 'Retry when API is bound' : undefined}
        onAction={model.stateCopy.retryAvailable ? onRetry : undefined}
        diagnosticLabel={
          model.stateCopy.diagnosticAvailable ? 'Open platform diagnostics' : undefined
        }
        onDiagnostic={model.stateCopy.diagnosticAvailable ? onDiagnostic : undefined}
      />

      <section className="health-overview__panels" aria-label={`${model.domain} health summary panels`}>
        {model.panels.map((panel) => (
          <article key={panel.id} className="health-overview__panel surface-card">
            <h3>{panel.title}</h3>
            <p className="health-overview__panel-value" data-placeholder="true">
              {panel.valueLabel}
            </p>
            <p className="health-overview__panel-purpose">{panel.purpose}</p>
            <p className="health-overview__panel-note">{panel.evidenceNote}</p>
          </article>
        ))}
      </section>

      <section className="health-overview__table-region" aria-label={`${model.label} entity table`}>
        <div className="health-overview__table-header">
          <h3>{model.domain === 'Network' ? 'Network entities' : 'Infrastructure hosts'}</h3>
          <p>
            Column contract prepared for future server-side pagination and filtering.
            Large estates must not be loaded entirely into the browser.
          </p>
        </div>

        <div
          className="health-overview__table-scroll"
          role="region"
          tabIndex={0}
          aria-label="Scrollable health table"
        >
          <table className="health-overview__table">
            <caption className="sr-only">
              {model.label} columns for {model.activeTab}. Values are placeholders until a Core
              read API is bound.
            </caption>
            <thead>
              <tr>
                {model.columns.map((column) => (
                  <th key={column.id} scope="col" title={column.description}>
                    {column.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {showTable ? (
                model.rows.map((row) => (
                  <tr key={row.id} data-evidence-status={row.status}>
                    {model.columns.map((column) => (
                      <td key={column.id}>{cellValue(column, row)}</td>
                    ))}
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={model.columns.length}>
                    <div className="health-overview__empty-rows" role="status">
                      No entity rows are bound. Table structure is ready for authorized Core
                      results; this is not a healthy-zero estate.
                    </div>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>

      <section
        className="health-overview__safeguards notice notice--info"
        aria-label="Monitoring evidence safeguards"
      >
        <strong>Evidence safeguards</strong>
        <ul>
          {model.safeguardingNotes.map((note) => (
            <li key={note}>{note}</li>
          ))}
        </ul>
      </section>
    </div>
  );
};

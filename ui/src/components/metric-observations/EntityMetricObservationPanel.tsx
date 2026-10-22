import React, { useEffect, useMemo, useState } from 'react';
import { PageState } from '../PageState';
import {
  MetricObservationBinding,
  isCompleteMetricObservationBinding,
} from '../../platform/metric-observations';
import {
  MetricObservationController,
  MetricObservationPanelView,
  createUnboundMetricObservationView,
} from '../../platform/metric-observation-state';

export interface EntityMetricObservationPanelProps {
  /**
   * Optional explicit tenant/entity/query + injected authenticated transport.
   * Incomplete/absent binding renders SOURCE_UNAVAILABLE and performs zero requests.
   * FOUNDATION_ACCESS must never be treated as authorization for this panel.
   */
  binding?: MetricObservationBinding | null;
  /** Optional heading override for embedding in UI-010 Overview/Monitoring. */
  heading?: string;
}

function formatDisplayValue(item: MetricObservationPanelView['observations'][number]): string {
  if (item.counterValue != null) {
    // Exact Counter64 decimal text — never Number()/parseFloat().
    return item.counterValue;
  }
  if (item.value != null) {
    return String(item.value);
  }
  return '—';
}

function qualityLabel(quality: number): string {
  switch (quality) {
    case 0:
      return 'UNKNOWN (0)';
    case 1:
      return 'GOOD (1)';
    case 2:
      return 'DEGRADED (2)';
    default:
      return `INVALID (${quality})`;
  }
}

export const EntityMetricObservationPanel: React.FC<EntityMetricObservationPanelProps> = ({
  binding = null,
  heading = 'Metric observations',
}) => {
  const controller = useMemo(() => new MetricObservationController(), []);
  const [view, setView] = useState<MetricObservationPanelView>(createUnboundMetricObservationView);

  useEffect(() => {
    const unsubscribe = controller.subscribe(setView);
    return () => {
      unsubscribe();
      controller.dispose();
    };
  }, [controller]);

  useEffect(() => {
    controller.setBinding(binding);
  }, [controller, binding]);

  const bound = isCompleteMetricObservationBinding(binding);
  const showTable = view.phase === 'ready' && view.observations.length > 0;

  return (
    <section
      className="metric-observation-panel"
      aria-labelledby="metric-observation-heading"
      data-phase={view.phase}
      data-observation-state={view.observationState ?? 'none'}
      data-freshness={view.freshness ?? 'none'}
      data-bound={bound ? 'true' : 'false'}
      data-truncated={view.truncated ? 'true' : 'false'}
    >
      <header className="metric-observation-panel__header">
        <h2 id="metric-observation-heading">{heading}</h2>
        <p className="metric-observation-panel__subtitle">
          Bounded per-entity observations with explicit quality, freshness and provenance.
          Missing or degraded telemetry is never presented as device-down.
        </p>
      </header>

      {view.request ? (
        <dl className="metric-observation-panel__identity">
          <div>
            <dt>Tenant</dt>
            <dd><code>{view.request.tenantId}</code></dd>
          </div>
          <div>
            <dt>Entity</dt>
            <dd><code>{view.request.entityId}</code></dd>
          </div>
          <div>
            <dt>Metric</dt>
            <dd><code>{view.request.metricId}</code></dd>
          </div>
          <div>
            <dt>Value kind</dt>
            <dd>{view.request.valueKind}</dd>
          </div>
          <div>
            <dt>Window</dt>
            <dd>
              <time dateTime={view.request.from}>{view.request.from}</time>
              {' → '}
              <time dateTime={view.request.to}>{view.request.to}</time>
              {' (to exclusive)'}
            </dd>
          </div>
          <div>
            <dt>Limit</dt>
            <dd>{view.request.limit}</dd>
          </div>
        </dl>
      ) : null}

      <div className="metric-observation-panel__state">
        <PageState
          kind={view.pageKind}
          title={view.title}
          message={view.message}
          context={
            view.freshness
              ? `Freshness: ${view.freshness}`
                + (view.observationState ? ` · State: ${view.observationState}` : '')
                + (view.truncated ? ' · Truncated' : '')
              : view.observationState
                ? `State: ${view.observationState}`
                : undefined
          }
          evidence={[
            ...(view.provenance
              ? [
                `Provenance class: ${view.provenance.provenanceClass}`,
                `Source view: ${view.provenance.sourceView}`,
                `Ordering: ${view.provenance.ordering}`,
              ]
              : []),
            ...(view.qualityNote ? [`Quality note: ${view.qualityNote}`] : []),
            'FOUNDATION_ACCESS is presentation-only and never authorizes metric reads.',
            'NO_DATA / STALE / MONITORING_DEGRADED / UNKNOWN / SOURCE_UNAVAILABLE must not be collapsed to DEVICE_DOWN.',
          ]}
          correlationId={view.correlationId ?? undefined}
          errorId={view.errorCode ?? undefined}
          actionLabel={view.retryAvailable ? 'Retry' : undefined}
          onAction={view.retryAvailable ? () => controller.retry() : undefined}
        />
      </div>

      {showTable ? (
        <div className="metric-observation-panel__table-wrap" role="region" aria-label="Observation rows">
          <table className="metric-observation-panel__table">
            <thead>
              <tr>
                <th scope="col">Timestamp</th>
                <th scope="col">Value</th>
                <th scope="col">Quality</th>
                <th scope="col">Flags</th>
                <th scope="col">Sensor</th>
                <th scope="col">Sequence</th>
                <th scope="col">Protocol</th>
              </tr>
            </thead>
            <tbody>
              {view.observations.map((row) => (
                <tr key={`${row.timestamp}:${row.sequenceId ?? ''}:${row.counterValue ?? row.value}`}>
                  <td><time dateTime={row.timestamp}>{row.timestamp}</time></td>
                  <td>
                    <code data-value-kind={row.counterValue != null ? 'COUNTER64' : 'NUMERIC'}>
                      {formatDisplayValue(row)}
                    </code>
                  </td>
                  <td>{qualityLabel(row.quality)}</td>
                  <td><code>{String(row.flags)}</code></td>
                  <td>{row.sensorId ?? '—'}</td>
                  <td>{row.sequenceId ?? '—'}</td>
                  <td>{row.sourceProtocol ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
};

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
  selectRenderableMetricObservationView,
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

  // Render-time ownership gate: reject stored React state that belongs to a
  // previous binding/transport before effects clear the controller.
  const displayView = selectRenderableMetricObservationView(view, binding);
  const bound = isCompleteMetricObservationBinding(binding);
  const showTable = displayView.phase === 'ready' && displayView.observations.length > 0;
  const retryEligible = displayView.retryAvailable;

  return (
    <section
      className="metric-observation-panel"
      aria-labelledby="metric-observation-heading"
      data-phase={displayView.phase}
      data-observation-state={displayView.observationState ?? 'none'}
      data-freshness={displayView.freshness ?? 'none'}
      data-bound={bound ? 'true' : 'false'}
      data-truncated={displayView.truncated ? 'true' : 'false'}
    >
      <header className="metric-observation-panel__header">
        <h2 id="metric-observation-heading">{heading}</h2>
        <p className="metric-observation-panel__subtitle">
          Bounded per-entity observations with explicit quality, freshness and provenance.
          Missing or degraded telemetry is never presented as device-down.
        </p>
      </header>

      {displayView.request ? (
        <dl className="metric-observation-panel__identity">
          <div>
            <dt>Tenant</dt>
            <dd><code>{displayView.request.tenantId}</code></dd>
          </div>
          <div>
            <dt>Entity</dt>
            <dd><code>{displayView.request.entityId}</code></dd>
          </div>
          <div>
            <dt>Metric</dt>
            <dd><code>{displayView.request.metricId}</code></dd>
          </div>
          <div>
            <dt>Value kind</dt>
            <dd>{displayView.request.valueKind}</dd>
          </div>
          <div>
            <dt>Window</dt>
            <dd>
              <time dateTime={displayView.request.from}>{displayView.request.from}</time>
              {' → '}
              <time dateTime={displayView.request.to}>{displayView.request.to}</time>
              {' (to exclusive)'}
            </dd>
          </div>
          <div>
            <dt>Limit</dt>
            <dd>{displayView.request.limit}</dd>
          </div>
        </dl>
      ) : null}

      <div className="metric-observation-panel__state">
        <PageState
          kind={displayView.pageKind}
          title={displayView.title}
          message={displayView.message}
          context={
            displayView.freshness
              ? `Freshness: ${displayView.freshness}`
                + (displayView.observationState ? ` · State: ${displayView.observationState}` : '')
                + (displayView.truncated ? ' · Truncated' : '')
              : displayView.observationState
                ? `State: ${displayView.observationState}`
                : undefined
          }
          evidence={[
            ...(displayView.provenance
              ? [
                `Provenance class: ${displayView.provenance.provenanceClass}`,
                `Source view: ${displayView.provenance.sourceView}`,
                `Ordering: ${displayView.provenance.ordering}`,
              ]
              : []),
            ...(displayView.qualityNote ? [`Quality note: ${displayView.qualityNote}`] : []),
            'FOUNDATION_ACCESS is presentation-only and never authorizes metric reads.',
            'NO_DATA / STALE / MONITORING_DEGRADED / UNKNOWN / SOURCE_UNAVAILABLE must not be collapsed to DEVICE_DOWN.',
          ]}
          correlationId={displayView.correlationId ?? undefined}
          errorId={displayView.errorCode ?? undefined}
          actionLabel={retryEligible ? 'Retry' : undefined}
          onAction={retryEligible ? () => controller.retry() : undefined}
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
              {displayView.observations.map((row) => (
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

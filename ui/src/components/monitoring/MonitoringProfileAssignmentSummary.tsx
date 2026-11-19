import React from 'react';
import {
  buildMonitoringProfileAssignmentVm,
  type MonitoringProfileAssignmentBuildInput,
  type MonitoringProfileAssignmentViewModel,
} from '../../platform/monitoring/assignment-presentation';
import { PageState } from '../PageState';

export interface MonitoringProfileAssignmentSummaryProps {
  /** Optional inert presentation input — never treated as a live Core binding here. */
  readonly input?: MonitoringProfileAssignmentBuildInput;
  /** Optional pre-built view model for isolated tests; when omitted, built from input. */
  readonly model?: MonitoringProfileAssignmentViewModel;
}

/**
 * Inert, read-only BUILD-070 monitoring profile assignment summary.
 *
 * Not mounted in App, not connected to backend/customer data, and exposes no
 * mutation controls. Uses PageState / screen-state conventions for loading,
 * empty, permission-restricted, stale and error.
 */
export const MonitoringProfileAssignmentSummary: React.FC<
  MonitoringProfileAssignmentSummaryProps
> = ({ input, model: modelProp }) => {
  const model = modelProp ?? buildMonitoringProfileAssignmentVm(input);

  return (
    <section
      className="monitoring-profile-assignment-summary"
      data-inert="true"
      data-data-bound="false"
      data-presentation-state={model.presentationState}
      aria-label="Monitoring profile assignment summary"
    >
      <PageState
        kind={model.pageStateKind}
        title={model.stateCopy.title}
        message={model.stateCopy.explanation}
        context={model.stateCopy.nextAction}
        evidence={model.stateCopy.evidence}
      />

      {model.showAssignmentFields ? (
        <section
          className="monitoring-profile-assignment-summary__fields"
          aria-label="Verified monitoring profile assignment fields"
        >
          <h3>Verified assignment fields</h3>
          <p className="monitoring-profile-assignment-summary__fields-note">
            Read-only projection. Values are verified optional contract fields only;
            missing fields stay absent and do not imply device down or healthy.
          </p>
          <dl>
            {model.fieldDisplays.map((field) => (
              <div key={field.id} className="monitoring-profile-assignment-summary__field">
                <dt id={`mpa-field-${field.id}-label`}>{field.label}</dt>
                <dd
                  aria-labelledby={`mpa-field-${field.id}-label`}
                  data-field-id={field.id}
                >
                  <code>{field.value}</code>
                </dd>
              </div>
            ))}
          </dl>
        </section>
      ) : null}

      <section
        className="monitoring-profile-assignment-summary__safeguards notice notice--info"
        aria-label="Monitoring profile assignment safeguards"
      >
        <strong>Evidence safeguards</strong>
        <ul>
          {model.safeguardingNotes.map((note) => (
            <li key={note}>{note}</li>
          ))}
        </ul>
      </section>
    </section>
  );
};

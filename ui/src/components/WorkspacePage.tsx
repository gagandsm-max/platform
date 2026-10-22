import React from 'react';
import { AccessContext, WorkspaceDefinition } from '../platform/navigation';
import type { MetricObservationBinding } from '../platform/metric-observations';
import { WorkspaceHost } from './WorkspaceHost';

interface WorkspacePageProps {
  workspace: WorkspaceDefinition;
  access: AccessContext;
  onNavigate: (workspace: WorkspaceDefinition) => void;
  /**
   * Optional authenticated metric-observation binding for UI-010.
   * Absent/incomplete → panel SOURCE_UNAVAILABLE with zero requests.
   */
  metricObservationBinding?: MetricObservationBinding | null;
}

/** Thin route adapter; remount host on workspace id change to clear tab state. */
export const WorkspacePage: React.FC<WorkspacePageProps> = ({
  workspace,
  access,
  onNavigate,
  metricObservationBinding = null,
}) => (
  <WorkspaceHost
    key={workspace.id}
    workspace={workspace}
    access={access}
    onNavigate={onNavigate}
    metricObservationBinding={metricObservationBinding}
  />
);

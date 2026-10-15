import React, { useMemo } from 'react';
import {
  buildNetworkHealthOverviewVm,
  screenDataStateToHealthPresentation,
  type HealthOverviewPresentationState,
} from '../../platform/health-overview';
import type { ScreenDataState } from '../../platform/screen-state';
import type { WorkspaceDefinition } from '../../platform/navigation';
import { HealthOverviewLayout } from './HealthOverviewLayout';

interface NetworkHealthOverviewProps {
  readonly workspace: WorkspaceDefinition;
  readonly activeTab?: string;
  /** Optional live screen data-state from a future query layer. */
  readonly dataState?: ScreenDataState;
  /** Explicit presentation override for controlled demos/tests only. */
  readonly presentationState?: HealthOverviewPresentationState;
  readonly dataBound?: boolean;
}

export const NetworkHealthOverview: React.FC<NetworkHealthOverviewProps> = ({
  workspace,
  activeTab,
  dataState,
  presentationState,
  dataBound = false,
}) => {
  const model = useMemo(
    () =>
      buildNetworkHealthOverviewVm({
        activeTab: activeTab ?? workspace.tabs[0] ?? 'Overview',
        capabilityStatus: workspace.backendCapability,
        dataBound,
        presentationState:
          presentationState
          ?? (dataBound && dataState
            ? screenDataStateToHealthPresentation(dataState)
            : undefined),
      }),
    [
      activeTab,
      dataBound,
      dataState,
      presentationState,
      workspace.backendCapability,
      workspace.tabs,
    ],
  );

  return <HealthOverviewLayout model={model} />;
};

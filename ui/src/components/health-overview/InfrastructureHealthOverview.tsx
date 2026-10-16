import React, { useMemo } from 'react';
import {
  buildInfrastructureHealthOverviewVm,
  resolveHealthOverviewProviderBinding,
  type HealthOverviewPresentationState,
} from '../../platform/health-overview';
import type { ScreenDataState } from '../../platform/screen-state';
import type { WorkspaceDefinition } from '../../platform/navigation';
import { HealthOverviewLayout } from './HealthOverviewLayout';

interface InfrastructureHealthOverviewProps {
  readonly workspace: WorkspaceDefinition;
  readonly activeTab?: string;
  /** Optional live screen data-state from a future query layer. */
  readonly dataState?: ScreenDataState;
  /** Explicit presentation override for controlled demos/tests only. */
  readonly presentationState?: HealthOverviewPresentationState;
  readonly dataBound?: boolean;
}

export const InfrastructureHealthOverview: React.FC<InfrastructureHealthOverviewProps> = ({
  workspace,
  activeTab,
  dataState,
  presentationState,
  dataBound = false,
}) => {
  const model = useMemo(() => {
    const binding = resolveHealthOverviewProviderBinding({
      dataBound,
      dataState,
      presentationState,
    });
    return buildInfrastructureHealthOverviewVm({
      activeTab: activeTab ?? workspace.tabs[0] ?? 'Overview',
      capabilityStatus: workspace.backendCapability,
      dataBound: binding.dataBound,
      presentationState: binding.presentationState,
    });
  }, [
    activeTab,
    dataBound,
    dataState,
    presentationState,
    workspace.backendCapability,
    workspace.tabs,
  ]);

  return <HealthOverviewLayout model={model} />;
};

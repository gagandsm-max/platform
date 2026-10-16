import React, { useMemo, useState } from 'react';
import { PageState } from './PageState';
import { InfrastructureHealthOverview } from './health-overview/InfrastructureHealthOverview';
import { NetworkHealthOverview } from './health-overview/NetworkHealthOverview';
import {
  AccessContext,
  WorkspaceDefinition,
  foundationDataStateForCapability,
  nextWorkspaceTabIndex,
  secondaryWorkspacesInDomain,
  workspaceTabDomId,
  workspaceTabPanelDomId,
} from '../platform/navigation';
import { resolveHealthOverviewHostDataState } from '../platform/health-overview';
import type { PageStateKind, ScreenDataState } from '../platform/screen-state';

interface WorkspaceHostProps {
  workspace: WorkspaceDefinition;
  access: AccessContext;
  onNavigate: (workspace: WorkspaceDefinition) => void;
  /**
   * Optional live data-state from a future query layer.
   * When omitted, foundation uses a non-Normal placeholder independent of capability status.
   */
  dataState?: ScreenDataState;
}

/**
 * Generic host for Platform_09 §5 workspaces.
 * Capability availability is displayed separately from authoritative screen/data state.
 */
export const WorkspaceHost: React.FC<WorkspaceHostProps> = ({
  workspace,
  access,
  onNavigate,
  dataState,
}) => {
  // Tab selection is indexed (not label-keyed) and remounted via key={workspace.id} from the parent.
  const [activeTabIndex, setActiveTabIndex] = useState(0);
  const safeTabIndex = workspace.tabs.length === 0
    ? 0
    : Math.min(activeTabIndex, workspace.tabs.length - 1);
  const activeTabLabel = workspace.tabs[safeTabIndex];

  const siblings = useMemo(
    () => secondaryWorkspacesInDomain(workspace.domain, access),
    [workspace.domain, access],
  );

  const moveTabFocus = (currentIndex: number, key: string) => {
    const nextIndex = nextWorkspaceTabIndex(currentIndex, workspace.tabs.length, key);
    if (nextIndex === currentIndex && !['Home', 'End', 'ArrowLeft', 'ArrowRight'].includes(key)) {
      return;
    }
    setActiveTabIndex(nextIndex);
    window.requestAnimationFrame(() => {
      document.getElementById(workspaceTabDomId(workspace.id, nextIndex))?.focus();
    });
  };

  const capabilityPending = workspace.backendCapability === 'PENDING_BACKEND';
  const isHealthOverview = workspace.id === 'ui-009' || workspace.id === 'ui-029';
  // UI-009/UI-029 unbound preview is Source Unavailable; other workspaces keep foundation empty.
  const resolvedDataState: PageStateKind = isHealthOverview
    ? resolveHealthOverviewHostDataState(dataState)
    : (dataState ?? foundationDataStateForCapability(workspace.backendCapability));

  return (
    <div className="workspace-host" data-workspace-id={workspace.id} data-ui-code={workspace.uiCode}>
      <header className="workspace-header">
        <div>
          <p className="eyebrow">
            <span>{workspace.uiCode}</span>
            <span aria-hidden="true"> · </span>
            <span>{workspace.domain}</span>
          </p>
          <h1 id="workspace-title">{workspace.label}</h1>
          <p className="workspace-description">{workspace.description}</p>
          <p className="workspace-roles">
            <span className="sr-only">Primary roles: </span>
            Presentation roles: {workspace.primaryRoles}. Server-side authorization remains authoritative.
          </p>
        </div>
        <div className="workspace-header__badges">
          <span className="status-pill status-pill--foundation">{workspace.uiCode}</span>
          <span
            className={capabilityPending ? 'status-pill status-pill--pending' : 'status-pill status-pill--ready'}
            data-capability={workspace.backendCapability}
          >
            {capabilityPending ? 'Capability pending' : 'Capability integrated'}
          </span>
          <span className="status-pill" data-data-state={resolvedDataState}>
            Data state: {resolvedDataState}
          </span>
        </div>
      </header>

      {siblings.length > 1 ? (
        <nav className="workspace-secondary-nav" aria-label={`${workspace.domain} workspaces`}>
          <ul>
            {siblings.map((entry) => (
              <li key={entry.id}>
                <button
                  type="button"
                  className={
                    entry.id === workspace.id
                      ? 'workspace-secondary-nav__link workspace-secondary-nav__link--active'
                      : 'workspace-secondary-nav__link'
                  }
                  aria-current={entry.id === workspace.id ? 'page' : undefined}
                  onClick={() => onNavigate(entry)}
                >
                  {entry.label}
                </button>
              </li>
            ))}
          </ul>
        </nav>
      ) : null}

      {workspace.tabs.length > 0 ? (
        <div className="workspace-tabs" role="tablist" aria-label={`${workspace.label} views`}>
          {workspace.tabs.map((tab, index) => {
            const tabId = workspaceTabDomId(workspace.id, index);
            const panelId = workspaceTabPanelDomId(workspace.id, index);
            const selected = index === safeTabIndex;
            return (
              <button
                key={tabId}
                type="button"
                role="tab"
                id={tabId}
                aria-controls={panelId}
                aria-selected={selected}
                tabIndex={selected ? 0 : -1}
                className={selected ? 'workspace-tab workspace-tab--active' : 'workspace-tab'}
                onClick={() => setActiveTabIndex(index)}
                onKeyDown={(event) => {
                  if (['Home', 'End', 'ArrowLeft', 'ArrowRight'].includes(event.key)) {
                    event.preventDefault();
                    moveTabFocus(index, event.key);
                  }
                }}
              >
                {tab}
              </button>
            );
          })}
        </div>
      ) : null}

      {workspace.supportsTimeContext || workspace.filters.length > 0 ? (
        <section className="workspace-filters" aria-label="Workspace filters and time context">
          <p className="workspace-filters__note">
            Filter and time-context controls are declared by the workspace contract. Live query execution
            remains server-side and is not fabricated in this foundation.
          </p>
          <ul className="workspace-filter-chips">
            {(workspace.supportsTimeContext ? ['Time range', 'Timezone', 'Auto-refresh'] : [])
              .concat(workspace.filters.filter((filter) => !['Time range', 'Timezone', 'Auto-refresh'].includes(filter)).slice(0, 8))
              .map((filter) => (
                <li key={filter}>
                  <span className="filter-chip">{filter}</span>
                </li>
              ))}
          </ul>
        </section>
      ) : null}

      <section
        className="workspace-state-region"
        role="tabpanel"
        id={workspace.tabs.length > 0 ? workspaceTabPanelDomId(workspace.id, safeTabIndex) : undefined}
        aria-labelledby={workspace.tabs.length > 0 ? workspaceTabDomId(workspace.id, safeTabIndex) : 'workspace-title'}
        data-data-state={resolvedDataState}
        data-capability={workspace.backendCapability}
      >
        {workspace.id === 'ui-009' ? (
          <NetworkHealthOverview
            workspace={workspace}
            activeTab={activeTabLabel}
            dataState={dataState}
            dataBound={dataState !== undefined}
          />
        ) : workspace.id === 'ui-029' ? (
          <InfrastructureHealthOverview
            workspace={workspace}
            activeTab={activeTabLabel}
            dataState={dataState}
            dataBound={dataState !== undefined}
          />
        ) : (
          <PageState
            kind={resolvedDataState}
            title={
              dataState
                ? `${workspace.label}`
                : 'No live data-state provider bound'
            }
            message={
              dataState
                ? `Authoritative data state for this workspace is ${resolvedDataState}.`
                : `Capability status (${workspace.backendCapability}) is independent of screen/data state. This foundation does not fabricate Normal operational health from capability integration alone.${activeTabLabel ? ` Active view metadata: ${activeTabLabel}.` : ''}`
            }
            evidence={[
              `Capability availability: ${workspace.backendCapability}`,
              `Resolved data state: ${resolvedDataState}`,
              workspace.valueClaimSensitive
                ? 'Value/MTTR claims are prohibited until the Platform Value evidence engine is integrated.'
                : 'Operational panels will bind to versioned Core APIs when a data-state provider is available.',
            ]}
          />
        )}
      </section>

      {workspace.oivRelevant ? (
        <section className="notice notice--info" aria-label="Operational intelligence disclosure">
          <strong>Operational intelligence:</strong> this workspace may participate in OIV/MTTR evidence
          flows. Unsupported causal claims, fabricated MTTR reduction percentages and fabricated financial
          savings are prohibited. Insufficient evidence must remain representable as inconclusive.
        </section>
      ) : null}

      {workspace.valueClaimSensitive ? (
        <section className="notice notice--warning" aria-label="Platform value disclosure">
          <strong>Platform Value / MTTR:</strong> methodology, cohort, sample-size and assumption disclosure
          remain mandatory before any improvement figure may be shown. No static savings or MTTR percentage
          is embedded in this UI foundation.
        </section>
      ) : null}
    </div>
  );
};

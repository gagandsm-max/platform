import React, { useMemo, useState } from 'react';
import { PageState } from './PageState';
import {
  AccessContext,
  WorkspaceDefinition,
  secondaryWorkspacesInDomain,
} from '../platform/navigation';

interface WorkspaceHostProps {
  workspace: WorkspaceDefinition;
  access: AccessContext;
  onNavigate: (workspace: WorkspaceDefinition) => void;
}

/**
 * Generic host for all Platform_09 workspaces. Backend-unimplemented capabilities use honest
 * empty/capability messaging — never fabricated Normal telemetry or value claims.
 */
export const WorkspaceHost: React.FC<WorkspaceHostProps> = ({
  workspace,
  access,
  onNavigate,
}) => {
  const [activeTab, setActiveTab] = useState(workspace.tabs[0] ?? 'Overview');
  const siblings = useMemo(
    () => secondaryWorkspacesInDomain(workspace.domain, access),
    [workspace.domain, access],
  );

  const capabilityPending = workspace.backendCapability === 'PENDING_BACKEND';
  const dataStateKind = capabilityPending ? 'empty' : 'normal';

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
          {capabilityPending ? (
            <span className="status-pill status-pill--pending">Capability pending</span>
          ) : (
            <span className="status-pill status-pill--ready">Capability integrated</span>
          )}
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
          {workspace.tabs.map((tab) => (
            <button
              key={tab}
              type="button"
              role="tab"
              id={`tab-${workspace.id}-${tab}`}
              aria-selected={tab === activeTab}
              className={tab === activeTab ? 'workspace-tab workspace-tab--active' : 'workspace-tab'}
              onClick={() => setActiveTab(tab)}
            >
              {tab}
            </button>
          ))}
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
        aria-labelledby="workspace-title"
        data-data-state={dataStateKind}
      >
        {capabilityPending ? (
          <PageState
            kind="empty"
            title="Workspace capability not yet integrated"
            message={`The ${workspace.label} route, navigation metadata and state contract are registered. Backend telemetry, mutations and production authorization for this workspace are not available yet, so Platform does not fabricate Normal operational data.`}
            evidence={[
              `Capability status: ${workspace.backendCapability}`,
              `Active view metadata: ${activeTab}`,
              workspace.valueClaimSensitive
                ? 'Value/MTTR claims are prohibited until the Platform Value evidence engine is integrated.'
                : 'Operational panels will bind to versioned Core APIs when this capability lands.',
            ]}
          />
        ) : (
          <PageState
            kind="normal"
            title={workspace.label}
            message="Backend capability is marked integrated for this workspace."
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

import React from 'react';
import { PageState } from './PageState';
import {
  AccessContext,
  PrimaryNavDestination,
  WorkspaceDefinition,
  relatedWorkspacesForCategory,
} from '../platform/navigation';
import { foundationDataStateForCapability } from '../platform/workspace-host-state';

interface CategoryLandingProps {
  destination: PrimaryNavDestination;
  access: AccessContext;
  onNavigateWorkspace: (workspace: WorkspaceDefinition) => void;
  onNavigatePrimary: (destination: PrimaryNavDestination) => void;
  studioChildren?: readonly PrimaryNavDestination[];
}

/**
 * Generic landing for Platform_09 §2.1 primary/application navigation destinations.
 * Not one of the §5 workspaces unless Mother Reference explicitly equates them.
 */
export const CategoryLanding: React.FC<CategoryLandingProps> = ({
  destination,
  access,
  onNavigateWorkspace,
  onNavigatePrimary,
  studioChildren = [],
}) => {
  const related = relatedWorkspacesForCategory(destination, access);
  const dataState = foundationDataStateForCapability(destination.backendCapability);

  return (
    <div
      className="category-landing"
      data-primary-nav-id={destination.id}
      data-primary-nav-kind={destination.kind}
    >
      <header className="workspace-header">
        <div>
          <p className="eyebrow">
            Primary navigation
            {destination.kind === 'compatibility' ? ' · compatibility route' : ''}
            {destination.kind === 'studio' ? ' · studio' : ''}
          </p>
          <h1>{destination.label}</h1>
          <p className="workspace-description">{destination.description}</p>
        </div>
        <div className="workspace-header__badges">
          <span className="status-pill status-pill--foundation">§2.1</span>
          <span
            className={
              destination.backendCapability === 'PENDING_BACKEND'
                ? 'status-pill status-pill--pending'
                : 'status-pill status-pill--ready'
            }
            data-capability={destination.backendCapability}
          >
            {destination.backendCapability === 'PENDING_BACKEND'
              ? 'Capability pending'
              : 'Capability integrated'}
          </span>
        </div>
      </header>

      {studioChildren.length > 0 ? (
        <nav className="workspace-secondary-nav" aria-label="Studio destinations">
          <ul>
            {studioChildren.map((child) => (
              <li key={child.id}>
                <button
                  type="button"
                  className="workspace-secondary-nav__link"
                  onClick={() => onNavigatePrimary(child)}
                >
                  {child.label}
                </button>
              </li>
            ))}
          </ul>
        </nav>
      ) : null}

      {related.length > 0 ? (
        <section aria-label="Related registered workspaces">
          <h2 className="category-landing__section-title">Registered workspaces</h2>
          <ul className="category-landing__workspace-list">
            {related.map((workspace) => (
              <li key={workspace.id}>
                <button
                  type="button"
                  className="workspace-secondary-nav__link"
                  onClick={() => onNavigateWorkspace(workspace)}
                >
                  <strong>{workspace.label}</strong>
                  <span aria-hidden="true"> · </span>
                  <span>{workspace.uiCode}</span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="workspace-state-region" data-data-state={dataState}>
        <PageState
          kind={dataState}
          title="Category capability not fabricating operational data"
          message={`${destination.label} is a primary/application navigation destination. Backend panels for this category are not fabricated in the UI foundation. Use related workspace routes or the command palette to open registered §5 workspaces.`}
          evidence={[
            `Primary destination id: ${destination.id}`,
            `Capability availability: ${destination.backendCapability}`,
            `Resolved foundation data state: ${dataState}`,
          ]}
        />
      </section>
    </div>
  );
};

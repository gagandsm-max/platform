import React from 'react';
import { WorkspaceDefinition } from '../platform/navigation';

interface WorkspacePageProps {
  workspace: WorkspaceDefinition;
}

const FOUNDATION_CAPABILITIES: Record<string, readonly string[]> = {
  home: ['Role-aware application shell', 'Workspace routing contract', 'Explicit page-state framework'],
  dashboards: ['Dashboard library shell', 'Saved-view entry point', 'Context-preserving navigation boundary'],
  inventory: ['Inventory workspace boundary', 'Server-side data contract placeholder', 'Entity drill-down navigation boundary'],
  provisioning: ['Provisioning workspace boundary', 'Discovery workflow entry point', 'Diagnostics-state contract placeholder'],
  topology: ['Topology workspace boundary', 'Layer/view navigation placeholder', 'Graph context preservation boundary'],
  alerts: ['Alert and incident workspace boundary', 'Qualified-state presentation boundary', 'Investigation hand-off boundary'],
  rca: ['Investigation workspace boundary', 'Evidence-oriented result surface', 'INCONCLUSIVE-capable presentation boundary'],
  ncm: ['NCM workspace boundary', 'Approval-aware action surface placeholder', 'Audit hand-off boundary'],
  ipam: ['IPAM workspace boundary', 'Address-space navigation placeholder', 'Conflict/history surface placeholder'],
  reports: ['Reports and SLA workspace boundary', 'Schedule/export surface placeholder', 'Server-side execution boundary'],
  integrations: ['Connector workspace boundary', 'Connector-health state placeholder', 'Test/action boundary'],
  studios: ['Studio launcher boundary', 'Dashboard/Report/Map/MIB/API/Device Pack destinations', 'Permission-aware entry points'],
  administration: ['Administration workspace boundary', 'Security-sensitive action separation', 'Resource-scope presentation boundary'],
  audit: ['Audit workspace boundary', 'Governance evidence surface placeholder', 'Correlation-context placeholder'],
  operations: ['Platform Operations workspace boundary', 'Component-health state placeholder', 'Support diagnostics entry point'],
};

export const WorkspacePage: React.FC<WorkspacePageProps> = ({ workspace }) => {
  const capabilities = FOUNDATION_CAPABILITIES[workspace.id] ?? [];

  return (
    <div className="workspace-page">
      <header className="workspace-header">
        <div>
          <p className="eyebrow">Workspace foundation</p>
          <h1>{workspace.label}</h1>
          <p>{workspace.description}</p>
        </div>
        <span className="status-pill status-pill--foundation">Foundation</span>
      </header>

      <section className="notice notice--info" aria-label="Implementation status">
        <strong>Integration status:</strong> this route and interaction shell are implemented. Backend data,
        mutation actions and production authorization are intentionally not fabricated; each will be wired to
        versioned Core APIs as those contracts become available.
      </section>

      <div className="foundation-grid">
        <section className="surface-card">
          <h2>Foundation delivered</h2>
          <ul className="check-list">
            {capabilities.map((capability) => (
              <li key={capability}>{capability}</li>
            ))}
          </ul>
        </section>

        <section className="surface-card">
          <h2>Required before feature completion</h2>
          <ul className="pending-list">
            <li>Versioned Core API integration and server-side tenant/resource authorization.</li>
            <li>Loading, empty, partial, stale, degraded and error-state wiring to real responses.</li>
            <li>Workflow-specific accessibility, E2E, performance and security validation.</li>
          </ul>
        </section>
      </div>
    </div>
  );
};

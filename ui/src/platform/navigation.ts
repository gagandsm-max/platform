export type WorkspacePermission =
  | 'workspace.home.view'
  | 'workspace.dashboards.view'
  | 'workspace.inventory.view'
  | 'workspace.provisioning.view'
  | 'workspace.topology.view'
  | 'workspace.alerts.view'
  | 'workspace.rca.view'
  | 'workspace.ncm.view'
  | 'workspace.ipam.view'
  | 'workspace.reports.view'
  | 'workspace.integrations.view'
  | 'workspace.studios.view'
  | 'workspace.administration.view'
  | 'workspace.audit.view'
  | 'workspace.operations.view';

export type WorkspaceGroup = 'operate' | 'manage' | 'govern';

export interface WorkspaceDefinition {
  id: string;
  label: string;
  shortLabel: string;
  path: string;
  group: WorkspaceGroup;
  permission: WorkspacePermission;
  description: string;
}

export interface AccessContext {
  permissions: ReadonlySet<WorkspacePermission>;
  multiTenant: boolean;
}

export const WORKSPACES: readonly WorkspaceDefinition[] = [
  {
    id: 'home',
    label: 'Home',
    shortLabel: 'HM',
    path: '/',
    group: 'operate',
    permission: 'workspace.home.view',
    description: 'Operational entry point and role-aware summary.',
  },
  {
    id: 'dashboards',
    label: 'Dashboards',
    shortLabel: 'DB',
    path: '/dashboards',
    group: 'operate',
    permission: 'workspace.dashboards.view',
    description: 'Role-aware system and customer dashboards.',
  },
  {
    id: 'inventory',
    label: 'Inventory',
    shortLabel: 'IN',
    path: '/inventory',
    group: 'operate',
    permission: 'workspace.inventory.view',
    description: 'Tenant-scoped canonical monitored entities.',
  },
  {
    id: 'provisioning',
    label: 'Provisioning & Discovery',
    shortLabel: 'PD',
    path: '/provisioning',
    group: 'operate',
    permission: 'workspace.provisioning.view',
    description: 'Guided onboarding, discovery and provisioning jobs.',
  },
  {
    id: 'topology',
    label: 'Topology & Maps',
    shortLabel: 'TM',
    path: '/topology',
    group: 'operate',
    permission: 'workspace.topology.view',
    description: 'Operational graph, topology and geographic views.',
  },
  {
    id: 'alerts',
    label: 'Alerts & Incidents',
    shortLabel: 'AI',
    path: '/alerts',
    group: 'operate',
    permission: 'workspace.alerts.view',
    description: 'Qualified alerts, incidents and operator workflow.',
  },
  {
    id: 'rca',
    label: 'RCA & Investigations',
    shortLabel: 'RC',
    path: '/rca',
    group: 'operate',
    permission: 'workspace.rca.view',
    description: 'Evidence, causal hypotheses, timelines and investigations.',
  },
  {
    id: 'ncm',
    label: 'Network Configuration',
    shortLabel: 'NC',
    path: '/network-configuration',
    group: 'manage',
    permission: 'workspace.ncm.view',
    description: 'Configuration governance, backup, compliance and jobs.',
  },
  {
    id: 'ipam',
    label: 'IPAM',
    shortLabel: 'IP',
    path: '/ipam',
    group: 'manage',
    permission: 'workspace.ipam.view',
    description: 'Address spaces, prefixes, assignments and conflicts.',
  },
  {
    id: 'reports',
    label: 'Reports & SLA',
    shortLabel: 'RS',
    path: '/reports',
    group: 'manage',
    permission: 'workspace.reports.view',
    description: 'Reporting, exports, schedules and SLA analysis.',
  },
  {
    id: 'integrations',
    label: 'Integrations',
    shortLabel: 'IG',
    path: '/integrations',
    group: 'manage',
    permission: 'workspace.integrations.view',
    description: 'ITSM, notifications and external connectors.',
  },
  {
    id: 'studios',
    label: 'Studios',
    shortLabel: 'ST',
    path: '/studios',
    group: 'manage',
    permission: 'workspace.studios.view',
    description: 'Dashboard, Report, Map, MIB, API and Device Pack Studios.',
  },
  {
    id: 'administration',
    label: 'Administration',
    shortLabel: 'AD',
    path: '/administration',
    group: 'govern',
    permission: 'workspace.administration.view',
    description: 'Tenant, identity, policy, credential and platform administration.',
  },
  {
    id: 'audit',
    label: 'Audit & Governance',
    shortLabel: 'AU',
    path: '/audit',
    group: 'govern',
    permission: 'workspace.audit.view',
    description: 'Security-sensitive activity, governance and audit evidence.',
  },
  {
    id: 'operations',
    label: 'Platform Operations',
    shortLabel: 'PO',
    path: '/platform-operations',
    group: 'govern',
    permission: 'workspace.operations.view',
    description: 'Core, Sensor, Agent and data-service operational health.',
  },
] as const;

export const ALL_WORKSPACE_PERMISSIONS = new Set<WorkspacePermission>(
  WORKSPACES.map((workspace) => workspace.permission),
);

export const findWorkspaceByPath = (path: string): WorkspaceDefinition | undefined => {
  const normalized = path !== '/' && path.endsWith('/') ? path.slice(0, -1) : path;
  return WORKSPACES.find((workspace) => workspace.path === normalized);
};

export const canAccessWorkspace = (
  workspace: WorkspaceDefinition,
  access: AccessContext,
): boolean => access.permissions.has(workspace.permission);

export const visibleWorkspaces = (
  access: AccessContext,
): readonly WorkspaceDefinition[] => WORKSPACES.filter((workspace) => canAccessWorkspace(workspace, access));

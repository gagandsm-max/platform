/**
 * Platform_09 §2.1 primary application navigation.
 *
 * This taxonomy is intentionally separate from the §5 UI-001..UI-098 workspace registry.
 * Category routes are product navigation destinations; they are not §5 workspaces unless
 * Mother Reference explicitly equates them.
 *
 * Presentation identifiers below are UI-local only. Core RBAC remains authoritative.
 */

export type PrimaryNavGroup = 'operate' | 'manage' | 'govern';

/**
 * UI-local presentation gate for primary navigation visibility.
 * Not a Core permission catalog entry.
 */
export type PrimaryNavPresentationPermission = `nav.${string}.view`;

export type PrimaryNavKind = 'category' | 'studio' | 'compatibility';

export interface PrimaryNavDestination {
  readonly id: string;
  readonly label: string;
  readonly shortLabel: string;
  readonly path: string;
  readonly group: PrimaryNavGroup;
  readonly kind: PrimaryNavKind;
  readonly description: string;
  readonly presentationPermission: PrimaryNavPresentationPermission;
  readonly parentId?: string;
  /** When true, shown in the primary sidebar (top-level §2.1 entries). */
  readonly sidebar: boolean;
  /**
   * Optional related §5 workspace ids for category landing discovery.
   * These are associations for navigation UX, not route aliases.
   */
  readonly relatedWorkspaceIds: readonly string[];
  readonly backendCapability: 'PENDING_BACKEND' | 'INTEGRATED';
}

/**
 * Authoritative §2.1 inventory plus:
 * - Studio children from §2.1
 * - `/provisioning` compatibility route (PRD §9: Inventory → Provisioning & Discovery;
 *   not a §2.1 top-level item and not UI-089 Migration Studio)
 */
export const PRIMARY_NAV_DESTINATIONS: readonly PrimaryNavDestination[] = [
  {
    id: 'home',
    label: 'Home',
    shortLabel: 'HM',
    path: '/',
    group: 'operate',
    kind: 'category',
    description: 'Product Home entry from Platform_09 §2.1. Not equated to any single §5 workspace.',
    presentationPermission: 'nav.home.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-001', 'ui-002', 'ui-003'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'dashboards',
    label: 'Dashboards',
    shortLabel: 'DB',
    path: '/dashboards',
    group: 'operate',
    kind: 'category',
    description: 'Dashboard library and role-aware dashboard navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.dashboards.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-001', 'ui-002', 'ui-005'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'inventory',
    label: 'Inventory',
    shortLabel: 'IN',
    path: '/inventory',
    group: 'operate',
    kind: 'category',
    description: 'Canonical monitored-entity inventory navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.inventory.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-009', 'ui-029', 'ui-030', 'ui-042'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    // PRD §9 places Provisioning & Discovery under Inventory. §2.1 does not list it top-level.
    // Retained as a compatibility/category route without mapping to UI-089 Migration Studio.
    id: 'provisioning',
    label: 'Provisioning & Discovery',
    shortLabel: 'PD',
    path: '/provisioning',
    group: 'operate',
    kind: 'compatibility',
    description:
      'Provisioning & Discovery landing (PRD §9 under Inventory). Not a Platform_09 §2.1 top-level item and not Migration Studio (UI-089).',
    presentationPermission: 'nav.provisioning.view',
    parentId: 'inventory',
    sidebar: false,
    relatedWorkspaceIds: [],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'topology',
    label: 'Topology & Maps',
    shortLabel: 'TM',
    path: '/topology',
    group: 'operate',
    kind: 'category',
    description: 'Topology and geographic map navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.topology.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-004'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'alerts',
    label: 'Alerts & Incidents',
    shortLabel: 'AI',
    path: '/alerts',
    group: 'operate',
    kind: 'category',
    description: 'Alert and incident operator navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.alerts.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-055', 'ui-056', 'ui-057', 'ui-059', 'ui-060'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'rca',
    label: 'RCA & Investigations',
    shortLabel: 'RC',
    path: '/rca',
    group: 'operate',
    kind: 'category',
    description: 'Investigation and RCA navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.rca.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-058', 'ui-061', 'ui-096'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'ncm',
    label: 'Network Configuration (NCM)',
    shortLabel: 'NC',
    path: '/network-configuration',
    group: 'manage',
    kind: 'category',
    description: 'NCM governance navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.ncm.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-062', 'ui-063', 'ui-064', 'ui-065'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'ipam',
    label: 'IPAM',
    shortLabel: 'IP',
    path: '/ipam',
    group: 'manage',
    kind: 'category',
    description: 'IP address management navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.ipam.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-066', 'ui-067', 'ui-068', 'ui-069'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'reports',
    label: 'Reports & SLA',
    shortLabel: 'RS',
    path: '/reports',
    group: 'manage',
    kind: 'category',
    description: 'Reporting and SLA navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.reports.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-070', 'ui-071', 'ui-072'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'integrations',
    label: 'Integrations',
    shortLabel: 'IG',
    path: '/integrations',
    group: 'manage',
    kind: 'category',
    description: 'External connector navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.integrations.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-080', 'ui-081'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'studios',
    label: 'Studios',
    shortLabel: 'ST',
    path: '/studios',
    group: 'manage',
    kind: 'category',
    description: 'Studio launcher (Platform_09 §2.1). Not Migration Studio (UI-089).',
    presentationPermission: 'nav.studios.view',
    sidebar: true,
    relatedWorkspaceIds: [],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'studio-dashboard',
    label: 'Dashboard Studio',
    shortLabel: 'DS',
    path: '/studios/dashboard',
    group: 'manage',
    kind: 'studio',
    description: 'Dashboard Studio child of Studios (Platform_09 §2.1).',
    presentationPermission: 'nav.studios.dashboard.view',
    parentId: 'studios',
    sidebar: false,
    relatedWorkspaceIds: [],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'studio-report',
    label: 'Report Studio',
    shortLabel: 'RS',
    path: '/studios/report',
    group: 'manage',
    kind: 'studio',
    description: 'Report Studio child of Studios (Platform_09 §2.1).',
    presentationPermission: 'nav.studios.report.view',
    parentId: 'studios',
    sidebar: false,
    relatedWorkspaceIds: [],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'studio-map',
    label: 'Map Studio',
    shortLabel: 'MS',
    path: '/studios/map',
    group: 'manage',
    kind: 'studio',
    description: 'Map Studio child of Studios (Platform_09 §2.1).',
    presentationPermission: 'nav.studios.map.view',
    parentId: 'studios',
    sidebar: false,
    relatedWorkspaceIds: [],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'studio-mib',
    label: 'MIB Studio',
    shortLabel: 'MB',
    path: '/studios/mib',
    group: 'manage',
    kind: 'studio',
    description: 'MIB Studio child of Studios (Platform_09 §2.1).',
    presentationPermission: 'nav.studios.mib.view',
    parentId: 'studios',
    sidebar: false,
    relatedWorkspaceIds: [],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'studio-api',
    label: 'API Studio',
    shortLabel: 'AS',
    path: '/studios/api',
    group: 'manage',
    kind: 'studio',
    description: 'API Studio child of Studios (Platform_09 §2.1).',
    presentationPermission: 'nav.studios.api.view',
    parentId: 'studios',
    sidebar: false,
    relatedWorkspaceIds: [],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'studio-device-pack',
    label: 'Device Pack Studio',
    shortLabel: 'DP',
    path: '/studios/device-pack',
    group: 'manage',
    kind: 'studio',
    description: 'Device Pack Studio child of Studios (Platform_09 §2.1).',
    presentationPermission: 'nav.studios.device-pack.view',
    parentId: 'studios',
    sidebar: false,
    relatedWorkspaceIds: [],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'administration',
    label: 'Administration',
    shortLabel: 'AD',
    path: '/administration',
    group: 'govern',
    kind: 'category',
    description: 'Platform administration navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.administration.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-084', 'ui-085'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'audit',
    label: 'Audit & Governance',
    shortLabel: 'AU',
    path: '/audit',
    group: 'govern',
    kind: 'category',
    description: 'Audit and governance navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.audit.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-082', 'ui-083', 'ui-084'],
    backendCapability: 'PENDING_BACKEND',
  },
  {
    id: 'operations',
    label: 'Platform Operations',
    shortLabel: 'PO',
    path: '/platform-operations',
    group: 'govern',
    kind: 'category',
    description: 'Core/Sensor/Agent/data-plane operations navigation (Platform_09 §2.1).',
    presentationPermission: 'nav.operations.view',
    sidebar: true,
    relatedWorkspaceIds: ['ui-073', 'ui-074', 'ui-075', 'ui-076', 'ui-077', 'ui-078', 'ui-079'],
    backendCapability: 'PENDING_BACKEND',
  },
] as const;

export const PRIMARY_NAV_GROUPS: readonly PrimaryNavGroup[] = ['operate', 'manage', 'govern'] as const;

export const AUTHORITATIVE_PRIMARY_SIDEBAR_LABELS = [
  'Home',
  'Dashboards',
  'Inventory',
  'Topology & Maps',
  'Alerts & Incidents',
  'RCA & Investigations',
  'Network Configuration (NCM)',
  'IPAM',
  'Reports & SLA',
  'Integrations',
  'Studios',
  'Administration',
  'Audit & Governance',
  'Platform Operations',
] as const;

export const STUDIO_CHILD_LABELS = [
  'Dashboard Studio',
  'Report Studio',
  'Map Studio',
  'MIB Studio',
  'API Studio',
  'Device Pack Studio',
] as const;

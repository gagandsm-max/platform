import test from 'node:test';
import assert from 'node:assert/strict';
import {
  ALL_WORKSPACE_PERMISSIONS,
  WORKSPACE_GROUPS,
  WORKSPACES,
  canAccessWorkspace,
  findWorkspaceByPath,
  firstAccessibleWorkspace,
  normalizeWorkspacePath,
  resolveWorkspaceRoute,
  visibleWorkspaces,
} from '../.test-dist/platform/navigation.js';
import {
  REQUIRED_SCREEN_STATES,
  SCREEN_STATE_LABELS,
} from '../.test-dist/platform/screen-state.js';
import { GLOBAL_SHELL_CAPABILITIES } from '../.test-dist/platform/shell-contract.js';

const EXPECTED_WORKSPACES = [
  'Home',
  'Dashboards',
  'Inventory',
  'Provisioning & Discovery',
  'Topology & Maps',
  'Alerts & Incidents',
  'RCA & Investigations',
  'Network Configuration',
  'IPAM',
  'Reports & SLA',
  'Integrations',
  'Studios',
  'Administration',
  'Audit & Governance',
  'Platform Operations',
];

const EXPECTED_PERMISSION_BY_ID = {
  home: 'workspace.home.view',
  dashboards: 'workspace.dashboards.view',
  inventory: 'workspace.inventory.view',
  provisioning: 'workspace.provisioning.view',
  topology: 'workspace.topology.view',
  alerts: 'workspace.alerts.view',
  rca: 'workspace.rca.view',
  ncm: 'workspace.ncm.view',
  ipam: 'workspace.ipam.view',
  reports: 'workspace.reports.view',
  integrations: 'workspace.integrations.view',
  studios: 'workspace.studios.view',
  administration: 'workspace.administration.view',
  audit: 'workspace.audit.view',
  operations: 'workspace.operations.view',
};

const EXPECTED_SCREEN_STATE_LABELS = [
  'Loading',
  'Empty',
  'Partial Data',
  'Stale Data',
  'Permission Restricted',
  'Not Licensed',
  'Query Limited',
  'Error',
  'Monitoring Degraded',
  'Normal',
];

const EXPECTED_SHELL_CAPABILITIES = [
  'authorized-tenant-selector',
  'global-search',
  'breadcrumbs',
  'time-range',
  'timezone',
  'comparison-period',
  'auto-refresh',
  'quick-actions',
  'notifications',
  'contextual-help',
  'account-menu',
  'favorites-recent',
  'command-palette',
  'theme',
];

test('workspace registry contains the governed primary navigation', () => {
  assert.deepEqual(WORKSPACES.map((workspace) => workspace.label), EXPECTED_WORKSPACES);
});

test('workspace ids, labels, paths and permissions are unique', () => {
  const ids = new Set(WORKSPACES.map((workspace) => workspace.id));
  const labels = new Set(WORKSPACES.map((workspace) => workspace.label));
  const paths = new Set(WORKSPACES.map((workspace) => workspace.path));
  const permissions = new Set(WORKSPACES.map((workspace) => workspace.permission));

  assert.equal(ids.size, WORKSPACES.length);
  assert.equal(labels.size, WORKSPACES.length);
  assert.equal(paths.size, WORKSPACES.length);
  assert.equal(permissions.size, WORKSPACES.length);
  assert.equal(ALL_WORKSPACE_PERMISSIONS.size, WORKSPACES.length);
  assert.ok(WORKSPACES.every((workspace) => workspace.path.startsWith('/')));
});

test('workspace permission mapping is explicit and stable', () => {
  assert.deepEqual(
    Object.fromEntries(WORKSPACES.map((workspace) => [workspace.id, workspace.permission])),
    EXPECTED_PERMISSION_BY_ID,
  );
});

test('every workspace belongs to a valid navigation group', () => {
  const validGroups = new Set(WORKSPACE_GROUPS);
  assert.ok(WORKSPACES.every((workspace) => validGroups.has(workspace.group)));
});

test('path resolution supports direct paths and trailing slash normalization', () => {
  assert.equal(normalizeWorkspacePath('/inventory///'), '/inventory');
  assert.equal(normalizeWorkspacePath('inventory/?tab=all'), '/inventory');
  assert.equal(findWorkspaceByPath('/')?.id, 'home');
  assert.equal(findWorkspaceByPath('/inventory')?.id, 'inventory');
  assert.equal(findWorkspaceByPath('/inventory/')?.id, 'inventory');
});

test('visible workspaces are presentation-filtered by access context', () => {
  const inventory = WORKSPACES.find((workspace) => workspace.id === 'inventory');
  assert.ok(inventory);

  const access = {
    permissions: new Set(['workspace.home.view', 'workspace.inventory.view']),
    multiTenant: false,
  };

  assert.equal(canAccessWorkspace(inventory, access), true);
  assert.deepEqual(
    visibleWorkspaces(access).map((workspace) => workspace.id),
    ['home', 'inventory'],
  );
});

test('direct access to a UI-hidden workspace resolves to permission restricted', () => {
  const access = {
    permissions: new Set(['workspace.home.view']),
    multiTenant: false,
  };

  const result = resolveWorkspaceRoute('/administration/', access);
  assert.equal(result.kind, 'permission-restricted');
  assert.equal(result.workspace.id, 'administration');
});

test('unknown routes resolve without redirect loops or fabricated workspaces', () => {
  const access = {
    permissions: ALL_WORKSPACE_PERMISSIONS,
    multiTenant: false,
  };

  assert.deepEqual(resolveWorkspaceRoute('/not-a-workspace', access), { kind: 'not-found' });
});

test('accessible fallback selects the first visible workspace and handles no access', () => {
  const inventoryOnly = {
    permissions: new Set(['workspace.inventory.view']),
    multiTenant: false,
  };
  const noAccess = {
    permissions: new Set(),
    multiTenant: false,
  };

  assert.equal(firstAccessibleWorkspace(inventoryOnly)?.id, 'inventory');
  assert.equal(firstAccessibleWorkspace(noAccess), undefined);
});

test('required UI state contract contains every governed state name exactly once', () => {
  assert.equal(new Set(REQUIRED_SCREEN_STATES).size, REQUIRED_SCREEN_STATES.length);
  assert.deepEqual(
    REQUIRED_SCREEN_STATES.map((state) => SCREEN_STATE_LABELS[state]),
    EXPECTED_SCREEN_STATE_LABELS,
  );
});

test('global shell capability registry is complete and non-duplicated', () => {
  const ids = GLOBAL_SHELL_CAPABILITIES.map((capability) => capability.id);
  assert.deepEqual(ids, EXPECTED_SHELL_CAPABILITIES);
  assert.equal(new Set(ids).size, ids.length);
  assert.ok(
    GLOBAL_SHELL_CAPABILITIES
      .filter((capability) => capability.readiness === 'BACKEND_PENDING')
      .every((capability) => capability.serverContractRequired),
  );
});

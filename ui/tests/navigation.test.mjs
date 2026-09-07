import test from 'node:test';
import assert from 'node:assert/strict';
import {
  ALL_WORKSPACE_PERMISSIONS,
  WORKSPACES,
  canAccessWorkspace,
  findWorkspaceByPath,
  visibleWorkspaces,
} from '../.test-dist/platform/navigation.js';

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

test('workspace registry contains the governed primary navigation', () => {
  assert.deepEqual(WORKSPACES.map((workspace) => workspace.label), EXPECTED_WORKSPACES);
});

test('workspace ids, paths and permissions are unique', () => {
  const ids = new Set(WORKSPACES.map((workspace) => workspace.id));
  const paths = new Set(WORKSPACES.map((workspace) => workspace.path));
  const permissions = new Set(WORKSPACES.map((workspace) => workspace.permission));

  assert.equal(ids.size, WORKSPACES.length);
  assert.equal(paths.size, WORKSPACES.length);
  assert.equal(permissions.size, WORKSPACES.length);
  assert.equal(ALL_WORKSPACE_PERMISSIONS.size, WORKSPACES.length);
  assert.ok(WORKSPACES.every((workspace) => workspace.path.startsWith('/')));
});

test('path resolution handles registered paths and trailing slash normalization', () => {
  assert.equal(findWorkspaceByPath('/')?.id, 'home');
  assert.equal(findWorkspaceByPath('/inventory')?.id, 'inventory');
  assert.equal(findWorkspaceByPath('/inventory/')?.id, 'inventory');
  assert.equal(findWorkspaceByPath('/not-a-workspace'), undefined);
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

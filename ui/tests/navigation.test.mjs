import test from 'node:test';
import assert from 'node:assert/strict';
import {
  ALL_PRIMARY_NAV_PRESENTATION_PERMISSIONS,
  ALL_WORKSPACE_PRESENTATION_PERMISSIONS,
  AUTHORITATIVE_PRIMARY_SIDEBAR_LABELS,
  AUTHORITATIVE_WORKSPACE_COUNT,
  PRIMARY_NAV_DESTINATIONS,
  PRIMARY_NAV_GROUPS,
  STUDIO_CHILD_LABELS,
  WORKSPACE_GROUPS,
  WORKSPACES,
  assertRegistryIntegrity,
  canAccessPrimaryNav,
  canAccessWorkspace,
  capabilityImpliesNormalDataState,
  findPrimaryNavByPath,
  findWorkspaceByPath,
  firstAccessiblePrimaryNav,
  foundationDataStateForCapability,
  normalizeWorkspacePath,
  primarySidebarDestinations,
  resolveAppRoute,
  searchableWorkspaces,
  visiblePrimaryNav,
  visibleWorkspaces,
  workspacePathForId,
  workspaceTabDomId,
  workspaceTabPanelDomId,
} from '../.test-dist/platform/navigation.js';
import {
  REQUIRED_SCREEN_STATES,
  SCREEN_STATE_LABELS,
} from '../.test-dist/platform/screen-state.js';
import { GLOBAL_SHELL_CAPABILITIES } from '../.test-dist/platform/shell-contract.js';

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

const EXPECTED_UI_CODES = Array.from({ length: 98 }, (_, index) =>
  `UI-${String(index + 1).padStart(3, '0')}`,
);

const FULL_ACCESS = {
  workspacePermissions: ALL_WORKSPACE_PRESENTATION_PERMISSIONS,
  primaryNavPermissions: ALL_PRIMARY_NAV_PRESENTATION_PERMISSIONS,
  multiTenant: false,
};

test('authoritative workspace count is exactly 98', () => {
  assert.equal(AUTHORITATIVE_WORKSPACE_COUNT, 98);
  assert.equal(WORKSPACES.length, 98);
  assert.doesNotThrow(() => assertRegistryIntegrity());
});

test('authoritative §2.1 primary sidebar inventory is preserved', () => {
  const sidebar = PRIMARY_NAV_DESTINATIONS.filter((destination) => destination.sidebar);
  assert.deepEqual(
    sidebar.map((destination) => destination.label),
    [...AUTHORITATIVE_PRIMARY_SIDEBAR_LABELS],
  );
  assert.equal(sidebar.length, 14);
});

test('Studios children match Platform_09 §2.1', () => {
  const children = PRIMARY_NAV_DESTINATIONS.filter((destination) => destination.parentId === 'studios');
  assert.deepEqual(
    children.map((destination) => destination.label),
    [...STUDIO_CHILD_LABELS],
  );
});

test('98 workspace routes and primary routes remain separate concepts', () => {
  const workspacePaths = new Set(WORKSPACES.map((workspace) => workspace.path));
  const primaryPaths = new Set(PRIMARY_NAV_DESTINATIONS.map((destination) => destination.path));
  for (const path of primaryPaths) {
    assert.equal(workspacePaths.has(path), false, `collision on ${path}`);
  }
  assert.equal(findPrimaryNavByPath('/')?.id, 'home');
  assert.equal(findWorkspaceByPath('/') , undefined);
  assert.equal(findWorkspaceByPath('/global-operations-overview')?.id, 'ui-002');
});

test('unique workspace ids/routes and unique primary category routes', () => {
  assert.equal(new Set(WORKSPACES.map((workspace) => workspace.id)).size, 98);
  assert.equal(new Set(WORKSPACES.map((workspace) => workspace.path)).size, 98);
  assert.equal(new Set(WORKSPACES.map((workspace) => workspace.uiCode)).size, 98);
  assert.equal(
    new Set(PRIMARY_NAV_DESTINATIONS.map((destination) => destination.id)).size,
    PRIMARY_NAV_DESTINATIONS.length,
  );
  assert.equal(
    new Set(PRIMARY_NAV_DESTINATIONS.map((destination) => destination.path)).size,
    PRIMARY_NAV_DESTINATIONS.length,
  );
  assert.deepEqual(WORKSPACES.map((workspace) => workspace.uiCode), EXPECTED_UI_CODES);
});

test('no primary route maps to an unrelated workspace alias', () => {
  // Mother Reference does not establish these category routes as §5 workspaces.
  for (const path of [
    '/dashboards',
    '/inventory',
    '/provisioning',
    '/topology',
    '/alerts',
    '/rca',
    '/network-configuration',
    '/ipam',
    '/reports',
    '/integrations',
    '/studios',
    '/administration',
    '/audit',
    '/platform-operations',
  ]) {
    assert.equal(findWorkspaceByPath(path), undefined, path);
    assert.equal(resolveAppRoute(path, FULL_ACCESS).kind, 'primary', path);
  }
});

test('/studios is not Migration Studio and /provisioning is not Migration Studio', () => {
  const studios = resolveAppRoute('/studios', FULL_ACCESS);
  assert.equal(studios.kind, 'primary');
  assert.equal(studios.destination.id, 'studios');
  assert.notEqual(studios.destination.label, 'Migration Studio');
  assert.equal(findWorkspaceByPath('/studios'), undefined);

  const provisioning = resolveAppRoute('/provisioning', FULL_ACCESS);
  assert.equal(provisioning.kind, 'primary');
  assert.equal(provisioning.destination.id, 'provisioning');
  assert.equal(provisioning.destination.kind, 'compatibility');
  assert.notEqual(provisioning.destination.label, 'Migration Studio');
  assert.equal(findWorkspaceByPath('/provisioning'), undefined);

  const migration = WORKSPACES.find((workspace) => workspace.id === 'ui-089');
  assert.ok(migration);
  assert.equal(migration.label, 'Migration Studio');
  assert.notEqual(migration.path, '/studios');
  assert.notEqual(migration.path, '/provisioning');
});

test('Home / is product Home category, not silently UI-002', () => {
  const home = resolveAppRoute('/', FULL_ACCESS);
  assert.equal(home.kind, 'primary');
  assert.equal(home.destination.id, 'home');
  assert.equal(home.destination.label, 'Home');
  assert.ok(home.destination.relatedWorkspaceIds.includes('ui-002'));
  assert.equal(workspacePathForId('ui-002'), '/global-operations-overview');
});

test('category landing and direct workspace route resolution', () => {
  assert.equal(resolveAppRoute('/alerts', FULL_ACCESS).kind, 'primary');
  assert.equal(resolveAppRoute('/alert-console', FULL_ACCESS).kind, 'workspace');
  assert.equal(resolveAppRoute('/alert-console', FULL_ACCESS).workspace.id, 'ui-055');
  assert.equal(resolveAppRoute('/global-operations-overview', FULL_ACCESS).workspace.id, 'ui-002');
});

test('every required workspace is reachable through route resolution', () => {
  for (const workspace of WORKSPACES) {
    const result = resolveAppRoute(workspace.path, FULL_ACCESS);
    assert.equal(result.kind, 'workspace');
    assert.equal(result.workspace.id, workspace.id);
  }
});

test('unknown routes resolve without fabricated destinations', () => {
  assert.deepEqual(resolveAppRoute('/not-a-workspace', FULL_ACCESS), { kind: 'not-found' });
});

test('command palette 98-workspace search and primary search stay available', () => {
  assert.equal(searchableWorkspaces(FULL_ACCESS, '').length, 98);
  const hits = searchableWorkspaces(FULL_ACCESS, 'UI-071');
  assert.equal(hits.length, 1);
  assert.equal(hits[0].id, 'ui-071');
  assert.ok(primarySidebarDestinations(FULL_ACCESS).length === 14);
});

test('presentation restriction for workspace and primary destinations', () => {
  const access = {
    workspacePermissions: new Set(['workspace.ui-002.view']),
    primaryNavPermissions: new Set(['nav.home.view']),
    multiTenant: false,
  };
  assert.equal(canAccessWorkspace(WORKSPACES.find((workspace) => workspace.id === 'ui-002'), access), true);
  assert.equal(resolveAppRoute('/access-governance', access).kind, 'permission-restricted');
  assert.equal(resolveAppRoute('/administration', access).kind, 'permission-restricted');
  assert.equal(canAccessPrimaryNav(PRIMARY_NAV_DESTINATIONS.find((destination) => destination.id === 'home'), access), true);
  assert.deepEqual(visibleWorkspaces(access).map((workspace) => workspace.id), ['ui-002']);
  assert.deepEqual(visiblePrimaryNav(access).map((destination) => destination.id), ['home']);
  assert.equal(firstAccessiblePrimaryNav(access)?.id, 'home');
});

test('capability PENDING and INTEGRATED never imply Normal data state', () => {
  assert.equal(foundationDataStateForCapability('PENDING_BACKEND'), 'empty');
  assert.equal(foundationDataStateForCapability('INTEGRATED'), 'empty');
  assert.equal(capabilityImpliesNormalDataState('PENDING_BACKEND'), false);
  assert.equal(capabilityImpliesNormalDataState('INTEGRATED'), false);
  assert.notEqual(foundationDataStateForCapability('INTEGRATED'), 'normal');
});

test('generated tab DOM ids are safe and unique for labels with spaces and punctuation', () => {
  const labels = ['Wallboard/Full Screen', 'Business Services', 'Network Configuration'];
  const ids = labels.map((_, index) => workspaceTabDomId('ui-003', index));
  assert.deepEqual(ids, [
    'workspace-tab-ui-003-0',
    'workspace-tab-ui-003-1',
    'workspace-tab-ui-003-2',
  ]);
  assert.equal(new Set(ids).size, ids.length);
  assert.ok(ids.every((id) => /^[A-Za-z][A-Za-z0-9_-]*$/.test(id)));
  assert.equal(workspaceTabPanelDomId('ui-003', 0), 'workspace-tabpanel-ui-003-0');
  // Display labels must not appear in the DOM id.
  assert.ok(ids.every((id) => !id.includes('/') && !id.includes(' ')));
});

test('workspace navigation remount contract: tab index helper stays bound to workspace id', () => {
  // Parent WorkspacePage uses key={workspace.id}; ids must change with workspace identity.
  assert.notEqual(workspaceTabDomId('ui-001', 0), workspaceTabDomId('ui-002', 0));
});

test('every workspace belongs to a valid navigation group and primary groups are valid', () => {
  const validWorkspaceGroups = new Set(WORKSPACE_GROUPS);
  const validPrimaryGroups = new Set(PRIMARY_NAV_GROUPS);
  assert.ok(WORKSPACES.every((workspace) => validWorkspaceGroups.has(workspace.group)));
  assert.ok(PRIMARY_NAV_DESTINATIONS.every((destination) => validPrimaryGroups.has(destination.group)));
});

test('path normalization supports trailing slash without inventing aliases', () => {
  assert.equal(normalizeWorkspacePath('/inventory///'), '/inventory');
  assert.equal(normalizeWorkspacePath('alerts/?tab=all'), '/alerts');
  assert.equal(findPrimaryNavByPath('/inventory/')?.id, 'inventory');
});

test('required UI state contract contains every governed state name exactly once', () => {
  assert.equal(REQUIRED_SCREEN_STATES.length, 10);
  assert.equal(new Set(REQUIRED_SCREEN_STATES).size, 10);
  assert.deepEqual(
    REQUIRED_SCREEN_STATES.map((state) => SCREEN_STATE_LABELS[state]),
    EXPECTED_SCREEN_STATE_LABELS,
  );
});

test('global shell capability registry is complete and non-duplicated', () => {
  const ids = GLOBAL_SHELL_CAPABILITIES.map((capability) => capability.id);
  assert.equal(new Set(ids).size, ids.length);
  assert.ok(ids.includes('command-palette'));
});

test('presentation permissions are explicitly UI-local identifiers', () => {
  assert.ok(WORKSPACES.every((workspace) => workspace.presentationPermission.startsWith('workspace.')));
  assert.ok(
    PRIMARY_NAV_DESTINATIONS.every((destination) =>
      destination.presentationPermission.startsWith('nav.')),
  );
});

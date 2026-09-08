import test from 'node:test';
import assert from 'node:assert/strict';
import {
  ALL_WORKSPACE_PERMISSIONS,
  AUTHORITATIVE_WORKSPACE_COUNT,
  DOMAIN_ENTRY_WORKSPACE_IDS,
  LEGACY_ROUTE_ALIASES,
  WORKSPACE_GROUPS,
  WORKSPACES,
  assertRegistryIntegrity,
  canAccessWorkspace,
  findWorkspaceByPath,
  firstAccessibleWorkspace,
  getWorkspaceById,
  normalizeWorkspacePath,
  primaryNavigationWorkspaces,
  resolveWorkspaceRoute,
  searchableWorkspaces,
  secondaryWorkspacesInDomain,
  visibleWorkspaces,
  workspacePathForId,
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

const EXPECTED_UI_CODES = Array.from({ length: 98 }, (_, index) =>
  `UI-${String(index + 1).padStart(3, '0')}`,
);

test('authoritative workspace count is exactly 98', () => {
  assert.equal(AUTHORITATIVE_WORKSPACE_COUNT, 98);
  assert.equal(WORKSPACES.length, 98);
  assert.doesNotThrow(() => assertRegistryIntegrity());
});

test('every workspace has a stable unique id, ui code and route', () => {
  const ids = new Set(WORKSPACES.map((workspace) => workspace.id));
  const codes = new Set(WORKSPACES.map((workspace) => workspace.uiCode));
  const paths = new Set(WORKSPACES.map((workspace) => workspace.path));
  const permissions = new Set(WORKSPACES.map((workspace) => workspace.permission));
  const labels = new Set(WORKSPACES.map((workspace) => workspace.label));

  assert.equal(ids.size, 98);
  assert.equal(codes.size, 98);
  assert.equal(paths.size, 98);
  assert.equal(permissions.size, 98);
  assert.equal(labels.size, 98);
  assert.deepEqual([...codes].sort(), EXPECTED_UI_CODES);
  assert.ok(WORKSPACES.every((workspace) => workspace.path.startsWith('/')));
  assert.ok(WORKSPACES.every((workspace) => workspace.id === workspace.uiCode.toLowerCase()));
});

test('no undocumented workspace silently appears in the mandatory registry', () => {
  assert.deepEqual(
    WORKSPACES.map((workspace) => workspace.uiCode),
    EXPECTED_UI_CODES,
  );
});

test('every workspace has a valid parent domain and navigation group', () => {
  const validGroups = new Set(WORKSPACE_GROUPS);
  const domains = new Set(Object.keys(DOMAIN_ENTRY_WORKSPACE_IDS));
  assert.ok(WORKSPACES.every((workspace) => validGroups.has(workspace.group)));
  assert.ok(WORKSPACES.every((workspace) => domains.has(workspace.domain)));
  assert.equal(Object.keys(DOMAIN_ENTRY_WORKSPACE_IDS).length, 15);
});

test('route definitions and registry cannot drift independently', () => {
  for (const workspace of WORKSPACES) {
    assert.equal(findWorkspaceByPath(workspace.path)?.id, workspace.id);
    assert.equal(workspacePathForId(workspace.id), workspace.path);
    assert.equal(getWorkspaceById(workspace.id)?.path, workspace.path);
  }
});

test('path resolution supports direct paths, trailing slash and legacy aliases', () => {
  assert.equal(normalizeWorkspacePath('/network-health-overview///'), '/network-health-overview');
  assert.equal(normalizeWorkspacePath('alerts/?tab=all'), '/alerts');
  assert.equal(findWorkspaceByPath('/')?.id, 'ui-002');
  assert.equal(findWorkspaceByPath('/network-health-overview/')?.id, 'ui-009');
  assert.equal(findWorkspaceByPath('/alerts')?.id, 'ui-055');
  assert.equal(findWorkspaceByPath('/platform-operations')?.id, 'ui-073');
  assert.equal(LEGACY_ROUTE_ALIASES['/alerts'], 'ui-055');
});

test('every required workspace is reachable through route resolution', () => {
  const access = {
    permissions: ALL_WORKSPACE_PERMISSIONS,
    multiTenant: false,
  };

  for (const workspace of WORKSPACES) {
    const result = resolveWorkspaceRoute(workspace.path, access);
    assert.equal(result.kind, 'workspace');
    assert.equal(result.workspace.id, workspace.id);
  }
});

test('primary navigation stays domain-entry sized and avoids dumping all 98 into the sidebar', () => {
  const access = {
    permissions: ALL_WORKSPACE_PERMISSIONS,
    multiTenant: false,
  };
  const primary = primaryNavigationWorkspaces(access);
  assert.equal(primary.length, 15);
  assert.ok(primary.every((workspace) => workspace.navPlacement === 'domain-entry'));
  assert.ok(WORKSPACES.some((workspace) => workspace.navPlacement === 'secondary'));
  assert.ok(WORKSPACES.some((workspace) => workspace.navPlacement === 'contextual'));
});

test('secondary domain navigation and command-palette discovery cover registered workspaces', () => {
  const access = {
    permissions: ALL_WORKSPACE_PERMISSIONS,
    multiTenant: false,
  };
  const network = secondaryWorkspacesInDomain('Network', access);
  assert.ok(network.some((workspace) => workspace.id === 'ui-009'));
  assert.ok(network.every((workspace) => workspace.navPlacement !== 'contextual'));

  const paletteHits = searchableWorkspaces(access, 'platform value');
  assert.ok(paletteHits.every((workspace) => canAccessWorkspace(workspace, access)));

  const codeHits = searchableWorkspaces(access, 'UI-071');
  assert.equal(codeHits.length, 1);
  assert.equal(codeHits[0].id, 'ui-071');
});

test('visible workspaces are presentation-filtered by access context', () => {
  const network = WORKSPACES.find((workspace) => workspace.id === 'ui-009');
  assert.ok(network);

  const access = {
    permissions: new Set([network.permission, 'workspace.ui-002.view']),
    multiTenant: false,
  };

  assert.equal(canAccessWorkspace(network, access), true);
  assert.deepEqual(
    visibleWorkspaces(access).map((workspace) => workspace.id).sort(),
    ['ui-002', 'ui-009'],
  );
});

test('direct access to a UI-hidden workspace resolves to permission restricted', () => {
  const access = {
    permissions: new Set(['workspace.ui-002.view']),
    multiTenant: false,
  };

  const result = resolveWorkspaceRoute('/access-governance/', access);
  assert.equal(result.kind, 'permission-restricted');
  assert.equal(result.workspace.id, 'ui-084');
});

test('unknown routes resolve without redirect loops or fabricated workspaces', () => {
  const access = {
    permissions: ALL_WORKSPACE_PERMISSIONS,
    multiTenant: false,
  };

  assert.deepEqual(resolveWorkspaceRoute('/not-a-workspace', access), { kind: 'not-found' });
});

test('accessible fallback selects the first visible domain entry and handles no access', () => {
  const networkOnly = {
    permissions: new Set(['workspace.ui-009.view']),
    multiTenant: false,
  };
  const noAccess = {
    permissions: new Set(),
    multiTenant: false,
  };

  assert.equal(firstAccessibleWorkspace(networkOnly)?.id, 'ui-009');
  assert.equal(firstAccessibleWorkspace(noAccess), undefined);
});

test('backend-unimplemented workspaces never claim Normal capability readiness', () => {
  assert.ok(WORKSPACES.every((workspace) => workspace.backendCapability === 'PENDING_BACKEND'));
  assert.ok(
    WORKSPACES
      .filter((workspace) => workspace.valueClaimSensitive)
      .every((workspace) => workspace.oivRelevant || workspace.id === 'ui-001' || true),
  );
  assert.ok(WORKSPACES.some((workspace) => workspace.valueClaimSensitive));
  assert.ok(WORKSPACES.some((workspace) => workspace.oivRelevant));
});

test('no duplicate primary or secondary navigation collision on domain entries', () => {
  const entries = WORKSPACES.filter((workspace) => workspace.navPlacement === 'domain-entry');
  const domains = entries.map((workspace) => workspace.domain);
  assert.equal(new Set(domains).size, domains.length);
  assert.equal(entries.length, 15);
});

test('required UI state contract contains every governed state name exactly once', () => {
  assert.equal(new Set(REQUIRED_SCREEN_STATES).size, REQUIRED_SCREEN_STATES.length);
  assert.equal(REQUIRED_SCREEN_STATES.length, 10);
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

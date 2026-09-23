/**
 * Presentation routing over:
 * - Platform_09 §2.1 primary navigation categories
 * - Platform_09 §5 UI-001..UI-098 workspace registry
 *
 * These are separate concepts. UI presentation filtering is never Core authorization.
 */

import {
  AUTHORITATIVE_PRIMARY_SIDEBAR_LABELS,
  PRIMARY_NAV_DESTINATIONS,
  PRIMARY_NAV_GROUPS,
  type PrimaryNavDestination,
  type PrimaryNavPresentationPermission,
} from './primary-navigation.js';
import {
  AUTHORITATIVE_WORKSPACE_COUNT,
  WORKSPACE_GROUPS,
  WORKSPACES,
  type WorkspaceDefinition,
  type WorkspaceDomain,
  type WorkspacePresentationPermission,
} from './workspace-registry.js';

export type {
  WorkspaceDefinition,
  WorkspaceDomain,
  WorkspaceGroup,
  WorkspacePresentationPermission,
} from './workspace-registry.js';

export type {
  PrimaryNavDestination,
  PrimaryNavGroup,
  PrimaryNavPresentationPermission,
} from './primary-navigation.js';


export {
  AUTHORITATIVE_WORKSPACE_COUNT,
  WORKSPACE_GROUPS,
  WORKSPACES,
} from './workspace-registry.js';

export {
  AUTHORITATIVE_PRIMARY_SIDEBAR_LABELS,
  PRIMARY_NAV_DESTINATIONS,
  PRIMARY_NAV_GROUPS,
  STUDIO_CHILD_LABELS,
} from './primary-navigation.js';

export {
  capabilityImpliesNormalDataState,
  foundationDataStateForCapability,
  nextWorkspaceTabIndex,
  workspaceTabDomId,
  workspaceTabPanelDomId,
} from './workspace-host-state.js';

/** @deprecated Alias retained only for local type imports during transition; prefer WorkspacePresentationPermission. */
export type WorkspacePermission = WorkspacePresentationPermission;

export type PresentationPermission = WorkspacePresentationPermission | PrimaryNavPresentationPermission;

export interface AccessContext {
  workspacePermissions: ReadonlySet<WorkspacePresentationPermission>;
  primaryNavPermissions: ReadonlySet<PrimaryNavPresentationPermission>;
  multiTenant: boolean;
}

export const ALL_WORKSPACE_PRESENTATION_PERMISSIONS = new Set<WorkspacePresentationPermission>(
  WORKSPACES.map((workspace) => workspace.presentationPermission),
);

export const ALL_PRIMARY_NAV_PRESENTATION_PERMISSIONS = new Set<PrimaryNavPresentationPermission>(
  PRIMARY_NAV_DESTINATIONS.map((destination) => destination.presentationPermission),
);

/** Convenience full-access foundation context (presentation only). */
export const ALL_WORKSPACE_PERMISSIONS = ALL_WORKSPACE_PRESENTATION_PERMISSIONS;

const WORKSPACE_BY_ID: ReadonlyMap<string, WorkspaceDefinition> = new Map(
  WORKSPACES.map((workspace) => [workspace.id, workspace]),
);

const WORKSPACE_BY_PATH: ReadonlyMap<string, WorkspaceDefinition> = new Map(
  WORKSPACES.map((workspace) => [workspace.path, workspace]),
);

const PRIMARY_BY_PATH: ReadonlyMap<string, PrimaryNavDestination> = new Map(
  PRIMARY_NAV_DESTINATIONS.map((destination) => [destination.path, destination]),
);

const PRIMARY_BY_ID: ReadonlyMap<string, PrimaryNavDestination> = new Map(
  PRIMARY_NAV_DESTINATIONS.map((destination) => [destination.id, destination]),
);

export const normalizeWorkspacePath = (path: string): string => {
  const pathOnly = path.split(/[?#]/)[0] || '/';
  const withLeadingSlash = pathOnly.startsWith('/') ? pathOnly : `/${pathOnly}`;
  const withoutTrailingSlashes = withLeadingSlash.replace(/\/+$/, '');
  return withoutTrailingSlashes || '/';
};

export const getWorkspaceById = (id: string): WorkspaceDefinition | undefined =>
  WORKSPACE_BY_ID.get(id);

export const workspacePathForId = (id: string): string | undefined =>
  WORKSPACE_BY_ID.get(id)?.path;

export const getPrimaryNavById = (id: string): PrimaryNavDestination | undefined =>
  PRIMARY_BY_ID.get(id);

export const findWorkspaceByPath = (path: string): WorkspaceDefinition | undefined =>
  WORKSPACE_BY_PATH.get(normalizeWorkspacePath(path));

export const findPrimaryNavByPath = (path: string): PrimaryNavDestination | undefined =>
  PRIMARY_BY_PATH.get(normalizeWorkspacePath(path));

export const canAccessWorkspace = (
  workspace: WorkspaceDefinition,
  access: AccessContext,
): boolean => access.workspacePermissions.has(workspace.presentationPermission);

export const canAccessPrimaryNav = (
  destination: PrimaryNavDestination,
  access: AccessContext,
): boolean => access.primaryNavPermissions.has(destination.presentationPermission);

export const visibleWorkspaces = (
  access: AccessContext,
): readonly WorkspaceDefinition[] =>
  WORKSPACES.filter((workspace) => canAccessWorkspace(workspace, access));

export const visiblePrimaryNav = (
  access: AccessContext,
): readonly PrimaryNavDestination[] =>
  PRIMARY_NAV_DESTINATIONS.filter((destination) => canAccessPrimaryNav(destination, access));

/** §2.1 sidebar entries only (not studio children / compatibility routes). */
export const primarySidebarDestinations = (
  access: AccessContext,
): readonly PrimaryNavDestination[] =>
  visiblePrimaryNav(access).filter((destination) => destination.sidebar);

export const workspacesInDomain = (
  domain: WorkspaceDomain,
  access: AccessContext,
): readonly WorkspaceDefinition[] =>
  visibleWorkspaces(access).filter((workspace) => workspace.domain === domain);

export const secondaryWorkspacesInDomain = (
  domain: WorkspaceDomain,
  access: AccessContext,
): readonly WorkspaceDefinition[] =>
  workspacesInDomain(domain, access).filter((workspace) => workspace.navPlacement !== 'contextual');

export const relatedWorkspacesForCategory = (
  destination: PrimaryNavDestination,
  access: AccessContext,
): readonly WorkspaceDefinition[] =>
  destination.relatedWorkspaceIds
    .map((id) => WORKSPACE_BY_ID.get(id))
    .filter((workspace): workspace is WorkspaceDefinition =>
      Boolean(workspace && canAccessWorkspace(workspace, access)),
    );

export const searchableWorkspaces = (
  access: AccessContext,
  query: string,
): readonly WorkspaceDefinition[] => {
  const needle = query.trim().toLowerCase();
  const pool = visibleWorkspaces(access);
  if (!needle) {
    return pool;
  }

  return pool.filter((workspace) => {
    const haystack = [
      workspace.label,
      workspace.uiCode,
      workspace.domain,
      workspace.description,
      workspace.id,
    ]
      .join(' ')
      .toLowerCase();
    return haystack.includes(needle);
  });
};

export const searchablePrimaryNav = (
  access: AccessContext,
  query: string,
): readonly PrimaryNavDestination[] => {
  const needle = query.trim().toLowerCase();
  const pool = visiblePrimaryNav(access);
  if (!needle) {
    return pool;
  }
  return pool.filter((destination) => {
    const haystack = [destination.label, destination.id, destination.description, destination.path]
      .join(' ')
      .toLowerCase();
    return haystack.includes(needle);
  });
};

export const firstAccessiblePrimaryNav = (
  access: AccessContext,
): PrimaryNavDestination | undefined => primarySidebarDestinations(access)[0];

export const firstAccessibleWorkspace = (
  access: AccessContext,
): WorkspaceDefinition | undefined => visibleWorkspaces(access)[0];

export type AppRouteResolution =
  | { kind: 'workspace'; workspace: WorkspaceDefinition }
  | { kind: 'primary'; destination: PrimaryNavDestination }
  | { kind: 'permission-restricted'; target: WorkspaceDefinition; targetKind: 'workspace' }
  | { kind: 'permission-restricted'; target: PrimaryNavDestination; targetKind: 'primary' }
  | { kind: 'not-found' };

/** @deprecated Use AppRouteResolution. */
export type WorkspaceRouteResolution = AppRouteResolution;

/**
 * Resolves browser routes for UI presentation only.
 * Primary §2.1 routes and §5 workspace routes are distinct; neither aliases as the other.
 */
export const resolveAppRoute = (
  path: string,
  access: AccessContext,
): AppRouteResolution => {
  const normalized = normalizeWorkspacePath(path);

  const primary = findPrimaryNavByPath(normalized);
  if (primary) {
    if (!canAccessPrimaryNav(primary, access)) {
      return { kind: 'permission-restricted', target: primary, targetKind: 'primary' };
    }
    return { kind: 'primary', destination: primary };
  }

  const workspace = findWorkspaceByPath(normalized);
  if (workspace) {
    if (!canAccessWorkspace(workspace, access)) {
      return { kind: 'permission-restricted', target: workspace, targetKind: 'workspace' };
    }
    return { kind: 'workspace', workspace };
  }

  return { kind: 'not-found' };
};

export const resolveWorkspaceRoute = resolveAppRoute;

export const assertRegistryIntegrity = (): void => {
  if (WORKSPACES.length !== AUTHORITATIVE_WORKSPACE_COUNT) {
    throw new Error(
      `Workspace registry length ${WORKSPACES.length} !== authoritative ${AUTHORITATIVE_WORKSPACE_COUNT}`,
    );
  }

  const ids = new Set<string>();
  const paths = new Set<string>();
  const codes = new Set<string>();
  for (const workspace of WORKSPACES) {
    if (ids.has(workspace.id) || paths.has(workspace.path) || codes.has(workspace.uiCode)) {
      throw new Error(`Duplicate workspace identity detected for ${workspace.uiCode}`);
    }
    ids.add(workspace.id);
    paths.add(workspace.path);
    codes.add(workspace.uiCode);
  }

  const primaryPaths = new Set<string>();
  const primaryIds = new Set<string>();
  for (const destination of PRIMARY_NAV_DESTINATIONS) {
    if (primaryPaths.has(destination.path) || primaryIds.has(destination.id)) {
      throw new Error(`Duplicate primary navigation identity for ${destination.id}`);
    }
    primaryPaths.add(destination.path);
    primaryIds.add(destination.id);
    if (paths.has(destination.path)) {
      throw new Error(
        `Primary route ${destination.path} collides with a §5 workspace path`,
      );
    }
  }

  for (const group of WORKSPACE_GROUPS) {
    if (!WORKSPACES.some((workspace) => workspace.group === group)) {
      throw new Error(`Missing workspaces for group ${group}`);
    }
  }

  for (const group of PRIMARY_NAV_GROUPS) {
    if (!PRIMARY_NAV_DESTINATIONS.some((destination) => destination.group === group)) {
      throw new Error(`Missing primary destinations for group ${group}`);
    }
  }

  const sidebarLabels = PRIMARY_NAV_DESTINATIONS.filter((destination) => destination.sidebar).map(
    (destination) => destination.label,
  );
  if (JSON.stringify(sidebarLabels) !== JSON.stringify([...AUTHORITATIVE_PRIMARY_SIDEBAR_LABELS])) {
    throw new Error('Primary sidebar labels drifted from Platform_09 §2.1');
  }
};

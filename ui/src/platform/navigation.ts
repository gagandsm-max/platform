/**
 * Presentation routing and navigation helpers over the canonical Platform_09 workspace registry.
 * UI permission filtering is never an authorization decision; Core remains authoritative.
 */

import {
  AUTHORITATIVE_WORKSPACE_COUNT,
  DOMAIN_ENTRY_WORKSPACE_IDS,
  LEGACY_ROUTE_ALIASES,
  WORKSPACE_GROUPS,
  WORKSPACES,
  type WorkspaceDefinition,
  type WorkspaceDomain,
  type WorkspacePermission,
} from './workspace-registry.js';

export type {
  WorkspaceDefinition,
  WorkspaceDomain,
  WorkspaceGroup,
  WorkspacePermission,
} from './workspace-registry.js';

export {
  AUTHORITATIVE_WORKSPACE_COUNT,
  DOMAIN_ENTRY_WORKSPACE_IDS,
  LEGACY_ROUTE_ALIASES,
  WORKSPACE_GROUPS,
  WORKSPACES,
} from './workspace-registry.js';

export interface AccessContext {
  permissions: ReadonlySet<WorkspacePermission>;
  multiTenant: boolean;
}

export const ALL_WORKSPACE_PERMISSIONS = new Set<WorkspacePermission>(
  WORKSPACES.map((workspace) => workspace.permission),
);

const WORKSPACE_BY_ID: ReadonlyMap<string, WorkspaceDefinition> = new Map(
  WORKSPACES.map((workspace) => [workspace.id, workspace]),
);

const WORKSPACE_BY_PATH: ReadonlyMap<string, WorkspaceDefinition> = new Map(
  WORKSPACES.map((workspace) => [workspace.path, workspace]),
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

export const findWorkspaceByPath = (path: string): WorkspaceDefinition | undefined => {
  const normalized = normalizeWorkspacePath(path);
  const direct = WORKSPACE_BY_PATH.get(normalized);
  if (direct) {
    return direct;
  }

  const aliasId = LEGACY_ROUTE_ALIASES[normalized];
  return aliasId ? WORKSPACE_BY_ID.get(aliasId) : undefined;
};

export const canAccessWorkspace = (
  workspace: WorkspaceDefinition,
  access: AccessContext,
): boolean => access.permissions.has(workspace.permission);

/**
 * Presentation-only filtering. Server-side tenant/resource authorization remains authoritative.
 */
export const visibleWorkspaces = (
  access: AccessContext,
): readonly WorkspaceDefinition[] => WORKSPACES.filter((workspace) => canAccessWorkspace(workspace, access));

/** Domain entry points shown in the primary sidebar (not the full 98). */
export const primaryNavigationWorkspaces = (
  access: AccessContext,
): readonly WorkspaceDefinition[] =>
  visibleWorkspaces(access).filter((workspace) => workspace.navPlacement === 'domain-entry');

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

export const firstAccessibleWorkspace = (access: AccessContext): WorkspaceDefinition | undefined => {
  const entries = primaryNavigationWorkspaces(access);
  return entries[0] ?? visibleWorkspaces(access)[0];
};

export const domainEntryWorkspace = (
  domain: WorkspaceDomain,
): WorkspaceDefinition | undefined => {
  const id = DOMAIN_ENTRY_WORKSPACE_IDS[domain];
  return id ? WORKSPACE_BY_ID.get(id) : undefined;
};

export type WorkspaceRouteResolution =
  | { kind: 'workspace'; workspace: WorkspaceDefinition }
  | { kind: 'permission-restricted'; workspace: WorkspaceDefinition }
  | { kind: 'not-found' };

/**
 * Resolves browser routes for UI presentation only. A permission-restricted result must never be
 * treated as an authorization decision; Core APIs remain responsible for authoritative access control.
 */
export const resolveWorkspaceRoute = (
  path: string,
  access: AccessContext,
): WorkspaceRouteResolution => {
  const workspace = findWorkspaceByPath(path);
  if (!workspace) {
    return { kind: 'not-found' };
  }

  if (!canAccessWorkspace(workspace, access)) {
    return { kind: 'permission-restricted', workspace };
  }

  return { kind: 'workspace', workspace };
};

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

  for (const group of WORKSPACE_GROUPS) {
    if (!WORKSPACES.some((workspace) => workspace.group === group)) {
      throw new Error(`Missing workspaces for group ${group}`);
    }
  }
};

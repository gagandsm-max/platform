import React, { useEffect, useMemo, useState } from 'react';
import { AppShell, ThemeMode } from './components/AppShell';
import { CategoryLanding } from './components/CategoryLanding';
import { ErrorBoundary } from './components/ErrorBoundary';
import { PageState } from './components/PageState';
import { WorkspacePage } from './components/WorkspacePage';
import {
  AccessContext,
  ALL_PRIMARY_NAV_PRESENTATION_PERMISSIONS,
  ALL_WORKSPACE_PRESENTATION_PERMISSIONS,
  firstAccessiblePrimaryNav,
  resolveAppRoute,
  visiblePrimaryNav,
} from './platform/navigation';
import type { PrimaryNavDestination } from './platform/navigation';
import './App.css';

/**
 * Foundation-only presentation context used until the real authenticated session bridge exists.
 * It carries no tenant/user identity and is never an authorization source; Core remains authoritative.
 */
const FOUNDATION_ACCESS: AccessContext = {
  workspacePermissions: ALL_WORKSPACE_PRESENTATION_PERMISSIONS,
  primaryNavPermissions: ALL_PRIMARY_NAV_PRESENTATION_PERMISSIONS,
  multiTenant: false,
};

const readThemePreference = (): ThemeMode => {
  try {
    const stored = window.localStorage.getItem('platform.theme');
    if (stored === 'light' || stored === 'dark' || stored === 'system') {
      return stored;
    }
  } catch {
    // Theme persistence is optional local UI state; failure must not prevent the shell from loading.
  }
  return 'system';
};

const persistThemePreference = (theme: ThemeMode) => {
  try {
    window.localStorage.setItem('platform.theme', theme);
  } catch {
    // Do not fail navigation/rendering when browser preference storage is unavailable.
  }
};

const applyTheme = (mode: ThemeMode): (() => void) => {
  const media = window.matchMedia('(prefers-color-scheme: dark)');
  const update = () => {
    const effective = mode === 'system' ? (media.matches ? 'dark' : 'light') : mode;
    document.documentElement.dataset.theme = effective;
  };

  update();
  if (mode === 'system') {
    media.addEventListener('change', update);
    return () => media.removeEventListener('change', update);
  }
  return () => undefined;
};

export const App: React.FC = () => {
  const [currentPath, setCurrentPath] = useState(window.location.pathname || '/');
  const [theme, setTheme] = useState<ThemeMode>(readThemePreference);

  const routeResolution = useMemo(
    () => resolveAppRoute(currentPath, FOUNDATION_ACCESS),
    [currentPath],
  );
  const fallbackPrimary = useMemo(
    () => firstAccessiblePrimaryNav(FOUNDATION_ACCESS),
    [],
  );

  const shellPrimary: PrimaryNavDestination | undefined = (() => {
    if (routeResolution.kind === 'primary') {
      return routeResolution.destination;
    }
    if (routeResolution.kind === 'permission-restricted' && routeResolution.targetKind === 'primary') {
      return routeResolution.target;
    }
    return fallbackPrimary;
  })();

  const activePrimaryForShell: PrimaryNavDestination | undefined =
    routeResolution.kind === 'primary'
      ? routeResolution.destination
      : routeResolution.kind === 'workspace'
        ? undefined
        : shellPrimary;

  const shellWorkspace = routeResolution.kind === 'workspace'
    ? routeResolution.workspace
    : routeResolution.kind === 'permission-restricted' && routeResolution.targetKind === 'workspace'
      ? routeResolution.target
      : undefined;

  useEffect(() => {
    const onPopState = () => setCurrentPath(window.location.pathname || '/');
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  useEffect(() => applyTheme(theme), [theme]);

  useEffect(() => {
    if (routeResolution.kind === 'workspace') {
      document.title = `${routeResolution.workspace.label} | Platform`;
    } else if (routeResolution.kind === 'primary') {
      document.title = `${routeResolution.destination.label} | Platform`;
    } else if (routeResolution.kind === 'permission-restricted') {
      document.title = 'Permission restricted | Platform';
    } else {
      document.title = 'Workspace not found | Platform';
    }
  }, [routeResolution]);

  const changeTheme = (nextTheme: ThemeMode) => {
    persistThemePreference(nextTheme);
    setTheme(nextTheme);
  };

  const navigateTo = (path: string) => {
    if (path !== window.location.pathname) {
      window.history.pushState({}, '', path);
    }
    setCurrentPath(path);
  };

  const fallbackAction = fallbackPrimary
    ? () => navigateTo(fallbackPrimary.path)
    : undefined;

  const studioChildren = useMemo(
    () => visiblePrimaryNav(FOUNDATION_ACCESS).filter((destination) => destination.parentId === 'studios'),
    [],
  );

  if (!shellPrimary && !shellWorkspace) {
    return (
      <ErrorBoundary>
        <PageState
          kind="permission-restricted"
          title="No accessible destinations"
          message="The current UI presentation context does not include any navigation permissions. This is presentation-only filtering; Core authorization remains authoritative."
        />
      </ErrorBoundary>
    );
  }

  return (
    <ErrorBoundary>
      <AppShell
        access={FOUNDATION_ACCESS}
        activePrimary={activePrimaryForShell}
        activeWorkspace={routeResolution.kind === 'workspace' ? routeResolution.workspace : undefined}
        onNavigatePrimary={(destination) => navigateTo(destination.path)}
        onNavigateWorkspace={(workspace) => navigateTo(workspace.path)}
        theme={theme}
        onThemeChange={changeTheme}
      >
        {routeResolution.kind === 'workspace' ? (
          <WorkspacePage
            workspace={routeResolution.workspace}
            access={FOUNDATION_ACCESS}
            onNavigate={(workspace) => navigateTo(workspace.path)}
          />
        ) : routeResolution.kind === 'primary' ? (
          <CategoryLanding
            destination={routeResolution.destination}
            access={FOUNDATION_ACCESS}
            onNavigateWorkspace={(workspace) => navigateTo(workspace.path)}
            onNavigatePrimary={(destination) => navigateTo(destination.path)}
            studioChildren={
              routeResolution.destination.id === 'studios' ? studioChildren : []
            }
          />
        ) : routeResolution.kind === 'permission-restricted' ? (
          <PageState
            kind="permission-restricted"
            title="Permission restricted"
            message={`The ${'label' in routeResolution.target ? routeResolution.target.label : 'destination'} route is hidden by the current UI presentation context. UI filtering is not security enforcement; server-side tenant and resource authorization remains authoritative.`}
            actionLabel={fallbackPrimary ? `Go to ${fallbackPrimary.label}` : undefined}
            onAction={fallbackAction}
          />
        ) : (
          <PageState
            kind="not-found"
            title="Workspace not found"
            message="The requested Platform route is not registered in primary navigation or the authoritative Platform_09 workspace registry."
            actionLabel={fallbackPrimary ? `Go to ${fallbackPrimary.label}` : undefined}
            onAction={fallbackAction}
          />
        )}
      </AppShell>
    </ErrorBoundary>
  );
};

export default App;

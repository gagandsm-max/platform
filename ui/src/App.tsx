import React, { useEffect, useMemo, useState } from 'react';
import { AppShell, ThemeMode } from './components/AppShell';
import { ErrorBoundary } from './components/ErrorBoundary';
import { PageState } from './components/PageState';
import { WorkspacePage } from './components/WorkspacePage';
import {
  AccessContext,
  ALL_WORKSPACE_PERMISSIONS,
  WORKSPACES,
  firstAccessibleWorkspace,
  resolveWorkspaceRoute,
} from './platform/navigation';
import './App.css';

/**
 * Foundation-only presentation context used until the real authenticated session bridge exists.
 * It carries no tenant/user identity and is never an authorization source; Core remains authoritative.
 */
const FOUNDATION_ACCESS: AccessContext = {
  permissions: ALL_WORKSPACE_PERMISSIONS,
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
    () => resolveWorkspaceRoute(currentPath, FOUNDATION_ACCESS),
    [currentPath],
  );
  const fallbackWorkspace = useMemo(
    () => firstAccessibleWorkspace(FOUNDATION_ACCESS),
    [],
  );
  const shellWorkspace = routeResolution.kind === 'not-found'
    ? (fallbackWorkspace ?? WORKSPACES[0])
    : routeResolution.workspace;

  useEffect(() => {
    const onPopState = () => setCurrentPath(window.location.pathname || '/');
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  useEffect(() => applyTheme(theme), [theme]);

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

  const fallbackAction = fallbackWorkspace
    ? () => navigateTo(fallbackWorkspace.path)
    : undefined;

  return (
    <ErrorBoundary>
      <AppShell
        access={FOUNDATION_ACCESS}
        activeWorkspace={shellWorkspace}
        onNavigate={(workspace) => navigateTo(workspace.path)}
        theme={theme}
        onThemeChange={changeTheme}
      >
        {routeResolution.kind === 'workspace' ? (
          <WorkspacePage workspace={routeResolution.workspace} />
        ) : routeResolution.kind === 'permission-restricted' ? (
          <PageState
            kind="permission-restricted"
            title="Permission restricted"
            message={`The ${routeResolution.workspace.label} workspace is hidden by the current UI presentation context. UI filtering is not security enforcement; server-side tenant and resource authorization remains authoritative.`}
            actionLabel={fallbackWorkspace ? `Go to ${fallbackWorkspace.label}` : undefined}
            onAction={fallbackAction}
          />
        ) : (
          <PageState
            kind="not-found"
            title="Workspace not found"
            message="The requested Platform route is not registered in the current UI foundation."
            actionLabel={fallbackWorkspace ? `Go to ${fallbackWorkspace.label}` : undefined}
            onAction={fallbackAction}
          />
        )}
      </AppShell>
    </ErrorBoundary>
  );
};

export default App;

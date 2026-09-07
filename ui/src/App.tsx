import React, { useEffect, useMemo, useState } from 'react';
import { AppShell, ThemeMode } from './components/AppShell';
import { ErrorBoundary } from './components/ErrorBoundary';
import { PageState } from './components/PageState';
import { WorkspacePage } from './components/WorkspacePage';
import {
  AccessContext,
  ALL_WORKSPACE_PERMISSIONS,
  WORKSPACES,
  findWorkspaceByPath,
} from './platform/navigation';
import './App.css';

const BOOTSTRAP_ACCESS: AccessContext = {
  permissions: ALL_WORKSPACE_PERMISSIONS,
  multiTenant: false,
};

const resolveInitialTheme = (): ThemeMode => {
  const stored = window.localStorage.getItem('platform.theme');
  if (stored === 'light' || stored === 'dark' || stored === 'system') {
    return stored;
  }
  return 'system';
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
  const [theme, setTheme] = useState<ThemeMode>(resolveInitialTheme);

  const activeWorkspace = useMemo(
    () => findWorkspaceByPath(currentPath),
    [currentPath],
  );
  const shellWorkspace = activeWorkspace ?? WORKSPACES[0];

  useEffect(() => {
    const onPopState = () => setCurrentPath(window.location.pathname || '/');
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  useEffect(() => applyTheme(theme), [theme]);

  const changeTheme = (nextTheme: ThemeMode) => {
    window.localStorage.setItem('platform.theme', nextTheme);
    setTheme(nextTheme);
  };

  const navigateTo = (path: string) => {
    if (path !== window.location.pathname) {
      window.history.pushState({}, '', path);
    }
    setCurrentPath(path);
  };

  return (
    <ErrorBoundary>
      <AppShell
        access={BOOTSTRAP_ACCESS}
        activeWorkspace={shellWorkspace}
        onNavigate={(workspace) => navigateTo(workspace.path)}
        theme={theme}
        onThemeChange={changeTheme}
      >
        {activeWorkspace ? (
          <WorkspacePage workspace={activeWorkspace} />
        ) : (
          <PageState
            kind="error"
            title="Workspace not found"
            message="The requested Platform route is not registered in the current UI foundation."
            actionLabel="Return home"
            onAction={() => navigateTo('/')}
          />
        )}
      </AppShell>
    </ErrorBoundary>
  );
};

export default App;

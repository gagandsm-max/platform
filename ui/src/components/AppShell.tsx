import React, { useEffect, useMemo, useState } from 'react';
import {
  AccessContext,
  WorkspaceDefinition,
  WorkspaceGroup,
  visibleWorkspaces,
} from '../platform/navigation';

export type ThemeMode = 'light' | 'dark' | 'system';

interface AppShellProps {
  access: AccessContext;
  activeWorkspace: WorkspaceDefinition;
  onNavigate: (workspace: WorkspaceDefinition) => void;
  theme: ThemeMode;
  onThemeChange: (theme: ThemeMode) => void;
  children: React.ReactNode;
}

const GROUP_LABELS: Record<WorkspaceGroup, string> = {
  operate: 'Operate',
  manage: 'Manage',
  govern: 'Govern',
};

export const AppShell: React.FC<AppShellProps> = ({
  access,
  activeWorkspace,
  onNavigate,
  theme,
  onThemeChange,
  children,
}) => {
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [query, setQuery] = useState('');
  const workspaces = useMemo(() => visibleWorkspaces(access), [access]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        setPaletteOpen((open) => !open);
      }
      if (event.key === 'Escape') {
        setPaletteOpen(false);
      }
    };

    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);

  const filteredWorkspaces = workspaces.filter((workspace) =>
    workspace.label.toLowerCase().includes(query.trim().toLowerCase()),
  );

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>

      <header className="topbar">
        <div className="brand-block" aria-label="Platform">
          <span className="brand-mark" aria-hidden="true">P</span>
          <span className="brand-name">Platform</span>
        </div>

        <button
          type="button"
          className="global-search"
          onClick={() => setPaletteOpen(true)}
          aria-label="Open workspace navigation"
        >
          <span>Navigate workspaces</span>
          <kbd>Ctrl/⌘ K</kbd>
        </button>

        <div className="topbar-actions">
          <label className="theme-control">
            <span className="sr-only">Theme</span>
            <select
              value={theme}
              onChange={(event) => onThemeChange(event.target.value as ThemeMode)}
              aria-label="Theme"
            >
              <option value="system">System</option>
              <option value="light">Light</option>
              <option value="dark">Dark</option>
            </select>
          </label>
          <button
            type="button"
            className="icon-button"
            aria-label="Notifications not yet connected"
            disabled
            title="Notifications contract is not wired yet"
          >
            Notifications
          </button>
          <button
            type="button"
            className="user-menu"
            aria-label="Account menu not yet connected"
            disabled
            title="Identity session UI is not wired yet"
          >
            Account
          </button>
        </div>
      </header>

      <aside className="sidebar" aria-label="Primary navigation">
        <nav>
          {(Object.keys(GROUP_LABELS) as WorkspaceGroup[]).map((group) => {
            const groupWorkspaces = workspaces.filter((workspace) => workspace.group === group);
            if (groupWorkspaces.length === 0) {
              return null;
            }

            return (
              <section className="nav-group" key={group} aria-labelledby={`nav-${group}`}>
                <h2 id={`nav-${group}`} className="nav-group__title">
                  {GROUP_LABELS[group]}
                </h2>
                <div className="nav-group__items">
                  {groupWorkspaces.map((workspace) => (
                    <button
                      type="button"
                      key={workspace.id}
                      className={`nav-link ${workspace.id === activeWorkspace.id ? 'nav-link--active' : ''}`}
                      aria-current={workspace.id === activeWorkspace.id ? 'page' : undefined}
                      onClick={() => onNavigate(workspace)}
                    >
                      <span className="nav-link__icon" aria-hidden="true">{workspace.shortLabel}</span>
                      <span>{workspace.label}</span>
                    </button>
                  ))}
                </div>
              </section>
            );
          })}
        </nav>
      </aside>

      <main className="main-content" id="main-content" tabIndex={-1}>
        <div className="page-toolbar" aria-label="Page context">
          <div className="breadcrumbs">
            <span>Platform</span>
            <span aria-hidden="true">/</span>
            <strong>{activeWorkspace.label}</strong>
          </div>
          <div className="context-controls">
            {access.multiTenant ? (
              <button type="button" className="context-button" disabled title="Backend tenant context not wired yet">
                Tenant
              </button>
            ) : null}
            <button type="button" className="context-button" disabled title="Time context contract pending backend integration">
              Time range
            </button>
            <button type="button" className="context-button" disabled title="Refresh contract pending backend integration">
              Auto-refresh
            </button>
          </div>
        </div>
        {children}
      </main>

      {paletteOpen ? (
        <div className="command-palette-backdrop" role="presentation" onMouseDown={() => setPaletteOpen(false)}>
          <section
            className="command-palette"
            role="dialog"
            aria-modal="true"
            aria-label="Command palette"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <div className="command-palette__header">
              <input
                autoFocus
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Navigate to a workspace"
                aria-label="Search workspaces"
              />
              <button type="button" className="palette-close" onClick={() => setPaletteOpen(false)}>
                Close
              </button>
            </div>
            <div className="command-palette__results">
              {filteredWorkspaces.length > 0 ? (
                filteredWorkspaces.map((workspace) => (
                  <button
                    type="button"
                    className="command-result"
                    key={workspace.id}
                    onClick={() => {
                      onNavigate(workspace);
                      setPaletteOpen(false);
                      setQuery('');
                    }}
                  >
                    <span className="nav-link__icon" aria-hidden="true">{workspace.shortLabel}</span>
                    <span>
                      <strong>{workspace.label}</strong>
                      <small>{workspace.description}</small>
                    </span>
                  </button>
                ))
              ) : (
                <p className="command-empty">No accessible workspace matches this search.</p>
              )}
            </div>
          </section>
        </div>
      ) : null}
    </div>
  );
};

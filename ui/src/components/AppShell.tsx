import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  AccessContext,
  PrimaryNavDestination,
  PrimaryNavGroup,
  WorkspaceDefinition,
  primarySidebarDestinations,
  searchablePrimaryNav,
  searchableWorkspaces,
  visiblePrimaryNav,
} from '../platform/navigation';

export type ThemeMode = 'light' | 'dark' | 'system';
export type DensityMode = 'compact' | 'comfortable' | 'large';

interface AppShellProps {
  access: AccessContext;
  activePrimary?: PrimaryNavDestination;
  activeWorkspace?: WorkspaceDefinition;
  onNavigatePrimary: (destination: PrimaryNavDestination) => void;
  onNavigateWorkspace: (workspace: WorkspaceDefinition) => void;
  theme: ThemeMode;
  onThemeChange: (theme: ThemeMode) => void;
  density: DensityMode;
  onDensityChange: (density: DensityMode) => void;
  children: React.ReactNode;
}

const GROUP_LABELS: Record<PrimaryNavGroup, string> = {
  operate: 'Operate',
  manage: 'Manage',
  govern: 'Govern',
};

const FOCUSABLE_SELECTOR = [
  'button:not([disabled])',
  'a[href]',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',');

export const AppShell: React.FC<AppShellProps> = ({
  access,
  activePrimary,
  activeWorkspace,
  onNavigatePrimary,
  onNavigateWorkspace,
  theme,
  onThemeChange,
  density,
  onDensityChange,
  children,
}) => {
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [query, setQuery] = useState('');
  const paletteDialogRef = useRef<HTMLElement | null>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);
  const sidebarItems = useMemo(() => primarySidebarDestinations(access), [access]);

  const openPalette = useCallback(() => {
    restoreFocusRef.current = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
    setPaletteOpen(true);
  }, []);

  const closePalette = useCallback(() => {
    const focusTarget = restoreFocusRef.current;
    restoreFocusRef.current = null;
    setPaletteOpen(false);
    setQuery('');
    window.requestAnimationFrame(() => focusTarget?.focus());
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        if (paletteOpen) {
          closePalette();
        } else {
          openPalette();
        }
        return;
      }

      if (event.key === 'Escape' && paletteOpen) {
        event.preventDefault();
        closePalette();
      }
    };

    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [closePalette, openPalette, paletteOpen]);

  const filteredWorkspaces = useMemo(
    () => searchableWorkspaces(access, query),
    [access, query],
  );
  const filteredPrimary = useMemo(
    () => searchablePrimaryNav(access, query),
    [access, query],
  );

  const trapPaletteFocus = (event: React.KeyboardEvent<HTMLElement>) => {
    if (event.key !== 'Tab' || !paletteDialogRef.current) {
      return;
    }

    const focusable = Array.from(
      paletteDialogRef.current.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR),
    );
    if (focusable.length === 0) {
      event.preventDefault();
      return;
    }

    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    const active = document.activeElement;

    if (event.shiftKey && (active === first || !paletteDialogRef.current.contains(active))) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && active === last) {
      event.preventDefault();
      first.focus();
    }
  };

  const crumbPrimary = activePrimary
    ?? (activeWorkspace
      ? visiblePrimaryNav(access).find((destination) =>
        destination.relatedWorkspaceIds.includes(activeWorkspace.id),
      )
      : undefined);
  const title = activeWorkspace?.label ?? activePrimary?.label ?? 'Platform';

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
          onClick={openPalette}
          aria-label="Open workspace command palette"
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
          <label className="density-control">
            <span className="sr-only">Density</span>
            <select
              value={density}
              onChange={(event) => onDensityChange(event.target.value as DensityMode)}
              aria-label="Density"
            >
              <option value="compact">Compact</option>
              <option value="comfortable">Comfortable</option>
              <option value="large">Large</option>
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
            className="icon-button"
            aria-label="Contextual help not yet connected"
            disabled
            title="Governed contextual help content is not wired yet"
          >
            Help
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
          {(Object.keys(GROUP_LABELS) as PrimaryNavGroup[]).map((group) => {
            const groupItems = sidebarItems.filter((destination) => destination.group === group);
            if (groupItems.length === 0) {
              return null;
            }

            return (
              <section className="nav-group" key={group} aria-labelledby={`nav-${group}`}>
                <h2 id={`nav-${group}`} className="nav-group__title">
                  {GROUP_LABELS[group]}
                </h2>
                <div className="nav-group__items">
                  {groupItems.map((destination) => {
                    const isActive = activePrimary?.id === destination.id
                      || Boolean(
                        activeWorkspace
                        && destination.relatedWorkspaceIds.includes(activeWorkspace.id)
                        && !activePrimary,
                      )
                      || (destination.id === 'home' && !activeWorkspace && activePrimary?.id === 'home');
                    return (
                      <button
                        type="button"
                        key={destination.id}
                        className={`nav-link ${isActive ? 'nav-link--active' : ''}`}
                        aria-current={isActive ? 'page' : undefined}
                        onClick={() => onNavigatePrimary(destination)}
                      >
                        <span className="nav-link__icon" aria-hidden="true">{destination.shortLabel}</span>
                        <span>{destination.label}</span>
                      </button>
                    );
                  })}
                </div>
              </section>
            );
          })}
        </nav>
      </aside>

      <main className="main-content" id="main-content" tabIndex={-1}>
        <div className="page-toolbar">
          <nav className="breadcrumbs" aria-label="Breadcrumb">
            <span>Platform</span>
            <span aria-hidden="true">/</span>
            {activeWorkspace ? (
              <>
                {crumbPrimary ? (
                  <>
                    <span>{crumbPrimary.label}</span>
                    <span aria-hidden="true">/</span>
                  </>
                ) : null}
                <strong>{activeWorkspace.label}</strong>
              </>
            ) : (
              <strong>{activePrimary?.label ?? title}</strong>
            )}
          </nav>
          <div className="context-controls" role="toolbar" aria-label="Global page context controls">
            {access.multiTenant ? (
              <button type="button" className="context-button" disabled title="Authorized tenant context is not wired yet">
                Tenant
              </button>
            ) : null}
            <button type="button" className="context-button" disabled title="Time context requires real query integration">
              Time range
            </button>
            <button type="button" className="context-button" disabled title="Timezone requires real session/query integration">
              Timezone
            </button>
            <button type="button" className="context-button" disabled title="Comparison requires a bounded query contract">
              Compare
            </button>
            <button type="button" className="context-button" disabled title="Refresh requires real query lifecycle integration">
              Auto-refresh
            </button>
            <button type="button" className="context-button" disabled title="Favorites and recents require persisted user state">
              Favorites / Recent
            </button>
            <button type="button" className="context-button" disabled title="Quick actions require authoritative server authorization">
              Quick actions
            </button>
          </div>
        </div>
        {children}
      </main>

      {paletteOpen ? (
        <div className="command-palette-backdrop" onMouseDown={closePalette}>
          <section
            ref={paletteDialogRef}
            className="command-palette"
            role="dialog"
            aria-modal="true"
            aria-labelledby="command-palette-title"
            onKeyDown={trapPaletteFocus}
            onMouseDown={(event) => event.stopPropagation()}
          >
            <h2 id="command-palette-title" className="sr-only">Workspace command palette</h2>
            <div className="command-palette__header">
              <input
                autoFocus
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search primary navigation and workspaces"
                aria-label="Search accessible workspaces"
              />
              <button type="button" className="palette-close" onClick={closePalette}>
                Close
              </button>
            </div>
            <div className="command-palette__results">
              {filteredPrimary.length === 0 && filteredWorkspaces.length === 0 ? (
                <p className="command-empty">No accessible destination matches this search.</p>
              ) : (
                <>
                  {filteredPrimary.map((destination) => (
                    <button
                      type="button"
                      className="command-result"
                      key={`primary-${destination.id}`}
                      onClick={() => {
                        onNavigatePrimary(destination);
                        closePalette();
                      }}
                    >
                      <span className="nav-link__icon" aria-hidden="true">{destination.shortLabel}</span>
                      <span>
                        <strong>{destination.label}</strong>
                        <small>Primary · {destination.path}</small>
                      </span>
                    </button>
                  ))}
                  {filteredWorkspaces.map((workspace) => (
                    <button
                      type="button"
                      className="command-result"
                      key={`workspace-${workspace.id}`}
                      onClick={() => {
                        onNavigateWorkspace(workspace);
                        closePalette();
                      }}
                    >
                      <span className="nav-link__icon" aria-hidden="true">{workspace.shortLabel}</span>
                      <span>
                        <strong>{workspace.label}</strong>
                        <small>
                          {workspace.uiCode} · {workspace.domain}
                        </small>
                      </span>
                    </button>
                  ))}
                </>
              )}
            </div>
          </section>
        </div>
      ) : null}
    </div>
  );
};

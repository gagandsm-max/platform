/**
 * Capability availability is independent of authoritative screen/data state.
 * Integrating a backend capability does not imply Normal telemetry, freshness, or health.
 */

import type { ScreenDataState } from './screen-state.js';
import type { BackendCapabilityStatus } from './workspace-registry.js';

/**
 * Foundation-only data-state selection when no live query/data-state provider is bound.
 * Never returns `normal` from capability status alone — including INTEGRATED.
 */
export const foundationDataStateForCapability = (
  _capability: BackendCapabilityStatus,
): Exclude<ScreenDataState, 'normal'> => 'empty';

export const capabilityImpliesNormalDataState = (
  capability: BackendCapabilityStatus,
): boolean => {
  void capability;
  return false;
};

/** Deterministic HTML-safe tab id: workspace stable id + tab index (never display label). */
export const workspaceTabDomId = (workspaceId: string, tabIndex: number): string => {
  if (!/^[a-z0-9-]+$/i.test(workspaceId)) {
    throw new Error(`Unsafe workspace id for DOM id: ${workspaceId}`);
  }
  if (!Number.isInteger(tabIndex) || tabIndex < 0) {
    throw new Error(`Unsafe tab index for DOM id: ${tabIndex}`);
  }
  return `workspace-tab-${workspaceId}-${tabIndex}`;
};

export const workspaceTabPanelDomId = (workspaceId: string, tabIndex: number): string =>
  `workspace-tabpanel-${workspaceId}-${tabIndex}`;

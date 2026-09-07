export type GlobalShellCapabilityId =
  | 'authorized-tenant-selector'
  | 'global-search'
  | 'breadcrumbs'
  | 'time-range'
  | 'timezone'
  | 'comparison-period'
  | 'auto-refresh'
  | 'quick-actions'
  | 'notifications'
  | 'contextual-help'
  | 'account-menu'
  | 'favorites-recent'
  | 'command-palette'
  | 'theme';

export type GlobalShellReadiness = 'SHELL_READY' | 'BACKEND_PENDING';

export interface GlobalShellCapability {
  id: GlobalShellCapabilityId;
  readiness: GlobalShellReadiness;
  serverContractRequired: boolean;
  note: string;
}

/**
 * Shell capability registry. BACKEND_PENDING controls remain disabled/non-operational until a real
 * server/session contract exists. This registry is UI framework metadata, not an authorization or API contract.
 */
export const GLOBAL_SHELL_CAPABILITIES: readonly GlobalShellCapability[] = [
  {
    id: 'authorized-tenant-selector',
    readiness: 'BACKEND_PENDING',
    serverContractRequired: true,
    note: 'Only authorized multi-tenant scopes may be supplied by the future session/security bridge.',
  },
  {
    id: 'global-search',
    readiness: 'BACKEND_PENDING',
    serverContractRequired: true,
    note: 'Entity/global search requires bounded server-side search APIs.',
  },
  {
    id: 'breadcrumbs',
    readiness: 'SHELL_READY',
    serverContractRequired: false,
    note: 'Current registered route context is reflected in shell breadcrumbs.',
  },
  {
    id: 'time-range',
    readiness: 'BACKEND_PENDING',
    serverContractRequired: true,
    note: 'Time selection requires real query/state integration.',
  },
  {
    id: 'timezone',
    readiness: 'BACKEND_PENDING',
    serverContractRequired: true,
    note: 'Timezone selection requires real user/query state integration.',
  },
  {
    id: 'comparison-period',
    readiness: 'BACKEND_PENDING',
    serverContractRequired: true,
    note: 'Comparison periods require bounded query contracts.',
  },
  {
    id: 'auto-refresh',
    readiness: 'BACKEND_PENDING',
    serverContractRequired: true,
    note: 'Refresh behavior requires real query lifecycle integration.',
  },
  {
    id: 'quick-actions',
    readiness: 'BACKEND_PENDING',
    serverContractRequired: true,
    note: 'Actions require server authorization and versioned mutation APIs.',
  },
  {
    id: 'notifications',
    readiness: 'BACKEND_PENDING',
    serverContractRequired: true,
    note: 'Notifications require a real server notification contract.',
  },
  {
    id: 'contextual-help',
    readiness: 'BACKEND_PENDING',
    serverContractRequired: true,
    note: 'Contextual help requires governed product/help content and context integration.',
  },
  {
    id: 'account-menu',
    readiness: 'BACKEND_PENDING',
    serverContractRequired: true,
    note: 'Account/session actions require the real identity session contract.',
  },
  {
    id: 'favorites-recent',
    readiness: 'BACKEND_PENDING',
    serverContractRequired: true,
    note: 'Favorites and recents require persisted user state.',
  },
  {
    id: 'command-palette',
    readiness: 'SHELL_READY',
    serverContractRequired: false,
    note: 'Registered accessible workspace navigation is available locally.',
  },
  {
    id: 'theme',
    readiness: 'SHELL_READY',
    serverContractRequired: false,
    note: 'Light, dark and system theme selection is local shell behavior.',
  },
] as const;

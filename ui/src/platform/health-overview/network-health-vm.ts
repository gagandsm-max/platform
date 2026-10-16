import {
  buildHealthOverviewStateCopy,
  resolveUnboundHealthPresentation,
} from './presentation-state.js';
import type {
  HealthOverviewBuildInput,
  HealthOverviewColumn,
  HealthOverviewPanelDescriptor,
  NetworkHealthOverviewVm,
} from './types.js';

export const NETWORK_HEALTH_COLUMNS: readonly HealthOverviewColumn[] = [
  {
    id: 'entity',
    label: 'Entity / Link',
    description: 'Canonical network entity or link identity when Core inventory is bound.',
  },
  {
    id: 'identifier',
    label: 'IP / Identifier',
    description: 'Observed address or stable identifier; mutable observations are not sole identity.',
  },
  {
    id: 'site',
    label: 'Site',
    description: 'Authorized site scope from server-side resource filters.',
  },
  {
    id: 'vendorModel',
    label: 'Vendor / Model',
    description: 'Device pack identification when certified inventory evidence exists.',
  },
  {
    id: 'status',
    label: 'Evidence status',
    description: 'Semantic evidence status — never fabricated DEVICE_DOWN from missing telemetry.',
  },
  {
    id: 'availability',
    label: 'Availability',
    description: 'Availability only when corroborating evidence contracts are satisfied.',
  },
  {
    id: 'primaryKpi',
    label: 'Primary KPI',
    description: 'Primary KPI placeholder pending bounded ClickHouse rollup query.',
  },
  {
    id: 'sensor',
    label: 'Sensor',
    description: 'Assigned Sensor / Sensor Pool owning collection.',
  },
  {
    id: 'alerts',
    label: 'Active alerts',
    description: 'Alert counts only from authorized alert APIs — not fabricated here.',
  },
  {
    id: 'lastUpdate',
    label: 'Last update',
    description: 'Telemetry freshness timestamp when the read contract provides it.',
  },
] as const;

const unboundPanels = (): readonly HealthOverviewPanelDescriptor[] => [
  {
    id: 'devices',
    title: 'Network devices',
    purpose: 'Estate device count for the authorized scope.',
    valueLabel: 'Not connected',
    evidenceNote: 'Awaiting tenant-scoped network inventory/health read API.',
  },
  {
    id: 'interfaces',
    title: 'Interfaces',
    purpose: 'Interface operational evidence coverage.',
    valueLabel: 'Not connected',
    evidenceNote: 'Interface admin/oper state requires SNMP/Device Pack evidence — not invented here.',
  },
  {
    id: 'freshness',
    title: 'Telemetry freshness',
    purpose: 'Collector freshness and quality for network monitors.',
    valueLabel: 'Unknown',
    evidenceNote: 'Freshness metadata is server-derived; without Core freshness evidence this panel stays Unknown.',
  },
  {
    id: 'collector',
    title: 'Collector health',
    purpose: 'Sensor reachability and collection quality, separate from target DOWN.',
    valueLabel: 'Source unavailable',
    evidenceNote: 'Collector failure must not be painted as device down.',
  },
] as const;

const DEFAULT_SAFEGUARDS = [
  'Do not display DEVICE_DOWN when monitoring evidence is missing or a collector is unavailable.',
  'Severity and evidentiary confidence remain separate dimensions.',
  'Synthetic fixtures belong only in explicitly identified non-production tests.',
] as const;

/**
 * Builds the production-safe unbound Network Health Overview view-model.
 * Never fabricates live device state, counters, or availability percentages.
 */
export const buildNetworkHealthOverviewVm = (
  input: HealthOverviewBuildInput = {},
): NetworkHealthOverviewVm => {
  const capabilityStatus = input.capabilityStatus ?? 'PENDING_BACKEND';
  const dataBound = input.dataBound ?? false;
  const presentationState = dataBound
    ? (input.presentationState ?? 'unknown')
    : resolveUnboundHealthPresentation(capabilityStatus, input.presentationState);
  const activeTab = input.activeTab ?? 'Overview';
  const rows = input.rows ?? [];

  return {
    uiCode: 'UI-009',
    workspaceId: 'ui-009',
    label: 'Network Health Overview',
    domain: 'Network',
    activeTab,
    presentationState,
    stateCopy: buildHealthOverviewStateCopy('Network', presentationState, input),
    capabilityStatus,
    dataBound,
    panels: unboundPanels(),
    columns: NETWORK_HEALTH_COLUMNS,
    rows,
    safeguardingNotes: DEFAULT_SAFEGUARDS,
  };
};

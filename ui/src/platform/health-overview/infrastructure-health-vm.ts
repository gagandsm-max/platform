import {
  buildHealthOverviewStateCopy,
  resolveUnboundHealthPresentation,
} from './presentation-state.js';
import type {
  HealthOverviewBuildInput,
  HealthOverviewColumn,
  HealthOverviewPanelDescriptor,
  InfrastructureHealthOverviewVm,
} from './types.js';

export const INFRASTRUCTURE_HEALTH_COLUMNS: readonly HealthOverviewColumn[] = [
  {
    id: 'host',
    label: 'Host / Entity',
    description: 'Canonical host entity identity anchored to entity_id / agent_id when bound.',
  },
  {
    id: 'os',
    label: 'OS / Platform',
    description: 'Observed OS/platform classification from Agent inventory evidence.',
  },
  {
    id: 'site',
    label: 'Site',
    description: 'Authorized site scope from server-side resource filters.',
  },
  {
    id: 'cpu',
    label: 'CPU',
    description: 'CPU utilization only when Agent/host metric evidence is fresh and authorized.',
  },
  {
    id: 'memory',
    label: 'Memory',
    description: 'Memory utilization only from authorized host metrics — not fabricated.',
  },
  {
    id: 'disk',
    label: 'Disk / Storage',
    description: 'Disk/storage capacity evidence when the read contract provides it.',
  },
  {
    id: 'network',
    label: 'Network',
    description: 'Host network evidence; distinct from full APM or always-on DPI.',
  },
  {
    id: 'method',
    label: 'Monitoring method',
    description: 'Agent / agentless method as reported by Core — not assumed.',
  },
  {
    id: 'status',
    label: 'Evidence status',
    description: 'Semantic evidence status — never fabricated HOST_DOWN from missing telemetry.',
  },
  {
    id: 'alerts',
    label: 'Alerts',
    description: 'Alert counts only from authorized alert APIs — not fabricated here.',
  },
] as const;

const unboundPanels = (): readonly HealthOverviewPanelDescriptor[] => [
  {
    id: 'servers',
    title: 'Servers / hosts',
    purpose: 'Linux and infrastructure host count for the authorized scope.',
    valueLabel: 'Not connected',
    evidenceNote: 'Awaiting tenant-scoped infrastructure inventory/health read API.',
  },
  {
    id: 'agents',
    title: 'Linux Agents',
    purpose: 'Agent enrollment and collection coverage.',
    valueLabel: 'Not connected',
    evidenceNote: 'Agent identity remains server-authoritative; UI does not invent enrollment state.',
  },
  {
    id: 'freshness',
    title: 'Telemetry freshness',
    purpose: 'Host metric freshness and quality.',
    valueLabel: 'Unknown',
    evidenceNote: 'Freshness metadata is server-derived; unbound UI stays Unknown.',
  },
  {
    id: 'collector',
    title: 'Collector / Agent health',
    purpose: 'Agent/Sensor health separate from host operational failure.',
    valueLabel: 'Source unavailable',
    evidenceNote: 'Collector or Agent unavailability must not be painted as host down.',
  },
] as const;

const DEFAULT_SAFEGUARDS = [
  'Do not display HOST_DOWN or DEVICE_DOWN when monitoring evidence is missing or a collector is unavailable.',
  'Process/service PID is runtime evidence only and never durable monitor identity.',
  'Synthetic fixtures belong only in explicitly identified non-production tests.',
] as const;

/**
 * Builds the production-safe unbound Infrastructure Health Overview view-model.
 * Never fabricates live host metrics, alerts, or availability percentages.
 */
export const buildInfrastructureHealthOverviewVm = (
  input: HealthOverviewBuildInput = {},
): InfrastructureHealthOverviewVm => {
  const capabilityStatus = input.capabilityStatus ?? 'PENDING_BACKEND';
  const dataBound = input.dataBound ?? false;
  const presentationState = dataBound
    ? (input.presentationState ?? 'unknown')
    : resolveUnboundHealthPresentation(capabilityStatus, input.presentationState);
  const activeTab = input.activeTab ?? 'Overview';
  const rows = input.rows ?? [];

  return {
    uiCode: 'UI-029',
    workspaceId: 'ui-029',
    label: 'Infrastructure Health Overview',
    domain: 'Infrastructure',
    activeTab,
    presentationState,
    stateCopy: buildHealthOverviewStateCopy('Infrastructure', presentationState, input),
    capabilityStatus,
    dataBound,
    panels: unboundPanels(),
    columns: INFRASTRUCTURE_HEALTH_COLUMNS,
    rows,
    safeguardingNotes: DEFAULT_SAFEGUARDS,
  };
};

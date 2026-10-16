import type {
  HealthOverviewBuildInput,
  HealthOverviewPresentationState,
  HealthOverviewStateCopy,
} from './types.js';

export const HEALTH_OVERVIEW_PRESENTATION_STATES: readonly HealthOverviewPresentationState[] = [
  'loading',
  'empty',
  'partial',
  'stale',
  'permission-restricted',
  'not-licensed',
  'query-limited',
  'error',
  'monitoring-degraded',
  'normal',
  'source-unavailable',
  'unknown',
] as const;

export const HEALTH_OVERVIEW_STATE_LABELS: Readonly<
  Record<HealthOverviewPresentationState, string>
> = {
  loading: 'Loading',
  empty: 'No monitoring data',
  partial: 'Partial evidence',
  stale: 'Stale data',
  'permission-restricted': 'Permission restricted',
  'not-licensed': 'Not licensed',
  'query-limited': 'Query limited',
  error: 'Error',
  'monitoring-degraded': 'Monitoring degraded',
  normal: 'Data available',
  'source-unavailable': 'Source unavailable',
  unknown: 'Unknown',
};

const baseEvidence = (
  domain: 'Network' | 'Infrastructure',
  input: HealthOverviewBuildInput,
): string[] => {
  const evidence = [
    `${domain} health overview is presentation-only until a tenant-scoped Core read API is bound.`,
    'FOUNDATION_ACCESS is not authorization; server-side tenant/RBAC remains authoritative.',
    'Missing collector or telemetry evidence must not be presented as device or host down.',
  ];
  if (input.affectedSource) {
    evidence.push(`Affected source: ${input.affectedSource}`);
  }
  if (input.correlationId) {
    evidence.push(`Correlation ID: ${input.correlationId}`);
  }
  if (input.errorId) {
    evidence.push(`Error ID: ${input.errorId}`);
  }
  return evidence;
};

export const buildHealthOverviewStateCopy = (
  domain: 'Network' | 'Infrastructure',
  kind: HealthOverviewPresentationState,
  input: HealthOverviewBuildInput = {},
): HealthOverviewStateCopy => {
  const evidence = baseEvidence(domain, input);
  const subject =
    domain === 'Network' ? 'network device and interface monitoring' : 'Linux host and infrastructure monitoring';

  switch (kind) {
    case 'loading':
      return {
        kind,
        title: `Loading ${domain.toLowerCase()} health`,
        explanation: `Requesting authorized ${subject} evidence. No operational conclusion is drawn while the query is in flight.`,
        nextAction: 'Wait for the bounded server query to complete, or cancel and retry.',
        evidence,
        retryAvailable: false,
        diagnosticAvailable: false,
      };
    case 'empty':
      return {
        kind,
        title: 'No monitoring data',
        explanation: `No ${subject} rows are available for the current presentation scope. This is not an outage assertion.`,
        nextAction: 'Confirm onboarding/discovery and monitoring assignment once Core inventory APIs are available.',
        evidence,
        retryAvailable: true,
        diagnosticAvailable: false,
      };
    case 'partial':
      return {
        kind,
        title: 'Partial evidence',
        explanation: `Some ${subject} sources returned incomplete evidence. Incomplete panels remain visible and labelled; missing sources are not treated as target failure.`,
        nextAction: 'Inspect the affected Sensor/Agent source before asserting resource health.',
        evidence: [
          ...evidence,
          input.affectedSource
            ? `Partial source identified: ${input.affectedSource}`
            : 'Affected source identity will come from the Core query contract when bound.',
        ],
        retryAvailable: true,
        diagnosticAvailable: true,
      };
    case 'stale':
      return {
        kind,
        title: 'Stale data',
        explanation: `Cached or last-known ${subject} evidence is older than the freshness contract. Stale evidence remains visible and labelled; it is not refreshed into a fabricated healthy zero.`,
        nextAction: 'Re-query when Core freshness metadata is available, or investigate collector lag.',
        evidence,
        retryAvailable: true,
        diagnosticAvailable: true,
      };
    case 'permission-restricted':
      return {
        kind,
        title: 'Permission restricted',
        explanation: `The current presentation context cannot show this ${domain.toLowerCase()} health scope. UI filtering is not security enforcement.`,
        nextAction: 'Request the required workspace permission from an authorized administrator; Core authz remains authoritative.',
        evidence,
        retryAvailable: false,
        diagnosticAvailable: false,
      };
    case 'not-licensed':
      return {
        kind,
        title: 'Not licensed',
        explanation: `This ${domain.toLowerCase()} health capability is not licensed for the current tenant. Not Licensed is distinct from Permission Restricted and must not ask the operator to request workspace permission.`,
        nextAction: 'Review licence entitlement with an authorized administrator; Core licence authority remains authoritative.',
        evidence,
        retryAvailable: false,
        diagnosticAvailable: false,
      };
    case 'query-limited':
      return {
        kind,
        title: 'Query limited',
        explanation: `The ${domain.toLowerCase()} health query hit a bounded row/scan limit. Results are truncated intentionally to protect large-estate browsers.`,
        nextAction: 'Narrow filters/site scope once server-side query contracts exist (see Issue #105).',
        evidence,
        retryAvailable: true,
        diagnosticAvailable: false,
      };
    case 'error':
      return {
        kind,
        title: `${domain} health query error`,
        explanation: `The ${subject} request failed. No device or host down conclusion is inferred from this transport/query failure alone.`,
        nextAction: 'Retry the query or open platform diagnostics with the correlation/error identifiers when present.',
        evidence,
        retryAvailable: true,
        diagnosticAvailable: true,
      };
    case 'monitoring-degraded':
      return {
        kind,
        title: 'Monitoring degraded',
        explanation: `Collector or telemetry quality for ${subject} is degraded. Monitoring degradation is distinct from target DOWN and must remain a first-class state.`,
        nextAction: 'Inspect Sensor/Agent health and evidence quality before changing operational severity.',
        evidence,
        retryAvailable: true,
        diagnosticAvailable: true,
      };
    case 'normal':
      return {
        kind,
        title: 'Data available / query normal',
        explanation: `Authorized ${subject} query completed normally. Data available / query normal is not a DEVICE_UP, HOST_UP, availability-success, or healthy-metrics claim; per-entity operational status requires corroborating source evidence.`,
        nextAction: 'Inspect entity evidence rows and collector freshness before asserting target health.',
        evidence,
        retryAvailable: false,
        diagnosticAvailable: true,
      };
    case 'source-unavailable':
      return {
        kind,
        title: 'Source unavailable',
        explanation: `No live Core read API is bound for ${subject}. Capability status may be pending; this foundation does not fabricate metrics, alerts, or availability.`,
        nextAction: 'Keep this presentation preview; integrate only after authenticated tenant-scoped health read APIs exist.',
        evidence,
        retryAvailable: false,
        diagnosticAvailable: false,
      };
    case 'unknown':
      return {
        kind,
        title: 'Unknown health evidence',
        explanation: `Evidence quality for ${subject} is insufficient to classify operational state. UNKNOWN remains explicit and must not collapse into DOWN.`,
        nextAction: 'Gather corroborating collector and reachability evidence before asserting failure.',
        evidence,
        retryAvailable: true,
        diagnosticAvailable: true,
      };
    default: {
      const _exhaustive: never = kind;
      return _exhaustive;
    }
  }
};

/**
 * Default unbound presentation for PENDING_BACKEND health overviews.
 * Capability integration alone never implies Normal.
 */
export const resolveUnboundHealthPresentation = (
  capabilityStatus: 'PENDING_BACKEND' | 'INTEGRATED',
  explicit?: HealthOverviewPresentationState,
): HealthOverviewPresentationState => {
  if (explicit) {
    return explicit;
  }
  void capabilityStatus;
  return 'source-unavailable';
};

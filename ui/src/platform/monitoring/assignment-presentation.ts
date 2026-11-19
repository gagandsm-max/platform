/**
 * BUILD-070 inert monitoring-profile assignment presentation (UI view-model only).
 *
 * Mirrors the bounded Core read projection from
 * `MonitoringProfileAssignmentController.AssignmentResponse` /
 * `MonitoringProfileEntityReadService.EntityAssignmentView`:
 * assignmentMode, profileId, profileVersionNo (immutable version),
 * effectiveFrom, rowVersion.
 *
 * Does not invent API contracts, fetch tenant data, mount routes, or expose
 * definition / capacity / assignedBy / secrets. Missing evidence must not be
 * presented as device DOWN or healthy UP.
 */

import type { PageStateKind } from '../screen-state.js';
import { SCREEN_STATE_LABELS } from '../screen-state.js';

/** Platform_17 / Core AssignmentMode vocabulary. */
export type MonitoringProfileAssignmentMode =
  | 'AUTO_FROM_CRITICALITY'
  | 'EXPLICIT_PROFILE';

export const MONITORING_PROFILE_ASSIGNMENT_MODES: readonly MonitoringProfileAssignmentMode[] = [
  'AUTO_FROM_CRITICALITY',
  'EXPLICIT_PROFILE',
] as const;

/**
 * Operator-facing states required for this inert summary.
 * Subset of Platform_09 §3.2 / PageStateKind — no Normal/healthy implication.
 */
export type MonitoringProfileAssignmentPresentationState =
  | 'loading'
  | 'empty'
  | 'permission-restricted'
  | 'stale'
  | 'error';

export const MONITORING_PROFILE_ASSIGNMENT_PRESENTATION_STATES: readonly MonitoringProfileAssignmentPresentationState[] =
  ['loading', 'empty', 'permission-restricted', 'stale', 'error'] as const;

export type MonitoringProfileAssignmentFieldId =
  | 'assignmentMode'
  | 'profileId'
  | 'profileVersionNo'
  | 'effectiveFrom'
  | 'rowVersion';

/** Untrusted optional input — each field is verified independently. */
export interface MonitoringProfileAssignmentRawFields {
  readonly assignmentMode?: unknown;
  readonly profileId?: unknown;
  readonly profileVersionNo?: unknown;
  readonly effectiveFrom?: unknown;
  readonly rowVersion?: unknown;
}

/** Only fields that passed contract checks. Absent keys were missing or rejected. */
export interface VerifiedMonitoringProfileAssignmentFields {
  readonly assignmentMode?: MonitoringProfileAssignmentMode;
  readonly profileId?: string;
  readonly profileVersionNo?: number;
  readonly effectiveFrom?: string;
  readonly rowVersion?: number;
}

export interface MonitoringProfileAssignmentFieldDisplay {
  readonly id: MonitoringProfileAssignmentFieldId;
  readonly label: string;
  readonly value: string;
}

export interface MonitoringProfileAssignmentStateCopy {
  readonly kind: MonitoringProfileAssignmentPresentationState;
  readonly title: string;
  readonly explanation: string;
  readonly nextAction: string;
  readonly evidence: readonly string[];
}

export interface MonitoringProfileAssignmentBuildInput {
  readonly presentationState?: MonitoringProfileAssignmentPresentationState;
  /** Optional raw assignment projection; never treated as live Core evidence here. */
  readonly assignment?: MonitoringProfileAssignmentRawFields | null;
  readonly correlationId?: string;
  readonly errorId?: string;
}

export interface MonitoringProfileAssignmentViewModel {
  readonly inert: true;
  readonly dataBound: false;
  readonly presentationState: MonitoringProfileAssignmentPresentationState;
  readonly pageStateKind: PageStateKind;
  readonly stateLabel: string;
  readonly stateCopy: MonitoringProfileAssignmentStateCopy;
  readonly verifiedFields: VerifiedMonitoringProfileAssignmentFields;
  readonly fieldDisplays: readonly MonitoringProfileAssignmentFieldDisplay[];
  /** False while loading, empty, permission-restricted, or error. */
  readonly showAssignmentFields: boolean;
  readonly safeguardingNotes: readonly string[];
}

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const FIELD_LABELS: Readonly<Record<MonitoringProfileAssignmentFieldId, string>> = {
  assignmentMode: 'Assignment mode',
  profileId: 'Profile ID',
  profileVersionNo: 'Immutable profile version',
  effectiveFrom: 'Effective from',
  rowVersion: 'Row version',
};

const ASSIGNMENT_MODE_LABELS: Readonly<Record<MonitoringProfileAssignmentMode, string>> = {
  AUTO_FROM_CRITICALITY: 'Automatic from Criticality',
  EXPLICIT_PROFILE: 'Explicit profile',
};

const DEFAULT_SAFEGUARDS = [
  'This surface is inert and read-only: it is not bound to Core or customer telemetry.',
  'Missing or unverified assignment fields must not be presented as device DOWN or healthy UP.',
  'Only verified optional fields from the BUILD-070 assignment projection are shown: assignmentMode, profileId, profileVersionNo, effectiveFrom, rowVersion.',
  'Profile definition, capacity estimates, assignedBy, credentials and secrets are never displayed.',
  'EXPLICIT_PROFILE is never silently replaced by a Criticality change (REQ-INF-016).',
] as const;

const isAssignmentMode = (value: unknown): value is MonitoringProfileAssignmentMode =>
  value === 'AUTO_FROM_CRITICALITY' || value === 'EXPLICIT_PROFILE';

const isPresentationState = (
  value: unknown,
): value is MonitoringProfileAssignmentPresentationState =>
  value === 'loading'
  || value === 'empty'
  || value === 'permission-restricted'
  || value === 'stale'
  || value === 'error';

/**
 * Verify optional assignment projection fields against the Core read contract.
 * Invalid or unknown values are dropped — never coerced into fabricated names/metrics.
 */
export const verifyMonitoringProfileAssignmentFields = (
  raw?: MonitoringProfileAssignmentRawFields | null,
): VerifiedMonitoringProfileAssignmentFields => {
  if (raw == null) {
    return {};
  }

  const verified: {
    -readonly [K in keyof VerifiedMonitoringProfileAssignmentFields]?: VerifiedMonitoringProfileAssignmentFields[K];
  } = {};

  if (isAssignmentMode(raw.assignmentMode)) {
    verified.assignmentMode = raw.assignmentMode;
  }

  if (typeof raw.profileId === 'string') {
    const trimmed = raw.profileId.trim();
    if (UUID_RE.test(trimmed)) {
      verified.profileId = trimmed.toLowerCase();
    }
  }

  if (typeof raw.profileVersionNo === 'number'
    && Number.isInteger(raw.profileVersionNo)
    && raw.profileVersionNo >= 1
    && Number.isSafeInteger(raw.profileVersionNo)) {
    verified.profileVersionNo = raw.profileVersionNo;
  }

  if (typeof raw.effectiveFrom === 'string') {
    const trimmed = raw.effectiveFrom.trim();
    const parsed = Date.parse(trimmed);
    if (trimmed.length > 0 && !Number.isNaN(parsed)) {
      // Preserve the verified ISO/instant text; do not invent a display timezone.
      verified.effectiveFrom = trimmed;
    }
  }

  if (typeof raw.rowVersion === 'number'
    && Number.isInteger(raw.rowVersion)
    && raw.rowVersion > 0
    && Number.isSafeInteger(raw.rowVersion)) {
    verified.rowVersion = raw.rowVersion;
  }

  return verified;
};

export const countVerifiedAssignmentFields = (
  fields: VerifiedMonitoringProfileAssignmentFields,
): number => {
  let count = 0;
  if (fields.assignmentMode !== undefined) count += 1;
  if (fields.profileId !== undefined) count += 1;
  if (fields.profileVersionNo !== undefined) count += 1;
  if (fields.effectiveFrom !== undefined) count += 1;
  if (fields.rowVersion !== undefined) count += 1;
  return count;
};

export const buildAssignmentFieldDisplays = (
  fields: VerifiedMonitoringProfileAssignmentFields,
): readonly MonitoringProfileAssignmentFieldDisplay[] => {
  const displays: MonitoringProfileAssignmentFieldDisplay[] = [];

  if (fields.assignmentMode !== undefined) {
    displays.push({
      id: 'assignmentMode',
      label: FIELD_LABELS.assignmentMode,
      value: ASSIGNMENT_MODE_LABELS[fields.assignmentMode],
    });
  }
  if (fields.profileId !== undefined) {
    displays.push({
      id: 'profileId',
      label: FIELD_LABELS.profileId,
      value: fields.profileId,
    });
  }
  if (fields.profileVersionNo !== undefined) {
    displays.push({
      id: 'profileVersionNo',
      label: FIELD_LABELS.profileVersionNo,
      value: String(fields.profileVersionNo),
    });
  }
  if (fields.effectiveFrom !== undefined) {
    displays.push({
      id: 'effectiveFrom',
      label: FIELD_LABELS.effectiveFrom,
      value: fields.effectiveFrom,
    });
  }
  if (fields.rowVersion !== undefined) {
    displays.push({
      id: 'rowVersion',
      label: FIELD_LABELS.rowVersion,
      value: String(fields.rowVersion),
    });
  }

  return displays;
};

const baseEvidence = (input: MonitoringProfileAssignmentBuildInput): string[] => {
  const evidence = [
    'Inert BUILD-070 assignment presentation — not connected to a live Core read API.',
    'FOUNDATION_ACCESS is not authorization; server-side tenant/RBAC remains authoritative.',
    'Missing assignment evidence must not be presented as device or host down.',
    'Presence of verified identity fields is not a healthy-metrics or DEVICE_UP claim.',
  ];
  if (input.correlationId) {
    evidence.push(`Correlation ID: ${input.correlationId}`);
  }
  if (input.errorId) {
    evidence.push(`Error ID: ${input.errorId}`);
  }
  return evidence;
};

export const buildMonitoringProfileAssignmentStateCopy = (
  kind: MonitoringProfileAssignmentPresentationState,
  input: MonitoringProfileAssignmentBuildInput = {},
): MonitoringProfileAssignmentStateCopy => {
  const evidence = baseEvidence(input);

  switch (kind) {
    case 'loading':
      return {
        kind,
        title: 'Loading monitoring profile assignment',
        explanation:
          'Requesting authorized assignment evidence. No operational conclusion is drawn while the query is in flight.',
        nextAction:
          'Wait for the bounded server query. This inert surface has no retry or mutation controls.',
        evidence,
      };
    case 'empty':
      return {
        kind,
        title: 'No monitoring profile assignment',
        explanation:
          'No assignment projection is available for the current presentation scope. This is not an outage or healthy-estate assertion.',
        nextAction:
          'Confirm entity onboarding and BUILD-070 assignment once a tenant-scoped Core read is bound.',
        evidence,
      };
    case 'permission-restricted':
      return {
        kind,
        title: 'Permission restricted',
        explanation:
          'The current presentation context cannot show this monitoring profile assignment. UI filtering is not security enforcement.',
        nextAction:
          'Request entity.read from an authorized administrator; Core authz remains authoritative.',
        evidence,
      };
    case 'stale':
      return {
        kind,
        title: 'Stale assignment data',
        explanation:
          'Last-known assignment fields are older than the freshness contract or are supplied only for inert presentation. Stale evidence remains labelled and is not refreshed into a fabricated healthy state.',
        nextAction:
          'Re-query when an authenticated Core assignment read is bound. Mutation controls are intentionally absent.',
        evidence,
      };
    case 'error':
      return {
        kind,
        title: 'Monitoring profile assignment error',
        explanation:
          'The assignment request failed. No device or host down conclusion is inferred from this transport/query failure alone.',
        nextAction:
          'Retain correlation/error identifiers for support. This inert surface does not offer retry mutation.',
        evidence,
      };
    default: {
      const _exhaustive: never = kind;
      return _exhaustive;
    }
  }
};

/**
 * Resolve presentation state for the inert summary.
 * Never defaults to Normal/healthy. Verified fields without an explicit state
 * surface as stale — not as a fresh operational success.
 */
export const resolveMonitoringProfileAssignmentPresentation = (
  input: MonitoringProfileAssignmentBuildInput,
  verified: VerifiedMonitoringProfileAssignmentFields,
): MonitoringProfileAssignmentPresentationState => {
  if (input.presentationState !== undefined && isPresentationState(input.presentationState)) {
    return input.presentationState;
  }
  if (countVerifiedAssignmentFields(verified) > 0) {
    return 'stale';
  }
  return 'empty';
};

export const shouldShowAssignmentFields = (
  presentationState: MonitoringProfileAssignmentPresentationState,
  verified: VerifiedMonitoringProfileAssignmentFields,
): boolean => {
  if (countVerifiedAssignmentFields(verified) < 1) {
    return false;
  }
  // Hide protected/incomplete payloads for loading, empty, denial and error.
  return presentationState === 'stale';
};

/**
 * Deterministic pure builder for the inert assignment summary view model.
 */
export const buildMonitoringProfileAssignmentVm = (
  input: MonitoringProfileAssignmentBuildInput = {},
): MonitoringProfileAssignmentViewModel => {
  const verifiedFields = verifyMonitoringProfileAssignmentFields(input.assignment);
  const presentationState = resolveMonitoringProfileAssignmentPresentation(input, verifiedFields);
  const stateCopy = buildMonitoringProfileAssignmentStateCopy(presentationState, input);
  const fieldDisplays = buildAssignmentFieldDisplays(verifiedFields);
  const showAssignmentFields = shouldShowAssignmentFields(presentationState, verifiedFields);

  return {
    inert: true,
    dataBound: false,
    presentationState,
    pageStateKind: presentationState,
    stateLabel: SCREEN_STATE_LABELS[presentationState],
    stateCopy,
    verifiedFields,
    fieldDisplays,
    showAssignmentFields,
    safeguardingNotes: DEFAULT_SAFEGUARDS,
  };
};

export const MONITORING_PROFILE_ASSIGNMENT_FIELD_LABELS = FIELD_LABELS;
export const MONITORING_PROFILE_ASSIGNMENT_MODE_LABELS = ASSIGNMENT_MODE_LABELS;

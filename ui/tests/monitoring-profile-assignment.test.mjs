import test from 'node:test';
import assert from 'node:assert/strict';
import {
  MONITORING_PROFILE_ASSIGNMENT_FIELD_LABELS,
  MONITORING_PROFILE_ASSIGNMENT_MODES,
  MONITORING_PROFILE_ASSIGNMENT_PRESENTATION_STATES,
  buildAssignmentFieldDisplays,
  buildMonitoringProfileAssignmentStateCopy,
  buildMonitoringProfileAssignmentVm,
  countVerifiedAssignmentFields,
  resolveMonitoringProfileAssignmentPresentation,
  shouldShowAssignmentFields,
  verifyMonitoringProfileAssignmentFields,
} from '../.test-dist/platform/monitoring/assignment-presentation.js';
import { SCREEN_STATE_LABELS } from '../.test-dist/platform/screen-state.js';

const VALID_PROFILE_ID = '018f0e0a-0000-7000-8000-0000000000e5';
const VALID_ASSIGNMENT = {
  assignmentMode: 'EXPLICIT_PROFILE',
  profileId: VALID_PROFILE_ID,
  profileVersionNo: 7,
  effectiveFrom: '2026-11-18T00:00:00Z',
  rowVersion: 15,
};

test('assignment presentation states are exactly the required inert set', () => {
  assert.deepEqual([...MONITORING_PROFILE_ASSIGNMENT_PRESENTATION_STATES], [
    'loading',
    'empty',
    'permission-restricted',
    'stale',
    'error',
  ]);
  for (const kind of MONITORING_PROFILE_ASSIGNMENT_PRESENTATION_STATES) {
    assert.ok(SCREEN_STATE_LABELS[kind]);
  }
});

test('assignment mode vocabulary matches Platform_17 / Core contract', () => {
  assert.deepEqual([...MONITORING_PROFILE_ASSIGNMENT_MODES], [
    'AUTO_FROM_CRITICALITY',
    'EXPLICIT_PROFILE',
  ]);
});

test('inert default view model is empty, unbound, and non-health-implying', () => {
  const model = buildMonitoringProfileAssignmentVm();
  assert.equal(model.inert, true);
  assert.equal(model.dataBound, false);
  assert.equal(model.presentationState, 'empty');
  assert.equal(model.pageStateKind, 'empty');
  assert.equal(model.stateLabel, SCREEN_STATE_LABELS.empty);
  assert.equal(model.showAssignmentFields, false);
  assert.equal(countVerifiedAssignmentFields(model.verifiedFields), 0);
  assert.equal(model.fieldDisplays.length, 0);
  assert.equal(/device\s+down|host\s+down|DEVICE_DOWN|HOST_DOWN/i.test(model.stateCopy.title), false);
  assert.equal(/healthy|DEVICE_UP|HOST_UP/i.test(model.stateCopy.title), false);
  assert.ok(
    model.safeguardingNotes.some((note) => /must not be presented as device DOWN or healthy UP/i.test(note)),
  );
});

test('verifyMonitoringProfileAssignmentFields accepts only contract-valid optional fields', () => {
  const verified = verifyMonitoringProfileAssignmentFields(VALID_ASSIGNMENT);
  assert.deepEqual(verified, {
    assignmentMode: 'EXPLICIT_PROFILE',
    profileId: VALID_PROFILE_ID,
    profileVersionNo: 7,
    effectiveFrom: '2026-11-18T00:00:00Z',
    rowVersion: 15,
  });
  assert.equal(countVerifiedAssignmentFields(verified), 5);
});

test('verifyMonitoringProfileAssignmentFields drops invalid and unknown values', () => {
  const verified = verifyMonitoringProfileAssignmentFields({
    assignmentMode: 'SOMETHING_ELSE',
    profileId: 'not-a-uuid',
    profileVersionNo: 0,
    effectiveFrom: '   ',
    rowVersion: -1,
    definition: { secret: 'nope' },
    capacityEstimate: 99,
    assignedBy: 'actor',
  });
  assert.deepEqual(verified, {});
  assert.equal(countVerifiedAssignmentFields(verified), 0);
});

test('partial verified fields remain optional and do not invent missing values', () => {
  const verified = verifyMonitoringProfileAssignmentFields({
    assignmentMode: 'AUTO_FROM_CRITICALITY',
    profileId: VALID_PROFILE_ID.toUpperCase(),
    profileVersionNo: 3.5,
    rowVersion: 2,
  });
  assert.deepEqual(verified, {
    assignmentMode: 'AUTO_FROM_CRITICALITY',
    profileId: VALID_PROFILE_ID,
    rowVersion: 2,
  });
  assert.equal(verified.profileVersionNo, undefined);
  assert.equal(verified.effectiveFrom, undefined);
});

test('verified fields without explicit state resolve to stale, never normal/healthy', () => {
  const verified = verifyMonitoringProfileAssignmentFields(VALID_ASSIGNMENT);
  assert.equal(
    resolveMonitoringProfileAssignmentPresentation({}, verified),
    'stale',
  );
  const model = buildMonitoringProfileAssignmentVm({ assignment: VALID_ASSIGNMENT });
  assert.equal(model.presentationState, 'stale');
  assert.notEqual(model.presentationState, 'normal');
  assert.equal(model.showAssignmentFields, true);
  assert.deepEqual(
    model.fieldDisplays.map((field) => field.id),
    ['assignmentMode', 'profileId', 'profileVersionNo', 'effectiveFrom', 'rowVersion'],
  );
  assert.equal(
    model.fieldDisplays.find((field) => field.id === 'assignmentMode')?.value,
    'Explicit profile',
  );
  assert.equal(
    model.fieldDisplays.find((field) => field.id === 'profileVersionNo')?.label,
    MONITORING_PROFILE_ASSIGNMENT_FIELD_LABELS.profileVersionNo,
  );
});

test('loading, empty, permission-restricted and error hide assignment fields', () => {
  const verified = verifyMonitoringProfileAssignmentFields(VALID_ASSIGNMENT);
  for (const kind of ['loading', 'empty', 'permission-restricted', 'error']) {
    assert.equal(shouldShowAssignmentFields(kind, verified), false);
    const model = buildMonitoringProfileAssignmentVm({
      presentationState: kind,
      assignment: VALID_ASSIGNMENT,
    });
    assert.equal(model.presentationState, kind);
    assert.equal(model.showAssignmentFields, false);
    // Verification still runs; display is gated.
    assert.equal(countVerifiedAssignmentFields(model.verifiedFields), 5);
  }
  assert.equal(shouldShowAssignmentFields('stale', verified), true);
});

test('every required presentation state has non-health-implying operator copy', () => {
  for (const kind of MONITORING_PROFILE_ASSIGNMENT_PRESENTATION_STATES) {
    const copy = buildMonitoringProfileAssignmentStateCopy(kind, {
      correlationId: 'corr-fixture',
      errorId: 'err-fixture',
    });
    assert.equal(copy.kind, kind);
    assert.ok(copy.title.length > 0);
    assert.ok(copy.explanation.length > 0);
    assert.ok(copy.nextAction.length > 0);
    assert.equal(/asserts?\s+(device|host)\s+down/i.test(copy.explanation), false);
    assert.equal(/device down|host down|DEVICE_DOWN|HOST_DOWN/i.test(copy.title), false);
    assert.equal(/DEVICE_UP|HOST_UP|healthy zero/i.test(copy.explanation), false);
    assert.ok(copy.evidence.some((item) => /must not be presented as device or host down/i.test(item)));
    assert.ok(copy.evidence.some((item) => /Correlation ID: corr-fixture/.test(item)));
    assert.ok(copy.evidence.some((item) => /Error ID: err-fixture/.test(item)));
  }
});

test('field displays expose accessible labels for verified fields only', () => {
  const displays = buildAssignmentFieldDisplays({
    assignmentMode: 'AUTO_FROM_CRITICALITY',
    profileVersionNo: 1,
  });
  assert.deepEqual(
    displays.map((field) => ({ id: field.id, label: field.label, value: field.value })),
    [
      {
        id: 'assignmentMode',
        label: 'Assignment mode',
        value: 'Automatic from Criticality',
      },
      {
        id: 'profileVersionNo',
        label: 'Immutable profile version',
        value: '1',
      },
    ],
  );
});

test('null assignment input is treated as absent, not healthy', () => {
  const model = buildMonitoringProfileAssignmentVm({
    presentationState: 'empty',
    assignment: null,
  });
  assert.equal(model.presentationState, 'empty');
  assert.equal(model.showAssignmentFields, false);
  assert.deepEqual(model.verifiedFields, {});
});

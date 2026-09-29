package control_test

import (
	"testing"
	"time"

	"platform/sensor/internal/control"
)

func TestStatusTrackerSyncPendingThenInSync(t *testing.T) {
	t.Parallel()
	tr := control.NewStatusTracker(&control.Metrics{})
	if tr.Snapshot().State != control.StateSyncPending {
		t.Fatalf("state=%s", tr.Snapshot().State)
	}
	tr.Observe(control.Result{Action: "NO_DESIRED"}, nil)
	if tr.Snapshot().State != control.StateInSync {
		t.Fatalf("state=%s", tr.Snapshot().State)
	}
	tr.Observe(control.Result{Action: "APPLIED", Generation: 3, Outcome: control.OutcomeApplied}, nil)
	snap := tr.Snapshot()
	if snap.State != control.StateInSync || snap.LastGeneration != 3 {
		t.Fatalf("%+v", snap)
	}
}

func TestStatusTrackerDegradedAndUntrusted(t *testing.T) {
	t.Parallel()
	tr := control.NewStatusTracker(&control.Metrics{})
	tr.Observe(control.Result{Action: "CORE_UNAVAILABLE", Code: control.CodeCoreUnavailable}, control.NewApplyFailedError("x", nil))
	if tr.Snapshot().State != control.StateDegraded {
		t.Fatalf("state=%s", tr.Snapshot().State)
	}
	tr.Observe(control.Result{Action: "APPLY_FAILED", Code: control.CodeRestartRequired, Outcome: control.OutcomeRejected}, nil)
	if tr.Snapshot().State != control.StateDegraded || tr.Snapshot().LastCode != control.CodeRestartRequired {
		t.Fatalf("%+v", tr.Snapshot())
	}
	tr.MarkUntrusted(control.CodeStateCorrupt)
	if tr.Snapshot().State != control.StateUntrusted {
		t.Fatalf("state=%s", tr.Snapshot().State)
	}
	tr.Observe(control.Result{Action: "DURABILITY_FAILED", Code: control.CodeStateDurabilityFailed}, nil)
	if tr.Snapshot().State != control.StateUntrusted {
		t.Fatalf("state=%s", tr.Snapshot().State)
	}
}

func TestStatusTrackerUntrustedStickyAcrossNoDesired(t *testing.T) {
	t.Parallel()
	tr := control.NewStatusTracker(&control.Metrics{})
	tr.MarkUntrusted(control.CodeStateCorrupt)
	tr.Observe(control.Result{Action: "NO_DESIRED"}, nil)
	if tr.Snapshot().State != control.StateUntrusted {
		t.Fatalf("MarkUntrusted + NO_DESIRED must stay STATE_UNTRUSTED, got %s", tr.Snapshot().State)
	}
}

func TestStatusTrackerDurabilityFailedStickyAcrossNoDesired(t *testing.T) {
	t.Parallel()
	tr := control.NewStatusTracker(&control.Metrics{})
	tr.Observe(control.Result{Action: "DURABILITY_FAILED", Code: control.CodeStateDurabilityFailed}, nil)
	if tr.Snapshot().State != control.StateUntrusted {
		t.Fatalf("state=%s", tr.Snapshot().State)
	}
	tr.Observe(control.Result{Action: "NO_DESIRED"}, nil)
	if tr.Snapshot().State != control.StateUntrusted {
		t.Fatalf("DURABILITY_FAILED + NO_DESIRED must stay STATE_UNTRUSTED, got %s", tr.Snapshot().State)
	}
}

func TestStatusTrackerAppliedClearsUntrusted(t *testing.T) {
	t.Parallel()
	tr := control.NewStatusTracker(&control.Metrics{})
	tr.MarkUntrusted(control.CodeStateCorrupt)
	tr.Observe(control.Result{Action: "APPLIED", Generation: 5, Outcome: control.OutcomeApplied}, nil)
	snap := tr.Snapshot()
	if snap.State != control.StateInSync {
		t.Fatalf("successful APPLIED must clear untrusted, got %s", snap.State)
	}
	if snap.LastSuccessfulReconcile.IsZero() || time.Since(snap.LastSuccessfulReconcile) > time.Second {
		t.Fatalf("last success=%v", snap.LastSuccessfulReconcile)
	}
}

func TestStatusTrackerLostAckRepairClearsUntrusted(t *testing.T) {
	t.Parallel()
	tr := control.NewStatusTracker(&control.Metrics{})
	tr.Observe(control.Result{Action: "DURABILITY_FAILED", Code: control.CodeStateDurabilityFailed}, nil)
	tr.Observe(control.Result{Action: "LOST_ACK_REPAIR", Generation: 7, Outcome: control.OutcomeApplied}, nil)
	if tr.Snapshot().State != control.StateInSync {
		t.Fatalf("successful LOST_ACK_REPAIR must clear untrusted, got %s", tr.Snapshot().State)
	}
}

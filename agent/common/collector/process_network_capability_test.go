package collector

import (
	"testing"
	"time"
)

func TestProcessNetworkCapabilityStateSetIsClosed(t *testing.T) {
	states := ProcessNetworkCapabilityStates()
	if len(states) != 6 {
		t.Fatalf("public state count=%d want=6: %v", len(states), states)
	}
	seen := make(map[ProcessNetworkCapabilityState]bool, len(states))
	for _, state := range states {
		if !state.Valid() {
			t.Fatalf("listed state is invalid: %q", state)
		}
		if seen[state] {
			t.Fatalf("duplicate state: %q", state)
		}
		seen[state] = true
	}
	if ProcessNetworkCapabilityState("COMPATIBLE").Valid() {
		t.Fatal("preflight compatibility must not become a seventh public state")
	}
}

func TestProcessNetworkCapabilityFreshUsesExclusiveValidityBound(t *testing.T) {
	validUntil := time.Date(2026, 10, 7, 2, 0, 0, 0, time.UTC)
	snapshot := ProcessNetworkCapabilitySnapshot{
		State:           ProcessNetworkCapabilityKernelIncompatible,
		ReasonCodes:     []ProcessNetworkCapabilityReason{ReasonBTFMissing},
		ValidUntil:      validUntil,
		PreflightPassed: true,
	}
	fresh := snapshot.Fresh(validUntil.Add(-time.Nanosecond))
	if fresh.State != ProcessNetworkCapabilityKernelIncompatible {
		t.Fatalf("evidence became stale early: %+v", fresh)
	}
	stale := snapshot.Fresh(validUntil)
	if stale.State != ProcessNetworkCapabilityUnavailable || len(stale.ReasonCodes) != 1 || stale.ReasonCodes[0] != ReasonEvidenceStale || stale.PreflightPassed {
		t.Fatalf("exclusive validity bound not enforced: %+v", stale)
	}
	later := snapshot.Fresh(validUntil.Add(time.Hour))
	if !stale.TransitionAt.Equal(validUntil) || !stale.NextRecheckAt.Equal(validUntil) || !later.TransitionAt.Equal(stale.TransitionAt) || !later.NextRecheckAt.Equal(stale.NextRecheckAt) {
		t.Fatalf("stale transition must remain at deterministic expiry: stale=%+v later=%+v", stale, later)
	}
}

func TestProcessNetworkCapabilitySnapshotCopyDoesNotAliasReasons(t *testing.T) {
	original := ProcessNetworkCapabilitySnapshot{
		State:       ProcessNetworkCapabilityUnavailable,
		ReasonCodes: []ProcessNetworkCapabilityReason{ReasonBackendNotWired},
	}
	copy := original.Copy()
	copy.ReasonCodes[0] = ReasonSourceUnavailable
	if original.ReasonCodes[0] != ReasonBackendNotWired {
		t.Fatal("snapshot copy aliases reason codes")
	}
}

func TestCurrentAuthorityCapabilityDoesNotAgeStale(t *testing.T) {
	expired := time.Date(2026, 10, 7, 1, 0, 0, 0, time.UTC)
	now := expired.Add(time.Hour)

	active := ProcessNetworkCapabilitySnapshot{
		State:            ProcessNetworkCapabilityActive,
		ValidUntil:       expired,
		CurrentAuthority: true,
	}
	if got := active.Fresh(now); got.State != ProcessNetworkCapabilityActive {
		t.Fatalf("runtime ACTIVE falsely staled: %+v", got)
	}

	denied := ProcessNetworkCapabilitySnapshot{
		State:            ProcessNetworkCapabilityPrivilegeDenied,
		ReasonCodes:      []ProcessNetworkCapabilityReason{ReasonPrivilegeDenied},
		ValidUntil:       expired,
		CurrentAuthority: true,
	}
	if got := denied.Fresh(now); got.State != ProcessNetworkCapabilityPrivilegeDenied || len(got.ReasonCodes) != 1 || got.ReasonCodes[0] != ReasonPrivilegeDenied {
		t.Fatalf("runtime failure falsely staled: %+v", got)
	}

	unwired := ProcessNetworkCapabilitySnapshot{
		State:            ProcessNetworkCapabilityUnavailable,
		ReasonCodes:      []ProcessNetworkCapabilityReason{ReasonBackendNotWired},
		CurrentAuthority: true,
		// Zero ValidUntil must not render EVIDENCE_STALE for current runtime authority.
	}
	if got := unwired.Fresh(now); got.State != ProcessNetworkCapabilityUnavailable || len(got.ReasonCodes) != 1 || got.ReasonCodes[0] != ReasonBackendNotWired {
		t.Fatalf("initial runtime BACKEND_NOT_WIRED falsely staled: %+v", got)
	}

	disabled := ProcessNetworkCapabilitySnapshot{
		State:            ProcessNetworkCapabilityUnavailable,
		ReasonCodes:      []ProcessNetworkCapabilityReason{ReasonFeatureDisabled},
		ValidUntil:       expired,
		CurrentAuthority: true,
	}
	if got := disabled.Fresh(now); got.State != ProcessNetworkCapabilityUnavailable || len(got.ReasonCodes) != 1 || got.ReasonCodes[0] != ReasonFeatureDisabled {
		t.Fatalf("FEATURE_DISABLED falsely staled: %+v", got)
	}

	historical := ProcessNetworkCapabilitySnapshot{
		State:       ProcessNetworkCapabilityKernelIncompatible,
		ReasonCodes: []ProcessNetworkCapabilityReason{ReasonBTFMissing},
		ValidUntil:  expired,
	}
	if got := historical.Fresh(now); got.State != ProcessNetworkCapabilityUnavailable || len(got.ReasonCodes) != 1 || got.ReasonCodes[0] != ReasonEvidenceStale {
		t.Fatalf("historical preflight must still become EVIDENCE_STALE: %+v", got)
	}
}

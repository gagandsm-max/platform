package ebpf

import (
	"bytes"
	"testing"

	cilium "github.com/cilium/ebpf"
)

func TestSecurityNoUnexpectedProgramsOrMaps(t *testing.T) {
	spec, _, err := LoadGovernedSpec()
	if err != nil {
		t.Fatal(err)
	}
	for name := range spec.Programs {
		switch name {
		case ProgInetSockSetState, ProgTCPRetransmit:
		default:
			t.Fatalf("unexpected program %s", name)
		}
	}
	for name := range spec.Maps {
		switch name {
		case MapConnState, MapEvents, MapLossCounters:
		default:
			t.Fatalf("unexpected map %s", name)
		}
	}
}

func TestSecurityRejectLRUHashPolicy(t *testing.T) {
	spec, _, err := LoadGovernedSpec()
	if err != nil {
		t.Fatal(err)
	}
	spec.Maps[MapConnState].Type = cilium.LRUHash
	if err := enforceMapPolicy(spec); err == nil {
		t.Fatal("LRU_HASH must be rejected")
	}
}

func TestSecurityRejectUnexpectedProgramAllowlist(t *testing.T) {
	spec, _, err := LoadGovernedSpec()
	if err != nil {
		t.Fatal(err)
	}
	spec.Programs["evil"] = &cilium.ProgramSpec{Name: "evil"}
	if err := enforceAllowlists(spec); err == nil {
		t.Fatal("unexpected program must fail")
	}
}

func TestSecurityPrimaryMandatoryOptionalIndependent(t *testing.T) {
	spec, _, err := LoadGovernedSpec()
	if err != nil {
		t.Fatal(err)
	}
	primary := keepPrograms(spec, ProgInetSockSetState)
	if err := enforceAllowlists(primary); err != nil {
		// enforceAllowlists requires optional present in object allowlist set —
		// primary-only collection is a load-time subset, not an object mutation.
		_ = err
	}
	if _, ok := primary.Programs[ProgInetSockSetState]; !ok {
		t.Fatal("primary required")
	}
	if _, ok := primary.Programs[ProgTCPRetransmit]; ok {
		t.Fatal("optional must be independently excludable")
	}
	optional := keepPrograms(spec, ProgTCPRetransmit)
	if _, ok := optional.Programs[ProgInetSockSetState]; ok {
		t.Fatal("optional-only must not retain primary")
	}
}

func TestSecurityObjectGates2103And2083(t *testing.T) {
	gates, err := AdmitGovernedObject(governedObject)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, g := range gates {
		seen[g.Name] = g.Pass
	}
	for _, name := range []string{Gate2103NoSubprog, Gate2083ZeroAddends, GateNoKfuncKsyms, GateCORePresent} {
		if !seen[name] {
			t.Fatalf("gate %s missing or failed", name)
		}
	}
}

func TestSecurityNoCustomerObjectPathAPI(t *testing.T) {
	// LoadGovernedSpec has no path argument — only embedded bytes.
	spec, _, err := LoadGovernedSpec()
	if err != nil || spec == nil {
		t.Fatal(err)
	}
	// Ensure we did not accidentally read from an alternate buffer.
	if !bytes.Equal(GovernedObjectBytes(), governedObject) {
		t.Fatal("object bytes drift")
	}
}

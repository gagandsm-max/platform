package ebpf

import (
	"testing"

	cilium "github.com/cilium/ebpf"
)

func TestGovernedObjectAdmissionPasses(t *testing.T) {
	gates, err := AdmitGovernedObject(governedObject)
	if err != nil {
		t.Fatalf("admit: %v gates=%v", err, gates)
	}
	want := map[string]bool{
		GateTextEmpty: true, Gate2103NoSubprog: true, GateNoKfuncKsyms: true,
		Gate2083ZeroAddends: true, GateCORePresent: true, GateProgramAllow: true, GateMapAllow: true,
	}
	for _, g := range gates {
		if !g.Pass || !want[g.Name] {
			t.Fatalf("gate %+v", g)
		}
		delete(want, g.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing gates %v", want)
	}
}

func TestGovernedObjectTamperRejected(t *testing.T) {
	// Corrupt ELF magic — must fail closed before kernel interaction.
	tampered := append([]byte(nil), governedObject...)
	if len(tampered) < 4 {
		t.Fatal("object too small")
	}
	copy(tampered[0:4], []byte{0x00, 0x00, 0x00, 0x00})
	if _, err := AdmitGovernedObject(tampered); err == nil {
		t.Fatal("expected ELF magic tamper rejection")
	}
	// Truncation must also fail closed.
	if _, err := AdmitGovernedObject(governedObject[:16]); err == nil {
		t.Fatal("expected truncated object rejection")
	}
}

func TestGovernedObjectEmptyRejected(t *testing.T) {
	if _, err := AdmitGovernedObject(nil); err == nil {
		t.Fatal("expected empty reject")
	}
}

func TestLoadGovernedSpecAllowlistsAndMapPolicy(t *testing.T) {
	spec, _, err := LoadGovernedSpec()
	if err != nil {
		t.Fatal(err)
	}
	if err := enforceAllowlists(spec); err != nil {
		t.Fatal(err)
	}
	if err := enforceMapPolicy(spec); err != nil {
		t.Fatal(err)
	}
	if spec.Maps[MapConnState].Type != cilium.Hash {
		t.Fatal("HASH required")
	}
	if _, ok := spec.Programs[ProgInetSockSetState]; !ok {
		t.Fatal("primary missing")
	}
	if _, ok := spec.Programs[ProgTCPRetransmit]; !ok {
		t.Fatal("optional missing from object")
	}
}

func TestKeepProgramsPrimaryOnly(t *testing.T) {
	spec, _, err := LoadGovernedSpec()
	if err != nil {
		t.Fatal(err)
	}
	primary := keepPrograms(spec, ProgInetSockSetState)
	if _, ok := primary.Programs[ProgTCPRetransmit]; ok {
		t.Fatal("optional must be excluded for primary-only load")
	}
	if _, ok := primary.Programs[ProgInetSockSetState]; !ok {
		t.Fatal("primary required")
	}
}

func TestNoKfuncInGovernedObject(t *testing.T) {
	gates, err := AdmitGovernedObject(governedObject)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range gates {
		if g.Name == GateNoKfuncKsyms && !g.Pass {
			t.Fatalf("%+v", g)
		}
	}
}

func TestResourceBoundsWithinCeiling(t *testing.T) {
	total := RequestedMapBytesEstimate + RequestedRingBytesExact
	if total > defaultMaximumResourceBytes {
		t.Fatalf("requested %d exceeds ceiling %d", total, defaultMaximumResourceBytes)
	}
	if StateMapMaxEntries != 4096 || RingBufferBytes != 1*1024*1024 {
		t.Fatal("bounds drift")
	}
	if BackendGoroutines != 2 || RawRecordChannel != 256 || IngestBatchSize != 256 {
		t.Fatal("userspace bounds drift")
	}
}

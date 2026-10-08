package ebpf

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"

	cilium "github.com/cilium/ebpf"
)

//go:embed bpf/object/process_net.o
var governedObject []byte

// GovernedObjectSHA256 is the SHA-256 of the embedded Clang-produced object.
// Updated when the governed object bytes change.
func GovernedObjectSHA256() string {
	sum := sha256.Sum256(governedObject)
	return hex.EncodeToString(sum[:])
}

// GovernedObjectBytes returns a copy of the embedded object.
func GovernedObjectBytes() []byte {
	out := make([]byte, len(governedObject))
	copy(out, governedObject)
	return out
}

// LoadGovernedSpec fail-closes object integrity and allowlists before any
// kernel map/program creation. No configurable arbitrary object path.
func LoadGovernedSpec() (*cilium.CollectionSpec, []objectGateResult, error) {
	if len(governedObject) == 0 {
		return nil, nil, errors.New("OBJECT_INTEGRITY_EMPTY")
	}
	gates, err := AdmitGovernedObject(governedObject)
	if err != nil {
		return nil, gates, err
	}
	spec, err := cilium.LoadCollectionSpecFromReader(bytes.NewReader(governedObject))
	if err != nil {
		return nil, gates, fmt.Errorf("OBJECT_INTEGRITY_SPEC: %w", err)
	}
	if err := enforceAllowlists(spec); err != nil {
		return nil, gates, err
	}
	if err := enforceMapPolicy(spec); err != nil {
		return nil, gates, err
	}
	return spec, gates, nil
}

func enforceAllowlists(spec *cilium.CollectionSpec) error {
	if spec == nil {
		return errors.New("OBJECT_ALLOWLIST_NIL_SPEC")
	}
	allowedProg := map[string]bool{
		ProgInetSockSetState: true,
		ProgTCPRetransmit:    true,
	}
	for name := range spec.Programs {
		if !allowedProg[name] {
			return fmt.Errorf("OBJECT_PROGRAM_ALLOWLIST: unexpected %s", name)
		}
	}
	if _, ok := spec.Programs[ProgInetSockSetState]; !ok {
		return errors.New("OBJECT_PROGRAM_ALLOWLIST: missing primary " + ProgInetSockSetState)
	}
	allowedMap := map[string]bool{
		MapConnState:      true,
		MapEvents:         true,
		MapLossCounters:   true,
	}
	for name := range spec.Maps {
		if !allowedMap[name] {
			return fmt.Errorf("OBJECT_MAP_ALLOWLIST: unexpected %s", name)
		}
	}
	for name := range allowedMap {
		if _, ok := spec.Maps[name]; !ok {
			return fmt.Errorf("OBJECT_MAP_ALLOWLIST: missing %s", name)
		}
	}
	return nil
}

func enforceMapPolicy(spec *cilium.CollectionSpec) error {
	state := spec.Maps[MapConnState]
	if state == nil {
		return errors.New("MAP_POLICY_MISSING_CONN_STATE")
	}
	if state.Type != cilium.Hash {
		return fmt.Errorf("MAP_POLICY_HASH_REQUIRED: got %s", state.Type)
	}
	if state.MaxEntries != StateMapMaxEntries {
		return fmt.Errorf("MAP_POLICY_MAX_ENTRIES: got %d want %d", state.MaxEntries, StateMapMaxEntries)
	}
	events := spec.Maps[MapEvents]
	if events == nil {
		return errors.New("MAP_POLICY_MISSING_EVENTS")
	}
	if events.Type != cilium.RingBuf {
		return fmt.Errorf("MAP_POLICY_RINGBUF_REQUIRED: got %s", events.Type)
	}
	if events.MaxEntries != RingBufferBytes {
		return fmt.Errorf("MAP_POLICY_RING_SIZE: got %d want %d", events.MaxEntries, RingBufferBytes)
	}
	return nil
}

func keepPrograms(spec *cilium.CollectionSpec, names ...string) *cilium.CollectionSpec {
	cpy := spec.Copy()
	keep := make(map[string]struct{}, len(names))
	for _, n := range names {
		keep[n] = struct{}{}
	}
	for name := range cpy.Programs {
		if _, ok := keep[name]; !ok {
			delete(cpy.Programs, name)
		}
	}
	return cpy
}

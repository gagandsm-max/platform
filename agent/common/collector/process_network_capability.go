package collector

import "time"

// ProcessNetworkCapabilityState is the complete public eBPF capability
// vocabulary. Preflight compatibility is evidence, not a seventh state.
type ProcessNetworkCapabilityState string

const (
	ProcessNetworkCapabilityActive             ProcessNetworkCapabilityState = "ACTIVE"
	ProcessNetworkCapabilityUnsupported        ProcessNetworkCapabilityState = "UNSUPPORTED"
	ProcessNetworkCapabilityUnavailable        ProcessNetworkCapabilityState = "UNAVAILABLE"
	ProcessNetworkCapabilityPrivilegeDenied    ProcessNetworkCapabilityState = "PRIVILEGE_DENIED"
	ProcessNetworkCapabilityKernelIncompatible ProcessNetworkCapabilityState = "KERNEL_INCOMPATIBLE"
	ProcessNetworkCapabilityResourceGuarded    ProcessNetworkCapabilityState = "RESOURCE_GUARDED"
)

// ProcessNetworkCapabilityStates returns a copy of the closed public state set.
func ProcessNetworkCapabilityStates() []ProcessNetworkCapabilityState {
	return []ProcessNetworkCapabilityState{
		ProcessNetworkCapabilityActive,
		ProcessNetworkCapabilityUnsupported,
		ProcessNetworkCapabilityUnavailable,
		ProcessNetworkCapabilityPrivilegeDenied,
		ProcessNetworkCapabilityKernelIncompatible,
		ProcessNetworkCapabilityResourceGuarded,
	}
}

func (s ProcessNetworkCapabilityState) Valid() bool {
	switch s {
	case ProcessNetworkCapabilityActive,
		ProcessNetworkCapabilityUnsupported,
		ProcessNetworkCapabilityUnavailable,
		ProcessNetworkCapabilityPrivilegeDenied,
		ProcessNetworkCapabilityKernelIncompatible,
		ProcessNetworkCapabilityResourceGuarded:
		return true
	default:
		return false
	}
}

// ProcessNetworkCapabilityReason is a bounded machine reason. Raw filesystem,
// kernel, verifier, command, or environment text must never be stored here.
type ProcessNetworkCapabilityReason string

const (
	ReasonFeatureDisabled       ProcessNetworkCapabilityReason = "FEATURE_DISABLED"
	ReasonBackendNotImplemented ProcessNetworkCapabilityReason = "BACKEND_NOT_IMPLEMENTED"
	ReasonOSUnsupported         ProcessNetworkCapabilityReason = "OS_UNSUPPORTED"
	ReasonArchUnsupported       ProcessNetworkCapabilityReason = "ARCH_UNSUPPORTED"
	ReasonKernelUnsupported     ProcessNetworkCapabilityReason = "KERNEL_UNSUPPORTED"
	ReasonBTFMissing            ProcessNetworkCapabilityReason = "BTF_MISSING"
	ReasonBTFUnreadable         ProcessNetworkCapabilityReason = "BTF_UNREADABLE"
	ReasonBTFIncompatible       ProcessNetworkCapabilityReason = "BTF_INCOMPATIBLE"
	ReasonCAPBPFMissing         ProcessNetworkCapabilityReason = "CAP_BPF_MISSING"
	ReasonCAPPerfmonMissing     ProcessNetworkCapabilityReason = "CAP_PERFMON_MISSING"
	ReasonCapabilityReadFailed  ProcessNetworkCapabilityReason = "CAPABILITY_READ_FAILED"
	ReasonPrivilegeDenied       ProcessNetworkCapabilityReason = "PRIVILEGE_DENIED"
	ReasonSecurityContextDenied ProcessNetworkCapabilityReason = "SECURITY_CONTEXT_DENIED"
	ReasonResourceLimit         ProcessNetworkCapabilityReason = "RESOURCE_LIMIT"
	ReasonEvidenceStale         ProcessNetworkCapabilityReason = "EVIDENCE_STALE"
	ReasonPreflightTimeout      ProcessNetworkCapabilityReason = "PREFLIGHT_TIMEOUT"
	ReasonPreflightCancelled    ProcessNetworkCapabilityReason = "PREFLIGHT_CANCELLED"
	ReasonSourceUnavailable        ProcessNetworkCapabilityReason = "SOURCE_UNAVAILABLE"
	ReasonObjectIntegrityFailed    ProcessNetworkCapabilityReason = "OBJECT_INTEGRITY_FAILED"
	ReasonVerifierRejected         ProcessNetworkCapabilityReason = "BACKEND_VERIFIER_REJECTED"
	ReasonTracepointUnavailable    ProcessNetworkCapabilityReason = "BACKEND_TRACEPOINT_UNAVAILABLE"
	ReasonAttachFailed             ProcessNetworkCapabilityReason = "BACKEND_ATTACH_FAILED"
	ReasonLoadFailed               ProcessNetworkCapabilityReason = "BACKEND_LOAD_FAILED"
	ReasonReaderFailed             ProcessNetworkCapabilityReason = "READER_FAILED"
	ReasonOptionalProbeUnavailable ProcessNetworkCapabilityReason = "OPTIONAL_PROBE_UNAVAILABLE"
	ReasonBackendNotWired          ProcessNetworkCapabilityReason = "BACKEND_NOT_WIRED"
	ReasonBackendStopped           ProcessNetworkCapabilityReason = "BACKEND_STOPPED"
)

// ProcessNetworkCapabilityPresence exposes capability evidence without raw
// /proc masks or UID-derived assumptions.
type ProcessNetworkCapabilityPresence struct {
	Effective bool `json:"effective"`
	Permitted bool `json:"permitted"`
	Bounding  bool `json:"bounding"`
	Ambient   bool `json:"ambient"`
}

// ProcessNetworkResourceEvidence is a non-allocating admission seam. Values
// are implementation safety estimates, not certified SKU capacity.
type ProcessNetworkResourceEvidence struct {
	RequestedMapBytes  uint64 `json:"requested_map_bytes"`
	RequestedRingBytes uint64 `json:"requested_ring_bytes"`
	MaximumBytes       uint64 `json:"maximum_implementation_bytes"`
	Guarded            bool   `json:"guarded"`
	LowWaterSatisfied  bool   `json:"low_water_satisfied"`
}

// ProcessNetworkCapabilitySnapshot is immutable optional support evidence.
// It deliberately contains no raw proc/sysfs content, paths, payload, command
// line, environment, verifier output, kernel address, or credential material.
type ProcessNetworkCapabilitySnapshot struct {
	State             ProcessNetworkCapabilityState    `json:"state"`
	ReasonCodes       []ProcessNetworkCapabilityReason `json:"reason_codes"`
	ObservedAt        time.Time                        `json:"observed_at"`
	TransitionAt      time.Time                        `json:"transition_at"`
	LastSuccessAt     time.Time                        `json:"last_success_at,omitempty"`
	ValidUntil        time.Time                        `json:"valid_until"`
	NextRecheckAt     time.Time                        `json:"next_recheck_at"`
	AttemptCount      uint32                           `json:"attempt_count"`
	KernelFingerprint string                           `json:"kernel_fingerprint"`
	OS                string                           `json:"os"`
	Arch              string                           `json:"arch"`
	KernelRelease     string                           `json:"kernel_release"`
	BootIDDigest      string                           `json:"boot_id_digest"`
	BTFPresent        bool                             `json:"btf_present"`
	BTFRegular        bool                             `json:"btf_regular"`
	BTFReadable       bool                             `json:"btf_readable"`
	BTFDigest         string                           `json:"btf_digest,omitempty"`
	BTFDigestBytes    int64                            `json:"btf_digest_bytes,omitempty"`
	CAPBPF            ProcessNetworkCapabilityPresence `json:"cap_bpf"`
	CAPPerfmon        ProcessNetworkCapabilityPresence `json:"cap_perfmon"`
	SecurityContext   string                           `json:"security_context"`
	KernelCompatible  bool                             `json:"kernel_compatible"`
	PreflightPassed   bool                             `json:"preflight_passed"`
	Resource          ProcessNetworkResourceEvidence   `json:"resource_guard"`
	SourceQuality     string                           `json:"source_quality"`
}

func (s ProcessNetworkCapabilitySnapshot) Available() bool {
	return s.State.Valid()
}

// Copy returns a deep copy suitable for crossing health/support boundaries.
func (s ProcessNetworkCapabilitySnapshot) Copy() ProcessNetworkCapabilitySnapshot {
	s.ReasonCodes = append([]ProcessNetworkCapabilityReason(nil), s.ReasonCodes...)
	return s
}

// Fresh returns a copy and converts expired evidence to the truthful public
// UNAVAILABLE/EVIDENCE_STALE state. ValidUntil is an exclusive bound.
func (s ProcessNetworkCapabilitySnapshot) Fresh(now time.Time) ProcessNetworkCapabilitySnapshot {
	out := s.Copy()
	now = now.UTC()
	if out.ValidUntil.IsZero() || !now.Before(out.ValidUntil) {
		out.State = ProcessNetworkCapabilityUnavailable
		out.ReasonCodes = []ProcessNetworkCapabilityReason{ReasonEvidenceStale}
		// Expiry is the deterministic state-transition boundary. Do not move
		// transition/recheck timestamps each time support evidence is rendered.
		out.TransitionAt = out.ValidUntil
		out.NextRecheckAt = out.ValidUntil
		out.PreflightPassed = false
	}
	return out
}

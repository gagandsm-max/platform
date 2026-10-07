package collector

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strings"
	"time"

	"platform/agent/common/telemetry"
)

// Process-to-network typed evidence (BUILD-011 Slice 5).
//
// Kernel-independent user-space model and local aggregation contract.
// This is not an eBPF backend, not a HostCollector, not scalar telemetry,
// and not Agent→Sensor transport. Physical delivery remains BUILD-016/062.
// Quality/source state never asserts host, process, service, or network DOWN.

const ProcessNetworkSchemaVersion = 1

const (
	defaultProcessNetworkWindow             = 60 * time.Second
	minProcessNetworkWindow                 = 15 * time.Second
	maxProcessNetworkWindow                 = 300 * time.Second
	maxProcessNetworkGlobalKeys             = 4096
	maxProcessNetworkKeysPerWorkload        = 64
	maxProcessNetworkInputBatch             = 1024
	maxProcessNetworkRetainedWindows        = 4
	maxProcessNetworkClosedWindows          = 2
	maxProcessNetworkAccountingStubs        = 8
	maxProcessNetworkContainerIDLen         = maxProcessServiceIdentityLen
	maxProcessNetworkProvenanceLen          = maxProcessServiceProvenanceLen
	ProcessNetworkSyntheticSourceProvenance = "agent.linux.process_network.synthetic"
)

var (
	ErrProcessNetworkObservedAtInvalid       = errors.New("PROCESS_NETWORK_OBSERVED_AT_INVALID: observed_at is required and must be non-zero")
	ErrProcessNetworkAccountingBoundExceeded = errors.New("PROCESS_NETWORK_ACCOUNTING_BOUND_EXCEEDED: exact-bucket accounting capacity exhausted")
)

// ProcessNetworkEventClass is a backend-neutral synthetic event class.
type ProcessNetworkEventClass string

const (
	ProcessNetworkEventOpen        ProcessNetworkEventClass = "OPEN"
	ProcessNetworkEventClose       ProcessNetworkEventClass = "CLOSE"
	ProcessNetworkEventFail        ProcessNetworkEventClass = "FAIL"
	ProcessNetworkEventBytes       ProcessNetworkEventClass = "BYTES"
	ProcessNetworkEventRTT         ProcessNetworkEventClass = "RTT"
	ProcessNetworkEventRetransmit  ProcessNetworkEventClass = "RETRANSMIT"
	ProcessNetworkEventBackendDrop ProcessNetworkEventClass = "BACKEND_DROP"
)

// ProcessNetworkProtocol is the allowlisted transport protocol.
type ProcessNetworkProtocol string

const (
	ProcessNetworkProtocolTCP ProcessNetworkProtocol = "TCP"
	ProcessNetworkProtocolUDP ProcessNetworkProtocol = "UDP"
)

// ProcessNetworkQualityState is record/collection quality, not operational DOWN.
type ProcessNetworkQualityState string

const (
	ProcessNetworkQualityValid           ProcessNetworkQualityState = "VALID"
	ProcessNetworkQualityPartial         ProcessNetworkQualityState = "PARTIAL"
	ProcessNetworkQualitySampled         ProcessNetworkQualityState = "SAMPLED"
	ProcessNetworkQualityResourceGuarded ProcessNetworkQualityState = "RESOURCE_GUARDED"
	ProcessNetworkQualityUnsupported     ProcessNetworkQualityState = "UNSUPPORTED"
)

// ProcessNetworkQualityAssertsHostProcessOrNetworkDown is always false.
func ProcessNetworkQualityAssertsHostProcessOrNetworkDown(state ProcessNetworkQualityState) bool {
	_ = state
	return false
}

// ProcessNetworkSyntheticEvent is a backend-neutral user-space input event.
// BACKEND_DROP must not require or fabricate a remote socket endpoint.
type ProcessNetworkSyntheticEvent struct {
	ObservedAt time.Time
	Class      ProcessNetworkEventClass

	ServiceID          string
	ExecutableIdentity string
	ExecutablePath     string
	ProcessName        string
	PID                *uint32
	ContainerID        string
	CgroupPath         string
	NetworkNamespace   *uint64

	Protocol   ProcessNetworkProtocol
	RemoteIP   string
	RemotePort uint16
	LocalIP    string
	LocalPort  *uint16

	BytesTx         *uint64
	BytesRx         *uint64
	DurationMs      *uint64
	RTTUs           *uint32
	RetransmitCount *uint32
	DroppedCount    *uint32
	Sampled         bool
	ResourceGuarded bool
}

// ProcessNetworkWorkloadIdentity is the durable aggregation identity.
// PID is never sufficient and is never the durable key.
type ProcessNetworkWorkloadIdentity struct {
	Kind               SelectorKind
	Value              string
	ServiceID          string
	ExecutableIdentity string
	ExecutablePath     string
	ProcessName        string
	ContainerID        string
	CgroupPath         string
	NetworkNamespace   *uint64
}

// ProcessNetworkEvidence is one ordinary real-destination aggregate record.
// RemoteIP is always a canonical unmapped IPv4 or IPv6 address — never a
// sentinel such as __overflow_other__.
type ProcessNetworkEvidence struct {
	SchemaVersion    int
	SourceProvenance string
	BucketStartAt    time.Time
	BucketEndAt      time.Time
	BucketSeconds    uint32

	Workload ProcessNetworkWorkloadIdentity
	PID      *uint32

	LocalIP    *netip.Addr
	LocalPort  *uint16
	RemoteIP   netip.Addr
	RemotePort uint16
	Protocol   ProcessNetworkProtocol
	IPVersion  string

	BytesTx            *uint64
	BytesRx            *uint64
	ActiveConnections  *uint32
	NewConnections     *uint32
	ClosedConnections  *uint32
	DurationTotalMs    *uint64
	MeanRTTUs          *uint32
	RetransmitCount    *uint32
	ConnectionFailures *uint32

	Quality             ProcessNetworkQualityState
	DroppedRecordsCount uint32
	ShedRecordsCount    uint32
	SampledEventsCount  uint32
	SampledFlag         bool
	PartialFlag         bool
	ResourceGuardedFlag bool
}

// ProcessNetworkOverflowSummary retains only aggregatable facts for traffic
// that is not represented by a real destination record. It is not an IP.
type ProcessNetworkOverflowSummary struct {
	Protocol               ProcessNetworkProtocol
	OverflowedRecordsCount uint64
	BytesTx                *uint64
	BytesRx                *uint64
	ActiveConnections      *uint32
	DurationTotalMs        *uint64
	MeanRTTUs              *uint32
	RetransmitCount        *uint32
	NewConnections         *uint32
	ClosedConnections      *uint32
	ConnectionFailures     *uint32
	EvictedCandidateCount  uint32
	ShedRecordsCount       uint32
	SampledEventsCount     uint32
}

// ProcessNetworkWindowAccounting is bounded local typed accounting for a
// window/workload (and protocol where required). Overflow is never encoded
// as a remote IP address.
type ProcessNetworkWindowAccounting struct {
	SchemaVersion          int
	SourceProvenance       string
	BucketStartAt          time.Time
	BucketEndAt            time.Time
	BucketSeconds          uint32
	Workload               *ProcessNetworkWorkloadIdentity
	DroppedRecordsCount    uint32
	ShedRecordsCount       uint32
	SampledEventsCount     uint32
	LateEventsCount        uint32
	EvictedCandidateCount  uint32
	OverflowedRecordsCount uint64
	Overflows              []ProcessNetworkOverflowSummary
	Quality                ProcessNetworkQualityState
	PartialFlag            bool
	SampledFlag            bool
	ResourceGuardedFlag    bool
}

// Validate checks ordinary real-destination invariants.
func (e ProcessNetworkEvidence) Validate() error {
	if e.SchemaVersion != ProcessNetworkSchemaVersion {
		return fmt.Errorf("PROCESS_NETWORK_SCHEMA_INVALID: unsupported schema_version %d", e.SchemaVersion)
	}
	if err := validateProcessNetworkProvenance(e.SourceProvenance); err != nil {
		return err
	}
	if err := validateBucketBounds(e.BucketStartAt, e.BucketEndAt, e.BucketSeconds); err != nil {
		return err
	}
	if err := e.Workload.validate(); err != nil {
		return err
	}
	if !e.RemoteIP.IsValid() || e.RemoteIP.IsUnspecified() {
		return errors.New("PROCESS_NETWORK_REMOTE_INVALID: ordinary evidence requires a real canonical IP")
	}
	if e.RemoteIP != e.RemoteIP.Unmap() {
		return errors.New("PROCESS_NETWORK_REMOTE_INVALID: IPv4-mapped IPv6 must be unmapped")
	}
	if e.RemotePort < 1 {
		return errors.New("PROCESS_NETWORK_REMOTE_PORT_INVALID: ordinary remote port must be 1..65535")
	}
	if err := validateOptionalLocalEndpoint(e.LocalIP, e.LocalPort); err != nil {
		return err
	}
	switch e.Protocol {
	case ProcessNetworkProtocolTCP, ProcessNetworkProtocolUDP:
	default:
		return fmt.Errorf("PROCESS_NETWORK_PROTOCOL_INVALID: %q", e.Protocol)
	}
	wantVer := "IPv4"
	if e.RemoteIP.Is6() {
		wantVer = "IPv6"
	}
	if e.IPVersion != wantVer {
		return fmt.Errorf("PROCESS_NETWORK_IP_VERSION_INVALID: want %s", wantVer)
	}
	if isForbiddenRemoteToken(e.RemoteIP.String()) {
		return errors.New("PROCESS_NETWORK_REMOTE_INVALID: overflow sentinels are not remote IPs")
	}
	if err := validateQuality(e.Quality); err != nil {
		return err
	}
	if e.Protocol == ProcessNetworkProtocolUDP {
		if e.ActiveConnections != nil || e.NewConnections != nil || e.ClosedConnections != nil ||
			e.DurationTotalMs != nil || e.MeanRTTUs != nil || e.RetransmitCount != nil || e.ConnectionFailures != nil {
			return errors.New("PROCESS_NETWORK_UDP_TCP_METRIC_FABRICATED: TCP-specific values must not be emitted for UDP")
		}
	}
	if ProcessNetworkQualityAssertsHostProcessOrNetworkDown(e.Quality) {
		return errors.New("PROCESS_NETWORK_QUALITY_DOWN_FORBIDDEN")
	}
	return nil
}

func (a ProcessNetworkWindowAccounting) Validate() error {
	if a.SchemaVersion != ProcessNetworkSchemaVersion {
		return fmt.Errorf("PROCESS_NETWORK_SCHEMA_INVALID: unsupported schema_version %d", a.SchemaVersion)
	}
	if err := validateProcessNetworkProvenance(a.SourceProvenance); err != nil {
		return err
	}
	if err := validateBucketBounds(a.BucketStartAt, a.BucketEndAt, a.BucketSeconds); err != nil {
		return err
	}
	if a.Workload != nil {
		if err := a.Workload.validate(); err != nil {
			return err
		}
	}
	if err := validateQuality(a.Quality); err != nil {
		return err
	}
	if len(a.Overflows) > maxProcessNetworkKeysPerWorkload*2 {
		return fmt.Errorf("PROCESS_NETWORK_OVERFLOW_BOUND_EXCEEDED: %d", len(a.Overflows))
	}
	for i := range a.Overflows {
		if err := a.Overflows[i].validate(); err != nil {
			return err
		}
	}
	return nil
}

func (o ProcessNetworkOverflowSummary) validate() error {
	switch o.Protocol {
	case ProcessNetworkProtocolTCP, ProcessNetworkProtocolUDP:
	default:
		return fmt.Errorf("PROCESS_NETWORK_PROTOCOL_INVALID: %q", o.Protocol)
	}
	if o.Protocol == ProcessNetworkProtocolUDP {
		if o.ActiveConnections != nil || o.NewConnections != nil || o.ClosedConnections != nil ||
			o.DurationTotalMs != nil || o.MeanRTTUs != nil || o.RetransmitCount != nil || o.ConnectionFailures != nil {
			return errors.New("PROCESS_NETWORK_UDP_TCP_METRIC_FABRICATED: TCP-specific values must not be emitted for UDP")
		}
	}
	if o.MeanRTTUs != nil && o.Protocol != ProcessNetworkProtocolTCP {
		return errors.New("PROCESS_NETWORK_UDP_TCP_METRIC_FABRICATED: mean RTT is TCP-only")
	}
	return nil
}

func validateBucketBounds(start, end time.Time, secs uint32) error {
	if start.IsZero() || end.IsZero() {
		return errors.New("PROCESS_NETWORK_BUCKET_INVALID: bucket bounds are required")
	}
	if end.Before(start) {
		return errors.New("PROCESS_NETWORK_BUCKET_INVALID: bucket end precedes start")
	}
	if secs == 0 {
		return errors.New("PROCESS_NETWORK_BUCKET_INVALID: bucket_duration_sec is required")
	}
	dur := end.Sub(start)
	if dur%time.Second != 0 {
		return errors.New("PROCESS_NETWORK_BUCKET_INVALID: bucket interval must be an exact whole-second duration")
	}
	want := time.Duration(secs) * time.Second
	if dur != want {
		return fmt.Errorf("PROCESS_NETWORK_BUCKET_INVALID: bucket_duration_sec %d does not match interval", secs)
	}
	return nil
}

func validateSyntheticLocalEndpoint(raw string, port *uint16) error {
	if raw != "" {
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			return fmt.Errorf("PROCESS_NETWORK_LOCAL_INVALID: %w", err)
		}
		addr = addr.Unmap()
		if !addr.IsValid() || addr.IsUnspecified() {
			return errors.New("PROCESS_NETWORK_LOCAL_INVALID: local IP must be a real address when present")
		}
	}
	if port != nil && *port < 1 {
		return errors.New("PROCESS_NETWORK_LOCAL_PORT_INVALID: local port must be 1..65535 when present")
	}
	return nil
}

func validateOptionalLocalEndpoint(ip *netip.Addr, port *uint16) error {
	if ip != nil {
		if !ip.IsValid() || ip.IsUnspecified() {
			return errors.New("PROCESS_NETWORK_LOCAL_INVALID: local IP must be a real canonical address when present")
		}
		if *ip != ip.Unmap() {
			return errors.New("PROCESS_NETWORK_LOCAL_INVALID: IPv4-mapped IPv6 must be unmapped")
		}
	}
	if port != nil && *port < 1 {
		return errors.New("PROCESS_NETWORK_LOCAL_PORT_INVALID: local port must be 1..65535 when present")
	}
	return nil
}

func validateProcessNetworkProvenance(v string) error {
	if v == "" {
		return errors.New("PROCESS_NETWORK_PROVENANCE_INVALID: source_provenance is required")
	}
	if strings.TrimSpace(v) != v {
		return errors.New("PROCESS_NETWORK_PROVENANCE_INVALID: leading/trailing whitespace is prohibited")
	}
	if len(v) > maxProcessNetworkProvenanceLen {
		return fmt.Errorf("PROCESS_NETWORK_PROVENANCE_INVALID: exceeds implementation safety bound %d", maxProcessNetworkProvenanceLen)
	}
	if hasControlRunes(v) {
		return errors.New("PROCESS_NETWORK_PROVENANCE_INVALID: control characters are prohibited")
	}
	return nil
}

func (w ProcessNetworkWorkloadIdentity) validate() error {
	switch w.Kind {
	case SelectorKindServiceID, SelectorKindExecutableIdentity, SelectorKindExecutablePath, SelectorKindProcessName:
	case "PID", "PROCESS_ID":
		return errors.New("PROCESS_NETWORK_IDENTITY_PID_REJECTED: PID is not durable aggregation identity")
	default:
		return fmt.Errorf("PROCESS_NETWORK_IDENTITY_INVALID: unsupported kind %q", w.Kind)
	}
	sel := ProcessServiceSelector{Kind: w.Kind, Value: w.Value}
	if err := sel.Validate(); err != nil {
		return fmt.Errorf("PROCESS_NETWORK_IDENTITY_INVALID: %w", err)
	}
	if err := optionalProcessToken("service_id", w.ServiceID, maxProcessServiceIdentityLen); err != nil {
		return err
	}
	if err := optionalProcessToken("executable_identity", w.ExecutableIdentity, maxProcessServiceIdentityLen); err != nil {
		return err
	}
	if err := optionalProcessToken("executable_path", w.ExecutablePath, maxProcessServiceIdentityLen); err != nil {
		return err
	}
	if err := optionalProcessToken("process_name", w.ProcessName, maxProcessServiceNameLen); err != nil {
		return err
	}
	if err := optionalProcessToken("container_id", w.ContainerID, maxProcessNetworkContainerIDLen); err != nil {
		return err
	}
	if err := optionalProcessToken("cgroup_path", w.CgroupPath, maxProcessServiceIdentityLen); err != nil {
		return err
	}
	return nil
}

func validateQuality(q ProcessNetworkQualityState) error {
	switch q {
	case ProcessNetworkQualityValid, ProcessNetworkQualityPartial, ProcessNetworkQualitySampled,
		ProcessNetworkQualityResourceGuarded, ProcessNetworkQualityUnsupported:
		return nil
	default:
		return fmt.Errorf("PROCESS_NETWORK_QUALITY_INVALID: %q", q)
	}
}

func isForbiddenRemoteToken(s string) bool {
	v := strings.TrimSpace(strings.ToLower(s))
	return v == "__overflow_other__" || v == "other_endpoints" || v == "overflow" || v == "other"
}

func canonicalizeRemoteAddr(raw string) (netip.Addr, error) {
	if isForbiddenRemoteToken(raw) {
		return netip.Addr{}, errors.New("PROCESS_NETWORK_REMOTE_INVALID: overflow sentinels are not remote IPs")
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("PROCESS_NETWORK_REMOTE_INVALID: %w", err)
	}
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsUnspecified() {
		return netip.Addr{}, errors.New("PROCESS_NETWORK_REMOTE_INVALID: ordinary evidence requires a real canonical IP")
	}
	return addr, nil
}

func durableWorkloadFromEvent(e ProcessNetworkSyntheticEvent) (ProcessNetworkWorkloadIdentity, error) {
	id := ProcessNetworkWorkloadIdentity{
		ServiceID:          e.ServiceID,
		ExecutableIdentity: e.ExecutableIdentity,
		ExecutablePath:     e.ExecutablePath,
		ProcessName:        e.ProcessName,
		ContainerID:        e.ContainerID,
		CgroupPath:         e.CgroupPath,
	}
	if e.NetworkNamespace != nil {
		ns := *e.NetworkNamespace
		id.NetworkNamespace = &ns
	}
	entry := ProcessServiceCatalogueEntry{
		ServiceID:          e.ServiceID,
		ExecutableIdentity: e.ExecutableIdentity,
		ExecutablePath:     e.ExecutablePath,
		ProcessFamilyName:  e.ProcessName,
	}
	sel, err := entry.PreferredDurableSelector()
	if err != nil {
		return ProcessNetworkWorkloadIdentity{}, errors.New("PROCESS_NETWORK_IDENTITY_PID_REJECTED: PID-only events are not durable aggregation identity")
	}
	id.Kind = sel.Kind
	id.Value = sel.Value
	if err := id.validate(); err != nil {
		return ProcessNetworkWorkloadIdentity{}, err
	}
	return id, nil
}

func (w ProcessNetworkWorkloadIdentity) aggregationKey() string {
	// Length-prefixed fields: literal '|' inside Value/ContainerID must not collide keys.
	var b strings.Builder
	writeLenPrefixed(&b, string(w.Kind))
	writeLenPrefixed(&b, w.Value)
	writeLenPrefixed(&b, w.ContainerID)
	if w.NetworkNamespace != nil {
		b.WriteByte('1')
		fmt.Fprintf(&b, "%d", *w.NetworkNamespace)
	} else {
		// Preserve absent-versus-zero namespace distinction.
		b.WriteByte('0')
	}
	return b.String()
}

func writeLenPrefixed(b *strings.Builder, s string) {
	fmt.Fprintf(b, "%d:", len(s))
	b.WriteString(s)
}

func destinationKey(addr netip.Addr, port uint16, proto ProcessNetworkProtocol) string {
	return string(proto) + "|" + addr.String() + "|" + fmt.Sprintf("%d", port)
}

func ipVersion(addr netip.Addr) string {
	if addr.Is4() {
		return "IPv4"
	}
	return "IPv6"
}

func uint64Ptr(v uint64) *uint64 { return &v }
func uint32Ptr(v uint32) *uint32 { return &v }
func uint16Ptr(v uint16) *uint16 { return &v }

func satAddUint64(cur, add uint64) (uint64, bool) {
	if add > math.MaxUint64-cur {
		return math.MaxUint64, true
	}
	return cur + add, false
}

func satAddUint32(cur, add uint32) (uint32, bool) {
	if add > math.MaxUint32-cur {
		return math.MaxUint32, true
	}
	return cur + add, false
}

func satIncUint64(cur uint64) (uint64, bool) { return satAddUint64(cur, 1) }
func satIncUint32(cur uint32) (uint32, bool) { return satAddUint32(cur, 1) }

func meanRTTUs(sum uint64, n uint32) (uint32, bool) {
	if n == 0 {
		return 0, false
	}
	avg := sum / uint64(n)
	if avg > math.MaxUint32 {
		return math.MaxUint32, true
	}
	return uint32(avg), false
}

func mergeQuality(cur, add ProcessNetworkQualityState) ProcessNetworkQualityState {
	if qualityRank(add) > qualityRank(cur) {
		return add
	}
	return cur
}

func qualityRank(q ProcessNetworkQualityState) int {
	switch q {
	case ProcessNetworkQualityValid:
		return 0
	case ProcessNetworkQualityPartial:
		return 1
	case ProcessNetworkQualitySampled:
		return 2
	case ProcessNetworkQualityResourceGuarded:
		return 3
	case ProcessNetworkQualityUnsupported:
		return 4
	default:
		return 0
	}
}

// EncodeProcessNetworkEvidenceAsScalarSamples rejects high-cardinality label abuse.
func EncodeProcessNetworkEvidenceAsScalarSamples(_ ProcessNetworkEvidence) ([]telemetry.Sample, error) {
	return nil, errors.New("PROCESS_NETWORK_SCALAR_ENCODING_REJECTED: structured process-network evidence must not be encoded as scalar Sample labels")
}

func (UnwiredStructuredEvidenceSink) AcceptProcessNetworkEvidence(_ context.Context, rec ProcessNetworkEvidence) error {
	if err := rec.Validate(); err != nil {
		return err
	}
	return ErrStructuredTransportNotWired
}

func (UnwiredStructuredEvidenceSink) AcceptProcessNetworkWindowAccounting(_ context.Context, acc ProcessNetworkWindowAccounting) error {
	if err := acc.Validate(); err != nil {
		return err
	}
	return ErrStructuredTransportNotWired
}

func (s *LocalRecordingStructuredEvidenceSink) AcceptProcessNetworkEvidence(_ context.Context, rec ProcessNetworkEvidence) error {
	if err := rec.Validate(); err != nil {
		return err
	}
	s.ProcessNetwork = append(s.ProcessNetwork, rec)
	return nil
}

func (s *LocalRecordingStructuredEvidenceSink) AcceptProcessNetworkWindowAccounting(_ context.Context, acc ProcessNetworkWindowAccounting) error {
	if err := acc.Validate(); err != nil {
		return err
	}
	s.ProcessNetworkAccounting = append(s.ProcessNetworkAccounting, acc)
	return nil
}

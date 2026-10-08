package ebpf

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"platform/agent/common/collector"

	"github.com/cilium/ebpf/ringbuf"
)

// ErrBackendStopDeadline is returned when StopContext's caller deadline expires
// before backend workers quiesce. Best-effort resource release still occurs.
var ErrBackendStopDeadline = errors.New("BACKEND_STOP_DEADLINE")

// Backend is the Slice-7 bounded Linux eBPF process-network kernel backend.
// Lifecycle seam for Slice 8: Start / StopContext / Capability.
// Does not redesign AgentService and does not claim physical transport.
type Backend struct {
	mu sync.Mutex

	cfg        Config
	aggregator *collector.ProcessNetworkAggregator
	preflight  *Checker

	handles *kernelHandles
	reader  *ringbuf.Reader
	rawCh   chan []byte
	loss    lossCounters

	cancel       context.CancelFunc
	workerRemain atomic.Int32
	workersDone  chan struct{}

	started           bool
	stopped           bool
	optionalAvailable bool
	capability        collector.ProcessNetworkCapabilitySnapshot
	bootWall          time.Time
	monoBaseNs        uint64

	// Observable Slice-7 runtime notes (not host/network DOWN signals).
	transportNotWired atomic.Uint64
	aggregatorFaults  atomic.Uint64
	readerFaults      atomic.Uint64
}

// BackendConfig extends preflight Config with aggregator wiring.
type BackendConfig struct {
	Config
	Aggregator *collector.ProcessNetworkAggregator
}

// NewBackend constructs an inactive backend. Start performs fail-closed load.
func NewBackend(cfg BackendConfig) (*Backend, error) {
	if cfg.Aggregator == nil {
		return nil, errors.New("BACKEND_AGGREGATOR_REQUIRED")
	}
	if cfg.MaximumBytes == 0 {
		cfg.MaximumBytes = defaultMaximumResourceBytes
	}
	if cfg.RequestedMapBytes == 0 {
		cfg.RequestedMapBytes = RequestedMapBytesEstimate
	}
	if cfg.RequestedRingBytes == 0 {
		cfg.RequestedRingBytes = RequestedRingBytesExact
	}
	if cfg.RequestedMapBytes+cfg.RequestedRingBytes > cfg.MaximumBytes {
		return nil, fmt.Errorf("BACKEND_RESOURCE_CEILING: requested map+ring %d exceeds implementation ceiling %d",
			cfg.RequestedMapBytes+cfg.RequestedRingBytes, cfg.MaximumBytes)
	}
	return &Backend{
		cfg:        cfg.Config,
		aggregator: cfg.Aggregator,
		preflight:  New(cfg.Config),
		capability: collector.ProcessNetworkCapabilitySnapshot{
			State:       collector.ProcessNetworkCapabilityUnavailable,
			ReasonCodes: []collector.ProcessNetworkCapabilityReason{collector.ReasonFeatureDisabled},
			ObservedAt:  time.Now().UTC(),
		},
	}, nil
}

// Capability returns a deep copy of the current capability snapshot.
func (b *Backend) Capability() collector.ProcessNetworkCapabilitySnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.capability.Copy()
}

// Status is an alias for Capability for the Slice-8 lifecycle seam.
func (b *Backend) Status() collector.ProcessNetworkCapabilitySnapshot {
	return b.Capability()
}

// LossSnapshot returns discrete loss counters.
func (b *Backend) LossSnapshot() map[LossClass]uint64 {
	return b.loss.Snapshot()
}

// TransportNotWiredCount reports observed ErrStructuredTransportNotWired
// non-delivery events (Slice-5 truthful boundary; not a kernel/decoder fault).
func (b *Backend) TransportNotWiredCount() uint64 {
	return b.transportNotWired.Load()
}

// AggregatorFaultCount reports local aggregator/runtime faults that degraded the backend.
func (b *Backend) AggregatorFaultCount() uint64 {
	return b.aggregatorFaults.Load()
}

// ReaderFaultCount reports unexpected ring-reader/runtime faults (not DECODER_REJECT).
func (b *Backend) ReaderFaultCount() uint64 {
	return b.readerFaults.Load()
}

// OptionalProbeAttached reports whether the independently degradable retransmit probe is live.
func (b *Backend) OptionalProbeAttached() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.optionalAvailable
}

// Start fail-closes before kernel interaction on object/preflight failure.
// Primary attach failure never becomes ACTIVE. Exactly two goroutines on success.
// No retry loop inside Start.
func (b *Backend) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started && !b.stopped {
		if b.capability.State != collector.ProcessNetworkCapabilityActive {
			return errors.New("BACKEND_RUNTIME_FAILED")
		}
		return nil
	}
	if b.stopped {
		return errors.New("BACKEND_STOPPED")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	snap := b.preflight.RunStartup(ctx, nil)
	b.capability = snap
	if !snap.PreflightPassed {
		// Never ACTIVE on preflight failure.
		if snap.State == collector.ProcessNetworkCapabilityActive {
			snap.State = collector.ProcessNetworkCapabilityUnavailable
			b.capability = snap
		}
		return fmt.Errorf("BACKEND_PREFLIGHT: %s", snap.State)
	}

	spec, _, err := LoadGovernedSpec()
	if err != nil {
		b.capability = b.failCapability(snap, collector.ProcessNetworkCapabilityUnavailable,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonObjectIntegrityFailed})
		return fmt.Errorf("BACKEND_OBJECT: %w", err)
	}

	handles, err, class := acquireKernel(spec)
	if err != nil {
		b.capability = mapAcquireFailure(snap, class)
		return fmt.Errorf("BACKEND_ACQUIRE(%s): %w", class, err)
	}

	rd, err := ringbuf.NewReader(handles.primary.Events)
	if err != nil {
		handles.Close()
		b.capability = b.failCapability(snap, collector.ProcessNetworkCapabilityUnavailable,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonReaderFailed})
		return fmt.Errorf("BACKEND_READER: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	b.handles = handles
	b.reader = rd
	b.rawCh = make(chan []byte, RawRecordChannel)
	b.optionalAvailable = handles.optAttached
	b.bootWall = time.Now().UTC()
	b.monoBaseNs = 0 // decoder falls back to wall clock when unset
	b.started = true
	b.stopped = false
	b.workersDone = make(chan struct{})
	b.workerRemain.Store(int32(BackendGoroutines))

	reasons := []collector.ProcessNetworkCapabilityReason{}
	if !handles.optAttached {
		reasons = append(reasons, collector.ReasonOptionalProbeUnavailable)
	}
	b.capability = b.activeCapability(snap, reasons)

	go b.readLoop(runCtx)
	go b.ingestLoop(runCtx)
	return nil
}

// StopContext is idempotent and releases links/maps/reader/goroutines.
// If workers have not quiesced before the caller deadline, returns
// ErrBackendStopDeadline after best-effort cleanup. Does not spawn a
// Wait-goroutine that can outlive the caller indefinitely.
func (b *Backend) StopContext(ctx context.Context) error {
	b.mu.Lock()
	if !b.started || b.stopped {
		b.stopped = true
		b.mu.Unlock()
		return nil
	}
	cancel := b.cancel
	rd := b.reader
	handles := b.handles
	done := b.workersDone
	b.stopped = true
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if rd != nil {
		_ = rd.Close()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var stopErr error
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			stopErr = fmt.Errorf("%w: %w", ErrBackendStopDeadline, ctx.Err())
		}
	}

	if handles != nil {
		handles.Close()
	}
	b.mu.Lock()
	b.handles = nil
	b.reader = nil
	b.cancel = nil
	if b.capability.State == collector.ProcessNetworkCapabilityActive {
		b.capability.State = collector.ProcessNetworkCapabilityUnavailable
		b.capability.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonBackendStopped}
		b.capability.ObservedAt = time.Now().UTC()
	}
	b.mu.Unlock()
	return stopErr
}

// workerExit marks one backend worker finished. The final worker closes workersDone.
// Must not call StopContext (avoids self-deadlock on the stop waiter).
func (b *Backend) workerExit() {
	if b.workerRemain.Add(-1) == 0 {
		close(b.workersDone)
	}
}

// failRuntime transitions away from ACTIVE for a typed runtime failure, cancels
// the run context, and closes the ring reader to unblock peers. Safe from a
// worker: never waits on workersDone / StopContext.
func (b *Backend) failRuntime(reason collector.ProcessNetworkCapabilityReason) {
	b.mu.Lock()
	if b.capability.State == collector.ProcessNetworkCapabilityActive {
		b.capability.State = collector.ProcessNetworkCapabilityUnavailable
		b.capability.ReasonCodes = []collector.ProcessNetworkCapabilityReason{reason}
		b.capability.ObservedAt = time.Now().UTC()
		b.capability.TransitionAt = b.capability.ObservedAt
		b.capability.PreflightPassed = false
	}
	cancel := b.cancel
	rd := b.reader
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if rd != nil {
		_ = rd.Close()
	}
}

func (b *Backend) activeCapability(base collector.ProcessNetworkCapabilitySnapshot, extra []collector.ProcessNetworkCapabilityReason) collector.ProcessNetworkCapabilitySnapshot {
	base.State = collector.ProcessNetworkCapabilityActive
	base.PreflightPassed = true
	base.ObservedAt = time.Now().UTC()
	base.TransitionAt = base.ObservedAt
	base.LastSuccessAt = base.ObservedAt
	base.ReasonCodes = append([]collector.ProcessNetworkCapabilityReason(nil), extra...)
	base.Resource.RequestedMapBytes = b.cfg.RequestedMapBytes
	base.Resource.RequestedRingBytes = b.cfg.RequestedRingBytes
	base.Resource.MaximumBytes = b.cfg.MaximumBytes
	base.ValidUntil = base.ObservedAt.Add(b.cfg.RecheckMin)
	base.NextRecheckAt = base.ValidUntil
	return base
}

func (b *Backend) failCapability(base collector.ProcessNetworkCapabilitySnapshot, state collector.ProcessNetworkCapabilityState, reasons []collector.ProcessNetworkCapabilityReason) collector.ProcessNetworkCapabilitySnapshot {
	base.State = state
	base.PreflightPassed = false
	base.ReasonCodes = append([]collector.ProcessNetworkCapabilityReason(nil), reasons...)
	base.ObservedAt = time.Now().UTC()
	base.TransitionAt = base.ObservedAt
	base.ValidUntil = base.ObservedAt.Add(b.cfg.RecheckMin)
	base.NextRecheckAt = base.ValidUntil
	return base
}

func mapAcquireFailure(base collector.ProcessNetworkCapabilitySnapshot, class string) collector.ProcessNetworkCapabilitySnapshot {
	now := time.Now().UTC()
	base.ObservedAt = now
	base.TransitionAt = now
	base.PreflightPassed = false
	switch class {
	case "privilege":
		base.State = collector.ProcessNetworkCapabilityPrivilegeDenied
		base.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonPrivilegeDenied}
	case "btf":
		base.State = collector.ProcessNetworkCapabilityKernelIncompatible
		base.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonBTFIncompatible}
	case "verifier":
		base.State = collector.ProcessNetworkCapabilityUnavailable
		base.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonVerifierRejected}
	case "tracepoint_unavailable":
		base.State = collector.ProcessNetworkCapabilityUnavailable
		base.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonTracepointUnavailable}
	case "attach":
		base.State = collector.ProcessNetworkCapabilityUnavailable
		base.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonAttachFailed}
	case "load":
		base.State = collector.ProcessNetworkCapabilityUnavailable
		base.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonLoadFailed}
	case "object_integrity":
		base.State = collector.ProcessNetworkCapabilityUnavailable
		base.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonObjectIntegrityFailed}
	default:
		base.State = collector.ProcessNetworkCapabilityUnavailable
		base.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonLoadFailed}
	}
	return base
}

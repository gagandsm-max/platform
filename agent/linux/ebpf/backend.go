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

// ErrBackendStopDeadline is returned when StopContext's bounded caller deadline
// expires before backend workers quiesce. Kernel handles remain worker-owned
// until the final worker exits; they are never force-closed on this error.
var ErrBackendStopDeadline = errors.New("BACKEND_STOP_DEADLINE")

const defaultBackendStopTimeout = 5 * time.Second

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
	shutdownCtx  context.Context

	readerCloseOnce sync.Once
	finalizeOnce    sync.Once
	closeHandles    func(*kernelHandles)

	started           bool
	stopRequested     bool
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
	initialReasons := []collector.ProcessNetworkCapabilityReason{collector.ReasonFeatureDisabled}
	if cfg.Enabled {
		// Enabled but not yet started: truthful until AgentService activation.
		initialReasons = []collector.ProcessNetworkCapabilityReason{collector.ReasonBackendNotWired}
	}
	now := time.Now().UTC()
	return &Backend{
		cfg:        cfg.Config,
		aggregator: cfg.Aggregator,
		preflight:  New(cfg.Config),
		closeHandles: func(handles *kernelHandles) {
			handles.Close()
		},
		capability: collector.ProcessNetworkCapabilitySnapshot{
			State:        collector.ProcessNetworkCapabilityUnavailable,
			ReasonCodes:  initialReasons,
			ObservedAt:   now,
			TransitionAt: now,
		},
	}, nil
}

// Capability returns a deep copy of the current capability snapshot.
// Runtime-backed capability is current authority: HTTP rendering must not
// age-stale it to EVIDENCE_STALE merely because ValidUntil elapsed.
func (b *Backend) Capability() collector.ProcessNetworkCapabilitySnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.capability.Copy()
	out.CurrentAuthority = true
	return out
}

// Status is an alias for Capability for the Slice-8 lifecycle seam.
func (b *Backend) Status() collector.ProcessNetworkCapabilitySnapshot {
	return b.Capability()
}

// SupportSnapshot returns backend-neutral loss/support counters for the
// loopback capability endpoint. Never a readiness input. Loads typed atomic
// counters directly — no per-call map allocation.
func (b *Backend) SupportSnapshot() collector.ProcessNetworkSupportSnapshot {
	return collector.ProcessNetworkSupportSnapshot{
		Present:               true,
		MapUpdateFail:         b.loss.mapUpdateFail.Load(),
		RingReserveFail:       b.loss.ringReserveFail.Load(),
		DecoderReject:         b.loss.decoderReject.Load(),
		UserspaceChannelFull:  b.loss.userspaceChanFull.Load(),
		TransportNotWired:     b.TransportNotWiredCount(),
		AggregatorFaults:      b.AggregatorFaultCount(),
		ReaderFaults:          b.ReaderFaultCount(),
		OptionalProbeAttached: b.OptionalProbeAttached(),
	}
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

func (b *Backend) stopRequestedLocked() bool {
	return b.stopRequested || b.stopped
}

func (b *Backend) isStopRequested() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stopRequestedLocked()
}

func (b *Backend) publishCapability(snap collector.ProcessNetworkCapabilitySnapshot) {
	b.mu.Lock()
	b.capability = snap
	b.mu.Unlock()
}

func (b *Backend) abortStartCapability(base collector.ProcessNetworkCapabilitySnapshot, err error) {
	reasons := []collector.ProcessNetworkCapabilityReason{collector.ReasonPreflightCancelled}
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		reasons = []collector.ProcessNetworkCapabilityReason{collector.ReasonPreflightTimeout}
	}
	if b.isStopRequested() {
		reasons = []collector.ProcessNetworkCapabilityReason{collector.ReasonBackendStopped}
	}
	b.publishCapability(b.failCapability(base, collector.ProcessNetworkCapabilityUnavailable, reasons))
}

// Start fail-closes before kernel interaction on object/preflight failure.
// Primary attach failure never becomes ACTIVE. Exactly two goroutines on success.
// No retry loop inside Start.
//
// Slice-8 cancellation safety: expensive preflight/load/attach work does not
// hold b.mu for the whole Start, Capability remains readable, and a canceled
// Agent runtime context cannot publish ACTIVE or leak unpublished handles.
func (b *Backend) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	b.mu.Lock()
	if b.stopRequestedLocked() {
		b.mu.Unlock()
		return errors.New("BACKEND_STOPPED")
	}
	if b.started {
		active := b.capability.State == collector.ProcessNetworkCapabilityActive
		b.mu.Unlock()
		if !active {
			return errors.New("BACKEND_RUNTIME_FAILED")
		}
		return nil
	}
	b.mu.Unlock()

	if err := ctx.Err(); err != nil {
		b.abortStartCapability(collector.ProcessNetworkCapabilitySnapshot{}, err)
		return err
	}

	snap := b.preflight.RunStartup(ctx, nil)
	b.publishCapability(snap)
	if b.isStopRequested() {
		b.abortStartCapability(snap, context.Canceled)
		return errors.New("BACKEND_STOPPED")
	}
	if err := ctx.Err(); err != nil {
		b.abortStartCapability(snap, err)
		return err
	}
	if !snap.PreflightPassed {
		// Never ACTIVE on preflight failure.
		if snap.State == collector.ProcessNetworkCapabilityActive {
			snap.State = collector.ProcessNetworkCapabilityUnavailable
			b.publishCapability(snap)
		}
		return fmt.Errorf("BACKEND_PREFLIGHT: %s", snap.State)
	}

	spec, _, err := LoadGovernedSpec()
	if err != nil {
		b.publishCapability(b.failCapability(snap, collector.ProcessNetworkCapabilityUnavailable,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonObjectIntegrityFailed}))
		return fmt.Errorf("BACKEND_OBJECT: %w", err)
	}
	if err := ctx.Err(); err != nil || b.isStopRequested() {
		if err == nil {
			err = context.Canceled
		}
		b.abortStartCapability(snap, err)
		if b.isStopRequested() {
			return errors.New("BACKEND_STOPPED")
		}
		return err
	}

	handles, err, class := acquireKernel(spec)
	if err != nil {
		b.publishCapability(mapAcquireFailure(snap, class))
		return fmt.Errorf("BACKEND_ACQUIRE(%s): %w", class, err)
	}
	if err := ctx.Err(); err != nil || b.isStopRequested() {
		handles.Close()
		if err == nil {
			err = context.Canceled
		}
		b.abortStartCapability(snap, err)
		if b.isStopRequested() {
			return errors.New("BACKEND_STOPPED")
		}
		return err
	}

	rd, err := ringbuf.NewReader(handles.primary.Events)
	if err != nil {
		handles.Close()
		b.publishCapability(b.failCapability(snap, collector.ProcessNetworkCapabilityUnavailable,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonReaderFailed}))
		return fmt.Errorf("BACKEND_READER: %w", err)
	}
	if err := ctx.Err(); err != nil || b.isStopRequested() {
		_ = rd.Close()
		handles.Close()
		if err == nil {
			err = context.Canceled
		}
		b.abortStartCapability(snap, err)
		if b.isStopRequested() {
			return errors.New("BACKEND_STOPPED")
		}
		return err
	}

	b.mu.Lock()
	if b.stopRequestedLocked() || b.started {
		b.mu.Unlock()
		_ = rd.Close()
		handles.Close()
		if b.started {
			return nil
		}
		b.abortStartCapability(snap, context.Canceled)
		return errors.New("BACKEND_STOPPED")
	}
	if err := ctx.Err(); err != nil {
		b.mu.Unlock()
		_ = rd.Close()
		handles.Close()
		b.abortStartCapability(snap, err)
		return err
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
	b.stopRequested = false
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
	b.mu.Unlock()
	return nil
}

// StopContext is idempotent and requests bounded graceful shutdown. The final
// worker owns kernel-handle release, so a stop deadline can never force-close a
// map/program/link that a worker may still dereference. No waiter goroutine is
// spawned.
func (b *Backend) StopContext(ctx context.Context) error {
	stopCtx, stopCancel := boundedBackendStopContext(ctx)
	defer stopCancel()

	b.mu.Lock()
	if !b.started {
		b.stopRequested = true
		b.stopped = true
		b.mu.Unlock()
		return nil
	}
	done := b.workersDone
	if b.stopRequested {
		b.mu.Unlock()
		return waitBackendWorkers(stopCtx, done)
	}
	cancel := b.cancel
	b.stopRequested = true
	b.shutdownCtx = stopCtx
	if b.capability.State == collector.ProcessNetworkCapabilityActive {
		now := time.Now().UTC()
		b.capability.State = collector.ProcessNetworkCapabilityUnavailable
		b.capability.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonBackendStopped}
		b.capability.ObservedAt = now
		b.capability.TransitionAt = now
		b.capability.PreflightPassed = false
	}
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	b.closeReader()
	return waitBackendWorkers(stopCtx, done)
}

func boundedBackendStopContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if _, bounded := parent.Deadline(); bounded {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, defaultBackendStopTimeout)
}

func waitBackendWorkers(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrBackendStopDeadline, ctx.Err())
	}
}

// workerExit marks one backend worker finished. The final worker releases the
// reader and kernel handles exactly once before advertising quiescence.
func (b *Backend) workerExit() {
	if b.workerRemain.Add(-1) == 0 {
		b.finalizeRuntimeResources()
		b.mu.Lock()
		done := b.workersDone
		b.mu.Unlock()
		if done != nil {
			close(done)
		}
	}
}

func (b *Backend) closeReader() {
	b.readerCloseOnce.Do(func() {
		b.mu.Lock()
		rd := b.reader
		b.mu.Unlock()
		if rd != nil {
			_ = rd.Close()
		}
	})
}

func (b *Backend) finalizeRuntimeResources() {
	b.finalizeOnce.Do(func() {
		b.closeReader()
		b.mu.Lock()
		handles := b.handles
		closeHandles := b.closeHandles
		b.handles = nil
		b.reader = nil
		b.cancel = nil
		b.shutdownCtx = nil
		b.optionalAvailable = false
		if b.stopRequested {
			b.stopped = true
		}
		b.mu.Unlock()

		if handles != nil {
			if closeHandles != nil {
				closeHandles(handles)
			} else {
				handles.Close()
			}
		}
	})
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
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	b.closeReader()
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

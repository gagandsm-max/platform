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

// ErrBackendShutdownFlush is returned when a stop-requested shutdown flush could
// not complete within the permitted budget. Pending records must not be reported
// as successfully shut down when the flush failed.
var ErrBackendShutdownFlush = errors.New("BACKEND_SHUTDOWN_FLUSH_INCOMPLETE")

// ErrBackendLossAccountingIncomplete is returned when a final kernel loss
// snapshot could not be established before loss-map resources were released.
var ErrBackendLossAccountingIncomplete = errors.New("BACKEND_LOSS_ACCOUNTING_INCOMPLETE")

// ErrBackendProducerQuiesceIncomplete is returned when kernel probe detachment
// failed or in-flight producer completion could not be proven before final
// loss accounting.
var ErrBackendProducerQuiesceIncomplete = errors.New("BACKEND_PRODUCER_QUIESCE_INCOMPLETE")

// ErrBackendRingDrainIncomplete is returned when pending kernel-ring records
// could not be proven fully drained before the reader was closed or the
// shutdown budget expired.
var ErrBackendRingDrainIncomplete = errors.New("BACKEND_RING_DRAIN_INCOMPLETE")

// ErrBackendFatalRuntime is returned when a reader or aggregator fatal failure
// was retained and later observed by StopContext.
var ErrBackendFatalRuntime = errors.New("BACKEND_FATAL_RUNTIME")

const defaultBackendStopTimeout = 5 * time.Second

// eventReader is the Backend ring consumption seam. Production wraps
// *ringbuf.Reader; tests may inject a deterministic reader to exercise drain
// without live BPF maps. SetDeadline has no error return to match cilium/ebpf.
type eventReader interface {
	SetDeadline(time.Time)
	ReadRaw() ([]byte, error)
	Close() error
}

// ringbufEventReader adapts *ringbuf.Reader to eventReader.
type ringbufEventReader struct {
	rd *ringbuf.Reader
}

func (r *ringbufEventReader) SetDeadline(t time.Time) {
	if r != nil && r.rd != nil {
		r.rd.SetDeadline(t)
	}
}

func (r *ringbufEventReader) ReadRaw() ([]byte, error) {
	if r == nil || r.rd == nil {
		return nil, ringbuf.ErrClosed
	}
	rec, err := r.rd.Read()
	if err != nil {
		return nil, err
	}
	raw := make([]byte, len(rec.RawSample))
	copy(raw, rec.RawSample)
	return raw, nil
}

func (r *ringbufEventReader) Close() error {
	if r == nil || r.rd == nil {
		return nil
	}
	return r.rd.Close()
}

// Backend is the Slice-7 bounded Linux eBPF process-network kernel backend.
// Lifecycle seam for Slice 8: Start / StopContext / Capability.
// Does not redesign AgentService and does not claim physical transport.
type Backend struct {
	mu sync.Mutex

	cfg        Config
	aggregator *collector.ProcessNetworkAggregator
	preflight  *Checker

	handles *kernelHandles
	reader  eventReader
	rawCh   chan []byte
	loss    lossCounters

	cancel       context.CancelFunc
	workerRemain atomic.Int32
	workersDone  chan struct{}
	shutdownCtx  context.Context

	readerCloseOnce  sync.Once
	rawChCloseOnce   sync.Once
	finalizeOnce     sync.Once
	quiesceOnce      sync.Once
	closeHandles     func(*kernelHandles)
	detachProbesFn   func(*kernelHandles) error

	// kernelLossSnapshotFn optionally overrides absolute kernel loss reads
	// (production uses BPF map Lookup). ok=false means no complete snapshot.
	kernelLossSnapshotFn func() (mapUpdate, ringReserve uint64, ok bool)

	started            bool
	stopRequested      bool
	stopped            bool
	optionalAvailable  bool
	producersQuiesced  bool
	capability         collector.ProcessNetworkCapabilitySnapshot
	bootWall           time.Time
	monoBaseNs         uint64
	shutdownFlushErr   error
	fatalRuntimeErr    error
	finalLossDone      bool

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
		detachProbesFn: func(handles *kernelHandles) error {
			return handles.CloseLinks()
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

	// Worker runtime is intentionally detached from the Agent Start/Stop parent
	// context once ACTIVE is committed. Cooperative abort during incomplete
	// activation still observes ctx/stopRequested above. After workers can
	// accept events, only Backend.StopContext may cancel them — and only after
	// installing shutdownCtx — so Agent.Stop cannot recreate the cancel-before-
	// flush race via an open activationDone window.
	runCtx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel
	b.handles = handles
	b.reader = &ringbufEventReader{rd: rd}
	b.rawCh = make(chan []byte, RawRecordChannel)
	b.rawChCloseOnce = sync.Once{}
	b.quiesceOnce = sync.Once{}
	b.readerCloseOnce = sync.Once{}
	b.finalizeOnce = sync.Once{}
	b.optionalAvailable = handles.optAttached
	b.bootWall = time.Now().UTC()
	b.monoBaseNs = 0 // decoder falls back to wall clock when unset
	b.started = true
	b.stopRequested = false
	b.stopped = false
	b.producersQuiesced = false
	b.finalLossDone = false
	b.shutdownFlushErr = nil
	b.fatalRuntimeErr = nil
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

// StopContext is idempotent and requests bounded graceful shutdown.
//
// Ownership: StopContext publishes stopRequested + shutdownCtx, cancels the
// worker runtime, and waits on workersDone under the caller deadline. Potentially
// blocking probe detach (link.Close) and kernel-ring drain run on the owned
// readLoop worker (or ingest when no reader is armed) — never synchronously on
// the Stop caller. Maps/programs/reader remain worker-owned until the final
// worker exits. No waiter goroutine is spawned. A stalled detach cannot block
// AgentService past the optional-enrichment sub-budget; Stop returns a typed
// incomplete/deadline error while cleanup ownership stays with the workers.
func (b *Backend) StopContext(ctx context.Context) error {
	stopCtx, stopCancel := boundedBackendStopContext(ctx)
	defer stopCancel()

	b.mu.Lock()
	if !b.started {
		b.stopRequested = true
		b.stopped = true
		fatalErr := b.fatalRuntimeErr
		flushErr := b.shutdownFlushErr
		b.mu.Unlock()
		return errors.Join(fatalErr, flushErr)
	}
	done := b.workersDone
	if b.stopRequested {
		b.mu.Unlock()
		return b.finishStop(stopCtx, done)
	}
	cancel := b.cancel
	b.stopRequested = true
	b.shutdownCtx = stopCtx
	switch b.capability.State {
	case collector.ProcessNetworkCapabilityActive, collector.ProcessNetworkCapabilityResourceGuarded:
		wasGuarded := b.capability.State == collector.ProcessNetworkCapabilityResourceGuarded ||
			b.capability.Resource.Guarded
		now := time.Now().UTC()
		b.capability.State = collector.ProcessNetworkCapabilityUnavailable
		b.capability.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonBackendStopped}
		b.capability.ObservedAt = now
		b.capability.TransitionAt = now
		b.capability.PreflightPassed = false
		// Stop from RESOURCE_GUARDED retains resource-guard evidence; ACTIVE stop
		// keeps Guarded=false so state and embedded flag stay consistent.
		b.capability.Resource.Guarded = wasGuarded
	}
	b.mu.Unlock()

	// Wake workers into the owned shutdown path. Do not call quiesceKernelProducers
	// or closeReader here: link.Close / Reader.Close may stall, and forcing either
	// on the Stop caller would starve protected Agent teardown or race workers
	// still using those FDs. Worker-owned shutdownReadPath / finalizeRuntimeResources
	// perform detach, drain, and release.
	if cancel != nil {
		cancel()
	}
	// When no reader is armed, readLoop is not the sole rawCh producer — Stop
	// owns channel close so ingest can observe producer completion and run the
	// quiesce+flush path. Production Start always arms a reader; this branch is
	// the ownership complement for ingest-only fixtures.
	if !b.readerArmed() {
		b.closeRawCh()
	}
	return b.finishStop(stopCtx, done)
}

func (b *Backend) finishStop(stopCtx context.Context, done <-chan struct{}) error {
	waitErr := waitBackendWorkers(stopCtx, done)
	b.mu.Lock()
	flushErr := b.shutdownFlushErr
	fatalErr := b.fatalRuntimeErr
	b.mu.Unlock()
	joined := errors.Join(fatalErr, flushErr, waitErr)
	if joined == nil {
		return nil
	}
	// Flush budget exhaustion is a stop-deadline class failure with explicit
	// incomplete-flush evidence (never a silent successful Stop).
	if flushErr != nil && (errors.Is(flushErr, context.DeadlineExceeded) ||
		(errors.Is(flushErr, context.Canceled) && errors.Is(stopCtx.Err(), context.DeadlineExceeded))) {
		return errors.Join(ErrBackendStopDeadline, ErrBackendShutdownFlush, joined)
	}
	if fatalErr != nil && !errors.Is(joined, ErrBackendFatalRuntime) {
		joined = errors.Join(ErrBackendFatalRuntime, joined)
	}
	return joined
}

// quiesceKernelProducers detaches probe links exactly once so new kernel events
// cannot be produced, while leaving maps/programs valid for drain and final
// loss snapshots. Concurrent/duplicate calls are safe.
func (b *Backend) quiesceKernelProducers() error {
	var detachErr error
	b.quiesceOnce.Do(func() {
		b.mu.Lock()
		handles := b.handles
		detachFn := b.detachProbesFn
		b.mu.Unlock()
		if handles == nil {
			b.mu.Lock()
			b.producersQuiesced = true
			b.optionalAvailable = false
			b.mu.Unlock()
			return
		}
		if detachFn != nil {
			detachErr = detachFn(handles)
		} else {
			detachErr = handles.CloseLinks()
		}
		b.mu.Lock()
		b.producersQuiesced = detachErr == nil
		b.optionalAvailable = false
		b.mu.Unlock()
		if detachErr != nil {
			b.noteShutdownEvidence(fmt.Errorf("%w: %v", ErrBackendProducerQuiesceIncomplete, detachErr))
		}
	})
	b.mu.Lock()
	quiesced := b.producersQuiesced
	b.mu.Unlock()
	if !quiesced && detachErr == nil {
		return ErrBackendProducerQuiesceIncomplete
	}
	return detachErr
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
		err := ctx.Err()
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %w", ErrBackendStopDeadline, err)
		}
		// Explicit cancellation remains cancellation — never mislabeled as a
		// stop-deadline exhaustion.
		return err
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

func (b *Backend) readerArmed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reader != nil
}

// closeRawCh closes the sole-producer raw record channel exactly once.
// Only readLoop may call this; ingestLoop never closes rawCh.
func (b *Backend) closeRawCh() {
	b.rawChCloseOnce.Do(func() {
		b.mu.Lock()
		ch := b.rawCh
		b.mu.Unlock()
		if ch != nil {
			close(ch)
		}
	})
}

func (b *Backend) finalizeRuntimeResources() {
	b.finalizeOnce.Do(func() {
		// Ensure producers are quiesced before any last-chance loss snapshot.
		_ = b.quiesceKernelProducers()
		b.closeReader()
		// Last-chance kernel loss reconcile while maps remain valid. Prefer the
		// ingest shutdown path; this covers fatal exits that skipped it.
		b.reconcileFinalKernelLoss(context.Background(), false)
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

// failRuntime transitions away from ACTIVE or RESOURCE_GUARDED for a typed
// runtime failure, retains fatal evidence independently of Stop, cancels the
// run context, and closes the ring reader to unblock peers. Safe from a worker:
// never waits on workersDone / StopContext. Fatal reader/aggregator failures
// retain precedence over a latched guard and over a later BACKEND_STOPPED stop.
func (b *Backend) failRuntime(reason collector.ProcessNetworkCapabilityReason) {
	b.mu.Lock()
	switch b.capability.State {
	case collector.ProcessNetworkCapabilityActive, collector.ProcessNetworkCapabilityResourceGuarded:
		wasGuarded := b.capability.State == collector.ProcessNetworkCapabilityResourceGuarded ||
			b.capability.Resource.Guarded
		b.capability.State = collector.ProcessNetworkCapabilityUnavailable
		b.capability.ReasonCodes = []collector.ProcessNetworkCapabilityReason{reason}
		b.capability.ObservedAt = time.Now().UTC()
		b.capability.TransitionAt = b.capability.ObservedAt
		b.capability.PreflightPassed = false
		// Fatal override of RESOURCE_GUARDED: state is no longer guarded, but
		// retain resource admission evidence for supportability.
		b.capability.Resource.Guarded = wasGuarded
	}
	b.fatalRuntimeErr = errors.Join(b.fatalRuntimeErr,
		fmt.Errorf("%w: %s", ErrBackendFatalRuntime, reason))
	cancel := b.cancel
	armed := b.reader != nil
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	b.closeReader()
	// When no reader was armed, readLoop is not the sole rawCh producer and has
	// already exited without closing the channel. Close it so ingest can finish
	// shutdown drain after a fatal cancel.
	if !armed {
		b.closeRawCh()
	}
}

// guardCapabilityForKernelLoss latches ACTIVE → RESOURCE_GUARDED with
// RESOURCE_LIMIT when positive reconciled kernel map/ring loss is observed.
// Guard remains latched until a governed healthy restart; zero additional loss
// must not fabricate automatic recovery to ACTIVE. Embedded Resource.Guarded
// must agree with State=RESOURCE_GUARDED.
func (b *Backend) guardCapabilityForKernelLoss() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.capability.State != collector.ProcessNetworkCapabilityActive {
		return
	}
	now := time.Now().UTC()
	b.capability.State = collector.ProcessNetworkCapabilityResourceGuarded
	b.capability.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonResourceLimit}
	b.capability.ObservedAt = now
	b.capability.TransitionAt = now
	b.capability.Resource.RequestedMapBytes = b.cfg.RequestedMapBytes
	b.capability.Resource.RequestedRingBytes = b.cfg.RequestedRingBytes
	b.capability.Resource.MaximumBytes = b.cfg.MaximumBytes
	b.capability.Resource.Guarded = true
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
	base.Resource.Guarded = false
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

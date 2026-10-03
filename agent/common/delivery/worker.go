package delivery

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/queue"
)

type DeliveryQueue interface {
	ReadBatch(maxRecords int, maxBytes int64) ([]queue.Record, error)
	CommitAccepted(records []queue.Record, acceptedThrough uint64) error
	Len() int
}

// lineageQueue is optional; DurableQueue implements it. Test fakes may omit it and
// remain on legacy v1 batch identity.
type lineageQueue interface {
	AgentStreamID() string
	LegacyCutoverSequence() uint64
}

type DeliveryStats struct {
	Attempts          int64
	Successes         int64
	LastAttemptTime   time.Time
	LastSuccessTime   time.Time
	LastFailureClass  client.FailureClass
	LastErrorCode     string
	ReceiverAvailable bool
	RetryAttempt      int
}

type Worker struct {
	q               DeliveryQueue
	transport       client.Transport
	agentID         string
	maxBatchRecords int
	maxBatchBytes   int64
	retryBase       time.Duration
	retryMax        time.Duration
	deliveryTimeout time.Duration
	logger          *slog.Logger
	mu              sync.Mutex
	stats           DeliveryStats
	stopCh          chan struct{}
	stopOnce        sync.Once
	stopDone        chan struct{} // closed exactly once after wg.Wait completes
	startOnce       sync.Once
	wg              sync.WaitGroup

	// classifyBarrier, when set by tests, runs after Deliver returns and before
	// failure/shutdown classification. Production remains nil.
	classifyBarrier func()
}

func NewWorker(
	q DeliveryQueue,
	tr client.Transport,
	agentID string,
	maxBatchRecords int,
	maxBatchBytes int64,
	retryBase time.Duration,
	retryMax time.Duration,
	deliveryTimeout time.Duration,
	logger *slog.Logger,
) (*Worker, error) {
	if q == nil {
		return nil, errors.New("DELIVERY_QUEUE_REQUIRED: queue must be provided")
	}
	if tr == nil {
		return nil, errors.New("DELIVERY_TRANSPORT_REQUIRED: transport must be provided")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errors.New("DELIVERY_AGENT_ID_REQUIRED: worker agent ID must be provided")
	}
	if maxBatchRecords <= 0 {
		maxBatchRecords = 100
	}
	if maxBatchBytes <= 0 {
		maxBatchBytes = 384 * 1024
	}
	if retryBase <= 0 {
		retryBase = time.Second
	}
	if retryMax <= 0 {
		retryMax = 60 * time.Second
	}
	if deliveryTimeout <= 0 {
		deliveryTimeout = 10 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}

	w := &Worker{
		q:               q,
		transport:       tr,
		agentID:         agentID,
		maxBatchRecords: maxBatchRecords,
		maxBatchBytes:   maxBatchBytes,
		retryBase:       retryBase,
		retryMax:        retryMax,
		deliveryTimeout: deliveryTimeout,
		logger:          logger,
		stopCh:          make(chan struct{}),
		stopDone:        make(chan struct{}),
	}
	w.stats.ReceiverAvailable = tr.Available()
	return w, nil
}

func (w *Worker) Start(ctx context.Context) {
	w.startOnce.Do(func() {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.runLoop(ctx)
		}()
	})
}

func (w *Worker) runLoop(ctx context.Context) {
	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		delivered, err := w.DeliverOnce(ctx)
		if err != nil {
			if errors.Is(err, errDeliveryAbortedByShutdown) {
				return
			}
			failClass := client.Classify(err)
			if failClass == client.FailureOversized || failClass == client.FailureBlocked {
				code := w.Stats().LastErrorCode
				if code == "" {
					code = "AGENT_DELIVERY_NOT_RETRYABLE"
				}
				w.logger.Error("Agent delivery is not retryable without operator intervention; queue retained",
					"event_code", code,
					"result", "RETAINED",
					"safe_context", map[string]any{
						"failure_class": string(failClass),
						"reason_class":  "not_retryable",
					},
				)
				select {
				case <-w.stopCh:
					return
				case <-ctx.Done():
					return
				}
			}

			w.mu.Lock()
			attempt := w.stats.RetryAttempt
			w.mu.Unlock()

			backoffDuration := client.Backoff(w.retryBase, w.retryMax, attempt, randomJitter())
			timer := time.NewTimer(backoffDuration)
			select {
			case <-timer.C:
			case <-w.stopCh:
				timer.Stop()
				return
			case <-ctx.Done():
				timer.Stop()
				return
			}
			continue
		}

		if !delivered {
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-timer.C:
			case <-w.stopCh:
				timer.Stop()
				return
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}
}

// DeliverOnce attempts to read a batch from queue and deliver it.
// Returns (delivered=true, err=nil) on success, (false, nil) if queue is empty,
// and (false, err) if delivery failed. Transport/auth/permanent errors never delete.
func (w *Worker) DeliverOnce(ctx context.Context) (bool, error) {
	records, err := w.q.ReadBatch(w.maxBatchRecords, w.maxBatchBytes)
	if err != nil {
		failClass := client.Classify(err)
		code := queueFailureCode(err)
		w.recordQueueFailure(failClass, code)
		w.logger.Error("Failed to read batch from queue",
			"event_code", code,
			"result", "FAILED",
			"safe_context", map[string]any{
				"failure_class": string(failClass),
				"reason_class":  "queue_read_failed",
			},
		)
		return false, err
	}

	if len(records) == 0 {
		return false, nil
	}

	streamID := ""
	cutover := uint64(0)
	if lq, ok := w.q.(lineageQueue); ok {
		streamID = lq.AgentStreamID()
		cutover = lq.LegacyCutoverSequence()
	}

	if cutover > 0 {
		firstLegacy := cutover > 0 && records[0].Sequence < cutover
		for i := 1; i < len(records); i++ {
			leg := records[i].Sequence < cutover
			if leg != firstLegacy {
				w.recordFailure(client.FailurePermanent, "DELIVERY_MIXED_CUTOVER_BATCH")
				return false, errors.New("DELIVERY_MIXED_CUTOVER_BATCH: batch must not span legacy cutover")
			}
		}
	}

	// Optional lineage: missing stream falls back to legacy v1 (test fakes).
	// DurableQueue always has a stream; cutover>0 + seq<cutover => legacy wire.
	legacy := streamID == "" || (cutover > 0 && records[0].Sequence < cutover)

	var batchID string
	schemaVersion := 1
	agentStreamID := ""
	if legacy {
		batchID = client.BuildLegacyBatchID(w.agentID, records)
	} else {
		if streamID == "" {
			w.recordFailure(client.FailurePermanent, "DELIVERY_STREAM_ID_REQUIRED")
			return false, errors.New("DELIVERY_STREAM_ID_REQUIRED: modern batch requires agentStreamId")
		}
		batchID = client.BuildLineageBatchID(streamID, w.agentID, records)
		schemaVersion = 2
		agentStreamID = streamID
	}
	if batchID == "" {
		w.recordFailure(client.FailurePermanent, "DELIVERY_BATCH_ID_FAILED")
		return false, errors.New("DELIVERY_BATCH_ID_FAILED: unable to compute batch identity")
	}

	// Each HTTP attempt gets a fresh correlation ID; batch ID stays stable across retries.
	corrID, genErr := client.CorrelationID()
	if genErr != nil {
		w.recordFailure(client.FailureTemporary, "DELIVERY_CORRELATION_ID_FAILED")
		return false, genErr
	}

	batch := client.Batch{
		CorrelationID: corrID,
		AgentID:       w.agentID,
		BatchID:       batchID,
		SchemaVersion: schemaVersion,
		AgentStreamID: agentStreamID,
		Records:       records,
	}

	w.mu.Lock()
	w.stats.Attempts++
	w.stats.LastAttemptTime = time.Now().UTC()
	w.stats.ReceiverAvailable = w.transport.Available()
	w.mu.Unlock()

	// WithTimeout preserves immutable Cause: DeadlineExceeded if the delivery
	// attempt deadline wins first; Canceled if parent/service cancellation wins first.
	delivCtx, cancel := context.WithTimeout(ctx, w.deliveryTimeout)
	defer cancel()

	ack, err := w.transport.Deliver(delivCtx, batch)
	delivCause := context.Cause(delivCtx)
	if w.classifyBarrier != nil {
		w.classifyBarrier()
	}
	if err != nil {
		// Classify from the attempt's Cause order — not from post-completion stopCh/parent state.
		if isExpectedShutdownDeliveryErr(delivCause, err) {
			return false, errDeliveryAbortedByShutdown
		}
		failClass := client.Classify(err)
		var de *client.DeliveryError
		code := ""
		if errors.As(err, &de) {
			code = de.Code
		}
		w.recordFailure(failClass, code)
		eventCode := code
		if eventCode == "" {
			eventCode = "AGENT_DELIVERY_ATTEMPT_FAILED"
		}
		safe := map[string]any{
			"failure_class": string(failClass),
			"record_count":  len(records),
			"reason_class":  "delivery_attempt_failed",
		}
		if schemaVersion == 2 && agentStreamID != "" {
			safe["agent_stream_configured"] = true
		}
		w.logger.Debug("Delivery attempt failed",
			"event_code", eventCode,
			"result", "FAILED",
			"safe_context", safe,
		)
		return false, err
	}

	if err := validateAck(records, w.agentID, batchID, corrID, ack); err != nil {
		w.recordFailure(client.FailurePermanent, "DELIVERY_ACK_INVALID")
		w.logger.Error("Invalid delivery ACK rejected; queue retained",
			"event_code", "DELIVERY_ACK_INVALID",
			"correlation_id", corrID,
			"result", "RETAINED",
			"safe_context", map[string]any{"reason_class": "ack_invalid"},
		)
		return false, err
	}

	if err := w.q.CommitAccepted(records, ack.AcceptedThrough); err != nil {
		failClass := client.Classify(err)
		code := "DELIVERY_COMMIT_FAILED"
		if failClass == client.FailureBlocked {
			code = queueFailureCode(err)
		}
		w.recordQueueFailure(failClass, code)
		w.logger.Error("Failed to commit acknowledged records in queue",
			"event_code", code,
			"correlation_id", corrID,
			"result", "FAILED",
			"safe_context", map[string]any{
				"failure_class":     string(failClass),
				"accepted_through":  ack.AcceptedThrough,
				"reason_class":      "commit_failed",
			},
		)
		return false, err
	}

	w.mu.Lock()
	w.stats.Successes++
	w.stats.LastSuccessTime = time.Now().UTC()
	w.stats.RetryAttempt = 0
	w.stats.LastFailureClass = ""
	w.stats.LastErrorCode = ""
	w.stats.ReceiverAvailable = w.transport.Available()
	w.mu.Unlock()

	ackSafe := map[string]any{
		"record_count":     len(records),
		"accepted_through": ack.AcceptedThrough,
	}
	if schemaVersion == 2 && agentStreamID != "" {
		ackSafe["agent_stream_configured"] = true
	}
	w.logger.Debug("Delivery batch acknowledged",
		"event_code", "AGENT_DELIVERY_OK",
		"correlation_id", corrID,
		"result", "ACCEPTED",
		"safe_context", ackSafe,
	)
	return true, nil
}

func validateAck(records []queue.Record, expectedAgentID, expectedBatchID, expectedCorrID string, ack client.Ack) error {
	if ack.AcceptedThrough == 0 {
		return errors.New("DELIVERY_ACK_EMPTY: empty ACK does not authorize deletion")
	}
	if strings.TrimSpace(ack.BatchID) == "" {
		return errors.New("DELIVERY_ACK_MISSING_BATCH_ID: batch-bound ACK required before deletion")
	}
	if strings.TrimSpace(ack.AgentID) == "" {
		return errors.New("DELIVERY_ACK_MISSING_AGENT_ID: agent-bound ACK required before deletion")
	}
	if strings.TrimSpace(ack.CorrelationID) == "" {
		return errors.New("DELIVERY_ACK_MISSING_CORRELATION_ID: correlation-bound ACK required before deletion")
	}
	if ack.BatchID != expectedBatchID {
		return fmt.Errorf("DELIVERY_ACK_BATCH_MISMATCH: ack batch_id %q does not match submitted %q", ack.BatchID, expectedBatchID)
	}
	if ack.AgentID != expectedAgentID {
		return fmt.Errorf("DELIVERY_ACK_AGENT_MISMATCH: ack agent_id %q does not match submitted %q", ack.AgentID, expectedAgentID)
	}
	if ack.CorrelationID != expectedCorrID {
		return fmt.Errorf("DELIVERY_ACK_CORRELATION_MISMATCH: ack correlation_id %q does not match submitted %q", ack.CorrelationID, expectedCorrID)
	}
	if len(records) == 0 {
		return errors.New("DELIVERY_ACK_EMPTY_BATCH: ACK without submitted records")
	}

	first := records[0].Sequence
	last := records[len(records)-1].Sequence
	if ack.AcceptedThrough < first {
		return fmt.Errorf("DELIVERY_ACK_STALE: acceptedThrough %d precedes submitted batch starting at %d", ack.AcceptedThrough, first)
	}
	if ack.AcceptedThrough > last {
		return fmt.Errorf("DELIVERY_ACK_BEYOND_BATCH: acceptedThrough %d exceeds last submitted sequence %d", ack.AcceptedThrough, last)
	}

	found := false
	for _, r := range records {
		if r.Sequence == ack.AcceptedThrough {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("DELIVERY_ACK_NOT_IN_BATCH: acceptedThrough %d was not in the submitted batch", ack.AcceptedThrough)
	}
	return nil
}

func (w *Worker) recordFailure(class client.FailureClass, code string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stats.RetryAttempt < math.MaxInt-1 {
		w.stats.RetryAttempt++
	}
	w.stats.LastFailureClass = class
	w.stats.LastErrorCode = code
	// Last delivery failed: do not report the Sensor receiver as available
	// merely because the HTTPS client was constructed.
	w.stats.ReceiverAvailable = false
}

func (w *Worker) recordQueueFailure(class client.FailureClass, code string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stats.RetryAttempt < math.MaxInt-1 {
		w.stats.RetryAttempt++
	}
	w.stats.LastFailureClass = class
	w.stats.LastErrorCode = code
	// Local queue failure: do not mutate ReceiverAvailable.
}

func queueFailureCode(err error) string {
	switch {
	case errors.Is(err, queue.ErrDeliveryBlocked):
		return "QUEUE_DELIVERY_BLOCKED"
	case errors.Is(err, queue.ErrQuarantineFull):
		return "QUEUE_QUARANTINE_FULL"
	case errors.Is(err, queue.ErrQuarantineDestExists):
		return "QUEUE_QUARANTINE_DEST_EXISTS"
	case errors.Is(err, queue.ErrUnknownSequence):
		return "QUEUE_UNKNOWN_SEQUENCE_BLOCK"
	case errors.Is(err, queue.ErrQuarantineUncertain):
		return "QUEUE_QUARANTINE_UNCERTAIN"
	case errors.Is(err, queue.ErrQuarantineSourceSyncFailed):
		return "QUEUE_QUARANTINE_SOURCE_SYNC_FAILED"
	case errors.Is(err, queue.ErrSequenceGap):
		return "QUEUE_SEQUENCE_GAP"
	case errors.Is(err, queue.ErrSequenceConflict):
		return "QUEUE_SEQUENCE_CONFLICT"
	case errors.Is(err, queue.ErrMetaUncertain):
		return "QUEUE_META_UNCERTAIN"
	case errors.Is(err, queue.ErrAckRecoveryConflict):
		return "QUEUE_ACK_RECOVERY_CONFLICT"
	case errors.Is(err, queue.ErrPendingAckCleanup):
		return "QUEUE_PENDING_ACK_CLEANUP"
	case errors.Is(err, queue.ErrSequenceExhausted):
		return "QUEUE_SEQUENCE_EXHAUSTED"
	default:
		return "QUEUE_READ_FAILED"
	}
}

func randomJitter() float64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	// Map to [-1, 1)
	u := binary.BigEndian.Uint64(b[:]) >> 11 // 53 bits
	f := float64(u) / (1 << 53)
	return f*2 - 1
}

// errDeliveryAbortedByShutdown is returned when parent/service cancellation won
// the attempt context race first. It must not mutate retry/failure stats.
var errDeliveryAbortedByShutdown = errors.New("DELIVERY_ABORTED_BY_SHUTDOWN")

func isExpectedShutdownDeliveryErr(delivCause error, err error) bool {
	if err == nil {
		return false
	}
	// Real per-attempt deadline won first — genuine failure even if Stop follows.
	if errors.Is(delivCause, context.DeadlineExceeded) {
		return false
	}
	// Parent/service cancellation won first.
	if !errors.Is(delivCause, context.Canceled) {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var de *client.DeliveryError
	if errors.As(err, &de) && de.Code == "DELIVERY_CONTEXT_ABORTED" {
		return true
	}
	return false
}

// SetClassifyBarrierForTest installs a hook invoked after Deliver returns and
// before classification. Production callers must leave this unset.
func (w *Worker) SetClassifyBarrierForTest(fn func()) {
	w.classifyBarrier = fn
}

func (w *Worker) beginStop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		go func() {
			w.wg.Wait()
			close(w.stopDone)
		}()
	})
}

// StopContext requests delivery-loop shutdown and waits until the loop exits or ctx ends.
// Exactly one waiter coordinator is started; repeated calls share stopDone.
func (w *Worker) StopContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	w.beginStop()
	select {
	case <-w.stopDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("DELIVERY_STOP_TIMEOUT: %w", ctx.Err())
	}
}

// Stop is a compatibility wrapper that waits without an external deadline.
func (w *Worker) Stop() {
	_ = w.StopContext(context.Background())
}

func (w *Worker) Stats() DeliveryStats {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stats
}

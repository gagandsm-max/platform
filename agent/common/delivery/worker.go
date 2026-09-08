package delivery

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/queue"
)

type DeliveryQueue interface {
	ReadBatch(maxRecords int, maxBytes int64) ([]queue.Record, error)
	DeleteThrough(seq uint64) error
	Len() int
}

type DeliveryStats struct {
	Attempts         int64
	Successes        int64
	LastAttemptTime  time.Time
	LastSuccessTime  time.Time
	LastFailureClass client.FailureClass
	LastErrorCode    string
	ReceiverAvailable bool
	RetryAttempt     int
}

type Worker struct {
	q               DeliveryQueue
	transport       client.Transport
	maxBatchRecords int
	maxBatchBytes   int64
	retryBase       time.Duration
	retryMax        time.Duration
	deliveryTimeout time.Duration
	logger          *slog.Logger
	mu              sync.Mutex
	stats           DeliveryStats
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

func NewWorker(
	q DeliveryQueue,
	tr client.Transport,
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
	if maxBatchRecords <= 0 {
		maxBatchRecords = 100
	}
	if maxBatchBytes <= 0 {
		maxBatchBytes = 1024 * 1024
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
		maxBatchRecords: maxBatchRecords,
		maxBatchBytes:   maxBatchBytes,
		retryBase:       retryBase,
		retryMax:        retryMax,
		deliveryTimeout: deliveryTimeout,
		logger:          logger,
		stopCh:          make(chan struct{}),
	}
	w.stats.ReceiverAvailable = tr.Available()
	return w, nil
}

func (w *Worker) Start(ctx context.Context) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.runLoop(ctx)
	}()
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
			// Backoff sleep on error
			w.mu.Lock()
			attempt := w.stats.RetryAttempt
			w.mu.Unlock()

			backoffDuration := client.Backoff(w.retryBase, w.retryMax, attempt, 0)
			select {
			case <-time.After(backoffDuration):
			case <-w.stopCh:
				return
			case <-ctx.Done():
				return
			}
			continue
		}

		if !delivered {
			// No records in queue; idle wait
			select {
			case <-time.After(500 * time.Millisecond):
			case <-w.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}
}

// DeliverOnce attempts to read a batch from queue and deliver it.
// Returns (delivered=true, err=nil) on success, (false, nil) if queue is empty,
// and (false, err) if delivery failed.
func (w *Worker) DeliverOnce(ctx context.Context) (bool, error) {
	records, err := w.q.ReadBatch(w.maxBatchRecords, w.maxBatchBytes)
	if err != nil {
		w.logger.Error("Failed to read batch from queue", "error", err)
		return false, err
	}

	if len(records) == 0 {
		return false, nil
	}

	corrID, err := client.CorrelationID()
	if err != nil {
		corrID = "corr-fallback"
	}

	batch := client.Batch{
		CorrelationID: corrID,
		Records:       records,
	}

	w.mu.Lock()
	w.stats.Attempts++
	w.stats.LastAttemptTime = time.Now().UTC()
	w.stats.ReceiverAvailable = w.transport.Available()
	w.mu.Unlock()

	delivCtx, cancel := context.WithTimeout(ctx, w.deliveryTimeout)
	defer cancel()

	ack, err := w.transport.Deliver(delivCtx, batch)
	if err != nil {
		failClass := client.Classify(err)
		var de *client.DeliveryError
		code := ""
		if errors.As(err, &de) {
			code = de.Code
		}

		w.mu.Lock()
		w.stats.RetryAttempt++
		w.stats.LastFailureClass = failClass
		w.stats.LastErrorCode = code
		w.stats.ReceiverAvailable = w.transport.Available()
		w.mu.Unlock()

		w.logger.Debug("Delivery attempt failed",
			"class", failClass,
			"code", code,
			"records", len(records),
			"error", err,
		)
		return false, err
	}

	// ACK received: delete acknowledged records from queue
	if ack.AcceptedThrough > 0 {
		if err := w.q.DeleteThrough(ack.AcceptedThrough); err != nil {
			w.logger.Error("Failed to delete acknowledged records from queue", "through", ack.AcceptedThrough, "error", err)
		}
	}

	w.mu.Lock()
	w.stats.Successes++
	w.stats.LastSuccessTime = time.Now().UTC()
	w.stats.RetryAttempt = 0
	w.stats.LastFailureClass = ""
	w.stats.LastErrorCode = ""
	w.stats.ReceiverAvailable = w.transport.Available()
	w.mu.Unlock()

	w.logger.Debug("Delivery batch acknowledged", "records", len(records), "acceptedThrough", ack.AcceptedThrough)
	return true, nil
}

func (w *Worker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

func (w *Worker) Stats() DeliveryStats {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stats
}

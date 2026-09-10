package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"platform/sensor/internal/observation"
)

type CollectorEngine struct {
	sensorID    string
	sensorType  string
	sink        observation.Sink
	interval    time.Duration
	logger      *slog.Logger
	stopCh      chan struct{}
	wg          sync.WaitGroup
	itemCounter atomic.Uint64
}

func NewCollectorEngine(
	sensorID, sensorType string,
	sink observation.Sink,
	interval time.Duration,
	logger *slog.Logger,
) *CollectorEngine {
	return &CollectorEngine{
		sensorID:   sensorID,
		sensorType: sensorType,
		sink:       sink,
		interval:   interval,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

func (e *CollectorEngine) Start(ctx context.Context) {
	e.logger.Info("Starting collector engine loop", "sensor_id", e.sensorID, "type", e.sensorType)

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				e.collectAndSpool(ctx)
			case <-e.stopCh:
				e.logger.Info("Collector harvest loop exiting")
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (e *CollectorEngine) collectAndSpool(ctx context.Context) {
	seq := e.itemCounter.Add(1)
	correlationID := fmt.Sprintf("%s-%d", e.sensorID, time.Now().UnixNano())

	payloadData := map[string]any{
		"metric_id": "sensor.heartbeat",
		"value":     1.0,
		"status":    "HEALTHY",
		"sequence":  seq,
	}
	payloadBytes, err := json.Marshal(payloadData)
	if err != nil {
		e.logger.Error("Failed to marshal collector payload", "error", err)
		return
	}

	env := observation.Envelope{
		SchemaVersion:   observation.SchemaVersion,
		BatchID:         fmt.Sprintf("batch-%s-%d", e.sensorID, time.Now().UnixNano()),
		RecordType:      "TELEMETRY_SAMPLE",
		CollectedAt:     time.Now().UTC(),
		SensorID:        e.sensorID,
		TenantID:        "default",
		EntityID:        e.sensorID,
		Job: observation.JobCorrelation{
			JobID:             fmt.Sprintf("job-%s-heartbeat", e.sensorID),
			AssignmentKey:     "assignment-collector",
			AssignmentVersion: 1,
			Attempt:           1,
		},
		Protocol:              "SENSOR",
		ProtocolVersion:       "1.0",
		Operation:             "COLLECT",
		Status:                "SUCCESS",
		Quality:               "GOOD",
		ProtocolSchemaVersion: 1,
		ProtocolPayload:       payloadBytes,
	}

	receipt, err := e.sink.Write(ctx, env)
	if err != nil {
		e.logger.Error("Failed to write observation to sink", "correlation_id", correlationID, "error", err)
	} else {
		e.logger.Debug("Observation written to durable sink", "correlation_id", correlationID, "sequence", receipt.Sequence, "record_id", receipt.RecordID)
	}
}

func (e *CollectorEngine) Stop() {
	e.logger.Info("Signaling collector engine to stop")
	close(e.stopCh)
	e.wg.Wait()
	e.logger.Info("Collector engine stopped cleanly")
}

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/spool"
)

type CollectorEngine struct {
	sensorID    string
	sensorType  string
	spool       *spool.DiskSpool
	client      *coreclient.CoreClient
	interval    time.Duration
	logger      *slog.Logger
	stopCh      chan struct{}
	wg          sync.WaitGroup
	itemCounter atomic.Uint64
}

func NewCollectorEngine(
	sensorID, sensorType string,
	diskSpool *spool.DiskSpool,
	client *coreclient.CoreClient,
	interval time.Duration,
	logger *slog.Logger,
) *CollectorEngine {
	return &CollectorEngine{
		sensorID:   sensorID,
		sensorType: sensorType,
		spool:      diskSpool,
		client:     client,
		interval:   interval,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

func (e *CollectorEngine) Start(ctx context.Context) {
	e.logger.Info("Starting collector engine loops", "sensor_id", e.sensorID, "type", e.sensorType)

	// Loop 1: Telemetry Generator & Spooler
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				e.collectAndSpool()
			case <-e.stopCh:
				e.logger.Info("Collector harvest loop exiting")
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Loop 2: Spool Forwarder / Drainer
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		drainTicker := time.NewTicker(5 * time.Second)
		defer drainTicker.Stop()

		for {
			select {
			case <-drainTicker.C:
				e.drainSpool(ctx)
			case <-e.stopCh:
				e.logger.Info("Collector spool drainer exiting")
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (e *CollectorEngine) collectAndSpool() {
	seq := e.itemCounter.Add(1)
	correlationID := fmt.Sprintf("%s-%d", e.sensorID, time.Now().UnixNano())

	payloadData := map[string]any{
		"sequence":    seq,
		"sensor_type": e.sensorType,
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
		"status":      "HEALTHY",
	}
	payloadBytes, _ := json.Marshal(payloadData)

	item := &spool.SpoolItem{
		ID:        fmt.Sprintf("item-%d", seq),
		SensorID:  e.sensorID,
		Timestamp: time.Now().Unix(),
		Type:      e.sensorType,
		Payload:   payloadBytes,
	}

	if err := e.spool.Write(item); err != nil {
		e.logger.Error("Failed to spool item", "correlation_id", correlationID, "error", err)
	} else {
		e.logger.Debug("Item successfully spooled", "correlation_id", correlationID, "item_id", item.ID)
	}
}

func (e *CollectorEngine) drainSpool(ctx context.Context) {
	files, err := e.spool.ListPendingFiles(5)
	if err != nil {
		e.logger.Warn("Failed to list spool files", "error", err)
		return
	}

	for _, file := range files {
		// In dev skeleton, verify Core connectivity before unlinking or draining
		connected, err := e.client.CheckConnectivity(ctx)
		if err != nil || !connected {
			e.logger.Debug("Core not connected; retaining spooled telemetry", "pending_file", file)
			return
		}
		// Core is reachable; remove drained file
		if err := e.spool.Remove(file); err != nil {
			e.logger.Warn("Failed to remove drained spool file", "file", file, "error", err)
		} else {
			e.logger.Debug("Successfully drained spool file", "file", file)
		}
	}
}

func (e *CollectorEngine) Stop() {
	e.logger.Info("Signaling collector engine to stop")
	close(e.stopCh)
	e.wg.Wait()
	e.logger.Info("Collector engine stopped cleanly")
}

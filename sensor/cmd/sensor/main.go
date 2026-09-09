package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"platform/sensor/internal/collector"
	"platform/sensor/internal/config"
	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/health"
	"platform/sensor/internal/observation"
	"platform/sensor/internal/spool"
	"platform/sensor/internal/uploader"
)

func main() {
	// Configure structured JSON logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With(
		"service", "platform-sensor",
	)
	slog.SetDefault(logger)

	cfg := config.LoadConfig()
	logger = logger.With("sensor_id", cfg.SensorID, "sensor_type", cfg.SensorType)
	logger.Info("Platform Sensor starting up",
		"version", "0.1.0-dev",
		"environment", cfg.Environment,
		"core_url", cfg.CoreURL,
		"spool_dir", cfg.SpoolDir,
		"watermark_percent", cfg.StorageWatermarkPercent,
	)

	// Initialize disk spool with checked Open
	diskSpool, err := spool.Open(spool.Config{
		Dir:            cfg.SpoolDir,
		MaxRecordBytes: 1024 * 1024,
		Logger:         logger,
	})
	if err != nil {
		logger.Error("Failed to initialize disk spool", "error", err)
		os.Exit(1)
	}

	// Initialize observation sink
	sink, err := observation.NewDiskSink(diskSpool, 1024*1024)
	if err != nil {
		logger.Error("Failed to initialize observation sink", "error", err)
		os.Exit(1)
	}

	// Initialize Core client with mTLS credentials
	client, err := coreclient.New(coreclient.Config{
		BaseURL:          cfg.CoreURL,
		CertificateFile:  cfg.TLSCertFile,
		PrivateKeyFile:   cfg.TLSKeyFile,
		CAFile:           cfg.CACertFile,
		RequestTimeout:   10 * time.Second,
		MaxResponseBytes: 1024 * 1024,
		Logger:           logger,
	})
	if err != nil {
		logger.Error("Failed to initialize Core client", "error", err)
		os.Exit(1)
	}

	// Initialize uploader
	up, err := uploader.New(uploader.Config{
		SensorID:             cfg.SensorID,
		BootEpoch:            fmt.Sprintf("epoch-%d", time.Now().Unix()),
		MaxBatchRecords:      100,
		MaxBatchPayloadBytes: 1024 * 1024,
		MaxRequestBytes:      2 * 1024 * 1024,
		MaxAttempts:          5,
		InitialBackoff:       500 * time.Millisecond,
		MaxBackoff:           5 * time.Second,
		JitterFraction:       0.1,
		IdleInterval:         1 * time.Second,
	}, diskSpool, client)
	if err != nil {
		logger.Error("Failed to initialize uploader", "error", err)
		os.Exit(1)
	}

	// Initialize health server
	healthServer := health.NewHealthServer(cfg.HealthPort, diskSpool, logger)
	go func() {
		if err := healthServer.Start(); err != nil {
			logger.Error("Health server encountered error", "error", err)
		}
	}()

	// Initialize and start collector engine and uploader
	ctx, cancel := context.WithCancel(context.Background())
	if err := up.Start(ctx); err != nil {
		logger.Error("Failed to start uploader", "error", err)
		os.Exit(1)
	}

	engine := collector.NewCollectorEngine(cfg.SensorID, cfg.SensorType, sink, cfg.PollInterval, logger)
	engine.Start(ctx)

	// Setup signal catching for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	logger.Info("Received termination signal, starting graceful shutdown", "signal", sig.String())

	// Graceful shutdown sequence
	cancel()
	engine.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := up.Shutdown(shutdownCtx); err != nil {
		logger.Warn("Uploader shutdown error", "error", err)
	}
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("Health server shutdown error", "error", err)
	}

	logger.Info("Platform Sensor shutdown complete")
}

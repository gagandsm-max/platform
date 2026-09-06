package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"platform/sensor/internal/collector"
	"platform/sensor/internal/config"
	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/health"
	"platform/sensor/internal/spool"
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

	// Initialize disk spool
	diskSpool, err := spool.NewDiskSpool(cfg.SpoolDir, cfg.StorageWatermarkPercent, logger)
	if err != nil {
		logger.Error("Failed to initialize disk spool", "error", err)
		os.Exit(1)
	}

	// Initialize Core client with mTLS credentials
	client := coreclient.NewCoreClient(cfg.CoreURL, cfg.TLSCertFile, cfg.TLSKeyFile, cfg.CACertFile, logger)

	// Initialize health server
	healthServer := health.NewHealthServer(cfg.HealthPort, diskSpool, logger)
	go func() {
		if err := healthServer.Start(); err != nil {
			logger.Error("Health server encountered error", "error", err)
		}
	}()

	// Initialize and start collector engine
	ctx, cancel := context.WithCancel(context.Background())
	engine := collector.NewCollectorEngine(cfg.SensorID, cfg.SensorType, diskSpool, client, cfg.PollInterval, logger)
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
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("Health server shutdown error", "error", err)
	}

	logger.Info("Platform Sensor shutdown complete")
}

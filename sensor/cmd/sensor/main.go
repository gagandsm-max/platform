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
	"platform/sensor/internal/discovery"
	"platform/sensor/internal/health"
	"platform/sensor/internal/spool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})).With(
		"service", "platform-sensor",
	)
	slog.SetDefault(logger)

	cfg := config.LoadConfig()
	logger = logger.With("sensor_id", cfg.SensorID, "sensor_type", cfg.SensorType)
	logger.Info("Platform Sensor starting up",
		"version", "0.1.0-dev", "environment", cfg.Environment, "core_url", cfg.CoreURL,
		"spool_dir", cfg.SpoolDir, "watermark_percent", cfg.StorageWatermarkPercent)

	diskSpool, err := spool.NewDiskSpool(cfg.SpoolDir, cfg.StorageWatermarkPercent, logger)
	if err != nil {
		logger.Error("Failed to initialize disk spool", "error_code", "SENSOR_SPOOL_INIT_FAILED")
		os.Exit(1)
	}

	// Existing pre-runtime telemetry path is retained unchanged. Discovery uses the checked mTLS client
	// because task/result authority depends on authenticated Sensor identity.
	collectorClient := coreclient.NewCoreClient(cfg.CoreURL, cfg.TLSCertFile, cfg.TLSKeyFile, cfg.CACertFile, logger)
	requestTimeout := cfg.PollInterval
	if requestTimeout <= 0 { requestTimeout = 10 * time.Second }
	discoveryClient, err := coreclient.New(coreclient.Config{
		BaseURL: cfg.CoreURL, CertificateFile: cfg.TLSCertFile, PrivateKeyFile: cfg.TLSKeyFile,
		CAFile: cfg.CACertFile, RequestTimeout: requestTimeout,
		MaxResponseBytes: 1 << 20, // defensive response guard; not a product capacity claim
		Logger: logger,
	})
	if err != nil {
		logger.Error("Failed to initialize authenticated discovery Core client", "error_code", "DISCOVERY_CORECLIENT_INIT_FAILED")
		os.Exit(1)
	}

	healthServer := health.NewHealthServer(cfg.HealthPort, diskSpool, logger)
	go func() {
		if err := healthServer.Start(); err != nil {
			logger.Error("Health server encountered error", "error_code", "SENSOR_HEALTH_SERVER_FAILED")
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	engine := collector.NewCollectorEngine(cfg.SensorID, cfg.SensorType, diskSpool, collectorClient, cfg.PollInterval, logger)
	engine.Start(ctx)

	discoveryExecutor := discovery.NewExecutor(cfg.SensorID, discovery.DefaultMaxConcurrent, logger)
	discoveryService := discovery.NewService(cfg.SensorID, discoveryClient, discoveryExecutor, cfg.PollInterval, logger)
	discoveryService.Start(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("Received termination signal, starting graceful shutdown", "signal", sig.String())

	cancel()
	discoveryService.Stop()
	engine.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("Health server shutdown error", "error_code", "SENSOR_HEALTH_SHUTDOWN_FAILED")
	}
	logger.Info("Platform Sensor shutdown complete")
}
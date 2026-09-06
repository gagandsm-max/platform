package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"platform/sensor/internal/spool"
)

type HealthServer struct {
	server *http.Server
	spool  *spool.DiskSpool
	logger *slog.Logger
}

func NewHealthServer(port int, diskSpool *spool.DiskSpool, logger *slog.Logger) *HealthServer {
	mux := http.NewServeMux()
	hs := &HealthServer{
		server: &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		spool:  diskSpool,
		logger: logger,
	}

	mux.HandleFunc("/health", hs.handleHealth)
	mux.HandleFunc("/healthz", hs.handleHealth)

	return hs
}

func (hs *HealthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	storagePercent, err := hs.spool.CheckStorageUsage()
	status := "UP"
	httpStatus := http.StatusOK

	if err != nil {
		hs.logger.Warn("Failed checking disk storage in health probe", "error", err)
	} else if storagePercent >= 95.0 {
		status = "OUT_OF_DISK"
		httpStatus = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          status,
		"storage_percent": fmt.Sprintf("%.2f%%", storagePercent),
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	})
}

func (hs *HealthServer) Start() error {
	hs.logger.Info("Starting sensor health server", "addr", hs.server.Addr)
	if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (hs *HealthServer) Shutdown(ctx context.Context) error {
	return hs.server.Shutdown(ctx)
}

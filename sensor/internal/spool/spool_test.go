package spool

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
)

func TestSpoolWriteAndDrain(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spool_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	spool, err := NewDiskSpool(tempDir, 80, logger)
	if err != nil {
		t.Fatalf("failed to create disk spool: %v", err)
	}

	item := &SpoolItem{
		ID:        "test-1",
		SensorID:  "sensor-test-01",
		Timestamp: 1234567890,
		Type:      "network",
		Payload:   json.RawMessage(`{"status":"OK"}`),
	}

	if err := spool.Write(item); err != nil {
		t.Fatalf("failed to write spool item: %v", err)
	}

	files, err := spool.ListPendingFiles(10)
	if err != nil {
		t.Fatalf("failed to list spool files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	if err := spool.Remove(files[0]); err != nil {
		t.Fatalf("failed to remove spool file: %v", err)
	}

	filesAfter, _ := spool.ListPendingFiles(10)
	if len(filesAfter) != 0 {
		t.Fatalf("expected 0 files after removal, got %d", len(filesAfter))
	}
}

func TestWatermarkThreshold(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spool_watermark_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Set watermark threshold to 0% so any disk usage triggers the guard
	spool, err := NewDiskSpool(tempDir, 0, logger)
	if err != nil {
		t.Fatalf("failed to create spool: %v", err)
	}

	item := &SpoolItem{
		ID:        "guard-test",
		SensorID:  "sensor-test-01",
		Timestamp: 1234567890,
		Type:      "network",
		Payload:   json.RawMessage(`{"data":"overflow"}`),
	}

	err = spool.Write(item)
	if err == nil {
		t.Fatalf("expected error due to 0%% storage watermark guard, but got nil")
	}
}

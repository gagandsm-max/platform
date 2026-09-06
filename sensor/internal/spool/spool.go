package spool

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"
)

type SpoolItem struct {
	ID        string          `json:"id"`
	SensorID  string          `json:"sensor_id"`
	Timestamp int64           `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type DiskSpool struct {
	dir              string
	watermarkPercent int
	mu               sync.Mutex
	logger           *slog.Logger
}

func NewDiskSpool(dir string, watermarkPercent int, logger *slog.Logger) (*DiskSpool, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create spool directory: %w", err)
	}
	return &DiskSpool{
		dir:              dir,
		watermarkPercent: watermarkPercent,
		logger:           logger,
	}, nil
}

// CheckStorageUsage returns current disk usage percentage for spool filesystem.
func (s *DiskSpool) CheckStorageUsage() (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.dir, &stat); err != nil {
		return 0, err
	}
	if stat.Blocks == 0 {
		return 0, nil
	}
	used := stat.Blocks - stat.Bavail
	percent := (float64(used) / float64(stat.Blocks)) * 100.0
	return percent, nil
}

// Write appends an item to the disk spool if below the 80% watermark.
func (s *DiskSpool) Write(item *SpoolItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	usagePercent, err := s.CheckStorageUsage()
	if err != nil {
		s.logger.Warn("Failed to inspect disk usage", "error", err)
	} else if usagePercent >= float64(s.watermarkPercent) {
		s.logger.Error("Spool rejected: storage watermark exceeded",
			"usage_percent", usagePercent,
			"watermark_percent", s.watermarkPercent,
			"spool_dir", s.dir,
		)
		return fmt.Errorf("spool storage watermark exceeded: %.1f%% >= %d%%", usagePercent, s.watermarkPercent)
	}

	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal spool item: %w", err)
	}

	fileName := fmt.Sprintf("%d-%s.json", time.Now().UnixNano(), item.ID)
	filePath := filepath.Join(s.dir, fileName)

	// Write atomically with temporary file
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0640); err != nil {
		return fmt.Errorf("failed to write spool file: %w", err)
	}
	return os.Rename(tmpPath, filePath)
}

// ListPendingFiles returns the oldest pending spool files sorted chronologically.
func (s *DiskSpool) ListPendingFiles(limit int) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			files = append(files, filepath.Join(s.dir, entry.Name()))
		}
	}

	sort.Strings(files)
	if len(files) > limit {
		files = files[:limit]
	}
	return files, nil
}

// Remove removes a successfully processed spool file.
func (s *DiskSpool) Remove(filePath string) error {
	return os.Remove(filePath)
}

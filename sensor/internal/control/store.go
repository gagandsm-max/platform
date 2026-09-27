package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	storeSchemaVersion = 1
	storeFileMode      = 0o640
	storeDirMode       = 0o750
)

// AppliedSnapshot is the durable local evidence of the last APPLIED control state.
// It must never contain secrets, credentials, TLS private keys or bootstrap tokens.
type AppliedSnapshot struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Generation    uint64               `json:"generation"`
	Checksum      string               `json:"checksum"`
	Configuration DesiredConfiguration `json:"configuration"`
	AppliedAt     time.Time            `json:"appliedAt"`
}

// Store persists AppliedSnapshot at an explicit absolute path.
type Store struct {
	path string
}

// NewStore validates path and returns a durable control-state store.
func NewStore(path string) (*Store, error) {
	if err := validateStorePath(path); err != nil {
		return nil, err
	}
	return &Store{path: path}, nil
}

// Path returns the configured absolute snapshot path.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Load returns the snapshot when present. Missing file returns (nil, nil).
// Corrupt/tampered snapshots return a fail-closed CodeStateCorrupt error and are not deleted.
func (s *Store) Load() (*AppliedSnapshot, error) {
	if s == nil {
		return nil, newCodedError(CodeStateCorrupt, "store is nil", nil)
	}
	fi, err := os.Lstat(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, newCodedError(CodeStateCorrupt, "failed to stat control snapshot", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, newCodedError(CodeStateCorrupt, "control snapshot path is a symlink", nil)
	}
	if !fi.Mode().IsRegular() {
		return nil, newCodedError(CodeStateCorrupt, "control snapshot path is not a regular file", nil)
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, newCodedError(CodeStateCorrupt, "failed to read control snapshot", err)
	}
	snap, err := decodeSnapshot(data)
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// Commit atomically publishes a validated APPLIED snapshot.
func (s *Store) Commit(snap AppliedSnapshot) error {
	if s == nil {
		return newCodedError(CodeStateDurabilityFailed, "store is nil", nil)
	}
	if err := validateSnapshot(snap); err != nil {
		return err
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return newCodedError(CodeStateDurabilityFailed, "failed to encode control snapshot", err)
	}
	dir := filepath.Dir(s.path)
	if err := ensureSafeDir(dir); err != nil {
		return err
	}
	return atomicWriteFile(s.path, payload, storeFileMode)
}

func decodeSnapshot(data []byte) (*AppliedSnapshot, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var snap AppliedSnapshot
	if err := dec.Decode(&snap); err != nil {
		return nil, newCodedError(CodeStateCorrupt, "control snapshot JSON is invalid", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, newCodedError(CodeStateCorrupt, "control snapshot contains trailing JSON", nil)
	}
	if err := validateSnapshot(snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func validateSnapshot(snap AppliedSnapshot) error {
	if snap.SchemaVersion != storeSchemaVersion {
		return newCodedError(CodeStateCorrupt, "unsupported control snapshot schemaVersion", nil)
	}
	if snap.Generation == 0 || snap.Generation > MaxSafeControlGeneration {
		return newCodedError(CodeStateCorrupt, "control snapshot generation is out of range", nil)
	}
	if !isLowerHex64(snap.Checksum) {
		return newCodedError(CodeStateCorrupt, "control snapshot checksum is not lowercase 64-hex", nil)
	}
	if err := validateDesiredConfigurationBounds(snap.Configuration); err != nil {
		return newCodedError(CodeStateCorrupt, "control snapshot configuration is invalid", err)
	}
	computed, err := ComputeDesiredChecksum(snap.Configuration)
	if err != nil {
		return newCodedError(CodeStateCorrupt, "failed to recompute snapshot checksum", err)
	}
	if computed != snap.Checksum {
		return newCodedError(CodeStateCorrupt, "control snapshot semantic checksum mismatch", nil)
	}
	if snap.AppliedAt.IsZero() {
		return newCodedError(CodeStateCorrupt, "control snapshot appliedAt is required", nil)
	}
	return nil
}

func validateStorePath(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return newCodedError(CodeStateDurabilityFailed, "control snapshot path must be absolute", nil)
	}
	cleaned := filepath.Clean(path)
	if cleaned != path {
		return newCodedError(CodeStateDurabilityFailed, "control snapshot path must be cleaned absolute", nil)
	}
	if filepath.Base(cleaned) == "." || filepath.Base(cleaned) == string(filepath.Separator) {
		return newCodedError(CodeStateDurabilityFailed, "control snapshot path is unsafe", nil)
	}
	dir := filepath.Dir(cleaned)
	if err := ensureSafeDir(dir); err != nil {
		return err
	}
	if fi, err := os.Lstat(cleaned); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return newCodedError(CodeStateDurabilityFailed, "control snapshot path must not be a symlink", nil)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return newCodedError(CodeStateDurabilityFailed, "failed to inspect control snapshot path", err)
	}
	return nil
}

func ensureSafeDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return newCodedError(CodeStateDurabilityFailed, "control snapshot parent directory is required", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return newCodedError(CodeStateDurabilityFailed, "control snapshot parent directory must not be a symlink", nil)
	}
	if !fi.IsDir() {
		return newCodedError(CodeStateDurabilityFailed, "control snapshot parent must be a directory", nil)
	}
	return nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".sensor-control-*.tmp")
	if err != nil {
		return newCodedError(CodeStateDurabilityFailed, "failed to create temporary control snapshot", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return newCodedError(CodeStateDurabilityFailed, "failed to set control snapshot permissions", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return newCodedError(CodeStateDurabilityFailed, "failed to write control snapshot", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return newCodedError(CodeStateDurabilityFailed, "failed to fsync control snapshot", err)
	}
	if err := tmp.Close(); err != nil {
		return newCodedError(CodeStateDurabilityFailed, "failed to close control snapshot", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return newCodedError(CodeStateDurabilityFailed, "failed to publish control snapshot atomically", err)
	}
	cleanup = false
	if err := syncDir(dir); err != nil {
		return err
	}
	return nil
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return newCodedError(CodeStateDurabilityFailed, "failed to open control snapshot directory for fsync", err)
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return newCodedError(CodeStateDurabilityFailed, "failed to fsync control snapshot directory", err)
	}
	return nil
}

func isLowerHex64(v string) bool {
	if len(v) != 64 {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func validateDesiredConfigurationBounds(cfg DesiredConfiguration) error {
	switch cfg.LogLevel {
	case "DEBUG", "INFO", "WARN", "ERROR":
	default:
		return fmt.Errorf("logLevel invalid")
	}
	s := cfg.Scheduler
	if s.MaxWorkers < 1 || s.MaxWorkers > MaxWorkers {
		return fmt.Errorf("maxWorkers out of range")
	}
	if s.MaxQueueDepth < 1 || s.MaxQueueDepth > MaxQueueDepth {
		return fmt.Errorf("maxQueueDepth out of range")
	}
	if s.MaxTrackedAssignments < 1 || s.MaxTrackedAssignments > MaxTrackedAssignments {
		return fmt.Errorf("maxTrackedAssignments out of range")
	}
	if s.MaxTrackedJobs < 1 || s.MaxTrackedJobs > MaxTrackedJobs {
		return fmt.Errorf("maxTrackedJobs out of range")
	}
	if s.MaxPayloadBytes < 1 || s.MaxPayloadBytes > MaxPayloadBytes {
		return fmt.Errorf("maxPayloadBytes out of range")
	}
	if s.DefaultExecutionTimeoutMs < 1 || s.DefaultExecutionTimeoutMs > MaxExecutionTimeoutMs {
		return fmt.Errorf("defaultExecutionTimeoutMs out of range")
	}
	if s.ShutdownGracePeriodMs < 1 || s.ShutdownGracePeriodMs > MaxShutdownGraceMs {
		return fmt.Errorf("shutdownGracePeriodMs out of range")
	}
	if s.RetryMaxAttempts < 1 || s.RetryMaxAttempts > MaxRetryAttempts {
		return fmt.Errorf("retryMaxAttempts out of range")
	}
	if s.RetryInitialBackoffMs < 1 || s.RetryInitialBackoffMs > MaxRetryBackoffMs {
		return fmt.Errorf("retryInitialBackoffMs out of range")
	}
	if s.RetryMaxBackoffMs < 1 || s.RetryMaxBackoffMs > MaxRetryBackoffMs {
		return fmt.Errorf("retryMaxBackoffMs out of range")
	}
	if s.RetryJitterBasisPoints < 0 || s.RetryJitterBasisPoints > MaxJitterBasisPoints {
		return fmt.Errorf("retryJitterBasisPoints out of range")
	}
	if len(s.Pools) < 1 || len(s.Pools) > MaxPoolCount {
		return fmt.Errorf("pools count out of range")
	}
	workers, queues := 0, 0
	for name, pool := range s.Pools {
		switch name {
		case "NETWORK_POLL", "STREAMING", "INFRA", "DISCOVERY", "NCM", "SYNTHETIC":
		default:
			return fmt.Errorf("unknown pool class")
		}
		if pool.Workers < 1 || pool.Workers > MaxWorkers {
			return fmt.Errorf("pool workers out of range")
		}
		if pool.QueueDepth < 1 || pool.QueueDepth > MaxQueueDepth {
			return fmt.Errorf("pool queueDepth out of range")
		}
		workers += pool.Workers
		queues += pool.QueueDepth
		if workers > s.MaxWorkers || queues > s.MaxQueueDepth {
			return fmt.Errorf("pool bounds exceed maxima")
		}
	}
	t := cfg.Transport
	if t.RequestTimeoutMs < 1 || t.RequestTimeoutMs > MaxExecutionTimeoutMs {
		return fmt.Errorf("requestTimeoutMs out of range")
	}
	if t.MaxRequestBytes < 1 || t.MaxRequestBytes > MaxRequestBytes {
		return fmt.Errorf("maxRequestBytes out of range")
	}
	if t.MaxResponseBytes < 1 || t.MaxResponseBytes > MaxResponseBytes {
		return fmt.Errorf("maxResponseBytes out of range")
	}
	if t.UploadBatchRecords < 1 || t.UploadBatchRecords > MaxUploadBatchRecords {
		return fmt.Errorf("uploadBatchRecords out of range")
	}
	if t.UploadBatchBytes < 1 || t.UploadBatchBytes > MaxUploadBatchBytes {
		return fmt.Errorf("uploadBatchBytes out of range")
	}
	return nil
}

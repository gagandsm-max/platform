package spool

import (
	"log/slog"
	"path/filepath"
	"time"
)

func (s *DiskSpool) CheckStorageUsage() (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.space(0)
	return st.usedPct, err
}
func NewDiskSpool(dir string, watermarkPercent int, logger *slog.Logger) (*DiskSpool, error) {
	if watermarkPercent != storageWatermarkPercent {
		return nil, newError(CodeInvalidConfig, "Sensor storage watermark is fixed at 80% and cannot be overridden", nil)
	}
	return Open(Config{Dir: dir, MaxRecordBytes: 1024 * 1024, Logger: logger})
}
func (s *DiskSpool) Write(item *SpoolItem) error {
	if item == nil {
		return newError(CodeInvalidRecord, "spool item is nil", nil)
	}
	_, err := s.Append(AppendRequest{Class: ClassOperational, Type: item.Type, Payload: append([]byte(nil), item.Payload...), CreatedAt: time.Unix(item.Timestamp, 0)})
	return err
}
func (s *DiskSpool) ListPendingFiles(limit int) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		return nil, newError(CodeInvalidConfig, "limit must be greater than zero", nil)
	}
	seqs, err := s.pendingSequencesLocked()
	if err != nil {
		return nil, err
	}
	if len(seqs) > limit {
		seqs = seqs[:limit]
	}
	out := make([]string, 0, len(seqs))
	for _, seq := range seqs {
		out = append(out, filepath.Join(s.dir, recordFileName(seq)))
	}
	return out, nil
}
func (s *DiskSpool) Remove(string) error {
	return newError(CodeUnsafeLegacyAPI, "path-based spool deletion is disabled; acknowledge an authoritative sequence watermark instead", nil)
}

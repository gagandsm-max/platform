package spool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func Open(cfg Config) (*DiskSpool, error) {
	if strings.TrimSpace(cfg.Dir) == "" {
		return nil, newError(CodeInvalidConfig, "spool directory is required", nil)
	}
	if !filepath.IsAbs(cfg.Dir) {
		return nil, newError(CodeInvalidConfig, "spool directory must be absolute", nil)
	}
	if cfg.MaxRecordBytes <= 0 {
		return nil, newError(CodeInvalidConfig, "maximum record bytes must be greater than zero", nil)
	}
	clean := filepath.Clean(cfg.Dir)
	if err := ensureDirSecure(clean, 0750); err != nil {
		return nil, err
	}
	quarantine := filepath.Join(clean, quarantineDirName)
	if err := ensureDirSecure(quarantine, 0750); err != nil {
		return nil, err
	}
	s := &DiskSpool{dir: clean, quarantineDir: quarantine, maxRecordBytes: cfg.MaxRecordBytes, logger: cfg.Logger}
	s.space = s.filesystemStatus
	if err := s.recover(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DiskSpool) Append(req AppendRequest) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Record{}, newError(CodeClosed, "spool is closed", nil)
	}
	if !validClass(req.Class) {
		return Record{}, newError(CodeInvalidRecord, "record class is unsupported", nil)
	}
	if req.Type == "" || len(req.Type) > 128 {
		return Record{}, newError(CodeInvalidRecord, "record type must be 1..128 bytes", nil)
	}
	if len(req.Payload) == 0 || len(req.Payload) > s.maxRecordBytes {
		return Record{}, newError(CodeInvalidRecord, "payload length is outside configured bounds", nil)
	}
	created := req.CreatedAt.UTC()
	if created.IsZero() {
		created = time.Now().UTC()
	}
	seq := s.state.NextSequence
	sum := sha256.Sum256(req.Payload)
	checksum := hex.EncodeToString(sum[:])
	rec := Record{SchemaVersion: recordSchemaVersion, Sequence: seq, Class: req.Class, Type: req.Type, CreatedAt: created, ContentLength: len(req.Payload), Checksum: checksum, Payload: append([]byte(nil), req.Payload...)}
	rec.ID = fmt.Sprintf("rec-%020d-%s", seq, checksum[:16])
	encoded, err := json.Marshal(rec)
	if err != nil {
		return Record{}, newError(CodeInvalidRecord, "record cannot be encoded", err)
	}
	status, err := s.space(int64(len(encoded)))
	if err != nil {
		s.degraded = true
		return Record{}, newError(CodeIO, "failed to inspect filesystem capacity", err)
	}
	projected := status.usedBytes + uint64(len(encoded))
	if status.totalBytes == 0 || projected*100 >= status.totalBytes*storageWatermarkPercent {
		return Record{}, newError(CodeProtection, "80% Sensor storage protection prevents accepting this record", nil)
	}
	if err := atomicWriteFile(filepath.Join(s.dir, recordFileName(seq)), encoded, 0640); err != nil {
		return Record{}, err
	}
	s.state.NextSequence++
	if err := s.persistStateLocked(); err != nil {
		s.degraded = true
		return Record{}, err
	}
	return rec, nil
}

func (s *DiskSpool) ReadBatch(maxRecords, maxPayloadBytes int) (Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Batch{}, newError(CodeClosed, "spool is closed", nil)
	}
	if maxRecords <= 0 || maxPayloadBytes <= 0 {
		return Batch{}, newError(CodeInvalidConfig, "batch bounds must be greater than zero", nil)
	}
	seqs, err := s.pendingSequencesLocked()
	if err != nil {
		return Batch{}, err
	}
	batch := Batch{Records: make([]Record, 0, minInt(maxRecords, len(seqs)))}
	expected := s.state.AckedThrough + 1
	for _, seq := range seqs {
		if seq < expected {
			continue
		}
		if seq != expected {
			s.degraded = true
			return Batch{}, newError(CodeSequence, "pending spool contains a sequence gap", nil)
		}
		path := filepath.Join(s.dir, recordFileName(seq))
		rec, err := s.readRecordFileLocked(path, seq)
		if err != nil {
			_ = s.quarantineLocked(path, "corrupt")
			s.degraded = true
			return Batch{}, err
		}
		if len(batch.Records) == maxRecords || (len(batch.Records) > 0 && batch.PayloadBytes+rec.ContentLength > maxPayloadBytes) {
			break
		}
		if len(batch.Records) == 0 && rec.ContentLength > maxPayloadBytes {
			return Batch{}, newError(CodeInvalidConfig, "next record exceeds requested batch byte bound", nil)
		}
		batch.Records = append(batch.Records, rec)
		batch.PayloadBytes += rec.ContentLength
		if batch.FirstSequence == 0 {
			batch.FirstSequence = rec.Sequence
		}
		batch.LastSequence = rec.Sequence
		expected++
	}
	return batch, nil
}

func (s *DiskSpool) AcknowledgeThrough(sequence uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return newError(CodeClosed, "spool is closed", nil)
	}
	if sequence <= s.state.AckedThrough {
		return nil
	}
	highest := s.state.NextSequence - 1
	if sequence > highest {
		return newError(CodeSequence, "acknowledgement cannot advance beyond the highest issued sequence", nil)
	}
	for seq := s.state.AckedThrough + 1; seq <= sequence; seq++ {
		path := filepath.Join(s.dir, recordFileName(seq))
		info, err := os.Lstat(path)
		if err != nil {
			return newError(CodeSequence, "acknowledgement cannot skip a missing sequence", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return newError(CodeUnsafePath, "acknowledgement target is not a regular spool record", nil)
		}
		if _, err := s.readRecordFileLocked(path, seq); err != nil {
			return err
		}
	}
	previous := s.state.AckedThrough
	s.state.AckedThrough = sequence
	if err := s.persistStateLocked(); err != nil {
		s.state.AckedThrough = previous
		return err
	}
	for seq := previous + 1; seq <= sequence; seq++ {
		if err := os.Remove(filepath.Join(s.dir, recordFileName(seq))); err != nil && !os.IsNotExist(err) {
			s.degraded = true
			return newError(CodeIO, "acknowledged record metadata is durable but file cleanup failed", err)
		}
	}
	return syncDir(s.dir)
}

func (s *DiskSpool) Stats() (Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seqs, err := s.pendingSequencesLocked()
	if err != nil {
		return Stats{}, err
	}
	var bytes int64
	var oldest time.Time
	for _, seq := range seqs {
		p := filepath.Join(s.dir, recordFileName(seq))
		info, err := os.Lstat(p)
		if err != nil {
			return Stats{}, newError(CodeIO, "failed to stat spool record", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			s.degraded = true
			return Stats{}, newError(CodeUnsafePath, "spool record path is unsafe", nil)
		}
		bytes += info.Size()
		rec, err := s.readRecordFileLocked(p, seq)
		if err != nil {
			s.degraded = true
			return Stats{}, err
		}
		if oldest.IsZero() || rec.CreatedAt.Before(oldest) {
			oldest = rec.CreatedAt
		}
	}
	status, err := s.space(0)
	if err != nil {
		s.degraded = true
		return Stats{}, newError(CodeIO, "failed to inspect filesystem capacity", err)
	}
	state := ProtectionSafe
	if status.usedPct >= storageWatermarkPercent {
		state = ProtectionTriggered
	}
	if s.degraded {
		state = ProtectionDegraded
	}
	out := Stats{NextSequence: s.state.NextSequence, AckedThrough: s.state.AckedThrough, RecordCount: len(seqs), SpoolBytes: bytes, FilesystemUsedPct: status.usedPct, ProtectionState: state, QuarantinedRecords: s.quarantined, CorruptionDetected: s.degraded}
	if !oldest.IsZero() {
		out.OldestRecordAge = time.Since(oldest)
		if out.OldestRecordAge < 0 {
			out.OldestRecordAge = 0
		}
	}
	return out, nil
}

// AckedThrough returns the durable acknowledged spool watermark. Sequences at
// or below this value have been removed from the pending set.
func (s *DiskSpool) AckedThrough() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, newError(CodeClosed, "spool is closed", nil)
	}
	return s.state.AckedThrough, nil
}

func (s *DiskSpool) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if err := s.persistStateLocked(); err != nil {
		return err
	}
	if err := syncDir(s.dir); err != nil {
		return err
	}
	s.closed = true
	return nil
}
func (s *DiskSpool) pendingSequencesLocked() ([]uint64, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, newError(CodeIO, "failed to list spool records", err)
	}
	seqs := make([]uint64, 0)
	for _, e := range entries {
		seq, ok := parseRecordFileName(e.Name())
		if ok && seq > s.state.AckedThrough {
			seqs = append(seqs, seq)
		}
	}
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	return seqs, nil
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

package spool

import (
	"os"
	"path/filepath"
)

// VisitPending walks every currently pending spool record in durable sequence
// order while holding the spool lock. The callback must not call back into the
// same DiskSpool. It is intended for recovery/reconciliation code that must
// derive secondary state from the spool without treating a bounded ReadBatch
// page as the complete backlog.
func (s *DiskSpool) VisitPending(visit func(Record) error) error {
	if visit == nil {
		return newError(CodeInvalidConfig, "pending spool visitor is required", nil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return newError(CodeClosed, "spool is closed", nil)
	}

	seqs, err := s.pendingSequencesLocked()
	if err != nil {
		return err
	}
	expected := s.state.AckedThrough + 1
	for _, seq := range seqs {
		if seq < expected {
			continue
		}
		if seq != expected {
			s.degraded = true
			return newError(CodeSequence, "pending spool contains a sequence gap", nil)
		}
		path := filepath.Join(s.dir, recordFileName(seq))
		rec, err := s.readRecordFileLocked(path, seq)
		if err != nil {
			_ = s.quarantineLocked(path, "corrupt")
			s.degraded = true
			return err
		}
		if err := visit(rec); err != nil {
			return err
		}
		expected++
	}
	return nil
}

// ReadPendingRecord returns one currently pending durable spool record by its
// spool sequence. It fails closed if the record is already acknowledged,
// outside the issued range, missing, unsafe, or corrupt.
func (s *DiskSpool) ReadPendingRecord(sequence uint64) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Record{}, newError(CodeClosed, "spool is closed", nil)
	}
	if sequence <= s.state.AckedThrough || sequence >= s.state.NextSequence {
		return Record{}, newError(CodeSequence, "requested spool sequence is not pending", nil)
	}

	path := filepath.Join(s.dir, recordFileName(sequence))
	info, err := os.Lstat(path)
	if err != nil {
		return Record{}, newError(CodeSequence, "pending spool record is missing", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		s.degraded = true
		return Record{}, newError(CodeUnsafePath, "pending spool record path is unsafe", nil)
	}
	rec, err := s.readRecordFileLocked(path, sequence)
	if err != nil {
		s.degraded = true
		return Record{}, err
	}
	return rec, nil
}

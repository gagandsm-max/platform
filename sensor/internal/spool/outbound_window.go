package spool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// outboundWindow is a versioned sidecar that freezes the exact [first,last]
// range selected for network transmission until durable ACK or explicit poison abandon.
// It is spool metadata, not a telemetry record. Older recover() scanners ignore it
// because it is not named record-*.json and is not state.json.
type outboundWindow struct {
	SchemaVersion int    `json:"schemaVersion"`
	StreamID      string `json:"streamId"`
	FirstSequence uint64 `json:"firstSequence"`
	LastSequence  uint64 `json:"lastSequence"`
	SizeRejected  bool   `json:"sizeRejected,omitempty"` // Legacy diagnostic; never suppresses replay.
}

// ReadOrReserveOutboundBatch returns the currently frozen outbound range, or
// selects a new batch and durably persists its exact [first,last] before returning.
// Concurrent spool growth after reservation cannot expand this window.
func (s *DiskSpool) ReadOrReserveOutboundBatch(maxRecords, maxPayloadBytes int) (Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Batch{}, newError(CodeClosed, "spool is closed", nil)
	}
	if maxRecords <= 0 || maxPayloadBytes <= 0 {
		return Batch{}, newError(CodeInvalidConfig, "batch bounds must be greater than zero", nil)
	}

	win, exists, err := s.loadOutboundWindowLocked()
	if err != nil {
		return Batch{}, err
	}
	if exists {
		if win.LastSequence <= s.state.AckedThrough {
			// Crash after durable ACK but before sidecar deletion: reconcile safely.
			if err := s.clearOutboundWindowLocked(); err != nil {
				return Batch{}, err
			}
		} else {
			batch, err := s.readExactRangeLocked(win.FirstSequence, win.LastSequence, maxRecords, maxPayloadBytes)
			if err != nil {
				return Batch{}, err
			}
			return batch, nil
		}
	}

	batch, err := s.readBatchLocked(maxRecords, maxPayloadBytes)
	if err != nil {
		return Batch{}, err
	}
	if len(batch.Records) == 0 {
		return batch, nil
	}
	win = outboundWindow{
		SchemaVersion: outboundWindowSchema,
		StreamID:      s.streamID,
		FirstSequence: batch.FirstSequence,
		LastSequence:  batch.LastSequence,
	}
	if err := s.persistOutboundWindowLocked(win); err != nil {
		return Batch{}, err
	}
	return batch, nil
}

// CompleteOutboundBatch durably ACKs through the frozen outbound window last
// sequence (must match), then clears the sidecar. Call only after exact application ACK.
func (s *DiskSpool) CompleteOutboundBatch(ackedThrough uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return newError(CodeClosed, "spool is closed", nil)
	}
	win, exists, err := s.loadOutboundWindowLocked()
	if err != nil {
		return err
	}
	if !exists {
		return newError(CodeOutboundWindow, "no outbound window to complete", nil)
	}
	if ackedThrough != win.LastSequence {
		return newError(CodeOutboundWindow, "acknowledgement must equal frozen outbound window last sequence", nil)
	}
	if err := s.acknowledgeThroughLocked(ackedThrough); err != nil {
		return err
	}
	return s.clearOutboundWindowLocked()
}

// AbandonOutboundBatch clears a frozen outbound window without acknowledging records.
// Allowed only for explicit pre-reservation poison allowlist rejections.
func (s *DiskSpool) AbandonOutboundBatch() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return newError(CodeClosed, "spool is closed", nil)
	}
	return s.clearOutboundWindowLocked()
}

// RebatchOutboundBatch is disabled: a current size rejection does not prove
// that an earlier identical request was never accepted. Without an authoritative
// Core receipt/fencing protocol, changing frozen identity can prevent ACK recovery.
func (s *DiskSpool) RebatchOutboundBatch(first, last uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return newError(CodeClosed, "spool is closed", nil)
	}
	win, exists, err := s.loadOutboundWindowLocked()
	if err != nil {
		return err
	}
	if !exists || win.FirstSequence != first || win.LastSequence != last {
		return newError(CodeOutboundWindow, "rebatch range must equal frozen outbound window", nil)
	}
	if _, err := s.readExactRangeLocked(first, last, 1, 1); err != nil {
		return err
	}
	return newError(CodeOutboundWindow, "automatic rebatching disabled; restore Core compatibility and replay the original frozen batch", nil)
}

// OutboundBatchSizeRejected reports legacy diagnostic metadata only. A flagged
// window remains eligible for the same bounded replay and ordinary ACK as any
// other frozen window; this flag must never authorize suppression or deletion.
func (s *DiskSpool) OutboundBatchSizeRejected() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false, newError(CodeClosed, "spool is closed", nil)
	}
	win, exists, err := s.loadOutboundWindowLocked()
	return exists && win.SizeRejected, err
}

func (s *DiskSpool) outboundWindowPath() string {
	return filepath.Join(s.dir, outboundWindowFileName)
}

func (s *DiskSpool) loadOutboundWindowLocked() (outboundWindow, bool, error) {
	path := s.outboundWindowPath()
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return outboundWindow{}, false, nil
		}
		return outboundWindow{}, false, newError(CodeIO, "failed to inspect outbound window sidecar", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return outboundWindow{}, false, newError(CodeUnsafePath, "outbound window sidecar must be a regular file", nil)
	}
	if info.Size() <= 0 || info.Size() > maxOutboundWindowBytes {
		return outboundWindow{}, false, newError(CodeOutboundWindow, "outbound window sidecar size is invalid", nil)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return outboundWindow{}, false, newError(CodeIO, "failed to read outbound window sidecar", err)
	}
	if len(raw) == 0 || len(raw) > maxOutboundWindowBytes {
		return outboundWindow{}, false, newError(CodeOutboundWindow, "outbound window sidecar payload exceeds bound", nil)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var win outboundWindow
	if err := dec.Decode(&win); err != nil {
		return outboundWindow{}, false, newError(CodeOutboundWindow, "outbound window sidecar JSON is corrupt", err)
	}
	// Strict sidecar contract: exactly one JSON value. Trailing whitespace is fine;
	// any trailing token/value fails closed.
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return outboundWindow{}, false, newError(CodeOutboundWindow, "outbound window sidecar has trailing JSON", err)
	}
	if err := s.validateOutboundWindowLocked(win); err != nil {
		return outboundWindow{}, false, err
	}
	return win, true, nil
}

func (s *DiskSpool) validateOutboundWindowLocked(win outboundWindow) error {
	if win.SchemaVersion != outboundWindowSchema {
		return newError(CodeOutboundWindow, "outbound window schemaVersion is unsupported", nil)
	}
	if strings.TrimSpace(win.StreamID) == "" || win.StreamID != s.streamID {
		return newError(CodeOutboundWindow, "outbound window streamId does not match spool stream identity", nil)
	}
	if win.FirstSequence == 0 || win.LastSequence < win.FirstSequence {
		return newError(CodeOutboundWindow, "outbound window sequence range is impossible", nil)
	}
	if win.SizeRejected && win.FirstSequence != win.LastSequence {
		return newError(CodeOutboundWindow, "size block requires a singleton window", nil)
	}
	if win.FirstSequence > MaxSafeSequence || win.LastSequence > MaxSafeSequence {
		return newError(CodeOutboundWindow, "outbound window sequence exceeds MAX_SAFE_SEQUENCE", nil)
	}
	return nil
}

func (s *DiskSpool) persistOutboundWindowLocked(win outboundWindow) error {
	if err := s.validateOutboundWindowLocked(win); err != nil {
		return err
	}
	// Never silently replace a valid frozen range with a different/larger range.
	existing, exists, err := s.loadOutboundWindowLocked()
	if err != nil {
		return err
	}
	if exists {
		if existing.FirstSequence != win.FirstSequence || existing.LastSequence != win.LastSequence || existing.StreamID != win.StreamID {
			return newError(CodeOutboundWindow, "refusing to replace valid frozen outbound window", nil)
		}
		return nil
	}
	data, err := json.Marshal(win)
	if err != nil {
		return newError(CodeOutboundWindow, "failed to encode outbound window", err)
	}
	if len(data) > maxOutboundWindowBytes {
		return newError(CodeOutboundWindow, "encoded outbound window exceeds size bound", nil)
	}
	return atomicWriteFile(s.outboundWindowPath(), data, 0640)
}

func (s *DiskSpool) clearOutboundWindowLocked() error {
	path := s.outboundWindowPath()
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return newError(CodeIO, "failed to inspect outbound window sidecar for clear", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return newError(CodeUnsafePath, "refusing to clear unsafe outbound window path", nil)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return newError(CodeIO, "failed to clear outbound window sidecar", err)
	}
	return syncDir(s.dir)
}

func (s *DiskSpool) readExactRangeLocked(first, last uint64, maxRecords, maxPayloadBytes int) (Batch, error) {
	if last < first {
		return Batch{}, newError(CodeOutboundWindow, "outbound window range is impossible", nil)
	}
	count := int(last - first + 1)
	if count <= 0 || uint64(count) != last-first+1 {
		return Batch{}, newError(CodeOutboundWindow, "outbound window range overflows", nil)
	}
	_ = maxRecords
	_ = maxPayloadBytes
	batch := Batch{Records: make([]Record, 0, count)}
	for seq := first; seq <= last; seq++ {
		if seq <= s.state.AckedThrough {
			return Batch{}, newError(CodeOutboundWindow, "frozen outbound window includes already-acknowledged sequence", nil)
		}
		path := filepath.Join(s.dir, recordFileName(seq))
		rec, err := s.readRecordFileLocked(path, seq)
		if err != nil {
			return Batch{}, newError(CodeOutboundWindow, fmt.Sprintf("frozen outbound window missing or corrupt record %d", seq), err)
		}
		batch.Records = append(batch.Records, rec)
		batch.PayloadBytes += rec.ContentLength
		if batch.FirstSequence == 0 {
			batch.FirstSequence = rec.Sequence
		}
		batch.LastSequence = rec.Sequence
		if seq == last {
			break
		}
		if seq == MaxSafeSequence {
			return Batch{}, newError(CodeOutboundWindow, "outbound window sequence would wrap", nil)
		}
	}
	if batch.FirstSequence != first || batch.LastSequence != last || len(batch.Records) != count {
		return Batch{}, newError(CodeOutboundWindow, "reconstructed outbound window does not match sidecar range", nil)
	}
	return batch, nil
}

// readBatchLocked is the unlocked body of ReadBatch.
func (s *DiskSpool) readBatchLocked(maxRecords, maxPayloadBytes int) (Batch, error) {
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

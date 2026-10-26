package spool

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func ensureDirSecure(path string, mode os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return newError(CodeIO, "failed to inspect spool directory", err)
		}
		if err := os.MkdirAll(path, mode); err != nil {
			return newError(CodeIO, "failed to create spool directory", err)
		}
		info, err = os.Lstat(path)
		if err != nil {
			return newError(CodeIO, "failed to inspect created spool directory", err)
		}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return newError(CodeUnsafePath, "spool path must be a real directory, not a symlink or non-directory", nil)
	}
	return nil
}

func (s *DiskSpool) recover() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadStateLocked(); err != nil {
		return err
	}
	maxSeq, err := s.reconcileEntriesLocked(true)
	if err != nil {
		return err
	}
	floor := maxSeq + 1
	if s.state.AckedThrough+1 > floor {
		floor = s.state.AckedThrough + 1
	}
	if floor == 0 {
		floor = 1
	}
	if s.state.NextSequence < floor {
		s.state.NextSequence = floor
	}
	if s.state.NextSequence == 0 {
		s.state.NextSequence = 1
	}
	if err := s.persistStateLocked(); err != nil {
		return err
	}
	if err := syncDir(s.dir); err != nil {
		return err
	}
	s.cleanupUnresolved = false
	return nil
}

// reconcileEntriesLocked applies DiskSpool ownership rules to spool directory
// entries. includePending=true validates unacked records (Open recover).
// includePending=false leaves pending records unread so acked cleanup under
// backlog does not re-scan/re-parse the live queue. Record-like artifacts are
// never deleted solely from filename + watermark: symlinks/partials/invalid
// names and corrupt bodies are quarantined; only checksum-validated records at
// or below AckedThrough may be removed.
func (s *DiskSpool) reconcileEntriesLocked(includePending bool) (uint64, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, newError(CodeIO, "failed to scan spool directory", err)
	}
	var maxSeq uint64
	for _, entry := range entries {
		name := entry.Name()
		if name == stateFileName || name == streamIDFileName || name == quarantineDirName || name == outboundWindowFileName {
			continue
		}
		full := filepath.Join(s.dir, name)
		if entry.Type()&os.ModeSymlink != 0 {
			if err := s.quarantineLocked(full, "symlink"); err != nil {
				return 0, err
			}
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			if err := s.quarantineLocked(full, "partial"); err != nil {
				return 0, err
			}
			continue
		}
		seq, ok := parseRecordFileName(name)
		if !ok {
			if strings.HasPrefix(name, "record-") {
				if err := s.quarantineLocked(full, "invalid-name"); err != nil {
					return 0, err
				}
			}
			continue
		}
		if !includePending && seq > s.state.AckedThrough {
			// Pending live telemetry: leave intact without reading.
			continue
		}
		rec, err := s.readRecordFileLocked(full, seq)
		if err != nil {
			if qerr := s.quarantineLocked(full, "corrupt"); qerr != nil {
				return 0, qerr
			}
			continue
		}
		if rec.Sequence <= s.state.AckedThrough {
			if err := s.removeRecordFileLocked(full); err != nil && !os.IsNotExist(err) {
				return 0, newError(CodeIO, "failed to remove already acknowledged record during recovery", err)
			}
			continue
		}
		if rec.Sequence > maxSeq {
			maxSeq = rec.Sequence
		}
	}
	return maxSeq, nil
}

func (s *DiskSpool) loadOrCreateStreamID() error {
	path := filepath.Join(s.dir, streamIDFileName)
	info, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return newError(CodeIO, "failed to inspect spool stream identity", err)
		}
		random := make([]byte, 16)
		if _, err := rand.Read(random); err != nil {
			return newError(CodeIO, "failed to generate spool stream identity", err)
		}
		streamID := "stream-" + hex.EncodeToString(random)
		if err := atomicWriteFile(path, []byte(streamID+"\n"), 0640); err != nil {
			return err
		}
		s.streamID = streamID
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return newError(CodeUnsafePath, "spool stream identity must be a regular file", nil)
	}
	if info.Size() <= 0 || info.Size() > 128 {
		return newError(CodeCorruption, "spool stream identity has invalid size", nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return newError(CodeIO, "failed to read spool stream identity", err)
	}
	streamID := strings.TrimSpace(string(data))
	if len(streamID) != len("stream-")+32 || !strings.HasPrefix(streamID, "stream-") {
		return newError(CodeCorruption, "spool stream identity has invalid format", nil)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(streamID, "stream-")); err != nil {
		return newError(CodeCorruption, "spool stream identity has invalid encoding", err)
	}
	s.streamID = streamID
	return nil
}

func (s *DiskSpool) loadStateLocked() error {
	path := filepath.Join(s.dir, stateFileName)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.state = diskState{SchemaVersion: stateSchemaVersion, NextSequence: 1}
			return nil
		}
		return newError(CodeIO, "failed to inspect spool state", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return newError(CodeUnsafePath, "spool state must be a regular file", nil)
	}
	if info.Size() > 64*1024 {
		return newError(CodeCorruption, "spool state exceeds maximum size", nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return newError(CodeIO, "failed to read spool state", err)
	}
	var st diskState
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&st); err != nil {
		return newError(CodeCorruption, "spool state is malformed", err)
	}
	if st.SchemaVersion != stateSchemaVersion || st.NextSequence == 0 || st.AckedThrough >= st.NextSequence {
		return newError(CodeCorruption, "spool state contains invalid sequence metadata", nil)
	}
	if st.NextSequence > MaxSafeSequence+1 || st.AckedThrough > MaxSafeSequence {
		return newError(CodeCorruption, "spool state sequence exceeds MAX_SAFE_SEQUENCE", nil)
	}
	s.state = st
	return nil
}

func (s *DiskSpool) readRecordFileLocked(path string, expected uint64) (Record, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Record{}, newError(CodeIO, "failed to inspect spool record", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Record{}, newError(CodeUnsafePath, "spool record must be a regular file", nil)
	}
	maxEncoded := int64(s.maxRecordBytes*2 + 16*1024)
	if info.Size() <= 0 || info.Size() > maxEncoded {
		return Record{}, newError(CodeCorruption, "spool record encoded size is invalid", nil)
	}
	f, err := os.Open(path)
	if err != nil {
		return Record{}, newError(CodeIO, "failed to open spool record", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxEncoded+1))
	if err != nil {
		return Record{}, newError(CodeIO, "failed to read spool record", err)
	}
	if int64(len(data)) > maxEncoded {
		return Record{}, newError(CodeCorruption, "spool record exceeds encoded size bound", nil)
	}
	var rec Record
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rec); err != nil {
		return Record{}, newError(CodeCorruption, "spool record is malformed", err)
	}
	if rec.SchemaVersion != recordSchemaVersion || rec.Sequence != expected || rec.ID == "" || !validClass(rec.Class) || rec.Type == "" || rec.ContentLength != len(rec.Payload) || rec.ContentLength <= 0 || rec.ContentLength > s.maxRecordBytes {
		return Record{}, newError(CodeCorruption, "spool record metadata is invalid", nil)
	}
	sum := sha256.Sum256(rec.Payload)
	if rec.Checksum != hex.EncodeToString(sum[:]) {
		return Record{}, newError(CodeCorruption, "spool record checksum mismatch", nil)
	}
	return rec, nil
}

func (s *DiskSpool) persistStateLocked() error {
	s.state.SchemaVersion = stateSchemaVersion
	data, err := json.Marshal(s.state)
	if err != nil {
		return newError(CodeIO, "failed to encode spool state", err)
	}
	return atomicWriteFile(filepath.Join(s.dir, stateFileName), data, 0640)
}
func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".spool-*.tmp")
	if err != nil {
		return newError(CodeIO, "failed to create temporary spool file", err)
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
		return newError(CodeIO, "failed to set spool file permissions", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return newError(CodeIO, "failed to write spool file", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return newError(CodeIO, "failed to fsync spool file", err)
	}
	if err := tmp.Close(); err != nil {
		return newError(CodeIO, "failed to close spool file", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return newError(CodeIO, "failed to publish spool file atomically", err)
	}
	cleanup = false
	return syncDir(dir)
}
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return newError(CodeIO, "failed to open spool directory for fsync", err)
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return newError(CodeIO, "failed to fsync spool directory", err)
	}
	return nil
}
func (s *DiskSpool) quarantineLocked(path, reason string) error {
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) {
		return newError(CodeUnsafePath, "refusing unsafe quarantine path", nil)
	}
	target := filepath.Join(s.quarantineDir, fmt.Sprintf("%d-%s-%s", time.Now().UnixNano(), sanitizeReason(reason), base))
	if err := os.Rename(path, target); err != nil {
		return newError(CodeIO, "failed to quarantine corrupt spool entry", err)
	}
	s.quarantined++
	s.degraded = true
	if s.logger != nil {
		s.logger.Warn("sensor spool entry quarantined",
			"error_code", CodeCorruption,
			"safe_context", map[string]any{
				"reason":     sanitizeReason(reason),
				"entry":      base,
				"spool_type": string(s.kind),
			})
	}
	return syncDir(s.dir)
}
func sanitizeReason(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, s)
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}
func recordFileName(seq uint64) string { return fmt.Sprintf("record-%020d.json", seq) }
func parseRecordFileName(name string) (uint64, bool) {
	if !strings.HasPrefix(name, "record-") || !strings.HasSuffix(name, ".json") {
		return 0, false
	}
	body := strings.TrimSuffix(strings.TrimPrefix(name, "record-"), ".json")
	if len(body) != 20 {
		return 0, false
	}
	seq, err := strconv.ParseUint(body, 10, 64)
	return seq, err == nil && seq > 0
}
func (s *DiskSpool) filesystemStatus(int64) (spaceStatus, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.dir, &stat); err != nil {
		return spaceStatus{}, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	available := stat.Bavail * uint64(stat.Bsize)
	used := total - available
	pct := 0.0
	if total > 0 {
		pct = float64(used) * 100 / float64(total)
	}
	return spaceStatus{totalBytes: total, usedBytes: used, usedPct: pct}, nil
}

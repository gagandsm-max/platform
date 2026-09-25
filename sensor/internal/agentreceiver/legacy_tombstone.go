package agentreceiver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const legacyClosedDirName = "legacy-closed"

// LegacyTombstoneRecord permanently closes live v1 wire for a tenant+agent after modern ownership.
type LegacyTombstoneRecord struct {
	Version                 int       `json:"version"`
	TenantID                string    `json:"tenantId"`
	AgentID                 string    `json:"agentId"`
	ClosedToAgentStreamID   string    `json:"closedToAgentStreamId"`
	MigratedThroughSequence uint64    `json:"migratedThroughSequence"`
	Timestamp               time.Time `json:"timestamp"`
}

func (s *SpoolAcceptanceSink) tombstoneDirPath() string {
	return filepath.Join(s.stateDir, legacyClosedDirName)
}

func (s *SpoolAcceptanceSink) tombstonePath(tenantID, agentID string) string {
	return filepath.Join(s.tombstoneDirPath(), sourceRangeStreamKey(tenantID, agentID)+".json")
}

func (s *SpoolAcceptanceSink) ensureTombstoneDir() error {
	dir := s.tombstoneDirPath()
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("%s: failed to create legacy-closed directory: %w", ErrCodeSinkFailed, err)
	}
	if fi, err := os.Lstat(dir); err != nil {
		return fmt.Errorf("%s: failed to inspect legacy-closed directory: %w", ErrCodeSinkFailed, err)
	} else if fi.Mode()&os.ModeSymlink != 0 {
		return errors.New(ErrCodeSinkFailed + ": legacy-closed directory is a symlink")
	}
	return nil
}

func (s *SpoolAcceptanceSink) loadTombstonesLocked() (map[string]*LegacyTombstoneRecord, error) {
	dir := s.tombstoneDirPath()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*LegacyTombstoneRecord), nil
		}
		return nil, fmt.Errorf("%s: failed to list legacy-closed directory: %w", ErrCodeSinkFailed, err)
	}
	loaded := make(map[string]*LegacyTombstoneRecord)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to inspect legacy tombstone entry: %w", ErrCodeSinkFailed, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, errors.New(ErrCodeSinkFailed + ": unsafe legacy tombstone entry type")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to read legacy tombstone: %w", ErrCodeSinkFailed, err)
		}
		var tomb LegacyTombstoneRecord
		if err := decodeStrictJSONObject(data, &tomb); err != nil {
			return nil, fmt.Errorf("%s: corrupt legacy tombstone JSON: %w", ErrCodeSinkFailed, err)
		}
		if err := validateTombstoneRecord(tomb); err != nil {
			return nil, err
		}
		key := strings.TrimSuffix(entry.Name(), ".json")
		expected := sourceRangeStreamKey(tomb.TenantID, tomb.AgentID)
		if key != expected {
			return nil, fmt.Errorf("%s: legacy tombstone file hash does not match tenant/agent identity", ErrCodeSinkFailed)
		}
		cp := tomb
		loaded[key] = &cp
	}
	return loaded, nil
}

func validateTombstoneRecord(tomb LegacyTombstoneRecord) error {
	if tomb.Version != legacyTombstoneVersion {
		return fmt.Errorf("%s: unsupported legacy tombstone version %d", ErrCodeSinkFailed, tomb.Version)
	}
	if err := validateDurableIdentity("tenantId", tomb.TenantID); err != nil {
		return fmt.Errorf("%s: %w", ErrCodeSinkFailed, err)
	}
	if err := validateDurableIdentity("agentId", tomb.AgentID); err != nil {
		return fmt.Errorf("%s: %w", ErrCodeSinkFailed, err)
	}
	if !ValidAgentStreamID(tomb.ClosedToAgentStreamID) {
		return fmt.Errorf("%s: legacy tombstone closedToAgentStreamId grammar invalid", ErrCodeSinkFailed)
	}
	if tomb.Timestamp.IsZero() {
		return fmt.Errorf("%s: legacy tombstone timestamp must be set", ErrCodeSinkFailed)
	}
	return nil
}

func (s *SpoolAcceptanceSink) getTombstoneLocked(tenantID, agentID string) *LegacyTombstoneRecord {
	return s.tombstones[sourceRangeStreamKey(tenantID, agentID)]
}

func (s *SpoolAcceptanceSink) publishTombstoneLocked(tomb LegacyTombstoneRecord) error {
	if err := validateTombstoneRecord(tomb); err != nil {
		return err
	}
	key := sourceRangeStreamKey(tomb.TenantID, tomb.AgentID)
	path := s.tombstonePath(tomb.TenantID, tomb.AgentID)
	if err := atomicWriteJSONFile(s.tombstoneDirPath(), path, ".legacy-closed-*.tmp", tomb); err != nil {
		return fmt.Errorf("%s: %w", ErrCodeSinkFailed, err)
	}
	cp := tomb
	s.tombstones[key] = &cp
	return nil
}

func (s *SpoolAcceptanceSink) tombstoneLegacyIfNeededLocked(tenantID, agentID, closedToStreamID string) error {
	if s.getTombstoneLocked(tenantID, agentID) != nil {
		return nil
	}
	legacyKey := sourceRangeStreamKey(tenantID, agentID)
	legacyState := s.streams[legacyKey]
	migratedThrough := uint64(0)
	if legacyState != nil && strings.TrimSpace(legacyState.AgentStreamID) == "" {
		migratedThrough = legacyState.ContiguousAcceptedThrough
	} else if legacyState == nil {
		// No legacy durable state — still publish closure marker when establishing modern ownership
		// after historical legacy absence is fine; only required when legacy state exists.
		// Caller decides whether to invoke; this helper always writes when missing.
	}
	return s.publishTombstoneLocked(LegacyTombstoneRecord{
		Version:                 legacyTombstoneVersion,
		TenantID:                tenantID,
		AgentID:                 agentID,
		ClosedToAgentStreamID:   closedToStreamID,
		MigratedThroughSequence: migratedThrough,
		Timestamp:               time.Now().UTC(),
	})
}

// StreamStaleOrFencedError rejects a non-active modern stream before spool mutation.
type StreamStaleOrFencedError struct {
	TenantID            string
	AgentID             string
	AgentStreamID       string
	ActiveAgentStreamID string
	FirstSequence       uint64
	LastSequence        uint64
}

func (e *StreamStaleOrFencedError) Error() string {
	return ErrCodeStreamStaleOrFenced + ": agent stream is stale or fenced"
}

func IsStreamStaleOrFenced(err error) bool {
	var target *StreamStaleOrFencedError
	return errors.As(err, &target)
}

// LegacyStreamClosedError rejects live v1 after modern ownership/tombstone.
type LegacyStreamClosedError struct {
	TenantID string
	AgentID  string
}

func (e *LegacyStreamClosedError) Error() string {
	return ErrCodeLegacyStreamClosed + ": legacy v1 agent stream is closed"
}

func IsLegacyStreamClosed(err error) bool {
	var target *LegacyStreamClosedError
	return errors.As(err, &target)
}

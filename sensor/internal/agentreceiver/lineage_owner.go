package agentreceiver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const lineageOwnerDirName = "lineage-owner"

// LineageOwnerRecord is the durable single-active modern lineage owner for a tenant+agent.
type LineageOwnerRecord struct {
	Version                            int       `json:"version"`
	TenantID                           string    `json:"tenantId"`
	AgentID                            string    `json:"agentId"`
	ActiveAgentStreamID                string    `json:"activeAgentStreamId"`
	RetiredAgentStreamIDs              []string  `json:"retiredAgentStreamIds,omitempty"`
	ActivatedAt                        time.Time `json:"activatedAt"`
	ActivatedUnderAssignmentGeneration int64     `json:"activatedUnderAssignmentGeneration,omitempty"`
	PreviousAgentStreamID              string    `json:"previousAgentStreamId,omitempty"`
}

func (s *SpoolAcceptanceSink) ownerDirPath() string {
	return filepath.Join(s.stateDir, lineageOwnerDirName)
}

func (s *SpoolAcceptanceSink) ownerPath(tenantID, agentID string) string {
	return filepath.Join(s.ownerDirPath(), sourceRangeStreamKey(tenantID, agentID)+".json")
}

func (s *SpoolAcceptanceSink) ensureOwnerDir() error {
	dir := s.ownerDirPath()
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("%s: failed to create lineage owner directory: %w", ErrCodeStreamOwnerCorrupt, err)
	}
	if fi, err := os.Lstat(dir); err != nil {
		return fmt.Errorf("%s: failed to inspect lineage owner directory: %w", ErrCodeStreamOwnerCorrupt, err)
	} else if fi.Mode()&os.ModeSymlink != 0 {
		return errors.New(ErrCodeStreamOwnerCorrupt + ": lineage owner directory is a symlink")
	}
	return nil
}

func (s *SpoolAcceptanceSink) loadOwnersLocked() (map[string]*LineageOwnerRecord, error) {
	dir := s.ownerDirPath()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*LineageOwnerRecord), nil
		}
		return nil, fmt.Errorf("%s: failed to list lineage owner directory: %w", ErrCodeStreamOwnerCorrupt, err)
	}
	loaded := make(map[string]*LineageOwnerRecord)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to inspect lineage owner entry: %w", ErrCodeStreamOwnerCorrupt, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, errors.New(ErrCodeStreamOwnerCorrupt + ": unsafe lineage owner entry type")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to read lineage owner: %w", ErrCodeStreamOwnerCorrupt, err)
		}
		var owner LineageOwnerRecord
		if err := decodeStrictJSONObject(data, &owner); err != nil {
			return nil, fmt.Errorf("%s: corrupt lineage owner JSON: %w", ErrCodeStreamOwnerCorrupt, err)
		}
		if err := validateOwnerRecord(owner); err != nil {
			return nil, err
		}
		key := strings.TrimSuffix(entry.Name(), ".json")
		expected := sourceRangeStreamKey(owner.TenantID, owner.AgentID)
		if key != expected {
			return nil, fmt.Errorf("%s: lineage owner file hash does not match tenant/agent identity", ErrCodeStreamOwnerCorrupt)
		}
		cp := owner
		loaded[key] = &cp
	}
	return loaded, nil
}

func validateOwnerRecord(owner LineageOwnerRecord) error {
	if owner.Version != lineageOwnerVersion {
		return fmt.Errorf("%s: unsupported lineage owner version %d", ErrCodeStreamOwnerCorrupt, owner.Version)
	}
	if err := validateDurableIdentity("tenantId", owner.TenantID); err != nil {
		return fmt.Errorf("%s: %w", ErrCodeStreamOwnerCorrupt, err)
	}
	if err := validateDurableIdentity("agentId", owner.AgentID); err != nil {
		return fmt.Errorf("%s: %w", ErrCodeStreamOwnerCorrupt, err)
	}
	if !ValidAgentStreamID(owner.ActiveAgentStreamID) {
		return fmt.Errorf("%s: lineage owner activeAgentStreamId grammar invalid", ErrCodeStreamOwnerCorrupt)
	}
	if owner.PreviousAgentStreamID != "" && !ValidAgentStreamID(owner.PreviousAgentStreamID) {
		return fmt.Errorf("%s: lineage owner previousAgentStreamId grammar invalid", ErrCodeStreamOwnerCorrupt)
	}
	if owner.ActivatedAt.IsZero() {
		return fmt.Errorf("%s: lineage owner activatedAt must be set", ErrCodeStreamOwnerCorrupt)
	}
	if err := validateRetiredAgentStreamIDs(owner.ActiveAgentStreamID, owner.RetiredAgentStreamIDs); err != nil {
		return err
	}
	return nil
}

func validateRetiredAgentStreamIDs(active string, retired []string) error {
	if len(retired) > maxRetiredAgentStreamIDs {
		return fmt.Errorf("%s: retiredAgentStreamIds exceeds bound %d", ErrCodeStreamOwnerCorrupt, maxRetiredAgentStreamIDs)
	}
	seen := make(map[string]struct{}, len(retired))
	for _, id := range retired {
		if !ValidAgentStreamID(id) {
			return fmt.Errorf("%s: retiredAgentStreamIds contains invalid stream grammar", ErrCodeStreamOwnerCorrupt)
		}
		if id == active {
			return fmt.Errorf("%s: activeAgentStreamId must not appear in retiredAgentStreamIds", ErrCodeStreamOwnerCorrupt)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%s: retiredAgentStreamIds contains duplicate stream id", ErrCodeStreamOwnerCorrupt)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateDurableIdentity(field, val string) error {
	val = strings.TrimSpace(val)
	if val == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if len(val) > 128 {
		return fmt.Errorf("%s length %d exceeds maximum of 128", field, len(val))
	}
	return nil
}

func ownerHasRetiredStream(owner *LineageOwnerRecord, streamID string) bool {
	if owner == nil {
		return false
	}
	for _, id := range owner.RetiredAgentStreamIDs {
		if id == streamID {
			return true
		}
	}
	return false
}

// buildRetiredAgentStreamIDs constructs the durable retired set for an ownership transition.
// It includes prior retired IDs, the previous active stream, and any locally known modern
// stream states for the same tenant+agent except the new active candidate.
func buildRetiredAgentStreamIDs(previous *LineageOwnerRecord, tenantID, agentID, newActive string, streams map[string]*streamStateEntry) ([]string, error) {
	set := make(map[string]struct{})
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || id == newActive {
			return
		}
		set[id] = struct{}{}
	}
	if previous != nil {
		for _, id := range previous.RetiredAgentStreamIDs {
			add(id)
		}
		add(previous.ActiveAgentStreamID)
		add(previous.PreviousAgentStreamID)
	}
	for _, st := range streams {
		if st == nil {
			continue
		}
		if st.TenantID != tenantID || st.AgentID != agentID {
			continue
		}
		if strings.TrimSpace(st.AgentStreamID) == "" {
			continue
		}
		add(st.AgentStreamID)
	}
	if len(set) > maxRetiredAgentStreamIDs {
		return nil, fmt.Errorf("%s: retiring stream would exceed retiredAgentStreamIds bound %d", ErrCodeStreamOwnerCorrupt, maxRetiredAgentStreamIDs)
	}
	out := make([]string, 0, len(set))
	for id := range set {
		if !ValidAgentStreamID(id) {
			return nil, fmt.Errorf("%s: retired candidate stream grammar invalid", ErrCodeStreamOwnerCorrupt)
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func (s *SpoolAcceptanceSink) getOwnerLocked(tenantID, agentID string) *LineageOwnerRecord {
	return s.owners[sourceRangeStreamKey(tenantID, agentID)]
}

func (s *SpoolAcceptanceSink) publishOwnerLocked(owner LineageOwnerRecord) error {
	if err := validateOwnerRecord(owner); err != nil {
		s.reconcileRequired = true
		return err
	}
	key := sourceRangeStreamKey(owner.TenantID, owner.AgentID)
	path := s.ownerPath(owner.TenantID, owner.AgentID)
	write := atomicWriteJSONFile
	if s.ownerPublisher != nil {
		write = s.ownerPublisher
	}
	if err := write(s.ownerDirPath(), path, ".lineage-owner-*.tmp", owner); err != nil {
		// Rename may have succeeded while dirsync/hook failed — reload disk before next accept.
		s.reconcileRequired = true
		return fmt.Errorf("%s: %w", ErrCodeStreamOwnerCorrupt, err)
	}
	cp := owner
	s.owners[key] = &cp
	return nil
}

// SetOwnerPublisherForTest replaces durable owner publication (temp/fsync/rename/dirsync).
func (s *SpoolAcceptanceSink) SetOwnerPublisherForTest(fn func(dir, path, tmpPattern string, v any) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ownerPublisher = fn
}

// RetiredStreamIDsForTest returns a copy of the durable retired set for tests.
func (s *SpoolAcceptanceSink) RetiredStreamIDsForTest(tenantID, agentID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	owner := s.getOwnerLocked(tenantID, agentID)
	if owner == nil {
		return nil
	}
	out := make([]string, len(owner.RetiredAgentStreamIDs))
	copy(out, owner.RetiredAgentStreamIDs)
	return out
}

// ReconcileRequiredForTest exposes reconcileRequired for uncertainty tests.
func (s *SpoolAcceptanceSink) ReconcileRequiredForTest() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reconcileRequired
}

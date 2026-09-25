package agentreceiver

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		if err := json.Unmarshal(data, &owner); err != nil {
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
	if strings.TrimSpace(owner.TenantID) == "" || strings.TrimSpace(owner.AgentID) == "" {
		return fmt.Errorf("%s: lineage owner missing tenant or agent identity", ErrCodeStreamOwnerCorrupt)
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
	return nil
}

func (s *SpoolAcceptanceSink) getOwnerLocked(tenantID, agentID string) *LineageOwnerRecord {
	return s.owners[sourceRangeStreamKey(tenantID, agentID)]
}

func (s *SpoolAcceptanceSink) publishOwnerLocked(owner LineageOwnerRecord) error {
	if err := validateOwnerRecord(owner); err != nil {
		return err
	}
	key := sourceRangeStreamKey(owner.TenantID, owner.AgentID)
	path := s.ownerPath(owner.TenantID, owner.AgentID)
	if err := atomicWriteJSONFile(s.ownerDirPath(), path, ".lineage-owner-*.tmp", owner); err != nil {
		return fmt.Errorf("%s: %w", ErrCodeStreamOwnerCorrupt, err)
	}
	cp := owner
	s.owners[key] = &cp
	return nil
}

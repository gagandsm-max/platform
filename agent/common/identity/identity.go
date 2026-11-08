package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const identityFileName = "identity.json"
const maxIdentityBytes = 16 * 1024

type AgentIdentity struct {
	AgentID  string `json:"agentId"`
	TenantID string `json:"tenantId"`
	HostID   string `json:"hostId"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

// GenerateOrLoadIdentity maintains backward-compatibility for simple tests while
// enforcing complete AgentIdentity fields.
func GenerateOrLoadIdentity(configuredID string) (*AgentIdentity, error) {
	return LoadOrGenerate("", configuredID, "tenant-default")
}

// LoadOrGenerate loads an existing persisted identity or generates a new one.
// If stateDir is empty, an in-memory identity is generated without disk persistence.
func LoadOrGenerate(stateDir, configuredID, tenantID string) (*AgentIdentity, error) {
	return loadOrGenerate(stateDir, configuredID, tenantID, syncIdentityDir)
}

func loadOrGenerate(stateDir, configuredID, tenantID string, syncDir func(string) error) (*AgentIdentity, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, errors.New("IDENTITY_TENANT_REQUIRED: tenantId must be provided")
	}

	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown-host"
	}
	hostname = strings.TrimSpace(hostname)

	hostID := detectHostID(hostname)

	if stateDir == "" {
		agentID := strings.TrimSpace(configuredID)
		if agentID == "" {
			var genErr error
			agentID, genErr = generateRandomID("agent")
			if genErr != nil {
				return nil, genErr
			}
		}
		return &AgentIdentity{
			AgentID:  agentID,
			TenantID: tenantID,
			HostID:   hostID,
			Hostname: hostname,
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
		}, nil
	}

	// Disk-backed persistence path
	absDir, err := filepath.Abs(stateDir)
	if err != nil {
		return nil, fmt.Errorf("IDENTITY_PATH_INVALID: %w", err)
	}
	if err := validateIdentityAncestors(absDir); err != nil {
		return nil, err
	}

	// Verify directory exists or create with 0700
	if fi, err := os.Lstat(absDir); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("IDENTITY_SYMLINK_REJECTED: state directory is a symlink")
		}
		if !fi.IsDir() {
			return nil, errors.New("IDENTITY_DIR_INVALID: state path is not a directory")
		}
		if fi.Mode().Perm()&0077 != 0 {
			if err := os.Chmod(absDir, 0700); err != nil {
				return nil, fmt.Errorf("IDENTITY_CHMOD_FAILED: %w", err)
			}
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(absDir, 0700); err != nil {
			return nil, fmt.Errorf("IDENTITY_MKDIR_FAILED: %w", err)
		}
	} else {
		return nil, fmt.Errorf("IDENTITY_STAT_FAILED: %w", err)
	}

	idPath := filepath.Join(absDir, identityFileName)

	// Check if file already exists
	if fi, err := os.Lstat(idPath); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("IDENTITY_SYMLINK_REJECTED: identity file is a symlink")
		}
		if !fi.Mode().IsRegular() {
			return nil, errors.New("IDENTITY_FILE_INVALID: identity file is not a regular file")
		}
		if runtime.GOOS == "linux" && fi.Mode().Perm()&0077 != 0 {
			return nil, errors.New("IDENTITY_PERMISSIONS: identity file must be private")
		}
		if fi.Size() > maxIdentityBytes {
			return nil, errors.New("IDENTITY_TOO_LARGE: identity metadata exceeds byte limit")
		}
		f, err := openIdentityFile(idPath)
		if err != nil {
			return nil, fmt.Errorf("IDENTITY_READ_FAILED: %w", err)
		}
		opened, statErr := f.Stat()
		if statErr != nil || !os.SameFile(fi, opened) || !opened.Mode().IsRegular() || (runtime.GOOS == "linux" && opened.Mode().Perm()&0077 != 0) || !singleIdentityLink(opened) {
			_ = f.Close()
			return nil, errors.New("IDENTITY_FILE_CHANGED: identity metadata is unsafe")
		}
		data, err := io.ReadAll(io.LimitReader(f, maxIdentityBytes+1))
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("IDENTITY_READ_FAILED: %w", err)
		}
		if len(data) > maxIdentityBytes {
			return nil, errors.New("IDENTITY_TOO_LARGE: identity metadata exceeds byte limit")
		}

		var existing AgentIdentity
		if err := json.Unmarshal(data, &existing); err != nil {
			return nil, fmt.Errorf("IDENTITY_CORRUPT: failed to parse identity json: %w", err)
		}

		existing.AgentID = strings.TrimSpace(existing.AgentID)
		existing.TenantID = strings.TrimSpace(existing.TenantID)
		existing.HostID = strings.TrimSpace(existing.HostID)

		if existing.AgentID == "" || existing.TenantID == "" || existing.HostID == "" {
			return nil, errors.New("IDENTITY_CORRUPT: missing required identity fields")
		}

		if existing.TenantID != tenantID {
			return nil, fmt.Errorf("IDENTITY_TENANT_MISMATCH: stored tenant %s does not match configured %s", existing.TenantID, tenantID)
		}

		if configuredID = strings.TrimSpace(configuredID); configuredID != "" && configuredID != existing.AgentID {
			return nil, fmt.Errorf("IDENTITY_AGENT_ID_MISMATCH: stored agent id %s does not match configured %s", existing.AgentID, configuredID)
		}

		// Keep runtime OS/Arch fresh, preserve persisted IDs
		existing.Hostname = hostname
		existing.OS = runtime.GOOS
		existing.Arch = runtime.GOARCH
		// Also reconcile a prior publication whose directory sync failed.
		if err := syncDir(absDir); err != nil {
			return nil, err
		}
		return &existing, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("IDENTITY_STAT_FAILED: %w", err)
	}

	// File does not exist: generate fresh identity and atomically persist
	agentID := strings.TrimSpace(configuredID)
	if agentID == "" {
		var genErr error
		agentID, genErr = generateRandomID("agent")
		if genErr != nil {
			return nil, genErr
		}
	}

	newIdent := &AgentIdentity{
		AgentID:  agentID,
		TenantID: tenantID,
		HostID:   hostID,
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
	}

	payload, err := json.MarshalIndent(newIdent, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("IDENTITY_MARSHAL_FAILED: %w", err)
	}
	if len(payload) > maxIdentityBytes {
		return nil, errors.New("IDENTITY_TOO_LARGE: identity metadata exceeds byte limit")
	}

	// Atomic write: write to temp file in same directory, then rename
	tmpFile, err := os.CreateTemp(absDir, "identity.*.tmp")
	if err != nil {
		return nil, fmt.Errorf("IDENTITY_TMP_CREATE_FAILED: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("IDENTITY_CHMOD_FAILED: %w", err)
	}

	if _, err := tmpFile.Write(payload); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("IDENTITY_WRITE_FAILED: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("IDENTITY_SYNC_FAILED: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("IDENTITY_CLOSE_FAILED: %w", err)
	}

	if err := publishIdentity(tmpName, idPath); err != nil {
		return nil, fmt.Errorf("IDENTITY_PUBLISH_FAILED: %w", err)
	}
	if err := syncDir(absDir); err != nil {
		return nil, err
	}

	return newIdent, nil
}

func detectHostID(hostname string) string {
	// 1. Try reading Linux machine-id
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if content, err := os.ReadFile(path); err == nil {
			trimmed := strings.TrimSpace(string(content))
			if len(trimmed) >= 16 {
				return "host-" + trimmed[:16]
			}
		}
	}

	// 2. Stable fallback: hash hostname and OS details
	h := sha256.New()
	h.Write([]byte(hostname))
	h.Write([]byte(runtime.GOOS))
	h.Write([]byte(runtime.GOARCH))
	digest := hex.EncodeToString(h.Sum(nil))
	return "host-" + digest[:16]
}

func generateRandomID(prefix string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("IDENTITY_ID_GENERATION_FAILED: %w", err)
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b)), nil
}

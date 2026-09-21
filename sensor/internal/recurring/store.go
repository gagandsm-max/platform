package recurring

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	ErrCodeDurableWrite      = "SENSOR_RECURRING_DURABLE_WRITE_FAILED"
	ErrCodeRestartFailClosed = "SENSOR_RECURRING_RESTART_FAIL_CLOSED"
	ErrCodeInvalidConfig     = "SENSOR_RECURRING_INVALID_CONFIG"

	appliedStateFile = "applied.json"
	stateDirName     = "state"
)

// Store is a filesystem durable store for applied recurring state.
type Store struct {
	Dir string
}

// NewStore validates absolute Dir and ensures state directory exists.
func NewStore(dir string) (*Store, error) {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("%s: Dir must be an absolute path", ErrCodeInvalidConfig)
	}
	s := &Store{Dir: dir}
	if err := os.MkdirAll(filepath.Join(dir, stateDirName), 0o700); err != nil {
		return nil, fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	return s, nil
}

func (s *Store) appliedPath() string {
	return filepath.Join(s.Dir, stateDirName, appliedStateFile)
}

// SaveApplied atomically persists applied state.
func (s *Store) SaveApplied(state AppliedState) error {
	if state.Definitions == nil {
		state.Definitions = []RecurringDefinition{}
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	return atomicWriteFile(s.appliedPath(), payload, 0o600)
}

// LoadApplied loads applied.json. Missing file returns empty generation-0 state
// with present=false. Symlink/non-regular/oversized/malformed/checksum mismatch
// fail closed.
func (s *Store) LoadApplied() (state AppliedState, present bool, err error) {
	path := s.appliedPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return AppliedState{Generation: 0, Definitions: []RecurringDefinition{}}, false, nil
	}
	if err != nil {
		return AppliedState{}, false, fmt.Errorf("%s: %w", ErrCodeRestartFailClosed, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return AppliedState{}, false, fmt.Errorf("%s: applied.json must be a regular non-symlink file", ErrCodeRestartFailClosed)
	}
	if info.Size() < 0 || info.Size() > MaxAppliedStateBytes {
		return AppliedState{}, false, fmt.Errorf("%s: applied.json exceeds size limit", ErrCodeRestartFailClosed)
	}

	data, err := readRegularFileLimited(path, MaxAppliedStateBytes)
	if err != nil {
		return AppliedState{}, false, err
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return AppliedState{}, false, fmt.Errorf("%s: malformed applied.json", ErrCodeRestartFailClosed)
	}
	if state.Definitions == nil {
		state.Definitions = []RecurringDefinition{}
	}
	if err := validateAppliedInvariants(state); err != nil {
		return AppliedState{}, false, err
	}
	return state, true, nil
}

func validateAppliedInvariants(state AppliedState) error {
	if state.Generation < 0 {
		return fmt.Errorf("%s: negative applied generation", ErrCodeRestartFailClosed)
	}
	if len(state.Definitions) > MaxDefinitions {
		return fmt.Errorf("%s: definition count exceeds max", ErrCodeRestartFailClosed)
	}
	if state.Generation == 0 {
		if len(state.Definitions) != 0 || state.Checksum != "" {
			return fmt.Errorf("%s: generation 0 requires empty definitions and checksum", ErrCodeRestartFailClosed)
		}
		return nil
	}
	computed := ComputeDesiredChecksum(state.Definitions)
	if computed != state.Checksum {
		return fmt.Errorf("%s: applied state checksum mismatch", ErrCodeRestartFailClosed)
	}
	for i := range state.Definitions {
		if err := ValidateDefinition(state.Definitions[i]); err != nil {
			return fmt.Errorf("%s: %w", ErrCodeRestartFailClosed, err)
		}
	}
	return nil
}

func atomicWriteFile(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
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
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	cleanup = false
	return syncDir(dir)
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	return nil
}

func readRegularFileLimited(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s: refusing symlink", ErrCodeRestartFailClosed)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", ErrCodeRestartFailClosed)
	}
	if info.Size() < 0 || info.Size() > maxBytes {
		return nil, fmt.Errorf("%s: exceeds size limit", ErrCodeRestartFailClosed)
	}

	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrCodeRestartFailClosed, err)
	}
	defer file.Close()

	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrCodeRestartFailClosed, err)
	}
	if !openedInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", ErrCodeRestartFailClosed)
	}
	if openedInfo.Size() < 0 || openedInfo.Size() > maxBytes {
		return nil, fmt.Errorf("%s: exceeds size limit", ErrCodeRestartFailClosed)
	}

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrCodeRestartFailClosed, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s: exceeds size limit", ErrCodeRestartFailClosed)
	}
	return data, nil
}

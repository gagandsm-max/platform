package packsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"platform/sensor/internal/coreclient"
)

const (
	ErrCodeInvalidConfig       = "SENSOR_PACK_INVALID_CONFIG"
	ErrCodeStateRegression     = "SENSOR_PACK_STATE_REGRESSION"
	ErrCodeStateConflict       = "SENSOR_PACK_STATE_CONFLICT"
	ErrCodeArtifactVerify      = "SENSOR_PACK_ARTIFACT_VERIFY_FAILED"
	ErrCodeDurableWrite        = "SENSOR_PACK_DURABLE_WRITE_FAILED"
	ErrCodeRestartFailClosed   = "SENSOR_PACK_RESTART_FAIL_CLOSED"
	ErrCodeAuthFailed          = "SENSOR_PACK_AUTH_FAILED"
	ErrCodeSyncTransient       = "SENSOR_PACK_SYNC_TRANSIENT"

	appliedStateFile            = "applied.json"
	artifactsDirName            = "artifacts"
	stateDirName                = "state"
	maxPackCount                = 256
	maxAppliedStateBytes  int64 = 512 * 1024
	schemaVersion               = "1.0.0"
)

// CorePackClient is the narrow Core surface used by pack sync.
type CorePackClient interface {
	FetchCapabilityPackDesiredState(ctx context.Context) (coreclient.CapabilityPackDesiredState, error)
	FetchCapabilityPackArtifact(ctx context.Context, packID, packVersion string, maxBytes int) (coreclient.CapabilityPackArtifact, error)
	AckCapabilityPackAppliedState(ctx context.Context, generation int64, checksum string) (coreclient.AppliedStateAck, error)
}

type Config struct {
	Dir          string
	PollInterval time.Duration
	Logger       *slog.Logger
}

type AppliedState struct {
	Generation int64             `json:"generation"`
	Checksum   string            `json:"checksum,omitempty"`
	Packs      []AppliedPackRef  `json:"packs"`
}

type AppliedPackRef struct {
	PackID         string `json:"packId"`
	PackVersion    string `json:"packVersion"`
	ManifestSHA256 string `json:"manifestSha256"`
}

type Status struct {
	DesiredGeneration  int64
	AppliedGeneration  int64
	PackCount          int
	LastSuccessfulSync time.Time
	LastErrorCode      string
	Degraded           bool
	// TrustedApplied is true only after successful recoverOnStart (empty gen0 or
	// fully validated applied state) or successful setSuccess after apply.
	// False when recovery fail-closed; transient sync errors do not clear it.
	TrustedApplied bool
}

type Syncer struct {
	cfg    Config
	client CorePackClient
	mu     sync.RWMutex
	status Status

	stop chan struct{}
	done chan struct{}
}

func New(cfg Config, client CorePackClient) (*Syncer, error) {
	if client == nil {
		return nil, fmt.Errorf("%s: core client is required", ErrCodeInvalidConfig)
	}
	dir := filepath.Clean(strings.TrimSpace(cfg.Dir))
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("%s: CapabilityPackDir must be an absolute path", ErrCodeInvalidConfig)
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	cfg.Dir = dir
	s := &Syncer{cfg: cfg, client: client, stop: make(chan struct{}), done: make(chan struct{})}
	if err := s.ensureDirs(); err != nil {
		return nil, err
	}
	if err := s.recoverOnStart(); err != nil {
		s.setError(ErrCodeRestartFailClosed, true)
		s.cfg.Logger.Error("capability pack restart recovery failed closed", "error_code", ErrCodeRestartFailClosed, "error", err)
		// Keep running; do not corrupt telemetry. Status is DEGRADED.
	}
	return s, nil
}

func (s *Syncer) Start(ctx context.Context) {
	go s.loop(ctx)
}

func (s *Syncer) Shutdown(ctx context.Context) error {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Syncer) Snapshot() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *Syncer) loop(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	s.syncOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case <-ticker.C:
			s.syncOnce(ctx)
		}
	}
}

func (s *Syncer) syncOnce(ctx context.Context) {
	desired, err := s.client.FetchCapabilityPackDesiredState(ctx)
	if err != nil {
		code := ErrCodeSyncTransient
		degraded := false
		if coreclient.IsCode(err, coreclient.CodeAuthentication) || coreclient.IsCode(err, coreclient.CodeAuthorization) {
			code = ErrCodeAuthFailed
			degraded = true
			// Fail closed without clearing applied.
		}
		s.setError(code, degraded)
		s.cfg.Logger.Warn("capability pack desired-state fetch failed", "error_code", code, "error", err)
		return
	}

	applied, appliedErr := s.loadApplied()
	if appliedErr != nil {
		s.setError(ErrCodeRestartFailClosed, true)
		return
	}
	if err := validateAppliedInvariants(applied); err != nil {
		s.setError(ErrCodeRestartFailClosed, true)
		s.cfg.Logger.Error("capability pack applied invariants failed during sync",
			"error_code", ErrCodeRestartFailClosed, "error", err)
		return
	}

	s.mu.Lock()
	s.status.DesiredGeneration = desired.Generation
	s.mu.Unlock()

	if desired.Generation == applied.Generation && strings.EqualFold(desired.Checksum, applied.Checksum) {
		// Local durable state is current. Core also reports the last applied state it
		// has durably recorded, allowing the Sensor to repair a lost ACK or a later
		// Core restore/regression without periodic blind re-ACK traffic.
		//
		// Generation+checksum match alone must not publish TrustedApplied when restart
		// recovery already failed closed: re-validate artifacts before setSuccess.
		if !s.Snapshot().TrustedApplied {
			if err := s.verifyAppliedArtifacts(applied); err != nil {
				s.setError(ErrCodeRestartFailClosed, true)
				s.cfg.Logger.Error("capability pack fast-path artifact revalidation failed",
					"error_code", ErrCodeRestartFailClosed, "error", err)
				return
			}
		}
		coreAppliedMatches := desired.AppliedGeneration == applied.Generation &&
			strings.EqualFold(desired.AppliedChecksum, applied.Checksum)
		if desired.Generation > 0 && !coreAppliedMatches {
			if err := s.ackApplied(ctx, desired.Generation, desired.Checksum); err != nil {
				s.setError(errorCode(err), true)
				s.cfg.Logger.Warn("capability pack applied-state ACK retry failed",
					"error_code", errorCode(err), "generation", desired.Generation, "error", err)
				return
			}
		}
		s.setSuccess(desired.Generation, len(desired.Packs))
		return
	}
	if desired.Generation < applied.Generation {
		s.setError(ErrCodeStateRegression, true)
		s.cfg.Logger.Error("capability pack desired generation regression",
			"error_code", ErrCodeStateRegression,
			"desired", desired.Generation, "applied", applied.Generation)
		return
	}
	if desired.Generation == applied.Generation && desired.Checksum != applied.Checksum {
		s.setError(ErrCodeStateConflict, true)
		s.cfg.Logger.Error("capability pack same-generation checksum conflict",
			"error_code", ErrCodeStateConflict)
		return
	}

	if err := s.applyDesired(ctx, desired); err != nil {
		s.setError(errorCode(err), true)
		s.cfg.Logger.Warn("capability pack apply failed; retaining prior applied state",
			"error_code", errorCode(err), "error", err)
		return
	}
	s.setSuccess(desired.Generation, len(desired.Packs))
}

func (s *Syncer) applyDesired(ctx context.Context, desired coreclient.CapabilityPackDesiredState) error {
	if len(desired.Packs) > maxPackCount {
		return fmt.Errorf("%s: pack count exceeds max", ErrCodeArtifactVerify)
	}

	staging := make([]AppliedPackRef, 0, len(desired.Packs))
	seenID := map[string]struct{}{}
	seenSHA := map[string]struct{}{}
	for _, p := range desired.Packs {
		if p.PackID == "" || p.PackVersion == "" || !isLowerHex64(p.ManifestSHA256) {
			return fmt.Errorf("%s: desired pack ref invalid", ErrCodeArtifactVerify)
		}
		if _, dup := seenID[p.PackID]; dup {
			return fmt.Errorf("%s: duplicate pack id", ErrCodeArtifactVerify)
		}
		seenID[p.PackID] = struct{}{}
		if _, dup := seenSHA[p.ManifestSHA256]; dup {
			return fmt.Errorf("%s: duplicate artifact sha", ErrCodeArtifactVerify)
		}
		seenSHA[p.ManifestSHA256] = struct{}{}
		staging = append(staging, AppliedPackRef{
			PackID: p.PackID, PackVersion: p.PackVersion, ManifestSHA256: p.ManifestSHA256,
		})
	}
	if computed := ComputeDesiredChecksum(staging); computed != desired.Checksum {
		return fmt.Errorf("%s: desired checksum does not match pack set", ErrCodeArtifactVerify)
	}

	for i, p := range desired.Packs {
		art, err := s.client.FetchCapabilityPackArtifact(ctx, p.PackID, p.PackVersion, p.ArtifactBytes)
		if err != nil {
			return err
		}
		if art.ManifestSHA256 != p.ManifestSHA256 {
			return fmt.Errorf("%s: artifact sha mismatch for %s", ErrCodeArtifactVerify, p.PackID)
		}
		if len(art.Bytes) != p.ArtifactBytes && p.ArtifactBytes > 0 {
			return fmt.Errorf("%s: artifact size mismatch for %s", ErrCodeArtifactVerify, p.PackID)
		}
		if err := verifyEnvelope(art.Bytes, p.PackID, p.PackVersion); err != nil {
			return err
		}
		if err := s.writeArtifactDurable(art.ManifestSHA256, art.Bytes); err != nil {
			return err
		}
		staging[i].ManifestSHA256 = art.ManifestSHA256
	}

	next := AppliedState{
		Generation: desired.Generation,
		Checksum:   desired.Checksum,
		Packs:      staging,
	}
	if err := s.writeAppliedDurable(next); err != nil {
		return err
	}
	// ACK only after durable applied.json commit.
	if err := s.ackApplied(ctx, desired.Generation, desired.Checksum); err != nil {
		return err
	}
	s.gcUnreferenced(seenSHA)
	return nil
}

func verifyEnvelope(raw []byte, packID, packVersion string) error {
	if len(raw) == 0 || len(raw) > coreclient.MaxCapabilityPackArtifactBytes {
		return fmt.Errorf("%s: artifact size out of bounds", ErrCodeArtifactVerify)
	}
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		PackID        string `json:"packId"`
		PackVersion   string `json:"packVersion"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("%s: artifact JSON invalid", ErrCodeArtifactVerify)
	}
	if envelope.SchemaVersion != schemaVersion {
		return fmt.Errorf("%s: unsupported schemaVersion", ErrCodeArtifactVerify)
	}
	if envelope.PackID != packID || envelope.PackVersion != packVersion {
		return fmt.Errorf("%s: envelope identity mismatch", ErrCodeArtifactVerify)
	}
	return nil
}

func (s *Syncer) writeArtifactDurable(sha string, data []byte) error {
	dir := filepath.Join(s.cfg.Dir, artifactsDirName)
	final := filepath.Join(dir, sha+".json")
	if _, err := os.Lstat(final); err == nil {
		existing, err := readRegularFileLimited(final, int64(coreclient.MaxCapabilityPackArtifactBytes), ErrCodeArtifactVerify)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(existing)
		if hex.EncodeToString(sum[:]) != sha {
			return fmt.Errorf("%s: existing artifact checksum mismatch", ErrCodeArtifactVerify)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	return atomicWriteFile(final, data, 0o600)
}

func (s *Syncer) writeAppliedDurable(state AppliedState) error {
	path := filepath.Join(s.cfg.Dir, stateDirName, appliedStateFile)
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrCodeDurableWrite, err)
	}
	return atomicWriteFile(path, payload, 0o600)
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

func (s *Syncer) loadApplied() (AppliedState, error) {
	path := filepath.Join(s.cfg.Dir, stateDirName, appliedStateFile)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return AppliedState{Generation: 0, Packs: []AppliedPackRef{}}, nil
	}
	if err != nil {
		return AppliedState{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return AppliedState{}, fmt.Errorf("%s: applied.json must be a regular non-symlink file", ErrCodeRestartFailClosed)
	}
	if info.Size() < 0 || info.Size() > maxAppliedStateBytes {
		return AppliedState{}, fmt.Errorf("%s: applied.json exceeds size limit", ErrCodeRestartFailClosed)
	}

	file, err := os.Open(path)
	if err != nil {
		return AppliedState{}, err
	}
	defer file.Close()

	openedInfo, err := file.Stat()
	if err != nil {
		return AppliedState{}, err
	}
	if !openedInfo.Mode().IsRegular() || openedInfo.Size() < 0 || openedInfo.Size() > maxAppliedStateBytes {
		return AppliedState{}, fmt.Errorf("%s: applied.json changed or exceeds size limit", ErrCodeRestartFailClosed)
	}

	data, err := io.ReadAll(io.LimitReader(file, maxAppliedStateBytes+1))
	if err != nil {
		return AppliedState{}, err
	}
	if int64(len(data)) > maxAppliedStateBytes {
		return AppliedState{}, fmt.Errorf("%s: applied.json exceeds size limit", ErrCodeRestartFailClosed)
	}

	var state AppliedState
	if err := json.Unmarshal(data, &state); err != nil {
		return AppliedState{}, fmt.Errorf("%s: malformed applied.json", ErrCodeRestartFailClosed)
	}
	return state, nil
}

func (s *Syncer) ackApplied(ctx context.Context, generation int64, checksum string) error {
	if generation == 0 {
		return nil
	}
	_, err := s.client.AckCapabilityPackAppliedState(ctx, generation, checksum)
	return err
}

func (s *Syncer) recoverOnStart() error {
	state, err := s.loadApplied()
	if err != nil {
		return err
	}
	return s.trustAppliedState(state)
}

// trustAppliedState validates applied.json invariants and on-disk artifacts, then
// publishes AppliedGeneration / PackCount / TrustedApplied. Used by recoverOnStart.
func (s *Syncer) trustAppliedState(state AppliedState) error {
	if err := validateAppliedInvariants(state); err != nil {
		return err
	}
	if err := s.verifyAppliedArtifacts(state); err != nil {
		return err
	}
	s.mu.Lock()
	s.status.AppliedGeneration = state.Generation
	s.status.PackCount = len(state.Packs)
	s.status.TrustedApplied = true
	s.mu.Unlock()
	return nil
}

// verifyAppliedArtifacts confirms every applied pack ref has a readable regular
// artifact whose content hash and envelope match the durable reference.
func (s *Syncer) verifyAppliedArtifacts(state AppliedState) error {
	for _, p := range state.Packs {
		path := filepath.Join(s.cfg.Dir, artifactsDirName, p.ManifestSHA256+".json")
		raw, err := readRegularFileLimited(path, int64(coreclient.MaxCapabilityPackArtifactBytes), ErrCodeRestartFailClosed)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%s: missing artifact %s", ErrCodeRestartFailClosed, p.ManifestSHA256)
			}
			return err
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != p.ManifestSHA256 {
			return fmt.Errorf("%s: artifact checksum mismatch", ErrCodeRestartFailClosed)
		}
		if err := verifyEnvelope(raw, p.PackID, p.PackVersion); err != nil {
			return fmt.Errorf("%s: artifact envelope invalid", ErrCodeRestartFailClosed)
		}
	}
	return nil
}

// validateAppliedInvariants checks structural rules and recomputes the canonical
// state checksum before any AppliedGeneration / TrustedApplied publication.
func validateAppliedInvariants(state AppliedState) error {
	if state.Generation < 0 {
		return fmt.Errorf("%s: negative applied generation", ErrCodeRestartFailClosed)
	}
	if len(state.Packs) > maxPackCount {
		return fmt.Errorf("%s: pack count exceeds max", ErrCodeRestartFailClosed)
	}
	if state.Generation == 0 {
		if len(state.Packs) != 0 || state.Checksum != "" {
			return fmt.Errorf("%s: generation 0 requires empty packs and checksum", ErrCodeRestartFailClosed)
		}
		return nil
	}
	if len(state.Packs) == 0 {
		if state.Checksum != "" {
			return fmt.Errorf("%s: empty pack set requires empty checksum", ErrCodeRestartFailClosed)
		}
	} else if !isLowerHex64(state.Checksum) {
		return fmt.Errorf("%s: applied checksum is not lowercase 64-hex", ErrCodeRestartFailClosed)
	}

	seenID := map[string]struct{}{}
	seenSHA := map[string]struct{}{}
	for _, p := range state.Packs {
		if p.PackID == "" || p.PackVersion == "" || !isLowerHex64(p.ManifestSHA256) {
			return fmt.Errorf("%s: applied pack ref invalid", ErrCodeRestartFailClosed)
		}
		if _, dup := seenID[p.PackID]; dup {
			return fmt.Errorf("%s: duplicate pack id", ErrCodeRestartFailClosed)
		}
		seenID[p.PackID] = struct{}{}
		if _, dup := seenSHA[p.ManifestSHA256]; dup {
			return fmt.Errorf("%s: duplicate applied artifact", ErrCodeRestartFailClosed)
		}
		seenSHA[p.ManifestSHA256] = struct{}{}
	}
	if computed := ComputeDesiredChecksum(state.Packs); computed != state.Checksum {
		return fmt.Errorf("%s: applied state checksum mismatch", ErrCodeRestartFailClosed)
	}
	return nil
}

// readRegularFileLimited Lstats first (reject symlink / non-regular including FIFO
// before any open/read), enforces max size, then open+fstat+LimitReader. Uses
// O_NONBLOCK so a TOCTOU race to a FIFO cannot hang the Sensor.
func readRegularFileLimited(path string, maxBytes int64, failCode string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s: refusing symlink", failCode)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", failCode)
	}
	if info.Size() < 0 || info.Size() > maxBytes {
		return nil, fmt.Errorf("%s: exceeds size limit", failCode)
	}

	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", failCode, err)
	}
	defer file.Close()

	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", failCode, err)
	}
	if !openedInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", failCode)
	}
	if openedInfo.Size() < 0 || openedInfo.Size() > maxBytes {
		return nil, fmt.Errorf("%s: exceeds size limit", failCode)
	}

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", failCode, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s: exceeds size limit", failCode)
	}
	return data, nil
}

func (s *Syncer) gcUnreferenced(keep map[string]struct{}) {
	dir := filepath.Join(s.cfg.Dir, artifactsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		sha := strings.TrimSuffix(name, ".json")
		if _, ok := keep[sha]; ok {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name)) // best-effort; failure does not invalidate state
	}
}

func (s *Syncer) ensureDirs() error {
	for _, d := range []string{
		filepath.Join(s.cfg.Dir, artifactsDirName),
		filepath.Join(s.cfg.Dir, stateDirName),
	} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("%s: %w", ErrCodeInvalidConfig, err)
		}
	}
	return nil
}

func (s *Syncer) setSuccess(gen int64, packCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.DesiredGeneration = gen
	s.status.AppliedGeneration = gen
	s.status.PackCount = packCount
	s.status.LastSuccessfulSync = time.Now().UTC()
	s.status.LastErrorCode = ""
	s.status.Degraded = false
	s.status.TrustedApplied = true
}

func (s *Syncer) setError(code string, degraded bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.LastErrorCode = code
	if degraded {
		s.status.Degraded = true
	}
	// Do not set TrustedApplied; transient/auth errors retain prior trust.
}

// TestingSetError records a sync error without clearing TrustedApplied.
// Intended for readiness/health unit tests only.
func (s *Syncer) TestingSetError(code string, degraded bool) {
	s.setError(code, degraded)
}

// TestingSetSuccess marks a successful apply for readiness/health unit tests.
func (s *Syncer) TestingSetSuccess(gen int64, packCount int) {
	s.setSuccess(gen, packCount)
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	for _, code := range []string{
		ErrCodeStateRegression, ErrCodeStateConflict, ErrCodeArtifactVerify,
		ErrCodeDurableWrite, ErrCodeAuthFailed, ErrCodeSyncTransient, ErrCodeRestartFailClosed,
	} {
		if strings.Contains(msg, code) {
			return code
		}
	}
	if coreclient.IsCode(err, coreclient.CodeAuthentication) || coreclient.IsCode(err, coreclient.CodeAuthorization) {
		return ErrCodeAuthFailed
	}
	if coreclient.IsTransient(err) {
		return ErrCodeSyncTransient
	}
	return ErrCodeSyncTransient
}

func isLowerHex64(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, c := range v {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ResolveCapabilityPackDir derives an absolute pack staging dir distinct from spool paths.
func ResolveCapabilityPackDir(explicit, spoolDir string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		clean := filepath.Clean(explicit)
		if !filepath.IsAbs(clean) {
			return "", fmt.Errorf("%s: SENSOR_CAPABILITY_PACK_DIR must be absolute", ErrCodeInvalidConfig)
		}
		return clean, nil
	}
	spool := filepath.Clean(strings.TrimSpace(spoolDir))
	if spool == "" || !filepath.IsAbs(spool) {
		return "", fmt.Errorf("%s: cannot derive CapabilityPackDir without absolute spool root", ErrCodeInvalidConfig)
	}
	// Sibling of spool under the same state root: .../sensor/capability-packs
	parent := filepath.Dir(spool)
	return filepath.Join(parent, "capability-packs"), nil
}

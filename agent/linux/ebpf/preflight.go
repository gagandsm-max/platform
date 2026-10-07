// Package ebpf provides a standard-library-only, non-loading Linux eBPF
// capability preflight. It never loads bytecode, creates maps, attaches hooks,
// executes a subprocess, or accepts production filesystem paths from config.
package ebpf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"platform/agent/common/collector"
)

const (
	kernelReleasePath   = "/proc/sys/kernel/osrelease"
	bootIDPath          = "/proc/sys/kernel/random/boot_id"
	btfPath             = "/sys/kernel/btf/vmlinux"
	capabilityPath      = "/proc/self/status"
	securityContextPath = "/proc/self/attr/current"

	maxKernelReleaseBytes = 256
	maxBootIDBytes        = 128
	maxCapabilityBytes    = 64 * 1024
	maxSecurityBytes      = 1024
	maxBTFDigestBytes     = 64 * 1024

	capPerfmonBit = 38
	capBPFBit     = 39

	defaultMaximumResourceBytes = 16 * 1024 * 1024
)

// Config contains bounded policy only. It intentionally has no BTF path,
// loader, program, attach, map, ring, root, or systemd capability setting.
type Config struct {
	Enabled            bool
	PreflightTimeout   time.Duration
	RecheckMin         time.Duration
	RecheckMax         time.Duration
	RequestedMapBytes  uint64
	RequestedRingBytes uint64
	MaximumBytes       uint64
	LowWaterSatisfied  bool
}

// CapabilityMasks is a parsed /proc/self/status capability view.
type CapabilityMasks struct {
	Effective uint64
	Permitted uint64
	Bounding  uint64
	Ambient   uint64
}

// Checker owns no background work. Call Run at startup or at the snapshot's
// NextRecheckAt; callers choose scheduling in a later authorized slice.
type Checker struct {
	cfg    Config
	source evidenceSource
	now    func() time.Time
	goos   string
	goarch string
}

type evidenceSource interface {
	ReadBounded(context.Context, string, int64) ([]byte, bool, error)
	Lstat(string) (os.FileInfo, error)
	Quality() string
}

type osEvidenceSource struct{}

func (osEvidenceSource) ReadBounded(ctx context.Context, path string, limit int64) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	f, err := os.Open(path) // #nosec G304 -- every production caller uses fixed constants above.
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, false, err
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if int64(len(b)) > limit {
		return b[:limit], true, nil
	}
	return b, false, nil
}

func (osEvidenceSource) Lstat(path string) (os.FileInfo, error) { return os.Lstat(path) }
func (osEvidenceSource) Quality() string                        { return "LIVE_BOUNDED" }

// New creates the production checker using only fixed procfs/sysfs paths.
func New(cfg Config) *Checker {
	return newChecker(cfg, osEvidenceSource{}, time.Now, runtime.GOOS, runtime.GOARCH)
}

func newChecker(cfg Config, source evidenceSource, now func() time.Time, goos, goarch string) *Checker {
	if cfg.PreflightTimeout <= 0 {
		cfg.PreflightTimeout = 2 * time.Second
	}
	if cfg.RecheckMin <= 0 {
		cfg.RecheckMin = 5 * time.Minute
	}
	if cfg.RecheckMax <= 0 {
		cfg.RecheckMax = time.Hour
	}
	if cfg.RecheckMax < cfg.RecheckMin {
		cfg.RecheckMax = cfg.RecheckMin
	}
	if cfg.MaximumBytes == 0 {
		cfg.MaximumBytes = defaultMaximumResourceBytes
	}
	if source == nil {
		source = osEvidenceSource{}
	}
	if now == nil {
		now = time.Now
	}
	return &Checker{cfg: cfg, source: source, now: now, goos: goos, goarch: goarch}
}

// Run collects a single bounded snapshot. A compatible result remains
// UNAVAILABLE/BACKEND_NOT_IMPLEMENTED because this slice contains no backend.
func (c *Checker) Run(parent context.Context, previous *collector.ProcessNetworkCapabilitySnapshot) collector.ProcessNetworkCapabilitySnapshot {
	observed := c.now().UTC()
	base := collector.ProcessNetworkCapabilitySnapshot{
		ObservedAt:      observed,
		OS:              c.goos,
		Arch:            c.goarch,
		SourceQuality:   c.source.Quality(),
		SecurityContext: "UNAVAILABLE",
		Resource: collector.ProcessNetworkResourceEvidence{
			RequestedMapBytes:  c.cfg.RequestedMapBytes,
			RequestedRingBytes: c.cfg.RequestedRingBytes,
			MaximumBytes:       c.cfg.MaximumBytes,
			LowWaterSatisfied:  c.cfg.LowWaterSatisfied,
		},
	}

	if !c.cfg.Enabled {
		base.SourceQuality = "DISABLED"
		return c.finish(base, previous, collector.ProcessNetworkCapabilityUnavailable,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonFeatureDisabled})
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, c.cfg.PreflightTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return c.finishReadFailure(base, previous, err, false)
	}

	if c.goos != "linux" {
		return c.finish(base, previous, collector.ProcessNetworkCapabilityUnsupported,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonOSUnsupported})
	}
	if !supportedArchitecture(c.goarch) {
		return c.finish(base, previous, collector.ProcessNetworkCapabilityUnsupported,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonArchUnsupported})
	}

	kernel, err := c.readText(ctx, kernelReleasePath, maxKernelReleaseBytes)
	if err != nil {
		return c.finishReadFailure(base, previous, err, false)
	}
	base.KernelRelease = kernel
	if !supportedKernelRelease(kernel) {
		base.KernelFingerprint = fingerprint(base)
		return c.finish(base, previous, collector.ProcessNetworkCapabilityUnsupported,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonKernelUnsupported})
	}

	bootID, err := c.readText(ctx, bootIDPath, maxBootIDBytes)
	if err != nil || !validBootID(bootID) {
		if err == nil {
			err = errUnsafeSource
		}
		return c.finishReadFailure(base, previous, err, false)
	}
	base.BootIDDigest = digestString(bootID)

	// Security context is optional/coarse support evidence. A read denial is
	// recorded without pretending it proves that the kernel would deny BPF.
	if _, _, secErr := c.source.ReadBounded(ctx, securityContextPath, maxSecurityBytes); secErr == nil {
		base.SecurityContext = "OBSERVED"
	} else if errors.Is(secErr, os.ErrPermission) {
		base.SecurityContext = "DENIED"
	}

	capBytes, truncated, err := c.source.ReadBounded(ctx, capabilityPath, maxCapabilityBytes)
	if err != nil {
		return c.finishReadFailure(base, previous, err, true)
	}
	if truncated || hasUnsafeControl(capBytes) {
		return c.finish(base, previous, collector.ProcessNetworkCapabilityUnavailable,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonCapabilityReadFailed})
	}
	masks, err := ParseCapabilityStatus(capBytes)
	if err != nil {
		return c.finish(base, previous, collector.ProcessNetworkCapabilityUnavailable,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonCapabilityReadFailed})
	}
	base.CAPBPF = presence(masks, capBPFBit)
	base.CAPPerfmon = presence(masks, capPerfmonBit)

	var privilegeReasons []collector.ProcessNetworkCapabilityReason
	if !base.CAPBPF.Effective || !base.CAPBPF.Bounding {
		privilegeReasons = append(privilegeReasons, collector.ReasonCAPBPFMissing)
	}
	if !base.CAPPerfmon.Effective || !base.CAPPerfmon.Bounding {
		privilegeReasons = append(privilegeReasons, collector.ReasonCAPPerfmonMissing)
	}
	if len(privilegeReasons) > 0 {
		privilegeReasons = append(privilegeReasons, collector.ReasonPrivilegeDenied)
		base.KernelFingerprint = fingerprint(base)
		return c.finish(base, previous, collector.ProcessNetworkCapabilityPrivilegeDenied, privilegeReasons)
	}

	fi, err := c.source.Lstat(btfPath)
	if err != nil {
		base.KernelFingerprint = fingerprint(base)
		if errors.Is(err, os.ErrNotExist) {
			return c.finish(base, previous, collector.ProcessNetworkCapabilityKernelIncompatible,
				[]collector.ProcessNetworkCapabilityReason{collector.ReasonBTFMissing})
		}
		if contextReason(err) != "" {
			return c.finishReadFailure(base, previous, err, false)
		}
		return c.finish(base, previous, collector.ProcessNetworkCapabilityKernelIncompatible,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonBTFUnreadable})
	}
	base.BTFPresent = true
	base.BTFRegular = fi.Mode().IsRegular() && fi.Mode()&os.ModeSymlink == 0
	if !base.BTFRegular {
		base.KernelFingerprint = fingerprint(base)
		return c.finish(base, previous, collector.ProcessNetworkCapabilityKernelIncompatible,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonBTFIncompatible})
	}
	btfPrefix, _, err := c.source.ReadBounded(ctx, btfPath, maxBTFDigestBytes)
	if err != nil {
		base.KernelFingerprint = fingerprint(base)
		if contextReason(err) != "" {
			return c.finishReadFailure(base, previous, err, false)
		}
		return c.finish(base, previous, collector.ProcessNetworkCapabilityKernelIncompatible,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonBTFUnreadable})
	}
	if len(btfPrefix) == 0 {
		base.KernelFingerprint = fingerprint(base)
		return c.finish(base, previous, collector.ProcessNetworkCapabilityKernelIncompatible,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonBTFIncompatible})
	}
	fiAfter, err := c.source.Lstat(btfPath)
	if err != nil || !stableBTFFile(fi, fiAfter) {
		base.KernelFingerprint = fingerprint(base)
		return c.finish(base, previous, collector.ProcessNetworkCapabilityKernelIncompatible,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonBTFIncompatible})
	}
	base.BTFReadable = true
	base.BTFDigest = digestBytes(btfPrefix)
	base.BTFDigestBytes = int64(len(btfPrefix))
	base.KernelCompatible = true
	base.KernelFingerprint = fingerprint(base)

	guarded := resourceGuarded(c.cfg.RequestedMapBytes, c.cfg.RequestedRingBytes, c.cfg.MaximumBytes)
	if previous != nil && previous.State == collector.ProcessNetworkCapabilityResourceGuarded && !c.cfg.LowWaterSatisfied {
		guarded = true
	}
	if guarded {
		base.Resource.Guarded = true
		return c.finish(base, previous, collector.ProcessNetworkCapabilityResourceGuarded,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonResourceLimit})
	}

	base.PreflightPassed = true
	// There is deliberately no backend status probe or load attempt in this slice.
	return c.finish(base, previous, collector.ProcessNetworkCapabilityUnavailable,
		[]collector.ProcessNetworkCapabilityReason{collector.ReasonBackendNotImplemented})
}

func (c *Checker) readText(ctx context.Context, path string, limit int64) (string, error) {
	b, truncated, err := c.source.ReadBounded(ctx, path, limit)
	if err != nil {
		return "", err
	}
	if truncated || len(b) == 0 || hasUnsafeControl(b) {
		return "", errUnsafeSource
	}
	value := strings.TrimSpace(string(b))
	if value == "" || strings.ContainsAny(value, "\r\n\t") {
		return "", errUnsafeSource
	}
	return value, nil
}

var errUnsafeSource = errors.New("unsafe bounded source")

func (c *Checker) finishReadFailure(base collector.ProcessNetworkCapabilitySnapshot, previous *collector.ProcessNetworkCapabilitySnapshot, err error, capability bool) collector.ProcessNetworkCapabilitySnapshot {
	if reason := contextReason(err); reason != "" {
		return c.finish(base, previous, collector.ProcessNetworkCapabilityUnavailable,
			[]collector.ProcessNetworkCapabilityReason{reason})
	}
	if capability && errors.Is(err, os.ErrPermission) {
		return c.finish(base, previous, collector.ProcessNetworkCapabilityPrivilegeDenied,
			[]collector.ProcessNetworkCapabilityReason{collector.ReasonCapabilityReadFailed, collector.ReasonPrivilegeDenied})
	}
	reason := collector.ReasonSourceUnavailable
	if capability {
		reason = collector.ReasonCapabilityReadFailed
	}
	return c.finish(base, previous, collector.ProcessNetworkCapabilityUnavailable,
		[]collector.ProcessNetworkCapabilityReason{reason})
}

func (c *Checker) finish(base collector.ProcessNetworkCapabilitySnapshot, previous *collector.ProcessNetworkCapabilitySnapshot, state collector.ProcessNetworkCapabilityState, reasons []collector.ProcessNetworkCapabilityReason) collector.ProcessNetworkCapabilitySnapshot {
	base.State = state
	base.ReasonCodes = append([]collector.ProcessNetworkCapabilityReason(nil), reasons...)
	base.TransitionAt = base.ObservedAt
	base.AttemptCount = 1
	if previous != nil {
		base.AttemptCount = saturatingIncrement(previous.AttemptCount)
		if previous.KernelFingerprint != "" && base.KernelFingerprint != "" && previous.KernelFingerprint != base.KernelFingerprint {
			base.AttemptCount = 1
		}
		if previous.State == state && equalReasons(previous.ReasonCodes, reasons) && previous.KernelFingerprint == base.KernelFingerprint {
			base.TransitionAt = previous.TransitionAt
		}
		base.LastSuccessAt = previous.LastSuccessAt
	}
	if base.PreflightPassed {
		base.LastSuccessAt = base.ObservedAt
	}
	delay := c.recheckDelay(state, base.AttemptCount)
	base.ValidUntil = base.ObservedAt.Add(delay)
	base.NextRecheckAt = base.ValidUntil
	return base
}

func (c *Checker) recheckDelay(state collector.ProcessNetworkCapabilityState, attempt uint32) time.Duration {
	switch state {
	case collector.ProcessNetworkCapabilityUnsupported, collector.ProcessNetworkCapabilityKernelIncompatible:
		return c.cfg.RecheckMax // stable: next call may cheaply compare fingerprint first.
	case collector.ProcessNetworkCapabilityPrivilegeDenied:
		return c.cfg.RecheckMax // avoid a tight retry against unchanged service privilege.
	case collector.ProcessNetworkCapabilityResourceGuarded:
		return minDuration(c.cfg.RecheckMax, maxDuration(c.cfg.RecheckMin*2, c.cfg.RecheckMin))
	case collector.ProcessNetworkCapabilityActive:
		return c.cfg.RecheckMin
	default:
		return exponentialDelay(c.cfg.RecheckMin, c.cfg.RecheckMax, attempt)
	}
}

// Fresh returns a copy and converts expired evidence to the truthful public
// UNAVAILABLE/EVIDENCE_STALE state. It never silently extends validity.
func Fresh(snapshot collector.ProcessNetworkCapabilitySnapshot, now time.Time) collector.ProcessNetworkCapabilitySnapshot {
	return snapshot.Fresh(now)
}

// KernelFingerprintChanged forces a full preflight after reboot/kernel/source
// change. Empty fingerprints are never treated as matching authority.
func KernelFingerprintChanged(previous, current collector.ProcessNetworkCapabilitySnapshot) bool {
	return previous.KernelFingerprint == "" || current.KernelFingerprint == "" || previous.KernelFingerprint != current.KernelFingerprint
}

// ParseCapabilityStatus strictly parses the four capability mask fields used by
// the preflight. UID is intentionally not consulted.
func ParseCapabilityStatus(data []byte) (CapabilityMasks, error) {
	if len(data) == 0 || len(data) > maxCapabilityBytes || hasUnsafeControl(data) {
		return CapabilityMasks{}, errors.New("CAPABILITY_STATUS_INVALID")
	}
	fields := map[string]*uint64{}
	var masks CapabilityMasks
	fields["CapEff"] = &masks.Effective
	fields["CapPrm"] = &masks.Permitted
	fields["CapBnd"] = &masks.Bounding
	fields["CapAmb"] = &masks.Ambient
	seen := make(map[string]bool, len(fields))
	for _, line := range strings.Split(string(data), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		dst, wanted := fields[name]
		if !wanted {
			continue
		}
		if seen[name] {
			return CapabilityMasks{}, errors.New("CAPABILITY_STATUS_DUPLICATE")
		}
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 16 {
			return CapabilityMasks{}, errors.New("CAPABILITY_MASK_INVALID")
		}
		parsed, err := strconv.ParseUint(value, 16, 64)
		if err != nil {
			return CapabilityMasks{}, errors.New("CAPABILITY_MASK_INVALID")
		}
		*dst = parsed
		seen[name] = true
	}
	for name := range fields {
		if !seen[name] {
			return CapabilityMasks{}, fmt.Errorf("CAPABILITY_FIELD_MISSING: %s", name)
		}
	}
	return masks, nil
}

func presence(m CapabilityMasks, bit uint) collector.ProcessNetworkCapabilityPresence {
	mask := uint64(1) << bit
	return collector.ProcessNetworkCapabilityPresence{
		Effective: m.Effective&mask != 0,
		Permitted: m.Permitted&mask != 0,
		Bounding:  m.Bounding&mask != 0,
		Ambient:   m.Ambient&mask != 0,
	}
}

func supportedArchitecture(arch string) bool { return arch == "amd64" || arch == "arm64" }

func supportedKernelRelease(release string) bool {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, errMajor := strconv.Atoi(parts[0])
	minorText := parts[1]
	for i, r := range minorText {
		if !unicode.IsDigit(r) {
			minorText = minorText[:i]
			break
		}
	}
	minor, errMinor := strconv.Atoi(minorText)
	if errMajor != nil || errMinor != nil || major < 0 || minor < 0 {
		return false
	}
	// CAP_BPF and CAP_PERFMON were introduced in Linux 5.8. This is an
	// implementation preflight floor, not Ubuntu/RHEL certification.
	return major > 5 || (major == 5 && minor >= 8)
}

func validBootID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}

func hasUnsafeControl(data []byte) bool {
	for _, b := range data {
		if b < 0x20 && b != '\n' && b != '\t' {
			return true
		}
		if b == 0x7f {
			return true
		}
	}
	return false
}

func resourceGuarded(mapBytes, ringBytes, maximum uint64) bool {
	if maximum == 0 || mapBytes > maximum || ringBytes > maximum {
		return true
	}
	return mapBytes > maximum-ringBytes // subtraction form avoids overflow.
}

func stableBTFFile(before, after os.FileInfo) bool {
	if before == nil || after == nil || !after.Mode().IsRegular() || after.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return false
	}
	if before.Sys() != nil && after.Sys() != nil && !os.SameFile(before, after) {
		return false
	}
	return true
}

func fingerprint(s collector.ProcessNetworkCapabilitySnapshot) string {
	return digestString(strings.Join([]string{
		s.OS, s.Arch, s.KernelRelease, s.BootIDDigest,
		strconv.FormatBool(s.BTFPresent), strconv.FormatBool(s.BTFRegular),
		strconv.FormatBool(s.BTFReadable), s.BTFDigest,
	}, "\x00"))
}

func digestString(value string) string { return digestBytes([]byte(value)) }
func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func contextReason(err error) collector.ProcessNetworkCapabilityReason {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return collector.ReasonPreflightTimeout
	case errors.Is(err, context.Canceled):
		return collector.ReasonPreflightCancelled
	default:
		return ""
	}
}

func saturatingIncrement(v uint32) uint32 {
	if v == math.MaxUint32 {
		return v
	}
	return v + 1
}

func exponentialDelay(minimum, maximum time.Duration, attempt uint32) time.Duration {
	if minimum <= 0 || maximum <= minimum {
		return maximum
	}
	delay := minimum
	for i := uint32(1); i < attempt && delay < maximum; i++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	return minDuration(delay, maximum)
}

func equalReasons(a, b []collector.ProcessNetworkCapabilityReason) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

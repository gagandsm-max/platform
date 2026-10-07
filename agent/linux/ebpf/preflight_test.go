package ebpf

import (
	"context"
	"io/fs"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"platform/agent/common/collector"
)

var fixtureNow = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

type fakeSource struct {
	mu            sync.Mutex
	data          map[string][]byte
	errs          map[string]error
	modes         map[string]fs.FileMode
	readLimits    map[string]int64
	lstatCalls    map[string]int
	lstatSequence map[string][]fs.FileMode
	blockPath     string
	quality       string
}

func (f *fakeSource) ReadBounded(ctx context.Context, path string, limit int64) ([]byte, bool, error) {
	f.mu.Lock()
	if f.readLimits == nil {
		f.readLimits = make(map[string]int64)
	}
	f.readLimits[path] = limit
	block := f.blockPath == path
	err := f.errs[path]
	data, ok := f.data[path]
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return nil, false, ctx.Err()
	}
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, os.ErrNotExist
	}
	copyData := append([]byte(nil), data...)
	if int64(len(copyData)) > limit {
		return copyData[:limit], true, nil
	}
	return copyData, false, nil
}

func (f *fakeSource) Lstat(path string) (os.FileInfo, error) {
	if err := f.errs["lstat:"+path]; err != nil {
		return nil, err
	}
	mode, ok := f.modes[path]
	if f.lstatCalls == nil {
		f.lstatCalls = make(map[string]int)
	}
	call := f.lstatCalls[path]
	f.lstatCalls[path] = call + 1
	if sequence := f.lstatSequence[path]; call < len(sequence) {
		mode, ok = sequence[call], true
	}
	if !ok {
		return nil, os.ErrNotExist
	}
	return fakeFileInfo{name: "vmlinux", mode: mode, size: int64(len(f.data[path]))}, nil
}

func (f *fakeSource) Quality() string {
	if f.quality == "" {
		return "FIXTURE"
	}
	return f.quality
}

type fakeFileInfo struct {
	name string
	mode fs.FileMode
	size int64
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return fixtureNow }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return nil }

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func supportedFixture(t *testing.T) *fakeSource {
	t.Helper()
	return &fakeSource{
		data: map[string][]byte{
			kernelReleasePath:   []byte("6.8.0-48-generic\n"),
			bootIDPath:          []byte("123e4567-e89b-12d3-a456-426614174000\n"),
			capabilityPath:      loadFixture(t, "proc_status_both_caps.txt"),
			securityContextPath: []byte("unconfined\n"),
			btfPath:             []byte{0x9f, 0xeb, 0x01, 0x00},
		},
		errs:  make(map[string]error),
		modes: map[string]fs.FileMode{btfPath: 0444},
	}
}

func checkerFor(t *testing.T, source *fakeSource, mutate func(*Config)) *Checker {
	t.Helper()
	cfg := Config{
		Enabled:           true,
		PreflightTimeout:  time.Second,
		RecheckMin:        time.Minute,
		RecheckMax:        16 * time.Minute,
		MaximumBytes:      1024,
		LowWaterSatisfied: true,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return newChecker(cfg, source, func() time.Time { return fixtureNow }, "linux", "amd64")
}

func assertStateReason(t *testing.T, got collector.ProcessNetworkCapabilitySnapshot, state collector.ProcessNetworkCapabilityState, reason collector.ProcessNetworkCapabilityReason) {
	t.Helper()
	if got.State != state {
		t.Fatalf("state=%s want=%s reasons=%v", got.State, state, got.ReasonCodes)
	}
	if len(got.ReasonCodes) == 0 || got.ReasonCodes[0] != reason {
		t.Fatalf("reasons=%v want first=%s", got.ReasonCodes, reason)
	}
}

func TestDisabledShortCircuitsWithoutReads(t *testing.T) {
	source := supportedFixture(t)
	c := checkerFor(t, source, func(cfg *Config) { cfg.Enabled = false })
	got := c.Run(context.Background(), nil)
	assertStateReason(t, got, collector.ProcessNetworkCapabilityUnavailable, collector.ReasonFeatureDisabled)
	if len(source.readLimits) != 0 {
		t.Fatalf("disabled preflight read sources: %v", source.readLimits)
	}
}

func TestUnsupportedOSAndArchitecture(t *testing.T) {
	for _, tc := range []struct {
		name   string
		goos   string
		arch   string
		reason collector.ProcessNetworkCapabilityReason
	}{
		{"non_linux", "windows", "amd64", collector.ReasonOSUnsupported},
		{"architecture", "linux", "386", collector.ReasonArchUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := supportedFixture(t)
			c := checkerFor(t, source, nil)
			c.goos, c.goarch = tc.goos, tc.arch
			got := c.Run(context.Background(), nil)
			assertStateReason(t, got, collector.ProcessNetworkCapabilityUnsupported, tc.reason)
		})
	}
}

func TestSupportedPreflightStillBackendNotImplemented(t *testing.T) {
	got := checkerFor(t, supportedFixture(t), nil).Run(context.Background(), nil)
	assertStateReason(t, got, collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotImplemented)
	if !got.PreflightPassed || !got.KernelCompatible || !got.BTFPresent || !got.BTFReadable || got.BTFDigest == "" {
		t.Fatalf("compatible evidence incomplete: %+v", got)
	}
	if got.State == collector.ProcessNetworkCapabilityActive {
		t.Fatal("preflight success must never claim ACTIVE")
	}
	if got.BootIDDigest == "123e4567-e89b-12d3-a456-426614174000" {
		t.Fatal("raw boot ID must not be exposed")
	}
}

func TestBTFMissingUnreadableAndSymlink(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		s := supportedFixture(t)
		delete(s.modes, btfPath)
		got := checkerFor(t, s, nil).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityKernelIncompatible, collector.ReasonBTFMissing)
	})
	t.Run("unreadable", func(t *testing.T) {
		s := supportedFixture(t)
		s.errs[btfPath] = os.ErrPermission
		got := checkerFor(t, s, nil).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityKernelIncompatible, collector.ReasonBTFUnreadable)
	})
	t.Run("symlink", func(t *testing.T) {
		s := supportedFixture(t)
		s.modes[btfPath] = os.ModeSymlink | 0777
		got := checkerFor(t, s, nil).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityKernelIncompatible, collector.ReasonBTFIncompatible)
	})
	t.Run("toctou_type_change", func(t *testing.T) {
		s := supportedFixture(t)
		s.lstatSequence = map[string][]fs.FileMode{btfPath: {0444, os.ModeSymlink | 0777}}
		got := checkerFor(t, s, nil).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityKernelIncompatible, collector.ReasonBTFIncompatible)
	})
}

func TestCapabilityParsingAndMissingCapabilities(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		s := supportedFixture(t)
		s.data[capabilityPath] = loadFixture(t, "proc_status_malformed.txt")
		got := checkerFor(t, s, nil).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityUnavailable, collector.ReasonCapabilityReadFailed)
	})
	t.Run("bpf_missing", func(t *testing.T) {
		s := supportedFixture(t)
		s.data[capabilityPath] = loadFixture(t, "proc_status_missing_bpf.txt")
		got := checkerFor(t, s, nil).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityPrivilegeDenied, collector.ReasonCAPBPFMissing)
	})
	t.Run("perfmon_missing", func(t *testing.T) {
		s := supportedFixture(t)
		s.data[capabilityPath] = []byte("CapEff:\t0000008000000000\nCapPrm:\t0000008000000000\nCapBnd:\t0000008000000000\nCapAmb:\t0\n")
		got := checkerFor(t, s, nil).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityPrivilegeDenied, collector.ReasonCAPPerfmonMissing)
	})
	t.Run("both_present", func(t *testing.T) {
		got := checkerFor(t, supportedFixture(t), nil).Run(context.Background(), nil)
		if !got.CAPBPF.Effective || !got.CAPBPF.Bounding || !got.CAPPerfmon.Effective || !got.CAPPerfmon.Bounding {
			t.Fatalf("capability presence not retained: %+v", got)
		}
	})
}

func TestCapabilityReadPermissionDeniedClassification(t *testing.T) {
	s := supportedFixture(t)
	s.errs[capabilityPath] = os.ErrPermission
	got := checkerFor(t, s, nil).Run(context.Background(), nil)
	assertStateReason(t, got, collector.ProcessNetworkCapabilityPrivilegeDenied, collector.ReasonCapabilityReadFailed)
	if len(got.ReasonCodes) < 2 || got.ReasonCodes[1] != collector.ReasonPrivilegeDenied {
		t.Fatalf("permission classification missing: %v", got.ReasonCodes)
	}
}

func TestResourceGuardAndOverflowSafety(t *testing.T) {
	for _, tc := range []struct {
		name string
		mapB uint64
		ring uint64
		max  uint64
	}{
		{"over_ceiling", 800, 300, 1024},
		{"integer_overflow", math.MaxUint64, 2, math.MaxUint64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkerFor(t, supportedFixture(t), func(cfg *Config) {
				cfg.RequestedMapBytes, cfg.RequestedRingBytes, cfg.MaximumBytes = tc.mapB, tc.ring, tc.max
			}).Run(context.Background(), nil)
			assertStateReason(t, got, collector.ProcessNetworkCapabilityResourceGuarded, collector.ReasonResourceLimit)
			if !got.Resource.Guarded {
				t.Fatal("resource guard evidence missing")
			}
		})
	}
}

func TestResourceGuardRequiresLowWaterBeforeClearing(t *testing.T) {
	guarded := checkerFor(t, supportedFixture(t), func(cfg *Config) {
		cfg.RequestedMapBytes, cfg.RequestedRingBytes = 800, 300
	}).Run(context.Background(), nil)
	assertStateReason(t, guarded, collector.ProcessNetworkCapabilityResourceGuarded, collector.ReasonResourceLimit)

	stillGuarded := checkerFor(t, supportedFixture(t), func(cfg *Config) {
		cfg.RequestedMapBytes, cfg.RequestedRingBytes = 100, 100
		cfg.LowWaterSatisfied = false
	}).Run(context.Background(), &guarded)
	assertStateReason(t, stillGuarded, collector.ProcessNetworkCapabilityResourceGuarded, collector.ReasonResourceLimit)

	cleared := checkerFor(t, supportedFixture(t), func(cfg *Config) {
		cfg.RequestedMapBytes, cfg.RequestedRingBytes = 100, 100
		cfg.LowWaterSatisfied = true
	}).Run(context.Background(), &guarded)
	assertStateReason(t, cleared, collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotImplemented)
}

func TestCancellationAndTimeout(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		s := supportedFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got := checkerFor(t, s, nil).Run(ctx, nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityUnavailable, collector.ReasonPreflightCancelled)
	})
	t.Run("timeout", func(t *testing.T) {
		s := supportedFixture(t)
		s.blockPath = kernelReleasePath
		c := checkerFor(t, s, func(cfg *Config) { cfg.PreflightTimeout = time.Millisecond })
		got := c.Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityUnavailable, collector.ReasonPreflightTimeout)
	})
}

func TestEvidenceStale(t *testing.T) {
	snapshot := checkerFor(t, supportedFixture(t), nil).Run(context.Background(), nil)
	got := Fresh(snapshot, snapshot.ValidUntil.Add(time.Nanosecond))
	assertStateReason(t, got, collector.ProcessNetworkCapabilityUnavailable, collector.ReasonEvidenceStale)
	if got.PreflightPassed {
		t.Fatal("stale evidence must not remain preflight-passed")
	}
}

func TestUnavailableBackoffIsBounded(t *testing.T) {
	s := supportedFixture(t)
	delete(s.data, kernelReleasePath)
	c := checkerFor(t, s, nil)
	first := c.Run(context.Background(), nil)
	second := c.Run(context.Background(), &first)
	if got := first.NextRecheckAt.Sub(first.ObservedAt); got != time.Minute {
		t.Fatalf("first backoff=%v", got)
	}
	if got := second.NextRecheckAt.Sub(second.ObservedAt); got != 2*time.Minute {
		t.Fatalf("second backoff=%v", got)
	}
	if second.AttemptCount != 2 {
		t.Fatalf("attempt=%d", second.AttemptCount)
	}
}

func TestKernelFingerprintChangesOnKernelOrBootChange(t *testing.T) {
	s := supportedFixture(t)
	c := checkerFor(t, s, nil)
	first := c.Run(context.Background(), nil)
	s.data[bootIDPath] = []byte("123e4567-e89b-12d3-a456-426614174001\n")
	bootChanged := c.Run(context.Background(), &first)
	if !KernelFingerprintChanged(first, bootChanged) {
		t.Fatal("boot ID change must require full preflight")
	}
	if bootChanged.AttemptCount != 1 {
		t.Fatalf("fingerprint change must reset attempt count, got %d", bootChanged.AttemptCount)
	}
	s.data[bootIDPath] = []byte("123e4567-e89b-12d3-a456-426614174000\n")
	s.data[kernelReleasePath] = []byte("6.9.0-test\n")
	kernelChanged := c.Run(context.Background(), &first)
	if !KernelFingerprintChanged(first, kernelChanged) {
		t.Fatal("kernel release change must require full preflight")
	}
}

func TestControlCharactersAndOversizedInputsRejected(t *testing.T) {
	t.Run("control", func(t *testing.T) {
		s := supportedFixture(t)
		s.data[kernelReleasePath] = []byte("6.8.0\x00hostile\n")
		got := checkerFor(t, s, nil).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityUnavailable, collector.ReasonSourceUnavailable)
	})
	t.Run("oversized_capabilities", func(t *testing.T) {
		s := supportedFixture(t)
		s.data[capabilityPath] = []byte(strings.Repeat("A", maxCapabilityBytes+1))
		got := checkerFor(t, s, nil).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityUnavailable, collector.ReasonCapabilityReadFailed)
	})
}

func TestBoundedReadLimitsAndFixedPaths(t *testing.T) {
	s := supportedFixture(t)
	_ = checkerFor(t, s, nil).Run(context.Background(), nil)
	want := map[string]int64{
		kernelReleasePath:   maxKernelReleaseBytes,
		bootIDPath:          maxBootIDBytes,
		capabilityPath:      maxCapabilityBytes,
		securityContextPath: maxSecurityBytes,
		btfPath:             maxBTFDigestBytes,
	}
	for path, limit := range want {
		if got := s.readLimits[path]; got != limit {
			t.Fatalf("path=%s limit=%d want=%d all=%v", path, got, limit, s.readLimits)
		}
	}
}

func TestSecurityContextDenialIsSafeEvidenceNotInventedBPFDenial(t *testing.T) {
	s := supportedFixture(t)
	s.errs[securityContextPath] = os.ErrPermission
	got := checkerFor(t, s, nil).Run(context.Background(), nil)
	assertStateReason(t, got, collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotImplemented)
	if got.SecurityContext != "DENIED" {
		t.Fatalf("security context=%q", got.SecurityContext)
	}
}

func TestRepeatedEvaluationDeterministic(t *testing.T) {
	s := supportedFixture(t)
	c := checkerFor(t, s, nil)
	a := c.Run(context.Background(), nil)
	b := c.Run(context.Background(), nil)
	if a.State != b.State || strings.Join(reasonStrings(a.ReasonCodes), ",") != strings.Join(reasonStrings(b.ReasonCodes), ",") || a.KernelFingerprint != b.KernelFingerprint {
		t.Fatalf("non-deterministic evaluation:\n%+v\n%+v", a, b)
	}
}

func TestTransitionTimestampStableUntilStateChanges(t *testing.T) {
	s := supportedFixture(t)
	c := checkerFor(t, s, nil)
	first := c.Run(context.Background(), nil)
	second := c.Run(context.Background(), &first)
	if !second.TransitionAt.Equal(first.TransitionAt) {
		t.Fatalf("unchanged state moved transition: %v -> %v", first.TransitionAt, second.TransitionAt)
	}
	c.now = func() time.Time { return fixtureNow.Add(time.Minute) }
	delete(s.modes, btfPath)
	changed := c.Run(context.Background(), &second)
	if !changed.TransitionAt.Equal(fixtureNow.Add(time.Minute)) {
		t.Fatalf("changed state transition=%v", changed.TransitionAt)
	}
}

func TestKernelFloorAndStatePrecedence(t *testing.T) {
	s := supportedFixture(t)
	s.data[kernelReleasePath] = []byte("5.4.0-old\n")
	s.data[capabilityPath] = loadFixture(t, "proc_status_malformed.txt")
	got := checkerFor(t, s, nil).Run(context.Background(), nil)
	assertStateReason(t, got, collector.ProcessNetworkCapabilityUnsupported, collector.ReasonKernelUnsupported)
}

func TestStatePrecedenceMatrix(t *testing.T) {
	t.Run("disabled_over_os", func(t *testing.T) {
		s := supportedFixture(t)
		c := checkerFor(t, s, func(cfg *Config) { cfg.Enabled = false })
		c.goos = "windows"
		got := c.Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityUnavailable, collector.ReasonFeatureDisabled)
	})
	t.Run("privilege_over_btf_and_resource", func(t *testing.T) {
		s := supportedFixture(t)
		s.data[capabilityPath] = loadFixture(t, "proc_status_missing_bpf.txt")
		delete(s.modes, btfPath)
		got := checkerFor(t, s, func(cfg *Config) { cfg.RequestedMapBytes = cfg.MaximumBytes + 1 }).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityPrivilegeDenied, collector.ReasonCAPBPFMissing)
	})
	t.Run("kernel_over_resource", func(t *testing.T) {
		s := supportedFixture(t)
		delete(s.modes, btfPath)
		got := checkerFor(t, s, func(cfg *Config) { cfg.RequestedMapBytes = cfg.MaximumBytes + 1 }).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityKernelIncompatible, collector.ReasonBTFMissing)
	})
	t.Run("resource_over_backend", func(t *testing.T) {
		got := checkerFor(t, supportedFixture(t), func(cfg *Config) { cfg.RequestedMapBytes = cfg.MaximumBytes + 1 }).Run(context.Background(), nil)
		assertStateReason(t, got, collector.ProcessNetworkCapabilityResourceGuarded, collector.ReasonResourceLimit)
	})
}

func TestParseCapabilityStatusRejectsDuplicateAndMissingMasks(t *testing.T) {
	for _, input := range []string{
		"CapEff:\t0\nCapEff:\t0\nCapPrm:\t0\nCapBnd:\t0\nCapAmb:\t0\n",
		"CapEff:\t0\nCapPrm:\t0\nCapBnd:\t0\n",
	} {
		if _, err := ParseCapabilityStatus([]byte(input)); err == nil {
			t.Fatalf("expected rejection: %q", input)
		}
	}
}

func TestAttemptSaturates(t *testing.T) {
	prev := checkerFor(t, supportedFixture(t), nil).Run(context.Background(), nil)
	prev.AttemptCount = math.MaxUint32
	got := checkerFor(t, supportedFixture(t), nil).Run(context.Background(), &prev)
	if got.AttemptCount != math.MaxUint32 {
		t.Fatalf("attempt wrapped: %d", got.AttemptCount)
	}
}

func TestLiveUnprivilegedPreflightIsReadOnlyAndNeverActive(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only live read evidence")
	}
	got := New(Config{Enabled: true, PreflightTimeout: time.Second}).Run(context.Background(), nil)
	if !got.State.Valid() || got.State == collector.ProcessNetworkCapabilityActive {
		t.Fatalf("live preflight returned invalid/active state: %+v", got)
	}
	if got.SourceQuality != "LIVE_BOUNDED" {
		t.Fatalf("source quality=%q", got.SourceQuality)
	}
}

func reasonStrings(in []collector.ProcessNetworkCapabilityReason) []string {
	out := make([]string, len(in))
	for i := range in {
		out[i] = string(in[i])
	}
	return out
}

//go:build windows

package collector

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
	"platform/agent/common/telemetry"
)

const (
	maxWindowsInterfaces = 256
	maxWindowsVolumes    = 256
	windowsVolumeTimeout = 2 * time.Second
)

var windowsSourceTimeout = 4 * time.Second

// One orphan per registered Windows source is the hard worst-case bound; it
// allows the other four sources to continue if any single native API hangs.
const windowsMaxOrphanedSources = 5

func NewDefaultCollectors(maxFilesystems int) []HostCollector {
	if maxFilesystems < 1 {
		maxFilesystems = 1
	}
	if maxFilesystems > maxWindowsVolumes {
		maxFilesystems = maxWindowsVolumes
	}
	p := nativeWindowsProvider{}
	return []HostCollector{
		newWindowsHostCollector("windows_cpu", p, maxFilesystems),
		newWindowsHostCollector("windows_memory", p, maxFilesystems),
		newWindowsHostCollector("windows_volumes", p, maxFilesystems),
		newWindowsHostCollector("windows_network", p, maxFilesystems),
		newWindowsHostCollector("windows_os", p, maxFilesystems),
	}
}

// windowsProvider is the deterministic seam for native source behavior. It
// returns only machine-level aggregate facts; process and user-profile data
// are deliberately outside this scalar collector.
type windowsProvider interface {
	read(context.Context, string, int) (windowsSnapshot, error)
}

type windowsHostCollector struct {
	provider    windowsProvider
	source      string
	maxVolumes  int
	mu          sync.Mutex
	previousCPU *cpuTimes
}

func newWindowsHostCollector(source string, provider windowsProvider, maxVolumes int) *windowsHostCollector {
	return &windowsHostCollector{source: source, provider: provider, maxVolumes: maxVolumes}
}

func (c *windowsHostCollector) Name() string { return c.source }

func (c *windowsHostCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s, err := readWindowsSourceBounded(ctx, c.source, c.provider, c.maxVolumes)
	out := make([]telemetry.Sample, 0, 32+len(s.volumes)*4+len(s.interfaces)*6)
	add := func(key string, value uint64, unit string, labels map[string]string) {
		out = append(out, telemetry.Sample{MetricKey: key, Value: float64(value), Unit: unit, Labels: labels})
	}
	if (c.source == "windows_os" || c.source == "windows_test") && s.uptimeSeconds != nil {
		out = append(out, telemetry.Sample{MetricKey: "system.uptime", Value: *s.uptimeSeconds, Unit: "s"})
	}
	if (c.source == "windows_cpu" || c.source == "windows_test") && s.logicalProcessors > 0 {
		add("system.cpu.count", uint64(s.logicalProcessors), "count", nil)
	}
	if (c.source == "windows_cpu" || c.source == "windows_test") && s.cpu != nil && s.logicalProcessors <= 64 {
		c.mu.Lock()
		if prev := c.previousCPU; prev != nil {
			if s.cpu.total > prev.total && s.cpu.idle >= prev.idle {
				dt, di := s.cpu.total-prev.total, s.cpu.idle-prev.idle
				if di <= dt {
					out = append(out, telemetry.Sample{MetricKey: "system.cpu.utilization", Value: 100 * (1 - float64(di)/float64(dt)), Unit: "%"})
				}
			}
		}
		current := *s.cpu
		c.previousCPU = &current
		c.mu.Unlock()
	}
	if (c.source == "windows_memory" || c.source == "windows_test") && s.memory != nil {
		m := s.memory
		if m.available <= m.total {
			add("system.memory.total", m.total, "bytes", nil)
			add("system.memory.available", m.available, "bytes", nil)
			used := m.total - m.available
			add("system.memory.used", used, "bytes", nil)
			out = append(out, telemetry.Sample{MetricKey: "system.memory.utilization", Value: percent(used, m.total), Unit: "%"})
		}
		if m.commitLimit > 0 && m.commitAvailable <= m.commitLimit {
			// GlobalMemoryStatusEx exposes system commit capacity, not Linux-style swap capacity.
			add("system.windows.memory.commit_limit", m.commitLimit, "bytes", nil)
			add("system.windows.memory.commit_available", m.commitAvailable, "bytes", nil)
			used := m.commitLimit - m.commitAvailable
			add("system.windows.memory.commit_used", used, "bytes", nil)
			out = append(out, telemetry.Sample{MetricKey: "system.windows.memory.commit_utilization", Value: percent(used, m.commitLimit), Unit: "%"})
		}
	}
	if (c.source == "windows_volumes" || c.source == "windows_test") && s.volumeTruncated {
		err = errors.Join(err, errors.New("WINDOWS_VOLUME_COVERAGE_TRUNCATED: configured volume limit reached"))
	}
	if (c.source == "windows_network" || c.source == "windows_test") && s.interfaceTruncated {
		err = errors.Join(err, errors.New("WINDOWS_INTERFACE_COVERAGE_TRUNCATED: interface limit reached"))
	}
	if c.source == "windows_volumes" || c.source == "windows_test" {
		for _, v := range dedupeVolumes(s.volumes, c.maxVolumes) {
			// Windows does not expose a portable inode-count equivalent for volumes.
			// Preserve capacity evidence and omit Linux-only inode metrics.
			labels := map[string]string{"mount": v.mount, "volume_id": v.id}
			add("system.filesystem.total", v.total, "bytes", labels)
			add("system.filesystem.available", v.available, "bytes", labels)
			if v.available <= v.total {
				used := v.total - v.available
				add("system.filesystem.used", used, "bytes", labels)
				if v.total > 0 {
					out = append(out, telemetry.Sample{MetricKey: "system.filesystem.utilization", Value: percent(used, v.total), Unit: "%", Labels: labels})
				}
			}
		}
	}
	if c.source == "windows_network" || c.source == "windows_test" {
		for _, n := range boundedInterfaces(s.interfaces) {
			labels := map[string]string{"interface": n.name, "interface_id": n.id}
			add("system.network.rx.bytes", n.rxBytes, "bytes", labels)
			add("system.network.tx.bytes", n.txBytes, "bytes", labels)
			add("system.network.rx.errors", n.rxErrors, "count", labels)
			add("system.network.tx.errors", n.txErrors, "count", labels)
			add("system.network.rx.dropped", n.rxDiscards, "count", labels)
			add("system.network.tx.dropped", n.txDiscards, "count", labels)
		}
	}
	if err == nil && len(out) == 0 {
		err = errors.New("WINDOWS_COLLECTOR_NO_EVIDENCE: native providers returned no usable values")
	}
	return out, err
}

var windowsSourceGates = map[string]chan struct{}{
	"windows_cpu": make(chan struct{}, 1), "windows_memory": make(chan struct{}, 1),
	"windows_volumes": make(chan struct{}, 1), "windows_network": make(chan struct{}, 1),
	"windows_os":   make(chan struct{}, 1),
	"windows_test": make(chan struct{}, 1),
}
var windowsOrphanBudget = make(chan struct{}, windowsMaxOrphanedSources)
var windowsBackoffMu sync.Mutex
var windowsBackoff = map[string]struct {
	until    time.Time
	failures uint8
}{}

// readWindowsSourceBounded isolates APIs that do not honor context. A timed-out
// call may remain orphaned, but at most one invocation per source can exist.
func readWindowsSourceBounded(ctx context.Context, source string, p windowsProvider, maxVolumes int) (windowsSnapshot, error) {
	gate := windowsSourceGates[source]
	if gate == nil {
		return windowsSnapshot{}, fmt.Errorf("WINDOWS_SOURCE_UNKNOWN: %s", source)
	}
	windowsBackoffMu.Lock()
	state := windowsBackoff[source]
	windowsBackoffMu.Unlock()
	if time.Now().Before(state.until) {
		return windowsSnapshot{}, fmt.Errorf("WINDOWS_SOURCE_BACKOFF: %s retry suppressed until %s", source, state.until.UTC().Format(time.RFC3339))
	}
	select {
	case gate <- struct{}{}:
	default:
		return windowsSnapshot{}, fmt.Errorf("WINDOWS_SOURCE_BUSY: %s already has an outstanding native call", source)
	}
	select {
	case windowsOrphanBudget <- struct{}{}:
	default:
		<-gate
		return windowsSnapshot{}, errors.New("WINDOWS_GLOBAL_HUNG_CALL_BUDGET_EXHAUSTED: native source capacity is occupied")
	}
	if err := ctx.Err(); err != nil {
		<-gate
		<-windowsOrphanBudget
		return windowsSnapshot{}, err
	}
	setBackoff := func(err error) {
		windowsBackoffMu.Lock()
		defer windowsBackoffMu.Unlock()
		if err == nil {
			delete(windowsBackoff, source)
			return
		}
		failures := windowsBackoff[source].failures
		if failures < 6 {
			failures++
		}
		delay := time.Second * time.Duration(1<<uint(failures-1))
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		windowsBackoff[source] = struct {
			until    time.Time
			failures uint8
		}{time.Now().Add(delay), failures}
	}
	type result struct {
		snapshot windowsSnapshot
		err      error
	}
	var backoffOnce sync.Once
	resolveBackoff := func(err error) {
		backoffOnce.Do(func() { setBackoff(err) })
	}
	done := make(chan result, 1)
	go func() {
		var r result
		defer func() {
			if recovered := recover(); recovered != nil {
				r.err = fmt.Errorf("WINDOWS_SOURCE_PANIC: %s provider panicked: %v", source, recovered)
			}
			resolveBackoff(r.err)
			// Release admission before publishing completion so an immediate
			// subsequent collection cannot observe a stale BUSY gate.
			<-gate
			<-windowsOrphanBudget
			done <- r
		}()
		r.snapshot, r.err = p.read(ctx, source, maxVolumes)
	}()
	timer := time.NewTimer(windowsSourceTimeout)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.snapshot, r.err
	case <-ctx.Done():
		resolveBackoff(ctx.Err())
		return windowsSnapshot{}, ctx.Err()
	case <-timer.C:
		err := fmt.Errorf("WINDOWS_SOURCE_TIMEOUT: %s exceeded %s", source, windowsSourceTimeout)
		resolveBackoff(err)
		return windowsSnapshot{}, err
	}
}

func percent(part, total uint64) float64 {
	if total == 0 {
		return math.NaN()
	}
	return math.Min(100, 100*float64(part)/float64(total))
}

type cpuTimes struct{ idle, total uint64 }
type memoryFacts struct{ total, available, commitLimit, commitAvailable uint64 }
type volumeFacts struct {
	id               string
	mount            string
	total, available uint64
}
type interfaceFacts struct {
	id, name                                                     string
	rxBytes, txBytes, rxErrors, txErrors, rxDiscards, txDiscards uint64
}
type windowsSnapshot struct {
	uptimeSeconds      *float64
	logicalProcessors  int
	cpu                *cpuTimes
	memory             *memoryFacts
	volumes            []volumeFacts
	interfaces         []interfaceFacts
	volumeTruncated    bool
	interfaceTruncated bool
}

func dedupeVolumes(in []volumeFacts, limit int) []volumeFacts {
	seen := make(map[string]bool)
	out := make([]volumeFacts, 0, minWindowsInt(len(in), limit))
	for _, v := range in {
		key := strings.ToUpper(strings.TrimSpace(v.mount))
		if v.id != "" {
			key = strings.ToUpper(strings.TrimSpace(v.id))
		}
		if key == "" || seen[key] || len(out) >= limit {
			continue
		}
		seen[key] = true
		v.mount = strings.TrimSpace(v.mount)
		out = append(out, v)
	}
	return out
}
func boundedInterfaces(in []interfaceFacts) []interfaceFacts {
	seen := map[string]bool{}
	out := make([]interfaceFacts, 0, minWindowsInt(len(in), maxWindowsInterfaces))
	for _, v := range in {
		name := boundedInterfaceName(v.name)
		key := strings.ToLower(strings.TrimSpace(v.id))
		if key == "" {
			continue
		}
		if name == "" {
			continue
		}
		if seen[key] {
			for i := range out {
				if strings.EqualFold(out[i].id, key) {
					out[i] = v
					out[i].name = name
					break
				}
			}
			continue
		}
		if len(out) >= maxWindowsInterfaces {
			break
		}
		seen[key] = true
		v.name = name
		out = append(out, v)
	}
	return out
}

func boundedInterfaceName(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsControl(r) {
			continue
		}
		if b.Len()+utf8.RuneLen(r) > 128 {
			break
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
func minWindowsInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type nativeWindowsProvider struct{}

func (nativeWindowsProvider) read(ctx context.Context, source string, maxVolumes int) (windowsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return windowsSnapshot{}, err
	}
	s := windowsSnapshot{}
	var errs []error
	if source == "windows_os" {
		upRaw, _, _ := procGetTickCount64.Call()
		up := float64(uint64(upRaw)) / 1000
		s.uptimeSeconds = &up
		return s, nil
	}
	if source == "windows_cpu" {
		if n, _, e := procGetActiveProcessorCount.Call(0xffff); n != 0 {
			s.logicalProcessors = int(n)
		} else {
			errs = append(errs, fmt.Errorf("WINDOWS_CPU_COUNT_UNAVAILABLE: %w", e))
		}
		if s.logicalProcessors > 0 && s.logicalProcessors <= 64 {
			if v, err := readSystemTimes(); err == nil {
				s.cpu = &v
			} else {
				errs = append(errs, fmt.Errorf("WINDOWS_CPU_UNAVAILABLE: %w", err))
			}
		}
		return s, errors.Join(errs...)
	}
	if source == "windows_memory" {
		if m, err := readMemoryStatus(); err == nil {
			s.memory = &m
		} else {
			errs = append(errs, fmt.Errorf("WINDOWS_MEMORY_UNAVAILABLE: %w", err))
		}
		return s, errors.Join(errs...)
	}
	if source == "windows_volumes" {
		vols, err := readFixedVolumes(ctx, maxVolumes)
		s.volumes = vols
		s.volumeTruncated = err != nil && strings.Contains(err.Error(), "WINDOWS_VOLUME_COVERAGE_TRUNCATED")
		return s, err
	}
	if source == "windows_network" {
		nics, truncated, err := readInterfaceTable()
		s.interfaces = nics
		s.interfaceTruncated = truncated
		return s, err
	}
	return s, fmt.Errorf("WINDOWS_SOURCE_UNKNOWN: %s", source)
}

type filetime struct{ low, high uint32 }

func (f filetime) value() uint64 { return uint64(f.high)<<32 | uint64(f.low) }

var procGetSystemTimes = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")
var procGetActiveProcessorCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetActiveProcessorCount")
var procGetTickCount64 = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount64")

func readSystemTimes() (cpuTimes, error) {
	var idle, kernel, user filetime
	r, _, e := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return cpuTimes{}, e
	}
	k, u := kernel.value(), user.value()
	if k < idle.value() {
		return cpuTimes{}, errors.New("invalid kernel/idle counters")
	}
	return cpuTimes{idle: idle.value(), total: k + u}, nil
}

type memoryStatusEx struct {
	length                                                                                uint32
	load                                                                                  uint32
	totalPhys, availPhys, totalPage, availPage, totalVirtual, availVirtual, availExtended uint64
}

var procGlobalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

func readMemoryStatus() (memoryFacts, error) {
	m := memoryStatusEx{length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	r, _, e := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if r == 0 {
		return memoryFacts{}, e
	}
	if m.totalPhys == 0 || m.availPhys > m.totalPhys {
		return memoryFacts{}, errors.New("invalid physical-memory counters")
	}
	return memoryFacts{total: m.totalPhys, available: m.availPhys, commitLimit: m.totalPage, commitAvailable: m.availPage}, nil
}

var procFindFirstVolume = windows.NewLazySystemDLL("kernel32.dll").NewProc("FindFirstVolumeW")
var procFindNextVolume = windows.NewLazySystemDLL("kernel32.dll").NewProc("FindNextVolumeW")
var procFindVolumeClose = windows.NewLazySystemDLL("kernel32.dll").NewProc("FindVolumeClose")
var procGetDriveType = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetDriveTypeW")
var procGetDiskFreeSpaceEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")
var procGetVolumePathNames = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetVolumePathNamesForVolumeNameW")
var volumeCallGate = make(chan struct{}, 1)

// volumeSourceError identifies a required native volume API that could not be
// resolved. LazyProc.Call panics on this condition, so resolve APIs explicitly
// before entering the per-volume worker goroutine.
type volumeSourceError struct {
	procedure string
	err       error
}

func (e *volumeSourceError) Error() string {
	return fmt.Sprintf("WINDOWS_VOLUME_API_UNAVAILABLE: %s: %v", e.procedure, e.err)
}

func (e *volumeSourceError) Unwrap() error { return e.err }

var findVolumeProcedure = func(p *windows.LazyProc) error { return p.Find() }

func resolveVolumeProcedures() error {
	for _, p := range []*windows.LazyProc{procFindFirstVolume, procFindNextVolume, procFindVolumeClose, procGetDriveType, procGetDiskFreeSpaceEx, procGetVolumePathNames} {
		if err := findVolumeProcedure(p); err != nil {
			return &volumeSourceError{procedure: p.Name, err: err}
		}
	}
	return nil
}

func readFixedVolumes(ctx context.Context, limit int) ([]volumeFacts, error) {
	if err := resolveVolumeProcedures(); err != nil {
		return nil, err
	}
	budget := time.NewTimer(10 * time.Second)
	defer budget.Stop()
	buf := make([]uint16, 1024)
	h, _, e := procFindFirstVolume.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if h == ^uintptr(0) {
		return nil, fmt.Errorf("WINDOWS_VOLUME_ENUMERATION_FAILED: %w", e)
	}
	defer procFindVolumeClose.Call(h)
	out := make([]volumeFacts, 0, limit)
	var errs []error
	advance := func() bool {
		next, _, nextErr := procFindNextVolume.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if next != 0 {
			return true
		}
		if errno, ok := nextErr.(windows.Errno); !ok || errno != windows.ERROR_NO_MORE_FILES {
			errs = append(errs, fmt.Errorf("WINDOWS_VOLUME_ENUMERATION_PARTIAL: %w", nextErr))
		}
		return false
	}
	for scanned := 0; scanned < 256 && len(out) < limit; scanned++ {
		root := decodeUTF16(buf)
		p := &buf[0]
		kind, _, _ := procGetDriveType.Call(uintptr(unsafe.Pointer(p)))
		if kind != 3 {
			if scanned == 255 {
				if advance() {
					errs = append(errs, errors.New("WINDOWS_VOLUME_COVERAGE_TRUNCATED: scan cap of 256 reached"))
				}
				break
			}
			if !advance() {
				break
			}
			continue
		} // DRIVE_FIXED; eligibility only, SAN calls may still hang.
		select {
		case volumeCallGate <- struct{}{}:
		case <-ctx.Done():
			return out, ctx.Err()
		case <-budget.C:
			return out, errors.Join(append(errs, errors.New("WINDOWS_VOLUME_BUDGET_EXHAUSTED: volume scan exceeded 10 seconds"))...)
		case <-time.After(windowsVolumeTimeout):
			errs = append(errs, fmt.Errorf("WINDOWS_VOLUME_QUERY_BUSY: %s", root))
			if !advance() {
				return out, errors.Join(errs...)
			}
			continue
		}
		done := make(chan struct{})
		var v volumeFacts
		var callErr error
		go func() {
			defer func() { <-volumeCallGate; close(done) }()
			var available, total, free uint64
			r, _, last := procGetDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&available)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
			if r == 0 {
				callErr = last
				return
			}
			if available > total {
				callErr = errors.New("available space exceeds total")
				return
			}
			mount, pathErr := readVolumeMountPath(root)
			if pathErr != nil {
				callErr = pathErr
				return
			}
			v = volumeFacts{id: strings.ToUpper(root), mount: mount, total: total, available: available}
		}()
		select {
		case <-done:
			if callErr != nil {
				errs = append(errs, fmt.Errorf("WINDOWS_VOLUME_UNAVAILABLE: %s: %w", root, callErr))
			} else {
				out = append(out, v)
			}
		case <-ctx.Done():
			return out, ctx.Err()
		case <-budget.C:
			errs = append(errs, errors.New("WINDOWS_VOLUME_BUDGET_EXHAUSTED: volume scan exceeded 10 seconds"))
			return out, errors.Join(errs...)
		case <-time.After(windowsVolumeTimeout):
			errs = append(errs, fmt.Errorf("WINDOWS_VOLUME_TIMEOUT: %s", root))
			return out, errors.Join(errs...)
		}
		if scanned == 255 {
			if advance() {
				errs = append(errs, errors.New("WINDOWS_VOLUME_COVERAGE_TRUNCATED: scan cap of 256 reached"))
			}
			break
		}
		if !advance() {
			break
		}
		if len(out) >= limit {
			errs = append(errs, errors.New("WINDOWS_VOLUME_COVERAGE_TRUNCATED: configured volume limit reached"))
			break
		}
	}
	return out, errors.Join(errs...)
}

func readVolumeMountPath(root string) (string, error) {
	name := append(utf16.Encode([]rune(root)), 0)
	var needed uint32
	r, _, callErr := procGetVolumePathNames.Call(uintptr(unsafe.Pointer(&name[0])), 0, 0, uintptr(unsafe.Pointer(&needed)))
	if r == 0 && callErr != windows.ERROR_MORE_DATA {
		return "", fmt.Errorf("WINDOWS_VOLUME_MOUNT_PATH_UNAVAILABLE: %w", callErr)
	}
	if needed == 0 || needed > 32768 {
		return "", errors.New("WINDOWS_VOLUME_MOUNT_PATH_INVALID_LENGTH")
	}
	paths := make([]uint16, needed)
	r, _, callErr = procGetVolumePathNames.Call(uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&paths[0])), uintptr(len(paths)), uintptr(unsafe.Pointer(&needed)))
	if r == 0 {
		return "", fmt.Errorf("WINDOWS_VOLUME_MOUNT_PATH_UNAVAILABLE: %w", callErr)
	}
	mounts, parseErr := parseVolumeMountPaths(paths)
	if parseErr != nil {
		return "", parseErr
	}
	if len(mounts) == 0 {
		return root, nil
	}
	sort.Strings(mounts)
	return mounts[0], nil
}

// parseVolumeMountPaths parses the bounded UTF-16 MULTI_SZ returned by
// GetVolumePathNamesForVolumeNameW, stopping at its double-NUL terminator.
func parseVolumeMountPaths(paths []uint16) ([]string, error) {
	if len(paths) > 32768 {
		return nil, errors.New("WINDOWS_VOLUME_MOUNT_PATH_INVALID_LENGTH")
	}
	var mounts []string
	for i := 0; i < len(paths); {
		if paths[i] == 0 { // empty item marks the MULTI_SZ terminator
			return mounts, nil
		}
		j := i
		for j < len(paths) && paths[j] != 0 {
			j++
		}
		if j == len(paths) {
			return nil, errors.New("WINDOWS_VOLUME_MOUNT_PATH_UNTERMINATED")
		}
		mount := strings.TrimSpace(string(utf16.Decode(paths[i:j])))
		if mount != "" {
			mounts = append(mounts, mount)
		}
		i = j + 1
	}
	return nil, errors.New("WINDOWS_VOLUME_MOUNT_PATH_UNTERMINATED")
}

// MIB_IF_ROW2 mirrors the documented Win32 layout. GetIfTable2 allocates the
// bounded-at-consumption native table; FreeMibTable always releases it.
type mibIfRow2 struct {
	luid                                                                                                                uint64
	index                                                                                                               uint32
	guid                                                                                                                [16]byte
	alias                                                                                                               [257]uint16
	description                                                                                                         [257]uint16
	physicalAddressLength                                                                                               uint32
	physicalAddress                                                                                                     [32]byte
	permanentAddress                                                                                                    [32]byte
	mtu, ifType, tunnelType, mediaType, physicalMediumType, accessType, directionType                                   uint32
	flags                                                                                                               uint8
	pad                                                                                                                 [3]byte
	operStatus, adminStatus, mediaConnectState                                                                          uint32
	networkGuid                                                                                                         [16]byte
	connectionType                                                                                                      uint32
	transmitSpeed, receiveSpeed, inOctets, inUcastPkts, inNUcastPkts, inDiscards, inErrors, inUnknownProtos             uint64
	inUcastOctets, inMulticastOctets, inBroadcastOctets, outOctets, outUcastPkts, outNUcastPkts, outDiscards, outErrors uint64
	outUcastOctets, outMulticastOctets, outBroadcastOctets, outQLen                                                     uint64
}
type mibIfTable2 struct {
	entries uint32
	rows    [1]mibIfRow2
}

var procGetIfTable2 = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetIfTable2")
var procFreeMibTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("FreeMibTable")

func readInterfaceTable() ([]interfaceFacts, bool, error) {
	var table *mibIfTable2
	r, _, _ := procGetIfTable2.Call(uintptr(unsafe.Pointer(&table)))
	if r != 0 {
		return nil, false, windows.Errno(r)
	}
	if table == nil {
		return nil, false, errors.New("empty interface table")
	}
	defer procFreeMibTable.Call(uintptr(unsafe.Pointer(table)))
	truncated := table.entries > maxWindowsInterfaces
	n := int(table.entries)
	if n > maxWindowsInterfaces {
		n = maxWindowsInterfaces
	}
	rows := unsafe.Slice(&table.rows[0], n)
	out := make([]interfaceFacts, 0, n)
	for i := range rows {
		row := &rows[i]
		name := decodeUTF16(row.alias[:])
		if name == "" {
			name = decodeUTF16(row.description[:])
		}
		guid := windows.GUID{Data1: binary.LittleEndian.Uint32(row.guid[0:4]), Data2: binary.LittleEndian.Uint16(row.guid[4:6]), Data3: binary.LittleEndian.Uint16(row.guid[6:8])}
		copy(guid.Data4[:], row.guid[8:16])
		id := windowsInterfaceID(guid, row.luid)
		out = append(out, interfaceFacts{id: id, name: name, rxBytes: row.inOctets, txBytes: row.outOctets, rxErrors: row.inErrors, txErrors: row.outErrors, rxDiscards: row.inDiscards, txDiscards: row.outDiscards})
	}
	return boundedInterfaces(out), truncated, nil
}

func windowsInterfaceID(guid windows.GUID, luid uint64) string {
	if guid == (windows.GUID{}) {
		return fmt.Sprintf("luid-%016x", luid)
	}
	return strings.ToLower(guid.String())
}

func decodeUTF16(v []uint16) string {
	for i, c := range v {
		if c == 0 {
			v = v[:i]
			break
		}
	}
	return strings.TrimSpace(string(utf16.Decode(v)))
}

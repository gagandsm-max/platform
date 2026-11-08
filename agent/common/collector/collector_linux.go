//go:build linux

package collector

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"platform/agent/common/telemetry"
)

func NewDefaultCollectors(maxFilesystems int) []HostCollector {
	return []HostCollector{
		NewCPUCollector(),
		NewMemoryCollector(),
		NewFilesystemCollector(maxFilesystems),
		NewPressureCollector(),
		NewDiskIOCollector(),
		NewNetCollector(),
		NewOSFactsCollector(),
	}
}

// -----------------------------------------------------------------------------
// CPU Collector
// -----------------------------------------------------------------------------

type CPUCollector struct {
	mu        sync.Mutex
	prevTotal uint64
	prevIdle  uint64
	readFile  func(string) ([]byte, error)
}

func NewCPUCollector() *CPUCollector {
	return &CPUCollector{}
}

func (c *CPUCollector) Name() string { return "cpu" }

func (c *CPUCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	read := c.readFile
	if read == nil {
		read = readCPUFile
	}
	stat, err := read("/proc/stat")
	if err != nil {
		return nil, errors.New("CPU_STAT_READ_FAILED")
	}
	total, idle, count, err := parseCPUStat(stat)
	if err != nil {
		return nil, err
	}
	load, err := read("/proc/loadavg")
	if err != nil {
		return nil, errors.New("CPU_LOAD_READ_FAILED")
	}
	loads, err := parseNonnegativeFloats(load, 3)
	if err != nil {
		return nil, errors.New("CPU_LOAD_PARSE_FAILED")
	}
	uptime, err := read("/proc/uptime")
	if err != nil {
		return nil, errors.New("CPU_UPTIME_READ_FAILED")
	}
	up, err := parseNonnegativeFloats(uptime, 1)
	if err != nil {
		return nil, errors.New("CPU_UPTIME_PARSE_FAILED")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.mu.Lock()
	prevTotal, prevIdle := c.prevTotal, c.prevIdle
	c.prevTotal, c.prevIdle = total, idle
	c.mu.Unlock()
	samples := []telemetry.Sample{
		{MetricKey: "system.cpu.count", Value: float64(count), Unit: "count"},
		{MetricKey: "system.load.1m", Value: loads[0], Unit: "load"},
		{MetricKey: "system.load.5m", Value: loads[1], Unit: "load"},
		{MetricKey: "system.load.15m", Value: loads[2], Unit: "load"},
		{MetricKey: "system.uptime", Value: up[0], Unit: "s"},
	}
	// A reset (including Linux iowait regression) is missing interval evidence,
	// not 100% utilization. Rebase once; the next monotonic interval can recover.
	available, reason := 0.0, "warmup"
	if prevTotal != 0 {
		reason = "counter_reset"
		if total >= prevTotal && idle >= prevIdle {
			dt, di := total-prevTotal, idle-prevIdle
			reason = "no_interval"
			if dt > 0 && di <= dt {
				available, reason = 1, "available"
				samples = append(samples, telemetry.Sample{MetricKey: "system.cpu.utilization", Value: 100 * (1 - float64(di)/float64(dt)), Unit: "%"})
			} else if di > dt {
				reason = "counter_reset"
			}
		}
	}
	samples = append(samples, telemetry.Sample{MetricKey: "system.cpu.utilization.available", Value: available, Unit: "bool", Labels: map[string]string{"reason": reason}})
	return samples, nil
}

func readCPUFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, procReadLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > procReadLimit {
		return nil, errors.New("CPU_SOURCE_LIMIT")
	}
	return data, nil
}

func parseNonnegativeFloats(data []byte, n int) ([]float64, error) {
	fields := strings.Fields(string(data))
	if len(fields) < n {
		return nil, errors.New("missing fields")
	}
	out := make([]float64, n)
	for i := range out {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return nil, errors.New("invalid value")
		}
		out[i] = v
	}
	return out, nil
}

func parseCPUStat(data []byte) (total, idle uint64, count int, err error) {
	bad := errors.New("CPU_STAT_PARSE_FAILED")
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if f[0] == "cpu" {
			if found || len(f) < 5 {
				return 0, 0, 0, bad
			}
			found = true
			// guest and guest_nice are already included in user/nice.
			for i := 1; i < len(f) && i <= 8; i++ {
				v, e := strconv.ParseUint(f[i], 10, 64)
				if e != nil || v > ^uint64(0)-total {
					return 0, 0, 0, bad
				}
				total += v
				if i == 4 || i == 5 {
					idle += v
				}
			}
		} else if strings.HasPrefix(f[0], "cpu") {
			if id, e := strconv.ParseUint(strings.TrimPrefix(f[0], "cpu"), 10, 32); e == nil && id < 1<<20 {
				count++
			}
		}
	}
	if !found || total == 0 || count == 0 {
		return 0, 0, 0, bad
	}
	return total, idle, count, nil
}

// -----------------------------------------------------------------------------
// Memory Collector
// -----------------------------------------------------------------------------

type MemoryCollector struct{}

func NewMemoryCollector() *MemoryCollector {
	return &MemoryCollector{}
}

func (c *MemoryCollector) Name() string { return "memory" }

func (c *MemoryCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	memFile, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, fmt.Errorf("MEMINFO_READ_FAILED: %w", err)
	}
	defer memFile.Close()

	info, err := parseMeminfo(io.LimitReader(memFile, 64*1024))
	if err != nil {
		return nil, err
	}

	samples, err := meminfoSamples(info)
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	vmFile, err := os.Open("/proc/vmstat")
	if err == nil {
		defer vmFile.Close()
		samples = append(samples, parseVmstatSamples(io.LimitReader(vmFile, 64*1024))...)
	}

	return samples, nil
}

type meminfoValues struct {
	totalKB   uint64
	availKB   uint64
	swapTot   uint64
	swapFree  uint64
	swapValid bool
}

func parseMeminfo(r io.Reader) (meminfoValues, error) {
	scanner := bufio.NewScanner(r)
	var totalKB, availKB, freeKB, buffersKB, cachedKB, swapTot, swapFree uint64
	var hasAvail bool
	seen := make(map[string]bool, 8)

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		switch key {
		case "MemTotal", "MemAvailable", "MemFree", "Buffers", "Cached", "SwapTotal", "SwapFree":
		default:
			continue
		}
		if seen[key] {
			return meminfoValues{}, errors.New("MEMINFO_PARSE_FAILED: duplicate field")
		}
		valFields := strings.Fields(parts[1])
		if len(valFields) != 2 || valFields[1] != "kB" {
			return meminfoValues{}, errors.New("MEMINFO_PARSE_FAILED: missing field or unit")
		}
		val, err := strconv.ParseUint(valFields[0], 10, 64)
		if err != nil {
			return meminfoValues{}, errors.New("MEMINFO_PARSE_FAILED: invalid field")
		}
		seen[key] = true
		switch key {
		case "MemTotal":
			totalKB = val
		case "MemAvailable":
			availKB = val
			hasAvail = true
		case "MemFree":
			freeKB = val
		case "Buffers":
			buffersKB = val
		case "Cached":
			cachedKB = val
		case "SwapTotal":
			swapTot = val
		case "SwapFree":
			swapFree = val
		}
	}

	if scanner.Err() != nil {
		return meminfoValues{}, errors.New("MEMINFO_READ_FAILED: incomplete source")
	}
	if totalKB == 0 {
		return meminfoValues{}, errors.New("MEMINFO_PARSE_FAILED: MemTotal was 0 or unreadable")
	}

	if !hasAvail {
		if !seen["MemFree"] || !seen["Buffers"] || !seen["Cached"] {
			return meminfoValues{}, errors.New("MEMINFO_PARSE_FAILED: availability missing")
		}
		if buffersKB > ^uint64(0)-freeKB || cachedKB > ^uint64(0)-freeKB-buffersKB {
			return meminfoValues{}, errors.New("MEMINFO_OVERFLOW: fallback availability")
		}
		availKB = freeKB + buffersKB + cachedKB
	}
	if availKB > totalKB || (seen["SwapTotal"] && seen["SwapFree"] && swapFree > swapTot) {
		return meminfoValues{}, errors.New("MEMINFO_PARSE_FAILED: inconsistent capacity")
	}

	return meminfoValues{
		totalKB:   totalKB,
		availKB:   availKB,
		swapTot:   swapTot,
		swapFree:  swapFree,
		swapValid: seen["SwapTotal"] && seen["SwapFree"],
	}, nil
}

func meminfoSamples(info meminfoValues) ([]telemetry.Sample, error) {
	usedKB := info.totalKB - info.availKB
	totalBytes, ok := kbToBytes(info.totalKB)
	if !ok {
		return nil, errors.New("MEMINFO_OVERFLOW: MemTotal exceeds convertible byte range")
	}
	availBytes, ok := kbToBytes(info.availKB)
	if !ok {
		return nil, errors.New("MEMINFO_OVERFLOW: MemAvailable exceeds convertible byte range")
	}
	usedBytes, ok := kbToBytes(usedKB)
	if !ok {
		return nil, errors.New("MEMINFO_OVERFLOW: used memory exceeds convertible byte range")
	}
	util := 0.0
	if totalBytes > 0 {
		util = (usedBytes / totalBytes) * 100.0
	}

	samples := []telemetry.Sample{
		{MetricKey: "system.memory.total", Value: totalBytes, Unit: "bytes"},
		{MetricKey: "system.memory.available", Value: availBytes, Unit: "bytes"},
		{MetricKey: "system.memory.used", Value: usedBytes, Unit: "bytes"},
		{MetricKey: "system.memory.utilization", Value: util, Unit: "%"},
	}

	swapTotal, okTot := kbToBytes(info.swapTot)
	swapAvail, okAvail := kbToBytes(info.swapFree)
	swapUsed, okUsed := kbToBytes(info.swapTot - info.swapFree)
	if info.swapValid && okTot && okAvail && okUsed {
		swapUtil := 0.0
		if swapTotal > 0 {
			swapUtil = (swapUsed / swapTotal) * 100.0
		}
		samples = append(samples,
			telemetry.Sample{MetricKey: "system.memory.swap.total", Value: swapTotal, Unit: "bytes"},
			telemetry.Sample{MetricKey: "system.memory.swap.available", Value: swapAvail, Unit: "bytes"},
			telemetry.Sample{MetricKey: "system.memory.swap.used", Value: swapUsed, Unit: "bytes"},
			telemetry.Sample{MetricKey: "system.memory.swap.utilization", Value: swapUtil, Unit: "%"},
		)
	}
	available := 0.0
	if info.swapValid && okTot && okAvail && okUsed {
		available = 1
	}
	samples = append(samples, telemetry.Sample{MetricKey: "system.memory.swap.available_evidence", Value: available, Unit: "bool"})
	return samples, nil
}

func kbToBytes(kb uint64) (float64, bool) {
	if kb > (^uint64(0))/1024 {
		return 0, false
	}
	return float64(kb * 1024), true
}

func parseVmstatSamples(r io.Reader) []telemetry.Sample {
	scanner := bufio.NewScanner(r)
	var samples []telemetry.Sample
	seen := make(map[string]struct{}, 5)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := fields[0]
		metric, unit, ok := vmstatMetric(key)
		if !ok {
			continue
		}
		if _, dup := seen[metric]; dup {
			continue
		}
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		seen[metric] = struct{}{}
		samples = append(samples, telemetry.Sample{
			MetricKey: metric,
			Value:     float64(val),
			Unit:      unit,
		})
	}
	return samples
}

func vmstatMetric(key string) (metric string, unit string, ok bool) {
	switch key {
	case "pgfault":
		return "system.memory.page.faults", "count", true
	case "pgmajfault":
		return "system.memory.page.major_faults", "count", true
	case "pswpin":
		return "system.memory.swap.pages_in", "pages", true
	case "pswpout":
		return "system.memory.swap.pages_out", "pages", true
	case "oom_kill":
		return "system.memory.oom_kills", "count", true
	default:
		return "", "", false
	}
}

// -----------------------------------------------------------------------------
// Filesystem Collector
// -----------------------------------------------------------------------------

type FilesystemCollector struct {
	maxMounts  int
	mu         sync.Mutex
	inflight   map[string]bool
	statfs     func(string, *syscall.Statfs_t) error
	openMounts func() (io.ReadCloser, error)
}

func NewFilesystemCollector(maxMounts int) *FilesystemCollector {
	if maxMounts <= 0 {
		maxMounts = 32
	}
	if maxMounts > 256 {
		maxMounts = 256
	}
	return &FilesystemCollector{maxMounts: maxMounts, inflight: make(map[string]bool)}
}

func (c *FilesystemCollector) Name() string { return "filesystem" }

var ignoredFSTypes = map[string]bool{
	"proc":        true,
	"sysfs":       true,
	"devpts":      true,
	"cgroup":      true,
	"cgroup2":     true,
	"pstore":      true,
	"bpf":         true,
	"autofs":      true,
	"mqueue":      true,
	"hugetlbfs":   true,
	"debugfs":     true,
	"tracefs":     true,
	"securityfs":  true,
	"fusectl":     true,
	"configfs":    true,
	"devtmpfs":    true,
	"rpc_pipefs":  true,
	"binfmt_misc": true,
	"nsfs":        true,
	"nfs":         true, "nfs4": true, "cifs": true, "smb3": true, "9p": true, "ceph": true, "afs": true,
	"fuse.sshfs": true, "fuse.glusterfs": true,
	"squashfs": true,
}

func (c *FilesystemCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	openMounts := c.openMounts
	if openMounts == nil {
		openMounts = func() (io.ReadCloser, error) { return os.Open("/proc/mounts") }
	}
	mountsFile, err := openMounts()
	if err != nil {
		return nil, fmt.Errorf("MOUNTS_READ_FAILED: %w", err)
	}
	defer mountsFile.Close()

	scanner := bufio.NewScanner(io.LimitReader(mountsFile, 64*1024))
	var samples []telemetry.Sample
	seenMounts := make(map[string]bool)
	mountCount := 0

	for scanner.Scan() {
		if mountCount >= c.maxMounts {
			break
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		mountPoint := unescapeMountField(fields[1])
		fsType := unescapeMountField(fields[2])

		if ignoredFSTypes[fsType] || strings.HasPrefix(fsType, "fuse.") || fsType == "fuse" || seenMounts[mountPoint] {
			continue
		}
		if len(mountPoint) > 128 || len(fsType) > 64 {
			continue
		}
		seenMounts[mountPoint] = true
		mountCount++ // Failed and blocked mounts consume the same cardinality budget.
		stat, reason := c.statMount(ctx, mountPoint)
		labels := map[string]string{"mount": mountPoint, "fstype": fsType}
		if reason != "available" {
			samples = append(samples, filesystemAvailability(labels, reason))
			continue
		}

		blockSize := uint64(stat.Bsize)
		if stat.Bsize <= 0 || stat.Blocks == 0 || stat.Blocks > ^uint64(0)/blockSize || stat.Bfree > stat.Blocks || stat.Bavail > stat.Blocks {
			samples = append(samples, filesystemAvailability(labels, "invalid_stats"))
			continue
		}

		totalBytes := float64(stat.Blocks * blockSize)
		freeBytes := float64(stat.Bfree * blockSize)
		availBytes := float64(stat.Bavail * blockSize)

		if totalBytes <= 0 {
			continue
		}

		usedBytes := totalBytes - freeBytes
		if usedBytes < 0 {
			usedBytes = 0
		}
		util := (usedBytes / totalBytes) * 100.0

		samples = append(samples, filesystemAvailability(labels, "available"))
		samples = append(samples,
			telemetry.Sample{MetricKey: "system.filesystem.total", Value: totalBytes, Unit: "bytes", Labels: labels},
			telemetry.Sample{MetricKey: "system.filesystem.available", Value: availBytes, Unit: "bytes", Labels: labels},
			telemetry.Sample{MetricKey: "system.filesystem.used", Value: usedBytes, Unit: "bytes", Labels: labels},
			telemetry.Sample{MetricKey: "system.filesystem.utilization", Value: util, Unit: "%", Labels: labels},
		)
		samples = append(samples, inodeSamples(stat.Files, stat.Ffree, labels)...)
	}

	return samples, nil
}

// statfs is not cancellable in the kernel. Keep ownership until it really returns:
// never spawn replacements for a stuck mount, and cap stuck workers process-wide.
var filesystemWorkers = make(chan struct{}, 2)

func (c *FilesystemCollector) statMount(ctx context.Context, mount string) (syscall.Statfs_t, string) {
	if ctx.Err() != nil {
		return syscall.Statfs_t{}, "cancelled"
	}
	c.mu.Lock()
	if c.inflight[mount] {
		c.mu.Unlock()
		return syscall.Statfs_t{}, "outstanding"
	}
	select {
	case filesystemWorkers <- struct{}{}:
	default:
		c.mu.Unlock()
		return syscall.Statfs_t{}, "resource_guarded"
	}
	if c.inflight == nil {
		c.inflight = make(map[string]bool)
	}
	c.inflight[mount] = true
	c.mu.Unlock()
	type result struct {
		stat syscall.Statfs_t
		err  error
	}
	ch := make(chan result, 1)
	statfs := c.statfs
	if statfs == nil {
		statfs = syscall.Statfs
	}
	go func() {
		defer func() { c.mu.Lock(); delete(c.inflight, mount); c.mu.Unlock(); <-filesystemWorkers }()
		var stat syscall.Statfs_t
		err := statfs(mount, &stat)
		ch <- result{stat, err}
	}()
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case r := <-ch:
		if r.err != nil {
			return syscall.Statfs_t{}, "read_failed"
		}
		return r.stat, "available"
	case <-ctx.Done():
		return syscall.Statfs_t{}, "cancelled"
	case <-timer.C:
		return syscall.Statfs_t{}, "timeout"
	}
}

func filesystemAvailability(labels map[string]string, reason string) telemetry.Sample {
	quality := map[string]string{"mount": labels["mount"], "fstype": labels["fstype"], "reason": reason}
	value := 0.0
	if reason == "available" {
		value = 1
	}
	return telemetry.Sample{MetricKey: "system.filesystem.available_evidence", Value: value, Unit: "bool", Labels: quality}
}

func inodeSamples(files, ffree uint64, labels map[string]string) []telemetry.Sample {
	if files == 0 {
		return nil
	}
	avail := ffree
	if avail > files {
		avail = files
	}
	used := files - avail
	util := (float64(used) / float64(files)) * 100.0
	return []telemetry.Sample{
		{MetricKey: "system.filesystem.inodes.total", Value: float64(files), Unit: "count", Labels: labels},
		{MetricKey: "system.filesystem.inodes.available", Value: float64(avail), Unit: "count", Labels: labels},
		{MetricKey: "system.filesystem.inodes.used", Value: float64(used), Unit: "count", Labels: labels},
		{MetricKey: "system.filesystem.inodes.utilization", Value: util, Unit: "%", Labels: labels},
	}
}

// unescapeMountField decodes /proc/mounts octal escapes (e.g. \040 -> space).
func unescapeMountField(in string) string {
	if !strings.Contains(in, `\`) {
		return in
	}
	var b strings.Builder
	b.Grow(len(in))
	for i := 0; i < len(in); i++ {
		if in[i] == '\\' && i+3 < len(in) {
			o1, e1 := strconv.ParseUint(in[i+1:i+2], 8, 8)
			o2, e2 := strconv.ParseUint(in[i+2:i+3], 8, 8)
			o3, e3 := strconv.ParseUint(in[i+3:i+4], 8, 8)
			if e1 == nil && e2 == nil && e3 == nil {
				b.WriteByte(byte(o1<<6 | o2<<3 | o3))
				i += 3
				continue
			}
		}
		b.WriteByte(in[i])
	}
	return b.String()
}

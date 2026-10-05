//go:build linux

package collector

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

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
}

func NewCPUCollector() *CPUCollector {
	return &CPUCollector{}
}

func (c *CPUCollector) Name() string { return "cpu" }

func (c *CPUCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var samples []telemetry.Sample

	// 1. Read /proc/stat
	statFile, err := os.Open("/proc/stat")
	if err == nil {
		defer statFile.Close()
		scanner := bufio.NewScanner(io.LimitReader(statFile, 64*1024))
		cpuCount := 0
		var curTotal, curIdle uint64

		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}

			if fields[0] == "cpu" {
				// cpu user nice system idle iowait irq softirq steal [guest guest_nice]
				// guest/guest_nice are already included in user/nice; do not double-count.
				var vals []uint64
				limit := len(fields) - 1
				if limit > 8 {
					limit = 8
				}
				for i := 1; i <= limit; i++ {
					v, err := strconv.ParseUint(fields[i], 10, 64)
					if err != nil {
						vals = nil
						curTotal = 0
						break
					}
					vals = append(vals, v)
					curTotal += v
				}
				if len(vals) >= 4 {
					curIdle = vals[3] // idle
					if len(vals) >= 5 {
						curIdle += vals[4] // iowait (idle for utilization purposes)
					}
				}
			} else if strings.HasPrefix(fields[0], "cpu") && len(fields[0]) > 3 {
				if _, err := strconv.Atoi(fields[0][3:]); err == nil {
					cpuCount++
				}
			}
		}

		if curTotal > 0 {
			c.mu.Lock()
			prevTotal := c.prevTotal
			prevIdle := c.prevIdle
			c.prevTotal = curTotal
			c.prevIdle = curIdle
			c.mu.Unlock()

			// First sample only seeds counters; since-boot ratio is not interval utilization.
			if prevTotal > 0 && curTotal >= prevTotal {
				deltaTotal := curTotal - prevTotal
				var deltaIdle uint64
				if curIdle >= prevIdle {
					deltaIdle = curIdle - prevIdle
				}
				if deltaTotal > 0 {
					util := 100.0 * (1.0 - (float64(deltaIdle) / float64(deltaTotal)))
					if util < 0 {
						util = 0
					}
					if util > 100 {
						util = 100
					}
					samples = append(samples, telemetry.Sample{
						MetricKey: "system.cpu.utilization",
						Value:     util,
						Unit:      "%",
					})
				}
			}
		}

		if cpuCount > 0 {
			samples = append(samples, telemetry.Sample{
				MetricKey: "system.cpu.count",
				Value:     float64(cpuCount),
				Unit:      "count",
			})
		}
	}

	// 2. Read /proc/loadavg
	loadBytes, err := os.ReadFile("/proc/loadavg")
	if err == nil {
		fields := strings.Fields(string(loadBytes))
		if len(fields) >= 3 {
			if l1, err := strconv.ParseFloat(fields[0], 64); err == nil {
				samples = append(samples, telemetry.Sample{
					MetricKey: "system.load.1m",
					Value:     l1,
					Unit:      "load",
				})
			}
			if l5, err := strconv.ParseFloat(fields[1], 64); err == nil {
				samples = append(samples, telemetry.Sample{
					MetricKey: "system.load.5m",
					Value:     l5,
					Unit:      "load",
				})
			}
			if l15, err := strconv.ParseFloat(fields[2], 64); err == nil {
				samples = append(samples, telemetry.Sample{
					MetricKey: "system.load.15m",
					Value:     l15,
					Unit:      "load",
				})
			}
		}
	}

	// 3. Read /proc/uptime
	upBytes, err := os.ReadFile("/proc/uptime")
	if err == nil {
		fields := strings.Fields(string(upBytes))
		if len(fields) >= 1 {
			if uptime, err := strconv.ParseFloat(fields[0], 64); err == nil {
				samples = append(samples, telemetry.Sample{
					MetricKey: "system.uptime",
					Value:     uptime,
					Unit:      "s",
				})
			}
		}
	}

	return samples, nil
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
	totalKB  uint64
	availKB  uint64
	swapTot  uint64
	swapFree uint64
}

func parseMeminfo(r io.Reader) (meminfoValues, error) {
	scanner := bufio.NewScanner(r)
	var totalKB, availKB, freeKB, buffersKB, cachedKB, swapTot, swapFree uint64
	var hasAvail bool

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valFields := strings.Fields(parts[1])
		if len(valFields) == 0 {
			continue
		}
		val, err := strconv.ParseUint(valFields[0], 10, 64)
		if err != nil {
			continue
		}

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

	if totalKB == 0 {
		return meminfoValues{}, errors.New("MEMINFO_PARSE_FAILED: MemTotal was 0 or unreadable")
	}

	if !hasAvail {
		availKB = freeKB + buffersKB + cachedKB
	}
	if availKB > totalKB {
		availKB = totalKB
	}
	if swapFree > swapTot {
		swapFree = swapTot
	}

	return meminfoValues{
		totalKB:  totalKB,
		availKB:  availKB,
		swapTot:  swapTot,
		swapFree: swapFree,
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
	if okTot && okAvail && okUsed {
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
		metric, ok := vmstatMetric(key)
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
			Unit:      "count",
		})
	}
	return samples
}

func vmstatMetric(key string) (string, bool) {
	switch key {
	case "pgfault":
		return "system.memory.page.faults", true
	case "pgmajfault":
		return "system.memory.page.major_faults", true
	case "pswpin":
		return "system.memory.swap.pages_in", true
	case "pswpout":
		return "system.memory.swap.pages_out", true
	case "oom_kill":
		return "system.memory.oom_kills", true
	default:
		return "", false
	}
}

// -----------------------------------------------------------------------------
// Filesystem Collector
// -----------------------------------------------------------------------------

type FilesystemCollector struct {
	maxMounts int
}

func NewFilesystemCollector(maxMounts int) *FilesystemCollector {
	if maxMounts <= 0 {
		maxMounts = 32
	}
	return &FilesystemCollector{maxMounts: maxMounts}
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
	"squashfs":    true,
}

func (c *FilesystemCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	mountsFile, err := os.Open("/proc/mounts")
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

		if ignoredFSTypes[fsType] || seenMounts[mountPoint] {
			continue
		}
		if len(mountPoint) > 128 || len(fsType) > 64 {
			continue
		}
		seenMounts[mountPoint] = true

		var stat syscall.Statfs_t
		if err := syscall.Statfs(mountPoint, &stat); err != nil {
			continue
		}

		blockSize := uint64(stat.Bsize)
		if blockSize == 0 || stat.Blocks == 0 {
			continue
		}
		if stat.Blocks > ^uint64(0)/blockSize {
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

		labels := map[string]string{
			"mount":  mountPoint,
			"fstype": fsType,
		}

		samples = append(samples,
			telemetry.Sample{MetricKey: "system.filesystem.total", Value: totalBytes, Unit: "bytes", Labels: labels},
			telemetry.Sample{MetricKey: "system.filesystem.available", Value: availBytes, Unit: "bytes", Labels: labels},
			telemetry.Sample{MetricKey: "system.filesystem.used", Value: usedBytes, Unit: "bytes", Labels: labels},
			telemetry.Sample{MetricKey: "system.filesystem.utilization", Value: util, Unit: "%", Labels: labels},
		)
		samples = append(samples, inodeSamples(stat.Files, stat.Ffree, labels)...)
		mountCount++
	}

	return samples, nil
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

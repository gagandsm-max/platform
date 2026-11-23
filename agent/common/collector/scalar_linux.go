//go:build linux

package collector

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"platform/agent/common/telemetry"
)

// Engineering safety ceilings for Slice 2 scalar collectors — not customer sizing.
const (
	maxDiskIODevices   = 64
	maxNetInterfaces   = 64
	maxDeviceNameLen   = 64
	maxInterfaceNameLen = 64
	diskSectorBytes    = 512
	procReadLimit      = 64 * 1024
)

// -----------------------------------------------------------------------------
// Pressure (PSI) Collector
// -----------------------------------------------------------------------------

type PressureCollector struct{}

func NewPressureCollector() *PressureCollector { return &PressureCollector{} }

func (c *PressureCollector) Name() string { return "pressure" }

func (c *PressureCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var samples []telemetry.Sample
	for _, resource := range []string{"cpu", "memory", "io"} {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		path := "/proc/pressure/" + resource
		f, err := os.Open(path)
		if err != nil {
			samples = append(samples, pressureAvailableSample(resource, false))
			continue
		}
		parsed, ok := parsePSI(io.LimitReader(f, procReadLimit))
		_ = f.Close()
		if !ok {
			samples = append(samples, pressureAvailableSample(resource, false))
			continue
		}
		samples = append(samples, pressureSamples(resource, parsed)...)
		samples = append(samples, pressureAvailableSample(resource, true))
	}
	return samples, nil
}

type psiScope struct {
	avg10, avg60, avg300 float64
	total                uint64
	present              bool
}

type psiData struct {
	some, full psiScope
}

func parsePSI(r io.Reader) (psiData, bool) {
	scanner := bufio.NewScanner(r)
	var data psiData
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			continue
		}
		scope := fields[0]
		avg10, ok10 := parsePSIField(fields, "avg10=")
		avg60, ok60 := parsePSIField(fields, "avg60=")
		avg300, ok300 := parsePSIField(fields, "avg300=")
		total, okTotal := parsePSITotal(fields, "total=")
		if !ok10 || !ok60 || !ok300 || !okTotal {
			continue
		}
		s := psiScope{avg10: avg10, avg60: avg60, avg300: avg300, total: total, present: true}
		switch scope {
		case "some":
			data.some = s
		case "full":
			data.full = s
		}
	}
	// Canonical requirement: at least one valid some/full scope.
	return data, data.some.present || data.full.present
}

func parsePSIField(fields []string, prefix string) (float64, bool) {
	for _, f := range fields[1:] {
		if !strings.HasPrefix(f, prefix) {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimPrefix(f, prefix), 64)
		if err != nil {
			return 0, false
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

func parsePSITotal(fields []string, prefix string) (uint64, bool) {
	for _, f := range fields[1:] {
		if !strings.HasPrefix(f, prefix) {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimPrefix(f, prefix), 10, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

func pressureSamples(resource string, data psiData) []telemetry.Sample {
	var out []telemetry.Sample
	appendScope := func(scope string, s psiScope) {
		if !s.present {
			return
		}
		labels := map[string]string{"resource": resource, "scope": scope}
		out = append(out,
			telemetry.Sample{MetricKey: "system.pressure.avg10", Value: s.avg10, Unit: "%", Labels: labels},
			telemetry.Sample{MetricKey: "system.pressure.avg60", Value: s.avg60, Unit: "%", Labels: labels},
			telemetry.Sample{MetricKey: "system.pressure.avg300", Value: s.avg300, Unit: "%", Labels: labels},
			telemetry.Sample{MetricKey: "system.pressure.total", Value: float64(s.total), Unit: "us", Labels: labels},
		)
	}
	appendScope("some", data.some)
	appendScope("full", data.full)
	return out
}

func pressureAvailableSample(resource string, available bool) telemetry.Sample {
	v := 0.0
	if available {
		v = 1.0
	}
	return telemetry.Sample{
		MetricKey: "system.pressure.available",
		Value:     v,
		Unit:      "bool",
		Labels:    map[string]string{"resource": resource},
	}
}

// -----------------------------------------------------------------------------
// Disk I/O Collector
// -----------------------------------------------------------------------------

type DiskIOCollector struct{}

func NewDiskIOCollector() *DiskIOCollector { return &DiskIOCollector{} }

func (c *DiskIOCollector) Name() string { return "diskio" }

func (c *DiskIOCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return nil, fmt.Errorf("DISKSTATS_READ_FAILED: %w", err)
	}
	defer f.Close()

	devices := parseDiskstats(io.LimitReader(f, procReadLimit))
	devices = selectDiskDevices(devices, maxDiskIODevices)

	var samples []telemetry.Sample
	for _, d := range devices {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		samples = append(samples, diskIOSamples(d)...)
	}
	return samples, nil
}

type diskstatsDevice struct {
	name           string
	readsCompleted uint64
	readSectors    uint64
	readTicks      uint64
	writesCompleted uint64
	writeSectors   uint64
	writeTicks     uint64
	ioInProgress   uint64
	ioTicks        uint64
	weightedTicks  uint64
}

func parseDiskstats(r io.Reader) []diskstatsDevice {
	scanner := bufio.NewScanner(r)
	var out []diskstatsDevice
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		// major minor name + at least 11 I/O fields (through weighted time).
		if len(fields) < 14 {
			continue
		}
		name := fields[2]
		if !isEmittableBlockDevice(name) {
			continue
		}
		vals := make([]uint64, 11)
		ok := true
		for i := 0; i < 11; i++ {
			v, err := strconv.ParseUint(fields[3+i], 10, 64)
			if err != nil {
				ok = false
				break
			}
			vals[i] = v
		}
		if !ok {
			continue
		}
		out = append(out, diskstatsDevice{
			name:            name,
			readsCompleted:  vals[0],
			readSectors:     vals[2],
			readTicks:       vals[3],
			writesCompleted: vals[4],
			writeSectors:    vals[6],
			writeTicks:      vals[7],
			ioInProgress:    vals[8],
			ioTicks:         vals[9],
			weightedTicks:   vals[10],
		})
	}
	return out
}

func isEmittableBlockDevice(name string) bool {
	if name == "" || len(name) > maxDeviceNameLen {
		return false
	}
	// Unambiguous kernel pseudo devices only — no invented topology ownership.
	if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "fd") {
		return false
	}
	return true
}

func selectDiskDevices(in []diskstatsDevice, limit int) []diskstatsDevice {
	if limit <= 0 {
		limit = maxDiskIODevices
	}
	sort.Slice(in, func(i, j int) bool { return in[i].name < in[j].name })
	if len(in) > limit {
		in = in[:limit]
	}
	return in
}

func diskIOSamples(d diskstatsDevice) []telemetry.Sample {
	labels := map[string]string{"device": d.name}
	samples := []telemetry.Sample{
		{MetricKey: "system.disk.read.operations", Value: float64(d.readsCompleted), Unit: "count", Labels: labels},
		{MetricKey: "system.disk.write.operations", Value: float64(d.writesCompleted), Unit: "count", Labels: labels},
		{MetricKey: "system.disk.read.time", Value: float64(d.readTicks), Unit: "ms", Labels: labels},
		{MetricKey: "system.disk.write.time", Value: float64(d.writeTicks), Unit: "ms", Labels: labels},
		{MetricKey: "system.disk.io.in_progress", Value: float64(d.ioInProgress), Unit: "count", Labels: labels},
		{MetricKey: "system.disk.io.time", Value: float64(d.ioTicks), Unit: "ms", Labels: labels},
		{MetricKey: "system.disk.io.weighted_time", Value: float64(d.weightedTicks), Unit: "ms", Labels: labels},
	}
	if readBytes, ok := sectorsToBytes(d.readSectors); ok {
		samples = append(samples, telemetry.Sample{
			MetricKey: "system.disk.read.bytes", Value: readBytes, Unit: "bytes", Labels: labels,
		})
	}
	if writeBytes, ok := sectorsToBytes(d.writeSectors); ok {
		samples = append(samples, telemetry.Sample{
			MetricKey: "system.disk.write.bytes", Value: writeBytes, Unit: "bytes", Labels: labels,
		})
	}
	return samples
}

func sectorsToBytes(sectors uint64) (float64, bool) {
	if sectors > (^uint64(0))/diskSectorBytes {
		return 0, false
	}
	return float64(sectors * diskSectorBytes), true
}

// -----------------------------------------------------------------------------
// Network Collector
// -----------------------------------------------------------------------------

type NetCollector struct {
	// operstateReader is overridable in tests; defaults to sysfs operstate.
	operstateReader func(iface string) string
}

func NewNetCollector() *NetCollector {
	return &NetCollector{operstateReader: readOperstate}
}

func (c *NetCollector) Name() string { return "net" }

func (c *NetCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, fmt.Errorf("NETDEV_READ_FAILED: %w", err)
	}
	defer f.Close()

	ifaces := parseNetDev(io.LimitReader(f, procReadLimit))
	ifaces = selectNetInterfaces(ifaces, maxNetInterfaces)

	reader := c.operstateReader
	if reader == nil {
		reader = readOperstate
	}

	var samples []telemetry.Sample
	for _, iface := range ifaces {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		samples = append(samples, netCounterSamples(iface)...)
		samples = append(samples, netStateSample(iface.name, reader(iface.name)))
	}
	return samples, nil
}

type netDevIface struct {
	name       string
	rxBytes    uint64
	rxPackets  uint64
	rxErrors   uint64
	rxDropped  uint64
	txBytes    uint64
	txPackets  uint64
	txErrors   uint64
	txDropped  uint64
}

func parseNetDev(r io.Reader) []netDevIface {
	scanner := bufio.NewScanner(r)
	var out []netDevIface
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Inter-") || strings.HasPrefix(line, "face") {
			continue
		}
		// "eth0: 1 2 3 ..." — split name from counters.
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		if !isEmittableInterface(name) {
			continue
		}
		fields := strings.Fields(line[colon+1:])
		if len(fields) < 16 {
			continue
		}
		vals := make([]uint64, 16)
		ok := true
		for i := 0; i < 16; i++ {
			v, err := strconv.ParseUint(fields[i], 10, 64)
			if err != nil {
				ok = false
				break
			}
			vals[i] = v
		}
		if !ok {
			continue
		}
		out = append(out, netDevIface{
			name:      name,
			rxBytes:   vals[0],
			rxPackets: vals[1],
			rxErrors:  vals[2],
			rxDropped: vals[3],
			txBytes:   vals[8],
			txPackets: vals[9],
			txErrors:  vals[10],
			txDropped: vals[11],
		})
	}
	return out
}

func isEmittableInterface(name string) bool {
	if name == "" || len(name) > maxInterfaceNameLen {
		return false
	}
	// Exclude loopback to avoid non-NIC noise (deterministic).
	if name == "lo" {
		return false
	}
	return true
}

func selectNetInterfaces(in []netDevIface, limit int) []netDevIface {
	if limit <= 0 {
		limit = maxNetInterfaces
	}
	sort.Slice(in, func(i, j int) bool { return in[i].name < in[j].name })
	if len(in) > limit {
		in = in[:limit]
	}
	return in
}

func netCounterSamples(iface netDevIface) []telemetry.Sample {
	labels := map[string]string{"interface": iface.name}
	return []telemetry.Sample{
		{MetricKey: "system.network.rx.bytes", Value: float64(iface.rxBytes), Unit: "bytes", Labels: labels},
		{MetricKey: "system.network.tx.bytes", Value: float64(iface.txBytes), Unit: "bytes", Labels: labels},
		{MetricKey: "system.network.rx.packets", Value: float64(iface.rxPackets), Unit: "count", Labels: labels},
		{MetricKey: "system.network.tx.packets", Value: float64(iface.txPackets), Unit: "count", Labels: labels},
		{MetricKey: "system.network.rx.errors", Value: float64(iface.rxErrors), Unit: "count", Labels: labels},
		{MetricKey: "system.network.tx.errors", Value: float64(iface.txErrors), Unit: "count", Labels: labels},
		{MetricKey: "system.network.rx.dropped", Value: float64(iface.rxDropped), Unit: "count", Labels: labels},
		{MetricKey: "system.network.tx.dropped", Value: float64(iface.txDropped), Unit: "count", Labels: labels},
	}
}

func netStateSample(iface, rawState string) telemetry.Sample {
	return telemetry.Sample{
		MetricKey: "system.network.interface.state",
		Value:     1,
		Unit:      "state",
		Labels: map[string]string{
			"interface": iface,
			"state":     normalizeOperstate(rawState),
		},
	}
}

const operstateReadLimit = 64

func readOperstate(iface string) string {
	// Bound path to sysfs class/net only; reject traversal via name checks already applied.
	path := filepath.Join("/sys/class/net", iface, "operstate")
	if !strings.HasPrefix(path, "/sys/class/net/") {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	return readBoundedTrimmed(f, operstateReadLimit)
}

// readBoundedTrimmed reads at most limit bytes from r (via LimitReader) and returns
// the trimmed string. Unreadable/empty input yields "" (normalized to unknown).
func readBoundedTrimmed(r io.Reader, limit int64) string {
	if limit <= 0 {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func normalizeOperstate(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "up":
		return "up"
	case "down":
		return "down"
	case "unknown":
		return "unknown"
	case "dormant":
		return "dormant"
	case "lowerlayerdown":
		return "lowerlayerdown"
	case "testing":
		return "testing"
	case "notpresent":
		return "notpresent"
	case "":
		return "unknown"
	default:
		return "other"
	}
}

// -----------------------------------------------------------------------------
// OS Facts Collector
// -----------------------------------------------------------------------------

type OSFactsCollector struct{}

func NewOSFactsCollector() *OSFactsCollector { return &OSFactsCollector{} }

func (c *OSFactsCollector) Name() string { return "osfacts" }

func (c *OSFactsCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, fmt.Errorf("PROCSTAT_READ_FAILED: %w", err)
	}
	defer f.Close()

	facts, ok := parseOSFacts(f)
	if !ok {
		return nil, fmt.Errorf("PROCSTAT_PARSE_FAILED: required osfacts fields missing")
	}
	return osfactsSamples(facts), nil
}

type osFacts struct {
	btime        uint64
	hasBtime     bool
	procsRunning uint64
	hasRunning   bool
	procsBlocked uint64
	hasBlocked   bool
}

// Allow the CPU section and interrupt counters to precede the three scalar
// rows. The byte budget bounds work; the reader retains only one 4 KiB chunk.
const osFactsReadLimit = 4 << 20

func parseOSFacts(r io.Reader) (osFacts, bool) {
	br := bufio.NewReader(io.LimitReader(r, osFactsReadLimit))
	var facts osFacts
	var skipRemainder bool
	for {
		line, err := br.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			skipRemainder = true
			continue
		}
		if err != nil {
			// Kernel rows end in newline. A partial number at the byte
			// budget, at EOF or at a read failure is not a scalar value.
			return osFacts{}, false
		}
		if skipRemainder {
			skipRemainder = false
			continue
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 || (line[0] != 'b' && line[0] != 'p') {
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) != 2 {
			continue
		}
		val, err := strconv.ParseUint(string(fields[1]), 10, 64)
		if err != nil {
			continue
		}
		switch string(fields[0]) {
		case "btime":
			facts.btime = val
			facts.hasBtime = true
		case "procs_running":
			facts.procsRunning = val
			facts.hasRunning = true
		case "procs_blocked":
			facts.procsBlocked = val
			facts.hasBlocked = true
		}
		if facts.hasBtime && facts.hasRunning && facts.hasBlocked {
			return facts, true
		}
	}
}

func osfactsSamples(facts osFacts) []telemetry.Sample {
	return []telemetry.Sample{
		{MetricKey: "system.boot.time", Value: float64(facts.btime), Unit: "s"},
		{MetricKey: "system.processes.running", Value: float64(facts.procsRunning), Unit: "count"},
		{MetricKey: "system.processes.blocked", Value: float64(facts.procsBlocked), Unit: "count"},
	}
}

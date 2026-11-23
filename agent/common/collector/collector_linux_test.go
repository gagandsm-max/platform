//go:build linux

package collector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestUnescapeMountField(t *testing.T) {
	if got := unescapeMountField(`/data/my\040disk`); got != "/data/my disk" {
		t.Fatalf("got %q", got)
	}
}

func TestCPUFirstSampleSkipsUtilization(t *testing.T) {
	cpu := NewCPUCollector()
	samples, err := cpu.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range samples {
		if s.MetricKey == "system.cpu.utilization" {
			t.Fatal("first CPU sample must not emit utilization")
		}
	}
	time.Sleep(20 * time.Millisecond)
	samples2, err := cpu.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range samples2 {
		if s.MetricKey == "system.cpu.utilization" {
			found = true
			if s.Value < 0 || s.Value > 100 {
				t.Fatalf("util out of bounds: %v", s.Value)
			}
		}
	}
	// Zero counter delta between samples is allowed on an idle container; seeded state is the gate.
	_ = found
	if cpu.previous.count == 0 {
		t.Fatal("expected CPU counters to be seeded after first sample")
	}
}

func TestCPUCollectCounterDiscontinuities(t *testing.T) {
	cpu := NewCPUCollector()
	cpu.procRoot = t.TempDir()
	writeProc := func(t *testing.T, name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(cpu.procRoot, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeProc(t, "loadavg", "1.25 2.50 3.75 1/100 42")
	writeProc(t, "uptime", "420 1000")

	// The same collector must recover on the first valid interval after each gap.
	for _, step := range []struct {
		name     string
		row      string  // user, nice, system, idle, iowait, irq, softirq, steal, guest, guest_nice
		wantUtil float64 // -1 means there is no usable interval.
	}{
		{"initial", "100 0 0 800 100", -1},
		{"ordinary", "105 0 0 800 115", 25},
		{"idle+iowait went backwards", "117 0 0 802 105", -1},
		{"recovered", "122 0 0 817 105", 25},
		{"idle delta exceeded total delta", "118 0 0 827 105", -1},
		{"genuinely idle", "118 0 0 832 105", 0},
		{"genuinely busy", "123 0 0 832 105", 100},
		{"unchanged", "123 0 0 832 105", -1},
		{"total reset", "2 0 0 5 1", -1},
		{"recovered after reset", "3 0 0 8 1", 25},
		{"layout gained accounted fields", "100 0 40 60 10 0 0 0 1 1", -1},
		{"iowait regression masked by idle increase", "105 0 40 65 5 0 0 0 1 1", -1},
		{"recovered from iowait regression", "107 0 41 67 6 0 0 0 1 1", 50},
		{"user regression masked by system increase", "103 0 45 75 6 0 0 0 1 1", -1},
		{"recovered from user regression", "106 0 46 77 8 0 0 0 1 1", 50},
		{"guest rollback excluded", "107 0 46 78 8 0 0 0 0 0", 50},
		{"fewer than four fields", "200 3 4", -1},
		{"reseed after short row", "200 3 4 100", -1},
		{"four field layout", "201 3 4 103", 25},
		{"malformed accounted field", "205 3 corrupt 108", -1},
		{"reseed after malformed row", "210 3 4 110", -1},
		{"valid again", "213 3 4 111", 75},
		{"layout grows", "220 3 4 111 0 0", -1},
		{"six field layout", "220 3 4 114 0 1", 25},
		{"layout shrinks", "230 3 4 115", -1},
		{"unrepresentable delta total", "18446744073709551615 300 4 115", -1},
		{"small delta despite overflowing cumulative sum", "18446744073709551615 301 4 118", 25},
		{"duplicate aggregate", "1 1 1 1\ncpu 2 2 2 2", -1},
		{"reseed after duplicate", "30 1 2 20 0 0 0", -1},
		{"seven field layout", "31 1 2 26 0 0 1", 25},
		{"counter value too large", "18446744073709551616 1 2 26 0 0 1", -1},
		{"reseed at all zeros", "0 0 0 0 0 0 0", -1},
		{"valid interval from zero", "1 0 0 7 0 0 0", 12.5},
	} {
		t.Run(step.name, func(t *testing.T) {
			writeProc(t, "stat", "cpu "+step.row+"\ncpu0 0 0 0 0\n")
			samples, err := cpu.Collect(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			values := make(map[string]float64)
			for _, s := range samples {
				values[s.MetricKey] = s.Value
			}
			for key, want := range map[string]float64{
				"system.cpu.count": 1, "system.load.1m": 1.25, "system.load.5m": 2.5,
				"system.load.15m": 3.75, "system.uptime": 420,
			} {
				if got, ok := values[key]; !ok || got != want {
					t.Errorf("%s: got %v (present=%v), want %v", key, got, ok, want)
				}
			}
			got, present := values["system.cpu.utilization"]
			if present != (step.wantUtil >= 0) || (present && got != step.wantUtil) {
				t.Errorf("utilization=%v present=%v, expectation=%v", got, present, step.wantUtil)
			}
		})
	}
}

func TestCPUCollectCountsCompleteLargeTopologies(t *testing.T) {
	const budget = 2 << 20
	const aggregate = "cpu 110 0 0 110\n"
	const counters = " 142953005 179814 77215431 1646430653 4453721 19874 1174722 8276 0 0"
	rows := func(cpus int, exactSize int) string {
		var b strings.Builder
		b.WriteString(aggregate)
		for i := 0; i < cpus; i++ {
			fmt.Fprintf(&b, "cpu%d%s", i, counters)
			if exactSize > 0 {
				// Evenly pad valid rows to place the final newline exactly at the boundary.
				end := len(aggregate) + (i+1)*(exactSize-len(aggregate))/cpus
				b.WriteString(strings.Repeat(" ", end-b.Len()-1))
			}
			b.WriteByte('\n')
		}
		return b.String()
	}
	large := rows(1536, 0)
	exact := rows(8192, budget)
	for _, tc := range []struct {
		name string
		stat string
		want int // Zero means the full count cannot be established.
	}{
		{"large topology and long interrupt tail", large + "intr 0" + strings.Repeat(" 0", 100000) + "\n", 1536},
		{"EOF at byte budget", exact, 8192},
		{"budget at complete row but more CPUs", exact + "cpu8192" + counters + "\n", 0},
		{"budget in CPU row", exact[:len(exact)-1] + " 0\n", 0},
		{"incomplete final CPU row", large + "cpu1536 1", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCPUCollector()
			c.procRoot = t.TempDir()
			write := func(name, contents string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(c.procRoot, name), []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write("stat", "cpu 100 0 0 100\ncpu0 0 0 0 0\n")
			write("loadavg", "1 2 3 1/10 1\n")
			write("uptime", "850 4500\n")
			if _, err := c.Collect(context.Background()); err != nil {
				t.Fatal(err)
			}
			write("stat", tc.stat)
			samples, err := c.Collect(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]float64)
			for _, sample := range samples {
				got[sample.MetricKey] = sample.Value
			}
			if count, present := got["system.cpu.count"]; present != (tc.want > 0) || count != float64(tc.want) {
				t.Errorf("CPU count = %v (present %v), want %d", count, present, tc.want)
			}
			for key, want := range map[string]float64{"system.cpu.utilization": 50, "system.load.1m": 1, "system.uptime": 850} {
				if got[key] != want {
					t.Errorf("%s = %v, want %v", key, got[key], want)
				}
			}
		})
	}
}

func TestReadCPUStatStopsAtInterruptsAndWithholdsCountOnIOError(t *testing.T) {
	const rows = "cpu 2 4 6 8\ncpu0 1 2 3 4\ncpu5 1 2 3 4\n"
	tail := strings.NewReader("intr 10" + strings.Repeat(" 0", 100000) + "\n")
	aggregate, count := readCPUStat(io.MultiReader(strings.NewReader(rows), tail))
	if aggregate.count != 4 || count != 2 {
		t.Fatalf("sparse CPU indices: aggregate fields %d, count %d", aggregate.count, count)
	}
	if tail.Len() < 190000 {
		t.Fatalf("consumed unnecessary interrupt counters, %d bytes remain", tail.Len())
	}
	aggregate, count = readCPUStat(io.MultiReader(strings.NewReader(rows), iotest.ErrReader(errors.New("read failed"))))
	if aggregate.count != 4 || count != 0 {
		t.Fatalf("interrupted CPU block: aggregate fields %d, count %d", aggregate.count, count)
	}
}

//go:build linux

package collector

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCPUSourceFailuresAreExplicit(t *testing.T) {
	for _, path := range []string{"/proc/stat", "/proc/loadavg", "/proc/uptime"} {
		t.Run(path, func(t *testing.T) {
			c := NewCPUCollector()
			c.readFile = func(p string) ([]byte, error) {
				if p == path {
					return nil, errors.New("private source detail")
				}
				return cpuFixture(p, "cpu 10 0 10 80 0 0 0 0\ncpu0 10 0 10 80\n"), nil
			}
			v, err := c.Collect(context.Background())
			if err == nil || len(v) != 0 || strings.Contains(err.Error(), "private") {
				t.Fatalf("samples=%v err=%v", v, err)
			}
			if c.prevTotal != 0 {
				t.Fatal("failed collection changed interval baseline")
			}
		})
	}
}

func cpuFixture(path, stat string) []byte {
	switch path {
	case "/proc/stat":
		return []byte(stat)
	case "/proc/loadavg":
		return []byte("1 2 3 1/5 123")
	default:
		return []byte("99 0")
	}
}

func TestCPUCounterResetDoesNotInventUtilization(t *testing.T) {
	c := NewCPUCollector()
	stat := "cpu 10 0 10 80 0 0 0 0\ncpu0 10 0 10 80\n"
	c.readFile = func(p string) ([]byte, error) { return cpuFixture(p, stat), nil }
	if _, err := c.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Total increases while idle regresses: the old path invented 100% CPU.
	stat = "cpu 70 0 20 70 0 0 0 0\ncpu0 70 0 20 70\n"
	v, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range v {
		if s.MetricKey == "system.cpu.utilization" {
			t.Fatalf("fabricated utilization: %v", s)
		}
		if s.MetricKey == "system.cpu.utilization.available" && (s.Value != 0 || s.Labels["reason"] != "counter_reset") {
			t.Fatalf("wrong quality: %v", s)
		}
	}
	stat = "cpu 80 0 20 80 0 0 0 0\ncpu0 80 0 20 80\n"
	v, err = c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range v {
		if s.MetricKey == "system.cpu.utilization" {
			found = true
			if s.Value != 50 {
				t.Fatalf("utilization=%v", s.Value)
			}
		}
	}
	if !found {
		t.Fatal("monotonic interval did not recover")
	}
}

func TestCPURejectsInvalidCountersAndNumbers(t *testing.T) {
	for _, raw := range []string{"", "cpu 1 2\ncpu0 1 2", "cpu 18446744073709551615 1 2 3\ncpu0 1 2", "cpu x 2 3 4\ncpu0 1 2", "cpu 1 2 3 4\ncpu 1 2 3 4\ncpu0 1 2"} {
		if _, _, _, err := parseCPUStat([]byte(raw)); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	for _, raw := range []string{"NaN", "+Inf", "-1", "", "1e999"} {
		if _, err := parseNonnegativeFloats([]byte(raw), 1); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	total, idle, count, err := parseCPUStat([]byte("cpu 10 0 10 80 0 0 0 0 99 99\ncpu0 1 2 3 4\n"))
	if err != nil || total != 100 || idle != 80 || count != 1 {
		t.Fatalf("guest double-count or parse error: %d %d %d %v", total, idle, count, err)
	}
}

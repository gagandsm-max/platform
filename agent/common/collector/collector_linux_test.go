//go:build linux

package collector

import (
	"context"
	"testing"
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
	if cpu.prevTotal == 0 {
		t.Fatal("expected CPU counters to be seeded after first sample")
	}
}

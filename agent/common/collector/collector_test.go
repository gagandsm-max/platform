package collector

import (
	"context"
	"runtime"
	"testing"
)

func TestCollectorsExecution(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Host collectors tested on Linux")
	}

	collectors := NewDefaultCollectors(16)
	if len(collectors) < 3 {
		t.Fatalf("expected at least 3 collectors, got %d", len(collectors))
	}

	ctx := context.Background()
	for _, c := range collectors {
		samples, err := c.Collect(ctx)
		if err != nil {
			t.Fatalf("collector %s failed: %v", c.Name(), err)
		}
		if len(samples) == 0 {
			t.Fatalf("collector %s produced 0 samples", c.Name())
		}
		for _, s := range samples {
			if s.MetricKey == "" {
				t.Fatalf("collector %s produced sample with empty metric key", c.Name())
			}
			if s.Unit == "" {
				t.Fatalf("collector %s produced sample %s with empty unit", c.Name(), s.MetricKey)
			}
		}
	}
}

func TestCollectorContextCancellation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Host collectors tested on Linux")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cpu := NewCPUCollector()
	_, err := cpu.Collect(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}

	mem := NewMemoryCollector()
	_, err = mem.Collect(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}

	fs := NewFilesystemCollector(4)
	_, err = fs.Collect(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

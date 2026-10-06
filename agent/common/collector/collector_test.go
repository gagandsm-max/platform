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
	wantNames := []string{"cpu", "memory", "filesystem", "pressure", "diskio", "net", "osfacts"}
	if len(collectors) != len(wantNames) {
		t.Fatalf("expected %d collectors, got %d", len(wantNames), len(collectors))
	}

	// Certified Slice-0/BUILD-010 collectors retain strict regression gates.
	strictCollectors := map[string]struct{}{
		"cpu": {}, "memory": {}, "filesystem": {},
	}

	ctx := context.Background()
	for i, c := range collectors {
		if c.Name() != wantNames[i] {
			t.Fatalf("collector[%d]=%s want %s", i, c.Name(), wantNames[i])
		}
		samples, err := c.Collect(ctx)
		_, strict := strictCollectors[c.Name()]
		if err != nil {
			if strict {
				t.Fatalf("collector %s returned unexpected error: %v", c.Name(), err)
			}
			// Slice-2 live collectors: explicit error is monitoring-health evidence, not host DOWN.
			t.Logf("collector %s returned explicit error: %v", c.Name(), err)
			continue
		}
		if len(samples) == 0 {
			if strict {
				t.Fatalf("collector %s produced 0 samples", c.Name())
			}
			t.Logf("collector %s produced 0 samples (allowed when source empty after bounds)", c.Name())
			continue
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

	for _, c := range NewDefaultCollectors(4) {
		if _, err := c.Collect(ctx); err == nil {
			t.Fatalf("expected error on cancelled context for %s", c.Name())
		}
	}
}

//go:build linux && resource

package collector

import (
	"context"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// Real proc/sys/statfs reads in a bounded container, not native certification.
// Run independently of race instrumentation for interpretable measurements.
func TestDefaultCollectorsResourceProfile(t *testing.T) {
	for repetition := 0; repetition < 3; repetition++ {
		collectors := NewDefaultCollectors(8)
		runtime.GC()
		var before, after runtime.MemStats
		var cpuBefore, cpuAfter syscall.Rusage
		runtime.ReadMemStats(&before)
		_ = syscall.Getrusage(syscall.RUSAGE_SELF, &cpuBefore)
		fdsBefore, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		goroutines := runtime.NumGoroutine()
		start := time.Now()
		sampleCount := 0
		for i := 0; i < 100; i++ {
			for _, c := range collectors {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				samples, err := c.Collect(ctx)
				cancel()
				if err != nil {
					t.Fatalf("%s: %v", c.Name(), err)
				}
				sampleCount += len(samples)
			}
			time.Sleep(100 * time.Millisecond)
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		_ = syscall.Getrusage(syscall.RUSAGE_SELF, &cpuAfter)
		fdsAfter, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("profile=container-real-default-collectors repetition=%d cycles=100 samples=%d elapsed_ms=%d cpu_user_us=%d cpu_system_us=%d heap_before=%d heap_after=%d alloc_bytes=%d goroutines_before=%d goroutines_after=%d fd_before=%d fd_after=%d maxrss_kib=%d block_in=%d block_out=%d", repetition, sampleCount, time.Since(start).Milliseconds(), cpuAfter.Utime.Nano()/1000-cpuBefore.Utime.Nano()/1000, cpuAfter.Stime.Nano()/1000-cpuBefore.Stime.Nano()/1000, before.HeapAlloc, after.HeapAlloc, after.TotalAlloc-before.TotalAlloc, goroutines, runtime.NumGoroutine(), len(fdsBefore), len(fdsAfter), cpuAfter.Maxrss, cpuAfter.Inblock-cpuBefore.Inblock, cpuAfter.Oublock-cpuBefore.Oublock)
		if runtime.NumGoroutine() > goroutines+2 || len(fdsAfter) > len(fdsBefore)+2 {
			t.Fatal("collector resources did not settle within worker bound")
		}
		if after.HeapAlloc > before.HeapAlloc+4*1024*1024 {
			t.Fatal("unexpected retained heap growth in bounded profile")
		}
	}
}

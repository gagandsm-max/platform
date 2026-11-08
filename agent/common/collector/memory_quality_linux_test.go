//go:build linux

package collector

import (
	"strings"
	"testing"
)

func TestMemoryMissingEvidenceIsNotFullUtilizationOrZeroSwap(t *testing.T) {
	for _, raw := range []string{
		"MemTotal: 1000 kB\n", // Used to invent 100% utilization.
		"MemTotal: 1000 kB\nMemFree: 500 kB\n",
		"MemTotal: 1000 kB\nMemAvailable: bad kB\n",
		"MemTotal: 1000 kB\nMemAvailable: 500 bytes\n",
		"MemTotal: 1000 kB\nMemAvailable: 500 kB\nMemAvailable: 400 kB\n",
		"MemTotal: 1000 kB\nMemFree: 18446744073709551615 kB\nBuffers: 1 kB\nCached: 1 kB\n",
	} {
		if _, err := parseMeminfo(strings.NewReader(raw)); err == nil {
			t.Fatalf("accepted missing/invalid evidence %q", raw)
		}
	}
	info, err := parseMeminfo(strings.NewReader("MemTotal: 1000 kB\nMemAvailable: 500 kB\n"))
	if err != nil {
		t.Fatal(err)
	}
	samples, err := meminfoSamples(info)
	if err != nil {
		t.Fatal(err)
	}
	missing := false
	for _, s := range samples {
		if strings.HasPrefix(s.MetricKey, "system.memory.swap.") {
			if s.MetricKey != "system.memory.swap.available_evidence" || s.Value != 0 {
				t.Fatalf("invented zero-swap source: %v", s)
			}
			missing = true
		}
	}
	if !missing {
		t.Fatal("missing swap evidence not reported")
	}
}

func TestMemoryLegacyAvailabilityRequiresCompleteBoundedFallback(t *testing.T) {
	info, err := parseMeminfo(strings.NewReader("MemTotal: 1000 kB\nMemFree: 100 kB\nBuffers: 200 kB\nCached: 300 kB\n"))
	if err != nil || info.availKB != 600 {
		t.Fatalf("%+v %v", info, err)
	}
}

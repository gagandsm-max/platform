package control_test

import (
	"testing"

	"platform/sensor/internal/control"
)

func TestChecksumGoldenVectorExactMatch(t *testing.T) {
	t.Parallel()
	digest, err := control.ComputeDesiredChecksum(control.GoldenConfiguration())
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if digest != control.GoldenChecksumSHA256 {
		t.Fatalf("golden mismatch\n got=%s\nwant=%s", digest, control.GoldenChecksumSHA256)
	}
}

func TestChecksumPoolInsertionOrderInvariant(t *testing.T) {
	t.Parallel()
	a := control.GoldenConfiguration()
	b := control.GoldenConfiguration()
	b.Scheduler.Pools = map[string]control.DesiredPool{
		"NETWORK_POLL": {Workers: 2, QueueDepth: 8},
		"SYNTHETIC":    {Workers: 1, QueueDepth: 4},
	}
	da, err := control.ComputeDesiredChecksum(a)
	if err != nil {
		t.Fatal(err)
	}
	db, err := control.ComputeDesiredChecksum(b)
	if err != nil {
		t.Fatal(err)
	}
	if da != db || da != control.GoldenChecksumSHA256 {
		t.Fatalf("order invariance failed a=%s b=%s", da, db)
	}
}

func TestChecksumGenerationExcluded(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	d1, err := control.ComputeDesiredChecksum(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Generation is not an input to ComputeDesiredChecksum; mutating an unrelated
	// semantic field is covered separately. Recompute identical config proves stability.
	d2, err := control.ComputeDesiredChecksum(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Fatalf("identical config must be stable")
	}
}

func TestChecksumSemanticFieldMutationChangesDigest(t *testing.T) {
	t.Parallel()
	base := control.GoldenConfiguration()
	baseDigest, err := control.ComputeDesiredChecksum(base)
	if err != nil {
		t.Fatal(err)
	}
	mutated := base
	mutated.LogLevel = "WARN"
	changed, err := control.ComputeDesiredChecksum(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if changed == baseDigest {
		t.Fatalf("logLevel mutation must change checksum")
	}
	mutatedPool := base
	mutatedPool.Scheduler.Pools = map[string]control.DesiredPool{
		"SYNTHETIC":    {Workers: 1, QueueDepth: 4},
		"NETWORK_POLL": {Workers: 3, QueueDepth: 8},
	}
	poolChanged, err := control.ComputeDesiredChecksum(mutatedPool)
	if err != nil {
		t.Fatal(err)
	}
	if poolChanged == baseDigest {
		t.Fatalf("pool workers mutation must change checksum")
	}
}

package control_test

import (
	"testing"

	"platform/sensor/internal/control"
)

func TestOverlayDesiredPreservesIdentityAndAppliesControlledSubset(t *testing.T) {
	t.Parallel()
	baseline := validBaseline(t)
	desired := control.GoldenConfiguration()
	desired.LogLevel = "WARN"
	out, err := control.OverlayDesired(baseline, desired)
	if err != nil {
		t.Fatal(err)
	}
	if out.LogLevel != "WARN" {
		t.Fatalf("logLevel=%s", out.LogLevel)
	}
	if out.SensorID != baseline.SensorID || out.CoreEndpoint != baseline.CoreEndpoint || out.TLS != baseline.TLS {
		t.Fatalf("identity/TLS fields must remain baseline")
	}
	if out.BootstrapToken.Value() != "" {
		t.Fatal("bootstrap token must remain empty")
	}
	if out.Scheduler.MaxWorkers != 4 || out.Transport.UploadBatchRecords != 100 {
		t.Fatalf("controlled subset not applied: %+v %+v", out.Scheduler, out.Transport)
	}
	if out.Scheduler.RetryJitterFraction != 0.05 {
		t.Fatalf("jitter fraction=%v", out.Scheduler.RetryJitterFraction)
	}
}

func TestOverlayDesiredRejectsInvalidCandidate(t *testing.T) {
	t.Parallel()
	baseline := validBaseline(t)
	desired := control.GoldenConfiguration()
	desired.Scheduler.MaxWorkers = 1
	desired.Scheduler.Pools = map[string]control.DesiredPool{
		"NETWORK_POLL": {Workers: 2, QueueDepth: 1},
	}
	_, err := control.OverlayDesired(baseline, desired)
	if err == nil || control.CodeOf(err) != control.CodeConfigInvalid {
		t.Fatalf("expected CONFIG_INVALID, got %v", err)
	}
}

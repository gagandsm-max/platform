package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"platform/agent/common"
	"platform/agent/common/client"
	"platform/agent/common/collector"
	"platform/agent/common/config"
	"platform/agent/common/identity"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testOptionalCfg(t *testing.T) *config.Config {
	t.Helper()
	tempDir := t.TempDir()
	return &config.Config{
		TenantID:           "tenant-test",
		SensorURL:          "https://sensor.platform.local:8443",
		CoreURL:            "https://core.platform.local:8443",
		StateDir:           tempDir,
		QueueDir:           filepath.Join(tempDir, "queue"),
		HealthAddress:      "127.0.0.1:0",
		CollectionInterval: time.Hour,
		CollectionTimeout:  500 * time.Millisecond,
		DeliveryTimeout:    500 * time.Millisecond,
		RetryBase:          10 * time.Millisecond,
		RetryMax:           50 * time.Millisecond,
		MaxBufferRecords:   100,
		MaxBufferBytes:     1024 * 1024,
		MaxRecordBytes:     64 * 1024,
		MaxBatchRecords:    10,
		MaxBatchBytes:      384 * 1024,
		MaxFilesystems:     8,
		EBPFEnabled:        true,
		EBPFRecheckMin:     time.Minute,
	}
}

func TestOptionalConstructionFailureDoesNotPreventBaseAgentStartup(t *testing.T) {
	cfg := testOptionalCfg(t)
	ident := &identity.AgentIdentity{AgentID: "agent-opt-construct", TenantID: cfg.TenantID, HostID: "host-opt"}
	logger := testLogger()

	opt := optionalProcessNetworkOption(cfg, logger, func(*config.Config) (common.ProcessNetworkRuntime, error) {
		return nil, errors.New("BACKEND_FIXTURE_CONSTRUCTION_FAIL")
	})
	svc, err := common.NewAgentService(cfg, ident, client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), logger, opt)
	if err != nil {
		t.Fatalf("NewAgentService must succeed after optional construction failure: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("base Agent Start must succeed: %v", err)
	}
	if got := svc.LifecycleState(); got != common.LifecycleRunning {
		t.Fatalf("lifecycle=%s", got)
	}
	if !svc.SchedulerStatus().Running {
		t.Fatal("scheduler must remain operational")
	}
	snap := svc.RuntimeSnapshot()
	if snap.ProcessNetworkCapability.State != collector.ProcessNetworkCapabilityUnavailable {
		t.Fatalf("capability state=%s", snap.ProcessNetworkCapability.State)
	}
	if len(snap.ProcessNetworkCapability.ReasonCodes) != 1 ||
		snap.ProcessNetworkCapability.ReasonCodes[0] != collector.ReasonSourceUnavailable {
		t.Fatalf("reasons=%v", snap.ProcessNetworkCapability.ReasonCodes)
	}
	if !snap.ProcessNetworkCapability.CurrentAuthority {
		t.Fatal("construction-failure evidence must remain current authority")
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestOptionalDisabledRemainsFeatureDisabledAuthority(t *testing.T) {
	cfg := testOptionalCfg(t)
	cfg.EBPFEnabled = false
	opt := optionalProcessNetworkOption(cfg, testLogger(), func(*config.Config) (common.ProcessNetworkRuntime, error) {
		t.Fatal("factory must not run when eBPF disabled")
		return nil, nil
	})
	svc, err := common.NewAgentService(cfg, &identity.AgentIdentity{AgentID: "agent-opt-disabled", TenantID: cfg.TenantID, HostID: "host-d"},
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), testLogger(), opt)
	if err != nil {
		t.Fatal(err)
	}
	snap := svc.RuntimeSnapshot().ProcessNetworkCapability
	if snap.State != collector.ProcessNetworkCapabilityUnavailable ||
		len(snap.ReasonCodes) != 1 || snap.ReasonCodes[0] != collector.ReasonFeatureDisabled ||
		!snap.CurrentAuthority {
		t.Fatalf("disabled capability=%+v", snap)
	}
	expired := snap.Fresh(snap.ValidUntil.Add(time.Hour))
	if expired.State != collector.ProcessNetworkCapabilityUnavailable ||
		len(expired.ReasonCodes) != 1 || expired.ReasonCodes[0] != collector.ReasonFeatureDisabled {
		t.Fatalf("FEATURE_DISABLED falsely staled: %+v", expired)
	}
}

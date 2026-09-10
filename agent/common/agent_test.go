package common

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"platform/agent/common/config"
	"platform/agent/common/identity"
)

func TestAgentServiceLifecycle(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &config.Config{
		TenantID:           "tenant-test",
		CoreURL:            "https://core.platform.local:8443",
		StateDir:           tempDir,
		QueueDir:           filepath.Join(tempDir, "queue"),
		HealthAddress:      "127.0.0.1:0", // ephemeral loopback port
		CollectionInterval: 50 * time.Millisecond,
		CollectionTimeout:  500 * time.Millisecond,
		DeliveryTimeout:    500 * time.Millisecond,
		RetryBase:          10 * time.Millisecond,
		RetryMax:           50 * time.Millisecond,
		MaxBufferRecords:   100,
		MaxBufferBytes:     1024 * 1024,
		MaxRecordBytes:     64 * 1024,
		MaxBatchRecords:    10,
		MaxBatchBytes:      1024 * 1024,
		MaxFilesystems:     8,
	}

	ident := &identity.AgentIdentity{
		AgentID:  "agent-test-svc",
		TenantID: "tenant-test",
		HostID:   "host-test-svc",
		Hostname: "node-test",
		OS:       "linux",
		Arch:     "amd64",
	}

	svc, err := NewAgentService(cfg, ident, nil, nil)
	if err != nil {
		t.Fatalf("failed to construct AgentService: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("failed to start AgentService: %v", err)
	}

	// Allow one collection cycle
	time.Sleep(100 * time.Millisecond)

	qStats := svc.QueueStats()
	if qStats.ActiveCount < 1 {
		t.Fatalf("expected at least 1 record buffered in queue, got %d", qStats.ActiveCount)
	}

	dStats := svc.DeliveryStats()
	if dStats.ReceiverAvailable {
		t.Fatal("expected receiver to not be available by default")
	}

	cancel()
	if err := svc.Stop(); err != nil {
		t.Fatalf("failed to stop AgentService: %v", err)
	}
}

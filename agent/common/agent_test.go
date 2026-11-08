package common

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/collector"
	"platform/agent/common/config"
	"platform/agent/common/identity"
)

func TestAgentServiceLifecycle(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &config.Config{
		TenantID:           "tenant-test",
		SensorURL:          "https://sensor.platform.local:8443",
		CoreURL:            "https://core.platform.local:8443",
		StateDir:           tempDir,
		QueueDir:           filepath.Join(tempDir, "queue"),
		HealthAddress:      "127.0.0.1:0",
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

	stub := client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST")
	svc, err := NewAgentService(cfg, ident, stub, nil, withCollectorRegistryForTest(singleCollectorRegistry(t, &integrityLifecycleCollector{}, cfg.CollectionInterval, cfg.CollectionTimeout)))
	if err != nil {
		t.Fatalf("failed to construct AgentService: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("failed to start AgentService: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	qStats := svc.QueueStats()
	if qStats.ActiveCount < 1 {
		t.Fatalf("expected at least 1 record buffered in queue, got %d", qStats.ActiveCount)
	}

	dStats := svc.DeliveryStats()
	if dStats.ReceiverAvailable {
		t.Fatal("expected injected unavailable transport to remain unavailable")
	}

	// Service owns runtime cancellation; caller cancel is not required for Stop.
	cancel()
	if err := svc.Stop(); err != nil {
		t.Fatalf("failed to stop AgentService: %v", err)
	}
	if got := svc.LifecycleState(); got != LifecycleStopped {
		t.Fatalf("expected STOPPED after clean Stop, got %s", got)
	}
	if qStats := svc.QueueStats(); qStats.ActiveCount < 1 {
		t.Fatal("shutdown must retain unacknowledged queue records")
	}
}

func TestAgentServiceWiresHTTPSTransport(t *testing.T) {
	tempDir := t.TempDir()
	certPath, keyPath := generateServiceTestCert(t, tempDir)

	cfg := &config.Config{
		TenantID:           "tenant-test",
		SensorURL:          "https://sensor.platform.local:8443",
		CAFile:             certPath,
		CertFile:           certPath,
		KeyFile:            keyPath,
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
		MaxBatchBytes:      1024 * 1024,
		MaxFilesystems:     8,
	}
	ident := &identity.AgentIdentity{
		AgentID:  "agent-prod-wire",
		TenantID: "tenant-test",
		HostID:   "host-prod-wire",
		Hostname: "node-test",
		OS:       "linux",
		Arch:     "amd64",
	}

	svc, err := NewAgentService(cfg, ident, nil, nil)
	if err != nil {
		t.Fatalf("production NewAgentService should wire HTTPS transport: %v", err)
	}
	if !svc.DeliveryStats().ReceiverAvailable {
		t.Fatal("production transport must report receiver available (Sensor contract is wired)")
	}
	if _, ok := svc.transport.(*client.HTTPSTransport); !ok {
		t.Fatalf("expected *client.HTTPSTransport, got %T", svc.transport)
	}
}

func TestOptionalEBPFCapabilityDoesNotBlockSchedulerStartup(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		TenantID:           "tenant-test",
		SensorURL:          "https://sensor.platform.local:8443",
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
	}
	ident := &identity.AgentIdentity{AgentID: "agent-ebpf-optional", TenantID: "tenant-test", HostID: "host-ebpf-optional", OS: "linux", Arch: "amd64"}
	capability := collector.ProcessNetworkCapabilitySnapshot{
		State:       collector.ProcessNetworkCapabilityKernelIncompatible,
		ReasonCodes: []collector.ProcessNetworkCapabilityReason{collector.ReasonBTFMissing},
		ObservedAt:  time.Now().UTC(),
		ValidUntil:  time.Now().UTC().Add(time.Hour),
	}
	svc, err := NewAgentService(cfg, ident, client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkCapability(capability))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("optional eBPF evidence blocked Agent start: %v", err)
	}
	defer func() { _ = svc.Stop() }()
	snapshot := svc.RuntimeSnapshot()
	if !snapshot.SchedulerRunning {
		t.Fatal("scheduler did not start while optional eBPF was incompatible")
	}
	if snapshot.ProcessNetworkCapability.State != collector.ProcessNetworkCapabilityKernelIncompatible {
		t.Fatalf("capability snapshot not preserved: %+v", snapshot.ProcessNetworkCapability)
	}
}

func generateServiceTestCert(t *testing.T, dir string) (string, string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "agent.platform.local"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, "agent.crt")
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_ = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	_ = certFile.Close()
	keyPath := filepath.Join(dir, "agent.key")
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_ = pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	_ = keyFile.Close()
	return certPath, keyPath
}

package snmpintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/observation"
	protocolsnmp "platform/sensor/internal/protocol/snmp"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
	"platform/sensor/internal/uploader"
)

// startLocalSNMPv2cServer launches a real UDP SNMP v2c server on an ephemeral loopback port
// that deterministically responds to GetRequest for OID .1.3.6.1.2.1.1.3.0 with TimeTicks 1234567.
func startLocalSNMPv2cServer(t *testing.T, expectedOID string, numericVal uint32) (int, func()) {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("failed to bind local SNMP UDP listener: %v", err)
	}

	port := conn.LocalAddr().(*net.UDPAddr).Port
	done := make(chan struct{})

	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			reqPacket, err := (&gosnmp.GoSNMP{}).UnmarshalTrap(buf[:n], false)
			if err != nil {
				continue
			}
			oid := expectedOID
			if len(reqPacket.Variables) > 0 && reqPacket.Variables[0].Name != "" {
				oid = reqPacket.Variables[0].Name
			}
			respPacket := &gosnmp.SnmpPacket{
				Version:   gosnmp.Version2c,
				Community: reqPacket.Community,
				PDUType:   gosnmp.GetResponse,
				RequestID: reqPacket.RequestID,
				Error:     gosnmp.NoError,
				Variables: []gosnmp.SnmpPDU{
					{
						Name:  oid,
						Type:  gosnmp.TimeTicks,
						Value: numericVal,
					},
				},
			}
			respBytes, err := respPacket.MarshalMsg()
			if err != nil {
				continue
			}
			_, _ = conn.WriteToUDP(respBytes, remoteAddr)
		}
	}()

	cleanup := func() {
		_ = conn.Close()
		<-done
	}

	return port, cleanup
}

type recordingTransport struct {
	batches       []coreclient.Batch
	failNext      bool
	failTransient bool
}

func (r *recordingTransport) Upload(ctx context.Context, b coreclient.Batch) (coreclient.UploadAck, error) {
	if r.failNext {
		r.failNext = false
		if r.failTransient {
			return coreclient.UploadAck{}, &uploader.Error{}
		}
		return coreclient.UploadAck{}, fmt.Errorf("transport rejected upload")
	}
	r.batches = append(r.batches, b)
	seq := b.LastSequence
	return coreclient.UploadAck{
		Status:                  "ACCEPTED",
		AcceptedThroughSequence: &seq,
		SensorIdentity:          b.SensorID,
	}, nil
}

func TestRealSNMPPipelineE2E(t *testing.T) {
	const (
		testOID        = ".1.3.6.1.2.1.1.3.0"
		expectedVal    = uint32(1234567)
		expectedSensor = "sensor-net-01"
		expectedTenant = "018f0e0a-0000-7000-8000-000000000001"
		expectedEntity = "018f0e0a-0000-7000-8000-000000000011"
	)

	// 1. Start local deterministic SNMP v2c UDP server
	snmpPort, stopServer := startLocalSNMPv2cServer(t, testOID, expectedVal)
	defer stopServer()

	// 2. Real SNMP Client & Executor
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	collector := protocolsnmp.NewCollector(nil)
	baseExecutor, err := protocolsnmp.NewExecutor(collector, 1<<20)
	if err != nil {
		t.Fatalf("failed to create SNMP base executor: %v", err)
	}

	// 3. Real Disk Spool & Observation Sink
	spoolDir := t.TempDir()
	diskSpool, err := spool.Open(spool.Config{
		Dir:            spoolDir,
		MaxRecordBytes: 1024 * 1024,
		Logger:         logger,
	})
	if err != nil {
		t.Fatalf("failed to open disk spool: %v", err)
	}

	sink, err := observation.NewDiskSink(diskSpool, 1024*1024)
	if err != nil {
		t.Fatalf("failed to create disk sink: %v", err)
	}

	// 4. Scheduler / Integration Executor
	integrationExec, err := NewExecutor(baseExecutor, sink, Config{
		SensorID:           expectedSensor,
		MaxSNMPResultBytes: 1024 * 1024,
		MaxEnvelopeBytes:   1024 * 1024,
	})
	if err != nil {
		t.Fatalf("failed to create integration executor: %v", err)
	}

	// 5. Execute Real SNMP Collection Job over UDP
	snmpReq := protocolsnmp.Request{
		Target:           "127.0.0.1",
		Port:             snmpPort,
		Version:          protocolsnmp.Version2c,
		Credentials:      protocolsnmp.Credentials{Community: &protocolsnmp.CommunityCredentials{Community: protocolsnmp.Secret("public")}},
		OIDs:             []string{testOID},
		Operation:        protocolsnmp.OperationGet,
		Timeout:          2 * time.Second,
		ExecutionTimeout: 5 * time.Second,
		Retry:            protocolsnmp.RetryPolicy{MaxAttempts: 1},
		Limits: protocolsnmp.Limits{
			MaxOIDs:            1,
			MaxWalkEntries:     128,
			MaxBulkRepetitions: 10,
			MaxResults:         10,
			MaxResultBytes:     1024 * 1024,
		},
		CorrelationID:    "e2e-snmp-test",
	}
	payload, err := protocolsnmp.EncodeSchedulerPayload(snmpReq)
	if err != nil {
		t.Fatalf("failed to encode scheduler payload: %v", err)
	}

	job := scheduler.Job{
		JobID:         "job-snmp-1",
		AssignmentKey: "assignment-snmp-1",
		TenantID:      scheduler.TenantID(expectedTenant),
		EntityID:      scheduler.EntityID(expectedEntity),
		Kind:          protocolsnmp.SchedulerJobKind,
		Class:         scheduler.ClassNetworkPoll,
		Payload:       payload,
		Attempt:       1,
	}

	execResult, err := integrationExec.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("integration execution failed over real UDP SNMP: %v", err)
	}
	if len(execResult.Payload) == 0 {
		t.Fatalf("expected non-empty execution receipt payload")
	}

	// 6. Verify Spool State
	stats, err := diskSpool.Stats()
	if err != nil {
		t.Fatalf("failed to read spool stats: %v", err)
	}
	if stats.RecordCount != 1 {
		t.Fatalf("expected 1 spooled record, got %d", stats.RecordCount)
	}

	spoolBatch, err := diskSpool.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatalf("failed to read batch from spool: %v", err)
	}
	if len(spoolBatch.Records) != 1 {
		t.Fatalf("expected 1 batch record, got %d", len(spoolBatch.Records))
	}
	rec := spoolBatch.Records[0]
	if rec.Sequence != 1 {
		t.Fatalf("expected sequence 1, got %d", rec.Sequence)
	}

	var env observation.Envelope
	if err := json.Unmarshal(rec.Payload, &env); err != nil {
		t.Fatalf("failed to unmarshal spool envelope: %v", err)
	}
	if env.Protocol != "SNMP" {
		t.Fatalf("expected protocol SNMP, got %s", env.Protocol)
	}
	if env.TenantID != expectedTenant || env.EntityID != expectedEntity || env.SensorID != expectedSensor {
		t.Fatalf("scope mismatch in envelope: %+v", env)
	}

	// 7. Verify Uploader Drains Spool
	tr := &recordingTransport{}
	up, err := uploader.New(uploader.Config{
		SensorID:             expectedSensor,
		StreamID:             diskSpool.StreamID(),
		MaxBatchRecords:      10,
		MaxBatchPayloadBytes: 1024 * 1024,
		MaxRequestBytes:      2 * 1024 * 1024,
		MaxAttempts:          3,
		InitialBackoff:       50 * time.Millisecond,
		MaxBackoff:           200 * time.Millisecond,
		JitterFraction:       0.1,
		IdleInterval:         100 * time.Millisecond,
	}, diskSpool, tr)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	uploaded, err := up.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("uploader RunOnce failed: %v", err)
	}
	if !uploaded {
		t.Fatalf("expected batch to be uploaded")
	}

	if len(tr.batches) != 1 {
		t.Fatalf("expected 1 uploaded batch, got %d", len(tr.batches))
	}
	uploadedBatch := tr.batches[0]
	if uploadedBatch.FirstSequence != 1 || uploadedBatch.LastSequence != 1 {
		t.Fatalf("batch sequence bounds mismatch: [%d..%d]", uploadedBatch.FirstSequence, uploadedBatch.LastSequence)
	}
	if uploadedBatch.BootEpoch != diskSpool.StreamID() {
		t.Fatalf("wire bootEpoch must carry durable spool stream ID, got %q want %q", uploadedBatch.BootEpoch, diskSpool.StreamID())
	}
	if uploadedBatch.Checksum == "" {
		t.Fatalf("batch checksum must not be empty")
	}

	// Spool must now be acknowledged through sequence 1
	postStats, err := diskSpool.Stats()
	if err != nil {
		t.Fatalf("failed to read post stats: %v", err)
	}
	if postStats.RecordCount != 0 {
		t.Fatalf("expected 0 pending records after ACK, got %d", postStats.RecordCount)
	}

	// Record file must be deleted from disk
	recordPath := filepath.Join(spoolDir, fmt.Sprintf("record-%020d.json", 1))
	if _, err := os.Stat(recordPath); !os.IsNotExist(err) {
		t.Fatalf("acknowledged record file still exists on disk: %s", recordPath)
	}
}

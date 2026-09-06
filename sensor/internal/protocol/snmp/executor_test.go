package snmp

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"platform/sensor/internal/scheduler"
)

func TestSchedulerPayloadDecodeValidation(t *testing.T) {
	req := baseV2Request()
	payload, err := EncodeSchedulerPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSchedulerPayload(payload, len(payload)+1)
	if err != nil {
		t.Fatalf("DecodeSchedulerPayload() error = %v", err)
	}
	if decoded.Target != req.Target || decoded.Operation != req.Operation || decoded.Credentials.Community.Community.value() != testCommunity {
		t.Fatalf("decoded request mismatch: %s", decoded.String())
	}

	unknown := strings.TrimSuffix(string(payload), "}") + `,"unexpected":true}`
	if _, err := DecodeSchedulerPayload([]byte(unknown), len(unknown)+1); !IsCode(err, CodeInvalidRequest) {
		t.Fatalf("unknown field error = %v", err)
	}
	trailing := append(append([]byte(nil), payload...), []byte(` {}`)...)
	if _, err := DecodeSchedulerPayload(trailing, len(trailing)+1); !IsCode(err, CodeInvalidRequest) {
		t.Fatalf("trailing document error = %v", err)
	}
	if _, err := DecodeSchedulerPayload(payload, len(payload)-1); !IsCode(err, CodeResourceLimit) {
		t.Fatalf("payload bound error = %v", err)
	}
}

func TestDecodedRequestDiagnosticRedactsSchedulerSecrets(t *testing.T) {
	req := baseV3Request(SecurityAuthPriv)
	payload, err := EncodeSchedulerPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSchedulerPayload(payload, len(payload)+1)
	if err != nil {
		t.Fatal(err)
	}
	rendered := decoded.String()
	if strings.Contains(rendered, testAuth) || strings.Contains(rendered, testPrivacy) {
		t.Fatalf("decoded request diagnostic leaked secret: %s", rendered)
	}
}

func TestSchedulerExecutorIntegration(t *testing.T) {
	s := newFakeSession()
	s.getPacket = valuePacket(".1.3.6.1.2.1.1.3.0", gosnmp.TimeTicks, uint32(123))
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	executor, err := NewExecutor(collector, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	var contract scheduler.Executor = executor

	req := baseV2Request()
	payload, err := EncodeSchedulerPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	result, err := contract.Execute(context.Background(), scheduler.Job{
		JobID:         "snmp-job-1",
		AssignmentKey: "assignment-1",
		TenantID:      "tenant-1",
		EntityID:      "entity-1",
		Kind:          SchedulerJobKind,
		Class:         scheduler.ClassNetworkPoll,
		Payload:       payload,
		Attempt:       1,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var decoded CollectionResult
	if err := json.Unmarshal(result.Payload, &decoded); err != nil {
		t.Fatalf("result JSON invalid: %v", err)
	}
	if len(decoded.Observations) != 1 {
		t.Fatalf("observations = %d, want 1", len(decoded.Observations))
	}
}

func TestSchedulerOutputCounter64IsLosslessString(t *testing.T) {
	s := newFakeSession()
	s.getPacket = valuePacket(".1.3.6.1.2.1.31.1.1.1.10.1", gosnmp.Counter64, uint64(math.MaxUint64))
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	executor, _ := NewExecutor(collector, 1<<20)
	req := baseV2Request()
	req.OIDs = []string{".1.3.6.1.2.1.31.1.1.1.10.1"}
	payload, _ := EncodeSchedulerPayload(req)
	result, err := executor.Execute(context.Background(), scheduler.Job{Kind: SchedulerJobKind, Class: scheduler.ClassNetworkPoll, Payload: payload, Attempt: 1})
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(result.Payload, &generic); err != nil {
		t.Fatal(err)
	}
	observations := generic["observations"].([]any)
	value := observations[0].(map[string]any)["value"].(map[string]any)
	if value["counter64"] != "18446744073709551615" {
		t.Fatalf("counter64 = %#v", value["counter64"])
	}
}

func TestIncorrectSchedulerRoutingIsPermanent(t *testing.T) {
	s := newFakeSession()
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	executor, _ := NewExecutor(collector, 1024)
	for _, job := range []scheduler.Job{
		{Kind: SchedulerJobKind, Class: scheduler.ClassInfra},
		{Kind: scheduler.JobKind("OTHER"), Class: scheduler.ClassNetworkPoll},
	} {
		_, err := executor.Execute(context.Background(), job)
		if !scheduler.IsCode(err, scheduler.CodePermanent) {
			t.Fatalf("routing error = %v, want scheduler permanent", err)
		}
	}
}

func TestTransientSNMPErrorMapsToSchedulerTransient(t *testing.T) {
	err := mapSchedulerError(newError(CodeTimeout, "timeout", timeoutNetError{}, true))
	if !scheduler.IsCode(err, scheduler.CodeTransient) {
		t.Fatalf("mapped error = %v, want scheduler transient", err)
	}
}

func TestPermanentSNMPErrorMapsToSchedulerPermanent(t *testing.T) {
	err := mapSchedulerError(newError(CodeAuthenticationFailed, "auth failed", gosnmp.ErrWrongDigest, false))
	if !scheduler.IsCode(err, scheduler.CodePermanent) {
		t.Fatalf("mapped error = %v, want scheduler permanent", err)
	}
}

func TestSchedulerContextCancellationPropagates(t *testing.T) {
	s := newFakeSession()
	s.blockGetUntilClose = true
	s.getStarted = make(chan struct{})
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	executor, _ := NewExecutor(collector, 1<<20)
	req := baseV2Request()
	payload, _ := EncodeSchedulerPayload(req)
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	go func() {
		_, err := executor.Execute(ctx, scheduler.Job{Kind: SchedulerJobKind, Class: scheduler.ClassNetworkPoll, Payload: payload, Attempt: 1})
		resultCh <- err
	}()
	select {
	case <-s.getStarted:
	case <-time.After(time.Second):
		t.Fatal("Get did not start")
	}
	cancel()
	select {
	case err := <-resultCh:
		if err != context.Canceled {
			t.Fatalf("Execute() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Execute did not return after cancellation")
	}
}

func FuzzSchedulerPayloadDecode(f *testing.F) {
	seed, _ := EncodeSchedulerPayload(baseV2Request())
	f.Add(seed)
	f.Add([]byte(`{"schemaVersion":1}`))
	f.Fuzz(func(t *testing.T, payload []byte) {
		_, _ = DecodeSchedulerPayload(payload, 4096)
	})
}

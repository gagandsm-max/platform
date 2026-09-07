package snmp

import (
	"context"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestOperationDispatch(t *testing.T) {
	cases := []struct {
		operation Operation
		prepare   func(*fakeSession)
		called    func(*fakeSession) int64
	}{
		{OperationGet, func(s *fakeSession) { s.getPacket = valuePacket(".1.3.6.1.2.1.1.3.0", gosnmp.Integer, int(1)) }, func(s *fakeSession) int64 { return s.getCount.Load() }},
		{OperationGetNext, func(s *fakeSession) { s.nextPacket = valuePacket(".1.3.6.1.2.1.1.4.0", gosnmp.Integer, int(1)) }, func(s *fakeSession) int64 { return s.nextCount.Load() }},
		{OperationGetBulk, func(s *fakeSession) { s.bulkPacket = valuePacket(".1.3.6.1.2.1.1.4.0", gosnmp.Integer, int(1)) }, func(s *fakeSession) int64 { return s.bulkCount.Load() }},
		{OperationWalk, func(s *fakeSession) {
			s.walkPDUs = []gosnmp.SnmpPDU{{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.Integer, Value: int(1)}}
		}, func(s *fakeSession) int64 { return s.walkCount.Load() }},
		{OperationBulkWalk, func(s *fakeSession) {
			s.bulkPDUs = []gosnmp.SnmpPDU{{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.Integer, Value: int(1)}}
		}, func(s *fakeSession) int64 { return s.bulkWalkCount.Load() }},
	}

	for _, tc := range cases {
		t.Run(string(tc.operation), func(t *testing.T) {
			s := newFakeSession()
			tc.prepare(s)
			collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
			req := baseV2Request()
			req.Operation = tc.operation
			if tc.operation == OperationGetBulk || tc.operation == OperationBulkWalk {
				req.Version = Version2c
			}
			if tc.operation == OperationWalk || tc.operation == OperationBulkWalk {
				req.OIDs = []string{".1.3.6.1.2.1.1"}
			}
			if _, err := collector.Collect(context.Background(), req); err != nil {
				t.Fatalf("Collect() error = %v", err)
			}
			if got := tc.called(s); got != 1 {
				t.Fatalf("operation call count = %d, want 1", got)
			}
		})
	}
}

func TestWalkResultCountBound(t *testing.T) {
	s := newFakeSession()
	for i := 1; i <= 3; i++ {
		s.walkPDUs = append(s.walkPDUs, gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.2.2.1.2." + string(rune('0'+i)), Type: gosnmp.OctetString, Value: []byte("eth")})
	}
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	req := baseV2Request()
	req.Operation = OperationWalk
	req.OIDs = []string{".1.3.6.1.2.1.2.2.1.2"}
	req.Limits.MaxWalkEntries = 2
	_, err := collector.Collect(context.Background(), req)
	if !IsCode(err, CodeResourceLimit) {
		t.Fatalf("Collect() error = %v, want %s", err, CodeResourceLimit)
	}
}

func TestResultByteBound(t *testing.T) {
	s := newFakeSession()
	s.getPacket = valuePacket(".1.3.6.1.2.1.1.4.0", gosnmp.OctetString, []byte(strings.Repeat("x", 512)))
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	req := baseV2Request()
	req.OIDs = []string{".1.3.6.1.2.1.1.4.0"}
	req.Limits.MaxResultBytes = 128
	_, err := collector.Collect(context.Background(), req)
	if !IsCode(err, CodeResourceLimit) {
		t.Fatalf("Collect() error = %v, want %s", err, CodeResourceLimit)
	}
}

func TestGetBulkDeterministicSourceOIDMapping(t *testing.T) {
	oidA := ".1.3.6.1.2.1.2.2.1.10"
	oidB := ".1.3.6.1.2.1.2.2.1.16"
	s := newFakeSession()
	s.bulkPacket = &gosnmp.SnmpPacket{Error: gosnmp.NoError, Variables: []gosnmp.SnmpPDU{
		{Name: oidA + ".1", Type: gosnmp.Counter32, Value: uint32(1)},
		{Name: oidB + ".1", Type: gosnmp.Counter32, Value: uint32(2)},
		{Name: oidA + ".2", Type: gosnmp.Counter32, Value: uint32(3)},
		{Name: oidB + ".2", Type: gosnmp.Counter32, Value: uint32(4)},
	}}
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	req := baseV2Request()
	req.Operation = OperationGetBulk
	req.OIDs = []string{oidA, oidB}
	result, err := collector.Collect(context.Background(), req)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	want := []string{oidA, oidB, oidA, oidB}
	for i, obs := range result.Observations {
		if obs.SourceOID != want[i] {
			t.Fatalf("observation %d source = %s, want %s", i, obs.SourceOID, want[i])
		}
	}
}

func TestAuthorizationResponseClassification(t *testing.T) {
	s := newFakeSession()
	s.getPacket = &gosnmp.SnmpPacket{Error: gosnmp.AuthorizationError, Variables: []gosnmp.SnmpPDU{{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.Null}}}
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	_, err := collector.Collect(context.Background(), baseV2Request())
	if !IsCode(err, CodeAuthorizationFailed) {
		t.Fatalf("Collect() error = %v, want %s", err, CodeAuthorizationFailed)
	}
}

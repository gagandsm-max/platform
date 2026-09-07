package observation

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestEventEnvelopeStrictRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 7, 1, 2, 3, 456000000, time.UTC)
	payload := json.RawMessage(`{"elements":[{"id":"id1","params":[{"name":"a","value":"b"}]},{"id":"id2","params":[{"name":"c","value":"d"}]}]}`)
	e := EventEnvelope{SchemaVersion: EventEnvelopeSchemaVersion, ReceivedAt: now, EventTime: &now, Source: EventSource{Transport: "UDP"}, Protocol: "SYSLOG", Format: "RFC5424", Quality: "GOOD", PayloadSchemaVersion: 1, Payload: payload}
	encoded, err := EncodeEventEnvelope(e, 4096, 2048)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := DecodeEventEnvelope(encoded, 4096, 2048)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(got.Payload, payload) {
		t.Fatalf("payload changed: %s", got.Payload)
	}
}

func TestEventEnvelopeSchemaVersionValidated(t *testing.T) {
	e := EventEnvelope{SchemaVersion: 99, ReceivedAt: time.Now().UTC(), Protocol: "SYSLOG", Format: "RFC5424", Quality: "GOOD", PayloadSchemaVersion: 1, Payload: json.RawMessage(`{}`)}
	if _, err := EncodeEventEnvelope(e, 4096, 2048); err == nil {
		t.Fatal("expected schema error")
	}
}

func TestEventEnvelopeUnknownFieldRejected(t *testing.T) {
	payload := []byte(`{"schemaVersion":1,"receivedAt":"2026-09-07T00:00:00Z","source":{},"protocol":"SYSLOG","format":"RFC5424","quality":"GOOD","payloadSchemaVersion":1,"payload":{},"unexpected":true}`)
	if _, err := DecodeEventEnvelope(payload, 4096, 2048); err == nil {
		t.Fatal("expected unknown-field rejection")
	}
}

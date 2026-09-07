package coreclient

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestUploadRejectsUnknownDurableAckFieldsFailClosed(t *testing.T) {
	pki := makeTestPKI(t)
	accepted := uint64(2)
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                    "ACCEPTED",
			"accepted_through_sequence": accepted,
			"spool_retention_required":  true,
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	ack, err := client.Upload(context.Background(), validBatch())
	if !IsCode(err, CodeProtocol) {
		t.Fatalf("expected fail-closed protocol error, got ack=%+v err=%v", ack, err)
	}
	if ack.AcceptedThroughSequence != nil {
		t.Fatalf("unsupported acknowledgement semantics exposed deletion watermark: %+v", ack)
	}
}

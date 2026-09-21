package coreclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchRecurringScheduleDesiredStateValidation(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != recurringScheduleDesiredStatePath {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(RecurringScheduleDesiredState{
			Generation:        1,
			Checksum:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			AppliedGeneration: 0,
			Definitions:       []RecurringScheduleDefinition{},
		})
	}))
	defer srv.Close()
	client := testClient(t, srv)
	out, err := client.FetchRecurringScheduleDesiredState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Generation != 1 {
		t.Fatalf("generation=%d", out.Generation)
	}
}

func TestAckRecurringScheduleConflictCodes(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != recurringScheduleAppliedStatePath {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": string(CodeRecurringStateStale)})
	}))
	defer srv.Close()
	client := testClient(t, srv)
	_, err := client.AckRecurringScheduleAppliedState(context.Background(), 2, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil || !IsCode(err, CodeRecurringStateStale) {
		t.Fatalf("expected stale code, got %v", err)
	}
}

func TestAckRecurringScheduleChecksumMismatch(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": string(CodeRecurringChecksumMismatch)})
	}))
	defer srv.Close()
	client := testClient(t, srv)
	_, err := client.AckRecurringScheduleAppliedState(context.Background(), 2, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err == nil || !IsCode(err, CodeRecurringChecksumMismatch) {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

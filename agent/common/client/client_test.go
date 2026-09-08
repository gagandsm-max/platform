package client

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUnavailableTransport(t *testing.T) {
	tr := NewUnavailableTransport("")
	if tr.Available() {
		t.Fatal("unexpected availability")
	}
	_, err := tr.Deliver(context.Background(), Batch{})
	if err == nil || Classify(err) != FailureReceiverUnavailable {
		t.Fatalf("bad classification %v", err)
	}
}
func TestBackoffBounded(t *testing.T) {
	base := time.Second
	max := 10 * time.Second
	if got := Backoff(base, max, 0, 0); got != base {
		t.Fatalf("got %v", got)
	}
	if got := Backoff(base, max, 20, 1); got > max {
		t.Fatalf("exceeded max %v", got)
	}
	if got := Backoff(base, max, 2, -1); got <= 0 {
		t.Fatalf("bad jitter %v", got)
	}
}
func TestClassify(t *testing.T) {
	cases := []struct {
		err  error
		want FailureClass
	}{{&DeliveryError{Code: "AUTH", Class: FailureAuthentication}, FailureAuthentication}, {context.DeadlineExceeded, FailureTemporary}, {errors.New("network"), FailureTemporary}}
	for _, tc := range cases {
		if got := Classify(tc.err); got != tc.want {
			t.Fatalf("got %s want %s", got, tc.want)
		}
	}
}
func TestCorrelationID(t *testing.T) {
	a, err := CorrelationID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := CorrelationID()
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a == b {
		t.Fatalf("bad ids %q %q", a, b)
	}
}

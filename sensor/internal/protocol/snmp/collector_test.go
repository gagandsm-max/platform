package snmp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
)

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "synthetic timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

var _ net.Error = timeoutNetError{}

func TestTransportTimeoutClassification(t *testing.T) {
	s := newFakeSession()
	s.getErr = timeoutNetError{}
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	_, err := collector.Collect(context.Background(), baseV2Request())
	if !IsCode(err, CodeTimeout) || !IsTransient(err) {
		t.Fatalf("Collect() error = %v, want transient %s", err, CodeTimeout)
	}
	if got := collector.Stats().ActiveCancellationWatchers; got != 0 {
		t.Fatalf("active cancellation watchers = %d, want 0", got)
	}
}

func TestAuthenticationFailureClassification(t *testing.T) {
	s := newFakeSession()
	s.getErr = gosnmp.ErrWrongDigest
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	_, err := collector.Collect(context.Background(), baseV2Request())
	if !IsCode(err, CodeAuthenticationFailed) || IsTransient(err) {
		t.Fatalf("Collect() error = %v, want permanent %s", err, CodeAuthenticationFailed)
	}
}

func TestPermanentAuthenticationFailureDoesNotRetry(t *testing.T) {
	var factories int
	collector := testCollector(func(context.Context, Request) (session, error) {
		factories++
		s := newFakeSession()
		s.getErr = gosnmp.ErrUnknownUsername
		return s, nil
	})
	req := baseV3Request(SecurityAuthNoPriv)
	req.Retry.MaxAttempts = 5
	req.Retry.InitialBackoff = time.Millisecond
	req.Retry.MaxBackoff = time.Millisecond
	_, err := collector.Collect(context.Background(), req)
	if !IsCode(err, CodeAuthenticationFailed) {
		t.Fatalf("Collect() error = %v", err)
	}
	if factories != 1 {
		t.Fatalf("session factories = %d, want 1 for permanent authentication failure", factories)
	}
}

func TestTransientRetryUsesFreshSession(t *testing.T) {
	var mu sync.Mutex
	var sessions []*fakeSession
	collector := testCollector(func(context.Context, Request) (session, error) {
		mu.Lock()
		defer mu.Unlock()
		s := newFakeSession()
		if len(sessions) == 0 {
			s.getErr = timeoutNetError{}
		} else {
			s.getPacket = valuePacket(".1.3.6.1.2.1.1.3.0", gosnmp.TimeTicks, uint32(7))
		}
		sessions = append(sessions, s)
		return s, nil
	})
	req := baseV2Request()
	req.Retry.MaxAttempts = 2
	req.Retry.InitialBackoff = time.Millisecond
	req.Retry.MaxBackoff = time.Millisecond
	result, err := collector.Collect(context.Background(), req)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(result.Observations) != 1 || len(sessions) != 2 || sessions[0] == sessions[1] {
		t.Fatalf("fresh-session retry invariant failed: sessions=%d result=%+v", len(sessions), result)
	}
	for i, s := range sessions {
		if got := s.closeCount.Load(); got != 1 {
			t.Fatalf("session %d close count = %d, want 1", i, got)
		}
	}
}

func TestContextCancellationClosesTransportAndTerminatesWatcher(t *testing.T) {
	s := newFakeSession()
	s.blockGetUntilClose = true
	s.getStarted = make(chan struct{})
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	go func() {
		_, err := collector.Collect(ctx, baseV2Request())
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
		if !IsCode(err, CodeCancelled) {
			t.Fatalf("Collect() error = %v, want %s", err, CodeCancelled)
		}
	case <-time.After(time.Second):
		t.Fatal("Collect did not return after cancellation")
	}
	if got := collector.Stats().ActiveCancellationWatchers; got != 0 {
		t.Fatalf("active cancellation watchers = %d, want 0", got)
	}
	if got := s.closeCount.Load(); got != 1 {
		t.Fatalf("close count = %d, want 1", got)
	}
}

func TestDeadlineExpirationTerminatesWatcher(t *testing.T) {
	s := newFakeSession()
	s.blockGetUntilClose = true
	s.getStarted = make(chan struct{})
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	req := baseV2Request()
	req.ExecutionTimeout = 20 * time.Millisecond
	_, err := collector.Collect(context.Background(), req)
	if !IsCode(err, CodeDeadlineExceeded) {
		t.Fatalf("Collect() error = %v, want %s", err, CodeDeadlineExceeded)
	}
	if got := collector.Stats().ActiveCancellationWatchers; got != 0 {
		t.Fatalf("active cancellation watchers = %d, want 0", got)
	}
}

func TestConnectionCloseOnCompletion(t *testing.T) {
	s := newFakeSession()
	s.getPacket = valuePacket(".1.3.6.1.2.1.1.3.0", gosnmp.TimeTicks, uint32(1))
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	if _, err := collector.Collect(context.Background(), baseV2Request()); err != nil {
		t.Fatal(err)
	}
	if got := s.closeCount.Load(); got != 1 {
		t.Fatalf("close count = %d, want 1", got)
	}
	if got := collector.Stats().ActiveCancellationWatchers; got != 0 {
		t.Fatalf("active cancellation watchers = %d, want 0", got)
	}
}

func TestConnectionCloseAfterOperationError(t *testing.T) {
	s := newFakeSession()
	s.getErr = errors.New("synthetic transport failure")
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	_, _ = collector.Collect(context.Background(), baseV2Request())
	if got := s.closeCount.Load(); got != 1 {
		t.Fatalf("close count = %d, want 1", got)
	}
	if got := collector.Stats().ActiveCancellationWatchers; got != 0 {
		t.Fatalf("active cancellation watchers = %d, want 0", got)
	}
}

func TestConnectionCloseAfterConnectError(t *testing.T) {
	s := newFakeSession()
	s.connectErr = errors.New("synthetic connect failure")
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	_, _ = collector.Collect(context.Background(), baseV2Request())
	if got := s.closeCount.Load(); got != 1 {
		t.Fatalf("close count = %d, want 1", got)
	}
	if got := collector.Stats().ActiveCancellationWatchers; got != 0 {
		t.Fatalf("active cancellation watchers = %d, want 0", got)
	}
}

func TestNoSecretAppearsInDiagnosticsErrorsOrLogger(t *testing.T) {
	credential := Credentials{V3: &V3Credentials{
		Username:        "unit-user",
		SecurityLevel:   SecurityAuthPriv,
		AuthProtocol:    AuthSHA256,
		AuthSecret:      Secret(testAuth),
		PrivacyProtocol: PrivacyAES,
		PrivacySecret:   Secret(testPrivacy),
		ContextName:     "unit-context",
	}}
	for _, rendered := range []string{fmt.Sprintf("%v", credential), fmt.Sprintf("%+v", credential), fmt.Sprintf("%#v", credential)} {
		if strings.Contains(rendered, testAuth) || strings.Contains(rendered, testPrivacy) || strings.Contains(rendered, testCommunity) {
			t.Fatalf("credential diagnostic leaked secret: %s", rendered)
		}
	}

	upstreamErr := fmt.Errorf("upstream carried %s and %s", testAuth, testPrivacy)
	typed := newError(CodeAuthenticationFailed, "authentication failed", upstreamErr, false)
	if strings.Contains(typed.Error(), testAuth) || strings.Contains(typed.Error(), testPrivacy) {
		t.Fatalf("typed error leaked wrapped cause: %s", typed.Error())
	}

	logger := &captureLogger{}
	s := newFakeSession()
	s.getErr = gosnmp.ErrWrongDigest
	collector := testCollector(func(context.Context, Request) (session, error) { return s, nil })
	collector.logger = logger
	req := baseV3Request(SecurityAuthPriv)
	_, _ = collector.Collect(context.Background(), req)
	captured := logger.String()
	for _, secret := range []string{testCommunity, testAuth, testPrivacy} {
		if strings.Contains(captured, secret) {
			t.Fatalf("logger leaked secret %q in %s", secret, captured)
		}
	}
}

func TestConcurrentIndependentRequestsDoNotShareCredentialsOrSessions(t *testing.T) {
	var mu sync.Mutex
	seen := make([]string, 0, 2)
	sessions := make([]*fakeSession, 0, 2)
	collector := testCollector(func(_ context.Context, req Request) (session, error) {
		s := newFakeSession()
		s.getPacket = valuePacket(".1.3.6.1.2.1.1.3.0", gosnmp.TimeTicks, uint32(1))
		mu.Lock()
		seen = append(seen, req.Credentials.Community.Community.value())
		sessions = append(sessions, s)
		mu.Unlock()
		return s, nil
	})

	reqA := baseV2Request()
	reqA.Credentials.Community.Community = Secret("test-only-community-A")
	reqB := baseV2Request()
	reqB.Credentials = Credentials{Community: &CommunityCredentials{Community: Secret("test-only-community-B")}}

	var wg sync.WaitGroup
	wg.Add(2)
	for _, req := range []Request{reqA, reqB} {
		req := req
		go func() {
			defer wg.Done()
			if _, err := collector.Collect(context.Background(), req); err != nil {
				t.Errorf("Collect() error = %v", err)
			}
		}()
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(sessions) != 2 || sessions[0] == sessions[1] {
		t.Fatalf("sessions not independent: %d", len(sessions))
	}
	joined := strings.Join(seen, ",")
	if !strings.Contains(joined, "test-only-community-A") || !strings.Contains(joined, "test-only-community-B") {
		t.Fatalf("credential isolation capture = %v", seen)
	}
	if got := collector.Stats().ActiveCancellationWatchers; got != 0 {
		t.Fatalf("active cancellation watchers = %d, want 0", got)
	}
}

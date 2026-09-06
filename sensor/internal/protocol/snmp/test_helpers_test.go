package snmp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gosnmp/gosnmp"
)

const (
	testCommunity = "test-only-community-value"
	testAuth      = "test-only-auth-value"
	testPrivacy   = "test-only-privacy-value"
)

func baseV2Request() Request {
	return Request{
		Target:           "127.0.0.1",
		Port:             161,
		Version:          Version2c,
		Credentials:      Credentials{Community: &CommunityCredentials{Community: Secret(testCommunity)}},
		OIDs:             []string{".1.3.6.1.2.1.1.3.0"},
		Operation:        OperationGet,
		Timeout:          100 * time.Millisecond,
		ExecutionTimeout: time.Second,
		Retry: RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: 0,
			MaxBackoff:     0,
			JitterFraction: 0,
		},
		Limits: Limits{
			MaxOIDs:            16,
			MaxWalkEntries:     128,
			MaxBulkRepetitions: 10,
			MaxResults:         128,
			MaxResultBytes:     1 << 20,
		},
		CorrelationID: "unit-correlation",
	}
}

func baseV3Request(level SecurityLevel) Request {
	req := baseV2Request()
	req.Version = Version3
	req.Credentials = Credentials{V3: &V3Credentials{
		Username:        "unit-user",
		SecurityLevel:   level,
		AuthProtocol:    AuthNone,
		PrivacyProtocol: PrivacyNone,
		ContextName:     "unit-context",
	}}
	switch level {
	case SecurityAuthNoPriv:
		req.Credentials.V3.AuthProtocol = AuthSHA256
		req.Credentials.V3.AuthSecret = Secret(testAuth)
	case SecurityAuthPriv:
		req.Credentials.V3.AuthProtocol = AuthSHA256
		req.Credentials.V3.AuthSecret = Secret(testAuth)
		req.Credentials.V3.PrivacyProtocol = PrivacyAES
		req.Credentials.V3.PrivacySecret = Secret(testPrivacy)
	}
	return req
}

type fakeSession struct {
	connectErr  error
	getPacket   *gosnmp.SnmpPacket
	getErr      error
	nextPacket  *gosnmp.SnmpPacket
	nextErr     error
	bulkPacket  *gosnmp.SnmpPacket
	bulkErr     error
	walkPDUs    []gosnmp.SnmpPDU
	walkErr     error
	bulkPDUs    []gosnmp.SnmpPDU
	bulkWalkErr error

	blockGetUntilClose bool
	getStarted         chan struct{}
	getStartedOnce     sync.Once
	closed             chan struct{}
	closeOnce          sync.Once
	closeCount         atomic.Int64
	connectCount       atomic.Int64
	getCount           atomic.Int64
	nextCount          atomic.Int64
	bulkCount          atomic.Int64
	walkCount          atomic.Int64
	bulkWalkCount      atomic.Int64
}

func newFakeSession() *fakeSession {
	return &fakeSession{closed: make(chan struct{})}
}

func (s *fakeSession) Connect() error {
	s.connectCount.Add(1)
	return s.connectErr
}

func (s *fakeSession) Close() error {
	s.closeCount.Add(1)
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func (s *fakeSession) Get(_ []string) (*gosnmp.SnmpPacket, error) {
	s.getCount.Add(1)
	if s.blockGetUntilClose {
		if s.getStarted != nil {
			s.getStartedOnce.Do(func() { close(s.getStarted) })
		}
		<-s.closed
		return nil, errors.New("transport closed")
	}
	return s.getPacket, s.getErr
}

func (s *fakeSession) GetNext(_ []string) (*gosnmp.SnmpPacket, error) {
	s.nextCount.Add(1)
	return s.nextPacket, s.nextErr
}

func (s *fakeSession) GetBulk(_ []string, _ uint8, _ uint32) (*gosnmp.SnmpPacket, error) {
	s.bulkCount.Add(1)
	return s.bulkPacket, s.bulkErr
}

func (s *fakeSession) Walk(_ string, fn gosnmp.WalkFunc) error {
	s.walkCount.Add(1)
	for _, pdu := range s.walkPDUs {
		if err := fn(pdu); err != nil {
			return err
		}
	}
	return s.walkErr
}

func (s *fakeSession) BulkWalk(_ string, fn gosnmp.WalkFunc) error {
	s.bulkWalkCount.Add(1)
	for _, pdu := range s.bulkPDUs {
		if err := fn(pdu); err != nil {
			return err
		}
	}
	return s.bulkWalkErr
}

func valuePacket(oid string, ber gosnmp.Asn1BER, value any) *gosnmp.SnmpPacket {
	return &gosnmp.SnmpPacket{
		Error:     gosnmp.NoError,
		Variables: []gosnmp.SnmpPDU{{Name: oid, Type: ber, Value: value}},
	}
}

func testCollector(factory sessionFactory) *Collector {
	return &Collector{
		factory: factory,
		now:     time.Now,
		wait: func(ctx context.Context, _ time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		},
		jitter: func(d time.Duration, _ float64) time.Duration { return d },
	}
}

type captureLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *captureLogger) Debug(msg string, args ...any) { l.add(msg, args...) }
func (l *captureLogger) Warn(msg string, args ...any)  { l.add(msg, args...) }
func (l *captureLogger) add(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprint(append([]any{msg}, args...)...))
}
func (l *captureLogger) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return fmt.Sprint(l.lines)
}

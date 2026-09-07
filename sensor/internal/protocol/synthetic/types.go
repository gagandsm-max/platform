package synthetic

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"platform/sensor/internal/scheduler"
)

const ResultSchemaVersion = 1

const (
	JobKindTCP  scheduler.JobKind = "SYNTHETIC_TCP"
	JobKindDNS  scheduler.JobKind = "SYNTHETIC_DNS"
	JobKindTLS  scheduler.JobKind = "SYNTHETIC_TLS"
	JobKindHTTP scheduler.JobKind = "SYNTHETIC_HTTP"
)

type Limits struct {
	MaxTimeout               time.Duration
	MaxSchedulerPayloadBytes int
	MaxResultBytes           int
	MaxTargetBytes           int
	MaxURLBytes              int
	MaxDNSAnswers            int
	MaxRedirects             int
	MaxResponseBytes         int
	MaxExpectedStatuses      int
	MaxExpectedContentBytes  int
	MaxHeaders               int
	MaxHeaderNameBytes       int
	MaxHeaderValueBytes      int
	MaxCABundleBytes         int
	MaxCertificateChain      int
	MaxSANs                  int
	MaxDiagnosticTextBytes   int
}

type TCPRequest struct {
	Target  string
	Port    int
	Timeout time.Duration
}

type TCPResult struct {
	SchemaVersion  int       `json:"schemaVersion"`
	Target         string    `json:"target"`
	Port           int       `json:"port"`
	StartedAt      time.Time `json:"startedAt"`
	CompletedAt    time.Time `json:"completedAt"`
	DurationMicros int64     `json:"durationMicros"`
	Success        bool      `json:"success"`
	ErrorCode      ErrorCode `json:"errorCode,omitempty"`
}

type DNSRecordType string

const (
	DNSRecordA    DNSRecordType = "A"
	DNSRecordAAAA DNSRecordType = "AAAA"
)

type DNSStatus string

const (
	DNSStatusOK       DNSStatus = "OK"
	DNSStatusNotFound DNSStatus = "NOT_FOUND"
	DNSStatusFailed   DNSStatus = "FAILED"
)

const DNSRCodeUnavailable = "UNAVAILABLE"

type DNSRequest struct {
	Hostname        string
	RecordType      DNSRecordType
	ResolverAddress string
	Timeout         time.Duration
	MaxAnswers      int
}

type DNSResult struct {
	SchemaVersion  int           `json:"schemaVersion"`
	Hostname       string        `json:"hostname"`
	RecordType     DNSRecordType `json:"recordType"`
	Resolver       string        `json:"resolver,omitempty"`
	StartedAt      time.Time     `json:"startedAt"`
	CompletedAt    time.Time     `json:"completedAt"`
	DurationMicros int64         `json:"durationMicros"`
	Status         DNSStatus     `json:"status"`
	RCode          string        `json:"rcode"`
	Answers        []string      `json:"answers"`
	ErrorCode      ErrorCode     `json:"errorCode,omitempty"`
}

type TLSRequest struct {
	Target      string
	Port        int
	ServerName  string
	Timeout     time.Duration
	CABundlePEM []byte
}

func (r TLSRequest) String() string {
	return fmt.Sprintf("TLSRequest{target=%s,port=%d,serverName=%s,timeout=%s,caBundle=[REDACTED:%d bytes]}", safeHost(r.Target), r.Port, safeHost(r.ServerName), r.Timeout, len(r.CABundlePEM))
}
func (r TLSRequest) GoString() string { return r.String() }

type CertificateSummary struct {
	Subject                  string    `json:"subject"`
	Issuer                   string    `json:"issuer"`
	NotBefore                time.Time `json:"notBefore"`
	NotAfter                 time.Time `json:"notAfter"`
	RemainingLifetimeSeconds int64     `json:"remainingLifetimeSeconds"`
	DNSNames                 []string  `json:"dnsNames,omitempty"`
	IPAddresses              []string  `json:"ipAddresses,omitempty"`
	OtherSANCount            int       `json:"otherSanCount,omitempty"`
}

type TLSResult struct {
	SchemaVersion     int                  `json:"schemaVersion"`
	Target            string               `json:"target"`
	Port              int                  `json:"port"`
	ServerName        string               `json:"serverName"`
	StartedAt         time.Time            `json:"startedAt"`
	CompletedAt       time.Time            `json:"completedAt"`
	DurationMicros    int64                `json:"durationMicros"`
	NegotiatedVersion string               `json:"negotiatedVersion"`
	CipherSuite       string               `json:"cipherSuite"`
	PeerCertificates  []CertificateSummary `json:"peerCertificates"`
	Success           bool                 `json:"success"`
	ErrorCode         ErrorCode            `json:"errorCode,omitempty"`
}

type HTTPMethod string

const (
	HTTPMethodGET  HTTPMethod = "GET"
	HTTPMethodHEAD HTTPMethod = "HEAD"
)

type HTTPRequest struct {
	URL                 string
	Method              HTTPMethod
	Headers             map[string]string
	Timeout             time.Duration
	FollowRedirects     bool
	MaxRedirects        int
	MaxResponseBytes    int
	ExpectedStatusCodes []int
	ExpectedContent     string
	CABundlePEM         []byte
}

func (r HTTPRequest) String() string {
	return fmt.Sprintf("HTTPRequest{method=%s,url=%s,headers=[REDACTED:%d],timeout=%s,followRedirects=%t,maxRedirects=%d,maxResponseBytes=%d,expectedStatuses=%d,expectedContent=[REDACTED:%d bytes],caBundle=[REDACTED:%d bytes]}",
		r.Method, safeURL(r.URL), len(r.Headers), r.Timeout, r.FollowRedirects, r.MaxRedirects, r.MaxResponseBytes, len(r.ExpectedStatusCodes), len(r.ExpectedContent), len(r.CABundlePEM))
}
func (r HTTPRequest) GoString() string { return r.String() }

type PhaseTimings struct {
	DNSMicros          *int64 `json:"dnsMicros,omitempty"`
	ConnectMicros      *int64 `json:"connectMicros,omitempty"`
	TLSHandshakeMicros *int64 `json:"tlsHandshakeMicros,omitempty"`
	TTFBMicros         *int64 `json:"ttfbMicros,omitempty"`
	TotalMicros        int64  `json:"totalMicros"`
}

type HTTPResult struct {
	SchemaVersion  int          `json:"schemaVersion"`
	Method         HTTPMethod   `json:"method"`
	URL            string       `json:"url"`
	FinalURL       string       `json:"finalUrl"`
	StartedAt      time.Time    `json:"startedAt"`
	CompletedAt    time.Time    `json:"completedAt"`
	StatusCode     int          `json:"statusCode"`
	ResponseBytes  int          `json:"responseBytes"`
	Redirects      int          `json:"redirects"`
	ContentMatched *bool        `json:"contentMatched,omitempty"`
	Timings        PhaseTimings `json:"timings"`
	Success        bool         `json:"success"`
	ErrorCode      ErrorCode    `json:"errorCode,omitempty"`
}

type MetricsSnapshot struct {
	Executed           uint64 `json:"executed"`
	Successful         uint64 `json:"successful"`
	Failed             uint64 `json:"failed"`
	Timeout            uint64 `json:"timeout"`
	Cancelled          uint64 `json:"cancelled"`
	TLSFailures        uint64 `json:"tlsFailures"`
	HTTPStatusMismatch uint64 `json:"httpStatusMismatch"`
	ContentMismatch    uint64 `json:"contentMismatch"`
	ResourceLimit      uint64 `json:"resourceLimit"`
}

type metrics struct {
	executed, successful, failed, timeout, cancelled, tlsFailures, httpStatusMismatch, contentMismatch, resourceLimit atomic.Uint64
}

func (m *metrics) snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		Executed: m.executed.Load(), Successful: m.successful.Load(), Failed: m.failed.Load(), Timeout: m.timeout.Load(), Cancelled: m.cancelled.Load(),
		TLSFailures: m.tlsFailures.Load(), HTTPStatusMismatch: m.httpStatusMismatch.Load(), ContentMismatch: m.contentMismatch.Load(), ResourceLimit: m.resourceLimit.Load(),
	}
}

func safeHost(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 128 {
		return s[:128] + "…"
	}
	return s
}

func safeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[INVALID_URL]"
	}
	host := u.Hostname()
	if p := u.Port(); p != "" {
		host = host + ":" + p
	}
	if host == "" {
		return "[INVALID_URL]"
	}
	return u.Scheme + "://" + host
}

func sortedHeaderKeys(h map[string]string) []string {
	out := make([]string, 0, len(h))
	for k := range h {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

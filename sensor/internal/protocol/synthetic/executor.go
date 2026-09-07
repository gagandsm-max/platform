package synthetic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"platform/sensor/internal/scheduler"
)

var _ scheduler.Executor = (*Executor)(nil)

type Executor struct{ collector *Collector }

func NewExecutor(c *Collector) *Executor { return &Executor{collector: c} }

type tcpJobRequest struct {
	SchemaVersion int    `json:"schemaVersion"`
	Target        string `json:"target"`
	Port          int    `json:"port"`
	TimeoutMillis int64  `json:"timeoutMillis"`
}
type dnsJobRequest struct {
	SchemaVersion   int           `json:"schemaVersion"`
	Hostname        string        `json:"hostname"`
	RecordType      DNSRecordType `json:"recordType"`
	ResolverAddress string        `json:"resolverAddress,omitempty"`
	TimeoutMillis   int64         `json:"timeoutMillis"`
	MaxAnswers      int           `json:"maxAnswers"`
}
type tlsJobRequest struct {
	SchemaVersion int    `json:"schemaVersion"`
	Target        string `json:"target"`
	Port          int    `json:"port"`
	ServerName    string `json:"serverName,omitempty"`
	TimeoutMillis int64  `json:"timeoutMillis"`
	CABundlePEM   string `json:"caBundlePem,omitempty"`
}
type httpJobRequest struct {
	SchemaVersion       int               `json:"schemaVersion"`
	URL                 string            `json:"url"`
	Method              HTTPMethod        `json:"method"`
	Headers             map[string]string `json:"headers,omitempty"`
	TimeoutMillis       int64             `json:"timeoutMillis"`
	FollowRedirects     bool              `json:"followRedirects"`
	MaxRedirects        int               `json:"maxRedirects"`
	MaxResponseBytes    int               `json:"maxResponseBytes"`
	ExpectedStatusCodes []int             `json:"expectedStatusCodes,omitempty"`
	ExpectedContent     string            `json:"expectedContent,omitempty"`
	CABundlePEM         string            `json:"caBundlePem,omitempty"`
}

func (e *Executor) Execute(ctx context.Context, j scheduler.Job) (scheduler.ExecutionResult, error) {
	if e == nil || e.collector == nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "synthetic executor is not initialized", nil, false))
	}
	if j.Class != scheduler.ClassSynthetic {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "synthetic job must use SYNTHETIC execution class", nil, false))
	}
	if len(j.Payload) == 0 {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "synthetic scheduler payload is required", nil, false))
	}
	if len(j.Payload) > e.collector.limits.MaxSchedulerPayloadBytes {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeResourceLimit, "synthetic scheduler payload exceeds configured bound", nil, false))
	}
	var payload any
	switch j.Kind {
	case JobKindTCP:
		var p tcpJobRequest
		if err := decodeStrict(j.Payload, &p); err != nil {
			return scheduler.ExecutionResult{}, scheduler.Permanent(err)
		}
		if p.SchemaVersion != 1 {
			return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "unsupported synthetic TCP payload schema", nil, false))
		}
		d, err := finiteMillis(p.TimeoutMillis)
		if err != nil {
			return scheduler.ExecutionResult{}, scheduler.Permanent(err)
		}
		payload, err = e.collector.TCP(ctx, TCPRequest{Target: p.Target, Port: p.Port, Timeout: d})
		if err != nil {
			return scheduler.ExecutionResult{}, mapScheduler(err)
		}
	case JobKindDNS:
		var p dnsJobRequest
		if err := decodeStrict(j.Payload, &p); err != nil {
			return scheduler.ExecutionResult{}, scheduler.Permanent(err)
		}
		if p.SchemaVersion != 1 {
			return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "unsupported synthetic DNS payload schema", nil, false))
		}
		d, err := finiteMillis(p.TimeoutMillis)
		if err != nil {
			return scheduler.ExecutionResult{}, scheduler.Permanent(err)
		}
		payload, err = e.collector.DNS(ctx, DNSRequest{Hostname: p.Hostname, RecordType: p.RecordType, ResolverAddress: p.ResolverAddress, Timeout: d, MaxAnswers: p.MaxAnswers})
		if err != nil {
			return scheduler.ExecutionResult{}, mapScheduler(err)
		}
	case JobKindTLS:
		var p tlsJobRequest
		if err := decodeStrict(j.Payload, &p); err != nil {
			return scheduler.ExecutionResult{}, scheduler.Permanent(err)
		}
		if p.SchemaVersion != 1 {
			return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "unsupported synthetic TLS payload schema", nil, false))
		}
		d, err := finiteMillis(p.TimeoutMillis)
		if err != nil {
			return scheduler.ExecutionResult{}, scheduler.Permanent(err)
		}
		payload, err = e.collector.TLS(ctx, TLSRequest{Target: p.Target, Port: p.Port, ServerName: p.ServerName, Timeout: d, CABundlePEM: []byte(p.CABundlePEM)})
		if err != nil {
			return scheduler.ExecutionResult{}, mapScheduler(err)
		}
	case JobKindHTTP:
		var p httpJobRequest
		if err := decodeStrict(j.Payload, &p); err != nil {
			return scheduler.ExecutionResult{}, scheduler.Permanent(err)
		}
		if p.SchemaVersion != 1 {
			return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "unsupported synthetic HTTP payload schema", nil, false))
		}
		d, err := finiteMillis(p.TimeoutMillis)
		if err != nil {
			return scheduler.ExecutionResult{}, scheduler.Permanent(err)
		}
		payload, err = e.collector.HTTP(ctx, HTTPRequest{URL: p.URL, Method: p.Method, Headers: p.Headers, Timeout: d, FollowRedirects: p.FollowRedirects, MaxRedirects: p.MaxRedirects, MaxResponseBytes: p.MaxResponseBytes, ExpectedStatusCodes: p.ExpectedStatusCodes, ExpectedContent: p.ExpectedContent, CABundlePEM: []byte(p.CABundlePEM)})
		if err != nil {
			return scheduler.ExecutionResult{}, mapScheduler(err)
		}
	default:
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "unsupported synthetic job kind", nil, false))
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeTransportFailure, "failed to encode synthetic result", nil, false))
	}
	if len(encoded) > e.collector.limits.MaxResultBytes {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeResourceLimit, "synthetic result exceeds configured bound", nil, false))
	}
	return scheduler.ExecutionResult{Payload: encoded}, nil
}

func decodeStrict(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return newError(CodeInvalidRequest, "invalid synthetic scheduler payload", nil, false)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return newError(CodeInvalidRequest, "synthetic scheduler payload contains trailing data", nil, false)
	}
	return nil
}

func mapScheduler(err error) error {
	switch CodeOf(err) {
	case CodeCancelled, CodeDeadlineExceeded:
		return err
	}
	if IsTransient(err) {
		return scheduler.Transient(err)
	}
	return scheduler.Permanent(err)
}

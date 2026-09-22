package identity

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type HTTPEnrollmentTransport struct {
	endpoint         *url.URL
	client           *http.Client
	maxResponseBytes int
}

type enrollmentWireRequest struct {
	Token  string `json:"token"`
	Role   string `json:"role"`
	SiteID string `json:"siteId,omitempty"`
	PoolID string `json:"poolId,omitempty"`
	CSRPem string `json:"csrPem"`
}

// enrollmentWireResponse deliberately omits privateKeyPem. DisallowUnknownFields
// rejects legacy Core responses that still return private-key material.
type enrollmentWireResponse struct {
	SensorID       string `json:"sensorId"`
	CertificatePEM string `json:"certificatePem"`
	CABundlePEM    string `json:"caBundlePem"`
}

// NewHTTPEnrollmentTransport requires the complete contract-supplied URL.
// Platform_17 does not freeze an enrollment path, so Sensor code does not
// invent or hard-code one.
func NewHTTPEnrollmentTransport(endpointURL, caFile string, timeout time.Duration, maxResponseBytes int) (*HTTPEnrollmentTransport, error) {
	u, err := url.Parse(strings.TrimSpace(endpointURL))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("SENSOR_ENROLLMENT_ENDPOINT_INVALID: endpoint must be an explicit HTTPS URL")
	}
	if timeout <= 0 || maxResponseBytes <= 0 {
		return nil, fmt.Errorf("SENSOR_ENROLLMENT_CONFIG_INVALID: timeout and response bound must be positive")
	}
	if caFile == "" {
		return nil, fmt.Errorf("SENSOR_ENROLLMENT_CA_REQUIRED: Core CA file is required")
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("SENSOR_ENROLLMENT_CA_INVALID: unable to read Core CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("SENSOR_ENROLLMENT_CA_INVALID: Core CA contains no usable certificate")
	}
	transport := &http.Transport{
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		ForceAttemptHTTP2: true,
	}
	return &HTTPEnrollmentTransport{
		endpoint:         u,
		client:           &http.Client{Transport: transport, Timeout: timeout},
		maxResponseBytes: maxResponseBytes,
	}, nil
}

func (t *HTTPEnrollmentTransport) Enroll(ctx context.Context, token []byte, request EnrollmentRequest) (EnrollmentResponse, error) {
	if t == nil || t.client == nil {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_TRANSPORT_INVALID: transport is not initialized")
	}
	if ctx == nil {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_REQUEST_INVALID: context is required")
	}
	if len(token) == 0 || strings.TrimSpace(request.Role) == "" {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_REQUEST_INVALID: token and role are required")
	}
	if strings.TrimSpace(request.CSRPem) == "" {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_REQUEST_INVALID: csrPem is required")
	}

	wire := enrollmentWireRequest{
		Token:  string(token),
		Role:   request.Role,
		SiteID: request.SiteID,
		PoolID: request.PoolID,
		CSRPem: request.CSRPem,
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_REQUEST_INVALID: request cannot be encoded")
	}
	// Defense-in-depth: never serialize private-key material on the wire.
	if strings.Contains(strings.ToLower(string(payload)), "privatekey") ||
		strings.Contains(string(payload), "BEGIN PRIVATE KEY") ||
		strings.Contains(string(payload), "BEGIN EC PRIVATE KEY") ||
		strings.Contains(string(payload), "BEGIN RSA PRIVATE KEY") {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_REQUEST_INVALID: private key material must not be transmitted")
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_REQUEST_INVALID: request cannot be created")
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	response, err := t.client.Do(httpRequest)
	if err != nil {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_TRANSPORT_FAILED: enrollment request failed")
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, int64(t.maxResponseBytes)+1))
	if err != nil {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_TRANSPORT_FAILED: response could not be read")
	}
	if len(body) > t.maxResponseBytes {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_RESPONSE_TOO_LARGE: response exceeds configured bound")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_REJECTED: Core returned HTTP status %d", response.StatusCode)
	}

	var decoded enrollmentWireResponse
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_RESPONSE_INVALID: Core enrollment response is invalid")
	}
	if decoded.SensorID == "" || decoded.CertificatePEM == "" || decoded.CABundlePEM == "" {
		return EnrollmentResponse{}, fmt.Errorf("SENSOR_ENROLLMENT_RESPONSE_INVALID: Core enrollment response is incomplete")
	}
	return EnrollmentResponse{
		SensorID:       decoded.SensorID,
		CertificatePEM: []byte(decoded.CertificatePEM),
		CABundlePEM:    []byte(decoded.CABundlePEM),
	}, nil
}

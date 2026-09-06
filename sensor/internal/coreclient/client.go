package coreclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type CoreClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

func NewCoreClient(baseURL string, logger *slog.Logger) *CoreClient {
	// Custom HTTP transport with TLS skip-verify option for dev or cert support
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
	}
	return &CoreClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   5 * time.Second,
		},
		logger: logger,
	}
}

// CheckConnectivity queries Core version/health endpoint.
func (c *CoreClient) CheckConnectivity(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/version", nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("core returned status %d: %s", resp.StatusCode, string(body))
	}
	return true, nil
}

// SendBatch delivers spooled telemetry to Core.
func (c *CoreClient) SendBatch(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/telemetry", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// In dev skeleton, if Core telemetry endpoint is not yet accepting POST, 404/200 is noted
	if resp.StatusCode >= 500 {
		return fmt.Errorf("core error status %d", resp.StatusCode)
	}
	return nil
}

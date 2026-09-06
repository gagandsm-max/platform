package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"platform/agent/common/identity"
)

type CoreClient struct {
	baseURL    string
	identity   *identity.AgentIdentity
	httpClient *http.Client
	logger     *slog.Logger
}

func NewCoreClient(baseURL string, ident *identity.AgentIdentity, logger *slog.Logger) *CoreClient {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:    5,
		IdleConnTimeout: 30 * time.Second,
	}
	return &CoreClient{
		baseURL:  baseURL,
		identity: ident,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   5 * time.Second,
		},
		logger: logger,
	}
}

func (c *CoreClient) Heartbeat(ctx context.Context) error {
	payload, err := json.Marshal(map[string]any{
		"agent_id":  c.identity.AgentID,
		"hostname":  c.identity.Hostname,
		"os":        c.identity.OS,
		"arch":      c.identity.Arch,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/agent/heartbeat", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("core heartbeat error %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *CoreClient) CheckCore(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/version", nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

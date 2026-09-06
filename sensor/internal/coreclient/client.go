package coreclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type CoreClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

func NewCoreClient(baseURL, certFile, keyFile, caCertFile string, logger *slog.Logger) *CoreClient {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		ServerName:         "core",
	}

	// Load CA certificate pool if provided
	if caCertFile != "" {
		caCert, err := os.ReadFile(caCertFile)
		if err == nil {
			caPool := x509.NewCertPool()
			if caPool.AppendCertsFromPEM(caCert) {
				tlsConfig.RootCAs = caPool
				logger.Info("Loaded CA certificate pool for Core verification", "path", caCertFile)
			}
		} else {
			logger.Warn("Failed to read CA cert file, falling back to skip-verify for dev", "path", caCertFile, "error", err)
			tlsConfig.InsecureSkipVerify = true
		}
	} else {
		// In dev without explicit CA file, allow skip verify for self-signed certificates
		tlsConfig.InsecureSkipVerify = true
	}

	// Load client certificate for mTLS if provided
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			logger.Error("Failed to load mTLS client certificate keypair", "cert", certFile, "key", keyFile, "error", err)
		} else {
			tlsConfig.Certificates = []tls.Certificate{cert}
			logger.Info("Configured mTLS client certificate for Sensor -> Core transport", "cert", certFile)
		}
	}

	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
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

// Heartbeat sends authenticated mTLS heartbeat to Core.
func (c *CoreClient) Heartbeat(ctx context.Context, sensorID, sensorType string) (bool, error) {
	url := fmt.Sprintf("%s/api/v1/sensor/heartbeat", c.baseURL)
	payload := []byte(fmt.Sprintf(`{"sensor_id":"%s","type":"%s"}`, sensorID, sensorType))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("heartbeat rejected status %d: %s", resp.StatusCode, string(body))
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

	if resp.StatusCode >= 500 {
		return fmt.Errorf("core error status %d", resp.StatusCode)
	}
	return nil
}

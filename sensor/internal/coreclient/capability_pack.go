package coreclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	capabilityPackDesiredStatePath = "/api/v1/sensor/capability-packs/desired-state"
	capabilityPackAppliedStatePath = "/api/v1/sensor/capability-packs/applied-state"
	capabilityPackArtifactPathFmt  = "/api/v1/sensor/capability-packs/artifacts/%s/versions/%s"

	HeaderPackSHA256  = "X-Platform-Pack-SHA256"
	HeaderPackID      = "X-Platform-Pack-ID"
	HeaderPackVersion = "X-Platform-Pack-Version"

	// MaxCapabilityPackArtifactBytes mirrors Core CapabilityPackLoader.MAX_MANIFEST_BYTES.
	MaxCapabilityPackArtifactBytes = 256 * 1024

	CodePackStateStale       ErrorCode = "ERR_SENSOR_PACK_STATE_STALE"
	CodePackChecksumMismatch ErrorCode = "ERR_SENSOR_PACK_CHECKSUM_MISMATCH"
)

type DesiredPackEntry struct {
	PackID         string `json:"packId"`
	PackVersion    string `json:"packVersion"`
	ManifestSHA256 string `json:"manifestSha256"`
	ArtifactBytes  int    `json:"artifactBytes"`
}

type CapabilityPackDesiredState struct {
	Generation        int64              `json:"generation"`
	Checksum          string             `json:"checksum"`
	AppliedGeneration int64              `json:"appliedGeneration"`
	AppliedChecksum   string             `json:"appliedChecksum"`
	Packs             []DesiredPackEntry `json:"packs"`
}

type CapabilityPackArtifact struct {
	PackID         string
	PackVersion    string
	ManifestSHA256 string
	Bytes          []byte
}

type AppliedStateAck struct {
	Status     string `json:"status"`
	Generation int64  `json:"generation"`
	Checksum   string `json:"checksum"`
}

func (c *Client) FetchCapabilityPackDesiredState(ctx context.Context) (CapabilityPackDesiredState, error) {
	var out CapabilityPackDesiredState
	if err := c.doJSON(ctx, http.MethodGet, capabilityPackDesiredStatePath, nil, "", "", &out); err != nil {
		return CapabilityPackDesiredState{}, err
	}
	if out.Generation < 0 {
		return CapabilityPackDesiredState{}, newError(CodeProtocol, "desired-state generation is negative", nil, false)
	}
	if out.Checksum != "" && !isLowerHex64(out.Checksum) {
		return CapabilityPackDesiredState{}, newError(CodeProtocol, "desired-state checksum is not lowercase 64-hex", nil, false)
	}
	if out.AppliedGeneration < 0 || out.AppliedGeneration > out.Generation {
		return CapabilityPackDesiredState{}, newError(CodeProtocol, "Core applied generation is outside desired-state bounds", nil, false)
	}
	if out.AppliedGeneration == 0 && out.AppliedChecksum != "" {
		return CapabilityPackDesiredState{}, newError(CodeProtocol, "Core applied checksum must be empty at generation zero", nil, false)
	}
	if out.AppliedGeneration > 0 && !isLowerHex64(out.AppliedChecksum) {
		return CapabilityPackDesiredState{}, newError(CodeProtocol, "Core applied checksum is not lowercase 64-hex", nil, false)
	}
	return out, nil
}

func (c *Client) FetchCapabilityPackArtifact(ctx context.Context, packID, packVersion string, maxBytes int) (CapabilityPackArtifact, error) {
	if c == nil {
		return CapabilityPackArtifact{}, newError(CodeInvalidConfig, "Core client is nil", nil, false)
	}
	if c.initErr != nil {
		return CapabilityPackArtifact{}, c.initErr
	}
	if ctx == nil {
		return CapabilityPackArtifact{}, newError(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	if err := contextClassified(ctx); err != nil {
		return CapabilityPackArtifact{}, err
	}
	packID = strings.TrimSpace(packID)
	packVersion = strings.TrimSpace(packVersion)
	if packID == "" || packVersion == "" {
		return CapabilityPackArtifact{}, newError(CodeInvalidConfig, "packId and packVersion are required", nil, false)
	}
	if maxBytes <= 0 || maxBytes > MaxCapabilityPackArtifactBytes {
		maxBytes = MaxCapabilityPackArtifactBytes
	}

	path := fmt.Sprintf(capabilityPackArtifactPathFmt, url.PathEscape(packID), url.PathEscape(packVersion))
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(c.baseURL.Path, "/") + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return CapabilityPackArtifact{}, newError(CodeProtocol, "failed to create pack artifact request", err, false)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return CapabilityPackArtifact{}, classifyTransport(ctx, err)
	}
	defer resp.Body.Close()
	body, err := readBounded(resp.Body, maxBytes)
	if err != nil {
		return CapabilityPackArtifact{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CapabilityPackArtifact{}, classifyStatus(resp.StatusCode)
	}

	shaHdr := strings.TrimSpace(strings.ToLower(resp.Header.Get(HeaderPackSHA256)))
	idHdr := strings.TrimSpace(resp.Header.Get(HeaderPackID))
	verHdr := strings.TrimSpace(resp.Header.Get(HeaderPackVersion))
	if !isLowerHex64(shaHdr) {
		return CapabilityPackArtifact{}, newError(CodeProtocol, "Core pack SHA-256 header is missing or invalid", nil, false)
	}
	if idHdr != packID || verHdr != packVersion {
		return CapabilityPackArtifact{}, newError(CodeProtocol, "Core pack identity headers do not match request", nil, false)
	}
	sum := sha256.Sum256(body)
	computed := hex.EncodeToString(sum[:])
	if computed != shaHdr {
		return CapabilityPackArtifact{}, newError(CodeProtocol, "pack artifact SHA-256 mismatch", nil, false)
	}
	// Preserve exact stored bytes (no re-encode).
	out := make([]byte, len(body))
	copy(out, body)
	return CapabilityPackArtifact{
		PackID:         idHdr,
		PackVersion:    verHdr,
		ManifestSHA256: shaHdr,
		Bytes:          out,
	}, nil
}

func (c *Client) AckCapabilityPackAppliedState(ctx context.Context, generation int64, checksum string) (AppliedStateAck, error) {
	payload := struct {
		Generation int64   `json:"generation"`
		Checksum   *string `json:"checksum"`
	}{Generation: generation}
	if checksum != "" {
		payload.Checksum = &checksum
	}
	var out AppliedStateAck
	if err := c.doJSONWithConflictCodes(ctx, http.MethodPut, capabilityPackAppliedStatePath, payload, &out); err != nil {
		return AppliedStateAck{}, err
	}
	return out, nil
}

func (c *Client) doJSONWithConflictCodes(ctx context.Context, method, path string, body any, out any) error {
	if c == nil {
		return newError(CodeInvalidConfig, "Core client is nil", nil, false)
	}
	if c.initErr != nil {
		return c.initErr
	}
	if ctx == nil {
		return newError(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	if err := contextClassified(ctx); err != nil {
		return err
	}
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(c.baseURL.Path, "/") + path

	payload, err := json.Marshal(body)
	if err != nil {
		return newError(CodeProtocol, "failed to encode Core request", err, false)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return newError(CodeProtocol, "failed to create Core request", err, false)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return classifyTransport(ctx, err)
	}
	defer resp.Body.Close()
	responseBody, err := readBounded(resp.Body, c.maxResponseBytes)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusConflict {
		var envelope struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(responseBody, &envelope)
		switch envelope.Code {
		case string(CodePackStateStale):
			return newError(CodePackStateStale, "Core rejected applied-state as stale", nil, false)
		case string(CodePackChecksumMismatch):
			return newError(CodePackChecksumMismatch, "Core rejected applied-state checksum", nil, false)
		}
		return classifyStatus(resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return classifyStatus(resp.StatusCode)
	}
	if out == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return newError(CodeProtocol, "Core response is not valid JSON", err, false)
	}
	return nil
}

func isLowerHex64(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, c := range v {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

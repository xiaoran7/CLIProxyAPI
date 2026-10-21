package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	MaxResponseBytes = 8 * 1024 * 1024 // 8 MB
)

// CpaClient handles direct loopback HTTP calls to CPA's management endpoints.
type CpaClient struct {
	client     *http.Client
	host       string
	port       int
	mgmtSecret string
}

// CpaResponse represents the outcome of a management API invocation.
type CpaResponse struct {
	StatusCode  int
	Bytes       []byte
	ContentType string
	Truncated   bool
	ETag        string
}

// NewCpaClient creates a new loopback management client.
func NewCpaClient(host string, port int, mgmtSecret string) *CpaClient {
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}
	if port <= 0 {
		port = 8000
	}
	return &CpaClient{
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
		host:       host,
		port:       port,
		mgmtSecret: strings.TrimSpace(mgmtSecret),
	}
}

// Request executes an HTTP call against CPA's management endpoints with size protection.
func (c *CpaClient) Request(ctx context.Context, method, path string, body []byte, contentType, apiVersion string) (*CpaResponse, error) {
	if contentType == "" {
		contentType = "application/json"
	}
	if apiVersion == "" {
		apiVersion = "v0"
	}

	fullPath := path
	if !strings.HasPrefix(path, "/v") {
		fullPath = fmt.Sprintf("/%s/management%s", apiVersion, path)
	}

	targetURL := fmt.Sprintf("http://%s:%d%s", c.host, c.port, fullPath)

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create cpa request failed: %w", err)
	}

	if c.mgmtSecret != "" {
		req.Header.Set("Authorization", "Bearer "+c.mgmtSecret)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cpa request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	isLogs := strings.Contains(fullPath, "logs")
	buf := &bytes.Buffer{}
	truncated := false

	// Stream read with 8MB cap
	lr := io.LimitReader(resp.Body, int64(MaxResponseBytes)+1)
	readBytes, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("read cpa response body failed: %w", err)
	}

	if len(readBytes) > MaxResponseBytes {
		truncated = true
		if isLogs {
			buf.Write(readBytes[:MaxResponseBytes])
		} else {
			return nil, fmt.Errorf("CPA response exceeded 8MB limit (%d bytes)", len(readBytes))
		}
	} else {
		buf.Write(readBytes)
	}

	outBytes := buf.Bytes()
	etag := ""
	if len(outBytes) > 0 {
		h := sha256.Sum256(outBytes)
		etag = hex.EncodeToString(h[:])
	}

	return &CpaResponse{
		StatusCode:  resp.StatusCode,
		Bytes:       outBytes,
		ContentType: resp.Header.Get("Content-Type"),
		Truncated:   truncated,
		ETag:        etag,
	}, nil
}

// RequestJSON issues a management request and decodes the JSON result into target.
func (c *CpaClient) RequestJSON(ctx context.Context, method, path string, body []byte, apiVersion string, target interface{}) error {
	resp, err := c.Request(ctx, method, path, body, "application/json", apiVersion)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("CPA management returned %d: %s", resp.StatusCode, string(resp.Bytes))
	}
	if target != nil && len(resp.Bytes) > 0 {
		if err := json.Unmarshal(resp.Bytes, target); err != nil {
			return fmt.Errorf("unmarshal CPA response failed: %w", err)
		}
	}
	return nil
}

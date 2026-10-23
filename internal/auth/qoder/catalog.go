// Package qoder provides authentication, COSY request signing, and token management
// for Qoder Global and Qoder Mainland China APIs.
package qoder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

// FetchUpstreamModels fetches the dynamic model catalog from the Qoder upstream gateway.
// It signs the request using COSY and the account credentials.
func FetchUpstreamModels(ctx context.Context, httpClient *http.Client, auth *cliproxyauth.Auth) ([]byte, error) {
	if auth == nil {
		return nil, fmt.Errorf("qoder: auth is nil")
	}

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	realm := ResolveRealm(auth)
	baseURL := ResolveBaseURL(realm)
	openAPIBase := ResolveOpenAPIBaseURL(realm)
	if auth.Attributes != nil {
		if bu := strings.TrimSpace(auth.Attributes["base_url"]); bu != "" {
			baseURL = strings.TrimRight(bu, "/")
		}
		if ob := strings.TrimSpace(auth.Attributes["openapi_base"]); ob != "" {
			openAPIBase = strings.TrimRight(ob, "/")
		}
	}

	jobToken, errJob := EnsureValidJobToken(ctx, client, auth)
	if errJob != nil {
		// If job token cannot be obtained, attempt using raw access token
		jobToken = AccessToken(auth)
	}

	// 1. Attempt gateway model list with COSY signing: /algo/api/v2/model/list?Encode=1 (POST then GET)
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		gatewayURL := baseURL + ModelListPath + "?Encode=1"
		reqGateway, errReq := http.NewRequestWithContext(ctx, method, gatewayURL, bytes.NewReader([]byte("{}")))
		if errReq == nil {
			reqGateway.Header.Set("Content-Type", "application/json")
			reqGateway.Header.Set("Accept", "application/json")
			if errSign := SignRequest(reqGateway, auth, jobToken, "{}"); errSign == nil {
				resp, errDo := client.Do(reqGateway)
				if errDo == nil {
					defer func() { _ = resp.Body.Close() }()
					if resp.StatusCode == http.StatusOK {
						body, errRead := io.ReadAll(resp.Body)
						if errRead == nil && len(body) > 0 {
							return body, nil
						}
					}
				}
			}
		}
	}

	// 2. Fallback to OpenAPI model list endpoint: /api/v1/cloud/models
	openapiURL := openAPIBase + OpenAPIModelListPath
	reqOpenAPI, errReq2 := http.NewRequestWithContext(ctx, http.MethodGet, openapiURL, nil)
	if errReq2 != nil {
		return nil, fmt.Errorf("qoder: create openapi model list request: %w", errReq2)
	}

	token := AccessToken(auth)
	if token != "" {
		reqOpenAPI.Header.Set("Authorization", "Bearer "+token)
	}
	reqOpenAPI.Header.Set("Accept", "application/json")
	reqOpenAPI.Header.Set("User-Agent", DefaultUserAgent)

	resp2, errDo2 := client.Do(reqOpenAPI)
	if errDo2 != nil {
		return nil, fmt.Errorf("qoder: fetch openapi models failed: %w", errDo2)
	}
	defer func() {
		_ = resp2.Body.Close()
	}()

	if resp2.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qoder: model list returned status %d", resp2.StatusCode)
	}

	body, errRead2 := io.ReadAll(resp2.Body)
	if errRead2 != nil {
		return nil, fmt.Errorf("qoder: read openapi models response: %w", errRead2)
	}

	return body, nil
}

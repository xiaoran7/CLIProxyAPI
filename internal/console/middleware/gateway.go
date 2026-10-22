package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// ShouldSkipGateway determines whether a path bypasses Console gateway processing
// (accounting, key authentication, quota limiting, and model guarding).
// This guarantees that native CPA management, OAuth redirects, and system health
// checks remain unhindered and are not subjected to proxy key checks or token metering.
func ShouldSkipGateway(path string) bool {
	if strings.HasPrefix(path, "/console") ||
		strings.HasPrefix(path, "/admin") ||
		strings.HasPrefix(path, "/member") ||
		strings.HasPrefix(path, "/v0/management") ||
		strings.HasPrefix(path, "/v8/management") ||
		strings.HasPrefix(path, "/api/member") ||
		strings.HasPrefix(path, "/anthropic/") ||
		strings.HasPrefix(path, "/codex/") ||
		strings.HasPrefix(path, "/antigravity/") ||
		strings.HasPrefix(path, "/devin/") ||
		strings.HasPrefix(path, "/oauth") ||
		strings.HasPrefix(path, "/auth") ||
		path == "/callback" ||
		path == "/management.html" ||
		strings.HasPrefix(path, "/management/") ||
		path == "/healthz" ||
		path == "/readyz" ||
		path == "/metrics" ||
		path == "/api/status/quota" ||
		strings.HasPrefix(path, "/api/status/quota") ||
		path == "/favicon.ico" ||
		path == "/" {
		return true
	}
	return false
}

// IsDiscoveryOrUtilityRequest checks if a request is a discovery, preflight, or utility call
// (such as models listing, CORS preflight, or health probe) rather than a model inference request.
func IsDiscoveryOrUtilityRequest(method, path string) bool {
	// 1. CORS Preflight & Head Probes
	if method == "OPTIONS" || method == "HEAD" {
		return true
	}

	// 2. Models Discovery & Metadata (OpenAI, Gemini, Claude, Codex)
	if method == "GET" {
		clean := strings.TrimRight(path, "/")
		if clean == "/models" ||
			strings.HasSuffix(clean, "/models") ||
			strings.Contains(clean, "/models/") ||
			strings.HasPrefix(clean, "/v1/videos") ||
			strings.HasPrefix(clean, "/openai/v1/videos") {
			return true
		}
	}

	return false
}

// Gateway returns the complete Gin middleware handlers for Console in execution order:
// 1. AccountingMiddleware: sets up response interception, TTFT timing, and transactional outbox flush.
// 2. KeyAuthMiddleware: extracts and authenticates multi-tenant API keys.
// 3. QuotaLimiterMiddleware: enforces 60s sliding window RPM limits and daily token quotas.
// 4. ModelGuardMiddleware: enforces model whitelists and filters /v1/models discovery responses.
func Gateway(eng *engine.Engine) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		AccountingMiddleware(eng),
		KeyAuthMiddleware(eng),
		QuotaLimiterMiddleware(eng),
		ModelGuardMiddleware(eng),
	}
}
